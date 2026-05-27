import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faMagnifyingGlass } from '@fortawesome/free-solid-svg-icons'
import {
  Box,
  CircularProgress,
  Dialog,
  InputAdornment,
  List,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  MenuItem,
  Stack,
  TextField,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'
import { useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api } from '../api/client'
import { useSearch, useSearchableProperties } from '../api/queries'
import type { Item, SearchHit } from '../api/types'
import { useNav } from '../state/nav'
import { iconForItem } from './icons'

type Mode = 'text' | 'property'

interface SubmittedQuery {
  q?: string
  propId?: string
  propValue?: string
}

// SearchDialog is a hub-scoped search lightbox: a free-text OR property search
// (with a property picker), results showing thumbnail + name (+ the matched
// property for property search), and a row click that does Show-in-Location.
export function SearchDialog({
  open,
  onClose,
  hubId,
  hubName,
}: {
  open: boolean
  onClose: () => void
  hubId: string | null
  hubName: string | null
}) {
  const [mode, setMode] = useState<Mode>('text')
  const [text, setText] = useState('')
  const [propId, setPropId] = useState('')
  const [propValue, setPropValue] = useState('')
  const [submitted, setSubmitted] = useState<SubmittedQuery | null>(null)

  const propsQ = useSearchableProperties(mode === 'property' ? hubId : null)
  const searchQ = useSearch({
    hubId,
    q: submitted?.q,
    propId: submitted?.propId,
    propValue: submitted?.propValue,
    enabled: !!submitted,
  })

  // Reset everything when the dialog closes so it opens fresh next time.
  useEffect(() => {
    if (!open) {
      setMode('text')
      setText('')
      setPropId('')
      setPropValue('')
      setSubmitted(null)
    }
  }, [open])

  const canSubmit =
    mode === 'text' ? text.trim() !== '' : propId !== '' && propValue.trim() !== ''

  const submit = () => {
    if (!canSubmit) return
    if (mode === 'text') setSubmitted({ q: text.trim() })
    else setSubmitted({ propId, propValue: propValue.trim() })
  }

  const propName = propsQ.data?.find((p) => p.id === propId)?.displayName

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth={false}
      PaperProps={{ sx: { width: '80vw', height: '80vh', maxWidth: 'none', m: 0 } }}
    >
      <Stack sx={{ height: '100%', p: 2.5, gap: 2 }}>
        <Typography variant="h6">
          Search{hubName ? ` · ${hubName}` : ''}
        </Typography>

        {/* Search form */}
        <Stack direction="row" spacing={1.5} alignItems="center">
          <ToggleButtonGroup
            size="small"
            exclusive
            value={mode}
            onChange={(_, m) => {
              if (m) {
                setMode(m as Mode)
                setSubmitted(null)
              }
            }}
          >
            <ToggleButton value="text" sx={{ textTransform: 'none', px: 2 }}>
              Free text
            </ToggleButton>
            <ToggleButton value="property" sx={{ textTransform: 'none', px: 2 }}>
              Property
            </ToggleButton>
          </ToggleButtonGroup>

          {mode === 'text' ? (
            <TextField
              autoFocus
              fullWidth
              size="small"
              placeholder="Search this hub…"
              value={text}
              onChange={(e) => setText(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && submit()}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <FontAwesomeIcon icon={faMagnifyingGlass} style={{ fontSize: 14 }} />
                  </InputAdornment>
                ),
              }}
            />
          ) : (
            <>
              <TextField
                select
                size="small"
                label="Property"
                value={propId}
                onChange={(e) => setPropId(e.target.value)}
                sx={{ minWidth: 220 }}
                disabled={propsQ.isLoading}
                helperText={propsQ.error ? 'Could not load properties' : undefined}
              >
                {(propsQ.data ?? []).map((p) => (
                  <MenuItem key={p.id} value={p.id}>
                    {p.displayName}
                  </MenuItem>
                ))}
              </TextField>
              <TextField
                fullWidth
                size="small"
                placeholder="Value…"
                value={propValue}
                onChange={(e) => setPropValue(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && submit()}
              />
            </>
          )}
        </Stack>

        {/* Results */}
        <Box sx={{ flex: 1, overflow: 'auto', borderTop: 1, borderColor: 'divider' }}>
          <Results
            query={searchQ}
            showMatched={mode === 'property'}
            propName={propName}
            onPicked={onClose}
          />
        </Box>
      </Stack>
    </Dialog>
  )
}

function Results({
  query,
  showMatched,
  propName,
  onPicked,
}: {
  query: ReturnType<typeof useSearch>
  showMatched: boolean
  propName?: string
  onPicked: () => void
}) {
  if (!query.isFetched && !query.isLoading) {
    return <Centered text="Enter a query above to search." />
  }
  if (query.isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
        <CircularProgress size={24} />
      </Box>
    )
  }
  if (query.error) return <Centered text={(query.error as Error).message} />
  const hits = query.data?.hits ?? []
  if (hits.length === 0) return <Centered text="No results." />

  return (
    <List dense disablePadding>
      {hits.map((h, i) => (
        <SearchRow
          key={`${h.itemId ?? h.name}-${i}`}
          hit={h}
          showMatched={showMatched}
          propName={propName}
          onPicked={onPicked}
        />
      ))}
    </List>
  )
}

function SearchRow({
  hit,
  showMatched,
  propName,
  onPicked,
}: {
  hit: SearchHit
  showMatched: boolean
  propName?: string
  onPicked: () => void
}) {
  const nav = useNav()
  const qc = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [thumbFailed, setThumbFailed] = useState(false)

  const canNav = !!hit.itemId && !!hit.hubId
  // matchedText carries highlight markers (e.g. <em>); strip tags for display.
  const matchedClean = hit.matched?.replace(/<[^>]+>/g, '').trim()
  const secondary = showMatched
    ? [propName, matchedClean].filter(Boolean).join(': ')
    : hit.kind

  const goTo = async () => {
    if (!canNav || busy) return
    setBusy(true)
    try {
      const loc = await qc.fetchQuery({
        queryKey: ['location', hit.hubId, hit.itemId],
        queryFn: () => api.itemLocation(hit.hubId!, hit.itemId!),
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
        id: hit.itemId!,
        name: hit.name,
        kind: hit.kind,
        isContainer: false,
      })
      onPicked()
    } catch {
      /* couldn't resolve location — leave the dialog open */
    } finally {
      setBusy(false)
    }
  }

  return (
    <ListItemButton onClick={goTo} disabled={!canNav || busy} sx={{ py: 0.75 }}>
      <ListItemIcon sx={{ minWidth: 44 }}>
        {hit.thumbnailUrl && !thumbFailed ? (
          <Box
            component="img"
            src={hit.thumbnailUrl}
            alt=""
            onError={() => setThumbFailed(true)}
            sx={{ width: 32, height: 32, objectFit: 'contain', borderRadius: 0.5, display: 'block' }}
          />
        ) : (
          <Box sx={{ color: 'text.secondary' }}>
            <FontAwesomeIcon
              icon={iconForItem({ id: '', name: '', kind: hit.kind, isContainer: false } as Item)}
              style={{ fontSize: 18 }}
            />
          </Box>
        )}
      </ListItemIcon>
      <ListItemText
        primary={hit.name}
        secondary={secondary}
        secondaryTypographyProps={{ variant: 'caption' }}
      />
      {busy && <CircularProgress size={14} sx={{ ml: 1 }} />}
    </ListItemButton>
  )
}

function Centered({ text }: { text: string }) {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
      <Typography variant="body2" color="text.secondary">
        {text}
      </Typography>
    </Box>
  )
}
