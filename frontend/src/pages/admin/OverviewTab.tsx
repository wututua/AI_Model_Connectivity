import { useCallback, useEffect, useRef, useState } from 'react'
import { Activity, CheckCircle2, Clock3, Play, RefreshCw, Server, Timer } from 'lucide-react'
import { api } from '../../api'
import type { Report, RunningState, RuntimeSettings, SafeProviderConfig } from '../../types'
import { relativeTime } from '../../utils/status'
import { useNow } from '../../hooks/useNow'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/card'
import { Skeleton } from '../../components/ui/skeleton'
import { Feedback, LoadingButton, TokenEstimateCard, normalizeSettings, useAutoMsg } from './shared'

export function OverviewTab({ readOnly = false }: { readOnly?: boolean }) {
  const now = useNow()
  const [state, setState] = useState<RunningState | null>(null)
  const [config, setConfig] = useState<{ providers: SafeProviderConfig[]; settings: RuntimeSettings } | null>(null)
  const [summary, setSummary] = useState<Report | null>(null)
  const [loading, setLoading] = useState(false)
  const [starting, setStarting] = useState(false)
  const [message, setMessage] = useAutoMsg()
  const watchedTask = useRef<number | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([api.detection(), readOnly ? Promise.resolve(null) : api.config(), api.status().catch(() => null)])
      .then(([runningState, adminConfig, report]) => {
        setState(runningState)
        if (runningState.running && !watchedTask.current) watchedTask.current = runningState.task_id
        setConfig(adminConfig ? { providers: adminConfig.providers, settings: normalizeSettings(adminConfig.settings) } : null)
        setSummary(report)
      })
      .catch(cause => setMessage(`错误：${(cause as Error).message}`))
      .finally(() => setLoading(false))
  }, [readOnly, setMessage])

  useEffect(() => { load() }, [load])
  useEffect(() => {
    let active = true
    let pending = false
    const timer = setInterval(async () => {
      if (pending) return
      pending = true
      try {
        const value = await api.detection()
        if (!active) return
        setState(value)
        if (value.running && !watchedTask.current) watchedTask.current = value.task_id
        if (watchedTask.current) {
          const taskId = watchedTask.current
          const task = await api.task(taskId)
          if (!active || watchedTask.current !== taskId || task.status === 'running') return
          const report = await api.status()
          if (!active || watchedTask.current !== taskId) return
          watchedTask.current = null
          setMessage(task.status === 'success' ? `检测任务 #${task.id} 已完成` : `错误：检测任务 #${task.id} ${task.status === 'canceled' ? '已取消' : '失败'}${task.error_message ? `：${task.error_message}` : ''}`)
          setSummary(report)
        }
      } catch {
        // Keep the accepted task ID and retry after a transient connection failure.
      } finally { pending = false }
    }, 2000)
    return () => { active = false; clearInterval(timer) }
  }, [setMessage])

  const run = async () => {
    setStarting(true); setMessage('')
    try {
      const { task } = await api.triggerCheck()
      watchedTask.current = task.id
      setMessage(`检测任务 #${task.id} 已启动`)
      load()
    }
    catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setStarting(false) }
  }

  if (!state && loading) return <OverviewSkeleton />

  const scheduleOn = state && (state.auto_check_interval_min_hours > 0 || state.auto_check_interval_max_hours > 0)

  return (
    <div className="space-y-5">
      <div className="flex justify-end"><Button variant="outline" size="sm" onClick={load} disabled={loading}><RefreshCw className={loading ? 'animate-spin' : ''} />刷新数据</Button></div>
      <Feedback message={message} />

      {state && (
        <Card className="shadow-panel">
          <CardHeader className="flex-row items-start justify-between space-y-0">
            <div>
              <div className="flex items-center gap-2">
                <CardTitle>检测服务</CardTitle>
                <Badge variant={state.running ? 'success' : 'muted'}><span className={`mr-1.5 size-1.5 rounded-full ${state.running ? 'animate-pulse bg-success' : 'bg-muted-foreground'}`} />{state.running ? '运行中' : '空闲'}</Badge>
              </div>
              <CardDescription className="mt-2">{state.running ? `正在执行${state.kind ? `「${state.kind}」` : ''}检测${state.provider_id ? `，目标 ${state.provider_id}` : ''}` : '当前没有正在执行的检测任务'}</CardDescription>
            </div>
            <div className="flex size-10 items-center justify-center rounded-md bg-primary/10 text-primary">{state.running ? <Activity className="animate-pulse" /> : <CheckCircle2 />}</div>
          </CardHeader>
          {!readOnly && (
            <CardContent className="flex flex-wrap gap-2">
              <LoadingButton onClick={run} loading={starting} disabled={state.running}><Play />立即检测</LoadingButton>
            </CardContent>
          )}
        </Card>
      )}

      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <MiniMetric icon={<Activity />} label="运行状态" value={state?.running ? '检测中' : '空闲'} tone={state?.running ? 'text-success' : ''} />
        <MiniMetric icon={<Timer />} label="自动检测" value={scheduleOn ? `${state?.auto_check_interval_min_hours}–${state?.auto_check_interval_max_hours}h` : '已关闭'} />
        <MiniMetric icon={<Server />} label="Provider" value={String(summary?.provider_count ?? '—')} />
        <MiniMetric icon={<Clock3 />} label="上次耗时" value={summary ? `${summary.elapsed_ms} ms` : '—'} />
      </div>

      {summary && summary.total > 0 && (
        <Card>
          <CardHeader><CardTitle className="text-sm">最近一次检测</CardTitle><CardDescription>{relativeTime(summary.generated_at, now, summary.stale_after_seconds).text}更新，共 {summary.total} 个模型，{summary.unknown_count ?? 0} 个未检测</CardDescription></CardHeader>
          <CardContent className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <ResultMetric label="正常" value={summary.ok_count} className="text-success" />
            <ResultMetric label="较慢" value={summary.slow_count} className="text-warning" />
            <ResultMetric label="异常" value={summary.error_count} className="text-destructive" />
            <ResultMetric label="检测总数" value={summary.total} />
          </CardContent>
        </Card>
      )}

      {config && <TokenEstimateCard providers={config.providers} settings={config.settings} />}
    </div>
  )
}

function MiniMetric({ icon, label, value, tone }: { icon: React.ReactNode; label: string; value: string; tone?: string }) {
  return <Card><CardContent className="p-4"><div className="flex items-center justify-between text-muted-foreground"><span className="data-label">{label}</span><span className="[&>svg]:size-4">{icon}</span></div><p className={`mt-3 font-mono text-xl font-semibold tabular-nums ${tone ?? ''}`}>{value}</p></CardContent></Card>
}

function ResultMetric({ label, value, className }: { label: string; value: number; className?: string }) {
  return <div><p className="data-label">{label}</p><p className={`mt-1 font-mono text-2xl font-semibold tabular-nums ${className ?? ''}`}>{value}</p></div>
}

function OverviewSkeleton() {
  return <div className="space-y-5"><Skeleton className="ml-auto h-8 w-24" /><Skeleton className="h-40" /><div className="grid grid-cols-2 gap-3 xl:grid-cols-4">{Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-24" />)}</div><Skeleton className="h-40" /></div>
}
