import { useState } from 'react'
import { ChevronDown, Clock3, Gauge, ShieldCheck } from 'lucide-react'
import type { ModelResult } from '../types'
import { cn } from '../lib/utils'
import { statusDotClass } from '../utils/status'
import { Button } from './ui/button'
import { StatusPill } from './StatusPill'
import { CurveChart } from './CurveChart'
import { StatusLights } from './StatusLights'

export function ModelRow({ result, showError, compact }: { result: ModelResult; showError: boolean; compact?: boolean }) {
  const [expanded, setExpanded] = useState(false)
  const hasDetail = Boolean(result.error || result.response_preview)

  if (compact) {
    return (
      <div className="grid min-h-11 grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 border-t px-4 py-2 first:border-t-0 hover:bg-muted/40">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`status-dot ${statusDotClass(result.status)}`} />
          <span className="truncate font-mono text-xs font-medium">{result.model}</span>
        </div>
        <span className="code-value text-muted-foreground">{result.latency_ms > 0 ? `${result.latency_ms} ms` : '—'}</span>
        <StatusPill status={result.status} label={result.status_label} />
      </div>
    )
  }

  return (
    <div className="relative overflow-hidden border-t first:border-t-0">
      {result.show_curve_chart && result.svg_path_line && (
        <CurveChart pathLine={result.svg_path_line} pathArea={result.svg_path_area} status={result.status} />
      )}
      <div className="relative z-[1] p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-start gap-2.5">
            <span className={`status-dot ${statusDotClass(result.status)} mt-1.5`} />
            <div className="min-w-0">
              <h3 className="truncate font-mono text-sm font-semibold">{result.model}</h3>
              <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span className="inline-flex items-center gap-1"><Clock3 className="size-3" />{result.latency_ms > 0 ? `${result.latency_ms} ms` : '无延迟数据'}</span>
                <span className="inline-flex items-center gap-1"><Gauge className="size-3" />24h {result.avg_latency_24h}</span>
                <span className="inline-flex items-center gap-1"><ShieldCheck className="size-3" />可用率 {result.availability}</span>
              </div>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <StatusPill status={result.status} label={result.status_label} />
            {hasDetail && (
              <Button variant="ghost" size="icon" className="size-8" onClick={() => setExpanded(value => !value)} aria-label={expanded ? '收起模型详情' : '展开模型详情'}>
                <ChevronDown className={cn('transition-transform', expanded && 'rotate-180')} />
              </Button>
            )}
          </div>
        </div>

        <div className="mt-3 grid grid-cols-2 gap-2 text-xs sm:grid-cols-4">
          <Metric label="P50" value={result.p50_latency_24h} />
          <Metric label="P95" value={result.p95_latency_24h} />
          <Metric label="P99" value={result.p99_latency_24h} />
          <Metric label="24h 样本" value={String(result.latency_samples_24h)} />
        </div>

        {result.history?.length > 0 && <StatusLights history={result.history} />}

        {expanded && (
          <div className="mt-3 space-y-2 rounded-md border bg-background/80 p-3 text-xs">
            {showError && result.error && <p className="whitespace-pre-wrap break-all font-mono text-destructive">{result.error}</p>}
            {result.response_preview && <p className="whitespace-pre-wrap break-all font-mono text-muted-foreground">{result.response_preview}</p>}
            <p className="text-muted-foreground">{result.weekly_success_text}</p>
          </div>
        )}
      </div>
    </div>
  )
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-muted/60 px-2.5 py-2">
      <span className="block text-[11px] text-muted-foreground">{label}</span>
      <span className="mt-0.5 block font-mono font-medium tabular-nums">{value || '—'}</span>
    </div>
  )
}
