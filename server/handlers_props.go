package server

import (
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

// handleProperties -> api.GetPhysicalProperties (query: cvId). Returns the
// component version's mass/geometry properties. Generation is async, so the
// frontend polls while the status is not yet COMPLETED.
func (s *Server) handleProperties(w http.ResponseWriter, r *http.Request) {
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
	pp, err := api.GetPhysicalProperties(ctx, token, cvID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, physicalPropertiesDTO(pp))
}
