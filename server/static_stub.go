//go:build !embed_ui

package server

import (
	"io/fs"
	"testing/fstest"
)

// stubIndexHTML is served when the binary is built without the embed_ui tag
// (plain `go build`, `make dev`, `go test`). It explains how to get the real
// UI. The dev workflow (`-server -dev`) bypasses this entirely by reverse-
// proxying to the Vite dev server.
const stubIndexHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>fusionlocalserver server</title>
  </head>
  <body>
    <main style="font-family: sans-serif; padding: 2rem;">
      <h1>fusionlocalserver server</h1>
      <p>
        This binary was built without the embedded web UI. Run
        <code>make build</code> (which runs <code>vite build</code> then
        <code>go build -tags embed_ui</code>) to ship the React/MUI frontend,
        or start the Vite dev server and run the binary with
        <code>-server -dev</code> for hot reload.
      </p>
    </main>
  </body>
</html>
`

// embeddedFS returns an in-memory FS holding only the stub shell, so
// embeddedHandler serves it for every route via its SPA fallback.
func embeddedFS() (fs.FS, error) {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(stubIndexHTML)},
	}, nil
}
