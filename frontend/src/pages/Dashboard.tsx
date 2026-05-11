import { useEffect, useState, useCallback, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Settings, RefreshCw, Zap, CheckCircle, AlertTriangle, XCircle, Clock, Sun, Moon, Monitor, Search, X, ArrowUpDown, LayoutDashboard, List } from 'lucide-react'
import type { Report, ProviderReport } from '../types'
import { api } from '../api'
import { useTheme } from '../hooks/useTheme'
import { useScrollNav } from '../hooks/useScrollNav'
import { useNavTransition } from '../hooks/useNavTransition'
import { relativeTime, statusClass } from '../utils/status'
import { StatusPill } from '../components/StatusPill'
import { ProviderCard } from '../components/ProviderCard'
import { SummaryCard } from '../components/SummaryCard'

// ── Main page ───────────────────────────────────────────────────────────────

export default function Dashboard() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'ok' | 'slow' | 'error'>('all')
  const [sortBy, setSortBy] = useState<'default' | 'status' | 'name' | 'latency' | 'models'>('default')
  const [viewMode, setViewMode] = useState<'detailed' | 'compact'>('detailed')
  const { theme, toggle: toggleTheme } = useTheme()
  const navVisible = useScrollNav()
  const navTo = useNavTransition()

  const fetchReport = useCallback(() =>
    api.status()
      .then(r => { setReport(r); setError(null) })
      .catch(e => setError((e as Error).message)),
  [])

  const handleRefresh = useCallback(() => {
    setRefreshing(true)
    fetchReport().finally(() => setRefreshing(false))
  }, [fetchReport])

  useEffect(() => {
    fetchReport()

    if (!window.EventSource) {
      // No SSE support: exponential backoff polling (30s → 60s → 120s)
      let delay = 30_000
      let timerId: ReturnType<typeof setTimeout>
      const poll = () => {
        fetchReport()
        delay = Math.min(delay * 2, 120_000)
        timerId = setTimeout(poll, delay)
      }
      timerId = setTimeout(poll, delay)
      return () => clearTimeout(timerId)
    }

    // SSE with exponential backoff reconnect on error
    let es: EventSource | null = null
    let reconnectDelay = 30_000
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let closed = false

    const connect = () => {
      es = new EventSource('/api/events')
      es.onmessage = e => {
        setReport(JSON.parse(e.data) as Report)
        setError(null)
        setLive(true)
        reconnectDelay = 30_000
      }
      es.onerror = () => {
        setLive(false)
        es?.close()
        if (!closed) {
          reconnectTimer = setTimeout(() => {
            reconnectDelay = Math.min(reconnectDelay * 2, 120_000)
            connect()
          }, reconnectDelay)
        }
      }
    }

    connect()
    return () => {
      closed = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [fetchReport])

  const sc = report ? statusClass(report.overall_class) : null

  const filteredProviders = useMemo(() => {
    if (!report?.providers) return []
    const q = search.trim().toLowerCase()
    const STATUS_ORDER = { error: 0, slow: 1, ok: 2 }

    const avgLatency = (p: ProviderReport) => {
      const valid = p.results.filter(r => r.latency_ms > 0)
      return valid.length ? valid.reduce((s, r) => s + r.latency_ms, 0) / valid.length : Infinity
    }

    return report.providers
      .map(p => ({
        ...p,
        results: p.results.filter(r =>
          (statusFilter === 'all' || r.status === statusFilter) &&
          (!q || r.model.toLowerCase().includes(q) || p.provider_name.toLowerCase().includes(q))
        ),
      }))
      .filter(p => p.results.length > 0 || (!q && statusFilter === 'all'))
      .sort((a, b) => {
        switch (sortBy) {
          case 'name':    return a.provider_name.localeCompare(b.provider_name)
          case 'latency': return avgLatency(a) - avgLatency(b)
          case 'models':  return b.model_count - a.model_count
          case 'status':  return (STATUS_ORDER[a.status as keyof typeof STATUS_ORDER] ?? 3)
                               - (STATUS_ORDER[b.status as keyof typeof STATUS_ORDER] ?? 3)
          default:        return 0  // 保持 API 返回的原始顺序
        }
      })
  }, [report, search, statusFilter, sortBy])

  const navBtnStyle: React.CSSProperties = { color: 'var(--muted)' }
  const navBtnHover = (e: React.MouseEvent<HTMLElement>) => (e.currentTarget.style.color = 'var(--text)')
  const navBtnLeave = (e: React.MouseEvent<HTMLElement>) => (e.currentTarget.style.color = 'var(--muted)')

  return (
    <div className="min-h-screen">
      {/* ── Navbar ─────────────────────────────────────────────── */}
      <nav
        className={`fixed top-0 left-0 right-0 z-30 backdrop-blur-glass border-b nav-glass ${navVisible ? 'translate-y-0 opacity-100' : '-translate-y-full opacity-0'}`}
        style={{ borderColor: 'var(--border)' }}
      >
        <div className="max-w-[1180px] mx-auto px-4 h-14 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Activity className="w-5 h-5" style={{ color: 'var(--ok)' }} />
            <span className="font-semibold" style={{ color: 'var(--text)' }}>
              {report?.title ?? '模型连通性'}
            </span>
            {live && (
              <span className="inline-flex items-center gap-1.5 text-[11px] font-mono" style={{ color: 'var(--ok)' }}>
                <span className="w-1.5 h-1.5 rounded-full animate-pulse" style={{ background: 'var(--ok)' }} />
                LIVE
              </span>
            )}
          </div>

          <div className="flex items-center gap-1">
            {report && sc && (
              <span className={`hidden sm:inline text-xs font-mono mr-2 text-${sc}`}>
                {report.overall_status}
              </span>
            )}
            <button
              onClick={handleRefresh}
              disabled={refreshing}
              className="p-1.5 rounded-lg transition-colors cursor-pointer disabled:opacity-50"
              style={navBtnStyle}
              onMouseEnter={navBtnHover}
              onMouseLeave={navBtnLeave}
              title="手动刷新"
              aria-label="手动刷新"
            >
              <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
            </button>
            <button
              onClick={toggleTheme}
              className="p-1.5 rounded-lg transition-colors cursor-pointer"
              style={navBtnStyle}
              onMouseEnter={navBtnHover}
              onMouseLeave={navBtnLeave}
              title={theme === 'dark' ? '深色 → 浅色' : theme === 'light' ? '浅色 → 跟随系统' : '跟随系统 → 深色'}
              aria-label="切换主题"
            >
              {theme === 'dark' ? <Moon className="w-4 h-4" /> : theme === 'light' ? <Sun className="w-4 h-4" /> : <Monitor className="w-4 h-4" />}
            </button>
            <Link
              to="/admin"
              onClick={e => { e.preventDefault(); navTo('/admin') }}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors cursor-pointer glass ml-1"
              style={navBtnStyle}
              onMouseEnter={navBtnHover}
              onMouseLeave={navBtnLeave}
            >
              <Settings className="w-3.5 h-3.5" />
              管理
            </Link>
          </div>
        </div>
      </nav>

      {/* ── Content ────────────────────────────────────────────── */}
      <main className="max-w-[1180px] mx-auto px-4 pt-[96px] pb-10 space-y-5">

        {/* Hero banner */}
        {report && sc && (
          <div className="glass rounded-[32px] px-8 py-6 flex flex-col sm:flex-row sm:items-center justify-between gap-6 anim-fade-in-up" style={{ animationDelay: '0ms' }}>
            <div>
              <p className="text-xs uppercase tracking-widest mb-2" style={{ color: 'var(--muted)', letterSpacing: '.16em' }}>
                整体状态
              </p>
              <div className="flex items-center gap-4">
                <span
                  className={`text-[clamp(28px,5vw,48px)] font-black tracking-tight leading-none glow-${sc}`}
                  style={{ color: `var(--${sc})`, letterSpacing: '-.04em' }}
                >
                  {report.overall_status}
                </span>
                <StatusPill status={report.overall_class} label={report.overall_class.toUpperCase()} large />
              </div>
              {(() => {
                const { text: timeText, stale } = relativeTime(report.generated_at)
                return (
                  <p className="text-sm mt-3" style={{ color: stale ? 'var(--slow)' : 'var(--muted)' }}>
                    更新于 <span title={report.generated_at}>{timeText}</span>
                    {stale && ' ⚠'}
                    {' · '}并发 {report.global_concurrency}/{report.provider_concurrency}
                  </p>
                )
              })()}
            </div>
            <div className="text-right shrink-0 flex flex-col gap-3 sm:gap-4">
              <div>
                <p className="text-xs uppercase tracking-widest mb-1" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>检测耗时</p>
                <p className="text-3xl font-mono font-bold" style={{ color: 'var(--text)' }}>
                  {report.elapsed_ms}
                  <span className="text-base font-normal ml-1" style={{ color: 'var(--muted)' }}>ms</span>
                </p>
              </div>
              {report.total > 0 && (
                <div>
                  <p className="text-xs uppercase tracking-widest mb-1" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>可用率</p>
                  <p className="text-3xl font-mono font-bold" style={{ color: report.ok_count === report.total ? 'var(--ok)' : report.error_count > 0 ? 'var(--error)' : 'var(--slow)' }}>
                    {Math.round((report.ok_count / report.total) * 100)}
                    <span className="text-base font-normal ml-0.5">%</span>
                  </p>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Summary cards – 6 cols */}
        {report && (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3.5">
            <SummaryCard icon={<Zap className="w-3.5 h-3.5" />}           label="总模型"   value={report.total}                                                     animDelay={80} />
            <SummaryCard icon={<CheckCircle className="w-3.5 h-3.5" />}   label="正常"     value={report.ok_count}    status={report.ok_count > 0 ? 'ok' : undefined}    animDelay={130} />
            <SummaryCard icon={<AlertTriangle className="w-3.5 h-3.5" />} label="较慢"     value={report.slow_count}  status={report.slow_count > 0 ? 'slow' : undefined}  animDelay={180} />
            <SummaryCard icon={<XCircle className="w-3.5 h-3.5" />}       label="异常"     value={report.error_count} status={report.error_count > 0 ? 'error' : undefined} animDelay={230} />
            <SummaryCard icon={<Activity className="w-3.5 h-3.5" />}      label="Provider" value={report.provider_count}                                              animDelay={280} />
            <SummaryCard icon={<Clock className="w-3.5 h-3.5" />}         label="耗时"     value={`${report.elapsed_ms} ms`}                                          animDelay={330} />
          </div>
        )}

        {/* Search & filter bar */}
        {report?.providers && report.providers.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 anim-fade-in" style={{ animationDelay: '380ms' }}>
            <div className="relative flex-1 min-w-[160px] max-w-xs">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 pointer-events-none" style={{ color: 'var(--muted)' }} />
              <input
                className="input-glass w-full rounded-xl pl-8 pr-8 py-2 text-sm"
                placeholder="搜索模型或 Provider…"
                value={search}
                onChange={e => setSearch(e.target.value)}
              />
              {search && (
                <button onClick={() => setSearch('')} className="absolute right-2.5 top-1/2 -translate-y-1/2 cursor-pointer" style={{ color: 'var(--muted)' }}>
                  <X className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
            {(['all', 'ok', 'slow', 'error'] as const).map(s => (
              <button
                key={s}
                onClick={() => setStatusFilter(s)}
                className="px-3 py-1.5 rounded-xl text-xs font-medium transition-colors cursor-pointer"
                style={statusFilter === s
                  ? { background: s === 'all' ? 'var(--card-strong)' : `rgba(${s === 'ok' ? '56,217,150' : s === 'slow' ? '246,196,83' : '255,107,122'},.2)`, color: s === 'all' ? 'var(--text)' : `var(--${s})`, border: `1px solid ${s === 'all' ? 'var(--border)' : `rgba(${s === 'ok' ? '56,217,150' : s === 'slow' ? '246,196,83' : '255,107,122'},.4)`}` }
                  : { background: 'transparent', color: 'var(--muted)', border: '1px solid transparent' }
                }
              >
                {{ all: '全部', ok: '正常', slow: '较慢', error: '异常' }[s]}
              </button>
            ))}

            {/* Sort selector */}
            <div className="flex items-center gap-1.5 ml-auto" style={{ color: 'var(--muted)' }}>
              <ArrowUpDown className="w-3.5 h-3.5 shrink-0" />
              <select
                value={sortBy}
                onChange={e => setSortBy(e.target.value as typeof sortBy)}
                className="input-glass rounded-xl px-2 py-1.5 text-xs cursor-pointer focus:outline-none"
              >
                <option value="default">默认顺序</option>
                <option value="status">按状态</option>
                <option value="name">按名称</option>
                <option value="latency">按延迟</option>
                <option value="models">按模型数</option>
              </select>
            </div>

            {/* View toggle */}
            <div className="flex items-center rounded-xl overflow-hidden border" style={{ borderColor: 'var(--border)' }}>
              <button
                onClick={() => setViewMode('detailed')}
                className="p-1.5 transition-colors cursor-pointer"
                style={viewMode === 'detailed' ? { background: 'var(--card-strong)', color: 'var(--text)' } : { color: 'var(--muted)' }}
                title="详细视图"
              >
                <LayoutDashboard className="w-4 h-4" />
              </button>
              <button
                onClick={() => setViewMode('compact')}
                className="p-1.5 transition-colors cursor-pointer"
                style={viewMode === 'compact' ? { background: 'var(--card-strong)', color: 'var(--text)' } : { color: 'var(--muted)' }}
                title="简洁视图"
              >
                <List className="w-4 h-4" />
              </button>
            </div>
          </div>
        )}

        {/* Provider errors */}
        {report?.provider_errors && report.provider_errors.length > 0 && (
          <div className="glass rounded-[24px] p-5" style={{ borderColor: 'rgba(255,107,122,.3)' }}>
            <h2 className="text-sm font-semibold flex items-center gap-2 mb-3" style={{ color: 'var(--error)' }}>
              <XCircle className="w-4 h-4" /> Provider 错误
            </h2>
            <div className="space-y-2">
              {report.provider_errors.map(e => (
                <div key={e.provider_id} className="flex items-start gap-2 text-sm">
                  <span className="font-mono shrink-0" style={{ color: 'var(--muted)' }}>{e.provider_id}</span>
                  <span className="font-mono text-xs" style={{ color: 'var(--error)', opacity: .8 }}>{e.error}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Provider grid */}
        {report?.providers && report.providers.length > 0 ? (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-[18px]">
            {filteredProviders.length === 0 ? (
              <div className="col-span-2 py-16 text-center" style={{ color: 'var(--muted)' }}>
                <Search className="w-8 h-8 mx-auto mb-3 opacity-40" />
                <p className="text-sm">未找到匹配的模型或 Provider</p>
              </div>
            ) : filteredProviders.map((provider, i) => (
              <ProviderCard key={provider.provider_id} provider={provider} showError={true} compact={viewMode === 'compact'} animDelay={440 + i * 80} />
            ))}
          </div>
        ) : !error && !report ? (
          <div className="flex flex-col items-center justify-center py-24" style={{ color: 'var(--muted)' }}>
            <Activity className="w-12 h-12 mb-4 animate-pulse" />
            <p className="text-lg">加载中...</p>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-24" style={{ color: 'var(--muted)' }}>
            <XCircle className="w-12 h-12 mb-4" style={{ color: 'var(--error)', opacity: .5 }} />
            <p className="text-lg mb-2">无法获取状态</p>
            <p className="text-sm font-mono mb-4" style={{ color: 'var(--error)', opacity: .7 }}>{error}</p>
            <Link
              to="/admin"
              onClick={e => { e.preventDefault(); navTo('/admin') }}
              className="flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium transition-colors cursor-pointer glass"
              style={{ color: 'var(--ok)', border: '1px solid rgba(56,217,150,.35)' }}
            >
              <Settings className="w-4 h-4" />前往管理面板触发检测
            </Link>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-24" style={{ color: 'var(--muted)' }}>
            <Activity className="w-12 h-12 mb-4" />
            <p className="mb-4">暂无数据</p>
            <Link
              to="/admin"
              onClick={e => { e.preventDefault(); navTo('/admin') }}
              className="flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium transition-colors cursor-pointer glass"
              style={{ color: 'var(--ok)', border: '1px solid rgba(56,217,150,.35)' }}
            >
              <Settings className="w-4 h-4" />前往管理面板触发检测
            </Link>
          </div>
        )}
      </main>

      <footer className="max-w-[1180px] mx-auto px-4 py-6 flex items-center justify-center">
        <p className="text-[11px] font-mono" style={{ color: 'var(--muted)', opacity: .5 }}>
          © {new Date().getFullYear()}&nbsp;
          <a
            href="https://github.com/wututua/AI_Model_Connectivity"
            target="_blank"
            rel="noopener noreferrer"
            className="transition-opacity hover:opacity-100"
          >
            AI Model Connectivity
          </a>
          &nbsp;by&nbsp;
          <a
            href="https://github.com/wututua"
            target="_blank"
            rel="noopener noreferrer"
            className="transition-opacity hover:opacity-100"
          >
            wututu
          </a>
          &nbsp;&amp;&nbsp;
          <a
            href="https://github.com/Meow-Calculations"
            target="_blank"
            rel="noopener noreferrer"
            className="transition-opacity hover:opacity-100"
          >
            Meow-Calculations
          </a>
        </p>
      </footer>
    </div>
  )
}
