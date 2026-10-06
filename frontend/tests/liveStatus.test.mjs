import assert from 'node:assert/strict'
import test from 'node:test'
import { fakeClock, loadUtility, settle } from './helpers.mjs'

const report = name => ({ generated_at: name, providers: [] })

function setup(overrides = {}) {
  const clock = fakeClock()
  const sources = []
  const reports = []
  const errors = []
  const live = []
  let fetches = 0
  const { startStatusUpdates } = loadUtility('liveStatus', clock.globals)
  const controller = startStatusUpdates({
    fetchReport: async () => report(`poll-${++fetches}`),
    onReport: value => reports.push(value),
    onError: error => errors.push(error),
    onLive: value => live.push(value),
    createSource: () => {
      const source = {
        closed: false,
        close() { this.closed = true },
        message(value) { this.onmessage({ data: JSON.stringify(value) }) },
        error() { this.onerror() },
      }
      sources.push(source)
      return source
    },
    ...overrides,
  })
  return { clock, sources, reports, errors, live, ...controller, get fetches() { return fetches } }
}

test('SSE failure resumes polling while reconnecting', async () => {
  const state = setup()
  await settle()
  state.sources[0].message(report('live'))
  await state.clock.advance(120_000)
  assert.equal(state.fetches, 1)
  state.sources[0].error()
  assert.equal(state.sources[0].closed, true)
  assert.equal(state.live.at(-1), false)
  await state.clock.advance(30_000)
  assert.equal(state.fetches, 2)
  assert.equal(state.sources.length, 2)
  state.sources[1].message(report('recovered'))
  await state.clock.advance(240_000)
  assert.equal(state.fetches, 2)
  assert.equal(state.live.at(-1), true)
  state.close()
  assert.equal(state.clock.pending, 0)
})

test('first-install SSE snapshots without timestamps remain live', async () => {
  const state = setup()
  await settle()
  for (const phase of ['unconfigured', 'pending']) {
    state.sources[0].message({ generated_at: '', providers: [], state: phase })
    assert.equal(state.live.at(-1), true)
    assert.equal(state.reports.at(-1).state, phase)
  }
  await state.clock.advance(120_000)
  assert.equal(state.fetches, 1)
  state.close()
})

test('poll-only mode backs off to 30, 60 and 120 seconds', async () => {
  const state = setup({ createSource: null })
  await state.clock.advance(30_000)
  assert.equal(state.fetches, 2)
  await state.clock.advance(59_999)
  assert.equal(state.fetches, 2)
  await state.clock.advance(1)
  assert.equal(state.fetches, 3)
  await state.clock.advance(120_000)
  assert.equal(state.fetches, 4)
  state.close()
})

test('older pending fetch cannot overwrite a newer SSE report', async () => {
  let resolve
  const state = setup({ fetchReport: () => new Promise(done => { resolve = done }) })
  state.sources[0].message(report('new'))
  resolve(report('old'))
  await settle()
  assert.deepEqual(state.reports.map(value => value.generated_at), ['new'])
  assert.equal(state.clock.pending, 0)
  state.close()
})

test('closing prevents callbacks and cancels reconnect and poll timers', async () => {
  let resolve
  const state = setup({ fetchReport: () => new Promise(done => { resolve = done }) })
  state.sources[0].error()
  state.close()
  resolve(report('late'))
  state.sources[0].message(report('queued'))
  await state.clock.advance(300_000)
  assert.equal(state.reports.length, 0)
  assert.equal(state.sources.length, 1)
  assert.equal(state.clock.pending, 0)
})

test('malformed events trigger fallback and stale sources cannot replace new data', async () => {
  const state = setup()
  await settle()
  state.sources[0].onmessage({ data: '{broken' })
  await state.clock.advance(30_000)
  state.sources[1].message(report('new'))
  state.sources[0].message(report('stale'))
  assert.equal(state.reports.at(-1).generated_at, 'new')
  assert.equal(state.sources[1].closed, false)
  state.close()
})

test('failed fetch reports an error but continues polling', async () => {
  let calls = 0
  const state = setup({
    createSource: null,
    fetchReport: async () => {
      if (++calls === 1) throw new Error('offline')
      return report('recovered')
    },
  })
  await settle()
  assert.equal(state.errors[0].message, 'offline')
  await state.clock.advance(30_000)
  assert.equal(state.reports.at(-1).generated_at, 'recovered')
  state.close()
})

test('slow fetches do not overlap during SSE reconnects', async () => {
  let calls = 0
  let resolve
  const state = setup({
    fetchReport: () => { calls++; return new Promise(done => { resolve = done }) },
  })
  state.sources[0].error()
  await state.clock.advance(90_000)
  assert.equal(calls, 1)
  resolve(report('done'))
  await settle()
  await state.clock.advance(30_000)
  assert.equal(calls, 2)
  state.close()
  resolve(report('ignored'))
  await settle()
})

for (const failure of [false, true]) {
  test(`manual refresh ${failure ? 'failure' : 'success'} cannot overwrite a same-timestamp SSE config update`, async () => {
    const requests = []
    const state = setup({
      fetchReport: () => new Promise((resolve, reject) => requests.push({ resolve, reject })),
    })
    requests[0].resolve({ ...report('same-time'), title: 'initial' })
    await settle()
    const refresh = state.refresh()
    state.sources[0].message({ ...report('same-time'), title: 'new config' })
    if (failure) requests[1].reject(new Error('stale failure'))
    else requests[1].resolve({ ...report('same-time'), title: 'old config' })
    await refresh
    assert.equal(state.reports.at(-1).title, 'new config')
    assert.equal(state.errors.length, 0)
    assert.equal(state.clock.pending, 0)
    state.close()
  })
}

test('manual requests supersede earlier requests and ignore callbacks after close', async () => {
  const requests = []
  const state = setup({
    createSource: null,
    fetchReport: () => new Promise((resolve, reject) => requests.push({ resolve, reject })),
  })
  const first = state.refresh()
  const second = state.refresh()
  requests[2].resolve(report('latest manual'))
  await second
  requests[1].resolve(report('older manual'))
  requests[0].reject(new Error('old polling error'))
  await first
  await settle()
  assert.deepEqual(state.reports.map(value => value.generated_at), ['latest manual'])
  assert.equal(state.errors.length, 0)
  assert.equal(state.clock.pending, 1)
  const pending = state.refresh()
  state.close()
  requests[3].reject(new Error('after close'))
  await pending
  await state.refresh()
  await state.clock.advance(300_000)
  assert.equal(requests.length, 4)
  assert.equal(state.errors.length, 0)
  assert.equal(state.clock.pending, 0)
})

test('polling does not overlap a slow manual refresh and resumes afterwards', async () => {
  const requests = []
  const state = setup({
    createSource: null,
    fetchReport: () => new Promise(resolve => requests.push(resolve)),
  })
  requests[0](report('initial'))
  await settle()
  const manual = state.refresh()
  await state.clock.advance(300_000)
  assert.equal(requests.length, 2)
  requests[1](report('manual'))
  await manual
  await state.clock.advance(60_000)
  assert.equal(requests.length, 3)
  state.close()
  requests[2](report('ignored'))
  await settle()
})
