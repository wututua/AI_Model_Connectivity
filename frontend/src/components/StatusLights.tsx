import { cn } from '../lib/utils'
import { barCls, STATUS_LABEL } from '../utils/status'

export function StatusLights({ history }: { history: string[] }) {
  return (
    <div className="history-strip grid h-2 w-full" style={{ gridTemplateColumns: `repeat(${history.length}, minmax(0, 1fr))`, gap: history.length > 80 ? 0 : 1 }} role="img" aria-label={`最近检测历史：${history.map(status => STATUS_LABEL[status] ?? '无数据').join('、')}`}>
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
