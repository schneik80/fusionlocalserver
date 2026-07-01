import { useCallback, useEffect, useState } from 'react'
import {
  deleteDraft,
  draftKey,
  listDrafts,
  newPageKey,
  putDraft,
  slugify,
  type WikiDraft,
} from './draftStore'

// useWikiDrafts loads a project's local drafts from IndexedDB and exposes
// mutators. It keeps an in-memory copy so the sidebar re-renders on every
// save/delete; each mutator writes through to IndexedDB then reloads.
export function useWikiDrafts(projectId: string | null) {
  const [drafts, setDrafts] = useState<WikiDraft[]>([])
  const [loading, setLoading] = useState(true)

  const reload = useCallback(async () => {
    if (!projectId) {
      setDrafts([])
      setLoading(false)
      return
    }
    setLoading(true)
    try {
      setDrafts(await listDrafts(projectId))
    } finally {
      setLoading(false)
    }
  }, [projectId])

  useEffect(() => {
    void reload()
  }, [reload])

  const save = useCallback(
    async (draft: WikiDraft) => {
      await putDraft(draft)
      await reload()
    },
    [reload],
  )

  const remove = useCallback(
    async (key: string) => {
      await deleteDraft(key)
      await reload()
    },
    [reload],
  )

  // create mints a new local-only page and persists it, returning the record so
  // the caller can open it straight into the editor.
  const create = useCallback(
    async (title: string): Promise<WikiDraft> => {
      if (!projectId) throw new Error('no project selected')
      const name = title.trim() || 'Untitled'
      const pageKey = newPageKey()
      const draft: WikiDraft = {
        key: draftKey(projectId, pageKey),
        projectId,
        pageKey,
        title: name,
        slug: slugify(name),
        markdown: `# ${name}\n\n`,
        status: 'draft',
        updatedAt: Date.now(),
      }
      await putDraft(draft)
      await reload()
      return draft
    },
    [projectId, reload],
  )

  // importPage pulls a published page's markdown into a new local draft linked to
  // its lineage urn + tip version (the "Edit as draft" flow). Reused across
  // devices: any published page can be pulled down, edited, and published back.
  const importPage = useCallback(
    async (args: {
      itemId: string
      tipVersion?: string
      title: string
      markdown: string
    }): Promise<WikiDraft> => {
      if (!projectId) throw new Error('no project selected')
      // Key linked drafts by the published itemId so re-importing updates the
      // same record rather than spawning duplicates.
      const draft: WikiDraft = {
        key: draftKey(projectId, args.itemId),
        projectId,
        pageKey: args.itemId,
        title: args.title,
        slug: slugify(args.title),
        markdown: args.markdown,
        baseItemId: args.itemId,
        baseVersion: args.tipVersion,
        status: 'published',
        updatedAt: Date.now(),
      }
      await putDraft(draft)
      await reload()
      return draft
    },
    [projectId, reload],
  )

  return { drafts, loading, reload, save, remove, create, importPage }
}
