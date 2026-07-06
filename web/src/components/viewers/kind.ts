// Picks which viewer renders an uploaded file, from its name (extension) with a
// MIME-type fallback. Fusion-native designs/drawings never reach here — they
// have no downloadable file — so this only covers generic uploads.

import { GCODE_EXTS } from './gcode/registry'

export type ViewerKind = 'image' | 'video' | 'pdf' | 'markdown' | 'gcode' | 'text' | 'none'

export function extOf(name: string): string {
  const m = /\.([a-z0-9]+)$/i.exec(name)
  return m ? m[1].toLowerCase() : ''
}

// Every raster/vector format browsers render natively in an <img>. Formats a
// browser can't display (tiff, heic, raw, …) are deliberately absent: listing
// them as images would embed references that render as broken images.
const IMAGE = new Set(['png', 'jpg', 'jpeg', 'jfif', 'gif', 'svg', 'webp', 'bmp', 'ico', 'avif', 'apng'])
const VIDEO = new Set(['mp4', 'm4v', 'webm', 'mov', 'ogv', 'ogg'])
// Plain-text / source formats the code viewer renders (g-code and markdown are
// routed separately, above).
const TEXT = new Set([
  'txt', 'log', 'csv', 'tsv', 'json', 'xml', 'yaml', 'yml', 'ini', 'cfg', 'conf', 'toml',
  'html', 'htm', 'css', 'scss', 'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'py', 'go', 'rs',
  'c', 'h', 'cpp', 'cc', 'hpp', 'sh', 'bash', 'sql', 'rb', 'java', 'kt', 'swift', 'lua',
  'pl', 'r', 'ps1', 'bat',
])

export function viewerKindFor(name: string, mimeType?: string): ViewerKind {
  const ext = extOf(name)
  if (ext === 'md' || ext === 'markdown') return 'markdown'
  if (GCODE_EXTS.has(ext)) return 'gcode'
  if (ext === 'pdf') return 'pdf'
  if (IMAGE.has(ext)) return 'image'
  if (VIDEO.has(ext)) return 'video'
  if (TEXT.has(ext)) return 'text'
  // Unknown extension: fall back to the server-reported MIME type.
  if (mimeType) {
    if (mimeType.startsWith('image/')) return 'image'
    if (mimeType.startsWith('video/')) return 'video'
    if (mimeType === 'application/pdf') return 'pdf'
    if (mimeType.startsWith('text/') || mimeType.includes('json') || mimeType.includes('xml')) return 'text'
  }
  return 'none'
}

// ViewerFile is the handle every viewer needs: the display name, the same-origin
// bytes URL (also the download href), and optional web link + human size for the
// fallback card.
export interface ViewerFile {
  name: string
  url: string
  webUrl?: string
  size?: string
}
