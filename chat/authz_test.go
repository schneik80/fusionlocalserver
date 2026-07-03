package chat

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

// rosterAuthorizer returns an Authorizer whose roster fetch goes over HTTP
// to a testutil.GraphQLServer serving the given members, plus a counter of
// upstream hits and a movable clock.
func rosterAuthorizer(t *testing.T, members []map[string]any) (*Authorizer, *atomic.Int64, *time.Time) {
	t.Helper()
	var hits atomic.Int64
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		hits.Add(1)
		return testutil.GraphQLResponse{Data: map[string]any{
			"project": map[string]any{
				"folderLevelProjectMembers": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results":    members,
				},
			},
		}}
	})
	restore := api.SetGraphqlEndpointForTesting(srv.URL)
	t.Cleanup(restore)

	clock := time.Now()
	a := NewAuthorizer()
	a.now = func() time.Time { return clock }
	return a, &hits, &clock
}

func member(id, email, role, status string) map[string]any {
	return map[string]any{
		"role":   role,
		"status": status,
		"user": map[string]any{
			"id": id, "userName": "user-" + id, "firstName": "", "lastName": "", "email": email,
		},
	}
}

func TestCapabilityMapping(t *testing.T) {
	a, _, _ := rosterAuthorizer(t, []map[string]any{
		member("u-viewer", "v@x.com", "VIEWER", "ACTIVE"),
		member("u-reader", "r@x.com", "READER", "ACTIVE"),
		member("u-editor", "e@x.com", "EDITOR", "ACTIVE"),
		member("u-manager", "m@x.com", "MANAGER", "ACTIVE"),
		member("u-admin", "a@x.com", "ADMINISTRATOR", "ACTIVE"),
		member("u-mystery", "q@x.com", "OVERLORD", "ACTIVE"), // future role → deny
		member("u-pending", "p@x.com", "EDITOR", "PENDING"),
	})
	ctx := context.Background()

	caps := []Capability{CapRead, CapPost, CapReact, CapEditOwn, CapCreateChannel, CapModerate}
	want := map[string][]bool{ //            read  post  react editOwn create moderate
		"u-viewer":  {true, false, false, false, false, false},
		"u-reader":  {true, false, false, false, false, false},
		"u-editor":  {true, true, true, true, true, false},
		"u-manager": {true, true, true, true, true, true},
		"u-admin":   {true, true, true, true, true, true},
		"u-mystery": {false, false, false, false, false, false}, // listed, unknown role → deny
		"u-pending": {false, false, false, false, false, false}, // listed but not ACTIVE → deny
		// Not individually listed, but the roster fetch (their own token)
		// succeeded → access is via a project group → contributor, not
		// moderator. This is the group-only member the phase-3 authorizer
		// wrongly locked out entirely.
		"u-grouponly": {true, true, true, true, true, false},
	}
	for user, expect := range want {
		for i, cap := range caps {
			got, err := a.Can(ctx, "tok", Identity{UserID: user}, "proj-1", cap)
			if err != nil {
				t.Fatalf("%s cap %d: %v", user, cap, err)
			}
			if got != expect[i] {
				t.Errorf("%s cap %d = %v, want %v", user, cap, got, expect[i])
			}
		}
	}
}

func TestRoleMatch_EmailFallbackAndCase(t *testing.T) {
	a, _, _ := rosterAuthorizer(t, []map[string]any{
		member("mdm-id-1", "Person@Example.COM", "editor", "active"), // lowercase wire values
	})
	ctx := context.Background()

	// No sub (profile fetch failed at login) → email match, case-insensitive.
	ok, err := a.Can(ctx, "tok", Identity{Email: "person@example.com"}, "p", CapPost)
	if err != nil || !ok {
		t.Fatalf("email fallback: ok=%v err=%v", ok, err)
	}
	// Role/status casing must not matter.
	ok, _ = a.Can(ctx, "tok", Identity{UserID: "mdm-id-1"}, "p", CapPost)
	if !ok {
		t.Fatal("lowercase role/status string denied")
	}
}

func TestRoleCache_TTLAndSingleflight(t *testing.T) {
	a, hits, clock := rosterAuthorizer(t, []map[string]any{
		member("u1", "u1@x.com", "EDITOR", "ACTIVE"),
	})
	ctx := context.Background()
	id := Identity{UserID: "u1"}

	for i := 0; i < 5; i++ {
		if ok, err := a.Can(ctx, "tok", id, "p", CapPost); !ok || err != nil {
			t.Fatalf("call %d: ok=%v err=%v", i, ok, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("roster fetched %d times within TTL, want 1", hits.Load())
	}

	*clock = clock.Add(61 * time.Second) // past positive TTL
	a.Can(ctx, "tok", id, "p", CapPost)
	if hits.Load() != 2 {
		t.Fatalf("roster fetched %d times after TTL expiry, want 2", hits.Load())
	}

	// Negative caching: a stranger is re-checked only after negTTL.
	stranger := Identity{UserID: "nobody"}
	a.Can(ctx, "tok", stranger, "p", CapRead)
	a.Can(ctx, "tok", stranger, "p", CapRead)
	if hits.Load() != 3 {
		t.Fatalf("negative result not cached: %d fetches", hits.Load())
	}
	*clock = clock.Add(16 * time.Second) // past negTTL
	a.Can(ctx, "tok", stranger, "p", CapRead)
	if hits.Load() != 4 {
		t.Fatalf("negative cache never expired: %d fetches", hits.Load())
	}
}

func TestRoleFetchError_NotCached(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if failing.Load() { // persistent outage (the client retries 5xx internally)
			return testutil.GraphQLResponse{Status: 502, Errors: []string{"upstream sad"}}
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"project": map[string]any{
				"folderLevelProjectMembers": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results":    []map[string]any{member("u1", "u1@x.com", "EDITOR", "ACTIVE")},
				},
			},
		}}
	})
	restore := api.SetGraphqlEndpointForTesting(srv.URL)
	t.Cleanup(restore)

	a := NewAuthorizer()
	ctx := context.Background()
	if _, err := a.Can(ctx, "tok", Identity{UserID: "u1"}, "p", CapPost); err == nil {
		t.Fatal("first call should surface the fetch error")
	}
	failing.Store(false)
	// The error must not have been cached as a deny.
	ok, err := a.Can(ctx, "tok", Identity{UserID: "u1"}, "p", CapPost)
	if err != nil || !ok {
		t.Fatalf("recovery call: ok=%v err=%v", ok, err)
	}
}

func TestCanAccessChannel_TwoLayer(t *testing.T) {
	a, _, _ := rosterAuthorizer(t, []map[string]any{
		member("u-editor", "e@x.com", "EDITOR", "ACTIVE"),
		member("u-member", "m@x.com", "EDITOR", "ACTIVE"),
		member("u-viewer", "v@x.com", "VIEWER", "ACTIVE"),
		member("u-admin", "a@x.com", "ADMINISTRATOR", "ACTIVE"),
	})
	ctx := context.Background()

	public := Channel{ID: "c1", Name: "general"}
	private := Channel{ID: "c2", Name: "secret", IsPrivate: true,
		Members: []ChannelMember{{UserID: "u-member", Role: "owner"}}}

	cases := []struct {
		name string
		id   Identity
		ch   Channel
		want bool
	}{
		{"public: any project role passes layer 1", Identity{UserID: "u-viewer"}, public, true},
		{"public: group-only member passes layer 1", Identity{UserID: "u-grouponly"}, public, true},
		{"private: editor not in ACL denied", Identity{UserID: "u-editor"}, private, false},
		{"private: ACL member allowed", Identity{UserID: "u-member"}, private, true},
		{"private: project admin allowed (policy knob)", Identity{UserID: "u-admin"}, private, true},
		{"private: group-only non-member denied (not a moderator)", Identity{UserID: "u-grouponly"}, private, false},
	}
	for _, c := range cases {
		got, err := a.CanAccessChannel(ctx, "tok", c.id, "p", c.ch)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsActiveMember_Strict(t *testing.T) {
	a, _, _ := rosterAuthorizer(t, []map[string]any{
		member("u-editor", "e@x.com", "EDITOR", "ACTIVE"),
		member("u-pending", "p@x.com", "EDITOR", "PENDING"),
	})
	ctx := context.Background()

	// Unlike Can, IsActiveMember never applies the group-derived fallback:
	// only a listed ACTIVE individual counts, so a group-only invitee can't
	// be confirmed (and thus can't be added to a private channel's ACL).
	cases := []struct {
		id   Identity
		want bool
	}{
		{Identity{UserID: "u-editor"}, true},     // listed & ACTIVE
		{Identity{UserID: "u-pending"}, false},   // listed but PENDING
		{Identity{UserID: "u-grouponly"}, false}, // not individually listed
	}
	for _, c := range cases {
		got, err := a.IsActiveMember(ctx, "tok", c.id, "p")
		if err != nil {
			t.Fatalf("%s: %v", c.id.UserID, err)
		}
		if got != c.want {
			t.Errorf("IsActiveMember(%s) = %v, want %v", c.id.UserID, got, c.want)
		}
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(2, 5) // 2/s, burst 5
	clock := time.Now()
	l.now = func() time.Time { return clock }

	for i := 0; i < 5; i++ {
		if !l.Allow("sess-1") {
			t.Fatalf("burst call %d denied", i)
		}
	}
	if l.Allow("sess-1") {
		t.Fatal("6th call within burst allowed")
	}
	if !l.Allow("sess-2") {
		t.Fatal("independent key throttled")
	}
	clock = clock.Add(time.Second) // +2 tokens
	if !l.Allow("sess-1") || !l.Allow("sess-1") {
		t.Fatal("refill after 1s should grant 2")
	}
	if l.Allow("sess-1") {
		t.Fatal("third call after 1s refill allowed")
	}
}

func TestRoleMatch_HelperTable(t *testing.T) {
	m := api.Member{UserID: "id-1", Email: "A@B.com"}
	cases := []struct {
		id   Identity
		want bool
	}{
		{Identity{UserID: "id-1"}, true},
		{Identity{UserID: "id-2"}, false},
		{Identity{Email: "a@b.COM"}, true},
		{Identity{}, false},
	}
	for _, c := range cases {
		if got := matchesMember(m, c.id); got != c.want {
			t.Errorf("matchesMember(%+v) = %v, want %v", c.id, got, c.want)
		}
	}
	if !strings.EqualFold("EDITOR", "editor") {
		t.Fatal("sanity")
	}
}
