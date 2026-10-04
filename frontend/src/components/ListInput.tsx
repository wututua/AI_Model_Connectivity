import { Plus, X } from 'lucide-react'
import { mergeModels, parseModels } from '../utils/models'
import { Button } from './ui/button'
import { Input } from './ui/input'

export function ListInput({ id, label, value, onChange, draft, onDraftChange }: {
  id: string; label: string; value: string[]; onChange: (value: string[]) => void
  draft: string; onDraftChange: (value: string) => void
}) {
  const commit = (text: string) => {
    onChange(mergeModels(value, parseModels(text)))
    onDraftChange('')
  }
  return <div className="min-w-0 rounded-md border border-input bg-background p-2">
    {value.length > 0 && <ul aria-label={label} className="mb-2 flex max-h-36 flex-wrap gap-1.5 overflow-y-auto scrollbar-thin">
      {value.map(item => <li key={item} className="flex max-w-full items-center gap-1 rounded bg-muted py-1 pl-2 pr-1 text-xs">
        <span className="min-w-0 break-all">{item}</span>
        <Button type="button" variant="ghost" size="icon" className="size-6 shrink-0" aria-label={`移除 ${item}`} title={`移除 ${item}`} onClick={() => onChange(value.filter(entry => entry !== item))}><X className="!size-3" /></Button>
      </li>)}
    </ul>}
    <div className="flex items-center gap-2">
      <Input id={id} value={draft} className="min-w-0 flex-1 border-0 bg-transparent shadow-none" placeholder="添加条目" onChange={event => onDraftChange(event.target.value)}
        onBlur={() => { if (draft.trim()) commit(draft) }}
        onKeyDown={event => {
          if (!event.nativeEvent.isComposing && ['Enter', ',', '，', ';', '；'].includes(event.key)) {
            event.preventDefault()
            commit(draft)
          }
        }}
        onPaste={event => {
          const text = event.clipboardData.getData('text')
          if (/[\n\r,;，；]/.test(text)) {
            event.preventDefault()
            const start = event.currentTarget.selectionStart ?? draft.length
            const end = event.currentTarget.selectionEnd ?? start
            commit(draft.slice(0, start) + text + draft.slice(end))
          }
        }} />
      <Button type="button" variant="ghost" size="icon" className="size-8 shrink-0" disabled={!draft.trim()} aria-label={`添加${label}`} title={`添加${label}`} onMouseDown={event => event.preventDefault()} onClick={() => commit(draft)}><Plus /></Button>
    </div>
  </div>
}
