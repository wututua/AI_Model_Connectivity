import { useState, useEffect, useCallback } from 'react'
import { RefreshCw } from 'lucide-react'
import { api } from '../../api'
import type { CheckTask } from '../../types'
import { Btn, Badge } from './shared'

export function TasksTab() {
  const [tasks, setTasks] = useState<CheckTask[]>([])
  const [loading, setLoading] = useState(false)
  const [offset, setOffset] = useState(0)
  const [filter, setFilter] = useState('')
  const LIMIT = 20

  const load = useCallback(() => {
    setLoading(true)
    api.tasks({ limit: LIMIT, offset, status: filter || undefined })
      .then(setTasks).catch(() => {}).finally(() => setLoading(false))
  }, [offset, filter])

  useEffect(() => { load() }, [load])

  const kindLabel: Record<string, string> = {
    manual: '手动', scheduled: '定时', startup: '启动', provider: 'Provider',
  }

  return (
    <div className="space-y-4 anim-fade-in">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>检测历史</h2>
        <div className="flex items-center gap-2">
          <select
            value={filter}
            onChange={e => { setFilter(e.target.value); setOffset(0) }}
            className="input-glass rounded-xl px-2 py-1.5 text-xs cursor-pointer focus:outline-none"
          >
            <option value="">全部状态</option>
            <option value="success">成功</option>
            <option value="error">错误</option>
            <option value="running">运行中</option>
            <option value="canceled">已取消</option>
          </select>
          <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" /></Btn>
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-left" style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)' }}>
              <th className="pb-2 pr-3 font-medium">ID</th>
              <th className="pb-2 pr-3 font-medium">类型</th>
              <th className="pb-2 pr-3 font-medium">状态</th>
              <th className="pb-2 pr-3 font-medium">开始时间</th>
              <th className="pb-2 pr-3 font-medium">耗时</th>
              <th className="pb-2 font-medium">结果</th>
            </tr>
          </thead>
          <tbody>
            {tasks.map(t => (
              <tr
                key={t.id}
                className="transition-colors"
                style={{ borderBottom: '1px solid var(--border)' }}
                onMouseEnter={e => (e.currentTarget.style.background = 'var(--row-hover)')}
                onMouseLeave={e => (e.currentTarget.style.background = '')}
              >
                <td className="py-2.5 pr-3 font-mono text-xs" style={{ color: 'var(--muted)' }}>#{t.id}</td>
                <td className="py-2.5 pr-3 text-xs">
                  <span style={{ color: 'var(--text)' }}>{kindLabel[t.kind] ?? t.kind}</span>
                  {t.provider_id && <span className="font-mono ml-1" style={{ color: 'var(--muted)', opacity: .7 }}>{t.provider_id}</span>}
                </td>
                <td className="py-2.5 pr-3">
                  <Badge status={t.status} />
                  {t.error_message && (
                    <p className="text-[11px] font-mono mt-0.5 max-w-[200px] truncate" style={{ color: 'var(--error)', opacity: .7 }}>{t.error_message}</p>
                  )}
                </td>
                <td className="py-2.5 pr-3 font-mono text-xs" style={{ color: 'var(--muted)' }}>
                  {t.started_at ? new Date(t.started_at).toLocaleString('zh-CN') : '—'}
                </td>
                <td className="py-2.5 pr-3 font-mono text-xs" style={{ color: 'var(--muted)' }}>
                  {t.elapsed_ms ? `${(t.elapsed_ms / 1000).toFixed(1)}s` : '—'}
                </td>
                <td className="py-2.5 text-xs font-mono">
                  {t.total > 0 && (
                    <span style={{ color: 'var(--muted)' }}>
                      <span style={{ color: 'var(--ok)' }}>{t.ok_count}</span>/
                      <span style={{ color: 'var(--slow)' }}>{t.slow_count}</span>/
                      <span style={{ color: 'var(--error)' }}>{t.error_count}</span>
                      <span style={{ color: 'var(--muted)', opacity: .6 }}> ({t.total})</span>
                    </span>
                  )}
                </td>
              </tr>
            ))}
            {!loading && tasks.length === 0 && (
              <tr>
                <td colSpan={6} className="py-12 text-center" style={{ color: 'var(--muted)', opacity: .5 }}>暂无检测记录</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center gap-3">
        <Btn onClick={() => setOffset(o => Math.max(0, o - LIMIT))} disabled={offset === 0} variant="ghost">
          上一页
        </Btn>
        <span className="text-xs font-mono" style={{ color: 'var(--muted)' }}>
          {offset + 1}–{offset + tasks.length} 条
        </span>
        <Btn onClick={() => setOffset(o => o + LIMIT)} disabled={tasks.length < LIMIT} variant="ghost">
          下一页
        </Btn>
      </div>
    </div>
  )
}
