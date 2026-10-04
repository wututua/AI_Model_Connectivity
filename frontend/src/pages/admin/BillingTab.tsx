import { useCallback, useEffect, useRef, useState } from 'react'
import { Activity, Coins, Hash, RefreshCw, TextCursorInput } from 'lucide-react'
import { api } from '../../api'
import type { BillingSummary } from '../../types'
import { Button } from '../../components/ui/button'
import { Skeleton } from '../../components/ui/skeleton'
import { UsageTrend } from '../../components/UsageTrend'
import { formatTokens, formatUsageDate } from '../../utils/usageChart'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '../../components/ui/tabs'
import { Feedback } from './shared'

const RANGES = [{ label: '7 天', days: 7 }, { label: '30 天', days: 30 }, { label: '90 天', days: 90 }]

export function BillingTab() {
  const [days, setDays] = useState(30)
  const [data, setData] = useState<BillingSummary | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const requestId = useRef(0)
  const load = useCallback(() => {
    const request = ++requestId.current
    setLoading(true); setError('')
    setData(current => current?.range_days === days ? current : null)
    api.billing(days)
      .then(result => { if (request === requestId.current) setData(result) })
      .catch(cause => { if (request === requestId.current) setError(`错误：${(cause as Error).message}`) })
      .finally(() => { if (request === requestId.current) setLoading(false) })
  }, [days])
  useEffect(() => { load(); return () => { requestId.current++ } }, [load])

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3"><Tabs value={String(days)} onValueChange={value => setDays(Number(value))}><TabsList>{RANGES.map(range => <TabsTrigger key={range.days} value={String(range.days)}>{range.label}</TabsTrigger>)}</TabsList></Tabs><Button variant="outline" size="icon" onClick={load} disabled={loading} aria-label="刷新 Token 用量"><RefreshCw className={loading ? 'animate-spin' : ''} /></Button></div>
      <Feedback message={error} />
      {loading && !data && <div className="space-y-5" aria-label="正在加载 Token 用量"><Skeleton className="h-24" /><Skeleton className="h-72" /><Skeleton className="h-52" /></div>}
      {data && <>
        <div className="grid grid-cols-2 gap-y-5 py-2 xl:grid-cols-4">
          <UsageMetric icon={<Coins />} label="总 Token" value={formatTokens(data.total_tokens)} />
          <UsageMetric icon={<TextCursorInput />} label="Prompt" value={formatTokens(data.total_prompt_tokens)} />
          <UsageMetric icon={<Activity />} label="Completion" value={formatTokens(data.total_completion_tokens)} />
          <UsageMetric icon={<Hash />} label="探测次数" value={formatTokens(data.total_probe_count)} />
        </div>
        <UsageTrend key={`${data.range_start}:${data.range_end}`} daily={data.daily} />
        <section aria-label="模型用量明细">
          <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-sm font-semibold">模型用量明细</h2>
            <p className="text-xs text-muted-foreground">{formatUsageDate(data.range_start, true)} 至 {formatUsageDate(data.range_end, true)}</p>
          </div>
          <div className="border-y bg-card">
            <div className="hidden sm:block">
              <Table className="min-w-[640px]">
                <TableHeader><TableRow><TableHead className="min-w-36">Provider</TableHead><TableHead>模型</TableHead><TableHead className="text-right">Prompt</TableHead><TableHead className="text-right">Completion</TableHead><TableHead className="text-right">Total</TableHead><TableHead className="whitespace-nowrap text-right">次数</TableHead></TableRow></TableHeader>
                <TableBody>{data.per_model.map(item => <TableRow key={`${item.provider_id}:${item.model}`}>
                  <TableCell><p className="text-sm font-medium">{item.provider_name || item.provider_id}</p><p className="font-mono text-xs text-muted-foreground">{item.provider_id}</p></TableCell>
                  <TableCell className="max-w-72 truncate font-mono text-xs" title={item.model}>{item.model}</TableCell>
                  <TableCell className="whitespace-nowrap text-right font-mono text-xs">{formatTokens(item.prompt_tokens)}</TableCell>
                  <TableCell className="whitespace-nowrap text-right font-mono text-xs">{formatTokens(item.completion_tokens)}</TableCell>
                  <TableCell className="whitespace-nowrap text-right font-mono text-xs font-semibold">{formatTokens(item.total_tokens)}</TableCell>
                  <TableCell className="whitespace-nowrap text-right font-mono text-xs text-muted-foreground">{item.probe_count}</TableCell>
                </TableRow>)}</TableBody>
              </Table>
            </div>
            <div className="sm:hidden">
              {data.per_model.length > 0 && <div className="flex justify-between border-b px-3 py-2 text-xs text-muted-foreground"><span>模型 / Provider</span><span>Token / 次数</span></div>}
              <dl className="divide-y">{data.per_model.map(item => <div key={`${item.provider_id}:${item.model}`} className="grid grid-cols-[minmax(0,1fr)_72px] items-center gap-3 px-3 py-3">
                <dt className="min-w-0"><span className="block break-all font-mono text-xs font-medium">{item.model}</span><span className="mt-1 block text-xs text-muted-foreground">{item.provider_name || item.provider_id}</span></dt>
                <dd className="text-right"><span className="block font-mono text-sm font-semibold tabular-nums">{formatTokens(item.total_tokens)}</span><span className="mt-1 block text-xs tabular-nums text-muted-foreground">{item.probe_count} 次</span></dd>
              </div>)}</dl>
            </div>
            {data.per_model.length === 0 && <div className="flex min-h-40 items-center justify-center text-sm text-muted-foreground">所选时间范围内暂无 Token 数据</div>}
          </div>
        </section>
        <p className="text-xs leading-relaxed text-muted-foreground">数据来自探测响应中的 usage 字段；未返回该字段的本地服务或代理会记为 0。</p>
      </>}
    </div>
  )
}

function UsageMetric({ icon, label, value }: { icon: React.ReactNode; label: string; value: string }) {
  return <div className="min-w-0 border-l px-4 odd:border-l-0 odd:pl-0 xl:odd:border-l xl:odd:pl-4 xl:first:border-l-0 xl:first:pl-0"><div className="flex items-center gap-2 text-muted-foreground"><span className="[&>svg]:size-3.5">{icon}</span><span className="data-label">{label}</span></div><p key={value} className="animate-value mt-2 font-mono text-2xl font-semibold tabular-nums">{value}</p></div>
}
