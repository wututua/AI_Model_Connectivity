import { statusClass } from '../utils/status'

export function CurveChart({ pathLine, pathArea, status }: { pathLine: string; pathArea: string; status: string }) {
  const tone = statusClass(status)
  const color = tone === 'ok' ? 'hsl(var(--success))' : tone === 'slow' ? 'hsl(var(--warning))' : 'hsl(var(--destructive))'
  return (
    <svg className="curve-overlay" viewBox="0 0 100 40" preserveAspectRatio="none" aria-hidden="true">
      <path d={pathArea} fill={color} opacity=".2" />
      <path d={pathLine} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
