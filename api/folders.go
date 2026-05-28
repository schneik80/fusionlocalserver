package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreateFolder creates a new folder in the project (optionally under
// parentFolderID; empty = project root) and returns it as a NavItem.
// Requires data:write/data:create scope.
func CreateFolder(ctx context.Context, token, projectID, parentFolderID, name string) (*NavItem, error) {
	const m = `
		mutation CreateFolder($input: CreateFolderInput!) {
			createFolder(input: $input) {
				folder { id name lastModifiedOn }
			}
		}`
	input := map[string]any{"projectId": projectID, "name": name}
	if parentFolderID != "" {
		input["parentFolderId"] = parentFolderID
	}
	data, err := gqlQuery(ctx, token, m, map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	var raw struct {
		CreateFolder struct {
			Folder struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				LastModifiedOn string `json:"lastModifiedOn"`
			} `json:"folder"`
		} `json:"createFolder"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("create folder decode: %w", err)
	}
	f := raw.CreateFolder.Folder
	return &NavItem{ID: f.ID, Name: f.Name, Kind: "folder", IsContainer: true, LastModifiedOn: f.LastModifiedOn}, nil
}

// RenameFolder renames a folder. Requires data:write scope.
func RenameFolder(ctx context.Context, token, projectID, folderID, name string) (*NavItem, error) {
	const m = `
		mutation RenameFolder($input: RenameFolderInput!) {
			renameFolder(input: $input) {
				folder { id name lastModifiedOn }
			}
		}`
	data, err := gqlQuery(ctx, token, m, map[string]any{
		"input": map[string]any{"projectId": projectID, "folderId": folderID, "name": name},
	})
	if err != nil {
		return nil, fmt.Errorf("rename folder: %w", err)
	}
	var raw struct {
		RenameFolder struct {
			Folder struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				LastModifiedOn string `json:"lastModifiedOn"`
			} `json:"folder"`
		} `json:"renameFolder"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("rename folder decode: %w", err)
	}
	f := raw.RenameFolder.Folder
	return &NavItem{ID: f.ID, Name: f.Name, Kind: "folder", IsContainer: true, LastModifiedOn: f.LastModifiedOn}, nil
}

// MoveFolder moves a folder to a new parent (under destinationProjectID;
// destinationFolderID empty = project root). v3's MoveFolderInput supports
// cross-project moves, but the UI currently confines moves to within the same
// project. Requires data:write scope.
func MoveFolder(ctx context.Context, token, folderID, destinationProjectID, destinationFolderID string) (*NavItem, error) {
	const m = `
		mutation MoveFolder($input: MoveFolderInput!) {
			moveFolder(input: $input) {
				folder { id name lastModifiedOn }
			}
		}`
	input := map[string]any{"folderId": folderID, "destinationProjectId": destinationProjectID}
	if destinationFolderID != "" {
		input["destinationFolderId"] = destinationFolderID
	}
	data, err := gqlQuery(ctx, token, m, map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("move folder: %w", err)
	}
	var raw struct {
		MoveFolder struct {
			Folder struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				LastModifiedOn string `json:"lastModifiedOn"`
			} `json:"folder"`
		} `json:"moveFolder"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("move folder decode: %w", err)
	}
	f := raw.MoveFolder.Folder
	return &NavItem{ID: f.ID, Name: f.Name, Kind: "folder", IsContainer: true, LastModifiedOn: f.LastModifiedOn}, nil
}

// DeleteFolder deletes a folder. v3's DeleteFolderInput takes hubId + folderId.
// Requires data:write scope.
func DeleteFolder(ctx context.Context, token, hubID, folderID string) error {
	const m = `
		mutation DeleteFolder($input: DeleteFolderInput!) {
			deleteFolder(input: $input) {
				folderId
			}
		}`
	_, err := gqlQuery(ctx, token, m, map[string]any{
		"input": map[string]any{"hubId": hubID, "folderId": folderID},
	})
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}
