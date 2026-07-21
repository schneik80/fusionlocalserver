import {
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  List,
  ListItemButton,
  Radio,
  Stack,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { useJobs } from '../api/queries'
import { encodeBatchRef, encodeJobRef } from '../components/productioncard/prodref'
import type { Job } from './types'
import { jobDisplayId } from './types'

// ProductionRefDialog picks a Job — or a specific Batch within it — from the
// current project and returns the encoded fls:job / fls:batch token to embed.
// Reused by chat, the wiki editor, and task details to link production work.
export function ProductionRefDialog({
  open,
  projectId,
  hubId,
  projectName,
  onClose,
  onPick,
}: {
  open: boolean
  projectId: string | null
  hubId: string
  projectName: string
  onClose: () => void
  // token is the raw fls: token; label is the display name, for callers (the
  // wiki) that need to wrap it in a markdown link. Chat/tasks ignore label.
  onPick: (token: string, label: string) => void
}) {
  const jobsQ = useJobs(projectId, open)
  const [expanded, setExpanded] = useState<string | null>(null)

  const jobs = jobsQ.data?.jobs ?? []

  const pickJob = (j: Job) => {
    onPick(encodeJobRef({ hubId, projectId: projectId!, projectName, jobId: j.id, jobName: j.name }), j.name)
    onClose()
  }
  const pickBatch = (j: Job, batchId: string, batchName: string) => {
    onPick(
      encodeBatchRef({
        hubId,
        projectId: projectId!,
        projectName,
        jobId: j.id,
        jobName: j.name,
        batchId,
        batchName,
      }),
      batchName,
    )
    onClose()
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Link a job or batch</DialogTitle>
      <DialogContent dividers>
        {jobs.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            {jobsQ.isLoading ? 'Loading…' : 'This project has no jobs yet.'}
          </Typography>
        ) : (
          <List dense disablePadding>
            {jobs.map((j) => (
              <Box key={j.id}>
                <ListItemButton onClick={() => pickJob(j)} sx={{ borderRadius: 1 }}>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="body2" fontWeight={600} noWrap>
                      {j.name}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      {jobDisplayId(j)} · {j.batches.length} batch{j.batches.length === 1 ? '' : 'es'} — click to link
                      the job
                    </Typography>
                  </Box>
                  {j.batches.length > 0 && (
                    <Typography
                      variant="caption"
                      color="primary"
                      onClick={(e) => {
                        e.stopPropagation()
                        setExpanded(expanded === j.id ? null : j.id)
                      }}
                      sx={{ cursor: 'pointer', ml: 1, flexShrink: 0 }}
                    >
                      {expanded === j.id ? 'hide batches' : 'or a batch…'}
                    </Typography>
                  )}
                </ListItemButton>
                {expanded === j.id && (
                  <Stack sx={{ pl: 3, py: 0.5 }}>
                    {j.batches.map((b) => (
                      <ListItemButton
                        key={b.id}
                        onClick={() => pickBatch(j, b.id, b.name)}
                        sx={{ borderRadius: 1, py: 0.25 }}
                      >
                        <Radio size="small" checked={false} sx={{ p: 0.5, mr: 0.5 }} />
                        <Typography variant="body2" noWrap>
                          {b.name}
                        </Typography>
                        <Typography variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                          {b.kind} · {new Date(b.runAt).toLocaleDateString()}
                        </Typography>
                      </ListItemButton>
                    ))}
                  </Stack>
                )}
              </Box>
            ))}
          </List>
        )}
      </DialogContent>
    </Dialog>
  )
}
