package server

import (
	"github.com/schneik80/fusionlocalserver/production"
)

// Production DTOs. Every DTO maps a production store type to the wire with
// fmtTime timestamps and non-nil slices, the same shape rules as the task
// DTOs. A Job carries its full graph (steps, edges, batches) so a single
// GET /api/production/job hydrates the whole canvas.

// ProdUserDTO is a job/step/batch participant on the wire.
type ProdUserDTO struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

// ProdDocDTO is a version-pinned document reference on the wire.
type ProdDocDTO struct {
	HubID                  string `json:"hubId"`
	ItemID                 string `json:"itemId"`
	Name                   string `json:"name"`
	Kind                   string `json:"kind,omitempty"`
	VersionID              string `json:"versionId"`
	VersionNumber          int    `json:"versionNumber"`
	RootComponentVersionID string `json:"rootComponentVersionId,omitempty"`
	DMProjectID            string `json:"dmProjectId,omitempty"`
}

// ProdPlanDocDTO is a plan document attached to a step.
type ProdPlanDocDTO struct {
	ID      string      `json:"id"`
	Doc     ProdDocDTO  `json:"doc"`
	AddedBy ProdUserDTO `json:"addedBy"`
	AddedAt string      `json:"addedAt"`
}

// ProdPlaceholderDTO is a per-batch document slot on a step.
type ProdPlaceholderDTO struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind,omitempty"`
	Required bool   `json:"required"`
}

// ProdStepDTO is one flow node.
type ProdStepDTO struct {
	ID           string               `json:"id"`
	Num          int64                `json:"num"`
	Title        string               `json:"title"`
	Description  string               `json:"description,omitempty"`
	X            float64              `json:"x"`
	Y            float64              `json:"y"`
	PlanDocs     []ProdPlanDocDTO     `json:"planDocs"`
	Placeholders []ProdPlaceholderDTO `json:"placeholders"`
	CreatedAt    string               `json:"createdAt"`
	UpdatedAt    string               `json:"updatedAt"`
}

// ProdEdgeDTO is one directed link between steps.
type ProdEdgeDTO struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// ProdBatchStepDTO is a frozen step within a batch: identity, pinned plan
// documents, and placeholder slots as they stood at batch creation.
type ProdBatchStepDTO struct {
	StepID       string               `json:"stepId"`
	Num          int64                `json:"num"`
	Title        string               `json:"title"`
	PlanDocs     []ProdPlanDocDTO     `json:"planDocs"`
	Placeholders []ProdPlaceholderDTO `json:"placeholders"`
}

// ProdFulfillmentDTO is one supplied document in a batch.
type ProdFulfillmentDTO struct {
	ID            string      `json:"id"`
	StepID        string      `json:"stepId"`
	PlaceholderID string      `json:"placeholderId,omitempty"`
	Doc           ProdDocDTO  `json:"doc"`
	Source        string      `json:"source,omitempty"`
	IsAsRun       bool        `json:"isAsRun"`
	SuppliedBy    ProdUserDTO `json:"suppliedBy"`
	SuppliedAt    string      `json:"suppliedAt"`
}

// ProdBatchDTO is a dated run of a job.
type ProdBatchDTO struct {
	ID           string               `json:"id"`
	Num          int64                `json:"num"`
	Name         string               `json:"name"`
	Kind         string               `json:"kind"`
	RunAt        string               `json:"runAt"`
	Status       string               `json:"status"`
	Steps        []ProdBatchStepDTO   `json:"steps"`
	Fulfillments []ProdFulfillmentDTO `json:"fulfillments"`
	Refs         []string             `json:"refs"`
	CreatedBy    ProdUserDTO          `json:"createdBy"`
	CreatedAt    string               `json:"createdAt"`
	UpdatedAt    string               `json:"updatedAt"`
}

// ProdJobDTO is a job with its full graph.
type ProdJobDTO struct {
	ID          string         `json:"id"`
	Num         int64          `json:"num"`
	ProjectID   string         `json:"projectId"`
	HubID       string         `json:"hubId"`
	ProjectName string         `json:"projectName"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []ProdStepDTO  `json:"steps"`
	Edges       []ProdEdgeDTO  `json:"edges"`
	Batches     []ProdBatchDTO `json:"batches"`
	CreatedBy   ProdUserDTO    `json:"createdBy"`
	CreatedAt   string         `json:"createdAt"`
	UpdatedAt   string         `json:"updatedAt"`
}

// ProdCapsDTO tells the SPA what the caller may do, so it can disable
// affordances for read-only roles (TaskCapsDTO precedent).
type ProdCapsDTO struct {
	Write    bool `json:"write"`
	Moderate bool `json:"moderate"`
}

// ProdJobListDTO is GET /api/production/jobs. Listed jobs carry the full graph
// (one mapping path); the list view uses only the summary fields.
type ProdJobListDTO struct {
	Jobs         []ProdJobDTO `json:"jobs"`
	Capabilities ProdCapsDTO  `json:"capabilities"`
}

func prodUserDTO(r production.UserRef) ProdUserDTO {
	return ProdUserDTO{ID: r.ID, Name: r.Name, Email: r.Email}
}

func prodDocDTO(d production.DocSnapshot) ProdDocDTO {
	return ProdDocDTO{
		HubID:                  d.HubID,
		ItemID:                 d.ItemID,
		Name:                   d.Name,
		Kind:                   d.Kind,
		VersionID:              d.VersionID,
		VersionNumber:          d.VersionNumber,
		RootComponentVersionID: d.RootComponentVersionID,
		DMProjectID:            d.DMProjectID,
	}
}

func prodPlanDocDTOs(docs []production.PlanDoc) []ProdPlanDocDTO {
	out := make([]ProdPlanDocDTO, 0, len(docs))
	for _, pd := range docs {
		out = append(out, ProdPlanDocDTO{
			ID:      pd.ID,
			Doc:     prodDocDTO(pd.Doc),
			AddedBy: prodUserDTO(pd.AddedBy),
			AddedAt: fmtTime(pd.AddedAt),
		})
	}
	return out
}

func prodPlaceholderDTOs(phs []production.Placeholder) []ProdPlaceholderDTO {
	out := make([]ProdPlaceholderDTO, 0, len(phs))
	for _, ph := range phs {
		out = append(out, ProdPlaceholderDTO{
			ID:       ph.ID,
			Label:    ph.Label,
			Kind:     ph.Kind,
			Required: ph.Required,
		})
	}
	return out
}

func prodStepDTO(st *production.Step) ProdStepDTO {
	return ProdStepDTO{
		ID:           st.ID,
		Num:          st.Num,
		Title:        st.Title,
		Description:  st.Description,
		X:            st.X,
		Y:            st.Y,
		PlanDocs:     prodPlanDocDTOs(st.PlanDocs),
		Placeholders: prodPlaceholderDTOs(st.Placeholders),
		CreatedAt:    fmtTime(st.CreatedAt),
		UpdatedAt:    fmtTime(st.UpdatedAt),
	}
}

func prodBatchDTO(b *production.Batch) ProdBatchDTO {
	out := ProdBatchDTO{
		ID:           b.ID,
		Num:          b.Num,
		Name:         b.Name,
		Kind:         b.Kind,
		RunAt:        fmtTime(b.RunAt),
		Status:       b.Status,
		Steps:        make([]ProdBatchStepDTO, 0, len(b.Steps)),
		Fulfillments: make([]ProdFulfillmentDTO, 0, len(b.Fulfillments)),
		Refs:         append([]string{}, b.Refs...),
		CreatedBy:    prodUserDTO(b.CreatedBy),
		CreatedAt:    fmtTime(b.CreatedAt),
		UpdatedAt:    fmtTime(b.UpdatedAt),
	}
	for _, bs := range b.Steps {
		out.Steps = append(out.Steps, ProdBatchStepDTO{
			StepID:       bs.StepID,
			Num:          bs.Num,
			Title:        bs.Title,
			PlanDocs:     prodPlanDocDTOs(bs.PlanDocs),
			Placeholders: prodPlaceholderDTOs(bs.Placeholders),
		})
	}
	for _, f := range b.Fulfillments {
		out.Fulfillments = append(out.Fulfillments, ProdFulfillmentDTO{
			ID:            f.ID,
			StepID:        f.StepID,
			PlaceholderID: f.PlaceholderID,
			Doc:           prodDocDTO(f.Doc),
			Source:        f.Source,
			IsAsRun:       f.IsAsRun,
			SuppliedBy:    prodUserDTO(f.SuppliedBy),
			SuppliedAt:    fmtTime(f.SuppliedAt),
		})
	}
	return out
}

func prodJobDTO(j production.Job, projectID, hubID, projectName string) ProdJobDTO {
	out := ProdJobDTO{
		ID:          j.ID,
		Num:         j.Num,
		ProjectID:   projectID,
		HubID:       hubID,
		ProjectName: projectName,
		Name:        j.Name,
		Description: j.Description,
		Steps:       make([]ProdStepDTO, 0, len(j.Steps)),
		Edges:       make([]ProdEdgeDTO, 0, len(j.Edges)),
		Batches:     make([]ProdBatchDTO, 0, len(j.Batches)),
		CreatedBy:   prodUserDTO(j.CreatedBy),
		CreatedAt:   fmtTime(j.CreatedAt),
		UpdatedAt:   fmtTime(j.UpdatedAt),
	}
	for _, st := range j.Steps {
		out.Steps = append(out.Steps, prodStepDTO(st))
	}
	for _, e := range j.Edges {
		out.Edges = append(out.Edges, ProdEdgeDTO{ID: e.ID, From: e.From, To: e.To})
	}
	for _, b := range j.Batches {
		out.Batches = append(out.Batches, prodBatchDTO(b))
	}
	return out
}
