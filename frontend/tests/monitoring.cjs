const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')
const { chromium } = require('playwright')

const origin = 'http://127.0.0.1:18083'
const webRoot = path.resolve(__dirname, '../../web')
const artifacts = path.resolve(__dirname, '../../dist/monitoring-browser')
const now = new Date().toISOString()
const provider = { id: 'production', name: 'Production', models: ['model-a'], enabled: true, probe_enabled: true }
const fixture = {
  settings: { version: 1, backup_interval_hours: 0, backup_keep: 7, monthly_budget: 5, rules: [], schedules: [], prices: [] },
  catalogs: [{ provider_id: 'production', revision: 'one', models: ['model-a', 'model-b'], approved: ['model-a'], added: ['model-b'], removed: [], updated_at: now }],
  catalog_events: [{ id: 1, provider_id: 'production', added: ['model-b'], removed: ['old-model'], created_at: now }],
  backups: [{ name: 'cg-20261007T010000-fixture.sqlite', size: 81920, sha256: 'a'.repeat(64), created_at: now, verified_at: now }],
  incidents: [{ id: 1, provider_id: 'production', model: 'model-a', revision: 'one', status: 'open', opened_at: now, last_seen_at: now, resolved_at: '', acknowledged_at: '', note: '' }],
  diagnostics: [{ id: 1, provider_id: 'production', model: 'model-a', status: 'error', error_type: 'rate_limit', checked_at: now, latency_ms: 320, first_token_ms: 0, capability: 'text', capability_status: 'request_failed', diagnostics: { dns_ms: 5, connect_ms: 12, tls_ms: 25, first_byte_ms: 310, http_status: 429, request_id: 'fixture-request', retry_after: '30' } }],
  events: [{ id: 1, kind: 'backup', detail: 'scheduled backup completed', created_at: now }],
  schedules: [{ provider_id: 'production', next_at: now, interval_minutes: 5 }],
  cost: { month: '2026-10', estimated_usd: 0.123, budget_usd: 5, budget_exceeded: false, unknown_probes: 2, items: [{ provider_id: 'production', model: 'model-a', estimated_usd: 0.123, priced_probes: 10, unknown_probes: 2 }] },
}

async function main() {
  await fs.mkdir(artifacts, { recursive: true })
  const browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome', headless: true })
  const errors = []
  let saves = 0
  let approvals = 0
  let acks = 0
  let verifies = 0
  let failNextRead = false
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
    await context.addInitScript(() => { localStorage.setItem('theme', 'light'); window.EventSource = undefined })
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin)
      const respond = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
      if (url.pathname === '/api/auth/session') return respond({ user: { id: 1, username: 'test-admin', role: 'admin', enabled: true, must_change_password: false }, csrf_token: 'fixture-csrf', status_login_required: true, expires_at: 9999999999 })
      if (url.pathname === '/api/admin/providers') return respond([provider])
      if (url.pathname === '/api/admin/monitoring') {
        if (failNextRead) { failNextRead = false; return respond({ error: 'fixture refresh failure' }, 503) }
        return respond(fixture)
      }
      if (url.pathname.startsWith('/api/admin/monitoring/')) {
        assert.equal(route.request().headers()['x-csrf-token'], 'fixture-csrf')
        const body = route.request().postDataJSON()
        if (url.pathname.endsWith('/settings')) {
          assert.equal(body.version, fixture.settings.version)
          fixture.settings = { ...body, version: body.version + 1 }
          fixture.settings.rules = fixture.settings.rules.map(rule => ({ ...rule, url: undefined, token: undefined, chat_id: undefined, credentials_set: !!rule.url || rule.credentials_set }))
          saves++
          return respond(fixture.settings)
        }
        if (url.pathname.endsWith('/approve')) {
          assert.equal(body.revision, 'one')
          approvals++
          fixture.catalogs[0].approved = fixture.catalogs[0].models
          fixture.catalogs[0].added = []
          return respond({ ok: true })
        }
        if (url.pathname.endsWith('/ack')) {
          assert.equal(body.id, 1)
          acks++
          fixture.incidents[0].note = body.note
          fixture.incidents[0].acknowledged_at = now
          return respond({ ok: true })
        }
        if (url.pathname.endsWith('/verify')) { verifies++; return respond({ ok: true }) }
        if (url.pathname.endsWith('/test-rule')) return respond({ ok: true })
        throw new Error(`Unexpected mutation: ${url.pathname}`)
      }
      if (url.pathname.startsWith('/api/')) throw new Error(`Unexpected API: ${url.pathname}`)
      const file = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/') ? path.join(webRoot, url.pathname) : path.join(webRoot, 'index.html')
      assert.ok(path.resolve(file).startsWith(webRoot + path.sep))
      return route.fulfill({ contentType: ({ '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' })[path.extname(file)] || 'application/octet-stream', body: await fs.readFile(file) })
    })
    const page = await context.newPage()
    page.on('pageerror', error => errors.push(error.message))
    page.setDefaultTimeout(10000)
    await page.goto(`${origin}/admin/monitoring`)
    await page.getByText('配置版本 1', { exact: true }).waitFor()
    for (const [name, width, height] of [['desktop', 1440, 960], ['mobile', 390, 844]]) {
      await page.setViewportSize({ width, height })
      for (const tab of ['诊断', '模型变更', '备份', '告警规则', '事件', '调度', '费用']) {
        await page.getByRole('tab', { name: tab, exact: true }).click()
        await page.screenshot({ path: path.join(artifacts, `${name}-${tab}.png`), fullPage: true, animations: 'disabled' })
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${name} ${tab} overflow`)
      }
    }
    await page.getByRole('tab', { name: '备份', exact: true }).click()
    await page.getByRole('button', { name: '校验 cg-20261007T010000-fixture.sqlite' }).click()
    await page.getByText('操作完成', { exact: true }).waitFor()
    assert.equal(verifies, 1)
    await page.getByLabel('保留数量', { exact: true }).fill('3')
    assert.equal(await page.getByRole('button', { name: '刷新监控中心' }).isDisabled(), true)
    await page.getByRole('button', { name: '保存配置', exact: true }).click()
    await page.getByText('配置版本 2', { exact: true }).waitFor()
    await page.getByRole('tab', { name: '告警规则', exact: true }).click()
    await page.getByRole('button', { name: '添加规则', exact: true }).click()
    await page.getByLabel('规则名称', { exact: true }).fill('Production alert')
    await page.getByLabel('Webhook URL', { exact: true }).fill('https://example.invalid/secret')
    await page.getByRole('switch', { name: '模型清单变更提醒' }).click()
    await page.getByRole('button', { name: '保存配置', exact: true }).click()
    await page.getByText('配置版本 3', { exact: true }).waitFor()
    assert.equal(await page.getByLabel('Webhook URL', { exact: true }).inputValue(), '')
    await page.screenshot({ path: path.join(artifacts, 'mobile-rule-form.png'), fullPage: true })
    await page.getByRole('tab', { name: '模型变更', exact: true }).click()
    await page.getByRole('button', { name: '批准新增模型（1）' }).click()
    await page.getByRole('button', { name: '批准', exact: true }).click()
    await page.getByRole('button', { name: '批准新增模型（0）' }).waitFor()
    await page.getByRole('tab', { name: '事件', exact: true }).click()
    await page.getByRole('button', { name: '接手 / 备注', exact: true }).click()
    await page.getByLabel('处理备注').fill('Checking the upstream rate limit.')
    await page.getByRole('button', { name: '确认', exact: true }).click()
    await page.getByText('Checking the upstream rate limit.', { exact: true }).waitFor()
    assert.equal(saves, 2)
    assert.equal(approvals, 1)
    assert.equal(acks, 1)
    await page.getByRole('tab', { name: '备份', exact: true }).click()
    await page.getByLabel('保留数量', { exact: true }).fill('4')
    failNextRead = true
    await page.getByRole('button', { name: '保存配置', exact: true }).click()
    await page.getByText('配置版本 4', { exact: true }).waitFor()
    await page.getByText('错误：fixture refresh failure', { exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: '保存配置', exact: true }).isDisabled(), true)
    assert.equal(await page.getByRole('button', { name: '刷新监控中心' }).isEnabled(), true)
    await page.getByRole('button', { name: '刷新监控中心' }).click()
    await page.getByText('已刷新', { exact: true }).waitFor()
    await page.getByLabel('保留数量', { exact: true }).fill('5')
    await page.getByRole('button', { name: '放弃更改并刷新' }).click()
    await page.getByRole('button', { name: '放弃并刷新', exact: true }).click()
    await page.getByText('已刷新', { exact: true }).waitFor()
    assert.equal(await page.getByLabel('保留数量', { exact: true }).inputValue(), '4')
    assert.deepEqual(errors, [])
    console.log(`PASS monitoring desktop/mobile, settings, secret redaction, catalog approval, incidents, backup verification; screenshots: ${artifacts}`)
  } finally { await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
