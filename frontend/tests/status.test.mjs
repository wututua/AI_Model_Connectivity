import assert from 'node:assert/strict'
import test from 'node:test'
import { loadUtility } from './helpers.mjs'

const { relativeTime, detectionSuccessRate, barCls, statusDotClass, STATUS_LABEL, reportPresentation } = loadUtility('status')

test('slow responses count as successful detections, empty reports have no percentage', () => {
  assert.equal(detectionSuccessRate({ total: 4, ok_count: 1, slow_count: 3 }), 100)
  assert.equal(detectionSuccessRate({ total: 3, ok_count: 1, slow_count: 1 }), 67)
  assert.equal(detectionSuccessRate({ total: 0, ok_count: 0, slow_count: 0 }), null)
})

test('relative ages advance and staleness respects the schedule', () => {
  const date = '2026-10-03T00:00:00Z'
  const now = Date.parse(date)
  assert.notEqual(relativeTime(date, now).text, relativeTime(date, now + 120_000).text)
  assert.equal(relativeTime(date, now + 601_000).stale, true)
  assert.equal(relativeTime(date, now + 3_600_000, 43_200).stale, false)
  assert.equal(relativeTime('2026-10-03T08:00:00+08:00', now).stale, false)
})

test('invalid, ambiguous legacy and future timestamps are not presented as fresh', () => {
  const now = Date.parse('2026-10-03T00:00:00Z')
  for (const date of ['', 'invalid', '2026-10-03 00:00:00', '2026-10-03T01:00:00Z']) {
    assert.equal(relativeTime(date, now).stale, true, date)
  }
})

test('unknown history is distinct from empty history', () => {
  assert.notEqual(barCls('unknown'), barCls('empty'))
  assert.ok(STATUS_LABEL.unknown)
})

test('unknown status dots are neutral and distinct from slow responses', () => {
  assert.equal(statusDotClass('unknown'), 'status-dot-unknown')
  assert.notEqual(statusDotClass('unknown'), statusDotClass('slow'))
})

test('report headings distinguish unknown models, real failures, slow responses and empty reports', () => {
  const base = { total: 3, error_count: 0, slow_count: 0, unknown_count: 0, provider_errors: [] }
  assert.equal(reportPresentation(base).status, 'ok')
  assert.equal(reportPresentation({ ...base, providers: [{ status: 'unknown' }] }).headline, '部分 Provider 尚未检测')
  assert.equal(reportPresentation({ ...base, unknown_count: 3 }).headline, '所有模型尚未检测')
  assert.equal(reportPresentation({ ...base, unknown_count: 1 }).headline, '部分模型尚未检测')
  assert.equal(reportPresentation({ ...base, unknown_count: 1, error_count: 1 }).status, 'error')
  assert.equal(reportPresentation({ ...base, slow_count: 1 }).status, 'slow')
  assert.equal(reportPresentation({ ...base, total: 0 }).headline, '暂无检测结果')
  assert.equal(reportPresentation({ ...base, total: 0, provider_errors: [{}] }).headline, '部分 Provider 请求失败')
})
