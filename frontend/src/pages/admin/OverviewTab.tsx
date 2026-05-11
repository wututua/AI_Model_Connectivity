import { useState, useEffect, useCallback } from 'react'
import { Play, Square, RefreshCw, CheckCircle, Loader2 } from 'lucide-react'
import { api } from '../../api'
import type { RunningState, SafeProviderConfig, RuntimeSettings, Report } from '../../types'
import { useAutoMsg, Btn, TokenEstimateCard, normalizeSettings } from './shared'

export function OverviewTab() {
  const [state, setState] = useState<RunningState | null>(null)
  const [cfg, setCfg] = useState<{ providers: SafeProviderConfig[]; settings: RuntimeSettings } | null>(null)
  const [summary, setSummary] = useState<Report | null>(null)
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
      <div className="flex items-center justify-between anim-fade-in-up" style={{ animationDelay: '0ms' }}>
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>检测控制</h2>
        <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" />刷新</Btn>
      </div>

      {state && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="glass rounded-[22px] p-4 anim-fade-in-up" style={{ animationDelay: '60ms' }}>
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

          <div className="glass rounded-[22px] p-4 anim-fade-in-up" style={{ animationDelay: '100ms' }}>
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

      <div className="flex flex-wrap gap-2 anim-fade-in-up" style={{ animationDelay: '140ms' }}>
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
