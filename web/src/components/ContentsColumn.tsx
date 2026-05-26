import { useFolderContents, useProjectContents } from '../api/queries'
import type { Item } from '../api/types'
import { useNav } from '../state/nav'
import { usePinToggle } from '../state/pins'
import { Column } from './Column'
import { ItemRow } from './ItemRow'

export function ContentsColumn() {
  const nav = useNav()
  const { pinnedIds, toggle } = usePinToggle()

  const atRoot = nav.folderStack.length === 0

  // At a project root, contents come from the combined folders+items endpoint;
  // inside a folder, from the folder-contents endpoint. The inactive query is
  // disabled by passing a null id.
  const rootQ = useProjectContents(atRoot ? (nav.project?.id ?? null) : null)
  const folderQ = useFolderContents(nav.hubId, atRoot ? null : nav.currentFolderId)

  const activeQ = atRoot ? rootQ : folderQ
  const list: Item[] = atRoot
    ? [...(rootQ.data?.folders ?? []), ...(rootQ.data?.items ?? [])]
    : (folderQ.data ?? [])

  const onRowClick = (item: Item) => {
    if (item.isContainer) nav.enterFolder(item)
    else nav.selectItem(item)
  }

  return (
    <Column
      title="Contents"
      width={320}
      loading={activeQ.isLoading}
      error={activeQ.error as Error | null}
      empty={!activeQ.isLoading && list.length === 0}
      emptyText={nav.project ? 'Empty folder' : 'Select a project'}
    >
      {list.map((item) => (
        <ItemRow
          key={item.id}
          item={item}
          selected={nav.selected?.id === item.id}
          onClick={() => onRowClick(item)}
          pinned={pinnedIds.has(item.id)}
          onTogglePin={toggle}
          classifyEnabled
        />
      ))}
    </Column>
  )
}
