package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// The Data Management API (data/v1 + oss/v2) is how the native .f3d/.f3z bytes
// are actually fetched — MFGDM exposes only a version URN, no download URL. The
// chain: version URN -> the version's OSS storage object -> an OSS signed S3
// download URL -> the bytes. All calls use the same bearer token and the
// data:read scope the app already holds. Same host as MFGDM, different paths,
// so we reuse httpClient.
// dmBaseURL is a var (not const) only so tests can point it at an httptest
// server; production never reassigns it.
var dmBaseURL = "https://developer.api.autodesk.com"

// dmBaseURLForTest overrides the Data Management base URL and returns a restore
// func. Test-only.
func dmBaseURLForTest(u string) func() {
	old := dmBaseURL
	dmBaseURL = u
	return func() { dmBaseURL = old }
}

// dmEscape percent-encodes a URN for use as a single path segment, matching
// JavaScript's encodeURIComponent (which APS examples use): ':' and '?' and '='
// in version URNs must be escaped. url.PathEscape leaves ':' unescaped, so we
// use QueryEscape (URNs contain no spaces, so the '+'-for-space quirk is moot).
func dmEscape(s string) string { return url.QueryEscape(s) }

// dmGet performs an authenticated GET against the Data Management API and
// returns the response body, failing on non-2xx.
func dmGet(ctx context.Context, token, fullURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("DM GET %s -> HTTP %d: %s", trimURL(fullURL), resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// VersionDownload describes a resolved native-file download.
type VersionDownload struct {
	StorageURN string // urn:adsk.objects:os.object:<bucket>/<object>
	FileName   string // e.g. "Cylinder Cap.f3d"
}

// GetVersionDownload looks up a Data Management version and returns its OSS
// storage object URN and filename. dmProjectID is the Data Management project
// id (the "b.…" id; the app holds it as the project's altId). versionURN is the
// MFGDM binary id (a "…fs.file:vf.…?version=N" URN).
func GetVersionDownload(ctx context.Context, token, dmProjectID, versionURN string) (*VersionDownload, error) {
	if dmProjectID == "" || versionURN == "" {
		return nil, fmt.Errorf("version download: empty project or version")
	}
	u := fmt.Sprintf("%s/data/v1/projects/%s/versions/%s", dmBaseURL, dmEscape(dmProjectID), dmEscape(versionURN))
	body, err := dmGet(ctx, token, u)
	if err != nil {
		return nil, fmt.Errorf("version lookup: %w", err)
	}
	var doc struct {
		Data struct {
			Attributes struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"attributes"`
			Relationships struct {
				Storage struct {
					Data struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"storage"`
			} `json:"relationships"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("version decode: %w", err)
	}
	storage := doc.Data.Relationships.Storage.Data.ID
	if storage == "" {
		return nil, fmt.Errorf("version has no storage object (no downloadable native file)")
	}
	name := doc.Data.Attributes.Name
	if name == "" {
		name = doc.Data.Attributes.DisplayName
	}
	return &VersionDownload{StorageURN: storage, FileName: name}, nil
}

// OSSSignedDownloadURL returns a presigned S3 GET URL for an OSS storage object
// URN (urn:adsk.objects:os.object:<bucket>/<object>). The returned URL is
// self-authenticated — download it with no bearer token (see DownloadFileToPath).
func OSSSignedDownloadURL(ctx context.Context, token, storageURN string) (string, error) {
	const prefix = "urn:adsk.objects:os.object:"
	rest := strings.TrimPrefix(storageURN, prefix)
	if rest == storageURN {
		return "", fmt.Errorf("unexpected storage URN %q", storageURN)
	}
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", fmt.Errorf("storage URN missing object key: %q", storageURN)
	}
	bucket, object := rest[:slash], rest[slash+1:]
	u := fmt.Sprintf("%s/oss/v2/buckets/%s/objects/%s/signeds3download",
		dmBaseURL, dmEscape(bucket), dmEscape(object))
	body, err := dmGet(ctx, token, u)
	if err != nil {
		return "", fmt.Errorf("signed download: %w", err)
	}
	var doc struct {
		URL    string   `json:"url"`
		URLs   []string `json:"urls"`
		Status string   `json:"status"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("signed download decode: %w", err)
	}
	if doc.URL != "" {
		return doc.URL, nil
	}
	if len(doc.URLs) > 0 {
		// Multipart (large object). For now we only support single-part objects;
		// most .f3d/.f3z are well under the chunking threshold. Surface a clear
		// error rather than silently downloading one chunk.
		return "", fmt.Errorf("object is chunked into %d parts (multipart download not yet supported)", len(doc.URLs))
	}
	return "", fmt.Errorf("signed download returned no URL (status %q)", doc.Status)
}

// ResolveDesignDownloadURL is the full chain: MFGDM version URN -> DM version ->
// OSS storage object -> signed S3 URL. Returns the signed URL plus the native
// filename (carrying the real .f3d/.f3z extension).
func ResolveDesignDownloadURL(ctx context.Context, token, dmProjectID, versionURN string) (signedURL, fileName string, err error) {
	vd, err := GetVersionDownload(ctx, token, dmProjectID, versionURN)
	if err != nil {
		return "", "", err
	}
	signed, err := OSSSignedDownloadURL(ctx, token, vd.StorageURN)
	if err != nil {
		return "", "", err
	}
	return signed, vd.FileName, nil
}

// trimURL strips the query string from a URL for error messages (signed URLs
// and tokens must never be logged).
func trimURL(u string) string {
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i]
	}
	return u
}
