import {
  Box,
  CircularProgress,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Paper,
  Stack,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tabs,
  Typography,
} from '@mui/material'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { api } from '../api/client'
import {
  useBOM,
  useClassify,
  useCustomProperties,
  useDescendants,
  useDesignActivity,
  useDrawings,
  useRollupActivity,
  useItemDetails,
  useProperties,
  useThumbnail,
  useUses,
  useWhereUsed,
} from '../api/queries'
import type { ComponentRef, Details, DrawingRef, Item, Measure } from '../api/types'
import { useNav } from '../state/nav'
import { iconForItem } from './icons'
import ActivityHeatmap from './ActivityHeatmap'
import PermissionsExplorer from './PermissionsExplorer'

// The Details metadata is now always shown (in the header, beside the
// thumbnail), so it is no longer a tab. The remaining tabs:
type TabKey =
  | 'history'
  | 'activity'
  | 'properties'
  | 'bom'
  | 'uses'
  | 'whereUsed'
  | 'drawings'
  | 'permissions'

const TAB_LABEL: Record<TabKey, string> = {
  history: 'History',
  activity: 'Activity',
  properties: 'Properties',
  bom: 'BOM',
  uses: 'Uses',
  whereUsed: 'Where Used',
  drawings: 'Drawings',
  permissions: 'Permissions',
}

// Designs get the full set; configured designs add Properties + BOM; drawings
// get Uses (the source design). Permissions (the project's groups + roles)
// applies to any document; everything else is History only.
function tabsFor(kind: string): TabKey[] {
  if (kind === 'design')
    return ['history', 'activity', 'properties', 'bom', 'uses', 'whereUsed', 'drawings', 'permissions']
  if (kind === 'configured') return ['history', 'activity', 'properties', 'bom', 'permissions']
  if (kind === 'drawing') return ['history', 'activity', 'uses', 'permissions']
  return ['history']
}

export function DetailsPanel() {
  const nav = useNav()
  const selected = nav.selected

  return (
    <Paper
      square
      variant="outlined"
      sx={{
        flex: 1,
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
  const [tab, setTab] = useState<TabKey>('history')

  // Reset to a valid tab whenever the selected item (and thus its tab set)
  // changes. key={item.id} already remounts this, but guard anyway.
  useEffect(() => {
    if (!available.includes(tab)) setTab('history')
  }, [available, tab])

  const detailsQ = useItemDetails(hubId, item.id)
  const cvId = item.componentVersionId || detailsQ.data?.rootComponentVersionId
  // Lazy assembly/part classification (cached, shared with the Contents column);
  // used to refine the Type label to "3D Design — Assembly" / "… — Part".
  const classifyQ = useClassify(cvId)
  const subtype = classifyQ.data?.subtype || item.subtype

  return (
    <>
      {/* Header: name, then the always-visible details metadata (left) beside
          the thumbnail (right). The metadata shows regardless of active tab. */}
      <Box sx={{ px: 2, pt: 1.5, pb: 1.5, borderBottom: 1, borderColor: 'divider' }}>
        <Typography variant="h6" noWrap title={item.name} gutterBottom>
          {item.name}
        </Typography>
        <Box sx={{ display: 'flex', gap: 2, alignItems: 'flex-start' }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <DetailsSummary
              query={detailsQ.data}
              kind={item.kind}
              subtype={subtype}
              loading={detailsQ.isLoading}
              error={detailsQ.error as Error | null}
            />
          </Box>
          <Thumbnail cvId={cvId} name={item.name} />
        </Box>
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
        {tab === 'history' && (
          <HistoryTab query={detailsQ.data} loading={detailsQ.isLoading} error={detailsQ.error as Error | null} />
        )}
        {tab === 'activity' && (
          <ActivityTab
            hubId={hubId}
            itemId={item.id}
            cvId={cvId}
            subtype={subtype}
            active={tab === 'activity'}
          />
        )}
        {tab === 'properties' && <PropertiesTab cvId={cvId} active />}
        {tab === 'bom' && <BOMTab cvId={cvId} active />}
        {tab === 'uses' && (
          <UsesTab kind={item.kind} hubId={hubId} itemId={item.id} cvId={cvId} active />
        )}
        {tab === 'whereUsed' && <WhereUsedTab cvId={cvId} active />}
        {tab === 'drawings' && <DrawingsTab hubId={hubId} designItemId={item.id} active />}
        {tab === 'permissions' && <PermissionsExplorer hubId={hubId} item={item} />}
      </Box>
    </>
  )
}

// THUMBNAIL_POLL_TIMEOUT_MS bounds how long we wait on a still-generating
// (PENDING) thumbnail before giving up — APS generates thumbnails on demand and
// some never resolve, so without this a stuck design spins and re-polls APS
// every 2s forever.
const THUMBNAIL_POLL_TIMEOUT_MS = 30_000

// Thumbnail renders the component's preview image, sitting to the right of the
// details metadata. Only designs (and configured designs) carry a
// componentVersionId, so for everything else cvId is undefined and nothing is
// shown (the metadata then takes the full width). Capped at 200×200.
function Thumbnail({ cvId, name }: { cvId?: string; name: string }) {
  // Give up on a perpetually-PENDING thumbnail after a window. Disabling the
  // query via `enabled` also stops its 2s polling.
  const [gaveUp, setGaveUp] = useState(false)
  const q = useThumbnail(cvId, !!cvId && !gaveUp)
  const status = q.data?.status

  useEffect(() => {
    setGaveUp(false)
    if (!cvId || status === 'SUCCESS' || status === 'FAILED') return
    const timer = window.setTimeout(() => setGaveUp(true), THUMBNAIL_POLL_TIMEOUT_MS)
    return () => window.clearTimeout(timer)
  }, [cvId, status])

  if (!cvId) return null
  // No image to show: a hard error, a failed/absent thumbnail, or we gave up
  // waiting for a still-generating one.
  if (q.isError || status === 'FAILED' || gaveUp) return null

  const ready = status === 'SUCCESS'

  return (
    <Box
      sx={{
        flexShrink: 0,
        width: 200,
        maxWidth: 200,
        aspectRatio: '1 / 1',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        bgcolor: 'action.hover',
        borderRadius: 1,
        overflow: 'hidden',
      }}
    >
      {ready ? (
        <Box
          component="img"
          // Same-origin proxy: the server caches the bytes (usually pre-warmed
          // by the classify probe) and streams them, avoiding a cross-origin
          // fetch to the APS CDN on every view.
          src={`/api/items/thumbnail/image?cvId=${encodeURIComponent(cvId)}`}
          alt={`${name} thumbnail`}
          sx={{ width: '100%', height: '100%', objectFit: 'contain' }}
        />
      ) : (
        <CircularProgress size={28} />
      )}
    </Box>
  )
}

function fmtDate(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  return isNaN(d.getTime()) ? s : d.toLocaleString()
}

// typeLabel renders a friendly document type, appending the lazily-classified
// Part/Assembly to designs — e.g. "3D Design — Assembly". Falls back to the raw
// GraphQL typename for kinds we don't have a friendly name for.
function typeLabel(kind: string, typename?: string, subtype?: string): string {
  const base =
    kind === 'design'
      ? '3D Design'
      : kind === 'configured'
        ? 'Configured Design'
        : kind === 'drawing'
          ? 'Drawing'
          : typename || kind || 'Document'
  const sub = subtype === 'assembly' ? 'Assembly' : subtype === 'part' ? 'Part' : ''
  return sub ? `${base} — ${sub}` : base
}

// Fusion-native document kinds. Their "extension" is an internal type marker
// (redundant with the Type row), so the Extension row is hidden for them and
// shown only for uploaded files like PDF, DXF, or PNG.
const FUSION_NATIVE_KINDS = new Set(['design', 'configured', 'drawing', 'schematic', 'pcb', 'ecad'])

// LabelGrid renders a two-column label/value grid, dropping empty rows.
function LabelGrid({ rows }: { rows: Array<[string, ReactNode]> }) {
  const present = rows.filter(([, v]) => v !== undefined && v !== '' && v !== null)
  if (present.length === 0) return null
  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: 'minmax(84px, auto) 1fr',
        columnGap: 2,
        rowGap: 0.75,
      }}
    >
      {present.map(([label, value]) => (
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
  )
}

// DetailsSummary is the always-visible metadata block in the header (formerly
// the Details tab).
function DetailsSummary({
  query,
  kind,
  subtype,
  loading,
  error,
}: {
  query?: Details
  kind: string
  subtype?: string
  loading: boolean
  error: Error | null
}) {
  if (loading) return <TabSpinner />
  if (error) return <TabError error={error} />
  if (!query) return <TabEmpty text="No details" />

  const rows: Array<[string, ReactNode]> = [
    ['Type', typeLabel(kind, query.typename, subtype)],
    ['Part number', query.partNumber],
    ['Description', query.partDesc],
    ['Material', query.material],
    ['Version', query.versionNumber ? `v${query.versionNumber}` : undefined],
    ['Size', query.size],
    ['Extension', FUSION_NATIVE_KINDS.has(kind) ? undefined : query.extensionType],
    ['Created', query.createdOn ? `${fmtDate(query.createdOn)} · ${query.createdBy ?? ''}` : undefined],
    ['Modified', query.modifiedOn ? `${fmtDate(query.modifiedOn)} · ${query.modifiedBy ?? ''}` : undefined],
    ['Milestone', query.isMilestone ? 'Yes' : undefined],
  ]
  return <LabelGrid rows={rows} />
}

// PropertiesTab shows the component version's physical (mass) properties from
// the v2 API. Generation is async, so it polls while computing.
// SectionLabel is a small uppercase heading for the Properties tab sections.
function SectionLabel({ children }: { children: ReactNode }) {
  return (
    <Typography variant="overline" sx={{ display: 'block', color: 'text.secondary', lineHeight: 1.6 }}>
      {children}
    </Typography>
  )
}

// PropertiesTab shows the component's custom/standard named properties (when
// any are available) above its physical/mass properties.
function PropertiesTab({ cvId, active }: { cvId?: string; active: boolean }) {
  const cpQ = useCustomProperties(cvId, active)
  const customRows: Array<[string, ReactNode]> = (cpQ.data ?? []).map((p) => [p.name, p.value])

  return (
    <Stack spacing={2}>
      {customRows.length > 0 && (
        <Box>
          <SectionLabel>Properties</SectionLabel>
          <LabelGrid rows={customRows} />
        </Box>
      )}
      <Box>
        <SectionLabel>Physical</SectionLabel>
        <PhysicalProperties cvId={cvId} active={active} />
      </Box>
    </Stack>
  )
}

function PhysicalProperties({ cvId, active }: { cvId?: string; active: boolean }) {
  const q = useProperties(cvId, active)
  if (q.isLoading) return <TabSpinner />
  if (q.error) return <TabError error={q.error as Error} />
  const p = q.data
  if (!p) return <TabEmpty text="No properties" />

  if (p.status !== 'COMPLETED') {
    return q.isFetching ? (
      <Stack direction="row" spacing={1.5} alignItems="center" sx={{ py: 2 }}>
        <CircularProgress size={18} />
        <Typography variant="body2" color="text.secondary">
          Computing physical properties…
        </Typography>
      </Stack>
    ) : (
      <TabEmpty text="Physical properties not available" />
    )
  }

  // APS returns full-precision floats (e.g. "25.68624402467584"); round to 4
  // significant figures for display, leaving non-numeric values untouched.
  const round = (s: string) => {
    const n = Number(s)
    return Number.isFinite(n) ? String(Number(n.toPrecision(4))) : s
  }
  const fmt = (m: Measure) =>
    m.display ? `${round(m.display)}${m.units ? ` ${m.units}` : ''}` : undefined
  const bbox =
    p.bboxLength.display && p.bboxWidth.display && p.bboxHeight.display
      ? `${round(p.bboxLength.display)} × ${round(p.bboxWidth.display)} × ${round(p.bboxHeight.display)}${
          p.bboxLength.units ? ` ${p.bboxLength.units}` : ''
        }`
      : undefined

  const rows: Array<[string, ReactNode]> = [
    ['Mass', fmt(p.mass)],
    ['Volume', fmt(p.volume)],
    ['Surface area', fmt(p.area)],
    ['Density', fmt(p.density)],
    ['Bounding box', bbox],
  ]
  if (!rows.some(([, v]) => !!v)) return <TabEmpty text="No physical properties" />
  return <LabelGrid rows={rows} />
}

// BOMTab shows a flat bill of materials: one row per unique sub-component with
// a quantity (the occurrence count — the v2 API has no explicit quantity field).
function BOMTab({ cvId, active }: { cvId?: string; active: boolean }) {
  const q = useBOM(cvId, active)
  if (q.isLoading) return <TabSpinner />
  if (q.error) return <TabError error={q.error as Error} />
  const rows = q.data ?? []
  if (rows.length === 0) return <TabEmpty text="No bill of materials" />

  const total = rows.reduce((n, r) => n + r.quantity, 0)
  return (
    <>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
        {rows.length} component{rows.length === 1 ? '' : 's'} · {total} occurrence{total === 1 ? '' : 's'}
      </Typography>
      <Table size="small" sx={{ '& td, & th': { px: 1, py: 0.5 } }}>
        <TableHead>
          <TableRow>
            <TableCell>Component</TableCell>
            <TableCell>Part №</TableCell>
            <TableCell>Material</TableCell>
            <TableCell align="right">Qty</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.map((b) => (
            <TableRow key={b.componentVersionId}>
              <TableCell title={b.partDesc || undefined}>{b.name}</TableCell>
              <TableCell>{b.partNumber || '—'}</TableCell>
              <TableCell>{b.material || '—'}</TableCell>
              <TableCell align="right">{b.quantity}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </>
  )
}

// HistoryTab lists the item's version history (most recent first).
function HistoryTab({
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
  const versions = query?.versions ?? []
  if (versions.length === 0) return <TabEmpty text="No version history" />

  return (
    <List dense disablePadding>
      {versions.map((v) => (
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

// ActivityTab shows the design's change activity as an isometric heat map,
// sourced from the GraphQL-backed design activity report. "Roll up child changes"
// merges every descendant document's activity in (computed server-side).
//
// The descendant walk runs only for assemblies (per the folder-level
// classification), so a leaf part loads the tab without that cost.
function ActivityTab({
  hubId,
  itemId,
  cvId,
  subtype,
  active,
}: {
  hubId: string | null
  itemId: string
  cvId?: string
  subtype?: string
  active: boolean
}) {
  const reportQ = useDesignActivity(active ? hubId : null, active ? itemId : null)

  // Only assemblies have children; gate the (potentially expensive) recursive
  // descendant enumeration on that so parts skip it entirely on tab load.
  const isAssembly = subtype === 'assembly'
  const descendantsQ = useDescendants(cvId, active && !!cvId && isAssembly)
  const childItemIds = useMemo(() => {
    const seen = new Set<string>()
    for (const u of descendantsQ.data ?? []) {
      const id = u.designItemId
      if (id && id !== itemId) seen.add(id)
    }
    return [...seen]
  }, [descendantsQ.data, itemId])
  const childCount = childItemIds.length
  const hasChildren = isAssembly && childCount > 0

  const [rollup, setRollup] = useState(false)

  // Roll-up is computed server-side (enumerate ids here, fan out + merge there)
  // so even a large assembly completes reliably in one request.
  const rollupQ = useRollupActivity(hubId, itemId, childItemIds, rollup && hasChildren)
  const rollupLoading =
    rollup && (descendantsQ.isLoading || rollupQ.isPending || rollupQ.isFetching)

  const report = rollup && !rollupLoading && rollupQ.data ? rollupQ.data : reportQ.data

  if (reportQ.isLoading) return <TabSpinner />
  if (reportQ.error) return <TabError error={reportQ.error as Error} />
  if (!report) return <TabEmpty text="No activity recorded" />
  return (
    <ActivityHeatmap
      report={report}
      childCount={isAssembly ? childCount : undefined}
      rollup={hasChildren ? { checked: rollup, loading: rollupLoading, onChange: setRollup } : undefined}
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
        <NavRow
          key={d.drawingItemId}
          itemId={d.drawingItemId}
          name={d.name}
          kind="drawing"
          secondary={`${fmtDate(d.modifiedOn)}${d.modifiedBy ? ` · ${d.modifiedBy}` : ''}`}
        />
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
        <NavRow
          key={r.id || r.designItemId || r.name}
          itemId={r.designItemId}
          name={r.designItemName || r.name}
          kind="design"
          componentVersionId={r.id}
          secondary={[r.partNumber, r.material].filter(Boolean).join(' · ') || undefined}
        />
      ))}
    </List>
  )
}

// NavRowIcon shows a design's thumbnail (the server-cached image proxy) when a
// componentVersionId is available, falling back to the kind icon on a miss.
function NavRowIcon({ cvId, kind }: { cvId?: string; kind: string }) {
  const [failed, setFailed] = useState(false)
  if (cvId && !failed) {
    return (
      <ListItemIcon sx={{ minWidth: 36 }}>
        <Box
          component="img"
          src={`/api/items/thumbnail/image?cvId=${encodeURIComponent(cvId)}`}
          alt=""
          onError={() => setFailed(true)}
          sx={{ width: 26, height: 26, objectFit: 'contain', borderRadius: 0.5, display: 'block' }}
        />
      </ListItemIcon>
    )
  }
  return (
    <ListItemIcon sx={{ minWidth: 36, color: 'text.secondary' }}>
      <FontAwesomeIcon icon={iconForItem({ id: '', name: '', kind, isContainer: false } as Item)} style={{ fontSize: 16 }} />
    </ListItemIcon>
  )
}

// NavRow is a clickable row for the Uses / Where Used / Drawings tabs. It mirrors
// the Projects/Contents rows — a ListItemButton picks up the themed hover and
// selected highlight — and on click navigates the browser to that document by
// resolving its location (project + folder path), the flow the Pins dialog uses.
function NavRow({
  itemId,
  name,
  kind,
  componentVersionId,
  secondary,
}: {
  itemId?: string
  name: string
  kind: string
  componentVersionId?: string
  secondary?: string
}) {
  const nav = useNav()
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  const canNav = !!itemId && !!nav.hubId
  const selected = !!itemId && nav.selected?.id === itemId

  const goTo = async () => {
    if (!canNav || busy) return
    setBusy(true)
    try {
      const loc = await qc.fetchQuery({
        queryKey: ['location', nav.hubId, itemId],
        queryFn: () => api.itemLocation(nav.hubId!, itemId!),
        staleTime: 5 * 60 * 1000,
      })
      const project: Item = {
        id: loc.projectId,
        name: loc.projectName,
        kind: 'project',
        altId: loc.projectAltId,
        isContainer: true,
      }
      const folderStack: Item[] = loc.folderPath.map((f) => ({
        id: f.id,
        name: f.name,
        kind: 'folder',
        isContainer: true,
      }))
      nav.navigate(project, folderStack, {
        id: itemId!,
        name,
        kind,
        componentVersionId,
        isContainer: false,
      })
    } catch {
      /* couldn't resolve the location — leave the user where they are */
    } finally {
      setBusy(false)
    }
  }

  return (
    <ListItem
      disablePadding
      secondaryAction={busy ? <CircularProgress size={14} sx={{ mr: 1 }} /> : undefined}
    >
      <ListItemButton selected={selected} onClick={goTo} disabled={!canNav} sx={{ py: 0.5 }}>
        <NavRowIcon cvId={componentVersionId} kind={kind} />
        <ListItemText
          primary={name}
          secondary={secondary}
          primaryTypographyProps={{ variant: 'body2', noWrap: true, fontWeight: selected ? 600 : 400 }}
          secondaryTypographyProps={{ variant: 'caption' }}
        />
      </ListItemButton>
    </ListItem>
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
