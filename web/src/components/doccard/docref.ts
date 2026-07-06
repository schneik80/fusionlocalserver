import type { Item } from '../../api/types'

// Doc refs are the compact, text-safe way wiki markdown and chat messages
// reference a hub document — link-unfurl style: the stored content carries
// only a small token, and renderers swap it for a rich DocumentCard at
// display time. The token is a pseudo-URL (`fls:doc?…`) so in the wiki it
// travels inside a normal markdown link (degrading to a plain named link in
// any other markdown renderer), and in chat it sits inline in the plain-text
// body.

export interface DocRef {
  hubId: string // GraphQL hub id
  itemId: string // item lineage urn
  name: string // display name at insert time (hydration refreshes it)
  kind: string // Item.kind hint at insert time (hydration refines it)
}

export const DOC_REF_PREFIX = 'fls:doc?'

export function encodeDocRef(ref: DocRef): string {
  const sp = new URLSearchParams()
  sp.set('hubId', ref.hubId)
  sp.set('itemId', ref.itemId)
  sp.set('name', ref.name)
  sp.set('kind', ref.kind)
  return DOC_REF_PREFIX + sp.toString()
}

export function parseDocRef(url: string): DocRef | null {
  if (!url.startsWith(DOC_REF_PREFIX)) return null
  const sp = new URLSearchParams(url.slice(DOC_REF_PREFIX.length))
  const hubId = sp.get('hubId') ?? ''
  const itemId = sp.get('itemId') ?? ''
  if (!hubId || !itemId) return null
  return {
    hubId,
    itemId,
    name: sp.get('name') || 'document',
    kind: sp.get('kind') || 'unknown',
  }
}

export function docRefFromItem(hubId: string, item: Item): DocRef {
  return { hubId, itemId: item.id, name: item.name, kind: item.kind }
}

// docRefMarkdown is the wiki-side form: a markdown link whose href is the
// token. Square brackets are stripped from the label only — the token itself
// carries the exact name, percent-encoded.
export function docRefMarkdown(ref: DocRef): string {
  return `[${ref.name.replace(/[[\]]/g, '')}](${encodeDocRef(ref)})`
}

// splitDocRefs splits plain text (chat bodies) into text runs and doc refs.
// The character class is exactly the application/x-www-form-urlencoded
// alphabet URLSearchParams emits, so trailing punctuation ("token.", "token)")
// never sticks to a token.
const TOKEN_RE = /fls:doc\?[A-Za-z0-9*\-._%&=+]+/g

export type DocRefPart = { text: string } | { ref: DocRef }

export function splitDocRefs(text: string): DocRefPart[] {
  const parts: DocRefPart[] = []
  let last = 0
  for (const m of text.matchAll(TOKEN_RE)) {
    const ref = parseDocRef(m[0])
    if (!ref) continue // malformed token: leave it as text
    if (m.index! > last) parts.push({ text: text.slice(last, m.index) })
    parts.push({ ref })
    last = m.index! + m[0].length
  }
  if (last < text.length) parts.push({ text: text.slice(last) })
  return parts.length ? parts : [{ text }]
}
