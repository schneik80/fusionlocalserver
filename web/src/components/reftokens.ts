import { parseDocRef, type DocRef } from './doccard/docref'
import { parseTaskRef, type TaskRef } from './taskcard/taskref'

// splitRefTokens splits plain text (chat bodies) into text runs and ref
// tokens of BOTH schemes — fls:doc (document cards) and fls:task (task
// cards). The character class is char-for-char the one in docref.ts:
// exactly the application/x-www-form-urlencoded alphabet URLSearchParams
// emits, so trailing punctuation ("token.", "token)") never sticks to a
// token. Malformed tokens stay text (docref.ts behaviour).
const TOKEN_RE = /fls:(?:doc|task)\?[A-Za-z0-9*\-._%&=+]+/g

export type RefPart = { text: string } | { doc: DocRef } | { task: TaskRef }

export function splitRefTokens(text: string): RefPart[] {
  const parts: RefPart[] = []
  let last = 0
  for (const m of text.matchAll(TOKEN_RE)) {
    let part: RefPart | null = null
    const doc = parseDocRef(m[0])
    if (doc) {
      part = { doc }
    } else {
      const task = parseTaskRef(m[0])
      if (task) part = { task }
    }
    if (!part) continue // malformed token: leave it as text
    if (m.index! > last) parts.push({ text: text.slice(last, m.index) })
    parts.push(part)
    last = m.index! + m[0].length
  }
  if (last < text.length) parts.push({ text: text.slice(last) })
  return parts.length ? parts : [{ text }]
}
