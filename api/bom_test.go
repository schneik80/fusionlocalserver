package api

import (
	"context"
	"strings"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func TestGetBOM_GroupsByComponentAndCountsQuantity(t *testing.T) {
	occ := func(id, name, pn string) map[string]any {
		return map[string]any{
			"componentVersion": map[string]any{"id": id, "name": name, "partNumber": pn},
		}
	}
	calls := 0
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		calls++
		if !strings.Contains(req.Query, "allOccurrences(pagination") {
			t.Errorf("query missing allOccurrences field: %q", req.Query)
		}
		switch calls {
		case 1:
			return testutil.GraphQLResponse{Data: map[string]any{
				"componentVersion": map[string]any{
					"allOccurrences": map[string]any{
						"pagination": map[string]any{"cursor": "P2"},
						"results":    []map[string]any{occ("cvA", "BoltA", "PN-1"), occ("cvA", "BoltA", "PN-1"), occ("cvB", "Washer", "")},
					},
				},
			}}
		case 2:
			return testutil.GraphQLResponse{Data: map[string]any{
				"componentVersion": map[string]any{
					"allOccurrences": map[string]any{
						"pagination": map[string]any{"cursor": ""},
						"results":    []map[string]any{occ("cvA", "BoltA", "PN-1"), occ("cvC", "Nut", "")},
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

	// First-seen order preserved; quantity = occurrence count across pages.
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
