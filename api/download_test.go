package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFileToPath(t *testing.T) {
	const body = "PK\x03\x04 fake f3z bytes"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A signed URL is self-authenticated: the request must carry no bearer.
		if r.Header.Get("Authorization") != "" {
			t.Errorf("download must not send Authorization header")
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "sub", "design.f3z")
	n, err := DownloadFileToPath(context.Background(), srv.URL, dest)
	if err != nil {
		t.Fatalf("DownloadFileToPath: %v", err)
	}
	if n != int64(len(body)) {
		t.Errorf("wrote %d bytes, want %d", n, len(body))
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != body {
		t.Errorf("content mismatch")
	}
	// No leftover temp .part files.
	ents, _ := os.ReadDir(filepath.Dir(dest))
	for _, e := range ents {
		if filepath.Ext(e.Name()) == ".part" {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestDownloadFileToPath_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "design.f3z")
	if _, err := DownloadFileToPath(context.Background(), srv.URL, dest); err == nil {
		t.Fatal("expected error on HTTP 403")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("failed download must not leave a file in place")
	}
}
