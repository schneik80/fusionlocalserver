import {
  Box,
  Dialog,
  DialogContent,
  DialogTitle,
  Stack,
  ToggleButton,
  ToggleButtonGroup,
  Typography,
} from '@mui/material'
import { useMeta } from '../api/queries'
import { useColorMode } from '../state/colorMode'

export function SettingsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { preference, setPreference } = useColorMode()
  const metaQ = useMeta()

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="xs">
      <DialogTitle>Settings</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={3} sx={{ py: 1 }}>
          <Field label="Theme">
            <ToggleButtonGroup
              size="small"
              exclusive
              value={preference}
              onChange={(_, v) => v && setPreference(v)}
            >
              <ToggleButton value="light">Light</ToggleButton>
              <ToggleButton value="dark">Dark</ToggleButton>
              <ToggleButton value="system">System</ToggleButton>
            </ToggleButtonGroup>
          </Field>

          <Field label="Region">
            <Typography variant="body2">{metaQ.data?.region ?? '—'}</Typography>
            <Typography variant="caption" color="text.secondary">
              Set at server startup (read-only).
            </Typography>
          </Field>

          <Field label="About">
            <Typography variant="body2">FusionDataCLI · {metaQ.data?.version ?? '—'}</Typography>
            <Typography variant="caption" color="text.secondary">
              Fusion open/insert and STEP download are not yet available in this build.
            </Typography>
          </Field>
        </Stack>
      </DialogContent>
    </Dialog>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {label}
      </Typography>
      <Stack spacing={0.5}>{children}</Stack>
    </Box>
  )
}
