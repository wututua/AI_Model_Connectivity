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
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = (localStorage.getItem('theme') as Theme) ?? 'dark'
    applyTheme(stored)
    return stored
  })

  useEffect(() => {
    localStorage.setItem('theme', theme)

    if (theme === 'auto') {
      const mq = window.matchMedia('(prefers-color-scheme: dark)')
      const onChange = () => applyTheme('auto')
      mq.addEventListener('change', onChange)
      return () => mq.removeEventListener('change', onChange)
    }
  }, [theme])

  // 循环：dark → light → auto → dark
  // 使用 View Transitions API 在截图层级做交叉淡入，避免 backdrop-filter 实时重绘
  const cycle = () => {
    const next: Theme = theme === 'dark' ? 'light' : theme === 'light' ? 'auto' : 'dark'
    const doApply = () => applyTheme(next)

    if ('startViewTransition' in document) {
      ;(document as unknown as { startViewTransition: (cb: () => void) => unknown })
        .startViewTransition(doApply)
    } else {
      doApply()
    }

    setTheme(next)
  }

  return { theme, toggle: cycle }
}
