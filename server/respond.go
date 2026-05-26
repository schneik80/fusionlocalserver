package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// errorResponse is the uniform error envelope every failing endpoint returns.
type errorResponse struct {
	Error string `json:"error"`
}

// writeJSON serialises v as JSON with the given status code. Encoding happens
// after WriteHeader, so a mid-encode error can't change the already-sent
// status; such errors are vanishingly rare for our DTOs and are dropped.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends the uniform {"error": "..."} envelope with the given status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

// statusForError maps an api/auth error to an HTTP status code. The wrapped
// APS layer returns plain fmt.Errorf chains, so detection leans on a couple of
// well-known markers; everything else is treated as an upstream (502) failure
// since the data ultimately comes from the APS GraphQL gateway.
func statusForError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(err, context.Canceled):
		// Client went away; no useful status, but 499-style isn't standard.
		return http.StatusGatewayTimeout
	case strings.Contains(err.Error(), "HTTP 401"), strings.Contains(err.Error(), "unauthorized"):
		return http.StatusUnauthorized
	case strings.Contains(err.Error(), "Access Denied"),
		strings.Contains(err.Error(), "does not have permission"),
		strings.Contains(err.Error(), "Forbidden"):
		return http.StatusForbidden
	default:
		return http.StatusBadGateway
	}
}
