import { barCls, STATUS_LABEL } from '../utils/status'

export function StatusLights({ history }: { history: string[] }) {
  const total = history.length
  return (
    <div className="flex flex-wrap gap-1 mt-2.5">
      {history.map((s, i) => {
        const label = STATUS_LABEL[s] ?? s
        const seq = total - i
        return (
          <div
            key={i}
            className={`w-2 h-2 rounded-full shrink-0 ${barCls(s)}`}
            title={`第 ${seq} 次 · ${label}`}
          />
        )
      })}
    </div>
  )
}
