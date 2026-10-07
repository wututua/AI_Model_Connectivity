import { useEffect, useState, type ReactNode } from 'react'
import { Check, ChevronLeft, ChevronRight, RefreshCw, RotateCcw, Search } from 'lucide-react'
import { api } from '../../api'
import type { DiagnosticRecord, HistoryPage, Incident, MonitoringHistoryQuery } from '../../monitoring'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Field } from './shared'

const emptyQuery: MonitoringHistoryQuery = { provider_id: '', model: '', status: '', start: '', end: '', capability: '', error_type: '', scope: '', limit: 50, before: 0 }
const date = (value: string) => value ? new Date(value).toLocaleString() : '-'
const ms = (value?: number) => value == null ? '-' : `${value} ms`

export function MonitoringHistory({ kind, active, refresh, readOnly, onAcknowledge }: {
  kind: 'diagnostics' | 'incidents'; active: boolean; refresh: number; readOnly: boolean; onAcknowledge?: (incident: Incident) => void
}) {
  const incidents = kind === 'incidents'
  const [draft, setDraft] = useState(emptyQuery)
  const [query, setQuery] = useState(emptyQuery)
  const [cursors, setCursors] = useState([0])
  const [page, setPage] = useState<HistoryPage<DiagnosticRecord | Incident> | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const before = cursors[cursors.length - 1]
  useEffect(() => {
    if (!active) return
    const controller = new AbortController()
    setLoading(true); setError(''); setPage(null)
    api.monitoringHistory(kind, { ...query, before }, controller.signal)
      .then(value => { if (!controller.signal.aborted) setPage(value) })
      .catch(e => { if (!controller.signal.aborted) setError((e as Error).message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [kind, active, query, before, refresh, reload])

  const apply = () => {
    try {
      const start = draft.start ? new Date(draft.start).toISOString() : ''
      const end = draft.end ? new Date(draft.end).toISOString() : ''
      if (start && end && start >= end) throw new Error('结束时间必须晚于开始时间')
      setQuery({ ...draft, provider_id: draft.provider_id.trim(), model: draft.model.trim(), start, end })
      setCursors([0])
    } catch (e) { setError((e as Error).message) }
  }
  const set = <K extends keyof MonitoringHistoryQuery>(key: K, value: MonitoringHistoryQuery[K]) => setDraft(current => ({ ...current, [key]: value }))
  const options = (label: string, key: 'status' | 'scope' | 'capability', values: [string, string][]) =>
    <Field label={label}><Select value={draft[key] || '__all'} onValueChange={value => set(key, value === '__all' ? '' : value)}>
      <SelectTrigger aria-label={label}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="__all">全部</SelectItem>{values.map(([id, name]) => <SelectItem key={id} value={id}>{name}</SelectItem>)}</SelectContent>
    </Select></Field>
  return <section className="space-y-4" aria-label={incidents ? '事件历史' : '诊断历史'}>
    <form className="space-y-3" onSubmit={e => { e.preventDefault(); apply() }}>
      <div className="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <Field label="Provider ID"><Input aria-label="Provider ID" maxLength={256} value={draft.provider_id} onChange={e => set('provider_id', e.target.value)} /></Field>
        <Field label="模型"><Input aria-label="模型" maxLength={1024} disabled={draft.scope === 'discovery'} value={draft.model} onChange={e => set('model', e.target.value)} /></Field>
        {options('状态', 'status', incidents ? [['open', '待恢复'], ['resolved', '已恢复'], ['superseded', '配置已变更']] : [['ok', '正常'], ['slow', '缓慢'], ['error', '异常'], ['unknown', '未知']])}
        {incidents
          ? <Field label="事件范围"><Select value={draft.scope || '__all'} onValueChange={scope => setDraft(current => ({ ...current, scope: scope === '__all' ? '' : scope, model: scope === 'discovery' ? '' : current.model }))}><SelectTrigger aria-label="事件范围"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="__all">全部</SelectItem><SelectItem value="models">模型检测</SelectItem><SelectItem value="discovery">模型发现</SelectItem></SelectContent></Select></Field>
          : options('检测能力', 'capability', [['text', '文本'], ['tools', '工具调用'], ['embedding', 'Embedding']])}
        <Field label={incidents ? '发现时间起' : '检测时间起'}><Input aria-label="开始时间" type="datetime-local" value={draft.start} onChange={e => set('start', e.target.value)} /></Field>
        <Field label="结束时间（不含）"><Input aria-label="结束时间" type="datetime-local" value={draft.end} onChange={e => set('end', e.target.value)} /></Field>
        {!incidents && <Field label="错误类型"><Input aria-label="错误类型" maxLength={64} value={draft.error_type} onChange={e => set('error_type', e.target.value)} /></Field>}
        <Field label="每页"><Select value={String(draft.limit)} onValueChange={value => set('limit', Number(value))}><SelectTrigger aria-label="每页记录数"><SelectValue /></SelectTrigger><SelectContent>{[25, 50, 100].map(size => <SelectItem key={size} value={String(size)}>{size}</SelectItem>)}</SelectContent></Select></Field>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm"><Search />筛选</Button>
        <Button type="button" variant="outline" size="icon" title="重置筛选" aria-label="重置筛选" onClick={() => { setDraft(emptyQuery); setQuery({ ...emptyQuery }); setCursors([0]) }}><RotateCcw /></Button>
        <Button type="button" variant="outline" size="icon" title="刷新历史" aria-label="刷新历史" disabled={loading} onClick={() => { setCursors([0]); setReload(n => n + 1) }}><RefreshCw /></Button>
      </div>
    </form>
    {error && <p role="alert" className="break-words text-sm text-destructive">{error}</p>}
    <div aria-busy={loading} className="min-h-24">
      {loading ? <p className="py-8 text-center text-sm text-muted-foreground">正在加载历史记录</p> : page && <>
        <div className="max-w-full overflow-x-auto border-y"><table className="w-full min-w-[640px] text-left text-xs">
          <thead className="bg-muted/40"><tr>{(incidents ? ['Provider / 模型', '状态', '发现 / 最近观测', '恢复 / 处理', '操作'] : ['Provider / 模型', '检测时间', '结果', '网络阶段', '响应阶段']).map(header => <th key={header} className="whitespace-nowrap px-3 py-3 font-medium">{header}</th>)}</tr></thead>
          <tbody className="divide-y">{page.items.map(item => incidents
            ? <IncidentRow key={item.id} value={item as Incident} disabled={readOnly} onAcknowledge={onAcknowledge} />
            : <DiagnosticRow key={item.id} row={item as DiagnosticRecord} />)}</tbody>
        </table></div>
        {!page.items.length && <p className="py-8 text-center text-sm text-muted-foreground">暂无记录</p>}
      </>}
    </div>
    <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
      <span>第 {cursors.length} 页{page ? ` · ${page.items.length} 条` : ''}</span>
      <div className="flex gap-1">
        <Button variant="outline" size="icon" title="上一页" aria-label="上一页" disabled={loading || cursors.length <= 1} onClick={() => setCursors(current => current.slice(0, -1))}><ChevronLeft /></Button>
        <Button variant="outline" size="icon" title="下一页" aria-label="下一页" disabled={loading || !page?.has_more} onClick={() => { if (page?.has_more) setCursors(current => [...current, page.next_before]) }}><ChevronRight /></Button>
      </div>
    </div>
  </section>
}

function Cell({ children }: { children: ReactNode }) { return <td className="max-w-72 break-words px-3 py-3 align-top">{children}</td> }
function DiagnosticRow({ row }: { row: DiagnosticRecord }) {
  return <tr>
    <Cell><span className="font-medium">{row.provider_id}</span><br /><span className="break-all">{row.model}</span><br /><span className="text-muted-foreground">{row.capability || 'text'}</span></Cell>
    <Cell>{date(row.checked_at)}</Cell>
    <Cell>{row.status} · {row.error_type || row.capability_status || '-'}<br />HTTP {row.diagnostics?.http_status || '-'}
      {row.diagnostics?.request_id && <details><summary className="cursor-pointer">请求 ID</summary><span className="break-all">{row.diagnostics.request_id}</span></details>}
      {row.diagnostics?.retry_after && <div>Retry-After: {row.diagnostics.retry_after}</div>}
    </Cell>
    <Cell>DNS {ms(row.diagnostics?.dns_ms)}<br />连接 {ms(row.diagnostics?.connect_ms)}<br />TLS {ms(row.diagnostics?.tls_ms)}{row.diagnostics?.connection_reused && <div>复用连接</div>}</Cell>
    <Cell>首字节 {ms(row.diagnostics?.first_byte_ms)}<br />首段文本 {ms(row.first_token_ms || undefined)}<br />完整响应 {ms(row.latency_ms)}</Cell>
  </tr>
}
function IncidentRow({ value, disabled, onAcknowledge }: { value: Incident; disabled: boolean; onAcknowledge?: (incident: Incident) => void }) {
  return <tr>
    <Cell>{value.provider_id}<br /><span className="break-all">{value.model || '模型发现'}</span></Cell>
    <Cell>{({ open: '待恢复', resolved: '已恢复', superseded: '配置已变更' } as Record<string, string>)[value.status] || value.status}</Cell>
    <Cell>{date(value.opened_at)}<br />{date(value.last_seen_at)}</Cell>
    <Cell>{date(value.resolved_at)}<br />{value.acknowledged_at ? `已接手 ${date(value.acknowledged_at)}` : '未接手'}<p className="break-words">{value.note}</p></Cell>
    <Cell><Button variant="outline" size="sm" disabled={disabled || value.status !== 'open'} onClick={() => onAcknowledge?.(value)}><Check />接手 / 备注</Button></Cell>
  </tr>
}
