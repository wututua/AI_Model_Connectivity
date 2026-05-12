import React, { useState, useRef } from 'react'
import { Loader2, Activity } from 'lucide-react'
import type { SafeProviderConfig, RuntimeSettings } from '../../types'

// ── Auto-clearing message hook ───────────────────────────────────────────────

export function useAutoMsg(delay = 3000) {
  const [msg, setMsgState] = useState('')
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const setMsg = (m: string) => {
    setMsgState(m)
    if (timerRef.current) clearTimeout(timerRef.current)
    if (m && !m.startsWith('错误')) {
      timerRef.current = setTimeout(() => setMsgState(''), delay)
    }
  }
  return [msg, setMsg] as const
}

// ── Shared UI primitives ─────────────────────────────────────────────────────

export function Spinner() {
  return <Loader2 className="w-4 h-4 animate-spin" />
}

export function Btn({
  onClick, disabled, loading, variant = 'default', children, className = '', title,
}: {
  onClick?: () => void
  disabled?: boolean
  loading?: boolean
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  children: React.ReactNode
  className?: string
  title?: string
}) {
  const base = 'inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-sm font-medium transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed'
  const variantStyle: Record<string, React.CSSProperties> = {
    default: { background: 'var(--card-strong)', color: 'var(--text)', border: '1px solid var(--border)' },
    primary: { background: 'rgba(56,217,150,.18)', color: 'var(--ok)', border: '1px solid rgba(56,217,150,.35)' },
    danger:  { background: 'rgba(255,107,122,.12)', color: 'var(--error)', border: '1px solid rgba(255,107,122,.3)' },
    ghost:   { background: 'transparent', color: 'var(--muted)', border: '1px solid transparent' },
  }
  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className={`${base} ${className}`}
      style={variantStyle[variant]}
      title={title}
    >
      {loading ? <Spinner /> : null}
      {children}
    </button>
  )
}

export function Badge({ status }: { status: string }) {
  const map: Record<string, string> = {
    ok: 'badge-ok', success: 'badge-ok',
    slow: 'badge-slow', paused: 'badge-slow',
    error: 'badge-error', canceled: 'badge-error',
    running: 'badge-ok',
  }
  const cls = map[status] ?? 'badge-unknown'
  const labels: Record<string, string> = {
    ok: '正常', success: '成功', slow: '较慢', error: '错误',
    canceled: '已取消', running: '运行中', paused: '已暂停',
  }
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-mono ${cls}`}>
      {labels[status] ?? status}
    </span>
  )
}

export function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs mb-1" style={{ color: 'var(--muted)' }}>{label}</label>
      {children}
      {hint && <p className="text-[11px] mt-1" style={{ color: 'var(--muted)', opacity: .6 }}>{hint}</p>}
    </div>
  )
}

export const inputCls = 'w-full rounded-xl px-3 py-1.5 text-sm font-mono transition-colors input-glass'

// ── Shared helpers ────────────────────────────────────────────────────────────

export function fmtNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(Math.round(n))
}

export function normalizeSettings(s: RuntimeSettings): RuntimeSettings {
  return {
    ...s,
    dashboard_theme: s.dashboard_theme ?? 'default',
    admin_theme: s.admin_theme ?? 'default',
    skip_models: s.skip_models ?? [],
    notify_providers: s.notify_providers ?? [],
    notify_models: s.notify_models ?? [],
  }
}

// ── Token Estimate Card ───────────────────────────────────────────────────────

const TOKENS_PER_MODEL = 40

export function TokenEstimateCard({ providers, settings }: {
  providers: SafeProviderConfig[]
  settings: RuntimeSettings
}) {
  const enabledProviders = providers.filter(p => p.enabled && p.probe_enabled)
  let totalModels: number | null = 0
  for (const p of enabledProviders) {
    const cnt = p.models.length
    if (cnt === 0) {
      if (settings.max_models_per_provider > 0) {
        totalModels = (totalModels ?? 0) + settings.max_models_per_provider
      } else {
        totalModels = null
        break
      }
    } else {
      const cap = settings.max_models_per_provider > 0 ? settings.max_models_per_provider : cnt
      totalModels = (totalModels ?? 0) + Math.min(cnt, cap)
    }
  }

  const minH = settings.auto_check_interval_min_hours
  const maxH = settings.auto_check_interval_max_hours
  const schedulingOn = minH > 0 || maxH > 0
  const avgH = schedulingOn ? ((minH <= 0 ? maxH : minH) + (maxH <= 0 ? minH : maxH)) / 2 : 0
  const dailyChecks = schedulingOn && avgH > 0 ? 24 / avgH : 0

  const modelsKnown = totalModels !== null
  const dailyTokens = modelsKnown && schedulingOn ? totalModels! * TOKENS_PER_MODEL * dailyChecks : null
  const monthlyTokens = dailyTokens !== null ? dailyTokens * 30 : null

  const row = (label: string, value: React.ReactNode, sub?: string) => (
    <div className="flex items-center justify-between py-2 last:border-0" style={{ borderBottom: '1px solid var(--border)' }}>
      <span className="text-xs" style={{ color: 'var(--muted)' }}>{label}</span>
      <span className="text-sm font-mono text-right" style={{ color: 'var(--text)' }}>
        {value}
        {sub && <span className="text-xs ml-1.5" style={{ color: 'var(--muted)' }}>{sub}</span>}
      </span>
    </div>
  )

  return (
    <div className="glass rounded-[22px] p-4">
      <div className="flex items-center gap-1.5 mb-3">
        <Activity className="w-3.5 h-3.5 text-amber-400" />
        <p className="text-xs font-medium text-amber-400">Token 消耗估算</p>
      </div>
      {row('参与检测 Provider', enabledProviders.length, `/ ${providers.length} 个已启用`)}
      {row(
        '探测模型总数',
        modelsKnown ? totalModels : '—',
        modelsKnown
          ? (settings.max_models_per_provider > 0 ? `(上限 ${settings.max_models_per_provider}/Provider)` : '')
          : '含有 Provider 未指定模型列表'
      )}
      {row('每次检测消耗', modelsKnown ? fmtNum(totalModels! * TOKENS_PER_MODEL) : '—', 'tokens（估算）')}
      {row(
        '定时检测频率',
        schedulingOn
          ? `${minH <= 0 ? maxH : minH}h – ${maxH <= 0 ? minH : maxH}h`
          : '已关闭'
      )}
      {schedulingOn && row('每日检测次数', dailyChecks > 0 ? `≈ ${dailyChecks.toFixed(1)} 次` : '—')}
      {row('每日消耗预估', dailyTokens !== null ? fmtNum(dailyTokens) : '—', 'tokens')}
      {row('每月消耗预估', monthlyTokens !== null ? fmtNum(monthlyTokens) : '—', 'tokens')}
      <p className="text-[10px] mt-2" style={{ color: 'var(--muted)', opacity: .55 }}>
        基于每模型约 {TOKENS_PER_MODEL} token（系统提示词 + 探测提示词 + 响应），实际消耗因模型而异。不含手动触发的检测。
      </p>
    </div>
  )
}
