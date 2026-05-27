package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// NamedProperty is one named property of a component shown in the Details
// "Properties" tab: an extended/base property (the Autodesk-defined attributes
// such as Stock Number, Vendor, Manufacturer) or a user-defined custom
// property. DisplayValue may be empty for a base property that hasn't been set.
type NamedProperty struct {
	Name         string
	DisplayValue string
}

// GetCustomProperties returns a component's "extended" properties for the
// Details panel: every (non-hidden) base-property DEFINITION on the hub —
// populated with the component's value where set — plus any user-defined custom
// properties that have a value. hubID supplies the base-property definitions
// (the fields exist hub-wide even when a given component leaves them blank);
// componentID supplies the values. Best-effort enrichment — callers should
// tolerate an empty list or an error.
func GetCustomProperties(ctx context.Context, token, hubID, componentID string) ([]NamedProperty, error) {
	if componentID == "" {
		return nil, fmt.Errorf("properties: empty componentID")
	}
	const q = `
		query GetItemProperties($hubId: ID!, $cv: ID!) {
			hub(hubId: $hubId) {
				basePropertyDefinitionCollections {
					results {
						definitions {
							results { id name isHidden isArchived }
						}
					}
				}
			}
			component(componentId: $cv) {
				baseProperties { results { displayValue value definition { id } } }
				customProperties { results { name displayValue value } }
			}
		}`
	data, err := gqlQuery(ctx, token, q, map[string]any{"hubId": hubID, "cv": componentID})
	if err != nil {
		return nil, fmt.Errorf("properties: %w", err)
	}

	var raw struct {
		Hub struct {
			BasePropertyDefinitionCollections struct {
				Results []struct {
					Definitions struct {
						Results []struct {
							ID         string `json:"id"`
							Name       string `json:"name"`
							IsHidden   bool   `json:"isHidden"`
							IsArchived bool   `json:"isArchived"`
						} `json:"results"`
					} `json:"definitions"`
				} `json:"results"`
			} `json:"basePropertyDefinitionCollections"`
		} `json:"hub"`
		Component struct {
			BaseProperties struct {
				Results []struct {
					DisplayValue string          `json:"displayValue"`
					Value        json.RawMessage `json:"value"`
					Definition   struct {
						ID string `json:"id"`
					} `json:"definition"`
				} `json:"results"`
			} `json:"baseProperties"`
			CustomProperties struct {
				Results []Property `json:"results"`
			} `json:"customProperties"`
		} `json:"component"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("properties decode: %w", err)
	}

	// Map the component's base-property values by definition id.
	values := make(map[string]string)
	for _, p := range raw.Component.BaseProperties.Results {
		v := p.DisplayValue
		if v == "" {
			v = (Property{Value: p.Value}).Str()
		}
		values[p.Definition.ID] = v
	}

	// Emit every visible base-property definition (in hub order), populated with
	// the component's value where present — so the user sees the full extended-
	// property surface even when most fields are blank.
	out := make([]NamedProperty, 0)
	for _, coll := range raw.Hub.BasePropertyDefinitionCollections.Results {
		for _, def := range coll.Definitions.Results {
			if def.IsHidden || def.IsArchived || def.Name == "" {
				continue
			}
			out = append(out, NamedProperty{Name: def.Name, DisplayValue: values[def.ID]})
		}
	}

	// Append populated custom properties.
	for _, p := range raw.Component.CustomProperties.Results {
		v := p.Str()
		if p.Name == "" || v == "" {
			continue
		}
		out = append(out, NamedProperty{Name: p.Name, DisplayValue: v})
	}
	return out, nil
}
