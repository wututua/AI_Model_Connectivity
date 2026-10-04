import { useId, useMemo, useState } from 'react'
import { Boxes, ChevronDown, Plus, RefreshCw, Search, Trash2, X } from 'lucide-react'
import { cn } from '../lib/utils'
import { mergeModels, parseModels } from '../utils/models'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Tooltip, TooltipContent, TooltipTrigger } from './ui/tooltip'

interface ModelPickerProps {
  value: string[]
  available: string[]
  onChange: (models: string[]) => void
  onSync: () => void
  syncing: boolean
  disabled?: boolean
  syncError: string
  syncMessage: string
}

export function ModelPicker({ value, available, onChange, onSync, syncing, disabled, syncError, syncMessage }: ModelPickerProps) {
  const id = useId()
  const [expanded, setExpanded] = useState(false)
  const [adding, setAdding] = useState(false)
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [manualModels, setManualModels] = useState(value)
  const [message, setMessage] = useState('')
  const choices = useMemo(() => mergeModels(available, manualModels, value), [available, manualModels, value])
  const selected = useMemo(() => new Set(value), [value])
  const matches = choices.filter(model => model.toLowerCase().includes(query.trim().toLowerCase()))

  const add = (text: string) => {
    const models = parseModels(text)
    if (!models.length) { setMessage('请输入模型 ID'); return }
    const next = mergeModels(value, models)
    setManualModels(current => mergeModels(current, models))
    onChange(next)
    setInput('')
    setMessage(next.length === value.length ? '模型已在列表中' : `已添加 ${next.length - value.length} 个模型`)
  }
  const toggle = (model: string) => onChange(selected.has(model) ? value.filter(item => item !== model) : mergeModels(value, [model]))

  return <div className="min-w-0 space-y-3" role="group" aria-labelledby={`${id}-label`}>
    <p id={`${id}-label`} className="text-sm font-medium">模型列表</p>
    <div className="min-w-0 rounded-md border border-input bg-background p-2">
      {value.length > 0 ? <ul aria-label="已选模型" className="grid max-h-52 grid-cols-1 gap-1.5 overflow-y-auto overscroll-contain pr-0.5 scrollbar-thin sm:grid-cols-2">
        {value.map(model => <li key={model} className="flex min-w-0 items-center gap-2 rounded bg-muted px-2 py-1 animate-enter">
          <Boxes className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate text-xs leading-6" title={model}>{model}</span>
          <Tooltip><TooltipTrigger asChild><Button type="button" size="icon" variant="ghost" className="size-6 shrink-0 text-muted-foreground hover:text-destructive" disabled={disabled} aria-label={`移除模型 ${model}`} onClick={() => onChange(value.filter(item => item !== model))}><X className="!size-3.5" /></Button></TooltipTrigger><TooltipContent>移除模型</TooltipContent></Tooltip>
        </li>)}
      </ul> : <div className="flex min-h-16 items-center justify-center gap-2 text-xs text-muted-foreground"><Boxes className="size-4" />自动获取全部模型</div>}
      <button type="button" disabled={disabled} aria-expanded={expanded} aria-controls={`${id}-choices`} aria-label="选择模型" onClick={() => setExpanded(current => !current)} className="mt-2 flex h-9 w-full items-center justify-between gap-2 border-t px-1 text-xs text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
        <span aria-live="polite">{value.length} 个模型</span><ChevronDown className={cn('size-4 disclosure-chevron', expanded && 'rotate-180')} />
      </button>
      <div id={`${id}-choices`} className="motion-disclosure" data-open={expanded} aria-hidden={!expanded} {...(!expanded ? { inert: '' } : {})}>
        <div className="min-h-0 overflow-hidden">
          <div className="relative mb-2 mt-1"><Search className="absolute left-2.5 top-2.5 size-4 text-muted-foreground" /><Input value={query} onChange={event => setQuery(event.target.value)} className="h-9 pl-8" placeholder="搜索模型" aria-label="搜索候选模型" disabled={disabled} /></div>
          <div className="max-h-44 overflow-y-auto overscroll-contain scrollbar-thin">
            {matches.map(model => <label key={model} className="flex cursor-pointer items-start gap-2 rounded px-2 py-2 text-xs transition-colors hover:bg-accent">
              <input type="checkbox" className="mt-0.5 size-3.5 shrink-0 accent-primary" checked={selected.has(model)} onChange={() => toggle(model)} disabled={disabled} />
              <span className="min-w-0 break-all leading-5">{model}</span>
            </label>)}
            {!matches.length && <p className="py-4 text-center text-xs text-muted-foreground">{choices.length ? '没有匹配的模型' : '暂无候选模型'}</p>}
          </div>
        </div>
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-2">
      <Button type="button" variant="outline" size="sm" disabled={disabled} aria-expanded={adding} aria-controls={`${id}-manual`} onClick={() => { setAdding(current => !current); setMessage('') }}><Plus />手动添加</Button>
      <Button type="button" variant="outline" size="sm" disabled={disabled || syncing} onClick={onSync} className="border-success/35 text-success hover:bg-success/10 hover:text-success"><RefreshCw className={syncing ? 'animate-spin' : ''} />{syncing ? '同步中' : '同步模型'}</Button>
      <Tooltip><TooltipTrigger asChild><Button type="button" variant="outline" size="icon" className="size-8 border-destructive/35 text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={disabled || !value.length} aria-label="清除所有模型" onClick={() => { onChange([]); setMessage('') }}><Trash2 /></Button></TooltipTrigger><TooltipContent>清空选择，恢复自动获取</TooltipContent></Tooltip>
    </div>
    {adding && <div id={`${id}-manual`} className="flex min-w-0 gap-2 animate-enter">
      <Input value={input} onChange={event => { setInput(event.target.value); setMessage('') }} disabled={disabled} className="min-w-0 flex-1 font-mono text-xs" placeholder="模型 ID" aria-label="手动添加模型" autoFocus
        onKeyDown={event => { if (event.key === 'Enter' && !event.nativeEvent.isComposing) { event.preventDefault(); add(input) } }}
        onPaste={event => { const text = event.clipboardData.getData('text'); if (/[\n\r,;，；]/.test(text)) { event.preventDefault(); add(text) } }} />
      <Button type="button" variant="secondary" size="icon" disabled={disabled || !input.trim()} onClick={() => add(input)} aria-label="添加模型" title="添加模型"><Plus /></Button>
    </div>}
    {message && <p role="status" className="text-xs text-muted-foreground">{message}</p>}
    {syncError ? <p role="alert" className="break-words text-xs text-destructive">{syncError}</p> : syncMessage && <p role="status" className="text-xs text-success">{syncMessage}</p>}
  </div>
}
