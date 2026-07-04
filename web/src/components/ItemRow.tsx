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
import { thumbnailSrc } from '../api/thumbnails'
import type { Item } from '../api/types'
import { useNav } from '../state/nav'
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
}

export function ItemRow({
  item,
  selected,
  onClick,
  pinned,
  onTogglePin,
  classifyEnabled,
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

  // Show the document's preview in place of the icon: designs via their MFGDM
  // thumbnail, drawings via the Model Derivative preview (keyed by item id + the
  // current project's altId). On a miss/404 fall back to the kind icon.
  const nav = useNav()
  const thumbSrc = thumbnailSrc({
    kind: display.kind,
    cvId: display.componentVersionId,
    itemId: item.id,
    projectAltId: nav.project?.altId,
  })
  const [thumbFailed, setThumbFailed] = useState(false)

  return (
    <ListItem
      disablePadding
      // The pin star stays quiet until wanted: hidden on unpinned rows,
      // revealed on hover or keyboard focus, and kept on permanently once the
      // item is pinned (removing the pin returns it to hover-only). The
      // right-padding is reserved either way (see ListItemButton pr) so the
      // star fading in never reflows the name.
      sx={{
        '& .pin-star': { opacity: pinned ? 1 : 0, transition: 'opacity 120ms' },
        '&:hover .pin-star, &:focus-within .pin-star': { opacity: 1 },
      }}
      secondaryAction={
        showStar ? (
          <Tooltip title={pinned ? 'Unpin' : 'Pin'}>
            <IconButton
              className="pin-star"
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
      <ListItemButton selected={selected} onClick={onClick} sx={{ pr: showStar ? 5 : 1.5 }}>
        <ListItemIcon sx={{ minWidth: 30, color: selected ? 'primary.main' : 'text.secondary' }}>
          {thumbSrc && !thumbFailed ? (
            <Box
              component="img"
              src={thumbSrc}
              alt=""
              onError={() => setThumbFailed(true)}
              sx={{ width: 22, height: 22, objectFit: 'contain', borderRadius: 0.5, display: 'block' }}
            />
          ) : (
            <FontAwesomeIcon icon={iconForItem(display)} style={{ fontSize: 15 }} />
          )}
        </ListItemIcon>
        <Box sx={{ minWidth: 0, display: 'flex', alignItems: 'baseline', gap: 0.75 }}>
          <Typography
            variant="body2"
            noWrap
            sx={{ minWidth: 0, fontWeight: selected ? 600 : 400 }}
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
        </Box>
      </ListItemButton>
    </ListItem>
  )
}
