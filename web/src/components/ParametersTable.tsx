import { useMemo, useState } from 'react'
import {
  Box,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material'
import type { ModelParameter } from '../api/types'

// ParametersTable renders the design's parameters (from synthesized.parameters)
// as a sortable, filterable table. userName is the Fusion ID handle (e.g.
// "d12"); name is the human label; expression is the display-formatted value
// ("40 mm") while value is the raw internal magnitude.
export function ParametersTable({ parameters }: { parameters: Record<string, ModelParameter> }) {
  const [filter, setFilter] = useState('')

  const rows = useMemo(() => {
    const list = Object.entries(parameters).map(([uuid, p]) => ({ uuid, ...p }))
    list.sort((a, b) => (a.name || a.userName || '').localeCompare(b.name || b.userName || ''))
    const f = filter.trim().toLowerCase()
    if (!f) return list
    return list.filter(
      (p) =>
        (p.name || '').toLowerCase().includes(f) ||
        (p.userName || '').toLowerCase().includes(f) ||
        (p.expression || '').toLowerCase().includes(f),
    )
  }, [parameters, filter])

  if (Object.keys(parameters).length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No parameters
      </Typography>
    )
  }

  return (
    <Box>
      <TextField
        size="small"
        fullWidth
        placeholder="Filter parameters…"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        sx={{ mb: 1 }}
      />
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
        {rows.length} parameter{rows.length === 1 ? '' : 's'}
      </Typography>
      <Table size="small" sx={{ '& td, & th': { px: 1, py: 0.5 } }}>
        <TableHead>
          <TableRow>
            <TableCell>Name</TableCell>
            <TableCell>ID</TableCell>
            <TableCell>Expression</TableCell>
            <TableCell align="right">Value</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.map((p) => (
            <TableRow key={p.uuid}>
              <TableCell title={p.type || undefined}>{p.name || '—'}</TableCell>
              <TableCell sx={{ color: 'text.secondary' }}>{p.userName || '—'}</TableCell>
              <TableCell>{p.expression || '—'}</TableCell>
              <TableCell align="right">
                {typeof p.value === 'number' ? trimNum(p.value) : '—'}
                {p.unit ? ` ${p.unit}` : ''}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Box>
  )
}

// trimNum rounds to a sane precision for display without trailing-float noise.
function trimNum(n: number): string {
  return String(Number(n.toPrecision(6)))
}
