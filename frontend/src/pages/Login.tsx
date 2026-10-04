import { useState } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router-dom'
import { Activity, ArrowLeft, LogIn, LogOut } from 'lucide-react'
import { api } from '../api'
import { useAuth } from '../hooks/useAuth'
import { PasswordInput } from '../components/PasswordInput'
import { ChangePassword } from '../components/ChangePassword'
import { ThemeToggle } from '../components/ThemeToggle'
import { Input } from '../components/ui/input'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card'
import { Field, LoadingButton } from './admin/shared'

export default function Login() {
  const { session, accept, logout } = useAuth()
  const [params] = useSearchParams()
  const requested = params.get('next') || '/admin/overview'
  const next = requested === '/' || /^\/admin(?:\/[a-z]+)?$/.test(requested) ? requested : '/admin/overview'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  if (session.user && !session.user.must_change_password) return <Navigate to={next} replace />
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy) return
    setBusy(true); setError('')
    try { accept(await api.login(username, password)); setPassword('') }
    catch (cause) { setError((cause as Error).message) }
    finally { setBusy(false) }
  }
  const exit = async () => { try { await logout() } catch (cause) { setError((cause as Error).message) } }
  return <div className="app-shell flex min-h-screen items-center justify-center px-4 py-8">
    <div className="absolute right-4 top-4"><ThemeToggle /></div>
    <div className="w-full max-w-md animate-enter">
      <div className="mb-6 flex items-center justify-center gap-3"><span className="flex size-9 items-center justify-center rounded-md bg-primary text-primary-foreground"><Activity className="size-5" /></span><h1 className="text-lg font-semibold">模型连通性</h1></div>
      <Card>
        <CardHeader><CardTitle className="text-lg">{session.user ? '设置新密码' : '登录账户'}</CardTitle>{session.user && <p className="break-all text-sm text-muted-foreground">{session.user.username}，请先更新初始密码</p>}</CardHeader>
        <CardContent>
          {session.user ? <ChangePassword /> : <form onSubmit={submit} className="space-y-4">
            <fieldset disabled={busy} className="space-y-4">
              <Field label="账号" htmlFor="username"><Input id="username" value={username} onChange={event => setUsername(event.target.value)} autoComplete="username" autoFocus required maxLength={32} /></Field>
              <Field label="密码" htmlFor="password"><PasswordInput id="password" value={password} onChange={event => setPassword(event.target.value)} autoComplete="current-password" required /></Field>
            </fieldset>
            {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
            <LoadingButton type="submit" loading={busy} className="w-full"><LogIn />登录</LoadingButton>
          </form>}
        </CardContent>
      </Card>
      <div className="mt-4 flex justify-center gap-3">
        {!session.status_login_required && <Button asChild variant="link"><Link to="/"><ArrowLeft />状态页</Link></Button>}
        {session.user && <Button variant="ghost" onClick={exit}><LogOut />退出登录</Button>}
      </div>
      {session.user && error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    </div>
  </div>
}
