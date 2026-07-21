import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFileLines } from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  Chip,
  CircularProgress,
  List,
  ListItemButton,
  Typography,
} from '@mui/material'
import { APP_RAIL_WIDTH } from '../components/Column'
import { RailHeader } from '../components/RailHeader'
import type { DraftStatus } from './draftStore'

// A sidebar entry is either a published page (remote), or a local draft — which
// may itself be linked to a published page (baseItemId) or purely local.
// 'behind' (remote moved ahead, local clean) and 'conflict' (both changed) are
// reconciled at merge time by comparing the draft's base against the live tip.
export type WikiEntryStatus = DraftStatus | 'remote' | 'behind' | 'conflict'

export interface WikiEntry {
  /** selection id: the draft key, or the page's item lineage urn */
  id: string
  kind: 'draft' | 'page'
  title: string
  status: WikiEntryStatus
  draftKey?: string
  itemId?: string
  modifiedOn?: string
}

const STATUS_META: Record<
  WikiEntryStatus,
  { label: string; color: 'default' | 'warning' | 'success' | 'info' | 'error' }
> = {
  draft: { label: 'Draft', color: 'default' },
  modified: { label: 'Edited', color: 'warning' },
  published: { label: 'Synced', color: 'success' },
  behind: { label: 'Update', color: 'info' },
  conflict: { label: 'Conflict', color: 'error' },
  remote: { label: 'Published', color: 'info' },
}

interface WikiSidebarProps {
  entries: WikiEntry[]
  selectedId: string | null
  onSelect: (entry: WikiEntry) => void
  onNew: () => void
  loading: boolean
  query: string
  onQuery: (q: string) => void
}

export function WikiSidebar({
  entries,
  selectedId,
  onSelect,
  onNew,
  loading,
  query,
  onQuery,
}: WikiSidebarProps) {
  return (
    <Box
      sx={{
        width: APP_RAIL_WIDTH,
        flexShrink: 0,
        borderRight: 1,
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
      }}
    >
      <RailHeader
        title="Pages"
        onNew={onNew}
        search={{ value: query, onChange: onQuery, placeholder: 'Search pages' }}
      />

      <Box sx={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
        {loading ? (
          <Box sx={{ p: 2, textAlign: 'center' }}>
            <CircularProgress size={18} />
          </Box>
        ) : entries.length === 0 ? (
          <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
            {query ? 'No matching pages' : 'No pages yet'}
          </Typography>
        ) : (
          <List dense disablePadding>
            {entries.map((e) => {
              const meta = STATUS_META[e.status]
              return (
                <ListItemButton
                  key={e.id}
                  selected={e.id === selectedId}
                  onClick={() => onSelect(e)}
                  sx={{ gap: 1, py: 0.5, px: 1.25 }}
                >
                  <FontAwesomeIcon
                    icon={faFileLines}
                    style={{ fontSize: 13, width: 16, opacity: 0.6, flexShrink: 0 }}
                  />
                  <Typography variant="body2" noWrap sx={{ flex: 1, minWidth: 0 }} title={e.title}>
                    {e.title}
                  </Typography>
                  <Chip
                    size="small"
                    label={meta.label}
                    color={meta.color}
                    variant="outlined"
                    sx={{ height: 17, fontSize: 9.5, flexShrink: 0 }}
                  />
                </ListItemButton>
              )
            })}
          </List>
        )}
      </Box>
    </Box>
  )
}
