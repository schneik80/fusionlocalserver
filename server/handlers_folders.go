package server

import (
	"encoding/json"
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

type createFolderRequest struct {
	ProjectID      string `json:"projectId"`
	ParentFolderID string `json:"parentFolderId,omitempty"`
	Name           string `json:"name"`
}

type renameFolderRequest struct {
	ProjectID string `json:"projectId"`
	FolderID  string `json:"folderId"`
	Name      string `json:"name"`
}

type moveFolderRequest struct {
	FolderID              string `json:"folderId"`
	DestinationProjectID  string `json:"destinationProjectId"`
	DestinationFolderID   string `json:"destinationFolderId,omitempty"`
}

type deleteFolderRequest struct {
	HubID    string `json:"hubId"`
	FolderID string `json:"folderId"`
}

// handleCreateFolder -> api.CreateFolder. POST body {projectId, parentFolderId?, name}.
// Returns the new folder as an ItemDTO (201).
func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	var req createFolderRequest
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
	f, err := api.CreateFolder(ctx, token, req.ProjectID, req.ParentFolderID, req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, itemDTO(*f))
}

// handleRenameFolder -> api.RenameFolder. POST body {projectId, folderId, name}.
func (s *Server) handleRenameFolder(w http.ResponseWriter, r *http.Request) {
	var req renameFolderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.ProjectID == "" || req.FolderID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "projectId, folderId, and name are required")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	f, err := api.RenameFolder(ctx, token, req.ProjectID, req.FolderID, req.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemDTO(*f))
}

// handleMoveFolder -> api.MoveFolder. POST body {folderId, destinationProjectId,
// destinationFolderId?}. destinationFolderId empty = project root.
func (s *Server) handleMoveFolder(w http.ResponseWriter, r *http.Request) {
	var req moveFolderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.FolderID == "" || req.DestinationProjectID == "" {
		writeError(w, http.StatusBadRequest, "folderId and destinationProjectId are required")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	f, err := api.MoveFolder(ctx, token, req.FolderID, req.DestinationProjectID, req.DestinationFolderID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, itemDTO(*f))
}

// handleDeleteFolder -> api.DeleteFolder. POST body {hubId, folderId}. 204 on success.
func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	var req deleteFolderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.HubID == "" || req.FolderID == "" {
		writeError(w, http.StatusBadRequest, "hubId and folderId are required")
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	if err := api.DeleteFolder(ctx, token, req.HubID, req.FolderID); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
