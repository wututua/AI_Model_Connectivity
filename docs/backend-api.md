# 前后端对接文档

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

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

### 2.1 账号与角色

| 用户组 | 权限 | 说明 |
|---|---|---|
| 管理员（`admin`） | 读写 | 所有 `/api/admin/*` 接口 |
| 普通用户（`user`） | 共享数据只读 | 可读取 detection、providers、tasks、billing、`/metrics`；仅可修改自己的密码 |

登录后使用服务端设置的 `cg_session` HttpOnly Cookie，不再把登录凭据存入 localStorage。写请求附带 session 响应中的 CSRF 值：

```
X-CSRF-Token: <csrf_token>
```

Cookie 为 SameSite Strict，24 小时绝对有效期；HTTPS 部署设置 `SECURE_COOKIES=true`。前端 `api.ts` 使用 `credentials: 'same-origin'`，CSRF 值仅保存在内存。

### 2.2 首次使用流程

1. 用户表为空时初始化管理员，账号默认 `admin`。密码取 `ADMIN_PASSWORD`，或迁移符合规则的旧管理 Token，否则随机生成并打印到终端。
2. `GET /api/auth/session` 返回 `user`、`csrf_token`、`expires_at` 和 `status_login_required`；匿名 `user=null`。登录使用 `POST /api/auth/login`。
3. `user.must_change_password=true` 时跳转改密界面，调用 `POST /api/auth/password`，提交 `current_password`、`password`。密码至少 8 位且包含大写、小写字母和数字，不强制特殊字符。
4. 接受新的 session 响应后才能继续。用户新建/密码被重置也需要改密；后端会阻止绕过此流程。

### 2.3 安全约束

- **认证失败限流**：同一 IP 连续认证失败会被限流，返回 **429** 并带 `Retry-After` 头（秒）。
- **来源与 CSRF 校验**：拒绝跨站写请求、缺少或错误的 CSRF 令牌。反向代理应保留原始 Host（含端口）。
- **只读会话**：前端按 `user.role` 隐藏修改按钮与管理标签页，后端独立拒绝普通用户写共享数据的请求。
- **监控访问**：`status_login_required=true` 时 REST 和 SSE 都要求已完成初始改密的账号会话。

---

## 3. 错误约定

所有错误响应为 JSON：

```json
{ "ok": false, "error": "错误描述" }
```

前端 `api.ts` 的 `decodeResponse` 提取 `error` 字段，无法解析时回退到 `HTTP <status>`。初始密码未修改时返回 403 和 `code: "password_change_required"`，前端刷新 session 并进入改密流程。

| 状态码 | 含义 |
|---|---|
| 400 | 请求体非法 / 参数校验失败（如密码太短、provider id 非法） |
| 401 | 未认证、登录失败或会话失效 |
| 403 | 权限不足、CSRF/来源校验失败、需要初始改密 |
| 404 | 资源不存在（如 `/api/status` 尚无报告、任务 id 不存在） |
| 405 | HTTP 方法不允许 |
| 409 | 已有检测任务正在运行（`check already running`） |
| 413 / 415 | 请求体过大 / 登录请求类型不是 JSON |
| 429 | 认证失败过多被限流，需按 `Retry-After` 等待 |
| 500 | 服务端内部错误 |
| 503 | 暂时无法读取访问策略或验证会话 |

---

## 4. 状态接口（由访问开关控制）

### 4.1 `GET /api/status`

获取最新检测报告（仪表盘首页数据）。`status_login_required=true` 时要求登录，否则可匿名读取。

- **404**：尚无报告（服务刚启动未跑过检测），前端需处理空态并提示手动触发检测。
- **响应**：`Report` 对象（见 6.1）。

### 4.2 `GET /api/events`（SSE 实时推送）

Server-Sent Events，推送最新 `Report`。

- 响应头：`Content-Type: text/event-stream`
- 连接建立后立即推送一次当前最新报告（若存在）。
- 之后每次检测完成推送一条 `data: <Report JSON>\n\n`。
- 每 5 秒发送 `: keep-alive` 注释帧保活，发送报告和心跳前复查访问权限。
- 权限失效时发送 `event: auth-required` 并关闭连接；前端刷新会话并跳转登录。
- **前端降级策略**（`Dashboard.tsx`）：浏览器不支持 `EventSource` 或连接失败时，降级为指数退避轮询 `/api/status`（30s → 60s → 120s）。

---

## 5. 管理接口（`/api/admin/*`）

> 除特别说明外均需管理员会话；标注「只读可用」的接口也接受普通用户会话。写接口需 `X-CSRF-Token`。

### 5.1 检测控制

#### `GET /api/admin/detection`（只读可用）

查询运行状态。登录状态通过 `/api/auth/session` 获取。

响应 `RunningState`：

```json
{
  "running": false,
  "task_id": 12,
  "kind": "manual",
  "provider_id": "",
  "auto_check_interval_min_hours": 1,
  "auto_check_interval_max_hours": 3,
  "read_only": false
}
```

- `read_only`：当前以普通用户账号访问时为 `true`。

#### `POST /api/admin/detection/start`

触发一次全量检测（同步执行，最长 30 分钟超时）。响应 `{ "ok": true, "report": Report }`；已有任务运行返回 **409**。

#### `POST /api/admin/detection/stop`

停止当前检测。响应 `{ "ok": true, "stopped": true }`。

#### `POST /api/admin/check`

与 `detection/start` 等效的全量检测触发入口（同步返回报告）。响应同上，409 语义相同。

### 5.2 用户管理

- `GET /api/admin/users`：返回 `User[]`。
- `POST /api/admin/users`：创建账号；请求 `UserInput`，返回 `User`。
- `PUT /api/admin/users/{id}`：修改账号；请求 `UserInput`，密码为空保留原值。
- `DELETE /api/admin/users/{id}`：删除账号。

`UserInput`：`username`、`password`、`role`（`admin`/`user`）、`enabled`。
`User`：`id`、`username`、`role`、`enabled`、`must_change_password`、`created_at`，不含密码/哈希。

用户名忽略大小写，3–32 位 ASCII 字母、数字、点、下划线或短横线，以字母/数字开头。修改、重置、禁用或删除立即撤销该用户会话。不能在此修改/删除当前账号，且必须保留一个启用的管理员。

个人注销用 `POST /api/auth/logout`；个人改密见 2.2。详见 [HTTP API](api.md#5-账号与用户管理)。

### 5.3 配置管理

#### `GET /api/admin/config`

获取完整配置。响应 `AdminConfig`：`{ "settings": RuntimeSettings, "providers": SafeProviderConfig[] }`（见 6.3 / 6.4）。

#### `PUT /api/admin/settings`

更新运行时设置。请求体为完整 `RuntimeSettings` 对象，响应为更新后的 `AdminConfig`。`status_login_required` 为状态监控登录开关。

敏感字段写策略（前端 `SettingsTab.tsx` 已适配）：

- `notify_webhook_url` / `notify_telegram_bot_token` / `notify_telegram_chat_id`：GET 时后端不返回原值，只返回 `*_set: true/false` 标记；PUT 时留空表示保持不变，传 `clear_*: true` 表示清除。

#### `GET /api/admin/config/export`

导出配置（settings + providers，api_key 不导出）。响应 `ConfigExport`。导入导出均不包含账号、密码、会话。

#### `POST /api/admin/config/import`

导入配置。请求体 `ConfigImport`：`{ "settings": RuntimeSettings, "providers": ProviderUpdate[] }`，响应更新后的 `AdminConfig`。

#### `POST /api/admin/config/reload`

从 `.env` 重载配置，响应 `AdminConfig`；成功后后端异步触发一次检测。

### 5.4 Provider 管理

#### `POST /api/admin/provider-models`（仅管理员）

读取尚未保存的连接草稿的模型列表，返回 `string[]`。请求 `ModelDiscoveryRequest`：`provider_id`（可选，引用已保存 Provider）、`type`、`base_url`、`api_key`、`clear_api_key`。

编辑时 Key 留空复用服务端已有值，显式清除优先于传入新 Key。复用旧 Key 时不能同时变更 Base URL，须重新输入或显式清除。不写配置、不调用聊天接口、不创建检测任务；超时受模型列表超时设置控制且不超过 30 秒。

前端 `ModelPicker` 使用双列标签、模型数量与可搜索勾选列表。同步结果与当前选择合并去重，保留手动模型；清空 `models` 恢复检测时自动发现全部模型。切换连接参数或关闭弹窗会取消未完成同步。

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

Prometheus 指标端点。仅在后端通过 `SetMetrics` 启用后可用，否则 404。需有效会话 Cookie；不再支持旧静态 Bearer，抓取方需要管理和更新登录会话。

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
  "unknown_count": 0,
  "stale_after_seconds": 600,
  "provider_count": 3,
  "providers": ["ProviderReport..."],
  "provider_errors": [{ "provider_id": "x", "provider_type": "openai", "error": "..." }],
  "overall_status": "DEGRADED",
  "overall_class": "error",
  "history_size": 96,
  "stats_window_days": 7,
  "theme": "light",
  "theme_label": "白天"
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
  "ok_count": 2, "slow_count": 0, "error_count": 0, "unknown_count": 0,
  "checked_at": "2024-01-01T12:00:00Z",
  "status": "ok", "status_label": "正常", "model_count": 2
}
```

`ModelResult` 关键字段：

| 字段 | 说明 |
|---|---|
| `model` / `current_model` / `is_current` | 模型名、Provider 当前模型、是否为当前模型 |
| `status` / `status_label` / `status_class` | `ok` / `slow` / `error` / `unknown` 及展示文案/样式类；unknown 表示发现失败后未检测到已知模型 |
| `checked_at` | 实际探测结束时间，UTC RFC3339；单 Provider 重测不会刷新其他 Provider 的时间 |
| `latency_ms` | 本次延迟（毫秒） |
| `response_preview` | 响应预览（已剥离 `<think>` 标签） |
| `error` | 错误信息 |
| `history` | 历史状态序列（用于 LED 状态灯） |
| `show_curve_chart` | 是否展示延迟曲线 |
| `svg_path_line` / `svg_path_area` / `time_labels` | 后端预计算的延迟曲线 SVG 路径与时间轴 |
| `avg_latency_24h` / `p50_latency_24h` / `p95_latency_24h` / `p99_latency_24h` / `latency_samples_24h` | 24h 延迟统计（字符串为已格式化文案） |
| `weekly_success_text` / `availability` | 统计窗口检测成功率文案，成功包括 ok/slow，分母包括 unknown |

### 6.3 `RuntimeSettings`（运行时设置）

仪表盘与检测参数：标题、超时（`timeout_seconds` / `model_list_timeout_seconds`）、慢阈值（`slow_threshold_ms`）、并发（`concurrency` / `provider_concurrency`）、模型过滤（`max_models_per_provider` / `skip_models`）、历史与曲线（`enable_history` / `show_curve_chart` / `history_size` / `max_history_records`）、统计窗口（`stats_window_days`）、错误详情开关（`show_error_detail`）、主题（`theme_mode` / `day_mode_start_hour` / `day_mode_end_hour`）、自动检测区间（`auto_check_interval_min_hours` / `auto_check_interval_max_hours`）、告警（`notify_*`，见 5.3 敏感字段策略）。

还包含 `status_login_required`（状态监控需要登录，默认 false）。完整字段列表见 `frontend/src/types.ts` 的 `RuntimeSettings`。

`notify_platform: "disabled"` 明确禁用通知并保留凭据；历史空值仍按 webhook 处理。公开报告和 SSE 每次输出按当前 `show_error_detail` 过滤，不依赖保存报告时的开关值。

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

1. **统一入口**：所有请求走 `frontend/src/api.ts` 的 `api` 对象，自动携带 Cookie 与写请求 CSRF 头；错误统一从 `error` 字段提取。
2. **登录态**：`AuthProvider` 启动、窗口聚焦和每 30 秒读取 session；按角色、初始改密标记及监控访问开关守卫路由。登录、改密后接受新的 session，忽略过期的并发刷新结果。
3. **实时刷新**：仪表盘优先 SSE（`/api/events`），网络失败自动降级轮询 `/api/status`，`auth-required` 触发会话刷新。
4. **长耗时操作**：触发检测类接口（start / check / rerun）同步执行，前端需有 loading 态；409 表示已有任务在跑。
5. **敏感字段不回显**：api_key、告警 webhook/bot token 等只显示"已设置"，编辑时留空即不变。
6. **URL 编码**：Provider id 允许特殊字符，拼接路径时必须 `encodeURIComponent`（`api.ts` 已处理）。
