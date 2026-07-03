import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  faBold,
  faCode,
  faImage,
  faItalic,
  faLink,
  faListUl,
  faQuoteRight,
} from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  IconButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { useTheme } from '@mui/material/styles'
import { basicSetup } from 'codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { tags as t } from '@lezer/highlight'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { WikiDraft } from './draftStore'
import { Markdown } from './Markdown'

interface WikiEditorProps {
  draft: WikiDraft
  markdownValue: string
  titleValue: string
  onChangeMarkdown: (md: string) => void
  onChangeTitle: (title: string) => void
  onDiscard: () => void
  onPublish?: () => void
  publishing?: boolean
  // Uploads an image and resolves to the markdown src + alt to embed. Undefined
  // falls back to inserting an `![alt](url)` placeholder for a manual URL.
  onUploadImage?: (file: File) => Promise<{ src: string; alt: string }>
  saved: boolean // true when the working copy is flushed to IndexedDB
}

// wrapSelection surrounds the current selection with markers (e.g. ** for bold),
// leaving the selection over the wrapped text so a second click toggles cleanly.
function wrapSelection(view: EditorView, before: string, after = before) {
  const { from, to } = view.state.selection.main
  const sel = view.state.sliceDoc(from, to)
  view.dispatch({
    changes: { from, to, insert: before + sel + after },
    selection: { anchor: from + before.length, head: from + before.length + sel.length },
  })
  view.focus()
}

// prefixLine inserts a prefix at the start of the line the cursor sits on
// (headings, list items, blockquotes).
function prefixLine(view: EditorView, prefix: string) {
  const line = view.state.doc.lineAt(view.state.selection.main.from)
  view.dispatch({ changes: { from: line.from, to: line.from, insert: prefix } })
  view.focus()
}

function insertLink(view: EditorView) {
  const { from, to } = view.state.selection.main
  const sel = view.state.sliceDoc(from, to) || 'text'
  const insert = `[${sel}](url)`
  view.dispatch({
    changes: { from, to, insert },
    // select the "url" placeholder so the user can type over it
    selection: { anchor: from + sel.length + 3, head: from + sel.length + 6 },
  })
  view.focus()
}

// insertImage drops an image reference at the cursor. Any selection becomes the
// alt text; the "url" placeholder is left selected to paste/type an image URL
// over. (Uploading an image file into the Wiki folder is a Phase 2 addition,
// riding the same APS upload path as publishing.)
function insertImage(view: EditorView) {
  const { from, to } = view.state.selection.main
  const sel = view.state.sliceDoc(from, to) || 'alt'
  const insert = `![${sel}](url)`
  view.dispatch({
    changes: { from, to, insert },
    // "![" + alt + "](" = sel.length + 4 chars precede the 3-char "url" placeholder
    selection: { anchor: from + sel.length + 4, head: from + sel.length + 7 },
  })
  view.focus()
}

export function WikiEditor({
  draft,
  markdownValue,
  titleValue,
  onChangeMarkdown,
  onChangeTitle,
  onDiscard,
  onPublish,
  publishing,
  onUploadImage,
  saved,
}: WikiEditorProps) {
  const theme = useTheme()
  const hostRef = useRef<HTMLDivElement | null>(null)
  const viewRef = useRef<EditorView | null>(null)
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [uploadingImage, setUploadingImage] = useState(false)
  // Hold the latest onChange in a ref so the update listener (installed once per
  // document) always calls the current callback without re-creating the editor.
  const onChangeRef = useRef(onChangeMarkdown)
  onChangeRef.current = onChangeMarkdown

  // A light MUI-derived CodeMirror theme so the editor blends with the app's
  // light/dark mode instead of CodeMirror's default white.
  const cmTheme = useMemo(
    () =>
      EditorView.theme(
        {
          '&': { height: '100%', backgroundColor: 'transparent', color: theme.palette.text.primary },
          '.cm-scroller': {
            overflow: 'auto',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
            fontSize: '13px',
            lineHeight: '1.6',
          },
          '.cm-gutters': {
            backgroundColor: 'transparent',
            color: theme.palette.text.disabled,
            border: 'none',
          },
          '.cm-activeLine': { backgroundColor: theme.palette.action.hover },
          '.cm-activeLineGutter': { backgroundColor: theme.palette.action.hover },
          '&.cm-focused .cm-cursor': { borderLeftColor: theme.palette.text.primary },
          '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
            backgroundColor: theme.palette.action.selected,
          },
        },
        { dark: theme.palette.mode === 'dark' },
      ),
    [theme],
  )

  // Markdown token colors. CodeMirror's default highlight style is light-mode
  // oriented — it paints link/url tokens near-black navy, unreadable on the dark
  // slate theme — so derive the readable tags from the MUI palette. This is added
  // after basicSetup (whose default style is a fallback), so it wins for these tags.
  const cmHighlight = useMemo(() => {
    const p = theme.palette
    return syntaxHighlighting(
      HighlightStyle.define([
        // Prose content stays at the full text color and only carries weight /
        // style — so headings, bold, italic, lists and inline code all read
        // crisply; earlier they inherited grey from the list/marker rules.
        { tag: t.heading, color: p.text.primary, fontWeight: '700' },
        { tag: t.strong, color: p.text.primary, fontWeight: '700' },
        { tag: t.emphasis, color: p.text.primary, fontStyle: 'italic' },
        { tag: t.strikethrough, color: p.text.primary, textDecoration: 'line-through' },
        { tag: t.list, color: p.text.primary },
        { tag: t.monospace, color: p.text.primary },
        // Links use the accent.
        { tag: [t.link, t.url, t.labelName], color: p.primary.main, textDecoration: 'underline' },
        // Blockquotes and the syntax punctuation (**, _, -, #, >, `) get one
        // readable muted tone — dim enough to distinguish, not washed out.
        { tag: t.quote, color: p.text.secondary },
        { tag: [t.processingInstruction, t.meta], color: p.text.secondary },
      ]),
    )
  }, [theme])

  // (Re)create the editor when the document changes (switching pages) or the
  // theme flips. Not on every keystroke — the editor owns the live doc.
  useEffect(() => {
    if (!hostRef.current) return
    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({
        doc: markdownValue,
        extensions: [
          basicSetup,
          markdown(),
          EditorView.lineWrapping,
          cmTheme,
          cmHighlight,
          EditorView.updateListener.of((u) => {
            if (u.docChanged) onChangeRef.current(u.state.doc.toString())
          }),
        ],
      }),
    })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
    // markdownValue is intentionally excluded: seeding once per document avoids
    // clobbering the cursor on each keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draft.key, cmTheme, cmHighlight])

  const act = (fn: (v: EditorView) => void) => () => {
    if (viewRef.current) fn(viewRef.current)
  }

  // Image button: with an uploader, pick a file, upload it, and insert the
  // resulting reference at the cursor; otherwise fall back to a URL placeholder.
  const onImageClick = onUploadImage
    ? () => fileInputRef.current?.click()
    : act(insertImage)

  async function handleImageFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = '' // allow re-selecting the same file
    if (!file || !onUploadImage || !viewRef.current) return
    setUploadingImage(true)
    try {
      const { src, alt } = await onUploadImage(file)
      const v = viewRef.current
      const { from, to } = v.state.selection.main
      v.dispatch({ changes: { from, to, insert: `![${alt}](${src})` } })
      v.focus()
    } catch {
      // eslint-disable-next-line no-alert
      alert('Image upload failed.')
    } finally {
      setUploadingImage(false)
    }
  }

  const status = saved ? 'Saved locally' : 'Saving…'

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0, minHeight: 0 }}>
      {/* Header: title + actions */}
      <Stack
        direction="row"
        spacing={1}
        alignItems="center"
        sx={{ px: 1.5, py: 1, borderBottom: 1, borderColor: 'divider' }}
      >
        <TextField
          value={titleValue}
          onChange={(e) => onChangeTitle(e.target.value)}
          placeholder="Page title"
          variant="standard"
          InputProps={{ disableUnderline: true, sx: { fontSize: 18, fontWeight: 600 } }}
          sx={{ flex: 1, minWidth: 0 }}
        />
        <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: 'nowrap' }}>
          {status}
        </Typography>
        <Tooltip title={onPublish ? 'Publish to the project Wiki folder' : 'Publishing unavailable'}>
          <span>
            <Button
              size="small"
              variant="contained"
              disabled={!onPublish || publishing}
              onClick={onPublish}
            >
              {publishing ? 'Publishing…' : 'Publish'}
            </Button>
          </span>
        </Tooltip>
        <Button size="small" color="inherit" onClick={onDiscard}>
          Close
        </Button>
      </Stack>

      {/* Formatting toolbar */}
      <Stack
        direction="row"
        spacing={0.25}
        alignItems="center"
        sx={{ px: 1, py: 0.5, borderBottom: 1, borderColor: 'divider', flexWrap: 'wrap' }}
      >
        <ToolBtn title="Heading 1" label="H1" onClick={act((v) => prefixLine(v, '# '))} />
        <ToolBtn title="Heading 2" label="H2" onClick={act((v) => prefixLine(v, '## '))} />
        <ToolBtn title="Heading 3" label="H3" onClick={act((v) => prefixLine(v, '### '))} />
        <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
        <ToolBtn title="Bold" icon={faBold} onClick={act((v) => wrapSelection(v, '**'))} />
        <ToolBtn title="Italic" icon={faItalic} onClick={act((v) => wrapSelection(v, '_'))} />
        <ToolBtn title="Inline code" icon={faCode} onClick={act((v) => wrapSelection(v, '`'))} />
        <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
        <ToolBtn title="Bulleted list" icon={faListUl} onClick={act((v) => prefixLine(v, '- '))} />
        <ToolBtn title="Quote" icon={faQuoteRight} onClick={act((v) => prefixLine(v, '> '))} />
        <ToolBtn title="Link" icon={faLink} onClick={act(insertLink)} />
        <ToolBtn
          title={onUploadImage ? 'Insert image (uploads to the page)' : 'Insert image URL'}
          icon={faImage}
          busy={uploadingImage}
          onClick={onImageClick}
        />
        <input
          ref={fileInputRef}
          type="file"
          accept="image/*"
          hidden
          onChange={handleImageFile}
        />
      </Stack>

      {/* Split pane: source (left) + live preview (right) */}
      <Box sx={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <Box
          ref={hostRef}
          sx={{
            flex: 1,
            minWidth: 0,
            borderRight: 1,
            borderColor: 'divider',
            overflow: 'hidden',
            '& .cm-editor': { height: '100%' },
            '& .cm-editor.cm-focused': { outline: 'none' },
          }}
        />
        <Box sx={{ flex: 1, minWidth: 0, overflowY: 'auto', p: 2 }}>
          <PreviewHeader status={draft.status} />
          <Markdown>{markdownValue || '_Nothing to preview yet._'}</Markdown>
        </Box>
      </Box>
    </Box>
  )
}

function ToolBtn({
  title,
  icon,
  label,
  busy,
  onClick,
}: {
  title: string
  icon?: typeof faBold
  label?: string
  busy?: boolean
  onClick: () => void
}) {
  return (
    <Tooltip title={title}>
      <IconButton
        size="small"
        disabled={busy}
        onClick={onClick}
        sx={{ color: 'text.secondary', width: 30, height: 30 }}
      >
        {busy ? (
          <CircularProgress size={13} />
        ) : icon ? (
          <FontAwesomeIcon icon={icon} style={{ fontSize: 13 }} />
        ) : (
          <Typography variant="caption" sx={{ fontWeight: 700, fontSize: 12 }}>
            {label}
          </Typography>
        )}
      </IconButton>
    </Tooltip>
  )
}

function PreviewHeader({ status }: { status: WikiDraft['status'] }) {
  return (
    <Stack direction="row" spacing={1} alignItems="center" sx={{ mb: 1 }}>
      <Typography variant="overline" color="text.secondary">
        Preview
      </Typography>
      <Chip
        size="small"
        label={status === 'draft' ? 'Local draft' : status === 'modified' ? 'Unpublished changes' : 'Published'}
        color={status === 'published' ? 'success' : 'default'}
        variant="outlined"
        sx={{ height: 18, fontSize: 10 }}
      />
    </Stack>
  )
}
