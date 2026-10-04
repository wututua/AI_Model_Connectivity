import { createBrowserRouter, RouterProvider, Outlet, Navigate, useLocation } from 'react-router-dom'
import type { ReactNode } from 'react'
import Dashboard from './pages/Dashboard'
import Admin from './pages/Admin'
import { TooltipProvider } from './components/ui/tooltip'
import { AuthProvider, useAuth } from './hooks/useAuth'
import Login from './pages/Login'
import { Button } from './components/ui/button'
import { Spinner } from './pages/admin/shared'

const router = createBrowserRouter([{
  element: <TooltipProvider delayDuration={300}><AuthProvider><SessionReady><Outlet /></SessionReady></AuthProvider></TooltipProvider>,
  children: [
    { path: '/', element: <RequireSession statusPage><Dashboard /></RequireSession> },
    { path: '/admin/*', element: <RequireSession><Admin /></RequireSession> },
    { path: '/login', element: <Login /> },
    { path: '*', element: <Navigate to="/" replace /> },
  ],
}])

export default function App() {
  return <RouterProvider router={router} />
}

function SessionReady({ children }: { children: ReactNode }) {
  const { loading, ready, error, refresh } = useAuth()
  if (loading) return <div className="flex min-h-screen items-center justify-center"><Spinner className="size-6 text-primary" /></div>
  if (error && !ready) return <div className="flex min-h-screen flex-col items-center justify-center gap-4 p-4"><p role="alert" className="text-sm text-destructive">{error}</p><Button onClick={() => void refresh()}>重试</Button></div>
  return <>
    {children}
    {error && <div role="alert" className="fixed bottom-4 left-4 right-4 z-50 flex items-center gap-3 rounded-md border border-warning/40 bg-background p-3 shadow-lg sm:left-auto sm:max-w-md">
      <p className="min-w-0 flex-1 text-sm text-warning">连接暂时中断，正在保留当前编辑内容。</p>
      <Button variant="outline" size="sm" onClick={() => void refresh()}>重试</Button>
    </div>}
  </>
}

function RequireSession({ children, statusPage }: { children: ReactNode; statusPage?: boolean }) {
  const { session } = useAuth()
  const location = useLocation()
  if ((!statusPage || session.status_login_required) && (!session.user || session.user.must_change_password)) return <Navigate to={`/login?next=${encodeURIComponent(location.pathname)}`} replace />
  return children
}
