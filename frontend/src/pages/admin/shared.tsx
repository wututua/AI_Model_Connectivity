import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { Activity, AlertCircle, Loader2 } from 'lucide-react'
import type { RuntimeSettings, SafeProviderConfig } from '../../types'
import { cn } from '../../lib/utils'
import { STATUS_LABEL, statusClass } from '../../utils/status'
import { Alert, AlertDescription } from '../../components/ui/alert'
import { Badge } from '../../components/ui/badge'
import { Button, type ButtonProps } from '../../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/card'
import { Label } from '../../components/ui/label'

export function useAutoMsg(delay = 3000) {
  const [message, setMessageState] = useState('')
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current) }, [])
  const setMessage = useCallback((value: string) => {
    setMessageState(value)
    if (timerRef.current) clearTimeout(timerRef.current)
    if (value && !value.startsWith('错误') && !value.startsWith('加载失败')) {
      timerRef.current = setTimeout(() => setMessageState(''), delay)
    }
  }, [delay])
  return [message, setMessage] as const
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cn('size-4 animate-spin', className)} />
}

export function LoadingButton({ loading, children, disabled, ...props }: ButtonProps & { loading?: boolean }) {
  return <Button disabled={disabled || loading} {...props}>{loading && <Spinner />}{children}</Button>
}

export function StatusBadge({ status }: { status: string }) {
  const tone = statusClass(status)
  const variant = tone === 'ok' ? 'success' : tone === 'slow' ? 'warning' : 'destructive'
  return <Badge variant={variant}>{STATUS_LABEL[status] ?? status}</Badge>
}

export function Field({ label, hint, htmlFor, children, className }: { label: string; hint?: string; htmlFor?: string; children: ReactNode; className?: string }) {
  return (
    <div className={cn('space-y-2', className)}>
      <Label htmlFor={htmlFor}>{label}</Label>
      {children}
      {hint && <p className="text-xs leading-relaxed text-muted-foreground">{hint}</p>}
    </div>
  )
}

export function Feedback({ message }: { message: string }) {
  if (!message) return null
  const error = message.startsWith('错误') || message.startsWith('加载失败') || message.startsWith('修改失败')
  return (
    <Alert variant={error ? 'destructive' : 'success'}>
      <AlertCircle />
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}

export function PageHeading({ title, description, actions }: { title: string; description: string; actions?: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div><h1 className="text-xl font-semibold">{title}</h1><p className="mt-1 text-sm text-muted-foreground">{description}</p></div>
      {actions && <div className="flex shrink-0 flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

export function fmtNum(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return String(Math.round(value))
}

export function normalizeSettings(settings: RuntimeSettings): RuntimeSettings {
  return {
    ...settings,
    notify_webhook_url: settings.notify_webhook_url ?? '',
    notify_telegram_bot_token: settings.notify_telegram_bot_token ?? '',
    notify_telegram_chat_id: settings.notify_telegram_chat_id ?? '',
    skip_models: settings.skip_models ?? [],
    notify_providers: settings.notify_providers ?? [],
    notify_models: settings.notify_models ?? [],
  }
}

const TOKENS_PER_MODEL = 40

export function TokenEstimateCard({ providers, settings }: { providers: SafeProviderConfig[]; settings: RuntimeSettings }) {
  const enabledProviders = providers.filter(provider => provider.enabled && provider.probe_enabled)
  let totalModels: number | null = 0
  for (const provider of enabledProviders) {
    if (provider.models.length === 0) {
      if (settings.max_models_per_provider > 0) totalModels += settings.max_models_per_provider
      else { totalModels = null; break }
    } else {
      const limit = settings.max_models_per_provider > 0 ? settings.max_models_per_provider : provider.models.length
      totalModels += Math.min(provider.models.length, limit)
    }
  }

  const minHours = settings.auto_check_interval_min_hours
  const maxHours = settings.auto_check_interval_max_hours
  const schedulingOn = minHours > 0 || maxHours > 0
  const lower = minHours <= 0 ? maxHours : minHours
  const upper = maxHours <= 0 ? minHours : maxHours
  const averageHours = schedulingOn ? (lower + upper) / 2 : 0
  const dailyChecks = averageHours > 0 ? 24 / averageHours : 0
  const dailyTokens = totalModels !== null && schedulingOn ? totalModels * TOKENS_PER_MODEL * dailyChecks : null

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm"><Activity className="size-4 text-warning" />Token 消耗估算</CardTitle>
        <CardDescription>按每模型约 {TOKENS_PER_MODEL} Token 估算，不含手动检测。</CardDescription>
      </CardHeader>
      <CardContent className="grid grid-cols-2 gap-x-6 gap-y-4 sm:grid-cols-4">
        <Estimate label="检测 Provider" value={`${enabledProviders.length} / ${providers.length}`} />
        <Estimate label="探测模型" value={totalModels === null ? '动态获取' : String(totalModels)} />
        <Estimate label="每日消耗" value={dailyTokens === null ? '—' : fmtNum(dailyTokens)} suffix="tokens" />
        <Estimate label="每月消耗" value={dailyTokens === null ? '—' : fmtNum(dailyTokens * 30)} suffix="tokens" />
      </CardContent>
    </Card>
  )
}

function Estimate({ label, value, suffix }: { label: string; value: string; suffix?: string }) {
  return <div><p className="data-label">{label}</p><p className="mt-1 font-mono text-lg font-semibold tabular-nums">{value}</p>{suffix && <p className="text-[11px] text-muted-foreground">{suffix}</p>}</div>
}
