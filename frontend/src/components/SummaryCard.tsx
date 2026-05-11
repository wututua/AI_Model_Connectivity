import type { ReactNode } from 'react'

export function SummaryCard({ icon, label, value, status, animDelay = 0 }: {
  icon: ReactNode
  label: string
  value: number | string
  status?: string
  animDelay?: number
}) {
  const valueColor = status
    ? (status === 'ok' ? 'var(--ok)' : status === 'slow' ? 'var(--slow)' : 'var(--error)')
    : 'var(--text)'
  return (
    <div className="glass summary-card rounded-[22px] px-4 py-4 anim-fade-in-up" style={{ animationDelay: `${animDelay}ms` }}>
      <div className="flex items-center gap-1.5 mb-2" style={{ color: 'var(--muted)' }}>
        {icon}
        <span className="text-xs uppercase tracking-widest" style={{ letterSpacing: '.16em' }}>{label}</span>
      </div>
      <strong className="block text-2xl font-mono font-bold" style={{ color: valueColor }}>
        {value}
      </strong>
    </div>
  )
}
