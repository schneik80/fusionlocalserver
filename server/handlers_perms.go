package server

import (
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

// handleProjectGroups -> api.GetProjectGroups (query: projectId). The groups
// (and roles) with access to the item's project — the Permissions tab.
func (s *Server) handleProjectGroups(w http.ResponseWriter, r *http.Request) {
	projectID, ok := reqParam(w, r, "projectId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	groups, err := api.GetProjectGroups(ctx, token, projectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]ProjectGroupDTO, len(groups))
	for i, g := range groups {
		out[i] = ProjectGroupDTO{ID: g.ID, Name: g.Name, Role: g.Role}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGroupMembers -> api.GetGroupMembers (query: hubId, groupId). Listing
// members needs hub-admin access; a 403 here is expected for ordinary users
// and the SPA shows it as "no permission" rather than an error.
func (s *Server) handleGroupMembers(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	groupID, ok := reqParam(w, r, "groupId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	members, err := api.GetGroupMembers(ctx, token, hubID, groupID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]GroupMemberDTO, len(members))
	for i, m := range members {
		out[i] = GroupMemberDTO{UserID: m.UserID, Name: m.Name, Email: m.Email, Status: m.Status}
	}
	writeJSON(w, http.StatusOK, out)
}
