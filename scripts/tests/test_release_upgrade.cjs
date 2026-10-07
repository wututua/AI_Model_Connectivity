// Black-box upgrade acceptance: only temporary databases and loopback upstreams.
const assert = require('node:assert/strict')
const { spawn, execFileSync } = require('node:child_process')
const { randomBytes } = require('node:crypto')
const fs = require('node:fs')
const http = require('node:http')
const net = require('node:net')
const os = require('node:os')
const path = require('node:path')
const { setTimeout: delay } = require('node:timers/promises')

async function main() {
  const [previousArg, candidateArg, webArg] = process.argv.slice(2)
  assert(previousArg && candidateArg, 'Usage: node scripts/tests/test_release_upgrade.cjs PREVIOUS_BINARY CANDIDATE_BINARY [CANDIDATE_WEB_DIR]')
  const previous = fs.realpathSync(previousArg)
  const candidate = fs.realpathSync(candidateArg)
  const candidateWeb = fs.realpathSync(webArg || path.join(path.dirname(candidate), 'web'))
  for (const binary of [previous, candidate]) {
    assert.match(execFileSync(binary, ['--version'], { encoding: 'utf8', windowsHide: true, timeout: 10000 }), /^model-connectivity /)
  }
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'cg-upgrade-'))
  const password = `Aa1${randomBytes(24).toString('hex')}`
  const changedPassword = `Bb2${randomBytes(24).toString('hex')}`
  const key = randomBytes(24).toString('hex')
  const calls = []
  let failure = false
  let child
  const session = { cookie: '', csrf: '' }
  const upstream = http.createServer(async (req, res) => {
    for await (const _ of req) { /* Drain bodies before sending the fixture. */ }
    calls.push({ path: req.url, authorized: [req.headers.authorization, req.headers['x-api-key'], req.headers['x-goog-api-key']].some(value => value === key || value === `Bearer ${key}`) })
    res.setHeader('X-Request-ID', 'upgrade-fixture')
    if (req.url === '/v1/chat/completions') {
      res.writeHead(failure ? 503 : 200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ choices: [{ message: { content: 'pang' } }], usage: { prompt_tokens: 2, completion_tokens: 3, total_tokens: 5 } }))
    } else if (req.url === '/v1/messages') {
      res.writeHead(200, { 'Content-Type': 'application/json' })
      res.end(JSON.stringify({ type: 'message', content: [{ type: 'text', text: 'pang' }], stop_reason: 'end_turn', usage: { input_tokens: 2, output_tokens: 3 } }))
    } else if (req.url === '/v1/models/fixture:streamGenerateContent?alt=sse') {
      res.writeHead(200, { 'Content-Type': 'text/event-stream' })
      res.end('data: {"candidates":[{"content":{"parts":[{"text":"pang"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3}}\n\n')
    } else {
      res.writeHead(404).end()
    }
  })
  try {
    await new Promise(resolve => upstream.listen(0, '127.0.0.1', resolve))
    const portProbe = net.createServer()
    await new Promise(resolve => portProbe.listen(0, '127.0.0.1', resolve))
    const port = portProbe.address().port
    await new Promise(resolve => portProbe.close(resolve))
    const base = `http://127.0.0.1:${port}`
    const request = async (method, route, body, status = 200, identity = session) => {
      const response = await fetch(base + route, {
        method, signal: AbortSignal.timeout(15000),
        headers: { 'Content-Type': 'application/json', Cookie: identity.cookie || '', 'X-CSRF-Token': identity.csrf || '' },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      const text = await response.text()
      assert.equal(response.status, status, `${method} ${route} returned unexpected status`)
      const cookie = response.headers.getSetCookie().find(value => value.startsWith('cg_session='))
      if (cookie) identity.cookie = cookie.split(';', 1)[0]
      const value = text ? JSON.parse(text) : null
      if (value?.csrf_token) identity.csrf = value.csrf_token
      return value
    }
    const start = async (binary, web) => {
      const env = Object.fromEntries(Object.entries(process.env).filter(([name]) =>
        /^(SystemRoot|WINDIR|PATH|TEMP|TMP|USERPROFILE|HOME|LOCALAPPDATA|APPDATA)$/i.test(name)))
      child = spawn(binary, ['serve'], {
        cwd: root, windowsHide: true, stdio: 'ignore',
        env: { ...env, APP_HOST: '127.0.0.1', APP_PORT: String(port), WEB_DIR: web, DATA_DIR: path.join(root, 'data'),
          ADMIN_USERNAME: 'admin', ADMIN_PASSWORD: password, STATUS_LOGIN_REQUIRED: 'true',
          AUTO_CHECK_RUN_ON_START: 'false', AUTO_CHECK_INTERVAL_MIN_HOURS: '0', AUTO_CHECK_INTERVAL_MAX_HOURS: '0' },
      })
      let spawnError
      child.on('error', error => { spawnError = error })
      const until = Date.now() + 30000
      while (Date.now() < until) {
        if (spawnError) throw spawnError
        assert.equal(child.exitCode, null, 'Service exited before becoming healthy')
        try {
          const response = await fetch(base + '/health', { signal: AbortSignal.timeout(1000) })
          if (response.ok) { await response.arrayBuffer(); return }
          await response.arrayBuffer()
        } catch { /* Wait for the isolated service to listen. */ }
        await delay(100)
      }
      throw new Error('Service health deadline exceeded')
    }
    const stop = async () => {
      if (!child || child.exitCode !== null || child.signalCode !== null) return
      const stopped = new Promise(resolve => child.once('exit', resolve))
      child.kill()
      await stopped
      child = null
    }
    const check = async expectedErrors => {
      const { task } = await request('POST', '/api/admin/check', undefined, 202)
      const until = Date.now() + 20000
      while (Date.now() < until) {
        const value = await request('GET', `/api/admin/tasks/${task.id}`)
        if (value.status !== 'running') {
          assert.equal(value.status, 'success', 'Check task failed to persist')
          assert.equal(value.error_count, expectedErrors)
          assert.equal(value.total, 1)
          return
        }
        await delay(50)
      }
      throw new Error('Check task deadline exceeded')
    }
    const billing = async () => {
      const { range_start, range_end, ...value } = await request('GET', '/api/admin/billing')
      return value
    }
    await start(previous, path.join(path.dirname(previous), 'web'))
    await request('POST', '/api/auth/login', { username: 'admin', password })
    await request('POST', '/api/auth/password', { current_password: password, password: changedPassword })
    const config = await request('GET', '/api/admin/config')
    await request('PUT', '/api/admin/settings', { ...config.settings, dashboard_title: 'Upgrade fixture', daily_request_limit: 20 })
    let provider = await request('POST', '/api/admin/providers', {
      id: 'upgrade', name: 'Upgrade fixture', type: 'gemini', base_url: `http://127.0.0.1:${upstream.address().port}/v1`,
      api_key: key, models: ['fixture'], enabled: true, probe_enabled: true,
    })
    const monitoring = await request('GET', '/api/admin/monitoring')
    await request('PUT', '/api/admin/monitoring/settings', {
      ...monitoring.settings, backup_keep: 3, monthly_budget: 10,
      prices: [{ provider_id: 'upgrade', model: 'fixture', input_per_million: 2, output_per_million: 3 }],
    })
    await check(0)
    failure = true
    await check(1)
    failure = false
    const backup = await request('POST', '/api/admin/monitoring/backup', undefined, 201)
    const before = {
      config: await request('GET', '/api/admin/config'), status: await request('GET', '/api/status'),
      billing: await billing(), monitoring: await request('GET', '/api/admin/monitoring'),
      tasks: await request('GET', '/api/admin/tasks'), users: await request('GET', '/api/admin/users'),
    }
    assert.equal(before.billing.total_tokens, 10)
    assert.equal(before.monitoring.diagnostics.length, 2)
    assert.equal(before.monitoring.incidents.length, 1)
    await stop()
    console.log('PASS previous release: real login, password change, configuration, probes, usage, incident and backup')

    await start(candidate, candidateWeb)
    assert.equal(calls.length, 2, 'Upgrade triggered an unsolicited upstream call')
    assert.deepEqual(await request('GET', '/api/admin/config'), before.config)
    assert.deepEqual(await request('GET', '/api/status'), before.status)
    assert.deepEqual(await billing(), before.billing)
    assert.deepEqual(await request('GET', '/api/admin/tasks'), before.tasks)
    assert.deepEqual(await request('GET', '/api/admin/users'), before.users)
    assert.deepEqual((await request('GET', '/api/admin/monitoring')).settings, before.monitoring.settings)
    await request('POST', '/api/admin/monitoring/verify', { name: backup.name })
    await request('GET', '/api/admin/audit', undefined, 401, {})
    await request('GET', '/api/status', undefined, 401, {})
    const history = await request('GET', '/api/admin/monitoring/diagnostics?provider_id=upgrade&limit=1')
    assert.equal(history.items.length, 1)
    assert.equal(history.has_more, true)
    const next = await request('GET', `/api/admin/monitoring/diagnostics?provider_id=upgrade&limit=1&before=${history.next_before}`)
    assert.equal(next.items.length, 1)
    assert(next.items[0].id < history.items[0].id)
    assert.equal(next.has_more, false)
    assert.deepEqual((await request('GET', '/api/admin/monitoring/incidents?provider_id=upgrade')).items, before.monitoring.incidents)
    console.log('PASS upgrade: preserved session/CSRF, settings, history, billing, tasks, users, backup and private status')

    for (const protocol of ['anthropic', 'gemini']) {
      const { api_key_set, connection_revision, ...draft } = provider
      provider = await request('PUT', '/api/admin/providers/upgrade', {
        ...draft, probe: { ...draft.probe, protocol, max_tokens: 32, token_limit_field: '', omit_temperature: true, stream: protocol === 'gemini' },
      })
      assert.equal(provider.api_key_set, true)
      assert.equal((await request('GET', '/api/status')).providers[0].status, 'unknown')
      await check(0)
    }
    assert.equal(calls.length, 4)
    assert(calls.every(call => call.authorized), 'Stored key was not preserved')
    assert.deepEqual(calls.map(call => call.path), ['/v1/chat/completions', '/v1/chat/completions', '/v1/messages', '/v1/models/fixture:streamGenerateContent?alt=sse'])
    assert.equal((await billing()).total_tokens, 20)
    const audit = await request('GET', '/api/admin/audit?action=providers.update')
    assert.equal(audit.items.length, 2)
    assert(audit.items.every(event => event.actor === 'admin' && event.result === 'success'))
    assert(!JSON.stringify(await request('GET', '/api/admin/audit')).includes(key), 'Credential leaked into audit')
    const newBackup = await request('POST', '/api/admin/monitoring/backup', undefined, 201)
    await request('POST', '/api/admin/monitoring/verify', { name: newBackup.name })
    await stop()
    await start(candidate, candidateWeb)
    assert.equal((await billing()).total_tokens, 20)
    assert.equal((await request('GET', '/api/admin/audit?action=providers.update')).items.length, 2)
    assert.equal((await request('GET', '/api/admin/monitoring/diagnostics?provider_id=upgrade')).items.length, 4)
    assert.equal(calls.length, 4)
    await stop()
    console.log('PASS candidate: native JSON/SSE, explicit protocol compatibility, pagination, audit, backup and restart persistence')
  } finally {
    if (child?.pid && child.exitCode === null && child.signalCode === null) {
      const stopped = new Promise(resolve => child.once('exit', resolve))
      child.kill()
      await stopped
    }
    upstream.closeAllConnections()
    await new Promise(resolve => upstream.close(resolve))
    const resolved = fs.realpathSync(root)
    assert.equal(path.dirname(resolved), fs.realpathSync(os.tmpdir()))
    assert(path.basename(resolved).startsWith('cg-upgrade-'))
    fs.rmSync(resolved, { recursive: true, force: true })
  }
}

main().catch(error => {
  // Assertion details and child logs could contain credentials; do not print them.
  console.error(`FAIL release upgrade acceptance: ${error.code || error.name}`)
  console.error(error.stack?.split('\n').find(line => line.includes('test_release_upgrade.cjs:'))?.trim() || '')
  process.exitCode = 1
})
