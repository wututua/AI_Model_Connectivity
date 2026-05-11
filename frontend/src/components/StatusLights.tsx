import { barCls, STATUS_LABEL } from '../utils/status'

export function StatusLights({ history }: { history: string[] }) {
  const total = history.length
  return (
    <div className="flex flex-wrap gap-1 mt-2.5">
      {history.map((s, i) => {
        const colored = s === 'ok' || s === 'slow' || s === 'error'
        const glow = s === 'ok'
          ? '0 0 5px rgba(56,217,150,.8)'
          : s === 'slow'
          ? '0 0 5px rgba(246,196,83,.8)'
          : s === 'error'
          ? '0 0 5px rgba(255,107,122,.8)'
          : undefined
        const label = STATUS_LABEL[s] ?? s
        const seq = total - i
        return (
          <div
            key={i}
            className={`w-2 h-2 rounded-full shrink-0 ${barCls(s)}`}
            style={colored ? { boxShadow: glow } : undefined}
            title={`第 ${seq} 次 · ${label}`}
          />
        )
      })}
    </div>
  )
}
