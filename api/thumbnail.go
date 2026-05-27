package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Thumbnail status values returned by the v3 Manufacturing Data Model API
// (IN_PROGRESS | SUCCESS | PENDING | FAILED | TIMEOUT). Generation is
// asynchronous: the first call on a freshly-saved component may report
// PENDING/IN_PROGRESS with an empty URL.
const (
	ThumbnailStatusPending = "PENDING"
	ThumbnailStatusSuccess = "SUCCESS"
	ThumbnailStatusFailed  = "FAILED"
)

// GetThumbnail asks the v3 Manufacturing Data Model API for the thumbnail of a
// component. componentID is the v3 Component id. Returns the generation status
// and, once status is SUCCESS, a signed download URL (empty otherwise). Callers
// should poll until status is SUCCESS or FAILED.
//
// The signed URL is self-authenticated (the signature is embedded in the URL),
// so it can be loaded directly as an <img> src without the bearer token.
func GetThumbnail(ctx context.Context, token, componentID string) (status, signedURL string, err error) {
	if componentID == "" {
		return "", "", fmt.Errorf("thumbnail: empty componentID")
	}

	const q = `
		query GetThumbnail($cv: ID!) {
			component(componentId: $cv) {
				thumbnail {
					status
					signedUrl
				}
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentID})
	if err != nil {
		return "", "", fmt.Errorf("thumbnail: %w", err)
	}

	var raw struct {
		Component struct {
			Thumbnail struct {
				Status    string `json:"status"`
				SignedURL string `json:"signedUrl"`
			} `json:"thumbnail"`
		} `json:"component"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", "", fmt.Errorf("thumbnail decode: %w", err)
	}
	t := raw.Component.Thumbnail
	return normalizeThumbStatus(t.Status), t.SignedURL, nil
}

// normalizeThumbStatus upper-cases the status, maps the empty/absent case (a
// nullable component/thumbnail with no GraphQL error) to FAILED, and treats the
// v3 TIMEOUT status as terminal failure — so a poller never waits forever for a
// SUCCESS that will never arrive.
func normalizeThumbStatus(s string) string {
	s = strings.ToUpper(s)
	if s == "" || s == "TIMEOUT" {
		return ThumbnailStatusFailed
	}
	return s
}

// ClassifyAndThumbnail issues a single component query that both classifies the
// design (assembly vs part, via the v3 Component.hasChildren flag) and fetches
// its thumbnail status/URL. The server fires one of these per design row as a
// folder loads, so combining them halves the per-row APS round trips and lets
// the thumbnail cache warm in the background off the same call. componentID is
// the v3 Component id.
//
// Shares classifySem with ClassifyAssembly so the combined fan-out stays within
// the same concurrency budget.
func ClassifyAndThumbnail(ctx context.Context, token, componentID string) (isAssembly bool, status, signedURL string, err error) {
	if componentID == "" {
		return false, "", "", fmt.Errorf("classify+thumbnail: empty componentID")
	}
	select {
	case classifySem <- struct{}{}:
	case <-ctx.Done():
		return false, "", "", ctx.Err()
	}
	defer func() { <-classifySem }()

	const q = `
		query ClassifyAndThumbnail($cv: ID!) {
			component(componentId: $cv) {
				hasChildren
				thumbnail {
					status
					signedUrl
				}
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentID})
	if err != nil {
		return false, "", "", err
	}
	var raw struct {
		Component struct {
			HasChildren bool `json:"hasChildren"`
			Thumbnail   struct {
				Status    string `json:"status"`
				SignedURL string `json:"signedUrl"`
			} `json:"thumbnail"`
		} `json:"component"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return false, "", "", fmt.Errorf("classify+thumbnail decode: %w", err)
	}
	c := raw.Component
	return c.HasChildren,
		normalizeThumbStatus(c.Thumbnail.Status),
		c.Thumbnail.SignedURL,
		nil
}

// maxThumbnailBytes caps a thumbnail download so a hostile/oversized URL can't
// exhaust memory. APS thumbnails are small PNGs (tens of KB).
const maxThumbnailBytes = 8 << 20 // 8 MiB

// FetchThumbnailImage downloads the bytes at a thumbnail's signed URL. Like
// DownloadFile, it sends no bearer token — APS signed URLs are
// self-authenticated, so attaching the token would leak it to the URL's host.
func FetchThumbnailImage(ctx context.Context, url string) (data []byte, contentType string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("thumbnail image HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxThumbnailBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxThumbnailBytes {
		return nil, "", fmt.Errorf("thumbnail image exceeds %d bytes", maxThumbnailBytes)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}
