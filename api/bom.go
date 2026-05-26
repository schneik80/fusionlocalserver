package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// BOMRow is one line of a design's bill of materials: a unique component and
// how many times it occurs anywhere in the assembly. The v2 Manufacturing Data
// Model has no explicit quantity field, so quantity is the count of occurrences
// of that component across the whole structure (see docs/api.md).
type BOMRow struct {
	ComponentVersionID string
	Name               string
	PartNumber         string
	PartDesc           string
	Material           string
	Quantity           int
}

// GetBOM returns a flat bill of materials for the given component version: one
// row per unique sub-component, with quantity = number of occurrences. It uses
// allOccurrences (every descendant, not just immediate children) and groups by
// component version id, preserving first-seen order.
func GetBOM(ctx context.Context, token, componentVersionID string) ([]BOMRow, error) {
	// limit 100 keeps each page under the APS query-complexity cap (~1000
	// points); pagination walks the rest.
	const qFirst = `
		query GetBOM($cvId: ID!) {
			componentVersion(componentVersionId: $cvId) {
				allOccurrences(pagination: { limit: 100 }) {
					pagination { cursor }
					results {
						componentVersion { id name partNumber partDescription materialName }
					}
				}
			}
		}`
	const qNext = `
		query GetBOMNext($cvId: ID!, $cursor: String!) {
			componentVersion(componentVersionId: $cvId) {
				allOccurrences(pagination: { cursor: $cursor, limit: 100 }) {
					pagination { cursor }
					results {
						componentVersion { id name partNumber partDescription materialName }
					}
				}
			}
		}`

	type occResult struct {
		ComponentVersion struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			PartNumber string `json:"partNumber"`
			PartDesc   string `json:"partDescription"`
			Material   string `json:"materialName"`
		} `json:"componentVersion"`
	}

	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"cvId": componentVersionID}, func(data json.RawMessage) (string, []occResult, error) {
		var r struct {
			ComponentVersion struct {
				AllOccurrences struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []occResult `json:"results"`
				} `json:"allOccurrences"`
			} `json:"componentVersion"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("bom: %w", err)
		}
		return r.ComponentVersion.AllOccurrences.Pagination.Cursor, r.ComponentVersion.AllOccurrences.Results, nil
	})
	if err != nil {
		return nil, err
	}

	idx := make(map[string]int, len(all))
	rows := make([]BOMRow, 0, len(all))
	for _, o := range all {
		cv := o.ComponentVersion
		if cv.ID == "" {
			continue
		}
		if i, ok := idx[cv.ID]; ok {
			rows[i].Quantity++
			continue
		}
		idx[cv.ID] = len(rows)
		rows = append(rows, BOMRow{
			ComponentVersionID: cv.ID,
			Name:               cv.Name,
			PartNumber:         cv.PartNumber,
			PartDesc:           cv.PartDesc,
			Material:           cv.Material,
			Quantity:           1,
		})
	}
	return rows, nil
}
