# 3D model viewer (Parameters · Timeline · 3D)

The Details pane gains three tabs for designs — **3D**, **Parameters**, and
**Timeline** — backed by an on-demand decode of the design's native Fusion file.
When a user opens any of the three tabs, the server lazily downloads the native
`.f3d`/`.f3z`, decodes it with the bundled **`f3d-reader`** CLI, caches the
result on disk, and serves a GLB plus the projected parameters/timeline. The
browser renders the GLB in a self-contained three.js viewer (ambient occlusion,
CAD edges, HDRI lighting, a component tree with per-node visibility).

This feature is deliberately split across three companion docs:

| Doc | Covers |
|---|---|
| [`3d-viewer-backend.md`](3d-viewer-backend.md) | The Go server: the MFGDM → Data Management → OSS signed-URL **download chain**, the on-disk cache, the async decode job, the `/api/items/model*` endpoints, and the `f3d-reader` integration. |
| [`3d-viewer-frontend.md`](3d-viewer-frontend.md) | The React/MUI tabs and the **three.js viewer** (N8AO, fat-line edges, IBL, tone mapping, the component tree), plus the model-viewer → three.js history and the bugs fixed along the way. |
| [`3d-viewer-operations.md`](3d-viewer-operations.md) | **Build/run** (`make bundle` vs `make build`), bundling the reader, the HDR asset, the model cache, and a **troubleshooting** table. |

## End-to-end data flow

```
User opens the 3D / Parameters / Timeline tab (design)
  → GET /api/items/model?hubId&itemId&ver&dmProjectId   (frontend: useModelStatus)
      → server kicks an async decode job (PENDING), bounded by modelSem
          1. MFGDM:  DesignItem.binary { id }            → version URN
          2. DM:     data/v1/.../versions/{urn}          → OSS storage URN + filename
          3. OSS:    oss/v2/.../signeds3download         → presigned S3 URL
          4. fetch the signed URL (no bearer)            → design.f3z on disk
          5. f3d-reader <input>                          → reader.json
          6. project synthesized.parameters + .timeline  → data.json
          7. f3d-reader --assembly-glb | --export-glb    → scene.glb
      → status flips to SUCCESS; cached under ~/.config/fusionlocalserver/models/<hash>/
  → GET /api/items/model/data   → { parameters, timeline }   (Parameters / Timeline tabs)
  → GET /api/items/model/glb    → binary glTF                 (3D tab → three.js viewer)
```

## Why these choices (the short version)

- **MFGDM has no direct download.** `Binary` exposes only an `id` (a Data
  Management *version* URN), so the bytes come via the Data Management API
  (`data/v1` + `oss/v2`), using the `data:read` scope already held. The DM
  *project* id comes from the frontend's nav context — the item's own `project`
  field can't be resolved on these hubs. See the backend doc.
- **The reader is invoked as a CLI, never imported.** It holds package-level
  wire state (not concurrency-safe) and pulls a non-stdlib zstd dependency;
  shelling out keeps the server std-lib-only. It's bundled next to the binary by
  `make bundle`.
- **The 3D view uses three.js directly, not `<model-viewer>`.** Owning the
  three.js version and the post-processing pipeline is what finally made ambient
  occlusion (N8AO) work. See the frontend doc's history section.

## Status

The MFGDM → Data Management → OSS download chain is **verified end-to-end on a
real Collaborative-Editing hub** (a real `.f3d` downloads, decodes, and renders
its parameters/timeline). The three.js viewer is in place with AO, edges, HDRI
lighting, and the component tree; the remaining work is cosmetic tuning (AO
intensity, edge weight, default brightness) and possible lazy-loading of the
viewer chunk. Open items are tracked at the end of `3d-viewer-operations.md`.
