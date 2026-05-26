package server

import (
	"context"
	"net/http"
	"time"
)

// handlerTimeout bounds every upstream APS call. Folder/version pagination and
// the iterative folder-ancestry walk can fan out into several round-trips, so
// 30s gives headroom while still capping a wedged gateway. It mirrors the
// TUI's nav-command timeout.
const handlerTimeout = 30 * time.Second

// reqCtx derives a timeout-bounded context from the request. The caller must
// defer the returned cancel.
func (s *Server) reqCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), handlerTimeout)
}

// token returns the caller's APS access token, which requireAuth resolved from
// their session and placed in the request context. It writes a 401 envelope and
// reports ok=false if absent (which shouldn't happen on a requireAuth'd route).
func (s *Server) token(ctx context.Context, w http.ResponseWriter, r *http.Request) (string, bool) {
	tok, ok := tokenFromCtx(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return "", false
	}
	return tok, true
}

// reqParam reads a required query parameter, or writes a 400 envelope and
// reports ok=false.
func reqParam(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	v := r.URL.Query().Get(name)
	if v == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter: "+name)
		return "", false
	}
	return v, true
}

// fail logs an upstream/handler error and writes the mapped status envelope.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := statusForError(err)
	s.logger.Error("handler error", "path", r.URL.Path, "query", r.URL.RawQuery, "status", status, "err", err)
	writeError(w, status, err.Error())
}

// handleAPINotFound returns a JSON 404 for unmatched /api/* paths so API
// clients never receive the SPA's index.html in place of an error envelope.
func (s *Server) handleAPINotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "no such API endpoint: "+r.URL.Path)
}
