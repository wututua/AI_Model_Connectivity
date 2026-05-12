import { useState, useEffect, useCallback, useMemo } from 'react'
import { RefreshCw, Coins } from 'lucide-react'
import { api } from '../../api'
import type { BillingSummary } from '../../types'
import { Btn } from './shared'

const RANGES: { label: string; days: number }[] = [
  { label: '7 天',  days: 7 },
  { label: '30 天', days: 30 },
  { label: '90 天', days: 90 },
]

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`
  if (n >= 1_000)     return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

export function BillingTab() {
  const [days, setDays] = useState(30)
  const [data, setData] = useState<BillingSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')

  const load = useCallback(() => {
    setLoading(true); setErr('')
    api.billing(days)
      .then(setData)
      .catch(e => setErr((e as Error).message))
      .finally(() => setLoading(false))
  }, [days])

  useEffect(() => { load() }, [load])

  // Daily trend visualization: pre-compute SVG path on the fly.
  const dailySvg = useMemo(() => {
    if (!data?.daily.length) return null
    const W = 560, H = 100, P = 6
    const max = Math.max(...data.daily.map(d => d.total_tokens), 1)
    const step = data.daily.length > 1 ? (W - P * 2) / (data.daily.length - 1) : 0
    const points = data.daily.map((d, i) => {
      const x = P + i * step
      const y = H - P - ((d.total_tokens / max) * (H - P * 2))
      return `${x.toFixed(1)},${y.toFixed(1)}`
    })
    const line = points.length === 1
      ? `M ${points[0]}`
      : `M ${points.join(' L ')}`
    const area = `${line} L ${(P + (data.daily.length - 1) * step).toFixed(1)},${H - P} L ${P},${H - P} Z`
    return { line, area, max, width: W, height: H }
  }, [data])

  return (
    <div className="space-y-4 anim-fade-in">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <h2 className="text-base font-semibold flex items-center gap-2" style={{ color: 'var(--text)' }}>
          <Coins className="w-4 h-4" style={{ color: 'var(--slow)' }} />
          Token 消耗
        </h2>
        <div className="flex items-center gap-1">
          {RANGES.map(r => (
            <button
              key={r.days}
              onClick={() => setDays(r.days)}
              className="px-2.5 py-1 rounded-lg text-xs cursor-pointer transition-colors"
              style={days === r.days
                ? { background: 'rgba(56,217,150,.15)', color: 'var(--ok)', fontWeight: 600 }
                : { color: 'var(--muted)' }}
            >
              {r.label}
            </button>
          ))}
          <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" /></Btn>
        </div>
      </div>

      {err && <p className="text-xs" style={{ color: 'var(--error)' }}>{err}</p>}

      {data && (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <SummaryStat label="总 Token" value={formatTokens(data.total_tokens)} accent="var(--text)" />
            <SummaryStat label="Prompt" value={formatTokens(data.total_prompt_tokens)} accent="var(--muted)" />
            <SummaryStat label="Completion" value={formatTokens(data.total_completion_tokens)} accent="var(--muted)" />
            <SummaryStat label="探测次数" value={String(data.total_probe_count)} accent="var(--muted)" />
          </div>

          {dailySvg && (
            <div className="glass rounded-[18px] p-4">
              <div className="flex items-baseline justify-between mb-2">
                <span className="text-xs font-medium" style={{ color: 'var(--muted)' }}>每日 Token 趋势</span>
                <span className="text-[10px] font-mono" style={{ color: 'var(--muted)', opacity: .6 }}>
                  峰值 {formatTokens(dailySvg.max)} · {data.daily.length} 天采样
                </span>
              </div>
              <svg viewBox={`0 0 ${dailySvg.width} ${dailySvg.height}`} className="w-full h-auto">
                <path d={dailySvg.area} fill="var(--slow)" opacity="0.18" />
                <path d={dailySvg.line} stroke="var(--slow)" strokeWidth="1.6" fill="none" strokeLinejoin="round" strokeLinecap="round" />
              </svg>
            </div>
          )}

          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs text-left" style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)' }}>
                  <th className="pb-2 pr-3 font-medium">Provider</th>
                  <th className="pb-2 pr-3 font-medium">Model</th>
                  <th className="pb-2 pr-3 font-medium text-right hidden sm:table-cell">Prompt</th>
                  <th className="pb-2 pr-3 font-medium text-right hidden sm:table-cell">Completion</th>
                  <th className="pb-2 pr-3 font-medium text-right">Total</th>
                  <th className="pb-2 font-medium text-right">次数</th>
                </tr>
              </thead>
              <tbody>
                {data.per_model.map(m => (
                  <tr
                    key={`${m.provider_id}::${m.model}`}
                    className="transition-colors"
                    style={{ borderBottom: '1px solid var(--border)' }}
                    onMouseEnter={e => (e.currentTarget.style.background = 'var(--row-hover)')}
                    onMouseLeave={e => (e.currentTarget.style.background = '')}
                  >
                    <td className="py-2.5 pr-3 text-xs">
                      <span style={{ color: 'var(--text)' }}>{m.provider_name || m.provider_id}</span>
                    </td>
                    <td className="py-2.5 pr-3 text-xs font-mono" style={{ color: 'var(--muted)' }}>{m.model}</td>
                    <td className="py-2.5 pr-3 text-xs font-mono text-right hidden sm:table-cell" style={{ color: 'var(--muted)' }}>
                      {formatTokens(m.prompt_tokens)}
                    </td>
                    <td className="py-2.5 pr-3 text-xs font-mono text-right hidden sm:table-cell" style={{ color: 'var(--muted)' }}>
                      {formatTokens(m.completion_tokens)}
                    </td>
                    <td className="py-2.5 pr-3 text-xs font-mono text-right" style={{ color: 'var(--text)', fontWeight: 600 }}>
                      {formatTokens(m.total_tokens)}
                    </td>
                    <td className="py-2.5 text-xs font-mono text-right" style={{ color: 'var(--muted)' }}>{m.probe_count}</td>
                  </tr>
                ))}
                {data.per_model.length === 0 && (
                  <tr>
                    <td colSpan={6} className="py-12 text-center" style={{ color: 'var(--muted)', opacity: .5 }}>
                      {loading ? '加载中…' : '所选时间窗口内暂无 Token 数据'}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          <p className="text-[11px]" style={{ color: 'var(--muted)', opacity: .6 }}>
            数据来源于每次探测响应中的 <code className="font-mono">usage</code> 字段；部分本地推理服务或代理可能不返回该字段，此时记为 0。
          </p>
        </>
      )}
    </div>
  )
}

function SummaryStat({ label, value, accent }: { label: string; value: string; accent: string }) {
  return (
    <div className="glass rounded-[18px] px-4 py-3">
      <div className="text-[10px] uppercase tracking-widest mb-1" style={{ color: 'var(--muted)', letterSpacing: '.16em' }}>{label}</div>
      <div className="text-xl font-mono font-bold" style={{ color: accent }}>{value}</div>
    </div>
  )
}
