package chat

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
	"golang.org/x/sync/singleflight"
)

// Capability is one thing a user may do in a project's chat. Capabilities
// derive from the caller's APS project role (docs/chat/PLAN.md §Decisions):
// Viewer/Reader read; Editor and up post, react, edit their own messages,
// and create channels; Manager/Administrator moderate (rename/archive
// channels, manage private-channel members, delete others' messages).
type Capability int

const (
	CapRead Capability = iota
	CapPost
	CapReact
	CapEditOwn
	CapCreateChannel
	CapModerate
)

// Role ranks. An unknown role string ranks below rankRead and can do
// nothing — deny-by-default for any FolderRoleEnum value this build
// doesn't recognize.
const (
	rankNone = iota
	rankRead
	rankWrite
	rankModerate
)

// roleRank maps APS FolderRoleEnum values (uppercase wire strings) to
// capability ranks. Matching is case-insensitive to be resilient to casing
// drift in the GraphQL schema.
var roleRank = map[string]int{
	"VIEWER":        rankRead,
	"READER":        rankRead,
	"EDITOR":        rankWrite,
	"MANAGER":       rankModerate,
	"ADMINISTRATOR": rankModerate,
}

// capRank is the minimum rank each capability requires.
var capRank = map[Capability]int{
	CapRead:          rankRead,
	CapPost:          rankWrite,
	CapReact:         rankWrite,
	CapEditOwn:       rankWrite,
	CapCreateChannel: rankWrite,
	CapModerate:      rankModerate,
}

// Authorizer answers "may this user do X in this project's chat" by
// fetching the project roster with the caller's own APS token
// (api.GetProjectMembers — readable by any project member, no hub-admin
// needed) and caching the resolved role per (user, project). Positive
// entries live for ttl, negative (not on the roster) for negTTL, so a
// removed user's REST access lapses within ttl at worst.
type Authorizer struct {
	fetch  func(ctx context.Context, token, projectID string) ([]api.Member, error)
	ttl    time.Duration
	negTTL time.Duration
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]roleEntry
	sf    singleflight.Group
}

type roleEntry struct {
	role  string // uppercase FolderRoleEnum; "" when not on the roster
	found bool
	at    time.Time
}

// NewAuthorizer returns an Authorizer with production wiring and TTLs.
func NewAuthorizer() *Authorizer {
	return &Authorizer{
		fetch:  api.GetProjectMembers,
		ttl:    60 * time.Second,
		negTTL: 15 * time.Second,
		now:    time.Now,
		cache:  make(map[string]roleEntry),
	}
}

// SetTTLsForTesting overrides the cache lifetimes so tests can exercise
// expiry-driven behavior (revocation teardown) without waiting out the
// production TTLs. Production code MUST NOT call this.
func (a *Authorizer) SetTTLsForTesting(positive, negative time.Duration) {
	a.ttl, a.negTTL = positive, negative
}

// Identity is the caller as chat authorization sees them: the OIDC subject
// plus their email. The roster row is matched by UserID when the profile
// carried a sub, falling back to a case-insensitive email match (open
// question #1 in docs/chat/PLAN.md — the fallback keeps either ID space
// working).
type Identity struct {
	UserID string
	Email  string
}

// Can reports whether the identity holds cap in the project.
func (a *Authorizer) Can(ctx context.Context, token string, id Identity, projectID string, cap Capability) (bool, error) {
	role, found, err := a.role(ctx, token, id, projectID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return roleRank[role] >= capRank[cap], nil
}

// CanAccessChannel is the design doc's two-layer rule (§1): project-level
// read access first, then — for private channels — the channel ACL, with
// project moderators (Manager/Administrator) always allowed through.
func (a *Authorizer) CanAccessChannel(ctx context.Context, token string, id Identity, projectID string, ch Channel) (bool, error) {
	role, found, err := a.role(ctx, token, id, projectID)
	if err != nil {
		return false, err
	}
	if !found || roleRank[role] < rankRead {
		return false, nil
	}
	if !ch.IsPrivate {
		return true, nil
	}
	if id.UserID != "" && memberIndex(ch.Members, id.UserID) >= 0 {
		return true, nil
	}
	return roleRank[role] >= rankModerate, nil
}

// role resolves the identity's project role through the cache. A roster
// fetch error is returned as-is (and not cached) so the handler can map it
// with s.fail — a flaky APS response must not lock a user out for negTTL.
func (a *Authorizer) role(ctx context.Context, token string, id Identity, projectID string) (string, bool, error) {
	key := id.UserID + "\x00" + strings.ToLower(id.Email) + "\x00" + projectID

	a.mu.Lock()
	if e, ok := a.cache[key]; ok {
		maxAge := a.ttl
		if !e.found {
			maxAge = a.negTTL
		}
		if a.now().Sub(e.at) < maxAge {
			a.mu.Unlock()
			return e.role, e.found, nil
		}
	}
	a.mu.Unlock()

	type result struct {
		role  string
		found bool
	}
	v, err, _ := a.sf.Do(key, func() (any, error) {
		members, err := a.fetch(ctx, token, projectID)
		if err != nil {
			return nil, err
		}
		res := result{}
		for _, m := range members {
			if !matchesMember(m, id) {
				continue
			}
			if !strings.EqualFold(m.Status, "ACTIVE") {
				break // on the roster but not active (PENDING/INACTIVE) → deny
			}
			res.role = strings.ToUpper(m.Role)
			res.found = true
			break
		}
		a.mu.Lock()
		a.cache[key] = roleEntry{role: res.role, found: res.found, at: a.now()}
		a.mu.Unlock()
		return res, nil
	})
	if err != nil {
		return "", false, err
	}
	res := v.(result)
	return res.role, res.found, nil
}

func matchesMember(m api.Member, id Identity) bool {
	if id.UserID != "" && m.UserID == id.UserID {
		return true
	}
	return id.Email != "" && strings.EqualFold(m.Email, id.Email)
}
