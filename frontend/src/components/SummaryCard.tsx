import type { ReactNode } from 'react'
import { Card, CardContent } from './ui/card'
import { cn } from '../lib/utils'

export function SummaryCard({ icon, label, value, status }: {
  icon: ReactNode
  label: string
  value: number | string
  status?: string
  animDelay?: number
}) {
  return (
    <Card>
      <CardContent className="p-4">
        <div className="mb-3 flex items-center justify-between text-muted-foreground">
          <span className="data-label">{label}</span>
          {icon}
        </div>
        <strong className={cn('metric-value block', status === 'ok' && 'text-success', status === 'slow' && 'text-warning', status === 'error' && 'text-destructive')}>
          {value}
        </strong>
      </CardContent>
    </Card>
  )
}
