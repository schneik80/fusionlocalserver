package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// NamedProperty is one named property of a component version (a custom or
// standard property), with its display value. This is the v2 analog of the v3
// API's baseProperties.
type NamedProperty struct {
	Name         string
	DisplayValue string
}

// GetCustomProperties returns a component version's custom/standard named
// properties. Best-effort enrichment for the Details panel — callers should
// tolerate an empty list (or an error) rather than treat it as fatal.
func GetCustomProperties(ctx context.Context, token, componentVersionID string) ([]NamedProperty, error) {
	if componentVersionID == "" {
		return nil, fmt.Errorf("custom properties: empty componentVersionID")
	}
	const q = `
		query GetCustomProperties($cv: ID!) {
			componentVersion(componentVersionId: $cv) {
				customProperties {
					results { name displayValue }
				}
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"cv": componentVersionID})
	if err != nil {
		return nil, fmt.Errorf("custom properties: %w", err)
	}
	var raw struct {
		ComponentVersion struct {
			CustomProperties struct {
				Results []struct {
					Name         string `json:"name"`
					DisplayValue string `json:"displayValue"`
				} `json:"results"`
			} `json:"customProperties"`
		} `json:"componentVersion"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("custom properties decode: %w", err)
	}
	out := make([]NamedProperty, 0, len(raw.ComponentVersion.CustomProperties.Results))
	for _, p := range raw.ComponentVersion.CustomProperties.Results {
		if p.Name == "" {
			continue
		}
		out = append(out, NamedProperty{Name: p.Name, DisplayValue: p.DisplayValue})
	}
	return out, nil
}
