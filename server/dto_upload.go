package server

// UploadJobDTO is the wire shape of one upload job (GET /api/uploads and the
// POST /api/uploads response). projectId/folderId/hubId are the GraphQL ids the
// client supplied at submission, echoed back so it can invalidate the matching
// contents queries when the job lands. bytesSent tracks the APS-side transfer
// (the browser→server spool completed before the job existed).
type UploadJobDTO struct {
	ID          string   `json:"id"`
	FileName    string   `json:"fileName"`
	Size        int64    `json:"size"`
	BytesSent   int64    `json:"bytesSent"`
	Status      string   `json:"status"`
	Error       string   `json:"error,omitempty"`
	HubID       string   `json:"hubId,omitempty"`
	ProjectID   string   `json:"projectId,omitempty"`
	FolderID    string   `json:"folderId,omitempty"`
	DMProjectID string   `json:"dmProjectId,omitempty"`
	FolderPath  []string `json:"folderPath"`
	ItemID      string   `json:"itemId,omitempty"`
	VersionID   string   `json:"versionId,omitempty"` // DM version urn of the created version (for version pinning)
	CreatedOn   string   `json:"createdOn"`
}

// uploadJobDTO snapshots a job's current state.
func uploadJobDTO(j *uploadJob) UploadJobDTO {
	j.mu.Lock()
	status, errMsg, itemID, versionID := j.status, j.errMsg, j.itemID, j.versionID
	j.mu.Unlock()
	sent := j.bytesSent.Load()
	if status == uploadDone {
		sent = j.Size // progress deltas can round short of the whole; done means done
	}
	folderPath := j.FolderPath
	if folderPath == nil {
		folderPath = []string{}
	}
	return UploadJobDTO{
		ID:          j.ID,
		FileName:    j.FileName,
		Size:        j.Size,
		BytesSent:   sent,
		Status:      string(status),
		Error:       errMsg,
		HubID:       j.HubID,
		ProjectID:   j.ProjectID,
		FolderID:    j.FolderID,
		DMProjectID: j.DMProjectID,
		FolderPath:  folderPath,
		ItemID:      itemID,
		VersionID:   versionID,
		CreatedOn:   fmtTime(j.CreatedAt),
	}
}

func uploadJobDTOs(jobs []*uploadJob) []UploadJobDTO {
	out := make([]UploadJobDTO, len(jobs))
	for i, j := range jobs {
		out[i] = uploadJobDTO(j)
	}
	return out
}
