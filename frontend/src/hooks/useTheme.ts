import { useState, useEffect } from 'react'

export type Theme = 'dark' | 'light'

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(() =>
    (localStorage.getItem('theme') as Theme) ?? 'dark'
  )

  useEffect(() => {
    document.body.setAttribute('data-theme', theme)
    localStorage.setItem('theme', theme)
  }, [theme])

  return {
    theme,
    toggle: () => setTheme(t => t === 'dark' ? 'light' : 'dark'),
  }
}
