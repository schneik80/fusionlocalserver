package api

import (
	"context"
	"fmt"
)

// Hub browsing over the Data Management API. The MFGDM GraphQL items listing
// only surfaces content Fusion has indexed — folders and files created through
// raw Data Management uploads (the wiki's own images folders, for one) never
// appear in itemsByFolder results. The in-place hub browser must see
// *everything*, so it lists straight from data/v1 like the wiki does,
// reusing its folder/pagination plumbing (wiki.go).

// BrowseFolder lists one folder's immediate children — subfolders and items —
// for the in-place hub document/folder browser. An empty folderID means the
// project root (resolved via topFolders). Item ids are lineage urns, the same
// id space /api/items/file streams by, so a listed file can be fetched as-is.
func BrowseFolder(ctx context.Context, token, dmHubID, dmProjectID, folderID string) ([]NavItem, error) {
	if folderID == "" {
		tops, err := dmTopFolders(ctx, token, dmHubID, dmProjectID)
		if err != nil {
			return nil, err
		}
		if len(tops) == 0 {
			return nil, fmt.Errorf("project has no root folder")
		}
		folderID = tops[0].ID
	}
	entries, err := dmFolderContents(ctx, token, dmProjectID, folderID)
	if err != nil {
		return nil, err
	}
	items := make([]NavItem, 0, len(entries))
	for _, e := range entries {
		if e.Attributes.Hidden {
			continue
		}
		switch e.Type {
		case "folders":
			items = append(items, NavItem{
				ID:          e.ID,
				Name:        e.name(),
				Kind:        "folder",
				IsContainer: true,
			})
		case "items":
			n := NavItem{
				ID:         e.ID,
				Name:       e.name(),
				Kind:       "unknown",
				ModifiedOn: parseTime(e.Attributes.LastModifiedTime),
			}
			// Recover a Fusion design/drawing/electronics kind from the file
			// extension so native documents keep their icons; plain uploads
			// (images, PDFs, …) stay "unknown" and are labelled by extension
			// in the UI.
			if k := kindFromExtension(e.name()); k != "" {
				n.Kind = k
				if n.Kind == "drawing" {
					n.Subtype = drawingSubtypeFromExtension(e.name())
				}
			}
			items = append(items, n)
		}
	}
	return items, nil
}
