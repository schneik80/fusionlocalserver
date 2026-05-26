package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"sync"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
)

// Session/pending lifetimes. Sessions live in memory only, so a process restart
// logs everyone out (a runtime port rebind does not — the stores outlive the
// listener). The pending store holds in-flight logins between the redirect and
// the callback.
const (
	sessionIdleTTL = 12 * time.Hour
	sessionAbsTTL  = 7 * 24 * time.Hour
	pendingTTL     = 5 * time.Minute
	janitorPeriod  = 10 * time.Minute
)

// randToken returns a URL-safe random token carrying nbytes of entropy. Used
// for opaque session ids and OAuth state values.
func randToken(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Session is one logged-in user's server-side state. The browser holds only an
// opaque cookie carrying ID; the APS tokens never leave the server.
type Session struct {
	ID        string
	Profile   auth.UserProfile
	CreatedAt time.Time

	lastSeen time.Time // guarded by SessionStore.mu

	// refreshMu serialises token refresh for this session. APS rotates the
	// refresh token on every use, so two concurrent refreshes of the same
	// session would invalidate each other; the lock plus a re-check of
	// token.Valid() guarantees at most one refresh per expiry boundary.
	refreshMu sync.Mutex
	token     *auth.TokenData // guarded by refreshMu
}

// SessionStore is an in-memory, expiring set of logged-in sessions keyed by an
// opaque random id. Per-user isolation: each session proxies APS calls under
// its own identity, replacing the old single shared token.
type SessionStore struct {
	mu      sync.Mutex
	byID    map[string]*Session
	idleTTL time.Duration
	absTTL  time.Duration
	logger  *slog.Logger
}

func NewSessionStore(idle, abs time.Duration, logger *slog.Logger) *SessionStore {
	return &SessionStore{
		byID:    make(map[string]*Session),
		idleTTL: idle,
		absTTL:  abs,
		logger:  logger,
	}
}

// Create mints a new session for the freshly-exchanged token pair. The id is
// generated only here (never derived from anything client-supplied), so login
// cannot fixate a session id.
func (s *SessionStore) Create(td *auth.TokenData, p auth.UserProfile) (*Session, error) {
	id, err := randToken(32)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	sess := &Session{ID: id, Profile: p, CreatedAt: now, lastSeen: now, token: td}
	s.mu.Lock()
	s.byID[id] = sess
	s.mu.Unlock()
	return sess, nil
}

// Get returns the live session for id, evicting and reporting absent if it has
// passed its idle or absolute deadline. A hit bumps the idle clock.
func (s *SessionStore) Get(id string) (*Session, bool) {
	if id == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.byID[id]
	if !ok {
		return nil, false
	}
	if s.expired(sess, time.Now()) {
		delete(s.byID, id)
		return nil, false
	}
	sess.lastSeen = time.Now()
	return sess, true
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	delete(s.byID, id)
	s.mu.Unlock()
}

// expired reports whether sess is past either deadline. Caller holds s.mu.
func (s *SessionStore) expired(sess *Session, now time.Time) bool {
	return now.After(sess.CreatedAt.Add(s.absTTL)) || now.After(sess.lastSeen.Add(s.idleTTL))
}

// StartJanitor sweeps expired sessions periodically until ctx is cancelled.
func (s *SessionStore) StartJanitor(ctx context.Context) {
	go func() {
		t := time.NewTicker(janitorPeriod)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sweep()
			}
		}
	}()
}

func (s *SessionStore) sweep() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sess := range s.byID {
		if s.expired(sess, now) {
			delete(s.byID, id)
		}
	}
}

// pendingEntry is the per-login state held between the authorize redirect and
// the callback: the PKCE verifier and the exact redirect_uri used (which the
// token exchange must echo byte-for-byte).
type pendingEntry struct {
	verifier    string
	redirectURI string
	createdAt   time.Time
}

// PendingStore holds in-flight logins keyed by their OAuth state value.
type PendingStore struct {
	mu  sync.Mutex
	m   map[string]pendingEntry
	ttl time.Duration
}

func NewPendingStore(ttl time.Duration) *PendingStore {
	return &PendingStore{m: make(map[string]pendingEntry), ttl: ttl}
}

func (p *PendingStore) Put(state string, e pendingEntry) {
	p.mu.Lock()
	p.m[state] = e
	p.mu.Unlock()
}

// Take returns and removes the entry for state. It reports absent if there is
// no entry or it has expired — single-use, so a replayed callback finds
// nothing.
func (p *PendingStore) Take(state string) (pendingEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.m[state]
	if !ok {
		return pendingEntry{}, false
	}
	delete(p.m, state)
	if time.Since(e.createdAt) > p.ttl {
		return pendingEntry{}, false
	}
	return e, true
}

func (p *PendingStore) StartJanitor(ctx context.Context) {
	go func() {
		t := time.NewTicker(janitorPeriod)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				p.sweep()
			}
		}
	}()
}

func (p *PendingStore) sweep() {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, e := range p.m {
		if now.Sub(e.createdAt) > p.ttl {
			delete(p.m, k)
		}
	}
}
