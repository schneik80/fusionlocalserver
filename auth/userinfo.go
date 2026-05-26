package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// userInfoEndpoint is a var so tests can point it at an httptest.Server.
var userInfoEndpoint = "https://api.userprofile.autodesk.com/userinfo"

// UserProfile is the minimal identity shown in the web UI for a logged-in
// session. Both fields are best-effort: a profile fetch that fails leaves them
// empty rather than blocking login.
type UserProfile struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// FetchUserProfile reads the OpenID Connect userinfo for the bearer token. It
// is a display nicety, so callers should treat an error as "unknown user"
// rather than a login failure.
func FetchUserProfile(ctx context.Context, accessToken string) (UserProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoEndpoint, nil)
	if err != nil {
		return UserProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return UserProfile{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UserProfile{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return UserProfile{}, fmt.Errorf("userinfo HTTP %d: %s", resp.StatusCode, body)
	}

	// APS userinfo returns OIDC claims; name/email are present for the
	// user-profile:read scope. Tolerate either OIDC or legacy field names.
	var u struct {
		Name      string `json:"name"`
		Email     string `json:"email"`
		UserName  string `json:"userName"`
		EmailID   string `json:"emailId"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return UserProfile{}, fmt.Errorf("parsing userinfo: %w", err)
	}

	p := UserProfile{Name: u.Name, Email: u.Email}
	if p.Name == "" {
		if u.UserName != "" {
			p.Name = u.UserName
		} else if fn := u.FirstName + " " + u.LastName; fn != " " {
			p.Name = fn
		}
	}
	if p.Email == "" {
		p.Email = u.EmailID
	}
	return p, nil
}
