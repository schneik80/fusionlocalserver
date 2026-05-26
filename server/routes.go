package server

import "net/http"

// routes builds the full HTTP handler: the /api/* JSON endpoints plus the
// static SPA catch-all, wrapped in the middleware chain. The Go 1.22 ServeMux
// resolves the most specific pattern, so the method-qualified /api routes win
// over the "/" catch-all, and "/api/" backstops any unmatched API path with a
// JSON 404 rather than letting it fall through to the SPA shell.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Meta.
	mux.HandleFunc("GET /api/meta", s.handleMeta)

	// Navigation.
	mux.HandleFunc("GET /api/hubs", s.handleHubs)
	mux.HandleFunc("GET /api/projects", s.handleProjects)
	mux.HandleFunc("GET /api/projects/contents", s.handleProjectContents)
	mux.HandleFunc("GET /api/folders/contents", s.handleFolderContents)
	mux.HandleFunc("GET /api/items/details", s.handleItemDetails)
	mux.HandleFunc("GET /api/items/location", s.handleItemLocation)

	// References.
	mux.HandleFunc("GET /api/items/uses", s.handleUses)
	mux.HandleFunc("GET /api/items/where-used", s.handleWhereUsed)
	mux.HandleFunc("GET /api/items/drawings", s.handleDrawings)
	mux.HandleFunc("GET /api/items/classify", s.handleClassify)
	mux.HandleFunc("GET /api/items/thumbnail", s.handleThumbnail)
	mux.HandleFunc("GET /api/items/thumbnail/image", s.handleThumbnailImage)
	mux.HandleFunc("GET /api/items/properties", s.handleProperties)

	// Settings.
	mux.HandleFunc("POST /api/settings/port", s.handleSetPort)

	// Pins.
	mux.HandleFunc("GET /api/pins", s.handlePinsList)
	mux.HandleFunc("POST /api/pins", s.handlePinsAdd)
	mux.HandleFunc("DELETE /api/pins", s.handlePinsRemove)

	// Stubs (501).
	mux.HandleFunc("POST /api/fusion/open", s.handleStub)
	mux.HandleFunc("POST /api/step/download", s.handleStub)

	// Unmatched API paths -> JSON 404 (kept off the SPA fallback).
	mux.HandleFunc("/api/", s.handleAPINotFound)

	// Static SPA for everything else.
	mux.Handle("/", s.staticHandler())

	// Middleware chain (outermost first): recover -> log -> dev CORS.
	return s.recoverPanic(s.logRequest(s.devCORS(mux)))
}
