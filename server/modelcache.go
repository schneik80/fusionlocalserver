package server

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// modelStatus is the lifecycle of a cached design decode. It mirrors the
// thumbnail PENDING/SUCCESS/FAILED pattern so the frontend can reuse the same
// poll-with-timeout shape.
type modelStatus string

const (
	modelPending modelStatus = "PENDING"
	modelSuccess modelStatus = "SUCCESS"
	modelFailed  modelStatus = "FAILED"
)

// Filenames within an entry's directory. The design file keeps its native
// extension (.f3d vs .f3z) so the reader can pick --export-glb vs --assembly-glb.
const (
	modelDataFile = "data.json" // projected {parameters, timeline}
	modelGLBFile  = "scene.glb" // exported binary glTF 2.0
	modelJSONFile = "reader.json"
)

// modelEntry is one cached design decode. It owns a directory under the cache
// root holding the downloaded design file, the reader's JSON, the projected
// data, and the exported GLB.
type modelEntry struct {
	key        string
	dir        string
	designName string // e.g. "Widget.f3z" — carries the native extension
	status     modelStatus
	errMsg     string // populated when status == modelFailed
	size       int64  // total bytes on disk for this entry (set on success)
	updated    time.Time
}

// modelCache is an on-disk, byte-capped, concurrency-safe cache of decoded
// designs keyed by the immutable version identity (hashed to a directory name).
// Unlike thumbCache it holds large artifacts on disk, not in memory; the
// in-memory map is just the index + job state.
type modelCache struct {
	mu       sync.Mutex
	root     string
	entries  map[string]*modelEntry
	maxBytes int64
	total    int64
}

// newModelCache creates (or adopts) the cache rooted at dir. It scans existing
// subdirectories: a directory holding the projected data.json is adopted as a
// SUCCESS entry (the GLB is optional — a graphics-less design has none);
// anything else (a partial/interrupted decode) is removed so it can be cleanly
// rebuilt. maxBytes caps total on-disk size; 0 disables eviction.
func newModelCache(root string, maxBytes int64) (*modelCache, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	c := &modelCache{
		root:     root,
		entries:  make(map[string]*modelEntry),
		maxBytes: maxBytes,
	}
	// Seed from disk. We only adopt complete entries; the directory name is the
	// hashed key, which we cannot reverse, so adopted entries carry an empty
	// key — they still serve cached bytes (the handler re-derives the dir from
	// the key on every request) but are addressed by hash on disk.
	dents, _ := os.ReadDir(root)
	for _, de := range dents {
		if !de.IsDir() {
			continue
		}
		dir := filepath.Join(root, de.Name())
		if !fileExists(filepath.Join(dir, modelDataFile)) {
			os.RemoveAll(dir) // partial/interrupted; rebuild on demand
			continue
		}
		sz := dirSize(dir)
		c.entries[de.Name()] = &modelEntry{
			dir:     dir,
			status:  modelSuccess,
			size:    sz,
			updated: dirMtime(dir),
		}
		c.total += sz
	}
	return c, nil
}

// keyHash maps a cache key (the version URN / tip identity) to a filesystem-safe
// directory name. The on-disk index is addressed by this hash.
func keyHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// dirFor returns the absolute directory for a key (not necessarily existing).
func (c *modelCache) dirFor(key string) string {
	return filepath.Join(c.root, keyHash(key))
}

// snapshot returns a copy of the entry for key, or nil if absent.
func (c *modelCache) snapshot(key string) *modelEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.entries[keyHash(key)]; e != nil {
		cp := *e
		return &cp
	}
	return nil
}

// begin claims a key for decoding. It returns started=true to exactly one
// caller (the one that should run the download+decode job); concurrent callers
// for the same key — or callers for an already-SUCCESS/PENDING key — get
// started=false. A previously FAILED key is reclaimed (status reset to PENDING)
// so the user can retry by re-opening the tab. On started=true the entry's
// directory is freshly (re)created empty.
func (c *modelCache) begin(key, designName string) (started bool, dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := keyHash(key)
	dir = filepath.Join(c.root, h)
	if e := c.entries[h]; e != nil {
		if e.status == modelFailed {
			// Reclaim for retry.
			c.total -= e.size
			os.RemoveAll(dir)
			_ = os.MkdirAll(dir, 0o700)
			e.status = modelPending
			e.errMsg = ""
			e.size = 0
			e.designName = designName
			e.updated = time.Now()
			return true, dir
		}
		return false, dir
	}
	os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o700)
	c.entries[h] = &modelEntry{
		key:        key,
		dir:        dir,
		designName: designName,
		status:     modelPending,
		updated:    time.Now(),
	}
	return true, dir
}

// markSuccess flips a PENDING entry to SUCCESS, recomputes its on-disk size, and
// evicts older entries if the cache is over its byte cap.
func (c *modelCache) markSuccess(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[keyHash(key)]
	if e == nil {
		return
	}
	sz := dirSize(e.dir)
	c.total += sz - e.size
	e.size = sz
	e.status = modelSuccess
	e.updated = time.Now()
	c.evictToCapLocked()
}

// markFailed records a decode failure. The directory is removed (it holds at
// most a partial download), but the index entry is kept so the status endpoint
// can report the error; the next begin() reclaims it.
func (c *modelCache) markFailed(key, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[keyHash(key)]
	if e == nil {
		return
	}
	os.RemoveAll(e.dir)
	c.total -= e.size
	e.size = 0
	e.status = modelFailed
	e.errMsg = msg
	e.updated = time.Now()
}

// touch updates an entry's recency on a cache hit so eviction stays LRU.
func (c *modelCache) touch(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.entries[keyHash(key)]; e != nil {
		e.updated = time.Now()
	}
}

// evictToCapLocked removes least-recently-used SUCCESS entries until total is
// within maxBytes. PENDING entries are never evicted (their job is in flight).
// Caller holds mu.
func (c *modelCache) evictToCapLocked() {
	if c.maxBytes <= 0 {
		return
	}
	for c.total > c.maxBytes {
		var victimKey string
		var oldest time.Time
		for k, e := range c.entries {
			if e.status != modelSuccess {
				continue
			}
			if victimKey == "" || e.updated.Before(oldest) {
				victimKey, oldest = k, e.updated
			}
		}
		if victimKey == "" {
			return // nothing evictable
		}
		v := c.entries[victimKey]
		os.RemoveAll(v.dir)
		c.total -= v.size
		delete(c.entries, victimKey)
	}
}

// dirSize sums the sizes of regular files directly under dir (the cache is flat
// per entry, so a shallow walk suffices).
func dirSize(dir string) int64 {
	var total int64
	dents, _ := os.ReadDir(dir)
	for _, de := range dents {
		if info, err := de.Info(); err == nil && !info.IsDir() {
			total += info.Size()
		}
	}
	return total
}

func dirMtime(dir string) time.Time {
	if st, err := os.Stat(dir); err == nil {
		return st.ModTime()
	}
	return time.Now()
}
