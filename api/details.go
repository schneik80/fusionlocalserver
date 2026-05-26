package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ItemDetails holds the rich metadata for a single item fetched from the API.
type ItemDetails struct {
	ID            string
	Name          string
	Typename      string // DesignItem | DrawingItem | ConfiguredDesignItem | BasicItem
	Size          string
	MimeType      string
	ExtensionType string
	FusionWebURL  string
	CreatedOn     time.Time
	CreatedBy     string
	ModifiedOn    time.Time
	ModifiedBy    string
	VersionNumber int
	// Design-specific (DesignItem / ConfiguredDesignItem)
	PartNumber  string
	PartDesc    string
	Material    string
	IsMilestone bool
	// RootComponentVersionID is the id of tipRootComponentVersion — required
	// as the componentVersionId argument when requesting a STEP derivative.
	RootComponentVersionID string
	// Version history (most recent first)
	Versions []VersionSummary
}

// VersionSummary is one entry in the version history list.
type VersionSummary struct {
	Number    int
	CreatedOn time.Time
	CreatedBy string
	Comment   string // version save comment (may be empty)
}

// GetItemDetails fetches rich metadata for a single item plus its version list.
func GetItemDetails(ctx context.Context, token, hubID, itemID string) (*ItemDetails, error) {
	const q = `
		query GetItemDetails($hubId: ID!, $itemId: ID!) {
			item(hubId: $hubId, itemId: $itemId) {
				__typename
				id
				name
				size
				mimeType
				extensionType
				createdOn
				createdBy  { firstName lastName }
				lastModifiedOn
				lastModifiedBy { firstName lastName }
				... on DesignItem {
					fusionWebUrl
					tipVersion { versionNumber }
					tipRootComponentVersion {
						id
						partNumber
						partDescription
						materialName
						isMilestone
					}
				}
				... on DrawingItem {
					fusionWebUrl
					tipVersion { versionNumber }
				}
				... on ConfiguredDesignItem {
					fusionWebUrl
					tipVersion { versionNumber }
				}
			}
			itemVersions(hubId: $hubId, itemId: $itemId) {
				results {
					versionNumber
					name
					createdOn
					createdBy { firstName lastName }
				}
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID, "itemId": itemID})
	if err != nil {
		return nil, fmt.Errorf("item details: %w", err)
	}

	var raw struct {
		Item struct {
			Typename      string  `json:"__typename"`
			ID            string  `json:"id"`
			Name          string  `json:"name"`
			Size          string  `json:"size"`
			MimeType      string  `json:"mimeType"`
			ExtensionType string  `json:"extensionType"`
			FusionWebURL  string  `json:"fusionWebUrl"`
			CreatedOn     string  `json:"createdOn"`
			CreatedBy     apiUser `json:"createdBy"`
			ModifiedOn    string  `json:"lastModifiedOn"`
			ModifiedBy    apiUser `json:"lastModifiedBy"`
			TipVersion    struct {
				VersionNumber int `json:"versionNumber"`
			} `json:"tipVersion"`
			TipRootComponentVersion struct {
				ID          string `json:"id"`
				PartNumber  string `json:"partNumber"`
				PartDesc    string `json:"partDescription"`
				Material    string `json:"materialName"`
				IsMilestone bool   `json:"isMilestone"`
			} `json:"tipRootComponentVersion"`
		} `json:"item"`
		ItemVersions struct {
			Results []struct {
				VersionNumber int     `json:"versionNumber"`
				Name          string  `json:"name"`
				CreatedOn     string  `json:"createdOn"`
				CreatedBy     apiUser `json:"createdBy"`
			} `json:"results"`
		} `json:"itemVersions"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("item details decode: %w", err)
	}

	d := &ItemDetails{
		ID:                     raw.Item.ID,
		Name:                   raw.Item.Name,
		Typename:               raw.Item.Typename,
		Size:                   raw.Item.Size,
		MimeType:               raw.Item.MimeType,
		ExtensionType:          raw.Item.ExtensionType,
		FusionWebURL:           raw.Item.FusionWebURL,
		CreatedOn:              parseTime(raw.Item.CreatedOn),
		CreatedBy:              raw.Item.CreatedBy.fullName(),
		ModifiedOn:             parseTime(raw.Item.ModifiedOn),
		ModifiedBy:             raw.Item.ModifiedBy.fullName(),
		VersionNumber:          raw.Item.TipVersion.VersionNumber,
		PartNumber:             raw.Item.TipRootComponentVersion.PartNumber,
		PartDesc:               raw.Item.TipRootComponentVersion.PartDesc,
		Material:               raw.Item.TipRootComponentVersion.Material,
		IsMilestone:            raw.Item.TipRootComponentVersion.IsMilestone,
		RootComponentVersionID: raw.Item.TipRootComponentVersion.ID,
	}

	// Versions — most recent first.
	for i := len(raw.ItemVersions.Results) - 1; i >= 0; i-- {
		v := raw.ItemVersions.Results[i]
		d.Versions = append(d.Versions, VersionSummary{
			Number:    v.VersionNumber,
			Comment:   v.Name,
			CreatedOn: parseTime(v.CreatedOn),
			CreatedBy: v.CreatedBy.fullName(),
		})
	}

	return d, nil
}

// apiUser is a helper for deserialising User objects.
type apiUser struct {
	First string `json:"firstName"`
	Last  string `json:"lastName"`
}

func (u apiUser) fullName() string {
	name := u.First
	if u.Last != "" {
		if name != "" {
			name += " "
		}
		name += u.Last
	}
	return name
}

// parseTime parses an ISO-8601 / RFC-3339 timestamp returned by the API.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, _ = time.Parse("2006-01-02T15:04:05.000Z", s)
	}
	return t
}
