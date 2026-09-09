# 配置参考

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) · [仓库与发布](repositories.md)

首次启动时，基础配置按 **真实环境变量 > `.env` 文件 > 代码默认值** 解析，然后把可管理的运行时设置和 Provider 写入 SQLite。后续启动时，SQLite 中的运行时设置和 Provider 会覆盖基础配置中的同名值。

`.env` 不存在也能启动；后台修改的运行时参数写入 SQLite，重启后继续生效。需要让 `.env` 中的运行时参数重新进入 SQLite 时，应在管理面板执行热加载，而不是只重启进程。

`.env` 语法（见 `internal/config/config.go` 的 `readEnvFile`）：

- 每行 `KEY=VALUE`，`export ` 前缀可选；
- `#` 开头为注释，空行忽略；
- 值首尾空白会被裁剪，首尾成对引号会被去掉；
- 列表类变量支持 `,` `;` 换行分隔，自动去重。

---

## 1. 服务

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_HOST` | `127.0.0.1` | 监听地址 |
| `APP_PORT` | `8080` | 监听端口，取值 1–65535 |
| `WEB_DIR` | `web` | 静态资源目录 |
| `DATA_DIR` | `data` | 数据目录 |
| `DATABASE_PATH` | `DATA_DIR/cg.sqlite` | SQLite 路径 |
| `DASHBOARD_TITLE` | `模型连通性` | 仪表盘标题 |
| `ADMIN_TOKEN` | 自动生成 | 管理接口密钥，见 [security.md](security.md) |

## 2. 探测

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

每次探测固定使用 `temperature=0`、`max_tokens=16`，仅发送 system + user 两条消息，把 token 消耗压到最低（每模型约 40 token）。

推理模型（DeepSeek-R1、QwQ 等）会输出 `<think>…`，后端用两条正则剥离完整标签和被截断的未闭合标签，只保留实际回复。

## 3. 历史与展示

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `ENABLE_HISTORY` | `true` | 是否记录历史；关闭后仍会计入 token 用量 |
| `SHOW_CURVE_CHART` | `true` | 是否生成延迟曲线 SVG |
| `STATS_WINDOW_DAYS` | `7` | 可用率统计窗口（1–3650） |
| `HISTORY_SIZE` | `30` | 仪表盘展示的历史条数（1–100000） |
| `MAX_HISTORY_RECORDS` | `500` | 每模型数据库保留记录数（1–1000000） |
| `SHOW_ERROR_DETAIL` | `true` | 是否展示错误详情；关闭时错误文本清空 |
| `THEME_MODE` | `auto` | `auto` / `dark` / `light` |
| `DAY_MODE_START_HOUR` | `8` | `auto` 下亮色起始小时（0–23） |
| `DAY_MODE_END_HOUR` | `18` | `auto` 下亮色结束小时（0–23） |

`auto` 主题按服务端当前小时判断：起始 ≤ 结束表示同一天区间，否则视为跨天（如 18 → 次日 8）。

## 4. 定时检测

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AUTO_CHECK_INTERVAL_MIN_HOURS` | `0` | 最小间隔（小时），有限非负数且 ≤ 8760 |
| `AUTO_CHECK_INTERVAL_MAX_HOURS` | `0` | 最大间隔（小时），同上 |
| `AUTO_CHECK_RUN_ON_START` | `false` | 启动后立即检测一次 |

- 两者都 ≤ 0 表示关闭定时检测；只设一个则另一个取相同值；min > max 时自动交换。
- 实际间隔 = `min + rand()*(max-min)` 小时，最小 1 分钟，用于错开多实例。
- 修改配置会唤醒调度器重新计算下一次时间。

## 5. 告警通知

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `NOTIFY_PLATFORM` | `webhook` | `webhook`/`discord`/`bark`/`wecom`/`wechat_work`/`dingtalk`/`telegram` |
| `NOTIFY_WEBHOOK_URL` | — | Webhook 地址（允许查询参数） |
| `NOTIFY_TELEGRAM_BOT_TOKEN` | — | Telegram Bot Token |
| `NOTIFY_TELEGRAM_CHAT_ID` | — | Telegram Chat ID |
| `NOTIFY_ON_RECOVERY` | `true` | 恢复时是否通知 |
| `NOTIFY_COOLDOWN_MINUTES` | `0` | 冷却时间（分钟），`0` 关闭 |
| `NOTIFY_PROVIDERS` | — | 仅对指定 Provider 告警（ID 或 Name） |
| `NOTIFY_MODELS` | — | 仅对指定模型告警（`model`/`provider/model`/`provider::model`） |

发送条件（见 `internal/notify/notify.go`）：

1. 平台配置完整（telegram 需 token + chat_id，其他需 webhook URL）；
2. 按 `NOTIFY_PROVIDERS` / `NOTIFY_MODELS` 过滤后重新计算聚合状态；
3. 与上次告警状态 `ok`/`slow`/`error` **不同**才发；
4. `ok` 且上次状态为空（首次启动）不发；
5. `ok` 且 `NOTIFY_ON_RECOVERY=false` 只落状态不发消息；
6. 冷却期内不发，且不更新状态（下一轮仍可发）。

各平台消息体：

| 平台 | JSON |
|------|------|
| `discord` | `{"content": "<text>"}` |
| `bark` | `{"title": "<title>", "body": "<text>"}` |
| `wecom` / `wechat_work` | `{"msgtype":"text","text":{"content":"<text>"}}` |
| `dingtalk` | `{"msgtype":"text","text":{"content":"<text>"}}` |
| `webhook` / 其他 | 完整 payload（`status`/`summary`/`provider_text` 等） |
| `telegram` | 请求 `https://api.telegram.org/bot<token>/sendMessage`，`{"chat_id":…,"text":…}` |

文本超过 10 行 Provider 明细会折叠为「其余 N 项已省略」。

## 6. Provider 配置

```env
PROVIDER_1_ID=openai-main
PROVIDER_1_NAME=OpenAI
PROVIDER_1_TYPE=openai
PROVIDER_1_BASE_URL=https://api.openai.com/v1
PROVIDER_1_API_KEY=sk-xxx
PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
PROVIDER_1_ENABLED=true
PROVIDER_1_PROBE_ENABLED=true
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

### 6.1 校验规则

- `ID`：非空、≤128 字符、无首尾空格、不含控制字符与 `/ \ ? #`、不得为 `.` 或 `..`；大小写不敏感去重。
- `BASE_URL`：必须 `http`/`https`；必须有 host；**不允许**用户信息（userinfo）、查询参数、fragment；禁止链路本地地址（169.254.0.0/16）。
- `NOTIFY_WEBHOOK_URL`：同上，但**允许**查询参数（钉钉/企业微信需要）。
- `ADMIN_TOKEN` / 只读密钥：16–256 位可打印 ASCII（33–126），不含空格。

### 6.2 图标匹配

`provider.IconFor(id, type, name)` 依次用 ID → TYPE → NAME 小写精确匹配内置图标表；未命中则把 `_`、`-`、空格拆分后的关键词做前缀/包含匹配。内置键见仓库 README 的图标清单；未匹配时前端回退为名称首两字母占位块。

## 7. 运行时修改与热加载

| 入口 | 行为 |
|------|------|
| 管理面板 **设置** / `PUT /api/admin/settings` | 校验后写入 SQLite 并立即生效，同时唤醒调度器 |
| 管理面板 **Provider** / `POST,PUT,DELETE /api/admin/providers` | 同上；`api_key` 空表示保留旧值，`clear_api_key=true` 清除 |
| `POST /api/admin/config/import` | 整体替换 settings + providers |
| `POST /api/admin/config/reload` | 重读 `.env`；仅当 `.env` 有 Provider 时覆盖库内 Provider；成功后异步触发一次检测 |

`reload` 在监听地址、`WEB_DIR`、`DATA_DIR`、`DATABASE_PATH`、`ADMIN_TOKEN` 发生变化时返回错误并拒绝加载——这些必须重启。

`PROBE_PROMPT`、`PROBE_SYSTEM_PROMPT` 和 `AUTO_CHECK_RUN_ON_START` 不属于 SQLite `RuntimeSettings`：两个提示词来自基础配置，可通过 `.env` 热加载；`AUTO_CHECK_RUN_ON_START` 只在进程启动时判断。Provider 默认由 SQLite 接管，只有热加载的 `.env` 明确包含 Provider 时才会覆盖并保存 Provider 列表。

敏感字段写策略：告警 webhook / Telegram token / chat id 在 GET 接口只返回 `*_set` 布尔；PUT 时留空表示保持不变，传 `clear_*=true` 表示清除。
