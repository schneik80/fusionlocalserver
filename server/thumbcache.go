package server

import (
	"sync"
	"time"
)

// thumbEntry is one cached thumbnail. meta (status + signed URL) is cheap and
// warmed during classify; png holds the actual bytes once fetched. The bytes
// are immutable for a given component version, so they never need refreshing;
// the signed URL expires, so urlAt bounds its reuse (irrelevant once png is
// present).
type thumbEntry struct {
	status  string
	url     string
	urlAt   time.Time
	png     []byte
	ctype   string
	updated time.Time // for LRU-style eviction
}

// thumbCache is a small, bounded, concurrency-safe cache keyed by component
// version id. It is shared across every client of the server, so warming it
// once (off the per-row classify probe) benefits every browser on the LAN.
type thumbCache struct {
	mu      sync.Mutex
	entries map[string]*thumbEntry
	max     int
	urlTTL  time.Duration
}

func newThumbCache(max int, urlTTL time.Duration) *thumbCache {
	return &thumbCache{
		entries: make(map[string]*thumbEntry),
		max:     max,
		urlTTL:  urlTTL,
	}
}

// putMeta records the status and signed URL (e.g. from the combined classify
// query), preserving any already-cached image bytes.
func (c *thumbCache) putMeta(cvID, status, url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.ensureLocked(cvID)
	e.status = status
	if url != "" {
		e.url = url
		e.urlAt = time.Now()
	}
	e.updated = time.Now()
}

// putImage records fetched image bytes.
func (c *thumbCache) putImage(cvID string, png []byte, ctype string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.ensureLocked(cvID)
	e.png = png
	e.ctype = ctype
	e.updated = time.Now()
}

// image returns cached bytes if present.
func (c *thumbCache) image(cvID string) (png []byte, ctype string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[cvID]
	if e == nil || e.png == nil {
		return nil, "", false
	}
	e.updated = time.Now()
	return e.png, e.ctype, true
}

// meta returns the cached status and signed URL. urlFresh reports whether the
// URL is non-empty and within the TTL; ok reports whether anything is cached.
func (c *thumbCache) meta(cvID string) (status, url string, urlFresh, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[cvID]
	if e == nil {
		return "", "", false, false
	}
	e.updated = time.Now()
	fresh := e.url != "" && time.Since(e.urlAt) < c.urlTTL
	return e.status, e.url, fresh, true
}

// ensureLocked returns the entry for cvID, creating it (and evicting the
// least-recently-used entry if at capacity) when absent. Caller holds mu.
func (c *thumbCache) ensureLocked(cvID string) *thumbEntry {
	if e := c.entries[cvID]; e != nil {
		return e
	}
	if c.max > 0 && len(c.entries) >= c.max {
		c.evictOldestLocked()
	}
	e := &thumbEntry{updated: time.Now()}
	c.entries[cvID] = e
	return e
}

// evictOldestLocked deletes the least-recently-updated entry. Caller holds mu.
func (c *thumbCache) evictOldestLocked() {
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if oldestKey == "" || e.updated.Before(oldest) {
			oldestKey, oldest = k, e.updated
		}
	}
	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
