export type StatusTone = 'ok' | 'slow' | 'error'

export function relativeTime(dateStr: string): { text: string; stale: boolean } {
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return { text: dateStr, stale: false }
  const diffSec = (Date.now() - date.getTime()) / 1000
  if (diffSec < 60) return { text: '刚刚', stale: false }
  if (diffSec < 3600) return { text: `${Math.floor(diffSec / 60)} 分钟前`, stale: diffSec > 600 }
  if (diffSec < 86400) return { text: `${Math.floor(diffSec / 3600)} 小时前`, stale: true }
  return { text: `${Math.floor(diffSec / 86400)} 天前`, stale: true }
}

export function statusClass(status: string): StatusTone {
  if (status === 'ok' || status === 'success' || status === 'running') return 'ok'
  if (status === 'slow' || status === 'paused') return 'slow'
  return 'error'
}

export function statusDotClass(status: string) {
  const tone = statusClass(status)
  if (tone === 'ok') return 'status-dot-ok'
  if (tone === 'slow') return 'status-dot-slow'
  return 'status-dot-error'
}

export function barCls(status: string) {
  if (status === 'ok') return 'history-ok'
  if (status === 'slow') return 'history-slow'
  if (status === 'error') return 'history-error'
  return ''
}

export const STATUS_LABEL: Record<string, string> = {
  ok: '正常', success: '成功', slow: '较慢', error: '异常',
  running: '运行中', paused: '已暂停', canceled: '已取消', '': '无数据',
}
