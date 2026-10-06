import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { RequestBudget, RunningState } from '../../types'
import { Badge } from '../../components/ui/badge'
import { Progress } from '../../components/ui/progress'

export function DetectionProgress({ state }: { state: RunningState }) {
  const progress = state.progress
  if (!state.running || !progress) return null
  const label = ({ discovering: '模型发现', probing: '模型检测', saving: '保存结果', notifying: '发送通知' } as Record<string, string>)[progress.phase] ?? '准备中'
  return <section className="space-y-3 border-t px-6 py-4" aria-label="检测进度">
    <div className="flex flex-wrap justify-between gap-2 text-sm"><span>{label}{progress.provider_id ? ` · ${progress.provider_id}` : ''}</span><span className="font-mono text-xs">{progress.completed} / {progress.total} · {Math.floor((state.elapsed_ms ?? 0) / 1000)} s</span></div>
    <Progress value={progress.total > 0 ? progress.completed / progress.total * 100 : 0} aria-label="已完成模型比例" />
    {!!progress.active.length && <ul className="grid gap-1 text-xs text-muted-foreground sm:grid-cols-2">{progress.active.slice(0, 10).map(item => <li className="break-all" key={JSON.stringify([item.provider_id, item.model])}>{item.provider_id} / {item.model}</li>)}{progress.active.length > 10 && <li>另有 {progress.active.length - 10} 个模型检测中</li>}</ul>}
  </section>
}

export function BudgetStatus() {
  const [budget, setBudget] = useState<RequestBudget | null>(null)
  const [error, setError] = useState('')
  useEffect(() => {
    let active = true, pending = false
    const load = async () => {
      if (pending) return
      pending = true
      try { const value = await api.budget(); if (active) { setBudget(value); setError('') } }
      catch { if (active) setError('请求预算暂时无法读取') }
      finally { pending = false }
    }
    void load()
    const timer = setInterval(load, 5000)
    return () => { active = false; clearInterval(timer) }
  }, [])
  return <section className="space-y-3 border-y py-4" aria-label="今日请求预算">
    <div className="flex flex-wrap items-center justify-between gap-2"><h2 className="text-sm font-semibold">今日上游请求</h2>{budget && <Badge variant={budget.exhausted ? 'warning' : 'muted'}>{budget.exhausted ? '预算已用尽，调度暂停' : budget.limit > 0 ? `剩余 ${budget.remaining}` : '未设上限'}</Badge>}</div>
    {budget && <><p className="font-mono text-sm">{budget.used} / {budget.limit > 0 ? budget.limit : '不限'}</p>{budget.limit > 0 && <Progress value={Math.min(100, budget.used / budget.limit * 100)} aria-label="请求预算使用比例" />}<p className="text-xs text-muted-foreground">UTC {budget.day} · 重置时间 {new Date(budget.resets_at).toLocaleString('zh-CN')}</p></>}
    {error && <p role="status" className="text-xs text-destructive">{error}</p>}
  </section>
}
