package server

import (
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

// SearchHitDTO mirrors api.SearchHit — one row in the search results.
type SearchHitDTO struct {
	Name         string `json:"name"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	Matched      string `json:"matched,omitempty"`
	ItemID       string `json:"itemId,omitempty"`
	HubID        string `json:"hubId,omitempty"`
	Kind         string `json:"kind"`
}

// SearchResponseDTO is the GET /api/search payload: a page of hits plus the
// cursor for the next page ("" when exhausted).
type SearchResponseDTO struct {
	Hits       []SearchHitDTO `json:"hits"`
	NextCursor string         `json:"nextCursor,omitempty"`
}

// SearchablePropertyDTO mirrors api.SearchableProperty — one option in the
// search form's property picker.
type SearchablePropertyDTO struct {
	DisplayName string `json:"displayName"`
	ID          string `json:"id"`
}

// handleSearch -> api.SearchByHub. Query params: hubId (required), and either
// q (free text) or propId+propValue (property search); cursor pages results.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	q := r.URL.Query()
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	hits, next, err := api.SearchByHub(ctx, token, hubID,
		q.Get("q"), q.Get("propId"), q.Get("propValue"), q.Get("cursor"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := SearchResponseDTO{Hits: make([]SearchHitDTO, 0, len(hits)), NextCursor: next}
	for _, h := range hits {
		out.Hits = append(out.Hits, SearchHitDTO{
			Name:         h.Name,
			ThumbnailURL: h.ThumbnailURL,
			Matched:      h.Matched,
			ItemID:       h.ItemID,
			HubID:        h.HubID,
			Kind:         h.Kind,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSearchableProperties -> api.GetSearchableProperties (query: hubId).
func (s *Server) handleSearchableProperties(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	props, err := api.GetSearchableProperties(ctx, token, hubID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]SearchablePropertyDTO, 0, len(props))
	for _, p := range props {
		out = append(out, SearchablePropertyDTO{DisplayName: p.DisplayName, ID: p.ID})
	}
	writeJSON(w, http.StatusOK, out)
}
