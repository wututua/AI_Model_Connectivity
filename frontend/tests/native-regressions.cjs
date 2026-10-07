const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

module.exports = async function nativeRegressions(browser, artifacts) {
  const origin = 'http://127.0.0.1:18193'
  const webRoot = path.resolve(__dirname, '../../web')
  for (const width of [320, 1440]) {
    const context = await browser.newContext({ viewport: { width, height: width < 640 ? 812 : 960 } })
    const page = await context.newPage()
    page.setDefaultTimeout(12000)
    const errors = [], discoveries = [], saves = []
    let delayed = null, release = () => {}
    page.on('pageerror', error => errors.push(error.message))
    await context.addInitScript(() => { window.EventSource = undefined; localStorage.setItem('theme', 'light') })
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin)
      const respond = json => route.fulfill({ json })
      if (url.pathname === '/api/auth/session') return respond({ user: { id: 1, username: 'native-admin', role: 'admin', enabled: true, must_change_password: false }, csrf_token: 'native-csrf', expires_at: 9999999999 })
      if (url.pathname === '/api/admin/providers') {
        if (route.request().method() === 'GET') return respond([])
        assert.equal(route.request().headers()['x-csrf-token'], 'native-csrf')
        saves.push(route.request().postDataJSON())
        return respond(saves.at(-1))
      }
      if (url.pathname === '/api/admin/detection') return respond({ running: false })
      if (url.pathname === '/api/admin/provider-models') {
        const query = route.request().postDataJSON()
        assert.equal(route.request().headers()['x-csrf-token'], 'native-csrf')
        discoveries.push(query)
        if (delayed) {
          const wait = delayed
          delayed = null
          await wait
        }
        return respond([`model-${query.protocol}`])
      }
      if (url.pathname.startsWith('/api/')) throw new Error(`Unexpected API: ${url.pathname}`)
      const file = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/') ? path.join(webRoot, url.pathname) : path.join(webRoot, 'index.html')
      assert.ok(file.startsWith(webRoot + path.sep))
      return route.fulfill({ contentType: { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream', body: await fs.readFile(file) })
    })
    const select = async (name, option) => {
      await page.getByRole('combobox', { name, exact: true }).click()
      await page.getByRole('option', { name: option, exact: true }).click()
    }
    try {
      await page.goto(origin + '/admin/providers')
      await page.getByRole('button', { name: '新增 Provider', exact: true }).click()
      await page.getByLabel('唯一 ID', { exact: true }).fill('native')
      await page.getByLabel('显示名称', { exact: true }).fill('Native provider')
      await page.getByLabel('Base URL', { exact: true }).fill('https://example.invalid/v1')
      await select('兼容预设', 'Chat 推理参数')
      await select('探测协议', 'Anthropic Messages')
      assert.equal(await page.getByRole('combobox', { name: 'Token 参数', exact: true }).count(), 0)
      assert.equal(await page.getByLabel('Temperature', { exact: true }).getAttribute('max'), '1')
      assert.equal(await page.getByLabel('Base URL', { exact: true }).inputValue(), 'https://example.invalid/v1')
      // Dispatch Escape in the registration window before the parent layer rerenders.
      await page.evaluate(() => {
        window.escapeDuringRegistration = false
        const onLayerUpdate = () => {
          const listbox = document.querySelector('[role="listbox"]')
          if (!listbox) return
          document.removeEventListener('dismissableLayer.update', onLayerUpdate)
          window.escapeDuringRegistration = true
          listbox.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
        }
        document.addEventListener('dismissableLayer.update', onLayerUpdate)
      })
      await page.getByRole('combobox', { name: '检测能力', exact: true }).click()
      assert.equal(await page.evaluate(() => window.escapeDuringRegistration), true, 'nested layer registration was not exercised')
      await page.getByRole('dialog').waitFor({ state: 'visible' })
      if (!await page.getByRole('listbox').count()) {
        await page.getByRole('combobox', { name: '检测能力', exact: true }).click()
      }
      assert.equal(await page.getByRole('option', { name: '工具调用结构', exact: true }).getAttribute('aria-disabled'), 'true')
      await page.waitForFunction(() => document.querySelector('[role="listbox"]')?.contains(document.activeElement))
      await page.keyboard.press('Escape')
      await page.getByRole('listbox').waitFor({ state: 'hidden' })
      await page.getByRole('dialog').waitFor({ state: 'visible' })
      assert.equal(discoveries.length, 0, 'protocol selection made automatic requests')
      delayed = new Promise(resolve => { release = resolve })
      await Promise.all([
        page.waitForRequest(request => new URL(request.url()).pathname === '/api/admin/provider-models'),
        page.getByRole('button', { name: '同步模型', exact: true }).click(),
      ])
      await select('探测协议', 'Gemini generateContent')
      release()
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
      assert.equal(await page.getByText('model-anthropic', { exact: true }).count(), 0, 'stale discovery changed selection')
      await page.getByRole('button', { name: '同步模型', exact: true }).click()
      await page.getByText('已同步 1 个模型', { exact: true }).waitFor()
      assert.equal(discoveries.at(-1).protocol, 'gemini')
      assert.equal(await page.getByLabel('Base URL', { exact: true }).getAttribute('placeholder'), 'https://generativelanguage.googleapis.com/v1beta')
      await page.getByRole('switch', { name: '流式探测', exact: true }).click()
      assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth))
      await page.screenshot({ path: path.join(artifacts, `native-provider-${width}.png`), fullPage: true })
      await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
      await page.getByRole('dialog').waitFor({ state: 'hidden' })
      assert.equal(saves[0].probe.protocol, 'gemini')
      assert.equal(saves[0].probe.token_limit_field, 'max_tokens')
      assert.equal(saves[0].probe.stream, true)
      assert.deepEqual(saves[0].models, ['model-gemini'])
      assert.deepEqual(errors, [])
      console.log(`PASS native protocol configuration, discovery cancellation, payload and viewport ${width}`)
    } catch (error) {
      console.error(`Native regression failure at viewport ${width}:`, await page.locator('body').innerText().catch(() => '(unavailable)'))
      await page.screenshot({ path: path.join(artifacts, `native-failure-${width}.png`), fullPage: true }).catch(() => {})
      throw error
    } finally { release(); await context.close() }
  }
}
