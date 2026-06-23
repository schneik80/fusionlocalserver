package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
)

// handleActivityReport -> api.GetActivityFeed + api.BuildReport.
//
// Query params:
//   - hub    (required) the hub slug, e.g. "imallc" (ItemDTO.slug from /api/hubs)
//   - scope  hub | project | folder | design  (default: hub)
//   - id     scope target id (required unless scope=hub):
//     project -> publishedTo group id, folder -> folder urn,
//     design  -> permalink id or lineage urn
//   - bucket hour | day | month | year (default: day)
//   - from   lower time bound (RFC3339 or epoch ms), optional
//   - to     upper time bound (RFC3339 or epoch ms), optional
//
// The whole hub feed is fetched once (it carries every level's hierarchy) and
// aggregated to the requested scope, so project/folder/design reports need no
// extra round-trips.
func (s *Server) handleActivityReport(w http.ResponseWriter, r *http.Request) {
	hub, ok := reqParam(w, r, "hub")
	if !ok {
		return
	}
	q := r.URL.Query()

	scope := api.Scope(q.Get("scope"))
	if scope == "" {
		scope = api.ScopeHub
	}
	switch scope {
	case api.ScopeHub, api.ScopeProject, api.ScopeFolder, api.ScopeDesign:
	default:
		writeError(w, http.StatusBadRequest, "invalid scope: must be hub, project, folder, or design")
		return
	}

	id := q.Get("id")
	if scope != api.ScopeHub && id == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter: id (for scope "+string(scope)+")")
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

	events, err := api.GetActivityFeed(ctx, token, hub)
	if err != nil {
		s.fail(w, r, err)
		return
	}

	rep := api.BuildReport(events, scope, id, api.Bucket(q.Get("bucket")), from, to)
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
