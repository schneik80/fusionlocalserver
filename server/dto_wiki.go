package server

import (
	"strings"

	"github.com/schneik80/fusionlocalserver/api"
)

// WikiPageDTO mirrors api.WikiPage — one published markdown page in a project's
// Wiki folder. title is the display name with its .md extension stripped, which
// is what the sidebar shows; name is the underlying file name.
type WikiPageDTO struct {
	ItemID     string `json:"itemId"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	TipVersion string `json:"tipVersion,omitempty"`
	ModifiedOn string `json:"modifiedOn,omitempty"`
	ModifiedBy string `json:"modifiedBy,omitempty"`
}

// WikiPageContentDTO is the markdown body of a single page (GET /api/wiki/page).
type WikiPageContentDTO struct {
	ItemID   string `json:"itemId"`
	Markdown string `json:"markdown"`
}

// wikiTitle drops a trailing .md (any case) so the sidebar shows a clean title.
func wikiTitle(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		return name[:len(name)-3]
	}
	return name
}

func wikiPageDTO(p api.WikiPage) WikiPageDTO {
	return WikiPageDTO{
		ItemID:     p.ItemID,
		Name:       p.Name,
		Title:      wikiTitle(p.Name),
		TipVersion: p.TipVersion,
		ModifiedOn: p.ModifiedOn,
		ModifiedBy: p.ModifiedBy,
	}
}

func wikiPageDTOs(ps []api.WikiPage) []WikiPageDTO {
	out := make([]WikiPageDTO, 0, len(ps))
	for _, p := range ps {
		out = append(out, wikiPageDTO(p))
	}
	return out
}
