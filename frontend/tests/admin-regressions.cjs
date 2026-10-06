const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

const origin = 'http://127.0.0.1:18186'
const webRoot = path.resolve(__dirname, '../../web')
const settings = {
  dashboard_title: 'Regression fixture', timeout_seconds: 30, model_list_timeout_seconds: 20,
  slow_threshold_ms: 800, concurrency: 1, provider_concurrency: 1, max_models_per_provider: 0,
  skip_models: [], enable_history: true, show_curve_chart: true, stats_window_days: 7,
  history_size: 30, max_history_records: 500, show_error_detail: true, theme_mode: 'auto',
  day_mode_start_hour: 8, day_mode_end_hour: 18, auto_check_interval_min_hours: 0,
  auto_check_interval_max_hours: 0, notify_platform: 'disabled', notify_providers: [],
  notify_models: [], status_login_required: false,
}
const idle = { running: false, task_id: 0, kind: '', provider_id: '', auto_check_interval_min_hours: 0, auto_check_interval_max_hours: 0 }

async function createContext(browser, width, api, errors) {
  const context = await browser.newContext({ viewport: { width, height: width < 640 ? 812 : 960 } })
  await context.addInitScript(() => {
    window.EventSource = undefined
    localStorage.setItem('theme', 'light')
  })
  await context.route('**/*', async route => {
    const url = new URL(route.request().url())
    assert.equal(url.origin, origin, 'Unexpected external request')
    if (url.pathname === '/api/auth/session') {
      return route.fulfill({ json: {
        user: { id: 1, username: 'regression-admin', role: 'admin', enabled: true, must_change_password: false },
        csrf_token: 'fixture', expires_at: 9999999999, status_login_required: false,
      } })
    }
    if (url.pathname.startsWith('/api/')) return api(route, url)
    const asset = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/')
    const file = asset ? path.join(webRoot, decodeURIComponent(url.pathname)) : path.join(webRoot, 'index.html')
    assert.ok(file.startsWith(webRoot + path.sep))
    const contentType = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream'
    return route.fulfill({ contentType, body: await fs.readFile(file) })
  })
  const page = await context.newPage()
  page.on('pageerror', error => errors.push(error.message))
  page.setDefaultTimeout(10000)
  return { context, page }
}

async function pendingModels(browser, artifacts, width, errors) {
  let providers = []
  let failSave = false
  const writes = []
  const { context, page } = await createContext(browser, width, async (route, url) => {
    if (url.pathname === '/api/admin/detection') return route.fulfill({ json: idle })
    if (url.pathname === '/api/admin/provider-models') return route.fulfill({ json: ['upstream-model', 'pending-model'] })
    if (url.pathname === '/api/admin/providers' && route.request().method() === 'GET') return route.fulfill({ json: providers })
    assert.ok(['/api/admin/providers', '/api/admin/providers/fixture'].includes(url.pathname), `Unexpected API: ${url.pathname}`)
    assert.ok(['POST', 'PUT'].includes(route.request().method()))
    const value = route.request().postDataJSON()
    writes.push(value)
    if (failSave) return route.fulfill({ status: 503, json: { error: 'Retry fixture' } })
    providers = [{ ...value, api_key: undefined, api_key_set: false }]
    return route.fulfill({ json: providers[0] })
  }, errors)
  const save = async () => {
    await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
    await page.locator('#provider-id').waitFor({ state: 'detached' })
  }
  const edit = async () => {
    if (width < 640) {
      await page.getByRole('button', { name: 'Fixture 更多操作', exact: true }).click()
      await page.getByRole('button', { name: '编辑 Provider', exact: true }).click()
    } else {
      await page.getByRole('button', { name: '编辑 Fixture', exact: true }).click()
    }
    await page.locator('#provider-id').waitFor()
    await page.getByRole('button', { name: '手动添加', exact: true }).click()
    await page.getByRole('textbox', { name: '手动添加模型', exact: true }).waitFor()
  }
  try {
    await page.goto(origin + '/admin/providers')
    await page.getByRole('button', { name: '新增 Provider', exact: true }).click()
    await page.locator('#provider-id').fill('fixture')
    await page.locator('#provider-name').fill('Fixture')
    await page.locator('#provider-url').fill('https://example.invalid/v1')
    await page.getByRole('button', { name: '手动添加', exact: true }).click()
    await page.getByRole('textbox', { name: '手动添加模型', exact: true }).fill('pending-model')
    await save()
    assert.deepEqual(writes.at(-1).models, ['pending-model'], 'Save lost the uncommitted model')

    await edit()
    await page.getByRole('textbox', { name: '手动添加模型', exact: true }).fill('retry-model')
    failSave = true
    await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
    await page.getByText('Retry fixture', { exact: true }).waitFor()
    assert.deepEqual(writes.at(-1).models, ['pending-model', 'retry-model'])
    await page.getByRole('list', { name: '已选模型', exact: true }).getByText('retry-model', { exact: true }).waitFor()
    await page.screenshot({ path: path.join(artifacts, `pending-model-${width}.png`), fullPage: true })
    failSave = false
    await save()
    assert.deepEqual(writes.at(-1).models, ['pending-model', 'retry-model'])

    await edit()
    const input = page.getByRole('textbox', { name: '手动添加模型', exact: true })
    await input.fill('manual-after-sync')
    await page.getByRole('button', { name: '同步模型', exact: true }).click()
    await page.getByText('已同步 2 个模型', { exact: true }).waitFor()
    await save()
    assert.deepEqual(writes.at(-1).models, ['pending-model', 'retry-model', 'upstream-model', 'manual-after-sync'])

    await edit()
    await input.fill('prefix-')
    await input.evaluate(element => {
      element.setSelectionRange(element.value.length, element.value.length)
      const clipboard = new DataTransfer()
      clipboard.setData('text/plain', 'one,two')
      element.dispatchEvent(new ClipboardEvent('paste', { clipboardData: clipboard, bubbles: true, cancelable: true }))
    })
    await save()
    assert.deepEqual(writes.at(-1).models.slice(-2), ['prefix-one', 'two'])

    await edit()
    await input.fill('discarded-draft')
    await page.getByRole('button', { name: '清除所有模型', exact: true }).click()
    await save()
    assert.deepEqual(writes.at(-1).models, [], 'Explicit clear must still enable automatic discovery')

    await edit()
    await input.fill('模型')
    await input.evaluate(element => element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true })))
    assert.equal(await input.inputValue(), '模型')
    await save()
    assert.deepEqual(writes.at(-1).models, ['模型'])
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
  } catch (error) {
    await page.screenshot({ path: path.join(artifacts, `pending-model-failure-${width}.png`), fullPage: true })
    throw error
  } finally {
    await context.close()
  }
}

async function overviewOrdering(browser, artifacts, heldEndpoint, fail, errors) {
  let task = null
  let report = {
    title: settings.dashboard_title, state: 'ready', generated_at: new Date().toISOString(),
    elapsed_ms: 100, total: 1, ok_count: 1, slow_count: 0, error_count: 0, unknown_count: 0,
    provider_count: 1, providers: [], provider_errors: [], stale_after_seconds: 600,
  }
  let holdNext = false
  let release
  let started
  const heldReady = new Promise(resolve => { started = resolve })
  const { context, page } = await createContext(browser, 1440, async (route, url) => {
    let response
    if (url.pathname === '/api/admin/check') {
      task = { id: 701, status: 'running', kind: 'manual', provider_id: '', error_message: '' }
      holdNext = true
      return route.fulfill({ status: 202, json: { ok: true, task } })
    }
    if (url.pathname === '/api/admin/detection') response = task?.status === 'running' ? { ...idle, running: true, task_id: task.id, kind: 'manual' } : idle
    else if (url.pathname === '/api/admin/budget') response = { day: '2026-10-06', used: 0, limit: 0, remaining: 0, exhausted: false, resets_at: '2026-10-07T00:00:00Z' }
    else if (url.pathname === '/api/admin/config') response = { settings, providers: [] }
    else if (url.pathname === '/api/status') response = report
    else if (url.pathname === '/api/admin/tasks/701') response = task
    else throw new Error(`Unexpected API: ${url.pathname}`)
    const body = JSON.stringify(response)
    if (holdNext && url.pathname === heldEndpoint) {
      holdNext = false
      await new Promise(resolve => { release = resolve; started() })
      if (fail) return route.fulfill({ status: 503, json: { error: 'Stale refresh failed' } })
    }
    return route.fulfill({ contentType: 'application/json', body })
  }, errors)
  try {
    await page.goto(origin + '/admin/overview')
    await page.getByText('100 ms', { exact: true }).waitFor()
    await page.getByRole('button', { name: '立即检测', exact: true }).click()
    let waitTimer
    await Promise.race([
      heldReady,
      new Promise((_, reject) => { waitTimer = setTimeout(() => reject(new Error(`No held request: ${heldEndpoint}`)), 10000) }),
    ]).finally(() => clearTimeout(waitTimer))
    task = { ...task, status: 'success' }
    report = { ...report, elapsed_ms: 4321 }
    await page.getByText('检测任务 #701 已完成', { exact: true }).waitFor()
    await page.getByText('4321 ms', { exact: true }).waitFor()
    release()
    await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.textContent.includes('刷新数据') && !button.disabled))
    assert.equal(await page.getByText('4321 ms', { exact: true }).count(), 1, 'Older refresh overwrote the completed task report')
    assert.equal(await page.getByText('100 ms', { exact: true }).count(), 0)
    assert.equal(await page.getByText('Stale refresh failed', { exact: false }).count(), 0)
    assert.equal(await page.getByRole('button', { name: '立即检测', exact: true }).isDisabled(), false)
    await page.screenshot({ path: path.join(artifacts, `overview-order-${heldEndpoint.split('/').at(-1)}-${fail}.png`), fullPage: true })
  } catch (error) {
    console.error('Overview ordering failure:', heldEndpoint, fail, await page.locator('body').innerText())
    throw error
  } finally {
    release?.()
    await context.close()
  }
}

module.exports = async function adminRegressions(browser, artifacts) {
  const errors = []
  for (const width of [1440, 375]) await pendingModels(browser, artifacts, width, errors)
  console.log('PASS pending model save, retry, sync, paste, explicit clearing and IME on desktop/mobile')
  for (const [endpoint, fail] of [
    ['/api/status', false], ['/api/status', true], ['/api/admin/config', false],
    ['/api/admin/config', true], ['/api/admin/detection', false],
  ]) await overviewOrdering(browser, artifacts, endpoint, fail, errors)
  assert.deepEqual(errors, [])
  console.log('PASS completed task report/state survive late status, config, detection and failed refresh responses')
}
