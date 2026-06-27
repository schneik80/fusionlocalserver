package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxDesignBytes caps a native-file download so a hostile or runaway signed URL
// cannot exhaust disk. Fusion .f3z assembly bundles are large (hundreds of MB
// to low GB); 2 GiB covers the real-world corpus with headroom.
const MaxDesignBytes = 2 << 30 // 2 GiB

// DownloadFileToPath streams the bytes at a signed URL to destPath. Like
// FetchThumbnailImage, it sends NO bearer token — APS signed URLs are
// self-authenticated, so attaching the token would leak it to the URL's host.
//
// The body is written to a temp file in destPath's directory and atomically
// renamed into place on success, so a cancelled or failed download never leaves
// a partial file that a later run would mistake for a complete cache entry. The
// transfer is bounded by MaxDesignBytes and cancellable via ctx.
func DownloadFileToPath(ctx context.Context, url, destPath string) (written int64, err error) {
	if url == "" {
		return 0, fmt.Errorf("download: empty url")
	}
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("download: mkdir %s: %w", dir, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return 0, fmt.Errorf("download HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tmp, err := os.CreateTemp(dir, ".download-*.part")
	if err != nil {
		return 0, fmt.Errorf("download: temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Clean up the temp file on any failure path; harmless no-op once renamed.
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	// LimitReader caps at MaxDesignBytes+1 so we can detect an oversized body
	// rather than silently truncating it.
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, MaxDesignBytes+1))
	if err != nil {
		return 0, fmt.Errorf("download: copy: %w", err)
	}
	if n > MaxDesignBytes {
		return 0, fmt.Errorf("download: file exceeds %d bytes", MaxDesignBytes)
	}
	if err = tmp.Sync(); err != nil {
		return 0, fmt.Errorf("download: sync: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return 0, fmt.Errorf("download: close: %w", err)
	}
	if err = os.Rename(tmpName, destPath); err != nil {
		return 0, fmt.Errorf("download: rename into place: %w", err)
	}
	return n, nil
}
