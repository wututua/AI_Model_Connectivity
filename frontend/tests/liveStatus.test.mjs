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
  const close = startStatusUpdates({
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
  return { clock, sources, reports, errors, live, close, get fetches() { return fetches } }
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
