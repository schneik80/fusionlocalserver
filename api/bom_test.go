package api

import (
	"context"
	"strings"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func TestGetBOM_GroupsByComponentAndCountsQuantity(t *testing.T) {
	// v3 bomRelations: each relation has its own quantity and a toComponent
	// whose fields are Property objects { value, displayValue }.
	rel := func(qty int, id, name, pn string) map[string]any {
		return map[string]any{
			"quantity": qty,
			"toComponent": map[string]any{
				"id":         id,
				"name":       map[string]any{"value": name, "displayValue": name},
				"partNumber": map[string]any{"value": pn, "displayValue": pn},
			},
		}
	}
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
						"results":    []map[string]any{rel(1, "cvA", "BoltA", "PN-1"), rel(1, "cvA", "BoltA", "PN-1"), rel(1, "cvB", "Washer", "")},
					},
				},
			}}
		case 2:
			return testutil.GraphQLResponse{Data: map[string]any{
				"component": map[string]any{
					"bomRelations": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results":    []map[string]any{rel(1, "cvA", "BoltA", "PN-1"), rel(1, "cvC", "Nut", "")},
					},
				},
			}}
		}
		t.Fatalf("unexpected extra call: %d", calls)
		return testutil.GraphQLResponse{}
	})
	swapEndpoint(t, srv.URL)

	rows, err := GetBOM(context.Background(), "tok", "CV1")
	if err != nil {
		t.Fatalf("GetBOM: %v", err)
	}
	if calls != 2 {
		t.Errorf("server calls = %d, want 2 (should paginate)", calls)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 unique components: %+v", len(rows), rows)
	}

	// First-seen order preserved; quantity = sum of relation quantities across pages.
	want := []struct {
		id  string
		qty int
	}{{"cvA", 3}, {"cvB", 1}, {"cvC", 1}}
	for i, w := range want {
		if rows[i].ComponentVersionID != w.id || rows[i].Quantity != w.qty {
			t.Errorf("row %d = {%s qty %d}, want {%s qty %d}", i, rows[i].ComponentVersionID, rows[i].Quantity, w.id, w.qty)
		}
	}
	if rows[0].Name != "BoltA" || rows[0].PartNumber != "PN-1" {
		t.Errorf("row 0 fields = %+v", rows[0])
	}
}
