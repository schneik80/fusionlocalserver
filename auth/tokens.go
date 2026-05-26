package auth

import "time"

// TokenData holds an OAuth access/refresh token pair with expiry. The server
// keeps one per logged-in session, in memory only.
type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Valid reports whether the access token is present and not about to expire.
// The 30s skew triggers a proactive refresh just before the real expiry.
func (t *TokenData) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.ExpiresAt.Add(-30*time.Second))
}
