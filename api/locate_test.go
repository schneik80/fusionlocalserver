package api

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

// TestGetItemLocation_WalksFolderAncestry seeds a fake APS server that
// returns three folder levels then a null parent, and asserts that
// GetItemLocation returns the project metadata plus a root→leaf folder
// path (reversed from the leaf-first walk order).
func TestGetItemLocation_WalksFolderAncestry(t *testing.T) {
	var calls atomic.Int32
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		n := calls.Add(1)
		switch n {
		case 1:
			// First query: item details + immediate parent folder.
			if !strings.Contains(req.Query, "LocateItem") {
				t.Errorf("call 1: expected LocateItem query, got %q", req.Query)
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"item": map[string]any{
					"project": map[string]any{
						"id":   "P1",
						"name": "RobotLab",
						"hub":  map[string]any{"id": "H1"},
						"alternativeIdentifiers": map[string]any{
							"dataManagementAPIProjectId": "a.proj1",
						},
					},
					"parentFolder": map[string]any{"id": "F-leaf", "name": "Subassemblies"},
				},
			}}
		case 2:
			// Walk: F-leaf → F-mid
			if got, _ := req.Variables["folderId"].(string); got != "F-leaf" {
				t.Errorf("call 2: folderId = %v, want F-leaf", req.Variables["folderId"])
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"folderByHubId": map[string]any{
					"parentFolder": map[string]any{"id": "F-mid", "name": "Engineering"},
				},
			}}
		case 3:
			// Walk: F-mid → F-root
			if got, _ := req.Variables["folderId"].(string); got != "F-mid" {
				t.Errorf("call 3: folderId = %v, want F-mid", req.Variables["folderId"])
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"folderByHubId": map[string]any{
					"parentFolder": map[string]any{"id": "F-root", "name": "Top"},
				},
			}}
		case 4:
			// Walk: F-root → null (project root).
			if got, _ := req.Variables["folderId"].(string); got != "F-root" {
				t.Errorf("call 4: folderId = %v, want F-root", req.Variables["folderId"])
			}
			return testutil.GraphQLResponse{Data: map[string]any{
				"folderByHubId": map[string]any{
					"parentFolder": nil,
				},
			}}
		}
		t.Fatalf("unexpected extra call: %d", n)
		return testutil.GraphQLResponse{}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetItemLocation(context.Background(), "tok", "H1", "I1")
	if err != nil {
		t.Fatalf("GetItemLocation: %v", err)
	}
	if got.HubID != "H1" || got.ProjectID != "P1" || got.ProjectName != "RobotLab" || got.ProjectAltID != "a.proj1" {
		t.Errorf("project metadata wrong: %+v", got)
	}
	want := []FolderRef{
		{ID: "F-root", Name: "Top"},
		{ID: "F-mid", Name: "Engineering"},
		{ID: "F-leaf", Name: "Subassemblies"},
	}
	if len(got.FolderPath) != len(want) {
		t.Fatalf("FolderPath len = %d, want %d (path=%+v)", len(got.FolderPath), len(want), got.FolderPath)
	}
	for i, w := range want {
		if got.FolderPath[i] != w {
			t.Errorf("FolderPath[%d] = %+v, want %+v", i, got.FolderPath[i], w)
		}
	}
}

// TestGetItemLocation_ProjectRootEmptyFolderPath confirms the empty-path
// case: an item that lives in the project root returns ItemLocation
// with an empty FolderPath (no walk queries fire).
func TestGetItemLocation_ProjectRootEmptyFolderPath(t *testing.T) {
	var calls atomic.Int32
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		calls.Add(1)
		return testutil.GraphQLResponse{Data: map[string]any{
			"item": map[string]any{
				"project": map[string]any{
					"id":                     "P1",
					"name":                   "Bare",
					"hub":                    map[string]any{"id": "H1"},
					"alternativeIdentifiers": map[string]any{"dataManagementAPIProjectId": "a.bare"},
				},
				"parentFolder": nil,
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetItemLocation(context.Background(), "tok", "H1", "I1")
	if err != nil {
		t.Fatalf("GetItemLocation: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("call count = %d, want 1 (no folder walk for root item)", calls.Load())
	}
	if len(got.FolderPath) != 0 {
		t.Errorf("FolderPath = %+v, want empty", got.FolderPath)
	}
}
