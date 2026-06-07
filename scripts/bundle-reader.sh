#!/usr/bin/env bash
# bundle-reader.sh — place the f3d-reader CLI next to the fusionlocalserver
# binary so the server can shell out to it at runtime.
#
# fusionlocalserver never imports any f3d-reader Go package (the decoder holds
# package-level wire state that isn't concurrency-safe and pulls in a non-stdlib
# zstd dependency we keep out of this binary). Instead it resolves a sibling
# binary at <exeDir>/f3d-reader/bin/f3d-reader and execs it. This script builds
# (or copies) that binary into place.
#
# Usage:
#   scripts/bundle-reader.sh [DEST_DIR]
#
# DEST_DIR defaults to the repo root (where `make build` writes
# ./fusionlocalserver). The reader lands at DEST_DIR/f3d-reader/bin/f3d-reader.
#
# Source resolution (first hit wins):
#   1. $F3D_READER_BIN — a prebuilt reader binary to copy verbatim (no build).
#   2. $F3D_READER_SRC — path to the f3d-reader source tree; built via `make cli`.
#   3. A few conventional checkout locations next to / near this repo.
set -euo pipefail

DEST_DIR="${1:-$(cd "$(dirname "$0")/.." && pwd)}"
READER_NAME="f3d-reader"
case "$(uname -s)" in
  MINGW* | MSYS* | CYGWIN*) READER_NAME="f3d-reader.exe" ;;
esac
OUT_DIR="$DEST_DIR/f3d-reader/bin"
OUT="$OUT_DIR/$READER_NAME"

mkdir -p "$OUT_DIR"

copy_extras() {
  # prism-textures.zip, when present beside the reader binary, supplies the
  # bitmap textures the GLB exporter references. Optional — the exporter runs
  # without it (untextured), so a miss is not fatal.
  local src_bin_dir="$1"
  if [ -f "$src_bin_dir/prism-textures.zip" ]; then
    cp -f "$src_bin_dir/prism-textures.zip" "$OUT_DIR/prism-textures.zip"
    echo "bundle-reader: copied prism-textures.zip"
  fi
}

# 1. Prebuilt binary supplied directly.
if [ -n "${F3D_READER_BIN:-}" ]; then
  [ -x "$F3D_READER_BIN" ] || { echo "bundle-reader: F3D_READER_BIN=$F3D_READER_BIN is not executable" >&2; exit 1; }
  cp -f "$F3D_READER_BIN" "$OUT"
  copy_extras "$(dirname "$F3D_READER_BIN")"
  echo "bundle-reader: copied prebuilt reader -> $OUT"
  exit 0
fi

# 2/3. Locate the source tree.
SRC="${F3D_READER_SRC:-}"
if [ -z "$SRC" ]; then
  for cand in \
    "$DEST_DIR/../fusion-next/f3d-reader" \
    "$DEST_DIR/../../fusion-next/f3d-reader" \
    "$HOME/git/fusion-next/f3d-reader" \
    "$HOME/Dropbox/Transfer/jh-source/fusion-next/f3d-reader"; do
    if [ -d "$cand" ]; then SRC="$cand"; break; fi
  done
fi
[ -n "$SRC" ] && [ -d "$SRC" ] || {
  echo "bundle-reader: f3d-reader source not found." >&2
  echo "  Set F3D_READER_SRC=/path/to/fusion-next/f3d-reader, or" >&2
  echo "  set F3D_READER_BIN=/path/to/prebuilt/f3d-reader to copy a prebuilt binary." >&2
  exit 1
}

echo "bundle-reader: building reader from $SRC"
make -C "$SRC" cli
[ -x "$SRC/bin/$READER_NAME" ] || { echo "bundle-reader: make cli did not produce $SRC/bin/$READER_NAME" >&2; exit 1; }
cp -f "$SRC/bin/$READER_NAME" "$OUT"
copy_extras "$SRC/bin"
echo "bundle-reader: bundled reader -> $OUT"
