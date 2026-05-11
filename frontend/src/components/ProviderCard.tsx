import type { ProviderReport } from '../types'
import { statusClass } from '../utils/status'
import { StatusPill } from './StatusPill'
import { ModelRow } from './ModelRow'

export function ProviderCard({ provider, showError, compact, animDelay = 0 }: {
  provider: ProviderReport
  showError: boolean
  compact?: boolean
  animDelay?: number
}) {
  const sc = statusClass(provider.status)
  const accentColor = sc === 'ok' ? 'var(--ok)' : sc === 'slow' ? 'var(--slow)' : 'var(--error)'
  const accentRgb = sc === 'ok' ? '56,217,150' : sc === 'slow' ? '246,196,83' : '255,107,122'

  return (
    <div className={`glass rounded-[28px] overflow-hidden border-${sc} transition-all duration-300 anim-scale-in`} style={{ animationDelay: `${animDelay}ms` }}>
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
          <span className="flex items-center gap-1.5 text-[11px] font-mono mr-1" style={{ color: 'var(--muted)' }}>
            <span style={{ color: 'var(--ok)' }}>{provider.ok_count}↑</span>
            {provider.slow_count > 0 && <span style={{ color: 'var(--slow)' }}>{provider.slow_count}~</span>}
            {provider.error_count > 0 && <span style={{ color: 'var(--error)' }}>{provider.error_count}✕</span>}
          </span>
          <StatusPill status={provider.status} label={provider.status_label} />
        </div>
      </div>

      <div className={`p-4 ${compact ? 'space-y-1.5' : 'space-y-3'}`}>
        {provider.results.map(result => (
          <ModelRow key={result.model} result={result} showError={showError} compact={compact} />
        ))}
      </div>
    </div>
  )
}
