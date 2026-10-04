import { Badge } from './ui/badge'
import { statusClass, statusDotClass } from '../utils/status'

export function StatusPill({ status, label, large }: { status: string; label: string; large?: boolean }) {
  const tone = statusClass(status)
  const variant = status === 'unknown' ? 'muted' : tone === 'ok' ? 'success' : tone === 'slow' ? 'warning' : 'destructive'
  return (
    <Badge variant={variant} className={`shrink-0 whitespace-nowrap ${large ? 'gap-2 px-3 py-1 text-sm' : 'gap-1.5'}`}>
      <span className={`status-dot ${statusDotClass(status)} size-1.5 shadow-none`} />
      {label}
    </Badge>
  )
}
