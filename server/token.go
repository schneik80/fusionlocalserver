package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/schneik80/fusionlocalserver/auth"
)

// proactiveSkew is how far before a token's expiry the background refresher
// wakes up. It must sit between the access token's lifetime (~1h) and the
// 30s skew baked into TokenData.Valid, so the proactive refresh lands while
// the cached token is still usable for in-flight requests.
const proactiveSkew = 60 * time.Second

// errNoRefresh is returned by Token when the cached token has expired and no
// refresh token is available to mint a new one. The server cannot run an
// interactive browser login mid-flight (that only happens once at startup),
// so the operator must restart. Handlers map this to HTTP 401.
var errNoRefresh = errors.New("token expired and no refresh token available — restart the server to re-authenticate")

// TokenManager owns the single APS identity the server proxies every request
// through. It reuses the auth package's disk-cached token (the same
// ~/.config/fusionlocalserver/tokens.json the TUI writes) and keeps it fresh.
//
// Concurrency model: a single mutex guards both the cached token and the
// refresh round-trip. APS rotates the refresh token on every use, so two
// concurrent refreshes against the same refresh token would brick the cache —
// the second call presents a refresh token APS has already invalidated.
// Serialising check-and-refresh under one lock guarantees at most one refresh
// per expiry boundary. Holding the lock across the (~hourly, sub-second)
// refresh HTTP call is intentional: concurrent callers block briefly and then
// observe the freshly-minted token.
type TokenManager struct {
	clientID     string
	clientSecret string
	logger       *slog.Logger

	mu sync.Mutex
	td *auth.TokenData
}

// NewTokenManager constructs a TokenManager for the given APS credentials.
// clientSecret may be empty for public (PKCE) clients.
func NewTokenManager(clientID, clientSecret string, logger *slog.Logger) *TokenManager {
	return &TokenManager{
		clientID:     clientID,
		clientSecret: clientSecret,
		logger:       logger,
	}
}

// Bootstrap establishes a usable token before the server starts serving. It
// runs the one and only interactive login the process will ever perform, so it
// must complete before ListenAndServe. Resolution order:
//
//  1. cached token from disk that is still Valid -> use as-is
//  2. expired cached token with a refresh token  -> Refresh
//  3. otherwise                                   -> interactive browser Login
//
// Fails fast when no client_id is configured.
func (tm *TokenManager) Bootstrap(ctx context.Context) error {
	if tm.clientID == "" {
		return errors.New("no APS client_id configured (build with CLIENT_ID, or set APS_CLIENT_ID / config.json)")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	td, err := auth.LoadTokens()
	if err != nil {
		return fmt.Errorf("loading cached tokens: %w", err)
	}
	tm.td = td

	switch {
	case td.Valid():
		tm.logger.Info("auth: using cached token", "expires_at", td.ExpiresAt.Format(time.RFC3339))
		return nil
	case td != nil && td.RefreshToken != "":
		tm.logger.Info("auth: cached token expired, refreshing")
		return tm.refreshLocked(ctx)
	default:
		tm.logger.Info("auth: no valid cached token — starting interactive login (browser opens on the server host)")
		newTd, err := auth.Login(ctx, tm.clientID, tm.clientSecret)
		if err != nil {
			return fmt.Errorf("interactive login failed: %w", err)
		}
		tm.td = newTd
		tm.logger.Info("auth: interactive login complete", "expires_at", newTd.ExpiresAt.Format(time.RFC3339))
		return nil
	}
}

// Token returns a currently-valid access token, refreshing transparently if
// the cached one has expired. This is the hot path: every handler calls it.
func (tm *TokenManager) Token(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if err := tm.ensureValidLocked(ctx); err != nil {
		return "", err
	}
	return tm.td.AccessToken, nil
}

// ensureValidLocked refreshes the cached token when it is no longer valid.
// The caller must hold tm.mu.
func (tm *TokenManager) ensureValidLocked(ctx context.Context) error {
	if tm.td.Valid() {
		return nil
	}
	if tm.td == nil || tm.td.RefreshToken == "" {
		return errNoRefresh
	}
	return tm.refreshLocked(ctx)
}

// refreshLocked exchanges the cached refresh token for a new token pair and
// persists it (auth.Refresh writes tokens.json). The caller must hold tm.mu.
func (tm *TokenManager) refreshLocked(ctx context.Context) error {
	td, err := auth.Refresh(ctx, tm.clientID, tm.clientSecret, tm.td.RefreshToken)
	if err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}
	tm.td = td
	tm.logger.Info("auth: token refreshed", "expires_at", td.ExpiresAt.Format(time.RFC3339))
	return nil
}

// StartRefresher launches a background goroutine that proactively refreshes the
// token ~proactiveSkew before it expires, so the ~hourly refresh stall never
// lands on a user request. Token() remains the correctness fallback if the
// refresher falls behind. The goroutine exits when ctx is cancelled (shutdown).
func (tm *TokenManager) StartRefresher(ctx context.Context) {
	go tm.refreshLoop(ctx)
}

func (tm *TokenManager) refreshLoop(ctx context.Context) {
	const errBackoff = 30 * time.Second
	for {
		tm.mu.Lock()
		hasRefresh := tm.td != nil && tm.td.RefreshToken != ""
		var expiry time.Time
		if tm.td != nil {
			expiry = tm.td.ExpiresAt
		}
		tm.mu.Unlock()

		if !hasRefresh {
			// Nothing to refresh with; Token() will surface 401 on expiry.
			return
		}

		wait := time.Until(expiry.Add(-proactiveSkew))
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		tm.mu.Lock()
		// Refresh only if no concurrent Token() call already advanced the token
		// past the expiry we were waiting on; otherwise we'd double-refresh and
		// invalidate the just-minted token.
		var err error
		if tm.td != nil && !tm.td.ExpiresAt.After(expiry) {
			err = tm.refreshLocked(ctx)
		}
		tm.mu.Unlock()

		if err != nil {
			tm.logger.Error("auth: proactive refresh failed; will retry", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(errBackoff):
			}
		}
	}
}
