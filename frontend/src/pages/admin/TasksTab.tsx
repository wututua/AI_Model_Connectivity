import { useCallback, useEffect, useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Clock3, RefreshCw } from 'lucide-react'
import { api } from '../../api'
import type { CheckTask } from '../../types'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { Card, CardContent } from '../../components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table'
import { Feedback, StatusBadge } from './shared'

const LIMIT = 20
const KIND_LABELS: Record<string, string> = { manual: '手动检测', scheduled: '定时检测', startup: '启动检测', provider: 'Provider 检测' }

export function TasksTab() {
  const [tasks, setTasks] = useState<CheckTask[]>([])
  const [loading, setLoading] = useState(false)
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState('all')
  const [error, setError] = useState('')

  const load = useCallback(() => {
    setLoading(true); setError('')
    api.tasks({ limit: LIMIT, offset, status: filter === 'all' ? undefined : filter })
      .then(setTasks).catch(cause => setError(`错误：${(cause as Error).message}`)).finally(() => setLoading(false))
  }, [filter, offset])
  useEffect(() => { load() }, [load])

  const pageSummary = useMemo(() => ({ success: tasks.filter(task => task.status === 'success').length, failed: tasks.filter(task => task.status === 'error').length, running: tasks.filter(task => task.status === 'running').length }), [tasks])

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap gap-2"><Badge variant="success">成功 {pageSummary.success}</Badge><Badge variant="destructive">错误 {pageSummary.failed}</Badge><Badge variant="warning">运行中 {pageSummary.running}</Badge></div>
        <div className="flex items-center gap-2">
          <Select value={filter} onValueChange={value => { setFilter(value); setOffset(0) }}><SelectTrigger className="w-[140px]"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部状态</SelectItem><SelectItem value="success">成功</SelectItem><SelectItem value="error">错误</SelectItem><SelectItem value="running">运行中</SelectItem><SelectItem value="canceled">已取消</SelectItem></SelectContent></Select>
          <Button variant="outline" size="icon" onClick={load} disabled={loading} aria-label="刷新任务历史"><RefreshCw className={loading ? 'animate-spin' : ''} /></Button>
        </div>
      </div>
      <Feedback message={error} />
      <Card><CardContent className="p-0"><Table>
        <TableHeader><TableRow><TableHead className="hidden sm:table-cell">任务</TableHead><TableHead>类型</TableHead><TableHead>状态</TableHead><TableHead>开始时间</TableHead><TableHead className="hidden md:table-cell">耗时</TableHead><TableHead className="text-right">结果</TableHead></TableRow></TableHeader>
        <TableBody>
          {tasks.map(task => (
            <TableRow key={task.id}>
              <TableCell className="hidden font-mono text-xs text-muted-foreground sm:table-cell">#{task.id}</TableCell>
              <TableCell><p className="text-sm font-medium">{KIND_LABELS[task.kind] ?? task.kind}</p>{task.provider_id && <p className="mt-0.5 font-mono text-xs text-muted-foreground">{task.provider_id}</p>}</TableCell>
              <TableCell><StatusBadge status={task.status} />{task.error_message && <p className="mt-1 max-w-64 truncate font-mono text-xs text-destructive" title={task.error_message}>{task.error_message}</p>}</TableCell>
              <TableCell className="font-mono text-xs text-muted-foreground"><span className="hidden sm:inline">{formatDate(task.started_at)}</span><span className="sm:hidden">{formatTime(task.started_at)}</span></TableCell>
              <TableCell className="hidden font-mono text-xs text-muted-foreground md:table-cell">{task.elapsed_ms ? `${(task.elapsed_ms / 1000).toFixed(1)}s` : '—'}</TableCell>
              <TableCell className="text-right"><div className="font-mono text-xs tabular-nums"><span className="text-success">{task.ok_count}</span><span className="text-muted-foreground"> / </span><span className="text-warning">{task.slow_count}</span><span className="text-muted-foreground"> / </span><span className="text-destructive">{task.error_count}</span></div><p className="mt-0.5 text-[11px] text-muted-foreground">共 {task.total}</p></TableCell>
            </TableRow>
          ))}
          {!loading && tasks.length === 0 && <TableRow><TableCell colSpan={6}><div className="flex min-h-52 flex-col items-center justify-center text-muted-foreground"><Clock3 className="mb-3 size-7" /><p className="text-sm">暂无检测任务</p></div></TableCell></TableRow>}
        </TableBody>
      </Table></CardContent></Card>
      <div className="flex items-center justify-between"><p className="text-xs text-muted-foreground">显示 {tasks.length ? offset + 1 : 0}–{offset + tasks.length} 条</p><div className="flex gap-2"><Button variant="outline" size="sm" onClick={() => setOffset(value => Math.max(0, value - LIMIT))} disabled={offset === 0}><ChevronLeft />上一页</Button><Button variant="outline" size="sm" onClick={() => setOffset(value => value + LIMIT)} disabled={tasks.length < LIMIT}>下一页<ChevronRight /></Button></div></div>
    </div>
  )
}

function formatDate(value: string) { return value ? new Date(value).toLocaleString('zh-CN') : '—' }
function formatTime(value: string) { return value ? new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : '—' }
