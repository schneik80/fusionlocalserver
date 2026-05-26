package server

import (
	"time"

	"github.com/schneik80/fusionlocalserver/api"
)

// The DTOs below mirror the api.* result structs but carry explicit camelCase
// json tags (the api structs have none). Times are rendered as RFC3339 strings
// — empty when zero — so the frontend never has to special-case Go's zero time.
// Slice fields are always emitted as [] (never null) so the React client can
// map over them unconditionally.

// MetaDTO is the server self-description returned by GET /api/meta.
type MetaDTO struct {
	Version string `json:"version"`
	Region  string `json:"region"`
	// Port is the currently bound listen port. PortConfigurable reports whether
	// it can be changed at runtime via POST /api/settings/port (false in dev
	// mode).
	Port             int  `json:"port"`
	PortConfigurable bool `json:"portConfigurable"`
}

// SetPortRequest is the POST /api/settings/port body.
type SetPortRequest struct {
	Port int `json:"port"`
}

// SetPortResponse acknowledges a port change. Restarting is always true on
// success — the listener rebinds, so the client must reconnect on the new port.
type SetPortResponse struct {
	Port       int  `json:"port"`
	Restarting bool `json:"restarting"`
}

// ItemDTO mirrors api.NavItem — a navigable node (hub/project/folder/design/…).
type ItemDTO struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Kind               string `json:"kind"`
	AltID              string `json:"altId,omitempty"`
	WebURL             string `json:"webUrl,omitempty"`
	IsContainer        bool   `json:"isContainer"`
	ComponentVersionID string `json:"componentVersionId,omitempty"`
	Subtype            string `json:"subtype,omitempty"`
}

// ContentsDTO is the combined folders+items payload for GET /api/projects/contents.
type ContentsDTO struct {
	Folders []ItemDTO `json:"folders"`
	Items   []ItemDTO `json:"items"`
}

// VersionDTO mirrors api.VersionSummary — one row of an item's version history.
type VersionDTO struct {
	Number    int    `json:"number"`
	CreatedOn string `json:"createdOn,omitempty"`
	CreatedBy string `json:"createdBy,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// DetailsDTO mirrors api.ItemDetails — the rich metadata for one item.
type DetailsDTO struct {
	ID                     string       `json:"id"`
	Name                   string       `json:"name"`
	Typename               string       `json:"typename"`
	Size                   string       `json:"size,omitempty"`
	MimeType               string       `json:"mimeType,omitempty"`
	ExtensionType          string       `json:"extensionType,omitempty"`
	FusionWebURL           string       `json:"fusionWebUrl,omitempty"`
	CreatedOn              string       `json:"createdOn,omitempty"`
	CreatedBy              string       `json:"createdBy,omitempty"`
	ModifiedOn             string       `json:"modifiedOn,omitempty"`
	ModifiedBy             string       `json:"modifiedBy,omitempty"`
	VersionNumber          int          `json:"versionNumber"`
	PartNumber             string       `json:"partNumber,omitempty"`
	PartDesc               string       `json:"partDesc,omitempty"`
	Material               string       `json:"material,omitempty"`
	IsMilestone            bool         `json:"isMilestone"`
	RootComponentVersionID string       `json:"rootComponentVersionId,omitempty"`
	Versions               []VersionDTO `json:"versions"`
}

// ComponentRefDTO mirrors api.ComponentRef — a row in the Uses / Where Used tab.
type ComponentRefDTO struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	PartNumber     string `json:"partNumber,omitempty"`
	PartDesc       string `json:"partDesc,omitempty"`
	Material       string `json:"material,omitempty"`
	DesignItemID   string `json:"designItemId,omitempty"`
	DesignItemName string `json:"designItemName,omitempty"`
	FusionWebURL   string `json:"fusionWebUrl,omitempty"`
}

// DrawingRefDTO mirrors api.DrawingRef — a row in the Drawings tab.
type DrawingRefDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DrawingItemID string `json:"drawingItemId"`
	ModifiedOn    string `json:"modifiedOn,omitempty"`
	ModifiedBy    string `json:"modifiedBy,omitempty"`
	FusionWebURL  string `json:"fusionWebUrl,omitempty"`
}

// FolderRefDTO mirrors api.FolderRef — one hop in a folder ancestry chain.
type FolderRefDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LocationDTO mirrors api.ItemLocation — where an item lives, for Show-in-Location.
type LocationDTO struct {
	HubID        string         `json:"hubId"`
	ProjectID    string         `json:"projectId"`
	ProjectAltID string         `json:"projectAltId,omitempty"`
	ProjectName  string         `json:"projectName"`
	FolderPath   []FolderRefDTO `json:"folderPath"`
}

// ClassifyDTO is the GET /api/items/classify result for one design row.
type ClassifyDTO struct {
	ComponentVersionID string `json:"componentVersionId"`
	IsAssembly         bool   `json:"isAssembly"`
	Subtype            string `json:"subtype"` // "assembly" | "part"
}

// ThumbnailDTO is the GET /api/items/thumbnail result. Generation is async, so
// status is "PENDING" | "SUCCESS" | "FAILED"; signedUrl is populated only once
// status is SUCCESS. The frontend polls while status is PENDING.
type ThumbnailDTO struct {
	Status    string `json:"status"`
	SignedURL string `json:"signedUrl,omitempty"`
}

// MeasureDTO is one physical-property value (display string + unit).
type MeasureDTO struct {
	Display string `json:"display,omitempty"`
	Units   string `json:"units,omitempty"`
}

// PhysicalPropertiesDTO mirrors api.PhysicalProperties — the GET
// /api/items/properties result. Status is "COMPLETED" | "FAILED" | (computing).
type PhysicalPropertiesDTO struct {
	Status     string     `json:"status"`
	Area       MeasureDTO `json:"area"`
	Volume     MeasureDTO `json:"volume"`
	Mass       MeasureDTO `json:"mass"`
	Density    MeasureDTO `json:"density"`
	BBoxLength MeasureDTO `json:"bboxLength"`
	BBoxWidth  MeasureDTO `json:"bboxWidth"`
	BBoxHeight MeasureDTO `json:"bboxHeight"`
}

// ---------------------------------------------------------------------------
// Mappers
// ---------------------------------------------------------------------------

func measureDTO(m api.Measure) MeasureDTO {
	return MeasureDTO{Display: m.Display, Units: m.Units}
}

func physicalPropertiesDTO(p *api.PhysicalProperties) PhysicalPropertiesDTO {
	return PhysicalPropertiesDTO{
		Status:     p.Status,
		Area:       measureDTO(p.Area),
		Volume:     measureDTO(p.Volume),
		Mass:       measureDTO(p.Mass),
		Density:    measureDTO(p.Density),
		BBoxLength: measureDTO(p.BBoxLength),
		BBoxWidth:  measureDTO(p.BBoxWidth),
		BBoxHeight: measureDTO(p.BBoxHeight),
	}
}

// fmtTime renders a timestamp as RFC3339, or "" when zero.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func itemDTO(n api.NavItem) ItemDTO {
	return ItemDTO{
		ID:                 n.ID,
		Name:               n.Name,
		Kind:               n.Kind,
		AltID:              n.AltID,
		WebURL:             n.WebURL,
		IsContainer:        n.IsContainer,
		ComponentVersionID: n.ComponentVersionID,
		Subtype:            n.Subtype,
	}
}

func itemDTOs(ns []api.NavItem) []ItemDTO {
	out := make([]ItemDTO, 0, len(ns))
	for _, n := range ns {
		out = append(out, itemDTO(n))
	}
	return out
}

func detailsDTO(d *api.ItemDetails) DetailsDTO {
	dto := DetailsDTO{
		ID:                     d.ID,
		Name:                   d.Name,
		Typename:               d.Typename,
		Size:                   d.Size,
		MimeType:               d.MimeType,
		ExtensionType:          d.ExtensionType,
		FusionWebURL:           d.FusionWebURL,
		CreatedOn:              fmtTime(d.CreatedOn),
		CreatedBy:              d.CreatedBy,
		ModifiedOn:             fmtTime(d.ModifiedOn),
		ModifiedBy:             d.ModifiedBy,
		VersionNumber:          d.VersionNumber,
		PartNumber:             d.PartNumber,
		PartDesc:               d.PartDesc,
		Material:               d.Material,
		IsMilestone:            d.IsMilestone,
		RootComponentVersionID: d.RootComponentVersionID,
		Versions:               make([]VersionDTO, 0, len(d.Versions)),
	}
	for _, v := range d.Versions {
		dto.Versions = append(dto.Versions, VersionDTO{
			Number:    v.Number,
			CreatedOn: fmtTime(v.CreatedOn),
			CreatedBy: v.CreatedBy,
			Comment:   v.Comment,
		})
	}
	return dto
}

func componentRefDTOs(refs []api.ComponentRef) []ComponentRefDTO {
	out := make([]ComponentRefDTO, 0, len(refs))
	for _, r := range refs {
		out = append(out, ComponentRefDTO{
			ID:             r.ID,
			Name:           r.Name,
			PartNumber:     r.PartNumber,
			PartDesc:       r.PartDesc,
			Material:       r.Material,
			DesignItemID:   r.DesignItemID,
			DesignItemName: r.DesignItemName,
			FusionWebURL:   r.FusionWebURL,
		})
	}
	return out
}

func drawingRefDTOs(refs []api.DrawingRef) []DrawingRefDTO {
	out := make([]DrawingRefDTO, 0, len(refs))
	for _, r := range refs {
		out = append(out, DrawingRefDTO{
			ID:            r.ID,
			Name:          r.Name,
			DrawingItemID: r.DrawingItemID,
			ModifiedOn:    fmtTime(r.ModifiedOn),
			ModifiedBy:    r.ModifiedBy,
			FusionWebURL:  r.FusionWebURL,
		})
	}
	return out
}

func locationDTO(loc *api.ItemLocation) LocationDTO {
	dto := LocationDTO{
		HubID:        loc.HubID,
		ProjectID:    loc.ProjectID,
		ProjectAltID: loc.ProjectAltID,
		ProjectName:  loc.ProjectName,
		FolderPath:   make([]FolderRefDTO, 0, len(loc.FolderPath)),
	}
	for _, f := range loc.FolderPath {
		dto.FolderPath = append(dto.FolderPath, FolderRefDTO{ID: f.ID, Name: f.Name})
	}
	return dto
}
