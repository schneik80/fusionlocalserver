package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/config"
)

func TestTokenData_Valid(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		td   *TokenData
		want bool
	}{
		{
			name: "nil receiver",
			td:   nil,
			want: false,
		},
		{
			name: "empty access token",
			td:   &TokenData{AccessToken: "", ExpiresAt: now.Add(1 * time.Hour)},
			want: false,
		},
		{
			name: "valid token, expires in 1h",
			td:   &TokenData{AccessToken: "abc", ExpiresAt: now.Add(1 * time.Hour)},
			want: true,
		},
		{
			name: "within 30s pre-expiry guard",
			td:   &TokenData{AccessToken: "abc", ExpiresAt: now.Add(10 * time.Second)},
			want: false,
		},
		{
			name: "just past 30s buffer",
			td:   &TokenData{AccessToken: "abc", ExpiresAt: now.Add(31 * time.Second)},
			want: true,
		},
		{
			name: "already expired",
			td:   &TokenData{AccessToken: "abc", ExpiresAt: now.Add(-1 * time.Second)},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.td.Valid(); got != tc.want {
				t.Errorf("TokenData.Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTokens_RoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	want := &TokenData{
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresAt:    time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := SaveTokens(want); err != nil {
		t.Fatalf("SaveTokens returned error: %v", err)
	}

	got, err := LoadTokens()
	if err != nil {
		t.Fatalf("LoadTokens returned error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadTokens returned nil TokenData")
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, want.AccessToken)
	}
	if got.RefreshToken != want.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", got.RefreshToken, want.RefreshToken)
	}
	if !got.ExpiresAt.Equal(want.ExpiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", got.ExpiresAt, want.ExpiresAt)
	}

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir() returned error: %v", err)
	}
	path := filepath.Join(dir, "tokens.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q) returned error: %v", path, err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Errorf("file mode = %o, want %o", got, want)
	}
}

func TestLoadTokens_Missing_NilNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	td, err := LoadTokens()
	if err != nil {
		t.Errorf("LoadTokens returned error: %v, want nil", err)
	}
	if td != nil {
		t.Errorf("LoadTokens returned %+v, want nil", td)
	}
}

func TestLoadTokens_Corrupt_DeletesAndReturnsNil(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir() returned error: %v", err)
	}
	path := filepath.Join(dir, "tokens.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0600); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", path, err)
	}

	td, err := LoadTokens()
	if err != nil {
		t.Errorf("LoadTokens returned error: %v, want nil", err)
	}
	if td != nil {
		t.Errorf("LoadTokens returned %+v, want nil", td)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected tokens file to be deleted; os.Stat error = %v", err)
	}
}

func TestDeleteTokens_Missing_NoError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := DeleteTokens(); err != nil {
		t.Errorf("DeleteTokens with no file returned error: %v", err)
	}
}

func TestDeleteTokens_Existing_Removes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	td := &TokenData{
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresAt:    time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := SaveTokens(td); err != nil {
		t.Fatalf("SaveTokens returned error: %v", err)
	}

	if err := DeleteTokens(); err != nil {
		t.Fatalf("DeleteTokens returned error: %v", err)
	}

	dir, err := config.Dir()
	if err != nil {
		t.Fatalf("config.Dir() returned error: %v", err)
	}
	path := filepath.Join(dir, "tokens.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected tokens file to be deleted; os.Stat error = %v", err)
	}
}
