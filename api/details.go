package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
	// Design-specific, read from tipRootModel -> component (v3 Property objects).
	PartNumber string
	PartDesc   string
	Material   string
	// RootComponentVersionID carries the v3 root Component id
	// (tipRootModel.component.id) — the handle the Details panel passes to the
	// thumbnail / properties / uses probes. (Name kept for cross-layer
	// compatibility; under v3 it is a Component id, not a ComponentVersion id.)
	RootComponentVersionID string
	// TipTimestamp is the time of the design's tip state (tipRootModel.timestamp).
	TipTimestamp time.Time
	// History is the time-based change log (most recent first). v3 has no
	// integer version numbers, so this replaces the old version list.
	History []HistoryEntry
}

// HistoryEntry is one entry in the v3 time-based history (a HistoryChange).
// Entries are identified by timestamp + id and labelled by change type rather
// than an integer version number.
type HistoryEntry struct {
	ID          string
	Timestamp   time.Time
	ChangeType  string // humanized __typename, e.g. "Version Created"
	Description string
	Author      string
}

// GetItemDetails fetches rich metadata for a single item plus its history.
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
					tipRootModel {
						timestamp
						component {
							id
							partNumber { value displayValue }
							description { value displayValue }
							materialName { value displayValue }
						}
					}
					history(pagination: { limit: 50 }) {
						results { __typename id description timestamp author { firstName lastName } }
					}
				}
				... on ConfiguredDesignItem {
					fusionWebUrl
					history(pagination: { limit: 50 }) {
						results { __typename id description timestamp author { firstName lastName } }
					}
				}
				... on DrawingItem {
					fusionWebUrl
					history(pagination: { limit: 50 }) {
						results { __typename id description timestamp author { firstName lastName } }
					}
				}
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID, "itemId": itemID})
	if err != nil {
		return nil, fmt.Errorf("item details: %w", err)
	}

	type histResult struct {
		Typename    string  `json:"__typename"`
		ID          string  `json:"id"`
		Description string  `json:"description"`
		Timestamp   string  `json:"timestamp"`
		Author      apiUser `json:"author"`
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
			TipRootModel  *struct {
				Timestamp string `json:"timestamp"`
				Component *struct {
					ID         string   `json:"id"`
					PartNumber Property `json:"partNumber"`
					Desc       Property `json:"description"`
					Material   Property `json:"materialName"`
				} `json:"component"`
			} `json:"tipRootModel"`
			History struct {
				Results []histResult `json:"results"`
			} `json:"history"`
		} `json:"item"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("item details decode: %w", err)
	}

	d := &ItemDetails{
		ID:            raw.Item.ID,
		Name:          raw.Item.Name,
		Typename:      raw.Item.Typename,
		Size:          raw.Item.Size,
		MimeType:      raw.Item.MimeType,
		ExtensionType: raw.Item.ExtensionType,
		FusionWebURL:  raw.Item.FusionWebURL,
		CreatedOn:     parseTime(raw.Item.CreatedOn),
		CreatedBy:     raw.Item.CreatedBy.fullName(),
		ModifiedOn:    parseTime(raw.Item.ModifiedOn),
		ModifiedBy:    raw.Item.ModifiedBy.fullName(),
	}
	if tm := raw.Item.TipRootModel; tm != nil {
		d.TipTimestamp = parseTime(tm.Timestamp)
		if c := tm.Component; c != nil {
			d.RootComponentVersionID = c.ID
			d.PartNumber = c.PartNumber.Str()
			d.PartDesc = c.Desc.Str()
			d.Material = c.Material.Str()
		}
	}

	for _, h := range raw.Item.History.Results {
		d.History = append(d.History, HistoryEntry{
			ID:          h.ID,
			Timestamp:   parseTime(h.Timestamp),
			ChangeType:  humanizeChangeType(h.Typename),
			Description: h.Description,
			Author:      h.Author.fullName(),
		})
	}
	// Most recent first (the API's order isn't guaranteed).
	sort.SliceStable(d.History, func(i, j int) bool {
		return d.History[i].Timestamp.After(d.History[j].Timestamp)
	})

	return d, nil
}

// humanizeChangeType turns a v3 HistoryChange __typename (e.g.
// "VersionCreatedHistoryChange") into a readable label ("Version Created").
func humanizeChangeType(typename string) string {
	s := strings.TrimSuffix(typename, "HistoryChange")
	if s == "" {
		return "Change"
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
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
