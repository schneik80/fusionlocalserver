// Extensible NC g-code syntax highlighting for the text viewer.
//
// There is no published CodeMirror-6 (or highlight.js) g-code package, so we
// build our own with a CodeMirror 5-style stream tokenizer — the right tool for
// a line-oriented, address-word language that needs highlighting but not a full
// syntax tree. `defineGcode` turns a data-only GcodeDialect spec into that
// tokenizer, so supporting a new NC flavor (Fanuc/Haas, LinuxCNC, Marlin,
// Heidenhain, Siemens…) is a drop-in registry entry, not new parser code.
//
// Token styles are @lezer/highlight tag names (see createTokenType in
// @codemirror/language); TextViewer maps them to theme colors via a
// HighlightStyle. Reference for token patterns: appliedengdesign/vscode-gcode-
// syntax (MIT) — its grammar, not its code.

import { StreamLanguage, type StreamParser } from '@codemirror/language'

// GcodeDialect describes one NC flavor: how it comments, and the highlight role
// of each address letter (G, M, X, F…). controlKeywords are whole words (IF,
// WHILE, GOTO…) some dialects use for macro flow.
export interface GcodeDialect {
  id: string
  name: string
  exts: string[] // lowercase, no dot — the file extensions this dialect owns
  lineComment?: string // e.g. ';' — from here to end-of-line is a comment
  blockComment?: [string, string] // e.g. ['(', ')'] — may span lines
  words: Record<string, string> // UPPERCASE address letter -> highlight tag
  controlKeywords?: string[] // whole-word macro-flow keywords
}

interface GcodeState {
  block: boolean // mid block-comment carried across lines
}

const NUM = /^[-+]?(?:\d+\.?\d*|\.\d+)/ // signed int/decimal operand
const LETTERS = /^[A-Za-z]+/

// defineGcode builds a CodeMirror stream parser from a dialect spec.
export function defineGcode(spec: GcodeDialect): StreamParser<GcodeState> {
  const controls = new Set((spec.controlKeywords ?? []).map((s) => s.toUpperCase()))
  const bOpen = spec.blockComment?.[0]
  const bClose = spec.blockComment?.[1]

  return {
    name: `gcode-${spec.id}`,
    startState: () => ({ block: false }),

    token(stream, state) {
      // Continue a block comment opened on an earlier line.
      if (state.block && bClose) {
        if (stream.skipTo(bClose)) {
          stream.match(bClose)
          state.block = false
        } else {
          stream.skipToEnd()
        }
        return 'comment'
      }

      if (stream.eatSpace()) return null
      const ch = stream.peek()
      if (ch == null) return null

      // Comments.
      if (spec.lineComment && stream.match(spec.lineComment)) {
        stream.skipToEnd()
        return 'comment'
      }
      if (bOpen && stream.match(bOpen)) {
        if (bClose) {
          if (stream.skipTo(bClose)) stream.match(bClose)
          else {
            state.block = true
            stream.skipToEnd()
          }
        } else {
          stream.skipToEnd()
        }
        return 'comment'
      }

      // Program start/end marker and block-skip.
      if (ch === '%') {
        stream.next()
        return 'meta'
      }
      if (ch === '/') {
        stream.next()
        return 'meta'
      }

      // Macro variables: #100, #<name>.
      if (ch === '#') {
        stream.next()
        if (stream.peek() === '<') {
          stream.eatWhile((c: string) => c !== '>')
          stream.eat('>')
        } else {
          stream.eatWhile(/[0-9]/)
        }
        return 'variableName.definition'
      }

      // Quoted strings (used by Siemens MSG, filenames…).
      if (ch === '"') {
        stream.next()
        stream.eatWhile((c: string) => c !== '"')
        stream.eat('"')
        return 'string'
      }

      // Expression brackets and operators.
      if (ch === '[' || ch === ']') {
        stream.next()
        return 'bracket'
      }
      if ('-+*=<>'.includes(ch)) {
        stream.next()
        return 'operator'
      }

      // Address words (letter + numeric operand) and macro-flow keywords.
      if (/[A-Za-z]/.test(ch)) {
        const run = stream.match(LETTERS) as RegExpMatchArray | null
        if (run) {
          const up = run[0].toUpperCase()
          if (controls.has(up)) return 'controlKeyword'
          stream.match(NUM) // consume the operand that follows an address letter
          return spec.words[up[0]] ?? 'variableName'
        }
      }

      // Bare number.
      if (stream.match(NUM)) return 'number'

      stream.next()
      return null
    },

    languageData: {
      commentTokens: {
        line: spec.lineComment,
        block: bOpen && bClose ? { open: bOpen, close: bClose } : undefined,
      },
    },
  }
}

// gcodeLanguage turns a dialect spec into a CodeMirror language extension.
export function gcodeLanguage(spec: GcodeDialect) {
  return StreamLanguage.define(defineGcode(spec))
}
