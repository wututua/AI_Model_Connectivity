import { useState, useEffect, useRef } from 'react'

export function useScrollNav(showThreshold = 80) {
  const [visible, setVisible] = useState(true)
  const lastY = useRef(0)
  const upDist = useRef(0)

  useEffect(() => {
    const onScroll = () => {
      const y = window.scrollY
      const dy = y - lastY.current

      if (y < 60) {
        setVisible(true)
        upDist.current = 0
      } else if (dy < 0) {
        upDist.current += -dy
        if (upDist.current >= showThreshold) setVisible(true)
      } else if (dy > 4) {
        upDist.current = 0
        setVisible(false)
      }

      lastY.current = y
    }

    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [showThreshold])

  return visible
}
