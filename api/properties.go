package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Physical-property generation status values (Manufacturing Data Model v3:
// SCHEDULED | QUEUED | IN_PROGRESS | COMPLETED | FAILED | CANCELLED).
// Generation is asynchronous, like thumbnails: a freshly-saved component may
// report a non-terminal status with empty measures, so callers poll.
const (
	PhysPropsStatusCompleted = "COMPLETED"
	PhysPropsStatusFailed    = "FAILED"
)

// Measure is one physical-property value: a display string plus its unit name.
type Measure struct {
	Display string
	Units   string
}

// PhysicalProperties holds a component's mass/geometry properties from the v3
// Manufacturing Data Model API (Component.primaryModel.physicalProperties).
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

// GetPhysicalProperties fetches a component's physical (mass) properties via
// Component.primaryModel.physicalProperties. componentID is the v3 Component id
// (the design's tip root component). Callers should treat a non-terminal status
// as "still computing" and may poll.
func GetPhysicalProperties(ctx context.Context, token, componentID string) (*PhysicalProperties, error) {
	if componentID == "" {
		return nil, fmt.Errorf("physical properties: empty componentID")
	}

	const q = `
		query GetPhysicalProperties($cv: ID!) {
			component(componentId: $cv) {
				primaryModel {
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
			}
		}`

	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentID})
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
		Component struct {
			PrimaryModel struct {
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
			} `json:"primaryModel"`
		} `json:"component"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("physical properties decode: %w", err)
	}

	pp := raw.Component.PrimaryModel.PhysicalProperties
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
