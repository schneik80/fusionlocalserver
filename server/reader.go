package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// The f3d-reader CLI is the only contract with the decoder — fusionlocalserver
// never imports any f3d-reader Go package (the decoder holds package-level wire
// state that is not concurrency-safe, and pulls in a non-stdlib zstd dependency
// we keep out of this binary). We bundle the platform reader binary next to the
// fusionlocalserver executable and shell out, mirroring how f3d-viewer drives it.

// readerBinName is the platform executable name.
func readerBinName() string {
	if runtime.GOOS == "windows" {
		return "f3d-reader.exe"
	}
	return "f3d-reader"
}

var (
	readerBinOnce sync.Once
	readerBinPath string
	readerBinErr  error
)

// resolveReaderBin locates the bundled f3d-reader binary, caching the result.
// Resolution order:
//  1. FLS_READER_BIN env override (dev / custom installs).
//  2. <exeDir>/f3d-reader/bin/f3d-reader — the release-bundle layout (symmetric
//     with f3d-viewer, so the same bundle script serves both).
//  3. <exeDir>/f3d-reader — a flat binary placed directly beside the server.
//  4. f3d-reader on $PATH — the dev convenience when both repos are built.
func resolveReaderBin() (string, error) {
	readerBinOnce.Do(func() {
		name := readerBinName()
		if env := strings.TrimSpace(os.Getenv("FLS_READER_BIN")); env != "" {
			if isExecutable(env) {
				readerBinPath = env
				return
			}
			readerBinErr = fmt.Errorf("FLS_READER_BIN=%q is not an executable file", env)
			return
		}
		exe, err := os.Executable()
		if err == nil {
			if resolved, lerr := filepath.EvalSymlinks(exe); lerr == nil {
				exe = resolved
			}
			dir := filepath.Dir(exe)
			for _, cand := range []string{
				filepath.Join(dir, "f3d-reader", "bin", name),
				filepath.Join(dir, name),
			} {
				if isExecutable(cand) {
					readerBinPath = cand
					return
				}
			}
		}
		if p, lerr := exec.LookPath(name); lerr == nil {
			readerBinPath = p
			return
		}
		readerBinErr = fmt.Errorf(
			"f3d-reader binary not found — bundle it next to the server (see `make bundle`) or set FLS_READER_BIN")
	})
	return readerBinPath, readerBinErr
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return false
	}
	// On Windows the exec bit is meaningless; presence is enough.
	if runtime.GOOS == "windows" {
		return true
	}
	return st.Mode()&0o111 != 0
}

// decodeDesignJSON runs `f3d-reader <input>` and captures its Shape A JSON to
// jsonOutPath. The synthesized.parameters / synthesized.timeline we need are in
// the stdout document regardless of --full, so we skip --full (it would only
// also extract opaque ZIP parts to disk, which we don't use).
func decodeDesignJSON(ctx context.Context, bin, inputPath, jsonOutPath string) error {
	out, err := os.Create(jsonOutPath)
	if err != nil {
		return fmt.Errorf("create reader.json: %w", err)
	}
	defer out.Close()

	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, bin, inputPath)
	cmd.Stdout = out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("f3d-reader decode failed: %w: %s", err, lastLine(stderr.String()))
	}
	return nil
}

// exportGLB runs the reader's GLB exporter. For a multi-design .f3z it uses
// --assembly-glb (composes the root + each XREF'd component at its world
// transform); for a single .f3d it uses --export-glb. Both require the design to
// carry cached OGS graphics — a cloud file saved without graphics yields a
// sparse or empty GLB, but the data tabs still work.
func exportGLB(ctx context.Context, bin, inputPath, glbOutPath string, assembly bool) error {
	flag := "--export-glb"
	if assembly {
		flag = "--assembly-glb"
	}
	var stderr strings.Builder
	cmd := exec.CommandContext(ctx, bin, flag, glbOutPath, inputPath)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("f3d-reader %s failed: %w: %s", flag, err, lastLine(stderr.String()))
	}
	return nil
}

// isAssemblyArchive reports whether a design's native file is a multi-design
// bundle (.f3z) versus a single design (.f3d), by extension.
func isAssemblyArchive(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".f3z")
}

// modelData is the compact projection shipped to the browser: just the
// parameters map and the ordered timeline, lifted verbatim from the reader's
// synthesized envelope. Lifting them as raw JSON avoids re-encoding their
// (potentially large) contents and keeps the response far smaller than the full
// reader.json.
type modelData struct {
	Parameters json.RawMessage `json:"parameters"`
	Timeline   json.RawMessage `json:"timeline"`
}

// projectModelData reads the reader's JSON, extracts synthesized.parameters and
// synthesized.timeline, and writes the compact data.json the frontend consumes.
func projectModelData(jsonPath, dataOutPath string) error {
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("read reader.json: %w", err)
	}
	var doc struct {
		Synthesized struct {
			Parameters json.RawMessage `json:"parameters"`
			Timeline   json.RawMessage `json:"timeline"`
		} `json:"synthesized"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse reader.json: %w", err)
	}
	out := modelData{
		Parameters: doc.Synthesized.Parameters,
		Timeline:   doc.Synthesized.Timeline,
	}
	if out.Parameters == nil {
		out.Parameters = json.RawMessage("{}")
	}
	if out.Timeline == nil {
		out.Timeline = json.RawMessage("[]")
	}
	buf, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal data.json: %w", err)
	}
	if err := os.WriteFile(dataOutPath, buf, 0o644); err != nil {
		return fmt.Errorf("write data.json: %w", err)
	}
	return nil
}

// lastLine returns the last non-empty line of s, for surfacing the most
// relevant line of a subprocess's stderr in an error message.
func lastLine(s string) string {
	s = strings.TrimRight(s, "\n\r \t")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}
