// Package testutil provides a shared test helper for spinning up an in-process
// httptest.Server that emulates the upstream APS GraphQL endpoint. Tests in the
// api package issue real HTTP requests against the fake; keeping that
// boilerplate here lets per-package test files stay focused on the behaviour
// under test.
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// GraphQLRequest is the decoded view of an APS GraphQL POST that the
// handler sees. AuthHeader is the raw Authorization header value (typically
// "Bearer <token>"); Region is the X-Ads-Region header (empty if unset).
type GraphQLRequest struct {
	Query      string
	Variables  map[string]any
	AuthHeader string
	Region     string
}

// GraphQLResponse is what a GraphQLServer handler returns for a given
// request. Either Data (marshaled into the response's "data" field) or
// Errors (each becomes {"message": ...} in the response's "errors" array)
// or both may be set. Status defaults to 200; RawBody, if non-empty, is
// sent verbatim and Data/Errors/Status are ignored.
type GraphQLResponse struct {
	Data    any
	Errors  []string
	Status  int
	RawBody string
}

// GraphQLServer starts an httptest.Server that decodes APS GraphQL
// requests and feeds them to handler, replying with whatever
// GraphQLResponse the handler returns. The server is closed via
// t.Cleanup, so callers don't have to defer Close().
//
// The fake doesn't enforce method or Content-Type — those are the
// caller's job to assert on the captured request when relevant.
func GraphQLServer(t *testing.T, handler func(req GraphQLRequest) GraphQLResponse) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("testutil: reading request body: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("testutil: decoding GraphQL request body %q: %v", body, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := handler(GraphQLRequest{
			Query:      req.Query,
			Variables:  req.Variables,
			AuthHeader: r.Header.Get("Authorization"),
			Region:     r.Header.Get("X-Ads-Region"),
		})

		if resp.Status == 0 {
			resp.Status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.Status)

		if resp.RawBody != "" {
			_, _ = io.WriteString(w, resp.RawBody)
			return
		}

		envelope := map[string]any{}
		if resp.Data != nil {
			envelope["data"] = resp.Data
		}
		if len(resp.Errors) > 0 {
			errs := make([]map[string]string, len(resp.Errors))
			for i, m := range resp.Errors {
				errs[i] = map[string]string{"message": m}
			}
			envelope["errors"] = errs
		}
		// Always emit a "data" key even when nil, so the client's empty-data
		// branch is reachable: the production gqlQuery returns an error if
		// the JSON has zero bytes in `data`. Encoding `nil` produces "null"
		// which still has length, so handlers that want to trigger the
		// "empty data" path should set Data to json.RawMessage("") explicitly
		// (or use RawBody for that).
		_ = json.NewEncoder(w).Encode(envelope)
	}))
	t.Cleanup(srv.Close)
	return srv
}
