// localStorage key for the persisted React Query cache. Kept in its own module
// (no side effects) so both main.tsx (which sets up persistence) and AppLayout
// (which clears it on logout) can import it without a circular dependency.
export const QUERY_CACHE_KEY = 'fls.queryCache'
