import { cn } from '../lib/utils'
import { barCls, STATUS_LABEL } from '../utils/status'

export function StatusLights({ history }: { history: string[] }) {
  return (
    <div className="mt-3 flex w-full gap-1" aria-label="最近检测历史">
      {history.map((status, index) => (
        <span
          key={`${status}-${index}`}
          className={cn('history-cell', barCls(status))}
          title={`第 ${history.length - index} 次检测：${STATUS_LABEL[status] ?? status}`}
        />
      ))}
    </div>
  )
}
