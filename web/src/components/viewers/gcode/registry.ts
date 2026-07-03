// The NC g-code dialect registry. Each entry is a data-only GcodeDialect the
// text viewer highlights; adding a controller flavor is a new entry here, not
// new code. File extensions map 1:1 to a dialect (gcodeDialectForExt); where
// real controllers share an extension (Fanuc and LinuxCNC both emit .nc/.ngc),
// the RS-274 generic dialect is the default and covers them well.
//
// Confirmed roadmap: generic ISO/RS-274 first (also serves Fanuc/Haas/LinuxCNC/
// Marlin, which are all RS-274 supersets), then the genuinely different
// syntaxes — Heidenhain conversational and Siemens Sinumerik — as their own
// entries below.

import type { GcodeDialect } from './gcode'

// Common address-letter roles shared by RS-274 family dialects. Axes read as
// variables, feed/speed as atoms, tool/offset as properties, line numbers dim.
const RS274_WORDS: Record<string, string> = {
  G: 'keyword',
  M: 'keyword',
  O: 'keyword', // program / subprogram number
  X: 'variableName',
  Y: 'variableName',
  Z: 'variableName',
  A: 'variableName',
  B: 'variableName',
  C: 'variableName',
  U: 'variableName',
  V: 'variableName',
  W: 'variableName',
  I: 'number',
  J: 'number',
  K: 'number',
  R: 'number',
  P: 'number',
  Q: 'number',
  L: 'number',
  F: 'atom', // feed rate
  S: 'atom', // spindle speed
  T: 'propertyName', // tool
  D: 'propertyName', // tool diameter / offset
  H: 'propertyName', // height offset
  E: 'propertyName', // extruder (Marlin) / parameter
  N: 'meta', // line number
}

// Generic ISO / RS-274 — the default. Handles both `( … )` and `;` comments so
// it reads CNC exports (Fanuc/Haas/LinuxCNC) and 3D-printer output (Marlin)
// alike, plus `#` macro variables and `[ ]` expressions.
const ISO_RS274: GcodeDialect = {
  id: 'iso',
  name: 'ISO / RS-274 (generic)',
  exts: ['nc', 'cnc', 'ngc', 'gc', 'gcode', 'gco', 'tap', 'ncp', 'eia', 'min'],
  lineComment: ';',
  blockComment: ['(', ')'],
  words: RS274_WORDS,
  controlKeywords: [
    'IF', 'THEN', 'ELSE', 'ENDIF', 'WHILE', 'DO', 'END', 'GOTO',
    'SUB', 'ENDSUB', 'CALL', 'RETURN', 'REPEAT',
  ],
}

// Heidenhain conversational (Klartext) — a distinct block-structured syntax
// (L / CC / CR / RND / CYCL DEF / TOOL CALL / LBL / FN). `;` comments.
const HEIDENHAIN: GcodeDialect = {
  id: 'heidenhain',
  name: 'Heidenhain (Klartext)',
  exts: ['h', 'i'],
  lineComment: ';',
  words: {
    ...RS274_WORDS,
    L: 'keyword', // linear block
    Q: 'variableName.definition', // Q parameters
  },
  controlKeywords: [
    'BEGIN', 'END', 'PGM', 'MM', 'INCH', 'TOOL', 'CALL', 'DEF', 'CYCL',
    'LBL', 'FN', 'CC', 'CR', 'RND', 'CHF', 'APPR', 'DEP', 'CT', 'FK',
  ],
}

// Siemens Sinumerik — R-parameters, MSG strings, GOTOF/GOTOB flow. `;`
// comments; main programs .MPF, subprograms .SPF.
const SIEMENS: GcodeDialect = {
  id: 'siemens',
  name: 'Siemens Sinumerik',
  exts: ['mpf', 'spf', 'arc'],
  lineComment: ';',
  words: {
    ...RS274_WORDS,
    R: 'variableName.definition', // R-parameters
  },
  controlKeywords: [
    'IF', 'ELSE', 'ENDIF', 'WHILE', 'ENDWHILE', 'FOR', 'ENDFOR', 'LOOP',
    'ENDLOOP', 'REPEAT', 'GOTOF', 'GOTOB', 'GOTOC', 'GOTO', 'MSG', 'DEF',
    'PROC', 'RET', 'CASE', 'STOPRE',
  ],
}

// All dialects, generic first (its extension claims win on lookup).
export const DIALECTS: GcodeDialect[] = [ISO_RS274, HEIDENHAIN, SIEMENS]

// The default dialect used when a file matches no more-specific one.
export const DEFAULT_GCODE_DIALECT = ISO_RS274

// gcodeDialectForExt returns the dialect that owns a file extension (lowercase,
// no dot), or undefined when it isn't g-code.
export function gcodeDialectForExt(ext: string): GcodeDialect | undefined {
  return DIALECTS.find((d) => d.exts.includes(ext))
}

// GCODE_EXTS is every extension any dialect claims — used to route a file to the
// g-code viewer.
export const GCODE_EXTS: ReadonlySet<string> = new Set(DIALECTS.flatMap((d) => d.exts))
