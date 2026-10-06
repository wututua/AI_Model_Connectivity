const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

const origin = 'http://127.0.0.1:18188'
const webRoot = path.resolve(__dirname, '../../web')

function delivery(id, status = 'success', kind = 'alert') {
  return {
    id, status, kind, retry_of: 0, platform: 'webhook',
    created_at: '2026-10-06T12:00:00Z', finished_at: '2026-10-06T12:00:01Z',
    elapsed_ms: 120, http_status: status === 'error' ? 503 : 200,
    summary: `Fixture notification ${id}`,
    error_message: status === 'error' ? '通知平台返回 HTTP 503' : '',
  }
}

module.exports = async function notificationRegressions(browser, artifacts) {
  for (const width of [320, 375, 1440]) {
    const context = await browser.newContext({ viewport: { width, height: width < 640 ? 812 : 960 } })
    const errors = []
    let role = 'admin'
    let items = Array.from({ length: 21 }, (_, index) => delivery(21 - index, index === 0 ? 'error' : 'success'))
    let testCalls = 0
    let retryCalls = 0
    let loadFails = false
    let testFails = false
    let holdList = false
    let listHeld
    let releaseList
    const listStarted = new Promise(resolve => { listHeld = resolve })
    const listGate = new Promise(resolve => { releaseList = resolve })
    let releaseTest
    let testStarted
    const testReady = new Promise(resolve => { testStarted = resolve })
    const testGate = new Promise(resolve => { releaseTest = resolve })
    const page = await context.newPage()
    page.setDefaultTimeout(10000)
    page.on('pageerror', error => errors.push(error.message))
    await context.addInitScript(() => {
      window.EventSource = undefined
      localStorage.setItem('theme', 'light')
    })
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin, 'Unexpected outbound request')
      const respond = (json, status = 200) => route.fulfill({ json, status })
      if (url.pathname === '/api/auth/session') return respond({
        user: { id: 1, username: 'fixture-admin', role, enabled: true, must_change_password: false },
        csrf_token: 'fixture-csrf', expires_at: 9999999999, status_login_required: false,
      })
      if (url.pathname === '/api/admin/detection') return respond({ running: false, task_id: 0 })
      if (url.pathname === '/api/status') return respond({ providers: [], total: 0 })
      if (url.pathname === '/api/admin/notifications') {
        assert.equal(role, 'admin')
        if (loadFails) return respond({ error: 'History unavailable' }, 503)
        const status = url.searchParams.get('status')
        const offset = Number(url.searchParams.get('offset') || 0)
        const filtered = items.filter(item => !status || item.status === status).slice(offset, offset + 20)
        if (holdList && !status) {
          holdList = false
          listHeld()
          await listGate
          return respond([delivery(999)]).catch(() => {})
        }
        return respond(filtered)
      }
      if (url.pathname === '/api/admin/notifications/test') {
        assert.equal(route.request().method(), 'POST')
        assert.equal(route.request().headers()['x-csrf-token'], 'fixture-csrf')
        testCalls++
        if (testFails) return respond({ error: '通知未启用或已保存的渠道配置不完整' }, 400)
        testStarted()
        await testGate
        const value = delivery(22, 'error', 'test')
        items.unshift(value)
        return respond(value)
      }
      if (url.pathname === '/api/admin/notifications/22/retry') {
        assert.equal(route.request().method(), 'POST')
        assert.equal(route.request().headers()['x-csrf-token'], 'fixture-csrf')
        retryCalls++
        const value = { ...delivery(23, 'success', 'retry'), retry_of: 22 }
        items.unshift(value)
        return respond(value)
      }
      if (url.pathname.startsWith('/api/')) throw new Error(`Unexpected API: ${url.pathname}`)
      const asset = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/')
      const file = asset ? path.join(webRoot, decodeURIComponent(url.pathname)) : path.join(webRoot, 'index.html')
      assert.ok(file.startsWith(webRoot + path.sep))
      const contentType = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream'
      return route.fulfill({ contentType, body: await fs.readFile(file) })
    })
    try {
      await page.goto(origin + '/admin/notifications')
      await page.getByRole('heading', { name: '通知记录', exact: true }).waitFor()
      await page.getByText('显示 1–20 条', { exact: true }).waitFor()
      await page.getByRole('button', { name: '下一页', exact: true }).click()
      await page.getByText('显示 21–21 条', { exact: true }).waitFor()
      await page.getByRole('button', { name: '上一页', exact: true }).click()
      await page.getByText('显示 1–20 条', { exact: true }).waitFor()

      const send = page.getByRole('button', { name: '发送测试通知', exact: true })
      await send.evaluate(button => { button.click(); button.click() })
      await testReady
      assert.equal(testCalls, 1, 'double click sent multiple test notifications')
      assert.equal(await send.isDisabled(), true)
      releaseTest()
      await page.getByRole('alert').filter({ hasText: '通知 #22 发送失败' }).waitFor()
      assert.equal(await page.getByText('通知 #22 已被平台接受', { exact: true }).count(), 0)

      const retry = page.getByRole('button', { name: '重试通知 22', exact: true })
      await retry.click()
      await page.getByRole('alertdialog').waitFor()
      await page.getByRole('button', { name: '取消', exact: true }).click()
      assert.equal(retryCalls, 0, 'cancel sent a retry')
      await retry.click()
      await page.getByRole('button', { name: '确认重试', exact: true }).click()
      await page.getByText('通知 #23 已被平台接受', { exact: true }).waitFor()
      assert.equal(retryCalls, 1)
      assert.equal(items.find(item => item.id === 22).status, 'error')
      assert.equal(await page.getByRole('button', { name: '重试通知 23', exact: true }).count(), 0)

      holdList = true
      await page.getByRole('button', { name: '刷新通知记录', exact: true }).click()
      await listStarted
      await page.getByRole('combobox', { name: '通知状态', exact: true }).click()
      await page.getByRole('option', { name: '发送失败', exact: true }).click()
      await page.getByText('显示 1–2 条', { exact: true }).waitFor()
      releaseList()
      await page.waitForTimeout(100)
      assert.equal(await page.getByText('Fixture notification 999', { exact: true }).count(), 0)
      assert.equal(await page.getByRole('button', { name: '重试通知 22', exact: true }).count(), 1)

      loadFails = true
      await page.getByRole('button', { name: '刷新通知记录', exact: true }).click()
      await page.getByRole('alert').filter({ hasText: 'History unavailable' }).waitFor()
      assert.equal(await page.getByText('暂无通知记录', { exact: true }).count(), 0)
      loadFails = false
      await page.getByRole('button', { name: '刷新通知记录', exact: true }).click()
      await page.getByText('显示 1–2 条', { exact: true }).waitFor()

      testFails = true
      await send.click()
      await page.getByRole('alert').filter({ hasText: '通知未启用' }).waitFor()
      await page.getByText('显示 1–20 条', { exact: true }).waitFor()
      for (const theme of ['light', 'dark']) {
        await page.evaluate(value => { document.body.dataset.theme = value }, theme)
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
        await page.screenshot({ path: path.join(artifacts, `notifications-${theme}-${width}.png`), fullPage: true, animations: 'disabled' })
      }
      items = []
      await page.getByRole('button', { name: '刷新通知记录', exact: true }).click()
      await page.getByText('暂无通知记录', { exact: true }).waitFor()

      role = 'user'
      await page.reload()
      await page.getByRole('heading', { name: '运行概览', exact: true }).waitFor()
      assert.equal(await page.getByRole('button', { name: '发送测试通知', exact: true }).count(), 0)
      assert.equal(await page.getByRole('button', { name: '通知记录', exact: true }).count(), 0)
      assert.deepEqual(errors, [])
    } catch (error) {
      console.error(await page.locator('body').innerText())
      await page.screenshot({ path: path.join(artifacts, `notifications-failure-${width}.png`), fullPage: true })
      throw error
    } finally {
      releaseList()
      releaseTest()
      await context.close()
    }
  }
  console.log('PASS notification test/retry, CSRF headers, history pagination/filter races, errors, mobile layout and admin-only access')
}
