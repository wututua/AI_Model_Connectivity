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
  let bulkHistory = false
  let failNextHistory = false
  let delayedHistory = null
  const historyRequests = []
  const releases = []
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
    await context.addInitScript(() => { localStorage.setItem('theme', 'light'); window.EventSource = undefined })
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin)
      const respond = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
      if (url.pathname === '/api/auth/session') return respond({ user: { id: 1, username: 'test-admin', role: 'admin', enabled: true, must_change_password: false }, csrf_token: 'fixture-csrf', status_login_required: true, expires_at: 9999999999 })
      if (url.pathname === '/api/admin/providers') return respond([provider])
      if (['/api/admin/monitoring/diagnostics', '/api/admin/monitoring/incidents'].includes(url.pathname)) {
        assert.equal(route.request().method(), 'GET')
        const query = Object.fromEntries(url.searchParams)
        historyRequests.push(query)
        if (query.provider_id === 'delayed' && delayedHistory) {
          const pending = delayedHistory
          delayedHistory = null
          pending.started()
          await pending.wait
          return respond({ error: 'stale history failure' }, 500)
        }
        if (failNextHistory) { failNextHistory = false; return respond({ error: 'history unavailable' }, 503) }
        const incidents = url.pathname.endsWith('/incidents')
        const initial = incidents ? fixture.incidents : fixture.diagnostics
        const source = bulkHistory ? Array.from({ length: 125 }, (_, index) => ({ ...initial[0], id: index + 1, model: `history-${index + 1}` })) : initial
        const matches = source.filter(row => (!query.provider_id || row.provider_id === query.provider_id) &&
          (!query.model || row.model === query.model) && (!query.status || row.status === query.status) &&
          (!query.before || row.id < Number(query.before))).sort((a, b) => b.id - a.id)
        const limit = Number(query.limit || 50)
        const items = matches.slice(0, limit), has_more = matches.length > limit
        return respond({ items, has_more, next_before: has_more ? items.at(-1).id : 0 })
      }
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
    for (const [name, width, height] of [['desktop', 1440, 960], ['mobile', 390, 844], ['narrow', 320, 812]]) {
      await page.setViewportSize({ width, height })
      for (const tab of ['诊断', '模型变更', '备份', '告警规则', '事件', '调度', '费用']) {
        await page.getByRole('tab', { name: tab, exact: true }).click()
        if (tab === '诊断' || tab === '事件') {
          await page.getByRole('region', { name: `${tab}历史` }).locator('tbody tr').first().waitFor()
        }
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
    bulkHistory = true
    await page.getByRole('tab', { name: '诊断', exact: true }).click()
    const history = page.getByRole('region', { name: '诊断历史' })
    await history.getByText('history-125', { exact: true }).waitFor()
    await history.getByRole('combobox', { name: '每页记录数' }).click()
    await page.getByRole('option', { name: '25', exact: true }).click()
    await history.getByRole('button', { name: '筛选', exact: true }).click()
    await history.getByText('第 1 页 · 25 条', { exact: true }).waitFor()
    await history.getByRole('button', { name: '下一页', exact: true }).click()
    await history.getByText('history-100', { exact: true }).waitFor()
    assert.equal(historyRequests.at(-1).before, '101')
    assert.equal(await history.getByText('history-125', { exact: true }).count(), 0)
    await history.getByRole('button', { name: '上一页', exact: true }).click()
    await history.getByText('history-125', { exact: true }).waitFor()
    await history.getByLabel('模型', { exact: true }).fill('history-12')
    await history.getByRole('button', { name: '筛选', exact: true }).click()
    await history.getByText('第 1 页 · 1 条', { exact: true }).waitFor()
    assert.equal(historyRequests.at(-1).before, undefined)
    await history.getByRole('button', { name: '重置筛选', exact: true }).click()
    await history.getByText('第 1 页 · 50 条', { exact: true }).waitFor()
    const pending = {}
    pending.ready = new Promise(resolve => { pending.started = resolve })
    pending.wait = new Promise(resolve => { pending.release = resolve })
    releases.push(pending.release)
    delayedHistory = pending
    await history.getByLabel('Provider ID', { exact: true }).fill('delayed')
    await history.getByRole('button', { name: '筛选', exact: true }).click()
    await pending.ready
    await history.getByLabel('Provider ID', { exact: true }).fill('production')
    await history.getByRole('button', { name: '筛选', exact: true }).click()
    await history.getByText('history-125', { exact: true }).waitFor()
    pending.release()
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    assert.equal(await history.getByText('stale history failure', { exact: true }).count(), 0)
    failNextHistory = true
    await history.getByRole('button', { name: '刷新历史' }).click()
    await history.getByText('history unavailable', { exact: true }).waitFor()
    assert.equal(await history.locator('tbody tr').count(), 0, 'failed refresh kept stale history visible')
    await history.getByRole('button', { name: '刷新历史' }).click()
    await history.getByText('history-125', { exact: true }).waitFor()
    await page.getByRole('tab', { name: '事件', exact: true }).click()
    await page.getByRole('region', { name: '事件历史' }).getByText('history-125', { exact: true }).waitFor()
    await page.getByRole('tab', { name: '诊断', exact: true }).click()
    assert.equal(await history.getByLabel('Provider ID', { exact: true }).inputValue(), 'production')
    await history.getByText('history-125', { exact: true }).waitFor()
    await page.screenshot({ path: path.join(artifacts, 'narrow-filtered-history.png'), fullPage: true })
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1))
    assert.deepEqual(errors, [])
    console.log(`PASS monitoring desktop/mobile, settings, secret redaction, catalog approval, incidents, backup verification; screenshots: ${artifacts}`)
  } finally { releases.forEach(release => release()); await browser.close() }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
