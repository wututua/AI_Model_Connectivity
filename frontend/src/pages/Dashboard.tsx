import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Settings, RefreshCw, Zap, CheckCircle, AlertTriangle, XCircle, Clock, Sun, Moon } from 'lucide-react'
import type { Report, ProviderReport, ModelResult } from '../types'
import { api } from '../api'
import { useTheme } from '../hooks/useTheme'
import { useScrollNav } from '../hooks/useScrollNav'

// ── Helpers ────────────────────────────────────────────────────────────────

function statusClass(status: string) {
  return status === 'ok' ? 'ok' : status === 'slow' ? 'slow' : 'error'
}

function StatusPill({ status, label, large }: { status: string; label: string; large?: boolean }) {
  const sz = large ? 'px-4 py-2.5 text-base font-black' : 'px-3 py-1.5 text-xs font-bold'
  return (
    <span
      className={`inline-flex items-center rounded-full border whitespace-nowrap font-mono ${sz} badge-${statusClass(status)}`}
    >
      {label}
    </span>
  )
}

function barCls(s: string) {
  if (s === 'ok') return 'bar-ok'
  if (s === 'slow') return 'bar-slow'
  if (s === 'error') return 'bar-error'
  return 'bar-empty'
}

function StatusLights({ history }: { history: string[] }) {
  return (
    <div className="flex flex-wrap gap-1 mt-2.5">
      {history.map((s, i) => {
        const colored = s === 'ok' || s === 'slow' || s === 'error'
        const glow = s === 'ok'
          ? '0 0 5px rgba(56,217,150,.8)'
          : s === 'slow'
          ? '0 0 5px rgba(246,196,83,.8)'
          : s === 'error'
          ? '0 0 5px rgba(255,107,122,.8)'
          : undefined
        return (
          <div
            key={i}
            className={`w-2.5 h-2.5 rounded-full shrink-0 transition-all ${barCls(s)}`}
            style={colored ? { boxShadow: glow } : undefined}
            title={s}
          />
        )
      })}
    </div>
  )
}

function CurveChart({ pathLine, pathArea, status }: { pathLine: string; pathArea: string; status: string }) {
  const color = status === 'ok' ? 'var(--ok)' : status === 'slow' ? 'var(--slow)' : 'var(--error)'
  return (
    <svg
      className="curve-overlay"
      viewBox="0 0 100 40"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <path d={pathArea} fill={color} opacity=".18" />
      <path d={pathLine} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

function ModelRow({ result, showError }: { result: ModelResult; showError: boolean }) {
  const sc = statusClass(result.status)
  const ledColor = sc === 'ok' ? 'var(--ok)' : sc === 'slow' ? 'var(--slow)' : 'var(--error)'
  const ledGlow = sc === 'ok'
    ? '0 0 6px rgba(56,217,150,.9)'
    : sc === 'slow'
    ? '0 0 6px rgba(246,196,83,.9)'
    : '0 0 6px rgba(255,107,122,.9)'
  const [hovered, setHovered] = useState(false)

  return (
    <div
      className="relative overflow-hidden rounded-[20px] p-4 border transition-all duration-200"
      style={{
        background: hovered ? 'var(--row-hover)' : 'var(--row-bg)',
        borderColor: hovered ? `rgba(${sc === 'ok' ? '56,217,150' : sc === 'slow' ? '246,196,83' : '255,107,122'},.3)` : 'var(--border)',
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      {result.show_curve_chart && result.svg_path_line && (
        <CurveChart pathLine={result.svg_path_line} pathArea={result.svg_path_area} status={result.status} />
      )}

      <div className="relative" style={{ zIndex: 1 }}>
        {/* Model name + status */}
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex items-center gap-2">
            {/* Current-detection LED */}
            <div
              className={`w-2 h-2 rounded-full shrink-0${sc === 'ok' ? ' animate-pulse' : ''}`}
              style={{ background: ledColor, boxShadow: ledGlow }}
            />
            <h3 className="font-mono text-sm font-medium truncate" style={{ color: 'var(--text)' }}>
              {result.model}
              {result.is_current && (
                <em className="ml-2 text-[11px] not-italic" style={{ color: 'var(--muted)' }}>[默认]</em>
              )}
            </h3>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <StatusPill status={result.status} label={result.status_label} />
            <span className="text-xs font-mono w-16 text-right" style={{ color: 'var(--muted)' }}>
              {result.latency_ms > 0 ? `${result.latency_ms} ms` : '—'}
            </span>
          </div>
        </div>

        {/* Error / preview */}
        {showError && result.error && (
          <p className="mt-1.5 text-xs font-mono truncate" style={{ color: 'var(--error)', opacity: .8 }}>
            {result.error}
          </p>
        )}
        {result.response_preview && !result.error && (
          <p className="mt-1.5 text-xs font-mono truncate" style={{ color: 'var(--muted)' }}>
            {result.response_preview}
          </p>
        )}

        {/* Stats */}
        <div className="flex flex-wrap items-center gap-4 mt-2 text-[11px] font-mono" style={{ color: 'var(--muted)', opacity: .75 }}>
          <span>24h均值 {result.avg_latency_24h}</span>
          <span>{result.weekly_success_text}</span>
          <span>可用率 {result.availability}</span>
        </div>

        {/* History LED lights */}
        {result.history && result.history.length > 0 && (
          <StatusLights history={result.history} />
        )}
      </div>
    </div>
  )
}

function ProviderCard({ provider, showError }: { provider: ProviderReport; showError: boolean }) {
  const sc = statusClass(provider.status)
  const accentColor = sc === 'ok' ? 'var(--ok)' : sc === 'slow' ? 'var(--slow)' : 'var(--error)'
  const accentRgb = sc === 'ok' ? '56,217,150' : sc === 'slow' ? '246,196,83' : '255,107,122'

  return (
    <div className={`glass rounded-[28px] overflow-hidden border-${sc} transition-all duration-300`}>
      {/* Header */}
      <div
        className="flex items-center justify-between px-5 py-4"
        style={{
          borderBottom: '1px solid var(--border)',
          background: `linear-gradient(90deg, rgba(${accentRgb},.06), transparent)`,
        }}
      >
        <div className="flex items-center gap-3">
          {provider.provider_logo ? (
            <img
              src={provider.provider_logo}
              alt={provider.provider_name}
              className="w-9 h-9 rounded-xl object-contain"
              style={{ background: 'var(--card-strong)' }}
            />
          ) : (
            <div className="w-9 h-9 rounded-xl flex items-center justify-center text-xs font-black font-mono"
              style={{ background: 'var(--card-strong)', color: accentColor }}>
              {provider.provider_name.slice(0, 2).toUpperCase()}
            </div>
          )}
          <div>
            <h2 className="text-sm font-semibold leading-tight" style={{ color: 'var(--text)' }}>
              {provider.provider_name}
            </h2>
            <p className="text-[11px] font-mono mt-0.5" style={{ color: 'var(--muted)' }}>
              {provider.provider_type} · {provider.model_count} 个模型
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <span className="hidden sm:flex items-center gap-2 text-[11px] font-mono mr-1" style={{ color: 'var(--muted)' }}>
            <span style={{ color: 'var(--ok)' }}>{provider.ok_count}↑</span>
            {provider.slow_count > 0 && <span style={{ color: 'var(--slow)' }}>{provider.slow_count}~</span>}
            {provider.error_count > 0 && <span style={{ color: 'var(--error)' }}>{provider.error_count}✕</span>}
          </span>
          <StatusPill status={provider.status} label={provider.status_label} />
        </div>
      </div>

      {/* Model rows */}
      <div className="p-4 space-y-3">
        {provider.results.map(result => (
          <ModelRow key={result.model} result={result} showError={showError} />
        ))}
      </div>
    </div>
  )
}

function SummaryCard({ icon, label, value, status }: {
  icon: React.ReactNode; label: string; value: number | string; status?: string
}) {
  const valueColor = status ? (status === 'ok' ? 'var(--ok)' : status === 'slow' ? 'var(--slow)' : 'var(--error)') : 'var(--text)'
  return (
    <div className="glass summary-card rounded-[22px] px-4 py-4">
      <div className="flex items-center gap-1.5 mb-2" style={{ color: 'var(--muted)' }}>
        {icon}
        <span className="text-xs uppercase tracking-widest" style={{ letterSpacing: '.16em' }}>{label}</span>
      </div>
      <strong className="block text-2xl font-mono font-bold" style={{ color: valueColor }}>
        {value}
      </strong>
    </div>
  )
}

// ── Main page ───────────────────────────────────────────────────────────────

export default function Dashboard() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)
  const { theme, toggle: toggleTheme } = useTheme()
  const navVisible = useScrollNav()

  const fetchReport = useCallback(() =>
    api.status()
      .then(r => { setReport(r); setError(null) })
      .catch(e => setError((e as Error).message)),
  [])

  useEffect(() => {
    fetchReport()

    if (!window.EventSource) {
      const id = setInterval(fetchReport, 30_000)
      return () => clearInterval(id)
    }

    const es = new EventSource('/api/events')
    es.onmessage = e => {
      setReport(JSON.parse(e.data) as Report)
      setError(null)
      setLive(true)
    }
    es.onerror = () => setLive(false)
    return () => es.close()
  }, [fetchReport])

  const sc = report ? statusClass(report.overall_class) : null

  const navBtnStyle: React.CSSProperties = { color: 'var(--muted)' }
  const navBtnHover = (e: React.MouseEvent<HTMLElement>) => (e.currentTarget.style.color = 'var(--text)')
  const navBtnLeave = (e: React.MouseEvent<HTMLElement>) => (e.currentTarget.style.color = 'var(--muted)')

  return (
    <div className="min-h-screen">
      {/* ── Navbar ─────────────────────────────────────────────── */}
      <nav
        className={`fixed top-0 left-0 right-0 z-30 backdrop-blur-glass border-b nav-glass transition-transform duration-300 ease-in-out ${navVisible ? 'translate-y-0' : '-translate-y-full'}`}
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
              onClick={fetchReport}
              className="p-1.5 rounded-lg transition-colors cursor-pointer"
              style={navBtnStyle}
              onMouseEnter={navBtnHover}
              onMouseLeave={navBtnLeave}
              title="手动刷新"
              aria-label="手动刷新"
            >
              <RefreshCw className="w-4 h-4" />
            </button>
            <button
              onClick={toggleTheme}
              className="p-1.5 rounded-lg transition-colors cursor-pointer"
              style={navBtnStyle}
              onMouseEnter={navBtnHover}
              onMouseLeave={navBtnLeave}
              title={theme === 'dark' ? '切换亮色模式' : '切换暗色模式'}
              aria-label="切换主题"
            >
              {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
            </button>
            <Link
              to="/admin"
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
          <div className="glass rounded-[32px] px-8 py-6 flex flex-col sm:flex-row sm:items-center justify-between gap-6">
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
              <p className="text-sm mt-3" style={{ color: 'var(--muted)' }}>
                更新于 {report.generated_at} · 并发 {report.global_concurrency}/{report.provider_concurrency}
              </p>
            </div>
            <div className="text-right shrink-0">
              <p className="text-xs uppercase tracking-widest mb-1" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>检测耗时</p>
              <p className="text-3xl font-mono font-bold" style={{ color: 'var(--text)' }}>
                {report.elapsed_ms}
                <span className="text-base font-normal ml-1" style={{ color: 'var(--muted)' }}>ms</span>
              </p>
            </div>
          </div>
        )}

        {/* Summary cards – 6 cols */}
        {report && (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3.5">
            <SummaryCard icon={<Zap className="w-3.5 h-3.5" />}           label="总模型"   value={report.total} />
            <SummaryCard icon={<CheckCircle className="w-3.5 h-3.5" />}   label="正常"     value={report.ok_count}    status={report.ok_count > 0 ? 'ok' : undefined} />
            <SummaryCard icon={<AlertTriangle className="w-3.5 h-3.5" />} label="较慢"     value={report.slow_count}  status={report.slow_count > 0 ? 'slow' : undefined} />
            <SummaryCard icon={<XCircle className="w-3.5 h-3.5" />}       label="异常"     value={report.error_count} status={report.error_count > 0 ? 'error' : undefined} />
            <SummaryCard icon={<Activity className="w-3.5 h-3.5" />}      label="Provider" value={report.provider_count} />
            <SummaryCard icon={<Clock className="w-3.5 h-3.5" />}         label="耗时"     value={`${report.elapsed_ms} ms`} />
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
            {report.providers.map(provider => (
              <ProviderCard key={provider.provider_id} provider={provider} showError={true} />
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
            <p className="text-sm font-mono" style={{ color: 'var(--error)', opacity: .7 }}>{error}</p>
            <p className="text-xs mt-3" style={{ color: 'var(--muted)' }}>
              请先触发一次检测：
              <code className="font-mono px-1.5 py-0.5 rounded ml-1" style={{ background: 'var(--card-strong)' }}>POST /api/admin/check</code>
            </p>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-24" style={{ color: 'var(--muted)' }}>
            <Activity className="w-12 h-12 mb-4" />
            <p>暂无数据，请触发检测</p>
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
        </p>
      </footer>
    </div>
  )
}
