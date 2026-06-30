import { IconButton, Menu, MenuItem, Tooltip } from '@mui/material'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faArrowDownWideShort } from '@fortawesome/free-solid-svg-icons'
import { useState } from 'react'
import { useFolderContents, useProjectContents } from '../api/queries'
import type { Item } from '../api/types'
import { useNav } from '../state/nav'
import { usePinToggle } from '../state/pins'
import { Column } from './Column'
import { ItemRow } from './ItemRow'

type SortKey = 'name' | 'modified'

// sortItems orders the contents list. "name" groups containers (folders) first,
// then alphabetises; "modified" is newest-first, with undated rows (folders)
// falling to the bottom by name.
function sortItems(list: Item[], sort: SortKey): Item[] {
  const arr = [...list]
  if (sort === 'modified') {
    arr.sort((a, b) => {
      const am = a.modifiedOn ? Date.parse(a.modifiedOn) : -Infinity
      const bm = b.modifiedOn ? Date.parse(b.modifiedOn) : -Infinity
      if (am !== bm) return bm - am
      return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
    })
  } else {
    arr.sort((a, b) => {
      if (a.isContainer !== b.isContainer) return a.isContainer ? -1 : 1
      return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' })
    })
  }
  return arr
}

function SortMenu({ value, onChange }: { value: SortKey; onChange: (v: SortKey) => void }) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const pick = (v: SortKey) => {
    onChange(v)
    setAnchor(null)
  }
  return (
    <>
      <Tooltip title="Sort">
        <IconButton
          size="small"
          aria-label="Sort contents"
          onClick={(e) => setAnchor(e.currentTarget)}
          sx={{ color: 'text.secondary' }}
        >
          <FontAwesomeIcon icon={faArrowDownWideShort} style={{ fontSize: 12 }} />
        </IconButton>
      </Tooltip>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}>
        <MenuItem selected={value === 'name'} onClick={() => pick('name')}>
          Name (A–Z)
        </MenuItem>
        <MenuItem selected={value === 'modified'} onClick={() => pick('modified')}>
          Last modified
        </MenuItem>
      </Menu>
    </>
  )
}

export function ContentsColumn() {
  const nav = useNav()
  const { pinnedIds, toggle } = usePinToggle()
  const [sort, setSort] = useState<SortKey>('name')

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
  const sorted = sortItems(list, sort)

  const onRowClick = (item: Item) => {
    if (item.isContainer) nav.enterFolder(item)
    else nav.selectItem(item)
  }

  return (
    <Column
      title="Contents"
      flex={1}
      action={list.length > 0 ? <SortMenu value={sort} onChange={setSort} /> : undefined}
      loading={activeQ.isLoading}
      error={activeQ.error as Error | null}
      empty={!activeQ.isLoading && list.length === 0}
      emptyText={nav.project ? 'Empty folder' : 'Select a project'}
    >
      {sorted.map((item) => (
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
