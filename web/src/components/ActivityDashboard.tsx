import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faChevronRight } from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  Breadcrumbs,
  Chip,
  CircularProgress,
  Divider,
  Link,
  Paper,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'
import { useMemo, useState } from 'react'
import {
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as RechartsTooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { useActivityReport, useHubs } from '../api/queries'
import type { ActivityBucket, ActivityChild } from '../api/types'
import { useNav } from '../state/nav'

const ACCENT = '#0696d7'
const BUCKETS: ActivityBucket[] = ['hour', 'day', 'month', 'year']

interface DrillLevel {
  scope: 'hub' | 'project' | 'folder' | 'design'
  id: string
  name: string
}

// childScope maps a report child's type to the scope to drill into.
function childScope(type: string): DrillLevel['scope'] {
  if (type === 'project' || type === 'folder' || type === 'design') return type
  return 'design'
}

export function ActivityDashboard() {
  const nav = useNav()
  const hubsQ = useHubs()
  const hub = hubsQ.data?.find((h) => h.id === nav.hubId)
  const slug = hub?.slug ?? null

  const [bucket, setBucket] = useState<ActivityBucket>('month')
  const [drill, setDrill] = useState<DrillLevel[]>([])

  // Root drill level follows the selected hub.
  const path = useMemo<DrillLevel[]>(
    () => [{ scope: 'hub' as const, id: '', name: nav.hubName ?? 'Hub' }, ...drill],
    [nav.hubName, drill],
  )
  const current = path[path.length - 1]

  const reportQ = useActivityReport(slug, current.scope, current.id, bucket)

  if (!nav.hubId) {
    return <Centered>Select a hub to view its activity.</Centered>
  }
  if (hubsQ.isLoading || (reportQ.isLoading && !reportQ.data)) {
    return (
      <Centered>
        <CircularProgress size={28} />
      </Centered>
    )
  }
  if (!slug) {
    return <Centered>This hub does not expose an activity feed.</Centered>
  }
  if (reportQ.isError) {
    return (
      <Centered>
        <Typography color="error">
          {(reportQ.error as Error)?.message ?? 'Failed to load activity.'}
        </Typography>
      </Centered>
    )
  }
  const rep = reportQ.data
  if (!rep) return null

  const drillInto = (child: ActivityChild) => {
    setDrill((d) => [...d, { scope: childScope(child.type), id: child.id, name: child.name }])
  }
  const gotoPath = (index: number) => {
    // index 0 is the hub root.
    setDrill((d) => d.slice(0, index))
  }

  const chartData = rep.timeline.map((b) => ({
    label: formatBucket(b.start, bucket),
    count: b.count,
  }))

  return (
    <Box sx={{ flex: 1, minHeight: 0, overflow: 'auto', p: 2 }}>
      {/* Scope breadcrumb + bucket selector */}
      <Stack direction="row" alignItems="center" sx={{ mb: 2, gap: 2, flexWrap: 'wrap' }}>
        <Breadcrumbs
          separator={<FontAwesomeIcon icon={faChevronRight} style={{ fontSize: 9 }} />}
          sx={{ flex: 1, minWidth: 200 }}
        >
          {path.map((p, i) =>
            i === path.length - 1 ? (
              <Typography key={i} color="text.primary" sx={{ fontWeight: 600 }}>
                {p.name || '(unnamed)'}
              </Typography>
            ) : (
              <Link
                key={i}
                component="button"
                underline="hover"
                color="inherit"
                onClick={() => gotoPath(i)}
              >
                {p.name || '(unnamed)'}
              </Link>
            ),
          )}
        </Breadcrumbs>
        <ToggleButtonGroup
          size="small"
          exclusive
          value={bucket}
          onChange={(_, v) => v && setBucket(v as ActivityBucket)}
        >
          {BUCKETS.map((b) => (
            <ToggleButton key={b} value={b} sx={{ textTransform: 'capitalize', px: 1.5 }}>
              {b}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
      </Stack>

      {/* Summary stat cards */}
      <Stack direction="row" sx={{ gap: 1.5, flexWrap: 'wrap', mb: 2 }}>
        <Stat label="Activity events" value={rep.totalEvents.toLocaleString()} />
        {rep.scope !== 'design' && <Stat label="Designs" value={rep.designCount.toLocaleString()} />}
        <Stat label="Versions" value={rep.versionCount.toLocaleString()} />
        <Stat label="Contributors" value={rep.contributorCount.toLocaleString()} />
        <Stat label="Created" value={formatDate(rep.createdOn)} />
        <Stat label="Last change" value={formatDate(rep.lastChange)} />
      </Stack>

      {/* Time series */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography variant="subtitle2" sx={{ mb: 1 }}>
          Activity over time ({bucket})
        </Typography>
        {chartData.length === 0 ? (
          <Empty>No activity in range.</Empty>
        ) : (
          <Box sx={{ height: 260 }}>
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={chartData} margin={{ top: 8, right: 8, bottom: 8, left: -16 }}>
                <CartesianGrid strokeDasharray="3 3" strokeOpacity={0.3} />
                <XAxis dataKey="label" tick={{ fontSize: 11 }} interval="preserveStartEnd" />
                <YAxis allowDecimals={false} tick={{ fontSize: 11 }} />
                <RechartsTooltip />
                <Bar dataKey="count" fill={ACCENT} radius={[3, 3, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </Box>
        )}
      </Paper>

      <Stack direction="row" sx={{ gap: 2, flexWrap: 'wrap', alignItems: 'flex-start' }}>
        {/* Contributors */}
        <Paper variant="outlined" sx={{ p: 2, flex: 1, minWidth: 280 }}>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>
            Contributors
          </Typography>
          {rep.contributors.length === 0 ? (
            <Empty>No contributors.</Empty>
          ) : (
            <Table size="small">
              <TableBody>
                {rep.contributors.map((c) => (
                  <TableRow key={c.accountId || c.displayName}>
                    <TableCell sx={{ border: 0 }}>{c.displayName || '(unknown)'}</TableCell>
                    <TableCell align="right" sx={{ border: 0 }}>
                      <Chip size="small" label={c.eventCount} />
                    </TableCell>
                    <TableCell align="right" sx={{ border: 0, color: 'text.secondary', fontSize: 12 }}>
                      {formatDate(c.lastSeen)}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Paper>

        {/* Breakdown (drill-down) */}
        {rep.scope !== 'design' && (
          <Paper variant="outlined" sx={{ p: 2, flex: 1, minWidth: 280 }}>
            <Typography variant="subtitle2" sx={{ mb: 1 }}>
              {breakdownTitle(rep.scope)}
            </Typography>
            {rep.children.length === 0 ? (
              <Empty>No items.</Empty>
            ) : (
              <Table size="small">
                <TableBody>
                  {rep.children.map((c) => (
                    <TableRow
                      key={c.id}
                      hover
                      sx={{ cursor: 'pointer' }}
                      onClick={() => drillInto(c)}
                    >
                      <TableCell sx={{ border: 0 }}>{c.name || '(unnamed)'}</TableCell>
                      <TableCell align="right" sx={{ border: 0 }}>
                        <Chip size="small" label={c.eventCount} />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Paper>
        )}
      </Stack>

      {/* Recent events */}
      <Paper variant="outlined" sx={{ p: 2, mt: 2 }}>
        <Typography variant="subtitle2" sx={{ mb: 1 }}>
          Recent activity
          {rep.eventsTruncated && (
            <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
              (showing latest {rep.events.length} of {rep.totalEvents})
            </Typography>
          )}
        </Typography>
        {rep.events.length === 0 ? (
          <Empty>No recent activity.</Empty>
        ) : (
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>When</TableCell>
                <TableCell>Who</TableCell>
                <TableCell>Action</TableCell>
                <TableCell>Item</TableCell>
                <TableCell align="right">Ver</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {rep.events.slice(0, 100).map((e, i) => (
                <TableRow key={`${e.entityId}-${i}`}>
                  <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatDateTime(e.timestamp)}</TableCell>
                  <TableCell>{e.actor?.displayName || '(unknown)'}</TableCell>
                  <TableCell sx={{ textTransform: 'capitalize' }}>{e.action}</TableCell>
                  <TableCell>{e.detail || e.entityName}</TableCell>
                  <TableCell align="right">{e.versionNumber || ''}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Paper>
    </Box>
  )
}

function breakdownTitle(scope: string): string {
  switch (scope) {
    case 'hub':
      return 'Projects'
    case 'project':
      return 'Folders'
    case 'folder':
      return 'Designs'
    default:
      return 'Breakdown'
  }
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Paper variant="outlined" sx={{ px: 2, py: 1.5, minWidth: 130, flex: '0 1 auto' }}>
      <Typography variant="h6" sx={{ fontWeight: 700, color: ACCENT, lineHeight: 1.2 }}>
        {value || '—'}
      </Typography>
      <Typography variant="caption" color="text.secondary">
        {label}
      </Typography>
    </Paper>
  )
}

function Centered({ children }: { children: React.ReactNode }) {
  return (
    <Box
      sx={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', p: 4, color: 'text.secondary' }}
    >
      {children}
    </Box>
  )
}

function Empty({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Divider sx={{ mb: 1 }} />
      <Typography variant="body2" color="text.secondary">
        {children}
      </Typography>
    </>
  )
}

// --- date formatting ---

function formatBucket(iso: string, bucket: ActivityBucket): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  switch (bucket) {
    case 'hour':
      return d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric' })
    case 'day':
      return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
    case 'month':
      return d.toLocaleDateString(undefined, { month: 'short', year: 'numeric' })
    case 'year':
      return String(d.getUTCFullYear())
    default:
      return iso
  }
}

function formatDate(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '' : d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

function formatDateTime(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return isNaN(d.getTime())
    ? ''
    : d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
}
