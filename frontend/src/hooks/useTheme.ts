import { useState, useEffect } from 'react'
import { applyTheme, nextTheme, readTheme, saveTheme, type Theme } from '../utils/theme'
export type { Theme } from '../utils/theme'

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(() => {
    const stored = readTheme()
    applyTheme(stored)
    return stored
  })

  useEffect(() => {
    saveTheme(theme)

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
    const next = nextTheme(theme)
    const doApply = () => applyTheme(next)

    if ('startViewTransition' in document && !window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      ;(document as unknown as { startViewTransition: (cb: () => void) => unknown })
        .startViewTransition(doApply)
    } else {
      doApply()
    }

    setTheme(next)
  }

  return { theme, toggle: cycle }
}
