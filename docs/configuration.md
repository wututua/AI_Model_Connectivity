# 配置参考

[项目首页](../README.md) · [文档索引](README.md) · [部署指南](deployment.md) · [HTTP API](api.md)

日常配置优先使用管理面板。下表列出环境变量、默认值及其行为，适用于首次初始化或只能在启动时读取的参数。

## 目录

- [配置优先级](#配置优先级)
- [服务](#服务)
- [探测](#探测)
- [历史与展示](#历史与展示)
- [定时检测](#定时检测)
- [告警通知](#告警通知)
- [Provider 配置](#provider-配置)
- [运行时修改与重启](#运行时修改与重启)

## 配置优先级

首次启动时，基础配置按 **进程环境变量 > 代码默认值** 解析，然后把可管理的运行时设置和 Provider 写入 SQLite。后续启动时，SQLite 中的运行时设置和 Provider 会覆盖基础配置中的同名值。

程序不读取本地配置文件。无需设置任何变量即可启动；Provider、通知、检测周期等日常配置通过管理面板或 JSON 导入维护，保存到 SQLite 后重启仍然生效。环境变量不会覆盖数据库中已有的运行时设置。

启动参数的设置方式：

- Bash：`export APP_PORT=8081`，随后启动程序。
- PowerShell：`$env:APP_PORT = '8081'`，随后启动程序。
- Docker：通过 `docker run -e APP_PORT=8081` 或 Compose 的 `environment` 注入，并同步调整端口映射。
- 列表类变量支持逗号、分号和换行分隔，自动去重。

从旧版升级时，请先将文件式启动配置迁移到上述环境变量。尤其要保留 `DATA_DIR`、`DATABASE_PATH` 和 `SECURE_COOKIES` 的原值，避免连接到另一份数据库或改变安全设置；不要通过删除数据库来重新初始化配置。

| 配置来源 | 何时使用 |
| --- | --- |
| 代码默认值 | 未提供相应启动变量时 |
| 进程环境变量 | 启动参数，以及空数据库的运行配置初始化 |
| SQLite | 已保存的 Provider 和运行设置，覆盖同名环境变量 |
| 管理面板 / API | 更新 SQLite 运行设置并立即应用 |

## 服务

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_HOST` | `127.0.0.1` | 监听地址 |
| `APP_PORT` | `8080` | 监听端口，取值 1–65535 |
| `WEB_DIR` | `web` | 静态资源目录 |
| `DATA_DIR` | `data` | 数据目录 |
| `DATABASE_PATH` | `DATA_DIR/cg.sqlite` | SQLite 路径 |
| `DASHBOARD_TITLE` | `模型连通性` | 仪表盘标题 |
| `ADMIN_USERNAME` | `admin` | 仅首次创建管理员时使用 |
| `ADMIN_PASSWORD` | 自动生成或迁移 | 仅首次创建管理员时使用；至少 8 位，包含大写、小写字母和数字 |
| `SECURE_COOKIES` | `false` | HTTPS 反向代理部署设置为 true，重启生效 |
| `STATUS_LOGIN_REQUIRED` | `false` | 状态监控是否要求普通用户或管理员登录；可在系统设置修改 |

已有用户时环境账号密码不会覆盖数据库账号。旧 `ADMIN_TOKEN` 仅在用户表为空时用于迁移，不再作为 API 凭据；详见[认证说明](security.md#认证)。

## 探测

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `TIMEOUT_SECONDS` | `30` | 单模型检测超时（秒），有限正数且 ≤ 86400 |
| `MODEL_LIST_TIMEOUT_SECONDS` | `20` | 获取模型列表超时（秒），同上 |
| `SLOW_THRESHOLD_MS` | `800` | 慢阈值（毫秒），> 0 |
| `CONCURRENCY` | `1` | 全局并发上限，1–1024；默认严格串行 |
| `PROVIDER_CONCURRENCY` | `1` | 单 Provider 并发上限，1–1024 |
| `MAX_MODELS_PER_PROVIDER` | `0` | 每个 Provider 最多探测模型数，0–100000；`0` 不限制 |
| `SKIP_MODELS` | — | 跳过模型：`model`、`provider/model`、`provider::model` |
| `PROBE_PROMPT` | `ping` | 用户提示词 |
| `PROBE_SYSTEM_PROMPT` | `No thinking. Respond only with exactly: pang. No extra words.` | 系统提示词 |

未配置 Provider 探测选项时仍使用 `temperature=0`、`max_tokens=16`。Provider 编辑页可覆盖输出上限、Token 参数、temperature、提示词、超时，以及 Chat / Responses 和流式协议；字段、预设与限制见[探测协议与参数](monitoring-features.md#探测协议与参数)。界面的输出预算参考不是实际消耗或费用上限。

后端会剥离 `<think>` / `<thinking>` 等思考标签，只保留实际回复。需要不同参数或较大推理预算的模型可能失败；回复为空、缺少消息或因长度上限截断均不算成功，不应仅因 HTTP 200 判断可用。

以下新增设置通过管理面板或 API 保存，不新增对应环境变量：

| 运行时字段 | 默认 | 说明 |
| --- | --- | --- |
| `daily_request_limit` | `0` | UTC 每日模型与模型发现请求次数上限，0 不限制 |
| `discovery_model_limit` | `0` | 自动发现原始模型数超过阈值时须先选择并保存模型，0 不限制 |
| `notify_failure_threshold` | `0` | 连续异常次数，0/1 等效一次 |
| `notify_recovery_threshold` | `0` | 连续正常恢复次数，0/1 等效一次 |
| `maintenance_start` / `maintenance_end` | 空 | 成对 RFC3339 时间，窗口内只静默正式告警 |

Provider 还可保存 `group`、`tags` 和嵌套的 `probe` 选项，导出/导入会保留这些非凭据字段。旧数据库缺失的新字段自动按上述默认值解释；完整设置 PUT 和配置导入仍是替换语义，旧客户端省略这些字段会恢复其默认值。

## 历史与展示

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `ENABLE_HISTORY` | `true` | 是否记录历史；关闭后仍会计入 token 用量 |
| `SHOW_CURVE_CHART` | `true` | 是否生成延迟曲线 SVG |
| `STATS_WINDOW_DAYS` | `7` | 检测成功率统计窗口（1–3650） |
| `HISTORY_SIZE` | `30` | 仪表盘展示的历史条数（1–100000） |
| `MAX_HISTORY_RECORDS` | `500` | 每模型数据库保留记录数（1–1000000） |
| `SHOW_ERROR_DETAIL` | `true` | 是否展示错误详情；关闭后公开 API、SSE 与合并报告按当前设置清空错误文本 |
| `THEME_MODE` | `auto` | 服务端报告主题：`auto` / `dark` / `light` |
| `DAY_MODE_START_HOUR` | `8` | 服务端 `auto` 亮色起始小时（0–23） |
| `DAY_MODE_END_HOUR` | `18` | 服务端 `auto` 亮色结束小时（0–23） |

报告中的 `auto` 主题按服务端当前小时判断：起始 ≤ 结束表示同一天区间，否则视为跨天（如 18 → 次日 8）。Web 界面由浏览器独立选择深色、浅色或跟随系统，其 `auto` 使用 `prefers-color-scheme`，不会按此时间表切换。

## 定时检测

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AUTO_CHECK_INTERVAL_MIN_HOURS` | `0` | 最小间隔（小时），有限非负数且 ≤ 8760 |
| `AUTO_CHECK_INTERVAL_MAX_HOURS` | `0` | 最大间隔（小时），同上 |
| `AUTO_CHECK_RUN_ON_START` | `false` | 启动后立即检测一次 |

- 两者都为 0 表示关闭定时检测；只设一个则另一个取相同值；min > max 时自动交换。负数不属于有效配置。
- 实际间隔 = `min + rand()*(max-min)` 小时，最小 1 分钟。独立部署可利用随机间隔错峰，但不能并发共用同一数据库。
- 修改配置会唤醒调度器重新计算下一次时间。

## 告警通知

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `NOTIFY_PLATFORM` | `webhook` | `disabled`/`webhook`/`discord`/`bark`/`wecom`/`wechat_work`/`dingtalk`/`telegram`；`disabled` 禁用，空值兼容旧版 webhook |
| `NOTIFY_WEBHOOK_URL` | — | Webhook 地址（允许查询参数） |
| `NOTIFY_TELEGRAM_BOT_TOKEN` | — | Telegram Bot Token |
| `NOTIFY_TELEGRAM_CHAT_ID` | — | Telegram Chat ID |
| `NOTIFY_ON_RECOVERY` | `true` | 恢复时是否通知 |
| `NOTIFY_COOLDOWN_MINUTES` | `0` | 冷却时间（分钟），`0` 关闭 |
| `NOTIFY_PROVIDERS` | — | 仅对指定 Provider 告警（ID 或 Name） |
| `NOTIFY_MODELS` | — | 仅对指定模型告警（`model`/`provider/model`/`provider::model`） |

发送条件（实现见 [notify.go](../internal/notify/notify.go)）：

1. 平台不是 `disabled` 且配置完整（telegram 需 token + chat_id，其他需 webhook URL）；
2. 按 `NOTIFY_PROVIDERS` / `NOTIFY_MODELS` 过滤后重新计算聚合状态；
3. 范围内存在没有模型结果的未检测 Provider，或范围为空/全部暂停且没有明确错误时，不发送通知，也不更新上次告警状态和冷却时间；
4. 与上次告警状态 `ok`/`slow`/`error` **不同**才发；已知模型的 `unknown` 结果和明确的模型发现失败仍按异常处理；
5. `ok` 且上次状态为空（首次启动）不发；
6. `ok` 且 `NOTIFY_ON_RECOVERY=false` 只落状态不发消息；
7. 冷却期内不发，且不更新状态（下一轮仍可发）。

两个过滤条件同时设置时取交集。按模型过滤时，模型发现失败只计入已匹配模型结果的 Provider，或被带 Provider 前缀的模型条件明确选中的 Provider。仅填写裸模型名且该 Provider 没有任何匹配结果时，不推断其属于通知范围。无关 Provider 的发现失败不会触发该范围的告警；没有检测证据也不会被当作恢复。

各平台消息体：

| 平台 | JSON |
|------|------|
| `discord` | `{"content": "<text>"}` |
| `bark` | `{"title": "<title>", "body": "<text>"}` |
| `wecom` / `wechat_work` | `{"msgtype":"text","text":{"content":"<text>"}}` |
| `dingtalk` | `{"msgtype":"text","text":{"content":"<text>"}}` |
| `webhook` | 完整 payload（`status`/`summary`/`provider_text` 等） |
| `telegram` | 请求 `https://api.telegram.org/bot<token>/sendMessage`，`{"chat_id":…,"text":…}` |

文本超过 10 行 Provider 明细会折叠为「其余 N 项已省略」。

保存配置后可在 **通知记录** 页发送测试通知。测试与手动重试不经过状态变化、范围和冷却判断，不调用模型，也不推进正式告警状态；平台禁用或配置不完整时仍会拒绝。它们验证的是已保存的通知渠道，不验证模型告警范围，详见[告警验证](operations.md#告警验证)。

## Provider 配置

日常管理请使用后台 **Provider** 页面。下面的 Bash 环境变量示例仅用于首次初始化空数据库：

```bash
export PROVIDER_1_ID=openai-main
export PROVIDER_1_NAME=OpenAI
export PROVIDER_1_TYPE=openai
export PROVIDER_1_BASE_URL=https://api.openai.com/v1
export PROVIDER_1_API_KEY='<替换为实际密钥>'
export PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
export PROVIDER_1_ENABLED=true
export PROVIDER_1_PROBE_ENABLED=true
go run ./cmd/cg
```

- 编号从 `1` 开始连续递增；某一组所有字段都为空即停止读取。
- `ID` 为空时自动填 `provider-<序号>`，`NAME` 为空取 `ID`，`TYPE` 为空取 `openai`。
- `BASE_URL` 尾部 `/` 会被裁掉。
- `MODELS` 留空时，每次检测前请求 `{BASE_URL}/models` 自动发现；请求失败会产生 `ProviderError`。

| 开关组合 | 行为 |
|----------|------|
| `ENABLED=true, PROBE_ENABLED=true` | 正常展示并探测 |
| `ENABLED=true, PROBE_ENABLED=false` | 仪表盘显示「已暂停」，不发送探测请求 |
| `ENABLED=false` | 完全移除，不展示不探测 |

### 校验规则

- `ID`：非空、≤128 字符、无首尾空格、不含控制字符与 `/ \ ? #`、不得为 `.` 或 `..`；大小写不敏感去重。
- `BASE_URL`：必须 `http`/`https`；必须有 host；**不允许**用户信息（userinfo）、查询参数、fragment；禁止链路本地地址（169.254.0.0/16）。
- `NOTIFY_WEBHOOK_URL`：同上，但**允许**查询参数（钉钉/企业微信需要）。
- 账号：3–32 位 ASCII 字母、数字、点、下划线或短横线，以字母/数字开头，忽略大小写。
- 密码：至少 8 位，必须包含 ASCII 大写字母、小写字母和数字，不强制特殊字符，最多 1024 字节。

私网与回环地址允许用于自托管 Provider，这不是完整的出站隔离。详细边界见[网络访问](security.md#网络访问)。

### 图标匹配

`provider.IconFor(id, type, name)` 依次用 ID、TYPE、NAME 小写精确匹配内置图标表，再按关键词匹配。完整映射见 [icons.go](../internal/provider/icons.go)，未命中时前端使用名称占位。品牌图标不代表支持该服务商的原生协议，检测仍使用 OpenAI 兼容接口。

## 运行时修改与重启

| 入口 | 行为 |
|------|------|
| 管理面板 **设置** / `PUT /api/admin/settings` | 校验后写入 SQLite 并立即生效，同时唤醒调度器 |
| 管理面板 **Provider** / `POST,PUT,DELETE /api/admin/providers` | 同上；`api_key` 空表示保留旧值，`clear_api_key=true` 清除 |
| `POST /api/admin/config/import` | 整体替换 settings + providers，导出文件不包含真实凭据 |

监听地址、`WEB_DIR`、`DATA_DIR`、`DATABASE_PATH`、`SECURE_COOKIES` 通过环境变量设置，变更后必须重启。`ADMIN_USERNAME` 和 `ADMIN_PASSWORD` 仅供首次初始化；改环境变量或重启都不会重置已有账号。

`STATUS_LOGIN_REQUIRED` 属于运行时设置，SQLite 保存值优先于环境变量。管理员可在 **系统设置 → 访问控制** 修改；它同时保护状态 REST 和 SSE，不只是前端页面。用户管理独立于配置导入导出。

`PROBE_PROMPT`、`PROBE_SYSTEM_PROMPT` 和 `AUTO_CHECK_RUN_ON_START` 不属于 SQLite `RuntimeSettings`，只在进程启动时读取。修改提示词需更新环境变量并重启。已保存的 Provider 由 SQLite 管理，只能通过后台或 JSON 导入更新，不会被后续启动环境变量替换。

敏感字段写策略：告警 webhook / Telegram token / chat id 在 GET 接口只返回 `*_set` 布尔；PUT 时留空表示保持不变，传 `clear_*=true` 表示清除。

已有 Provider 更换 Base URL 时，必须重新填写 Key 或显式清除；此规则同样适用于配置导入及模型同步。运行设置接口提交完整对象，不是部分 PATCH，详见[配置 API](api.md#配置)。
