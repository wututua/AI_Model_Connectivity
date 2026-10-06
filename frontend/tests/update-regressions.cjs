const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

const origin = 'http://127.0.0.1:18189'
const webRoot = path.resolve(__dirname, '../../web')

module.exports = async function updateRegressions(browser, artifacts) {
  for (const width of [320, 375, 1440]) {
    const context = await browser.newContext({ viewport: { width, height: width < 640 ? 812 : 960 } })
    const page = await context.newPage()
    page.setDefaultTimeout(10000)
    let role = 'admin'
    let starts = 0
    let statusFails = false
    let rejectStart = false
    let delayStatus = false
    let statusCalls = 0
    let resolves = 0
    let resolveFails = false
    let startMode = 'normal'
    const heldStarts = []
    let nextID = 0
    const requestID = () => (++nextID).toString(16).padStart(32, '0')
    let status = { request_id: requestID(), version: 'v1.0.0-beta.2', commit: 'fixture', platform: 'linux/amd64', deployment: 'systemd', supported: true, reason: '', job: null }
    const errors = []
    const checked = channel => ({
      channel, checked_at: new Date().toISOString(), available: true,
      release: { version: channel === 'stable' ? 'v1.0.0' : 'v1.1.0-rc.1', notes: '## 中文\n更新说明\n## English\nRelease notes\n<script>alert("unsafe")</script>',
        url: 'https://github.com/wututua/AI_Model_Connectivity/releases/tag/v1.0.0',
        published_at: new Date().toISOString(), prerelease: channel === 'preview', package_available: true },
    })
    await context.addInitScript(() => {
      const original = window.setTimeout.bind(window)
      window.setTimeout = (fn, delay, ...args) => original(fn, delay === 25000 && window.fastUpdateTimeout ? 300 : delay, ...args)
    })
    page.on('pageerror', error => errors.push(error.message))
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin)
      const respond = (json, code = 200) => route.fulfill({ json, status: code })
      if (url.pathname === '/api/auth/session') return respond({
        user: { id: 1, username: 'fixture-admin', role, enabled: true, must_change_password: false },
        csrf_token: 'fixture-csrf', expires_at: 9999999999, status_login_required: false,
      })
      if (url.pathname === '/api/admin/detection') return respond({ running: false, task_id: 0 })
      if (url.pathname === '/api/status') return respond({ providers: [], total: 0 })
      if (url.pathname === '/api/admin/updates') {
        assert.equal(role, 'admin')
        statusCalls++
        if (delayStatus) await new Promise(resolve => setTimeout(resolve, 3400))
        if (statusFails) return respond({ error: 'Service restarting' }, 503)
        return respond(status)
      }
      if (url.pathname === '/api/admin/updates/check') {
        assert.equal(route.request().method(), 'POST')
        assert.equal(route.request().headers()['x-csrf-token'], 'fixture-csrf')
        return respond(checked(route.request().postDataJSON().channel))
      }
      if (url.pathname === '/api/admin/updates/resolve') {
        assert.equal(role, 'admin')
        assert.equal(route.request().method(), 'POST')
        assert.equal(route.request().headers()['x-csrf-token'], 'fixture-csrf')
        const input = route.request().postDataJSON()
        assert.match(input.request_id, /^[0-9a-f]{32}$/)
        resolves++
        if (resolveFails) return respond({ error: 'Resolution unavailable' }, 503)
        if (input.request_id === status.request_id) status.request_id = requestID()
        return respond(status)
      }
      if (url.pathname === '/api/admin/updates/start') {
        assert.equal(route.request().method(), 'POST')
        assert.equal(route.request().headers()['x-csrf-token'], 'fixture-csrf')
        const input = route.request().postDataJSON()
        assert.equal(input.confirm, true)
        assert.equal(input.request_id, status.request_id)
        starts++
        if (rejectStart) return respond({ error: '检测正在运行' }, 409)
        if (startMode === 'unavailable') return respond({ error: 'Unavailable before enqueue' }, 503)
        if (startMode === 'hang-unqueued') { heldStarts.push({ route, input }); return }
        status.request_id = requestID()
        status.job = { id: input.request_id, version: input.version, channel: input.channel, status: 'running', stage: 'downloading',
          created_at: new Date().toISOString(), updated_at: new Date().toISOString(), message: '正在下载发布包' }
        if (startMode === 'other-target') {
          status.job = { ...status.job, version: 'v1.0.0', channel: 'stable' }
          return route.abort()
        }
        if (startMode === 'hang-queued') { heldStarts.push({ route, input }); return }
        if (startMode === 'lost-complete') {
          status.job = { ...status.job, status: 'succeeded', stage: 'complete', message: '更新完成' }
          status.version = input.version
          return route.abort()
        }
        return respond(status.job, 202)
      }
      if (url.pathname.startsWith('/api/')) throw new Error(`Unexpected API ${url.pathname}`)
      const asset = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/')
      const file = asset ? path.join(webRoot, decodeURIComponent(url.pathname)) : path.join(webRoot, 'index.html')
      assert.ok(file.startsWith(webRoot + path.sep))
      const contentType = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream'
      return route.fulfill({ contentType, body: await fs.readFile(file) })
    })
    try {
      await page.goto(origin + '/admin/updates')
      await page.getByText('v1.0.0-beta.2', { exact: true }).waitFor()
      if (width === 320) {
        delayStatus = true
        const before = statusCalls
        await page.getByRole('button', { name: '刷新更新状态', exact: true }).click()
        await page.waitForTimeout(3450)
        assert.equal(statusCalls - before, 1, 'slow status polls overlapped')
        delayStatus = false
      }
      await page.getByRole('button', { name: '检查更新', exact: true }).click()
      await page.getByRole('heading', { name: 'v1.0.0', exact: true }).waitFor()
      assert.equal(await page.locator('pre').innerText(), checked('stable').release.notes)
      await page.getByRole('button', { name: '安装更新', exact: true }).click()
      await page.getByRole('button', { name: '取消', exact: true }).click()
      assert.equal(starts, 0)
      await page.getByRole('combobox', { name: '更新通道', exact: true }).click()
      await page.getByRole('option', { name: '预发布（含 beta / RC）', exact: true }).click()
      assert.equal(await page.getByRole('heading', { name: 'v1.0.0', exact: true }).count(), 0)
      await page.getByRole('button', { name: '检查更新', exact: true }).click()
      await page.getByRole('heading', { name: 'v1.1.0-rc.1', exact: true }).waitFor()
      rejectStart = true
      await page.getByRole('button', { name: '安装更新', exact: true }).click()
      await page.getByRole('button', { name: '确认更新', exact: true }).click()
      await page.getByRole('alert').filter({ hasText: '更新请求被拒绝' }).waitFor()
      rejectStart = false
      await page.getByRole('button', { name: '安装更新', exact: true }).click()
      await page.getByRole('button', { name: '确认更新', exact: true }).evaluate(button => { button.click(); button.click() })
      await page.getByRole('status').filter({ hasText: '下载发布包' }).waitFor()
      assert.equal(starts, 2)
      assert.equal(await page.getByRole('button', { name: '安装更新', exact: true }).isDisabled(), true)
      statusFails = true
      await page.getByRole('button', { name: '刷新更新状态', exact: true }).click()
      await page.getByRole('alert').filter({ hasText: 'Service restarting' }).waitFor()
      assert.equal(await page.getByRole('button', { name: '安装更新', exact: true }).isDisabled(), true)
      statusFails = false
      status.job = { ...status.job, status: 'rolled_back', stage: 'restored', message: '更新失败，已恢复之前的程序、数据库和服务' }
      await page.getByRole('button', { name: '刷新更新状态', exact: true }).click()
      await page.getByRole('heading', { name: '已回滚', exact: true }).waitFor()
      for (const theme of ['light', 'dark']) {
        await page.evaluate(value => { document.body.dataset.theme = value }, theme)
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
        await page.screenshot({ path: path.join(artifacts, `updates-${theme}-${width}.png`), fullPage: true })
      }
      const install = page.getByRole('button', { name: '安装更新', exact: true })
      const refresh = page.getByRole('button', { name: '刷新更新状态', exact: true })
      const submit = async () => {
        await install.click()
        await page.getByRole('button', { name: '确认更新', exact: true }).click()
      }
      const resolvedText = page.getByText('提交未排队或已结束，旧请求已失效。', { exact: true })

      startMode = 'unavailable'
      const beforeFailure = starts
      const failedID = status.request_id
      await submit()
      await resolvedText.waitFor()
      assert.equal(await install.isDisabled(), false, '503 before enqueue did not recover')
      assert.notEqual(status.request_id, failedID, 'resolution did not invalidate the old request')
      assert.equal(starts, beforeFailure + 1, 'failed submission retried automatically')
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      await page.screenshot({ path: path.join(artifacts, `updates-recovery-${width}.png`), fullPage: true })

      resolveFails = true
      await submit()
      await page.getByRole('alert').filter({ hasText: 'Resolution unavailable' }).waitFor()
      assert.equal(await install.isDisabled(), true, 'uncertain request was unlocked without server confirmation')
      const beforeResolve = resolves
      const rechecked = page.waitForResponse(response => response.url().endsWith('/api/admin/updates/resolve'))
      await refresh.click()
      await rechecked
      assert.ok(resolves > beforeResolve, 'manual refresh did not resolve the uncertain request')
      assert.equal(starts, beforeFailure + 2)
      resolveFails = false
      await refresh.click()
      await resolvedText.waitFor()
      assert.equal(await install.isDisabled(), false)

      startMode = 'hang-queued'
      const beforeHanging = statusCalls
      const beforeQueued = starts
      await submit()
      await page.getByRole('heading', { name: '更新中', exact: true }).waitFor()
      await page.getByRole('alertdialog').waitFor({ state: 'hidden' })
      assert.ok(statusCalls > beforeHanging, 'submission blocked status polling')
      assert.equal(starts, beforeQueued + 1)
      assert.equal(await install.isDisabled(), true)
      status.job = { ...status.job, status: 'rolled_back', stage: 'restored', message: '已恢复' }
      await refresh.click()
      await page.getByRole('heading', { name: '已回滚', exact: true }).waitFor()

      startMode = 'hang-unqueued'
      await page.evaluate(() => { window.fastUpdateTimeout = true })
      const beforeTimeout = starts
      await submit()
      await resolvedText.waitFor()
      assert.equal(await install.isDisabled(), false, 'timed out, unqueued request stayed locked')
      assert.equal(starts, beforeTimeout + 1)
      const delayed = heldStarts[heldStarts.length - 1]
      assert.notEqual(delayed.input.request_id, status.request_id, 'late request could still enqueue')
      await delayed.route.fulfill({ status: 409, json: { error: 'Expired request' } }).catch(() => {})
      await page.evaluate(() => { window.fastUpdateTimeout = false })

      startMode = 'other-target'
      const beforeOther = starts
      await submit()
      await page.getByText('另一个更新请求已受理。', { exact: true }).waitFor()
      assert.equal(starts, beforeOther + 1)
      assert.equal(await install.isDisabled(), true)
      status.job = { ...status.job, status: 'rolled_back', stage: 'restored', message: '已恢复' }
      await refresh.click()
      await page.getByRole('heading', { name: '已回滚', exact: true }).waitFor()

      startMode = 'lost-complete'
      const beforeLostResponse = starts
      await submit()
      await page.getByRole('heading', { name: '更新成功', exact: true }).waitFor()
      assert.equal(starts, beforeLostResponse + 1, 'lost response caused a second installation')
      assert.equal(await page.getByRole('alert').filter({ hasText: '未能确认提交结果' }).count(), 0)
      await page.getByRole('button', { name: '检查更新', exact: true }).click()
      await page.getByRole('heading', { name: 'v1.1.0-rc.1', exact: true }).waitFor()
      assert.equal(await install.isDisabled(), true, 'installed version was offered again')

      status = { ...status, deployment: 'docker', supported: false, reason: 'Docker 部署请在宿主机更新镜像并重建容器。', job: null }
      await page.getByRole('button', { name: '刷新更新状态', exact: true }).click()
      await page.getByText(status.reason, { exact: true }).waitFor()
      assert.equal(await page.getByRole('button', { name: '安装更新', exact: true }).count(), 0)
      role = 'user'
      await page.reload()
      await page.getByRole('heading', { name: '运行概览', exact: true }).waitFor()
      assert.equal(await page.getByRole('button', { name: '检查更新', exact: true }).count(), 0)
      assert.deepEqual(errors, [])
    } catch (error) {
      await page.screenshot({ path: path.join(artifacts, `updates-failure-${width}.png`), fullPage: true })
      throw error
    } finally {
      for (const held of heldStarts) await held.route.abort().catch(() => {})
      await context.close()
    }
  }
  console.log('PASS update confirmation, CSRF, duplicate submits, channels, reconnect, rollback, deployment support, mobile and role protection')
  console.log('PASS update handoff recovery, pending POST polling, submission timeout, safe 503 resolution, lost responses and no automatic resubmission')
}
