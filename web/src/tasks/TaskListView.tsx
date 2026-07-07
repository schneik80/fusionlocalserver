import { Box, ListItem, ListItemButton, Stack, Typography } from '@mui/material'
import { Column } from '../components/Column'
import { PriorityDot, fmtDue, isOverdue } from './chips'
import { TaskDetails } from './TaskDetails'
import { STATUS_LABEL, taskDisplayId, type Task, type TaskCaps } from './types'

// TaskListView is the project tab's list-with-details view: a Column-shell
// task list beside the shared TaskDetails pane (ProjectsColumn ›
// DetailsPanel idiom).
export function TaskListView({
  tasks,
  caps,
  loading,
  error,
  selectedId,
  onSelect,
}: {
  tasks: Task[]
  caps?: TaskCaps
  loading: boolean
  error: Error | null
  selectedId: string | null
  onSelect: (id: string | null) => void
}) {
  const sorted = [...tasks].sort((a, b) => b.num - a.num) // newest first
  const selected = tasks.find((t) => t.id === selectedId) ?? null

  return (
    <Box sx={{ flex: 1, minHeight: 0, display: 'flex' }}>
      <Column
        title="Tasks"
        width={320}
        loading={loading}
        error={error}
        empty={!loading && tasks.length === 0}
        emptyText="No tasks in this project yet"
      >
        {sorted.map((t) => (
          <TaskRow key={t.id} task={t} selected={t.id === selectedId} onClick={() => onSelect(t.id)} />
        ))}
      </Column>
      <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
        {selected ? (
          <TaskDetails key={selected.id} task={selected} caps={caps} onDeleted={() => onSelect(null)} />
        ) : (
          <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Typography variant="body2" color="text.secondary">
              Select a task to view its details
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  )
}

// TaskRow is the shared dense task row (project list + Tasks screen).
export function TaskRow({
  task,
  selected,
  onClick,
  showProject = false,
}: {
  task: Task
  selected: boolean
  onClick: () => void
  showProject?: boolean
}) {
  const overdue = isOverdue(task.dueDate, task.status)
  const meta = [
    taskDisplayId(task),
    showProject ? task.projectName : STATUS_LABEL[task.status],
    task.assignee?.name || task.assignee?.email,
    task.dueDate ? `due ${fmtDue(task.dueDate)}` : undefined,
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <ListItem disablePadding>
      <ListItemButton selected={selected} onClick={onClick} sx={{ alignItems: 'flex-start', py: 0.75 }}>
        <Box sx={{ pt: 0.75, pr: 1, display: 'flex' }}>
          <PriorityDot priority={task.priority} />
        </Box>
        <Stack sx={{ minWidth: 0, flex: 1 }}>
          <Typography
            variant="body2"
            noWrap
            sx={{
              textDecoration: task.status === 'done' ? 'line-through' : undefined,
              color: task.status === 'done' ? 'text.secondary' : undefined,
            }}
          >
            {task.title}
          </Typography>
          <Typography variant="caption" noWrap color={overdue ? 'error.main' : 'text.secondary'}>
            {meta}
          </Typography>
        </Stack>
      </ListItemButton>
    </ListItem>
  )
}
