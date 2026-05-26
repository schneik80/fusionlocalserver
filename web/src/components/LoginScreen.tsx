import { FontAwesomeIcon } from '@fortawesome/react-fontawesome'
import { faCubes } from '@fortawesome/free-solid-svg-icons'
import { Alert, Box, Button, Paper, Stack, Typography } from '@mui/material'

// authErrorMessage maps the ?auth_error=<reason> the server appends on a failed
// callback to something a person can read.
function authErrorMessage(reason: string): string {
  switch (reason) {
    case 'state_mismatch':
    case 'state_expired':
      return 'The sign-in attempt expired or could not be verified. Please try again.'
    case 'no_code':
    case 'exchange_failed':
      return 'Autodesk did not complete the sign-in. Please try again.'
    case 'session_failed':
      return 'The server could not start a session. Please try again.'
    case 'access_denied':
      return 'Sign-in was cancelled or access was denied.'
    default:
      return `Sign-in failed (${reason}).`
  }
}

export function LoginScreen() {
  const authError = new URLSearchParams(window.location.search).get('auth_error')

  return (
    <Box
      sx={{
        height: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        p: 2,
      }}
    >
      <Paper elevation={3} sx={{ p: 4, maxWidth: 380, width: '100%' }}>
        <Stack spacing={2.5} alignItems="center" textAlign="center">
          <FontAwesomeIcon icon={faCubes} style={{ fontSize: 40, color: '#0696d7' }} />
          <Typography variant="h5" sx={{ fontWeight: 600 }}>
            fusionlocalserver
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Sign in with your Autodesk account to browse your Fusion hubs,
            projects, and designs.
          </Typography>
          {authError && (
            <Alert severity="error" sx={{ width: '100%' }}>
              {authErrorMessage(authError)}
            </Alert>
          )}
          <Button
            variant="contained"
            size="large"
            fullWidth
            onClick={() => window.location.assign('/api/auth/login')}
          >
            Sign in with Autodesk
          </Button>
        </Stack>
      </Paper>
    </Box>
  )
}
