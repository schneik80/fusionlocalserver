package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// NamedProperty is one named property of a component (an extended/base property
// or a user-defined custom property), with its display value, shown in the
// Details "Properties" tab.
type NamedProperty struct {
	Name         string
	DisplayValue string
}

// GetCustomProperties returns a component's populated named properties: the v3
// extended baseProperties (the Autodesk-defined attributes — Stock Number,
// Vendor, Manufacturer, …) plus any user-defined customProperties. componentID
// is the v3 Component id. Best-effort enrichment for the Details panel — callers
// should tolerate an empty list (or an error) rather than treat it as fatal.
func GetCustomProperties(ctx context.Context, token, componentID string) ([]NamedProperty, error) {
	if componentID == "" {
		return nil, fmt.Errorf("properties: empty componentID")
	}
	const q = `
		query GetComponentProperties($cv: ID!) {
			component(componentId: $cv) {
				baseProperties {
					results { name displayValue value }
				}
				customProperties {
					results { name displayValue value }
				}
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentID})
	if err != nil {
		return nil, fmt.Errorf("properties: %w", err)
	}
	type propList struct {
		Results []Property `json:"results"`
	}
	var raw struct {
		Component struct {
			BaseProperties   propList `json:"baseProperties"`
			CustomProperties propList `json:"customProperties"`
		} `json:"component"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("properties decode: %w", err)
	}
	results := append(raw.Component.BaseProperties.Results, raw.Component.CustomProperties.Results...)
	out := make([]NamedProperty, 0, len(results))
	for _, p := range results {
		v := p.Str()
		if p.Name == "" || v == "" {
			continue // skip unpopulated properties
		}
		out = append(out, NamedProperty{Name: p.Name, DisplayValue: v})
	}
	return out, nil
}
