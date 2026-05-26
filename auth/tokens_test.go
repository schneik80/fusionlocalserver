package auth

import (
	"testing"
	"time"
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
