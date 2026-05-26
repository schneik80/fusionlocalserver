//go:build embed_ui

package server

import (
	"embed"
	"io/fs"
)

// webdistFS holds the built React/MUI SPA. The `all:` prefix embeds files whose
// names start with `_` or `.` (Vite emits hashed asset chunks under assets/ and
// the bundler may produce such names).
//
// This file is compiled only with `-tags embed_ui` (set by `make build` /
// `make install`), which run `vite build` into server/webdist first. Building
// with the tag but without a populated server/webdist/ is a compile error —
// run `make web` first.
//
//go:embed all:webdist
var webdistFS embed.FS

// embeddedFS returns the root of the built SPA.
func embeddedFS() (fs.FS, error) {
	return fs.Sub(webdistFS, "webdist")
}
