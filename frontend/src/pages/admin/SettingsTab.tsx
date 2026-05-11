import { useState, useEffect } from 'react'
import { Save } from 'lucide-react'
import { api } from '../../api'
import type { RuntimeSettings } from '../../types'
import { useAutoMsg, Btn, Spinner, Field, inputCls, normalizeSettings } from './shared'

export function SettingsTab() {
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
          {numInput('timeout_seconds', '单模型超时（秒）', '每个模型等待响应的最大时间，超时计为错误')}
          {numInput('model_list_timeout_seconds', '模型列表超时（秒）', '从 /v1/models 拉取模型列表的超时时间')}
          {numInput('slow_threshold_ms', '较慢阈值（毫秒）', '响应时间超过此值视为「较慢」状态')}
          {numInput('concurrency', '全局并发数', '同时检测的最大模型数，1 = 完全串行')}
          {numInput('provider_concurrency', 'Provider 并发数', '单个 Provider 内同时检测的模型数')}
          {numInput('max_models_per_provider', '每 Provider 最大模型数', '0 = 不限；限制可减少 Token 消耗')}
          {numInput('auto_check_interval_min_hours', '自动检测最小间隔（小时）', '0 = 关闭定时检测；实际间隔在 min~max 之间随机')}
          {numInput('auto_check_interval_max_hours', '自动检测最大间隔（小时）', '与最小间隔相同则为固定间隔')}
        </div>
        <div className="mt-3">
          <Field label="跳过模型" hint="逗号分隔，格式：model-name 或 provider-id/model 或 provider-id::model">
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
          {numInput('stats_window_days', '统计窗口（天）', '可用率和均值的计算时间窗口')}
          {numInput('history_size', '历史条数', '仪表盘 LED 点显示的最近检测次数')}
          {numInput('max_history_records', '最大保留记录', '数据库保留的历史记录上限，超出自动清理')}
        </div>
        <div className="flex flex-wrap gap-4 mt-3">
          {toggle('enable_history', '启用历史')}
          {toggle('show_curve_chart', '显示延迟曲线图')}
          {toggle('show_error_detail', '在仪表盘显示错误详情')}
        </div>
      </section>

      <section>
        <h3 className="text-xs font-medium uppercase tracking-widest mb-3" style={{ color: 'var(--muted)', letterSpacing: '.12em' }}>告警通知</h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {textInput('notify_platform', '平台', '支持：webhook / discord / bark / wecom / dingtalk / telegram')}
          {textInput('notify_webhook_url', 'Webhook URL', '适用于 webhook / discord / wecom / dingtalk')}
          {textInput('notify_telegram_bot_token', 'Telegram Bot Token', '通过 @BotFather 创建')}
          {textInput('notify_telegram_chat_id', 'Telegram Chat ID', '用户或频道的数字 ID')}
          {numInput('notify_cooldown_minutes', '冷却时间（分钟）', '同一模型两次告警的最短间隔，0 = 不限')}
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
