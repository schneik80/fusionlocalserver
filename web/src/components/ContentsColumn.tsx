import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faPlus } from '@fortawesome/free-solid-svg-icons'
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  Menu,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
} from '@mui/material'
import { useEffect, useMemo, useState } from 'react'
import {
  useFolderContents,
  useFolderMutations,
  useProjectContents,
} from '../api/queries'
import type { Item } from '../api/types'
import { useNav } from '../state/nav'
import { usePinToggle } from '../state/pins'
import { Column } from './Column'
import { ItemRow } from './ItemRow'

type SortKey = 'name' | 'modified'

type MenuState = { folder: Item; x: number; y: number } | null

export function ContentsColumn() {
  const nav = useNav()
  const { pinnedIds, toggle } = usePinToggle()

  const atRoot = nav.folderStack.length === 0
  const parentFolderId = atRoot ? undefined : nav.currentFolderId ?? undefined

  // At a project root, contents come from the combined folders+items endpoint;
  // inside a folder, from the folder-contents endpoint. The inactive query is
  // disabled by passing a null id.
  const rootQ = useProjectContents(atRoot ? (nav.project?.id ?? null) : null)
  const folderQ = useFolderContents(nav.hubId, atRoot ? null : nav.currentFolderId)

  // The mutations hook invalidates both project-root and folder-contents
  // queries on success, so the active column refreshes regardless of where we
  // are in the tree.
  const mut = useFolderMutations(
    nav.hubId,
    nav.project?.id ?? null,
    nav.currentFolderId,
  )

  // Top-level folders of the current project — used to populate the Move
  // dialog's destination picker. Always available (the project-root query is
  // cheap and cached); fetched even when we're inside a subfolder.
  const projectRootQ = useProjectContents(nav.project?.id ?? null)
  const topLevelFolders: Item[] = projectRootQ.data?.folders ?? []

  const activeQ = atRoot ? rootQ : folderQ
  const rawFolders: Item[] = atRoot
    ? (rootQ.data?.folders ?? [])
    : (folderQ.data ?? []).filter((i) => i.kind === 'folder')
  const rawItems: Item[] = atRoot
    ? (rootQ.data?.items ?? [])
    : (folderQ.data ?? []).filter((i) => i.kind !== 'folder')

  // Session-scoped sort — folders stay grouped above items.
  const [sortKey, setSortKey] = useState<SortKey>('name')
  const list: Item[] = useMemo(() => {
    const sorted = (xs: Item[]) => sortItems(xs, sortKey)
    return [...sorted(rawFolders), ...sorted(rawItems)]
  }, [rawFolders, rawItems, sortKey])

  const onRowClick = (item: Item) => {
    if (item.isContainer) nav.enterFolder(item)
    else nav.selectItem(item)
  }

  // Dialog / menu state. Only folder rows get a context menu.
  const [menu, setMenu] = useState<MenuState>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [renaming, setRenaming] = useState<Item | null>(null)
  const [moving, setMoving] = useState<Item | null>(null)
  const [deleting, setDeleting] = useState<Item | null>(null)

  const canCreate = !!nav.project?.id

  return (
    <>
      <Column
        title="Contents"
        width={320}
        loading={activeQ.isLoading}
        error={activeQ.error as Error | null}
        empty={!activeQ.isLoading && list.length === 0}
        emptyText={nav.project ? 'Empty folder' : 'Select a project'}
        headerAction={
          <Stack direction="row" spacing={0.5} alignItems="center">
            <TextField
              select
              size="small"
              value={sortKey}
              onChange={(e) => setSortKey(e.target.value as SortKey)}
              variant="standard"
              SelectProps={{ disableUnderline: true }}
              sx={{
                '& .MuiInputBase-input': {
                  fontSize: 11,
                  py: 0,
                  color: 'text.secondary',
                  textTransform: 'uppercase',
                  letterSpacing: 0.5,
                },
              }}
            >
              <MenuItem value="name" sx={{ fontSize: 12 }}>
                Name
              </MenuItem>
              <MenuItem value="modified" sx={{ fontSize: 12 }}>
                Last Modified
              </MenuItem>
            </TextField>
            <Tooltip title="New folder">
              <span>
                <IconButton
                  size="small"
                  disabled={!canCreate}
                  onClick={() => setCreateOpen(true)}
                  sx={{ color: 'text.secondary' }}
                  aria-label="New folder"
                >
                  <FontAwesomeIcon icon={faPlus} style={{ fontSize: 13 }} />
                </IconButton>
              </span>
            </Tooltip>
          </Stack>
        }
      >
        {list.map((item) => {
          const isFolder = item.kind === 'folder'
          return (
            <ItemRow
              key={item.id}
              item={item}
              selected={nav.selected?.id === item.id}
              onClick={() => onRowClick(item)}
              pinned={pinnedIds.has(item.id)}
              onTogglePin={toggle}
              classifyEnabled
              onContextMenu={
                isFolder
                  ? (e) => {
                      e.preventDefault()
                      setMenu({ folder: item, x: e.clientX, y: e.clientY })
                    }
                  : undefined
              }
            />
          )
        })}
      </Column>

      {/* Folder row context menu */}
      <Menu
        open={!!menu}
        onClose={() => setMenu(null)}
        anchorReference="anchorPosition"
        anchorPosition={menu ? { top: menu.y, left: menu.x } : undefined}
      >
        <MenuItem
          onClick={() => {
            setRenaming(menu!.folder)
            setMenu(null)
          }}
        >
          Rename…
        </MenuItem>
        <MenuItem
          onClick={() => {
            setMoving(menu!.folder)
            setMenu(null)
          }}
        >
          Move…
        </MenuItem>
        <MenuItem
          onClick={() => {
            setDeleting(menu!.folder)
            setMenu(null)
          }}
        >
          Delete…
        </MenuItem>
      </Menu>

      {/* Create folder */}
      <NameDialog
        key={createOpen ? 'create-open' : 'create-closed'}
        open={createOpen}
        title="New folder"
        label="Folder name"
        confirmLabel="Create"
        pending={mut.create.isPending}
        error={mut.create.error as Error | null}
        onClose={() => setCreateOpen(false)}
        onSubmit={(name) =>
          mut.create.mutate(
            { name, parentFolderId },
            { onSuccess: () => setCreateOpen(false) },
          )
        }
      />

      {/* Rename folder */}
      <NameDialog
        key={renaming ? `rename-${renaming.id}` : 'rename-closed'}
        open={!!renaming}
        title="Rename folder"
        label="Folder name"
        confirmLabel="Rename"
        initial={renaming?.name}
        pending={mut.rename.isPending}
        error={mut.rename.error as Error | null}
        onClose={() => setRenaming(null)}
        onSubmit={(name) =>
          mut.rename.mutate(
            { folderId: renaming!.id, name },
            { onSuccess: () => setRenaming(null) },
          )
        }
      />

      {/* Move folder */}
      <MoveFolderDialog
        key={moving ? `move-${moving.id}` : 'move-closed'}
        open={!!moving}
        folder={moving}
        candidates={topLevelFolders}
        pending={mut.move.isPending}
        error={mut.move.error as Error | null}
        onClose={() => setMoving(null)}
        onSubmit={(destinationFolderId) =>
          mut.move.mutate(
            { folderId: moving!.id, destinationFolderId },
            { onSuccess: () => setMoving(null) },
          )
        }
      />

      {/* Delete folder confirm */}
      <Dialog open={!!deleting} onClose={() => setDeleting(null)}>
        <DialogTitle>Delete folder?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Delete folder “{deleting?.name}”? Its contents will also be removed.
          </DialogContentText>
          {mut.del.error && (
            <DialogContentText sx={{ mt: 1, color: 'error.main' }}>
              {(mut.del.error as Error).message}
            </DialogContentText>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleting(null)}>Cancel</Button>
          <Button
            color="error"
            disabled={mut.del.isPending}
            onClick={() =>
              mut.del.mutate(deleting!.id, { onSuccess: () => setDeleting(null) })
            }
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

// sortItems orders folders/items within their group. Name is ascending (locale
// aware); Last Modified is descending and falls back to "" so missing
// timestamps sink to the bottom. RFC3339 sorts correctly as a string compare.
function sortItems(xs: Item[], key: SortKey): Item[] {
  const ys = xs.slice()
  if (key === 'name') {
    ys.sort((a, b) => a.name.localeCompare(b.name))
  } else {
    ys.sort((a, b) => (b.lastModifiedOn ?? '').localeCompare(a.lastModifiedOn ?? ''))
  }
  return ys
}

function NameDialog({
  open,
  title,
  label,
  confirmLabel,
  initial,
  pending,
  error,
  onClose,
  onSubmit,
}: {
  open: boolean
  title: string
  label: string
  confirmLabel: string
  initial?: string
  pending?: boolean
  error?: Error | null
  onClose: () => void
  onSubmit: (name: string) => void
}) {
  const [value, setValue] = useState('')
  // Seed (or re-seed) the field whenever the dialog opens or its target changes.
  useEffect(() => {
    if (open) setValue(initial ?? '')
  }, [open, initial])
  const submit = () => {
    const v = value.trim()
    if (v) onSubmit(v)
  }
  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          fullWidth
          margin="dense"
          label={label}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          error={!!error}
          helperText={error?.message}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={submit} disabled={pending || !value.trim()}>
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

function MoveFolderDialog({
  open,
  folder,
  candidates,
  pending,
  error,
  onClose,
  onSubmit,
}: {
  open: boolean
  folder: Item | null
  candidates: Item[]
  pending?: boolean
  error?: Error | null
  onClose: () => void
  onSubmit: (destinationFolderId?: string) => void
}) {
  // The empty string sentinel represents "project root" (the move endpoint
  // takes an empty destinationFolderId for that). Reset to root on each open.
  const [dest, setDest] = useState<string>('')
  useEffect(() => {
    if (open) setDest('')
  }, [open, folder?.id])

  // Exclude the folder being moved from its own destination choices.
  const choices = candidates.filter((f) => f.id !== folder?.id)

  const submit = () => onSubmit(dest === '' ? undefined : dest)

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Move folder</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 1 }}>
          Move “{folder?.name}” to:
        </DialogContentText>
        <TextField
          select
          autoFocus
          fullWidth
          margin="dense"
          label="Destination"
          value={dest}
          onChange={(e) => setDest(e.target.value)}
          error={!!error}
          helperText={error?.message}
        >
          <MenuItem value="">(project root)</MenuItem>
          {choices.map((f) => (
            <MenuItem key={f.id} value={f.id}>
              {f.name}
            </MenuItem>
          ))}
        </TextField>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={submit} disabled={pending}>
          Move
        </Button>
      </DialogActions>
    </Dialog>
  )
}
