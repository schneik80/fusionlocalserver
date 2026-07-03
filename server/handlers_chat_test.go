package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
	"github.com/schneik80/fusionlocalserver/auth"
	"github.com/schneik80/fusionlocalserver/chat"
	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

const chatTestProject = "urn:project:1"

// newChatTestServer builds a Server with a real chat store over a TempDir
// and an authorizer pointed at a fake APS roster: one VIEWER, one EDITOR,
// one MANAGER, and an extra EDITOR ("member") for private-channel ACLs.
func newChatTestServer(t *testing.T) *Server {
	t.Helper()
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		row := func(id, email, role string) map[string]any {
			return map[string]any{
				"role": role, "status": "ACTIVE",
				"user": map[string]any{"id": id, "userName": id, "firstName": "", "lastName": "", "email": email},
			}
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"project": map[string]any{
				"folderLevelProjectMembers": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results": []map[string]any{
						row("u-viewer", "viewer@x.io", "VIEWER"),
						row("u-editor", "editor@x.io", "EDITOR"),
						row("u-member", "member@x.io", "EDITOR"),
						row("u-manager", "manager@x.io", "MANAGER"),
					},
				},
			},
		}}
	})
	restore := api.SetGraphqlEndpointForTesting(srv.URL)
	t.Cleanup(restore)

	store, err := chat.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)

	return &Server{
		logger:     quietLogger(),
		clientID:   "test-client",
		sessions:   NewSessionStore(sessionIdleTTL, sessionAbsTTL, quietLogger()),
		pending:    NewPendingStore(pendingTTL),
		chat:       store,
		chatAuthz:  chat.NewAuthorizer(),
		chatMsgLim: chat.NewLimiter(2, 5),
		chatOpLim:  chat.NewLimiter(10.0/60.0, 10),
	}
}

// login creates a session for the given identity and returns its cookie.
func login(t *testing.T, s *Server, sub, name, email string) *http.Cookie {
	t.Helper()
	sess, err := s.sessions.Create(
		&auth.TokenData{AccessToken: "tok-" + sub, ExpiresAt: time.Now().Add(time.Hour)},
		auth.UserProfile{Sub: sub, Name: name, Email: email},
	)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: sess.ID}
}

// chatDo issues a request against the running test server and decodes the
// JSON reply into out (unless out is nil), returning the status code.
func chatDo(t *testing.T, base, method, path string, cookie *http.Cookie, body any, out any) int {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+path, buf)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: decoding %q: %v", method, path, data, err)
		}
	}
	return resp.StatusCode
}

func chatURL(path string, kv ...string) string {
	q := "projectId=" + chatTestProject
	for i := 0; i+1 < len(kv); i += 2 {
		q += "&" + kv[i] + "=" + kv[i+1]
	}
	return path + "?" + q
}

func TestChat_RequiresSession(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)

	if code := chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), nil, nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", code)
	}
}

func TestChat_RootChannelAndRoleGates(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	viewer := login(t, s, "u-viewer", "Vera Viewer", "viewer@x.io")
	editor := login(t, s, "u-editor", "Ed Editor", "editor@x.io")

	// First touch creates the root channel; viewers can read it.
	var list ChannelListDTO
	if code := chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), viewer, nil, &list); code != http.StatusOK {
		t.Fatalf("viewer list status = %d", code)
	}
	if len(list.Channels) != 1 || !list.Channels[0].IsRoot || list.Channels[0].Name != "general" {
		t.Fatalf("root channel wrong: %+v", list.Channels)
	}
	root := list.Channels[0].ID

	// Viewer cannot post…
	msgIn := map[string]any{"body": "hi", "clientMsgId": "cm-viewer"}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), viewer, msgIn, nil); code != http.StatusForbidden {
		t.Fatalf("viewer post status = %d, want 403", code)
	}
	// …and cannot create channels.
	chIn := map[string]any{"name": "nope"}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/channels"), viewer, chIn, nil); code != http.StatusForbidden {
		t.Fatalf("viewer create-channel status = %d, want 403", code)
	}

	// Editor posts fine; a thread reply to it works; a reply-to-reply 400s.
	var posted MessageDTO
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor,
		map[string]any{"body": "top", "clientMsgId": "cm-top"}, &posted); code != http.StatusCreated {
		t.Fatalf("editor post status = %d, want 201", code)
	}
	var reply MessageDTO
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor,
		map[string]any{"body": "reply", "clientMsgId": "cm-reply", "threadRootSeq": posted.Seq}, &reply); code != http.StatusCreated {
		t.Fatalf("reply status = %d, want 201", code)
	}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor,
		map[string]any{"body": "nested", "clientMsgId": "cm-nested", "threadRootSeq": reply.Seq}, nil); code != http.StatusBadRequest {
		t.Fatalf("reply-to-reply status = %d, want 400", code)
	}
}

func TestChat_IdempotentCreateOverHTTP(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed", "editor@x.io")

	var list ChannelListDTO
	chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), editor, nil, &list)
	root := list.Channels[0].ID

	in := map[string]any{"body": "once", "clientMsgId": "cm-same"}
	var first, second MessageDTO
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor, in, &first); code != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", code)
	}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor, in, &second); code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", code)
	}
	if second.Seq != first.Seq {
		t.Fatalf("replay minted a new message: %d vs %d", second.Seq, first.Seq)
	}
	var msgs MessageListDTO
	chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/messages", "channelId", root), editor, nil, &msgs)
	if len(msgs.Messages) != 1 {
		t.Fatalf("timeline has %d messages, want 1", len(msgs.Messages))
	}
}

func TestChat_BodyLimits(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed", "editor@x.io")

	var list ChannelListDTO
	chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), editor, nil, &list)
	root := list.Channels[0].ID

	// Over the 4000-rune message cap but under the transport cap → 400 from the store.
	over := map[string]any{"body": strings.Repeat("x", 4001), "clientMsgId": "cm-big"}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor, over, nil); code != http.StatusBadRequest {
		t.Fatalf("over-cap body status = %d, want 400", code)
	}
	// Over the 64 KiB transport cap → decode fails → 400.
	huge := map[string]any{"body": strings.Repeat("x", 70_000), "clientMsgId": "cm-huge"}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor, huge, nil); code != http.StatusBadRequest {
		t.Fatalf("oversize transport body status = %d, want 400", code)
	}
}

func TestChat_MessageRateLimit(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed", "editor@x.io")

	var list ChannelListDTO
	chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), editor, nil, &list)
	root := list.Channels[0].ID

	got429 := false
	for i := 0; i < 7; i++ {
		in := map[string]any{"body": "spam", "clientMsgId": fmt.Sprintf("cm-%d", i)}
		if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor, in, nil); code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("burst of 7 posts never hit the rate limit (burst is 5)")
	}
}

func TestChat_PrivateChannelHiddenFromNonMembers(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	manager := login(t, s, "u-manager", "Mia Manager", "manager@x.io")
	editor := login(t, s, "u-editor", "Ed", "editor@x.io")
	member := login(t, s, "u-member", "Mel Member", "member@x.io")

	var priv ChannelDTO
	in := map[string]any{"name": "secret", "isPrivate": true, "memberIds": []string{"u-member"}}
	if code := chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/channels"), manager, in, &priv); code != http.StatusCreated {
		t.Fatalf("create private status = %d, want 201", code)
	}

	// Not in the sidebar for a non-member…
	var list ChannelListDTO
	chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), editor, nil, &list)
	for _, ch := range list.Channels {
		if ch.ID == priv.ID {
			t.Fatal("private channel listed for a non-member")
		}
	}
	// …and 404 (not 403) when addressed directly.
	if code := chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/messages", "channelId", priv.ID), editor, nil, nil); code != http.StatusNotFound {
		t.Fatalf("non-member direct fetch status = %d, want 404", code)
	}

	// ACL member and project manager both see it.
	for name, cookie := range map[string]*http.Cookie{"member": member, "manager": manager} {
		var l ChannelListDTO
		chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), cookie, nil, &l)
		found := false
		for _, ch := range l.Channels {
			found = found || ch.ID == priv.ID
		}
		if !found {
			t.Fatalf("%s cannot see the private channel", name)
		}
	}
}

func TestChat_EditAndModerationRules(t *testing.T) {
	s := newChatTestServer(t)
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed", "editor@x.io")
	other := login(t, s, "u-member", "Mel", "member@x.io")
	manager := login(t, s, "u-manager", "Mia", "manager@x.io")

	var list ChannelListDTO
	chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), editor, nil, &list)
	root := list.Channels[0].ID
	var posted MessageDTO
	chatDo(t, ts.URL, http.MethodPost, chatURL("/api/chat/messages", "channelId", root), editor,
		map[string]any{"body": "mine", "clientMsgId": "cm-1"}, &posted)
	seq := fmt.Sprintf("%d", posted.Seq)

	// Another editor can neither edit nor delete someone else's message.
	if code := chatDo(t, ts.URL, http.MethodPatch, chatURL("/api/chat/messages", "channelId", root, "seq", seq), other,
		map[string]any{"body": "hijacked"}, nil); code != http.StatusForbidden {
		t.Fatalf("foreign edit status = %d, want 403", code)
	}
	if code := chatDo(t, ts.URL, http.MethodDelete, chatURL("/api/chat/messages", "channelId", root, "seq", seq), other, nil, nil); code != http.StatusForbidden {
		t.Fatalf("foreign delete status = %d, want 403", code)
	}
	// The author edits their own.
	var edited MessageDTO
	if code := chatDo(t, ts.URL, http.MethodPatch, chatURL("/api/chat/messages", "channelId", root, "seq", seq), editor,
		map[string]any{"body": "mine, fixed"}, &edited); code != http.StatusOK || edited.EditedAt == "" {
		t.Fatalf("own edit status = %d, editedAt = %q", code, edited.EditedAt)
	}
	// A manager deletes anyone's.
	var deleted MessageDTO
	if code := chatDo(t, ts.URL, http.MethodDelete, chatURL("/api/chat/messages", "channelId", root, "seq", seq), manager, nil, &deleted); code != http.StatusOK || !deleted.Deleted {
		t.Fatalf("moderator delete status = %d, deleted = %v", code, deleted.Deleted)
	}
}

func TestChat_UnavailableStore(t *testing.T) {
	s := newChatTestServer(t)
	s.chat = nil // config dir unavailable at startup
	ts := httptest.NewServer(s.routes())
	t.Cleanup(ts.Close)
	editor := login(t, s, "u-editor", "Ed", "editor@x.io")

	if code := chatDo(t, ts.URL, http.MethodGet, chatURL("/api/chat/channels"), editor, nil, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("nil store status = %d, want 503", code)
	}
}
