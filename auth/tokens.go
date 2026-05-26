package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/schneik80/fusionlocalserver/config"
)

// TokenData holds an OAuth access/refresh token pair with expiry.
type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Valid reports whether the access token is present and not about to expire.
func (t *TokenData) Valid() bool {
	return t != nil && t.AccessToken != "" && time.Now().Before(t.ExpiresAt.Add(-30*time.Second))
}

func tokensPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tokens.json"), nil
}

// LoadTokens reads saved tokens from disk. Returns nil, nil if no file exists.
func LoadTokens() (*TokenData, error) {
	path, err := tokensPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var td TokenData
	if err := json.Unmarshal(data, &td); err != nil {
		// Corrupted token file — delete it and treat as unauthenticated.
		_ = os.Remove(path)
		return nil, nil
	}
	return &td, nil
}

// SaveTokens writes tokens to disk with mode 0600.
func SaveTokens(td *TokenData) error {
	path, err := tokensPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(td, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// DeleteTokens removes any cached token file from disk. Used when the UI
// needs to force a re-authentication (e.g. the server rejected the token).
// Missing file is treated as success.
func DeleteTokens() error {
	path, err := tokensPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
