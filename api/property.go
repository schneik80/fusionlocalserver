package api

import (
	"encoding/json"
	"strings"
)

// Property mirrors the v3 GraphQL Property object. A major v3 change from v2 is
// that a component/model's name, partNumber, materialName and description are
// no longer plain strings — they are Property objects carrying a typed value
// plus a localized display string. Select `{ value displayValue }` (and
// optionally `definition { id }`) wherever a Property is returned, decode into
// this struct, and call Str() for a human-readable string.
type Property struct {
	Name         string          `json:"name"`
	DisplayValue string          `json:"displayValue"`
	Value        json.RawMessage `json:"value"`
	Definition   struct {
		ID string `json:"id"`
	} `json:"definition"`
}

// Str returns the best human-readable representation: the localized
// displayValue when present, otherwise the raw scalar `value` rendered as a
// string (PropertyValue is an opaque scalar, usually a JSON string or number).
// Returns "" when the property is absent/null.
func (p Property) Str() string {
	if p.DisplayValue != "" {
		return p.DisplayValue
	}
	if len(p.Value) == 0 || string(p.Value) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(p.Value, &s) == nil {
		return s
	}
	return strings.Trim(string(p.Value), `"`)
}

// NamedValue is one resolved property (name + display value) used by the
// Details "Properties" tab. In v3 these come from Component.baseProperties /
// customProperties / allProperties.
type NamedValue struct {
	Name  string
	Value string
}
