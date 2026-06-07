package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectModelData(t *testing.T) {
	dir := t.TempDir()
	readerJSON := filepath.Join(dir, "reader.json")
	// A minimal Shape A document with a parameters map, a timeline array, and
	// unrelated sections that must be dropped from the projection.
	doc := `{
		"source": {"filename": "x.f3z"},
		"synthesized": {
			"parameters": {"u1": {"userName": "d1", "name": "Width", "expression": "40 mm", "value": 4.0, "unit": "mm"}},
			"timeline": [{"index": 0, "type": "SketchFeature", "name": "Sketch1"}],
			"entities": {"big": "drop me"},
			"scene": {"drop": true}
		}
	}`
	if err := os.WriteFile(readerJSON, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, "data.json")
	if err := projectModelData(readerJSON, dataPath); err != nil {
		t.Fatalf("projectModelData: %v", err)
	}

	var got struct {
		Parameters map[string]any `json:"parameters"`
		Timeline   []any          `json:"timeline"`
		Entities   any            `json:"entities"`
	}
	raw, _ := os.ReadFile(dataPath)
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal projected data: %v", err)
	}
	if len(got.Parameters) != 1 {
		t.Errorf("parameters: got %d want 1", len(got.Parameters))
	}
	if len(got.Timeline) != 1 {
		t.Errorf("timeline: got %d want 1", len(got.Timeline))
	}
	if got.Entities != nil {
		t.Errorf("entities should not be projected, got %v", got.Entities)
	}
}

func TestProjectModelData_MissingSections(t *testing.T) {
	dir := t.TempDir()
	readerJSON := filepath.Join(dir, "reader.json")
	if err := os.WriteFile(readerJSON, []byte(`{"synthesized":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(dir, "data.json")
	if err := projectModelData(readerJSON, dataPath); err != nil {
		t.Fatalf("projectModelData: %v", err)
	}
	raw, _ := os.ReadFile(dataPath)
	// Absent sections become empty containers, never null, so the frontend can
	// render without null guards.
	if string(raw) != `{"parameters":{},"timeline":[]}` {
		t.Errorf("got %s", raw)
	}
}

func TestIsAssemblyArchive(t *testing.T) {
	if !isAssemblyArchive("Widget.f3z") || !isAssemblyArchive("a.F3Z") {
		t.Error(".f3z should be an assembly archive")
	}
	if isAssemblyArchive("Part.f3d") {
		t.Error(".f3d is not an assembly archive")
	}
}
