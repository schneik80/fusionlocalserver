package api

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func TestParseTime(t *testing.T) {
	mustParse := func(layout, s string) time.Time {
		t.Helper()
		v, err := time.Parse(layout, s)
		if err != nil {
			t.Fatalf("setup: parsing %q with %q: %v", s, layout, err)
		}
		return v
	}

	cases := []struct {
		name     string
		input    string
		wantZero bool
		want     time.Time
	}{
		{
			name:     "empty string returns zero time",
			input:    "",
			wantZero: true,
		},
		{
			name:  "RFC3339 UTC",
			input: "2024-01-15T10:30:45Z",
			want:  mustParse(time.RFC3339, "2024-01-15T10:30:45Z"),
		},
		{
			name:  "fractional Z fallback",
			input: "2024-01-15T10:30:45.123Z",
			want:  mustParse("2006-01-02T15:04:05.000Z", "2024-01-15T10:30:45.123Z"),
		},
		{
			name:  "RFC3339 with positive offset",
			input: "2024-01-15T10:30:45+02:00",
			want:  mustParse(time.RFC3339, "2024-01-15T10:30:45+02:00"),
		},
		{
			name:     "garbage returns zero time",
			input:    "garbage",
			wantZero: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseTime(tc.input)
			if tc.wantZero {
				if !got.IsZero() {
					t.Errorf("parseTime(%q) = %v, want zero time", tc.input, got)
				}
				return
			}
			if !got.Equal(tc.want) {
				t.Errorf("parseTime(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetItemDetails_AllFields(t *testing.T) {
	raw, err := os.ReadFile("testdata/details_design.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshaling fixture: %v", err)
	}

	var sawHubID, sawItemID bool
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if v, ok := req.Variables["hubId"].(string); ok && v == "h1" {
			sawHubID = true
		} else {
			t.Errorf("Variables[hubId] = %v, want \"h1\"", req.Variables["hubId"])
		}
		if v, ok := req.Variables["itemId"].(string); ok && v == "item-1" {
			sawItemID = true
		} else {
			t.Errorf("Variables[itemId] = %v, want \"item-1\"", req.Variables["itemId"])
		}
		return testutil.GraphQLResponse{Data: data}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetItemDetails(context.Background(), "tok", "h1", "item-1")
	if err != nil {
		t.Fatalf("GetItemDetails: %v", err)
	}
	if !sawHubID || !sawItemID {
		t.Errorf("handler missed an assertion: hubId=%v itemId=%v", sawHubID, sawItemID)
	}

	if got.ID != "urn:item:abc" {
		t.Errorf("ID = %q, want %q", got.ID, "urn:item:abc")
	}
	if got.Name != "Widget A" {
		t.Errorf("Name = %q, want %q", got.Name, "Widget A")
	}
	if got.Typename != "DesignItem" {
		t.Errorf("Typename = %q, want %q", got.Typename, "DesignItem")
	}
	if got.Size != "12345678" {
		t.Errorf("Size = %q, want %q", got.Size, "12345678")
	}
	if got.MimeType != "application/vnd.autodesk.fusion360" {
		t.Errorf("MimeType = %q, want %q", got.MimeType, "application/vnd.autodesk.fusion360")
	}
	if got.ExtensionType != "Fusion360" {
		t.Errorf("ExtensionType = %q, want %q", got.ExtensionType, "Fusion360")
	}
	if got.FusionWebURL != "https://fusion.example/widget-a" {
		t.Errorf("FusionWebURL = %q, want %q", got.FusionWebURL, "https://fusion.example/widget-a")
	}

	wantCreated := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)
	if !got.CreatedOn.Equal(wantCreated) {
		t.Errorf("CreatedOn = %v, want %v", got.CreatedOn, wantCreated)
	}
	wantModified := time.Date(2024, 2, 20, 14, 0, 0, 0, time.UTC)
	if !got.ModifiedOn.Equal(wantModified) {
		t.Errorf("ModifiedOn = %v, want %v", got.ModifiedOn, wantModified)
	}

	if got.CreatedBy != "Ada Lovelace" {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, "Ada Lovelace")
	}
	if got.ModifiedBy != "Grace Hopper" {
		t.Errorf("ModifiedBy = %q, want %q", got.ModifiedBy, "Grace Hopper")
	}
	if got.PartNumber != "WGT-001" {
		t.Errorf("PartNumber = %q, want %q", got.PartNumber, "WGT-001")
	}
	if got.PartDesc != "The widget A" {
		t.Errorf("PartDesc = %q, want %q", got.PartDesc, "The widget A")
	}
	if got.Material != "Aluminum 6061" {
		t.Errorf("Material = %q, want %q", got.Material, "Aluminum 6061")
	}
	if got.RootComponentVersionID != "urn:cv:xyz" {
		t.Errorf("RootComponentVersionID = %q, want %q", got.RootComponentVersionID, "urn:cv:xyz")
	}
	wantTip := time.Date(2024, 2, 20, 14, 0, 0, 0, time.UTC)
	if !got.TipTimestamp.Equal(wantTip) {
		t.Errorf("TipTimestamp = %v, want %v", got.TipTimestamp, wantTip)
	}

	// Time-based history, sorted most-recent first (h3, h2, h1).
	if len(got.History) != 3 {
		t.Fatalf("len(History) = %d, want 3", len(got.History))
	}
	if got.History[0].ID != "h3" || got.History[2].ID != "h1" {
		t.Errorf("History order = [%s..%s], want [h3..h1]", got.History[0].ID, got.History[2].ID)
	}
	if got.History[0].ChangeType != "Properties Updated" {
		t.Errorf("History[0].ChangeType = %q, want %q", got.History[0].ChangeType, "Properties Updated")
	}
	if got.History[0].Description != "third edit" {
		t.Errorf("History[0].Description = %q, want %q", got.History[0].Description, "third edit")
	}
	if got.History[0].Author != "Grace Hopper" {
		t.Errorf("History[0].Author = %q, want %q", got.History[0].Author, "Grace Hopper")
	}
}

func TestGetItemDetails_DrawingItem_NoComponentVersion(t *testing.T) {
	data := map[string]any{
		"item": map[string]any{
			"__typename":     "DrawingItem",
			"id":             "urn:item:dwg",
			"name":           "Sheet 1",
			"size":           "0",
			"mimeType":       "application/dwg",
			"extensionType":  "DrawingItem",
			"createdOn":      "2024-03-01T09:00:00Z",
			"createdBy":      map[string]any{"firstName": "X", "lastName": "Y"},
			"lastModifiedOn": "2024-03-02T09:00:00Z",
			"lastModifiedBy": map[string]any{"firstName": "X", "lastName": "Y"},
			"fusionWebUrl":   "https://example/dwg",
			"history":        map[string]any{"results": []any{}},
		},
	}

	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: data}
	})
	swapEndpoint(t, srv.URL)

	got, err := GetItemDetails(context.Background(), "tok", "h1", "item-dwg")
	if err != nil {
		t.Fatalf("GetItemDetails: %v", err)
	}
	if got.Typename != "DrawingItem" {
		t.Errorf("Typename = %q, want %q", got.Typename, "DrawingItem")
	}
	if got.RootComponentVersionID != "" {
		t.Errorf("RootComponentVersionID = %q, want empty", got.RootComponentVersionID)
	}
	if got.PartNumber != "" {
		t.Errorf("PartNumber = %q, want empty", got.PartNumber)
	}
	if got.Material != "" {
		t.Errorf("Material = %q, want empty", got.Material)
	}
}

func TestApiUser_FullName(t *testing.T) {
	cases := []struct {
		name  string
		first string
		last  string
		want  string
	}{
		{name: "both empty", first: "", last: "", want: ""},
		{name: "first only", first: "Ada", last: "", want: "Ada"},
		{name: "last only", first: "", last: "Lovelace", want: "Lovelace"},
		{name: "both present", first: "Ada", last: "Lovelace", want: "Ada Lovelace"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := apiUser{First: tc.first, Last: tc.last}
			got := u.fullName()
			if got != tc.want {
				t.Errorf("apiUser{%q,%q}.fullName() = %q, want %q", tc.first, tc.last, got, tc.want)
			}
		})
	}
}
