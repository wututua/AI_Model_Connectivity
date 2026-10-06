import { useEffect, useRef, useState } from 'react'
import { Play, RotateCw } from 'lucide-react'
import { api } from '../../api'
import type { SafeProviderConfig } from '../../types'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog'
import { Button } from '../../components/ui/button'
import { Badge } from '../../components/ui/badge'
import { Feedback, LoadingButton } from './shared'

export function ModelCheckDialog({ provider, onClose, onAccepted }: { provider: SafeProviderConfig; onClose: () => void; onAccepted: (id: number) => void }) {
  const [models, setModels] = useState<{ name: string; status: string; checked: string }[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const lock = useRef(false)
  useEffect(() => {
    let active = true
    setModels([]); setSelected([]); setLoading(true); setError('')
    api.status().then(report => {
      if (!active) return
      const rows = report.providers.find(group => group.provider_id === provider.id)?.results ?? []
      setModels(rows.map(row => ({ name: row.model, status: row.status, checked: row.checked_at ?? '' })))
    }).catch(e => { if (active) setError(`加载失败：${e.message}`) }).finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [provider])
  const run = async (failed: boolean) => {
    if (lock.current) return
    lock.current = true; setBusy(true); setError('')
    try {
      const { task } = await api.checkModels({ provider_id: provider.id, ...(failed ? { failed_only: true } : { targets: selected.map(model => ({ provider_id: provider.id, model })) }) })
      onAccepted(task.id); onClose()
    } catch (e) { setError(`错误：${(e as Error).message}`) }
    finally { lock.current = false; setBusy(false) }
  }
  return <Dialog open onOpenChange={open => { if (!open && !busy) onClose() }}><DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
    <DialogHeader><DialogTitle className="break-all pr-6">{provider.name} · 模型检测</DialogTitle><DialogDescription>将调用所选模型并消耗 Token。未重测模型保留原检测时间。</DialogDescription></DialogHeader>
    <Feedback message={error} />
    {loading ? <p role="status" className="text-sm text-muted-foreground">正在加载模型</p> : <div className="divide-y border-y">
      {!!models.length && <label className="flex items-center gap-3 py-3 text-sm"><input type="checkbox" disabled={busy} checked={selected.length === models.length} onChange={e => setSelected(e.target.checked ? models.map(m => m.name) : [])} />全选（{models.length}）</label>}
      {models.map(model => <label key={model.name} className="flex min-w-0 items-center gap-3 py-3 text-sm">
        <input type="checkbox" disabled={busy} checked={selected.includes(model.name)} onChange={e => setSelected(current => e.target.checked ? [...current, model.name] : current.filter(name => name !== model.name))} />
        <span className="min-w-0 flex-1 break-all">{model.name}{model.checked && <span className="mt-1 block text-xs text-muted-foreground">{new Date(model.checked).toLocaleString('zh-CN')}</span>}</span>
        <Badge variant={model.status === 'error' ? 'destructive' : model.status === 'ok' ? 'success' : 'muted'}>{({ error: '失败', ok: '正常', slow: '较慢', unknown: '未检测' } as Record<string, string>)[model.status] ?? model.status}</Badge>
      </label>)}
      {!models.length && <p className="py-5 text-sm text-muted-foreground">暂无可检测模型</p>}
    </div>}
    <DialogFooter className="flex-wrap gap-2"><Button variant="outline" disabled={busy || loading || !models.some(m => m.status === 'error')} onClick={() => void run(true)}><RotateCw />重测失败项</Button><LoadingButton loading={busy} disabled={loading || selected.length === 0 || selected.length > 1000} onClick={() => void run(false)}><Play />检测所选（{selected.length}）</LoadingButton></DialogFooter>
  </DialogContent></Dialog>
}
