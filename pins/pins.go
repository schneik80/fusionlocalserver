package pins

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schneik80/fusionlocalserver/config"
)

// FolderRef is a single hop in a folder ancestry chain (mirrors api.FolderRef).
type FolderRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Pin represents a bookmarked hub item (project, folder, or document).
// ProjectID and FolderPath are captured at pin time so navigation to
// projects and folders doesn't require an API call. HubID is retained on
// the record even though pins are now stored per-hub on disk — the
// stored hub scope is the authority, but the field is useful for the
// legacy-file migration and for the cross-hub navigation safety check.
type Pin struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Kind         string    `json:"kind"`
	HubID        string    `json:"hub_id"`
	ProjectID    string    `json:"project_id,omitempty"`
	ProjectAltID string    `json:"project_alt_id,omitempty"`
	// FolderPath is the ancestor chain from project root to the item:
	//   - project: empty
	//   - folder:  root-to-leaf path including the folder itself
	//   - document: root-to-leaf path of the containing folder
	FolderPath []FolderRef `json:"folder_path,omitempty"`
	PinnedAt   time.Time   `json:"pinned_at"`
}

// pinsFileForHub returns the absolute path to the pins file for the given
// hub. The hub ID is sanitized so that URN-format identifiers (which
// contain ':' and '/') produce filenames that round-trip cleanly across
// macOS, Linux, and Windows.
func pinsFileForHub(hubID string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pins-"+sanitizeHubID(hubID)+".json"), nil
}

// sanitizeHubID maps a hub ID to a filesystem-safe slug: any character
// outside [A-Za-z0-9_.\-] is replaced with '_'. The result is capped at
// 120 chars to stay well clear of per-platform path length limits.
func sanitizeHubID(hubID string) string {
	if hubID == "" {
		return "_unset"
	}
	var b strings.Builder
	for _, r := range hubID {
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

// legacyPinsPath is the historical single-file location pre-dating
// hub-scoped storage. It's only read by MigrateLegacy.
func legacyPinsPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pins.json"), nil
}

// Load reads pinned items for the given hub. Returns an empty slice when
// the hub-specific file is absent or corrupt rather than propagating an
// error that would block startup.
func Load(hubID string) ([]Pin, error) {
	path, err := pinsFileForHub(hubID)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Pin{}, nil
	}
	if err != nil {
		return nil, err
	}
	var ps []Pin
	if err := json.Unmarshal(data, &ps); err != nil {
		return []Pin{}, nil
	}
	return ps, nil
}

// Save writes the pin list for the given hub with mode 0600.
func Save(hubID string, ps []Pin) error {
	path, err := pinsFileForHub(hubID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// MigrateLegacy promotes any pins from the historical single-file
// pins.json into hub-scoped files. Called once at startup; idempotent
// thereafter because the legacy file is renamed to pins.json.bak on
// success. Pins lacking a HubID are dropped (they predate the hub field
// and have no way to be located deterministically).
func MigrateLegacy() error {
	legacy, err := legacyPinsPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var ps []Pin
	if err := json.Unmarshal(data, &ps); err != nil {
		// Corrupt — back up and bail out; per-hub files start clean.
		return os.Rename(legacy, legacy+".bak")
	}
	byHub := make(map[string][]Pin, 4)
	for _, p := range ps {
		if p.HubID == "" {
			continue
		}
		byHub[p.HubID] = append(byHub[p.HubID], p)
	}
	for hubID, hubPins := range byHub {
		existing, _ := Load(hubID)
		merged := append(existing, hubPins...)
		// Dedupe by ID — last write wins (existing first, then legacy).
		seen := make(map[string]struct{}, len(merged))
		out := merged[:0:0]
		for _, p := range merged {
			if _, ok := seen[p.ID]; ok {
				continue
			}
			seen[p.ID] = struct{}{}
			out = append(out, p)
		}
		if err := Save(hubID, out); err != nil {
			return err
		}
	}
	return os.Rename(legacy, legacy+".bak")
}

// IsPinned reports whether the given item ID is in the pin list.
func IsPinned(ps []Pin, id string) bool {
	for _, p := range ps {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Add prepends p to ps unless its ID is already present.
func Add(ps []Pin, p Pin) []Pin {
	if IsPinned(ps, p.ID) {
		return ps
	}
	p.PinnedAt = time.Now()
	return append([]Pin{p}, ps...)
}

// Remove returns a new slice with the item matching id omitted.
func Remove(ps []Pin, id string) []Pin {
	out := ps[:0:0]
	for _, p := range ps {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}

// IsPinnable reports whether an item of the given kind may be pinned.
// Hubs are excluded; projects, folders, and document kinds (including
// drawings, configured designs, and Fusion Electronics types) are
// allowed.
func IsPinnable(kind string) bool {
	switch kind {
	case "project", "folder",
		"design", "drawing", "configured",
		"schematic", "pcb", "ecad":
		return true
	}
	return false
}
