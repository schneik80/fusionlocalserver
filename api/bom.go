package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// BOMRow is one line of a design's bill of materials: a direct child component
// and its quantity. v3 exposes a real quantity on each BOM relation (unlike v2,
// which had to count occurrences). ComponentVersionID carries the child's v3
// Component id.
type BOMRow struct {
	ComponentVersionID string
	Name               string
	PartNumber         string
	PartDesc           string
	Material           string
	Quantity           int
}

// GetBOM returns the immediate bill of materials for the given component: one
// row per direct child with its quantity, from Component.bomRelations.
// componentID is the v3 Component id. v3's bomRelations supports depth 1 only,
// so this is the immediate-children BOM (with real quantities), not a fully
// flattened multi-level tree.
func GetBOM(ctx context.Context, token, componentID string) ([]BOMRow, error) {
	const qFirst = `
		query GetBOM($cv: ID!) {
			component(componentId: $cv) {
				bomRelations(depth: 1, pagination: { limit: 50 }) {
					pagination { cursor }
					results {
						quantity
						toComponent {
							id
							name { value displayValue }
							partNumber { value displayValue }
							description { value displayValue }
							materialName { value displayValue }
						}
					}
				}
			}
		}`
	const qNext = `
		query GetBOMNext($cv: ID!, $cursor: String!) {
			component(componentId: $cv) {
				bomRelations(depth: 1, pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results {
						quantity
						toComponent {
							id
							name { value displayValue }
							partNumber { value displayValue }
							description { value displayValue }
							materialName { value displayValue }
						}
					}
				}
			}
		}`

	type bomResult struct {
		Quantity    int `json:"quantity"`
		ToComponent struct {
			ID         string   `json:"id"`
			Name       Property `json:"name"`
			PartNumber Property `json:"partNumber"`
			Desc       Property `json:"description"`
			Material   Property `json:"materialName"`
		} `json:"toComponent"`
	}

	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"cv": componentID}, func(data json.RawMessage) (string, []bomResult, error) {
		var r struct {
			Component struct {
				BomRelations struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []bomResult `json:"results"`
				} `json:"bomRelations"`
			} `json:"component"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("bom: %w", err)
		}
		return r.Component.BomRelations.Pagination.Cursor, r.Component.BomRelations.Results, nil
	})
	if err != nil {
		return nil, err
	}

	// Collapse duplicate child components (same component in multiple
	// relations), summing quantity and preserving first-seen order.
	idx := make(map[string]int, len(all))
	rows := make([]BOMRow, 0, len(all))
	for _, o := range all {
		c := o.ToComponent
		if c.ID == "" {
			continue
		}
		qty := o.Quantity
		if qty <= 0 {
			qty = 1
		}
		if i, ok := idx[c.ID]; ok {
			rows[i].Quantity += qty
			continue
		}
		idx[c.ID] = len(rows)
		rows = append(rows, BOMRow{
			ComponentVersionID: c.ID,
			Name:               c.Name.Str(),
			PartNumber:         c.PartNumber.Str(),
			PartDesc:           c.Desc.Str(),
			Material:           c.Material.Str(),
			Quantity:           qty,
		})
	}
	return rows, nil
}
