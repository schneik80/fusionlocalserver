package api

import (
	"context"
	"strings"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func TestGetOccurrences_DecodesAndPaginates(t *testing.T) {
	calls := 0
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		calls++
		if !strings.Contains(req.Query, "occurrences(pagination") {
			t.Errorf("query missing occurrences field: %q", req.Query)
		}
		if got, _ := req.Variables["cvId"].(string); got != "CV1" {
			t.Errorf("cvId variable = %v, want \"CV1\"", req.Variables["cvId"])
		}
		switch calls {
		case 1:
			return testutil.GraphQLResponse{Data: map[string]any{
				"componentVersion": map[string]any{
					"occurrences": map[string]any{
						"pagination": map[string]any{"cursor": "P2"},
						"results": []map[string]any{
							{
								"id": "occ1",
								"componentVersion": map[string]any{
									"id":              "cvA",
									"name":            "BoltA",
									"partNumber":      "PN-1",
									"partDescription": "M3 bolt",
									"materialName":    "Steel",
									"designItemVersion": map[string]any{
										"item": map[string]any{
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
				"componentVersion": map[string]any{
					"occurrences": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results": []map[string]any{
							{
								"id": "occ2",
								"componentVersion": map[string]any{
									"id":   "cvB",
									"name": "Washer",
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
		if !strings.Contains(req.Query, "whereUsed(pagination") {
			t.Errorf("query missing whereUsed field: %q", req.Query)
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"componentVersion": map[string]any{
				"whereUsed": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results": []map[string]any{
						{
							"id":         "cvParent",
							"name":       "MainAssembly",
							"partNumber": "ASM-100",
							"designItemVersion": map[string]any{
								"item": map[string]any{
									"id":           "diParent",
									"name":         "Main.f3d",
									"fusionWebUrl": "https://x/main",
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
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"componentVersion": map[string]any{
				"whereUsed": map[string]any{
					"pagination": map[string]any{"cursor": ""},
					"results": []map[string]any{
						// Three versions of the same parent design.
						{
							"id":   "cv-v1",
							"name": "Robot v1",
							"designItemVersion": map[string]any{
								"item": map[string]any{"id": "di-robot", "name": "Robot.f3d"},
							},
						},
						{
							"id":   "cv-v2",
							"name": "Robot v2",
							"designItemVersion": map[string]any{
								"item": map[string]any{"id": "di-robot", "name": "Robot.f3d"},
							},
						},
						{
							"id":   "cv-v3",
							"name": "Robot v3",
							"designItemVersion": map[string]any{
								"item": map[string]any{"id": "di-robot", "name": "Robot.f3d"},
							},
						},
						// A different parent design — should not be collapsed.
						{
							"id":   "cv-arm",
							"name": "Arm",
							"designItemVersion": map[string]any{
								"item": map[string]any{"id": "di-arm", "name": "Arm.f3d"},
							},
						},
						// Orphan with no DesignItemID — must pass through.
						{
							"id":   "cv-orphan-1",
							"name": "Orphan1",
						},
						{
							"id":   "cv-orphan-2",
							"name": "Orphan2",
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
		if !strings.Contains(req.Query, "tipDrawingVersion") {
			t.Errorf("query missing tipDrawingVersion field: %q", req.Query)
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"item": map[string]any{
				"tipDrawingVersion": map[string]any{
					"componentVersion": map[string]any{
						"id":              "cv-source",
						"name":            "Robot",
						"partNumber":      "PN-99",
						"partDescription": "Main robot body",
						"materialName":    "Steel",
						"designItemVersion": map[string]any{
							"item": map[string]any{
								"id":           "di-robot",
								"name":         "Robot.f3d",
								"fusionWebUrl": "https://x/robot",
							},
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
// tipDrawingVersion (rare; freshly-created drawing without a saved
// version), return an empty slice rather than a single all-empty ref.
func TestGetDrawingSource_NoTipReturnsEmpty(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"item": map[string]any{
				"tipDrawingVersion": nil,
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

// TestGetDrawingsForDesign_PaginatesVersions confirms the function
// walks across multiple pages of DesignItem.versions when one round
// trip isn't enough to capture all versions. APS caps query
// complexity at 1000 points so the per-page outer limit is small (10
// versions). Designs with many versions trigger additional round
// trips — this test simulates that by returning a cursor on the
// first page and "" on the second.
func TestGetDrawingsForDesign_PaginatesVersions(t *testing.T) {
	page := 0
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		page++
		switch page {
		case 1:
			if _, ok := req.Variables["cursor"]; ok {
				t.Errorf("first call should not include cursor variable; got %v", req.Variables)
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"item": map[string]any{
					"versions": map[string]any{
						"pagination": map[string]any{"cursor": "PAGE2"},
						"results": []map[string]any{
							{
								"drawingItemVersions": map[string]any{
									"results": []map[string]any{
										{
											"lastModifiedOn": "2026-01-01T00:00:00Z",
											"item": map[string]any{
												"id":           "di-top-view",
												"name":         "Top View",
												"fusionWebUrl": "https://x/top",
											},
										},
									},
								},
							},
						},
					},
				},
			}}
		case 2:
			if got, _ := req.Variables["cursor"].(string); got != "PAGE2" {
				t.Errorf("second call cursor = %v, want PAGE2", req.Variables["cursor"])
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"item": map[string]any{
					"versions": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results": []map[string]any{
							{
								"drawingItemVersions": map[string]any{
									"results": []map[string]any{
										{
											"lastModifiedOn": "2026-02-01T00:00:00Z",
											"item": map[string]any{
												"id":           "di-section",
												"name":         "Section A-A",
												"fusionWebUrl": "https://x/sec",
											},
										},
									},
								},
							},
						},
					},
				},
			}}
		}
		t.Fatalf("unexpected extra call: %d", page)
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

// TestGetDrawingsForDesign_AggregatesAndDedupes asserts that drawings
// are walked across every design version, deduped by their lineage URN
// (so the same drawing showing up under multiple design versions
// collapses to one row), and that the dedup keeps the most-recently-
// modified version's metadata. Mirrors the real APS shape where one
// drawing has versions tied to multiple design versions.
func TestGetDrawingsForDesign_AggregatesAndDedupes(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if !strings.Contains(req.Query, "drawingItemVersions") || !strings.Contains(req.Query, "DesignItem") {
			t.Errorf("query shape unexpected: %q", req.Query)
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"item": map[string]any{
				"versions": map[string]any{
					"results": []map[string]any{
						{
							"drawingItemVersions": map[string]any{
								"results": []map[string]any{
									// One drawing, two versions (same lineage).
									{
										"id":             "dv-1-a",
										"name":           "Top View",
										"lastModifiedOn": "2026-01-15T10:30:00Z",
										"lastModifiedBy": map[string]any{"firstName": "Ada"},
										"item": map[string]any{
											"id":           "di-top-view",
											"name":         "Top View",
											"fusionWebUrl": "https://x/top",
										},
									},
									{
										"id":             "dv-1-b",
										"name":           "Top View",
										"lastModifiedOn": "2026-03-01T08:00:00Z", // newer
										"lastModifiedBy": map[string]any{"firstName": "Bob"},
										"item": map[string]any{
											"id":           "di-top-view",
											"name":         "Top View",
											"fusionWebUrl": "https://x/top",
										},
									},
								},
							},
						},
						{
							"drawingItemVersions": map[string]any{
								"results": []map[string]any{
									{
										"id":             "dv-2",
										"name":           "Section A-A",
										"lastModifiedOn": "2026-02-10T12:00:00Z",
										"lastModifiedBy": map[string]any{"firstName": "Carol"},
										"item": map[string]any{
											"id":           "di-section",
											"name":         "Section A-A",
											"fusionWebUrl": "https://x/section",
										},
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

	got, err := GetDrawingsForDesign(context.Background(), "tok", "H1", "di-design")
	if err != nil {
		t.Fatalf("GetDrawingsForDesign: %v", err)
	}
	// Expected: 2 unique drawings, sorted newest-first.
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (one Top View dedup + one Section); got %+v", len(got), got)
	}
	if got[0].DrawingItemID != "di-top-view" {
		t.Errorf("first row should be Top View (newest modified), got %+v", got[0])
	}
	if got[0].ModifiedBy != "Bob" {
		t.Errorf("dedup should keep newest entry's metadata, got ModifiedBy=%q", got[0].ModifiedBy)
	}
	if got[1].DrawingItemID != "di-section" {
		t.Errorf("second row should be Section A-A, got %+v", got[1])
	}
}
