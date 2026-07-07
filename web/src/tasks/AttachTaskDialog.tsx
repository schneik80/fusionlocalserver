import { faMagnifyingGlass } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  InputAdornment,
  List,
  TextField,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { useTasks } from '../api/queries'
import { TaskRow } from './TaskListView'
import type { Task } from './types'

// AttachTaskDialog picks one of the current project's tasks — the task
// sibling of the hub browser's document pick. Chat's composer and the wiki
// editor's toolbar both use it to drop an fls:task token into their drafts.
export function AttachTaskDialog({
  open,
  projectId,
  onClose,
  onPick,
}: {
  open: boolean
  projectId: string | null
  onClose: () => void
  onPick: (task: Task) => void
}) {
  const tasksQ = useTasks(projectId, open)
  const [search, setSearch] = useState('')

  const q = search.trim().toLowerCase()
  const tasks = (tasksQ.data?.tasks ?? [])
    .filter((t) => !q || t.title.toLowerCase().includes(q))
    .sort((a, b) => b.num - a.num)

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>Attach a task</DialogTitle>
      <DialogContent sx={{ p: 0, display: 'flex', flexDirection: 'column', height: 420 }}>
        <Box sx={{ px: 2, pb: 1 }}>
          <TextField
            fullWidth
            size="small"
            autoFocus
            placeholder="Search tasks"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <FontAwesomeIcon icon={faMagnifyingGlass} style={{ fontSize: 13 }} />
                </InputAdornment>
              ),
            }}
          />
        </Box>
        <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto', borderTop: 1, borderColor: 'divider' }}>
          {tasksQ.isLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
              <CircularProgress size={22} />
            </Box>
          ) : tasksQ.error ? (
            <Typography variant="body2" color="error" sx={{ p: 2, textAlign: 'center' }}>
              {(tasksQ.error as Error).message}
            </Typography>
          ) : tasks.length === 0 ? (
            <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
              {q ? 'No tasks match the search' : 'No tasks in this project yet'}
            </Typography>
          ) : (
            <List dense disablePadding>
              {tasks.map((t) => (
                <TaskRow key={t.id} task={t} selected={false} onClick={() => onPick(t)} />
              ))}
            </List>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
      </DialogActions>
    </Dialog>
  )
}
