import { memo, useEffect, useId, useState } from 'react'
import { ChevronDown, Clock3 } from 'lucide-react'
import type { ModelResult } from '../types'
import { cn } from '../lib/utils'
import { statusDotClass } from '../utils/status'
import { Button } from './ui/button'
import { StatusPill } from './StatusPill'
import { CurveChart } from './CurveChart'
import { StatusLights } from './StatusLights'

export const ModelRow = memo(function ModelRow({ result, showError, compact }: { result: ModelResult; showError: boolean; compact?: boolean }) {
  const [expanded, setExpanded] = useState(false)
  const [mounted, setMounted] = useState(false)
  const id = useId()
  useEffect(() => {
    if (expanded) { setMounted(true); return }
    if (!mounted) return
    // Keep details through the closing transition, then release their DOM.
    const timer = setTimeout(() => setMounted(false), 280)
    return () => clearTimeout(timer)
  }, [expanded, mounted])

  return (
    <article className={cn('model-row border-b', compact && 'model-row-compact')} data-expanded={expanded}>
      <div className="model-summary">
        <div className="model-name flex min-w-0 items-center gap-2.5">
          <span className={`status-dot ${statusDotClass(result.status)}`} />
          <h3 className={cn('min-w-0 font-mono text-xs font-medium sm:text-sm', !expanded && 'truncate')} title={result.model}>{result.model}</h3>
        </div>
        <div className="model-latency">
          <span className="model-metric-label">响应延迟</span>
          <span key={result.latency_ms} className="animate-value inline-flex items-center gap-1.5 font-mono text-xs tabular-nums">
            <Clock3 className="size-3 text-muted-foreground sm:hidden" />
            {result.latency_ms > 0 ? `${result.latency_ms.toLocaleString()} ms` : '无延迟数据'}
          </span>
          <span className="ml-3 text-xs text-muted-foreground sm:hidden">成功率 {result.availability || 'N/A'}</span>
        </div>
        <div className="model-availability">
          <span className="model-metric-label">检测成功率</span>
          <span className="font-mono text-xs tabular-nums">{result.availability || 'N/A'}</span>
        </div>
        {!compact && <div className="model-history min-w-0">
          {result.history?.length > 0 ? <StatusLights history={result.history} /> : <span className="text-xs text-muted-foreground">暂无历史</span>}
        </div>}
        <div className="model-status"><StatusPill status={result.status} label={result.status_label} /></div>
        <Button id={`${id}-toggle`} variant="ghost" size="icon" className="model-toggle size-8" onClick={() => setExpanded(value => !value)} aria-expanded={expanded} aria-controls={`${id}-detail`} aria-label={`${expanded ? '收起' : '展开'}模型详情`} title={`${expanded ? '收起' : '展开'}模型详情`}>
          <ChevronDown className={cn('disclosure-chevron', expanded && 'rotate-180')} />
        </Button>
      </div>
      <div id={`${id}-detail`} className="motion-disclosure" data-open={expanded} aria-hidden={!expanded} role="region" aria-label={`${result.model} 检测详情`} {...(!expanded ? { inert: '' } : {})}>
        <div className="min-h-0 overflow-hidden">
          {(expanded || mounted) && <div className="model-detail grid gap-4 border-t border-dashed px-3 py-4 sm:px-5 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="min-w-0">
              <dl className="grid grid-cols-2 gap-x-4 gap-y-4 sm:grid-cols-4">
                <Metric label="P50 / 24h" value={result.p50_latency_24h} />
                <Metric label="P95 / 24h" value={result.p95_latency_24h} />
                <Metric label="P99 / 24h" value={result.p99_latency_24h} />
                <Metric label="24h 样本" value={String(result.latency_samples_24h ?? 0)} />
              </dl>
              <p className="mt-4 text-xs text-muted-foreground">24h 平均延迟 <span className="font-mono text-foreground">{result.avg_latency_24h || 'N/A'}</span></p>
              {result.weekly_success_text && <p className="mt-2 text-xs text-muted-foreground">{result.weekly_success_text}</p>}
            </div>
            {result.show_curve_chart && result.svg_path_line && <div className="min-w-0">
              <p className="mb-2 text-xs text-muted-foreground">延迟趋势</p>
              <CurveChart pathLine={result.svg_path_line} pathArea={result.svg_path_area} status={result.status} animate={expanded} />
              {result.time_labels?.length > 0 && <div className="mt-1 flex justify-between gap-3 text-xs text-muted-foreground"><span>{result.time_labels[0]}</span><span>{result.time_labels[result.time_labels.length - 1]}</span></div>}
            </div>}
            {compact && result.history?.length > 0 && <div className="lg:col-span-2"><StatusLights history={result.history} /></div>}
            {showError && result.error && <div className="min-w-0 border-l-2 border-destructive pl-3 lg:col-span-2"><p className="mb-1 text-xs font-medium text-destructive">错误详情</p><p className="whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-destructive">{result.error}</p></div>}
            {result.response_preview && <div className="min-w-0 lg:col-span-2"><p className="mb-1 text-xs text-muted-foreground">响应内容</p><p className="max-h-48 overflow-auto whitespace-pre-wrap break-all font-mono text-xs leading-relaxed">{result.response_preview}</p></div>}
          </div>}
        </div>
      </div>
    </article>
  )
})

function Metric({ label, value }: { label: string; value: string }) {
  return <div><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 font-mono text-sm font-medium tabular-nums">{value || 'N/A'}</dd></div>
}
