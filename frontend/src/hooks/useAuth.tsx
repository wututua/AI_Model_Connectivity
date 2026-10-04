import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { api, applySession } from '../api'
import type { AuthSession } from '../types'

const anonymous: AuthSession = { user: null, csrf_token: '', expires_at: 0, status_login_required: true }
const AuthContext = createContext<{
  session: AuthSession
  loading: boolean
  ready: boolean
  error: string
  refresh: () => Promise<void>
  accept: (session: AuthSession) => void
  logout: () => Promise<void>
} | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState(anonymous)
  const [loading, setLoading] = useState(true)
  const [ready, setReady] = useState(false)
  const [error, setError] = useState('')
  const revision = useRef(0)
  const accept = useCallback((value: AuthSession) => { revision.current++; applySession(value); setSession(value); setReady(true); setError(''); setLoading(false) }, [])
  const refresh = useCallback(async () => {
    const id = ++revision.current
    try {
      const value = await api.session()
      if (id === revision.current) { applySession(value); setSession(value); setReady(true); setError('') }
    } catch (cause) {
      if (id === revision.current) setError((cause as Error).message)
    } finally { if (id === revision.current) setLoading(false) }
  }, [])
  const logout = useCallback(async () => {
    await api.logout()
    accept(anonymous)
    await refresh()
  }, [accept, refresh])

  useEffect(() => {
    void refresh()
    const unauthorized = () => { accept(anonymous); void refresh() }
    const update = () => { void refresh() }
    const timer = setInterval(update, 30_000)
    window.addEventListener('cg:unauthorized', unauthorized)
    window.addEventListener('cg:session-refresh', update)
    window.addEventListener('focus', update)
    return () => {
      revision.current++
      clearInterval(timer)
      window.removeEventListener('cg:unauthorized', unauthorized)
      window.removeEventListener('cg:session-refresh', update)
      window.removeEventListener('focus', update)
    }
  }, [accept, refresh])
  return <AuthContext.Provider value={{ session, loading, ready, error, refresh, accept, logout }}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error('AuthProvider is required')
  return value
}
