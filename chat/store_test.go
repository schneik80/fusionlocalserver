package chat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testProject = "urn:adsk.wipprod:fs.folder:co.example/123"

func newTestStore(t *testing.T) *Store {
	return newStoreAt(t, t.TempDir())
}

// newStoreAt opens a Store over dir and registers its Close as cleanup —
// Windows can't remove the TempDir while message-log handles are open.
func newStoreAt(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestEnsureRoot_CreatesOnceAndPersists(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	root, err := s.EnsureRoot(testProject)
	if err != nil {
		t.Fatalf("EnsureRoot: %v", err)
	}
	if !root.IsRoot || root.Name != RootChannelName || root.IsPrivate {
		t.Fatalf("unexpected root channel: %+v", root)
	}

	// Idempotent within the same store instance.
	again, err := s.EnsureRoot(testProject)
	if err != nil {
		t.Fatalf("EnsureRoot (again): %v", err)
	}
	if again.ID != root.ID {
		t.Fatalf("root recreated: %q != %q", again.ID, root.ID)
	}

	// Survives a restart: a second store over the same dir sees the same root.
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore (reload): %v", err)
	}
	reloaded, err := s2.EnsureRoot(testProject)
	if err != nil {
		t.Fatalf("EnsureRoot (reload): %v", err)
	}
	if reloaded.ID != root.ID || !reloaded.IsRoot {
		t.Fatalf("root not persisted: %+v", reloaded)
	}

	chans, err := s2.Channels(testProject)
	if err != nil {
		t.Fatalf("Channels: %v", err)
	}
	if len(chans) != 1 {
		t.Fatalf("want exactly one channel after reload, got %d", len(chans))
	}
}

func TestLoadMeta_FutureVersionRefused(t *testing.T) {
	s := newTestStore(t)
	dir := s.projectDir(testProject)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	future, _ := json.Marshal(map[string]any{"version": metaVersion + 1, "projectId": testProject})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), future, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := s.EnsureRoot(testProject); !errors.Is(err, ErrFutureVersion) {
		t.Fatalf("want ErrFutureVersion, got %v", err)
	}
	// The future-versioned file must be left untouched.
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil || string(data) != string(future) {
		t.Fatalf("future meta.json modified (err=%v)", err)
	}
}

func TestLoadMeta_CorruptFileBackedUp(t *testing.T) {
	s := newTestStore(t)
	dir := s.projectDir(testProject)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	root, err := s.EnsureRoot(testProject)
	if err != nil {
		t.Fatalf("EnsureRoot over corrupt meta: %v", err)
	}
	if !root.IsRoot {
		t.Fatalf("no root after corrupt recovery: %+v", root)
	}
	if _, err := os.Stat(filepath.Join(dir, "meta.json.bak")); err != nil {
		t.Fatalf("corrupt meta.json not backed up: %v", err)
	}
}

func TestSanitizeID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "_unset"},
		{"urn:adsk.wipprod:fs.folder:co.x/1", "urn_adsk.wipprod_fs.folder_co.x_1"},
		{"simple-Id_0.9", "simple-Id_0.9"},
		{strings.Repeat("a", 200), strings.Repeat("a", 120)},
	}
	for _, c := range cases {
		if got := sanitizeID(c.in); got != c.want {
			t.Errorf("sanitizeID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEnsureRoot_ConcurrentSingleRoot(t *testing.T) {
	s := newTestStore(t)
	const n = 16
	ids := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			c, err := s.EnsureRoot(testProject)
			if err != nil {
				ids <- "err:" + err.Error()
				return
			}
			ids <- c.ID
		}()
	}
	first := <-ids
	for i := 1; i < n; i++ {
		if got := <-ids; got != first {
			t.Fatalf("concurrent EnsureRoot diverged: %q vs %q", got, first)
		}
	}
	chans, err := s.Channels(testProject)
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 1 {
		t.Fatalf("want 1 channel, got %d", len(chans))
	}
}
