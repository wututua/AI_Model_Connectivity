import { useState, type ComponentType } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { Activity, ArrowLeft, BarChart3, Clock3, Database, FileJson, KeyRound, LayoutDashboard, LogOut, Settings2, ShieldCheck, Users } from 'lucide-react'
import { useAuth } from '../hooks/useAuth'
import { ThemeToggle } from '../components/ThemeToggle'
import { ChangePassword } from '../components/ChangePassword'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Separator } from '../components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '../components/ui/tooltip'
import { cn } from '../lib/utils'
import { OverviewTab } from './admin/OverviewTab'
import { ProvidersTab } from './admin/ProvidersTab'
import { SettingsTab } from './admin/SettingsTab'
import { TasksTab } from './admin/TasksTab'
import { ConfigTab } from './admin/ConfigTab'
import { BillingTab } from './admin/BillingTab'
import { UsersTab } from './admin/UsersTab'

type Tab = 'overview' | 'providers' | 'settings' | 'tasks' | 'billing' | 'config' | 'users' | 'account'
const TABS: { id: Tab; label: string; icon: ComponentType<{ className?: string }>; ownerOnly?: boolean }[] = [
  { id: 'overview', label: '运行概览', icon: LayoutDashboard },
  { id: 'providers', label: 'Provider', icon: Database },
  { id: 'settings', label: '系统设置', icon: Settings2, ownerOnly: true },
  { id: 'tasks', label: '任务历史', icon: Clock3 },
  { id: 'billing', label: 'Token 用量', icon: BarChart3 },
  { id: 'users', label: '用户管理', icon: Users, ownerOnly: true },
  { id: 'config', label: '配置管理', icon: FileJson, ownerOnly: true },
  { id: 'account', label: '账户安全', icon: KeyRound },
]

export default function Admin() {
  const navigate = useNavigate()
  const location = useLocation()
  const { session, logout } = useAuth()
  const [loggingOut, setLoggingOut] = useState(false)
  const [error, setError] = useState('')
  const user = session.user!
  const readOnly = user.role !== 'admin'
  const tabs = TABS.filter(tab => !tab.ownerOnly || !readOnly)
  const pathTab = location.pathname.split('/')[2] as Tab | undefined
  const activeTab = tabs.some(tab => tab.id === pathTab) ? pathTab! : 'overview'
  const activeInfo = tabs.find(tab => tab.id === activeTab)!
  const selectTab = (tab: Tab) => navigate(`/admin/${tab}`)
  const exit = async () => {
    setLoggingOut(true); setError('')
    try { await logout(); navigate('/login', { replace: true }) }
    catch (cause) { setError((cause as Error).message) }
    finally { setLoggingOut(false) }
  }
  return <div className="min-h-screen bg-background">
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur-xl">
      <div className="flex h-14 items-center justify-between px-4 lg:px-6">
        <div className="flex min-w-0 items-center gap-2 sm:gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground"><Activity className="size-4" /></div>
          <div className="min-w-0"><p className="truncate text-sm font-semibold">模型连通性</p><p className="hidden truncate text-xs text-muted-foreground sm:block" title={user.username}>{user.username}</p></div>
          <Badge variant={readOnly ? 'muted' : 'success'} className="hidden shrink-0 whitespace-nowrap sm:inline-flex"><ShieldCheck className="mr-1 size-3" />{readOnly ? '普通用户' : '管理员'}</Badge>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button asChild variant="ghost" size="sm"><Link to="/" aria-label="返回状态页"><ArrowLeft /><span className="hidden sm:inline">状态页</span></Link></Button>
          <ThemeToggle />
          <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={exit} disabled={loggingOut} aria-label="退出登录"><LogOut /></Button></TooltipTrigger><TooltipContent>退出登录</TooltipContent></Tooltip>
        </div>
      </div>
    </header>
    <div className="mx-auto grid min-h-[calc(100vh-3.5rem)] max-w-[1500px] lg:grid-cols-[230px_minmax(0,1fr)]">
      <aside className="hidden border-r bg-muted/20 p-4 lg:block">
        <nav className="sticky top-[4.5rem] space-y-1" aria-label="管理后台导航">
          <p className="mb-3 px-2 text-xs font-medium text-muted-foreground">{readOnly ? '工作台' : '管理导航'}</p>
          {tabs.map(tab => {
            const Icon = tab.icon
            return <button key={tab.id} onClick={() => selectTab(tab.id)} aria-current={activeTab === tab.id ? 'page' : undefined} className={cn('admin-nav-item flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', activeTab === tab.id && 'bg-primary/10 text-primary')}><Icon className="size-4" />{tab.label}</button>
          })}
          <Separator className="my-4" />
          <div className="px-3 py-2"><p className="break-all text-sm font-medium">{user.username}</p><p className="mt-1 text-xs text-muted-foreground">{readOnly ? '普通用户 · 只读' : '管理员'}</p></div>
        </nav>
      </aside>
      <main className="min-w-0 px-4 py-5 sm:px-6 lg:px-8 lg:py-7">
        <div className="mb-5 lg:hidden">
          <Select value={activeTab} onValueChange={value => selectTab(value as Tab)}><SelectTrigger aria-label="当前页面"><SelectValue /></SelectTrigger><SelectContent>{tabs.map(tab => <SelectItem key={tab.id} value={tab.id}>{tab.label}</SelectItem>)}</SelectContent></Select>
        </div>
        {error && <p role="alert" className="mb-4 text-sm text-destructive">{error}</p>}
        <h1 className="mb-5 text-xl font-semibold">{activeInfo.label}</h1>
        <div key={`${activeTab}:${user.role}`} className="animate-enter">
          {activeTab === 'overview' && <OverviewTab readOnly={readOnly} />}
          {activeTab === 'providers' && <ProvidersTab readOnly={readOnly} />}
          {activeTab === 'settings' && !readOnly && <SettingsTab />}
          {activeTab === 'tasks' && <TasksTab />}
          {activeTab === 'billing' && <BillingTab />}
          {activeTab === 'users' && !readOnly && <UsersTab />}
          {activeTab === 'config' && !readOnly && <ConfigTab />}
          {activeTab === 'account' && <ChangePassword />}
        </div>
      </main>
    </div>
  </div>
}
