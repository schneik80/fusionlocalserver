import type { Details } from './types'

// DocumentState is a selected document's lifecycle state, shown as a badge in
// the details header:
//   • wip      — an ordinary work-in-progress save (the tip is not a milestone)
//   • version  — the tip is marked a milestone
//   • released — the tip is a formally released revision (Rev A/B/C…)
//
// This module is the single place to GET (derive) the state and label it, so as
// the API surfaces richer release signals the only change needed is here.
export type DocumentState =
  | { kind: 'wip' }
  | { kind: 'version' }
  | { kind: 'released'; revision: string }

// The fields a document's state is derived from. Keeping this a Pick (rather
// than the whole Details) lets callers SET the state from any API shape that
// carries these signals — current (isMilestone) or future (revision).
export type DocumentStateInput = Pick<Details, 'isMilestone' | 'revision'>

// documentState derives the state from a document's details. Precedence is
// most-promoted first: a release revision wins over a milestone, which wins over
// a plain save. `revision` has no API source yet, so today this returns
// 'released' only once that field is populated server-side.
export function documentState(d: DocumentStateInput): DocumentState {
  if (d.revision) return { kind: 'released', revision: d.revision }
  if (d.isMilestone) return { kind: 'version' }
  return { kind: 'wip' }
}

// documentStateLabel is the human-readable badge text for a state.
export function documentStateLabel(s: DocumentState): string {
  switch (s.kind) {
    case 'released':
      return `Released - Rev ${s.revision}`
    case 'version':
      return 'Version'
    case 'wip':
      return 'WIP'
  }
}
