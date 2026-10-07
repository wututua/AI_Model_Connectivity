import { useEffect, useState } from 'react'
import { ChevronLeft, ChevronRight, RefreshCw, RotateCcw, Search } from 'lucide-react'
import { api } from '../../api'
import type { AuditPage, AuditQuery } from '../../monitoring'
import { Button } from '../../components/ui/button'
import { Input } from '../../components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Field } from './shared'

const empty: AuditQuery = { actor: '', action: '', result: '', start: '', end: '', limit: 50, before: 0 }
const results: Record<string, string> = { success: '成功', accepted: '已受理', denied: '已拒绝', error: '失败' }
const actionNames: Record<string, string> = {
  'auth.login': '登录', 'auth.logout': '退出登录', 'auth.password': '修改密码',
  'users.create': '创建用户', 'users.update': '修改用户', 'users.delete': '删除用户',
  'settings.update': '修改运行设置', 'config.import': '导入配置', 'config.export': '导出配置',
  'providers.create': '创建 Provider', 'providers.update': '修改 Provider', 'providers.delete': '删除 Provider', 'providers.batch': '批量修改 Provider', 'providers.discover': '同步模型',
  'detection.start': '启动检测', 'detection.selected': '检测所选模型', 'detection.stop': '停止检测', 'detection.provider': '重新检测 Provider',
  'monitoring.settings': '修改监控设置', 'monitoring.backup': '创建备份', 'monitoring.verify': '校验备份', 'monitoring.approve': '批准模型清单', 'monitoring.ack': '接手事件', 'monitoring.test-rule': '测试告警规则',
  'notifications.test': '测试通知', 'notifications.retry': '重试通知',
  'metrics.create': '创建指标凭据', 'metrics.rotate': '轮换指标凭据', 'metrics.revoke': '撤销指标凭据',
  'updates.check': '检查更新', 'updates.start': '提交更新', 'updates.resolve': '确认更新提交', 'data.export': '导出数据',
}

export function AuditTab() {
  const [draft, setDraft] = useState(empty)
  const [query, setQuery] = useState(empty)
  const [cursors, setCursors] = useState([0])
  const [page, setPage] = useState<AuditPage | null>(null)
  const [actions, setActions] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [refresh, setRefresh] = useState(0)
  const before = cursors[cursors.length - 1]
  useEffect(() => {
    const controller = new AbortController()
    setLoading(true); setError(''); setPage(null)
    api.audit({ ...query, before }, controller.signal)
      .then(value => { if (!controller.signal.aborted) { setPage(value); setActions(value.actions) } })
      .catch(e => { if (!controller.signal.aborted) setError((e as Error).message) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [query, before, refresh])
  const set = <K extends keyof AuditQuery>(key: K, value: AuditQuery[K]) => setDraft(current => ({ ...current, [key]: value }))
  const apply = () => {
    try {
      const start = draft.start ? new Date(draft.start).toISOString() : ''
      const end = draft.end ? new Date(draft.end).toISOString() : ''
      if (start && end && start >= end) throw new Error('结束时间必须晚于开始时间')
      setQuery({ ...draft, actor: draft.actor.trim(), start, end }); setCursors([0])
    } catch (e) { setError((e as Error).message) }
  }
  return <section className="space-y-4" aria-label="操作审计记录">
    <p className="text-xs text-muted-foreground">最近 90 天 · 最多 10,000 条</p>
    <form className="space-y-3" onSubmit={e => { e.preventDefault(); apply() }}>
      <div className="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <Field label="操作者"><Input aria-label="操作者" maxLength={128} value={draft.actor} onChange={e => set('actor', e.target.value)} /></Field>
        <Field label="操作"><Select value={draft.action || '__all'} onValueChange={value => set('action', value === '__all' ? '' : value)}><SelectTrigger aria-label="操作"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="__all">全部操作</SelectItem>{actions.map(action => <SelectItem key={action} value={action}>{actionNames[action] || action}</SelectItem>)}</SelectContent></Select></Field>
        <Field label="结果"><Select value={draft.result || '__all'} onValueChange={value => set('result', value === '__all' ? '' : value)}><SelectTrigger aria-label="结果"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="__all">全部结果</SelectItem>{Object.entries(results).map(([id, label]) => <SelectItem key={id} value={id}>{label}</SelectItem>)}</SelectContent></Select></Field>
        <Field label="开始时间"><Input aria-label="开始时间" type="datetime-local" value={draft.start} onChange={e => set('start', e.target.value)} /></Field>
        <Field label="结束时间（不含）"><Input aria-label="结束时间" type="datetime-local" value={draft.end} onChange={e => set('end', e.target.value)} /></Field>
        <Field label="每页"><Select value={String(draft.limit)} onValueChange={value => set('limit', Number(value))}><SelectTrigger aria-label="每页记录数"><SelectValue /></SelectTrigger><SelectContent>{[25, 50, 100].map(size => <SelectItem key={size} value={String(size)}>{size}</SelectItem>)}</SelectContent></Select></Field>
      </div>
      <div className="flex gap-2">
        <Button type="submit" size="sm"><Search />筛选</Button>
        <Button type="button" variant="outline" size="icon" title="重置筛选" aria-label="重置筛选" onClick={() => { setDraft(empty); setQuery({ ...empty }); setCursors([0]) }}><RotateCcw /></Button>
        <Button type="button" variant="outline" size="icon" title="刷新审计记录" aria-label="刷新审计记录" disabled={loading} onClick={() => { setCursors([0]); setRefresh(n => n + 1) }}><RefreshCw /></Button>
      </div>
    </form>
    {error && <p role="alert" className="break-words text-sm text-destructive">{error}</p>}
    <div aria-busy={loading} className="min-h-24">
      {loading ? <p className="py-8 text-center text-sm text-muted-foreground">正在加载审计记录</p> : page && <>
        <div className="max-w-full overflow-x-auto border-y"><table className="w-full min-w-[600px] text-left text-xs">
          <thead className="bg-muted/40"><tr>{['时间', '操作者', '操作', '结果', 'HTTP'].map(header => <th key={header} className="px-3 py-3 font-medium">{header}</th>)}</tr></thead>
          <tbody className="divide-y">{page.items.map(event => <tr key={event.id}>
            <td className="px-3 py-3 align-top">{new Date(event.created_at).toLocaleString()}<p className="text-muted-foreground">#{event.id}</p></td>
            <td className="max-w-48 break-all px-3 py-3 align-top">{event.actor || '未认证'}{event.actor_id > 0 && <p className="text-muted-foreground">#{event.actor_id} · {event.role === 'admin' ? '管理员' : '普通用户'}</p>}</td>
            <td className="px-3 py-3 align-top">{actionNames[event.action] || event.action}<p className="font-mono text-muted-foreground">{event.action}</p></td>
            <td className="px-3 py-3 align-top">{results[event.result] || event.result}</td>
            <td className="px-3 py-3 align-top font-mono">{event.http_status}</td>
          </tr>)}</tbody>
        </table></div>
        {!page.items.length && <p className="py-8 text-center text-sm text-muted-foreground">暂无审计记录</p>}
      </>}
    </div>
    <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span>第 {cursors.length} 页{page ? ` · ${page.items.length} 条` : ''}</span><div className="flex gap-1">
      <Button variant="outline" size="icon" title="上一页" aria-label="上一页" disabled={loading || cursors.length <= 1} onClick={() => setCursors(current => current.slice(0, -1))}><ChevronLeft /></Button>
      <Button variant="outline" size="icon" title="下一页" aria-label="下一页" disabled={loading || !page?.has_more} onClick={() => { if (page?.has_more) setCursors(current => [...current, page.next_before]) }}><ChevronRight /></Button>
    </div></div>
  </section>
}
