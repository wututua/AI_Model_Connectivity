import React, { useState, useEffect, useCallback, useRef } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity, ArrowLeft, Play, Square, RefreshCw, Plus, Edit2, Trash2,
  Save, Download, Upload, RotateCcw, CheckCircle, XCircle,
  Clock, Loader2, LogOut, Eye, EyeOff, Sun, Moon, Settings, FileJson, Database,
} from 'lucide-react'
import { api, getToken, setToken } from '../api'
import { useTheme } from '../hooks/useTheme'
import { useScrollNav } from '../hooks/useScrollNav'
import type {
  RunningState, SafeProviderConfig, ProviderUpdate,
  CheckTask, RuntimeSettings, ConfigExport,
} from '../types'

// ── Helpers ─────────────────────────────────────────────────────────────────

function useAutoMsg(delay = 3000) {
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

// ── Shared UI ───────────────────────────────────────────────────────────────

function Spinner() {
  return <Loader2 className="w-4 h-4 animate-spin" />
}

function Btn({
  onClick, disabled, loading, variant = 'default', children, className = '', title,
}: {
  onClick?: () => void; disabled?: boolean; loading?: boolean
  variant?: 'default' | 'primary' | 'danger' | 'ghost'
  children: React.ReactNode; className?: string; title?: string
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

function Badge({ status }: { status: string }) {
  const map: Record<string, string> = {
    ok: 'badge-ok', success: 'badge-ok',
    slow: 'badge-slow',
    error: 'badge-error', canceled: 'badge-error',
    running: 'badge-ok',
  }
  const cls = map[status] ?? 'badge-unknown'
  const labels: Record<string, string> = {
    ok: '正常', success: '成功', slow: '较慢', error: '错误',
    canceled: '已取消', running: '运行中',
  }
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-mono ${cls}`}>
      {labels[status] ?? status}
    </span>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label className="block text-xs mb-1" style={{ color: 'var(--muted)' }}>{label}</label>
      {children}
      {hint && <p className="text-[11px] mt-1" style={{ color: 'var(--muted)', opacity: .6 }}>{hint}</p>}
    </div>
  )
}

const inputCls = 'w-full rounded-xl px-3 py-1.5 text-sm font-mono transition-colors input-glass'

// ── Token Gate ──────────────────────────────────────────────────────────────

function TokenGate({ onEnter }: { onEnter: () => void }) {
  const [value, setValue] = useState('')
  const [show, setShow] = useState(false)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(false)
  // tracks whether the server requires a token (set to true after first 401)
  const [tokenRequired, setTokenRequired] = useState(false)

  const submit = async () => {
    const token = value.trim()
    if (tokenRequired && !token) {
      setErr('服务器已设置 ADMIN_TOKEN，不能留空')
      return
    }
    setLoading(true)
    setErr('')
    // set token for the verification request only; clear on failure
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

// ── Token Estimator ──────────────────────────────────────────────────────────

const TOKENS_PER_MODEL = 40 // conservative: ~35 input (prompts) + ~5 output

function fmtNum(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(Math.round(n))
}

function TokenEstimateCard({ providers, settings }: {
  providers: SafeProviderConfig[]
  settings: RuntimeSettings
}) {
  const enabledProviders = providers.filter(p => p.enabled)
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
      {row('启用 Provider', enabledProviders.length, `/ ${providers.length} 个`)}
      {row(
        '探测模型总数',
        modelsKnown ? totalModels : '—',
        modelsKnown
          ? (settings.max_models_per_provider > 0 ? `(上限 ${settings.max_models_per_provider}/Provider)` : '')
          : '含有 Provider 未指定模型列表'
      )}
      {row(
        '每次检测消耗',
        modelsKnown ? fmtNum(totalModels! * TOKENS_PER_MODEL) : '—',
        'tokens（估算）'
      )}
      {row(
        '定时检测频率',
        schedulingOn
          ? `${minH <= 0 ? maxH : minH}h – ${maxH <= 0 ? minH : maxH}h`
          : '已关闭'
      )}
      {schedulingOn && row(
        '每日检测次数',
        dailyChecks > 0 ? `≈ ${dailyChecks.toFixed(1)} 次` : '—'
      )}
      {row(
        '每日消耗预估',
        dailyTokens !== null ? fmtNum(dailyTokens) : '—',
        'tokens'
      )}
      {row(
        '每月消耗预估',
        monthlyTokens !== null ? fmtNum(monthlyTokens) : '—',
        'tokens'
      )}
      <p className="text-[10px] mt-2" style={{ color: 'var(--muted)', opacity: .55 }}>
        基于每模型约 {TOKENS_PER_MODEL} token（系统提示词 + 探测提示词 + 响应），实际消耗因模型而异。
        不含手动触发的检测。
      </p>
    </div>
  )
}

// ── Overview Tab ─────────────────────────────────────────────────────────────

function OverviewTab() {
  const [state, setState] = useState<RunningState | null>(null)
  const [cfg, setCfg] = useState<{ providers: SafeProviderConfig[]; settings: RuntimeSettings } | null>(null)
  const [summary, setSummary] = useState<import('../types').Report | null>(null)
  const [loading, setLoading] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)
  const [msg, setMsg] = useAutoMsg()

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([api.detection(), api.config(), api.status().catch(() => null)])
      .then(([s, c, rep]) => {
        setState(s)
        setCfg({ providers: c.providers, settings: normalizeSettings(c.settings) })
        setSummary(rep)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const withMsg = async (fn: () => Promise<unknown>, successMsg: string) => {
    setActionLoading(true)
    setMsg('')
    try {
      await fn()
      setMsg(successMsg)
      load()
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setActionLoading(false)
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>检测控制</h2>
        <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" />刷新</Btn>
      </div>

      {state && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="glass rounded-[22px] p-4">
            <p className="text-xs mb-2" style={{ color: 'var(--muted)' }}>当前状态</p>
            <div className="flex items-center gap-2">
              {state.running
                ? <><Loader2 className="w-4 h-4 animate-spin" style={{ color: 'var(--ok)' }} /><span className="font-medium" style={{ color: 'var(--ok)' }}>运行中</span></>
                : <><CheckCircle className="w-4 h-4" style={{ color: 'var(--ok)' }} /><span className="font-medium" style={{ color: 'var(--ok)' }}>空闲</span></>
              }
            </div>
            {state.running && (
              <p className="text-xs font-mono mt-2" style={{ color: 'var(--muted)' }}>
                类型: {state.kind}{state.provider_id && ` · ${state.provider_id}`}
              </p>
            )}
          </div>

          <div className="glass rounded-[22px] p-4">
            <p className="text-xs mb-2" style={{ color: 'var(--muted)' }}>自动检测</p>
            <p className="font-mono text-sm" style={{ color: 'var(--text)' }}>
              {state.auto_check_interval_min_hours <= 0 && state.auto_check_interval_max_hours <= 0
                ? '已关闭'
                : `${state.auto_check_interval_min_hours}h – ${state.auto_check_interval_max_hours}h`
              }
            </p>
          </div>
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <Btn
          variant="primary"
          loading={actionLoading}
          disabled={state?.running}
          onClick={() => withMsg(() => api.triggerCheck(), '检测已触发，完成后自动刷新')}
        >
          <Play className="w-3.5 h-3.5" />触发检测
        </Btn>
        <Btn
          variant="danger"
          loading={actionLoading}
          disabled={!state?.running}
          onClick={() => withMsg(() => api.stopDetection(), '已停止')}
        >
          <Square className="w-3.5 h-3.5" />停止检测
        </Btn>
      </div>

      {msg && (
        <p className="text-sm font-mono" style={{ color: msg.startsWith('错误') ? 'var(--error)' : 'var(--ok)' }}>
          {msg}
        </p>
      )}

      {summary && summary.total > 0 && (
        <div className="glass rounded-[22px] p-4">
          <p className="text-xs mb-3 uppercase tracking-widest" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>上次检测摘要</p>
          <div className="flex flex-wrap gap-4 text-sm font-mono mb-2">
            <span style={{ color: 'var(--ok)' }}>✓ {summary.ok_count} 正常</span>
            {summary.slow_count > 0 && <span style={{ color: 'var(--slow)' }}>~ {summary.slow_count} 较慢</span>}
            {summary.error_count > 0 && <span style={{ color: 'var(--error)' }}>✕ {summary.error_count} 异常</span>}
            <span style={{ color: 'var(--muted)' }}>共 {summary.total} 个模型 · {summary.elapsed_ms} ms</span>
          </div>
          <p className="text-xs" style={{ color: 'var(--muted)', opacity: .6 }}>{summary.generated_at}</p>
        </div>
      )}

      {cfg && <TokenEstimateCard providers={cfg.providers} settings={cfg.settings} />}
    </div>
  )
}

// ── Provider Modal ───────────────────────────────────────────────────────────

function ProviderModal({
  initial,
  onSave,
  onClose,
}: {
  initial: SafeProviderConfig | null
  onSave: (id: string | null, update: ProviderUpdate) => Promise<void>
  onClose: () => void
}) {
  const [form, setForm] = useState<{
    id: string; name: string; type: string; base_url: string
    api_key: string; clear_api_key: boolean; models: string; enabled: boolean
  }>(() => ({
    id: initial?.id ?? '',
    name: initial?.name ?? '',
    type: initial?.type ?? 'openai',
    base_url: initial?.base_url ?? '',
    api_key: '',
    clear_api_key: false,
    models: initial?.models.join(', ') ?? '',
    enabled: initial?.enabled ?? true,
  }))
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')

  const set = <K extends keyof typeof form>(key: K, val: (typeof form)[K]) =>
    setForm(f => ({ ...f, [key]: val }))

  const submit = async () => {
    if (!form.name.trim() || !form.base_url.trim()) {
      setErr('名称和 Base URL 为必填项')
      return
    }
    setSaving(true)
    setErr('')
    try {
      await onSave(initial?.id ?? null, {
        id: form.id.trim(),
        name: form.name.trim(),
        type: form.type.trim() || 'openai',
        base_url: form.base_url.trim(),
        api_key: form.api_key,
        clear_api_key: form.clear_api_key,
        models: form.models.split(',').map(m => m.trim()).filter(Boolean),
        enabled: form.enabled,
      })
    } catch (e) {
      setErr((e as Error).message)
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 backdrop-blur-sm" style={{ background: 'rgba(11,16,32,.75)' }}>
      <div className="w-full max-w-lg glass rounded-[24px] overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4" style={{ borderBottom: '1px solid var(--border)' }}>
          <h3 className="font-semibold" style={{ color: 'var(--text)' }}>
            {initial ? '编辑 Provider' : '新增 Provider'}
          </h3>
          <button onClick={onClose} className="cursor-pointer transition-colors" style={{ color: 'var(--muted)' }} onMouseEnter={e=>(e.currentTarget.style.color='var(--text)')} onMouseLeave={e=>(e.currentTarget.style.color='var(--muted)')}>
            <XCircle className="w-5 h-5" />
          </button>
        </div>

        <div className="p-5 space-y-4 max-h-[70vh] overflow-y-auto">
          <div className="grid grid-cols-2 gap-3">
            <Field label="ID（唯一标识）">
              <input
                className={inputCls}
                value={form.id}
                onChange={e => set('id', e.target.value)}
                placeholder="openai-main"
                disabled={!!initial}
              />
            </Field>
            <Field label="名称 *">
              <input className={inputCls} value={form.name} onChange={e => set('name', e.target.value)} placeholder="OpenAI" />
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Field label="类型">
              <input className={inputCls} value={form.type} onChange={e => set('type', e.target.value)} placeholder="openai" />
            </Field>
            <Field label="Base URL *">
              <input className={inputCls} value={form.base_url} onChange={e => set('base_url', e.target.value)} placeholder="https://api.openai.com/v1" />
            </Field>
          </div>

          <Field label={`API Key${initial?.api_key_set ? '（已设置，留空保留）' : ''}`}>
            <input
              type="password"
              className={inputCls}
              value={form.api_key}
              onChange={e => set('api_key', e.target.value)}
              placeholder={initial?.api_key_set ? '留空保留已有 Key' : 'sk-...'}
            />
            {initial?.api_key_set && (
              <label className="flex items-center gap-2 mt-1.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.clear_api_key}
                  onChange={e => set('clear_api_key', e.target.checked)}
                  className="rounded"
                />
                <span className="text-xs" style={{ color: 'var(--error)' }}>清除已有 Key</span>
              </label>
            )}
          </Field>

          <Field label="模型列表" hint="逗号分隔；留空则自动从 /v1/models 获取">
            <input className={inputCls} value={form.models} onChange={e => set('models', e.target.value)} placeholder="gpt-4o-mini, gpt-4.1-mini" />
          </Field>

          <label className="flex items-center gap-2 cursor-pointer">
            <input type="checkbox" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} className="rounded" />
            <span className="text-sm" style={{ color: 'var(--text)' }}>启用此 Provider</span>
          </label>

          {err && <p className="text-red-400 text-xs">{err}</p>}
        </div>

        <div className="flex justify-end gap-2 px-5 py-4" style={{ borderTop: '1px solid var(--border)' }}>
          <Btn onClick={onClose} variant="ghost">取消</Btn>
          <Btn onClick={submit} loading={saving} variant="primary">
            <Save className="w-3.5 h-3.5" />保存
          </Btn>
        </div>
      </div>
    </div>
  )
}

// ── Providers Tab ─────────────────────────────────────────────────────────────

function ProvidersTab() {
  const [providers, setProviders] = useState<SafeProviderConfig[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<SafeProviderConfig | null | 'new'>(null)
  const [msg, setMsg] = useAutoMsg()
  const [deleting, setDeleting] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  const [rerunning, setRerunning] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    api.providers().then(setProviders).catch(() => {}).finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleSave = async (id: string | null, update: ProviderUpdate) => {
    if (id) {
      await api.updateProvider(id, update)
    } else {
      await api.createProvider(update)
    }
    setEditing(null)
    setMsg('已保存')
    load()
  }

  const handleDelete = async (id: string) => {
    if (confirmDelete !== id) {
      setConfirmDelete(id)
      return
    }
    setConfirmDelete(null)
    setDeleting(id)
    try {
      await api.deleteProvider(id)
      setMsg('已删除')
      load()
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setDeleting(null)
    }
  }

  const handleRerun = async (id: string, name: string) => {
    setRerunning(id)
    setMsg('')
    try {
      await api.rerunProvider(id)
      setMsg(`「${name}」重新检测已触发，完成后结果将更新至仪表盘`)
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setRerunning(null)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>Provider 管理</h2>
        <div className="flex items-center gap-2 flex-wrap">
          <input
            className="input-glass rounded-xl px-3 py-1.5 text-xs w-44"
            placeholder="搜索 ID / 名称 / URL…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" /></Btn>
          <Btn onClick={() => setEditing('new')} variant="primary"><Plus className="w-3.5 h-3.5" />新增</Btn>
        </div>
      </div>

      {msg && (
        <p className="text-sm font-mono" style={{ color: msg.startsWith('错误') ? 'var(--error)' : 'var(--ok)' }}>
          {msg}
        </p>
      )}

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-left" style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)' }}>
              <th className="pb-2 pr-4 font-medium">ID / 名称</th>
              <th className="pb-2 pr-4 font-medium">类型</th>
              <th className="pb-2 pr-4 font-medium">Base URL</th>
              <th className="pb-2 pr-4 font-medium">模型</th>
              <th className="pb-2 pr-4 font-medium">Key</th>
              <th className="pb-2 pr-4 font-medium">状态</th>
              <th className="pb-2 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {providers.filter(p => {
              if (!search.trim()) return true
              const q = search.toLowerCase()
              return p.id.toLowerCase().includes(q) || p.name.toLowerCase().includes(q) || p.base_url.toLowerCase().includes(q)
            }).map(p => (
              <tr key={p.id} className="transition-colors" style={{ borderBottom: '1px solid var(--border)' }}>
                <td className="py-3 pr-4">
                  <div className="font-mono text-xs" style={{ color: 'var(--muted)' }}>{p.id}</div>
                  <div style={{ color: 'var(--text)' }}>{p.name}</div>
                </td>
                <td className="py-3 pr-4 font-mono text-xs" style={{ color: 'var(--muted)' }}>{p.type}</td>
                <td className="py-3 pr-4 max-w-[180px]">
                  <span className="font-mono text-xs truncate block" style={{ color: 'var(--muted)' }} title={p.base_url}>
                    {p.base_url}
                  </span>
                </td>
                <td className="py-3 pr-4 text-xs" style={{ color: 'var(--muted)' }}>
                  {p.models.length === 0 ? <span className="italic">自动</span> : p.models.length}
                </td>
                <td className="py-3 pr-4">
                  {p.api_key_set
                    ? <span className="text-xs" style={{ color: 'var(--ok)' }}>●</span>
                    : <span className="text-xs" style={{ color: 'var(--muted)', opacity: .4 }}>—</span>}
                </td>
                <td className="py-3 pr-4">
                  <Badge status={p.enabled ? 'ok' : 'error'} />
                </td>
                <td className="py-3">
                  <div className="flex items-center gap-1">
                    <Btn variant="ghost" onClick={() => handleRerun(p.id, p.name)} loading={rerunning === p.id} className="shrink-0" title="重新检测此 Provider">
                      <RefreshCw className="w-3.5 h-3.5" />
                    </Btn>
                    <Btn variant="ghost" onClick={() => setEditing(p)}>
                      <Edit2 className="w-3.5 h-3.5" />
                    </Btn>
                    {confirmDelete === p.id ? (
                      <>
                        <Btn variant="danger" onClick={() => handleDelete(p.id)} loading={deleting === p.id} className="text-[11px] px-2">
                          确认删除
                        </Btn>
                        <Btn variant="ghost" onClick={() => setConfirmDelete(null)} className="text-[11px] px-2">
                          取消
                        </Btn>
                      </>
                    ) : (
                      <Btn variant="danger" onClick={() => handleDelete(p.id)} loading={deleting === p.id}>
                        <Trash2 className="w-3.5 h-3.5" />
                      </Btn>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {!loading && providers.length === 0 && (
              <tr>
                <td colSpan={7} className="py-12 text-center" style={{ color: 'var(--muted)', opacity: .5 }}>暂无 Provider</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {editing !== null && (
        <ProviderModal
          initial={editing === 'new' ? null : editing}
          onSave={handleSave}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}

// ── Settings Tab ──────────────────────────────────────────────────────────────

function normalizeSettings(s: RuntimeSettings): RuntimeSettings {
  return {
    ...s,
    skip_models: s.skip_models ?? [],
    notify_providers: s.notify_providers ?? [],
    notify_models: s.notify_models ?? [],
  }
}

function SettingsTab() {
  const [form, setForm] = useState<RuntimeSettings | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useAutoMsg()

  useEffect(() => {
    setLoading(true)
    api.config()
      .then(c => setForm(normalizeSettings(c.settings)))
      .catch(e => setMsg(`加载失败：${(e as Error).message}`))
      .finally(() => setLoading(false))
  }, [])

  const set = <K extends keyof RuntimeSettings>(key: K, val: RuntimeSettings[K]) =>
    setForm(f => f ? { ...f, [key]: val } : f)

  const save = async () => {
    if (!form) return
    setSaving(true)
    setMsg('')
    try {
      await api.updateSettings(form)
      setMsg('已保存')
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="flex justify-center py-12"><Spinner /></div>
  }

  if (!form) {
    return (
      <div className="py-12 text-center">
        <p className="text-sm" style={{ color: 'var(--error)' }}>{msg || '加载失败，请刷新重试'}</p>
      </div>
    )
  }

  const numInput = (key: keyof RuntimeSettings, label: string, hint?: string) => (
    <Field label={label} hint={hint}>
      <input
        type="number"
        className={inputCls}
        value={form[key] as number}
        onChange={e => set(key, Number(e.target.value) as RuntimeSettings[typeof key])}
      />
    </Field>
  )

  const textInput = (key: keyof RuntimeSettings, label: string, hint?: string) => (
    <Field label={label} hint={hint}>
      <input
        className={inputCls}
        value={form[key] as string}
        onChange={e => set(key, e.target.value as RuntimeSettings[typeof key])}
      />
    </Field>
  )

  const toggle = (key: keyof RuntimeSettings, label: string) => (
    <label className="flex items-center gap-2 cursor-pointer">
      <input
        type="checkbox"
        checked={form[key] as boolean}
        onChange={e => set(key, e.target.checked as RuntimeSettings[typeof key])}
        className="rounded"
      />
      <span className="text-sm" style={{ color: 'var(--text)' }}>{label}</span>
    </label>
  )

  return (
    <div className="space-y-6 max-w-2xl">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>运行时设置</h2>
        <Btn onClick={save} loading={saving} variant="primary">
          <Save className="w-3.5 h-3.5" />保存
        </Btn>
      </div>

      {msg && <p className="text-sm font-mono" style={{ color: msg.startsWith('错误') ? 'var(--error)' : 'var(--ok)' }}>{msg}</p>}

      <section>
        <h3 className="text-xs font-medium uppercase tracking-widest mb-3" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>基础</h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {textInput('dashboard_title', '仪表盘标题')}
          <Field label="主题模式" hint="已由前端接管（localStorage），此字段不再生效">
            <input className={`${inputCls} opacity-40 cursor-not-allowed`} value={form.theme_mode} readOnly tabIndex={-1} />
          </Field>
          <Field label="日间模式起始时（0-23）" hint="已由前端接管，此字段不再生效">
            <input type="number" className={`${inputCls} opacity-40 cursor-not-allowed`} value={form.day_mode_start_hour} readOnly tabIndex={-1} />
          </Field>
          <Field label="日间模式结束时（0-23）" hint="已由前端接管，此字段不再生效">
            <input type="number" className={`${inputCls} opacity-40 cursor-not-allowed`} value={form.day_mode_end_hour} readOnly tabIndex={-1} />
          </Field>
        </div>
      </section>

      <section>
        <h3 className="text-xs font-medium uppercase tracking-widest mb-3" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>检测</h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {numInput('timeout_seconds', '单模型超时（秒）')}
          {numInput('model_list_timeout_seconds', '模型列表超时（秒）')}
          {numInput('slow_threshold_ms', '较慢阈值（毫秒）')}
          {numInput('concurrency', '全局并发数')}
          {numInput('provider_concurrency', 'Provider 并发数')}
          {numInput('max_models_per_provider', '每 Provider 最大模型数', '0 = 不限')}
          {numInput('auto_check_interval_min_hours', '自动检测最小间隔（小时）', '0 = 关闭')}
          {numInput('auto_check_interval_max_hours', '自动检测最大间隔（小时）')}
        </div>
        <div className="mt-3">
          <Field label="跳过模型" hint="逗号分隔，支持 model / provider/model / provider::model">
            <input
              className={inputCls}
              value={form.skip_models.join(', ')}
              onChange={e => set('skip_models', e.target.value.split(',').map(s => s.trim()).filter(Boolean))}
            />
          </Field>
        </div>
      </section>

      <section>
        <h3 className="text-xs font-medium uppercase tracking-widest mb-3" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>历史</h3>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
          {numInput('stats_window_days', '统计窗口（天）')}
          {numInput('history_size', '历史条数')}
          {numInput('max_history_records', '最大保留记录')}
        </div>
        <div className="flex flex-wrap gap-4 mt-3">
          {toggle('enable_history', '启用历史')}
          {toggle('show_curve_chart', '显示延迟曲线')}
          {toggle('show_error_detail', '显示错误详情')}
        </div>
      </section>

      <section>
        <h3 className="text-xs font-medium uppercase tracking-widest mb-3" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>告警通知</h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {textInput('notify_platform', '平台', 'webhook / discord / bark / wecom / dingtalk / telegram')}
          {textInput('notify_webhook_url', 'Webhook URL')}
          {textInput('notify_telegram_bot_token', 'Telegram Bot Token')}
          {textInput('notify_telegram_chat_id', 'Telegram Chat ID')}
          {numInput('notify_cooldown_minutes', '冷却时间（分钟）', '0 = 不限')}
        </div>
        <div className="flex flex-wrap gap-4 mt-3">
          {toggle('notify_on_recovery', '恢复时通知')}

        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 mt-3">
          <Field label="只对这些 Provider 告警" hint="逗号分隔 ID 或名称，空表示全部">
            <input
              className={inputCls}
              value={form.notify_providers.join(', ')}
              onChange={e => set('notify_providers', e.target.value.split(',').map(s => s.trim()).filter(Boolean))}
            />
          </Field>
          <Field label="只对这些模型告警" hint="逗号分隔，空表示全部">
            <input
              className={inputCls}
              value={form.notify_models.join(', ')}
              onChange={e => set('notify_models', e.target.value.split(',').map(s => s.trim()).filter(Boolean))}
            />
          </Field>
        </div>
      </section>
    </div>
  )
}

// ── Tasks Tab ──────────────────────────────────────────────────────────────────

function TasksTab() {
  const [tasks, setTasks] = useState<CheckTask[]>([])
  const [loading, setLoading] = useState(false)
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState('')
  const LIMIT = 20

  const load = useCallback(() => {
    setLoading(true)
    api.tasks({ limit: LIMIT, offset, status: filter || undefined })
      .then(setTasks).catch(() => {}).finally(() => setLoading(false))
  }, [offset, filter])

  useEffect(() => { load() }, [load])

  const kindLabel: Record<string, string> = {
    manual: '手动', scheduled: '定时', startup: '启动', provider: 'Provider',
  }

return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>检测历史</h2>
        <div className="flex items-center gap-2">
          <select
            value={filter}
            onChange={e => { setFilter(e.target.value); setOffset(0) }}
            className="input-glass rounded-xl px-2 py-1.5 text-xs cursor-pointer focus:outline-none"
          >
            <option value="">全部状态</option>
            <option value="success">成功</option>
            <option value="error">错误</option>
            <option value="running">运行中</option>
            <option value="canceled">已取消</option>
          </select>
          <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" /></Btn>
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-left" style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)' }}>
              <th className="pb-2 pr-3 font-medium">ID</th>
              <th className="pb-2 pr-3 font-medium">类型</th>
              <th className="pb-2 pr-3 font-medium">状态</th>
              <th className="pb-2 pr-3 font-medium">开始时间</th>
              <th className="pb-2 pr-3 font-medium">耗时</th>
              <th className="pb-2 font-medium">结果</th>
            </tr>
          </thead>
          <tbody>
            {tasks.map(t => (
              <tr
                key={t.id}
                className="transition-colors"
                style={{ borderBottom: '1px solid var(--border)' }}
                onMouseEnter={e => (e.currentTarget.style.background = 'var(--row-hover)')}
                onMouseLeave={e => (e.currentTarget.style.background = '')}
              >
                <td className="py-2.5 pr-3 font-mono text-xs" style={{ color: 'var(--muted)' }}>#{t.id}</td>
                <td className="py-2.5 pr-3 text-xs">
                  <span style={{ color: 'var(--text)' }}>{kindLabel[t.kind] ?? t.kind}</span>
                  {t.provider_id && <span className="font-mono ml-1" style={{ color: 'var(--muted)', opacity: .7 }}>{t.provider_id}</span>}
                </td>
                <td className="py-2.5 pr-3">
                  <Badge status={t.status} />
                  {t.error_message && (
                    <p className="text-[11px] font-mono mt-0.5 max-w-[200px] truncate" style={{ color: 'var(--error)', opacity: .7 }}>{t.error_message}</p>
                  )}
                </td>
                <td className="py-2.5 pr-3 font-mono text-xs" style={{ color: 'var(--muted)' }}>
                  {t.started_at ? new Date(t.started_at).toLocaleString('zh-CN') : '—'}
                </td>
                <td className="py-2.5 pr-3 font-mono text-xs" style={{ color: 'var(--muted)' }}>
                  {t.elapsed_ms ? `${(t.elapsed_ms / 1000).toFixed(1)}s` : '—'}
                </td>
                <td className="py-2.5 text-xs font-mono">
                  {t.total > 0 && (
                    <span style={{ color: 'var(--muted)' }}>
                      <span style={{ color: 'var(--ok)' }}>{t.ok_count}</span>/
                      <span style={{ color: 'var(--slow)' }}>{t.slow_count}</span>/
                      <span style={{ color: 'var(--error)' }}>{t.error_count}</span>
                      <span style={{ color: 'var(--muted)', opacity: .6 }}> ({t.total})</span>
                    </span>
                  )}
                </td>
              </tr>
            ))}
            {!loading && tasks.length === 0 && (
              <tr>
                <td colSpan={6} className="py-12 text-center" style={{ color: 'var(--muted)', opacity: .5 }}>暂无检测记录</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center gap-3">
        <Btn onClick={() => setOffset(o => Math.max(0, o - LIMIT))} disabled={offset === 0} variant="ghost">
          上一页
        </Btn>
        <span className="text-xs font-mono" style={{ color: 'var(--muted)' }}>
          {offset + 1}–{offset + tasks.length} 条
        </span>
        <Btn onClick={() => setOffset(o => o + LIMIT)} disabled={tasks.length < LIMIT} variant="ghost">
          下一页
        </Btn>
      </div>
    </div>
  )
}

// ── Config Tab ─────────────────────────────────────────────────────────────────

function ConfigTab() {
  const [exportData, setExportData] = useState('')
  const [importText, setImportText] = useState('')
  const [loading, setLoading] = useState<string | null>(null)
  const [msg, setMsg] = useAutoMsg()

  const withLoad = async (key: string, fn: () => Promise<unknown>, successMsg: string) => {
    setLoading(key)
    setMsg('')
    try {
      await fn()
      setMsg(successMsg)
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setLoading(null)
    }
  }

  const doExport = async () => {
    setLoading('export')
    setMsg('')
    try {
      const data: ConfigExport = await api.exportConfig()
      const json = JSON.stringify(data, null, 2)
      setExportData(json)
      const blob = new Blob([json], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url; a.download = 'cg-config.json'; a.click()
      URL.revokeObjectURL(url)
      setMsg('配置已导出')
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setLoading(null)
    }
  }

  const doImport = () => withLoad('import', async () => {
    const parsed = JSON.parse(importText)
    await api.importConfig(parsed)
  }, '配置已导入')

  const doReload = () => withLoad('reload', () => api.reloadConfig(), '已从 .env 重新加载配置')

  return (
    <div className="space-y-6 max-w-2xl">
      <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>配置管理</h2>

      {msg && <p className="text-sm font-mono" style={{ color: msg.startsWith('错误') ? 'var(--error)' : 'var(--ok)' }}>{msg}</p>}

      <section className="glass rounded-[22px] p-5 space-y-3">
        <h3 className="text-sm font-medium" style={{ color: 'var(--text)' }}>导出配置</h3>
        <p className="text-xs" style={{ color: 'var(--muted)' }}>下载当前设置和 Provider 列表（不含 API Key）为 JSON 文件</p>
        <Btn onClick={doExport} loading={loading === 'export'} variant="default">
          <Download className="w-3.5 h-3.5" />导出 JSON
        </Btn>
        {exportData && (
          <textarea
            readOnly
            value={exportData}
            className={`${inputCls} h-32 resize-none text-[11px]`}
          />
        )}
      </section>

      <section className="glass rounded-[22px] p-5 space-y-3">
        <h3 className="text-sm font-medium" style={{ color: 'var(--text)' }}>导入配置</h3>
        <p className="text-xs" style={{ color: 'var(--muted)' }}>粘贴 JSON 配置并导入；已有 API Key 将自动保留</p>
        <textarea
          value={importText}
          onChange={e => setImportText(e.target.value)}
          placeholder='{"settings":{...},"providers":[...]}'
          className={`${inputCls} h-40 resize-none text-[11px]`}
        />
        <Btn onClick={doImport} loading={loading === 'import'} variant="primary" disabled={!importText.trim()}>
          <Upload className="w-3.5 h-3.5" />导入
        </Btn>
      </section>

      <section className="glass rounded-[22px] p-5 space-y-3">
        <h3 className="text-sm font-medium" style={{ color: 'var(--text)' }}>热加载 .env</h3>
        <p className="text-xs" style={{ color: 'var(--muted)' }}>重新读取 .env 文件并热加载配置，不重启服务</p>
        <Btn onClick={doReload} loading={loading === 'reload'} variant="default">
          <RotateCcw className="w-3.5 h-3.5" />重载 .env
        </Btn>
      </section>
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
      {/* Navbar */}
      <nav
        className={`fixed top-0 left-0 right-0 z-30 backdrop-blur-glass border-b nav-glass transition-transform duration-300 ease-in-out ${navVisible ? 'translate-y-0' : '-translate-y-full'}`}
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
              title={theme === 'dark' ? '切换亮色模式' : '切换暗色模式'}
              aria-label="切换主题"
            >
              {theme === 'dark' ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
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
        {/* Sidebar */}
        <aside className="w-44 shrink-0 hidden sm:block">
          <nav className="space-y-0.5">
            {TABS.map(t => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="w-full flex items-center gap-2.5 px-3 py-2 rounded-xl text-sm transition-colors cursor-pointer"
                style={tab === t.id
                  ? { background: 'rgba(56,217,150,.12)', color: 'var(--ok)', fontWeight: 600 }
                  : { color: 'var(--muted)' }
                }
                onMouseEnter={e => { if (tab !== t.id) (e.currentTarget as HTMLElement).style.color = 'var(--text)' }}
                onMouseLeave={e => { if (tab !== t.id) (e.currentTarget as HTMLElement).style.color = 'var(--muted)' }}
              >
                {t.icon}{t.label}
              </button>
            ))}
          </nav>
        </aside>

        {/* Mobile tab bar */}
        <div className="sm:hidden w-full">
          <div className="flex overflow-x-auto gap-1 pb-3 mb-4" style={{ borderBottom: '1px solid var(--border)' }}>
            {TABS.map(t => (
              <button
                key={t.id}
                onClick={() => setTab(t.id)}
                className="flex-none flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs whitespace-nowrap transition-colors cursor-pointer"
                style={tab === t.id
                  ? { background: 'rgba(56,217,150,.12)', color: 'var(--ok)', fontWeight: 600 }
                  : { background: 'var(--card)', color: 'var(--muted)' }
                }
              >
                {t.label}
              </button>
            ))}
          </div>
        </div>

        {/* Content */}
        <main className="flex-1 min-w-0">
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
