package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ErrFutureVersion is returned when a project's meta.json was written by a
// newer build than this one. The caller must refuse to serve chat for that
// project rather than risk rewriting data it doesn't understand.
var ErrFutureVersion = errors.New("chat: data written by a newer version")

// Store owns all chat persistence. One Store per server; all mutation of a
// project's data happens under that project's mutex, so the single process
// is the only writer (multi-process servers sharing a config dir are a
// documented non-goal).
type Store struct {
	dir string // root directory, e.g. ~/.config/fusionlocalserver/chat

	mu       sync.Mutex // guards projects map
	projects map[string]*projectState
}

// projectState is the in-memory index for one project's chat data.
type projectState struct {
	mu   sync.Mutex
	meta *projectMeta
}

// projectMeta mirrors meta.json: channel definitions plus the counters that
// must survive restarts. Message content lives in per-channel JSONL logs,
// not here, so this file stays small enough for whole-file atomic rewrites.
type projectMeta struct {
	Version       int        `json:"version"`
	ProjectID     string     `json:"projectId"`
	EventEpoch    int64      `json:"eventEpoch"` // bumped at startup; prefixes SSE event ids
	NextChannelID int64      `json:"nextChannelId"`
	Channels      []*Channel `json:"channels"`
}

// NewStore returns a Store rooted at dir, creating it if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("chat: creating store dir: %w", err)
	}
	return &Store{dir: dir, projects: make(map[string]*projectState)}, nil
}

// EnsureRoot loads (or initialises) the project's chat metadata and
// guarantees the root channel exists — the file-store analog of the design
// doc's "root channel created in the same transaction as the project":
// projects are APS-side, so first chat access is the creation hook.
// Idempotent; safe under concurrent calls.
func (s *Store) EnsureRoot(projectID string) (*Channel, error) {
	ps, err := s.project(projectID)
	if err != nil {
		return nil, err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for _, c := range ps.meta.Channels {
		if c.IsRoot {
			return c, nil
		}
	}
	root := &Channel{
		ID:         fmt.Sprintf("c%d", ps.meta.NextChannelID),
		Name:       RootChannelName,
		IsRoot:     true,
		CreatedAt:  time.Now().UTC(),
		NextMsgSeq: 1,
	}
	ps.meta.NextChannelID++
	ps.meta.Channels = append(ps.meta.Channels, root)
	if err := s.saveMeta(projectID, ps.meta); err != nil {
		// Roll back the in-memory append so a later retry re-creates it.
		ps.meta.Channels = ps.meta.Channels[:len(ps.meta.Channels)-1]
		ps.meta.NextChannelID--
		return nil, err
	}
	return root, nil
}

// Channels returns the project's channels (root guaranteed present). The
// returned slice is never nil. Visibility filtering for private channels is
// the authorizer's job, not the store's.
func (s *Store) Channels(projectID string) ([]*Channel, error) {
	if _, err := s.EnsureRoot(projectID); err != nil {
		return nil, err
	}
	ps, err := s.project(projectID)
	if err != nil {
		return nil, err
	}
	ps.mu.Lock()
	defer ps.mu.Unlock()
	out := make([]*Channel, len(ps.meta.Channels))
	copy(out, ps.meta.Channels)
	return out, nil
}

// project returns the cached state for projectID, loading meta.json on
// first access.
func (s *Store) project(projectID string) (*projectState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ps, ok := s.projects[projectID]; ok {
		return ps, nil
	}
	meta, err := s.loadMeta(projectID)
	if err != nil {
		return nil, err
	}
	ps := &projectState{meta: meta}
	s.projects[projectID] = ps
	return ps, nil
}

// projectDir maps a project URN to its on-disk directory, sanitized the
// same way pins maps hub IDs to filenames (URNs contain ':' and '/').
func (s *Store) projectDir(projectID string) string {
	return filepath.Join(s.dir, sanitizeID(projectID))
}

func (s *Store) metaPath(projectID string) string {
	return filepath.Join(s.projectDir(projectID), "meta.json")
}

// loadMeta reads a project's meta.json. Absent → fresh empty meta. Newer
// version → ErrFutureVersion (never rewrite what we don't understand).
// Corrupt → rename to .bak and start clean rather than block chat for the
// whole project (pins.MigrateLegacy precedent).
func (s *Store) loadMeta(projectID string) (*projectMeta, error) {
	path := s.metaPath(projectID)
	fresh := &projectMeta{
		Version:       metaVersion,
		ProjectID:     projectID,
		EventEpoch:    1,
		NextChannelID: 1,
		Channels:      []*Channel{},
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fresh, nil
	}
	if err != nil {
		return nil, fmt.Errorf("chat: reading %s: %w", path, err)
	}
	var meta projectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		_ = os.Rename(path, path+".bak")
		return fresh, nil
	}
	if meta.Version > metaVersion {
		return nil, fmt.Errorf("%w: meta.json v%d > v%d", ErrFutureVersion, meta.Version, metaVersion)
	}
	// meta.Version < metaVersion: in-place upgrade functions slot in here
	// when metaVersion moves past 1.
	if meta.Channels == nil {
		meta.Channels = []*Channel{}
	}
	return &meta, nil
}

// saveMeta atomically rewrites meta.json (temp file + rename, 0600), so a
// crash mid-write can never leave a half-written file behind.
func (s *Store) saveMeta(projectID string, meta *projectMeta) error {
	dir := s.projectDir(projectID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("chat: creating project dir: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "meta-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.metaPath(projectID)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// sanitizeID maps a URN-format identifier to a filesystem-safe slug: any
// character outside [A-Za-z0-9_.\-] becomes '_', capped at 120 chars (same
// rules as pins.sanitizeHubID so both stores age identically on disk).
func sanitizeID(id string) string {
	if id == "" {
		return "_unset"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}
