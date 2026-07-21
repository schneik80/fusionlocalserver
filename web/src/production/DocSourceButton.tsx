import { faChevronDown, faCloudArrowUp, faFolderOpen } from '@fortawesome/free-solid-svg-icons'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { Box, Button, CircularProgress, Menu, MenuItem, Snackbar } from '@mui/material'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { UploadJob } from '../api/types'
import { extOf } from '../components/viewers/kind'
import { useNav } from '../state/nav'
import { useUploads } from '../state/uploads'
import { AttachDocDialog } from './AttachDocDialog'
import type { DocPin } from './types'

// DocSourceButton lets a user supply a document from either of the two sources
// the product calls for — browsing the Fusion Team hub, or uploading a local
// file. Both paths resolve to a DocPin the server version-pins.
//
// The upload path rides the app's shared upload infrastructure: the accepted
// job is seeded into the ['uploads'] query cache exactly like
// UploadsProvider.startUpload does, so the provider's polling takes over, the
// job shows in the upload footer (cancelable there), and its completion
// triggers the provider's contents invalidation. This component just watches
// the provider's job list for its job to finish, then pins the version the
// upload created (job.versionId) — not whatever the tip is by then.

// tracked remembers the upload this button is waiting on, with the pin fields
// captured at submit time (nav may move while the transfer runs).
interface trackedUpload {
  jobId: string
  hubId: string
  dmProjectId: string
}

export function DocSourceButton({
  label,
  icon,
  variant = 'outlined',
  onPin,
}: {
  label: string
  icon?: IconDefinition
  variant?: 'text' | 'outlined' | 'contained'
  onPin: (pin: DocPin, source: 'hub' | 'upload') => void
}) {
  const nav = useNav()
  const qc = useQueryClient()
  const { jobs } = useUploads()
  const fileRef = useRef<HTMLInputElement>(null)
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null)
  const [browseOpen, setBrowseOpen] = useState(false)
  const [tracked, setTracked] = useState<trackedUpload | null>(null)
  const [uploadingName, setUploadingName] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const ready = !!nav.hubId && !!nav.project?.altId

  const doUpload = async (file: File) => {
    if (!nav.hubId || !nav.project?.altId) return
    setUploadingName(file.name)
    setError(null)
    try {
      const job = await api.uploadFile(
        {
          hubId: nav.hubId,
          dmProjectId: nav.project.altId,
          projectId: nav.project.id,
          folderPath: [], // project root; the server resolves it
        },
        file,
      )
      // Seed the shared uploads cache so UploadsProvider's polling, the
      // footer, and the contents invalidation all pick this job up.
      qc.setQueryData<UploadJob[]>(['uploads'], (old) => [...(old ?? []), job])
      setTracked({ jobId: job.id, hubId: nav.hubId, dmProjectId: nav.project.altId })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Upload failed')
      setUploadingName(null)
    }
  }

  // Watch the provider's job list for the tracked upload to reach a terminal
  // state. Effect-based (not a private poll loop), so it dies with the
  // component and never duplicates the provider's polling.
  const onPinRef = useRef(onPin)
  onPinRef.current = onPin
  useEffect(() => {
    if (!tracked) return
    const j = jobs.find((x) => x.id === tracked.jobId)
    if (!j) return
    if (j.status === 'done' && j.itemId) {
      onPinRef.current(
        {
          hubId: tracked.hubId,
          itemId: j.itemId,
          dmProjectId: tracked.dmProjectId,
          name: j.fileName,
          kind: kindHint(j.fileName),
          versionId: j.versionId, // pin the version this upload created
        },
        'upload',
      )
      setTracked(null)
      setUploadingName(null)
    } else if (j.status === 'error' || j.status === 'canceled') {
      if (j.status === 'error') setError(j.error || 'Upload failed')
      setTracked(null)
      setUploadingName(null)
    }
  }, [jobs, tracked])

  return (
    <>
      <Button
        size="small"
        variant={variant}
        disabled={!ready || !!uploadingName}
        onClick={(e) => setMenuAnchor(e.currentTarget)}
        startIcon={
          uploadingName ? (
            <CircularProgress size={12} />
          ) : icon ? (
            <FontAwesomeIcon icon={icon} style={{ fontSize: 11 }} />
          ) : undefined
        }
        endIcon={!uploadingName && <FontAwesomeIcon icon={faChevronDown} style={{ fontSize: 9 }} />}
        sx={{ textTransform: 'none' }}
      >
        {uploadingName ? (
          <Box component="span" sx={{ maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            Uploading {uploadingName}…
          </Box>
        ) : (
          label
        )}
      </Button>

      <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={() => setMenuAnchor(null)}>
        <MenuItem
          onClick={() => {
            setMenuAnchor(null)
            setBrowseOpen(true)
          }}
        >
          <FontAwesomeIcon icon={faFolderOpen} style={{ fontSize: 12, marginRight: 8, width: 16 }} />
          Browse the hub…
        </MenuItem>
        <MenuItem
          onClick={() => {
            setMenuAnchor(null)
            fileRef.current?.click()
          }}
        >
          <FontAwesomeIcon icon={faCloudArrowUp} style={{ fontSize: 12, marginRight: 8, width: 16 }} />
          Upload a file…
        </MenuItem>
      </Menu>

      <input
        ref={fileRef}
        type="file"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0]
          e.target.value = '' // allow re-selecting the same file
          if (f) void doUpload(f)
        }}
      />

      <AttachDocDialog
        open={browseOpen}
        hubId={nav.hubId ?? null}
        initialProject={nav.project ?? null}
        onClose={() => setBrowseOpen(false)}
        onPicked={(pin) => {
          setBrowseOpen(false)
          onPin(pin, 'hub')
        }}
      />

      <Snackbar
        open={!!error}
        autoHideDuration={5000}
        onClose={() => setError(null)}
        message={error ?? ''}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      />
    </>
  )
}

// kindHint maps an uploaded filename to an Item kind for the thumbnail/icon.
// The fallback must be 'unknown', not a made-up value: DetailsPanel's tabsFor()
// keys the whole tab set off kind, and only 'unknown' gets a Preview tab (the
// server calls plain uploads "unknown" too — see api/browse.go).
function kindHint(name: string): string {
  const ext = extOf(name)
  if (ext === 'f3d' || ext === 'f3z' || ext === 'wire') return 'design'
  if (ext === 'f2d' || ext === 'f2t') return 'drawing'
  return 'unknown'
}
