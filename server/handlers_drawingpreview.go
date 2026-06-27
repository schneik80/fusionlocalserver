package server

import (
	"context"
	"net/http"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/schneik80/fusionlocalserver/api"
)

var (
	drawingPreviewCache = sync.Map{}        // itemId -> previewCacheEntry
	drawingPreviewFlight singleflight.Group // deduplicate concurrent requests
)

func (s *Server) handleDrawingPreview(w http.ResponseWriter, r *http.Request) {
	// The DM API resolves the file straight from the item lineage id + project,
	// so hubId (still sent by the frontend) is not needed here and is ignored.
	itemID, ok := reqParam(w, r, "itemId")
	if !ok {
		return
	}
	dmProjectID, ok := reqParam(w, r, "dmProjectId")
	if !ok {
		return
	}

	// Check cache first
	if cached, ok := drawingPreviewCache.Load(itemID); ok {
		entry := cached.(previewCacheEntry)
		w.Header().Set("Content-Type", entry.contentType)
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Write(entry.data)
		return
	}

	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}

	// Single-flight: deduplicate concurrent requests for the same itemId
	result, err, _ := drawingPreviewFlight.Do(itemID, func() (interface{}, error) {
		return s.fetchDrawingPreview(ctx, token, itemID, dmProjectID)
	})

	if err != nil {
		s.fail(w, r, err)
		return
	}

	entry := result.(previewCacheEntry)
	drawingPreviewCache.Store(itemID, entry)

	w.Header().Set("Content-Type", entry.contentType)
	w.Header().Set("Cache-Control", "max-age=86400")
	w.Write(entry.data)
}

type previewCacheEntry struct {
	data        []byte
	contentType string
}

func (s *Server) fetchDrawingPreview(ctx context.Context, token, itemID, dmProjectID string) (previewCacheEntry, error) {
	// Fusion drawings are composite docs (.f2d) with no downloadable storage —
	// the native file can't be fetched and unzipped. Instead, render the
	// version's preview through the Model Derivative API (its derivative already
	// exists), sized up to 400x400 for a crisper drawing preview.
	versionURN, err := api.GetItemTipVersion(ctx, token, dmProjectID, itemID)
	if err != nil {
		return previewCacheEntry{}, err
	}
	data, contentType, err := api.GetVersionThumbnail(ctx, token, versionURN, 400, 400)
	if err != nil {
		return previewCacheEntry{}, err
	}
	return previewCacheEntry{data: data, contentType: contentType}, nil
}
