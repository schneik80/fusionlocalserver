package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ProjectGroup is a group with access to a project, and its role
// (e.g. "ADMINISTRATOR", "EDITOR"). The Manufacturing Data Model exposes
// access at the project level via groups; there is no per-user/member list on
// project, so this is the "who has access" surface.
type ProjectGroup struct {
	ID   string
	Name string
	Role string
}

// GroupMember is one user in a group. Listing members requires elevated
// (hub-admin) permissions, so GetGroupMembers may return an authorization
// error for ordinary users — callers should treat that as "not permitted"
// rather than a hard failure.
type GroupMember struct {
	UserID string
	Name   string
	Email  string
	Status string
}

// GetProjectGroups returns the groups (and roles) with access to the project.
func GetProjectGroups(ctx context.Context, token, projectID string) ([]ProjectGroup, error) {
	const qFirst = `
		query GetProjectGroups($id: ID!) {
			project(projectId: $id) {
				groups(pagination: { limit: 50 }) {
					pagination { cursor }
					results { id name role }
				}
			}
		}`
	const qNext = `
		query GetProjectGroupsNext($id: ID!, $cursor: String!) {
			project(projectId: $id) {
				groups(pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results { id name role }
				}
			}
		}`

	type groupResult struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}

	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"id": projectID}, func(data json.RawMessage) (string, []groupResult, error) {
		var r struct {
			Project struct {
				Groups struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []groupResult `json:"results"`
				} `json:"groups"`
			} `json:"project"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("project groups: %w", err)
		}
		return r.Project.Groups.Pagination.Cursor, r.Project.Groups.Results, nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]ProjectGroup, len(all))
	for i, g := range all {
		out[i] = ProjectGroup{ID: g.ID, Name: g.Name, Role: g.Role}
	}
	return out, nil
}

// Member is an individual user with a role on a project or folder, plus an
// invitation status (ACTIVE / INACTIVE / PENDING). Project contributors
// (project.folderLevelProjectMembers) and folder-level members (folder.members)
// both surface here — these are individuals, distinct from groups.
type Member struct {
	UserID string
	Name   string
	Email  string
	Role   string
	Status string
}

type memberRow struct {
	Role   string `json:"role"`
	Status string `json:"status"`
	User   struct {
		ID        string `json:"id"`
		UserName  string `json:"userName"`
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
		Email     string `json:"email"`
	} `json:"user"`
}

func memberFromRow(m memberRow) Member {
	name := strings.TrimSpace(m.User.FirstName + " " + m.User.LastName)
	if name == "" {
		name = m.User.UserName
	}
	return Member{UserID: m.User.ID, Name: name, Email: m.User.Email, Role: m.Role, Status: m.Status}
}

// GetProjectMembers returns the project's individual members (contributors) with
// their role — project.folderLevelProjectMembers. Unlike GetGroupMembers this
// does not need hub-admin: a project member can see who else is on the project.
func GetProjectMembers(ctx context.Context, token, projectID string) ([]Member, error) {
	const qFirst = `
		query PM($id: ID!) {
			project(projectId: $id) {
				folderLevelProjectMembers(pagination: { limit: 50 }) {
					pagination { cursor }
					results { role status user { id userName firstName lastName email } }
				}
			}
		}`
	const qNext = `
		query PMN($id: ID!, $cursor: String!) {
			project(projectId: $id) {
				folderLevelProjectMembers(pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results { role status user { id userName firstName lastName email } }
				}
			}
		}`
	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"id": projectID}, func(data json.RawMessage) (string, []memberRow, error) {
		var r struct {
			Project struct {
				Members struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []memberRow `json:"results"`
				} `json:"folderLevelProjectMembers"`
			} `json:"project"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("project members: %w", err)
		}
		return r.Project.Members.Pagination.Cursor, r.Project.Members.Results, nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(all))
	for _, m := range all {
		out = append(out, memberFromRow(m))
	}
	return out, nil
}

// GetFolderMembers returns a folder's individual members (folder.members) with
// their per-folder role and status — the surface where access is granted,
// raised, lowered, or removed relative to the parent.
func GetFolderMembers(ctx context.Context, token, hubID, folderID string) ([]Member, error) {
	const qFirst = `
		query FM($hubId: ID!, $fid: ID!) {
			folderByHubId(hubId: $hubId, folderId: $fid) {
				members(pagination: { limit: 50 }) {
					pagination { cursor }
					results { role status user { id userName firstName lastName email } }
				}
			}
		}`
	const qNext = `
		query FMN($hubId: ID!, $fid: ID!, $cursor: String!) {
			folderByHubId(hubId: $hubId, folderId: $fid) {
				members(pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results { role status user { id userName firstName lastName email } }
				}
			}
		}`
	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"hubId": hubID, "fid": folderID}, func(data json.RawMessage) (string, []memberRow, error) {
		var r struct {
			Folder struct {
				Members struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []memberRow `json:"results"`
				} `json:"members"`
			} `json:"folderByHubId"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("folder members: %w", err)
		}
		return r.Folder.Members.Pagination.Cursor, r.Folder.Members.Results, nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(all))
	for _, m := range all {
		out = append(out, memberFromRow(m))
	}
	return out, nil
}

// GetFolderGroups returns the groups (and roles) with access to a folder.
func GetFolderGroups(ctx context.Context, token, hubID, folderID string) ([]ProjectGroup, error) {
	const qFirst = `
		query FG($hubId: ID!, $fid: ID!) {
			folderByHubId(hubId: $hubId, folderId: $fid) {
				groups(pagination: { limit: 50 }) {
					pagination { cursor }
					results { id name role }
				}
			}
		}`
	const qNext = `
		query FGN($hubId: ID!, $fid: ID!, $cursor: String!) {
			folderByHubId(hubId: $hubId, folderId: $fid) {
				groups(pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results { id name role }
				}
			}
		}`
	type groupRow struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Role string `json:"role"`
	}
	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"hubId": hubID, "fid": folderID}, func(data json.RawMessage) (string, []groupRow, error) {
		var r struct {
			Folder struct {
				Groups struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []groupRow `json:"results"`
				} `json:"groups"`
			} `json:"folderByHubId"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("folder groups: %w", err)
		}
		return r.Folder.Groups.Pagination.Cursor, r.Folder.Groups.Results, nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]ProjectGroup, len(all))
	for i, g := range all {
		out[i] = ProjectGroup{ID: g.ID, Name: g.Name, Role: g.Role}
	}
	return out, nil
}

// GetGroupMembers returns the users in a group. group() requires hubId.
// Requires hub-admin access; returns an authorization error otherwise.
func GetGroupMembers(ctx context.Context, token, hubID, groupID string) ([]GroupMember, error) {
	const qFirst = `
		query GetGroupMembers($hubId: ID!, $gid: ID!) {
			group(hubId: $hubId, groupId: $gid) {
				members(pagination: { limit: 50 }) {
					pagination { cursor }
					results { status user { id userName firstName lastName email } }
				}
			}
		}`
	const qNext = `
		query GetGroupMembersNext($hubId: ID!, $gid: ID!, $cursor: String!) {
			group(hubId: $hubId, groupId: $gid) {
				members(pagination: { cursor: $cursor, limit: 50 }) {
					pagination { cursor }
					results { status user { id userName firstName lastName email } }
				}
			}
		}`

	type memberResult struct {
		Status string `json:"status"`
		User   struct {
			ID        string `json:"id"`
			UserName  string `json:"userName"`
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
			Email     string `json:"email"`
		} `json:"user"`
	}

	all, err := allPages(ctx, token, qFirst, qNext, map[string]any{"hubId": hubID, "gid": groupID}, func(data json.RawMessage) (string, []memberResult, error) {
		var r struct {
			Group struct {
				Members struct {
					Pagination struct {
						Cursor string `json:"cursor"`
					} `json:"pagination"`
					Results []memberResult `json:"results"`
				} `json:"members"`
			} `json:"group"`
		}
		if err := json.Unmarshal(data, &r); err != nil {
			return "", nil, fmt.Errorf("group members: %w", err)
		}
		return r.Group.Members.Pagination.Cursor, r.Group.Members.Results, nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]GroupMember, 0, len(all))
	for _, m := range all {
		name := strings.TrimSpace(m.User.FirstName + " " + m.User.LastName)
		if name == "" {
			name = m.User.UserName
		}
		out = append(out, GroupMember{UserID: m.User.ID, Name: name, Email: m.User.Email, Status: m.Status})
	}
	return out, nil
}
