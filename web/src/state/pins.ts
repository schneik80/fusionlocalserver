import { useCallback, useMemo } from 'react'
import { usePinMutations, usePins } from '../api/queries'
import type { Item } from '../api/types'
import { useNav } from './nav'

// Kinds that may be pinned — mirrors pins.IsPinnable in the Go backend
// (hubs and unknown items are excluded).
const PINNABLE = new Set([
  'project',
  'folder',
  'design',
  'drawing',
  'configured',
  'schematic',
  'pcb',
  'ecad',
])

export function isPinnable(kind: string): boolean {
  return PINNABLE.has(kind)
}

// usePinToggle exposes the current hub's pinned-id set plus a toggle that
// captures project + folder context at pin time, exactly like the TUI's
// togglePin: a project pins itself; a folder/document pins under the selected
// project, and a folder also appends itself to the folder_path so navigation
// can drill straight into it.
export function usePinToggle() {
  const nav = useNav()
  const pinsQ = usePins(nav.hubId)
  const { add, remove } = usePinMutations(nav.hubId)

  const pinnedIds = useMemo(
    () => new Set((pinsQ.data ?? []).map((p) => p.id)),
    [pinsQ.data],
  )

  const toggle = useCallback(
    (item: Item) => {
      if (!nav.hubId || !isPinnable(item.kind)) return
      if (pinnedIds.has(item.id)) {
        remove.mutate(item.id)
        return
      }
      const isProject = item.kind === 'project'
      const folderPath = isProject
        ? []
        : nav.folderStack.map((f) => ({ id: f.id, name: f.name }))
      if (item.kind === 'folder') {
        folderPath.push({ id: item.id, name: item.name })
      }
      add.mutate({
        id: item.id,
        name: item.name,
        kind: item.kind,
        project_id: isProject ? item.id : nav.project?.id,
        project_alt_id: isProject ? item.altId : nav.project?.altId,
        folder_path: folderPath,
      })
    },
    [nav.hubId, nav.project, nav.folderStack, pinnedIds, add, remove],
  )

  return { pinnedIds, toggle }
}
