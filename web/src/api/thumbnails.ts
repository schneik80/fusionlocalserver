// thumbnailSrc builds the same-origin preview-image URL for a document, or
// returns null when there's no previewable image (non-document kinds, or a
// drawing whose project id isn't known yet). Callers render it in an <img> with
// an onError fallback to the kind icon.
//
// Designs / configured designs render from their MFGDM component-version
// thumbnail (server-cached, classify-warmed). Drawings (.f2d) are Fusion
// composite docs with no MFGDM thumbnail and no downloadable native file, so
// they render through the Model Derivative preview, keyed by item lineage id +
// Data Management project id (the project's altId).
export function thumbnailSrc(opts: {
  kind: string
  cvId?: string
  itemId?: string
  projectAltId?: string
}): string | null {
  const { kind, cvId, itemId, projectAltId } = opts
  if (kind === 'drawing') {
    return itemId && projectAltId
      ? `/api/items/drawing/preview?itemId=${encodeURIComponent(itemId)}&dmProjectId=${encodeURIComponent(projectAltId)}`
      : null
  }
  return cvId ? `/api/items/thumbnail/image?cvId=${encodeURIComponent(cvId)}` : null
}
