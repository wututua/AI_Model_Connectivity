export type StatusTone = 'ok' | 'slow' | 'error'

export function reportPresentation(report: { total: number; error_count: number; slow_count: number; unknown_count?: number; provider_errors?: unknown[]; providers?: { status: string }[] }) {
  if (report.error_count > 0) return { status: 'error', label: '异常', headline: '检测到服务异常' }
  if (report.provider_errors?.length) return { status: 'error', label: '请求失败', headline: '部分 Provider 请求失败' }
  if (report.total === 0) return { status: 'unknown', label: '暂无数据', headline: '暂无检测结果' }
  if ((report.unknown_count ?? 0) > 0) return {
    status: 'unknown', label: '未检测',
    headline: report.unknown_count === report.total ? '所有模型尚未检测' : '部分模型尚未检测',
  }
  if (report.providers?.some(provider => provider.status === 'unknown')) return { status: 'unknown', label: '未检测', headline: '部分 Provider 尚未检测' }
  if (report.slow_count > 0) return { status: 'slow', label: '较慢', headline: '部分服务响应较慢' }
  return { status: 'ok', label: '正常', headline: '所有服务运行正常' }
}

export function relativeTime(dateStr: string, now = Date.now(), staleAfterSeconds = 600): { text: string; stale: boolean } {
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime()) || !/(Z|[+-]\d{2}:\d{2})$/i.test(dateStr)) return { text: dateStr || '无时间记录', stale: true }
  const diffSec = (now - date.getTime()) / 1000
  const stale = diffSec > staleAfterSeconds || diffSec < -60
  if (diffSec < 60) return { text: '刚刚', stale }
  if (diffSec < 3600) return { text: `${Math.floor(diffSec / 60)} 分钟前`, stale }
  if (diffSec < 86400) return { text: `${Math.floor(diffSec / 3600)} 小时前`, stale }
  return { text: `${Math.floor(diffSec / 86400)} 天前`, stale }
}

export function detectionSuccessRate(report: { total: number; ok_count: number; slow_count: number }): number | null {
  return report.total > 0 ? Math.round(((report.ok_count + report.slow_count) / report.total) * 100) : null
}

export function statusClass(status: string): StatusTone {
  if (status === 'ok' || status === 'success' || status === 'running') return 'ok'
  if (status === 'slow' || status === 'paused' || status === 'unknown') return 'slow'
  return 'error'
}

export function statusDotClass(status: string) {
  if (status === 'unknown') return 'status-dot-unknown'
  const tone = statusClass(status)
  if (tone === 'ok') return 'status-dot-ok'
  if (tone === 'slow') return 'status-dot-slow'
  return 'status-dot-error'
}

export function barCls(status: string) {
  if (status === 'ok') return 'history-ok'
  if (status === 'slow') return 'history-slow'
  if (status === 'error') return 'history-error'
  if (status === 'unknown') return 'history-unknown'
  return ''
}

export const STATUS_LABEL: Record<string, string> = {
  ok: '正常', success: '成功', slow: '较慢', error: '异常',
  running: '运行中', paused: '已暂停', canceled: '已取消', unknown: '未检测', '': '无数据',
}
