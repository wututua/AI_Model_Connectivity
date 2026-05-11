import { useState } from 'react'
import type { ModelResult } from '../types'
import { statusClass } from '../utils/status'
import { StatusPill } from './StatusPill'
import { CurveChart } from './CurveChart'
import { StatusLights } from './StatusLights'

export function ModelRow({ result, showError, compact }: { result: ModelResult; showError: boolean; compact?: boolean }) {
  const sc = statusClass(result.status)
  const ledColor = sc === 'ok' ? 'var(--ok)' : sc === 'slow' ? 'var(--slow)' : 'var(--error)'
  const ledGlow = sc === 'ok'
    ? '0 0 6px rgba(56,217,150,.9)'
    : sc === 'slow'
    ? '0 0 6px rgba(246,196,83,.9)'
    : '0 0 6px rgba(255,107,122,.9)'
  const [hovered, setHovered] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const hasDetail = !!result.error

  if (compact) {
    return (
      <div
        className="flex items-center justify-between gap-3 px-3 py-2 rounded-[14px] border transition-all duration-150"
        style={{
          background: hovered ? 'var(--row-hover)' : 'var(--row-bg)',
          borderColor: hovered ? `rgba(${sc === 'ok' ? '56,217,150' : sc === 'slow' ? '246,196,83' : '255,107,122'},.3)` : 'var(--border)',
        }}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        <div className="flex items-center gap-2 min-w-0">
          <div
            className="w-2 h-2 rounded-full shrink-0"
            style={{ background: ledColor, boxShadow: ledGlow }}
          />
          <span className="font-mono text-xs truncate" style={{ color: 'var(--text)' }}>{result.model}</span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <StatusPill status={result.status} label={result.status_label} />
          <span className="text-[11px] font-mono w-14 text-right" style={{ color: 'var(--muted)' }}>
            {result.latency_ms > 0 ? `${result.latency_ms} ms` : '—'}
          </span>
        </div>
      </div>
    )
  }

  return (
    <div
      className={`relative overflow-hidden rounded-[20px] p-4 border transition-all duration-200 ${hasDetail ? 'cursor-pointer' : ''}`}
      style={{
        background: hovered ? 'var(--row-hover)' : 'var(--row-bg)',
        borderColor: hovered ? `rgba(${sc === 'ok' ? '56,217,150' : sc === 'slow' ? '246,196,83' : '255,107,122'},.3)` : 'var(--border)',
      }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={() => hasDetail && setExpanded(e => !e)}
    >
      {result.show_curve_chart && result.svg_path_line && (
        <CurveChart pathLine={result.svg_path_line} pathArea={result.svg_path_area} status={result.status} />
      )}

      <div className="relative" style={{ zIndex: 1 }}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex items-center gap-2">
            <div
              className="w-2 h-2 rounded-full shrink-0"
              style={{ background: ledColor, boxShadow: ledGlow }}
            />
            <h3 className="font-mono text-sm font-medium truncate" style={{ color: 'var(--text)' }}>
              {result.model}
            </h3>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <StatusPill status={result.status} label={result.status_label} />
            <span className="text-xs font-mono w-16 text-right" style={{ color: 'var(--muted)' }}>
              {result.latency_ms > 0 ? `${result.latency_ms} ms` : '—'}
            </span>
          </div>
        </div>

        {showError && result.error && (
          <p className={`mt-1.5 text-xs font-mono ${expanded ? 'whitespace-pre-wrap break-all' : 'truncate'}`} style={{ color: 'var(--error)', opacity: .8 }}>
            {result.error}
          </p>
        )}

        {hasDetail && !expanded && (
          <p className="text-[10px] mt-1" style={{ color: 'var(--muted)', opacity: .5 }}>点击展开详情</p>
        )}

        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-2 text-[11px] font-mono" style={{ color: 'var(--muted)', opacity: .75 }}>
          <span>24h均值 {result.avg_latency_24h}</span>
          <span className="hidden sm:inline">{result.weekly_success_text}</span>
          <span>可用率 {result.availability}</span>
        </div>

        {result.history && result.history.length > 0 && (
          <StatusLights history={result.history} />
        )}
      </div>
    </div>
  )
}
