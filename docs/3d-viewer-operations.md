# 3D / Parameters / Timeline — Operations (BUILD, RUN, TROUBLESHOOTING)

Operational guide for the "3D / Parameters / Timeline" feature: how to build it,
run it, and recover when it misbehaves. For the design of the pieces it ties
together, see the companion docs:

- **`docs/3d-viewer-backend.md`** — the download/decode/export pipeline (the
  MFGDM → DM → OSS chain, the model cache, the f3d-reader invocation).
- **`docs/3d-viewer-frontend.md`** — the React tabs (3D / Parameters / Timeline),
  `SceneViewer`, and the three.js render pipeline.

This document is intentionally narrow: the *operations* (BUILD / RUN /
TROUBLESHOOTING). It is written against the actual `Makefile`,
`scripts/bundle-reader.sh`, `server/reader.go`, and `server/server.go`.

---

## 1. The f3d-reader dependency (read this first)

FusionLocalServer does **not** import any f3d-reader Go package. The decoder
holds package-level wire state that is not concurrency-safe and pulls in a
non-stdlib zstd dependency that this binary deliberately keeps out. Instead,
**FusionLocalServer shells out to the `f3d-reader` CLI** — the binary built from
the sibling `fusion-next/f3d-reader` repo — and execs it as a subprocess
(see the package comment in `server/reader.go`).

### Where the binary must live

At runtime the server resolves the reader **relative to its own executable**.
`resolveReaderBin()` in `server/reader.go` tries, in order:

1. **`FLS_READER_BIN`** env override — an explicit path to an executable reader
   (dev / custom installs). If set but not executable, startup of the feature
   errors clearly rather than falling through.
2. **`<exeDir>/f3d-reader/bin/f3d-reader`** — the release-bundle layout. This is
   the canonical location and what `make bundle` produces. (`.exe` on Windows.)
3. **`<exeDir>/f3d-reader`** — a flat binary placed directly beside the server.
4. **`f3d-reader` on `$PATH`** — dev convenience when both repos are built.

If none resolve, the feature returns:

> `f3d-reader binary not found — bundle it next to the server (see `make bundle`) or set FLS_READER_BIN`

`<exeDir>` is the directory of the resolved (symlinks followed) server
executable — so for `./fusionlocalserver` it is the repo root, and the reader is
expected at `./f3d-reader/bin/f3d-reader`.

### What the server runs

- **Decode** (`f3d-reader <input>`): captures the reader's JSON to disk; the
  server lifts `synthesized.parameters` and `synthesized.timeline` into a compact
  `data.json` (this is what drives the Parameters and Timeline tabs).
- **GLB export**: `--assembly-glb <out> <input>` for a multi-design `.f3z`
  (composes root + each XREF'd component at its world transform), or
  `--export-glb <out> <input>` for a single `.f3d`. Both require the design to
  carry **cached OGS graphics** — a cloud file saved without graphics yields a
  sparse/empty GLB, though the data tabs still work.

---

## 2. BUILD & RUN workflow

### Make targets (from the `Makefile`)

| Target | What it does |
| --- | --- |
| `make web` | Builds the React/MUI UI into `server/webdist/` (`npm install && npm run build`). Build output; gitignored. |
| `make build` | `make web` **+** `go build -tags embed_ui` (embeds the UI, bakes the APS `client_id`/region via ldflags). Produces `./fusionlocalserver`. **Does NOT bundle the reader.** Requires `CLIENT_ID` (from `.aps-client-id` or `make build CLIENT_ID=...`). |
| `make install` | Same as `build` but `go install`s the binary. |
| `make bundle` | **`make build` + `scripts/bundle-reader.sh`** — the complete setup. Builds the server/UI *and* places the reader at `./f3d-reader/bin/f3d-reader`. **Use this for a working 3D feature.** |
| `make run` | `make build` then runs `./fusionlocalserver $(ARGS)`. (Note: `run` does **not** bundle — see the gotcha below.) |
| `make dev` | Plain `go build` (no `embed_ui`, no baked `client_id`) → stub UI. For HMR dev paired with `npm run dev`. |
| `make clean` | `rm -f fusionlocalserver` **and `rm -rf f3d-reader`** — removes the bundled reader directory. |
| `make check` | `go vet ./...` + `go test -race ./...`. |

### The gotcha that bit during development

- **`make build` rebuilds the server + embedded UI but does NOT bundle the
  reader.** A fresh `make build` (or `make run`, which calls `build`) leaves the
  3D feature without a reader unless one was already bundled, and the feature
  errors **"f3d-reader binary not found"**.
- **`make clean` deletes the `f3d-reader/` directory.** After a `clean`, the
  reader is gone and the next run errors with the same message until you re-bundle.
- **`make bundle` is the one that does both** (build + copy the reader). Prefer it
  whenever you want a complete, runnable setup.

### Recommended loops

- **Complete one-shot:**
  ```sh
  make bundle && ./fusionlocalserver -tls -public-url https://<host>:8080
  ```
- **Quick iteration:** `make build` for fast rebuilds, then re-run
  `./scripts/bundle-reader.sh` whenever the `f3d-reader/` directory is missing
  (after a `make clean`, a fresh checkout, etc.). The bundle script is cheap when
  the reader source is unchanged.

### How `scripts/bundle-reader.sh` finds the reader

The reader lands at `DEST_DIR/f3d-reader/bin/f3d-reader` (`DEST_DIR` defaults to
the repo root). Source resolution — **first hit wins**:

1. **`$F3D_READER_BIN`** — a prebuilt reader binary; copied verbatim (no build).
   Must be executable.
2. **`$F3D_READER_SRC`** — path to the f3d-reader source tree; built via
   `make -C "$SRC" cli`, then the produced `bin/f3d-reader` is copied.
3. **Conventional checkout locations** near this repo, tried in order:
   - `<repo>/../fusion-next/f3d-reader`
   - `<repo>/../../fusion-next/f3d-reader`
   - `$HOME/git/fusion-next/f3d-reader`
   - `$HOME/Dropbox/Transfer/jh-source/fusion-next/f3d-reader`

If nothing resolves, the script prints how to set `F3D_READER_SRC` /
`F3D_READER_BIN` and exits non-zero.

It also copies **`prism-textures.zip`** when it sits beside the source/prebuilt
binary (it supplies the bitmap textures the GLB exporter references). This is
**optional** — the exporter runs without it (untextured), so a miss is not fatal.

### Dev / HMR (no embedded UI)

```sh
make dev                              # go-only stub UI, no baked client_id
cd web && npm run dev                 # Vite HMR dev server
# run the server with dev creds + an explicit reader:
APS_CLIENT_ID=<your-client-id> FLS_READER_BIN=/path/to/built/f3d-reader \
  ./fusionlocalserver -dev
```

In dev, point `FLS_READER_BIN` at a built reader instead of bundling.

### HTTPS / serving

- Run with **`-tls`** (and **`-public-url https://<host>:8080`** so generated
  URLs are correct behind the public hostname).
- **There is no `-port` flag.** The port is taken from
  `~/.config/fusionlocalserver/server.json` (or the web UI's **Settings**
  dialog), defaulting to `8080`.

---

## 3. The HDR lighting asset

- The bundled environment map is **`web/public/environments/Generic.hdr`**
  (Radiance HDR, ~610 KB). It **is committed** to the repo.
- Vite copies everything under `web/public/` into the build output, which
  `make build`/`make web` embed into the binary (`-tags embed_ui`). At runtime it
  is served at **`/environments/Generic.hdr`**.
- The frontend loads it via `SceneViewer`'s `RGBELoader` from the constant
  `HDR_URL = '/environments/Generic.hdr'`, then uses the resulting texture as the
  scene's **environment image** (image-based lighting) for the three.js viewer.

### Swapping the HDR

1. Drop a replacement `.hdr` into `web/public/environments/` (keep the
   `Generic.hdr` name to avoid touching code, or update `HDR_URL` in
   `web/src/components/SceneViewer.tsx`).
2. Rebuild so Vite re-copies and the binary re-embeds it:
   `make build` (or `make bundle`).

A successful load is confirmed in the browser console by a harmless
`RGBELoader has been deprecated` warning (see Troubleshooting).

---

## 4. On-disk model cache

Decoded designs are cached under the config dir at
**`~/.config/fusionlocalserver/models/<hash>/`** (created by `server.go` as
`<configDir>/models`). Each per-design-version directory holds:

- **`data.json`** — the projected `{ parameters, timeline }` the data tabs consume.
- **`scene.glb`** — the exported binary glTF 2.0 the 3D tab renders.

`<hash>` is a **sha256 of the immutable design-version identity** (it is not
reversible to the source key). Entries carry a status (`PENDING` / `SUCCESS` /
`FAILED`), mirroring the thumbnail-cache pattern.

### Forcing a fresh download + decode

To wipe the cache (e.g. after debugging the pipeline):

```sh
rm -rf ~/.config/fusionlocalserver/models/*
```

The next time you open the 3D/Parameters/Timeline tab for a design, the server
re-runs the download → decode → export pipeline.

### FAILED entries auto-retry

A `FAILED` entry is **reclaimed to `PENDING` on the next request** — i.e. simply
re-opening the tab retries the job. You do not need to clear the cache to retry a
transient failure; clearing is only needed to discard a *successful* cached
result.

---

## 5. Frontend dependencies

The 3D viewer added three npm packages (all in `web/package.json`):

- **`three`** — the WebGL engine.
- **`postprocessing`** (pmndrs) — the `EffectComposer` / `EffectPass` pipeline
  (tone mapping, etc.).
- **`n8ao`** — the `N8AOPostPass` ambient-occlusion pass (ships no `.d.ts`; a
  minimal `web/src/n8ao.d.ts` declares it).

`node_modules` is **gitignored**; `npm install` (run automatically by
`make web`/`make build`/`make bundle`) recreates it.

---

## 6. TROUBLESHOOTING

| Symptom | Cause | Fix |
| --- | --- | --- |
| Server logs / feature returns **"reader: f3d-reader binary not found"** | No reader bundled at `<exeDir>/f3d-reader/bin/f3d-reader` (and none on `$PATH`). Usually after a `make build`/`make run` (which don't bundle) or a `make clean` (which deletes `f3d-reader/`). | Run **`make bundle`** (or just `./scripts/bundle-reader.sh`). For dev, set **`FLS_READER_BIN`** to a built reader. |
| 3D tab shows **"No 3D geometry"** | The design version was saved **without cached OGS graphics**, so the GLB export is empty/sparse. | Not a server bug. The **Parameters and Timeline tabs still work**. To get geometry, save/re-save the design from Fusion with graphics cached. |
| Model job **FAILED** with stage **`binary`** / **`resolve`** / **`download`** | The MFGDM → DM → OSS download chain couldn't fetch the native file. The `resolve`/`download` stages most often fail when the **DM project id (`dmProjectId`) is missing**. | Open the design **from within its project** so the nav carries the DM project id, then re-open the tab (FAILED auto-retries). See `docs/3d-viewer-backend.md` for the full chain. |
| **Thumbnail image 404** in the browser console | APS simply has no thumbnail for that component. | **Expected / harmless** — the `<img>` hides itself. No action. |
| Console warning **"RGBELoader has been deprecated"** | three's internal HDR loader prints this. | **Harmless** — it actually *confirms the HDR loaded*. No action. |
| **White screen** on the 3D tab | Previously a viewer crash (an SMAA-merge issue and an edge-overlay **infinite recursion**). | **Fixed.** The viewer no longer crashes; an **`ErrorBoundary`** (`web/src/components/ErrorBoundary.tsx`, wrapping the viewer in `DetailsPanel`) now shows the error **in-tab** instead of blanking the whole app. If you still hit a white screen, capture the boundary's message. |
| Stale geometry / parameters after a fix | A previous **`SUCCESS`** result is cached on disk. | `rm -rf ~/.config/fusionlocalserver/models/*` to force a fresh download + decode. |

---

## 7. Current status / open items

- **Backend chain verified end-to-end on a real CE hub.** The
  **MFGDM → DM → OSS** download chain is confirmed: a real `.f3d` downloads,
  decodes, and renders **parameters/timeline** from an actual design. (See
  `docs/3d-viewer-backend.md`.)
- **Viewer visual result to re-confirm.** After the recursion fix, the three.js
  viewer rewrite's **final visual output should be re-verified**. Remaining work
  is **cosmetic tuning**: AO intensity, edge weight, and the default brightness.
  (See `docs/3d-viewer-frontend.md`.)
- **Possible future work:**
  - **Lazy-load the viewer chunk** (the three/postprocessing/n8ao bundle is large).
  - **Multipart OSS download for very large objects.** Today
    `OSSSignedDownloadURL` **errors clearly on chunked objects** rather than
    guessing a single-URL download.
