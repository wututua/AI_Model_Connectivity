# HTTP API 参考

基础路径默认为 `http://127.0.0.1:8080`。所有响应为 UTF-8 JSON；受认证接口额外返回 `Cache-Control: no-store`。

## 1. 认证

```
Authorization: Bearer <ADMIN_TOKEN 或 只读分享密钥>
```

- 管理密钥：全部 `/api/admin/*`。
- 只读密钥：仅 `GET /api/admin/detection`、`GET /api/admin/providers`、`GET /api/admin/tasks`、`GET /api/admin/tasks/{id}`、`GET /api/admin/billing`、`GET /metrics`。
- 只读密钥访问写接口返回 `403`（不计入失败限流）。
- 同一来源 IP 一分钟内 10 次认证失败后返回 `429`，响应带 `Retry-After`（秒）。

## 2. 错误码

| 状态码 | 场景 |
|--------|------|
| 400 | 请求体非法、参数校验失败 |
| 401 | 未认证或密钥错误 |
| 403 | 只读密钥访问写接口 / 公网监听未配置 `ADMIN_TOKEN` |
| 404 | `/api/status` 尚无报告；`/metrics` 未启用；任务不存在 |
| 405 | 方法不允许 |
| 409 | 已有检测任务运行（body: `check already running`） |
| 413 | 请求体超过 1 MiB |
| 429 | 认证失败限流 |
| 500 | 服务端错误 |

错误体统一为 `{"ok": false, "error": "..."}`。

---

## 3. 公开接口

### `GET /health`

```json
{ "ok": true }
```

容器健康检查入口（也可执行 `model-connectivity healthcheck`）。

### `GET /api/status`

返回最新 `Report`。无报告时返回 `404` + `{"ok":false,"error":"no report available"}`，前端展示空态并引导手动触发检测。

### `GET /api/events`（SSE）

`Content-Type: text/event-stream`。连接建立后先补发一次最新报告，之后每次检测完成推送 `data: <Report JSON>\n\n`，每 25 秒发送 `: keep-alive`。

### `GET /`、`GET /admin`

Web 静态资源。非 `/api/` 且磁盘上无对应文件的路径回退到 `index.html`，交给前端路由。

---

## 4. 检测控制

### `GET /api/admin/detection`（只读可用）

```json
{
  "running": false,
  "task_id": 0,
  "kind": "manual",
  "provider_id": "",
  "auto_check_interval_min_hours": 6,
  "auto_check_interval_max_hours": 12,
  "first_use": false,
  "read_only": false
}
```

前端用它做登录校验：`401` 停留在密钥输入页，`first_use=true` 强制改密，`read_only=true` 隐藏写操作标签页。

### `POST /api/admin/detection/start`、`POST /api/admin/check`

触发一次全量检测，**同步执行**（服务端上下文超时 30 分钟），返回 `{"ok":true,"report":{...}}`。已有任务运行时返回 `409`。

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/admin/check
```

### `POST /api/admin/detection/stop`

停止当前检测，返回 `{"ok":true,"stopped":true}`。未完成任务标记 `canceled`，不更新报告/历史/告警。

---

## 5. 密钥管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/api/admin/token` | `{"token":"新密钥"}`，16–256 位；外部已配置 `ADMIN_TOKEN` 时拒绝 |
| `GET` | `/api/admin/view-token` | 返回 `{"ok":true,"token":"..."}`，未设置为空串 |
| `POST` | `/api/admin/view-token` | 设置/轮换只读密钥；空 body 或空 token 时后端生成 16 字节随机值 |
| `DELETE` | `/api/admin/view-token` | 吊销只读密钥，立即失效 |

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{}' http://127.0.0.1:8080/api/admin/view-token
# {"ok":true,"token":"xxxx..."}
```

---

## 6. 配置

### `GET /api/admin/config`（仅管理）

返回 `{"settings": RuntimeSettings, "providers": SafeProviderConfig[]}`。Provider 只返回 `api_key_set` 布尔，通知凭据只返回 `*_set`。

### `PUT /api/admin/settings`

请求完整 `RuntimeSettings`，返回更新后的 `AdminConfig`。

```bash
curl -X PUT -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"slow_threshold_ms":1200,"timeout_seconds":30,"concurrency":4,"provider_concurrency":2,"notify_platform":"dingtalk","notify_webhook_url":"https://oapi.dingtalk.com/robot/send?access_token=xxx"}' \
  http://127.0.0.1:8080/api/admin/settings
```

### `GET /api/admin/config/export`、`POST /api/admin/config/import`、`POST /api/admin/config/reload`

- 导出：`{"settings":…,"providers":[…]}`，不含 API Key 与通知凭据。
- 导入：`{"settings":…,"providers":[ProviderUpdate…]}`。
- 重载：重读 `.env`，成功后异步触发一次检测；监听地址/路径/`ADMIN_TOKEN` 变更会被拒绝。

---

## 7. Provider 管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/admin/providers` | 列表（只读可用） |
| `POST` | `/api/admin/providers` | 新增 |
| `PUT` | `/api/admin/providers/{id}` | 修改（`api_key` 空保留，`clear_api_key` 清除） |
| `DELETE` | `/api/admin/providers/{id}` | 删除 |
| `POST` | `/api/admin/providers/{id}/rerun` | 单独重跑（同步，30 分钟超时） |

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"id":"ollama-local","name":"Ollama","type":"ollama","base_url":"http://127.0.0.1:11434/v1","models":["llama3.1"],"enabled":true,"probe_enabled":true}' \
  http://127.0.0.1:8080/api/admin/providers
```

`{id}` 需 URL 编码。单 Provider 重跑会用 `report.MergeProvider` 把结果合并回上次完整报告，其余 Provider 数据保持不变。

---

## 8. 任务历史

### `GET /api/admin/tasks?limit=&offset=&status=&provider_id=`（只读可用）

`limit` 默认 50、上限 200；`offset` ≥ 0；按 `started_at DESC, id DESC` 返回 `CheckTask[]`。

### `GET /api/admin/tasks/{id}`（只读可用）

`CheckTask` 字段：`id`、`kind`（manual/scheduled/startup/provider）、`status`（running/success/error/canceled）、`provider_id`、`started_at`、`finished_at`、`elapsed_ms`、`ok_count`、`slow_count`、`error_count`、`total`、`error_message`、`report_generated_at`。

---

## 9. 用量统计

### `GET /api/admin/billing?days=30`（只读可用）

`days` 默认 30、上限 365，返回 `BillingSummary`：`range_days`、`range_start`、`range_end`、`total_*`、`per_model[]`、`daily[]`。统计按 UTC 自然日，最多保留 365 天。

```bash
curl -H "Authorization: Bearer $TOKEN" 'http://127.0.0.1:8080/api/admin/billing?days=7'
```

---

## 10. Prometheus 指标

### `GET /metrics`（只读可用）

需携带认证头。未通过 `SetMetrics` 启用时返回 `404`。指标清单见 [operations.md](operations.md#2-prometheus-指标)。

---

## 11. 请求体限制

- 上限 1 MiB，超限返回 `413`。
- 必须是单个 JSON 对象；多个 JSON 值或顶层非对象返回 `400`。
- `/api/admin/view-token` 的 `POST` 允许空 body。

## 12. 数据结构

字段为 snake_case，与 `frontend/src/types.ts` 完全对应：

`Report`、`ProviderReport`、`ModelResult`、`ProviderError`、`RunningState`、`RuntimeSettings`、`SafeProviderConfig`、`ProviderUpdate`、`AdminConfig`、`CheckTask`、`ConfigExport`、`ConfigImport`、`BillingSummary`。

完整字段说明与 TS 定义见 [backend-api.md](backend-api.md)。
