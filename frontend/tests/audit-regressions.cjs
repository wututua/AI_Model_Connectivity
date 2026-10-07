const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

module.exports = async function auditRegressions(browser, artifacts) {
  const origin = 'http://127.0.0.1:18194'
  const webRoot = path.resolve(__dirname, '../../web')
  for (const width of [320, 1440]) {
    const context = await browser.newContext({ viewport: { width, height: width === 320 ? 812 : 960 } })
    const page = await context.newPage()
    page.setDefaultTimeout(12000)
    const errors = [], queries = [], releases = []
    let role = 'admin', delay = null, failNext = false
    const events = Array.from({ length: 75 }, (_, index) => ({
      id: 75 - index, actor_id: 1, actor: 'fixture-admin', role: 'admin', action: index === 0 ? 'updates.start' : 'providers.update',
      result: index === 0 ? 'accepted' : 'success', http_status: index === 0 ? 202 : 200, created_at: new Date().toISOString(),
    }))
    await context.addInitScript(() => { window.EventSource = undefined; localStorage.setItem('theme', 'light') })
    page.on('pageerror', error => errors.push(error.message))
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin)
      const respond = (json, status = 200) => route.fulfill({ json, status })
      if (url.pathname === '/api/auth/session') return respond({ user: { id: 1, username: 'fixture-admin', role, enabled: true, must_change_password: false }, csrf_token: 'audit-csrf', expires_at: 9999999999 })
      if (url.pathname === '/api/admin/audit') {
        assert.equal(role, 'admin', 'ordinary user requested audit')
        assert.equal(route.request().method(), 'GET')
        const query = Object.fromEntries(url.searchParams)
        queries.push(query)
        if (query.actor === 'delayed' && delay) {
          const pending = delay
          delay = null
          pending.started()
          await pending.wait
          return respond({ error: 'late audit failure' }, 500)
        }
        if (failNext) { failNext = false; return respond({ error: 'audit unavailable' }, 503) }
        const matches = events.filter(row => (!query.actor || row.actor === query.actor) &&
          (!query.result || row.result === query.result) && (!query.action || row.action === query.action) &&
          (!query.before || row.id < Number(query.before)))
        const limit = Number(query.limit || 50), items = matches.slice(0, limit), has_more = matches.length > limit
        return respond({ items, has_more, next_before: has_more ? items.at(-1).id : 0, actions: ['auth.login', 'providers.update', 'updates.start'] })
      }
      if (url.pathname === '/api/admin/config') return respond({ settings: {}, providers: [] })
      if (url.pathname === '/api/admin/detection') return respond({ running: false })
      if (url.pathname === '/api/status') return respond({ providers: [], total: 0, ok_count: 0, slow_count: 0, error_count: 0 })
      if (url.pathname === '/api/admin/budget') return respond({ used: 0, limit: 0, remaining: 0, exhausted: false })
      if (url.pathname.startsWith('/api/')) throw new Error(`Unexpected API: ${url.pathname}`)
      const file = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/') ? path.join(webRoot, url.pathname) : path.join(webRoot, 'index.html')
      assert.ok(file.startsWith(webRoot + path.sep))
      return route.fulfill({ contentType: { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream', body: await fs.readFile(file) })
    })
    try {
      await page.goto(origin + '/admin/audit')
      const audit = page.getByRole('region', { name: '操作审计记录' })
      await audit.getByText('第 1 页 · 50 条', { exact: true }).waitFor()
      await audit.getByRole('cell', { name: '已受理', exact: true }).waitFor()
      await audit.getByRole('button', { name: '下一页', exact: true }).click()
      await audit.getByText('第 2 页 · 25 条', { exact: true }).waitFor()
      assert.equal(queries.at(-1).before, '26')
      assert.equal(await audit.getByRole('button', { name: '下一页', exact: true }).isDisabled(), true)
      await audit.getByRole('combobox', { name: '结果', exact: true }).click()
      await page.getByRole('option', { name: '已受理', exact: true }).click()
      await audit.getByRole('button', { name: '筛选', exact: true }).click()
      await audit.getByText('第 1 页 · 1 条', { exact: true }).waitFor()
      assert.equal(queries.at(-1).result, 'accepted')
      assert.equal(queries.at(-1).before, undefined)
      for (const theme of ['light', 'dark']) {
        await page.evaluate(theme => { document.body.dataset.theme = theme; window.scrollTo(0, 0) }, theme)
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
        await page.screenshot({ path: path.join(artifacts, `audit-${theme}-${width}.png`), fullPage: true, animations: 'disabled' })
      }
      await audit.getByRole('button', { name: '重置筛选', exact: true }).click()
      await audit.getByText('第 1 页 · 50 条', { exact: true }).waitFor()
      const pending = {}
      pending.ready = new Promise(resolve => { pending.started = resolve })
      pending.wait = new Promise(resolve => { pending.release = resolve })
      releases.push(pending.release)
      delay = pending
      await audit.getByLabel('操作者', { exact: true }).fill('delayed')
      await audit.getByRole('button', { name: '筛选', exact: true }).click()
      await pending.ready
      await audit.getByLabel('操作者', { exact: true }).fill('fixture-admin')
      await audit.getByRole('button', { name: '筛选', exact: true }).click()
      await audit.getByText('第 1 页 · 50 条', { exact: true }).waitFor()
      pending.release()
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
      assert.equal(await audit.getByText('late audit failure', { exact: true }).count(), 0)
      failNext = true
      await audit.getByRole('button', { name: '刷新审计记录' }).click()
      await audit.getByText('audit unavailable', { exact: true }).waitFor()
      assert.equal(await audit.locator('tbody tr').count(), 0)
      await audit.getByRole('button', { name: '刷新审计记录' }).click()
      await audit.getByText('第 1 页 · 50 条', { exact: true }).waitFor()
      await audit.getByLabel('操作者', { exact: true }).fill('missing')
      await audit.getByRole('button', { name: '筛选', exact: true }).click()
      await audit.getByText('暂无审计记录', { exact: true }).waitFor()
      role = 'user'
      const reads = queries.length
      await page.goto(origin + '/admin/audit')
      await page.getByRole('heading', { name: '运行概览', exact: true }).waitFor()
      assert.equal(await page.getByRole('region', { name: '操作审计记录' }).count(), 0)
      assert.equal(queries.length, reads)
      assert.deepEqual(errors, [])
      console.log(`PASS audit filters, cursor, cancellation, errors, accepted status, permissions and viewport ${width}`)
    } finally { releases.forEach(release => release()); await context.close() }
  }
}
