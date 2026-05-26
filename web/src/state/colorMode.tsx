import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import type { ColorMode } from '../theme'

const STORAGE_KEY = 'fdc.colorMode'

interface ColorModeCtx {
  mode: ColorMode
  /** explicit user choice, or null when following the system preference */
  preference: ColorMode | 'system'
  setPreference: (p: ColorMode | 'system') => void
  toggle: () => void
}

const Ctx = createContext<ColorModeCtx | null>(null)

function systemMode(): ColorMode {
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

function loadPreference(): ColorMode | 'system' {
  const v = localStorage.getItem(STORAGE_KEY)
  if (v === 'light' || v === 'dark') return v
  return 'system'
}

export function ColorModeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ColorMode | 'system'>(
    loadPreference,
  )
  const [system, setSystem] = useState<ColorMode>(systemMode)

  // Track OS theme changes while the user is on "system".
  useEffect(() => {
    const mql = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => setSystem(mql.matches ? 'dark' : 'light')
    mql.addEventListener('change', onChange)
    return () => mql.removeEventListener('change', onChange)
  }, [])

  const setPreference = useCallback((p: ColorMode | 'system') => {
    setPreferenceState(p)
    if (p === 'system') localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, p)
  }, [])

  const mode: ColorMode = preference === 'system' ? system : preference

  const toggle = useCallback(() => {
    setPreference(mode === 'dark' ? 'light' : 'dark')
  }, [mode, setPreference])

  const value = useMemo(
    () => ({ mode, preference, setPreference, toggle }),
    [mode, preference, setPreference, toggle],
  )

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useColorMode(): ColorModeCtx {
  const ctx = useContext(Ctx)
  if (!ctx) throw new Error('useColorMode must be used within ColorModeProvider')
  return ctx
}
