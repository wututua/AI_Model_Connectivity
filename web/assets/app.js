let hasRendered = false;

async function loadStatus() {
  const response = await fetch('/api/status', { cache: 'no-store' });
  if (!response.ok) throw new Error('no report available');
  return response.json();
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, function(ch) {
    return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch];
  });
}

function renderSummary(report) {
  const items = [['总模型', report.total], ['正常', report.ok_count], ['较慢', report.slow_count], ['异常', report.error_count], ['Provider', report.provider_count], ['耗时', report.elapsed_ms + ' ms']];
  document.getElementById('summary').innerHTML = items.map(function(item) {
    return '<article class="summary-card"><span>' + escapeHTML(item[0]) + '</span><strong>' + escapeHTML(item[1]) + '</strong></article>';
  }).join('');
}

function renderHistory(items) {
  return '<div class="history">' + (items || []).map(function(item) { return '<i class="bar ' + escapeHTML(item) + '"></i>'; }).join('') + '</div>';
}

function renderCurve(item) {
  if (!item.show_curve_chart || !item.svg_path_line) return '';
  return '<svg class="curve" viewBox="0 0 100 40" preserveAspectRatio="none" aria-hidden="true"><path class="curve-area" d="' + escapeHTML(item.svg_path_area) + '"></path><path class="curve-line" d="' + escapeHTML(item.svg_path_line) + '"></path></svg>';
}

function renderProviders(report) {
  const providers = report.providers || [];
  document.getElementById('providers').innerHTML = providers.map(function(provider) {
    const rows = (provider.results || []).map(function(item) {
      return '<section class="model-row ' + escapeHTML(item.status) + '">' + renderCurve(item) + '<div class="model-main"><div><h3>' + escapeHTML(item.model) + '</h3><p>' + escapeHTML(item.response_preview || item.error || '') + '</p></div><span class="model-status ' + escapeHTML(item.status) + '">' + escapeHTML(item.status_label) + '</span></div><div class="model-meta"><span>' + escapeHTML(item.latency_ms) + ' ms</span><span>24h ' + escapeHTML(item.avg_latency_24h) + '</span><span>' + escapeHTML(report.stats_window_days) + 'd ' + escapeHTML(item.weekly_success_text) + '</span><span>可用率 ' + escapeHTML(item.availability) + '</span></div>' + renderHistory(item.history) + '</section>';
    }).join('');
    const logo = provider.provider_logo ? '<img src="' + escapeHTML(provider.provider_logo) + '" alt="">' : '<span class="logo-fallback"></span>';
    return '<article class="provider-card ' + escapeHTML(provider.status) + '"><header class="provider-head"><div class="provider-title">' + logo + '<div><h2>' + escapeHTML(provider.provider_name) + '</h2><p>' + escapeHTML(provider.provider_type) + ' · ' + escapeHTML(provider.provider_id) + '</p></div></div><span class="provider-status ' + escapeHTML(provider.status) + '">' + escapeHTML(provider.status_label) + '</span></header><div class="models">' + rows + '</div></article>';
  }).join('');
}

function renderErrors(report) {
  const el = document.getElementById('errors');
  const errors = report.provider_errors || [];
  if (!errors.length) {
    el.hidden = true;
    el.innerHTML = '';
    return;
  }
  el.hidden = false;
  el.innerHTML = '<h2>Provider 错误</h2>' + errors.map(function(item) { return '<p><strong>' + escapeHTML(item.provider_id) + '</strong> ' + escapeHTML(item.error) + '</p>'; }).join('');
}

function render(report) {
  hasRendered = true;
  document.title = report.title || '模型连通性';
  document.body.dataset.theme = report.theme || 'dark';
  document.getElementById('title').textContent = report.title || '模型连通性';
  document.getElementById('subtitle').textContent = '生成时间 ' + (report.generated_at || 'N/A') + ' · 主题 ' + (report.theme_label || 'N/A') + ' · 并发 ' + report.global_concurrency + '/' + report.provider_concurrency;
  const overall = document.getElementById('overall');
  overall.textContent = report.overall_status || 'UNKNOWN';
  overall.className = 'status-pill ' + (report.overall_class || '');
  renderSummary(report);
  renderProviders(report);
  renderErrors(report);
}

function showError(error) {
  document.getElementById('subtitle').textContent = '无法读取 /api/status，请先运行后端检测。';
  document.getElementById('providers').innerHTML = '<article class="provider-card error"><p>' + escapeHTML(error.message) + '</p></article>';
}

function startEvents() {
  if (!window.EventSource) return false;
  const events = new EventSource('/api/events');
  events.onmessage = function(event) {
    render(JSON.parse(event.data));
  };
  events.onerror = function() {
    if (hasRendered) {
      loadStatus().then(render).catch(function() {});
      return;
    }
    loadStatus().then(render).catch(showError);
  };
  return true;
}

loadStatus().then(render).catch(showError);
if (!startEvents()) {
  setInterval(function() {
    loadStatus().then(render).catch(showError);
  }, 30000);
}
