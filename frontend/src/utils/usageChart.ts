import type { BillingDaily } from '../types'

export function buildUsageChart(daily: BillingDaily[]) {
  if (!daily.length) return null
  const sorted = [...daily].sort((left, right) => left.day.localeCompare(right.day))
  const values = sorted.map(day => Math.max(0, Number.isFinite(day.total_tokens) ? day.total_tokens : 0))
  const peak = Math.max(...values)
  const step = 10 ** Math.floor(Math.log10(Math.max(peak, 1))) / 2
  const ceiling = Math.max(1, Math.ceil(peak / step) * step)
  const points = sorted.map((day, index) => ({
    day: day.day,
    value: values[index],
    x: sorted.length === 1 ? 50 : 2 + index / (sorted.length - 1) * 96,
    y: 92 - values[index] / ceiling * 84,
  }))
  const line = `M ${points.map(point => `${point.x},${point.y}`).join(' L ')}`
  const area = `${line} L ${points[points.length - 1].x},92 L ${points[0].x},92 Z`
  return { points, line, area, peak, ceiling }
}

export function formatTokens(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return String(value)
}

export function formatUsageDate(value: string, includeYear = false) {
  const date = new Date(value.length === 10 ? `${value}T00:00:00` : value)
  if (Number.isNaN(date.getTime())) return '暂无日期'
  return date.toLocaleDateString('zh-CN', { year: includeYear ? 'numeric' : undefined, month: '2-digit', day: '2-digit' })
}
