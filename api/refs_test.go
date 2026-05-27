package api

import (
	"context"
	"strings"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

// prop builds a v3 Property object { value, displayValue }.
func prop(s string) map[string]any {
	return map[string]any{"value": s, "displayValue": s}
}

func TestGetOccurrences_DecodesAndPaginates(t *testing.T) {
	calls := 0
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		calls++
		if !strings.Contains(req.Query, "bomRelations(depth") {
			t.Errorf("query missing bomRelations field: %q", req.Query)
		}
		if got, _ := req.Variables["cv"].(string); got != "CV1" {
			t.Errorf("cv variable = %v, want \"CV1\"", req.Variables["cv"])
		}
		switch calls {
		case 1:
			return testutil.GraphQLResponse{Data: map[string]any{
				"component": map[string]any{
					"bomRelations": map[string]any{
						"pagination": map[string]any{"cursor": "P2"},
						"results": []map[string]any{
							{
								"toComponent": map[string]any{
									"id":           "cvA",
									"name":         prop("BoltA"),
									"partNumber":   prop("PN-1"),
									"description":  prop("M3 bolt"),
									"materialName": prop("Steel"),
									"primaryModel": map[string]any{
										"designItem": map[string]any{
											"id":           "diA",
											"name":         "BoltA.f3d",
											"fusionWebUrl": "https://x/a",
										},
									},
								},
							},
						},
					},
				},
			}}
		case 2:
			return testutil.GraphQLResponse{Data: map[string]any{
				"component": map[string]any{
					"bomRelations": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results": []map[string]any{
							{
								"toComponent": map[string]any{
									"id":   "cvB",
									"name": prop("Washer"),
								},
							},
						},
					},
				},
			}}
		}
		t.Fatalf("unexpected extra call: %d", calls)
		return testutil.GraphQLResponse{}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetOccurrences(context.Background(), "tok", "CV1")
	if err != nil {
		t.Fatalf("GetOccurrences: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "BoltA" || got[0].PartNumber != "PN-1" || got[0].DesignItemName != "BoltA.f3d" || got[0].FusionWebURL != "https://x/a" {
		t.Errorf("first ref decoded wrong: %+v", got[0])
	}
	if got[1].Name != "Washer" || got[1].PartNumber != "" {
		t.Errorf("second ref decoded wrong: %+v", got[1])
	}
}

func TestGetWhereUsed_DecodesResults(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if !strings.Contains(req.Query, "assemblyRelations(depth") {
			t.Errorf("query missing assemblyRelations field: %q", req.Query)
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"component": map[string]any{
				"primaryModel": map[string]any{
					"id": "selfModel",
					"assemblyRelations": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results": []map[string]any{
							{
								// toModel == self, so fromModel is a parent that uses us.
								"toModel": map[string]any{"id": "selfModel"},
								"fromModel": map[string]any{
									"id": "parentModel",
									"designItem": map[string]any{
										"id":           "diParent",
										"name":         "Main.f3d",
										"fusionWebUrl": "https://x/main",
									},
									"component": map[string]any{
										"id":         "cvParent",
										"name":       prop("MainAssembly"),
										"partNumber": prop("ASM-100"),
									},
								},
							},
						},
					},
				},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetWhereUsed(context.Background(), "tok", "CV1")
	if err != nil {
		t.Fatalf("GetWhereUsed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "MainAssembly" || got[0].PartNumber != "ASM-100" || got[0].DesignItemName != "Main.f3d" {
		t.Errorf("ref decoded wrong: %+v", got[0])
	}
}

// TestGetWhereUsed_DedupesByDesignItem confirms the per-version collapse:
// when APS returns multiple ComponentVersion rows that all hang off the
// same parent DesignItem (one per version of that parent), the function
// keeps only the first occurrence so the user sees each parent design
// once. Orphan rows with no DesignItemID are passed through unchanged.
func TestGetWhereUsed_DedupesByDesignItem(t *testing.T) {
	// rel wraps a fromModel parent into an assemblyRelation whose toModel is
	// the queried model itself (so GetWhereUsed keeps it).
	rel := func(compID, diID, diName string) map[string]any {
		from := map[string]any{
			"id":        "fm-" + compID,
			"component": map[string]any{"id": compID, "name": prop(compID)},
		}
		if diID != "" {
			from["designItem"] = map[string]any{"id": diID, "name": diName}
		}
		return map[string]any{
			"toModel":   map[string]any{"id": "selfModel"},
			"fromModel": from,
		}
	}
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"component": map[string]any{
				"primaryModel": map[string]any{
					"id": "selfModel",
					"assemblyRelations": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results": []map[string]any{
							// Three versions of the same parent design.
							rel("cv-v1", "di-robot", "Robot.f3d"),
							rel("cv-v2", "di-robot", "Robot.f3d"),
							rel("cv-v3", "di-robot", "Robot.f3d"),
							// A different parent design — should not be collapsed.
							rel("cv-arm", "di-arm", "Arm.f3d"),
							// Orphans with no DesignItemID — must pass through.
							rel("cv-orphan-1", "", ""),
							rel("cv-orphan-2", "", ""),
						},
					},
				},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetWhereUsed(context.Background(), "tok", "CV1")
	if err != nil {
		t.Fatalf("GetWhereUsed: %v", err)
	}
	// Expected: 1 collapsed Robot row + 1 Arm row + 2 orphans = 4 entries
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4 (1 Robot + 1 Arm + 2 orphans); got %+v", len(got), got)
	}
	if got[0].ID != "cv-v1" || got[0].DesignItemID != "di-robot" {
		t.Errorf("first entry = %+v, want cv-v1 / di-robot (first-seen wins)", got[0])
	}
	if got[1].ID != "cv-arm" {
		t.Errorf("second entry ID = %q, want cv-arm", got[1].ID)
	}
	if got[2].ID != "cv-orphan-1" || got[3].ID != "cv-orphan-2" {
		t.Errorf("orphans collapsed unexpectedly: %+v / %+v", got[2], got[3])
	}
}

// TestGetDrawingSource_DecodesTipReference confirms the drawing → source
// design lookup returns a single ComponentRef populated with the design
// item's lineage URN, name, and web URL — the values the Uses tab
// renders and the Show-in-Location handler navigates to.
func TestGetDrawingSource_DecodesTipReference(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if !strings.Contains(req.Query, "tipDrawing") {
			t.Errorf("query missing tipDrawing field: %q", req.Query)
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"item": map[string]any{
				"tipDrawing": map[string]any{
					"model": map[string]any{
						"component": map[string]any{
							"id":           "cv-source",
							"name":         prop("Robot"),
							"partNumber":   prop("PN-99"),
							"description":  prop("Main robot body"),
							"materialName": prop("Steel"),
						},
						"designItem": map[string]any{
							"id":           "di-robot",
							"name":         "Robot.f3d",
							"fusionWebUrl": "https://x/robot",
						},
					},
				},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetDrawingSource(context.Background(), "tok", "H1", "di-drawing")
	if err != nil {
		t.Fatalf("GetDrawingSource: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (drawings reference one source)", len(got))
	}
	r := got[0]
	if r.Name != "Robot" || r.PartNumber != "PN-99" {
		t.Errorf("componentVersion fields wrong: %+v", r)
	}
	if r.DesignItemID != "di-robot" || r.DesignItemName != "Robot.f3d" || r.FusionWebURL != "https://x/robot" {
		t.Errorf("designItem fields wrong: %+v", r)
	}
}

// TestGetDrawingSource_NoTipReturnsEmpty: if the drawing has no
// tipDrawing (rare; freshly-created drawing without a saved version),
// return an empty slice rather than a single all-empty ref.
func TestGetDrawingSource_NoTipReturnsEmpty(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"item": map[string]any{
				"tipDrawing": nil,
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetDrawingSource(context.Background(), "tok", "H1", "di-drawing")
	if err != nil {
		t.Fatalf("GetDrawingSource: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0 for drawing with no tip version; got %+v", len(got), got)
	}
}

// drawItemJSON builds one itemsByProject row for a DrawingItem whose tip
// drawing's source design is designItemID.
func drawItemJSON(id, name, modifiedOn, designItemID string) map[string]any {
	return map[string]any{
		"__typename":     "DrawingItem",
		"id":             id,
		"name":           name,
		"lastModifiedOn": modifiedOn,
		"fusionWebUrl":   "https://x/" + id,
		"tipDrawing": map[string]any{
			"model": map[string]any{
				"designItem": map[string]any{"id": designItemID},
			},
		},
	}
}

// TestGetDrawingsForDesign_PaginatesVersions confirms the function resolves
// the design's project, then walks across multiple pages of itemsByProject
// when one round trip isn't enough. APS caps query complexity at 1000 points
// so the per-page limit is small. This test simulates pagination by returning
// a cursor on the first project-items page and "" on the second.
func TestGetDrawingsForDesign_PaginatesVersions(t *testing.T) {
	page := 0
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		// First the project-resolution query, then the paginated item walk.
		if strings.Contains(req.Query, "project { id }") {
			if got, _ := req.Variables["itemId"].(string); got != "di-design" {
				t.Errorf("project lookup itemId = %v, want di-design", req.Variables["itemId"])
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"item": map[string]any{"project": map[string]any{"id": "proj-1"}},
			}}
		}
		if !strings.Contains(req.Query, "itemsByProject") {
			t.Errorf("unexpected query: %q", req.Query)
		}
		page++
		switch page {
		case 1:
			if _, ok := req.Variables["cursor"]; ok {
				t.Errorf("first items call should not include cursor variable; got %v", req.Variables)
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"itemsByProject": map[string]any{
					"pagination": map[string]any{"cursor": "PAGE2"},
					"results": []map[string]any{
						drawItemJSON("di-top-view", "Top View", "2026-01-01T00:00:00Z", "di-design"),
					},
				},
			}}
		case 2:
			if got, _ := req.Variables["cursor"].(string); got != "PAGE2" {
				t.Errorf("second items call cursor = %v, want PAGE2", req.Variables["cursor"])
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"itemsByProject": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results": []map[string]any{
						drawItemJSON("di-section", "Section A-A", "2026-02-01T00:00:00Z", "di-design"),
					},
				},
			}}
		}
		t.Fatalf("unexpected extra items call: %d", page)
		return testutil.GraphQLResponse{}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetDrawingsForDesign(context.Background(), "tok", "H1", "di-design")
	if err != nil {
		t.Fatalf("GetDrawingsForDesign: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (one per page); got %+v", len(got), got)
	}
	// Sorted newest-first — Section (Feb) before Top View (Jan).
	if got[0].DrawingItemID != "di-section" || got[1].DrawingItemID != "di-top-view" {
		t.Errorf("sort order wrong: %+v", got)
	}
}

// TestGetDrawingsForDesign_FiltersAndSorts asserts that the project-wide
// itemsByProject walk keeps only DrawingItems whose tipDrawing.model.designItem
// matches the target design, drops drawings sourced from other designs and
// non-drawing rows, and returns the survivors sorted newest-first.
func TestGetDrawingsForDesign_FiltersAndSorts(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if strings.Contains(req.Query, "project { id }") {
			return testutil.GraphQLResponse{Data: map[string]any{
				"item": map[string]any{"project": map[string]any{"id": "proj-1"}},
			}}
		}
		if !strings.Contains(req.Query, "itemsByProject") ||
			!strings.Contains(req.Query, "... on DrawingItem") ||
			!strings.Contains(req.Query, "tipDrawing") {
			t.Errorf("query shape unexpected: %q", req.Query)
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"itemsByProject": map[string]any{
				"pagination": map[string]any{"cursor": ""},
				"results": []map[string]any{
					// Two drawings sourced from the target design.
					func() map[string]any {
						d := drawItemJSON("di-top-view", "Top View", "2026-03-01T08:00:00Z", "di-design")
						d["lastModifiedBy"] = map[string]any{"firstName": "Bob"}
						return d
					}(),
					func() map[string]any {
						d := drawItemJSON("di-section", "Section A-A", "2026-02-10T12:00:00Z", "di-design")
						d["lastModifiedBy"] = map[string]any{"firstName": "Carol"}
						return d
					}(),
					// Drawing sourced from a DIFFERENT design — must be filtered out.
					drawItemJSON("di-other", "Other Drawing", "2026-04-01T00:00:00Z", "di-other-design"),
					// A non-drawing row — must be filtered out.
					{"__typename": "DesignItem", "id": "i-design", "name": "SomeDesign", "lastModifiedOn": "2026-05-01T00:00:00Z"},
				},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetDrawingsForDesign(context.Background(), "tok", "H1", "di-design")
	if err != nil {
		t.Fatalf("GetDrawingsForDesign: %v", err)
	}
	// Expected: 2 matching drawings, sorted newest-first.
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (Top View + Section); got %+v", len(got), got)
	}
	if got[0].DrawingItemID != "di-top-view" {
		t.Errorf("first row should be Top View (newest modified), got %+v", got[0])
	}
	if got[0].ModifiedBy != "Bob" {
		t.Errorf("expected ModifiedBy=Bob for newest row, got %q", got[0].ModifiedBy)
	}
	if got[1].DrawingItemID != "di-section" {
		t.Errorf("second row should be Section A-A, got %+v", got[1])
	}
}
