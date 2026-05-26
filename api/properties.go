package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Physical-property generation status values (Manufacturing Data Model v2).
// Generation is asynchronous, like thumbnails: a freshly-saved version may
// report a non-COMPLETED status with empty measures.
const (
	PhysPropsStatusCompleted = "COMPLETED"
	PhysPropsStatusFailed    = "FAILED"
)

// Measure is one physical-property value: a display string plus its unit name.
type Measure struct {
	Display string
	Units   string
}

// PhysicalProperties holds a component version's mass/geometry properties from
// the v2 Manufacturing Data Model API.
type PhysicalProperties struct {
	Status     string
	Area       Measure
	Volume     Measure
	Mass       Measure
	Density    Measure
	BBoxLength Measure
	BBoxWidth  Measure
	BBoxHeight Measure
}

// GetPhysicalProperties fetches a component version's physical (mass) properties.
// Callers should treat a non-COMPLETED status as "still computing" and may poll.
func GetPhysicalProperties(ctx context.Context, token, componentVersionID string) (*PhysicalProperties, error) {
	if componentVersionID == "" {
		return nil, fmt.Errorf("physical properties: empty componentVersionID")
	}

	const q = `
		query GetPhysicalProperties($cv: ID!) {
			componentVersion(componentVersionId: $cv) {
				physicalProperties {
					status
					area    { displayValue definition { units { name } } }
					volume  { displayValue definition { units { name } } }
					mass    { displayValue definition { units { name } } }
					density { displayValue definition { units { name } } }
					boundingBox {
						length { displayValue definition { units { name } } }
						width  { displayValue definition { units { name } } }
						height { displayValue definition { units { name } } }
					}
				}
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentVersionID})
	if err != nil {
		return nil, fmt.Errorf("physical properties: %w", err)
	}

	type measure struct {
		DisplayValue string `json:"displayValue"`
		Definition   struct {
			Units struct {
				Name string `json:"name"`
			} `json:"units"`
		} `json:"definition"`
	}
	var raw struct {
		ComponentVersion struct {
			PhysicalProperties *struct {
				Status      string  `json:"status"`
				Area        measure `json:"area"`
				Volume      measure `json:"volume"`
				Mass        measure `json:"mass"`
				Density     measure `json:"density"`
				BoundingBox struct {
					Length measure `json:"length"`
					Width  measure `json:"width"`
					Height measure `json:"height"`
				} `json:"boundingBox"`
			} `json:"physicalProperties"`
		} `json:"componentVersion"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("physical properties decode: %w", err)
	}

	pp := raw.ComponentVersion.PhysicalProperties
	if pp == nil {
		// Null physicalProperties (e.g. no geometry) — report FAILED so the UI
		// renders "unavailable" rather than spinning forever.
		return &PhysicalProperties{Status: PhysPropsStatusFailed}, nil
	}

	m := func(x measure) Measure { return Measure{Display: x.DisplayValue, Units: x.Definition.Units.Name} }
	status := strings.ToUpper(pp.Status)
	if status == "" {
		status = PhysPropsStatusFailed
	}
	return &PhysicalProperties{
		Status:     status,
		Area:       m(pp.Area),
		Volume:     m(pp.Volume),
		Mass:       m(pp.Mass),
		Density:    m(pp.Density),
		BBoxLength: m(pp.BoundingBox.Length),
		BBoxWidth:  m(pp.BoundingBox.Width),
		BBoxHeight: m(pp.BoundingBox.Height),
	}, nil
}
