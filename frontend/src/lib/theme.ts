// User-selectable color theme preference: 'system' | 'light' | 'dark'.
//
// Applied via `data-theme="light" | "dark"` on <html>; when 'system', the
// attribute is removed so CSS falls back to @media (prefers-color-scheme: dark).

import { writable } from 'svelte/store'

export type ThemeMode = 'system' | 'light' | 'dark'

const STORAGE_KEY = 'gotapaper:theme'
const ORDER: readonly ThemeMode[] = ['system', 'light', 'dark']

function readStored(): ThemeMode {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw === 'light' || raw === 'dark' || raw === 'system') return raw
  } catch {
    // Ignore storage errors in restricted contexts.
  }
  return 'system'
}

function applyTheme(mode: ThemeMode) {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  if (mode === 'system') {
    root.removeAttribute('data-theme')
  } else {
    root.setAttribute('data-theme', mode)
  }
}

const initial = readStored()
applyTheme(initial)

export const theme = writable<ThemeMode>(initial)

export function setTheme(mode: ThemeMode) {
  theme.set(mode)
  applyTheme(mode)
  try {
    if (mode === 'system') {
      localStorage.removeItem(STORAGE_KEY)
    } else {
      localStorage.setItem(STORAGE_KEY, mode)
    }
  } catch {
    // Ignore storage errors.
  }
}

export function cycleTheme(current: ThemeMode): ThemeMode {
  const next = ORDER[(ORDER.indexOf(current) + 1) % ORDER.length]
  setTheme(next)
  return next
}

export function themeLabel(mode: ThemeMode): string {
  switch (mode) {
    case 'light':
      return 'Light'
    case 'dark':
      return 'Dark'
    default:
      return 'System'
  }
}
