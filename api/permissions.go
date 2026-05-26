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
