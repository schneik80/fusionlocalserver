import { faDiagramProject, faListCheck, faPaperclip } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { Box, Button, CircularProgress, Stack, Typography } from '@mui/material'
import { useCallback, useEffect, useRef, useState } from 'react'
import {
  Tldraw,
  createTLStore,
  defaultBindingUtils,
  defaultShapeUtils,
  getSnapshot,
  loadSnapshot,
  type Editor,
} from 'tldraw'
import 'tldraw/tldraw.css'
import { getAssetUrlsByMetaUrl } from '@tldraw/assets/urls'
import { api } from '../api/client'
import { useColorMode } from '../state/colorMode'
import { useNav } from '../state/nav'
import { docRefFromItem, encodeDocRef } from '../components/doccard/docref'
import { encodeTaskRef, taskRefFromTask } from '../components/taskcard/taskref'
import { HubBrowserDialog } from '../components/hubbrowser/HubBrowserDialog'
import { AttachTaskDialog } from '../tasks/AttachTaskDialog'
import { ProductionRefDialog } from '../production/ProductionRefDialog'
import { CARD_H, CARD_W, FLS_CARD_TYPE, FlsCardShapeUtil } from './cardshape'
import './whiteboard.css'

// tldraw loads its fonts, icons and translations from cdn.tldraw.com by
// default. This app ships a strict CSP (default-src 'self'), so every one of
// those requests was blocked: the fonts never arrived (unreadable canvas text)
// and the blocked translations fetch rejected inside React's commit phase,
// which is what killed the board a few seconds after it opened.
//
// Resolving the assets through the bundler instead makes Vite emit them as
// same-origin files, so nothing is fetched cross-origin, the CSP stays strict,
// and the whiteboard works offline like the rest of this local-first app.
// Built once at module scope: tldraw requires this object be stable.
const ASSET_URLS = getAssetUrlsByMetaUrl()

// How long the canvas sits idle before persisting. Long enough that a stroke or
// a drag isn't a request each, short enough that a closed tab loses little.
const SAVE_DEBOUNCE_MS = 1500

type Pending = 'task' | 'production' | 'document' | null

// WhiteboardCanvas hosts one tldraw board: it loads the stored document once,
// autosaves the document scope on a debounce, and adds the app's own card
// shapes to the canvas. The tldraw UI is skinned to the app in whiteboard.css.
//
// The document is owned here rather than by react-query: it is large, it is
// written far more often than it is read, and the editor is its source of
// truth while a board is open — a polling cache would fight the user's strokes.
export function WhiteboardCanvas({
  projectId,
  boardId,
  canWrite,
}: {
  projectId: string
  boardId: string
  canWrite: boolean
}) {
  const nav = useNav()
  const { mode } = useColorMode()
  // The store's schema must know EVERY shape the editor can create, not just
  // ours: building it from the custom util alone leaves out draw/geo/arrow/text
  // and the binding utils, so the default tools write records the schema
  // rejects — which surfaces as the whole editor failing to mount.
  const [store] = useState(() =>
    createTLStore({
      shapeUtils: [...defaultShapeUtils, FlsCardShapeUtil],
      bindingUtils: [...defaultBindingUtils],
    }),
  )
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [pending, setPending] = useState<Pending>(null)

  const editorRef = useRef<Editor | null>(null)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // Suppresses the change burst that loading a document naturally produces —
  // without it, opening a board would immediately save it straight back.
  const hydrated = useRef(false)

  // ---- load once per board ----
  useEffect(() => {
    let cancelled = false
    hydrated.current = false
    setLoading(true)
    setLoadError(null)
    api
      .whiteboardDoc(projectId, boardId)
      .then((doc) => {
        if (cancelled) return
        // We store only the document scope, so restore it as such — session
        // state (camera, selection) stays per-user and is never persisted.
        if (doc) loadSnapshot(store, { document: doc as never })
      })
      .catch((e) => {
        if (!cancelled) setLoadError(e instanceof Error ? e.message : 'could not load this whiteboard')
      })
      .finally(() => {
        if (cancelled) return
        setLoading(false)
        hydrated.current = true
      })
    return () => {
      cancelled = true
    }
  }, [projectId, boardId, store])

  const flush = useCallback(async () => {
    if (!hydrated.current || !canWrite) return
    const { document } = getSnapshot(store)
    setSaving(true)
    try {
      await api.whiteboardDocSave(projectId, boardId, document)
    } catch {
      // Autosave is best-effort: the next change reschedules, and the editor
      // still holds the user's work. Surfacing a toast on every blip would be
      // noisier than useful.
    } finally {
      setSaving(false)
    }
  }, [store, projectId, boardId, canWrite])

  // ---- autosave the document scope ----
  useEffect(() => {
    if (!canWrite) return
    const unlisten = store.listen(
      () => {
        if (!hydrated.current) return
        if (saveTimer.current) clearTimeout(saveTimer.current)
        saveTimer.current = setTimeout(() => void flush(), SAVE_DEBOUNCE_MS)
      },
      // Only real document edits: 'session' covers camera/selection, which are
      // per-user view state and must not dirty a shared board.
      { scope: 'document', source: 'user' },
    )
    return () => {
      unlisten()
      if (saveTimer.current) clearTimeout(saveTimer.current)
    }
  }, [store, flush, canWrite])

  // Persist on unmount (switching boards, leaving the tab) so the last edits
  // inside the debounce window aren't lost.
  useEffect(() => {
    return () => {
      if (saveTimer.current) {
        clearTimeout(saveTimer.current)
        void flush()
      }
    }
  }, [flush])

  // Follow the app's light/dark mode rather than tldraw's own default.
  useEffect(() => {
    editorRef.current?.user.updateUserPreferences({ colorScheme: mode })
  }, [mode])

  // Drop a card at the centre of the current viewport.
  const placeCard = (token: string) => {
    const editor = editorRef.current
    if (!editor || !token) return
    const { x, y } = editor.getViewportPageBounds().center
    editor.createShape({
      type: FLS_CARD_TYPE,
      x: x - CARD_W / 2,
      y: y - CARD_H / 2,
      props: { w: CARD_W, h: CARD_H, token },
    })
  }

  return (
    <Box sx={{ flex: 1, minWidth: 0, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
      {canWrite && (
        <Stack
          direction="row"
          spacing={1}
          alignItems="center"
          sx={{ px: 1.5, py: 0.75, borderBottom: 1, borderColor: 'divider', flexShrink: 0 }}
        >
          <Typography variant="caption" color="text.secondary">
            Place a card
          </Typography>
          <Button
            size="small"
            startIcon={<FontAwesomeIcon icon={faListCheck} style={{ fontSize: 11 }} />}
            onClick={() => setPending('task')}
            sx={{ textTransform: 'none' }}
          >
            Task
          </Button>
          <Button
            size="small"
            startIcon={<FontAwesomeIcon icon={faDiagramProject} style={{ fontSize: 11 }} />}
            onClick={() => setPending('production')}
            sx={{ textTransform: 'none' }}
          >
            Job / batch
          </Button>
          <Button
            size="small"
            startIcon={<FontAwesomeIcon icon={faPaperclip} style={{ fontSize: 11 }} />}
            onClick={() => setPending('document')}
            sx={{ textTransform: 'none' }}
          >
            Document
          </Button>
          <Box sx={{ flex: 1 }} />
          <Typography variant="caption" color="text.disabled" sx={{ transition: 'opacity .2s' }}>
            {saving ? 'Saving…' : 'Saved'}
          </Typography>
        </Stack>
      )}

      <Box sx={{ flex: 1, minHeight: 0, position: 'relative' }} className="fls-tldraw">
        {loading && (
          <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', zIndex: 2 }}>
            <CircularProgress size={22} />
          </Box>
        )}
        {loadError ? (
          <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', px: 3 }}>
            <Typography variant="body2" color="error">
              {loadError}
            </Typography>
          </Box>
        ) : (
          <Tldraw
            store={store}
            assetUrls={ASSET_URLS}
            shapeUtils={[FlsCardShapeUtil]}
            onMount={(editor) => {
              editorRef.current = editor
              editor.user.updateUserPreferences({ colorScheme: mode })
              editor.updateInstanceState({ isReadonly: !canWrite })
            }}
          />
        )}
      </Box>

      {pending === 'task' && (
        <AttachTaskDialog
          open
          projectId={projectId}
          onClose={() => setPending(null)}
          onPick={(task) => {
            setPending(null)
            placeCard(encodeTaskRef(taskRefFromTask(task)))
          }}
        />
      )}
      {pending === 'production' && (
        <ProductionRefDialog
          open
          projectId={projectId}
          hubId={nav.hubId ?? ''}
          projectName={nav.project?.name ?? ''}
          onClose={() => setPending(null)}
          onPick={(token) => placeCard(token)}
        />
      )}
      {pending === 'document' && (
        <HubBrowserDialog
          open
          hubId={nav.hubId ?? null}
          title="Place a document card"
          pickLabel="Place"
          initialProject={nav.project ?? null}
          onClose={() => setPending(null)}
          onPick={(pick) => {
            setPending(null)
            if (!pick.item) return
            placeCard(encodeDocRef(docRefFromItem(pick.hubId, pick.item)))
          }}
        />
      )}
    </Box>
  )
}
