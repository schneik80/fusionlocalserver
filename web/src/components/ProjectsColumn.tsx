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
  TextField,
  Tooltip,
} from '@mui/material'
import { useEffect, useState } from 'react'
import { useProjectMutations, useProjects } from '../api/queries'
import type { Item } from '../api/types'
import { useNav } from '../state/nav'
import { usePinToggle } from '../state/pins'
import { Column } from './Column'
import { ItemRow } from './ItemRow'

type MenuState = { project: Item; x: number; y: number } | null

export function ProjectsColumn() {
  const nav = useNav()
  const projectsQ = useProjects(nav.hubId)
  const { pinnedIds, toggle } = usePinToggle()
  const mut = useProjectMutations(nav.hubId)

  const [menu, setMenu] = useState<MenuState>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [renaming, setRenaming] = useState<Item | null>(null)
  const [archiving, setArchiving] = useState<Item | null>(null)

  const projects = projectsQ.data ?? []
  const canCreate = !!nav.hubId

  return (
    <>
      <Column
        title="Projects"
        width={280}
        loading={projectsQ.isLoading}
        error={projectsQ.error as Error | null}
        empty={!projectsQ.isLoading && projects.length === 0}
        emptyText={nav.hubId ? 'No projects in this hub' : 'Select a hub to begin'}
        headerAction={
          <Tooltip title="New project">
            <span>
              <IconButton
                size="small"
                disabled={!canCreate}
                onClick={() => setCreateOpen(true)}
                sx={{ color: 'text.secondary' }}
                aria-label="New project"
              >
                <FontAwesomeIcon icon={faPlus} style={{ fontSize: 13 }} />
              </IconButton>
            </span>
          </Tooltip>
        }
      >
        {projects.map((p) => (
          <ItemRow
            key={p.id}
            item={p}
            selected={nav.project?.id === p.id}
            onClick={() => nav.selectProject(p)}
            pinned={pinnedIds.has(p.id)}
            onTogglePin={toggle}
            onContextMenu={(e) => {
              e.preventDefault()
              setMenu({ project: p, x: e.clientX, y: e.clientY })
            }}
          />
        ))}
      </Column>

      {/* Row context menu */}
      <Menu
        open={!!menu}
        onClose={() => setMenu(null)}
        anchorReference="anchorPosition"
        anchorPosition={menu ? { top: menu.y, left: menu.x } : undefined}
      >
        <MenuItem
          onClick={() => {
            setRenaming(menu!.project)
            setMenu(null)
          }}
        >
          Rename…
        </MenuItem>
        <MenuItem
          onClick={() => {
            setArchiving(menu!.project)
            setMenu(null)
          }}
        >
          Archive…
        </MenuItem>
      </Menu>

      {/* Create */}
      <NameDialog
        key={createOpen ? 'create-open' : 'create-closed'}
        open={createOpen}
        title="New project"
        label="Project name"
        confirmLabel="Create"
        pending={mut.create.isPending}
        error={mut.create.error as Error | null}
        onClose={() => setCreateOpen(false)}
        onSubmit={(name) =>
          mut.create.mutate(name, { onSuccess: () => setCreateOpen(false) })
        }
      />

      {/* Rename */}
      <NameDialog
        key={renaming ? `rename-${renaming.id}` : 'rename-closed'}
        open={!!renaming}
        title="Rename project"
        label="Project name"
        confirmLabel="Rename"
        initial={renaming?.name}
        pending={mut.rename.isPending}
        error={mut.rename.error as Error | null}
        onClose={() => setRenaming(null)}
        onSubmit={(name) =>
          mut.rename.mutate(
            { projectId: renaming!.id, name },
            { onSuccess: () => setRenaming(null) },
          )
        }
      />

      {/* Archive confirm */}
      <Dialog open={!!archiving} onClose={() => setArchiving(null)}>
        <DialogTitle>Archive project?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Archive “{archiving?.name}”? It will be hidden from the project list. This
            can be undone from Fusion.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setArchiving(null)}>Cancel</Button>
          <Button
            color="warning"
            disabled={mut.archive.isPending}
            onClick={() =>
              mut.archive.mutate(archiving!.id, { onSuccess: () => setArchiving(null) })
            }
          >
            Archive
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
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
