import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Stack,
  Typography,
} from '@mui/material'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useWikiPage, useWikiPages } from '../api/queries'
import { useNav } from '../state/nav'
import { slugify, type DraftStatus, type WikiDraft } from './draftStore'
import { Markdown } from './Markdown'
import { useWikiDrafts } from './useDrafts'
import { WikiEditor } from './WikiEditor'
import { WikiSidebar, type WikiEntry } from './WikiSidebar'

// Autosave debounce: how long after the last keystroke the working copy is
// flushed to IndexedDB.
const AUTOSAVE_MS = 600

// WikiApp is the whole project wiki experience, mounted inside the project-level
// tab shell (and reused when the browser drills into the Wiki folder). Left: a
// flat list merging published pages with local drafts. Right: a reader, or the
// split-pane editor. Publishing (upload to APS) is Phase 2; everything here —
// authoring local drafts, reading published pages, pulling one down as a draft —
// works under the current read-only scope.
export function WikiApp({ active = true }: { active?: boolean }) {
  const nav = useNav()
  const project = nav.project
  const hubId = nav.hubId
  const dmProjectId = project?.altId ?? null
  const projectId = project?.id ?? null

  // The pane stays mounted while the Dashboard tab is showing; gate the published
  // -pages fetch on the Wiki tab actually being open so it costs no APS calls for
  // users who never visit it (mirrors DetailsPanel's per-tab query gating).
  const pagesQ = useWikiPages(hubId, dmProjectId, active)
  const { drafts, loading: draftsLoading, save, remove, create, importPage } = useWikiDrafts(projectId)

  // Keep a ref to the latest drafts so effects/handlers can read them without
  // taking `drafts` as a dependency (which would retrigger on every save).
  const draftsRef = useRef<WikiDraft[]>(drafts)
  draftsRef.current = drafts

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [query, setQuery] = useState('')

  // Editor working copy (source of truth while editing; flushed on a debounce).
  const [workingMd, setWorkingMd] = useState('')
  const [workingTitle, setWorkingTitle] = useState('')
  const [saved, setSaved] = useState(true)

  // This pane stays mounted across project switches (BrowserStage keeps slot B
  // panes alive), so clear any selection/edit that belongs to the prior project.
  useEffect(() => {
    setSelectedId(null)
    setEditingKey(null)
  }, [projectId])

  // Merge published pages and local drafts into one flat, searchable list. A
  // draft linked to a published page (baseItemId) supersedes the remote row.
  const entries = useMemo<WikiEntry[]>(() => {
    const pages = pagesQ.data ?? []
    const linked = new Map<string, WikiDraft>()
    const localOnly: WikiDraft[] = []
    for (const d of drafts) {
      if (d.baseItemId) linked.set(d.baseItemId, d)
      else localOnly.push(d)
    }
    const out: WikiEntry[] = []
    for (const p of pages) {
      const d = linked.get(p.itemId)
      if (d) {
        out.push({ id: d.key, kind: 'draft', title: d.title, status: d.status, draftKey: d.key, itemId: p.itemId, modifiedOn: p.modifiedOn })
      } else {
        out.push({ id: p.itemId, kind: 'page', title: p.title, status: 'remote', itemId: p.itemId, modifiedOn: p.modifiedOn })
      }
    }
    for (const d of localOnly) {
      out.push({ id: d.key, kind: 'draft', title: d.title, status: d.status, draftKey: d.key })
    }
    out.sort((a, b) => a.title.localeCompare(b.title))
    const q = query.trim().toLowerCase()
    return q ? out.filter((e) => e.title.toLowerCase().includes(q)) : out
  }, [pagesQ.data, drafts, query])

  const selectedEntry = entries.find((e) => e.id === selectedId) ?? null
  const editingDraft = editingKey ? drafts.find((d) => d.key === editingKey) ?? null : null

  function beginEdit(d: WikiDraft) {
    setEditingKey(d.key)
    setSelectedId(d.key)
    setWorkingMd(d.markdown)
    setWorkingTitle(d.title)
    setSaved(true)
  }

  // Autosave the working copy while editing. Reads the baseline from the ref so
  // it doesn't re-run when `save()` reloads the drafts list.
  useEffect(() => {
    if (!editingKey) return
    const base = draftsRef.current.find((d) => d.key === editingKey)
    if (!base) return
    if (base.markdown === workingMd && base.title === workingTitle) {
      setSaved(true)
      return
    }
    setSaved(false)
    const t = setTimeout(() => {
      void save({
        ...base,
        title: workingTitle || 'Untitled',
        slug: slugify(workingTitle || 'untitled'),
        markdown: workingMd,
        status: base.baseItemId ? 'modified' : 'draft',
        updatedAt: Date.now(),
      }).then(() => setSaved(true))
    }, AUTOSAVE_MS)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [workingMd, workingTitle, editingKey])

  async function closeEditor() {
    const base = editingKey ? draftsRef.current.find((d) => d.key === editingKey) : null
    if (base && (base.markdown !== workingMd || base.title !== workingTitle)) {
      await save({
        ...base,
        title: workingTitle || 'Untitled',
        slug: slugify(workingTitle || 'untitled'),
        markdown: workingMd,
        status: base.baseItemId ? 'modified' : 'draft',
        updatedAt: Date.now(),
      })
    }
    setEditingKey(null)
  }

  async function handleNew() {
    const d = await create('Untitled')
    beginEdit(d)
  }

  async function handleDelete(key: string) {
    await remove(key)
    if (selectedId === key) setSelectedId(null)
    if (editingKey === key) setEditingKey(null)
  }

  async function handleEditAsDraft(payload: { itemId: string; title: string; tipVersion?: string; markdown: string }) {
    const d = await importPage(payload)
    beginEdit(d)
  }

  return (
    <Box sx={{ display: 'flex', flex: 1, minWidth: 0, minHeight: 0 }}>
      <WikiSidebar
        entries={entries}
        selectedId={selectedId}
        onSelect={(e) => {
          setSelectedId(e.id)
          setEditingKey(null)
        }}
        onNew={handleNew}
        loading={draftsLoading || pagesQ.isLoading}
        query={query}
        onQuery={setQuery}
      />

      <Box sx={{ flex: 1, minWidth: 0, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        {pagesQ.error && (
          <Alert severity="warning" variant="outlined" square sx={{ borderLeft: 0, borderRight: 0, borderTop: 0 }}>
            Couldn't load published pages. Local drafts are still available.
          </Alert>
        )}

        {editingDraft ? (
          <WikiEditor
            draft={editingDraft}
            markdownValue={workingMd}
            titleValue={workingTitle}
            onChangeMarkdown={setWorkingMd}
            onChangeTitle={setWorkingTitle}
            onDiscard={closeEditor}
            saved={saved}
            // onPublish is intentionally omitted until Phase 2 (upload to APS).
          />
        ) : selectedEntry?.kind === 'draft' ? (
          <DraftReader
            draft={drafts.find((d) => d.key === selectedEntry.draftKey) ?? null}
            onEdit={beginEdit}
            onDelete={handleDelete}
          />
        ) : selectedEntry?.kind === 'page' ? (
          <PublishedReader
            dmProjectId={dmProjectId}
            page={pagesQ.data?.find((p) => p.itemId === selectedEntry.itemId) ?? null}
            onEditAsDraft={handleEditAsDraft}
          />
        ) : (
          <EmptyState onNew={handleNew} />
        )}
      </Box>
    </Box>
  )
}

function EmptyState({ onNew }: { onNew: () => void }) {
  return (
    <Stack spacing={1.5} alignItems="center" justifyContent="center" sx={{ flex: 1, p: 4, textAlign: 'center' }}>
      <Typography variant="h6" color="text.secondary">
        Project Wiki
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 360 }}>
        Author markdown pages for this project. Drafts are saved on this device until you
        publish them to the project's Wiki folder.
      </Typography>
      <Button variant="outlined" onClick={onNew}>
        New page
      </Button>
    </Stack>
  )
}

// DraftReader shows a local draft's rendered markdown with edit/delete actions.
function DraftReader({
  draft,
  onEdit,
  onDelete,
}: {
  draft: WikiDraft | null
  onEdit: (d: WikiDraft) => void
  onDelete: (key: string) => void
}) {
  if (!draft) return <EmptyState onNew={() => {}} />
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      <Stack
        direction="row"
        spacing={1}
        alignItems="center"
        sx={{ px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}
      >
        <Typography variant="h6" noWrap sx={{ flex: 1, minWidth: 0, fontWeight: 600 }}>
          {draft.title}
        </Typography>
        <StatusChip status={draft.status} />
        <Button size="small" variant="contained" onClick={() => onEdit(draft)}>
          Edit
        </Button>
        <Button size="small" color="error" onClick={() => onDelete(draft.key)}>
          Delete
        </Button>
      </Stack>
      <Box sx={{ flex: 1, overflowY: 'auto', p: 2 }}>
        <Markdown>{draft.markdown}</Markdown>
      </Box>
    </Box>
  )
}

// PublishedReader fetches and renders a published page, offering "Edit as draft"
// to pull it into the local editor.
function PublishedReader({
  dmProjectId,
  page,
  onEditAsDraft,
}: {
  dmProjectId: string | null
  page: { itemId: string; title: string; tipVersion?: string } | null
  onEditAsDraft: (payload: { itemId: string; title: string; tipVersion?: string; markdown: string }) => void
}) {
  const contentQ = useWikiPage(dmProjectId, page?.itemId ?? null, !!page)

  if (!page) return null
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      <Stack
        direction="row"
        spacing={1}
        alignItems="center"
        sx={{ px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}
      >
        <Typography variant="h6" noWrap sx={{ flex: 1, minWidth: 0, fontWeight: 600 }}>
          {page.title}
        </Typography>
        <StatusChip status="remote" />
        <Button
          size="small"
          variant="contained"
          disabled={!contentQ.data}
          onClick={() =>
            contentQ.data &&
            onEditAsDraft({
              itemId: page.itemId,
              title: page.title,
              tipVersion: page.tipVersion,
              markdown: contentQ.data.markdown,
            })
          }
        >
          Edit as draft
        </Button>
      </Stack>
      <Box sx={{ flex: 1, overflowY: 'auto', p: 2 }}>
        {contentQ.isLoading ? (
          <Box sx={{ p: 2, textAlign: 'center' }}>
            <CircularProgress size={18} />
          </Box>
        ) : contentQ.error ? (
          <Alert severity="error" variant="outlined">
            Couldn't download this page.
          </Alert>
        ) : (
          <Markdown>{contentQ.data?.markdown ?? ''}</Markdown>
        )}
      </Box>
    </Box>
  )
}

function StatusChip({ status }: { status: DraftStatus | 'remote' }) {
  const meta =
    status === 'draft'
      ? { label: 'Local draft', color: 'default' as const }
      : status === 'modified'
        ? { label: 'Unpublished changes', color: 'warning' as const }
        : status === 'published'
          ? { label: 'Synced', color: 'success' as const }
          : { label: 'Published', color: 'info' as const }
  return <Chip size="small" label={meta.label} color={meta.color} variant="outlined" sx={{ height: 20 }} />
}
