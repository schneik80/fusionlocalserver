package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
)

// handleActivityReport -> api.GetDesignActivity + api.BuildReport. Design scope
// only (the notifications feed is first-party-gated for this app's token).
//
// Query params:
//   - hubId  (required) the GraphQL hub id (urn:adsk.ace:prod.scope:…)
//   - id     (required) the item/lineage id (urn:adsk.wipprod:dm.lineage:…)
//   - scope  optional; if given must be "design"
//   - bucket hour | day | month | year (default: day)
//   - from   lower time bound (RFC3339 or epoch ms), optional
//   - to     upper time bound (RFC3339 or epoch ms), optional
func (s *Server) handleActivityReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Design scope only. Activity is sourced from the Manufacturing Data Model
	// GraphQL — the Fusion Team notifications feed is first-party-gated (returns
	// HTTP 500 for this app's token), so hub/project/folder reports are not
	// offered. hubId is the GraphQL hub id and id the item/lineage id, the same
	// pair the Details endpoint uses.
	if scope := q.Get("scope"); scope != "" && scope != string(api.ScopeDesign) {
		writeError(w, http.StatusBadRequest, "only scope=design is supported")
		return
	}

	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	itemID := q.Get("id")
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter: id (the item/lineage id)")
		return
	}
	from, ok := parseQueryTime(w, q.Get("from"), "from")
	if !ok {
		return
	}
	to, ok := parseQueryTime(w, q.Get("to"), "to")
	if !ok {
		return
	}

	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}

	events, err := api.GetDesignActivity(ctx, token, hubID, itemID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rep := api.BuildReport(events, api.ScopeDesign, itemID, api.Bucket(q.Get("bucket")), from, to)
	writeJSON(w, http.StatusOK, activityReportDTO(rep))
}

// parseQueryTime parses an optional time query param accepting either RFC3339
// or epoch-milliseconds. Empty yields the zero time (unbounded). On a malformed
// value it writes a 400 and reports ok=false.
func parseQueryTime(w http.ResponseWriter, v, name string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, true
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
		return time.UnixMilli(ms).UTC(), true
	}
	writeError(w, http.StatusBadRequest, "invalid time for "+name+": use RFC3339 or epoch milliseconds")
	return time.Time{}, false
}
