export function CurveChart({ pathLine, pathArea, status }: { pathLine: string; pathArea: string; status: string }) {
  const color = status === 'ok' ? 'var(--ok)' : status === 'slow' ? 'var(--slow)' : 'var(--error)'
  return (
    <svg
      className="curve-overlay"
      viewBox="0 0 100 40"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <path d={pathArea} fill={color} opacity=".18" />
      <path d={pathLine} fill="none" stroke={color} strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}
