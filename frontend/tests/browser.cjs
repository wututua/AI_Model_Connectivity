const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const os = require('node:os')
const path = require('node:path')
const { chromium } = require('playwright')

const origin = 'http://127.0.0.1:18081'
const webRoot = path.resolve(__dirname, '../../web')
const now = new Date().toISOString()
const providers = [
  { id: 'production-main', name: 'Production Gateway', type: 'openai', base_url: 'https://example.invalid/v1', models: ['model-a', 'model-b'], enabled: true, probe_enabled: true, api_key_set: true },
  { id: 'backup', name: 'Backup Gateway', type: 'openai', base_url: 'https://example.invalid/v1', models: ['model-c'], enabled: true, probe_enabled: true, api_key_set: true },
]
const initialSettings = {
  dashboard_title: 'Audit Monitor', timeout_seconds: 30, model_list_timeout_seconds: 20,
  slow_threshold_ms: 800, concurrency: 4, provider_concurrency: 2, max_models_per_provider: 0,
  skip_models: [], enable_history: true, show_curve_chart: true, stats_window_days: 7,
  history_size: 30, max_history_records: 500, show_error_detail: true, theme_mode: 'auto',
  day_mode_start_hour: 6, day_mode_end_hour: 18, auto_check_interval_min_hours: 0,
  auto_check_interval_max_hours: 0, notify_platform: 'disabled', notify_cooldown_minutes: 30,
  notify_providers: [], notify_models: [], status_login_required: false,
}
function makeReport(status = 'ok', count = 3) {
  const results = Array.from({ length: count }, (_, i) => ({
    provider_id: 'production-main', provider_name: 'Production Gateway', provider_type: 'openai',
    model: `model-${i}`, status, status_label: status === 'unknown' ? '未检测' : '正常',
    latency_ms: status === 'unknown' ? 0 : 300 + i, checked_at: now, response_preview: 'pong',
    history: Array.from({ length: 30 }, () => status), availability: status === 'unknown' ? 'N/A' : '100%',
    show_curve_chart: true, svg_path_line: 'M0,30 L30,10 L60,20 L100,15',
    svg_path_area: 'M0,30 L30,10 L60,20 L100,15 L100,40 L0,40 Z',
    avg_latency_24h: '300 ms', p50_latency_24h: '300 ms', p95_latency_24h: '340 ms',
    p99_latency_24h: '360 ms', latency_samples_24h: 30, error: '',
  }))
  const counts = { ok_count: status === 'ok' ? count : 0, slow_count: 0, error_count: 0, unknown_count: status === 'unknown' ? count : 0 }
  return {
    title: 'Audit Monitor', generated_at: now, elapsed_ms: 1500, global_concurrency: 4,
    provider_concurrency: 2, total: count, ...counts, provider_count: 1, provider_errors: [],
    overall_status: status === 'unknown' ? 'DEGRADED' : 'OPERATIONAL',
    overall_class: status === 'unknown' ? 'error' : 'ok', stale_after_seconds: 600,
    providers: [{ provider_id: 'production-main', provider_name: 'Production Gateway',
      provider_type: 'openai', provider_logo: '', checked_at: now, model_count: count,
      status, status_label: status === 'unknown' ? '未检测' : '正常', ...counts, results }],
  }
}

async function main() {
  const browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || 'chrome', headless: true })
  const artifacts = process.env.BROWSER_ARTIFACT_DIR || await fs.mkdtemp(path.join(os.tmpdir(), 'connectivity-browser-'))
  await fs.mkdir(artifacts, { recursive: true })
  const errors = []
  let report = makeReport()
  let settings = structuredClone(initialSettings)
  let sessionFailure = false
  let signedIn = true
  let role = 'admin'
  let taskDelays = false
  let saveFails = false
  let writes = 0
  let acceptedTask = null
  let reportFailures = 0
  let finishTaskOnDetection = false
  let providerWrites = 0
  let configImports = 0
  const delayed = []
  let page
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 } })
    await context.addInitScript(() => {
      window.EventSource = undefined
      localStorage.setItem('theme', 'light')
    })
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      if (url.origin !== origin) throw new Error(`Unexpected network request: ${url.origin}`)
      if (url.pathname.startsWith('/api/')) {
        const respond = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
        if (route.request().method() !== 'GET') {
          if (url.pathname === '/api/admin/config/import') {
            assert.equal(route.request().method(), 'POST')
            const value = route.request().postDataJSON()
            assert.deepEqual(value.providers, providers)
            settings = value.settings
            configImports++
            return respond({ settings, providers })
          }
          if (url.pathname === '/api/admin/providers') {
            assert.equal(route.request().method(), 'POST')
            assert.equal(route.request().postDataJSON().id, 'production-main')
            return respond({ error: 'provider id already exists' }, 409)
          }
          if (url.pathname === '/api/admin/providers/production-main') {
            assert.equal(route.request().method(), 'PUT')
            const draft = route.request().postDataJSON()
            assert.equal(draft.api_key, 'explicit-replacement')
            providerWrites++
            return respond(providers[0])
          }
          if (url.pathname === '/api/admin/check' || url.pathname === '/api/admin/providers/production-main/rerun') {
            assert.equal(route.request().method(), 'POST')
            acceptedTask = { id: 101, kind: 'manual', status: 'running', provider_id: url.pathname.includes('providers') ? 'production-main' : '', started_at: now, elapsed_ms: 0, total: 0, ok_count: 0, slow_count: 0, error_count: 0 }
            return respond({ ok: true, task: acceptedTask }, 202)
          }
          assert.equal(url.pathname, '/api/admin/settings', 'Only mocked settings/check mutations are allowed')
          assert.equal(route.request().method(), 'PUT')
          if (saveFails) return respond({ error: 'Mock save failure' }, 500)
          writes++
          settings = route.request().postDataJSON()
          return respond({ settings, providers })
        }
        if (url.pathname === '/api/auth/session') return sessionFailure
          ? respond({ error: 'Mock session failure' }, 503)
          : respond({ user: signedIn ? { id: 1, username: 'audit-admin', role, enabled: true, must_change_password: false } : null, csrf_token: 'audit', status_login_required: false, expires_at: 9999999999 })
        if (url.pathname === '/api/status') {
          if (reportFailures > 0) {
            reportFailures--
            return respond({ error: 'Mock report failure' }, 503)
          }
          return respond(report)
        }
        if (url.pathname === '/api/admin/config' || url.pathname === '/api/admin/config/export') return respond({ settings, providers })
        if (url.pathname === '/api/admin/providers') return respond(providers)
        if (url.pathname === '/api/admin/detection') {
          const state = { running: acceptedTask?.status === 'running', task_id: acceptedTask?.id || 0, provider_id: acceptedTask?.provider_id || '', auto_check_interval_min_hours: 0, auto_check_interval_max_hours: 0 }
          if (finishTaskOnDetection && acceptedTask) {
            finishTaskOnDetection = false
            acceptedTask.status = 'success'
          }
          return respond(state)
        }
        if (url.pathname === '/api/admin/tasks/101') return respond(acceptedTask)
        if (url.pathname === '/api/admin/tasks') {
          const offset = Number(url.searchParams.get('offset') || 0)
          const status = url.searchParams.get('status') || 'success'
          if (taskDelays && status === 'error') await new Promise(resolve => delayed.push(resolve))
          return respond(Array.from({ length: 20 }, (_, i) => ({
            id: offset + i + 1, kind: 'manual', status, started_at: now, elapsed_ms: 1000,
            ok_count: 3, slow_count: 0, error_count: 0, total: 3, provider_id: '',
          })))
        }
        throw new Error(`Unexpected API: ${url.pathname}`)
      }
      const asset = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/')
      const file = asset ? path.join(webRoot, decodeURIComponent(url.pathname)) : path.join(webRoot, 'index.html')
      assert.ok(path.resolve(file).startsWith(webRoot + path.sep))
      const type = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream'
      return route.fulfill({ contentType: type, body: await fs.readFile(file) })
    })
    page = await context.newPage()
    page.on('pageerror', error => errors.push(error.message))
    page.setDefaultTimeout(10000)
    const capture = async name => {
      await page.evaluate(() => document.fonts.ready)
      await page.screenshot({ path: path.join(artifacts, name + '.png'), fullPage: true, animations: 'disabled' })
    }
    const navigate = async tab => {
      await page.getByRole('button', { name: tab, exact: true }).click()
    }

    sessionFailure = true
    await page.goto(origin + '/admin/settings')
    await page.getByText('Mock session failure', { exact: true }).waitFor()
    assert.equal(await page.locator('#dashboard-title').count(), 0)
    sessionFailure = false
    await page.getByRole('button', { name: '重试', exact: true }).click()
    await page.locator('#dashboard-title').waitFor()

    await page.locator('#skip-models').pressSequentially('alpha,beta')
    await page.locator('#dashboard-title').click()
    assert.deepEqual(await page.getByRole('list', { name: '跳过模型', exact: true }).locator('li span').allTextContents(), ['alpha', 'beta'])
    await page.locator('#skip-models').fill('unfinished')
    await page.getByRole('button', { name: '保存全部设置', exact: true }).click()
    await page.getByText('系统设置已保存', { exact: true }).waitFor()
    assert.deepEqual(settings.skip_models, ['alpha', 'beta', 'unfinished'])
    assert.equal(writes, 1)
    await page.locator('#skip-models').evaluate(element => {
      const clipboard = new DataTransfer()
      clipboard.setData('text/plain', 'alpha,gamma\nomega')
      element.dispatchEvent(new ClipboardEvent('paste', { clipboardData: clipboard, bubbles: true, cancelable: true }))
    })
    assert.deepEqual(await page.getByRole('list', { name: '跳过模型', exact: true }).locator('li span').allTextContents(), ['alpha', 'beta', 'unfinished', 'gamma', 'omega'])
    await page.getByRole('button', { name: '移除 gamma', exact: true }).click()
    await page.locator('#skip-models').fill('中文模型')
    const composingPrevented = await page.locator('#skip-models').evaluate(element =>
      !element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', isComposing: true, bubbles: true, cancelable: true })),
    )
    assert.equal(composingPrevented, false)
    await page.locator('#skip-models').press('Enter')
    assert.equal(await page.locator('#skip-models').inputValue(), '')
    await page.getByRole('button', { name: '保存全部设置', exact: true }).click()
    await page.waitForFunction(() => document.querySelector('[role="status"]')?.textContent?.includes('已同步'))
    assert.deepEqual(settings.skip_models, ['alpha', 'beta', 'unfinished', 'omega', '中文模型'])
    await page.getByRole('tab', { name: '通知', exact: true }).click()
    await page.locator('#notify-providers').pressSequentially('one,two')
    await page.locator('#notify-models').pressSequentially('model-x;model-y')
    await page.getByRole('button', { name: '保存全部设置', exact: true }).click()
    await page.waitForFunction(() => document.querySelector('[role="status"]')?.textContent?.includes('已同步'))
    assert.deepEqual(settings.notify_providers, ['one', 'two'])
    assert.deepEqual(settings.notify_models, ['model-x', 'model-y'])
    await page.getByRole('tab', { name: '基础与检测', exact: true }).click()
    await page.locator('#dashboard-title').fill('Unsaved audit title')
    await navigate('Provider')
    await page.getByRole('alertdialog').waitFor()
    await page.getByRole('button', { name: '继续编辑', exact: true }).click()
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Unsaved audit title')

    sessionFailure = true
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await page.getByText('连接暂时中断，正在保留当前编辑内容。', { exact: true }).waitFor()
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Unsaved audit title')
    sessionFailure = false
    await page.getByRole('button', { name: '重试', exact: true }).click()
    await page.getByText('连接暂时中断，正在保留当前编辑内容。', { exact: true }).waitFor({ state: 'hidden' })
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Unsaved audit title')
    saveFails = true
    await page.getByRole('button', { name: '保存全部设置', exact: true }).click()
    await page.getByText('错误：Mock save failure', { exact: true }).waitFor()
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Unsaved audit title')
    saveFails = false
    await navigate('Provider')
    await page.getByRole('button', { name: '放弃更改并离开', exact: true }).click()
    await page.getByRole('heading', { name: 'Provider', exact: true }).waitFor()
    await navigate('系统设置')
    await page.locator('#dashboard-title').fill('Back navigation draft')
    await page.evaluate(() => history.back())
    await page.getByRole('alertdialog').waitFor()
    await page.getByRole('button', { name: '继续编辑', exact: true }).click()
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Back navigation draft')
    signedIn = false
    await page.evaluate(() => window.dispatchEvent(new Event('focus')))
    await page.locator('#username').waitFor()
    assert.equal(await page.locator('#dashboard-title').count(), 0)
    assert.equal(await page.getByRole('alertdialog').count(), 0)
    signedIn = true
    await page.goto(origin + '/admin/settings')
    await page.locator('#dashboard-title').waitFor()
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Audit Monitor')
    await capture('settings-desktop')
    await page.setViewportSize({ width: 320, height: 812 })
    await page.locator('#dashboard-title').fill('Mobile draft')
    await page.getByRole('combobox', { name: '当前页面', exact: true }).click()
    await page.getByRole('option', { name: 'Provider', exact: true }).click()
    await page.getByRole('alertdialog').waitFor()
    await capture('unsaved-mobile')
    await page.getByRole('button', { name: '继续编辑', exact: true }).click()
    // Let dialog focus restoration finish before Playwright selects the input.
    await page.getByRole('alertdialog').waitFor({ state: 'detached' })
    await page.locator('#dashboard-title').fill('Audit Monitor')
    assert.equal(await page.locator('#dashboard-title').inputValue(), 'Audit Monitor')
    await page.waitForFunction(() => document.querySelector('[role="status"]')?.textContent?.includes('已同步'))
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
    await capture('settings-mobile')
    await page.setViewportSize({ width: 1440, height: 960 })
    console.log('PASS settings tags, pending input, save retry, navigation guard, browser back, transient session failure and real session expiry')

    await navigate('配置管理')
    await page.getByRole('button', { name: '导出 JSON', exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: '重新加载', exact: true }).count(), 0)
    const downloadReady = page.waitForEvent('download')
    await page.getByRole('button', { name: '导出 JSON', exact: true }).click()
    const download = await downloadReady
    assert.match(download.suggestedFilename(), /^model-connectivity-config-\d{4}-\d{2}-\d{2}\.json$/)
    await page.getByText('配置已导出', { exact: true }).waitFor()
    const exported = JSON.parse(await page.getByRole('textbox', { name: '导出的配置内容', exact: true }).inputValue())
    assert.deepEqual(exported, { settings, providers })
    await page.locator('textarea:not([readonly])').fill(JSON.stringify(exported, null, 2))
    await page.getByRole('button', { name: '导入配置', exact: true }).click()
    await page.getByRole('alertdialog').waitFor()
    await page.getByRole('button', { name: '取消', exact: true }).click()
    await page.getByRole('alertdialog').waitFor({ state: 'detached' })
    assert.equal(configImports, 0)
    await page.getByRole('button', { name: '导入配置', exact: true }).click()
    await page.getByRole('button', { name: '确认导入', exact: true }).click()
    await page.getByText('配置已导入并生效', { exact: true }).waitFor()
    await page.getByRole('alertdialog').waitFor({ state: 'detached' })
    assert.equal(configImports, 1)
    for (const width of [1440, 320, 375]) {
      await page.setViewportSize({ width, height: width === 1440 ? 960 : 812 })
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      await capture(`config-${width}`)
    }
    await page.setViewportSize({ width: 1440, height: 960 })
    console.log('PASS configuration export/download, confirmed import, removed reload control and desktop/mobile layout')

    await navigate('运行概览')
    await page.getByRole('button', { name: '立即检测', exact: true }).click()
    await page.getByText('检测任务 #101 已启动', { exact: true }).waitFor()
    await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.textContent.includes('立即检测') && button.disabled))
    assert.equal(await page.getByText('检测任务 #101 已完成', { exact: true }).count(), 0)
    await capture('task-running-desktop')
    report = { ...makeReport(), elapsed_ms: 2468 }
    reportFailures = 1
    acceptedTask.status = 'success'
    await page.getByText('检测任务 #101 已完成', { exact: true }).waitFor()
    await page.getByText('2468 ms', { exact: true }).waitFor()
    assert.equal(reportFailures, 0)
    await navigate('Provider')
    await page.waitForFunction(() => !document.querySelector('button[aria-label="重新检测 Production Gateway"]')?.disabled)
    await page.getByRole('button', { name: '重新检测 Production Gateway', exact: true }).click()
    await page.getByText('「Production Gateway」检测任务 #101 已启动', { exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: '重新检测 Backup Gateway', exact: true }).isDisabled(), true)
    finishTaskOnDetection = true
    report = { ...makeReport(), elapsed_ms: 3579 }
    await navigate('运行概览')
    await page.getByText('检测任务 #101 已完成', { exact: true }).waitFor()
    await page.getByText('3579 ms', { exact: true }).waitFor()
    assert.equal(finishTaskOnDetection, false)
    await page.getByRole('button', { name: '立即检测', exact: true }).click()
    await page.getByText('检测任务 #101 已启动', { exact: true }).waitFor()
    acceptedTask.status = 'error'
    await page.getByText('错误：检测任务 #101 失败', { exact: true }).waitFor()
    acceptedTask = null
    console.log('PASS asynchronous tasks, busy controls, fast completion after navigation, report retries and error feedback')

    await navigate('任务历史')
    await page.getByText('#1', { exact: true }).waitFor()
    await page.getByRole('button', { name: '下一页', exact: true }).click()
    await page.getByText('#21', { exact: true }).waitFor()
    assert.equal(await page.getByText('显示 21–40 条', { exact: true }).count(), 1)
    taskDelays = true
    await page.getByRole('combobox').click()
    await page.getByRole('option', { name: '错误', exact: true }).click()
    await page.getByRole('status', { name: '正在加载任务历史', exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: '下一页', exact: true }).isDisabled(), true)
    await page.getByRole('combobox').click()
    await page.getByRole('option', { name: '成功', exact: true }).click()
    await page.getByText('#1', { exact: true }).waitFor()
    delayed.splice(0).forEach(resolve => resolve())
    await page.waitForTimeout(150)
    assert.equal(await page.locator('tbody').getByText('错误', { exact: true }).count(), 0)
    assert.equal(await page.getByText('显示 1–20 条', { exact: true }).count(), 1)
    console.log('PASS task pagination, loading state and stale response protection')

    await navigate('Provider')
    await page.getByRole('button', { name: '新增 Provider', exact: true }).click()
    await page.locator('#provider-id').fill('production-main')
    await page.locator('#provider-name').fill('Duplicate draft')
    await page.locator('#provider-url').fill('https://other.invalid/v1')
    await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
    await page.getByText('provider id already exists', { exact: true }).waitFor()
    assert.equal(await page.locator('#provider-name').inputValue(), 'Duplicate draft')
    await page.getByRole('button', { name: '取消', exact: true }).click()
    for (const width of [320, 375]) {
      await page.setViewportSize({ width, height: 812 })
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      const actions = page.getByRole('button', { name: 'Production Gateway 更多操作', exact: true })
      await actions.click()
      await page.getByRole('button', { name: '编辑 Provider', exact: true }).click()
      await page.locator('#provider-name').waitFor()
      assert.equal(await page.locator('#provider-name').inputValue(), 'Production Gateway')
      await page.locator('#provider-url').fill('https://other.invalid/v1')
      await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
      await page.getByText('Base URL 已变更，请重新填写 API Key 或明确清除原密钥', { exact: true }).waitFor()
      assert.equal(providerWrites, width === 320 ? 0 : 1)
      await page.locator('#provider-key').fill('explicit-replacement')
      await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
      await page.locator('#provider-name').waitFor({ state: 'detached' })
      await actions.click()
      await page.getByRole('button', { name: '删除 Provider', exact: true }).click()
      await page.getByRole('alertdialog').waitFor()
      await page.getByRole('button', { name: '取消', exact: true }).click()
      await capture(`providers-${width}`)
    }
    role = 'user'
    await page.reload()
    await page.getByRole('heading', { name: 'Provider', exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: /更多操作|新增 Provider/ }).count(), 0)
    role = 'admin'
    assert.equal(providerWrites, 2)
    console.log('PASS duplicate create feedback, explicit key on endpoint change, mobile edit/delete dialogs and read-only access')

    for (const state of ['unconfigured', 'pending']) {
      report = { ...makeReport('ok', 0), state, generated_at: '', providers: [], provider_count: state === 'pending' ? 2 : 0 }
      await page.goto(origin)
      await page.getByRole('heading', { name: state === 'pending' ? '等待首次检测' : '尚未配置监控服务', exact: true }).waitFor()
      assert.equal(await page.getByRole('heading', { name: '无法获取服务状态', exact: true }).count(), 0)
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      await capture(`first-install-${state}`)
    }
    console.log('PASS first-install empty and pending states')

    report = makeReport('unknown')
    await page.goto(origin)
    await page.getByRole('heading', { name: '所有模型尚未检测', exact: true }).waitFor()
    assert.equal(await page.getByRole('heading', { name: '检测到服务异常', exact: true }).count(), 0)
    report = makeReport('ok', 100)
    await page.reload()
    await page.getByRole('heading', { name: 'Production Gateway', exact: true }).waitFor()
    assert.equal(await page.locator('.model-detail').count(), 0)
    assert.equal(await page.locator('svg[aria-label="历史延迟趋势"]').count(), 0)
    const nodeCount = await page.locator('*').count()
    await page.getByRole('button', { name: '展开模型详情', exact: true }).first().click()
    await page.locator('.model-detail').waitFor()
    assert.equal(await page.locator('svg[aria-label="历史延迟趋势"]').count(), 1)
    await page.getByRole('button', { name: '收起模型详情', exact: true }).click()
    await page.locator('.model-detail').waitFor({ state: 'detached' })
    await page.getByRole('button', { name: '紧凑视图', exact: true }).click()
    await page.getByRole('combobox', { name: 'Provider 排序', exact: true }).click()
    await page.getByRole('option', { name: '按延迟', exact: true }).click()
    await page.reload()
    await page.getByRole('heading', { name: 'Production Gateway', exact: true }).waitFor()
    assert.equal(await page.getByRole('button', { name: '紧凑视图', exact: true }).getAttribute('aria-pressed'), 'true')
    assert.equal(await page.getByRole('combobox', { name: 'Provider 排序', exact: true }).innerText(), '按延迟')
    assert.equal(await page.locator('.history-strip').count(), 0)
    await page.evaluate(() => document.fonts.ready)
    const fonts = await page.evaluate(() => performance.getEntriesByType('resource')
      .filter(entry => new URL(entry.name).pathname.endsWith('.ttf')).map(entry => ({ name: entry.name, bytes: entry.decodedBodySize })))
    assert.equal(fonts.length, 2)
    assert.equal(fonts.some(font => font.name.includes('Medium')), false)
    assert.ok(fonts.every(font => new URL(font.name).searchParams.get('v')?.length === 64))
    console.log(`PASS unknown status, lazy details (${nodeCount} DOM nodes for 100 models), persisted preferences and two font requests (${fonts.reduce((sum, font) => sum + font.bytes, 0)} bytes)`)

    report = makeReport()
    await page.reload()
    await page.getByRole('heading', { name: 'Production Gateway', exact: true }).waitFor()
    await page.getByRole('button', { name: '详细视图', exact: true }).click()
    for (const theme of ['light', 'dark']) {
      await page.evaluate(theme => { document.body.dataset.theme = theme }, theme)
      for (const width of [320, 375, 1440]) {
        await page.setViewportSize({ width, height: width === 1440 ? 960 : 812 })
        assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
        await capture(`dashboard-${theme}-${width}`)
      }
    }
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.getByRole('button', { name: '展开模型详情', exact: true }).first().click()
    assert.equal(await page.locator('.model-detail').count(), 1)
    const duration = await page.locator('.motion-disclosure').last().evaluate(element => getComputedStyle(element).transitionDuration)
    assert.ok(duration.split(',').every(value => parseFloat(value) < 0.001))
    assert.deepEqual(errors, [])
    console.log(`PASS desktop/mobile themes and reduced motion; screenshots: ${artifacts}`)
    await require('./status-regressions.cjs')(browser, artifacts)
    await require('./admin-regressions.cjs')(browser, artifacts)
  } catch (error) {
    if (page && !page.isClosed()) {
      console.error('Browser failure URL:', page.url())
      console.error('Browser failure page:', await page.locator('body').innerText().catch(() => '(unavailable)'))
      await page.screenshot({ path: path.join(artifacts, 'failure.png'), fullPage: true }).catch(() => {})
    }
    throw error
  } finally {
    delayed.splice(0).forEach(resolve => resolve())
    await browser.close()
  }
}
main().catch(error => { console.error(error); process.exitCode = 1 })
