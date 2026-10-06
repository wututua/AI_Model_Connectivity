import { useCallback, useEffect, useRef, useState } from 'react'
import { Copy, Download, KeyRound, Plus, RefreshCw, RotateCw, Trash2 } from 'lucide-react'
import { api, downloadAdminFile } from '../../api'
import type { IssuedMetricsToken, MetricsToken } from '../../types'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '../../components/ui/tabs'
import { Tooltip, TooltipContent, TooltipTrigger } from '../../components/ui/tooltip'
import { Feedback, Field, LoadingButton } from './shared'

export function OperationsTab() {
  return <Tabs defaultValue="export"><TabsList><TabsTrigger value="export">数据导出</TabsTrigger><TabsTrigger value="metrics">指标凭据</TabsTrigger></TabsList><TabsContent value="export" className="mt-5"><Exports /></TabsContent><TabsContent value="metrics" className="mt-5"><MetricsTokens /></TabsContent></Tabs>
}

function Exports() {
  const [kind, setKind] = useState('history')
  const [start, setStart] = useState(() => new Date(Date.now() - 6 * 86400000).toISOString().slice(0, 10))
  const [end, setEnd] = useState(() => new Date().toISOString().slice(0, 10))
  const [provider, setProvider] = useState('')
  const [model, setModel] = useState('')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const lock = useRef(false)
  const download = async (diagnostics = false) => {
    if (lock.current) return
    lock.current = true; setBusy(true); setMessage('')
    try {
      if (!diagnostics && (!start || !end || start > end)) throw new Error('请填写有效的日期范围')
      const query = new URLSearchParams({ kind, start, end, provider_id: provider.trim(), model: model.trim() })
      await downloadAdminFile(diagnostics ? '/api/admin/diagnostics' : `/api/admin/export?${query}`, diagnostics ? 'diagnostics.json' : `${kind}-${start}-${end}.csv`)
      setMessage('文件已生成')
    } catch (e) { setMessage(`错误：${(e as Error).message}`) }
    finally { lock.current = false; setBusy(false) }
  }
  return <div className="space-y-5">
    <Feedback message={message} />
    <fieldset disabled={busy} className="field-grid">
      <Field label="数据类型"><Select value={kind} onValueChange={setKind}><SelectTrigger aria-label="导出数据类型"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="history">检测历史</SelectItem><SelectItem value="usage">每日 Token 用量</SelectItem></SelectContent></Select></Field>
      <div />
      <Field label="开始日期（UTC）" htmlFor="export-start"><Input id="export-start" type="date" value={start} onChange={e => setStart(e.target.value)} /></Field>
      <Field label="结束日期（UTC）" htmlFor="export-end"><Input id="export-end" type="date" value={end} onChange={e => setEnd(e.target.value)} /></Field>
      <Field label="Provider ID" htmlFor="export-provider"><Input id="export-provider" value={provider} onChange={e => setProvider(e.target.value)} placeholder="全部 Provider" /></Field>
      <Field label="模型" htmlFor="export-model"><Input id="export-model" value={model} onChange={e => setModel(e.target.value)} placeholder="全部模型" /></Field>
    </fieldset>
    <div className="flex flex-wrap gap-2 border-t pt-4"><LoadingButton loading={busy} onClick={() => void download()}><Download />导出 CSV</LoadingButton><Button variant="outline" disabled={busy} onClick={() => void download(true)}><Download />脱敏诊断</Button></div>
  </div>
}

function MetricsTokens() {
  const [tokens, setTokens] = useState<MetricsToken[]>([])
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  const [issued, setIssued] = useState<IssuedMetricsToken | null>(null)
  const [confirm, setConfirm] = useState<{ token: MetricsToken; rotate: boolean } | null>(null)
  const mounted = useRef(false)
  const locked = useRef(false)
  const request = useRef(0)
  const load = useCallback(async () => {
    const id = ++request.current
    setLoading(true)
    try {
      const data = await api.metricsTokens()
      if (mounted.current && request.current === id) setTokens(data)
    } catch (e) { if (mounted.current && request.current === id) setMessage(`加载失败：${(e as Error).message}`) }
    finally { if (mounted.current && request.current === id) setLoading(false) }
  }, [])
  useEffect(() => { mounted.current = true; void load(); return () => { mounted.current = false; request.current++ } }, [load])
  const act = async (target?: { token: MetricsToken; rotate: boolean }) => {
    if (locked.current) return
    locked.current = true; setBusy(true); setMessage('')
    try {
      const result = !target ? await api.createMetricsToken(name) : target.rotate ? await api.rotateMetricsToken(target.token.id) : (await api.revokeMetricsToken(target.token.id), null)
      if (!mounted.current) return
      setIssued(result); setName(''); setConfirm(null); setMessage(result ? '凭据已生成，请妥善保管' : '凭据已撤销')
      await load()
    } catch (e) { if (mounted.current) { setMessage(`错误：${(e as Error).message}`); setConfirm(null) } }
    finally { locked.current = false; if (mounted.current) setBusy(false) }
  }
  const copy = async () => {
    try { await navigator.clipboard.writeText(issued?.token ?? ''); setMessage('凭据已复制') }
    catch { setMessage('错误：剪贴板不可用，请选中凭据手动复制') }
  }
  return <div className="space-y-5">
    <div className="flex flex-wrap items-end gap-2"><div className="min-w-0 flex-1"><Field label="凭据名称" htmlFor="metrics-name"><Input id="metrics-name" maxLength={64} value={name} disabled={busy} onChange={e => setName(e.target.value)} placeholder="prometheus-production" /></Field></div><LoadingButton loading={busy} disabled={!name.trim()} onClick={() => void act()}><Plus />创建</LoadingButton><Button variant="outline" size="icon" aria-label="刷新指标凭据" title="刷新指标凭据" disabled={loading || busy} onClick={() => void load()}><RefreshCw /></Button></div>
    <Feedback message={message} />
    {issued && <section className="space-y-3 border-y py-4" aria-label="新指标凭据"><h3 className="flex items-center gap-2 text-sm font-semibold"><KeyRound className="size-4" />{issued.name}</h3><p className="text-xs text-warning">完整凭据仅本次显示，仅授权读取 /metrics。</p><div className="flex gap-2"><Input value={issued.token} readOnly aria-label="完整指标凭据" className="min-w-0 font-mono" /><Button variant="outline" size="icon" aria-label="复制指标凭据" title="复制指标凭据" onClick={() => void copy()}><Copy /></Button></div><Button size="sm" variant="outline" onClick={() => setIssued(null)}>关闭凭据</Button></section>}
    <div className="divide-y border-y" aria-busy={loading}>{tokens.map(token => <div key={token.id} className="flex min-w-0 items-center gap-3 py-4"><div className="min-w-0 flex-1"><p className="break-all text-sm font-medium">{token.name}</p><p className="mt-1 text-xs text-muted-foreground">#{token.id} · {new Date(token.rotated_at).toLocaleString('zh-CN')}</p></div><Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" aria-label={`轮换 ${token.name}`} disabled={busy} onClick={() => setConfirm({ token, rotate: true })}><RotateCw /></Button></TooltipTrigger><TooltipContent>轮换凭据</TooltipContent></Tooltip><Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" aria-label={`撤销 ${token.name}`} disabled={busy} onClick={() => setConfirm({ token, rotate: false })}><Trash2 /></Button></TooltipTrigger><TooltipContent>撤销凭据</TooltipContent></Tooltip></div>)}{!tokens.length && <p className="py-8 text-center text-sm text-muted-foreground">{loading ? '正在加载' : '暂无指标凭据'}</p>}</div>
    <AlertDialog open={!!confirm} onOpenChange={open => { if (!open && !busy) setConfirm(null) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{confirm?.rotate ? '轮换' : '撤销'}指标凭据？</AlertDialogTitle><AlertDialogDescription>「{confirm?.token.name}」的原凭据将立即失效。{confirm?.rotate ? '请更新采集端配置。' : '采集端将无法继续使用此凭据。'}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={e => { e.preventDefault(); if (confirm) void act(confirm) }}>{busy ? '处理中' : '确认'}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>
}
