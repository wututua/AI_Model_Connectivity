const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

const webRoot = path.resolve(__dirname, '../../web')
const origin = 'http://127.0.0.1:18185'
const initial = {
  title: 'Initial snapshot', state: 'ready', generated_at: new Date().toISOString(),
  elapsed_ms: 100, global_concurrency: 1, provider_concurrency: 1,
  total: 0, ok_count: 0, slow_count: 0, error_count: 0, unknown_count: 0,
  provider_count: 0, providers: [], provider_errors: [],
  overall_status: 'OPERATIONAL', overall_class: 'ok', stale_after_seconds: 600,
}

async function routeApp(context, status) {
  await context.route('**/*', async route => {
    const url = new URL(route.request().url())
    assert.equal(url.origin, origin)
    if (url.pathname === '/api/auth/session') {
      return route.fulfill({ json: { user: null, csrf_token: '', expires_at: 0, status_login_required: false } })
    }
    if (url.pathname === '/api/status') return status(route)
    assert.ok(!url.pathname.startsWith('/api/'), `Unexpected API: ${url.pathname}`)
    const asset = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/')
    const file = asset ? path.join(webRoot, decodeURIComponent(url.pathname)) : path.join(webRoot, 'index.html')
    assert.ok(file.startsWith(webRoot + path.sep))
    const contentType = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream'
    return route.fulfill({ contentType, body: await fs.readFile(file) })
  })
}

module.exports = async function statusRegressions(browser, artifacts) {
  const errors = []
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
  let release
  try {
    await context.addInitScript(() => {
      window.EventSource = class {
        constructor() { window.testSource = this }
        addEventListener() {}
        close() {}
      }
    })
    let held = false
    let failure = false
    let started
    await routeApp(context, async route => {
      if (held) {
        started()
        await new Promise(resolve => { release = resolve })
      }
      return failure
        ? route.fulfill({ status: 503, json: { error: 'stale refresh error' } })
        : route.fulfill({ json: initial })
    })
    const page = await context.newPage()
    page.on('pageerror', error => errors.push(error.message))
    await page.goto(origin)
    await page.locator('header').getByText(initial.title, { exact: true }).waitFor()
    for (const fail of [false, true]) {
      held = true
      failure = fail
      const ready = new Promise(resolve => { started = resolve })
      await page.getByRole('button', { name: '刷新状态', exact: true }).click()
      await ready
      const newer = { ...initial, title: `Same timestamp config ${fail}` }
      await page.evaluate(report => window.testSource.onmessage({ data: JSON.stringify(report) }), newer)
      await page.locator('header').getByText(newer.title, { exact: true }).waitFor()
      release()
      await page.waitForFunction(() => !document.querySelector('button[aria-label="刷新状态"]').disabled)
      assert.equal(await page.locator('header').getByText(newer.title, { exact: true }).count(), 1)
      assert.equal(await page.getByText('stale refresh error', { exact: true }).count(), 0)
    }
    await page.screenshot({ path: path.join(artifacts, 'manual-refresh-sse.png'), fullPage: true })
  } finally {
    release?.()
    await context.close()
  }

  for (const storageMode of ['getter', 'read', 'write', 'invalid']) {
    const blocked = await browser.newContext({ viewport: { width: 375, height: 812 }, reducedMotion: 'reduce', colorScheme: 'light' })
    try {
      await blocked.addInitScript(mode => {
        window.EventSource = undefined
        if (mode === 'getter') {
          Object.defineProperty(window, 'localStorage', { get() { throw new DOMException('Storage blocked', 'SecurityError') } })
        } else if (mode === 'invalid') {
          localStorage.setItem('theme', 'not-a-theme')
        } else {
          Storage.prototype[mode === 'read' ? 'getItem' : 'setItem'] = () => { throw new DOMException('Storage blocked', 'SecurityError') }
        }
      }, storageMode)
      await routeApp(blocked, route => route.fulfill({ json: initial }))
      const page = await blocked.newPage()
      page.on('pageerror', error => errors.push(error.message))
      await page.goto(origin)
      await page.locator('header').getByText(initial.title, { exact: true }).waitFor()
      assert.equal(await page.locator('body').getAttribute('data-theme'), 'dark')
      await page.getByRole('button', { name: '切换为浅色主题', exact: true }).click()
      assert.equal(await page.locator('body').getAttribute('data-theme'), 'light')
      await page.getByRole('link', { name: '登录账户', exact: true }).click()
      await page.locator('#username').waitFor()
      assert.equal(await page.getByRole('button', { name: '切换为跟随系统', exact: true }).count(), 1)
      assert.equal(await page.locator('body').getAttribute('data-theme'), 'light')
      await page.getByRole('button', { name: '切换为跟随系统', exact: true }).click()
      await page.emulateMedia({ colorScheme: 'dark' })
      await page.waitForFunction(() => document.body.dataset.theme === 'dark')
      await page.emulateMedia({ colorScheme: 'light' })
      await page.waitForFunction(() => document.body.dataset.theme === 'light')
      await page.getByRole('link', { name: '状态页', exact: true }).click()
      await page.locator('header').waitFor()
      await page.getByRole('button', { name: '切换为深色主题', exact: true }).click()
      assert.equal(await page.locator('body').getAttribute('data-theme'), 'dark')
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      await page.screenshot({ path: path.join(artifacts, `theme-storage-${storageMode}.png`), fullPage: true })
    } finally {
      await blocked.close()
    }
  }
  assert.deepEqual(errors, [])
  console.log('PASS manual refresh/SSE ordering, blocked storage, invalid theme, navigation persistence and system theme changes')
}
