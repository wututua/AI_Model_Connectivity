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
- [通知记录](#通知记录)
- [用量统计](#用量统计)
- [模型重测与批量管理](#模型重测与批量管理)
- [预算与导出](#预算与导出)
- [Prometheus 指标](#prometheus-指标)
- [请求体限制](#请求体限制)
- [系统更新](#系统更新)
- [数据结构](#数据结构)

## 认证

管理 API 使用账号密码登录，服务端设置 `cg_session` Cookie（HttpOnly、SameSite=Strict、24 小时有效）。旧管理 Bearer Token 无效；新增指标凭据仅能通过 Bearer 读取 `/metrics`，不能访问其他 API。

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
| 413 | 请求体超过 1 MiB，或导出超过 10000 行 |
| 415 | 登录请求未使用 `application/json` |
| 429 | 认证失败限流，或每日上游请求预算用尽 |
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

运行时还返回 `elapsed_ms` 和 `progress`：`phase`（`discovering/probing/saving/notifying`）、`provider_id`（发现阶段）、`total`、`completed`、`active`（`provider_id` / `model` 数组）。空闲时阶段为 `idle`。进度不包含部分正式结果，不替代任务终态或报告。

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

## 通知记录

以下接口仅限已完成初始改密的管理员；写请求需要 CSRF，均不接受临时渠道或凭据参数：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/api/admin/notifications?limit=20&offset=0&status=` | 返回 `NotificationDelivery[]`，按 ID 倒序 |
| `POST` | `/api/admin/notifications/test` | 向当前已保存的渠道发送测试通知 |
| `POST` | `/api/admin/notifications/{id}/retry` | 向当前已保存的渠道重发失败或结果未知记录的历史摘要 |

`limit` 为 1–200，默认 20；`offset` 非负；`status` 可为空或 `sending` / `success` / `error` / `unknown`，非法参数返回 400。

发送接口同步等待平台响应，网络请求最长 10 秒。已完成并记录的尝试返回 **200 与记录本身**，须检查 `status`：HTTP 200 不保证通知发送成功，平台可能在 HTTP 200 回执中明确拒绝。`success` 仅表示平台已接受，不代表最终用户已收到。

未启用或缺少配置返回 400，记录不存在或已过期返回 404，并发手动发送或重试成功/进行中记录返回 409。记录写入失败返回 500；如果发送已发生但结果落库失败，接收结果可能未知，切勿盲目自动重试。

记录字段：`id`、`kind`（`alert` / `test` / `retry`）、`retry_of`（原记录 ID，非重试为 0）、`platform`、`status`、`created_at`、`finished_at`、`elapsed_ms`、`http_status`（没有响应时为 0）、`summary`、`error_message`。不包含渠道 URL、凭据、平台原始回执或上游错误正文。

测试和重试不调用模型，不修改正式告警状态或冷却时间。重试生成独立记录，并明确标记为历史摘要，不完整重放旧消息详情；使用的是请求开始时已保存的渠道，而非历史地址。超时或断线可能发生在平台已经接收之后，重发可能重复。

只有实际尝试发送时才建立记录；过滤、冷却和状态未变化导致的跳过不记入历史。新增记录时清理 90 天之前和最近 1,000 条之外的非进行中记录；重启遗留 `sending` 标为 `unknown`，不会自动重发。

## 用量统计

### `GET /api/admin/billing?days=30`（只读可用）

`days` 默认 30、上限 365，返回 `BillingSummary`：`range_days`、`range_start`、`range_end`、`total_*`、`per_model[]`、`daily[]`。统计按 UTC 自然日，最多保留 365 天；只计入已确认的上游用量，不能替代账单。

```bash
curl -b cookies.txt 'http://127.0.0.1:8080/api/admin/billing?days=7'
```

## 模型重测与批量管理

### `POST /api/admin/detection/selected`

仅管理员。指定模型：

```json
{"targets":[{"provider_id":"openai-main","model":"model-a"}]}
```

或重测最新报告失败项：

```json
{"failed_only":true,"provider_id":"openai-main"}
```

`provider_id` 可省略以选择所有 Provider 的失败模型；`targets` 与 `failed_only` 互斥，每次 1–1000 个模型。仅当前有效检测范围内的模型可选，失败项仅指 `error`。返回 `202` / `AcceptedCheck`，任务类型 `models` 或 `failed`；`400` 表示选择为空或无效，`409` 已运行，`429` 预算用尽，`503` 关闭中。模型重测不清除发现错误，不刷新其他模型的结果或时间，不参与正式告警。

### `POST /api/admin/providers/batch`

仅管理员，原子更新 1–1000 个 Provider：

```json
{"ids":["provider-a","provider-b"],"action":"group","group":"production"}
```

`action` 支持 `enable`、`disable`、`pause`、`resume`、`group`。启用和参与检测为独立开关，`enable` 不会顺便清除暂停。返回更新后的 `AdminConfig`；无效 ID 时整次失败。复制配置由前端使用现有创建接口完成，不复制密钥。

## 预算与导出

均仅允许管理员：

| 接口 | 响应 |
| --- | --- |
| `GET /api/admin/budget` | `day`、`used`、`limit`、`remaining`、`exhausted`、`resets_at`；limit=0 表示不限 |
| `GET /api/admin/export?kind=history&start=2026-10-01&end=2026-10-06&provider_id=example&model=model-a` | 带 BOM 的 UTF-8 CSV |
| `GET /api/admin/diagnostics` | 明确白名单的脱敏诊断 JSON |

导出 `kind` 为 `history` / `usage`，起止日期必填、UTC 且含结束日，最多 366 天、10000 行。Provider 和模型过滤为可选的精确匹配。超过行数返回 `413`，不返回截断文件。历史导出不含原始错误/响应，诊断不含名称、地址、提示词、用户或凭据。详见[数据导出](monitoring-features.md#数据导出)。

## Prometheus 指标

### `GET /metrics`（只读可用）

需有效会话 Cookie，或独立指标凭据的 `Authorization: Bearer <token>` 请求头；不接受 URL 查询参数。凭据只对 `GET /metrics` 生效。常规启动默认注册；自定义嵌入服务未调用 `SetMetrics` 时返回 `404`。旧管理 Bearer 无效，见[指标采集](operations.md#prometheus-指标)。

### 指标凭据管理

仅管理员会话可访问，写操作需要 CSRF：

| 接口 | 行为 |
| --- | --- |
| `GET /api/admin/metrics-tokens` | 列出 ID、名称、创建/轮换时间，不返回凭据或哈希 |
| `POST /api/admin/metrics-tokens` | body 为 `{"name":"prometheus"}`，返回 `201` 和含 `token` 的新凭据 |
| `POST /api/admin/metrics-tokens/{id}/rotate` | 原子轮换，返回 `200` 和新的 `token`，旧值立即失效 |
| `DELETE /api/admin/metrics-tokens/{id}` | 撤销，返回 `{"ok":true}` |

名称 1–64 字符，最多 20 个凭据。完整值仅创建/轮换时返回，服务级凭据无自动过期，不随管理员改密而失效，需显式轮换/撤销。不存在的 ID 返回 `404`。

## 系统更新

仅管理员会话可用，POST 需要 CSRF：

| 接口 | 行为 |
| --- | --- |
| `GET /api/admin/updates` | 当前版本、commit、平台、部署能力、最近更新任务和一次性 `request_id`；不请求 GitHub |
| `POST /api/admin/updates/check` | `{"channel":"stable"}` 或 `{"channel":"preview"}`，返回发布时间、说明、版本、附件可用性；成功结果缓存 5 分钟 |
| `POST /api/admin/updates/start` | `{"channel":"preview","version":"v1.0.0-rc.1","confirm":true,"request_id":"状态接口返回的值"}`，接受后返回 `202` 和更新任务，任务 `id` 等于 `request_id` |
| `POST /api/admin/updates/resolve` | `{"request_id":"本次提交使用的值"}`，使尚未入队的旧请求失效，并返回最新状态及新的提交凭据；不会启动或重试安装 |

版本不是检查到的更高版本、通道不合法或缺少提交凭据返回 `400`；凭据已失效、部署不支持、检测正在运行或已有未确认更新返回 `409`；网络、状态文件或执行器不可用返回 `503`。`202` 只代表请求已排队，不代表更新成功。执行器可能在应用停服期间更新状态，重新连接后继续查询。

`request_id` 是 32 位十六进制的一次性凭据，排队、结果确认或应用重启后失效。同一时刻不同管理页面可能拿到相同凭据，只有最先受理的请求能入队。提交期间可继续 GET 查询状态。提交超时、断线或收到 `5xx` 后，调用 `resolve` 确认结果，不要自动重发 `start`。结果中的任务 ID、版本和通道匹配时继续跟踪任务；不匹配时，旧请求已不能再入队，但它也可能已结束并被更晚任务覆盖，或被另一管理页面的提交抢先受理。确认接口失败时保持结果未知，待恢复连接后重试确认。再次安装必须由用户重新确认，并使用最新凭据。

任务状态为 `pending`、`running`、`succeeded`、`failed`、`rolled_back` 或 `recovery_required`。`stage` 提供核对、下载、校验、备份、安装、重启及恢复阶段。断电后持续 `running` 不表示仍有进程运行，需结合 systemd 日志排查。详见[系统更新](system-updates.md)。

## 请求体限制

- 上限 1 MiB，超限返回 `413`。
- 必须是单个 JSON 对象；多个 JSON 值或顶层非对象返回 `400`。

## 数据结构

字段为 snake_case，前端使用的类型定义见 [types.ts](../frontend/src/types.ts)，完整响应以 Go JSON 定义为准，客户端应容忍额外字段：

`Report`、`ProviderReport`、`ModelResult`、`ProviderError`、`RunningState`、`RuntimeSettings`、`SafeProviderConfig`、`ProviderUpdate`、`AdminConfig`、`CheckTask`、`ConfigExport`、`ConfigImport`、`BillingSummary`、`User`、`UserInput`、`AuthSession`。

字段语义和跨请求约定见[前端集成](backend-api.md#数据类型)。报告定义位于 [report.go](../internal/report/report.go)，配置定义位于 [runtime.go](../internal/config/runtime.go)，任务与用量定义位于 [sqlite.go](../internal/storage/sqlite.go)。
