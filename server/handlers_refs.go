package server

import (
	"net/http"

	"github.com/schneik80/FusionDataCLI/api"
)

// handleUses is polymorphic, mirroring the TUI's Uses tab:
//   - designs: occurrences of the component version (query: cvId)
//   - drawings: the source design the drawing was made from
//     (query: hubId, drawingItemId)
//
// Both shapes return a list of ComponentRef rows.
func (s *Server) handleUses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cvID := q.Get("cvId")
	hubID := q.Get("hubId")
	drawingItemID := q.Get("drawingItemId")

	if cvID == "" && (hubID == "" || drawingItemID == "") {
		writeError(w, http.StatusBadRequest, "provide either cvId, or both hubId and drawingItemId")
		return
	}

	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}

	var (
		refs []api.ComponentRef
		err  error
	)
	if cvID != "" {
		refs, err = api.GetOccurrences(ctx, token, cvID)
	} else {
		refs, err = api.GetDrawingSource(ctx, token, hubID, drawingItemID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, componentRefDTOs(refs))
}

// handleWhereUsed -> api.GetWhereUsed (query: cvId).
func (s *Server) handleWhereUsed(w http.ResponseWriter, r *http.Request) {
	cvID, ok := reqParam(w, r, "cvId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	refs, err := api.GetWhereUsed(ctx, token, cvID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, componentRefDTOs(refs))
}

// handleDrawings -> api.GetDrawingsForDesign (query: hubId, designItemId).
func (s *Server) handleDrawings(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	designItemID, ok := reqParam(w, r, "designItemId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	refs, err := api.GetDrawingsForDesign(ctx, token, hubID, designItemID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, drawingRefDTOs(refs))
}

// handleClassify -> api.ClassifyAssembly (query: cvId). The frontend fires one
// request per design row after a folder loads to upgrade the icon to
// assembly/part. Concurrency is bounded inside the api package (classifySem
// caps at 8), so the server can accept the fan-out without extra throttling.
func (s *Server) handleClassify(w http.ResponseWriter, r *http.Request) {
	cvID, ok := reqParam(w, r, "cvId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	isAssembly, err := api.ClassifyAssembly(ctx, token, cvID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	subtype := "part"
	if isAssembly {
		subtype = "assembly"
	}
	writeJSON(w, http.StatusOK, ClassifyDTO{
		ComponentVersionID: cvID,
		IsAssembly:         isAssembly,
		Subtype:            subtype,
	})
}
