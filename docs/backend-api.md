# 前后端对接文档

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) · [仓库与发布](repositories.md)

本文档面向前端开发，汇总前端需要后端配合的全部内容：HTTP API、SSE 实时推送、认证机制、数据结构与错误约定。

- 前端代码位置：`frontend/`（React + TypeScript + Vite），构建产物输出到 `web/`，由后端静态托管
- 前端 API 封装：`frontend/src/api.ts`
- 前端类型定义：`frontend/src/types.ts`（与本文档字段一一对应）
- 后端路由注册：`internal/web/server.go`

---

## 1. 服务与部署

| 项 | 说明 |
|---|---|
| 默认地址 | `http://127.0.0.1:8080`（环境变量 `APP_HOST` / `APP_PORT`） |
| 静态托管 | 后端托管 `web/` 目录；非 `/api/` 且无扩展名的路径回退到 `index.html`（SPA 路由，如 `/admin`） |
| 健康检查 | `GET /health` → `{ "ok": true }`，无需认证 |
| 开发代理 | Vite dev server 已配置代理：`/api`、`/health` → `http://localhost:8080` |
| 请求体上限 | 1 MB（`maxRequestBody`），超出返回错误 |
| 响应头 | 所有受认证接口返回 `Cache-Control: no-store` |

---

## 2. 认证机制

### 2.1 两种 Token

| Token | 权限 | 说明 |
|---|---|---|
| 管理 Token（`ADMIN_TOKEN`） | 读写 | 所有 `/api/admin/*` 接口 |
| 只读 Token（View Token） | 只读 | 仅允许 GET 类接口：detection 状态、providers 列表、tasks、billing、`/metrics`；访问写接口返回 **403** |

请求头格式：

```
Authorization: Bearer <token>
```

前端将管理 Token 存于 `localStorage`，key 为 `cg_admin_token`（见 `api.ts` 的 `getToken/setToken`）。

### 2.2 首次使用流程

1. 未设置 `ADMIN_TOKEN` 时，后端首次启动自动生成随机密钥并打印到终端。
2. `GET /api/admin/detection` 返回的 `first_use: true` 表示首次使用，前端强制弹出改密表单（`Admin.tsx`）。
3. 前端调用 `POST /api/admin/token` 设置新密码（至少 16 位），完成后进入管理面板。

### 2.3 安全约束

- **认证失败限流**：同一 IP 连续认证失败会被限流，返回 **429** 并带 `Retry-After` 头（秒）。
- **公网绑定保护**：`APP_HOST` 为非回环地址且未设置 `ADMIN_TOKEN` 时，进程会拒绝启动；管理接口仍保留 **403** 作为纵深防御。
- **只读会话**：`GET /api/admin/detection` 返回 `read_only: true` 时，前端隐藏「设置」「导入导出」标签页（`Admin.tsx`）。

---

## 3. 错误约定

所有错误响应为 JSON：

```json
{ "ok": false, "error": "错误描述" }
```

部分接口返回纯文本错误（如 `unauthorized`、`read-only token cannot access this endpoint`），前端 `api.ts` 的 `decodeResponse` 统一提取 `error` 字段或回退到 `HTTP <status>`。

| 状态码 | 含义 |
|---|---|
| 400 | 请求体非法 / 参数校验失败（如 token 太短、provider id 非法） |
| 401 | 未认证（token 缺失或错误） |
| 403 | 只读 token 访问写接口；或公网未设 token |
| 404 | 资源不存在（如 `/api/status` 尚无报告、任务 id 不存在） |
| 405 | HTTP 方法不允许 |
| 409 | 已有检测任务正在运行（`check already running`） |
| 429 | 认证失败过多被限流，需按 `Retry-After` 等待 |
| 500 | 服务端内部错误 |

---

## 4. 公开接口（无需认证）

### 4.1 `GET /api/status`

获取最新检测报告（仪表盘首页数据）。

- **404**：尚无报告（服务刚启动未跑过检测），前端需处理空态并提示手动触发检测。
- **响应**：`Report` 对象（见 6.1）。

### 4.2 `GET /api/events`（SSE 实时推送）

Server-Sent Events，推送最新 `Report`。

- 响应头：`Content-Type: text/event-stream`
- 连接建立后立即推送一次当前最新报告（若存在）。
- 之后每次检测完成推送一条 `data: <Report JSON>\n\n`。
- 每 25 秒发送 `: keep-alive` 注释帧保活。
- **前端降级策略**（`Dashboard.tsx`）：浏览器不支持 `EventSource` 或连接失败时，降级为指数退避轮询 `/api/status`（30s → 60s → 120s）。

---

## 5. 管理接口（`/api/admin/*`）

> 除特别说明外均需 `Authorization: Bearer <ADMIN_TOKEN>`；标注「只读可用」的接口也接受 View Token。

### 5.1 检测控制

#### `GET /api/admin/detection`（只读可用）

查询运行状态。**该接口同时承担"登录校验"职责**：token 错误返回 401，前端据此判断是否进入面板。

响应 `RunningState`：

```json
{
  "running": false,
  "task_id": 12,
  "kind": "manual",
  "provider_id": "",
  "auto_check_interval_min_hours": 1,
  "auto_check_interval_max_hours": 3,
  "first_use": false,
  "read_only": false
}
```

- `first_use`：为 `true` 时前端强制改密（只读会话恒为 `false`）。
- `read_only`：当前以只读 token 访问时为 `true`。

#### `POST /api/admin/detection/start`

触发一次全量检测（同步执行，最长 30 分钟超时）。响应 `{ "ok": true, "report": Report }`；已有任务运行返回 **409**。

#### `POST /api/admin/detection/stop`

停止当前检测。响应 `{ "ok": true, "stopped": true }`。

#### `POST /api/admin/check`

与 `detection/start` 等效的全量检测触发入口（同步返回报告）。响应同上，409 语义相同。

### 5.2 密钥管理

#### `POST /api/admin/token`

修改管理密钥。请求 `{ "token": "<至少16位>" }`，响应 `{ "ok": true }`。

#### `GET /api/admin/view-token`

获取当前只读 token。响应 `{ "ok": true, "token": "..." }`（未设置时为空串）。

#### `POST /api/admin/view-token`

设置/轮换只读 token。请求体可选：`{ "token": "自定义值" }`；空 body 或空 token 时后端自动生成 16 字节随机 token。响应 `{ "ok": true, "token": "<生效值>" }`。

#### `DELETE /api/admin/view-token`

吊销只读 token。响应 `{ "ok": true }`。

### 5.3 配置管理

#### `GET /api/admin/config`

获取完整配置。响应 `AdminConfig`：`{ "settings": RuntimeSettings, "providers": SafeProviderConfig[] }`（见 6.3 / 6.4）。

#### `PUT /api/admin/settings`

更新运行时设置。请求体为完整 `RuntimeSettings` 对象，响应为更新后的 `AdminConfig`。

敏感字段写策略（前端 `SettingsTab.tsx` 已适配）：

- `notify_webhook_url` / `notify_telegram_bot_token` / `notify_telegram_chat_id`：GET 时后端不返回原值，只返回 `*_set: true/false` 标记；PUT 时留空表示保持不变，传 `clear_*: true` 表示清除。

#### `GET /api/admin/config/export`

导出配置（settings + providers，api_key 不导出）。响应 `ConfigExport`。

#### `POST /api/admin/config/import`

导入配置。请求体 `ConfigImport`：`{ "settings": RuntimeSettings, "providers": ProviderUpdate[] }`，响应更新后的 `AdminConfig`。

#### `POST /api/admin/config/reload`

从 `.env` 重载配置，响应 `AdminConfig`；成功后后端异步触发一次检测。

### 5.4 Provider 管理

#### `GET /api/admin/providers`（只读可用）

响应 `SafeProviderConfig[]`：

```json
[{
  "id": "openai",
  "name": "OpenAI",
  "type": "openai",
  "base_url": "https://api.openai.com",
  "models": ["gpt-4o"],
  "enabled": true,
  "probe_enabled": true,
  "api_key_set": true
}]
```

> 安全说明：永不返回真实 `api_key`，只返回 `api_key_set` 布尔标记。

#### `POST /api/admin/providers`

新增 Provider。请求体 `ProviderUpdate`（见 6.5），响应创建后的 `SafeProviderConfig`。

#### `PUT /api/admin/providers/{id}`

更新 Provider。路径参数 `id` 需 URL 编码。请求体 `ProviderUpdate`；`api_key` 留空表示不变，`clear_api_key: true` 表示清除。

#### `DELETE /api/admin/providers/{id}`

删除 Provider。响应 `{ "ok": true }`。

#### `POST /api/admin/providers/{id}/rerun`

单独重跑该 Provider 的检测（同步，最长 30 分钟）。响应 `{ "ok": true, "report": Report }`，409 语义同上。

**Provider id 校验规则**（后端 `ValidateProviderID`）：非空、≤128 字符、不允许首尾空格、不允许控制字符及 `/ \ ? #`、不允许 `.` / `..`。

**base_url 校验规则**：合法 http/https URL，不允许 query 参数和 fragment，禁止 link-local 地址（SSRF 防护）。

### 5.5 任务历史

#### `GET /api/admin/tasks?limit=&offset=&status=&provider_id=`（只读可用）

分页查询检测任务。响应 `CheckTask[]`（见 6.6），按时间倒序。

- `limit` / `offset`：分页参数（前端默认 limit=20）。
- `status`：按状态过滤（如 `success` / `failed`）。
- `provider_id`：按 Provider 过滤。

#### `GET /api/admin/tasks/{id}`（只读可用）

查询单个任务详情，响应 `CheckTask`。

### 5.6 用量统计

#### `GET /api/admin/billing?days=30`（只读可用）

Token 消耗估算。`days` 默认 30，最大 365。响应 `BillingSummary`（见 6.7）。

### 5.7 监控指标

#### `GET /metrics`（只读可用）

Prometheus 指标端点。仅在后端通过 `SetMetrics` 启用后可用，否则 404。scrape 配置需携带 `Authorization` 头。

---

## 6. 数据结构

字段命名统一为 snake_case，与 `frontend/src/types.ts` 一一对应。

### 6.1 `Report`（检测报告，`/api/status` 与 SSE 推送体）

```json
{
  "title": "仪表盘标题",
  "generated_at": "2024-01-01T12:00:00Z",
  "elapsed_ms": 1234,
  "global_concurrency": 8,
  "provider_concurrency": 2,
  "total": 20,
  "ok_count": 18,
  "slow_count": 1,
  "error_count": 1,
  "provider_count": 3,
  "providers": ["ProviderReport..."],
  "provider_errors": [{ "provider_id": "x", "provider_type": "openai", "error": "..." }],
  "overall_status": "ok",
  "overall_class": "ok",
  "history_size": 96,
  "stats_window_days": 7,
  "theme": "auto",
  "theme_label": "跟随系统"
}
```

### 6.2 `ProviderReport` / `ModelResult`

```json
{
  "provider_id": "openai",
  "provider_type": "openai",
  "provider_name": "OpenAI",
  "provider_logo": "…",
  "current_model": "gpt-4o",
  "results": ["ModelResult..."],
  "ok_count": 2, "slow_count": 0, "error_count": 0,
  "status": "ok", "status_label": "正常", "model_count": 2
}
```

`ModelResult` 关键字段：

| 字段 | 说明 |
|---|---|
| `model` / `current_model` / `is_current` | 模型名、Provider 当前模型、是否为当前模型 |
| `status` / `status_label` / `status_class` | 三态：`ok` / `slow` / `error` 及展示文案/样式类 |
| `latency_ms` | 本次延迟（毫秒） |
| `response_preview` | 响应预览（已剥离 `<think>` 标签） |
| `error` | 错误信息 |
| `history` | 历史状态序列（用于 LED 状态灯） |
| `show_curve_chart` | 是否展示延迟曲线 |
| `svg_path_line` / `svg_path_area` / `time_labels` | 后端预计算的延迟曲线 SVG 路径与时间轴 |
| `avg_latency_24h` / `p50_latency_24h` / `p95_latency_24h` / `p99_latency_24h` / `latency_samples_24h` | 24h 延迟统计（字符串为已格式化文案） |
| `weekly_success_text` / `availability` | 统计窗口可用率文案 |

### 6.3 `RuntimeSettings`（运行时设置）

仪表盘与检测参数：标题、超时（`timeout_seconds` / `model_list_timeout_seconds`）、慢阈值（`slow_threshold_ms`）、并发（`concurrency` / `provider_concurrency`）、模型过滤（`max_models_per_provider` / `skip_models`）、历史与曲线（`enable_history` / `show_curve_chart` / `history_size` / `max_history_records`）、统计窗口（`stats_window_days`）、错误详情开关（`show_error_detail`）、主题（`theme_mode` / `day_mode_start_hour` / `day_mode_end_hour`）、自动检测区间（`auto_check_interval_min_hours` / `auto_check_interval_max_hours`）、告警（`notify_*`，见 5.3 敏感字段策略）。

完整字段列表见 `frontend/src/types.ts` 的 `RuntimeSettings`。

### 6.4 `SafeProviderConfig` / 6.5 `ProviderUpdate`

见 5.4。`ProviderUpdate` 相比 `SafeProviderConfig` 多 `api_key`、`clear_api_key`，少 `api_key_set`。

### 6.6 `CheckTask`（检测任务）

```json
{
  "id": 12,
  "kind": "manual",
  "status": "success",
  "provider_id": "",
  "started_at": "…", "finished_at": "…",
  "elapsed_ms": 1234,
  "ok_count": 18, "slow_count": 1, "error_count": 1, "total": 20,
  "error_message": "",
  "report_generated_at": "…"
}
```

### 6.7 `BillingSummary`（用量统计）

```json
{
  "range_days": 30,
  "range_start": "…", "range_end": "…",
  "total_prompt_tokens": 0, "total_completion_tokens": 0,
  "total_tokens": 0, "total_probe_count": 0,
  "per_model": [{ "provider_id": "…", "provider_name": "…", "provider_type": "…", "model": "…", "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "probe_count": 0 }],
  "daily": [{ "day": "2024-01-01", "prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0, "probe_count": 0 }]
}
```

---

## 7. 前端对接要点

1. **统一入口**：所有请求走 `frontend/src/api.ts` 的 `api` 对象，自动携带 token 与 JSON 头；错误统一从 `error` 字段提取。
2. **登录态**：进入 `/admin` 先调 `api.detection()`，401 则停留在密钥输入页；`first_use` 强制改密；`read_only` 控制标签页可见性。
3. **实时刷新**：仪表盘优先 SSE（`/api/events`），失败自动降级轮询 `/api/status`。
4. **长耗时操作**：触发检测类接口（start / check / rerun）同步执行，前端需有 loading 态；409 表示已有任务在跑。
5. **敏感字段不回显**：api_key、告警 webhook/bot token 等只显示"已设置"，编辑时留空即不变。
6. **URL 编码**：Provider id 允许特殊字符，拼接路径时必须 `encodeURIComponent`（`api.ts` 已处理）。
