# HTTP API 参考

[项目首页](../README.md) · [文档索引](README.md) · [前端集成](backend-api.md) · [安全与访问控制](security.md)

基础地址默认为 `http://127.0.0.1:8080`。业务 REST 接口返回 UTF-8 JSON；SSE 使用事件流，`/metrics` 使用 Prometheus 文本格式，静态资源按文件类型响应。受认证接口返回 `Cache-Control: no-store`。

## 目录

- [认证](#认证)
- [错误码](#错误码)
- [状态与静态接口](#状态与静态接口)
- [检测控制](#检测控制)
- [账号与用户管理](#账号与用户管理)
- [配置](#配置)
- [Provider 管理](#provider-管理)
- [任务历史](#任务历史)
- [用量统计](#用量统计)
- [Prometheus 指标](#prometheus-指标)
- [请求体限制](#请求体限制)
- [数据结构](#数据结构)

## 认证

使用账号密码登录，服务端设置 `cg_session` Cookie（HttpOnly、SameSite=Strict、24 小时有效）。不再接受 Bearer Token。

- 管理员（`admin`）：全部管理接口。
- 普通用户（`user`）：可读取 detection、providers、tasks、billing、metrics，并修改自己的密码；不能修改任何共享配置或管理其他账号。
- `POST/PUT/DELETE` 等受认证写请求必须携带登录或 session 响应中的 `X-CSRF-Token`；浏览器请求还会检查来源。
- 密码至少 8 位，包含大写字母、小写字母和数字；不强制特殊字符，最多 1024 字节。
- 同一来源 IP 一分钟内 10 次未成功的密码验证尝试后返回 `429`，响应带 `Retry-After`（秒）；密码验证还有全局并发限制。

### 登录示例

以下命令使用 Bash。在受限的本地目录操作，`login.json` 内容为：

```json
{"username":"admin","password":"<替换为当前密码>"}
```

限制凭据文件权限后登录，Cookie 文件由 curl 创建：

```bash
umask 077
chmod 600 login.json
curl -c cookies.txt -H 'Content-Type: application/json' \
  --data-binary @login.json \
  http://127.0.0.1:8080/api/auth/login
```

将响应中的 `csrf_token` 设为后续请求使用的 `CSRF`：

```bash
CSRF='<替换为响应中的 csrf_token>'
```

若 `user.must_change_password=true`，先调用 `/api/auth/password`。受限的 `password.json` 内容为：

```json
{"current_password":"<当前密码>","password":"<符合规则的新密码>"}
```

```bash
chmod 600 password.json
curl -X POST -b cookies.txt -c cookies.txt \
  -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
  --data-binary @password.json \
  http://127.0.0.1:8080/api/auth/password
```

改密后会话和 CSRF 都会轮换，更新 `CSRF` 再调用管理接口。示例文件与 Cookie 不可提交到 Git，也不要在共享终端记录真实密码；操作结束后注销并清理本地凭据文件。Windows 可使用 `curl.exe`，并通过 ACL 保护文件。

## 错误码

| 状态码 | 场景 |
|--------|------|
| 400 | 请求体非法、参数校验失败 |
| 401 | 未登录、账号密码错误或会话失效 |
| 403 | 权限不足、CSRF/来源校验失败或需要修改初始密码 |
| 404 | 任务或 Provider 不存在；自定义服务未注册指标 |
| 405 | 方法不允许 |
| 409 | 已有检测任务运行（body: `check already running`）或账号已并发变更 |
| 413 | 请求体超过 1 MiB |
| 415 | 登录请求未使用 `application/json` |
| 429 | 认证失败限流 |
| 500 | 服务端错误 |
| 503 | 服务关闭中，或认证、访问策略暂时不可用 |

错误体包含 `error`。强制改密返回 `{"error":"请先修改初始密码","code":"password_change_required"}`；认证依赖暂时不可用返回 `503`。

## 状态与静态接口

### `GET /health`

```json
{ "ok": true }
```

容器健康检查入口（也可执行 `model-connectivity healthcheck`），不检查上游模型、账单或报告时效。

### `GET /api/status`

返回最新 `Report`。`status_login_required=true` 时需要已完成初始改密的普通用户或管理员会话，否则允许匿名读取。首次安装也返回 `200`：`state=unconfigured` 表示无启用 Provider，`pending` 表示等待首次检测，`ready` 表示已有报告；未检测时 `generated_at=""`。Provider 删除、停用、暂停与名称变更立即投影到当前状态及 SSE，不伪造新的检测时间。

### `GET /api/events`（SSE）

`Content-Type: text/event-stream`。访问策略与 `/api/status` 相同。连接建立后先补发一次最新报告，之后每次检测完成推送 `data: <Report JSON>\n\n`，每 5 秒发送 `: keep-alive`。发送报告和心跳前重新校验权限；会话失效或匿名访问被关闭时发送 `event: auth-required` 并断开。

### `GET /`、`GET /admin`、`GET /login`

Web 静态资源。非 `/api/` 且磁盘上无对应文件的路径回退到 `index.html`，交给前端路由。

## 检测控制

### `GET /api/admin/detection`（只读可用）

```json
{
  "running": false,
  "task_id": 0,
  "kind": "",
  "provider_id": "",
  "auto_check_interval_min_hours": 6,
  "auto_check_interval_max_hours": 12,
  "read_only": false
}
```

`read_only=true` 表示普通用户。登录状态与初始改密要求由 `/api/auth/session` 返回。

### `POST /api/admin/detection/start`、`POST /api/admin/check`

触发一次全量检测，立即返回 **202 Accepted**、`{"ok":true,"task":CheckTask}` 和 `Location: /api/admin/tasks/{id}`。任务由服务端后台执行，最长 30 分钟；浏览器关闭、切换页面或断开连接不会取消已接受的任务。通过任务详情查询 `running/success/error/canceled`，完成后读取 `/api/status` 或 SSE。已有任务运行时返回 `409`，服务关闭中返回 `503`。

```bash
curl -X POST -b cookies.txt -H "X-CSRF-Token: $CSRF" http://127.0.0.1:8080/api/admin/check
```

### `POST /api/admin/detection/stop`

仅保留管理员 API，前端不显示停止按钮。返回 `{"ok":true,"stopped":true}`。未完成任务标记 `canceled`，不更新报告/历史/告警，但已确认响应的用量仍会入账。正常停服也会取消并收尾后台任务。

## 账号与用户管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/auth/session` | 返回当前会话；匿名时 `user=null`，仍返回状态页访问策略 |
| `POST` | `/api/auth/login` | `{"username":"admin","password":"..."}`；返回会话并设置 Cookie |
| `POST` | `/api/auth/logout` | 注销当前会话并清除 Cookie |
| `POST` | `/api/auth/password` | `{"current_password":"...","password":"..."}`；吊销该用户旧会话并签发新会话 |
| `GET` | `/api/admin/users` | 仅管理员，返回 `User[]` |
| `POST` | `/api/admin/users` | 仅管理员，创建用户，返回 `User` |
| `PUT` | `/api/admin/users/{id}` | 仅管理员，修改用户/重置密码并吊销其会话 |
| `DELETE` | `/api/admin/users/{id}` | 仅管理员，删除用户并吊销其会话 |

会话响应：`{"user":User|null,"csrf_token":"...","expires_at":Unix秒,"status_login_required":false}`。
`User` 字段：`id`、`username`、`role`（`admin`/`user`）、`enabled`、`must_change_password`、`created_at`；不会返回密码或密码哈希。

创建与修改用户请求包含 `username`、`password`、`role`、`enabled`。用户名为 3–32 位 ASCII 字母、数字、点、下划线或短横线，字母/数字开头，忽略大小写。修改时密码留空表示不变。初始密码与管理员重置密码均要求用户首次登录后修改。

不能在用户管理中修改或删除当前账号，也不能禁用、删除或降级最后一个启用的管理员。个人改密使用 `/api/auth/password`。

创建普通用户的 JSON 请求示例，请使用自己生成的密码并保护请求文件：

```json
{"username":"viewer","password":"<符合规则的随机初始密码>","role":"user","enabled":true}
```

## 配置

### `GET /api/admin/config`（仅管理）

返回 `{"settings": RuntimeSettings, "providers": SafeProviderConfig[]}`。Provider 只返回 `api_key_set` 布尔，通知凭据只返回 `*_set`。

### `PUT /api/admin/settings`

请求完整 `RuntimeSettings`，返回更新后的 `AdminConfig`。`status_login_required` 控制监控页是否要求登录，保存后立即生效。

```bash
# settings.json 应来自当前 config 响应的 settings，修改需要变更的字段后完整提交。
curl -X PUT -b cookies.txt -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
  --data-binary @settings.json \
  http://127.0.0.1:8080/api/admin/settings
```

### `GET /api/admin/config/export`、`POST /api/admin/config/import`

- 导出：`{"settings":…,"providers":[…]}`，不含 API Key 与通知凭据，也不包含用户、密码和会话。
- 导入：`{"settings":…,"providers":[ProviderUpdate…]}`，整体替换运行配置，不是增量合并；先保存现有配置并检查 Provider 列表。

敏感字段留空表示保留，显式 `clear_*=true` 清除；在全新数据库导入时需另行补充凭据。配置导出不含历史与用量，不能替代数据库备份。

## Provider 管理

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/admin/providers` | 列表（只读可用） |
| `POST` | `/api/admin/providers` | 新增 |
| `PUT` | `/api/admin/providers/{id}` | 修改（`api_key` 空保留，`clear_api_key` 清除） |
| `DELETE` | `/api/admin/providers/{id}` | 删除 |
| `POST` | `/api/admin/providers/{id}/rerun` | 单独重跑，返回 202 和 task，后台最长 30 分钟；不存在、停用或暂停的 Provider 返回 400 |
| `POST` | `/api/admin/provider-models` | 仅管理员，读取编辑草稿对应的上游模型列表，不保存配置、不触发探测 |

模型同步请求为 `{"provider_id":"已保存的ID，可省略","type":"openai","base_url":"https://example.test/v1","api_key":"","clear_api_key":false}`，返回模型 ID 数组。新 Provider 可不传 `provider_id`，无需先保存。已有 Provider 的 `api_key` 留空时复用存储的 Key；若同时变更 Base URL，必须重新填写 Key 或显式清除，避免把旧凭据发送到新地址。

新增时 ID 忽略大小写判重，重复返回 `409`，不会覆盖已有配置；修改或删除不存在的 ID 返回 `404`。上述 Base URL 与密钥校验同样适用于保存及配置导入，校验失败返回 `400`，整次更新不生效。

连接地址、密钥或协议改变后，该 Provider 的当前结果立即变为“未检测”，旧检测时间、延迟和错误信息清空；历史统计及已产生的用量保留。仅修改名称不使结果失效。状态快照携带随机的 `connection_revision`，它不是密钥哈希；旧版本正在运行的检测不能恢复当前健康状态，包括改回原值或删除后重建的情况。

同步使用现有安全 HTTP 客户端，调用 `{base_url}/models`，超时为 `model_list_timeout_seconds` 且最多 30 秒。该接口同样要求会话与 CSRF，不返回凭据；前端只在保存 Provider 时提交选定的 `models`。

```bash
curl -X POST -b cookies.txt -H "X-CSRF-Token: $CSRF" -H 'Content-Type: application/json' \
  -d '{"id":"ollama-local","name":"Ollama","type":"ollama","base_url":"http://127.0.0.1:11434/v1","models":["llama3.1"],"enabled":true,"probe_enabled":true}' \
  http://127.0.0.1:8080/api/admin/providers
```

`{id}` 需 URL 编码。单 Provider 重跑会用 `report.MergeProvider` 把结果合并回上次完整报告，其余 Provider 数据保持不变。

## 任务历史

### `GET /api/admin/tasks?limit=&offset=&status=&provider_id=`（只读可用）

`limit` 默认 50、上限 200；`offset` ≥ 0；按 `started_at DESC, id DESC` 返回 `CheckTask[]`。状态值为 `running`、`success`、`error`、`canceled`，不是 `failed`。

### `GET /api/admin/tasks/{id}`（只读可用）

`CheckTask` 字段：`id`、`kind`（manual/scheduled/startup/provider）、`status`（running/success/error/canceled）、`provider_id`、`started_at`、`finished_at`、`elapsed_ms`、`ok_count`、`slow_count`、`error_count`、`total`、`error_message`、`report_generated_at`。

任务 `total` 只统计本任务实际执行的模型探测，不包含单 Provider 重测时保留的其他 Provider 结果，也不包含发现失败产生的 `unknown` 样本。服务重启后遗留的 `running` 任务会标记为 `canceled`。

报告额外提供 `unknown_count`、`stale_after_seconds` 和 Provider/模型级 `checked_at`。`generated_at` 与 `checked_at` 为带时区的 RFC3339 时间；公开报告和 SSE 始终按当前 `show_error_detail` 设置过滤错误详情。

## 用量统计

### `GET /api/admin/billing?days=30`（只读可用）

`days` 默认 30、上限 365，返回 `BillingSummary`：`range_days`、`range_start`、`range_end`、`total_*`、`per_model[]`、`daily[]`。统计按 UTC 自然日，最多保留 365 天；只计入已确认的上游用量，不能替代账单。

```bash
curl -b cookies.txt 'http://127.0.0.1:8080/api/admin/billing?days=7'
```

## Prometheus 指标

### `GET /metrics`（只读可用）

需有效会话 Cookie，即使监控页公开也需要登录。常规启动默认注册；自定义嵌入服务未调用 `SetMetrics` 时返回 `404`。旧版静态 Bearer 抓取不再可用，见[指标采集](operations.md#prometheus-指标)。

## 请求体限制

- 上限 1 MiB，超限返回 `413`。
- 必须是单个 JSON 对象；多个 JSON 值或顶层非对象返回 `400`。

## 数据结构

字段为 snake_case，前端使用的类型定义见 [types.ts](../frontend/src/types.ts)，完整响应以 Go JSON 定义为准，客户端应容忍额外字段：

`Report`、`ProviderReport`、`ModelResult`、`ProviderError`、`RunningState`、`RuntimeSettings`、`SafeProviderConfig`、`ProviderUpdate`、`AdminConfig`、`CheckTask`、`ConfigExport`、`ConfigImport`、`BillingSummary`、`User`、`UserInput`、`AuthSession`。

字段语义和跨请求约定见[前端集成](backend-api.md#数据类型)。报告定义位于 [report.go](../internal/report/report.go)，配置定义位于 [runtime.go](../internal/config/runtime.go)，任务与用量定义位于 [sqlite.go](../internal/storage/sqlite.go)。
