import { ref, readonly } from 'vue'

const STORAGE_KEY = 'portforge.theme'

// The OS preference is only consulted when the user has never chosen. Once they
// toggle, their choice wins for good — a desktop app that silently flips theme
// because the system went dark at sunset is worse than one that stays put.
function initialTheme() {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored === 'light' || stored === 'dark') return stored
  return window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

const theme = ref(initialTheme())

// data-theme always holds a concrete 'light' or 'dark', never absent, so the
// stylesheet needs no prefers-color-scheme branch and there is one source of truth.
function apply(value) {
  document.documentElement.setAttribute('data-theme', value)
}

apply(theme.value)

export function useTheme() {
  function setTheme(value) {
    if (value !== 'light' && value !== 'dark') return
    theme.value = value
    localStorage.setItem(STORAGE_KEY, value)
    apply(value)
  }

  function toggleTheme() {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  return { theme: readonly(theme), setTheme, toggleTheme }
}
