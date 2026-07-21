import {
  faArrowsToDot,
  faMagnifyingGlassMinus,
  faMagnifyingGlassPlus,
  faPlus,
} from '@fortawesome/free-solid-svg-icons'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { Box, IconButton, Tooltip, Typography } from '@mui/material'
import { alpha, useTheme } from '@mui/material/styles'
import { useEffect, useMemo, useRef, useState } from 'react'
import type { useJobGraphMutations } from '../api/queries'
import type { Job, ProdStep } from './types'

// JobCanvas is the interactive flow editor: a pan/zoom SVG canvas (the engine
// is lifted from RelationGraph — view={scale,tx,ty}, wheel zoom-at-cursor,
// drag-pan, fit() via ResizeObserver) with draggable, positioned step nodes
// and hand-drawn cubic-bezier edges. Steps carry their own x/y (persisted), so
// a drag PATCHes the step on release; edges are drawn from a node's out-port to
// a target node. No graph library — inline SVG + MUI sx transitions, matching
// the rest of the app.

const W = 176
const H = 74
const PORT_R = 7
const PAD = 80
const MIN_VIEW_H = 320

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v))

// Horizontal cubic bezier from a right-hand out-port to a left-hand in-port —
// reads as a left-to-right process flow.
function edgePath(sx: number, sy: number, ex: number, ey: number): string {
  const cx = Math.max(40, Math.abs(ex - sx) * 0.5)
  return `M ${sx} ${sy} C ${sx + cx} ${sy} ${ex - cx} ${ey} ${ex} ${ey}`
}

type Graph = ReturnType<typeof useJobGraphMutations>

export function JobCanvas({
  job,
  canWrite,
  graph,
  selectedStepId,
  onSelectStep,
}: {
  job: Job
  canWrite: boolean
  graph: Graph
  selectedStepId: string | null
  onSelectStep: (id: string | null) => void
}) {
  const theme = useTheme()
  const accent = theme.palette.primary.main
  const edgeColor = theme.palette.text.secondary

  const vpRef = useRef<HTMLDivElement>(null)
  const [view, setView] = useState({ scale: 1, tx: 0, ty: 0 })

  // Interaction refs. Exactly one of pan / nodeDrag / edgeDraw is active.
  const pan = useRef<{ x: number; y: number } | null>(null)
  const nodeDrag = useRef<{
    id: string
    startX: number
    startY: number
    origX: number
    origY: number
    scale: number // view scale captured at drag start — see onMouseMove
    moved: boolean
  } | null>(null)
  const hoverNode = useRef<string | null>(null)

  // Render-affecting drag state.
  const [dragPos, setDragPos] = useState<{ id: string; x: number; y: number } | null>(null)
  const [edgeDraw, setEdgeDraw] = useState<{ from: string; lx: number; ly: number } | null>(null)

  // Effective graph-space position of a step (drag override wins).
  const posOf = (st: ProdStep) => (dragPos?.id === st.id ? dragPos : { x: st.x, y: st.y })

  // Bounds over the PERSISTED node positions only — deliberately not the live
  // drag position. Folding dragPos in would shift the layer origin (ox/oy)
  // mid-drag whenever the leftmost/topmost node moves outward, sliding every
  // other node under the cursor. The dragged node may render outside these
  // bounds during the move (the SVG has overflow: visible); bounds catch up
  // when the position PATCH lands.
  const bounds = useMemo(() => {
    if (job.steps.length === 0) return { minX: 0, minY: 0, w: 600, h: 400 }
    let minX = Infinity
    let minY = Infinity
    let maxX = -Infinity
    let maxY = -Infinity
    for (const st of job.steps) {
      minX = Math.min(minX, st.x)
      minY = Math.min(minY, st.y)
      maxX = Math.max(maxX, st.x + W)
      maxY = Math.max(maxY, st.y + H)
    }
    return { minX: minX - PAD, minY: minY - PAD, w: maxX - minX + PAD * 2, h: maxY - minY + PAD * 2 }
  }, [job.steps])
  const ox = -bounds.minX
  const oy = -bounds.minY

  const fit = () => {
    const vp = vpRef.current
    if (!vp) return
    const scale = clamp(Math.min(vp.clientWidth / bounds.w, vp.clientHeight / bounds.h), 0.3, 1.5)
    setView({
      scale,
      tx: (vp.clientWidth - bounds.w * scale) / 2,
      ty: (vp.clientHeight - bounds.h * scale) / 2,
    })
  }
  // Fit once on first mount / when the job changes identity. Subsequent edits
  // must not yank the viewport, so this deliberately depends only on job.id.
  useEffect(() => {
    fit()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [job.id])

  // Re-fit when the viewport resizes (panel drag, tab show, window resize).
  const fitRef = useRef(fit)
  fitRef.current = fit
  useEffect(() => {
    const vp = vpRef.current
    if (!vp || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => fitRef.current())
    ro.observe(vp)
    return () => ro.disconnect()
  }, [])

  const zoomAt = (factor: number, cx: number, cy: number) =>
    setView((v) => {
      const scale = clamp(v.scale * factor, 0.3, 2)
      const k = scale / v.scale
      return { scale, tx: cx - (cx - v.tx) * k, ty: cy - (cy - v.ty) * k }
    })

  const onWheel = (e: React.WheelEvent) => {
    // No zooming while a node drag or edge draw is active: the drag math and
    // the dashed preview are anchored in the current view.
    if (nodeDrag.current || edgeDraw) return
    const rect = vpRef.current?.getBoundingClientRect()
    if (!rect) return
    zoomAt(e.deltaY < 0 ? 1.12 : 0.89, e.clientX - rect.left, e.clientY - rect.top)
  }

  // Screen → layer coordinates (the layer is translated by tx/ty then scaled).
  const toLayer = (clientX: number, clientY: number) => {
    const rect = vpRef.current!.getBoundingClientRect()
    return {
      lx: (clientX - rect.left - view.tx) / view.scale,
      ly: (clientY - rect.top - view.ty) / view.scale,
    }
  }

  const onMouseMove = (e: React.MouseEvent) => {
    if (nodeDrag.current) {
      const d = nodeDrag.current
      // Divide by the scale captured at drag START: the total-delta math must
      // never be rescaled retroactively by a zoom change mid-drag.
      const dx = (e.clientX - d.startX) / d.scale
      const dy = (e.clientY - d.startY) / d.scale
      if (Math.abs(dx) > 2 || Math.abs(dy) > 2) d.moved = true
      setDragPos({ id: d.id, x: d.origX + dx, y: d.origY + dy })
      return
    }
    if (edgeDraw) {
      const { lx, ly } = toLayer(e.clientX, e.clientY)
      setEdgeDraw((s) => (s ? { ...s, lx, ly } : s))
      return
    }
    if (pan.current) {
      const dx = e.clientX - pan.current.x
      const dy = e.clientY - pan.current.y
      pan.current = { x: e.clientX, y: e.clientY }
      setView((v) => ({ ...v, tx: v.tx + dx, ty: v.ty + dy }))
    }
  }

  const endInteractions = () => {
    // Persist a node move on release (only if it actually moved).
    if (nodeDrag.current && dragPos && nodeDrag.current.moved) {
      const { id } = nodeDrag.current
      const { x, y } = dragPos
      graph.updateStep.mutate(
        { stepId: id, patch: { x, y } },
        { onSettled: () => setDragPos((p) => (p?.id === id ? null : p)) },
      )
    } else {
      setDragPos(null)
    }
    // Complete an edge draw if released over a different node.
    if (edgeDraw) {
      const target = hoverNode.current
      if (target && target !== edgeDraw.from) {
        graph.addEdge.mutate({ from: edgeDraw.from, to: target })
      }
      setEdgeDraw(null)
    }
    nodeDrag.current = null
    pan.current = null
  }

  const addStepAtCenter = () => {
    const vp = vpRef.current
    if (!vp) return
    // View center in graph coords, offset so the node lands centered.
    const gx = (vp.clientWidth / 2 - view.tx) / view.scale - ox - W / 2
    const gy = (vp.clientHeight / 2 - view.ty) / view.scale - oy - H / 2
    graph.addStep.mutate(
      { title: `Step ${job.steps.length + 1}`, x: gx, y: gy },
      { onSuccess: (j) => onSelectStep(j.steps[j.steps.length - 1]?.id ?? null) },
    )
  }

  const edges = job.edges
    .map((e) => {
      const from = job.steps.find((s) => s.id === e.from)
      const to = job.steps.find((s) => s.id === e.to)
      if (!from || !to) return null
      const fp = posOf(from)
      const tp = posOf(to)
      return {
        id: e.id,
        d: edgePath(fp.x + W + ox, fp.y + H / 2 + oy, tp.x + ox, tp.y + H / 2 + oy),
      }
    })
    .filter(Boolean) as { id: string; d: string }[]

  return (
    <Box
      ref={vpRef}
      onWheel={onWheel}
      onMouseDown={(e) => {
        // Empty-canvas press starts a pan and clears the selection.
        pan.current = { x: e.clientX, y: e.clientY }
        onSelectStep(null)
      }}
      onMouseMove={onMouseMove}
      onMouseUp={endInteractions}
      onMouseLeave={endInteractions}
      sx={{
        position: 'relative',
        flex: 1,
        minHeight: MIN_VIEW_H,
        overflow: 'hidden',
        bgcolor: alpha(theme.palette.text.primary, theme.palette.mode === 'dark' ? 0.03 : 0.02),
        cursor: pan.current ? 'grabbing' : 'grab',
        userSelect: 'none',
      }}
    >
      <Box
        sx={{
          position: 'absolute',
          top: 0,
          left: 0,
          width: bounds.w,
          height: bounds.h,
          transformOrigin: '0 0',
          transform: `translate(${view.tx}px, ${view.ty}px) scale(${view.scale})`,
        }}
      >
        <svg
          width={bounds.w}
          height={bounds.h}
          style={{ position: 'absolute', inset: 0, overflow: 'visible' }}
        >
          {edges.map((e) => (
            <path key={e.id} d={e.d} fill="none" stroke={edgeColor} strokeOpacity={0.55} strokeWidth={1.75} />
          ))}
          {edgeDraw &&
            (() => {
              const from = job.steps.find((s) => s.id === edgeDraw.from)
              if (!from) return null
              const p = posOf(from)
              return (
                <path
                  d={edgePath(p.x + W + ox, p.y + H / 2 + oy, edgeDraw.lx, edgeDraw.ly)}
                  fill="none"
                  stroke={accent}
                  strokeWidth={2}
                  strokeDasharray="5 4"
                />
              )
            })()}
        </svg>

        {job.steps.map((st) => {
          const p = posOf(st)
          return (
            <StepNode
              key={st.id}
              step={st}
              left={p.x + ox}
              top={p.y + oy}
              accent={accent}
              selected={st.id === selectedStepId}
              canWrite={canWrite}
              onBodyDown={(e) => {
                e.stopPropagation()
                if (!canWrite) {
                  onSelectStep(st.id)
                  return
                }
                nodeDrag.current = {
                  id: st.id,
                  startX: e.clientX,
                  startY: e.clientY,
                  origX: p.x,
                  origY: p.y,
                  scale: view.scale,
                  moved: false,
                }
              }}
              onBodyUp={() => {
                // A press that didn't move is a select/open.
                if (nodeDrag.current && !nodeDrag.current.moved) onSelectStep(st.id)
              }}
              onPortDown={(e) => {
                e.stopPropagation()
                const { lx, ly } = toLayer(e.clientX, e.clientY)
                setEdgeDraw({ from: st.id, lx, ly })
              }}
              onEnter={() => (hoverNode.current = st.id)}
              onLeave={() => {
                if (hoverNode.current === st.id) hoverNode.current = null
              }}
            />
          )
        })}
      </Box>

      {/* empty state */}
      {job.steps.length === 0 && (
        <Box
          sx={{
            position: 'absolute',
            inset: 0,
            display: 'grid',
            placeItems: 'center',
            color: 'text.secondary',
            fontSize: 13,
            pointerEvents: 'none',
            textAlign: 'center',
            px: 3,
          }}
        >
          {canWrite ? 'Add the first step to start building this job’s flow.' : 'This job has no steps yet.'}
        </Box>
      )}

      {/* toolbar */}
      <Box sx={{ position: 'absolute', top: 8, right: 8, display: 'flex', gap: 0.5, alignItems: 'center' }}>
        {canWrite && (
          <Tooltip title="Add step">
            <IconButton
              size="small"
              onMouseDown={(e) => e.stopPropagation()}
              onClick={addStepAtCenter}
              sx={{ bgcolor: 'primary.main', color: 'primary.contrastText', '&:hover': { bgcolor: 'primary.dark' } }}
            >
              <FontAwesomeIcon icon={faPlus} style={{ fontSize: 12 }} />
            </IconButton>
          </Tooltip>
        )}
        <ToolBtn label="Zoom out" icon={faMagnifyingGlassMinus} onClick={() => zoomAt(0.83, (vpRef.current?.clientWidth ?? 0) / 2, (vpRef.current?.clientHeight ?? 0) / 2)} />
        <Typography variant="caption" sx={{ minWidth: 34, textAlign: 'center', fontVariantNumeric: 'tabular-nums', color: 'text.secondary' }}>
          {Math.round(view.scale * 100)}%
        </Typography>
        <ToolBtn label="Zoom in" icon={faMagnifyingGlassPlus} onClick={() => zoomAt(1.2, (vpRef.current?.clientWidth ?? 0) / 2, (vpRef.current?.clientHeight ?? 0) / 2)} />
        <ToolBtn label="Fit to view" icon={faArrowsToDot} onClick={fit} />
      </Box>
    </Box>
  )
}

function ToolBtn({ label, icon, onClick }: { label: string; icon: typeof faArrowsToDot; onClick: () => void }) {
  return (
    <Tooltip title={label}>
      <IconButton
        size="small"
        onMouseDown={(e) => e.stopPropagation()}
        onClick={onClick}
        sx={{ bgcolor: 'background.paper', border: 1, borderColor: 'divider', '&:hover': { bgcolor: 'background.paper' } }}
      >
        <FontAwesomeIcon icon={icon} style={{ fontSize: 12 }} />
      </IconButton>
    </Tooltip>
  )
}

function StepNode({
  step,
  left,
  top,
  accent,
  selected,
  canWrite,
  onBodyDown,
  onBodyUp,
  onPortDown,
  onEnter,
  onLeave,
}: {
  step: ProdStep
  left: number
  top: number
  accent: string
  selected: boolean
  canWrite: boolean
  onBodyDown: (e: React.MouseEvent) => void
  onBodyUp: (e: React.MouseEvent) => void
  onPortDown: (e: React.MouseEvent) => void
  onEnter: () => void
  onLeave: () => void
}) {
  const [hovered, setHovered] = useState(false)
  const docCount = step.planDocs.length
  const slotCount = step.placeholders.length

  return (
    <Box
      onMouseEnter={() => {
        setHovered(true)
        onEnter()
      }}
      onMouseLeave={() => {
        setHovered(false)
        onLeave()
      }}
      onMouseDown={onBodyDown}
      onMouseUp={onBodyUp}
      sx={{
        position: 'absolute',
        left,
        top,
        width: W,
        height: H,
        p: 1,
        border: selected ? 2 : 1,
        borderRadius: 1.5,
        borderColor: selected ? accent : hovered ? alpha(accent, 0.6) : 'divider',
        bgcolor: 'background.paper',
        boxShadow: hovered || selected ? 3 : 0,
        cursor: canWrite ? 'grab' : 'pointer',
        transition: 'border-color .1s, box-shadow .1s',
        display: 'flex',
        flexDirection: 'column',
        gap: 0.5,
        overflow: 'hidden',
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75, minWidth: 0 }}>
        <Box
          sx={{
            width: 20,
            height: 20,
            borderRadius: '50%',
            flexShrink: 0,
            display: 'grid',
            placeItems: 'center',
            fontSize: 10,
            fontWeight: 700,
            color: 'primary.contrastText',
            bgcolor: 'primary.main',
          }}
        >
          {step.num}
        </Box>
        <Typography variant="body2" fontWeight={600} noWrap sx={{ flex: 1, minWidth: 0 }} title={step.title}>
          {step.title}
        </Typography>
      </Box>
      <Typography variant="caption" color="text.secondary" sx={{ fontSize: 10.5 }}>
        {docCount} doc{docCount === 1 ? '' : 's'} · {slotCount} slot{slotCount === 1 ? '' : 's'}
      </Typography>

      {/* out-port: drag to another node to connect */}
      {canWrite && (
        <Tooltip title="Drag to connect" placement="right">
          <Box
            onMouseDown={onPortDown}
            sx={{
              position: 'absolute',
              right: -PORT_R,
              top: H / 2 - PORT_R,
              width: PORT_R * 2,
              height: PORT_R * 2,
              borderRadius: '50%',
              bgcolor: accent,
              border: 2,
              borderColor: 'background.paper',
              cursor: 'crosshair',
              opacity: hovered ? 1 : 0.35,
              transition: 'opacity .1s',
            }}
          />
        </Tooltip>
      )}
    </Box>
  )
}
