const assert = require('node:assert/strict')
const fs = require('node:fs/promises')
const path = require('node:path')

const origin = 'http://127.0.0.1:18190'
const webRoot = path.resolve(__dirname, '../../web')
const now = new Date().toISOString()
const baseSettings = {
  dashboard_title: 'Feature Monitor', timeout_seconds: 30, model_list_timeout_seconds: 20,
  slow_threshold_ms: 800, concurrency: 2, provider_concurrency: 2, max_models_per_provider: 0,
  skip_models: [], enable_history: true, show_curve_chart: true, stats_window_days: 7,
  history_size: 30, max_history_records: 500, show_error_detail: true, theme_mode: 'auto',
  day_mode_start_hour: 8, day_mode_end_hour: 18, auto_check_interval_min_hours: 0, auto_check_interval_max_hours: 0,
  notify_platform: 'disabled', notify_cooldown_minutes: 0, notify_on_recovery: true,
  notify_providers: [], notify_models: [], status_login_required: false,
}

module.exports = async function featureRegressions(browser, artifacts) {
  for (const width of [320, 375, 1440]) {
    const context = await browser.newContext({ viewport: { width, height: width < 640 ? 812 : 960 }, acceptDownloads: true })
    const page = await context.newPage()
    page.setDefaultTimeout(12000)
    const errors = []
    let role = 'admin', settings = structuredClone(baseSettings), check = null
    const mutations = []
    let tokens = [], issued = 0
    let failProviderDelete = false
    let delayedProviderRefresh = null
    const releaseRefreshes = []
    const delayProviderRefresh = (status = 200) => {
      const pending = { status }
      pending.ready = new Promise(resolve => { pending.started = resolve })
      pending.wait = new Promise(resolve => { pending.release = resolve })
      releaseRefreshes.push(pending.release)
      delayedProviderRefresh = pending
      return pending
    }
    let providers = [
      { id: 'alpha', name: 'Alpha', type: 'openai', base_url: 'https://example.invalid/v1', models: ['m1', 'm2'], enabled: true, probe_enabled: true, api_key_set: true, group: 'production', tags: ['primary'] },
      { id: 'beta', name: 'Beta', type: 'openai', base_url: 'https://example.invalid/v1', models: ['m3'], enabled: true, probe_enabled: true, api_key_set: false, group: 'backup', tags: [] },
    ]
    const report = {
      generated_at: now, title: 'Feature Monitor', total: 2, provider_count: 1, elapsed_ms: 120, ok_count: 1, slow_count: 0, error_count: 1,
      providers: [{ provider_id: 'alpha', results: [{ model: 'm1', status: 'error', checked_at: now }, { model: 'm2', status: 'ok', checked_at: now }] }],
    }
    await context.addInitScript(() => { window.EventSource = undefined; localStorage.setItem('theme', 'light') })
    page.on('pageerror', error => errors.push(error.message))
    await context.route('**/*', async route => {
      const url = new URL(route.request().url())
      assert.equal(url.origin, origin, 'Unexpected external request')
      const method = route.request().method()
      const respond = (json, status = 200) => route.fulfill({ json, status })
      if (url.pathname === '/api/auth/session') return respond({ user: { id: 1, username: 'feature-admin', role, enabled: true, must_change_password: false }, csrf_token: 'feature-csrf', expires_at: 9999999999, status_login_required: false })
      if (url.pathname.startsWith('/api/') && method !== 'GET') {
        assert.equal(role, 'admin')
        assert.equal(route.request().headers()['x-csrf-token'], 'feature-csrf')
        const value = route.request().postData() ? route.request().postDataJSON() : null
        mutations.push({ path: url.pathname, method, value })
        if (url.pathname === '/api/admin/providers/batch') {
          if (!value.ids.length || value.ids.some(id => !providers.some(p => p.id === id))) {
            return respond({ error: 'provider not found' }, 404)
          }
          providers = providers.map(p => value.ids.includes(p.id) ? { ...p, ...(value.action === 'group' ? { group: value.group } : { probe_enabled: value.action === 'resume' }) } : p)
          return respond({ settings, providers })
        }
        if (url.pathname.startsWith('/api/admin/providers/') && method === 'DELETE') {
          if (failProviderDelete) return respond({ error: 'provider delete failed' }, 500)
          const id = decodeURIComponent(url.pathname.split('/').at(-1))
          if (!providers.some(p => p.id === id)) return respond({ error: 'provider not found' }, 404)
          providers = providers.filter(p => p.id !== id)
          return respond({ ok: true })
        }
        if (url.pathname === '/api/admin/providers/alpha') {
          providers[0] = { ...providers[0], ...value, api_key_set: true }
          return respond(providers[0])
        }
        if (url.pathname === '/api/admin/providers') {
          providers.push({ ...value, api_key_set: false })
          return respond(providers.at(-1))
        }
        if (url.pathname === '/api/admin/detection/selected') {
          check = { id: 41, kind: 'models', status: 'running', started_at: now, provider_id: 'alpha', total: 1 }
          return respond({ ok: true, task: check }, 202)
        }
        if (url.pathname === '/api/admin/detection/stop') { check.status = 'canceled'; return respond({ stopped: true }) }
        if (url.pathname === '/api/admin/settings') { settings = value; return respond({ settings, providers }) }
        if (url.pathname === '/api/admin/metrics-tokens' || url.pathname.endsWith('/rotate')) {
          issued++
          const token = { id: 1, name: value?.name ?? 'monitor', created_at: now, rotated_at: now }
          tokens = [token]
          return respond({ ...token, token: `cgm_fixture_${issued}` }, method === 'POST' ? 201 : 200)
        }
        if (url.pathname === '/api/admin/metrics-tokens/1' && method === 'DELETE') { tokens = []; return respond({ ok: true }) }
        throw new Error(`Unexpected mutation: ${url.pathname}`)
      }
      if (url.pathname === '/api/admin/providers') {
        const snapshot = structuredClone(providers)
        if (delayedProviderRefresh) {
          const pending = delayedProviderRefresh
          delayedProviderRefresh = null
          pending.started()
          await pending.wait
          return respond(pending.status === 200 ? snapshot : { error: 'late provider refresh failure' }, pending.status)
        }
        return respond(snapshot)
      }
      if (url.pathname === '/api/admin/config') return respond({ settings, providers })
      if (url.pathname === '/api/status') return respond(report)
      if (url.pathname === '/api/admin/detection') return respond({
        running: check?.status === 'running', task_id: check?.status === 'running' ? 41 : 0, kind: check?.kind ?? '', provider_id: check?.provider_id ?? '',
        auto_check_interval_min_hours: 0, auto_check_interval_max_hours: 0, elapsed_ms: 4200,
        progress: { phase: 'probing', total: 2, completed: 1, provider_id: '', active: [{ provider_id: 'alpha', model: 'm1' }] },
      })
      if (url.pathname === '/api/admin/tasks/41') return respond(check)
      if (url.pathname === '/api/admin/budget') return respond({ day: '2026-10-06', used: 4, limit: 10, remaining: 6, exhausted: false, resets_at: '2026-10-07T00:00:00Z' })
      if (url.pathname === '/api/admin/metrics-tokens') return respond(tokens)
      if (url.pathname === '/api/admin/export') {
        assert.equal(url.searchParams.get('provider_id'), 'alpha')
        assert.equal(url.searchParams.get('model'), 'm1')
        return route.fulfill({ contentType: 'text/csv', body: 'provider_id,model\r\nalpha,m1\r\n' })
      }
      if (url.pathname === '/api/admin/diagnostics') return respond({ provider_count: 2, os: 'fixture' })
      if (url.pathname.startsWith('/api/')) throw new Error(`Unexpected API: ${url.pathname}`)
      const file = url.pathname.startsWith('/assets/') || url.pathname.startsWith('/fonts/') ? path.join(webRoot, url.pathname) : path.join(webRoot, 'index.html')
      assert.ok(file.startsWith(webRoot + path.sep))
      return route.fulfill({ contentType: { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.ttf': 'font/ttf' }[path.extname(file)] || 'application/octet-stream', body: await fs.readFile(file) })
    })
    const noOverflow = async () => assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `overflow at ${width}: ${page.url()}`)
    const alphaStatus = () => width < 640
      ? page.locator('section[aria-label="Alpha"]')
      : page.getByRole('row').filter({ has: page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }) })
    const flushRender = () => page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    const providerAction = async (desktop, mobile) => {
      if (width < 640) {
        await page.getByRole('button', { name: 'Alpha 更多操作', exact: true }).click()
        await page.getByRole('button', { name: mobile, exact: true }).click()
      } else await page.getByRole('button', { name: desktop, exact: true }).click()
    }
    try {
      await page.goto(origin + '/admin/providers')
      await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).waitFor()
      await page.getByRole('combobox', { name: '分组与标签' }).click()
      await page.getByRole('option', { name: '分组 · production', exact: true }).click()
      assert.equal(await page.getByRole('checkbox', { name: '选择 Provider Beta', exact: true }).count(), 0)
      await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).check()
      const staleRefresh = delayProviderRefresh()
      const staleResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/admin/providers')
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await staleRefresh.ready
      await page.getByRole('button', { name: '应用到 1 项', exact: true }).click()
      await page.getByRole('button', { name: '确认应用', exact: true }).click()
      await page.getByText('批量操作已保存', { exact: true }).waitFor()
      assert.equal(providers[0].probe_enabled, false)
      assert.equal(providers[1].probe_enabled, true)
      assert.ok((await alphaStatus().innerText()).includes('不参与检测'))
      staleRefresh.release()
      await staleResponse
      await flushRender()
      assert.ok((await alphaStatus().innerText()).includes('不参与检测'), 'late refresh reverted a successful batch pause')
      assert.equal(await page.getByRole('button', { name: '刷新', exact: true }).isEnabled(), true)
      await page.screenshot({ path: path.join(artifacts, `batch-refresh-order-${width}.png`), fullPage: true, animations: 'disabled' })
      await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).check()
      await page.getByRole('combobox', { name: '批量操作', exact: true }).click()
      await page.getByRole('option', { name: '恢复检测', exact: true }).click()
      const failedRefresh = delayProviderRefresh(500)
      const failedResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/admin/providers')
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await failedRefresh.ready
      await page.getByRole('button', { name: '应用到 1 项', exact: true }).click()
      await page.getByRole('button', { name: '确认应用', exact: true }).click()
      await page.getByRole('alertdialog').waitFor({ state: 'hidden' })
      failedRefresh.release()
      await failedResponse
      await flushRender()
      assert.equal(await page.getByText('错误：late provider refresh failure', { exact: true }).count(), 0)
      assert.equal((await alphaStatus().innerText()).includes('不参与检测'), false)
      assert.equal(await page.getByRole('button', { name: '刷新', exact: true }).isEnabled(), true)
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.textContent === '刷新' && !button.disabled))

      await providerAction('编辑 Alpha', '编辑 Provider')
      await page.getByRole('combobox', { name: '兼容预设', exact: true }).click()
      await page.getByRole('option', { name: 'Chat 推理参数', exact: true }).click()
      assert.equal(await page.getByRole('switch', { name: '发送 temperature', exact: true }).isChecked(), false)
      await page.getByLabel('超时（秒，0 为全局值）', { exact: true }).fill('3')
      await page.getByLabel('用户提示词', { exact: true }).fill('custom ping')
      await noOverflow()
      await page.screenshot({ path: path.join(artifacts, `probe-options-${width}.png`), fullPage: true, animations: 'disabled' })
      await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
      await page.getByRole('dialog').waitFor({ state: 'hidden' })
      assert.equal(providers[0].probe.max_tokens, 1024)
      assert.equal(providers[0].probe.token_limit_field, 'max_completion_tokens')
      assert.equal(providers[0].probe.prompt, 'custom ping')

      await providerAction('复制 Alpha', '复制配置（不含密钥）')
      await page.getByLabel('唯一 ID', { exact: true }).fill('alpha-copy')
      assert.equal(await page.getByLabel('API Key', { exact: true }).inputValue(), '')
      await page.getByRole('button', { name: '保存 Provider', exact: true }).click()
      await page.getByRole('dialog').waitFor({ state: 'hidden' })
      const copy = mutations.find(m => m.path === '/api/admin/providers').value
      assert.equal(copy.api_key, '')
      assert.equal(copy.enabled, false)
      assert.deepEqual(copy.models, ['m1', 'm2'])

      const originalRows = report.providers[0].results
      for (const scope of [
        { name: 'skip', skip: ['m2'], max: 0, rows: [originalRows[0]] },
        { name: 'limit', skip: [], max: 1, rows: [originalRows[0]] },
        { name: 'first check', skip: ['m2'], max: 0, rows: [{ model: 'm1', status: 'unknown' }] },
        { name: 'all excluded', skip: ['m1', 'm2'], max: 0, rows: [] },
      ]) {
        settings.skip_models = scope.skip
        settings.max_models_per_provider = scope.max
        report.providers[0].results = scope.rows
        await providerAction('模型检测 Alpha', '模型检测')
        await page.getByText('正在加载模型', { exact: true }).waitFor({ state: 'hidden' })
        const dialog = page.getByRole('dialog')
        assert.equal(await dialog.getByRole('checkbox', { name: /^m2/ }).count(), 0, `${scope.name}: excluded model was offered`)
        if (scope.rows.length) {
          await dialog.getByRole('checkbox', { name: '全选（1）', exact: true }).check()
          assert.equal(await dialog.getByRole('button', { name: '检测所选（1）', exact: true }).isEnabled(), true)
          assert.equal(await dialog.getByRole('checkbox', { name: /^m1/ }).isChecked(), true)
          if (scope.name === 'first check') {
            await dialog.getByText('未检测', { exact: true }).waitFor()
            assert.equal(await dialog.getByRole('button', { name: '重测失败项', exact: true }).isEnabled(), false)
          }
        } else {
          await dialog.getByText('暂无可检测模型', { exact: true }).waitFor()
          assert.equal(await dialog.getByRole('checkbox').count(), 0)
          assert.equal(await dialog.getByRole('button', { name: '检测所选（0）', exact: true }).isEnabled(), false)
        }
        await noOverflow()
        if (scope.name === 'skip') await page.screenshot({ path: path.join(artifacts, `model-scope-${width}.png`), fullPage: true, animations: 'disabled' })
        await page.keyboard.press('Escape')
        await dialog.waitFor({ state: 'detached' })
      }
      settings.skip_models = []; settings.max_models_per_provider = 0
      report.providers[0].results = originalRows
      await providerAction('模型检测 Alpha', '模型检测')
      await page.getByRole('checkbox', { name: /^m1/ }).check()
      await page.getByRole('button', { name: '检测所选（1）', exact: true }).click()
      await page.getByRole('dialog').waitFor({ state: 'hidden' })
      assert.deepEqual(mutations.find(m => m.path === '/api/admin/detection/selected').value.targets, [{ provider_id: 'alpha', model: 'm1' }])
      await page.goto(origin + '/admin/overview')
      await page.getByRole('region', { name: '检测进度', exact: true }).waitFor()
      await page.getByText('alpha / m1', { exact: true }).waitFor()
      await page.getByText('剩余 6', { exact: true }).waitFor()
      await noOverflow()
      await page.screenshot({ path: path.join(artifacts, `progress-${width}.png`), fullPage: true, animations: 'disabled' })
      await page.getByRole('button', { name: '停止检测', exact: true }).click()
      assert.equal(check.status, 'canceled')

      await page.goto(origin + '/admin/settings?tab=notify')
      await page.getByLabel('连续异常告警阈值', { exact: true }).fill('3')
      await page.getByLabel('连续正常恢复阈值', { exact: true }).fill('2')
      await page.getByLabel('维护开始', { exact: true }).fill('2026-10-06T18:00')
      await page.getByLabel('维护结束', { exact: true }).fill('2026-10-06T19:00')
      await page.getByRole('tab', { name: '历史与调度', exact: true }).click()
      await page.getByLabel('每日上游请求上限', { exact: true }).fill('100')
      await page.getByLabel('自动发现确认阈值', { exact: true }).fill('20')
      await page.getByRole('button', { name: '保存全部设置', exact: true }).click()
      await page.getByText('系统设置已保存', { exact: true }).waitFor()
      assert.equal(settings.daily_request_limit, 100)
      assert.equal(settings.notify_failure_threshold, 3)
      assert.ok(settings.maintenance_start.endsWith('Z'))

      await page.goto(origin + '/admin/operations')
      await page.getByLabel('Provider ID', { exact: true }).fill('alpha')
      await page.getByLabel('模型', { exact: true }).fill('m1')
      const download = page.waitForEvent('download')
      await page.getByRole('button', { name: '导出 CSV', exact: true }).click()
      assert.ok((await download).suggestedFilename().endsWith('.csv'))
      await page.getByRole('tab', { name: '指标凭据', exact: true }).click()
      await page.getByLabel('凭据名称', { exact: true }).fill('monitor')
      await page.getByRole('button', { name: '创建', exact: true }).click()
      await page.getByLabel('完整指标凭据', { exact: true }).waitFor()
      assert.equal(await page.getByLabel('完整指标凭据', { exact: true }).inputValue(), 'cgm_fixture_1')
      await page.getByRole('button', { name: '关闭凭据', exact: true }).click()
      await page.getByRole('button', { name: '轮换 monitor', exact: true }).click()
      await page.getByRole('button', { name: '确认', exact: true }).click()
      await page.getByLabel('完整指标凭据', { exact: true }).waitFor()
      assert.equal(await page.getByLabel('完整指标凭据', { exact: true }).inputValue(), 'cgm_fixture_2')
      for (const theme of ['light', 'dark']) {
        await page.evaluate(theme => { document.body.dataset.theme = theme }, theme)
        await noOverflow()
        await page.screenshot({ path: path.join(artifacts, `operations-${theme}-${width}.png`), fullPage: true, animations: 'disabled' })
      }
      await page.getByRole('button', { name: '撤销 monitor', exact: true }).click()
      await page.getByRole('button', { name: '确认', exact: true }).click()
      await page.getByText('暂无指标凭据', { exact: true }).waitFor()
      assert.equal(tokens.length, 0)

      await page.goto(origin + '/admin/providers')
      await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).check()
      await page.getByRole('checkbox', { name: '选择 Provider Beta', exact: true }).check()
      await page.getByRole('textbox', { name: '搜索 Provider', exact: true }).fill('Beta')
      await page.getByRole('combobox', { name: '分组与标签' }).click()
      await page.getByRole('option', { name: '分组 · backup', exact: true }).click()
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.textContent === '刷新' && !button.disabled))
      await page.getByText('已选 2', { exact: true }).waitFor()
      assert.equal(await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).count(), 0)
      const beta = providers.find(p => p.id === 'beta')
      providers = providers.filter(p => p.id !== 'beta')
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await page.getByText('已选 1', { exact: true }).waitFor()
      providers.push(beta)
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await page.getByRole('checkbox', { name: '选择 Provider Beta', exact: true }).waitFor()
      assert.equal(await page.getByRole('checkbox', { name: '选择 Provider Beta', exact: true }).isChecked(), false)
      await page.getByRole('textbox', { name: '搜索 Provider', exact: true }).fill('')
      await page.getByRole('combobox', { name: '分组与标签' }).click()
      await page.getByRole('option', { name: '全部分组与标签', exact: true }).click()
      assert.equal(await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).isChecked(), true, 'hidden selection was cleared')

      failProviderDelete = true
      await providerAction('删除 Alpha', '删除 Provider')
      const rejectedDelete = page.waitForResponse(response => response.request().method() === 'DELETE')
      await page.getByRole('button', { name: '确认删除', exact: true }).click()
      assert.equal((await rejectedDelete).status(), 500)
      await page.getByRole('button', { name: '确认删除', exact: true }).waitFor()
      await page.getByRole('button', { name: '取消', exact: true }).click()
      assert.equal(await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).isChecked(), true)
      await page.getByText('已选 1', { exact: true }).waitFor()

      failProviderDelete = false
      const deletedRefresh = delayProviderRefresh(500)
      const deletedResponse = page.waitForResponse(response => new URL(response.url()).pathname === '/api/admin/providers')
      await providerAction('删除 Alpha', '删除 Provider')
      await page.getByRole('button', { name: '确认删除', exact: true }).click()
      await page.getByRole('alertdialog').waitFor({ state: 'hidden' })
      await deletedRefresh.ready
      await page.getByText('已选 0', { exact: true }).waitFor()
      assert.equal(await page.getByRole('checkbox', { name: '选择 Provider Alpha', exact: true }).count(), 0)
      deletedRefresh.release()
      await deletedResponse
      await page.getByText('错误：late provider refresh failure', { exact: true }).waitFor()
      await page.getByText('已选 0', { exact: true }).waitFor()
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await page.waitForFunction(() => [...document.querySelectorAll('button')].some(button => button.textContent === '刷新' && !button.disabled))
      await page.getByRole('checkbox', { name: '选择 Provider Beta', exact: true }).check()
      await page.getByRole('button', { name: '应用到 1 项', exact: true }).click()
      await page.getByRole('button', { name: '确认应用', exact: true }).click()
      await page.getByText('批量操作已保存', { exact: true }).waitFor()
      assert.deepEqual(mutations.at(-1).value.ids, ['beta'], 'batch submitted a deleted provider')
      assert.equal(providers.find(p => p.id === 'beta').probe_enabled, false)
      await page.getByText('已选 0', { exact: true }).waitFor()
      await noOverflow()
      await page.screenshot({ path: path.join(artifacts, `provider-selection-${width}.png`), fullPage: true, animations: 'disabled' })

      role = 'user'
      await page.goto(origin + '/admin/operations')
      await page.getByRole('heading', { name: '运行概览', exact: true }).waitFor()
      assert.equal(await page.getByRole('tab', { name: '指标凭据', exact: true }).count(), 0)
      assert.deepEqual(errors, [])
      console.log(`PASS feature workflows, model scope, stale batch refresh, deleted/hidden selections, permissions and viewport ${width}`)
    } catch (error) {
      console.error(await page.locator('body').innerText())
      await page.screenshot({ path: path.join(artifacts, `feature-failure-${width}.png`), fullPage: true }).catch(() => {})
      throw error
    } finally { releaseRefreshes.forEach(release => release()); await context.close() }
  }
}
