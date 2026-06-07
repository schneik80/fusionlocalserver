package server

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schneik80/fusionlocalserver/api"
)

// modelDecodeConcurrency caps how many reader subprocesses run at once. Each
// decode of a large assembly can use a lot of memory, so this stays small.
const modelDecodeConcurrency = 2

// modelCacheMaxBytes caps the on-disk decoded-model cache. Entries are the
// compact data.json plus the exported GLB; least-recently-used entries are
// evicted once the total exceeds this.
const modelCacheMaxBytes = 4 << 30 // 4 GiB

// modelJobTimeout bounds one full download+decode+export job. Large assemblies
// can take minutes to decode, so this is generous; it is a backstop against a
// wedged subprocess, not a latency target. The job runs on a background context
// (not the polling request's) so it survives across the frontend's status polls.
const modelJobTimeout = 8 * time.Minute

// ModelStatusDTO is the response of GET /api/items/model. It mirrors the
// thumbnail status shape so the frontend can reuse the poll-with-timeout
// pattern. HasGLB tells the client whether a 3D view is available (a design
// saved without cached graphics decodes fine but exports an empty/absent GLB).
type ModelStatusDTO struct {
	Status string `json:"status"` // PENDING | SUCCESS | FAILED
	Error  string `json:"error,omitempty"`
	HasGLB bool   `json:"hasGlb,omitempty"`
}

// modelCacheKey derives the disk-cache key for a request. The key is the
// immutable version identity: hub + item, plus an optional caller-supplied
// version token (the item's tip timestamp / lastModifiedOn) so a re-saved
// design decodes fresh instead of serving a stale cache entry.
func modelCacheKey(r *http.Request) (key, hubID, itemID string, ok bool) {
	q := r.URL.Query()
	hubID = q.Get("hubId")
	itemID = q.Get("itemId")
	if hubID == "" || itemID == "" {
		return "", "", "", false
	}
	ver := q.Get("ver")
	return hubID + "\n" + itemID + "\n" + ver, hubID, itemID, true
}

// handleModelStatus reports (and, on first call, kicks off) the decode of a
// design's native file into a cached GLB + parameters/timeline. It returns
// immediately: the first caller for a key launches a background job and gets
// PENDING; subsequent polls observe PENDING/SUCCESS/FAILED.
func (s *Server) handleModelStatus(w http.ResponseWriter, r *http.Request) {
	if s.models == nil {
		writeError(w, http.StatusServiceUnavailable, "model cache unavailable")
		return
	}
	key, hubID, itemID, ok := modelCacheKey(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "missing required query parameters: hubId, itemId")
		return
	}

	if e := s.models.snapshot(key); e != nil {
		switch e.status {
		case modelSuccess:
			s.models.touch(key)
			writeJSON(w, http.StatusOK, ModelStatusDTO{
				Status: string(modelSuccess),
				HasGLB: fileExists(filepath.Join(e.dir, modelGLBFile)),
			})
		case modelFailed:
			writeJSON(w, http.StatusOK, ModelStatusDTO{Status: string(modelFailed), Error: e.errMsg})
		default:
			writeJSON(w, http.StatusOK, ModelStatusDTO{Status: string(modelPending)})
		}
		return
	}

	// No entry yet — capture the caller's token, claim the key, and launch the
	// job. begin() guarantees exactly one starter even under concurrent polls.
	ctx, cancel := s.reqCtx(r)
	defer cancel()
	token, ok := s.token(ctx, w, r)
	if !ok {
		return
	}
	// The Data Management project id (the project's altId) is needed to fetch
	// the native file; the frontend passes it from nav context since the item's
	// own `project` field can't be resolved on these hubs.
	dmProjectID := r.URL.Query().Get("dmProjectId")
	started, _ := s.models.begin(key, "")
	if started {
		go s.runModelJob(key, hubID, itemID, dmProjectID, token)
	}
	writeJSON(w, http.StatusOK, ModelStatusDTO{Status: string(modelPending)})
}

// runModelJob performs the full pipeline for one design: resolve the native
// file's signed URL, download it, decode it to JSON, project the
// parameters/timeline, and export a GLB. It runs on a background context with a
// generous timeout so it outlives the polling requests. The large intermediates
// (the downloaded archive and reader.json) are removed on success, leaving only
// the two served artifacts: data.json and scene.glb.
func (s *Server) runModelJob(key, hubID, itemID, dmProjectID, token string) {
	// Bound concurrent decodes (each spawns a memory-hungry reader subprocess).
	s.modelSem <- struct{}{}
	defer func() { <-s.modelSem }()

	ctx, cancel := context.WithTimeout(context.Background(), modelJobTimeout)
	defer cancel()

	fail := func(stage string, err error) {
		s.logger.Error("model job failed", "stage", stage, "itemId", itemID, "err", err)
		s.models.markFailed(key, stage+": "+err.Error())
	}

	bin, err := resolveReaderBin()
	if err != nil {
		fail("reader", err)
		return
	}
	if dmProjectID == "" {
		fail("binary", &stageError{"no Data Management project id (open the design from within its project)"})
		return
	}

	bsnap := s.models.snapshot(key)
	if bsnap == nil {
		return // evicted/reclaimed mid-flight; nothing to do
	}
	dir := bsnap.dir

	// 1. MFGDM gives the native file's version URN; Data Management turns that
	// into a signed S3 download URL (MFGDM exposes no download URL itself).
	db, err := api.GetDesignBinary(ctx, token, hubID, itemID)
	if err != nil {
		fail("binary", err)
		return
	}
	signedURL, fileName, err := api.ResolveDesignDownloadURL(ctx, token, dmProjectID, db.VersionURN)
	if err != nil {
		fail("resolve", err)
		return
	}

	// 2. Download to the entry directory, keeping the native extension so the
	// reader picks the right GLB exporter (.f3z → assembly, .f3d → single).
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".f3d" && ext != ".f3z" {
		ext = ".f3z" // safest default: try the assembly path
	}
	designPath := filepath.Join(dir, "design"+ext)
	if _, err := api.DownloadFileToPath(ctx, signedURL, designPath); err != nil {
		fail("download", err)
		return
	}

	// 3. Decode → reader.json, then project the compact data.json.
	jsonPath := filepath.Join(dir, modelJSONFile)
	if err := decodeDesignJSON(ctx, bin, designPath, jsonPath); err != nil {
		fail("decode", err)
		return
	}
	if err := projectModelData(jsonPath, filepath.Join(dir, modelDataFile)); err != nil {
		fail("project", err)
		return
	}

	// 4. Export the GLB. Non-fatal: a design without cached OGS graphics
	// produces no usable mesh, but the parameters/timeline are still valid, so
	// we still mark the job SUCCESS — the 3D view simply reports "no geometry".
	glbPath := filepath.Join(dir, modelGLBFile)
	assembly := ext == ".f3z"
	if err := exportGLB(ctx, bin, designPath, glbPath, assembly); err != nil {
		if assembly {
			// A .f3z that isn't a multi-design bundle (or lacks member
			// graphics) — retry as a single-design export before giving up.
			if err2 := exportGLB(ctx, bin, designPath, glbPath, false); err2 != nil {
				s.logger.Warn("model GLB export failed", "itemId", itemID, "assembly_err", err, "single_err", err2)
				os.Remove(glbPath)
			}
		} else {
			s.logger.Warn("model GLB export failed", "itemId", itemID, "err", err)
			os.Remove(glbPath)
		}
	}

	// 5. Drop the large intermediates; keep only the served artifacts.
	os.Remove(designPath)
	os.Remove(jsonPath)

	s.models.markSuccess(key)
}

// handleModelData streams the compact {parameters, timeline} JSON for a decoded
// design. The frontend only calls this after status is SUCCESS.
func (s *Server) handleModelData(w http.ResponseWriter, r *http.Request) {
	if s.models == nil {
		writeError(w, http.StatusServiceUnavailable, "model cache unavailable")
		return
	}
	key, _, _, ok := modelCacheKey(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "missing required query parameters: hubId, itemId")
		return
	}
	e := s.models.snapshot(key)
	if e == nil || e.status != modelSuccess {
		writeError(w, http.StatusNotFound, "model data not ready")
		return
	}
	path := filepath.Join(e.dir, modelDataFile)
	if !fileExists(path) {
		writeError(w, http.StatusNotFound, "model data not ready")
		return
	}
	s.models.touch(key)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, path)
}

// handleModelGLB streams the exported binary glTF for a decoded design.
// ServeFile gives us conditional requests and Range support for free, so the
// GLB never has to be buffered in the server's memory.
func (s *Server) handleModelGLB(w http.ResponseWriter, r *http.Request) {
	if s.models == nil {
		writeError(w, http.StatusServiceUnavailable, "model cache unavailable")
		return
	}
	key, _, _, ok := modelCacheKey(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "missing required query parameters: hubId, itemId")
		return
	}
	e := s.models.snapshot(key)
	if e == nil || e.status != modelSuccess {
		writeError(w, http.StatusNotFound, "model not ready")
		return
	}
	path := filepath.Join(e.dir, modelGLBFile)
	if !fileExists(path) {
		writeError(w, http.StatusNotFound, "no geometry for this model")
		return
	}
	s.models.touch(key)
	w.Header().Set("Content-Type", "model/gltf-binary")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, path)
}

// stageError is a tiny error type for job-stage failures that aren't wrapping an
// existing error.
type stageError struct{ msg string }

func (e *stageError) Error() string { return e.msg }
