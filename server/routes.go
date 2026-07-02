package server

import "net/http"

// routes builds the full HTTP handler: the /api/* JSON endpoints plus the
// static SPA catch-all, wrapped in the middleware chain. The Go 1.22 ServeMux
// resolves the most specific pattern, so the method-qualified /api routes win
// over the "/" catch-all, and "/api/" backstops any unmatched API path with a
// JSON 404 rather than letting it fall through to the SPA shell.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// Public: server self-description, the auth flow, and the /api 404
	// backstop. These must be reachable before a user has a session.
	mux.HandleFunc("GET /api/meta", s.handleMeta)
	mux.HandleFunc("GET /api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("GET /api/auth/callback", s.handleAuthCallback)
	mux.HandleFunc("GET /api/auth/me", s.handleAuthMe)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/", s.handleAPINotFound)

	// Protected: every data route requires a logged-in session. prot wraps a
	// handler with requireAuth, which resolves the session's APS token into the
	// request context (or replies 401).
	prot := func(h http.HandlerFunc) http.HandlerFunc {
		return s.requireAuth(h).ServeHTTP
	}

	// Navigation.
	mux.HandleFunc("GET /api/hubs", prot(s.handleHubs))
	mux.HandleFunc("GET /api/projects", prot(s.handleProjects))
	mux.HandleFunc("GET /api/projects/contents", prot(s.handleProjectContents))
	mux.HandleFunc("GET /api/folders/contents", prot(s.handleFolderContents))
	mux.HandleFunc("GET /api/items/details", prot(s.handleItemDetails))
	mux.HandleFunc("GET /api/items/location", prot(s.handleItemLocation))
	// Raw bytes of an uploaded (non-native) file's tip, for the preview viewers.
	mux.HandleFunc("GET /api/items/file", prot(s.handleFile))

	// References.
	mux.HandleFunc("GET /api/items/uses", prot(s.handleUses))
	mux.HandleFunc("GET /api/items/descendants", prot(s.handleDescendants))
	mux.HandleFunc("GET /api/items/where-used", prot(s.handleWhereUsed))
	mux.HandleFunc("GET /api/items/drawings", prot(s.handleDrawings))
	mux.HandleFunc("GET /api/items/bom", prot(s.handleBOM))

	// Permissions (project groups + roles; group members need hub-admin access).
	mux.HandleFunc("GET /api/projects/groups", prot(s.handleProjectGroups))
	mux.HandleFunc("GET /api/permissions/path", prot(s.handlePermissionsPath))
	mux.HandleFunc("GET /api/groups/members", prot(s.handleGroupMembers))
	mux.HandleFunc("GET /api/items/classify", prot(s.handleClassify))
	mux.HandleFunc("GET /api/items/thumbnail", prot(s.handleThumbnail))
	mux.HandleFunc("GET /api/items/thumbnail/image", prot(s.handleThumbnailImage))
	mux.HandleFunc("GET /api/items/drawing/preview", prot(s.handleDrawingPreview))
	mux.HandleFunc("GET /api/items/properties", prot(s.handleProperties))
	mux.HandleFunc("GET /api/items/custom-properties", prot(s.handleCustomProperties))

	// Activity reports (per design; rollup merges in child documents).
	mux.HandleFunc("GET /api/activity/report", prot(s.handleActivityReport))
	mux.HandleFunc("POST /api/activity/rollup", prot(s.handleActivityRollup))

	// Settings.
	mux.HandleFunc("POST /api/settings/port", prot(s.handleSetPort))

	// Debug (only live when launched with -v; otherwise 404s). A live, real-doc
	// probe for discovering how a version exposes its root component version.
	mux.HandleFunc("GET /api/debug/version-probe", prot(s.handleDebugVersionProbe))

	// Pins.
	mux.HandleFunc("GET /api/pins", prot(s.handlePinsList))
	mux.HandleFunc("POST /api/pins", prot(s.handlePinsAdd))
	mux.HandleFunc("DELETE /api/pins", prot(s.handlePinsRemove))

	// Wiki (project-scoped markdown pages in a project-root "Wiki" folder).
	mux.HandleFunc("GET /api/wiki/pages", prot(s.handleWikiPages))
	mux.HandleFunc("GET /api/wiki/page", prot(s.handleWikiPage))
	mux.HandleFunc("POST /api/wiki/publish", prot(s.handleWikiPublish))
	mux.HandleFunc("POST /api/wiki/rename", prot(s.handleWikiRename))
	mux.HandleFunc("POST /api/wiki/image", prot(s.handleWikiImageUpload))
	mux.HandleFunc("GET /api/wiki/image", prot(s.handleWikiImage))

	// Static SPA for everything else.
	mux.Handle("/", s.staticHandler())

	// Middleware chain (outermost first): recover -> log -> security headers ->
	// canonical-host redirect -> dev CORS.
	return s.recoverPanic(s.logRequest(s.securityHeaders(s.canonicalRedirect(s.devCORS(mux)))))
}
