package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// File viewing serves the raw bytes of an uploaded (non-native) file's tip
// version to the browser so the SPA can preview images, PDFs, video and text.
// It reuses the wiki download plumbing (item tip -> version storage object ->
// signed S3 GET) but streams the bytes rather than buffering them, forwarding
// the browser's Range header so large video/PDF can seek without pulling the
// whole object through the server. Fusion-native designs and drawings have no
// downloadable OSS storage, so this only serves generic uploaded files.

// OpenFile resolves an item's tip file and opens an HTTP response streaming its
// bytes from OSS. rangeHeader, when non-empty, is forwarded to S3 so the caller
// can serve 206 partial content (video seeking, progressive PDF), mirroring the
// upstream Content-Range. The caller owns resp.Body and must Close it. name is
// the stored file name, for the caller's Content-Disposition / type derivation.
func OpenFile(ctx context.Context, token, dmProjectID, itemID, rangeHeader string) (resp *http.Response, name string, err error) {
	versionURN, err := GetItemTipVersion(ctx, token, dmProjectID, itemID)
	if err != nil {
		return nil, "", err
	}
	storageURN, name, err := versionStorage(ctx, token, dmProjectID, versionURN)
	if err != nil {
		return nil, "", err
	}
	signedURL, err := signedS3DownloadURL(ctx, token, storageURN)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, "", err
	}
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	resp, err = httpClient.Do(req) // signed URL carries its own auth; no Bearer header
	if err != nil {
		return nil, "", fmt.Errorf("download object: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, "", fmt.Errorf("download object -> HTTP %d", resp.StatusCode)
	}
	return resp, name, nil
}

// versionStorage reads a version's OSS storage object urn
// (urn:adsk.objects:os.object:<bucket>/<object>) and stored file name from
// data/v1 in a single request.
func versionStorage(ctx context.Context, token, dmProjectID, versionURN string) (storageURN, name string, err error) {
	u := fmt.Sprintf("%s/data/v1/projects/%s/versions/%s",
		dmBaseURL, dmEscape(dmProjectID), dmEscape(versionURN))
	body, err := dmGet(ctx, token, u)
	if err != nil {
		return "", "", fmt.Errorf("version storage: %w", err)
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
		return "", "", fmt.Errorf("version storage decode: %w", err)
	}
	storageURN = doc.Data.Relationships.Storage.Data.ID
	if storageURN == "" {
		return "", "", fmt.Errorf("version %s has no storage object", trimURL(versionURN))
	}
	name = doc.Data.Attributes.DisplayName
	if name == "" {
		name = doc.Data.Attributes.Name
	}
	return storageURN, name, nil
}

// signedS3DownloadURL asks OSS for a short-lived signed S3 GET url for a storage
// object. The returned url is self-authenticated (carries its own signature; no
// bearer token is sent when fetching it).
func signedS3DownloadURL(ctx context.Context, token, storageURN string) (string, error) {
	bucket, object, ok := parseOSSObjectURN(storageURN)
	if !ok {
		return "", fmt.Errorf("unrecognised storage urn")
	}
	signURL := fmt.Sprintf("%s/oss/v2/buckets/%s/objects/%s/signeds3download",
		dmBaseURL, dmEscape(bucket), dmEscape(object))
	body, err := dmGet(ctx, token, signURL)
	if err != nil {
		return "", fmt.Errorf("sign download: %w", err)
	}
	var doc struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("sign download decode: %w", err)
	}
	if doc.URL == "" {
		return "", fmt.Errorf("sign download: no url in response")
	}
	return doc.URL, nil
}

// ContentTypeForName returns the MIME type a file should be served under,
// derived from its name. Exported so the file handler can label streamed bytes
// correctly (e.g. video/mp4, application/pdf) regardless of what OSS recorded at
// upload time.
func ContentTypeForName(name string) string { return contentTypeForName(name) }
