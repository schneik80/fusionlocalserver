package server

import (
	"testing"
	"time"
)

func TestThumbCache_MetaAndImage(t *testing.T) {
	c := newThumbCache(16, time.Minute)

	// Absent.
	if _, _, _, ok := c.meta("cv-1"); ok {
		t.Fatal("meta ok=true for absent key")
	}
	if _, _, ok := c.image("cv-1"); ok {
		t.Fatal("image ok=true for absent key")
	}

	c.putMeta("cv-1", "SUCCESS", "https://signed.example/t.png")
	status, url, fresh, ok := c.meta("cv-1")
	if !ok || status != "SUCCESS" || url == "" || !fresh {
		t.Fatalf("meta = (%q,%q,fresh=%v,ok=%v), want SUCCESS/url/fresh/ok", status, url, fresh, ok)
	}

	// Image bytes absent until putImage.
	if _, _, ok := c.image("cv-1"); ok {
		t.Fatal("image present before putImage")
	}
	c.putImage("cv-1", []byte("PNGDATA"), "image/png")
	png, ctype, ok := c.image("cv-1")
	if !ok || string(png) != "PNGDATA" || ctype != "image/png" {
		t.Fatalf("image = (%q,%q,ok=%v)", png, ctype, ok)
	}

	// putMeta must preserve already-cached bytes.
	c.putMeta("cv-1", "SUCCESS", "https://signed.example/t2.png")
	if png, _, ok := c.image("cv-1"); !ok || string(png) != "PNGDATA" {
		t.Fatalf("bytes lost after putMeta: ok=%v png=%q", ok, png)
	}
}

func TestThumbCache_Eviction(t *testing.T) {
	c := newThumbCache(2, time.Minute)
	c.putMeta("a", "SUCCESS", "u")
	time.Sleep(2 * time.Millisecond)
	c.putMeta("b", "SUCCESS", "u")
	time.Sleep(2 * time.Millisecond)
	// Touch "a" so "b" is now the least-recently-updated.
	c.putMeta("a", "SUCCESS", "u")
	time.Sleep(2 * time.Millisecond)
	c.putMeta("c", "SUCCESS", "u") // over capacity → evict oldest ("b")

	if _, _, _, ok := c.meta("b"); ok {
		t.Error("expected 'b' to be evicted")
	}
	if _, _, _, ok := c.meta("a"); !ok {
		t.Error("expected 'a' to survive")
	}
	if _, _, _, ok := c.meta("c"); !ok {
		t.Error("expected 'c' to be present")
	}
}

func TestThumbCache_URLStaleness(t *testing.T) {
	c := newThumbCache(16, time.Millisecond)
	c.putMeta("cv-1", "SUCCESS", "https://signed.example/t.png")
	time.Sleep(5 * time.Millisecond)
	_, _, fresh, ok := c.meta("cv-1")
	if !ok {
		t.Fatal("entry missing")
	}
	if fresh {
		t.Error("URL should be stale after TTL elapsed")
	}
}
