package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleThumbnailImage_ServesCachedBytes(t *testing.T) {
	s := &Server{logger: quietLogger(), thumbs: newThumbCache(8, time.Minute)}
	s.thumbs.putImage("cv-1", []byte("PNGDATA"), "image/png")

	req := httptest.NewRequest(http.MethodGet, "/api/items/thumbnail/image?cvId=cv-1", nil)
	rec := httptest.NewRecorder()
	s.handleThumbnailImage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if rec.Body.String() != "PNGDATA" {
		t.Errorf("body = %q, want PNGDATA", rec.Body.String())
	}
}

func TestHandleThumbnailImage_MissingCvId(t *testing.T) {
	s := &Server{logger: quietLogger(), thumbs: newThumbCache(8, time.Minute)}
	req := httptest.NewRequest(http.MethodGet, "/api/items/thumbnail/image", nil)
	rec := httptest.NewRecorder()
	s.handleThumbnailImage(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
