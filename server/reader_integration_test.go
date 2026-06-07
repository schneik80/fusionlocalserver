package server

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestReaderPipeline_E2E drives the real decode pipeline (decodeDesignJSON →
// projectModelData → exportGLB) against the bundled f3d-reader binary and a
// graphics-bearing fixture. It is opt-in: it skips unless both the reader
// binary and a fixture are reachable, so `make check` passes on machines
// without the reader bundled.
//
// To run it:
//
//	F3D_TEST_F3D=/path/to/fusion-next/f3d-reader/fixtures/ogs_cylinder2.f3d \
//	  go test ./server/ -run TestReaderPipeline_E2E -v
//
// It auto-discovers the reader at ../f3d-reader/bin/f3d-reader (the `make
// bundle` location) or honours FLS_READER_BIN.
func TestReaderPipeline_E2E(t *testing.T) {
	fixture := os.Getenv("F3D_TEST_F3D")
	if fixture == "" {
		t.Skip("set F3D_TEST_F3D to a graphics-bearing .f3d/.f3z to run the reader E2E")
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Skipf("fixture %q not found", fixture)
	}

	bin := os.Getenv("FLS_READER_BIN")
	if bin == "" {
		name := "f3d-reader"
		if runtime.GOOS == "windows" {
			name = "f3d-reader.exe"
		}
		bin = filepath.Join("..", "f3d-reader", "bin", name)
	}
	if _, err := exec.LookPath(bin); err != nil {
		if _, err2 := os.Stat(bin); err2 != nil {
			t.Skipf("reader binary not found at %q (run `make bundle`)", bin)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()

	// 1. Decode → reader.json.
	jsonPath := filepath.Join(dir, modelJSONFile)
	if err := decodeDesignJSON(ctx, bin, fixture, jsonPath); err != nil {
		t.Fatalf("decodeDesignJSON: %v", err)
	}

	// 2. Project → data.json with the parameters + timeline the UI consumes.
	dataPath := filepath.Join(dir, modelDataFile)
	if err := projectModelData(jsonPath, dataPath); err != nil {
		t.Fatalf("projectModelData: %v", err)
	}
	raw, _ := os.ReadFile(dataPath)
	var data struct {
		Parameters map[string]json.RawMessage `json:"parameters"`
		Timeline   []json.RawMessage          `json:"timeline"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("data.json not valid: %v", err)
	}
	t.Logf("projected %d parameters, %d timeline entries", len(data.Parameters), len(data.Timeline))

	// 3. Export GLB (the fixture carries OGS graphics, so this should succeed).
	glbPath := filepath.Join(dir, modelGLBFile)
	if err := exportGLB(ctx, bin, fixture, glbPath, isAssemblyArchive(fixture)); err != nil {
		t.Fatalf("exportGLB: %v", err)
	}
	head := make([]byte, 4)
	f, err := os.Open(glbPath)
	if err != nil {
		t.Fatalf("open glb: %v", err)
	}
	defer f.Close()
	if _, err := f.Read(head); err != nil {
		t.Fatalf("read glb header: %v", err)
	}
	if string(head) != "glTF" {
		t.Fatalf("GLB magic = %q, want \"glTF\"", head)
	}
}
