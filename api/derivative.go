package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// mdBaseURL is the Model Derivative service base (US region, matching the app's
// hubs). A var only so tests can point it at an httptest server.
var mdBaseURL = "https://developer.api.autodesk.com"

// maxPreviewBytes caps a rendered preview download so a hostile/oversized URL
// can't exhaust memory. MD thumbnails up to 400x400 are well under this.
const maxPreviewBytes = 32 << 20 // 32 MiB

// GetVersionThumbnail fetches a rendered preview PNG for a Data Management
// version URN via the Model Derivative API. Fusion composite docs (e.g. .f2d
// drawings) expose no downloadable native file, but their translated derivative
// carries a thumbnail the service can render up to 400x400 — larger and crisper
// than the default MFGDM thumbnail. width/height must each be 100, 200, or 400.
//
// The URN is the source version URN (…fs.file:vf.<id>?version=N), base64url
// encoded without padding, as the Model Derivative API expects. Sends the bearer
// token; the derivative must already exist (the DM version's `derivatives`
// relationship indicates it does).
func GetVersionThumbnail(ctx context.Context, token, versionURN string, width, height int) (data []byte, contentType string, err error) {
	if versionURN == "" {
		return nil, "", fmt.Errorf("thumbnail: empty version urn")
	}
	urn := base64.RawURLEncoding.EncodeToString([]byte(versionURN))
	u := fmt.Sprintf("%s/modelderivative/v2/designdata/%s/thumbnail?width=%d&height=%d",
		mdBaseURL, urn, width, height)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, "", fmt.Errorf("MD thumbnail HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxPreviewBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxPreviewBytes {
		return nil, "", fmt.Errorf("MD thumbnail exceeds %d bytes", maxPreviewBytes)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/png"
	}
	return data, ct, nil
}
