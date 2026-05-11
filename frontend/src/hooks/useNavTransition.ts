import { useNavigate } from 'react-router-dom'

export function useNavTransition() {
  const navigate = useNavigate()

  return (to: string) => {
    if ('startViewTransition' in document) {
      ;(document as unknown as { startViewTransition: (cb: () => void) => unknown })
        .startViewTransition(() => { navigate(to) })
    } else {
      navigate(to)
    }
  }
}
