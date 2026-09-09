import { useEffect, useMemo, useState, type ComponentType } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import {
  Activity, ArrowLeft, BarChart3, Clock3, Database, Eye, EyeOff,
  FileJson, KeyRound, LayoutDashboard, LogOut, Settings2, ShieldCheck,
} from 'lucide-react'
import { api, getToken, setToken } from '../api'
import type { RunningState } from '../types'
import { ThemeToggle } from '../components/ThemeToggle'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../components/ui/card'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Separator } from '../components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '../components/ui/tooltip'
import { cn } from '../lib/utils'
import { LoadingButton, Spinner } from './admin/shared'
import { OverviewTab } from './admin/OverviewTab'
import { ProvidersTab } from './admin/ProvidersTab'
import { SettingsTab } from './admin/SettingsTab'
import { TasksTab } from './admin/TasksTab'
import { ConfigTab } from './admin/ConfigTab'
import { BillingTab } from './admin/BillingTab'

type Tab = 'overview' | 'providers' | 'settings' | 'tasks' | 'billing' | 'config'

const TABS: { id: Tab; label: string; description: string; icon: ComponentType<{ className?: string }>; ownerOnly?: boolean }[] = [
  { id: 'overview', label: '运行概览', description: '检测状态、运行控制与最近结果', icon: LayoutDashboard },
  { id: 'providers', label: 'Provider', description: '管理接口地址、模型与检测状态', icon: Database },
  { id: 'settings', label: '系统设置', description: '检测、历史、调度与通知策略', icon: Settings2, ownerOnly: true },
  { id: 'tasks', label: '任务历史', description: '查看检测任务、耗时与结果', icon: Clock3 },
  { id: 'billing', label: 'Token 用量', description: '分析模型探测产生的 Token 消耗', icon: BarChart3 },
  { id: 'config', label: '配置管理', description: '导入、导出和重新加载配置', icon: FileJson, ownerOnly: true },
]

export default function Admin() {
  const navigate = useNavigate()
  const location = useLocation()
  const [session, setSession] = useState<RunningState | null>(null)
  const [verifying, setVerifying] = useState(Boolean(getToken()))

  useEffect(() => {
    const handleUnauthorized = () => setSession(null)
    window.addEventListener('cg:unauthorized', handleUnauthorized)
    return () => window.removeEventListener('cg:unauthorized', handleUnauthorized)
  }, [])

  useEffect(() => {
    if (!getToken()) return
    api.detection().then(setSession).catch(() => setToken('')).finally(() => setVerifying(false))
  }, [])

  if (verifying) return <div className="flex min-h-screen items-center justify-center bg-background"><Spinner className="size-6 text-primary" /></div>
  if (!session) return <TokenGate onEnter={state => { setSession(state); navigate('/admin/overview', { replace: true }) }} />
  if (session.first_use && !session.read_only) return <ChangeTokenForm onDone={() => setSession({ ...session, first_use: false })} />

  const availableTabs = TABS.filter(tab => !tab.ownerOnly || !session.read_only)
  const pathTab = location.pathname.split('/')[2] as Tab | undefined
  const activeTab = availableTabs.some(tab => tab.id === pathTab) ? pathTab! : 'overview'
  const activeInfo = availableTabs.find(tab => tab.id === activeTab) ?? availableTabs[0]

  const selectTab = (tab: Tab) => navigate(`/admin/${tab}`)
  const logout = () => { setToken(''); setSession(null); navigate('/admin', { replace: true }) }

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-40 border-b bg-background/92 backdrop-blur-xl">
        <div className="flex h-14 items-center justify-between px-4 lg:px-6">
          <div className="flex items-center gap-3">
            <div className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground"><Activity className="size-4" /></div>
            <div><p className="text-sm font-semibold">模型连通性</p><p className="hidden text-xs text-muted-foreground sm:block">管理后台</p></div>
            {session.read_only && <Badge variant="warning"><ShieldCheck className="mr-1 size-3" />只读</Badge>}
          </div>
          <div className="flex items-center gap-1">
            <Button asChild variant="ghost" size="sm"><Link to="/"><ArrowLeft />状态页</Link></Button>
            <ThemeToggle />
            <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={logout} aria-label="退出登录"><LogOut /></Button></TooltipTrigger><TooltipContent>退出登录</TooltipContent></Tooltip>
          </div>
        </div>
      </header>

      <div className="mx-auto grid min-h-[calc(100vh-3.5rem)] max-w-[1500px] lg:grid-cols-[230px_minmax(0,1fr)]">
        <aside className="hidden border-r bg-muted/20 p-4 lg:block">
          <nav className="sticky top-[4.5rem] space-y-1" aria-label="管理后台导航">
            <p className="mb-3 px-2 text-xs font-medium text-muted-foreground">管理导航</p>
            {availableTabs.map(tab => {
              const Icon = tab.icon
              return (
                <button key={tab.id} onClick={() => selectTab(tab.id)} className={cn('flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground', activeTab === tab.id && 'bg-accent text-foreground')}>
                  <Icon className="size-4" />{tab.label}
                </button>
              )
            })}
            <Separator className="my-4" />
            <div className="rounded-md border bg-background p-3">
              <p className="text-xs font-medium">当前权限</p>
              <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{session.read_only ? '可查看运行数据，不能修改配置。' : '拥有完整配置与运行控制权限。'}</p>
            </div>
          </nav>
        </aside>

        <main className="min-w-0 px-4 py-5 sm:px-6 lg:px-8 lg:py-7">
          <div className="mb-5 lg:hidden">
            <Label className="sr-only">当前页面</Label>
            <Select value={activeTab} onValueChange={value => selectTab(value as Tab)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>{availableTabs.map(tab => <SelectItem key={tab.id} value={tab.id}>{tab.label}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <div className="mb-6">
            <h1 className="text-xl font-semibold">{activeInfo.label}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{activeInfo.description}</p>
          </div>
          <div key={activeTab} className="animate-enter">
            {activeTab === 'overview' && <OverviewTab readOnly={session.read_only} />}
            {activeTab === 'providers' && <ProvidersTab readOnly={session.read_only} />}
            {activeTab === 'settings' && !session.read_only && <SettingsTab />}
            {activeTab === 'tasks' && <TasksTab />}
            {activeTab === 'billing' && <BillingTab />}
            {activeTab === 'config' && !session.read_only && <ConfigTab />}
          </div>
        </main>
      </div>
    </div>
  )
}

function PasswordField({ id, value, onChange, show, setShow, placeholder, autoFocus, onSubmit }: {
  id: string; value: string; onChange: (value: string) => void; show: boolean; setShow: (value: boolean) => void
  placeholder: string; autoFocus?: boolean; onSubmit: () => void
}) {
  return (
    <div className="relative">
      <Input id={id} type={show ? 'text' : 'password'} value={value} onChange={event => onChange(event.target.value)} onKeyDown={event => event.key === 'Enter' && onSubmit()} placeholder={placeholder} autoFocus={autoFocus} className="pr-10 font-mono" />
      <Button type="button" variant="ghost" size="icon" className="absolute right-0 top-0" onClick={() => setShow(!show)} aria-label={show ? '隐藏密钥' : '显示密钥'}>{show ? <EyeOff /> : <Eye />}</Button>
    </div>
  )
}

function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="app-shell flex min-h-screen items-center justify-center p-4">
      <div className="w-full max-w-md">
        <div className="mb-5 flex items-center justify-center gap-2"><div className="flex size-9 items-center justify-center rounded-md bg-primary text-primary-foreground"><Activity className="size-5" /></div><span className="font-semibold">模型连通性</span></div>
        {children}
        <div className="mt-4 text-center"><Button asChild variant="link" size="sm"><Link to="/"><ArrowLeft />返回状态页</Link></Button></div>
      </div>
    </div>
  )
}

function TokenGate({ onEnter }: { onEnter: (state: RunningState) => void }) {
  const [value, setValue] = useState('')
  const [show, setShow] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [tokenRequired, setTokenRequired] = useState(false)

  const submit = async () => {
    if (loading) return
    const token = value.trim()
    if (tokenRequired && !token) { setError('服务器要求提供管理或只读密钥'); return }
    setLoading(true); setError(''); setToken(token)
    try { onEnter(await api.detection()) }
    catch (cause) { setToken(''); setTokenRequired(true); setError(`验证失败：${(cause as Error).message}`) }
    finally { setLoading(false) }
  }

  return (
    <AuthLayout>
      <Card className="shadow-panel">
        <CardHeader><CardTitle className="flex items-center gap-2"><KeyRound className="size-5 text-primary" />访问管理后台</CardTitle><CardDescription>输入 Admin Token 获取完整权限，或使用 View Token 只读访问。</CardDescription></CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2"><Label htmlFor="admin-token">访问密钥</Label><PasswordField id="admin-token" value={value} onChange={next => { setValue(next); setError('') }} show={show} setShow={setShow} placeholder="Admin Token / View Token" autoFocus onSubmit={submit} /></div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <LoadingButton className="w-full" onClick={submit} loading={loading}>验证并进入</LoadingButton>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}

function ChangeTokenForm({ onDone }: { onDone: () => void }) {
  const [value, setValue] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showValue, setShowValue] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const strength = useMemo(() => Math.min(100, (value.length / 24) * 100), [value])

  const submit = async () => {
    const token = value.trim()
    if (token.length < 16) { setError('密钥至少需要 16 位'); return }
    if (token !== confirm.trim()) { setError('两次输入的密钥不一致'); return }
    setLoading(true); setError('')
    try { await api.changeToken(token); setToken(token); onDone() }
    catch (cause) { setError(`修改失败：${(cause as Error).message}`) }
    finally { setLoading(false) }
  }

  return (
    <AuthLayout>
      <Card className="shadow-panel">
        <CardHeader><CardTitle className="flex items-center gap-2"><ShieldCheck className="size-5 text-warning" />设置新的管理密钥</CardTitle><CardDescription>当前密钥由系统自动生成。首次登录必须设置至少 16 位的新密钥。</CardDescription></CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2"><Label htmlFor="new-token">新密钥</Label><PasswordField id="new-token" value={value} onChange={next => { setValue(next); setError('') }} show={showValue} setShow={setShowValue} placeholder="至少 16 位" autoFocus onSubmit={submit} /><div className="h-1 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary transition-all" style={{ width: `${strength}%` }} /></div></div>
          <div className="space-y-2"><Label htmlFor="confirm-token">确认新密钥</Label><PasswordField id="confirm-token" value={confirm} onChange={next => { setConfirm(next); setError('') }} show={showConfirm} setShow={setShowConfirm} placeholder="再次输入新密钥" onSubmit={submit} /></div>
          {error && <p className="text-sm text-destructive">{error}</p>}
          <LoadingButton className="w-full" onClick={submit} loading={loading}>保存密钥并继续</LoadingButton>
        </CardContent>
      </Card>
    </AuthLayout>
  )
}
