import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Activity, AlertTriangle, ArrowUpDown, CheckCircle2, Clock3, Gauge,
  LayoutGrid, List, RefreshCw, Search, Server, Settings2, X, XCircle,
} from 'lucide-react'
import type { ProviderReport, Report } from '../types'
import { api } from '../api'
import { cn } from '../lib/utils'
import { relativeTime, statusClass } from '../utils/status'
import { ProviderCard } from '../components/ProviderCard'
import { StatusPill } from '../components/StatusPill'
import { SummaryCard } from '../components/SummaryCard'
import { ThemeToggle } from '../components/ThemeToggle'
import { Alert, AlertDescription, AlertTitle } from '../components/ui/alert'
import { Badge } from '../components/ui/badge'
import { Button } from '../components/ui/button'
import { Card, CardContent } from '../components/ui/card'
import { Input } from '../components/ui/input'
import { Progress } from '../components/ui/progress'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../components/ui/select'
import { Skeleton } from '../components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipTrigger } from '../components/ui/tooltip'

type StatusFilter = 'all' | 'ok' | 'slow' | 'error'
type SortMode = 'default' | 'status' | 'name' | 'latency' | 'models'
type ViewMode = 'detailed' | 'compact'

export default function Dashboard() {
  const [report, setReport] = useState<Report | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [live, setLive] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [sortBy, setSortBy] = useState<SortMode>('default')
  const [viewMode, setViewMode] = useState<ViewMode>('detailed')

  const fetchReport = useCallback(() => api.status()
    .then(data => { setReport(data); setError(null) })
    .catch(cause => setError((cause as Error).message)), [])

  const handleRefresh = useCallback(() => {
    setRefreshing(true)
    fetchReport().finally(() => setRefreshing(false))
  }, [fetchReport])

  useEffect(() => {
    fetchReport()
    if (!window.EventSource) {
      let delay = 30_000
      let timerId: ReturnType<typeof setTimeout>
      const poll = () => {
        fetchReport()
        delay = Math.min(delay * 2, 120_000)
        timerId = setTimeout(poll, delay)
      }
      timerId = setTimeout(poll, delay)
      return () => clearTimeout(timerId)
    }

    let source: EventSource | null = null
    let reconnectDelay = 30_000
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let closed = false

    const connect = () => {
      source = new EventSource('/api/events')
      source.onmessage = event => {
        try {
          setReport(JSON.parse(event.data) as Report)
          setError(null)
          setLive(true)
          reconnectDelay = 30_000
        } catch {
          return
        }
      }
      source.onerror = () => {
        setLive(false)
        source?.close()
        if (!closed) {
          reconnectTimer = setTimeout(() => {
            reconnectDelay = Math.min(reconnectDelay * 2, 120_000)
            connect()
          }, reconnectDelay)
        }
      }
    }

    connect()
    return () => {
      closed = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      source?.close()
    }
  }, [fetchReport])

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
      .filter(provider => provider.results.length > 0 || (!query && statusFilter === 'all'))
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
      <header className="sticky top-0 z-40 border-b bg-background/92 backdrop-blur-xl">
        <div className="mx-auto flex h-14 max-w-[1320px] items-center justify-between px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground"><Activity className="size-4" /></div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">{report?.title ?? '模型连通性'}</p>
              <p className="hidden text-xs text-muted-foreground sm:block">AI 服务运行状态</p>
            </div>
            {live && <Badge variant="success" className="hidden sm:inline-flex"><span className="mr-1 size-1.5 animate-pulse rounded-full bg-success" />实时</Badge>}
          </div>
          <div className="flex items-center gap-1">
            <Tooltip>
              <TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={handleRefresh} disabled={refreshing} aria-label="刷新状态"><RefreshCw className={cn(refreshing && 'animate-spin')} /></Button></TooltipTrigger>
              <TooltipContent>刷新状态</TooltipContent>
            </Tooltip>
            <ThemeToggle />
            <Button asChild variant="outline" size="sm" className="ml-1"><Link to="/admin"><Settings2 />管理后台</Link></Button>
          </div>
        </div>
      </header>

      <main className="page-surface mx-auto min-h-[calc(100vh-3.5rem)] max-w-[1320px] border-x px-4 py-6 sm:px-6 lg:py-8">
        {!report && !error ? <DashboardSkeleton /> : null}

        {error && !report ? (
          <EmptyState error={error} />
        ) : report ? (
          <div className="space-y-5 animate-enter">
            <StatusOverview report={report} live={live} />

            <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
              <SummaryCard icon={<Gauge className="size-4" />} label="模型总数" value={report.total} />
              <SummaryCard icon={<CheckCircle2 className="size-4" />} label="正常" value={report.ok_count} status="ok" />
              <SummaryCard icon={<AlertTriangle className="size-4" />} label="较慢" value={report.slow_count} status={report.slow_count ? 'slow' : undefined} />
              <SummaryCard icon={<XCircle className="size-4" />} label="异常" value={report.error_count} status={report.error_count ? 'error' : undefined} />
              <SummaryCard icon={<Server className="size-4" />} label="Provider" value={report.provider_count} />
              <SummaryCard icon={<Clock3 className="size-4" />} label="检测耗时" value={`${report.elapsed_ms} ms`} />
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
                <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
                  {filteredProviders.map(provider => <ProviderCard key={provider.provider_id} provider={provider} showError compact={viewMode === 'compact'} />)}
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

function StatusOverview({ report, live }: { report: Report; live: boolean }) {
  const tone = statusClass(report.overall_class)
  const availability = report.total > 0 ? Math.round((report.ok_count / report.total) * 100) : 0
  const updated = relativeTime(report.generated_at)
  return (
    <Card className="overflow-hidden border-l-4 border-l-primary shadow-panel">
      <CardContent className="grid gap-6 p-5 md:grid-cols-[1fr_300px] md:items-center">
        <div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <StatusPill status={report.overall_class} label={report.overall_status} large />
            <Badge variant={live ? 'success' : 'muted'}>{live ? '实时连接' : '等待推送'}</Badge>
            {updated.stale && <Badge variant="warning">数据可能已过期</Badge>}
          </div>
          <h1 className="text-2xl font-semibold sm:text-3xl">{tone === 'ok' ? '所有服务运行正常' : tone === 'slow' ? '部分服务响应较慢' : '检测到服务异常'}</h1>
          <p className="mt-2 text-sm text-muted-foreground">更新于 <span title={report.generated_at}>{updated.text}</span>，全局 / Provider 并发为 {report.global_concurrency} / {report.provider_concurrency}</p>
        </div>
        <div className="rounded-lg border bg-muted/30 p-4">
          <div className="flex items-end justify-between"><span className="data-label">当前可用率</span><strong className="font-mono text-3xl font-semibold tabular-nums">{availability}%</strong></div>
          <Progress value={availability} className="mt-3" />
          <div className="mt-3 flex justify-between text-xs text-muted-foreground"><span>{report.ok_count} 个正常</span><span>{report.total} 个模型</span></div>
        </div>
      </CardContent>
    </Card>
  )
}

function FilterBar(props: {
  search: string; setSearch: (value: string) => void
  statusFilter: StatusFilter; setStatusFilter: (value: StatusFilter) => void
  sortBy: SortMode; setSortBy: (value: SortMode) => void
  viewMode: ViewMode; setViewMode: (value: ViewMode) => void
  resultCount: number
}) {
  return (
    <Card>
      <CardContent className="flex flex-col gap-3 p-3 lg:flex-row lg:items-center">
        <div className="relative min-w-0 flex-1 lg:max-w-sm">
          <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input value={props.search} onChange={event => props.setSearch(event.target.value)} placeholder="搜索 Provider、ID 或模型" className="pl-9 pr-9" />
          {props.search && <Button variant="ghost" size="icon" className="absolute right-0 top-0" onClick={() => props.setSearch('')} aria-label="清空搜索"><X /></Button>}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="inline-flex rounded-md border bg-background p-0.5">
            {(['all', 'ok', 'slow', 'error'] as const).map(status => (
              <Button key={status} variant={props.statusFilter === status ? 'secondary' : 'ghost'} size="sm" className="h-7" onClick={() => props.setStatusFilter(status)}>
                {{ all: '全部', ok: '正常', slow: '较慢', error: '异常' }[status]}
              </Button>
            ))}
          </div>
          <div className="flex min-w-[148px] items-center gap-2">
            <ArrowUpDown className="size-4 shrink-0 text-muted-foreground" />
            <Select value={props.sortBy} onValueChange={value => props.setSortBy(value as SortMode)}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="default">默认顺序</SelectItem><SelectItem value="status">按状态</SelectItem><SelectItem value="name">按名称</SelectItem><SelectItem value="latency">按延迟</SelectItem><SelectItem value="models">按模型数</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="inline-flex rounded-md border bg-background p-0.5">
            <Tooltip><TooltipTrigger asChild><Button variant={props.viewMode === 'detailed' ? 'secondary' : 'ghost'} size="icon" className="size-7" onClick={() => props.setViewMode('detailed')} aria-label="详细视图"><LayoutGrid /></Button></TooltipTrigger><TooltipContent>详细视图</TooltipContent></Tooltip>
            <Tooltip><TooltipTrigger asChild><Button variant={props.viewMode === 'compact' ? 'secondary' : 'ghost'} size="icon" className="size-7" onClick={() => props.setViewMode('compact')} aria-label="紧凑视图"><List /></Button></TooltipTrigger><TooltipContent>紧凑视图</TooltipContent></Tooltip>
          </div>
          <Badge variant="outline">{props.resultCount} 个 Provider</Badge>
        </div>
      </CardContent>
    </Card>
  )
}

function DashboardSkeleton() {
  return <div className="space-y-5"><Skeleton className="h-44 w-full" /><div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">{Array.from({ length: 6 }).map((_, index) => <Skeleton key={index} className="h-24" />)}</div><Skeleton className="h-16" /><div className="grid gap-4 xl:grid-cols-2"><Skeleton className="h-96" /><Skeleton className="h-96" /></div></div>
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
