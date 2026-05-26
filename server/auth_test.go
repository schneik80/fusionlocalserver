package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
)

func newAuthTestServer() *Server {
	return &Server{
		logger:   quietLogger(),
		clientID: "test-client",
		sessions: NewSessionStore(sessionIdleTTL, sessionAbsTTL, quietLogger()),
		pending:  NewPendingStore(pendingTTL),
	}
}

func TestRequireAuth_NoCookie(t *testing.T) {
	s := newAuthTestServer()
	h := s.requireAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next handler must not run without a session")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/hubs", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuth_UnknownSession(t *testing.T) {
	s := newAuthTestServer()
	h := s.requireAuth(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("next handler must not run for an unknown session")
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/hubs", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "bogus"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuth_InjectsToken(t *testing.T) {
	s := newAuthTestServer()
	sess, _ := s.sessions.Create(
		&auth.TokenData{AccessToken: "tok-123", ExpiresAt: time.Now().Add(time.Hour)},
		auth.UserProfile{},
	)

	var gotTok string
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTok, _ = tokenFromCtx(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/hubs", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotTok != "tok-123" {
		t.Errorf("token in context = %q, want tok-123", gotTok)
	}
}

func TestHandleAuthMe(t *testing.T) {
	s := newAuthTestServer()

	rec := httptest.NewRecorder()
	s.handleAuthMe(rec, httptest.NewRequest(http.MethodGet, "/api/auth/me", nil))
	if !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Errorf("unauthenticated me body = %q", rec.Body.String())
	}

	sess, _ := s.sessions.Create(
		&auth.TokenData{AccessToken: "AT", ExpiresAt: time.Now().Add(time.Hour)},
		auth.UserProfile{Name: "Ada", Email: "ada@x.io"},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	rec = httptest.NewRecorder()
	s.handleAuthMe(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `"authenticated":true`) || !strings.Contains(body, "ada@x.io") {
		t.Errorf("authenticated me body = %q", body)
	}
}

func TestHandleAuthLogin_RedirectAndPending(t *testing.T) {
	s := newAuthTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://host.lan:8080/api/auth/login", nil)
	rec := httptest.NewRecorder()
	s.handleAuthLogin(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	state := u.Query().Get("state")
	if state == "" {
		t.Fatal("authorize URL is missing the state param")
	}
	if got, want := u.Query().Get("redirect_uri"), "http://host.lan:8080/api/auth/callback"; got != want {
		t.Errorf("redirect_uri = %q, want %q", got, want)
	}

	var pendingCookie string
	for _, c := range rec.Result().Cookies() {
		if c.Name == pendingCookieName {
			pendingCookie = c.Value
		}
	}
	if pendingCookie != state {
		t.Errorf("pending cookie = %q, want it to equal state %q", pendingCookie, state)
	}
	if _, ok := s.pending.Take(state); !ok {
		t.Error("pending store has no entry for the issued state")
	}
}

func TestHandleAuthCallback_ErrorParam(t *testing.T) {
	s := newAuthTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?error=access_denied&error_description=nope", nil)
	rec := httptest.NewRecorder()
	s.handleAuthCallback(rec, req)
	assertAuthErrorRedirect(t, rec, "access_denied")
}

func TestHandleAuthCallback_StateMismatch(t *testing.T) {
	s := newAuthTestServer()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=c&state=abc", nil)
	req.AddCookie(&http.Cookie{Name: pendingCookieName, Value: "different"})
	rec := httptest.NewRecorder()
	s.handleAuthCallback(rec, req)
	assertAuthErrorRedirect(t, rec, "state_mismatch")
}

func TestHandleAuthCallback_HappyPath(t *testing.T) {
	s := newAuthTestServer()

	const state = "the-state"
	const redirectURI = "http://h/api/auth/callback"
	s.pending.Put(state, pendingEntry{verifier: "v", redirectURI: redirectURI, createdAt: time.Now()})

	prevEx, prevUI := authExchange, authUserInfo
	t.Cleanup(func() { authExchange, authUserInfo = prevEx, prevUI })
	authExchange = func(ctx context.Context, id, secret, code, verifier, ru string) (*auth.TokenData, error) {
		if code != "the-code" || verifier != "v" || ru != redirectURI {
			t.Errorf("exchange args: code=%q verifier=%q redirect=%q", code, verifier, ru)
		}
		return &auth.TokenData{AccessToken: "AT", RefreshToken: "RT", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}
	authUserInfo = func(context.Context, string) (auth.UserProfile, error) {
		return auth.UserProfile{Name: "Grace", Email: "grace@x.io"}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=the-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: pendingCookieName, Value: state})
	rec := httptest.NewRecorder()
	s.handleAuthCallback(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("callback success: status=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}

	var sid string
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			sid = c.Value
		}
	}
	if sid == "" {
		t.Fatal("no session cookie set on success")
	}
	sess, ok := s.sessions.Get(sid)
	if !ok {
		t.Fatal("session not found after a successful callback")
	}
	if sess.Profile.Email != "grace@x.io" {
		t.Errorf("session profile = %+v", sess.Profile)
	}
	if _, ok := s.pending.Take(state); ok {
		t.Error("pending entry was not consumed by the callback")
	}
}

func TestHandleAuthLogout(t *testing.T) {
	s := newAuthTestServer()
	sess, _ := s.sessions.Create(
		&auth.TokenData{AccessToken: "AT", ExpiresAt: time.Now().Add(time.Hour)},
		auth.UserProfile{},
	)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sess.ID})
	rec := httptest.NewRecorder()
	s.handleAuthLogout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if _, ok := s.sessions.Get(sess.ID); ok {
		t.Error("session was not deleted on logout")
	}
}

// TestSessionToken_RefreshesExactlyOnce verifies the per-session refresh lock:
// many concurrent requests on one expired session trigger a single refresh
// (APS rotates the refresh token, so a double refresh would brick the session).
func TestSessionToken_RefreshesExactlyOnce(t *testing.T) {
	s := newAuthTestServer()
	sess, _ := s.sessions.Create(
		&auth.TokenData{AccessToken: "old", RefreshToken: "rt", ExpiresAt: time.Now().Add(-time.Minute)},
		auth.UserProfile{},
	)

	prev := authRefresh
	t.Cleanup(func() { authRefresh = prev })
	var calls int32
	authRefresh = func(context.Context, string, string, string) (*auth.TokenData, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond) // widen the race window
		return &auth.TokenData{AccessToken: "new", RefreshToken: "rt2", ExpiresAt: time.Now().Add(time.Hour)}, nil
	}

	const n = 8
	var wg sync.WaitGroup
	toks := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tok, err := s.sessionToken(context.Background(), sess)
			if err != nil {
				t.Errorf("sessionToken: %v", err)
			}
			toks[i] = tok
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("refresh calls = %d, want 1", got)
	}
	for i, tk := range toks {
		if tk != "new" {
			t.Errorf("goroutine %d token = %q, want new", i, tk)
		}
	}
}

func TestSessionToken_NoRefreshToken(t *testing.T) {
	s := newAuthTestServer()
	sess, _ := s.sessions.Create(
		&auth.TokenData{AccessToken: "old", ExpiresAt: time.Now().Add(-time.Minute)},
		auth.UserProfile{},
	)
	if _, err := s.sessionToken(context.Background(), sess); err == nil {
		t.Error("expected an error when the token is expired with no refresh token")
	}
}

func assertAuthErrorRedirect(t *testing.T, rec *httptest.ResponseRecorder, wantReason string) {
	t.Helper()
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/?auth_error=") || !strings.Contains(loc, wantReason) {
		t.Errorf("Location = %q, want /?auth_error=...%s", loc, wantReason)
	}
}
