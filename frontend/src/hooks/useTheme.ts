import { useState, useEffect } from 'react'

export type Theme = 'dark' | 'light' | 'auto'

function getSystemTheme(): 'dark' | 'light' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  const resolved = theme === 'auto' ? getSystemTheme() : theme
  document.body.setAttribute('data-theme', resolved)
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(() =>
    (localStorage.getItem('theme') as Theme) ?? 'dark'
  )

  useEffect(() => {
    applyTheme(theme)
    localStorage.setItem('theme', theme)

    if (theme === 'auto') {
      const mq = window.matchMedia('(prefers-color-scheme: dark)')
      const onChange = () => applyTheme('auto')
      mq.addEventListener('change', onChange)
      return () => mq.removeEventListener('change', onChange)
    }
  }, [theme])

  // 循环：dark → light → auto → dark
  const cycle = () => setTheme(t =>
    t === 'dark' ? 'light' : t === 'light' ? 'auto' : 'dark'
  )

  return { theme, toggle: cycle }
}
