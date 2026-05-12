export function relativeTime(dateStr: string): { text: string; stale: boolean } {
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return { text: dateStr, stale: false }
  const diffSec = (Date.now() - d.getTime()) / 1000
  let text: string
  if (diffSec < 60) text = '刚刚'
  else if (diffSec < 3600) text = `${Math.floor(diffSec / 60)} 分钟前`
  else if (diffSec < 86400) text = `${Math.floor(diffSec / 3600)} 小时前`
  else text = `${Math.floor(diffSec / 86400)} 天前`
  return { text, stale: diffSec > 600 }
}

export function statusClass(status: string) {
  return status === 'ok' ? 'ok' : status === 'slow' ? 'slow' : 'error'
}

export function barCls(s: string) {
  if (s === 'ok') return 'bar-ok'
  if (s === 'slow') return 'bar-slow'
  if (s === 'error') return 'bar-error'
  return 'bar-empty'
}

export const STATUS_LABEL: Record<string, string> = {
  ok: '正常',
  slow: '较慢',
  error: '异常',
  '': '无数据',
}
