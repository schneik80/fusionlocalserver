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

// Navigation state for the three-column browser. The hub is chosen from the
// rail/switcher; project lives in the Projects column; folderStack is the
// drill-down path inside the Contents column (mirrored by the breadcrumb);
// selected is the document whose Details panel is shown.
export interface NavState {
  hubId: string | null
  hubName: string | null
  project: Item | null
  folderStack: Item[]
  selected: Item | null
}

const initialState: NavState = {
  hubId: null,
  hubName: null,
  project: null,
  folderStack: [],
  selected: null,
}

type Action =
  | { type: 'selectHub'; id: string; name: string }
  | { type: 'selectProject'; project: Item }
  | { type: 'enterFolder'; folder: Item }
  | { type: 'selectItem'; item: Item }
  | { type: 'gotoProjectRoot' }
  | { type: 'gotoFolder'; index: number }
  | {
      type: 'navigate'
      project: Item
      folderStack: Item[]
      selected: Item | null
    }

function reducer(state: NavState, action: Action): NavState {
  switch (action.type) {
    case 'selectHub':
      if (action.id === state.hubId) return state
      return { ...initialState, hubId: action.id, hubName: action.name }
    case 'selectProject':
      return { ...state, project: action.project, folderStack: [], selected: null }
    case 'enterFolder':
      return {
        ...state,
        folderStack: [...state.folderStack, action.folder],
        selected: null,
      }
    case 'selectItem':
      return { ...state, selected: action.item }
    case 'gotoProjectRoot':
      return { ...state, folderStack: [], selected: null }
    case 'gotoFolder':
      return {
        ...state,
        folderStack: state.folderStack.slice(0, action.index + 1),
        selected: null,
      }
    case 'navigate':
      return {
        ...state,
        project: action.project,
        folderStack: action.folderStack,
        selected: action.selected,
      }
    default:
      return state
  }
}

interface NavCtx extends NavState {
  selectHub: (id: string, name: string) => void
  selectProject: (project: Item) => void
  enterFolder: (folder: Item) => void
  selectItem: (item: Item) => void
  gotoProjectRoot: () => void
  gotoFolder: (index: number) => void
  navigate: (project: Item, folderStack: Item[], selected: Item | null) => void
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
      selectHub: (id, name) => {
        saveLastHub(id, name)
        dispatch({ type: 'selectHub', id, name })
      },
      selectProject: (project) => dispatch({ type: 'selectProject', project }),
      enterFolder: (folder) => dispatch({ type: 'enterFolder', folder }),
      selectItem: (item) => dispatch({ type: 'selectItem', item }),
      gotoProjectRoot: () => dispatch({ type: 'gotoProjectRoot' }),
      gotoFolder: (index) => dispatch({ type: 'gotoFolder', index }),
      navigate: (project, folderStack, selected) =>
        dispatch({ type: 'navigate', project, folderStack, selected }),
    }
  }, [state])

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useNav(): NavCtx {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useNav must be used within NavProvider')
  return ctx
}
