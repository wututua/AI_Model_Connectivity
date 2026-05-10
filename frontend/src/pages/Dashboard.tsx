import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Settings, RefreshCw, Zap, CheckCircle, AlertTriangle, XCircle, Clock } from 'lucide-react'
import type { Report, ProviderReport, ModelResult } from '../types'
import { api } from '../api'

// ── Helpers ────────────────────────────────────────────────────────────────

function statusBadge(status: string, label: string) {
  const cls =
    status === 'ok' ? 'badge-ok' :
    status === 'slow' ? 'badge-slow' :
    status === 'error' ? 'badge-error' : 'badge-unknown'
  return (
    <span className={`inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-medium font-mono ${cls}`}>
      {label}
    </span>
  )
}

function BarChart({ history }: { history: string[] }) {
  return (
    <div className="flex gap-0.5 mt-2.5">
      {history.map((s, i) => (
        <div
          key={i}
          className={`h-3 flex-1 rounded-sm transition-all ${
            s === 'ok' ? 'bar-ok' :
            s === 'slow' ? 'bar-slow' :
            s === 'error' ? 'bar-error' : 'bar-empty'
          }`}
          title={s}
        />
      ))}
    </div>
  )
}

function CurveChart({ pathLine, pathArea, status }: { pathLine: string; pathArea: string; status: string }) {
  const color = status === 'ok' ? '#22c55e' : status === 'slow' ? '#eab308' : '#ef4444'
  return (
    <svg
      className="absolute inset-x-0 bottom-0 w-full h-16 opacity-[0.12]"
      viewBox="0 0 100 40"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <path d={pathArea} fill={color} />
      <path d={pathLine} fill="none" stroke={color} strokeWidth="2" />
    </svg>
  )
}

function ModelRow({ result, showError }: { result: ModelResult; showError: boolean }) {
  const statusIcon =
    result.status === 'ok' ? <CheckCircle className="w-3.5 h-3.5 text-green-400 shrink-0" /> :
    result.status === 'slow' ? <AlertTriangle className="w-3.5 h-3.5 text-yellow-400 shrink-0" /> :
    <XCircle className="w-3.5 h-3.5 text-red-400 shrink-0" />

  return (
    <div className="relative overflow-hidden px-4 py-3 border-t border-slate-800/60 hover:bg-slate-800/30 transition-colors">
      {result.show_curve_chart && result.svg_path_line && (
        <CurveChart pathLine={result.svg_path_line} pathArea={result.svg_path_area} status={result.status} />
      )}
      <div className="relative z-10">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            {statusIcon}
            <span className="font-mono text-sm text-slate-200 truncate">
              {result.model}
              {result.is_current && (
                <span className="ml-2 text-[10px] text-slate-500 font-sans">[默认]</span>
              )}
            </span>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            {statusBadge(result.status, result.status_label)}
            <span className="text-xs text-slate-500 font-mono w-16 text-right">
              {result.latency_ms > 0 ? `${result.latency_ms} ms` : '—'}
            </span>
          </div>
        </div>

        {showError && result.error && (
          <p className="mt-1 ml-5 text-xs text-red-400/80 font-mono truncate">{result.error}</p>
        )}
        {result.response_preview && !result.error && (
          <p className="mt-1 ml-5 text-xs text-slate-500 truncate">{result.response_preview}</p>
        )}

        <div className="flex items-center gap-4 ml-5 mt-2 text-[11px] text-slate-600 font-mono">
          <span>24h均值 {result.avg_latency_24h}</span>
          <span>{result.weekly_success_text}</span>
          <span>可用率 {result.availability}</span>
        </div>

        {result.history && result.history.length > 0 && (
          <div className="ml-5">
            <BarChart history={result.history} />
          </div>
        )}
      </div>
    </div>
  )
}

function ProviderCard({ provider, showError }: { provider: ProviderReport; showError: boolean }) {
  const borderColor =
    provider.status === 'error' ? 'border-red-500/30' :
    provider.status === 'slow' ? 'border-yellow-500/30' : 'border-slate-700/60'

  const headerGlow =
    provider.status === 'error' ? 'from-red-500/5' :
    provider.status === 'slow' ? 'from-yellow-500/5' : 'from-green-500/5'

  return (
    <div className={`bg-slate-900 border ${borderColor} rounded-2xl overflow-hidden`}>
      <div className={`flex items-center justify-between px-4 py-3.5 bg-gradient-to-r ${headerGlow} to-transparent`}>
        <div className="flex items-center gap-3">
          {provider.provider_logo ? (
            <img
              src={provider.provider_logo}
              alt={provider.provider_name}
              className="w-8 h-8 rounded-xl object-contain bg-slate-800 p-1"
            />
          ) : (
            <div className="w-8 h-8 rounded-xl bg-slate-800 flex items-center justify-center">
              <span className="text-slate-300 text-xs font-bold font-mono">
                {provider.provider_name.slice(0, 2).toUpperCase()}
              </span>
            </div>
          )}
          <div>
            <h3 className="text-sm font-semibold text-white leading-tight">{provider.provider_name}</h3>
            <p className="text-[11px] text-slate-500 font-mono mt-0.5">
              {provider.provider_type} · {provider.model_count} 个模型
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <div className="hidden sm:flex items-center gap-2 text-[11px] text-slate-500 font-mono mr-1">
            <span className="text-green-400">{provider.ok_count}↑</span>
            {provider.slow_count > 0 && <span className="text-yellow-400">{provider.slow_count}~</span>}
            {provider.error_count > 0 && <span className="text-red-400">{provider.error_count}✕</span>}
          </div>
          {statusBadge(provider.status, provider.status_label)}
        </div>
      </div>

      <div>
        {provider.results.map(result => (
          <ModelRow key={result.model} result={result} showError={showError} />
        ))}
      </div>
    </div>
  )
}

function SummaryCard({ icon, label, value, highlight }: {
  icon: React.ReactNode; label: string; value: number | string; highlight?: string
}) {
  return (
    <div className="bg-slate-900 border border-slate-800 rounded-xl px-4 py-3.5">
      <div className="flex items-center gap-2 text-slate-500 mb-2">
        {icon}
        <span className="text-xs">{label}</span>
      </div>
      <p className={`text-2xl font-bold font-mono ${highlight ?? 'text-white'}`}>{value}</p>
    </div>
  )
}

// ── Main page ───────────────────────────────────────────────────────────────

export default function Dashboard() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)

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

  const overallOk = report?.overall_class === 'ok'

  return (
    <div className="min-h-screen bg-slate-950">
      {/* Navbar */}
      <nav className="sticky top-0 z-30 border-b border-slate-800 bg-slate-950/90 backdrop-blur-sm">
        <div className="max-w-7xl mx-auto px-4 h-14 flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <Activity className="w-5 h-5 text-blue-400" />
            <span className="font-semibold text-white">
              {report?.title ?? '模型连通性'}
            </span>
            {live && (
              <span className="inline-flex items-center gap-1 text-[11px] text-green-400 font-mono">
                <span className="w-1.5 h-1.5 rounded-full bg-green-400 animate-pulse" />
                LIVE
              </span>
            )}
          </div>
          <div className="flex items-center gap-3">
            {report && (
              <span className={`hidden sm:inline text-xs font-mono ${overallOk ? 'text-green-400' : 'text-red-400'}`}>
                {report.overall_status}
              </span>
            )}
            <button
              onClick={fetchReport}
              className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800 transition-colors cursor-pointer"
              title="手动刷新"
              aria-label="手动刷新"
            >
              <RefreshCw className="w-4 h-4" />
            </button>
            <Link
              to="/admin"
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 hover:text-white text-sm transition-colors cursor-pointer"
            >
              <Settings className="w-3.5 h-3.5" />
              管理
            </Link>
          </div>
        </div>
      </nav>

      <main className="max-w-7xl mx-auto px-4 py-6 space-y-6">
        {/* Hero status banner */}
        {report && (
          <div className={`rounded-2xl border px-6 py-5 flex flex-col sm:flex-row sm:items-center justify-between gap-4 ${
            overallOk
              ? 'border-green-500/20 bg-green-500/5'
              : 'border-red-500/20 bg-red-500/5'
          }`}>
            <div>
              <div className="flex items-center gap-3">
                <span className={`text-2xl font-bold tracking-tight ${overallOk ? 'text-green-400 glow-ok' : 'text-red-400 glow-error'}`}>
                  {report.overall_status}
                </span>
              </div>
              <p className="text-sm text-slate-400 mt-1">
                更新于 {report.generated_at} · 主题 {report.theme_label} · 并发 {report.global_concurrency}/{report.provider_concurrency}
              </p>
            </div>
            <div className="text-right">
              <p className="text-xs text-slate-500">检测耗时</p>
              <p className="font-mono text-lg text-white">{report.elapsed_ms} ms</p>
            </div>
          </div>
        )}

        {/* Summary cards */}
        {report && (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
            <SummaryCard icon={<Zap className="w-3.5 h-3.5" />} label="总模型" value={report.total} />
            <SummaryCard icon={<CheckCircle className="w-3.5 h-3.5 text-green-400" />} label="正常" value={report.ok_count} highlight="text-green-400" />
            <SummaryCard icon={<AlertTriangle className="w-3.5 h-3.5 text-yellow-400" />} label="较慢" value={report.slow_count} highlight={report.slow_count > 0 ? 'text-yellow-400' : undefined} />
            <SummaryCard icon={<XCircle className="w-3.5 h-3.5 text-red-400" />} label="异常" value={report.error_count} highlight={report.error_count > 0 ? 'text-red-400' : undefined} />
            <SummaryCard icon={<Activity className="w-3.5 h-3.5" />} label="Provider" value={report.provider_count} />
            <SummaryCard icon={<Clock className="w-3.5 h-3.5" />} label="耗时" value={`${report.elapsed_ms} ms`} />
          </div>
        )}

        {/* Provider errors */}
        {report && report.provider_errors && report.provider_errors.length > 0 && (
          <div className="bg-red-500/5 border border-red-500/20 rounded-xl p-4">
            <h2 className="text-sm font-semibold text-red-400 mb-3 flex items-center gap-2">
              <XCircle className="w-4 h-4" /> Provider 错误
            </h2>
            <div className="space-y-1.5">
              {report.provider_errors.map(e => (
                <div key={e.provider_id} className="flex items-start gap-2 text-sm">
                  <span className="text-slate-400 font-mono shrink-0">{e.provider_id}</span>
                  <span className="text-red-400/80 font-mono text-xs">{e.error}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Provider grid */}
        {report && report.providers && report.providers.length > 0 ? (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {report.providers.map(provider => (
              <ProviderCard
                key={provider.provider_id}
                provider={provider}
                showError={true}
              />
            ))}
          </div>
        ) : !error && !report ? (
          <div className="flex flex-col items-center justify-center py-24 text-slate-600">
            <Activity className="w-12 h-12 mb-4 animate-pulse" />
            <p className="text-lg">加载中...</p>
          </div>
        ) : error ? (
          <div className="flex flex-col items-center justify-center py-24 text-slate-500">
            <XCircle className="w-12 h-12 mb-4 text-red-500/50" />
            <p className="text-lg mb-2">无法获取状态</p>
            <p className="text-sm font-mono text-red-400/70">{error}</p>
            <p className="text-xs text-slate-600 mt-3">请先触发一次检测：<code className="font-mono bg-slate-800 px-1.5 py-0.5 rounded">POST /api/admin/check</code></p>
          </div>
        ) : (
          <div className="flex flex-col items-center justify-center py-24 text-slate-600">
            <Activity className="w-12 h-12 mb-4" />
            <p>暂无数据，请触发检测</p>
          </div>
        )}
      </main>
    </div>
  )
}
