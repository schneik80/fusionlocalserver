# 3D / Parameters / Timeline — Server Backend

This documents the **server side** of the "3D / Parameters / Timeline" feature.
When a user opens that tab for a design, the server lazily downloads the design's
native `.f3d`/`.f3z` file, decodes it with the bundled `f3d-reader` CLI, caches
the result on disk, and serves a GLB mesh plus a projected parameters/timeline
JSON document.

The flow deliberately mirrors the existing **thumbnail** subsystem: a status
endpoint that returns `PENDING`/`SUCCESS`/`FAILED`, a background job kicked off on
the first poll, and an on-disk cache keyed by an immutable version identity.

## Purpose

The native design file is the source of truth for both geometry and the
parameter/timeline metadata. MFGDM (the v3 GraphQL API used by the rest of the
app) does **not** expose any of that directly — it only hands back a *version
URN* pointing at the file in Data Management. So the server:

1. Resolves that version URN (MFGDM GraphQL).
2. Turns it into a presigned S3 download URL (Data Management + OSS).
3. Streams the native file to a per-design cache directory.
4. Runs the `f3d-reader` CLI to (a) extract `synthesized.parameters` /
   `synthesized.timeline` and (b) export a binary glTF (`.glb`).
5. Keeps only the two served artifacts (`data.json`, `scene.glb`) and serves
   them on demand.

All of this happens once per design version; subsequent opens are pure cache
hits.

---

## Data flow (sequence)

```
Frontend (3D tab opens)
   │  GET /api/items/model?hubId&itemId&ver&dmProjectId
   ▼
handleModelStatus  ──first call──►  modelCache.begin(key)  ──► go runModelJob(...)
   │  (returns PENDING immediately)                              │
   │                                                             ▼
   │   runModelJob (background ctx, bounded by modelSem):
   │     1. resolveReaderBin()                  locate f3d-reader CLI
   │     2. api.GetDesignBinary()               MFGDM: item → binary.id (version URN)
   │     3. api.ResolveDesignDownloadURL()      DM version → OSS object → signed S3 URL
   │     4. api.DownloadFileToPath()            stream native file → <dir>/design.f3z
   │     5. decodeDesignJSON()                  f3d-reader <input>  → reader.json
   │     6. projectModelData()                  lift synthesized.{parameters,timeline} → data.json
   │     7. exportGLB()                         f3d-reader --assembly-glb/--export-glb → scene.glb
   │     (delete design.* and reader.json; keep data.json + scene.glb)
   │     modelCache.markSuccess(key)
   │
   ├─ Frontend polls GET /api/items/model  ──► PENDING … then SUCCESS (hasGlb)
   ├─ GET /api/items/model/data            ──► data.json  ({parameters, timeline})
   └─ GET /api/items/model/glb             ──► scene.glb   (model/gltf-binary, Range-capable)
```

---

## The download chain (the hard part)

MFGDM exposes the native file **only** as `DesignItem.binary { id }`. The
`Binary` GraphQL type has nothing but an `id` field — there is **no download
URL**. That `id` is a Data Management *version* URN, for example:

```
urn:adsk.wipprod:fs.file:vf.<lineage>?version=5
```

`GetDesignBinary` (`api/binary.go`) issues the GraphQL query and returns a
`DesignBinary{ VersionURN }`. It selects `binary { id }` across `DesignItem`,
`ConfiguredDesignItem`, and `DrawingItem` inline fragments.

### Critical caveat: do NOT select `project` on the item

You must **not** select `project` on the item in the same (or any) query used
here. On these hubs the `DesignItem.project` resolver errors with:

> Invalid Project ID. Ensure it starts with `urn:adsk:workspace`… use
> `projectByDataManagementAPIId`.

That same resolver behavior is why `binary` was hard to retrieve at first — any
query that pulled `project` alongside it failed. The doc comment on
`GetDesignBinary` calls this out explicitly.

Because the item cannot give us a usable Data Management project id, the project
id needed for the download is taken from the **frontend's navigation context**
instead: `nav.project.altId` (the project's `dataManagementAPIProjectId`, the
`b.…` id). The frontend passes it to the status endpoint as the `dmProjectId`
query parameter. It is **never** resolved from the item.

### Data Management download chain (`api/datamanagement.go`)

Once we have a version URN and the `dmProjectId`, `ResolveDesignDownloadURL`
ties the chain together:

1. **`GetVersionDownload(ctx, token, dmProjectID, versionURN)`**
   `GET data/v1/projects/{dmProjectId}/versions/{url-encoded version URN}`.
   Reads `data.relationships.storage.data.id` — an OSS object URN of the form
   `urn:adsk.objects:os.object:{bucket}/{object}` — plus the filename
   (`attributes.name`, falling back to `displayName`). Returns a
   `VersionDownload{ StorageURN, FileName }`. If there is no storage object the
   version has no downloadable native file and this errors.

2. **`OSSSignedDownloadURL(ctx, token, storageURN)`**
   Splits the OSS URN into `{bucket}/{object}`, then
   `GET oss/v2/buckets/{bucket}/objects/{key}/signeds3download` → a presigned S3
   URL (the `url` field). If the response is multipart (`urls[]`, chunked large
   object) it returns a clear "multipart not yet supported" error rather than
   downloading a single chunk.

3. **`DownloadFileToPath(ctx, url, destPath)`** (`api/download.go`)
   Fetches the presigned URL with **no bearer token** — APS signed URLs are
   self-authenticated and attaching the token would leak it to the URL's host.
   Streams to a `.download-*.part` temp file in the destination directory and
   atomically `rename`s it into place on success, so a cancelled/failed download
   never leaves a partial file that a later run would mistake for a complete
   cache entry. Bounded by `MaxDesignBytes` (2 GiB) and cancellable via context.

`ResolveDesignDownloadURL` returns `(signedURL, fileName)`; the filename carries
the real `.f3d`/`.f3z` extension, which the job uses to pick the GLB exporter.

### URN escaping

`dmEscape` (= `url.QueryEscape`) percent-encodes URNs for use as a single path
segment, matching JavaScript's `encodeURIComponent` (which the APS examples use).
`url.PathEscape` is wrong here because it leaves `:` unescaped, and version URNs
contain `:`, `?`, and `=`.

### Base URL

`dmBaseURL` (`https://developer.api.autodesk.com`) is a `var`, not a `const`,
solely so tests can point `dmGet` at an `httptest` server; production never
reassigns it. `dmGet` is the small authenticated GET helper (sets
`Authorization: Bearer`, fails on non-2xx, limits the error body).

---

## On-disk cache (`server/modelcache.go`)

`modelCache` is an on-disk, byte-capped, concurrency-safe cache. Unlike the
in-memory thumbnail cache it stores large artifacts on disk; the in-memory map
is just the index + job state.

### Key and layout

The cache key is the **immutable version identity**:

```
hubId + "\n" + itemId + "\n" + ver
```

where `ver` is the item's tip timestamp (`lastModifiedOn` / `tipTimestamp`),
supplied by the frontend as the `ver` query param. A re-saved design therefore
gets a new key and decodes fresh instead of serving stale data
(`modelCacheKey` in `server/handlers_model.go`).

`keyHash` is the `sha256` of that key, hex-encoded, used as the directory name
under the cache root:

```
~/.config/fusionlocalserver/models/<sha256-hash>/
    data.json    # projected {parameters, timeline}
    scene.glb    # exported binary glTF 2.0  (absent if the design has no graphics)
```

The cache root is `<config.Dir()>/models`, wired up in `server.go` (see below).

### Lifecycle

- **`newModelCache(root, maxBytes)`** — seeds the index from disk on startup.
  A directory holding `data.json` is adopted as a `SUCCESS` entry (the GLB is
  optional). Anything else (a partial/interrupted decode) is removed so it
  rebuilds cleanly. Adopted entries carry an empty `key` (the hash is not
  reversible) but still serve cached bytes, since the handler re-derives the
  directory from the key on each request.
- **`begin(key, designName)`** — claims a key. Returns `started=true` to
  exactly **one** caller (under concurrent polls); concurrent or
  already-`SUCCESS`/`PENDING` callers get `started=false`. A previously `FAILED`
  key is *reclaimed* (status reset to `PENDING`, directory recreated empty) so
  re-opening the tab retries. On `started=true` the entry directory is freshly
  recreated empty.
- **`markSuccess(key)`** — flips `PENDING` → `SUCCESS`, recomputes on-disk size,
  and runs `evictToCapLocked`.
- **`markFailed(key, msg)`** — removes the directory (holds at most a partial
  download) but keeps the index entry so the status endpoint can report
  `errMsg`; the next `begin` reclaims it.
- **`touch(key)`** — bumps recency on a cache hit, keeping eviction LRU.
- **`snapshot(key)`** — returns a copy of the entry (or nil) for the handlers.

### Eviction

`evictToCapLocked` removes least-recently-used `SUCCESS` entries until `total`
is within `maxBytes` (`modelCacheMaxBytes`, 4 GiB). **`PENDING` entries are
never evicted** — their job is in flight. `maxBytes <= 0` disables eviction.
`dirSize` shallow-walks an entry directory (the layout is flat per entry).

---

## The async job + endpoints (`server/handlers_model.go`)

This follows the thumbnail-style `PENDING`/`SUCCESS`/`FAILED` model. All three
routes are registered behind `requireAuth` (see routes below).

### Tunables

- `modelDecodeConcurrency = 2` — caps concurrent reader subprocesses (each
  large-assembly decode is memory-hungry). Enforced by `modelSem`.
- `modelCacheMaxBytes = 4 << 30` — 4 GiB on-disk cap.
- `modelJobTimeout = 8 * time.Minute` — backstop against a wedged subprocess for
  one full download+decode+export. The job runs on a **background context**
  (`context.WithTimeout(context.Background(), …)`), **not** the polling request's
  context, so it survives across the frontend's status polls.

### `GET /api/items/model` → `handleModelStatus`

Status endpoint. Query params: `hubId`, `itemId`, `ver` (optional, recommended),
`dmProjectId`.

- If `s.models` is nil (cache couldn't be created), replies `503`.
- If an entry exists, returns its status. For `SUCCESS` it also reports
  `hasGlb` (whether `scene.glb` exists — a graphics-less design decodes fine but
  produces no mesh) and `touch`es the entry.
- If no entry exists, it resolves the session token, reads `dmProjectId` from
  the query, calls `begin(key, "")`, and — if it won the claim — launches
  `go s.runModelJob(...)`. Returns `PENDING` immediately.

The response DTO is `ModelStatusDTO{ Status, Error, HasGLB }`.

### `runModelJob(key, hubID, itemID, dmProjectID, token)`

The full pipeline, run as a goroutine:

1. Acquire `s.modelSem` (bounded concurrency); create the timeout context.
2. `resolveReaderBin()` — fail stage `reader` if the CLI is missing.
3. Guard: empty `dmProjectID` fails stage `binary` with a clear message ("open
   the design from within its project").
4. `snapshot(key)` to recover the entry directory; bail quietly if it was
   evicted/reclaimed mid-flight.
5. `api.GetDesignBinary` → version URN (stage `binary`).
6. `api.ResolveDesignDownloadURL` → signed URL + filename (stage `resolve`).
7. `api.DownloadFileToPath` → `<dir>/design.<ext>` (stage `download`). The
   extension is taken from the resolved filename; anything other than `.f3d` /
   `.f3z` defaults to `.f3z` (the assembly path is the safer guess).
8. `decodeDesignJSON` → `reader.json` (stage `decode`).
9. `projectModelData` → `data.json` (stage `project`).
10. `exportGLB` → `scene.glb`. **GLB export failure is non-fatal**: a design
    saved without cached OGS graphics yields no mesh, but the parameters/timeline
    are still valid, so the job still ends `SUCCESS`. A `.f3z` that fails the
    assembly export is retried as a single-design export before giving up; on
    total failure the partial `scene.glb` is removed and a warning logged.
11. Delete the large intermediates (`design.<ext>`, `reader.json`); keep only
    `data.json` + `scene.glb`.
12. `markSuccess(key)`.

Any stage error calls `fail(stage, err)` → logs and `markFailed(key,
"<stage>: <err>")`. `stageError` is a tiny error type for stage failures that
don't wrap an existing error.

### `GET /api/items/model/data` → `handleModelData`

Serves the projected `{parameters, timeline}` JSON. Requires the entry to be
`SUCCESS` and `data.json` to exist (else `404 "model data not ready"`). Uses
`http.ServeFile`, `Content-Type: application/json; charset=utf-8`,
`Cache-Control: private, max-age=300`, and `touch`es the entry.

### `GET /api/items/model/glb` → `handleModelGLB`

Serves the exported binary glTF. Requires `SUCCESS` and `scene.glb` to exist
(else `404 "no geometry for this model"`). Uses `http.ServeFile`, so conditional
requests and **Range** support come for free and the GLB is never buffered in
server memory. `Content-Type: model/gltf-binary`,
`Cache-Control: private, max-age=300`, and `touch`es the entry.

---

## Routes (`server/routes.go`)

The three model routes are registered as Go 1.22 method-qualified patterns,
each wrapped by `prot(...)` (= `requireAuth`):

```go
mux.HandleFunc("GET /api/items/model",      prot(s.handleModelStatus))
mux.HandleFunc("GET /api/items/model/data", prot(s.handleModelData))
mux.HandleFunc("GET /api/items/model/glb",  prot(s.handleModelGLB))
```

`requireAuth` resolves the session's APS token into the request context (or
replies `401`), so the model endpoints never see an unauthenticated request.

## Server wiring (`server/server.go`)

The `Server` struct holds:

```go
models   *modelCache      // on-disk decoded-design cache, shared across clients
modelSem chan struct{}    // bounds concurrent reader subprocesses
```

In `Run`:

- `modelSem` is `make(chan struct{}, modelDecodeConcurrency)`.
- The cache is created under `<config.Dir()>/models`
  (`= ~/.config/fusionlocalserver/models`) via
  `newModelCache(filepath.Join(cfgDir, "models"), modelCacheMaxBytes)`.
- Cache creation failure is **non-fatal**: `s.models` stays nil, the model
  endpoints reply `503`, and the rest of the server runs normally. `config.Dir()`
  resolves `~/.config/fusionlocalserver`.

---

## Reader integration (`server/reader.go`)

### Why shell out instead of importing `f3d-reader`

`fusionlocalserver` never imports any `f3d-reader` Go package. The decoder holds
**package-level wire state that is not concurrency-safe**, and it pulls in a
**non-stdlib zstd dependency**. Shelling out keeps `fusionlocalserver`
std-lib-only and lets multiple decodes run as isolated processes (bounded by
`modelSem`), mirroring how `f3d-viewer` drives the same CLI.

### Locating the binary — `resolveReaderBin`

Resolves once (`sync.Once`) and caches the path. Resolution order:

1. **`FLS_READER_BIN`** env override (dev / custom installs). If set but not
   executable, this is an error.
2. **`<exeDir>/f3d-reader/bin/f3d-reader`** — the release-bundle layout
   (symmetric with `f3d-viewer`, so the same bundle script serves both).
3. **`<exeDir>/f3d-reader`** — a flat binary placed directly beside the server.
4. **`f3d-reader` on `$PATH`** — dev convenience when both repos are built.

`exeDir` is derived from `os.Executable()` with symlinks resolved.
`readerBinName()` returns `f3d-reader.exe` on Windows. `isExecutable` checks the
exec bit (presence is enough on Windows). If nothing is found, the error points
the user at `make bundle` or `FLS_READER_BIN`.

### Decode — `decodeDesignJSON`

Runs `f3d-reader <input>` (plain — **not** `--full`) and captures stdout to
`reader.json`. The `synthesized.parameters` / `synthesized.timeline` we need are
in the stdout document regardless of `--full`; `--full` would only additionally
extract opaque ZIP parts to disk, which this feature never uses. Subprocess
stderr's last line is surfaced in any error (`lastLine`).

### GLB export — `exportGLB`

Picks the exporter flag by archive type: `--assembly-glb` for a `.f3z`
(composes the root + each XREF'd component at its world transform), else
`--export-glb` for a single `.f3d`. `isAssemblyArchive` decides by extension.
Both require the design to carry cached OGS graphics; a cloud file saved without
graphics yields a sparse/empty GLB (the data tabs still work). The job's
fallback (assembly → single) lives in `runModelJob`, not here.

### Projection — `projectModelData`

Reads `reader.json`, lifts `synthesized.parameters` and `synthesized.timeline`
as **raw JSON** (`json.RawMessage`) into a compact `modelData{ Parameters,
Timeline }`, and writes `data.json`. Lifting verbatim avoids re-encoding the
(potentially large) contents and keeps the served response far smaller than the
full `reader.json`. Missing fields default to `{}` (parameters) and `[]`
(timeline).

---

## Bundling the reader at build time

### `scripts/bundle-reader.sh`

Places the `f3d-reader` CLI at `<DEST_DIR>/f3d-reader/bin/f3d-reader` — exactly
the path `resolveReaderBin` looks for in step 2. `DEST_DIR` defaults to the repo
root (where `make build` writes `./fusionlocalserver`). Source resolution
(first hit wins):

1. **`$F3D_READER_BIN`** — copy a prebuilt reader binary verbatim (no build).
2. **`$F3D_READER_SRC`** — path to the `f3d-reader` source tree; built via
   `make -C "$SRC" cli`.
3. A few conventional checkout locations near this repo (sibling `fusion-next`
   checkouts, `~/git/...`, etc.).

It also copies an optional `prism-textures.zip` (bitmap textures the GLB
exporter references) next to the binary when present — a miss is non-fatal
(untextured export).

### Makefile `bundle` target

```make
bundle: build
	./scripts/bundle-reader.sh
```

`make bundle` builds the server (`build`) then runs the script, landing the
reader at `./f3d-reader/bin/f3d-reader` — resolved relative to the server's own
executable at runtime. For dev without bundling, set `FLS_READER_BIN` to an
already-built reader.

---

## Scopes / security

- **No new OAuth scope.** The whole download chain runs under the `data:read`
  scope the app already holds (`authScope` in `auth/oauth.go` already includes
  `data:read`). Data Management `data/v1` and OSS `oss/v2` GETs reuse the same
  bearer token and the same `httpClient` as MFGDM.
- **No bearer token on signed-URL downloads.** `DownloadFileToPath` deliberately
  sends no `Authorization` header — APS presigned S3 URLs are self-authenticated,
  and attaching the token would leak it to the URL's host. (Same rule as
  thumbnail image fetches.)
- **Signed URLs are never logged.** `trimURL` strips the query string from any
  URL that appears in a Data Management error message, so signed URLs and tokens
  never reach the logs. Job-failure logging records the stage and error, not the
  signed URL.
- **All three endpoints require auth.** They are wrapped in `requireAuth` via
  `prot(...)` in `routes.go`.
- **Disk-exhaustion guard.** Downloads are capped at `MaxDesignBytes` (2 GiB),
  and the on-disk cache is byte-capped (`modelCacheMaxBytes`, 4 GiB) with LRU
  eviction.

---

## Tests

- `api/binary_test.go` — `GetDesignBinary` GraphQL decode (version URN
  extraction, the no-binary error).
- `api/download_test.go` — the Data Management chain
  (`GetVersionDownload` / `OSSSignedDownloadURL` / `ResolveDesignDownloadURL`)
  and `DownloadFileToPath` (no-token, atomic rename, size cap), against an
  `httptest` server via `dmBaseURLForTest`.
- `server/modelcache_test.go` — cache lifecycle: `begin`/`markSuccess`/
  `markFailed`/`touch`, single-starter under concurrency, FAILED reclaim, LRU
  eviction, disk seeding.
- `server/reader_test.go` — `resolveReaderBin` ordering, `isAssemblyArchive`,
  `projectModelData` projection/defaults, `lastLine`.
- `server/reader_integration_test.go` — **opt-in** end-to-end decode + GLB
  export. Requires `F3D_TEST_F3D` (a real design file) and a bundled/resolvable
  reader binary; skipped otherwise.
