import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faArrowUpRightFromSquare } from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  Button,
  CircularProgress,
  Divider,
  Link,
  List,
  ListItem,
  ListItemText,
  Paper,
  Stack,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from '@mui/material'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import {
  useDrawings,
  useItemDetails,
  useUses,
  useWhereUsed,
} from '../api/queries'
import type { ComponentRef, Details, DrawingRef, Item } from '../api/types'
import { useNav } from '../state/nav'

type TabKey = 'details' | 'uses' | 'whereUsed' | 'drawings'

const TAB_LABEL: Record<TabKey, string> = {
  details: 'Details',
  uses: 'Uses',
  whereUsed: 'Where Used',
  drawings: 'Drawings',
}

// Tab availability mirrors the TUI: designs get all four; drawings get Details
// + Uses (the source design); everything else is Details only.
function tabsFor(kind: string): TabKey[] {
  if (kind === 'design') return ['details', 'uses', 'whereUsed', 'drawings']
  if (kind === 'drawing') return ['details', 'uses']
  return ['details']
}

export function DetailsPanel() {
  const nav = useNav()
  const selected = nav.selected

  return (
    <Paper
      square
      variant="outlined"
      sx={{
        flex: '0 0 35%',
        minWidth: 320,
        display: 'flex',
        flexDirection: 'column',
        borderTop: 0,
        borderBottom: 0,
        borderRight: 0,
        overflow: 'hidden',
      }}
    >
      {selected ? (
        <SelectedDetails key={selected.id} hubId={nav.hubId} item={selected} />
      ) : (
        <Box
          sx={{
            height: '100%',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            p: 3,
          }}
        >
          <Typography variant="body2" color="text.secondary">
            Select a document to view its details
          </Typography>
        </Box>
      )}
    </Paper>
  )
}

function SelectedDetails({ hubId, item }: { hubId: string | null; item: Item }) {
  const available = useMemo(() => tabsFor(item.kind), [item.kind])
  const [tab, setTab] = useState<TabKey>('details')

  // Reset to a valid tab whenever the selected item (and thus its tab set)
  // changes. key={item.id} on this component already remounts it, but guard
  // anyway in case the kind set shrinks.
  useEffect(() => {
    if (!available.includes(tab)) setTab('details')
  }, [available, tab])

  const detailsQ = useItemDetails(hubId, item.id)
  const cvId = item.componentVersionId || detailsQ.data?.rootComponentVersionId

  return (
    <>
      <Box sx={{ px: 2, pt: 1.5, pb: 1, borderBottom: 1, borderColor: 'divider' }}>
        <Typography variant="h6" noWrap title={item.name}>
          {item.name}
        </Typography>
        <StubActions details={detailsQ.data} />
      </Box>

      <Tabs
        value={tab}
        onChange={(_, v: TabKey) => setTab(v)}
        variant="scrollable"
        scrollButtons="auto"
        sx={{ minHeight: 40, borderBottom: 1, borderColor: 'divider', flexShrink: 0 }}
      >
        {available.map((t) => (
          <Tab key={t} value={t} label={TAB_LABEL[t]} sx={{ minHeight: 40, py: 0 }} />
        ))}
      </Tabs>

      <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0, p: 2 }}>
        {tab === 'details' && (
          <DetailsTab query={detailsQ.data} loading={detailsQ.isLoading} error={detailsQ.error as Error | null} />
        )}
        {tab === 'uses' && (
          <UsesTab kind={item.kind} hubId={hubId} itemId={item.id} cvId={cvId} active />
        )}
        {tab === 'whereUsed' && <WhereUsedTab cvId={cvId} active />}
        {tab === 'drawings' && <DrawingsTab hubId={hubId} designItemId={item.id} active />}
      </Box>
    </>
  )
}

// Disabled placeholders for the stubbed Fusion + STEP actions.
function StubActions({ details }: { details?: Details }) {
  return (
    <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 1 }}>
      {details?.fusionWebUrl && (
        <Button
          size="small"
          variant="outlined"
          component={Link}
          href={details.fusionWebUrl}
          target="_blank"
          rel="noopener"
          startIcon={<FontAwesomeIcon icon={faArrowUpRightFromSquare} style={{ fontSize: 12 }} />}
        >
          Open on web
        </Button>
      )}
      <Tooltip title="Coming soon">
        <span>
          <Button size="small" variant="outlined" disabled>
            Open in Fusion
          </Button>
        </span>
      </Tooltip>
      <Tooltip title="Coming soon">
        <span>
          <Button size="small" variant="outlined" disabled>
            Insert
          </Button>
        </span>
      </Tooltip>
      <Tooltip title="Coming soon">
        <span>
          <Button size="small" variant="outlined" disabled>
            Download STEP
          </Button>
        </span>
      </Tooltip>
    </Stack>
  )
}

function fmtDate(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  return isNaN(d.getTime()) ? s : d.toLocaleString()
}

function DetailsTab({
  query,
  loading,
  error,
}: {
  query?: Details
  loading: boolean
  error: Error | null
}) {
  if (loading) return <TabSpinner />
  if (error) return <TabError error={error} />
  if (!query) return <TabEmpty text="No details" />

  const rows: Array<[string, ReactNode]> = [
    ['Type', query.typename],
    ['Part number', query.partNumber],
    ['Description', query.partDesc],
    ['Material', query.material],
    ['Version', query.versionNumber ? `v${query.versionNumber}` : undefined],
    ['Size', query.size],
    ['MIME', query.mimeType],
    ['Extension', query.extensionType],
    ['Created', query.createdOn ? `${fmtDate(query.createdOn)} · ${query.createdBy ?? ''}` : undefined],
    ['Modified', query.modifiedOn ? `${fmtDate(query.modifiedOn)} · ${query.modifiedBy ?? ''}` : undefined],
    ['Milestone', query.isMilestone ? 'Yes' : undefined],
  ]

  return (
    <Box>
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: 'minmax(90px, auto) 1fr',
          columnGap: 2,
          rowGap: 0.75,
        }}
      >
        {rows
          .filter(([, v]) => v !== undefined && v !== '' && v !== null)
          .map(([label, value]) => (
            <Box key={label} sx={{ display: 'contents' }}>
              <Typography variant="caption" color="text.secondary" sx={{ pt: 0.25 }}>
                {label}
              </Typography>
              <Typography variant="body2" sx={{ wordBreak: 'break-word' }}>
                {value}
              </Typography>
            </Box>
          ))}
      </Box>

      {query.versions.length > 0 && (
        <>
          <Divider sx={{ my: 2 }} />
          <Typography variant="subtitle2" gutterBottom>
            Version history
          </Typography>
          <List dense disablePadding>
            {query.versions.map((v) => (
              <ListItem key={v.number} disablePadding sx={{ py: 0.25 }}>
                <ListItemText
                  primary={`v${v.number}${v.comment ? ` — ${v.comment}` : ''}`}
                  secondary={`${fmtDate(v.createdOn)}${v.createdBy ? ` · ${v.createdBy}` : ''}`}
                  primaryTypographyProps={{ variant: 'body2' }}
                  secondaryTypographyProps={{ variant: 'caption' }}
                />
              </ListItem>
            ))}
          </List>
        </>
      )}
    </Box>
  )
}

function UsesTab({
  kind,
  hubId,
  itemId,
  cvId,
  active,
}: {
  kind: string
  hubId: string | null
  itemId: string
  cvId?: string
  active: boolean
}) {
  const isDrawing = kind === 'drawing'
  const q = useUses({
    cvId: isDrawing ? undefined : cvId,
    hubId: isDrawing ? hubId ?? undefined : undefined,
    drawingItemId: isDrawing ? itemId : undefined,
    enabled: active && (isDrawing ? !!hubId : !!cvId),
  })
  return (
    <RefList
      loading={q.isLoading}
      error={q.error as Error | null}
      refs={q.data}
      emptyText={isDrawing ? 'No source design' : 'No sub-components'}
    />
  )
}

function WhereUsedTab({ cvId, active }: { cvId?: string; active: boolean }) {
  const q = useWhereUsed(cvId, active)
  return (
    <RefList
      loading={q.isLoading}
      error={q.error as Error | null}
      refs={q.data}
      emptyText="Not used by any design"
    />
  )
}

function DrawingsTab({
  hubId,
  designItemId,
  active,
}: {
  hubId: string | null
  designItemId: string
  active: boolean
}) {
  const q = useDrawings(hubId, designItemId, active)
  if (q.isLoading) return <TabSpinner />
  if (q.error) return <TabError error={q.error as Error} />
  const drawings = q.data ?? []
  if (drawings.length === 0) return <TabEmpty text="No drawings reference this design" />
  return (
    <List dense disablePadding>
      {drawings.map((d: DrawingRef) => (
        <ListItem key={d.drawingItemId} disablePadding sx={{ py: 0.5 }}>
          <ListItemText
            primary={d.name}
            secondary={`${fmtDate(d.modifiedOn)}${d.modifiedBy ? ` · ${d.modifiedBy}` : ''}`}
            primaryTypographyProps={{ variant: 'body2', noWrap: true }}
            secondaryTypographyProps={{ variant: 'caption' }}
          />
        </ListItem>
      ))}
    </List>
  )
}

function RefList({
  loading,
  error,
  refs,
  emptyText,
}: {
  loading: boolean
  error: Error | null
  refs?: ComponentRef[]
  emptyText: string
}) {
  if (loading) return <TabSpinner />
  if (error) return <TabError error={error} />
  const list = refs ?? []
  if (list.length === 0) return <TabEmpty text={emptyText} />
  return (
    <List dense disablePadding>
      {list.map((r) => (
        <ListItem key={r.id || r.designItemId || r.name} disablePadding sx={{ py: 0.5 }}>
          <ListItemText
            primary={r.designItemName || r.name}
            secondary={[r.partNumber, r.material].filter(Boolean).join(' · ') || undefined}
            primaryTypographyProps={{ variant: 'body2', noWrap: true }}
            secondaryTypographyProps={{ variant: 'caption' }}
          />
        </ListItem>
      ))}
    </List>
  )
}

const TabSpinner = () => (
  <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
    <CircularProgress size={22} />
  </Box>
)

const TabError = ({ error }: { error: Error }) => (
  <Typography variant="body2" color="error">
    {error.message}
  </Typography>
)

const TabEmpty = ({ text }: { text: string }) => (
  <Typography variant="body2" color="text.secondary">
    {text}
  </Typography>
)
