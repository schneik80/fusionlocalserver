package api

import (
	"context"
	"testing"

	"github.com/schneik80/fusionlocalserver/internal/testutil"
)

func measureJSON(value, units string) map[string]any {
	return map[string]any{
		"displayValue": value,
		"definition":   map[string]any{"units": map[string]any{"name": units}},
	}
}

func TestGetPhysicalProperties_Completed(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		if v, _ := req.Variables["cv"].(string); v != "cv-1" {
			t.Errorf("Variables[cv] = %v, want cv-1", req.Variables["cv"])
		}
		return testutil.GraphQLResponse{Data: map[string]any{
			"component": map[string]any{
				"primaryModel": map[string]any{
					"physicalProperties": map[string]any{
						"status":  "COMPLETED",
						"area":    measureJSON("12.5", "cm^2"),
						"volume":  measureJSON("3.4", "cm^3"),
						"mass":    measureJSON("0.027", "kg"),
						"density": measureJSON("8.0", "g/cm^3"),
						"boundingBox": map[string]any{
							"length": measureJSON("10", "mm"),
							"width":  measureJSON("20", "mm"),
							"height": measureJSON("30", "mm"),
						},
					},
				},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	pp, err := GetPhysicalProperties(context.Background(), "tok", "cv-1")
	if err != nil {
		t.Fatalf("GetPhysicalProperties: %v", err)
	}
	if pp.Status != PhysPropsStatusCompleted {
		t.Errorf("status = %q, want COMPLETED", pp.Status)
	}
	if pp.Mass.Display != "0.027" || pp.Mass.Units != "kg" {
		t.Errorf("mass = %+v, want {0.027 kg}", pp.Mass)
	}
	if pp.BBoxWidth.Display != "20" || pp.BBoxWidth.Units != "mm" {
		t.Errorf("bbox width = %+v, want {20 mm}", pp.BBoxWidth)
	}
}

func TestGetPhysicalProperties_NullMapsToFailed(t *testing.T) {
	srv := testutil.GraphQLServer(t, func(req testutil.GraphQLRequest) testutil.GraphQLResponse {
		return testutil.GraphQLResponse{Data: map[string]any{
			"component": map[string]any{
				"primaryModel": map[string]any{"physicalProperties": nil},
			},
		}}
	})
	swapEndpoint(t, srv.URL)

	pp, err := GetPhysicalProperties(context.Background(), "tok", "cv-1")
	if err != nil {
		t.Fatalf("GetPhysicalProperties: %v", err)
	}
	if pp.Status != PhysPropsStatusFailed {
		t.Errorf("status = %q, want FAILED", pp.Status)
	}
}
