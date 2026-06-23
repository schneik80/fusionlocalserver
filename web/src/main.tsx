import '@fontsource/montserrat/400.css'
import '@fontsource/montserrat/500.css'
import '@fontsource/montserrat/600.css'
import '@fontsource/montserrat/700.css'

import { QueryClient } from '@tanstack/react-query'
import { createSyncStoragePersister } from '@tanstack/query-sync-storage-persister'
import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import { QUERY_CACHE_KEY } from './queryPersist'
import { ColorModeProvider } from './state/colorMode'

const DAY = 24 * 60 * 60 * 1000

// gcTime must outlive maxAge so inactive queries survive long enough to persist.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { refetchOnWindowFocus: false, retry: 1, gcTime: DAY },
  },
})

// Persist the browsing cache so a reload paints hubs / projects / contents /
// details instantly, then revalidates in the background.
const persister = createSyncStoragePersister({
  storage: window.localStorage,
  key: QUERY_CACHE_KEY,
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{
        persister,
        maxAge: DAY,
        // Bump when query shapes change to invalidate stale persisted caches.
        buster: 'fls-2',
        dehydrateOptions: {
          // Persist only successful, non-volatile queries. Auth state must stay
          // fresh (and persisting it could briefly show a prior user's state).
          shouldDehydrateQuery: (q) =>
            q.state.status === 'success' && q.queryKey[0] !== 'authMe',
        },
      }}
    >
      <ColorModeProvider>
        <App />
      </ColorModeProvider>
    </PersistQueryClientProvider>
  </StrictMode>,
)
