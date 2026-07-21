import { faPlus } from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import {
  Box,
  Button,
  List,
  ListItemButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { alpha } from '@mui/material/styles'
import { useState } from 'react'
import { useProductionMutations } from '../api/queries'
import type { JobSummary, ProdCaps } from './types'
import { jobDisplayId } from './types'

// JobList is the left rail: the project's jobs plus an inline create form.
// Creation is gated on the caller's write capability (disabled with a reason
// for read-only roles) — the same posture as the tasks New button.
export function JobList({
  projectId,
  hubId,
  projectName,
  jobs,
  caps,
  loading,
  error,
  selectedId,
  onSelect,
}: {
  projectId: string
  hubId: string
  projectName: string
  jobs: JobSummary[]
  caps?: ProdCaps
  loading: boolean
  error: Error | null
  selectedId: string | null
  onSelect: (id: string) => void
}) {
  const { createJob } = useProductionMutations(projectId)
  const [adding, setAdding] = useState(false)
  const [name, setName] = useState('')
  const canWrite = caps?.write ?? false

  const submit = () => {
    const trimmed = name.trim()
    if (!trimmed) return
    createJob.mutate(
      { hubId, projectName, name: trimmed },
      {
        onSuccess: (j) => {
          onSelect(j.id)
          setName('')
          setAdding(false)
        },
      },
    )
  }

  return (
    <Box
      sx={{
        width: 260,
        flexShrink: 0,
        borderRight: 1,
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
      }}
    >
      <Stack
        direction="row"
        alignItems="center"
        sx={{ px: 1, py: 0.75, borderBottom: 1, borderColor: 'divider', flexShrink: 0 }}
      >
        <Typography variant="subtitle2" sx={{ flex: 1, pl: 0.5 }}>
          Jobs
        </Typography>
        <Tooltip
          title={
            canWrite || loading
              ? ''
              : 'Your project role is read-only — creating jobs needs Editor access'
          }
        >
          <span>
            <Button
              size="small"
              variant="contained"
              disabled={!canWrite}
              startIcon={<FontAwesomeIcon icon={faPlus} style={{ fontSize: 11 }} />}
              onClick={() => setAdding((v) => !v)}
              sx={{ py: 0.25, textTransform: 'none' }}
            >
              New
            </Button>
          </span>
        </Tooltip>
      </Stack>

      {adding && (
        <Box sx={{ p: 1, borderBottom: 1, borderColor: 'divider' }}>
          <TextField
            autoFocus
            fullWidth
            size="small"
            placeholder="Job name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') submit()
              if (e.key === 'Escape') {
                setAdding(false)
                setName('')
              }
            }}
          />
          <Stack direction="row" spacing={1} sx={{ mt: 1 }} justifyContent="flex-end">
            <Button
              size="small"
              onClick={() => {
                setAdding(false)
                setName('')
              }}
              sx={{ textTransform: 'none' }}
            >
              Cancel
            </Button>
            <Button
              size="small"
              variant="contained"
              onClick={submit}
              disabled={!name.trim() || createJob.isPending}
              sx={{ textTransform: 'none' }}
            >
              Create
            </Button>
          </Stack>
        </Box>
      )}

      <Box sx={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
        {error ? (
          <Typography variant="caption" color="error" sx={{ p: 2, display: 'block' }}>
            Failed to load jobs.
          </Typography>
        ) : jobs.length === 0 && !loading ? (
          <Typography variant="caption" color="text.secondary" sx={{ p: 2, display: 'block' }}>
            No jobs yet.
          </Typography>
        ) : (
          <List dense disablePadding>
            {jobs.map((j) => (
              <ListItemButton
                key={j.id}
                selected={j.id === selectedId}
                onClick={() => onSelect(j.id)}
                sx={{
                  flexDirection: 'column',
                  alignItems: 'flex-start',
                  gap: 0.25,
                  py: 0.75,
                  transition: 'background-color .1s',
                  '&.Mui-selected': {
                    bgcolor: (t) => alpha(t.palette.primary.main, 0.12),
                  },
                }}
              >
                <Typography variant="body2" fontWeight={600} noWrap sx={{ width: '100%' }}>
                  {j.name}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                  {jobDisplayId(j)} · {j.stepCount} step{j.stepCount === 1 ? '' : 's'} ·{' '}
                  {j.batchCount} batch{j.batchCount === 1 ? '' : 'es'}
                  {j.activeBatchCount > 0 ? ` · ${j.activeBatchCount} in flight` : ''}
                </Typography>
              </ListItemButton>
            ))}
          </List>
        )}
      </Box>
    </Box>
  )
}
