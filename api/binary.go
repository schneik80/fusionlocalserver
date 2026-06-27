package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// DesignBinary identifies the native source file of a design via its version
// URN. MFGDM's DesignItem.binary { id } returns a Data Management version URN
// (e.g. "urn:adsk.wipprod:fs.file:vf.<lineage>?version=5"); the actual bytes are
// fetched through the Data Management API (see datamanagement.go), not MFGDM —
// MFGDM exposes no download URL.
type DesignBinary struct {
	VersionURN string
}

// GetDesignBinary resolves an item's native-file version URN via MFGDM's
// DesignItem.binary { id }. hubID/itemID are the MFGDM (workspace) ids the rest
// of the app uses; the returned VersionURN is a Data Management version id.
//
// NOTE: selecting binary requires the item's own ids to be valid workspace URNs
// (the same ones the Details query uses). Do NOT also select `project` on the
// item in the same query — that field's resolver errors on these hubs ("Invalid
// Project ID … use projectByDataManagementAPIId"). The DM project id needed for
// the download comes from the caller's navigation context instead.
func GetDesignBinary(ctx context.Context, token, hubID, itemID string) (*DesignBinary, error) {
	if hubID == "" || itemID == "" {
		return nil, fmt.Errorf("design binary: empty hubID or itemID")
	}
	const q = `
		query GetDesignBinary($hubId: ID!, $itemId: ID!) {
			item(hubId: $hubId, itemId: $itemId) {
				__typename
				name
				... on DesignItem { binary { id } }
				... on ConfiguredDesignItem { binary { id } }
				... on DrawingItem { binary { id } }
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID, "itemId": itemID})
	if err != nil {
		return nil, fmt.Errorf("design binary: %w", err)
	}
	var raw struct {
		Item struct {
			Typename string `json:"__typename"`
			Name     string `json:"name"`
			Binary   *struct {
				ID string `json:"id"`
			} `json:"binary"`
		} `json:"item"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("design binary decode: %w", err)
	}
	if raw.Item.Binary == nil || raw.Item.Binary.ID == "" {
		return nil, fmt.Errorf("design binary: item %q (%s) has no binary version", raw.Item.Name, raw.Item.Typename)
	}
	return &DesignBinary{VersionURN: raw.Item.Binary.ID}, nil
}
