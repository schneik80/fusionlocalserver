// Package auth implements the OAuth 2.0 Authorization Code + PKCE primitives
// the server uses to log each user in against Autodesk APS. It is deliberately
// transport-agnostic: it builds authorize URLs, exchanges codes, and refreshes
// tokens, but it never opens a browser, runs a local listener, or persists
// anything — the server owns the redirect endpoint and the per-user session
// store. The redirect URI is passed in by the caller (derived per request) so
// the same code serves localhost, a LAN IP, or a future TLS origin.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints are vars (not consts) so tests can swap them for an
// httptest.Server URL. Production code never reassigns them.
var (
	authEndpoint  = "https://developer.api.autodesk.com/authentication/v2/authorize"
	tokenEndpoint = "https://developer.api.autodesk.com/authentication/v2/token"
	authScope     = "data:read user-profile:read"
)

// NewPKCE generates a fresh PKCE verifier and its S256 challenge. The caller
// holds the verifier through the redirect and presents it at the token
// exchange; the challenge travels on the authorize URL.
func NewPKCE() (verifier, challenge string, err error) {
	verifier, err = newVerifier()
	if err != nil {
		return "", "", fmt.Errorf("generating PKCE verifier: %w", err)
	}
	return verifier, verifierToChallenge(verifier), nil
}

// Refresh exchanges a refresh token for a new access/refresh token pair. It
// does not persist anything; the caller (the session store) holds the result.
// APS rotates the refresh token on every use, so callers must serialise
// concurrent refreshes of the same token.
func Refresh(ctx context.Context, clientID, clientSecret, refreshToken string) (*TokenData, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return doTokenRequest(ctx, clientID, clientSecret, form)
}

func newVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func verifierToChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// BuildAuthURL constructs the APS authorize URL for the Authorization Code +
// PKCE flow. state is the CSRF token echoed back on the callback; redirectURI
// must match the one later sent to ExchangeCode and be registered on the APS
// app.
func BuildAuthURL(clientID, challenge, redirectURI, state string) string {
	p := url.Values{}
	p.Set("client_id", clientID)
	p.Set("response_type", "code")
	p.Set("redirect_uri", redirectURI)
	p.Set("scope", authScope)
	p.Set("state", state)
	p.Set("code_challenge", challenge)
	p.Set("code_challenge_method", "S256")
	return authEndpoint + "?" + p.Encode()
}

// ExchangeCode trades an authorization code for tokens. redirectURI must be
// byte-identical to the one used in BuildAuthURL (APS rejects a mismatch).
func ExchangeCode(ctx context.Context, clientID, clientSecret, code, verifier, redirectURI string) (*TokenData, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	return doTokenRequest(ctx, clientID, clientSecret, form)
}

// doTokenRequest posts to the APS token endpoint, authenticating the client via
// HTTP Basic Auth (client_id:client_secret) when a secret is present, or by
// including client_id in the form body for public clients.
func doTokenRequest(ctx context.Context, clientID, clientSecret string, form url.Values) (*TokenData, error) {
	if clientSecret != "" {
		// Confidential client: authenticate via Basic Auth.
		// Do not include client_id in the body.
	} else {
		// Public client: include client_id in the body.
		form.Set("client_id", clientID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if clientSecret != "" {
		req.SetBasicAuth(clientID, clientSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return nil, fmt.Errorf("parsing token response (HTTP %d): %w\nbody: %s", resp.StatusCode, err, raw)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("token error %s: %s", tr.Error, tr.ErrorDesc)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed (HTTP %d): %s", resp.StatusCode, raw)
	}
	return &TokenData{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}
