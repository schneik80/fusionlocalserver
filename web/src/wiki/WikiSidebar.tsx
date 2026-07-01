import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faFileLines, faPlus } from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  List,
  ListItemButton,
  Stack,
  TextField,
  Typography,
} from '@mui/material'
import type { DraftStatus } from './draftStore'

// A sidebar entry is either a published page (remote), or a local draft — which
// may itself be linked to a published page (baseItemId) or purely local.
export type WikiEntryStatus = DraftStatus | 'remote'

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

const STATUS_META: Record<WikiEntryStatus, { label: string; color: 'default' | 'warning' | 'success' | 'info' }> = {
  draft: { label: 'Draft', color: 'default' },
  modified: { label: 'Edited', color: 'warning' },
  published: { label: 'Synced', color: 'success' },
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
        width: 240,
        flexShrink: 0,
        borderRight: 1,
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        minHeight: 0,
      }}
    >
      <Stack spacing={1} sx={{ p: 1 }}>
        <Button
          size="small"
          variant="outlined"
          startIcon={<FontAwesomeIcon icon={faPlus} style={{ fontSize: 12 }} />}
          onClick={onNew}
          fullWidth
        >
          New page
        </Button>
        <TextField
          size="small"
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          placeholder="Search pages"
          fullWidth
        />
      </Stack>

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
