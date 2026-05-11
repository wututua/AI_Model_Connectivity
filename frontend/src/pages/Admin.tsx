import React, { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity, ArrowLeft, Eye, EyeOff,
  Loader2, LogOut, Moon, Sun, Monitor, Settings, FileJson, Database, Clock,
} from 'lucide-react'
import { api, getToken, setToken } from '../api'
import { useTheme } from '../hooks/useTheme'
import { useScrollNav } from '../hooks/useScrollNav'
import { Btn, inputCls } from './admin/shared'
import { OverviewTab } from './admin/OverviewTab'
import { ProvidersTab } from './admin/ProvidersTab'
import { SettingsTab } from './admin/SettingsTab'
import { TasksTab } from './admin/TasksTab'
import { ConfigTab } from './admin/ConfigTab'

// ── Token Gate ──────────────────────────────────────────────────────────────

function TokenGate({ onEnter }: { onEnter: () => void }) {
  const [value, setValue] = useState('')
  const [show, setShow] = useState(false)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  const [tokenRequired, setTokenRequired] = useState(false)

  const submit = async () => {
    const token = value.trim()
    if (tokenRequired && !token) {
      setErr('服务器已设置 ADMIN_TOKEN，不能留空')
      return
    }
    setLoading(true)
    setErr('')
    setToken(token)
    try {
      await api.detection()
      onEnter()
    } catch (e) {
      setToken('')
      const msg = (e as Error).message ?? ''
      if (msg.toLowerCase().includes('unauthorized') || msg === '401') {
        setTokenRequired(true)
        setErr('Token 错误或未提供，请检查后重试')
      } else {
        setErr(`验证失败：${msg}`)
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="glass rounded-[28px] p-8">
          <div className="flex items-center gap-2 mb-6">
            <Activity className="w-5 h-5" style={{ color: 'var(--ok)' }} />
            <span className="font-semibold" style={{ color: 'var(--text)' }}>管理员认证</span>
          </div>
          <p className="text-sm mb-5" style={{ color: 'var(--muted)' }}>
            输入 <code className="font-mono px-1 rounded" style={{ background: 'var(--card-strong)' }}>ADMIN_TOKEN</code> 访问管理面板。
            {!tokenRequired && <span className="block mt-1 text-xs" style={{ color: 'var(--muted)', opacity: .65 }}>若服务运行在 localhost 且未设置 Token，可直接留空进入。</span>}
          </p>
          <div className="relative mb-3">
            <input
              type={show ? 'text' : 'password'}
              value={value}
              onChange={e => { setValue(e.target.value); setErr('') }}
              onKeyDown={e => e.key === 'Enter' && !loading && submit()}
              placeholder={tokenRequired ? 'Admin Token（必填）' : 'Admin Token（可为空）'}
              className={`${inputCls} pr-9`}
              style={tokenRequired && !value.trim() ? { borderColor: 'rgba(246,196,83,.5)' } : undefined}
              autoFocus
            />
            <button
              onClick={() => setShow(!show)}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 cursor-pointer transition-colors"
              style={{ color: 'var(--muted)' }}
              onMouseEnter={e => (e.currentTarget.style.color = 'var(--text)')}
              onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted)')}
              aria-label={show ? '隐藏 Token' : '显示 Token'}
            >
              {show ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
            </button>
          </div>
          {err && <p className="text-xs mb-3" style={{ color: 'var(--error)' }}>{err}</p>}
          <Btn variant="primary" onClick={submit} loading={loading} className="w-full justify-center">
            进入管理面板
          </Btn>
        </div>
        <div className="text-center mt-4">
          <Link to="/" className="text-xs transition-colors cursor-pointer" style={{ color: 'var(--muted)', opacity: .6 }}>
            返回仪表盘
          </Link>
        </div>
      </div>
    </div>
  )
}

// ── Admin Shell ────────────────────────────────────────────────────────────────

type Tab = 'overview' | 'providers' | 'settings' | 'tasks' | 'config'

const TABS: { id: Tab; label: string; icon: React.ReactNode }[] = [
  { id: 'overview',  label: '检测控制', icon: <Activity className="w-4 h-4" /> },
  { id: 'providers', label: 'Provider',  icon: <Database className="w-4 h-4" /> },
  { id: 'settings',  label: '设置',      icon: <Settings className="w-4 h-4" /> },
  { id: 'tasks',     label: '任务历史',  icon: <Clock className="w-4 h-4" /> },
  { id: 'config',    label: '配置管理',  icon: <FileJson className="w-4 h-4" /> },
]

export default function Admin() {
  const { theme, toggle: toggleTheme } = useTheme()
  const navVisible = useScrollNav()
  const [authed, setAuthed] = useState(false)
  const [verifying, setVerifying] = useState(!!getToken())
  const [tab, setTab] = useState<Tab>('overview')

  useEffect(() => {
    if (!getToken()) return
    api.detection()
      .then(() => setAuthed(true))
      .catch(() => setToken(''))
      .finally(() => setVerifying(false))
  }, [])

  if (verifying) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="w-6 h-6 animate-spin" style={{ color: 'var(--ok)' }} />
      </div>
    )
  }

  if (!authed) {
    return <TokenGate onEnter={() => setAuthed(true)} />
  }

  return (
    <div className="min-h-screen flex flex-col">
      <nav
        className={`fixed top-0 left-0 right-0 z-30 backdrop-blur-glass border-b nav-glass ${navVisible ? 'translate-y-0 opacity-100' : '-translate-y-full opacity-0'}`}
        style={{ borderColor: 'var(--border)' }}
      >
        <div className="max-w-[1180px] mx-auto px-4 h-14 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <Activity className="w-5 h-5" style={{ color: 'var(--ok)' }} />
            <span className="font-semibold" style={{ color: 'var(--text)' }}>管理面板</span>
          </div>
          <div className="flex items-center gap-1">
            <Link
              to="/"
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors cursor-pointer"
              style={{ color: 'var(--muted)' }}
              onMouseEnter={e => (e.currentTarget.style.color = 'var(--text)')}
              onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted)')}
            >
              <ArrowLeft className="w-3.5 h-3.5" />仪表盘
            </Link>
            <button
              onClick={toggleTheme}
              className="p-1.5 rounded-lg transition-colors cursor-pointer"
              style={{ color: 'var(--muted)' }}
              onMouseEnter={e => (e.currentTarget.style.color = 'var(--text)')}
              onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted)')}
              title={theme === 'dark' ? '深色 → 浅色' : theme === 'light' ? '浅色 → 跟随系统' : '跟随系统 → 深色'}
              aria-label="切换主题"
            >
              {theme === 'dark' ? <Moon className="w-4 h-4" /> : theme === 'light' ? <Sun className="w-4 h-4" /> : <Monitor className="w-4 h-4" />}
            </button>
            <button
              onClick={() => { setToken(''); setAuthed(false) }}
              className="p-1.5 rounded-lg transition-colors cursor-pointer"
              style={{ color: 'var(--muted)' }}
              onMouseEnter={e => (e.currentTarget.style.color = 'var(--error)')}
              onMouseLeave={e => (e.currentTarget.style.color = 'var(--muted)')}
              title="退出登录"
            >
              <LogOut className="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </nav>

      <div className="flex flex-1 max-w-[1180px] mx-auto w-full px-4 pt-[80px] pb-6 gap-6">
        <aside className="w-44 shrink-0 hidden sm:block">
          <nav className="space-y-0.5">
            {TABS.map((t, i) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-sm transition-colors cursor-pointer anim-slide-in-left"
                style={{
                  ...(tab === t.id
                    ? { background: 'rgba(56,217,150,.12)', color: 'var(--ok)', fontWeight: 600 }
                    : { color: 'var(--muted)' }),
                  animationDelay: `${i * 45}ms`,
                }}
                onMouseEnter={e => { if (tab !== t.id) (e.currentTarget as HTMLElement).style.color = 'var(--text)' }}
                onMouseLeave={e => { if (tab !== t.id) (e.currentTarget as HTMLElement).style.color = 'var(--muted)' }}
              >
                {t.icon}{t.label}
              </button>
            ))}
          </nav>
        </aside>

        <div className="sm:hidden w-full">
          <div className="flex overflow-x-auto gap-1 pb-3 mb-4" style={{ borderBottom: '1px solid var(--border)' }}>
            {TABS.map((t, i) => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="flex-none flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs whitespace-nowrap transition-colors cursor-pointer anim-slide-in-down"
                style={{
                  ...(tab === t.id
                    ? { background: 'rgba(56,217,150,.12)', color: 'var(--ok)', fontWeight: 600 }
                    : { background: 'var(--card)', color: 'var(--muted)' }),
                  animationDelay: `${i * 40}ms`,
                }}
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>

        <main key={tab} className="flex-1 min-w-0 anim-fade-in">
          {tab === 'overview'  && <OverviewTab />}
          {tab === 'providers' && <ProvidersTab />}
          {tab === 'settings'  && <SettingsTab />}
          {tab === 'tasks'     && <TasksTab />}
          {tab === 'config'    && <ConfigTab />}
        </main>
      </div>
    </div>
  )
}
