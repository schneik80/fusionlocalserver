package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Wiki pages are plain markdown files stored in a project-root "Wiki" folder in
// Fusion Team. Everything here stays in Data-Management REST id space, keyed off
// the DM hub id (hub alternativeIdentifier) and DM project id (project
// alternativeIdentifier) the caller already holds. That sidesteps the MFGDM
// GraphQL folder id ↔ Data-Management folder id mismatch: an upload/download
// needs DM folder / item / storage ids, so we resolve the folder tree, list, and
// fetch bytes all through data/v1 + oss/v2 rather than mixing id spaces.
//
// Phase 1 is read-only (list + download); publishing lives in wiki_publish.go.

// WikiFolderName is the project-root folder that holds published wiki pages.
const WikiFolderName = "Wiki"

// wikiExt is the file extension a page is stored under.
const wikiExt = ".md"

// WikiPage is one published markdown page — a .md item in the project's Wiki
// folder. TipVersion is the current version urn, used both to fetch bytes and as
// the base-version token a draft records for stale-publish detection.
type WikiPage struct {
	ItemID     string // DM item lineage urn (also the MFGDM item id)
	Name       string // file name, e.g. "getting-started.md"
	TipVersion string // tip version urn
	ModifiedOn string // RFC3339 as returned by DM (passed through untouched)
	ModifiedBy string
}

// dmEntity is a JSON:API resource object as returned by the Data Management API
// for folders and items (only the fields the wiki needs are decoded).
type dmEntity struct {
	Type       string `json:"type"` // "folders" | "items"
	ID         string `json:"id"`
	Attributes struct {
		Name                 string `json:"name"`
		DisplayName          string `json:"displayName"`
		LastModifiedTime     string `json:"lastModifiedTime"`
		LastModifiedUserName string `json:"lastModifiedUserName"`
		Hidden               bool   `json:"hidden"` // deleted entries linger as hidden
	} `json:"attributes"`
	Relationships struct {
		Tip struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"tip"`
	} `json:"relationships"`
}

// name returns the entity's display name, falling back to its raw name.
func (e dmEntity) name() string {
	if e.Attributes.DisplayName != "" {
		return e.Attributes.DisplayName
	}
	return e.Attributes.Name
}

// GetHubDataManagementID resolves a MFGDM hub id to its Data-Management hub id
// (the "b."-prefixed id data/v1 and project/v1 URLs expect). The wiki endpoints
// take the GraphQL hub id like every other route, so we translate here rather
// than plumbing a second id through the whole frontend.
func GetHubDataManagementID(ctx context.Context, token, hubID string) (string, error) {
	const q = `
		query HubDMID($hubId: ID!) {
			hub(hubId: $hubId) {
				alternativeIdentifiers { dataManagementAPIHubId }
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID})
	if err != nil {
		return "", err
	}
	var r struct {
		Hub struct {
			AlternativeIdentifiers struct {
				DataManagementAPIHubID string `json:"dataManagementAPIHubId"`
			} `json:"alternativeIdentifiers"`
		} `json:"hub"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("hub dm id decode: %w", err)
	}
	id := r.Hub.AlternativeIdentifiers.DataManagementAPIHubID
	if id == "" {
		return "", fmt.Errorf("hub %s has no data management id", hubID)
	}
	return id, nil
}

// dmTopFolders returns a project's top-level folders (project/v1). For a Fusion
// Team project this is the single root folder under which everything else — and
// our Wiki folder — lives.
func dmTopFolders(ctx context.Context, token, dmHubID, dmProjectID string) ([]dmEntity, error) {
	u := fmt.Sprintf("%s/project/v1/hubs/%s/projects/%s/topFolders",
		dmBaseURL, dmEscape(dmHubID), dmEscape(dmProjectID))
	body, err := dmGet(ctx, token, u)
	if err != nil {
		return nil, fmt.Errorf("top folders: %w", err)
	}
	var doc struct {
		Data []dmEntity `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("top folders decode: %w", err)
	}
	return doc.Data, nil
}

// dmFolderContents lists a folder's immediate children (data/v1), following
// pagination links so a large folder is fully enumerated. Pages are requested
// at 100 entries (DM defaults to 200) to keep each verbose JSON:API response
// comfortably inside dmGet's body cap; the next links preserve the size.
func dmFolderContents(ctx context.Context, token, dmProjectID, folderID string) ([]dmEntity, error) {
	next := fmt.Sprintf("%s/data/v1/projects/%s/folders/%s/contents?page%%5Blimit%%5D=100",
		dmBaseURL, dmEscape(dmProjectID), dmEscape(folderID))
	var out []dmEntity
	for next != "" {
		body, err := dmGet(ctx, token, next)
		if err != nil {
			return nil, fmt.Errorf("folder contents: %w", err)
		}
		var doc struct {
			Data  []dmEntity `json:"data"`
			Links struct {
				Next struct {
					Href string `json:"href"`
				} `json:"next"`
			} `json:"links"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("folder contents decode: %w", err)
		}
		out = append(out, doc.Data...)
		next = doc.Links.Next.Href
	}
	return out, nil
}

// resolveWikiFolder finds the project root folder and, within it, the "Wiki"
// folder. wikiID is empty (with a nil error) when no Wiki folder exists yet — a
// project simply has no pages until one is published. rootID is always returned
// so a caller that wants to create the folder knows where to put it.
func resolveWikiFolder(ctx context.Context, token, dmHubID, dmProjectID string) (rootID, wikiID string, err error) {
	tops, err := dmTopFolders(ctx, token, dmHubID, dmProjectID)
	if err != nil {
		return "", "", err
	}
	if len(tops) == 0 {
		return "", "", fmt.Errorf("project has no root folder")
	}
	rootID = tops[0].ID
	children, err := dmFolderContents(ctx, token, dmProjectID, rootID)
	if err != nil {
		return "", "", err
	}
	for _, c := range children {
		if c.Type == "folders" && strings.EqualFold(c.name(), WikiFolderName) {
			return rootID, c.ID, nil
		}
	}
	return rootID, "", nil
}

// ListWikiPages returns the markdown pages published in a project's Wiki folder,
// resolving the folder from the DM hub + project ids. An absent Wiki folder
// yields an empty slice, not an error.
func ListWikiPages(ctx context.Context, token, dmHubID, dmProjectID string) ([]WikiPage, error) {
	_, wikiID, err := resolveWikiFolder(ctx, token, dmHubID, dmProjectID)
	if err != nil {
		return nil, err
	}
	if wikiID == "" {
		return []WikiPage{}, nil
	}
	entries, err := dmFolderContents(ctx, token, dmProjectID, wikiID)
	if err != nil {
		return nil, err
	}
	pages := make([]WikiPage, 0, len(entries))
	for _, e := range entries {
		if e.Type != "items" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.name()), wikiExt) {
			continue
		}
		pages = append(pages, WikiPage{
			ItemID:     e.ID,
			Name:       e.name(),
			TipVersion: e.Relationships.Tip.Data.ID,
			ModifiedOn: e.Attributes.LastModifiedTime,
			ModifiedBy: e.Attributes.LastModifiedUserName,
		})
	}
	return pages, nil
}

// DownloadWikiPage fetches the markdown text of a page's tip version. The item
// lineage id is the same urn MFGDM returns, so the caller passes the item id it
// listed. The path is: item tip → version storage object → OSS signed S3
// download → bytes.
func DownloadWikiPage(ctx context.Context, token, dmProjectID, itemID string) (string, error) {
	versionURN, err := GetItemTipVersion(ctx, token, dmProjectID, itemID)
	if err != nil {
		return "", err
	}
	storageURN, err := versionStorageURN(ctx, token, dmProjectID, versionURN)
	if err != nil {
		return "", err
	}
	return downloadOSSObject(ctx, token, storageURN)
}

// versionStorageURN reads a version's OSS storage object urn
// (urn:adsk.objects:os.object:<bucket>/<object>) from data/v1. The stored file
// name is read too (see versionStorage in files.go); wiki callers only need the
// urn.
func versionStorageURN(ctx context.Context, token, dmProjectID, versionURN string) (string, error) {
	urn, _, err := versionStorage(ctx, token, dmProjectID, versionURN)
	return urn, err
}

// parseOSSObjectURN splits an OSS object urn
// (urn:adsk.objects:os.object:<bucket>/<objectKey>) into its bucket and object
// key. The object key may itself contain '/', so only the first separator is
// split on.
func parseOSSObjectURN(urn string) (bucket, object string, ok bool) {
	const marker = "os.object:"
	i := strings.Index(urn, marker)
	if i < 0 {
		return "", "", false
	}
	rest := urn[i+len(marker):]
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", "", false
	}
	bucket = rest[:slash]
	object = rest[slash+1:]
	return bucket, object, bucket != "" && object != ""
}

// downloadOSSObject fetches an OSS object's bytes as a string (used for markdown
// pages). Downloading an object we can read requires only data:read.
func downloadOSSObject(ctx context.Context, token, storageURN string) (string, error) {
	data, _, err := downloadOSSObjectBytes(ctx, token, storageURN)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// downloadOSSObjectBytes fetches an OSS object via a signed S3 download URL and
// returns its bytes and the content type S3 reports (set at upload time). Capped
// at 32 MiB so a wiki image can't blow up the handler. For streaming a file of
// arbitrary size to the browser (with Range support), see OpenFile.
func downloadOSSObjectBytes(ctx context.Context, token, storageURN string) ([]byte, string, error) {
	signedURL, err := signedS3DownloadURL(ctx, token, storageURN)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := httpClient.Do(req) // signed URL carries its own auth; no Bearer header
	if err != nil {
		return nil, "", fmt.Errorf("download object: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download object -> HTTP %d", resp.StatusCode)
	}
	return data, resp.Header.Get("Content-Type"), nil
}
