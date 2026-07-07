import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  TextField,
} from '@mui/material'
import { useEffect, useState } from 'react'
import { useChatMembers, useTaskMutations } from '../api/queries'
import {
  PRIORITIES,
  PRIORITY_LABEL,
  STATUSES,
  STATUS_LABEL,
  taskDisplayId,
  type Task,
  type TaskPriority,
  type TaskStatus,
  type TaskUser,
} from './types'

// TaskEditDialog is the full create/edit form. The assignee picker feeds
// off the same project roster chat uses (useChatMembers) — no parallel
// membership source. Create mode needs hubId/projectName because the
// project's task file self-describes for cross-project listings.
export function TaskEditDialog({
  open,
  onClose,
  projectId,
  hubId,
  projectName,
  task,
  onSaved,
}: {
  open: boolean
  onClose: () => void
  projectId: string
  hubId: string
  projectName: string
  task?: Task // edit mode when present
  onSaved?: (t: Task) => void
}) {
  const muts = useTaskMutations(projectId)
  const membersQ = useChatMembers(projectId, open)

  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [status, setStatus] = useState<TaskStatus>('todo')
  const [priority, setPriority] = useState<TaskPriority>('medium')
  const [dueDate, setDueDate] = useState('')
  const [assigneeId, setAssigneeId] = useState('')

  // Re-seed the form each time the dialog opens (it stays mounted across
  // opens in some hosts).
  useEffect(() => {
    if (!open) return
    setTitle(task?.title ?? '')
    setDescription(task?.description ?? '')
    setStatus(task?.status ?? 'todo')
    setPriority(task?.priority ?? 'medium')
    setDueDate(task?.dueDate ?? '')
    setAssigneeId(task?.assignee?.id ?? '')
    muts.create.reset()
    muts.update.reset()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, task?.id])

  const members = membersQ.data ?? []
  // Keep a group-derived assignee (not on the individual roster) pickable.
  const extraAssignee =
    task?.assignee && !members.some((m) => m.userId === task.assignee!.id) ? task.assignee : null

  const pending = muts.create.isPending || muts.update.isPending
  const err = (task ? muts.update.error : muts.create.error) as Error | null

  function resolveAssignee(): TaskUser | undefined {
    if (!assigneeId) return undefined
    const m = members.find((mm) => mm.userId === assigneeId)
    if (m) return { id: m.userId, name: m.name, email: m.email }
    if (extraAssignee && extraAssignee.id === assigneeId) return extraAssignee
    return { id: assigneeId }
  }

  function save() {
    const assignee = resolveAssignee()
    const done = (t: Task) => {
      onClose()
      onSaved?.(t)
    }
    if (task) {
      muts.update.mutate(
        {
          taskId: task.id,
          patch: {
            title,
            description,
            status,
            priority,
            ...(dueDate ? { dueDate } : { clearDueDate: true }),
            ...(assignee ? { assignee } : { clearAssignee: true }),
          },
        },
        { onSuccess: done },
      )
    } else {
      muts.create.mutate(
        { hubId, projectName, title, description, status, priority, dueDate, assignee },
        { onSuccess: done },
      )
    }
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{task ? `Edit ${taskDisplayId(task)}` : 'New task'}</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 0.5 }}>
          {err && <Alert severity="error">{err.message}</Alert>}
          <TextField
            label="Title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            autoFocus
            fullWidth
            size="small"
            inputProps={{ maxLength: 200 }}
          />
          <TextField
            label="Description (markdown)"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            fullWidth
            size="small"
            multiline
            minRows={3}
            maxRows={10}
          />
          <Stack direction="row" spacing={2}>
            <TextField
              select
              label="Status"
              value={status}
              onChange={(e) => setStatus(e.target.value as TaskStatus)}
              size="small"
              sx={{ flex: 1 }}
            >
              {STATUSES.map((s) => (
                <MenuItem key={s} value={s}>
                  {STATUS_LABEL[s]}
                </MenuItem>
              ))}
            </TextField>
            <TextField
              select
              label="Priority"
              value={priority}
              onChange={(e) => setPriority(e.target.value as TaskPriority)}
              size="small"
              sx={{ flex: 1 }}
            >
              {PRIORITIES.map((p) => (
                <MenuItem key={p} value={p}>
                  {PRIORITY_LABEL[p]}
                </MenuItem>
              ))}
            </TextField>
          </Stack>
          <Stack direction="row" spacing={2}>
            <TextField
              label="Due date"
              type="date"
              value={dueDate}
              onChange={(e) => setDueDate(e.target.value)}
              size="small"
              sx={{ flex: 1 }}
              InputLabelProps={{ shrink: true }}
            />
            <TextField
              select
              label="Assignee"
              value={assigneeId}
              onChange={(e) => setAssigneeId(e.target.value)}
              size="small"
              sx={{ flex: 1 }}
              disabled={membersQ.isLoading}
            >
              <MenuItem value="">Unassigned</MenuItem>
              {extraAssignee && (
                <MenuItem value={extraAssignee.id}>
                  {extraAssignee.name || extraAssignee.email || extraAssignee.id}
                </MenuItem>
              )}
              {members.map((m) => (
                <MenuItem key={m.userId} value={m.userId}>
                  {m.name || m.email || m.userId}
                </MenuItem>
              ))}
            </TextField>
          </Stack>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={pending}>
          Cancel
        </Button>
        <Button variant="contained" onClick={save} disabled={pending || !title.trim()}>
          {task ? 'Save' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
