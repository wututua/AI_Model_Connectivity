import { useEffect, useState, useCallback } from 'react'
import {
  Activity, RefreshCw, Settings, CheckCircle2, AlertTriangle, XCircle,
  Server, Zap, Clock,
} from 'lucide-react'
import { api } from '../api'
import type { Report, ModelResult } from '../types'

function badgeClass(status: string): string {
  switch (status) {
    case 'ok':    return 'argon-badge argon-badge-success'
    case 'slow':  return 'argon-badge argon-badge-warning'
    case 'error': return 'argon-badge argon-badge-danger'
    default:      return 'argon-badge argon-badge-default'
  }
}

function dotClass(status: string): string {
  switch (status) {
    case 'ok':    return 'argon-dot argon-dot-success'
    case 'slow':  return 'argon-dot argon-dot-warning'
    case 'error': return 'argon-dot argon-dot-danger'
    default:      return 'argon-dot argon-dot-muted'
  }
}

function relativeTime(iso: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60)    return '刚刚'
  if (diff < 3600)  return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${Math.floor(diff / 86400)} 天前`
}

function StatCard({ icon, label, value, tint }: {
  icon: React.ReactNode
  label: string
  value: number | string
  tint: 'primary' | 'success' | 'warning' | 'danger' | 'info'
}) {
  const tintBg: Record<typeof tint, string> = {
    primary: 'linear-gradient(87deg, #5e72e4 0, #825ee4 100%)',
    success: 'linear-gradient(87deg, #2dce89 0, #2dcecc 100%)',
    warning: 'linear-gradient(87deg, #fb6340 0, #fbb140 100%)',
    danger:  'linear-gradient(87deg, #f5365c 0, #f56036 100%)',
    info:    'linear-gradient(87deg, #11cdef 0, #1171ef 100%)',
  }
  return (
    <div className="argon-card anim-argon-in">
      <div className="argon-card-body">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-[11px] uppercase tracking-[.08em] font-semibold mb-1" style={{ color: 'var(--argon-muted)' }}>
              {label}
            </p>
            <p className="text-2xl font-bold leading-none" style={{ color: 'var(--argon-heading)' }}>
              {value}
            </p>
          </div>
          <div
            className="w-12 h-12 rounded-full flex items-center justify-center text-white shrink-0"
            style={{ background: tintBg[tint], boxShadow: '0 4px 6px rgba(50,50,93,.11), 0 1px 3px rgba(0,0,0,.08)' }}
          >
            {icon}
          </div>
        </div>
      </div>
    </div>
  )
}

function ModelRow({ result }: { result: ModelResult }) {
  return (
    <tr className="hover:bg-[#f6f9fc] transition-colors">
      <td className="px-4 py-3 align-middle">
        <div className="flex items-center gap-2">
          <span className={dotClass(result.status)} />
          <span className="font-mono text-[13px]" style={{ color: 'var(--argon-heading)' }}>{result.model}</span>
        </div>
      </td>
      <td className="px-4 py-3 align-middle">
        <span className={badgeClass(result.status)}>{result.status_label}</span>
      </td>
      <td className="px-4 py-3 align-middle text-right font-mono text-[13px]" style={{ color: 'var(--argon-text)' }}>
        {result.latency_ms > 0 ? `${result.latency_ms} ms` : '—'}
      </td>
      <td className="px-4 py-3 align-middle text-right font-mono text-[13px] hidden sm:table-cell" style={{ color: 'var(--argon-muted)' }}>
        {result.avg_latency_24h || '—'}
      </td>
      <td className="px-4 py-3 align-middle text-right font-mono text-[13px] hidden md:table-cell" style={{ color: 'var(--argon-muted)' }}>
        {result.availability || '—'}
      </td>
    </tr>
  )
}

export default function Dashboard() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  const fetchReport = useCallback(() =>
    api.status()
      .then(r => { setReport(r); setError(null) })
      .catch(e => setError((e as Error).message)),
  [])

  useEffect(() => {
    fetchReport()
    if (!window.EventSource) return
    const es = new EventSource('/api/events')
    es.onmessage = e => {
      try {
        setReport(JSON.parse(e.data) as Report)
        setError(null)
      } catch { /* ignore */ }
    }
    return () => es.close()
  }, [fetchReport])

  const handleRefresh = () => {
    setRefreshing(true)
    fetchReport().finally(() => setRefreshing(false))
  }

  const okRate = report && report.total > 0
    ? Math.round((report.ok_count / report.total) * 100)
    : null

  return (
    <div className="min-h-screen">
      {/* ── Hero ────────────────────────────────────────────────── */}
      <header className="argon-hero pb-32 pt-8">
        <div className="max-w-7xl mx-auto px-6 relative z-10">
          <div className="flex items-center justify-between mb-8">
            <div className="flex items-center gap-2.5">
              <div className="w-9 h-9 rounded-lg bg-white/10 flex items-center justify-center backdrop-blur">
                <Activity className="w-5 h-5 text-white" />
              </div>
              <h1 className="text-lg font-semibold text-white">{report?.title ?? '模型连通性'}</h1>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={handleRefresh}
                disabled={refreshing}
                className="argon-btn argon-btn-secondary !py-2 !px-3 !shadow-none bg-white/10 !text-white hover:!bg-white/20 disabled:opacity-50"
                aria-label="手动刷新"
              >
                <RefreshCw className={`w-4 h-4 ${refreshing ? 'animate-spin' : ''}`} />
              </button>
              {/* Hard navigation: the admin route's theme is decided server-side
                  by admin_theme, not the dashboard SPA's React Router. */}
              <a
                href="/admin"
                className="argon-btn argon-btn-secondary !text-[#32325d] no-underline"
              >
                <Settings className="w-4 h-4" />管理
              </a>
            </div>
          </div>

          <div className="text-white/80 max-w-2xl">
            <p className="text-[13px] uppercase tracking-[.08em] font-semibold mb-2 text-white/60">整体状态</p>
            <h2 className="text-3xl font-bold text-white mb-2">{report?.overall_status ?? '加载中…'}</h2>
            <p className="text-sm text-white/70">
              {report
                ? <>共 {report.provider_count} 个 Provider · {report.total} 个模型 · 更新于 {relativeTime(report.generated_at)}</>
                : '正在拉取最新检测结果…'
              }
            </p>
          </div>
        </div>
      </header>

      {/* ── Content ─────────────────────────────────────────────── */}
      <main className="max-w-7xl mx-auto px-6 -mt-24 relative z-10 pb-12 space-y-6">
        {/* Stat row */}
        <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
          <StatCard icon={<Zap className="w-5 h-5" />}        label="总模型"  value={report?.total ?? '—'}         tint="primary" />
          <StatCard icon={<CheckCircle2 className="w-5 h-5" />} label="正常"    value={report?.ok_count ?? '—'}      tint="success" />
          <StatCard icon={<AlertTriangle className="w-5 h-5" />} label="较慢"    value={report?.slow_count ?? '—'}    tint="warning" />
          <StatCard icon={<XCircle className="w-5 h-5" />}     label="异常"    value={report?.error_count ?? '—'}   tint="danger" />
          <StatCard icon={<Clock className="w-5 h-5" />}       label="可用率"  value={okRate !== null ? `${okRate}%` : '—'} tint="info" />
        </div>

        {/* Error state */}
        {error && (
          <div className="argon-card anim-argon-in" style={{ borderLeft: '4px solid var(--argon-danger)' }}>
            <div className="argon-card-body">
              <div className="flex items-start gap-3">
                <XCircle className="w-5 h-5 mt-0.5 shrink-0" style={{ color: 'var(--argon-danger)' }} />
                <div className="flex-1">
                  <p className="font-semibold mb-1" style={{ color: 'var(--argon-heading)' }}>无法获取状态</p>
                  <p className="text-sm font-mono break-all" style={{ color: 'var(--argon-text)' }}>{error}</p>
                  <a href="/admin" className="argon-btn argon-btn-primary mt-4 no-underline">
                    <Settings className="w-4 h-4" />前往管理面板触发检测
                  </a>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Providers */}
        {report && report.providers && report.providers.length > 0 && (
          <div className="space-y-4">
            {report.providers.map((p, i) => (
              <div key={p.provider_id} className="argon-card anim-argon-in" style={{ animationDelay: `${i * 40}ms` }}>
                <div className="argon-card-header flex items-center justify-between flex-wrap gap-2">
                  <div className="flex items-center gap-3">
                    <div
                      className="w-10 h-10 rounded-lg flex items-center justify-center shrink-0"
                      style={{ background: 'var(--argon-card-soft)', color: 'var(--argon-primary)' }}
                    >
                      <Server className="w-5 h-5" />
                    </div>
                    <div>
                      <h3 className="text-base font-semibold leading-tight" style={{ color: 'var(--argon-heading)' }}>
                        {p.provider_name}
                      </h3>
                      <p className="text-xs font-mono mt-0.5" style={{ color: 'var(--argon-muted)' }}>
                        {p.model_count} 个模型
                      </p>
                    </div>
                  </div>
                  <span className={badgeClass(p.status)}>{p.status_label}</span>
                </div>
                <div className="overflow-x-auto">
                  <table className="w-full">
                    <thead className="bg-[#f6f9fc] text-[10px] uppercase tracking-[.08em] font-semibold" style={{ color: 'var(--argon-muted)' }}>
                      <tr>
                        <th className="px-4 py-3 text-left">模型</th>
                        <th className="px-4 py-3 text-left">状态</th>
                        <th className="px-4 py-3 text-right">延迟</th>
                        <th className="px-4 py-3 text-right hidden sm:table-cell">24h 均值</th>
                        <th className="px-4 py-3 text-right hidden md:table-cell">可用率</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y" style={{ borderColor: 'var(--argon-border)' }}>
                      {p.results.map(r => <ModelRow key={r.model} result={r} />)}
                    </tbody>
                  </table>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Empty state */}
        {!error && report && (!report.providers || report.providers.length === 0) && (
          <div className="argon-card anim-argon-in">
            <div className="argon-card-body text-center py-16">
              <Activity className="w-12 h-12 mx-auto mb-3" style={{ color: 'var(--argon-muted)', opacity: .5 }} />
              <p className="mb-4" style={{ color: 'var(--argon-text)' }}>暂无数据</p>
              <a href="/admin" className="argon-btn argon-btn-primary no-underline">
                <Settings className="w-4 h-4" />前往管理面板触发检测
              </a>
            </div>
          </div>
        )}

        {/* Loading */}
        {!report && !error && (
          <div className="argon-card anim-argon-in">
            <div className="argon-card-body text-center py-16" style={{ color: 'var(--argon-muted)' }}>
              <Activity className="w-12 h-12 mx-auto mb-3 animate-pulse" />
              <p>加载中…</p>
            </div>
          </div>
        )}

        <footer className="text-center text-xs pt-4" style={{ color: 'var(--argon-muted)' }}>
          Argon Theme · Powered by AI Model Connectivity
        </footer>
      </main>
    </div>
  )
}
