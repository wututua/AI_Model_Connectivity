import test from 'node:test'
import assert from 'node:assert/strict'
import { loadUtility } from './helpers.mjs'

test('dashboard preferences persist view and sort independently of transient filters', () => {
  const store = new Map()
  const localStorage = { getItem: key => store.get(key), setItem: (key, value) => store.set(key, value) }
  const { readDashboardPreferences, saveDashboardPreferences } = loadUtility('dashboardPreferences', { localStorage })
  assert.equal(readDashboardPreferences().viewMode, 'detailed')
  saveDashboardPreferences({ sortBy: 'latency', viewMode: 'compact' })
  assert.equal(readDashboardPreferences().viewMode, 'compact')
  assert.equal(readDashboardPreferences().sortBy, 'latency')
})

test('invalid or unavailable preference storage falls back safely', () => {
  for (const value of ['bad JSON', 'null', '[]', '{"sortBy":"invalid","viewMode":"invalid"}']) {
    const { readDashboardPreferences } = loadUtility('dashboardPreferences', { localStorage: { getItem: () => value } })
    assert.equal(readDashboardPreferences().viewMode, 'detailed')
    assert.equal(readDashboardPreferences().sortBy, 'default')
  }
  const { readDashboardPreferences, saveDashboardPreferences } = loadUtility('dashboardPreferences', {
    localStorage: { getItem() { throw new Error('denied') }, setItem() { throw new Error('denied') } },
  })
  assert.equal(readDashboardPreferences().sortBy, 'default')
  assert.doesNotThrow(() => saveDashboardPreferences({ sortBy: 'name', viewMode: 'compact' }))
})
