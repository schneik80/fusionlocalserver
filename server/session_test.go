package server

import (
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
)

func TestRandToken_UniqueAndNonEmpty(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		tok, err := randToken(32)
		if err != nil {
			t.Fatalf("randToken: %v", err)
		}
		if tok == "" {
			t.Fatal("randToken returned empty string")
		}
		if seen[tok] {
			t.Fatalf("randToken returned a duplicate: %q", tok)
		}
		seen[tok] = true
	}
}

func TestSessionStore_CreateGetDelete(t *testing.T) {
	st := NewSessionStore(sessionIdleTTL, sessionAbsTTL, quietLogger())
	td := &auth.TokenData{AccessToken: "AT", ExpiresAt: time.Now().Add(time.Hour)}

	sess, err := st.Create(td, auth.UserProfile{Email: "a@b.c"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("Create returned a session with an empty ID")
	}

	got, ok := st.Get(sess.ID)
	if !ok || got != sess {
		t.Fatalf("Get after Create: ok=%v got=%v want=%v", ok, got, sess)
	}

	st.Delete(sess.ID)
	if _, ok := st.Get(sess.ID); ok {
		t.Error("Get after Delete still returned the session")
	}
}

func TestSessionStore_GetUnknownOrEmpty(t *testing.T) {
	st := NewSessionStore(sessionIdleTTL, sessionAbsTTL, quietLogger())
	if _, ok := st.Get(""); ok {
		t.Error("Get(\"\") returned ok")
	}
	if _, ok := st.Get("does-not-exist"); ok {
		t.Error("Get(unknown) returned ok")
	}
}

func TestSessionStore_IdleExpiry(t *testing.T) {
	st := NewSessionStore(time.Hour, 7*24*time.Hour, quietLogger())
	sess, _ := st.Create(&auth.TokenData{AccessToken: "AT"}, auth.UserProfile{})

	// Push lastSeen past the idle TTL.
	st.mu.Lock()
	st.byID[sess.ID].lastSeen = time.Now().Add(-2 * time.Hour)
	st.mu.Unlock()

	if _, ok := st.Get(sess.ID); ok {
		t.Error("idle-expired session was not evicted")
	}
	st.mu.Lock()
	_, present := st.byID[sess.ID]
	st.mu.Unlock()
	if present {
		t.Error("idle-expired session was not removed from the map")
	}
}

func TestSessionStore_AbsoluteExpiry(t *testing.T) {
	st := NewSessionStore(12*time.Hour, time.Hour, quietLogger())
	sess, _ := st.Create(&auth.TokenData{AccessToken: "AT"}, auth.UserProfile{})

	st.mu.Lock()
	st.byID[sess.ID].CreatedAt = time.Now().Add(-2 * time.Hour)
	st.mu.Unlock()

	if _, ok := st.Get(sess.ID); ok {
		t.Error("absolute-expired session was not evicted")
	}
}

func TestSessionStore_Sweep(t *testing.T) {
	st := NewSessionStore(time.Hour, 7*24*time.Hour, quietLogger())
	live, _ := st.Create(&auth.TokenData{AccessToken: "live"}, auth.UserProfile{})
	dead, _ := st.Create(&auth.TokenData{AccessToken: "dead"}, auth.UserProfile{})

	st.mu.Lock()
	st.byID[dead.ID].lastSeen = time.Now().Add(-2 * time.Hour)
	st.mu.Unlock()

	st.sweep()

	if _, ok := st.Get(live.ID); !ok {
		t.Error("sweep evicted a live session")
	}
	st.mu.Lock()
	_, present := st.byID[dead.ID]
	st.mu.Unlock()
	if present {
		t.Error("sweep did not evict the expired session")
	}
}

func TestPendingStore_TakeIsSingleUse(t *testing.T) {
	p := NewPendingStore(pendingTTL)
	p.Put("state1", pendingEntry{verifier: "v", redirectURI: "u", createdAt: time.Now()})

	e, ok := p.Take("state1")
	if !ok || e.verifier != "v" || e.redirectURI != "u" {
		t.Fatalf("first Take: ok=%v e=%+v", ok, e)
	}
	if _, ok := p.Take("state1"); ok {
		t.Error("second Take returned ok; entry should be single-use")
	}
}

func TestPendingStore_Expired(t *testing.T) {
	p := NewPendingStore(5 * time.Minute)
	p.Put("s", pendingEntry{verifier: "v", createdAt: time.Now().Add(-10 * time.Minute)})
	if _, ok := p.Take("s"); ok {
		t.Error("expired pending entry returned ok")
	}
}
