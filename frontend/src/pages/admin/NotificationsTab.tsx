import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Bell, ChevronLeft, ChevronRight, RefreshCw, RotateCw, Send, Settings2 } from 'lucide-react'
import { api } from '../../api'
import type { NotificationDelivery } from '../../types'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '../../components/ui/tooltip'
import { Feedback, ListSkeleton, LoadingButton } from './shared'

const LIMIT = 20
const STATUS = { sending: '发送中', success: '平台已接受', error: '发送失败', unknown: '结果未知' } as const
const KIND = { alert: '状态告警', test: '测试通知', retry: '手动重试', operations: '运维提醒' } as const
const PLATFORM: Record<string, string> = { webhook: 'Webhook', discord: 'Discord', telegram: 'Telegram', bark: 'Bark', wecom: '企业微信', wechat_work: '企业微信', dingtalk: '钉钉' }

export function NotificationsTab() {
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState('all')
  const [data, setData] = useState<{ offset: number; filter: string; items: NotificationDelivery[] } | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [retry, setRetry] = useState<NotificationDelivery | null>(null)
  const mounted = useRef(false)
  const pending = useRef<AbortController | null>(null)
  const requestID = useRef(0)
  const sending = useRef(false)
  const items = data?.offset === offset && data.filter === filter ? data.items : []

  const load = useCallback(() => {
    pending.current?.abort()
    const controller = new AbortController()
    pending.current = controller
    const id = ++requestID.current
    setLoading(true); setLoadError('')
    api.notifications({ limit: LIMIT, offset, status: filter === 'all' ? '' : filter }, controller.signal)
      .then(items => { if (id === requestID.current) setData({ offset, filter, items }) })
      .catch(error => { if (id === requestID.current && !controller.signal.aborted) setLoadError(`加载失败：${(error as Error).message}`) })
      .finally(() => { if (id === requestID.current) setLoading(false) })
  }, [offset, filter])

  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])
  useEffect(() => {
    load()
    return () => { requestID.current++; pending.current?.abort() }
  }, [load])
  const hasSending = items.some(item => item.status === 'sending')
  useEffect(() => {
    if (!hasSending || loading) return
    const timer = setTimeout(load, 2000)
    return () => clearTimeout(timer)
  }, [hasSending, loading, load])

  const send = async (previous?: NotificationDelivery) => {
    if (sending.current) return
    sending.current = true
    setBusy(true); setMessage('')
    try {
      const result = previous ? await api.retryNotification(previous.id) : await api.testNotification()
      if (!mounted.current) return
      setMessage(result.status === 'success' ? `通知 #${result.id} 已被平台接受` : `错误：通知 #${result.id} 发送失败，${result.error_message}`)
      setRetry(null)
    } catch (error) {
      if (mounted.current) setMessage(`错误：${(error as Error).message}`)
    } finally {
      sending.current = false
      if (mounted.current) {
        setBusy(false)
        setRetry(null)
        setOffset(0); setFilter('all')
        if (offset === 0 && filter === 'all') load()
      }
    }
  }

  const retryButton = (item: NotificationDelivery) => (item.status === 'error' || item.status === 'unknown') && (
    <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" disabled={busy} onClick={() => setRetry(item)} aria-label={`重试通知 ${item.id}`}><RotateCw /></Button></TooltipTrigger><TooltipContent>重发历史摘要</TooltipContent></Tooltip>
  )

  return <div className="space-y-5">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <Select value={filter} onValueChange={value => { setFilter(value); setOffset(0) }} disabled={busy}>
        <SelectTrigger className="w-[150px]" aria-label="通知状态"><SelectValue /></SelectTrigger>
        <SelectContent><SelectItem value="all">全部状态</SelectItem>{Object.entries(STATUS).map(([key, label]) => <SelectItem key={key} value={key}>{label}</SelectItem>)}</SelectContent>
      </Select>
      <div className="flex flex-wrap gap-2">
        <Tooltip><TooltipTrigger asChild><Button asChild variant="outline" size="icon"><Link to="/admin/settings?tab=notify" aria-label="通知配置"><Settings2 /></Link></Button></TooltipTrigger><TooltipContent>通知配置</TooltipContent></Tooltip>
        <Tooltip><TooltipTrigger asChild><Button variant="outline" size="icon" onClick={load} disabled={loading} aria-label="刷新通知记录"><RefreshCw className={loading ? 'animate-spin' : ''} /></Button></TooltipTrigger><TooltipContent>刷新</TooltipContent></Tooltip>
        <LoadingButton loading={busy} onClick={() => void send()}><Send />发送测试通知</LoadingButton>
      </div>
    </div>
    <Feedback message={message} />
    <Feedback message={loadError} />
    {loading && !items.length && <ListSkeleton label="正在加载通知记录" />}
    {!loading && !loadError && !items.length && <div className="flex min-h-52 flex-col items-center justify-center border-y text-muted-foreground"><Bell className="mb-3 size-7" /><p className="text-sm">暂无通知记录</p></div>}
    {!!items.length && <>
      <div className="hidden border-y md:block" aria-busy={loading}>
        <Table>
          <TableHeader><TableRow><TableHead>时间与类型</TableHead><TableHead>渠道</TableHead><TableHead>发送结果</TableHead><TableHead>摘要</TableHead><TableHead className="w-12"><span className="sr-only">操作</span></TableHead></TableRow></TableHeader>
          <TableBody>{items.map(item => <TableRow key={item.id}>
            <TableCell className="align-top"><p className="whitespace-nowrap text-xs">{formatDate(item.created_at)}</p><p className="mt-1 text-xs text-muted-foreground">#{item.id} · {KIND[item.kind]}{item.retry_of ? ` · 原记录 #${item.retry_of}` : ''}</p></TableCell>
            <TableCell className="align-top text-xs">{PLATFORM[item.platform] ?? item.platform}{item.rule_id && <p className="mt-1 break-all text-muted-foreground">规则 {item.rule_id}</p>}</TableCell>
            <TableCell className="align-top"><DeliveryStatus value={item} /></TableCell>
            <TableCell className="max-w-sm align-top"><p className="break-words text-xs">{item.summary}</p>{item.error_message && <p className="mt-1 break-words text-xs text-destructive">{item.error_message}</p>}</TableCell>
            <TableCell className="align-top">{retryButton(item)}</TableCell>
          </TableRow>)}</TableBody>
        </Table>
      </div>
      <div className="divide-y border-y md:hidden" aria-busy={loading}>{items.map(item => <section key={item.id} className="min-w-0 space-y-2 py-4" aria-label={`通知 ${item.id}`}>
        <div className="flex items-start justify-between gap-3"><div className="min-w-0"><p className="text-sm font-medium">{KIND[item.kind]} · {PLATFORM[item.platform] ?? item.platform}</p><p className="mt-1 text-xs text-muted-foreground">#{item.id} · {formatDate(item.created_at)}</p></div>{retryButton(item)}</div>
        <DeliveryStatus value={item} />
        {item.retry_of > 0 && <p className="text-xs text-muted-foreground">原记录 #{item.retry_of}</p>}
        {item.rule_id && <p className="break-all text-xs text-muted-foreground">规则 {item.rule_id}</p>}
        <p className="break-words text-xs">{item.summary}</p>
        {item.error_message && <p className="break-words text-xs text-destructive">{item.error_message}</p>}
      </section>)}</div>
    </>}
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-xs text-muted-foreground" role="status">{loading ? '正在加载' : loadError ? '加载失败' : items.length ? `显示 ${offset + 1}–${offset + items.length} 条` : '当前页无记录'}</p>
      <div className="flex gap-2"><Button variant="outline" size="sm" disabled={loading || busy || offset === 0} onClick={() => setOffset(value => Math.max(0, value - LIMIT))}><ChevronLeft />上一页</Button><Button variant="outline" size="sm" disabled={loading || busy || !!loadError || items.length < LIMIT} onClick={() => setOffset(value => value + LIMIT)}>下一页<ChevronRight /></Button></div>
    </div>
    <AlertDialog open={!!retry} onOpenChange={open => { if (!open && !busy) setRetry(null) }}>
      <AlertDialogContent>
        <AlertDialogHeader><AlertDialogTitle>重试通知 #{retry?.id}？</AlertDialogTitle><AlertDialogDescription>将向当前已保存的渠道重发历史摘要，不更新正式告警状态。接收结果未确认的请求可能已送达，请先核对接收端。</AlertDialogDescription></AlertDialogHeader>
        <p className="break-words text-sm">{retry?.summary}</p>
        <AlertDialogFooter><AlertDialogCancel disabled={busy}>取消</AlertDialogCancel><AlertDialogAction disabled={busy} onClick={event => { event.preventDefault(); if (retry) void send(retry) }}>{busy ? '正在重试' : '确认重试'}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
}

function DeliveryStatus({ value }: { value: NotificationDelivery }) {
  const variant = value.status === 'success' ? 'success' : value.status === 'error' ? 'destructive' : value.status === 'sending' ? 'warning' : 'muted'
  return <div className="space-y-1"><Badge variant={variant}>{STATUS[value.status]}</Badge>{value.finished_at && <p className="text-xs text-muted-foreground">{value.http_status > 0 ? `HTTP ${value.http_status} · ` : ''}{value.elapsed_ms} ms</p>}</div>
}

function formatDate(value: string) {
  return new Date(value).toLocaleString('zh-CN')
}
