import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Check, DatabaseBackup, Plus, RefreshCw, RotateCcw, Save, Send, ShieldCheck, Trash2 } from 'lucide-react'
import { api } from '../../api'
import type { MonitoringData, MonitoringSettings } from '../../monitoring'
import type { SafeProviderConfig } from '../../types'
import { UnsavedChanges } from '../../components/UnsavedChanges'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import { Textarea } from '../../components/ui/textarea'
import { Switch } from '../../components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog'
import { Feedback, Field } from './shared'

const date = (value: string) => value ? new Date(value).toLocaleString() : '-'
const ms = (value?: number) => value == null ? '-' : `${value} ms`
const dollars = (value: number) => `$${value.toFixed(6)}`
const tabs = [['diagnostics', '诊断'], ['catalog', '模型变更'], ['backups', '备份'], ['rules', '告警规则'], ['incidents', '事件'], ['schedules', '调度'], ['costs', '费用']] as const

export function MonitoringTab() {
  const [data, setData] = useState<MonitoringData | null>(null)
  const [draft, setDraft] = useState<MonitoringSettings | null>(null)
  const [providers, setProviders] = useState<SafeProviderConfig[]>([])
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [tab, setTab] = useState('diagnostics')
  const [ack, setAck] = useState<number | null>(null)
  const [note, setNote] = useState('')
  const [approval, setApproval] = useState<string | null>(null)
  const [discard, setDiscard] = useState(false)
  const alive = useRef(false)
  const locked = useRef(false)
  const dirty = !!data && !!draft && JSON.stringify(draft) !== JSON.stringify(data.settings)
  useEffect(() => {
    alive.current = true
    const controller = new AbortController()
    setBusy(true)
    Promise.all([api.monitoring(controller.signal), api.providers()])
      .then(([value, list]) => { if (alive.current && !controller.signal.aborted) { setData(value); setDraft(value.settings); setProviders(list) } })
      .catch(e => { if (alive.current && !controller.signal.aborted) setMessage(`错误：${(e as Error).message}`) })
      .finally(() => { if (alive.current && !controller.signal.aborted) setBusy(false) })
    return () => { alive.current = false; controller.abort() }
  }, [])
  const act = async (operation?: () => Promise<unknown>) => {
    if (locked.current || busy) return
    locked.current = true; setBusy(true); setMessage('')
    try {
      if (operation) await operation()
      if (!alive.current) return
      const value = await api.monitoring()
      if (!alive.current) return
      setData(value); setDraft(value.settings); setAck(null); setApproval(null)
      setMessage(operation ? '操作完成' : '已刷新')
    } catch (e) { if (alive.current) setMessage(`错误：${(e as Error).message}`) }
    finally { locked.current = false; if (alive.current) setBusy(false) }
  }
  const update = <K extends keyof MonitoringSettings>(key: K, value: MonitoringSettings[K]) => setDraft(previous => previous ? { ...previous, [key]: value } : previous)
  const editItem = <K extends 'rules' | 'schedules' | 'prices'>(key: K, index: number, patch: Partial<MonitoringSettings[K][number]>) =>
    update(key, draft![key].map((item, i) => i === index ? { ...item, ...patch } : item) as MonitoringSettings[K])
  const remove = (key: 'rules' | 'schedules' | 'prices', index: number) => update(key, draft![key].filter((_, i) => i !== index) as never)
  const selector = (value: string, onChange: (value: string) => void, all = false) => <Select value={value || '__all'} onValueChange={next => onChange(next === '__all' ? '' : next)}>
    <SelectTrigger aria-label="Provider"><SelectValue /></SelectTrigger><SelectContent>
      <SelectItem value="__all">{all ? '全部 Provider' : '选择 Provider'}</SelectItem>
      {value && !providers.some(p => p.id === value) && <SelectItem value={value}>{value}（已移除）</SelectItem>}
      {providers.map(p => <SelectItem key={p.id} value={p.id}>{p.name || p.id}</SelectItem>)}
    </SelectContent>
  </Select>
  return <div className="space-y-4">
    <UnsavedChanges dirty={dirty} />
    <div className="flex flex-wrap items-center justify-between gap-2">
      <span className="text-xs text-muted-foreground">{data ? `配置版本 ${data.settings.version}` : '监控数据'}</span>
      <div className="flex gap-2">
        {dirty && <Button variant="outline" size="icon" title="放弃更改并刷新" aria-label="放弃更改并刷新" disabled={busy} onClick={() => setDiscard(true)}><RotateCcw /></Button>}
        <Button variant="outline" size="icon" title="刷新" aria-label="刷新监控中心" disabled={busy || dirty} onClick={() => void act()}><RefreshCw className={busy ? 'animate-spin' : ''} /></Button>
        <Button disabled={!dirty || busy} onClick={() => { if (draft) void act(async () => {
          const saved = await api.saveMonitoring(draft)
          if (alive.current) { setDraft(saved); setData(previous => previous ? { ...previous, settings: saved } : previous) }
        }) }}><Save />保存配置</Button>
      </div>
    </div>
    <Feedback message={message} />
    {!data || !draft ? <p className="py-8 text-sm text-muted-foreground">{busy ? '正在加载' : '暂无数据'}</p> : <Tabs value={tab} onValueChange={setTab}>
      <TabsList className="h-auto flex-wrap justify-start gap-y-1">{tabs.map(([id, label]) => <TabsTrigger key={id} value={id}>{label}</TabsTrigger>)}</TabsList>
      <fieldset disabled={busy} className="min-w-0">
        <TabsContent value="diagnostics" className="mt-5 space-y-3">
          <DataTable headers={['Provider / 模型', '检测时间', '结果', '网络阶段', '响应阶段']}>
            {data.diagnostics.map(row => <tr key={row.id}>
              <Cell><span className="font-medium">{row.provider_id}</span><br />{row.model}<br /><span className="text-muted-foreground">{row.capability || 'text'}</span></Cell>
              <Cell>{date(row.checked_at)}</Cell>
              <Cell>{row.status} · {row.error_type || row.capability_status || '-'}<br />HTTP {row.diagnostics?.http_status || '-'}
                {row.diagnostics?.request_id && <details><summary className="cursor-pointer">请求 ID</summary><span className="break-all">{row.diagnostics.request_id}</span></details>}
                {row.diagnostics?.retry_after && <div>Retry-After: {row.diagnostics.retry_after}</div>}
              </Cell>
              <Cell>DNS {ms(row.diagnostics?.dns_ms)}<br />连接 {ms(row.diagnostics?.connect_ms)}<br />TLS {ms(row.diagnostics?.tls_ms)}{row.diagnostics?.connection_reused && <div>复用连接</div>}</Cell>
              <Cell>首字节 {ms(row.diagnostics?.first_byte_ms)}<br />首段文本 {ms(row.first_token_ms || undefined)}<br />完整响应 {ms(row.latency_ms)}</Cell>
            </tr>)}
          </DataTable>
          {!data.diagnostics.length && <Empty />}
        </TabsContent>
        <TabsContent value="catalog" className="mt-5 space-y-5">
          <div className="divide-y border-y">{data.catalogs.map(catalog => <section key={catalog.provider_id} className="space-y-3 py-4">
            <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="break-all text-sm font-semibold">{catalog.provider_id}</h2>
              <Button size="sm" variant="outline" disabled={dirty || !catalog.added.length} onClick={() => setApproval(catalog.provider_id)}><Check />批准新增模型（{catalog.added.length}）</Button></div>
            <p className="text-xs text-muted-foreground">{date(catalog.updated_at)} · 当前 {catalog.models.length} 个 · 待批准 {catalog.added.length} 个</p>
            {!!catalog.added.length && <p className="break-all text-sm text-warning">{catalog.added.join(', ')}</p>}
          </section>)}{!data.catalogs.length && <Empty />}</div>
          <DataTable headers={['时间', 'Provider', '新增', '移除']}>{data.catalog_events.map(event => <tr key={event.id}><Cell>{date(event.created_at)}</Cell><Cell>{event.provider_id}</Cell><Cell>{event.added.join(', ') || '-'}</Cell><Cell>{event.removed.join(', ') || '-'}</Cell></tr>)}</DataTable>
        </TabsContent>
        <TabsContent value="backups" className="mt-5 space-y-5">
          <div className="field-grid"><NumberField label="备份周期（小时，0 为关闭）" value={draft.backup_interval_hours} max={8760} onChange={n => update('backup_interval_hours', n)} /><NumberField label="保留数量" value={draft.backup_keep} min={1} max={100} onChange={n => update('backup_keep', n)} /></div>
          <Button variant="outline" disabled={dirty} onClick={() => void act(() => api.monitoringAction('backup'))}><DatabaseBackup />立即备份</Button>
          <DataTable headers={['备份', '大小', '校验时间', '操作']}>{data.backups.map(backup => <tr key={backup.name}><Cell><span className="break-all">{backup.name}</span><br />{date(backup.created_at)}<details><summary className="cursor-pointer">SHA-256</summary><span className="break-all">{backup.sha256}</span></details></Cell><Cell>{(backup.size / 1048576).toFixed(2)} MiB</Cell><Cell>{date(backup.verified_at)}</Cell><Cell><Button variant="ghost" size="icon" title="校验备份" aria-label={`校验 ${backup.name}`} disabled={dirty} onClick={() => void act(() => api.monitoringAction('verify', { name: backup.name }))}><ShieldCheck /></Button></Cell></tr>)}</DataTable>
          {!data.backups.length && <Empty />}
          <DataTable headers={['时间', '运行记录']}>{data.events.map(event => <tr key={event.id}><Cell>{date(event.created_at)}</Cell><Cell>{event.kind}: {event.detail}</Cell></tr>)}</DataTable>
        </TabsContent>
        <TabsContent value="rules" className="mt-5 space-y-5">
          <Button variant="outline" disabled={draft.rules.length >= 50} onClick={() => update('rules', [...draft.rules, { id: crypto.randomUUID(), name: '', enabled: false, provider_id: '', model: '', platform: 'webhook', failure_threshold: 2, recovery_threshold: 2, cooldown_minutes: 30 }])}><Plus />添加规则</Button>
          <div className="divide-y border-y">{draft.rules.map((rule, index) => <section key={rule.id} className="space-y-4 py-5">
            <div className="flex items-center justify-between gap-3"><label className="flex items-center gap-2 text-sm"><Switch checked={rule.enabled} onCheckedChange={enabled => editItem('rules', index, { enabled })} aria-label={`启用规则 ${index + 1}`} />启用</label><div className="flex gap-1"><Button size="icon" variant="ghost" title="测试已保存规则" aria-label={`测试规则 ${index + 1}`} disabled={dirty} onClick={() => void act(() => api.monitoringAction('test-rule', { id: rule.id }))}><Send /></Button><DeleteButton label={`删除规则 ${index + 1}`} onClick={() => remove('rules', index)} /></div></div>
            <div className="field-grid">
              <TextField label="规则名称" value={rule.name} onChange={name => editItem('rules', index, { name })} />
              <Field label="Provider">{selector(rule.provider_id, provider_id => editItem('rules', index, { provider_id }), true)}</Field>
              <TextField label="模型（空为全部）" value={rule.model} onChange={model => editItem('rules', index, { model })} />
              <Field label="通知渠道"><Select value={rule.platform} onValueChange={platform => editItem('rules', index, { platform, url: '', token: '', chat_id: '', credentials_set: false })}><SelectTrigger aria-label="通知渠道"><SelectValue /></SelectTrigger><SelectContent>{['webhook', 'telegram', 'discord', 'bark', 'wecom', 'dingtalk'].map(platform => <SelectItem key={platform} value={platform}>{platform}</SelectItem>)}</SelectContent></Select></Field>
              {rule.platform === 'telegram' ? <><TextField label="Bot Token" secret placeholder={rule.credentials_set ? '已保存' : ''} value={rule.token || ''} onChange={token => editItem('rules', index, { token })} /><TextField label="Chat ID" secret placeholder={rule.credentials_set ? '已保存' : ''} value={rule.chat_id || ''} onChange={chat_id => editItem('rules', index, { chat_id })} /></> : <TextField label="Webhook URL" secret placeholder={rule.credentials_set ? '已保存' : 'https://'} value={rule.url || ''} onChange={url => editItem('rules', index, { url })} />}
              <NumberField label="连续异常次数" value={rule.failure_threshold} min={1} max={100} onChange={failure_threshold => editItem('rules', index, { failure_threshold })} />
              <NumberField label="连续恢复次数" value={rule.recovery_threshold} min={1} max={100} onChange={recovery_threshold => editItem('rules', index, { recovery_threshold })} />
              <NumberField label="冷却（分钟）" value={rule.cooldown_minutes} max={525600} onChange={cooldown_minutes => editItem('rules', index, { cooldown_minutes })} />
              <label className="flex items-center justify-between gap-3 text-sm">模型清单变更<Switch checked={!!rule.catalog_changes} onCheckedChange={catalog_changes => editItem('rules', index, { catalog_changes })} aria-label="模型清单变更提醒" /></label>
              {!rule.provider_id && !rule.model && <>
                <label className="flex items-center justify-between gap-3 text-sm">备份失败<Switch checked={!!rule.backup_failures} onCheckedChange={backup_failures => editItem('rules', index, { backup_failures })} aria-label="备份失败提醒" /></label>
                <label className="flex items-center justify-between gap-3 text-sm">月度预算<Switch checked={!!rule.budget_alerts} onCheckedChange={budget_alerts => editItem('rules', index, { budget_alerts })} aria-label="月度预算提醒" /></label>
              </>}
            </div>
          </section>)}{!draft.rules.length && <Empty />}</div>
        </TabsContent>
        <TabsContent value="incidents" className="mt-5">
          <DataTable headers={['Provider / 模型', '状态', '发现 / 最近观测', '恢复 / 处理', '操作']}>{data.incidents.map(incident => <tr key={incident.id}>
            <Cell>{incident.provider_id}<br />{incident.model || '模型发现'}</Cell><Cell>{({ open: '待恢复', resolved: '已恢复', superseded: '配置已变更' } as Record<string, string>)[incident.status] || incident.status}</Cell>
            <Cell>{date(incident.opened_at)}<br />{date(incident.last_seen_at)}</Cell><Cell>{date(incident.resolved_at)}<br />{incident.acknowledged_at ? `已接手 ${date(incident.acknowledged_at)}` : '未接手'}<p className="break-words">{incident.note}</p></Cell>
            <Cell><Button variant="outline" size="sm" disabled={dirty || incident.status !== 'open'} onClick={() => { setAck(incident.id); setNote(incident.note) }}><Check />接手 / 备注</Button></Cell>
          </tr>)}</DataTable>{!data.incidents.length && <Empty />}
        </TabsContent>
        <TabsContent value="schedules" className="mt-5 space-y-5">
          <Button variant="outline" onClick={() => update('schedules', [...draft.schedules, { provider_id: '', interval_minutes: 0, slow_threshold_ms: 0, maintenance_start: '', maintenance_end: '' }])}><Plus />添加 Provider 策略</Button>
          <div className="divide-y border-y">{draft.schedules.map((schedule, index) => <section key={index} className="space-y-4 py-5">
            <div className="flex justify-end"><DeleteButton label={`删除策略 ${index + 1}`} onClick={() => remove('schedules', index)} /></div>
            <div className="field-grid">
              <Field label="Provider">{selector(schedule.provider_id, provider_id => editItem('schedules', index, { provider_id }))}</Field>
              <NumberField label="检测周期（分钟，0 为继承）" value={schedule.interval_minutes} max={525600} onChange={interval_minutes => editItem('schedules', index, { interval_minutes })} />
              <NumberField label="慢响应阈值（毫秒，0 为继承）" value={schedule.slow_threshold_ms} max={86400000} onChange={slow_threshold_ms => editItem('schedules', index, { slow_threshold_ms })} />
              <div />
              <DateField label="维护开始（UTC）" value={schedule.maintenance_start} onChange={maintenance_start => editItem('schedules', index, { maintenance_start })} />
              <DateField label="维护结束（UTC）" value={schedule.maintenance_end} onChange={maintenance_end => editItem('schedules', index, { maintenance_end })} />
            </div>
          </section>)}</div>
          <DataTable headers={['Provider', '下次检测', '周期']}>{data.schedules.map(schedule => <tr key={schedule.provider_id}><Cell>{schedule.provider_id}</Cell><Cell>{date(schedule.next_at)}</Cell><Cell>{schedule.interval_minutes} min</Cell></tr>)}</DataTable>
        </TabsContent>
        <TabsContent value="costs" className="mt-5 space-y-5">
          <div className="grid grid-cols-1 gap-4 border-y py-4 sm:grid-cols-3"><Metric label={`${data.cost.month} 估算（USD）`} value={dollars(data.cost.estimated_usd)} /><Metric label="未知费用请求" value={String(data.cost.unknown_probes)} /><Metric label="预算状态" value={data.cost.budget_exceeded ? '已达预算' : data.cost.budget_usd > 0 ? dollars(data.cost.budget_usd) : '未设置'} /></div>
          <NumberField label="月度提醒预算（USD，0 为关闭）" value={draft.monthly_budget} max={1e9} step="any" onChange={n => update('monthly_budget', n)} />
          <p className="text-xs text-warning">费用按记录入库时的价格估算，不包含未知消耗，不是账单或强制费用上限。历史金额不重算。</p>
          <Button variant="outline" onClick={() => update('prices', [...draft.prices, { provider_id: '', model: '', input_per_million: 0, output_per_million: 0 }])}><Plus />添加模型价格</Button>
          <div className="divide-y border-y">{draft.prices.map((price, index) => <section key={index} className="space-y-3 py-4"><div className="flex justify-end"><DeleteButton label={`删除价格 ${index + 1}`} onClick={() => remove('prices', index)} /></div><div className="field-grid">
            <Field label="Provider">{selector(price.provider_id, provider_id => editItem('prices', index, { provider_id }))}</Field>
            <TextField label="模型" value={price.model} onChange={model => editItem('prices', index, { model })} />
            <NumberField label="输入 USD / 百万 Token" value={price.input_per_million} max={1e9} step="any" onChange={input_per_million => editItem('prices', index, { input_per_million })} />
            <NumberField label="输出 USD / 百万 Token" value={price.output_per_million} max={1e9} step="any" onChange={output_per_million => editItem('prices', index, { output_per_million })} />
          </div></section>)}</div>
          <DataTable headers={['Provider / 模型', '估算 USD', '已计价请求', '未知请求']}>{data.cost.items.map(item => <tr key={JSON.stringify([item.provider_id, item.model])}><Cell>{item.provider_id}<br />{item.model}</Cell><Cell>{dollars(item.estimated_usd)}</Cell><Cell>{item.priced_probes}</Cell><Cell>{item.unknown_probes}</Cell></tr>)}</DataTable>
        </TabsContent>
      </fieldset>
    </Tabs>}
    <AlertDialog open={ack !== null} onOpenChange={open => { if (!open && !busy) setAck(null) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>接手事件 #{ack}</AlertDialogTitle><AlertDialogDescription>接手不改变模型检测结果，恢复由后续检测确认。</AlertDialogDescription></AlertDialogHeader><Textarea aria-label="处理备注" value={note} maxLength={2000} disabled={busy} onChange={e => setNote(e.target.value)} /><AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={e => { e.preventDefault(); void act(() => api.monitoringAction('ack', { id: ack, note })) }}>确认</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
    <AlertDialog open={approval !== null} onOpenChange={open => { if (!open && !busy) setApproval(null) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>批准新增模型？</AlertDialogTitle><AlertDialogDescription>新增模型将参加后续自动检测，可能产生额外 Token 消耗。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={e => { e.preventDefault(); const catalog = data?.catalogs.find(c => c.provider_id === approval); if (catalog) void act(() => api.monitoringAction('approve', { provider_id: catalog.provider_id, revision: catalog.revision, updated_at: catalog.updated_at })) }}>批准</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
    <AlertDialog open={discard} onOpenChange={setDiscard}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>放弃未保存的配置？</AlertDialogTitle><AlertDialogDescription>将重新读取服务器配置，本地更改不会保存。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>继续编辑</AlertDialogCancel><AlertDialogAction onClick={() => { setDraft(data?.settings || null); setDiscard(false); void act() }}>放弃并刷新</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>
}

function NumberField({ label, value, onChange, min = 0, max, step = '1' }: { label: string; value: number; onChange: (value: number) => void; min?: number; max: number; step?: string }) {
  return <Field label={label}><Input aria-label={label} type="number" min={min} max={max} step={step} value={value} onChange={e => { const n = Number(e.target.value); if (Number.isFinite(n)) onChange(n) }} /></Field>
}
function TextField({ label, value, onChange, secret = false, placeholder = '' }: { label: string; value: string; onChange: (value: string) => void; secret?: boolean; placeholder?: string }) {
  return <Field label={label}><Input aria-label={label} value={value} type={secret ? 'password' : 'text'} autoComplete="off" placeholder={placeholder} onChange={e => onChange(e.target.value)} /></Field>
}
function DateField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return <Field label={label}><Input aria-label={label} type="datetime-local" value={value ? value.slice(0, 16) : ''} onChange={e => onChange(e.target.value ? `${e.target.value}:00Z` : '')} /></Field>
}
function DeleteButton({ label, onClick }: { label: string; onClick: () => void }) { return <Button variant="ghost" size="icon" title={label} aria-label={label} onClick={onClick}><Trash2 /></Button> }
function Empty() { return <p className="py-8 text-center text-sm text-muted-foreground">暂无记录</p> }
function Metric({ label, value }: { label: string; value: string }) { return <div className="min-w-0"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-2 break-all font-mono text-lg font-semibold">{value}</p></div> }
function Cell({ children }: { children: ReactNode }) { return <td className="max-w-72 break-words px-3 py-3 align-top">{children}</td> }
function DataTable({ headers, children }: { headers: string[]; children: ReactNode }) { return <div className="max-w-full overflow-x-auto border-y"><table className="w-full min-w-[640px] text-left text-xs"><thead className="bg-muted/40"><tr>{headers.map(header => <th key={header} className="whitespace-nowrap px-3 py-3 font-medium">{header}</th>)}</tr></thead><tbody className="divide-y">{children}</tbody></table></div> }
