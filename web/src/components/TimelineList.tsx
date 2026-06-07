import { List, ListItem, ListItemText, Typography, Chip, Box } from '@mui/material'
import type { ModelTimelineEntry } from '../api/types'

// TimelineList renders the design's parametric feature history (from
// synthesized.timeline) as an ordered list, in timeline order. Each entry shows
// its position, a display name, and a humanized feature type.
export function TimelineList({ timeline }: { timeline: ModelTimelineEntry[] }) {
  if (!timeline || timeline.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        No timeline
      </Typography>
    )
  }
  return (
    <>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
        {timeline.length} feature{timeline.length === 1 ? '' : 's'}
      </Typography>
      <List dense disablePadding>
        {timeline.map((t, i) => {
          const label = t.displayName || t.name || `Feature ${t.index ?? i + 1}`
          const num = (typeof t.index === 'number' ? t.index : i) + 1
          return (
            <ListItem key={t.uuid || i} disablePadding sx={{ py: 0.25, gap: 1 }}>
              <Box
                sx={{
                  width: 28,
                  flexShrink: 0,
                  textAlign: 'right',
                  color: 'text.secondary',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                <Typography variant="caption">{num}</Typography>
              </Box>
              <ListItemText
                primary={label}
                primaryTypographyProps={{ variant: 'body2', noWrap: true }}
              />
              {t.type && (
                <Chip
                  label={humanizeFeatureType(t.type)}
                  size="small"
                  variant="outlined"
                  sx={{ height: 20, '& .MuiChip-label': { px: 0.75, fontSize: 11 } }}
                />
              )}
            </ListItem>
          )
        })}
      </List>
    </>
  )
}

// humanizeFeatureType turns a reader feature type (e.g.
// "adsk::fusion::ExtrudeFeature" or "SketchFeature") into a short label
// ("Extrude", "Sketch").
function humanizeFeatureType(type: string): string {
  let s = type
  const colon = s.lastIndexOf('::')
  if (colon >= 0) s = s.slice(colon + 2)
  s = s.replace(/Feature$/, '')
  return s || type
}
