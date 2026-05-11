import { statusClass } from '../utils/status'

export function StatusPill({ status, label, large }: { status: string; label: string; large?: boolean }) {
  const sz = large ? 'px-4 py-2.5 text-base font-black' : 'px-3 py-1.5 text-xs font-bold'
  return (
    <span
      className={`inline-flex items-center rounded-full border whitespace-nowrap font-mono ${sz} badge-${statusClass(status)}`}
    >
      {label}
    </span>
  )
}
