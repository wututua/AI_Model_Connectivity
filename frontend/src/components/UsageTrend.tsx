import { useMemo, useState, type KeyboardEvent, type PointerEvent } from 'react'
import type { BillingDaily } from '../types'
import { buildUsageChart, formatTokens, formatUsageDate } from '../utils/usageChart'

export function UsageTrend({ daily }: { daily: BillingDaily[] }) {
  const chart = useMemo(() => buildUsageChart(daily), [daily])
  const [selected, setSelected] = useState<number | null>(null)
  if (!chart) return <div className="flex h-60 items-center justify-center border-y text-sm text-muted-foreground">所选时间范围内暂无趋势数据</div>

  const active = Math.min(selected ?? chart.points.length - 1, chart.points.length - 1)
  const point = chart.points[active]
  const selectPoint = (event: PointerEvent<HTMLDivElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    const fraction = ((event.clientX - bounds.left) / bounds.width - 0.02) / 0.96
    setSelected(Math.max(0, Math.min(chart.points.length - 1, Math.round(fraction * (chart.points.length - 1)))))
  }
  const selectWithKey = (event: KeyboardEvent<HTMLDivElement>) => {
    let next = active
    if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') next--
    else if (event.key === 'ArrowRight' || event.key === 'ArrowUp') next++
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = chart.points.length - 1
    else return
    event.preventDefault()
    setSelected(Math.max(0, Math.min(chart.points.length - 1, next)))
  }

  return (
    <section className="border-y py-5" aria-label="每日 Token 趋势">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div><h2 className="text-sm font-semibold">每日 Token 趋势</h2><p className="mt-1 text-xs text-muted-foreground">{chart.points.length} 天采样 <span className="mx-1">/</span>峰值 {formatTokens(chart.peak)}</p></div>
        <div className="text-right" aria-live="off">
          <p className="font-mono text-lg font-semibold tabular-nums">{point.value.toLocaleString()} <span className="font-sans text-xs font-normal text-muted-foreground">Token</span></p>
          <time className="text-xs text-muted-foreground" dateTime={point.day}>{formatUsageDate(point.day, true)}</time>
        </div>
      </div>
      <div className="grid grid-cols-[2.75rem_minmax(0,1fr)] gap-2">
        <div className="relative h-44 select-none font-mono text-xs tabular-nums text-muted-foreground sm:h-52" aria-hidden="true">
          {[1, 0.5, 0].map(fraction => <span key={fraction} className="absolute right-0 -translate-y-1/2" style={{ top: `${92 - fraction * 84}%` }}>{formatTokens(chart.ceiling * fraction)}</span>)}
        </div>
        <div className="relative h-44 min-w-0 touch-pan-y rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background sm:h-52"
          role="slider" tabIndex={0} aria-label="每日 Token 用量" aria-valuemin={0} aria-valuemax={chart.points.length - 1} aria-valuenow={active}
          aria-valuetext={`${formatUsageDate(point.day, true)}，${point.value.toLocaleString()} Token`}
          onPointerMove={selectPoint} onPointerDown={selectPoint} onKeyDown={selectWithKey}>
          <svg viewBox="0 0 100 100" preserveAspectRatio="none" className="h-full w-full overflow-visible" aria-hidden="true">
            {[8, 50, 92].map(y => <line key={y} x1="0" x2="100" y1={y} y2={y} stroke="hsl(var(--border))" strokeDasharray="3 4" vectorEffect="non-scaling-stroke" />)}
            <path d={chart.area} fill="hsl(var(--primary))" opacity=".08" />
            <path className="chart-line" d={chart.line} fill="none" stroke="hsl(var(--primary))" strokeWidth="2" strokeLinejoin="round" vectorEffect="non-scaling-stroke" />
            <line x1={point.x} x2={point.x} y1="4" y2="92" stroke="hsl(var(--muted-foreground))" strokeDasharray="3 3" opacity=".6" vectorEffect="non-scaling-stroke" />
          </svg>
          <span className="pointer-events-none absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-background bg-primary ring-2 ring-primary/25" style={{ left: `${point.x}%`, top: `${point.y}%` }} />
        </div>
        <div className="col-start-2 flex justify-between gap-2 text-xs text-muted-foreground" aria-hidden="true">
          <span>{formatUsageDate(chart.points[0].day)}</span><span>{chart.points.length > 1 && formatUsageDate(chart.points[chart.points.length - 1].day)}</span>
        </div>
      </div>
    </section>
  )
}
