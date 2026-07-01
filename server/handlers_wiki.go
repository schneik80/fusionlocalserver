package server

import (
	"net/http"

	"github.com/schneik80/fusionlocalserver/api"
)

// Wiki handlers serve the project-scoped markdown pages stored in a project-root
// "Wiki" folder. They take the GraphQL hub id (translated to the DM hub id here)
// and the project's data-management id (its altId), matching how the rest of the
// app addresses Data-Management resources (see handleDrawingPreview). Phase 1 is
// read-only; publishing is added in Phase 2.

// handleWikiPages lists a project's published wiki pages.
// GET /api/wiki/pages?hubId=<graphql hub id>&dmProjectId=<project altId>
func (s *Server) handleWikiPages(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
	dmProjectID, ok := reqParam(w, r, "dmProjectId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	dmHubID, err := api.GetHubDataManagementID(ctx, token, hubID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pages, err := api.ListWikiPages(ctx, token, dmHubID, dmProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, wikiPageDTOs(pages))
}

// handleWikiPage returns the markdown body of a single published page.
// GET /api/wiki/page?dmProjectId=<project altId>&itemId=<item lineage urn>
func (s *Server) handleWikiPage(w http.ResponseWriter, r *http.Request) {
	dmProjectID, ok := reqParam(w, r, "dmProjectId")
	if !ok {
		return
	}
	itemID, ok := reqParam(w, r, "itemId")
	if !ok {
		return
	}
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	md, err := api.DownloadWikiPage(ctx, token, dmProjectID, itemID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, WikiPageContentDTO{ItemID: itemID, Markdown: md})
}
