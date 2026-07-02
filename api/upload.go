package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// General file upload into any project folder — the drag-and-drop upload
// feature. It reuses the wiki's verified Data-Management sequence (create
// storage → push bytes to OSS through signed S3 URLs → create item or version)
// but streams from an io.ReaderAt instead of buffering, splits large files into
// signed multi-part PUTs, and reports byte progress so a job view can render a
// live bar. Everything stays in DM id space (see wiki.go); the target folder is
// resolved by walking display names from the project root because MFGDM GraphQL
// folder ids don't translate to DM folder ids.

// TokenSource returns a currently-valid APS access token. Background uploads can
// outlive a single access token's lifetime, so the upload sequence asks for a
// token before each authenticated call rather than holding one string.
type TokenSource func(context.Context) (string, error)

// OSS part sizing: files up to ossPartSize go up as the single-URL PUT the wiki
// verified live; larger files split into equal parts. One signeds3upload request
// returns at most ossMaxParts URLs, so beyond ossMaxParts×ossPartSize the part
// size grows instead of the count.
const (
	ossPartSize = 64 << 20
	ossMaxParts = 25
)

// ResolveFolderPath resolves a project folder to its DM id by walking display
// names from the project's root folder. An empty path resolves to the root
// itself (a project-root upload).
func ResolveFolderPath(ctx context.Context, token, dmHubID, dmProjectID string, names []string) (string, error) {
	tops, err := dmTopFolders(ctx, token, dmHubID, dmProjectID)
	if err != nil {
		return "", err
	}
	if len(tops) == 0 {
		return "", fmt.Errorf("project has no root folder")
	}
	cur := tops[0].ID
	for _, name := range names {
		id, err := findSubfolderID(ctx, token, dmProjectID, cur, name)
		if err != nil {
			return "", err
		}
		if id == "" {
			return "", fmt.Errorf("folder %q not found in project", name)
		}
		cur = id
	}
	return cur, nil
}

// UploadFileToFolder uploads size bytes from src into folderID as filename. A
// same-named item in the folder gains a new version (matching Fusion Team's own
// upload semantics); otherwise a new item is created. progress, when non-nil,
// receives byte deltas as they reach S3. Returns the item lineage urn and the
// resulting version urn.
func UploadFileToFolder(ctx context.Context, tokenFn TokenSource, dmProjectID, folderID, filename string, src io.ReaderAt, size int64, progress func(int64)) (string, string, error) {
	token, err := tokenFn(ctx)
	if err != nil {
		return "", "", err
	}
	itemID, err := findItemByName(ctx, token, dmProjectID, folderID, filename)
	if err != nil {
		return "", "", err
	}
	storageURN, err := createStorage(ctx, token, dmProjectID, folderID, filename)
	if err != nil {
		return "", "", err
	}
	if err := uploadToOSSStream(ctx, tokenFn, storageURN, filename, src, size, progress); err != nil {
		return "", "", err
	}
	// The S3 phase can outlast the token that signed it; re-resolve before the
	// item/version creation calls.
	token, err = tokenFn(ctx)
	if err != nil {
		return "", "", err
	}
	if itemID == "" {
		return createItem(ctx, token, dmProjectID, folderID, filename, storageURN)
	}
	versionID, err := createVersion(ctx, token, dmProjectID, itemID, filename, storageURN)
	return itemID, versionID, err
}

// uploadToOSSStream pushes size bytes from src to the object's OSS storage via
// signed S3 uploads (GET signed urls → PUT each part → POST finalize). It is the
// streaming, multi-part counterpart of the wiki's buffered uploadToOSS.
func uploadToOSSStream(ctx context.Context, tokenFn TokenSource, storageURN, filename string, src io.ReaderAt, size int64, progress func(int64)) error {
	bucket, object, ok := parseOSSObjectURN(storageURN)
	if !ok {
		return fmt.Errorf("unrecognised storage urn")
	}
	partSize, parts := ossPartPlan(size)
	signURL := fmt.Sprintf("%s/oss/v2/buckets/%s/objects/%s/signeds3upload",
		dmBaseURL, dmEscape(bucket), dmEscape(object))
	token, err := tokenFn(ctx)
	if err != nil {
		return err
	}
	body, err := dmGet(ctx, token, fmt.Sprintf("%s?parts=%d", signURL, parts))
	if err != nil {
		return fmt.Errorf("sign upload: %w", err)
	}
	var s struct {
		UploadKey string   `json:"uploadKey"`
		URLs      []string `json:"urls"`
	}
	if err := json.Unmarshal(body, &s); err != nil {
		return fmt.Errorf("sign upload decode: %w", err)
	}
	if len(s.URLs) < parts || s.UploadKey == "" {
		return fmt.Errorf("sign upload: wanted %d urls, got %d", parts, len(s.URLs))
	}

	for i := 0; i < parts; i++ {
		off := int64(i) * partSize
		n := min(partSize, size-off)
		var rd io.Reader = io.NewSectionReader(src, off, n)
		if progress != nil {
			rd = &progressReader{r: rd, fn: progress}
		}
		put, err := http.NewRequestWithContext(ctx, http.MethodPut, s.URLs[i], rd)
		if err != nil {
			return err
		}
		put.ContentLength = n
		put.Header.Set("Content-Type", contentTypeForName(filename))
		presp, err := httpClient.Do(put) // signed URL carries its own auth; no Bearer header
		if err != nil {
			return fmt.Errorf("upload bytes (part %d/%d): %w", i+1, parts, err)
		}
		io.Copy(io.Discard, io.LimitReader(presp.Body, 1<<16))
		presp.Body.Close()
		if presp.StatusCode < 200 || presp.StatusCode >= 300 {
			return fmt.Errorf("upload bytes (part %d/%d) -> HTTP %d", i+1, parts, presp.StatusCode)
		}
	}

	token, err = tokenFn(ctx)
	if err != nil {
		return err
	}
	if _, err := dmPost(ctx, token, signURL, mustJSON(map[string]any{"uploadKey": s.UploadKey})); err != nil {
		return fmt.Errorf("finalize upload: %w", err)
	}
	return nil
}

// ossPartPlan sizes the signed-S3 parts for a file: one part up to ossPartSize,
// then equal ossPartSize parts, growing the part size once the count would pass
// ossMaxParts (a single sign request returns at most that many URLs).
func ossPartPlan(size int64) (partSize int64, parts int) {
	partSize = ossPartSize
	if size <= partSize {
		return partSize, 1
	}
	parts = int((size + partSize - 1) / partSize)
	if parts > ossMaxParts {
		parts = ossMaxParts
		partSize = (size + int64(parts) - 1) / int64(parts)
	}
	return partSize, parts
}

// progressReader counts bytes as an upload body is consumed, forwarding deltas
// to fn (which must tolerate being called from the transport's goroutine).
type progressReader struct {
	r  io.Reader
	fn func(int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.fn(int64(n))
	}
	return n, err
}
