package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// SearchHit is one result from a hub search, flattened for the UI. ItemID +
// HubID identify the navigable document (so a row can drive Show-in-Location);
// ThumbnailURL is a direct signed image URL (may be empty); Matched is the
// matched text for a property search (empty for free-text).
type SearchHit struct {
	Name         string
	Score        float64
	ThumbnailURL string
	Matched      string
	ItemID       string
	HubID        string
	Kind         string // design | drawing | folder | unknown
}

// SearchableProperty is one property a hub allows searching/filtering on. ID is
// the propertyDefinition id to pass back as a search field.
type SearchableProperty struct {
	DisplayName string
	ID          string
}

// GetSearchableProperties returns the properties available for property-based
// search in the given hub (used to populate the search form's property picker).
func GetSearchableProperties(ctx context.Context, token, hubID string) ([]SearchableProperty, error) {
	const q = `
		query SearchableProps($hubId: ID!) {
			searchablePropertiesByHub(hubId: $hubId, pagination: { limit: 50 }) {
				results {
					displayName
					propertyDefinition { id }
				}
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID})
	if err != nil {
		return nil, fmt.Errorf("searchable properties: %w", err)
	}
	var raw struct {
		SearchablePropertiesByHub struct {
			Results []struct {
				DisplayName        string `json:"displayName"`
				PropertyDefinition struct {
					ID string `json:"id"`
				} `json:"propertyDefinition"`
			} `json:"results"`
		} `json:"searchablePropertiesByHub"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("searchable properties decode: %w", err)
	}
	out := make([]SearchableProperty, 0, len(raw.SearchablePropertiesByHub.Results))
	for _, r := range raw.SearchablePropertiesByHub.Results {
		if r.PropertyDefinition.ID == "" {
			continue
		}
		out = append(out, SearchableProperty{DisplayName: r.DisplayName, ID: r.PropertyDefinition.ID})
	}
	return out, nil
}

// SearchByHub runs a hub-wide search. Supply EITHER freeText (full-text) OR a
// propDefID+propValue pair (property search); if both are empty the result is
// empty. cursor pages results ("" for the first page). Returns the hits plus
// the next-page cursor ("" when exhausted).
func SearchByHub(ctx context.Context, token, hubID, freeText, propDefID, propValue, cursor string) ([]SearchHit, string, error) {
	crit := map[string]any{}
	switch {
	case freeText != "":
		crit["query"] = freeText
	case propDefID != "":
		crit["searchFields"] = []map[string]any{{
			"searchableProperty": propDefID,
			"PropertyQuery":      []string{propValue},
		}}
	default:
		return nil, "", nil
	}

	page := map[string]any{"limit": 25}
	if cursor != "" {
		page["cursor"] = cursor
	}

	const q = `
		query SearchByHub($hubId: ID!, $crit: SearchInput, $page: PaginationInput) {
			searchByHub(hubId: $hubId, searchCriteria: $crit, pagination: $page) {
				pagination { cursor }
				results {
					name
					score
					thumbnail { signedUrl }
					matches { matchedText }
					searchResultObject {
						__typename
						... on Component { id primaryModel { designItem { id hub { id } } } }
						... on Model { id designItem { id hub { id } } }
						... on DesignItem { id hub { id } }
						... on DrawingItem { id hub { id } }
						... on ConfiguredDesignItem { id hub { id } }
						... on BasicItem { id hub { id } }
						... on Folder { id project { hub { id } } }
					}
				}
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID, "crit": crit, "page": page})
	if err != nil {
		return nil, "", fmt.Errorf("search: %w", err)
	}

	type hubRef struct {
		ID string `json:"id"`
	}
	var raw struct {
		SearchByHub struct {
			Pagination struct {
				Cursor string `json:"cursor"`
			} `json:"pagination"`
			Results []struct {
				Name      string  `json:"name"`
				Score     float64 `json:"score"`
				Thumbnail struct {
					SignedURL string `json:"signedUrl"`
				} `json:"thumbnail"`
				Matches []struct {
					MatchedText string `json:"matchedText"`
				} `json:"matches"`
				Object struct {
					Typename     string `json:"__typename"`
					ID           string `json:"id"`
					Hub          hubRef `json:"hub"`
					PrimaryModel struct {
						DesignItem struct {
							ID  string `json:"id"`
							Hub hubRef `json:"hub"`
						} `json:"designItem"`
					} `json:"primaryModel"`
					DesignItem struct {
						ID  string `json:"id"`
						Hub hubRef `json:"hub"`
					} `json:"designItem"`
					Project struct {
						Hub hubRef `json:"hub"`
					} `json:"project"`
				} `json:"searchResultObject"`
			} `json:"results"`
		} `json:"searchByHub"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, "", fmt.Errorf("search decode: %w", err)
	}

	hits := make([]SearchHit, 0, len(raw.SearchByHub.Results))
	for _, r := range raw.SearchByHub.Results {
		o := r.Object
		var itemID, hubID, kind string
		switch o.Typename {
		case "Component":
			itemID = o.PrimaryModel.DesignItem.ID
			hubID = o.PrimaryModel.DesignItem.Hub.ID
			kind = "design"
		case "Model":
			itemID = o.DesignItem.ID
			hubID = o.DesignItem.Hub.ID
			kind = "design"
		case "DesignItem", "ConfiguredDesignItem":
			itemID = o.ID
			hubID = o.Hub.ID
			kind = "design"
		case "DrawingItem":
			itemID = o.ID
			hubID = o.Hub.ID
			kind = "drawing"
		case "Folder":
			itemID = o.ID
			hubID = o.Project.Hub.ID
			kind = "folder"
		default: // BasicItem and anything else
			itemID = o.ID
			hubID = o.Hub.ID
			kind = "unknown"
		}
		matched := ""
		if len(r.Matches) > 0 {
			matched = r.Matches[0].MatchedText
		}
		hits = append(hits, SearchHit{
			Name:         r.Name,
			Score:        r.Score,
			ThumbnailURL: r.Thumbnail.SignedURL,
			Matched:      matched,
			ItemID:       itemID,
			HubID:        hubID,
			Kind:         kind,
		})
	}
	return hits, raw.SearchByHub.Pagination.Cursor, nil
}
