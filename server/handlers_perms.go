package server

import (
	"net/http"
	"sync"

	"github.com/schneik80/fusionlocalserver/api"
)

// MemberDTO is an individual user with a role + status on a project or folder.
type MemberDTO struct {
	UserID string `json:"userId"`
	Name   string `json:"name"`
	Email  string `json:"email,omitempty"`
	Role   string `json:"role"`
	Status string `json:"status,omitempty"`
}

// PermLayerDTO is one layer of the access path (the project, or a folder) with
// the groups and individual members granted there.
type PermLayerDTO struct {
	Type    string            `json:"type"` // "project" | "folder"
	ID      string            `json:"id"`
	Name    string            `json:"name,omitempty"`
	Groups  []ProjectGroupDTO `json:"groups"`
	Members []MemberDTO       `json:"members"`
}

func groupDTOs(gs []api.ProjectGroup) []ProjectGroupDTO {
	out := make([]ProjectGroupDTO, 0, len(gs))
	for _, g := range gs {
		out = append(out, ProjectGroupDTO{ID: g.ID, Name: g.Name, Role: g.Role})
	}
	return out
}

func memberDTOs(ms []api.Member) []MemberDTO {
	out := make([]MemberDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, MemberDTO{UserID: m.UserID, Name: m.Name, Email: m.Email, Role: m.Role, Status: m.Status})
	}
	return out
}

// handlePermissionsPath returns the access at each layer of a document's path:
// the project (groups + folder-level project members), then each folder
// (members + groups), in root→leaf order. The frontend resolves the per-principal
// inheritance/override cascade from these layers. Query params: hubId, projectId,
// and repeated folderId (+ optional projectName / folderName) in root→leaf order.
// Per-layer fetch errors yield an empty part rather than failing the whole call.
func (s *Server) handlePermissionsPath(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	hubID := q.Get("hubId")
	projectID := q.Get("projectId")
	if hubID == "" || projectID == "" {
		writeError(w, http.StatusBadRequest, "hubId and projectId are required")
		return
	}
	folderIDs := q["folderId"]
	folderNames := q["folderName"]

	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}

	layers := make([]PermLayerDTO, 1+len(folderIDs))
	var wg sync.WaitGroup

	// Project layer: groups + folder-level project members.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var g []api.ProjectGroup
		var m []api.Member
		var w2 sync.WaitGroup
		w2.Add(2)
		go func() { defer w2.Done(); g, _ = api.GetProjectGroups(ctx, token, projectID) }()
		go func() { defer w2.Done(); m, _ = api.GetProjectMembers(ctx, token, projectID) }()
		w2.Wait()
		layers[0] = PermLayerDTO{Type: "project", ID: projectID, Name: q.Get("projectName"), Groups: groupDTOs(g), Members: memberDTOs(m)}
	}()

	// Folder layers: members + groups.
	for i, fid := range folderIDs {
		wg.Add(1)
		go func(i int, fid string) {
			defer wg.Done()
			var g []api.ProjectGroup
			var m []api.Member
			var w2 sync.WaitGroup
			w2.Add(2)
			go func() { defer w2.Done(); g, _ = api.GetFolderGroups(ctx, token, hubID, fid) }()
			go func() { defer w2.Done(); m, _ = api.GetFolderMembers(ctx, token, hubID, fid) }()
			w2.Wait()
			name := ""
			if i < len(folderNames) {
				name = folderNames[i]
			}
			layers[i+1] = PermLayerDTO{Type: "folder", ID: fid, Name: name, Groups: groupDTOs(g), Members: memberDTOs(m)}
		}(i, fid)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, layers)
}

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
