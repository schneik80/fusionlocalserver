package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/schneik80/fusionlocalserver/api"
)

var (
	drawingPreviewCache = sync.Map{}        // itemId -> previewCacheEntry
	drawingPreviewFlight singleflight.Group // deduplicate concurrent requests
)

func (s *Server) handleDrawingPreview(w http.ResponseWriter, r *http.Request) {
	hubID, ok := reqParam(w, r, "hubId")
	if !ok {
		return
	}
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
		return s.fetchDrawingPreview(ctx, token, hubID, itemID, dmProjectID)
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

func (s *Server) fetchDrawingPreview(ctx context.Context, token, hubID, itemID, dmProjectID string) (previewCacheEntry, error) {
	// Resolve version URN
	bin, err := api.GetDesignBinary(ctx, token, hubID, itemID)
	if err != nil {
		return previewCacheEntry{}, err
	}
	if bin == nil || bin.VersionURN == "" {
		return previewCacheEntry{}, fmt.Errorf("item has no native file")
	}

	// Resolve to signed S3 URL
	signedURL, _, err := api.ResolveDesignDownloadURL(ctx, token, dmProjectID, bin.VersionURN)
	if err != nil {
		return previewCacheEntry{}, err
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp(os.TempDir(), "fls-drawing-*.f2d")
	if err != nil {
		return previewCacheEntry{}, err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	_, err = api.DownloadFileToPath(ctx, signedURL, tmpPath)
	if err != nil {
		return previewCacheEntry{}, err
	}

	// Extract preview image
	data, contentType, err := api.ExtractDrawingPreview(tmpPath)
	if err != nil {
		return previewCacheEntry{}, err
	}

	return previewCacheEntry{data: data, contentType: contentType}, nil
}
