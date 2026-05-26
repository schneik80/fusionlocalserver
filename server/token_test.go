package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/schneik80/FusionDataCLI/auth"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// writeFakeTokens drops a tokens.json under the test's temp HOME so
// auth.LoadTokens (via config.Dir) reads it. The caller must have set HOME to
// a temp dir first via t.Setenv.
func writeFakeTokens(t *testing.T, td *auth.TokenData) {
	t.Helper()
	dir := filepath.Join(os.Getenv("HOME"), ".config", "fusiondatacli")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(td)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tokens.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapUsesCachedValidToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeFakeTokens(t, &auth.TokenData{
		AccessToken:  "cached-access",
		RefreshToken: "cached-refresh",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	tm := NewTokenManager("client-id", "", quietLogger())
	if err := tm.Bootstrap(context.Background()); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	tok, err := tm.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "cached-access" {
		t.Fatalf("token = %q, want cached-access", tok)
	}
}

func TestBootstrapFailsWithoutClientID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tm := NewTokenManager("", "", quietLogger())
	if err := tm.Bootstrap(context.Background()); err == nil {
		t.Fatal("Bootstrap with empty client_id should fail")
	}
}

func TestTokenExpiredNoRefresh(t *testing.T) {
	tm := NewTokenManager("client-id", "", quietLogger())
	// Expired token, no refresh token: nothing to refresh with.
	tm.td = &auth.TokenData{AccessToken: "stale", ExpiresAt: time.Now().Add(-time.Minute)}

	_, err := tm.Token(context.Background())
	if !errors.Is(err, errNoRefresh) {
		t.Fatalf("Token err = %v, want errNoRefresh", err)
	}
}

func TestTokenConcurrentValid(t *testing.T) {
	tm := NewTokenManager("client-id", "", quietLogger())
	tm.td = &auth.TokenData{AccessToken: "concurrent", ExpiresAt: time.Now().Add(time.Hour)}

	const n = 64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			tok, err := tm.Token(context.Background())
			if err != nil || tok != "concurrent" {
				t.Errorf("Token = %q, %v", tok, err)
			}
		}()
	}
	wg.Wait()
}
