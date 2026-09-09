import { useState } from 'react'
import { Boxes, CircleAlert, CircleCheck, Timer } from 'lucide-react'
import type { ProviderReport } from '../types'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from './ui/card'
import { StatusPill } from './StatusPill'
import { ModelRow } from './ModelRow'

export function ProviderCard({ provider, showError, compact }: {
  provider: ProviderReport
  showError: boolean
  compact?: boolean
  animDelay?: number
}) {
  const [failedLogo, setFailedLogo] = useState('')
  return (
    <Card className="overflow-hidden shadow-panel">
      <CardHeader className="border-b p-4">
        <div className="flex items-start justify-between gap-4">
          <div className="flex min-w-0 items-center gap-3">
            {provider.provider_logo && failedLogo !== provider.provider_logo ? (
              <img
                src={provider.provider_logo}
                alt=""
                className="size-9 shrink-0 rounded-md border bg-white object-contain p-1"
                referrerPolicy="no-referrer"
                onError={() => setFailedLogo(provider.provider_logo)}
              />
            ) : (
              <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted font-mono text-xs font-semibold text-muted-foreground">
                {provider.provider_name.slice(0, 2).toUpperCase()}
              </div>
            )}
            <div className="min-w-0">
              <CardTitle className="truncate text-sm">{provider.provider_name}</CardTitle>
              <CardDescription className="mt-1 flex items-center gap-1.5 font-mono text-xs">
                <span>{provider.provider_type}</span><span>·</span><span>{provider.model_count} 个模型</span>
              </CardDescription>
            </div>
          </div>
          <StatusPill status={provider.status} label={provider.status_label} />
        </div>
        <div className="grid grid-cols-3 gap-2 pt-2">
          <ProviderMetric icon={<CircleCheck />} label="正常" value={provider.ok_count} tone="text-success" />
          <ProviderMetric icon={<Timer />} label="较慢" value={provider.slow_count} tone="text-warning" />
          <ProviderMetric icon={<CircleAlert />} label="异常" value={provider.error_count} tone="text-destructive" />
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {(provider.results ?? []).length > 0 ? (
          provider.results.map(result => <ModelRow key={result.model} result={result} showError={showError} compact={compact} />)
        ) : (
          <div className="flex min-h-28 flex-col items-center justify-center gap-2 text-muted-foreground">
            <Boxes className="size-5" /><span className="text-sm">没有匹配的模型</span>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function ProviderMetric({ icon, label, value, tone }: { icon: React.ReactNode; label: string; value: number; tone: string }) {
  return (
    <div className="flex items-center gap-2 rounded-md bg-muted/60 px-2.5 py-2">
      <span className={`${tone} [&>svg]:size-3.5`}>{icon}</span>
      <span className="text-xs text-muted-foreground">{label}</span>
      <strong className="ml-auto font-mono text-xs tabular-nums">{value}</strong>
    </div>
  )
}
