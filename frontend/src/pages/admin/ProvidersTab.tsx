import { useState, useEffect, useCallback } from 'react'
import { Edit2, Trash2, Save, Plus, RefreshCw, XCircle } from 'lucide-react'
import { api } from '../../api'
import type { SafeProviderConfig, ProviderUpdate } from '../../types'
import { useAutoMsg, Btn, Badge, Field, inputCls } from './shared'

function ProviderModal({
  initial,
  onSave,
  onClose,
}: {
  initial: SafeProviderConfig | null
  onSave: (id: string | null, update: ProviderUpdate) => Promise<void>
  onClose: () => void
}) {
  const [form, setForm] = useState<{
    id: string; name: string; type: string; base_url: string
    api_key: string; clear_api_key: boolean; models: string
    enabled: boolean; probe_enabled: boolean
  }>(() => ({
    id: initial?.id ?? '',
    name: initial?.name ?? '',
    type: initial?.type ?? 'openai',
    base_url: initial?.base_url ?? '',
    api_key: '',
    clear_api_key: false,
    models: initial?.models.join(', ') ?? '',
    enabled: initial?.enabled ?? true,
    probe_enabled: initial?.probe_enabled ?? true,
  }))
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState('')

  const set = <K extends keyof typeof form>(key: K, val: (typeof form)[K]) =>
    setForm(f => ({ ...f, [key]: val }))

  const submit = async () => {
    if (!form.name.trim() || !form.base_url.trim()) {
      setErr('名称和 Base URL 为必填项')
      return
    }
    setSaving(true)
    setErr('')
    try {
      await onSave(initial?.id ?? null, {
        id: form.id.trim(),
        name: form.name.trim(),
        type: form.type.trim() || 'openai',
        base_url: form.base_url.trim(),
        api_key: form.api_key,
        clear_api_key: form.clear_api_key,
        models: form.models.split(',').map(m => m.trim()).filter(Boolean),
        enabled: form.enabled,
        probe_enabled: form.probe_enabled,
      })
    } catch (e) {
      setErr((e as Error).message)
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 backdrop-blur-sm" style={{ background: 'rgba(11,16,32,.75)' }}>
      <div className="w-full max-w-lg glass rounded-[24px] overflow-hidden anim-scale-in">
        <div className="flex items-center justify-between px-5 py-4" style={{ borderBottom: '1px solid var(--border)' }}>
          <h3 className="font-semibold" style={{ color: 'var(--text)' }}>
            {initial ? '编辑 Provider' : '新增 Provider'}
          </h3>
          <button onClick={onClose} className="cursor-pointer transition-colors" style={{ color: 'var(--muted)' }} onMouseEnter={e=>(e.currentTarget.style.color='var(--text)')} onMouseLeave={e=>(e.currentTarget.style.color='var(--muted)')}>
            <XCircle className="w-5 h-5" />
          </button>
        </div>

        <div className="p-5 space-y-4 max-h-[70vh] overflow-y-auto">
          <div className="grid grid-cols-2 gap-3">
            <Field label="ID（唯一标识）">
              <input
                className={inputCls}
                value={form.id}
                onChange={e => set('id', e.target.value)}
                placeholder="openai-main"
                disabled={!!initial}
              />
            </Field>
            <Field label="名称 *">
              <input className={inputCls} value={form.name} onChange={e => set('name', e.target.value)} placeholder="OpenAI" />
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Field label="类型">
              <input className={inputCls} value={form.type} onChange={e => set('type', e.target.value)} placeholder="openai" />
            </Field>
            <Field label="Base URL *">
              <input className={inputCls} value={form.base_url} onChange={e => set('base_url', e.target.value)} placeholder="https://api.openai.com/v1" />
            </Field>
          </div>

          <Field label={`API Key${initial?.api_key_set ? '（已设置，留空保留）' : ''}`}>
            <input
              type="password"
              className={inputCls}
              value={form.api_key}
              onChange={e => set('api_key', e.target.value)}
              placeholder={initial?.api_key_set ? '留空保留已有 Key' : 'sk-...'}
            />
            {initial?.api_key_set && (
              <label className="flex items-center gap-2 mt-1.5 cursor-pointer">
                <input
                  type="checkbox"
                  checked={form.clear_api_key}
                  onChange={e => set('clear_api_key', e.target.checked)}
                  className="rounded"
                />
                <span className="text-xs" style={{ color: 'var(--error)' }}>清除已有 Key</span>
              </label>
            )}
          </Field>

          <Field label="模型列表" hint="逗号分隔；留空则自动从 /v1/models 获取">
            <input className={inputCls} value={form.models} onChange={e => set('models', e.target.value)} placeholder="gpt-4o-mini, gpt-4.1-mini" />
          </Field>

          <div className="flex flex-col gap-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} className="rounded" />
              <span className="text-sm" style={{ color: 'var(--text)' }}>启用此 Provider</span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={form.probe_enabled}
                onChange={e => set('probe_enabled', e.target.checked)}
                className="rounded"
                disabled={!form.enabled}
              />
              <span className="text-sm" style={{ color: form.enabled ? 'var(--text)' : 'var(--muted)', opacity: form.enabled ? 1 : .5 }}>
                参与检测运行
              </span>
              <span className="text-[11px] font-mono" style={{ color: 'var(--muted)', opacity: .6 }}>
                （关闭后保留配置但不会被探测）
              </span>
            </label>
          </div>

          {err && <p className="text-red-400 text-xs">{err}</p>}
        </div>

        <div className="flex justify-end gap-2 px-5 py-4" style={{ borderTop: '1px solid var(--border)' }}>
          <Btn onClick={onClose} variant="ghost">取消</Btn>
          <Btn onClick={submit} loading={saving} variant="primary">
            <Save className="w-3.5 h-3.5" />保存
          </Btn>
        </div>
      </div>
    </div>
  )
}

export function ProvidersTab() {
  const [providers, setProviders] = useState<SafeProviderConfig[]>([])
  const [loading, setLoading] = useState(false)
  const [editing, setEditing] = useState<SafeProviderConfig | null | 'new'>(null)
  const [msg, setMsg] = useAutoMsg()
  const [deleting, setDeleting] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  const [rerunning, setRerunning] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const load = useCallback(() => {
    setLoading(true)
    api.providers().then(setProviders).catch(() => {}).finally(() => setLoading(false))
  }, [])

  useEffect(() => { load() }, [load])

  const handleSave = async (id: string | null, update: ProviderUpdate) => {
    if (id) {
      await api.updateProvider(id, update)
    } else {
      await api.createProvider(update)
    }
    setEditing(null)
    setMsg('已保存')
    load()
  }

  const handleDelete = async (id: string) => {
    if (confirmDelete !== id) {
      setConfirmDelete(id)
      return
    }
    setConfirmDelete(null)
    setDeleting(id)
    try {
      await api.deleteProvider(id)
      setMsg('已删除')
      load()
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setDeleting(null)
    }
  }

  const handleRerun = async (id: string, name: string) => {
    setRerunning(id)
    setMsg('')
    try {
      await api.rerunProvider(id)
      setMsg(`「${name}」重新检测已触发，完成后结果将更新至仪表盘`)
    } catch (e) {
      setMsg(`错误：${(e as Error).message}`)
    } finally {
      setRerunning(null)
    }
  }

  const filtered = providers.filter(p => {
    if (!search.trim()) return true
    const q = search.toLowerCase()
    return p.id.toLowerCase().includes(q) || p.name.toLowerCase().includes(q) || p.base_url.toLowerCase().includes(q)
  })

  return (
    <div className="space-y-4 anim-fade-in">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold" style={{ color: 'var(--text)' }}>Provider 管理</h2>
        <div className="flex items-center gap-2 flex-wrap">
          <input
            className="input-glass rounded-xl px-3 py-1.5 text-xs w-44"
            placeholder="搜索 ID / 名称 / URL…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <Btn onClick={load} loading={loading} variant="ghost"><RefreshCw className="w-3.5 h-3.5" /></Btn>
          <Btn onClick={() => setEditing('new')} variant="primary"><Plus className="w-3.5 h-3.5" />新增</Btn>
        </div>
      </div>

      {msg && (
        <p className="text-sm font-mono anim-slide-in-down" style={{ color: msg.startsWith('错误') ? 'var(--error)' : 'var(--ok)' }}>
          {msg}
        </p>
      )}

      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="text-xs text-left" style={{ borderBottom: '1px solid var(--border)', color: 'var(--muted)' }}>
              <th className="pb-2 pr-4 font-medium">ID / 名称</th>
              <th className="pb-2 pr-4 font-medium">类型</th>
              <th className="pb-2 pr-4 font-medium">Base URL</th>
              <th className="pb-2 pr-4 font-medium">模型</th>
              <th className="pb-2 pr-4 font-medium">Key</th>
              <th className="pb-2 pr-3 font-medium">启用</th>
              <th className="pb-2 pr-4 font-medium">检测</th>
              <th className="pb-2 font-medium">操作</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((p, i) => (
              <tr
                key={p.id}
                className="transition-colors anim-fade-in-up"
                style={{ borderBottom: '1px solid var(--border)', animationDelay: `${i * 30}ms` }}
              >
                <td className="py-3 pr-4">
                  <div className="font-mono text-xs" style={{ color: 'var(--muted)' }}>{p.id}</div>
                  <div style={{ color: 'var(--text)' }}>{p.name}</div>
                </td>
                <td className="py-3 pr-4 font-mono text-xs" style={{ color: 'var(--muted)' }}>{p.type}</td>
                <td className="py-3 pr-4 max-w-[180px]">
                  <span className="font-mono text-xs truncate block" style={{ color: 'var(--muted)' }} title={p.base_url}>
                    {p.base_url}
                  </span>
                </td>
                <td className="py-3 pr-4 text-xs" style={{ color: 'var(--muted)' }}>
                  {p.models.length === 0 ? <span className="italic">自动</span> : p.models.length}
                </td>
                <td className="py-3 pr-4">
                  {p.api_key_set
                    ? <span className="text-xs" style={{ color: 'var(--ok)' }}>●</span>
                    : <span className="text-xs" style={{ color: 'var(--muted)', opacity: .4 }}>—</span>}
                </td>
                <td className="py-3 pr-3">
                  <Badge status={p.enabled ? 'ok' : 'error'} />
                </td>
                <td className="py-3 pr-4">
                  <Badge status={!p.enabled ? 'canceled' : p.probe_enabled ? 'ok' : 'paused'} />
                </td>
                <td className="py-3">
                  <div className="flex items-center gap-1">
                    <Btn variant="ghost" onClick={() => handleRerun(p.id, p.name)} loading={rerunning === p.id} className="shrink-0" title="重新检测此 Provider">
                      <RefreshCw className="w-3.5 h-3.5" />
                    </Btn>
                    <Btn variant="ghost" onClick={() => setEditing(p)}>
                      <Edit2 className="w-3.5 h-3.5" />
                    </Btn>
                    {confirmDelete === p.id ? (
                      <>
                        <Btn variant="danger" onClick={() => handleDelete(p.id)} loading={deleting === p.id} className="text-[11px] px-2">
                          确认删除
                        </Btn>
                        <Btn variant="ghost" onClick={() => setConfirmDelete(null)} className="text-[11px] px-2">
                          取消
                        </Btn>
                      </>
                    ) : (
                      <Btn variant="danger" onClick={() => handleDelete(p.id)} loading={deleting === p.id}>
                        <Trash2 className="w-3.5 h-3.5" />
                      </Btn>
                    )}
                  </div>
                </td>
              </tr>
            ))}
            {!loading && providers.length === 0 && (
              <tr>
                <td colSpan={8} className="py-12 text-center" style={{ color: 'var(--muted)', opacity: .5 }}>暂无 Provider</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {editing !== null && (
        <ProviderModal
          initial={editing === 'new' ? null : editing}
          onSave={handleSave}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  )
}
