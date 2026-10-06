import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Copy, Database, Edit2, KeyRound, ListChecks, MoreHorizontal, Plus, RefreshCw, RotateCw, Search, Trash2 } from 'lucide-react'
import { api } from '../../api'
import type { ProviderUpdate, SafeProviderConfig } from '../../types'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '../../components/ui/alert-dialog'
import { Badge } from '../../components/ui/badge'
import { Button } from '../../components/ui/button'
import { Card, CardContent } from '../../components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../../components/ui/dialog'
import { Input } from '../../components/ui/input'
import { ModelPicker } from '../../components/ModelPicker'
import { Switch } from '../../components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '../../components/ui/tooltip'
import { Feedback, Field, ListSkeleton, LoadingButton, useAutoMsg } from './shared'
import { mergeModels, parseModels } from '../../utils/models'
import { ProbeFields, defaultProbe } from './ProbeFields'
import { ModelCheckDialog } from './ModelCheckDialog'

type EditingState = (SafeProviderConfig & { isCopy?: boolean }) | 'new' | null

export function ProvidersTab({ readOnly = false }: { readOnly?: boolean }) {
  const [providers, setProviders] = useState<SafeProviderConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<EditingState>(null)
  const [deleteTarget, setDeleteTarget] = useState<SafeProviderConfig | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [rerunning, setRerunning] = useState<string | null>(null)
  const [busy, setBusy] = useState(true)
  const watchedTask = useRef<number | null>(null)
  const [search, setSearch] = useState('')
  const [message, setMessage] = useAutoMsg()
  const [actionTarget, setActionTarget] = useState<SafeProviderConfig | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)
  const [scope, setScope] = useState('all')
  const [selected, setSelected] = useState<string[]>([])
  const [batchAction, setBatchAction] = useState('pause')
  const [batchGroup, setBatchGroup] = useState('')
  const [batchConfirm, setBatchConfirm] = useState(false)
  const [batchBusy, setBatchBusy] = useState(false)
  const [modelTarget, setModelTarget] = useState<SafeProviderConfig | null>(null)
  const loadRequest = useRef(0)

  const load = useCallback(() => {
    const request = ++loadRequest.current
    setLoading(true); setLoadFailed(false)
    api.providers()
      .then(value => {
        if (request !== loadRequest.current) return
        const ids = new Set(value.map(provider => provider.id))
        setProviders(value)
        setSelected(current => current.filter(id => ids.has(id)))
      })
      .catch(cause => {
        if (request !== loadRequest.current) return
        setLoadFailed(true); setMessage(`错误：${(cause as Error).message}`)
      })
      .finally(() => { if (request === loadRequest.current) setLoading(false) })
  }, [setMessage])

  useEffect(() => {
    load()
    return () => { loadRequest.current++ }
  }, [load])
  useEffect(() => {
    if (readOnly) return
    let active = true
    let pending = false
    const poll = async () => {
      if (pending) return
      pending = true
      try {
        const state = await api.detection()
        if (!active) return
        setBusy(state.running)
        setRerunning(state.running ? state.provider_id || null : null)
        if (watchedTask.current) {
          const task = await api.task(watchedTask.current)
          if (!active || task.status === 'running') return
          watchedTask.current = null
          setMessage(task.status === 'success' ? `检测任务 #${task.id} 已完成` : `错误：检测任务 #${task.id} ${task.status === 'canceled' ? '已取消' : '失败'}`)
        }
      } catch {
        // Preserve the last known state while reconnecting.
      } finally { pending = false }
    }
    void poll()
    const timer = setInterval(poll, 2000)
    return () => { active = false; clearInterval(timer) }
  }, [readOnly, setMessage])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    return providers.filter(provider =>
      (scope === 'all' || scope === `g:${provider.group ?? ''}` || (provider.tags ?? []).some(tag => scope === `t:${tag}`)) &&
      (!query || [provider.id, provider.name, provider.type, provider.base_url, provider.group ?? '', ...(provider.tags ?? []), ...provider.models].some(value => value.toLowerCase().includes(query))))
  }, [providers, search, scope])
  const groups = [...new Set(providers.map(p => p.group ?? ''))].sort()
  const tags = [...new Set(providers.flatMap(p => p.tags ?? []))].sort()
  const copy = (provider: SafeProviderConfig) => setEditing({ ...provider, id: '', name: `${provider.name} 副本`, api_key_set: false, enabled: false, isCopy: true })
  const selection = (provider: SafeProviderConfig) => <input type="checkbox" className="size-4 shrink-0" aria-label={`选择 Provider ${provider.name}`} disabled={batchBusy} checked={selected.includes(provider.id)} onChange={e => setSelected(current => e.target.checked ? [...current, provider.id] : current.filter(id => id !== provider.id))} />
  const batch = async () => {
    if (batchBusy || !selected.length) return
    setBatchBusy(true)
    try {
      const value = await api.batchProviders(selected, batchAction, batchGroup)
      loadRequest.current++
      setLoading(false); setLoadFailed(false)
      setProviders(value.providers); setSelected([]); setBatchConfirm(false); setMessage('批量操作已保存')
    } catch (e) { setBatchConfirm(false); setMessage(`错误：${(e as Error).message}`) }
    finally { setBatchBusy(false) }
  }

  const save = async (id: string | null, update: ProviderUpdate) => {
    if (id) await api.updateProvider(id, update)
    else await api.createProvider(update)
    setEditing(null); setMessage('Provider 已保存'); load()
  }

  const remove = async () => {
    if (!deleteTarget) return
    const id = deleteTarget.id
    setDeleting(true)
    try {
      await api.deleteProvider(id)
      setProviders(current => current.filter(provider => provider.id !== id))
      setSelected(current => current.filter(selectedID => selectedID !== id))
      setDeleteTarget(null); setMessage('Provider 已删除'); load()
    }
    catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setDeleting(false) }
  }

  const rerun = async (provider: SafeProviderConfig) => {
    setRerunning(provider.id); setMessage('')
    setBusy(true)
    try {
      const { task } = await api.rerunProvider(provider.id)
      watchedTask.current = task.id
      setMessage(`「${provider.name}」检测任务 #${task.id} 已启动`)
    } catch (cause) { setRerunning(null); setBusy(false); setMessage(`错误：${(cause as Error).message}`) }
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative w-full sm:max-w-sm"><Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" /><Input value={search} onChange={event => setSearch(event.target.value)} aria-label="搜索 Provider" placeholder="搜索名称、ID、URL 或模型" className="pl-9" /></div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={load} disabled={loading}><RefreshCw className={loading ? 'animate-spin' : ''} />刷新</Button>
          {!readOnly && <Button size="sm" onClick={() => setEditing('new')}><Plus />新增 Provider</Button>}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Select value={scope} onValueChange={setScope}><SelectTrigger className="w-[190px]" aria-label="分组与标签"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">全部分组与标签</SelectItem>{groups.map(group => <SelectItem key={`g:${group}`} value={`g:${group}`}>分组 · {group || '未分组'}</SelectItem>)}{tags.map(tag => <SelectItem key={`t:${tag}`} value={`t:${tag}`}>标签 · {tag}</SelectItem>)}</SelectContent></Select>
        {!readOnly && <label className="flex items-center gap-2 text-sm"><input type="checkbox" aria-label="选择当前筛选 Provider" disabled={batchBusy || !filtered.length} checked={!!filtered.length && filtered.every(p => selected.includes(p.id))} onChange={e => setSelected(current => e.target.checked ? [...new Set([...current, ...filtered.map(p => p.id)])] : current.filter(id => !filtered.some(p => p.id === id)))} />已选 {selected.length}</label>}
      </div>
      {!readOnly && selected.length > 0 && <div className="flex flex-wrap items-center gap-2 border-y py-3">
        <Select value={batchAction} onValueChange={setBatchAction} disabled={batchBusy}><SelectTrigger className="w-[150px]" aria-label="批量操作"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="pause">暂停检测</SelectItem><SelectItem value="resume">恢复检测</SelectItem><SelectItem value="enable">启用 Provider</SelectItem><SelectItem value="disable">停用 Provider</SelectItem><SelectItem value="group">设置分组</SelectItem></SelectContent></Select>
        {batchAction === 'group' && <Input aria-label="批量分组名称" className="w-[180px]" maxLength={128} value={batchGroup} onChange={e => setBatchGroup(e.target.value)} placeholder="分组名称" disabled={batchBusy} />}
        <Button size="sm" disabled={batchBusy} onClick={() => setBatchConfirm(true)}><ListChecks />应用到 {selected.length} 项</Button>
      </div>}
      <Feedback message={message} />

      <div className="divide-y border-y sm:hidden" aria-busy={loading}>
        {loading && !providers.length && <ListSkeleton label="正在加载 Provider" />}
        {filtered.map(provider => <section key={provider.id} className="min-w-0 py-4" aria-label={provider.name}>
          <div className="flex items-start gap-2">
            {!readOnly && selection(provider)}
            <div className="min-w-0 flex-1"><h2 className="break-words text-sm font-semibold">{provider.name}</h2><p className="mt-1 break-all text-xs text-muted-foreground">{provider.id} · {provider.type}</p></div>
            {!readOnly && <Button variant="ghost" size="icon" className="size-10 shrink-0" aria-label={`${provider.name} 更多操作`} title="更多操作" onClick={() => setActionTarget(provider)}><MoreHorizontal /></Button>}
          </div>
          <p className="mt-2 break-all text-xs text-muted-foreground">{provider.base_url}</p>
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <Badge variant={provider.enabled ? 'success' : 'muted'}>{provider.enabled ? '已启用' : '已停用'}</Badge>
            <span>{provider.enabled && provider.probe_enabled ? '参与检测' : '不参与检测'}</span>
            <span>{provider.models.length ? `${provider.models.length} 个模型` : '自动获取模型'}</span>
            {provider.group && <Badge variant="outline">{provider.group}</Badge>}
            {(provider.tags ?? []).map(tag => <span key={tag}>#{tag}</span>)}
          </div>
        </section>)}
        {!loading && !loadFailed && !filtered.length && <p className="py-12 text-center text-sm text-muted-foreground">{providers.length ? '没有匹配的 Provider' : '尚未添加 Provider'}</p>}
      </div>

      <Card className="hidden sm:block" aria-busy={loading}>
        <CardContent className="p-0">
          <Table>
            <TableHeader><TableRow><TableHead>Provider</TableHead><TableHead className="hidden md:table-cell">连接地址</TableHead><TableHead className="hidden sm:table-cell">模型</TableHead><TableHead>状态</TableHead>{!readOnly && <TableHead className="w-[204px] text-right">操作</TableHead>}</TableRow></TableHeader>
            <TableBody>
              {loading && !providers.length && <TableRow><TableCell colSpan={readOnly ? 4 : 5}><ListSkeleton label="正在加载 Provider" /></TableCell></TableRow>}
              {filtered.map(provider => (
                <TableRow key={provider.id}>
                  <TableCell><div className="flex items-center gap-3">{!readOnly && selection(provider)}<div className="min-w-0"><p className="truncate font-medium">{provider.name}</p><p className="truncate font-mono text-xs text-muted-foreground">{provider.id} · {provider.type}</p><p className="mt-1 break-words text-xs text-muted-foreground">{[provider.group, ...(provider.tags ?? []).map(tag => `#${tag}`)].filter(Boolean).join(' · ')}</p></div></div></TableCell>
                  <TableCell className="hidden max-w-[320px] md:table-cell"><p className="truncate font-mono text-xs text-muted-foreground" title={provider.base_url}>{provider.base_url}</p><p className="mt-1 flex items-center gap-1 text-xs text-muted-foreground"><KeyRound className="size-3" />{provider.api_key_set ? 'API Key 已设置' : '未设置 API Key'}</p></TableCell>
                  <TableCell className="hidden sm:table-cell"><span className="font-mono text-xs">{provider.models.length || '自动'}</span></TableCell>
                  <TableCell className="whitespace-nowrap"><div className="flex flex-col items-start gap-1"><Badge variant={provider.enabled ? 'success' : 'muted'}>{provider.enabled ? '已启用' : '已停用'}</Badge><span className="text-xs text-muted-foreground">{provider.enabled && provider.probe_enabled ? '参与检测' : '不参与检测'}</span></div></TableCell>
                  {!readOnly && <TableCell>
                    <div className="flex justify-end gap-1">
                      <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={() => setModelTarget(provider)} disabled={busy || !provider.enabled || !provider.probe_enabled} aria-label={`模型检测 ${provider.name}`}><ListChecks /></Button></TooltipTrigger><TooltipContent>模型检测</TooltipContent></Tooltip>
                      <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={() => copy(provider)} aria-label={`复制 ${provider.name}`}><Copy /></Button></TooltipTrigger><TooltipContent>复制配置（不含密钥）</TooltipContent></Tooltip>
                      <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={() => rerun(provider)} disabled={!provider.enabled || !provider.probe_enabled || busy || rerunning !== null} aria-label={`重新检测 ${provider.name}`}><RotateCw className={rerunning === provider.id ? 'animate-spin' : ''} /></Button></TooltipTrigger><TooltipContent>重新检测</TooltipContent></Tooltip>
                      <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={() => setEditing(provider)} aria-label={`编辑 ${provider.name}`}><Edit2 /></Button></TooltipTrigger><TooltipContent>编辑</TooltipContent></Tooltip><Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" className="text-destructive hover:text-destructive" onClick={() => setDeleteTarget(provider)} aria-label={`删除 ${provider.name}`}><Trash2 /></Button></TooltipTrigger><TooltipContent>删除</TooltipContent></Tooltip>
                    </div>
                  </TableCell>}
                </TableRow>
              ))}
              {!loading && !loadFailed && filtered.length === 0 && <TableRow><TableCell colSpan={readOnly ? 4 : 5}><div className="flex min-h-52 flex-col items-center justify-center text-muted-foreground"><Database className="mb-3 size-7" /><p className="text-sm">{providers.length ? '没有匹配的 Provider' : '尚未添加 Provider'}</p></div></TableCell></TableRow>}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      {!readOnly && <Dialog open={Boolean(actionTarget)} onOpenChange={open => { if (!open) setActionTarget(null) }}>
        <DialogContent className="max-w-sm">
          <DialogHeader><DialogTitle className="break-all pr-6">{actionTarget?.name}</DialogTitle><DialogDescription>Provider 操作</DialogDescription></DialogHeader>
          {actionTarget && <div className="grid gap-2">
            <Button variant="outline" className="h-11 justify-start" disabled={busy || !actionTarget.enabled || !actionTarget.probe_enabled} onClick={() => { setModelTarget(actionTarget); setActionTarget(null) }}><ListChecks />模型检测</Button>
            <Button variant="outline" className="h-11 justify-start" onClick={() => { copy(actionTarget); setActionTarget(null) }}><Copy />复制配置（不含密钥）</Button>
            <Button variant="outline" className="h-11 justify-start" disabled={!actionTarget.enabled || !actionTarget.probe_enabled || busy || rerunning !== null} onClick={() => { void rerun(actionTarget); setActionTarget(null) }}><RotateCw />重新检测</Button>
            <Button variant="outline" className="h-11 justify-start" onClick={() => { setEditing(actionTarget); setActionTarget(null) }}><Edit2 />编辑 Provider</Button>
            <Button variant="outline" className="h-11 justify-start text-destructive hover:text-destructive" onClick={() => { setDeleteTarget(actionTarget); setActionTarget(null) }}><Trash2 />删除 Provider</Button>
          </div>}
        </DialogContent>
      </Dialog>}

      {!readOnly && <ProviderDialog key={editing === 'new' ? 'new' : editing?.id ?? 'closed'} value={editing} onOpenChange={open => !open && setEditing(null)} onSave={save} />}
      {!readOnly && modelTarget && <ModelCheckDialog provider={modelTarget} onClose={() => setModelTarget(null)} onAccepted={id => { watchedTask.current = id; setBusy(true); setMessage(`模型检测任务 #${id} 已启动`) }} />}
      <AlertDialog open={batchConfirm} onOpenChange={open => { if (!batchBusy) setBatchConfirm(open) }}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>应用批量更改？</AlertDialogTitle><AlertDialogDescription>将修改已选的 {selected.length} 个 Provider，历史记录不变。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={batchBusy}>取消</AlertDialogCancel><AlertDialogAction disabled={batchBusy || !selected.length} onClick={e => { e.preventDefault(); void batch() }}>{batchBusy ? '保存中' : '确认应用'}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>

      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={open => !open && !deleting && setDeleteTarget(null)}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除 Provider？</AlertDialogTitle><AlertDialogDescription>将删除「{deleteTarget?.name}」及其配置。历史检测记录不会被此次操作修改。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel><AlertDialogAction onClick={event => { event.preventDefault(); remove() }} disabled={deleting}>{deleting ? '删除中…' : '确认删除'}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ProviderDialog({ value, onOpenChange, onSave }: { value: EditingState; onOpenChange: (open: boolean) => void; onSave: (id: string | null, update: ProviderUpdate) => Promise<void> }) {
  const seed = value && value !== 'new' ? value : null
  const initial = seed?.isCopy ? null : seed
  const [form, setForm] = useState({
    id: seed?.id ?? '', name: seed?.name ?? '', type: seed?.type ?? 'openai', base_url: seed?.base_url ?? '',
    api_key: '', clear_api_key: false, models: mergeModels(seed?.models ?? []), enabled: seed?.enabled ?? true, probe_enabled: seed?.probe_enabled ?? true,
    group: seed?.group ?? '', tags: (seed?.tags ?? []).join(', '), probe: { ...defaultProbe, ...seed?.probe },
  })
  const [saving, setSaving] = useState(false)
  const [modelDraft, setModelDraft] = useState('')
  const [error, setError] = useState('')
  const [available, setAvailable] = useState<string[]>([])
  const [syncing, setSyncing] = useState(false)
  const [syncError, setSyncError] = useState('')
  const [syncMessage, setSyncMessage] = useState('')
  const pending = useRef<AbortController | null>(null)
  const set = <Key extends keyof typeof form>(key: Key, next: (typeof form)[Key]) => setForm(current => ({ ...current, [key]: next }))
  const endpointChanged = initial && form.base_url.trim().replace(/\/+$/, '') !== initial.base_url.trim().replace(/\/+$/, '')
  const keyConfirmationRequired = initial?.api_key_set && endpointChanged && !form.api_key && !form.clear_api_key
  const keyChangeError = 'Base URL 已变更，请重新填写 API Key 或明确清除原密钥'

  useEffect(() => {
    pending.current?.abort()
    pending.current = null
    setAvailable([]); setSyncing(false); setSyncError(''); setSyncMessage('')
    return () => { pending.current?.abort(); pending.current = null }
  }, [form.base_url, form.api_key, form.clear_api_key, form.type])

  const sync = async () => {
    if (pending.current || saving) return
    if (!form.base_url.trim()) { setSyncError('请先填写 Base URL'); return }
    if (keyConfirmationRequired) { setSyncError(keyChangeError); return }
    const controller = new AbortController()
    pending.current = controller
    setSyncing(true); setSyncError(''); setSyncMessage('')
    try {
      const models = await api.discoverModels({ provider_id: initial?.id, type: form.type, base_url: form.base_url, api_key: form.api_key, clear_api_key: form.clear_api_key }, controller.signal)
      if (controller.signal.aborted) return
      setAvailable(models)
      setForm(current => ({ ...current, models: mergeModels(current.models, models) }))
      setSyncMessage(models.length ? `已同步 ${models.length} 个模型` : '接口未返回可用模型')
    } catch (cause) {
      if (!controller.signal.aborted) setSyncError((cause as Error).message)
    } finally {
      if (pending.current === controller) { pending.current = null; setSyncing(false) }
    }
  }

  const submit = async () => {
    if (saving || syncing) return
    if (!form.id.trim() || !form.name.trim() || !form.base_url.trim()) { setError('ID、名称和 Base URL 均为必填项'); return }
    if (keyConfirmationRequired) { setError(keyChangeError); return }
    const models = mergeModels(form.models, parseModels(modelDraft))
    setForm(current => ({ ...current, models }))
    setModelDraft('')
    setSaving(true); setError('')
    try {
      await onSave(initial?.id ?? null, {
        id: form.id.trim(), name: form.name.trim(), type: form.type.trim() || 'openai', base_url: form.base_url.trim(), api_key: form.api_key,
        clear_api_key: form.clear_api_key, models, enabled: form.enabled, probe_enabled: form.enabled && form.probe_enabled,
        group: form.group.trim(), tags: parseModels(form.tags), probe: form.probe,
      })
    } catch (cause) { setError((cause as Error).message); setSaving(false) }
  }

  return (
    <Dialog open={value !== null} onOpenChange={open => { if (!saving) onOpenChange(open) }}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader><DialogTitle>{initial ? '编辑 Provider' : '新增 Provider'}</DialogTitle><DialogDescription>配置 OpenAI 兼容接口、模型范围与检测状态。</DialogDescription></DialogHeader>
        <fieldset disabled={saving} className="field-grid min-w-0 py-2">
          <Field label="唯一 ID" htmlFor="provider-id" hint={initial ? '创建后不可修改' : '建议使用小写字母、数字和连字符'}><Input id="provider-id" value={form.id} onChange={event => set('id', event.target.value)} placeholder="openai-main" disabled={Boolean(initial)} className="font-mono" /></Field>
          <Field label="显示名称" htmlFor="provider-name"><Input id="provider-name" value={form.name} onChange={event => set('name', event.target.value)} placeholder="OpenAI" /></Field>
          <Field label="Provider 类型" htmlFor="provider-type"><Input id="provider-type" value={form.type} onChange={event => set('type', event.target.value)} placeholder="openai" className="font-mono" /></Field>
          <Field label="Base URL" htmlFor="provider-url"><Input id="provider-url" type="url" value={form.base_url} onChange={event => set('base_url', event.target.value)} placeholder="https://api.openai.com/v1" className="font-mono" /></Field>
          <Field label="分组" htmlFor="provider-group"><Input id="provider-group" maxLength={128} value={form.group} onChange={e => set('group', e.target.value)} /></Field>
          <Field label="标签" htmlFor="provider-tags"><Input id="provider-tags" value={form.tags} onChange={e => set('tags', e.target.value)} placeholder="production, backup" /></Field>
          <Field className="md:col-span-2" label={`API Key${initial?.api_key_set ? (endpointChanged ? '（地址已变更）' : '（留空保留现有值）') : ''}`} htmlFor="provider-key"><Input id="provider-key" type="password" value={form.api_key} onChange={event => set('api_key', event.target.value)} placeholder={initial?.api_key_set ? '已设置' : 'sk-...'} className="font-mono" /></Field>
          {initial?.api_key_set && <ToggleRow className="md:col-span-2" label="清除现有 API Key" description="保存后移除服务端存储的 Key" checked={form.clear_api_key} onCheckedChange={value => set('clear_api_key', value)} danger />}
          <div className="min-w-0 md:col-span-2"><ModelPicker value={form.models} available={available} onChange={models => set('models', models)} draft={modelDraft} onDraftChange={setModelDraft} onSync={sync} syncing={syncing} disabled={saving} syncError={syncError} syncMessage={syncMessage} /></div>
          <ToggleRow className="md:col-span-2" label="启用 Provider" description="停用后不会展示或参与检测" checked={form.enabled} onCheckedChange={value => set('enabled', value)} />
          <ToggleRow className="md:col-span-2" label="参与检测" description="关闭后保留配置和展示，但跳过连通性探测" checked={form.probe_enabled} onCheckedChange={value => set('probe_enabled', value)} disabled={!form.enabled} />
          <ProbeFields value={form.probe} onChange={value => set('probe', value)} />
        </fieldset>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>取消</Button><LoadingButton onClick={submit} loading={saving} disabled={syncing}>保存 Provider</LoadingButton></DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ToggleRow({ label, description, checked, onCheckedChange, disabled, danger, className }: { label: string; description: string; checked: boolean; onCheckedChange: (value: boolean) => void; disabled?: boolean; danger?: boolean; className?: string }) {
  return <div className={`flex items-center justify-between gap-4 rounded-md border p-3 ${className ?? ''}`}><div><p className={`text-sm font-medium ${danger ? 'text-destructive' : ''}`}>{label}</p><p className="mt-0.5 text-xs text-muted-foreground">{description}</p></div><Switch checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} /></div>
}
