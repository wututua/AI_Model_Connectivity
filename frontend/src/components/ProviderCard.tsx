import { useId, useState } from 'react'
import { Boxes, ChevronDown } from 'lucide-react'
import type { ProviderReport } from '../types'
import { Button } from './ui/button'
import { StatusPill } from './StatusPill'
import { ModelRow } from './ModelRow'
import { relativeTime } from '../utils/status'
import { cn } from '../lib/utils'

export function ProviderCard({ provider, showError, compact, now, staleAfterSeconds, animDelay = 0 }: {
  provider: ProviderReport
  showError: boolean
  compact?: boolean
  animDelay?: number
  now?: number
  staleAfterSeconds?: number
}) {
  const [failedLogo, setFailedLogo] = useState('')
  const [collapsed, setCollapsed] = useState(false)
  const id = useId()
  const updated = relativeTime(provider.checked_at ?? '', now, staleAfterSeconds)

  return (
    <section className="provider-section animate-enter" style={{ animationDelay: `${animDelay}ms` }} aria-labelledby={`${id}-title`}>
      <div className="flex items-center justify-between gap-2 border-b px-1 pb-3 sm:gap-4">
        <div className="flex min-w-0 items-center gap-3">
          {provider.provider_logo && failedLogo !== provider.provider_logo ? (
            <img src={provider.provider_logo} alt="" className="size-9 shrink-0 rounded-md border bg-white object-contain p-1" referrerPolicy="no-referrer" onError={() => setFailedLogo(provider.provider_logo)} />
          ) : (
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-card font-mono text-xs font-semibold text-muted-foreground" aria-hidden="true">
              {provider.provider_name.slice(0, 2).toUpperCase()}
            </div>
          )}
          <div className="min-w-0">
            <h2 id={`${id}-title`} className="text-sm font-semibold leading-5 sm:truncate" title={provider.provider_name}>{provider.provider_name}</h2>
            <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
              <span>{provider.provider_type}</span><span>{provider.model_count} 个模型</span>
              <span className="hidden sm:inline" title={provider.checked_at}>{provider.checked_at ? `检测于 ${updated.text}` : '检测时间待更新'}</span>
              {updated.stale && <span className="text-warning">数据可能已过期</span>}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1 sm:gap-3">
          <div className="hidden items-center gap-3 text-xs tabular-nums text-muted-foreground lg:flex" aria-label="Provider 模型统计">
            <span><span className="mr-1 font-mono text-success">{provider.ok_count}</span>正常</span>
            <span><span className="mr-1 font-mono text-warning">{provider.slow_count}</span>较慢</span>
            <span><span className="mr-1 font-mono text-destructive">{provider.error_count}</span>异常</span>
          </div>
          <StatusPill status={provider.status} label={provider.status_label} />
          <Button variant="ghost" size="icon" className="size-8" aria-label={`${collapsed ? '展开' : '收起'} ${provider.provider_name}`} aria-expanded={!collapsed} aria-controls={`${id}-models`} onClick={() => setCollapsed(value => !value)} title={collapsed ? '展开 Provider' : '收起 Provider'}>
            <ChevronDown className={cn('disclosure-chevron', collapsed && '-rotate-90')} />
          </Button>
        </div>
      </div>
      <div className="motion-disclosure" data-open={!collapsed} id={`${id}-models`} aria-hidden={collapsed} {...(collapsed ? { inert: '' } : {})}>
        <div className="min-h-0 overflow-hidden">
          {(provider.results ?? []).length > 0 ? (
            provider.results.map(result => <ModelRow key={result.model} result={result} showError={showError} compact={compact} />)
          ) : (
            <div className="flex min-h-28 flex-col items-center justify-center gap-2 text-muted-foreground">
              <Boxes className="size-5" /><span className="text-sm">没有匹配的模型</span>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
