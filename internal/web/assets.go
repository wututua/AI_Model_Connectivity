package web

const indexHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>模型连通性</title>
  <link rel="stylesheet" href="assets/style.css">
</head>
<body>
  <main class="shell">
    <section class="hero">
      <div>
        <p class="eyebrow">Model Connectivity</p>
        <h1 id="title">模型连通性</h1>
        <p id="subtitle">等待检测结果...</p>
      </div>
      <div class="status-pill" id="overall">UNKNOWN</div>
    </section>
    <section class="summary" id="summary"></section>
    <section id="providers" class="providers"></section>
    <section id="errors" class="errors"></section>
  </main>
  <script src="assets/app.js"></script>
</body>
</html>
`

const appJS = `let hasRendered = false;

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
`

const styleCSS = `:root {
  color-scheme: dark;
  --bg: #0b1020;
  --card: rgba(18, 27, 52, .82);
  --card-strong: rgba(29, 42, 78, .9);
  --text: #edf2ff;
  --muted: #96a3bd;
  --border: rgba(148, 163, 184, .22);
  --ok: #38d996;
  --slow: #f6c453;
  --error: #ff6b7a;
  --shadow: 0 24px 80px rgba(0, 0, 0, .35);
}
body[data-theme="light"] { color-scheme: light; --bg: #eef3ff; --card: rgba(255, 255, 255, .86); --card-strong: rgba(255, 255, 255, .95); --text: #162033; --muted: #5f6f8a; --border: rgba(86, 104, 134, .18); --shadow: 0 24px 80px rgba(86, 104, 134, .18); }
* { box-sizing: border-box; }
[hidden] { display: none !important; }
body { margin: 0; min-height: 100vh; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: radial-gradient(circle at top left, rgba(74, 144, 226, .35), transparent 36rem), var(--bg); color: var(--text); }
.shell { width: min(1180px, calc(100vw - 32px)); margin: 0 auto; padding: 40px 0; }
.hero, .summary-card, .provider-card, .errors { border: 1px solid var(--border); background: var(--card); box-shadow: var(--shadow); backdrop-filter: blur(18px); }
.hero { display: flex; justify-content: space-between; gap: 24px; align-items: center; padding: 34px; border-radius: 32px; }
.eyebrow { margin: 0 0 8px; color: var(--muted); text-transform: uppercase; letter-spacing: .16em; font-size: 12px; }
h1 { margin: 0; font-size: clamp(34px, 7vw, 62px); letter-spacing: -.05em; }
#subtitle { margin: 12px 0 0; color: var(--muted); }
.status-pill, .provider-status, .model-status { border-radius: 999px; padding: 8px 13px; font-weight: 800; font-size: 12px; background: var(--card-strong); border: 1px solid var(--border); }
.status-pill { font-size: 16px; padding: 14px 20px; }
.ok { color: var(--ok); } .slow { color: var(--slow); } .error { color: var(--error); }
.summary { display: grid; grid-template-columns: repeat(6, 1fr); gap: 14px; margin: 20px 0; }
.summary-card { border-radius: 22px; padding: 18px; }
.summary-card span, .provider-title p, .model-row p, .model-meta { color: var(--muted); }
.summary-card strong { display: block; margin-top: 10px; font-size: 26px; }
.providers { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; }
.provider-card { border-radius: 28px; padding: 22px; overflow: hidden; }
.provider-head, .provider-title, .model-main, .model-meta { display: flex; align-items: center; }
.provider-head, .model-main { justify-content: space-between; gap: 14px; }
.provider-title { gap: 12px; }
.provider-title img, .logo-fallback { width: 38px; height: 38px; border-radius: 12px; background: var(--card-strong); }
.provider-title h2, .model-main h3 { margin: 0; }
.provider-title p, .model-row p { margin: 4px 0 0; font-size: 13px; overflow-wrap: anywhere; word-break: break-word; }
.models { display: grid; gap: 12px; margin-top: 18px; }
.model-row { position: relative; overflow: hidden; border: 1px solid var(--border); border-radius: 20px; padding: 16px; background: rgba(255,255,255,.04); }
.model-main, .model-meta, .history { position: relative; z-index: 1; }
.model-main h3 em { margin-left: 6px; color: var(--muted); font-style: normal; font-size: 12px; }
.model-meta { gap: 12px; flex-wrap: wrap; margin-top: 12px; font-size: 12px; }
.history { display: flex; gap: 3px; margin-top: 12px; }
.bar { flex: 1; min-width: 4px; height: 14px; border-radius: 999px; background: rgba(148, 163, 184, .2); }
.bar.ok { background: var(--ok); } .bar.slow { background: var(--slow); } .bar.error { background: var(--error); }
.curve { position: absolute; inset: auto 0 0 0; width: 100%; height: 70%; opacity: .16; }
.curve-line { fill: none; stroke: currentColor; stroke-width: 2; } .curve-area { fill: currentColor; opacity: .18; }
.errors { margin-top: 18px; border-radius: 24px; padding: 20px; }
.errors p { overflow-wrap: anywhere; word-break: break-word; }
@media (max-width: 900px) { .summary, .providers { grid-template-columns: 1fr 1fr; } }
@media (max-width: 640px) { .hero { align-items: flex-start; flex-direction: column; } .summary, .providers { grid-template-columns: 1fr; } }
`
