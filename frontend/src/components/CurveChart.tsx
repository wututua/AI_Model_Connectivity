import { statusClass } from '../utils/status'

export function CurveChart({ pathLine, pathArea, status, animate }: { pathLine: string; pathArea: string; status: string; animate?: boolean }) {
  const tone = statusClass(status)
  const color = status === 'unknown' ? 'hsl(var(--muted-foreground))' : tone === 'ok' ? 'hsl(var(--success))' : tone === 'slow' ? 'hsl(var(--warning))' : 'hsl(var(--destructive))'
  return (
    <svg className="h-20 w-full overflow-visible" viewBox="0 0 100 40" preserveAspectRatio="none" role="img" aria-label="历史延迟趋势">
      {[10, 25, 40].map(y => <line key={y} x1="0" x2="100" y1={y} y2={y} stroke="hsl(var(--border))" strokeDasharray="2 3" vectorEffect="non-scaling-stroke" />)}
      <path d={pathArea} fill={color} opacity=".08" />
      <path className={animate ? 'chart-line' : undefined} d={pathLine} fill="none" stroke={color} strokeWidth="1.75" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
