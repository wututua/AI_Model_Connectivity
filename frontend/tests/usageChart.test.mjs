import assert from 'node:assert/strict'
import test from 'node:test'
import { loadUtility } from './helpers.mjs'

const { buildUsageChart, formatTokens, formatUsageDate } = loadUtility('usageChart')
const day = (date, total) => ({ day: date, total_tokens: total })

test('empty trends have no chart; single samples are centered and finite', () => {
  assert.equal(buildUsageChart([]), null)
  const chart = buildUsageChart([day('2026-10-01', 0)])
  assert.equal(chart.points[0].x, 50)
  assert.equal(chart.points[0].y, 92)
  assert.equal(chart.ceiling, 1)
  assert.ok(!chart.line.includes('NaN'))
})

test('trends sort dates without mutating input and keep points within the plot', () => {
  const daily = [day('2026-10-03', 14000), day('2026-10-01', 2000), day('2026-10-02', 7000)]
  const chart = buildUsageChart(daily)
  assert.equal(daily[0].day, '2026-10-03')
  assert.equal(chart.points[0].day, '2026-10-01')
  assert.equal(chart.points.at(-1).x, 98)
  assert.equal(chart.peak, 14000)
  assert.equal(chart.ceiling, 15000)
  assert.ok(chart.points.every(point => point.x >= 2 && point.x <= 98 && point.y >= 8 && point.y <= 92))
})

test('invalid or negative usage cannot produce a broken path', () => {
  const chart = buildUsageChart([day('2026-10-01', NaN), day('2026-10-02', -1), day('2026-10-03', Infinity)])
  assert.equal(chart.peak, 0)
  assert.ok(chart.points.every(point => point.y === 92))
})

test('usage labels format counts and dates with a fallback for invalid timestamps', () => {
  assert.equal(formatTokens(500), '500')
  assert.equal(formatTokens(1500), '1.5K')
  assert.equal(formatTokens(1250000), '1.25M')
  assert.match(formatUsageDate('2026-10-01', true), /2026/)
  assert.equal(formatUsageDate('invalid'), '暂无日期')
})
