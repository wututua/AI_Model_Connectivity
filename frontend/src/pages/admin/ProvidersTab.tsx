import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Database, Edit2, KeyRound, MoreHorizontal, Plus, RefreshCw, RotateCw, Search, Trash2 } from 'lucide-react'
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '../../components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '../../components/ui/tooltip'
import { Feedback, Field, ListSkeleton, LoadingButton, useAutoMsg } from './shared'
import { mergeModels } from '../../utils/models'

type EditingState = SafeProviderConfig | 'new' | null

export function ProvidersTab({ readOnly = false }: { readOnly?: boolean }) {
  const [providers, setProviders] = useState<SafeProviderConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [editing, setEditing] = useState<EditingState>(null)
  const [deleteTarget, setDeleteTarget] = useState<SafeProviderConfig | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [rerunning, setRerunning] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [message, setMessage] = useAutoMsg()
  const [actionTarget, setActionTarget] = useState<SafeProviderConfig | null>(null)
  const [loadFailed, setLoadFailed] = useState(false)

  const load = useCallback(() => {
    setLoading(true); setLoadFailed(false)
    api.providers().then(setProviders).catch(cause => { setLoadFailed(true); setMessage(`错误：${(cause as Error).message}`) }).finally(() => setLoading(false))
  }, [setMessage])

  useEffect(() => { load() }, [load])

  const filtered = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return providers
    return providers.filter(provider => [provider.id, provider.name, provider.type, provider.base_url, ...provider.models].some(value => value.toLowerCase().includes(query)))
  }, [providers, search])

  const save = async (id: string | null, update: ProviderUpdate) => {
    if (id) await api.updateProvider(id, update)
    else await api.createProvider(update)
    setEditing(null); setMessage('Provider 已保存'); load()
  }

  const remove = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try { await api.deleteProvider(deleteTarget.id); setDeleteTarget(null); setMessage('Provider 已删除'); load() }
    catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setDeleting(false) }
  }

  const rerun = async (provider: SafeProviderConfig) => {
    setRerunning(provider.id); setMessage('')
    try { await api.rerunProvider(provider.id); setMessage(`已触发「${provider.name}」的检测任务`) }
    catch (cause) { setMessage(`错误：${(cause as Error).message}`) }
    finally { setRerunning(null) }
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
      <Feedback message={message} />

      <div className="divide-y border-y sm:hidden" aria-busy={loading}>
        {loading && !providers.length && <ListSkeleton label="正在加载 Provider" />}
        {filtered.map(provider => <section key={provider.id} className="min-w-0 py-4" aria-label={provider.name}>
          <div className="flex items-start gap-2">
            <div className="min-w-0 flex-1"><h2 className="break-words text-sm font-semibold">{provider.name}</h2><p className="mt-1 break-all text-xs text-muted-foreground">{provider.id} · {provider.type}</p></div>
            {!readOnly && <Button variant="ghost" size="icon" className="size-10 shrink-0" aria-label={`${provider.name} 更多操作`} title="更多操作" onClick={() => setActionTarget(provider)}><MoreHorizontal /></Button>}
          </div>
          <p className="mt-2 break-all text-xs text-muted-foreground">{provider.base_url}</p>
          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <Badge variant={provider.enabled ? 'success' : 'muted'}>{provider.enabled ? '已启用' : '已停用'}</Badge>
            <span>{provider.enabled && provider.probe_enabled ? '参与检测' : '不参与检测'}</span>
            <span>{provider.models.length ? `${provider.models.length} 个模型` : '自动获取模型'}</span>
          </div>
        </section>)}
        {!loading && !loadFailed && !filtered.length && <p className="py-12 text-center text-sm text-muted-foreground">{providers.length ? '没有匹配的 Provider' : '尚未添加 Provider'}</p>}
      </div>

      <Card className="hidden sm:block" aria-busy={loading}>
        <CardContent className="p-0">
          <Table>
            <TableHeader><TableRow><TableHead>Provider</TableHead><TableHead className="hidden md:table-cell">连接地址</TableHead><TableHead className="hidden sm:table-cell">模型</TableHead><TableHead>状态</TableHead>{!readOnly && <TableHead className="w-[132px] text-right">操作</TableHead>}</TableRow></TableHeader>
            <TableBody>
              {loading && !providers.length && <TableRow><TableCell colSpan={readOnly ? 4 : 5}><ListSkeleton label="正在加载 Provider" /></TableCell></TableRow>}
              {filtered.map(provider => (
                <TableRow key={provider.id}>
                  <TableCell><div className="flex items-center gap-3"><div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted font-mono text-xs font-semibold">{provider.name.slice(0, 2).toUpperCase()}</div><div className="min-w-0"><p className="truncate font-medium">{provider.name}</p><p className="truncate font-mono text-xs text-muted-foreground">{provider.id} · {provider.type}</p></div></div></TableCell>
                  <TableCell className="hidden max-w-[320px] md:table-cell"><p className="truncate font-mono text-xs text-muted-foreground" title={provider.base_url}>{provider.base_url}</p><p className="mt-1 flex items-center gap-1 text-xs text-muted-foreground"><KeyRound className="size-3" />{provider.api_key_set ? 'API Key 已设置' : '未设置 API Key'}</p></TableCell>
                  <TableCell className="hidden sm:table-cell"><span className="font-mono text-xs">{provider.models.length || '自动'}</span></TableCell>
                  <TableCell className="whitespace-nowrap"><div className="flex flex-col items-start gap-1"><Badge variant={provider.enabled ? 'success' : 'muted'}>{provider.enabled ? '已启用' : '已停用'}</Badge><span className="text-xs text-muted-foreground">{provider.enabled && provider.probe_enabled ? '参与检测' : '不参与检测'}</span></div></TableCell>
                  {!readOnly && <TableCell>
                    <div className="flex justify-end gap-1">
                      <Tooltip><TooltipTrigger asChild><Button variant="ghost" size="icon" onClick={() => rerun(provider)} disabled={!provider.enabled || !provider.probe_enabled || rerunning !== null} aria-label={`重新检测 ${provider.name}`}><RotateCw className={rerunning === provider.id ? 'animate-spin' : ''} /></Button></TooltipTrigger><TooltipContent>重新检测</TooltipContent></Tooltip>
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
            <Button variant="outline" className="h-11 justify-start" disabled={!actionTarget.enabled || !actionTarget.probe_enabled || rerunning !== null} onClick={() => { void rerun(actionTarget); setActionTarget(null) }}><RotateCw />重新检测</Button>
            <Button variant="outline" className="h-11 justify-start" onClick={() => { setEditing(actionTarget); setActionTarget(null) }}><Edit2 />编辑 Provider</Button>
            <Button variant="outline" className="h-11 justify-start text-destructive hover:text-destructive" onClick={() => { setDeleteTarget(actionTarget); setActionTarget(null) }}><Trash2 />删除 Provider</Button>
          </div>}
        </DialogContent>
      </Dialog>}

      {!readOnly && <ProviderDialog key={editing === 'new' ? 'new' : editing?.id ?? 'closed'} value={editing} onOpenChange={open => !open && setEditing(null)} onSave={save} />}

      <AlertDialog open={Boolean(deleteTarget)} onOpenChange={open => !open && !deleting && setDeleteTarget(null)}>
        <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>删除 Provider？</AlertDialogTitle><AlertDialogDescription>将删除「{deleteTarget?.name}」及其配置。历史检测记录不会被此次操作修改。</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={deleting}>取消</AlertDialogCancel><AlertDialogAction onClick={event => { event.preventDefault(); remove() }} disabled={deleting}>{deleting ? '删除中…' : '确认删除'}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ProviderDialog({ value, onOpenChange, onSave }: { value: EditingState; onOpenChange: (open: boolean) => void; onSave: (id: string | null, update: ProviderUpdate) => Promise<void> }) {
  const initial = value && value !== 'new' ? value : null
  const [form, setForm] = useState({
    id: initial?.id ?? '', name: initial?.name ?? '', type: initial?.type ?? 'openai', base_url: initial?.base_url ?? '',
    api_key: '', clear_api_key: false, models: mergeModels(initial?.models ?? []), enabled: initial?.enabled ?? true, probe_enabled: initial?.probe_enabled ?? true,
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [available, setAvailable] = useState<string[]>([])
  const [syncing, setSyncing] = useState(false)
  const [syncError, setSyncError] = useState('')
  const [syncMessage, setSyncMessage] = useState('')
  const pending = useRef<AbortController | null>(null)
  const set = <Key extends keyof typeof form>(key: Key, next: (typeof form)[Key]) => setForm(current => ({ ...current, [key]: next }))

  useEffect(() => {
    pending.current?.abort()
    pending.current = null
    setAvailable([]); setSyncing(false); setSyncError(''); setSyncMessage('')
    return () => { pending.current?.abort(); pending.current = null }
  }, [form.base_url, form.api_key, form.clear_api_key, form.type])

  const sync = async () => {
    if (pending.current || saving) return
    if (!form.base_url.trim()) { setSyncError('请先填写 Base URL'); return }
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
    setSaving(true); setError('')
    try {
      await onSave(initial?.id ?? null, {
        id: form.id.trim(), name: form.name.trim(), type: form.type.trim() || 'openai', base_url: form.base_url.trim(), api_key: form.api_key,
        clear_api_key: form.clear_api_key, models: form.models, enabled: form.enabled, probe_enabled: form.enabled && form.probe_enabled,
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
          <Field className="md:col-span-2" label={`API Key${initial?.api_key_set ? '（留空保留现有值）' : ''}`} htmlFor="provider-key"><Input id="provider-key" type="password" value={form.api_key} onChange={event => set('api_key', event.target.value)} placeholder={initial?.api_key_set ? '已设置' : 'sk-...'} className="font-mono" /></Field>
          {initial?.api_key_set && <ToggleRow className="md:col-span-2" label="清除现有 API Key" description="保存后移除服务端存储的 Key" checked={form.clear_api_key} onCheckedChange={value => set('clear_api_key', value)} danger />}
          <div className="min-w-0 md:col-span-2"><ModelPicker value={form.models} available={available} onChange={models => set('models', models)} onSync={sync} syncing={syncing} disabled={saving} syncError={syncError} syncMessage={syncMessage} /></div>
          <ToggleRow className="md:col-span-2" label="启用 Provider" description="停用后不会展示或参与检测" checked={form.enabled} onCheckedChange={value => set('enabled', value)} />
          <ToggleRow className="md:col-span-2" label="参与检测" description="关闭后保留配置和展示，但跳过连通性探测" checked={form.probe_enabled} onCheckedChange={value => set('probe_enabled', value)} disabled={!form.enabled} />
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
