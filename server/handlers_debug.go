package server

import (
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

// handleDebugVersionProbe runs api.ProbeVersionMilestones against a real
// document and returns each candidate query's raw outcome — a retained,
// -v-only diagnostic for confirming (or re-discovering) how a version exposes
// its root component version / isMilestone flag against the live schema.
//
// It is gated on debug tracing being active (the -v flag): with tracing off the
// route 404s exactly like any unknown /api path, so it is never reachable in a
// normal run. Query params: hubId, itemId.
func (s *Server) handleDebugVersionProbe(w http.ResponseWriter, r *http.Request) {
	if !api.DebugEnabled() {
		s.handleAPINotFound(w, r)
		return
	}
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	itemID, ok := reqParam(w, r, "itemId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, api.ProbeVersionMilestones(ctx, token, hubID, itemID))
}
