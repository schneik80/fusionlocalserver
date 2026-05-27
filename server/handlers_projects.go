package server

import (
	"encoding/json"
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

type createProjectRequest struct {
	HubID string `json:"hubId"`
	Name  string `json:"name"`
}

type renameProjectRequest struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
}

type archiveProjectRequest struct {
	ProjectID string `json:"projectId"`
}

// handleCreateProject -> api.CreateProject. POST body {hubId, name}. Returns the
// new project as an ItemDTO (201).
func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.HubID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "hubId and name are required")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	p, err := api.CreateProject(ctx, token, req.HubID, req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemDTO(*p))
}

// handleRenameProject -> api.RenameProject. POST body {projectId, name}.
func (s *Server) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	var req renameProjectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.ProjectID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "projectId and name are required")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	p, err := api.RenameProject(ctx, token, req.ProjectID, req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemDTO(*p))
}

// handleArchiveProject -> api.ArchiveProject. POST body {projectId}. 204 on success.
func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	var req archiveProjectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.ProjectID == "" {
		writeError(w, http.StatusBadRequest, "projectId is required")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	if err := api.ArchiveProject(ctx, token, req.ProjectID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
