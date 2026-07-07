import {
  createContext,
  useContext,
  useMemo,
  useReducer,
  type ReactNode,
} from 'react'
import type { Item } from '../api/types'

// The last selected hub is remembered in localStorage (per browser) so a
// reload/return lands back in the hub you were using. It is restored only after
// the hub list loads and only if the saved hub is still in it (see AppLayout),
// so it can't strand a user on a hub they no longer have.
const HUB_STORAGE_KEY = 'fls.lastHub'

interface SavedHub {
  id: string
  name: string
}

export function loadLastHub(): SavedHub | null {
  try {
    const raw = localStorage.getItem(HUB_STORAGE_KEY)
    if (!raw) return null
    const v = JSON.parse(raw) as SavedHub
    return v && typeof v.id === 'string' ? v : null
  } catch {
    return null
  }
}

function saveLastHub(id: string, name: string) {
  try {
    localStorage.setItem(HUB_STORAGE_KEY, JSON.stringify({ id, name }))
  } catch {
    /* storage unavailable (private mode / quota) — non-fatal */
  }
}

// The rail's top-level view: the document browser or the cross-project
// Tasks screen. Both stay mounted (AppLayout display-toggles them) so
// switching apps never loses browser drill-down or task-list state.
export type AppKind = 'browser' | 'tasks'

// Navigation state for the three-column browser. The hub is chosen from the
// rail/switcher; project lives in the Projects column; folderStack is the
// drill-down path inside the Contents column (mirrored by the breadcrumb);
// selected is the document whose Details panel is shown.
export interface NavState {
  app: AppKind
  hubId: string | null
  hubName: string | null
  project: Item | null
  folderStack: Item[]
  selected: Item | null
  // The Details tab to open on the next selection (consumed at mount). Set by
  // navigate() so a cross-document jump can land on the same tab it came from;
  // cleared by plain selection so ordinary clicks default to History.
  selectedTab: string | null
}

const initialState: NavState = {
  app: 'browser',
  hubId: null,
  hubName: null,
  project: null,
  folderStack: [],
  selected: null,
  selectedTab: null,
}

type Action =
  | { type: 'setApp'; app: AppKind }
  | { type: 'selectHub'; id: string; name: string }
  | { type: 'selectProject'; project: Item }
  | { type: 'enterFolder'; folder: Item }
  | { type: 'selectItem'; item: Item }
  | { type: 'clearProject' }
  | { type: 'gotoProjectRoot' }
  | { type: 'gotoFolder'; index: number }
  | {
      type: 'navigate'
      project: Item
      folderStack: Item[]
      selected: Item | null
      tab?: string
    }

function reducer(state: NavState, action: Action): NavState {
  switch (action.type) {
    case 'setApp':
      return state.app === action.app ? state : { ...state, app: action.app }
    case 'selectHub':
      if (action.id === state.hubId) return state
      // The Tasks screen is hub-independent (my-tasks spans every project on
      // this server), so switching hubs keeps the current app.
      return { ...initialState, app: state.app, hubId: action.id, hubName: action.name }
    case 'selectProject':
      return { ...state, project: action.project, folderStack: [], selected: null, selectedTab: null }
    case 'enterFolder':
      return {
        ...state,
        folderStack: [...state.folderStack, action.folder],
        selected: null,
        selectedTab: null,
      }
    case 'selectItem':
      return { ...state, selected: action.item, selectedTab: null }
    case 'clearProject':
      // Back to the hub level (projects list); keep the hub, drop everything below it.
      return { ...state, project: null, folderStack: [], selected: null, selectedTab: null }
    case 'gotoProjectRoot':
      return { ...state, folderStack: [], selected: null, selectedTab: null }
    case 'gotoFolder':
      return {
        ...state,
        folderStack: state.folderStack.slice(0, action.index + 1),
        selected: null,
        selectedTab: null,
      }
    case 'navigate':
      // Cross-document jumps (document cards, where-used, …) land in the
      // browser regardless of which app issued them.
      return {
        ...state,
        app: 'browser',
        project: action.project,
        folderStack: action.folderStack,
        selected: action.selected,
        selectedTab: action.tab ?? null,
      }
    default:
      return state
  }
}

interface NavCtx extends NavState {
  setApp: (app: AppKind) => void
  selectHub: (id: string, name: string) => void
  selectProject: (project: Item) => void
  enterFolder: (folder: Item) => void
  selectItem: (item: Item) => void
  clearProject: () => void
  gotoProjectRoot: () => void
  gotoFolder: (index: number) => void
  navigate: (project: Item, folderStack: Item[], selected: Item | null, tab?: string) => void
  /** id of the folder whose contents the Contents column currently shows, or null at project root */
  currentFolderId: string | null
}

const Ctx = createContext<NavCtx | null>(null)

export function NavProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState)

  const value = useMemo<NavCtx>(() => {
    const top = state.folderStack[state.folderStack.length - 1]
    return {
      ...state,
      currentFolderId: top ? top.id : null,
      setApp: (app) => dispatch({ type: 'setApp', app }),
      selectHub: (id, name) => {
        saveLastHub(id, name)
        dispatch({ type: 'selectHub', id, name })
      },
      selectProject: (project) => dispatch({ type: 'selectProject', project }),
      enterFolder: (folder) => dispatch({ type: 'enterFolder', folder }),
      selectItem: (item) => dispatch({ type: 'selectItem', item }),
      clearProject: () => dispatch({ type: 'clearProject' }),
      gotoProjectRoot: () => dispatch({ type: 'gotoProjectRoot' }),
      gotoFolder: (index) => dispatch({ type: 'gotoFolder', index }),
      navigate: (project, folderStack, selected, tab) =>
        dispatch({ type: 'navigate', project, folderStack, selected, tab }),
    }
  }, [state])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useNav(): NavCtx {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useNav must be used within NavProvider')
  return ctx
}
