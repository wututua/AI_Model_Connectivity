import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity, ArrowUpDown, Clock3, LayoutGrid, List, RefreshCw, Search, Server, Settings2, X, XCircle,
} from 'lucide-react'
import type { ProviderReport, Report } from '../types'
import { api } from '../api'
import { cn } from '../lib/utils'
import { detectionSuccessRate, relativeTime, reportPresentation } from '../utils/status'
import { readDashboardPreferences, saveDashboardPreferences, type SortMode, type ViewMode } from '../utils/dashboardPreferences'
import { startStatusUpdates, type StatusController } from '../utils/liveStatus'
import { useNow } from '../hooks/useNow'
import { useAuth } from '../hooks/useAuth'
import { ProviderCard } from '../components/ProviderCard'
import { StatusPill } from '../components/StatusPill'
import { ThemeToggle } from '../components/ThemeToggle'
import { Alert, AlertDescription, AlertTitle } from '../components/ui/alert'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Skeleton } from '../components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '../components/ui/tooltip'

type StatusFilter = 'all' | 'ok' | 'slow' | 'error' | 'unknown'

export default function Dashboard() {
  const { session } = useAuth()
  const now = useNow()
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [preferences, setPreferences] = useState(readDashboardPreferences)
  const { sortBy, viewMode } = preferences
  const setSortBy = (sortBy: SortMode) => setPreferences(current => ({ ...current, sortBy }))
  const setViewMode = (viewMode: ViewMode) => setPreferences(current => ({ ...current, viewMode }))
  useEffect(() => saveDashboardPreferences(preferences), [preferences])

  const updates = useRef<StatusController | null>(null)

  const handleRefresh = useCallback(() => {
    const controller = updates.current
    if (!controller) return
    setRefreshing(true)
    void controller.refresh().finally(() => {
      if (updates.current === controller) setRefreshing(false)
    })
  }, [])

  useEffect(() => {
    const controller = startStatusUpdates({
      fetchReport: api.status,
      onReport: data => { setReport(data); setError(null) },
      onError: cause => setError(cause.message),
      onLive: setLive,
    })
    updates.current = controller
    return () => {
      updates.current = null
      controller.close()
    }
  }, [])

  const filteredProviders = useMemo(() => {
    if (!report?.providers) return []
    const query = search.trim().toLowerCase()
    const statusOrder: Record<string, number> = { error: 0, slow: 1, ok: 2 }
    const averageLatency = (provider: ProviderReport) => {
      const values = provider.results.filter(result => result.latency_ms > 0)
      return values.length ? values.reduce((sum, result) => sum + result.latency_ms, 0) / values.length : Number.POSITIVE_INFINITY
    }

    return report.providers
      .map(provider => ({
        ...provider,
        results: (provider.results ?? []).filter(result =>
          (statusFilter === 'all' || result.status === statusFilter) &&
          (!query || result.model.toLowerCase().includes(query) || provider.provider_name.toLowerCase().includes(query) || provider.provider_id.toLowerCase().includes(query)),
        ),
      }))
      .filter(provider => provider.results.length > 0 || (
        provider.model_count === 0 &&
        (!query || provider.provider_name.toLowerCase().includes(query) || provider.provider_id.toLowerCase().includes(query)) &&
        (statusFilter === 'all' || provider.status === statusFilter)
      ))
      .sort((left, right) => {
        if (sortBy === 'name') return left.provider_name.localeCompare(right.provider_name)
        if (sortBy === 'latency') return averageLatency(left) - averageLatency(right)
        if (sortBy === 'models') return right.model_count - left.model_count
        if (sortBy === 'status') return (statusOrder[left.status] ?? 3) - (statusOrder[right.status] ?? 3)
        return 0
      })
  }, [report, search, sortBy, statusFilter])

  return (
    <div className="app-shell">
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur-xl">
        <div className="mx-auto flex h-14 max-w-[1320px] items-center justify-between px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground"><Activity className="size-4" /></div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">{report?.title ?? '模型连通性'}</p>
              <p className="hidden text-xs text-muted-foreground sm:block">AI 服务运行状态</p>
            </div>
            {live && <Badge variant="success" className="hidden sm:inline-flex"><span className="live-dot mr-1 size-1.5 rounded-full bg-success" />实时</Badge>}
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <Tooltip>
              <TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={handleRefresh} disabled={refreshing} aria-label="刷新状态"><RefreshCw className={cn(refreshing && 'animate-spin')} /></Button></TooltipTrigger>
              <TooltipContent>刷新状态</TooltipContent>
            </Tooltip>
            <ThemeToggle />
            <Button asChild variant="outline" size="sm" className="ml-1"><Link to={session.user ? '/admin' : '/login'} aria-label={session.user ? '管理后台' : '登录账户'}><Settings2 /><span className="hidden sm:inline">{session.user ? '工作台' : '登录'}</span></Link></Button>
          </div>
        </div>
      </header>

      <main className="mx-auto min-h-[calc(100vh-3.5rem)] max-w-[1320px] px-4 py-5 sm:px-6 lg:py-7">
        {!report && !error ? <DashboardSkeleton /> : null}

        {error && !report ? (
          <EmptyState error={error} />
        ) : report?.state === 'pending' || report?.state === 'unconfigured' ? (
          <div className="flex min-h-[55vh] flex-col items-center justify-center text-center">
            <Server className="size-8 text-muted-foreground" />
            <h1 className="mt-4 text-xl font-semibold">{report.state === 'unconfigured' ? '尚未配置监控服务' : '等待首次检测'}</h1>
            <p className="mt-2 text-sm text-muted-foreground">{report.state === 'unconfigured' ? '暂无启用的 Provider' : `${report.provider_count} 个 Provider，暂无检测报告`}</p>
            {session.user?.role === 'admin' && <Button asChild className="mt-5"><Link to={report.state === 'unconfigured' ? '/admin/providers' : '/admin/overview'}><Settings2 />{report.state === 'unconfigured' ? '配置 Provider' : '运行概览'}</Link></Button>}
            {error && <p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}
          </div>
        ) : report ? (
          <div className="space-y-5 animate-enter">
            <StatusOverview report={report} live={live} now={now} />
            {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

            <div className="metric-strip grid grid-cols-5 border-y py-3 sm:py-4">
              <StatusMetric label="模型总数" value={report.total} />
              <StatusMetric label="正常" value={report.ok_count} tone="text-success" />
              <StatusMetric label="较慢" value={report.slow_count} tone="text-warning" />
              <StatusMetric label="异常" value={report.error_count} tone="text-destructive" />
              <StatusMetric label="未检测" value={report.unknown_count ?? 0} tone="text-muted-foreground" />
            </div>

            {report.provider_errors?.length > 0 && (
              <Alert variant="destructive">
                <XCircle />
                <AlertTitle>Provider 请求失败</AlertTitle>
                <AlertDescription className="mt-2 space-y-1">
                  {report.provider_errors.map(item => <p key={item.provider_id}><span className="font-mono font-medium">{item.provider_id}</span>：{item.error}</p>)}
                </AlertDescription>
              </Alert>
            )}

            {report.providers?.length > 0 && (
              <FilterBar search={search} setSearch={setSearch} statusFilter={statusFilter} setStatusFilter={setStatusFilter} sortBy={sortBy} setSortBy={setSortBy} viewMode={viewMode} setViewMode={setViewMode} resultCount={filteredProviders.length} />
            )}

            {report.providers?.length > 0 ? (
              filteredProviders.length > 0 ? (
                <div className="space-y-6">
                  {filteredProviders.map((provider, index) => <ProviderCard key={provider.provider_id} provider={provider} showError compact={viewMode === 'compact'} now={now} staleAfterSeconds={report.stale_after_seconds} animDelay={Math.min(index * 45, 180)} />)}
                </div>
              ) : (
                <div className="flex min-h-52 flex-col items-center justify-center rounded-lg border border-dashed text-muted-foreground"><Search className="mb-3 size-6" /><p className="text-sm">没有匹配的 Provider 或模型</p></div>
              )
            ) : (
              <div className="flex min-h-60 flex-col items-center justify-center rounded-lg border border-dashed text-center text-muted-foreground">
                <Server className="mb-3 size-7" /><p className="font-medium text-foreground">暂无检测数据</p><p className="mt-1 text-sm">进入管理后台添加 Provider 并触发检测</p>
                <Button asChild className="mt-4"><Link to="/admin"><Settings2 />管理后台</Link></Button>
              </div>
            )}
          </div>
        ) : null}
      </main>
    </div>
  )
}

function StatusOverview({ report, live, now }: { report: Report; live: boolean; now: number }) {
  const presentation = reportPresentation(report)
  const availability = detectionSuccessRate(report)
  const updated = relativeTime(report.generated_at, now, report.stale_after_seconds)
  return (
    <section aria-label="运行状态总览">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <StatusPill status={presentation.status} label={presentation.label} />
        <span className="text-xs text-muted-foreground">{live ? '实时连接' : '定时更新'}</span>
        {updated.stale && <Badge variant="warning">数据可能已过期</Badge>}
        {(report.unknown_count ?? 0) > 0 && <Badge variant="muted">{report.unknown_count} 个未检测</Badge>}
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-4">
        <div className="min-w-0">
          <h1 className="text-lg font-semibold leading-snug sm:text-2xl">{presentation.headline}</h1>
          <p className="mt-2 text-xs text-muted-foreground">更新于 <time dateTime={report.generated_at} title={report.generated_at}>{updated.text}</time></p>
        </div>
        <div className="text-right">
          <strong key={availability} className="animate-value block font-mono text-2xl font-semibold tabular-nums sm:text-3xl">{availability === null ? 'N/A' : `${availability}%`}</strong>
          <span className="mt-1 block text-xs text-muted-foreground">本次检测成功率</span>
        </div>
      </div>
      <div className="mt-4 flex h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden="true">
        {[['bg-success', report.ok_count], ['bg-warning', report.slow_count], ['bg-destructive', report.error_count], ['bg-muted-foreground/50', report.unknown_count ?? 0]].map(([color, count]) => (
          <span key={color} className={cn('status-meter', color as string)} style={{ width: `${report.total ? Number(count) / report.total * 100 : 0}%` }} />
        ))}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-xs text-muted-foreground">
        <span className="inline-flex items-center gap-1.5"><Server className="size-3.5" />{report.provider_count} 个 Provider</span>
        <span className="inline-flex items-center gap-1.5"><Clock3 className="size-3.5" />检测耗时 {report.elapsed_ms.toLocaleString()} ms</span>
        <span className="hidden sm:inline">并发 {report.global_concurrency} / {report.provider_concurrency}</span>
      </div>
    </section>
  )
}

function StatusMetric({ label, value, tone }: { label: string; value: number; tone?: string }) {
  return <div className="min-w-0 border-l px-2 first:border-l-0 first:pl-0 sm:px-5">
    <p className="text-xs text-muted-foreground">{label}</p>
    <strong key={value} className={cn('animate-value mt-1 block font-mono text-lg font-semibold tabular-nums sm:text-2xl', tone)}>{value.toLocaleString()}</strong>
  </div>
}

function FilterBar(props: {
  search: string; setSearch: (value: string) => void
  statusFilter: StatusFilter; setStatusFilter: (value: StatusFilter) => void
  sortBy: SortMode; setSortBy: (value: SortMode) => void
  viewMode: ViewMode; setViewMode: (value: ViewMode) => void
  resultCount: number
}) {
  return (
    <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="relative min-w-0 flex-1 xl:max-w-xs">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={props.search} onChange={event => props.setSearch(event.target.value)} aria-label="搜索 Provider、ID 或模型" placeholder="搜索 Provider、ID 或模型" className="bg-card pl-9 pr-9" />
          {props.search && <Button variant="ghost" size="icon" className="absolute right-0 top-0" onClick={() => props.setSearch('')} aria-label="清空搜索"><X /></Button>}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex rounded-md bg-muted p-0.5" role="group" aria-label="状态筛选">
            {(['all', 'ok', 'slow', 'error', 'unknown'] as const).map(status => (
              <Button key={status} variant="ghost" size="sm" className={cn('h-8 px-2.5 sm:px-3', props.statusFilter === status && 'bg-card text-foreground shadow-sm')} aria-pressed={props.statusFilter === status} onClick={() => props.setStatusFilter(status)}>
                {{ all: '全部', ok: '正常', slow: '较慢', error: '异常', unknown: '未检测' }[status]}
              </Button>
            ))}
          </div>
          <div className="flex min-w-[148px] items-center gap-2">
            <ArrowUpDown className="size-4 shrink-0 text-muted-foreground" />
            <Select value={props.sortBy} onValueChange={value => props.setSortBy(value as SortMode)}>
              <SelectTrigger aria-label="Provider 排序"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="default">默认顺序</SelectItem><SelectItem value="status">按状态</SelectItem><SelectItem value="name">按名称</SelectItem><SelectItem value="latency">按延迟</SelectItem><SelectItem value="models">按模型数</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="inline-flex rounded-md border bg-background p-0.5">
            <Tooltip><TooltipTrigger asChild><Button variant={props.viewMode === 'detailed' ? 'secondary' : 'ghost'} size="icon" className="size-8" onClick={() => props.setViewMode('detailed')} aria-label="详细视图" aria-pressed={props.viewMode === 'detailed'}><LayoutGrid /></Button></TooltipTrigger><TooltipContent>详细视图</TooltipContent></Tooltip>
            <Tooltip><TooltipTrigger asChild><Button variant={props.viewMode === 'compact' ? 'secondary' : 'ghost'} size="icon" className="size-8" onClick={() => props.setViewMode('compact')} aria-label="紧凑视图" aria-pressed={props.viewMode === 'compact'}><List /></Button></TooltipTrigger><TooltipContent>紧凑视图</TooltipContent></Tooltip>
          </div>
          <span className="ml-auto whitespace-nowrap text-xs text-muted-foreground xl:ml-1" role="status">{props.resultCount} 个结果</span>
        </div>
    </div>
  )
}

function DashboardSkeleton() {
  return <div className="space-y-5" aria-label="正在加载状态"><Skeleton className="h-36 w-full" /><Skeleton className="h-20" /><Skeleton className="h-9" /><Skeleton className="h-72" /><Skeleton className="h-40" /></div>
}

function EmptyState({ error }: { error: string }) {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center text-center">
      <div className="flex size-12 items-center justify-center rounded-lg bg-destructive/10 text-destructive"><XCircle /></div>
      <h1 className="mt-4 text-xl font-semibold">无法获取服务状态</h1>
      <p className="mt-2 max-w-lg font-mono text-sm text-muted-foreground">{error}</p>
      <Button asChild className="mt-5"><Link to="/admin"><Settings2 />前往管理后台</Link></Button>
    </div>
  )
}
