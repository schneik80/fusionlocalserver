import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faStar } from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  IconButton,
  ListItem,
  ListItemButton,
  ListItemIcon,
  Tooltip,
  Typography,
} from '@mui/material'
import { useState } from 'react'
import { useClassify } from '../api/queries'
import type { Item } from '../api/types'
import { isPinnable } from '../state/pins'
import { iconForItem, typeTag } from './icons'

interface ItemRowProps {
  item: Item
  selected: boolean
  onClick: () => void
  pinned: boolean
  onTogglePin?: (item: Item) => void
  /** when true, designs without a subtype fetch a classify refinement */
  classifyEnabled?: boolean
  /** right-click handler, e.g. to open a row context menu (Projects) */
  onContextMenu?: (e: React.MouseEvent) => void
}

export function ItemRow({
  item,
  selected,
  onClick,
  pinned,
  onTogglePin,
  classifyEnabled,
  onContextMenu,
}: ItemRowProps) {
  // Refine an unclassified design's icon to assembly/part. The query is
  // disabled (cvId undefined) for every other row.
  const cvId =
    classifyEnabled && item.kind === 'design' && !item.subtype
      ? item.componentVersionId
      : undefined
  const classify = useClassify(cvId)
  const display: Item = {
    ...item,
    subtype: item.subtype || classify.data?.subtype,
  }

  const tag = typeTag(display)
  const showStar = !!onTogglePin && isPinnable(item.kind)

  // Designs carry a componentVersionId, so show their thumbnail in place of the
  // icon (same-origin proxy, server-cached + classify-warmed). On a miss/404
  // (not yet generated, or no thumbnail) fall back to the kind icon.
  const thumbCvId = display.componentVersionId
  const [thumbFailed, setThumbFailed] = useState(false)

  const modified = fmtShortDate(item.lastModifiedOn)

  return (
    <ListItem
      disablePadding
      secondaryAction={
        showStar ? (
          <Tooltip title={pinned ? 'Unpin' : 'Pin'}>
            <IconButton
              edge="end"
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                onTogglePin?.(item)
              }}
              sx={{ color: pinned ? 'primary.main' : 'text.disabled' }}
            >
              <FontAwesomeIcon icon={faStar} style={{ fontSize: 13 }} />
            </IconButton>
          </Tooltip>
        ) : undefined
      }
    >
      <Tooltip
        title={
          <RowTooltipBody
            item={item}
            pinned={pinned}
            thumbCvId={thumbCvId}
            thumbFailed={thumbFailed}
          />
        }
        placement="right"
        enterDelay={500}
        enterNextDelay={200}
        arrow
        componentsProps={{
          tooltip: {
            sx: {
              maxWidth: 280,
              p: 0,
              bgcolor: 'background.paper',
              color: 'text.primary',
              boxShadow: 4,
              border: 1,
              borderColor: 'divider',
            },
          },
          arrow: { sx: { color: 'background.paper' } },
        }}
      >
      <ListItemButton
        selected={selected}
        onClick={onClick}
        onContextMenu={onContextMenu}
        sx={{ pr: showStar ? 5 : 1.5 }}
      >
        <ListItemIcon sx={{ minWidth: 30, color: selected ? 'primary.main' : 'text.secondary' }}>
          {thumbCvId && !thumbFailed ? (
            <Box
              component="img"
              src={`/api/items/thumbnail/image?cvId=${encodeURIComponent(thumbCvId)}`}
              alt=""
              onError={() => setThumbFailed(true)}
              sx={{ width: 22, height: 22, objectFit: 'contain', borderRadius: 0.5, display: 'block' }}
            />
          ) : (
            <FontAwesomeIcon icon={iconForItem(display)} style={{ fontSize: 15 }} />
          )}
        </ListItemIcon>
        <Box
          sx={{
            flex: 1,
            minWidth: 0,
            display: 'flex',
            alignItems: 'baseline',
            gap: 0.75,
          }}
        >
          <Typography
            variant="body2"
            noWrap
            sx={{ minWidth: 0, flexShrink: 1, fontWeight: selected ? 600 : 400 }}
            title={item.name}
          >
            {item.name}
          </Typography>
          {tag && (
            <Typography
              variant="caption"
              sx={{ color: 'text.disabled', flexShrink: 0, fontSize: 10 }}
            >
              · {tag}
            </Typography>
          )}
          {modified && (
            <Typography
              variant="caption"
              color="text.secondary"
              noWrap
              title={item.lastModifiedOn}
              sx={{
                ml: 'auto',
                pl: 1,
                flexShrink: 0,
                fontSize: 10,
              }}
            >
              {modified}
            </Typography>
          )}
        </Box>
      </ListItemButton>
      </Tooltip>
    </ListItem>
  )
}

// RowTooltipBody is the rich hover card for a Contents row: large thumbnail (if
// a design with a componentVersionId), the full untruncated name with a pin
// indicator when pinned, and the full last-modified timestamp.
function RowTooltipBody({
  item,
  pinned,
  thumbCvId,
  thumbFailed,
}: {
  item: Item
  pinned: boolean
  thumbCvId?: string
  thumbFailed: boolean
}) {
  return (
    <Box sx={{ p: 1.25, display: 'flex', flexDirection: 'column', gap: 1, minWidth: 200 }}>
      {thumbCvId && !thumbFailed && (
        <Box
          component="img"
          src={`/api/items/thumbnail/image?cvId=${encodeURIComponent(thumbCvId)}`}
          alt=""
          sx={{
            width: '100%',
            maxHeight: 200,
            objectFit: 'contain',
            borderRadius: 0.5,
            display: 'block',
            bgcolor: 'action.hover',
          }}
        />
      )}
      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75 }}>
        <Typography
          variant="body2"
          sx={{ flex: 1, wordBreak: 'break-word', fontWeight: 500 }}
        >
          {item.name}
        </Typography>
        {pinned && (
          <Box component="span" sx={{ color: 'primary.main', lineHeight: 1, mt: 0.25 }}>
            <FontAwesomeIcon icon={faStar} style={{ fontSize: 12 }} />
          </Box>
        )}
      </Box>
      {item.lastModifiedOn && (
        <Typography variant="caption" color="text.secondary">
          Modified: {fmtLongDate(item.lastModifiedOn)}
        </Typography>
      )}
    </Box>
  )
}

// fmtLongDate is the verbose locale date+time for the row tooltip (the inline
// timestamp uses fmtShortDate; this one shows the full picture on hover).
function fmtLongDate(s?: string): string {
  if (!s) return ''
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return d.toLocaleString()
}

// fmtShortDate renders an RFC3339 timestamp as a compact locale date (no time)
// for the per-row "last modified" column. Mirrors DetailsPanel.fmtDate's
// fallback behaviour; the title attribute preserves the full ISO string.
function fmtShortDate(s?: string): string {
  if (!s) return ''
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return d.toLocaleDateString()
}
