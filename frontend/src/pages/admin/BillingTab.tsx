import { useCallback, useEffect, useMemo, useState } from 'react'
import { Activity, Coins, Hash, RefreshCw, TextCursorInput } from 'lucide-react'
import { api } from '../../api'
import type { BillingSummary } from '../../types'
import { Button } from '../../components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../../components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '../../components/ui/tabs'
import { Feedback } from './shared'

const RANGES = [{ label: '7 天', days: 7 }, { label: '30 天', days: 30 }, { label: '90 天', days: 90 }]

export function BillingTab() {
  const [days, setDays] = useState(30)
  const [data, setData] = useState<BillingSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const load = useCallback(() => { setLoading(true); setError(''); api.billing(days).then(setData).catch(cause => setError(`错误：${(cause as Error).message}`)).finally(() => setLoading(false)) }, [days])
  useEffect(() => { load() }, [load])

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3"><Tabs value={String(days)} onValueChange={value => setDays(Number(value))}><TabsList>{RANGES.map(range => <TabsTrigger key={range.days} value={String(range.days)}>{range.label}</TabsTrigger>)}</TabsList></Tabs><Button variant="outline" size="icon" onClick={load} disabled={loading} aria-label="刷新 Token 用量"><RefreshCw className={loading ? 'animate-spin' : ''} /></Button></div>
      <Feedback message={error} />
      {data && <>
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <UsageMetric icon={<Coins />} label="总 Token" value={formatTokens(data.total_tokens)} />
          <UsageMetric icon={<TextCursorInput />} label="Prompt" value={formatTokens(data.total_prompt_tokens)} />
          <UsageMetric icon={<Activity />} label="Completion" value={formatTokens(data.total_completion_tokens)} />
          <UsageMetric icon={<Hash />} label="探测次数" value={formatTokens(data.total_probe_count)} />
        </div>
        <UsageTrend data={data} />
        <Card><CardHeader><CardTitle className="text-sm">模型用量明细</CardTitle><CardDescription>{data.range_start} 至 {data.range_end}</CardDescription></CardHeader><CardContent className="p-0"><Table>
          <TableHeader><TableRow><TableHead>Provider</TableHead><TableHead>模型</TableHead><TableHead className="hidden sm:table-cell text-right">Prompt</TableHead><TableHead className="hidden md:table-cell text-right">Completion</TableHead><TableHead className="text-right">Total</TableHead><TableHead className="text-right">次数</TableHead></TableRow></TableHeader>
          <TableBody>{data.per_model.map(item => <TableRow key={`${item.provider_id}:${item.model}`}><TableCell><p className="text-sm font-medium">{item.provider_name || item.provider_id}</p><p className="font-mono text-xs text-muted-foreground">{item.provider_id}</p></TableCell><TableCell className="max-w-72 truncate font-mono text-xs" title={item.model}>{item.model}</TableCell><TableCell className="hidden text-right font-mono text-xs sm:table-cell">{formatTokens(item.prompt_tokens)}</TableCell><TableCell className="hidden text-right font-mono text-xs md:table-cell">{formatTokens(item.completion_tokens)}</TableCell><TableCell className="text-right font-mono text-xs font-semibold">{formatTokens(item.total_tokens)}</TableCell><TableCell className="text-right font-mono text-xs text-muted-foreground">{item.probe_count}</TableCell></TableRow>)}</TableBody>
        </Table>{data.per_model.length === 0 && <div className="flex min-h-40 items-center justify-center text-sm text-muted-foreground">所选时间范围内暂无 Token 数据</div>}</CardContent></Card>
        <p className="text-xs leading-relaxed text-muted-foreground">数据来自探测响应中的 usage 字段；未返回该字段的本地服务或代理会记为 0。</p>
      </>}
    </div>
  )
}

function UsageMetric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return <Card><CardContent className="p-4"><div className="flex items-center justify-between text-muted-foreground"><span className="data-label">{label}</span><span className="[&>svg]:size-4">{icon}</span></div><p className="mt-3 font-mono text-2xl font-semibold tabular-nums">{value}</p></CardContent></Card>
}

function UsageTrend({ data }: { data: BillingSummary }) {
  const chart = useMemo(() => {
    if (!data.daily.length) return null
    const width = 800, height = 160, padding = 8
    const max = Math.max(...data.daily.map(day => day.total_tokens), 1)
    const step = data.daily.length > 1 ? (width - padding * 2) / (data.daily.length - 1) : 0
    const points = data.daily.map((day, index) => `${padding + index * step},${height - padding - (day.total_tokens / max) * (height - padding * 2)}`)
    const line = points.length === 1 ? `M ${points[0]}` : `M ${points.join(' L ')}`
    const area = `${line} L ${padding + (data.daily.length - 1) * step},${height - padding} L ${padding},${height - padding} Z`
    return { width, height, line, area, max }
  }, [data])

  if (!chart) return null
  return <Card><CardHeader className="flex-row items-start justify-between space-y-0"><div><CardTitle className="text-sm">每日 Token 趋势</CardTitle><CardDescription>{data.daily.length} 天采样</CardDescription></div><span className="font-mono text-xs text-muted-foreground">峰值 {formatTokens(chart.max)}</span></CardHeader><CardContent><svg viewBox={`0 0 ${chart.width} ${chart.height}`} className="h-44 w-full" preserveAspectRatio="none" aria-label="每日 Token 消耗趋势"><path d={chart.area} fill="hsl(var(--primary))" opacity=".12" /><path d={chart.line} stroke="hsl(var(--primary))" strokeWidth="2" fill="none" vectorEffect="non-scaling-stroke" /></svg><div className="mt-2 flex justify-between text-xs text-muted-foreground"><span>{data.daily[0]?.day}</span><span>{data.daily[data.daily.length - 1]?.day}</span></div></CardContent></Card>
}

function formatTokens(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return String(value)
}
