package server

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEntryFiles drops a data.json (and optionally a GLB) into an entry dir so
// it looks like a completed decode.
func writeEntryFiles(t *testing.T, dir string, glbBytes int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, modelDataFile), []byte(`{"parameters":{},"timeline":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if glbBytes > 0 {
		if err := os.WriteFile(filepath.Join(dir, modelGLBFile), make([]byte, glbBytes), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestModelCache_BeginIsExclusive(t *testing.T) {
	c, err := newModelCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	started1, dir := c.begin("k1", "")
	if !started1 {
		t.Fatal("first begin should start the job")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("begin should create the dir: %v", err)
	}
	started2, _ := c.begin("k1", "")
	if started2 {
		t.Fatal("second begin for the same key must not start a second job")
	}
	if e := c.snapshot("k1"); e == nil || e.status != modelPending {
		t.Fatalf("expected a PENDING entry, got %+v", e)
	}
}

func TestModelCache_SuccessAndFailedReclaim(t *testing.T) {
	c, err := newModelCache(t.TempDir(), 0)
	if err != nil {
		t.Fatal(err)
	}
	_, dir := c.begin("k1", "")
	writeEntryFiles(t, dir, 100)
	c.markSuccess("k1")
	if e := c.snapshot("k1"); e == nil || e.status != modelSuccess || e.size == 0 {
		t.Fatalf("expected SUCCESS with non-zero size, got %+v", e)
	}
	// A SUCCESS key is not re-begun.
	if started, _ := c.begin("k1", ""); started {
		t.Error("a SUCCESS key must not be re-begun")
	}

	c.markFailed("k1", "boom")
	if e := c.snapshot("k1"); e == nil || e.status != modelFailed || e.errMsg != "boom" {
		t.Fatalf("expected FAILED entry, got %+v", e)
	}
	// A FAILED key is reclaimed for retry.
	started, _ := c.begin("k1", "")
	if !started {
		t.Error("a FAILED key should be reclaimable for retry")
	}
}

func TestModelCache_EvictsOverCap(t *testing.T) {
	c, err := newModelCache(t.TempDir(), 250) // cap fits two 100-byte entries, not three
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a", "b", "c"} {
		_, dir := c.begin(k, "")
		writeEntryFiles(t, dir, 100)
		c.markSuccess(k)
	}
	// "a" (least recently used) should have been evicted to stay under cap.
	if c.snapshot("a") != nil {
		t.Error("LRU entry 'a' should have been evicted")
	}
	if c.snapshot("c") == nil {
		t.Error("most-recent entry 'c' should remain")
	}
}

func TestModelCache_SeedsFromDisk(t *testing.T) {
	root := t.TempDir()
	// A complete entry (data.json present) is adopted; a partial one is purged.
	complete := filepath.Join(root, keyHash("done"))
	partial := filepath.Join(root, keyHash("partial"))
	os.MkdirAll(complete, 0o700)
	os.MkdirAll(partial, 0o700)
	writeEntryFiles(t, complete, 50)
	os.WriteFile(filepath.Join(partial, "design.f3z"), []byte("incomplete"), 0o644)

	c, err := newModelCache(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if e := c.snapshot("done"); e == nil || e.status != modelSuccess {
		t.Errorf("complete entry should be adopted as SUCCESS, got %+v", e)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Error("partial entry directory should have been removed on seed")
	}
}
