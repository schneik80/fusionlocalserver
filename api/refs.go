package api

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// ComponentRef describes a component shown in the Uses or Where Used tab. For
// Uses, this is a direct child component used by the current design (or, for a
// drawing, its source design). For Where Used, this is a parent component that
// consumes the current design. IDs are v3 Component ids.
type ComponentRef struct {
	ID             string // Component.id
	Name           string
	PartNumber     string
	PartDesc       string
	Material       string
	DesignItemID   string // owning DesignItem.id, if exposed
	DesignItemName string // owning DesignItem.name, if exposed
	FusionWebURL   string // owning DesignItem.fusionWebUrl, if exposed
}

// DrawingRef describes a drawing shown in the Drawings tab. DrawingItemID is the
// parent DrawingItem.id — needed to drive Show-In-Location navigation from this
// row to the Contents column.
type DrawingRef struct {
	ID            string
	Name          string
	DrawingItemID string
	ModifiedOn    time.Time
	ModifiedBy    string
	FusionWebURL  string
}

// v3CompRef is the shared shape for a component-with-its-owning-design used by
// the Uses / Where Used queries. name/partNumber/description/materialName are
// v3 Property objects.
type v3CompRef struct {
	ID           string   `json:"id"`
	Name         Property `json:"name"`
	PartNumber   Property `json:"partNumber"`
	Desc         Property `json:"description"`
	Material     Property `json:"materialName"`
	PrimaryModel struct {
		DesignItem v3DesignItem `json:"designItem"`
	} `json:"primaryModel"`
}

type v3DesignItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	FusionWebURL string `json:"fusionWebUrl"`
}

func (c v3CompRef) toRef(di v3DesignItem) ComponentRef {
	if di.ID == "" {
		di = c.PrimaryModel.DesignItem
	}
	return ComponentRef{
		ID:             c.ID,
		Name:           c.Name.Str(),
		PartNumber:     c.PartNumber.Str(),
		PartDesc:       c.Desc.Str(),
		Material:       c.Material.Str(),
		DesignItemID:   di.ID,
		DesignItemName: di.Name,
		FusionWebURL:   di.FusionWebURL,
	}
}

const v3CompRefFields = `
	id
	name { value displayValue }
	partNumber { value displayValue }
	description { value displayValue }
	materialName { value displayValue }
	primaryModel { designItem { id name fusionWebUrl } }`

// GetOccurrences returns the immediate sub-components of the given component
// (the "Uses" relationship), via Component.bomRelations. componentID is the v3
// Component id.
func GetOccurrences(ctx context.Context, token, componentID string) ([]ComponentRef, error) {
	const qFirst = `
		query GetOccurrences($cv: ID!) {
			component(componentId: $cv) {
				bomRelations(depth: 1, pagination: { limit: 50 }) {
					pagination { cursor }
					results { toComponent { ` + v3CompRefFields + ` } }
				}
			}
		}`
	const qNext = `
		query GetOccurrencesNext($cv: ID!, $cursor: String!) {
			component(componentId: $cv) {
				bomRelations(depth: 1, pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results { toComponent { ` + v3CompRefFields + ` } }
				}
			}
		}`

	type occResult struct {
		ToComponent v3CompRef `json:"toComponent"`
	}

	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"cv": componentID}, func(data json.RawMessage) (string, []occResult, error) {
		var r struct {
			Component struct {
				BomRelations struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []occResult `json:"results"`
				} `json:"bomRelations"`
			} `json:"component"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("occurrences: %w", err)
		}
		return r.Component.BomRelations.Pagination.Cursor, r.Component.BomRelations.Results, nil
	})
	if err != nil {
		return nil, err
	}

	refs := make([]ComponentRef, 0, len(all))
	for _, o := range all {
		refs = append(refs, o.ToComponent.toRef(v3DesignItem{}))
	}
	return refs, nil
}

// GetDrawingSource returns the source design referenced by the given drawing
// item. For a drawing, "Uses" means the design it was made from — the tip
// drawing's model's owning DesignItem (Drawing.model -> {component, designItem}).
// Returned as a slice so the Uses renderer stays polymorphic across designs
// (occurrences) and drawings (sources).
func GetDrawingSource(ctx context.Context, token, hubID, drawingItemID string) ([]ComponentRef, error) {
	const q = `
		query GetDrawingSource($hubId: ID!, $itemId: ID!) {
			item(hubId: $hubId, itemId: $itemId) {
				... on DrawingItem {
					tipDrawing {
						model {
							component {
								id
								name { value displayValue }
								partNumber { value displayValue }
								description { value displayValue }
								materialName { value displayValue }
							}
							designItem { id name fusionWebUrl }
						}
					}
				}
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID, "itemId": drawingItemID})
	if err != nil {
		return nil, fmt.Errorf("drawing source: %w", err)
	}

	var raw struct {
		Item struct {
			TipDrawing struct {
				Model struct {
					Component struct {
						ID         string   `json:"id"`
						Name       Property `json:"name"`
						PartNumber Property `json:"partNumber"`
						Desc       Property `json:"description"`
						Material   Property `json:"materialName"`
					} `json:"component"`
					DesignItem v3DesignItem `json:"designItem"`
				} `json:"model"`
			} `json:"tipDrawing"`
		} `json:"item"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("drawing source decode: %w", err)
	}

	m := raw.Item.TipDrawing.Model
	if m.Component.ID == "" && m.DesignItem.ID == "" {
		return nil, nil
	}
	return []ComponentRef{{
		ID:             m.Component.ID,
		Name:           m.Component.Name.Str(),
		PartNumber:     m.Component.PartNumber.Str(),
		PartDesc:       m.Component.Desc.Str(),
		Material:       m.Component.Material.Str(),
		DesignItemID:   m.DesignItem.ID,
		DesignItemName: m.DesignItem.Name,
		FusionWebURL:   m.DesignItem.FusionWebURL,
	}}, nil
}

// GetWhereUsed returns the parent components that reference the given component
// (the reverse "Where Used" relationship).
//
// v3 has no first-class reverse query. The only candidate is reading the
// component's primaryModel.assemblyRelations and keeping relations where this
// model is the `toModel` (the child) so each `fromModel` is a parent. NOTE:
// assemblyRelations is documented as "assembly relations of this model"
// (its downward sub-tree), so this may return nothing — confirmed by live test.
// Implemented this way so the behaviour is empirically verifiable; if it yields
// nothing, where-used is not supported on v3 and the tab should be hidden.
func GetWhereUsed(ctx context.Context, token, componentID string) ([]ComponentRef, error) {
	const qFirst = `
		query GetWhereUsed($cv: ID!) {
			component(componentId: $cv) {
				primaryModel {
					id
					assemblyRelations(depth: 1, pagination: { limit: 50 }) {
						pagination { cursor }
						results {
							toModel { id }
							fromModel { ` + v3FromModelFields + ` }
						}
					}
				}
			}
		}`
	const qNext = `
		query GetWhereUsedNext($cv: ID!, $cursor: String!) {
			component(componentId: $cv) {
				primaryModel {
					id
					assemblyRelations(depth: 1, pagination: { cursor: $cursor, limit: 50 }) {
						pagination { cursor }
						results {
							toModel { id }
							fromModel { ` + v3FromModelFields + ` }
						}
					}
				}
			}
		}`

	type relResult struct {
		ToModel struct {
			ID string `json:"id"`
		} `json:"toModel"`
		FromModel struct {
			ID         string       `json:"id"`
			DesignItem v3DesignItem `json:"designItem"`
			Component  v3CompRef    `json:"component"`
		} `json:"fromModel"`
	}

	var selfModelID string
	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"cv": componentID}, func(data json.RawMessage) (string, []relResult, error) {
		var r struct {
			Component struct {
				PrimaryModel struct {
					ID                string `json:"id"`
					AssemblyRelations struct {
						Pagination struct {
							Cursor string `json:"cursor"`
						} `json:"pagination"`
						Results []relResult `json:"results"`
					} `json:"assemblyRelations"`
				} `json:"primaryModel"`
			} `json:"component"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("whereUsed: %w", err)
		}
		selfModelID = r.Component.PrimaryModel.ID
		return r.Component.PrimaryModel.AssemblyRelations.Pagination.Cursor, r.Component.PrimaryModel.AssemblyRelations.Results, nil
	})
	if err != nil {
		return nil, err
	}

	// Keep relations where THIS model is the child (toModel == self); the
	// fromModel is then the parent that uses it. Dedupe by owning DesignItem.
	seen := make(map[string]bool, len(all))
	refs := make([]ComponentRef, 0, len(all))
	for _, rel := range all {
		if selfModelID != "" && rel.ToModel.ID != selfModelID {
			continue
		}
		diid := rel.FromModel.DesignItem.ID
		if diid != "" {
			if seen[diid] {
				continue
			}
			seen[diid] = true
		}
		refs = append(refs, rel.FromModel.Component.toRef(rel.FromModel.DesignItem))
	}
	return refs, nil
}

// v3FromModelFields is v3CompRefFields plus the model-level designItem
// (used inside fromModel, which carries both a component and a designItem).
const v3FromModelFields = `
	id
	designItem { id name fusionWebUrl }
	component {
		id
		name { value displayValue }
		partNumber { value displayValue }
		description { value displayValue }
		materialName { value displayValue }
	}`

// GetDrawingsForDesign returns the drawings whose source design is the given
// design item. v3 has no design->drawings field, so this scans the design's
// project for DrawingItems whose tipDrawing.model.designItem matches. This is a
// project-wide walk (one paginated query), so it can be costly for large
// projects. Returns drawings sorted by latest modification (most recent first).
func GetDrawingsForDesign(ctx context.Context, token, hubID, designItemID string) ([]DrawingRef, error) {
	// Resolve the design's project first.
	projData, err := gqlQuery(ctx, token, `
		query DesignProject($hubId: ID!, $itemId: ID!) {
			item(hubId: $hubId, itemId: $itemId) { project { id } }
		}`, map[string]any{"hubId": hubID, "itemId": designItemID})
	if err != nil {
		return nil, fmt.Errorf("drawings: locate project: %w", err)
	}
	var pj struct {
		Item struct {
			Project struct {
				ID string `json:"id"`
			} `json:"project"`
		} `json:"item"`
	}
	if err := json.Unmarshal(projData, &pj); err != nil {
		return nil, fmt.Errorf("drawings: project decode: %w", err)
	}
	projectID := pj.Item.Project.ID
	if projectID == "" {
		return nil, nil
	}

	// Page size is capped low: each row's tipDrawing.model.designItem chain is
	// expensive, and the v3 gateway enforces a 1000-point query-complexity cap
	// (limit 50 scored 1061). 20 keeps a single page well under the cap; large
	// projects just take more pages.
	const qFirst = `
		query ProjectDrawings($projectId: ID!) {
			itemsByProject(projectId: $projectId, pagination: { limit: 20 }) {
				pagination { cursor }
				results {
					__typename
					id name lastModifiedOn lastModifiedBy { firstName lastName }
					... on DrawingItem {
						fusionWebUrl
						tipDrawing { model { designItem { id } } }
					}
				}
			}
		}`
	const qNext = `
		query ProjectDrawingsNext($projectId: ID!, $cursor: String!) {
			itemsByProject(projectId: $projectId, pagination: { cursor: $cursor, limit: 20 }) {
				pagination { cursor }
				results {
					__typename
					id name lastModifiedOn lastModifiedBy { firstName lastName }
					... on DrawingItem {
						fusionWebUrl
						tipDrawing { model { designItem { id } } }
					}
				}
			}
		}`

	type drawItem struct {
		Typename     string  `json:"__typename"`
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		ModifiedOn   string  `json:"lastModifiedOn"`
		ModifiedBy   apiUser `json:"lastModifiedBy"`
		FusionWebURL string  `json:"fusionWebUrl"`
		TipDrawing   struct {
			Model struct {
				DesignItem struct {
					ID string `json:"id"`
				} `json:"designItem"`
			} `json:"model"`
		} `json:"tipDrawing"`
	}

	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"projectId": projectID}, func(data json.RawMessage) (string, []drawItem, error) {
		var r struct {
			ItemsByProject struct {
				Pagination struct {
					Cursor string `json:"cursor"`
				} `json:"pagination"`
				Results []drawItem `json:"results"`
			} `json:"itemsByProject"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("project drawings: %w", err)
		}
		return r.ItemsByProject.Pagination.Cursor, r.ItemsByProject.Results, nil
	})
	if err != nil {
		return nil, err
	}

	refs := make([]DrawingRef, 0)
	for _, d := range all {
		if d.Typename != "DrawingItem" || d.TipDrawing.Model.DesignItem.ID != designItemID {
			continue
		}
		refs = append(refs, DrawingRef{
			ID:            d.ID,
			Name:          d.Name,
			DrawingItemID: d.ID,
			ModifiedOn:    parseTime(d.ModifiedOn),
			ModifiedBy:    d.ModifiedBy.fullName(),
			FusionWebURL:  d.FusionWebURL,
		})
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].ModifiedOn.After(refs[j].ModifiedOn)
	})
	return refs, nil
}
