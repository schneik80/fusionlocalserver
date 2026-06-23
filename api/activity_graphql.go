package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// GraphQL-backed activity acquisition. The first-party Fusion Team notifications
// feed (api/activity.go) rejects this app's token with HTTP 500, so design-scope
// activity is sourced from the Manufacturing Data Model GraphQL instead — the
// same itemVersions history the Details panel already uses, under the token that
// works everywhere else. Each version becomes one ActivityEvent, so the existing
// BuildReport aggregation/DTO/UI layers consume it unchanged.
//
// Unlike the feed, GraphQL ids are the DM/GraphQL ids the browser nav uses:
// hubID is the GraphQL hub id (not the feed slug) and itemID is the lineage urn.

// GetDesignActivity fetches a single design's version history via GraphQL and
// normalizes each version into an ActivityEvent. hubID is the GraphQL hub id and
// itemID is the item (lineage) id, exactly as the Details endpoint takes them.
//
// It uses a lean query — base item fields plus itemVersions — deliberately
// NOT requesting tipRootComponentVersion property fields. Those properties fail
// with "Individual property data … not yet available in the Fusion Cloud
// Information Model" for items not yet migrated, which would nullify the whole
// query; activity reports don't need them.
func GetDesignActivity(ctx context.Context, token, hubID, itemID string) ([]ActivityEvent, error) {
	// Deliberately minimal. fusionWebUrl is omitted: the MFG service intermittently
	// returns INTERNAL_SERVER_ERROR for that one field (path ["item","fusionWebUrl"],
	// errorType UNKNOWN, depth 2), which gqlQuery treats as retriable and then fails
	// the whole call — discarding the otherwise-complete item + itemVersions data.
	// The report doesn't need the web URL.
	const q = `
		query DesignActivity($hubId: ID!, $itemId: ID!) {
			item(hubId: $hubId, itemId: $itemId) {
				__typename
				id
				name
				extensionType
				createdOn
				createdBy { firstName lastName }
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
		return nil, fmt.Errorf("design activity: %w", err)
	}

	var raw struct {
		Item struct {
			Typename      string  `json:"__typename"`
			ID            string  `json:"id"`
			Name          string  `json:"name"`
			ExtensionType string  `json:"extensionType"`
			CreatedOn     string  `json:"createdOn"`
			CreatedBy     apiUser `json:"createdBy"`
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
		return nil, fmt.Errorf("design activity decode: %w", err)
	}

	d := &ItemDetails{
		ID:            raw.Item.ID,
		Name:          raw.Item.Name,
		Typename:      raw.Item.Typename,
		ExtensionType: raw.Item.ExtensionType,
		CreatedOn:     parseTime(raw.Item.CreatedOn),
		CreatedBy:     raw.Item.CreatedBy.fullName(),
	}
	for _, v := range raw.ItemVersions.Results {
		d.Versions = append(d.Versions, VersionSummary{
			Number:    v.VersionNumber,
			Comment:   v.Name,
			CreatedOn: parseTime(v.CreatedOn),
			CreatedBy: v.CreatedBy.fullName(),
		})
	}
	return designEventsFromDetails(d, hubID), nil
}

// designEventsFromDetails maps a design's version list to activity events. It is
// pure (no I/O) so the normalization is unit-testable. Events carry the lineage
// urn and the GraphQL hub id so inScope(design)/aggregation match on either id.
func designEventsFromDetails(d *ItemDetails, hubID string) []ActivityEvent {
	if d == nil {
		return nil
	}
	events := make([]ActivityEvent, 0, len(d.Versions))
	for _, v := range d.Versions {
		action := ActionUpdated
		if v.Number <= 1 {
			action = ActionCreated
		}
		events = append(events, ActivityEvent{
			EntityType:    "design",
			EntityID:      d.ID,
			EntityName:    d.Name,
			Timestamp:     v.CreatedOn,
			Action:        action,
			Actor:         Actor{DisplayName: v.CreatedBy},
			VersionNumber: v.Number,
			HubID:         hubID,
			LineageURN:    d.ID,
			FileType:      d.ExtensionType,
			WebURL:        d.FusionWebURL,
			CreatedOn:     d.CreatedOn,
			Owner:         Actor{DisplayName: d.CreatedBy},
			Detail:        v.Comment,
			Source:        "graphql",
		})
	}
	return events
}
