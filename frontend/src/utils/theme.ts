export type Theme = 'dark' | 'light' | 'auto'

let memoryTheme: Theme | undefined

export function readTheme(): Theme {
  if (memoryTheme) return memoryTheme
  try {
    const stored = window.localStorage.getItem('theme')
    if (stored === 'dark' || stored === 'light' || stored === 'auto') return stored
  } catch {
    // Storage can be unavailable even when the property exists.
  }
  return 'dark'
}

export function saveTheme(theme: Theme) {
  memoryTheme = theme
  try {
    window.localStorage.setItem('theme', theme)
  } catch {
    // Keep the choice for navigation/remounts within this page.
  }
}

export function nextTheme(theme: Theme): Theme {
  return theme === 'dark' ? 'light' : theme === 'light' ? 'auto' : 'dark'
}

export function applyTheme(theme: Theme) {
  const resolved = theme === 'auto'
    ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
    : theme
  document.body.setAttribute('data-theme', resolved)
}
