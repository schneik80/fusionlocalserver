package server

import "net/http"

// handleStub serves the Fusion Open/Insert and STEP download endpoints, which
// are intentionally unimplemented this iteration. The UI disables their
// buttons; this 501 is the backend half of that contract, kept explicit so the
// routes exist and can be wired up later without a frontend change.
func (s *Server) handleStub(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented yet: Fusion open/insert and STEP download are stubbed in this build")
}

// handleAPINotFound returns a JSON 404 for unmatched /api/* paths so API
// clients never receive the SPA's index.html in place of an error envelope.
func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "no such API endpoint: "+r.URL.Path)
}
