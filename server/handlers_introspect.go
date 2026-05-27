package server

// TEMPORARY diagnostic — remove after the v3 schema is confirmed.
//
// handleIntrospectV3 runs per-type GraphQL introspection against the LIVE v3
// endpoint using the caller's session token, and logs the result to the server
// log. It exists only to map the real v3 schema (the bundled demo schema may be
// a curated/stale subset). Auth-gated like every other data route.
//
// Usage:
//   GET /api/_introspect-v3            -> a fixed list of core object types
//   GET /api/_introspect-v3?type=Name  -> one type in full detail (fields+arg
//                                         types, enumValues, inputFields,
//                                         possibleTypes) — enums, inputs, unions.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const v3Endpoint = "https://developer.api.autodesk.com/mfg/v3/graphql/public"

func (s *Server) handleIntrospectV3(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}

	names := []string{
		"Component", "Model", "DesignItem", "ConfiguredDesignItem",
		"Drawing", "DrawingItem", "AssemblyRelation", "BOMRelation",
		"Item", "Hub", "Project",
	}
	if t := r.URL.Query().Get("type"); t != "" {
		names = []string{t}
	}

	out := map[string]any{}
	for _, t := range names {
		ti, err := introspectType(ctx, token, t)
		if err != nil {
			s.logger.Error("introspect v3", "type", t, "err", err)
			out[t] = "ERROR: " + err.Error()
			continue
		}
		s.logger.Info("introspect v3", "type", ti.Name, "kind", ti.Kind, "fields", strings.Join(ti.fieldStrings(), ", "))
		if len(ti.EnumValues) > 0 {
			s.logger.Info("introspect v3 enum", "type", ti.Name, "values", strings.Join(enumNames(ti.EnumValues), ", "))
		}
		if len(ti.InputFields) > 0 {
			s.logger.Info("introspect v3 input", "type", ti.Name, "inputFields", strings.Join(inputStrings(ti.InputFields), ", "))
		}
		if len(ti.PossibleTypes) > 0 {
			s.logger.Info("introspect v3 union", "type", ti.Name, "possibleTypes", strings.Join(possibleNames(ti.PossibleTypes), ", "))
		}
		out[t] = ti
	}
	writeJSON(w, http.StatusOK, out)
}

type introspectTypeRef struct {
	Kind   string             `json:"kind"`
	Name   string             `json:"name"`
	OfType *introspectTypeRef `json:"ofType"`
}

func (tr introspectTypeRef) String() string {
	switch tr.Kind {
	case "NON_NULL":
		if tr.OfType != nil {
			return tr.OfType.String() + "!"
		}
	case "LIST":
		if tr.OfType != nil {
			return "[" + tr.OfType.String() + "]"
		}
	}
	if tr.Name != "" {
		return tr.Name
	}
	return tr.Kind
}

type introspectArg struct {
	Name string            `json:"name"`
	Type introspectTypeRef `json:"type"`
}

type introspectField struct {
	Name string            `json:"name"`
	Args []introspectArg   `json:"args"`
	Type introspectTypeRef `json:"type"`
}

type introspectTypeInfo struct {
	Name       string            `json:"name"`
	Kind       string            `json:"kind"`
	Fields     []introspectField `json:"fields"`
	EnumValues []struct {
		Name string `json:"name"`
	} `json:"enumValues"`
	InputFields   []introspectArg `json:"inputFields"`
	PossibleTypes []struct {
		Name string `json:"name"`
	} `json:"possibleTypes"`
}

func (ti introspectTypeInfo) fieldStrings() []string {
	out := make([]string, 0, len(ti.Fields))
	for _, f := range ti.Fields {
		entry := f.Name
		if len(f.Args) > 0 {
			parts := make([]string, 0, len(f.Args))
			for _, a := range f.Args {
				parts = append(parts, a.Name+":"+a.Type.String())
			}
			entry += "(" + strings.Join(parts, ", ") + ")"
		}
		entry += ":" + f.Type.String()
		out = append(out, entry)
	}
	return out
}

func enumNames(vals []struct {
	Name string `json:"name"`
}) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, v.Name)
	}
	return out
}

func possibleNames(vals []struct {
	Name string `json:"name"`
}) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, v.Name)
	}
	return out
}

func inputStrings(args []introspectArg) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		out = append(out, a.Name+":"+a.Type.String())
	}
	return out
}

func introspectType(ctx context.Context, token, name string) (introspectTypeInfo, error) {
	// name comes from a fixed in-code list or a controlled query param (only used
	// to read schema metadata), so inlining it sidesteps the v3 gateway's
	// variable handling.
	const ref = `kind name ofType { kind name ofType { kind name ofType { kind name } } }`
	query := fmt.Sprintf(`query {
  __type(name: %q) {
    name kind
    fields(includeDeprecated: true) {
      name
      args { name type { %s } }
      type { %s }
    }
    enumValues(includeDeprecated: true) { name }
    inputFields { name type { %s } }
    possibleTypes { name }
  }
}`, name, ref, ref, ref)

	body, _ := json.Marshal(map[string]any{"query": query})
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, v3Endpoint, bytes.NewReader(body))
	if err != nil {
		return introspectTypeInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return introspectTypeInfo{}, err
	}
	defer resp.Body.Close()

	var parsed struct {
		Data struct {
			Type *introspectTypeInfo `json:"__type"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return introspectTypeInfo{}, fmt.Errorf("decode (http %d): %w", resp.StatusCode, err)
	}
	if len(parsed.Errors) > 0 {
		return introspectTypeInfo{}, fmt.Errorf("http %d: %s", resp.StatusCode, parsed.Errors[0].Message)
	}
	if parsed.Data.Type == nil {
		return introspectTypeInfo{}, fmt.Errorf("http %d: no such type", resp.StatusCode)
	}
	return *parsed.Data.Type, nil
}
