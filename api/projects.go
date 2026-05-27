package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreateProject creates a new project in the given hub (v3 createProject
// mutation) and returns it as a NavItem. Requires data:write/data:create scope.
func CreateProject(ctx context.Context, token, hubID, name string) (*NavItem, error) {
	const m = `
		mutation CreateProject($input: CreateProjectInput!) {
			createProject(input: $input) {
				project { id name fusionWebUrl }
			}
		}`
	data, err := gqlQuery(ctx, token, m, map[string]any{
		"input": map[string]any{"hubId": hubID, "name": name},
	})
	if err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}
	var raw struct {
		CreateProject struct {
			Project struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				FusionWebURL string `json:"fusionWebUrl"`
			} `json:"project"`
		} `json:"createProject"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("create project decode: %w", err)
	}
	p := raw.CreateProject.Project
	return &NavItem{ID: p.ID, Name: p.Name, Kind: "project", WebURL: p.FusionWebURL, IsContainer: true}, nil
}

// RenameProject renames a project (v3 renameProject mutation). Requires
// data:write scope.
func RenameProject(ctx context.Context, token, projectID, name string) (*NavItem, error) {
	const m = `
		mutation RenameProject($input: RenameProjectInput!) {
			renameProject(input: $input) {
				project { id name fusionWebUrl }
			}
		}`
	data, err := gqlQuery(ctx, token, m, map[string]any{
		"input": map[string]any{"projectId": projectID, "name": name},
	})
	if err != nil {
		return nil, fmt.Errorf("rename project: %w", err)
	}
	var raw struct {
		RenameProject struct {
			Project struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				FusionWebURL string `json:"fusionWebUrl"`
			} `json:"project"`
		} `json:"renameProject"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("rename project decode: %w", err)
	}
	p := raw.RenameProject.Project
	return &NavItem{ID: p.ID, Name: p.Name, Kind: "project", WebURL: p.FusionWebURL, IsContainer: true}, nil
}

// ArchiveProject archives a project (v3 archiveProject mutation; reversible via
// restoreProject). Requires data:write scope.
func ArchiveProject(ctx context.Context, token, projectID string) error {
	const m = `
		mutation ArchiveProject($input: ArchiveProjectInput!) {
			archiveProject(input: $input) {
				project { id }
			}
		}`
	_, err := gqlQuery(ctx, token, m, map[string]any{
		"input": map[string]any{"projectId": projectID},
	})
	if err != nil {
		return fmt.Errorf("archive project: %w", err)
	}
	return nil
}
