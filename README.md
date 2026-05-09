# AI_Model_Connectivity

一个用于检测 OpenAI 兼容模型接口连通性的独立 Web 状态页。

## 功能

- 检测 `/v1/chat/completions` 和 `/v1/models`
- 支持多个 Provider 和多个模型
- 支持全局并发、单 Provider 并发、跳过指定模型
- 展示正常、较慢、异常状态
- 记录历史、24h 平均延迟、统计窗口可用率
- 提供 Web 仪表盘、状态 API 和 SSE 实时更新
- 支持 Telegram、Discord、Bark、企业微信、钉钉和通用 Webhook 告警通知
- 提供后台 API，可配置 Provider、修改阈值、启停检测、查看任务、导入导出配置

## 部署方式

### 首次启动

1. 先从 Release 下载压缩包，或准备好源码目录。
2. 复制配置文件：

```bash
cp .env.example .env
```

3. 打开 `.env`，至少填一个 Provider：

```env
PROVIDER_1_ID=openai-main
PROVIDER_1_NAME=OpenAI
PROVIDER_1_TYPE=openai
PROVIDER_1_BASE_URL=https://api.openai.com/v1
PROVIDER_1_API_KEY=sk-xxx
PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
PROVIDER_1_ENABLED=true
```

4. 启动程序后打开：

```text
http://127.0.0.1:8080/
```

5. 如果页面为空，先手动执行一次检测：

```bash
curl -X POST http://127.0.0.1:8080/api/check
```

#### 二进制部署

1. 从 Release 下载对应平台的压缩包。
2. 解压后得到可执行文件、`README.md` 和 `.env.example`。
3. 复制并编辑配置文件：

```bash
cp .env.example .env
```

4. 至少配置一个 Provider：

```env
PROVIDER_1_ID=openai-main
PROVIDER_1_NAME=OpenAI
PROVIDER_1_TYPE=openai
PROVIDER_1_BASE_URL=https://api.openai.com/v1
PROVIDER_1_API_KEY=sk-xxx
PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
PROVIDER_1_ENABLED=true
```

5. 启动服务：

```bash
./model-connectivity
```

Windows 下直接运行 `model-connectivity.exe`。

#### 手动部署

如果不使用 Release 包，也可以直接从源码运行：

```bash
cp .env.example .env
```

编辑 `.env` 后执行：

```bash
go run ./cmd/cg
```

只运行一次检测：

```bash
go run ./cmd/cg check
```

打开：

```text
http://127.0.0.1:8080/
```

手动检测：

```bash
curl -X POST http://127.0.0.1:8080/api/check
```

## 配置

主要配置见 `.env.example`。

### 服务

- `APP_HOST`：监听地址，默认 `127.0.0.1`
- `APP_PORT`：监听端口，默认 `8080`
- `WEB_DIR`：Web 文件目录，默认 `web`
- `DATA_DIR`：数据目录，默认 `data`
- `DATABASE_PATH`：SQLite 数据库路径，留空时默认 `DATA_DIR/cg.sqlite`
- `DASHBOARD_TITLE`：页面标题
- `ADMIN_TOKEN`：保护 `POST /api/check`。公开监听 `0.0.0.0` / `::` 时必须设置

### 探测

- `TIMEOUT_SECONDS`：单模型检测超时
- `MODEL_LIST_TIMEOUT_SECONDS`：获取模型列表超时
- `SLOW_THRESHOLD_MS`：超过该耗时标记为“较慢”
- `CONCURRENCY`：全局最大并发
- `PROVIDER_CONCURRENCY`：单 Provider 最大并发
- `MAX_MODELS_PER_PROVIDER`：每个 Provider 最多检测模型数，`0` 表示不限制
- `SKIP_MODELS`：跳过模型，支持 `model`、`provider/model`、`provider::model`

### 历史和显示

- `ENABLE_HISTORY`：保存历史记录
- `SHOW_CURVE_CHART`：显示延迟曲线
- `STATS_WINDOW_DAYS`：统计窗口天数
- `HISTORY_SIZE`：历史条长度
- `MAX_HISTORY_RECORDS`：每个模型最多保留记录数
- `SHOW_ERROR_DETAIL`：显示错误详情
- `THEME_MODE`：`auto`、`dark`、`light`

### 定时检测

- `AUTO_CHECK_INTERVAL_MIN_HOURS`：最小检测间隔，`0` 表示关闭
- `AUTO_CHECK_INTERVAL_MAX_HOURS`：最大检测间隔
- `AUTO_CHECK_RUN_ON_START`：启动后立即检测一次

建议把测试周期拉长一点，减少不必要的 token 消耗。示例：

```env
AUTO_CHECK_INTERVAL_MIN_HOURS=6
AUTO_CHECK_INTERVAL_MAX_HOURS=12
AUTO_CHECK_RUN_ON_START=true
```

### 告警通知

- `NOTIFY_PLATFORM`：告警平台，支持 `webhook`、`discord`、`bark`、`wecom`、`wechat_work`、`dingtalk`、`telegram`
- `NOTIFY_WEBHOOK_URL`：Webhook 地址，Discord、Bark、企业微信、钉钉和通用 Webhook 使用该配置
- `NOTIFY_TELEGRAM_BOT_TOKEN` / `NOTIFY_TELEGRAM_CHAT_ID`：Telegram Bot 通知配置
- `NOTIFY_ON_RECOVERY`：从异常/较慢恢复正常时是否发送恢复通知，默认 `true`
- `NOTIFY_COOLDOWN_MINUTES`：告警冷却时间，避免状态抖动频繁通知，`0` 表示关闭
- `NOTIFY_PROVIDERS`：只对指定 Provider 告警，支持 `provider_id` 或 `provider_name`，留空表示全部
- `NOTIFY_MODELS`：只对指定模型告警，支持 `model`、`provider/model`、`provider::model`，留空表示全部

告警会在筛选后的整体状态发生变化时发送。首次启动且状态正常时不会发送通知。

示例：

```env
NOTIFY_PLATFORM=dingtalk
NOTIFY_WEBHOOK_URL=https://example.com/robot/send?access_token=xxx
NOTIFY_ON_RECOVERY=true
NOTIFY_COOLDOWN_MINUTES=30
NOTIFY_PROVIDERS=openai-main,ollama-local
NOTIFY_MODELS=openai-main/gpt-4o-mini,llama3.1
```

## Provider

```env
PROVIDER_1_ID=openai-main
PROVIDER_1_NAME=OpenAI
PROVIDER_1_TYPE=openai
PROVIDER_1_BASE_URL=https://api.openai.com/v1
PROVIDER_1_API_KEY=sk-xxx
PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
PROVIDER_1_ENABLED=true

PROVIDER_2_ID=ollama-local
PROVIDER_2_NAME=Ollama
PROVIDER_2_TYPE=ollama
PROVIDER_2_BASE_URL=http://127.0.0.1:11434/v1
PROVIDER_2_API_KEY=
PROVIDER_2_MODELS=llama3.1
PROVIDER_2_ENABLED=true
```

`PROVIDER_N_MODELS` 为空时，会尝试请求 `{BASE_URL}/models` 获取模型列表。

## API

- `GET /health`：健康检查
- `GET /api/status`：最新状态
- `POST /api/check`：触发检测
- `GET /api/events`：SSE 实时更新
- `GET /`：Web 仪表盘

### 后台 API

后台 API 复用 `ADMIN_TOKEN`，请求头格式：`Authorization: Bearer <ADMIN_TOKEN>`。本地监听 `127.0.0.1` 且未设置 `ADMIN_TOKEN` 时允许访问；公开监听时必须设置 token。

- `GET /api/admin/detection`：查看检测运行状态和自动检测间隔
- `POST /api/admin/detection/start`：立即开始一次完整检测
- `POST /api/admin/detection/stop`：停止当前正在运行的检测
- `GET /api/admin/config`：查看安全版当前配置
- `PUT /api/admin/settings`：修改阈值、检测参数、自动检测间隔
- `GET /api/admin/providers`：查看 Provider 列表，不返回 API key
- `POST /api/admin/providers`：新增 Provider
- `PUT /api/admin/providers/{id}`：修改 Provider；不传 `api_key` 时保留旧 key
- `DELETE /api/admin/providers/{id}`：删除 Provider
- `POST /api/admin/providers/{id}/rerun`：只重跑某个 Provider，不覆盖当前仪表盘最新报告
- `GET /api/admin/tasks`：查看历史检测任务
- `GET /api/admin/tasks/{id}`：查看任务详情
- `GET /api/admin/config/export`：导出配置，不包含密钥
- `POST /api/admin/config/import`：导入配置并保存到 SQLite

`.env` 仍作为初始配置来源；后台修改会保存到 SQLite，重启后继续生效。

### 后台快速使用

1. 设置 `ADMIN_TOKEN` 后启动服务。
2. 请求后台接口时带上：`Authorization: Bearer <ADMIN_TOKEN>`。
3. 先用 `GET /api/admin/config` 查看当前配置，再用 `PUT /api/admin/settings` 或 `POST /api/admin/providers` 修改。
4. 导出配置默认不包含密钥；导入配置会写入 SQLite 并在重启后继续生效。

## 数据文件

```text
web/index.html
web/assets/app.js
web/assets/style.css
data/cg.sqlite
```

历史检测结果、最新报告和告警状态会保存到 SQLite。首次启动时，如果存在旧的 `data/latest_report.json`、`data/probe_history.json` 或 `data/notify_state.txt`，会自动尝试导入到 SQLite，旧文件不会被删除。

## 安全提示

该项目会真实调用模型接口并消耗 token。测试周期建议调长一点，单次检测通常不会消耗很多，但频繁自动检测会累计消耗。公开部署时请设置 `ADMIN_TOKEN`。
