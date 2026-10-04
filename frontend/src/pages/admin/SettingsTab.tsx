import { useEffect, useId, useState } from 'react'
import { Check, Save } from 'lucide-react'
import { api } from '../../api'
import type { RuntimeSettings } from '../../types'
import { Badge } from '../../components/ui/badge'
import { Input } from '../../components/ui/input'
import { Label } from '../../components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Skeleton } from '../../components/ui/skeleton'
import { Switch } from '../../components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs'
import { Feedback, Field, LoadingButton, normalizeSettings, useAutoMsg } from './shared'
import { useAuth } from '../../hooks/useAuth'
import { UnsavedChanges } from '../../components/UnsavedChanges'
import { ListInput } from '../../components/ListInput'
import { mergeModels, parseModels } from '../../utils/models'

const emptyDrafts = { skip_models: '', notify_providers: '', notify_models: '' }

export function SettingsTab() {
  const { refresh } = useAuth()
  const [form, setForm] = useState<RuntimeSettings | null>(null)
  const [savedForm, setSavedForm] = useState<RuntimeSettings | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useAutoMsg()
  const [listDrafts, setListDrafts] = useState(emptyDrafts)

  useEffect(() => {
    let active = true
    api.config().then(config => {
      if (!active) return
      const normalized = normalizeSettings(config.settings)
      setForm(normalized)
      setSavedForm(normalized)
    }).catch(cause => { if (active) setMessage(`加载失败：${(cause as Error).message}`) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [setMessage])

  const dirty = JSON.stringify(form) !== JSON.stringify(savedForm) || Object.values(listDrafts).some(value => value.trim())

  const set = <Key extends keyof RuntimeSettings>(key: Key, value: RuntimeSettings[Key]) => setForm(current => current ? { ...current, [key]: value } : current)
  const save = async () => {
    if (!form || saving || !dirty) return
    setSaving(true); setMessage('')
    try {
      const updated = await api.updateSettings({
        ...form,
        skip_models: mergeModels(form.skip_models, parseModels(listDrafts.skip_models)),
        notify_providers: mergeModels(form.notify_providers, parseModels(listDrafts.notify_providers)),
        notify_models: mergeModels(form.notify_models, parseModels(listDrafts.notify_models)),
      })
      const normalized = normalizeSettings(updated.settings)
      setForm(normalized); setSavedForm(normalized); setMessage('系统设置已保存')
      setListDrafts(emptyDrafts)
      await refresh()
    }
    catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setSaving(false) }
  }

  if (loading) return <SettingsSkeleton />
  if (!form) return <Feedback message={message || '加载失败：无法获取系统设置'} />

  return (
    <div className="space-y-4">
      <UnsavedChanges dirty={dirty || saving} />
      <Tabs defaultValue="probe">
        <div className="sticky top-14 z-20 -mx-1 flex flex-wrap items-center justify-between gap-3 border-b bg-background/95 px-1 py-3 backdrop-blur-xl">
          <TabsList className="grid h-auto w-full grid-cols-2 gap-1 sm:inline-grid sm:w-auto sm:grid-cols-4">
            <TabsTrigger value="probe">基础与检测</TabsTrigger><TabsTrigger value="history">历史与调度</TabsTrigger><TabsTrigger value="notify">通知</TabsTrigger><TabsTrigger value="access">访问控制</TabsTrigger>
          </TabsList>
          <div className="flex w-full items-center justify-between gap-3 sm:w-auto">
            <span role="status" className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground">
              {dirty ? <span className="size-1.5 rounded-full bg-warning" /> : <Check className="size-3.5 text-success" />}
              {saving ? '正在保存' : dirty ? '未保存的更改' : '已同步'}
            </span>
            <LoadingButton onClick={save} loading={saving} disabled={!dirty}><Save />保存全部设置</LoadingButton>
          </div>
        </div>
        {message && <div className="mt-4 animate-enter"><Feedback message={message} /></div>}

        <fieldset disabled={saving} className="min-w-0">
        <TabsContent value="probe" className="mt-4">
          <SettingsCard title="基础信息" description="控制公开状态页的基本展示。">
            <Field label="仪表盘标题" htmlFor="dashboard-title"><Input id="dashboard-title" value={form.dashboard_title} onChange={event => set('dashboard_title', event.target.value)} /></Field>
          </SettingsCard>
          <SettingsCard title="检测参数" description="配置探测超时、状态阈值与并发限制。">
            <div className="field-grid">
              <NumberField id="timeout" label="单模型超时" suffix="秒" value={form.timeout_seconds} onChange={value => set('timeout_seconds', value)} hint="每个模型等待响应的最大时间" />
              <NumberField id="model-timeout" label="模型列表超时" suffix="秒" value={form.model_list_timeout_seconds} onChange={value => set('model_list_timeout_seconds', value)} hint="从 /v1/models 获取列表的超时时间" />
              <NumberField id="slow-threshold" label="较慢阈值" suffix="毫秒" value={form.slow_threshold_ms} onChange={value => set('slow_threshold_ms', value)} hint="超过此延迟标记为较慢" />
              <NumberField id="global-concurrency" label="全局并发" value={form.concurrency} onChange={value => set('concurrency', value)} hint="所有 Provider 同时检测的模型数" />
              <NumberField id="provider-concurrency" label="Provider 并发" value={form.provider_concurrency} onChange={value => set('provider_concurrency', value)} hint="单 Provider 内同时检测的模型数" />
              <NumberField id="max-models" label="每 Provider 最大模型数" value={form.max_models_per_provider} onChange={value => set('max_models_per_provider', value)} hint="0 表示不限制" />
              <Field className="md:col-span-2" label="跳过模型" htmlFor="skip-models"><ListInput id="skip-models" label="跳过模型" value={form.skip_models} onChange={value => set('skip_models', value)} draft={listDrafts.skip_models} onDraftChange={value => setListDrafts(current => ({ ...current, skip_models: value }))} /></Field>
            </div>
          </SettingsCard>
        </TabsContent>

        <TabsContent value="history" className="mt-4">
          <SettingsCard title="历史数据" description="控制统计窗口、保留策略与状态页展示。">
            <div className="field-grid">
              <NumberField id="stats-window" label="统计窗口" suffix="天" value={form.stats_window_days} onChange={value => set('stats_window_days', value)} />
              <NumberField id="history-size" label="状态页历史条数" value={form.history_size} onChange={value => set('history_size', value)} />
              <NumberField id="max-history" label="数据库最大保留记录" value={form.max_history_records} onChange={value => set('max_history_records', value)} />
            </div>
            <div className="mt-5 divide-y border-t">
              <SwitchRow label="启用历史记录" description="保存每次模型探测结果" checked={form.enable_history} onCheckedChange={value => set('enable_history', value)} />
              <SwitchRow label="显示延迟曲线" description="在状态页模型行展示延迟趋势" checked={form.show_curve_chart} onCheckedChange={value => set('show_curve_chart', value)} />
              <SwitchRow label="显示错误详情" description="允许状态页展开查看模型错误信息" checked={form.show_error_detail} onCheckedChange={value => set('show_error_detail', value)} />
            </div>
          </SettingsCard>
          <SettingsCard title="自动检测" description="实际间隔会在最小值和最大值之间随机选择。设置为 0 可关闭。">
            <div className="field-grid">
              <NumberField id="schedule-min" label="最小间隔" suffix="小时" value={form.auto_check_interval_min_hours} onChange={value => set('auto_check_interval_min_hours', value)} />
              <NumberField id="schedule-max" label="最大间隔" suffix="小时" value={form.auto_check_interval_max_hours} onChange={value => set('auto_check_interval_max_hours', value)} />
            </div>
          </SettingsCard>
        </TabsContent>

        <TabsContent value="notify" className="mt-4">
          <SettingsCard title="告警通知" description="模型异常与恢复时向指定平台发送通知。">
            <div className="field-grid">
              <Field label="通知平台"><Select value={form.notify_platform || 'webhook'} onValueChange={value => set('notify_platform', value)}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="disabled">不启用</SelectItem><SelectItem value="webhook">Webhook</SelectItem><SelectItem value="discord">Discord</SelectItem><SelectItem value="bark">Bark</SelectItem><SelectItem value="wecom">企业微信</SelectItem><SelectItem value="dingtalk">钉钉</SelectItem><SelectItem value="telegram">Telegram</SelectItem></SelectContent></Select></Field>
              <NumberField id="notify-cooldown" label="通知冷却时间" suffix="分钟" value={form.notify_cooldown_minutes} onChange={value => set('notify_cooldown_minutes', value)} hint="两次聚合状态通知的最短间隔，0 表示不限制" />
              <SecretField id="webhook-url" label="Webhook URL" value={form.notify_webhook_url} configured={form.notify_webhook_url_set} onChange={value => set('notify_webhook_url', value)} placeholder="https://..." />
              <SecretField id="telegram-token" label="Telegram Bot Token" value={form.notify_telegram_bot_token} configured={form.notify_telegram_bot_token_set} onChange={value => set('notify_telegram_bot_token', value)} />
              <SecretField id="telegram-chat" label="Telegram Chat ID" value={form.notify_telegram_chat_id} configured={form.notify_telegram_chat_id_set} onChange={value => set('notify_telegram_chat_id', value)} />
              <Field label="Provider 范围" htmlFor="notify-providers"><ListInput id="notify-providers" label="Provider 范围" value={form.notify_providers} onChange={value => set('notify_providers', value)} draft={listDrafts.notify_providers} onDraftChange={value => setListDrafts(current => ({ ...current, notify_providers: value }))} /></Field>
              <Field className="md:col-span-2" label="模型范围" htmlFor="notify-models"><ListInput id="notify-models" label="模型范围" value={form.notify_models} onChange={value => set('notify_models', value)} draft={listDrafts.notify_models} onDraftChange={value => setListDrafts(current => ({ ...current, notify_models: value }))} /></Field>
            </div>
            <div className="mt-5 divide-y border-t">
              <SwitchRow label="恢复时通知" description="模型从异常恢复正常时发送通知" checked={form.notify_on_recovery} onCheckedChange={value => set('notify_on_recovery', value)} />
              <SwitchRow label="清除 Webhook URL" description="保存后清除服务端现有值" checked={Boolean(form.clear_notify_webhook_url)} onCheckedChange={value => set('clear_notify_webhook_url', value)} danger />
              <SwitchRow label="清除 Telegram Bot Token" description="保存后清除服务端现有值" checked={Boolean(form.clear_notify_telegram_bot_token)} onCheckedChange={value => set('clear_notify_telegram_bot_token', value)} danger />
              <SwitchRow label="清除 Telegram Chat ID" description="保存后清除服务端现有值" checked={Boolean(form.clear_notify_telegram_chat_id)} onCheckedChange={value => set('clear_notify_telegram_chat_id', value)} danger />
            </div>
          </SettingsCard>
        </TabsContent>

        <TabsContent value="access" className="mt-4">
          <SettingsCard title="状态页访问" description="控制监控数据的公开范围。">
            <SwitchRow label="查看状态监控需要登录" description="开启后，仅已登录的管理员或普通用户可以查看监控页面和实时数据。" checked={form.status_login_required} onCheckedChange={value => set('status_login_required', value)} />
          </SettingsCard>
        </TabsContent>
        </fieldset>
      </Tabs>
    </div>
  )
}

function SettingsCard({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  const id = useId()
  return <section className="settings-section" aria-labelledby={id}><div><h2 id={id} className="text-sm font-semibold">{title}</h2><p className="mt-2 text-xs leading-relaxed text-muted-foreground">{description}</p></div><div className="min-w-0">{children}</div></section>
}

function NumberField({ id, label, suffix, value, onChange, hint }: { id: string; label: string; suffix?: string; value: number; onChange: (value: number) => void; hint?: string }) {
  return <Field label={label} htmlFor={id} hint={hint}><div className="relative"><Input id={id} type="number" value={value} onChange={event => onChange(Number(event.target.value))} className={suffix ? 'pr-16 font-mono' : 'font-mono'} />{suffix && <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{suffix}</span>}</div></Field>
}

function SwitchRow({ label, description, checked, onCheckedChange, danger }: { label: string; description: string; checked: boolean; onCheckedChange: (value: boolean) => void; danger?: boolean }) {
  const id = useId()
  return <div className="flex items-center justify-between gap-4 py-4"><div><Label htmlFor={id} className={danger ? 'text-destructive' : ''}>{label}</Label><p id={`${id}-description`} className="mt-1 text-xs text-muted-foreground">{description}</p></div><Switch id={id} aria-describedby={`${id}-description`} checked={checked} onCheckedChange={onCheckedChange} /></div>
}

function SecretField({ id, label, value, configured, onChange, placeholder }: { id: string; label: string; value: string; configured?: boolean; onChange: (value: string) => void; placeholder?: string }) {
  return <Field label={label} htmlFor={id} hint={configured ? '服务端已有值，留空将保留' : undefined}><div className="relative"><Input id={id} type="password" value={value} onChange={event => onChange(event.target.value)} placeholder={configured ? '已设置' : placeholder} className="pr-20 font-mono" />{configured && <Badge variant="success" className="absolute right-2 top-1/2 -translate-y-1/2">已设置</Badge>}</div></Field>
}

function SettingsSkeleton() {
  return <div className="space-y-4"><Skeleton className="ml-auto h-9 w-32" /><Skeleton className="h-9 w-full max-w-lg" /><Skeleton className="h-72" /></div>
}
