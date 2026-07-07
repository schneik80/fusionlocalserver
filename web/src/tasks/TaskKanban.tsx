import {
  DndContext,
  PointerSensor,
  closestCorners,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Avatar, Box, Paper, Stack, Tooltip, Typography } from '@mui/material'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTaskMutations } from '../api/queries'
import { PriorityDot, fmtDue, isOverdue } from './chips'
import { TaskViewDialog } from './TaskViewDialog'
import {
  STATUSES,
  STATUS_LABEL,
  taskDisplayId,
  type Task,
  type TaskCaps,
  type TaskList,
  type TaskStatus,
} from './types'

// TaskKanban is the project tab's board view: one column per status, cards
// ordered by rank. Drag between columns changes status; drag within a
// column reorders (both are the same PATCH {status, rank} — a drop
// computes its rank as the midpoint of its new neighbours). The move is
// applied optimistically to the tasks cache so the card doesn't snap back
// while the PATCH is in flight; settle-side invalidation reconciles.
// Clicking a card (PointerSensor's activation distance keeps clicks from
// starting drags) opens the task's details dialog.
export function TaskKanban({
  projectId,
  tasks,
  caps,
  loading,
  error,
}: {
  projectId: string
  tasks: Task[]
  caps?: TaskCaps
  loading: boolean
  error: Error | null
}) {
  const qc = useQueryClient()
  const muts = useTaskMutations(projectId)
  const [openTaskId, setOpenTaskId] = useState<string | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))
  const canWrite = caps ? caps.write : true

  const byStatus = new Map<TaskStatus, Task[]>(STATUSES.map((s) => [s, []]))
  for (const t of tasks) byStatus.get(t.status)?.push(t)
  for (const list of byStatus.values()) list.sort((a, b) => a.rank - b.rank || a.num - b.num)

  function handleDragEnd(ev: DragEndEvent) {
    const { active, over } = ev
    if (!over || active.id === over.id) return
    const task = tasks.find((t) => t.id === active.id)
    if (!task) return

    // The drop target is either a column surface ("col:<status>") or a card.
    let status: TaskStatus
    let overTaskId: string | null = null
    if (String(over.id).startsWith('col:')) {
      status = String(over.id).slice(4) as TaskStatus
    } else {
      const overTask = tasks.find((t) => t.id === over.id)
      if (!overTask) return
      status = overTask.status
      overTaskId = overTask.id
    }

    // Insertion point among the target column's cards (dragged card removed).
    const column = (byStatus.get(status) ?? []).filter((t) => t.id !== task.id)
    let index = column.length // column drop → append
    if (overTaskId) {
      index = column.findIndex((t) => t.id === overTaskId)
      if (index < 0) index = column.length
      // Dragging downward within a column lands *after* the card it's over
      // (the slot the card vacated), matching the sortable preview.
      else if (status === task.status && task.rank < column[index].rank) index += 1
    }
    const before = column[index - 1]?.rank
    const after = column[index]?.rank
    const rank =
      before === undefined && after === undefined
        ? 1024
        : before === undefined
          ? after! - 1024
          : after === undefined
            ? before + 1024
            : (before + after) / 2

    if (status === task.status && rank === task.rank) return

    // Optimistic: move the card in the cache now; the PATCH settles it.
    qc.setQueryData<TaskList>(['tasks', projectId], (cur) =>
      cur
        ? {
            ...cur,
            tasks: cur.tasks.map((t) => (t.id === task.id ? { ...t, status, rank } : t)),
          }
        : cur,
    )
    muts.update.mutate(
      { taskId: task.id, patch: { status, rank } },
      { onError: () => void qc.invalidateQueries({ queryKey: ['tasks', projectId] }) },
    )
  }

  if (error) {
    return (
      <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Typography variant="body2" color="error">
          {error.message}
        </Typography>
      </Box>
    )
  }

  return (
    <>
      <DndContext sensors={sensors} collisionDetection={closestCorners} onDragEnd={handleDragEnd}>
        <Box sx={{ flex: 1, minHeight: 0, display: 'flex', gap: 1, p: 1, overflowX: 'auto' }}>
          {STATUSES.map((status) => (
            <BoardColumn
              key={status}
              status={status}
              tasks={byStatus.get(status) ?? []}
              disabled={!canWrite}
              loading={loading}
              onOpen={setOpenTaskId}
            />
          ))}
        </Box>
      </DndContext>
      {openTaskId && (
        <TaskViewDialog
          open
          projectId={projectId}
          taskId={openTaskId}
          onClose={() => setOpenTaskId(null)}
        />
      )}
    </>
  )
}

function BoardColumn({
  status,
  tasks,
  disabled,
  loading,
  onOpen,
}: {
  status: TaskStatus
  tasks: Task[]
  disabled: boolean
  loading: boolean
  onOpen: (id: string) => void
}) {
  const { setNodeRef, isOver } = useDroppable({ id: `col:${status}` })
  return (
    <Paper
      variant="outlined"
      sx={{
        width: 240,
        minWidth: 200,
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        bgcolor: 'action.hover',
        borderColor: isOver ? 'primary.main' : 'divider',
        transition: 'border-color 120ms',
      }}
    >
      <Stack direction="row" alignItems="center" spacing={0.75} sx={{ px: 1.25, py: 0.75 }}>
        <Typography variant="subtitle2" sx={{ fontSize: 12, textTransform: 'uppercase', letterSpacing: 0.5 }}>
          {STATUS_LABEL[status]}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {loading ? '…' : tasks.length}
        </Typography>
      </Stack>
      <SortableContext items={tasks.map((t) => t.id)} strategy={verticalListSortingStrategy}>
        <Box ref={setNodeRef} sx={{ flex: 1, minHeight: 60, overflowY: 'auto', px: 0.75, pb: 0.75 }}>
          <Stack spacing={0.75}>
            {tasks.map((t) => (
              <BoardCard key={t.id} task={t} disabled={disabled} onOpen={() => onOpen(t.id)} />
            ))}
          </Stack>
        </Box>
      </SortableContext>
    </Paper>
  )
}

function BoardCard({ task, disabled, onOpen }: { task: Task; disabled: boolean; onOpen: () => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: task.id,
    disabled,
  })
  const overdue = isOverdue(task.dueDate, task.status)
  const initials = (task.assignee?.name || task.assignee?.email || '')
    .split(/[\s@]+/)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? '')
    .join('')

  return (
    <Paper
      ref={setNodeRef}
      {...attributes}
      {...listeners}
      onClick={onOpen}
      variant="outlined"
      sx={{
        p: 1,
        cursor: disabled ? 'pointer' : 'grab',
        opacity: isDragging ? 0.4 : 1,
        transform: CSS.Transform.toString(transform),
        transition,
        userSelect: 'none',
        '&:hover': { borderColor: 'primary.main' },
      }}
    >
      <Typography
        variant="body2"
        sx={{
          lineHeight: 1.35,
          display: '-webkit-box',
          WebkitLineClamp: 2,
          WebkitBoxOrient: 'vertical',
          overflow: 'hidden',
          textDecoration: task.status === 'done' ? 'line-through' : undefined,
        }}
      >
        {task.title}
      </Typography>
      <Stack direction="row" alignItems="center" spacing={0.75} sx={{ mt: 0.75 }}>
        <Typography variant="caption" color="text.secondary">
          {taskDisplayId(task)}
        </Typography>
        <PriorityDot priority={task.priority} />
        {task.dueDate && (
          <Typography variant="caption" color={overdue ? 'error.main' : 'text.secondary'}>
            {fmtDue(task.dueDate)}
          </Typography>
        )}
        <Box sx={{ flex: 1 }} />
        {initials && (
          <Tooltip title={task.assignee?.name || task.assignee?.email || ''}>
            <Avatar sx={{ width: 20, height: 20, fontSize: 10 }}>{initials}</Avatar>
          </Tooltip>
        )}
      </Stack>
    </Paper>
  )
}
