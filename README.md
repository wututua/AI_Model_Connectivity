# 模型连通性 Web 状态页

这是一个独立 Go 程序，用于检测模型接口连通性并生成 Web 状态页。当前版本不依赖 AstrBot，不导出图片，后端负责检测并把结果写入 `web/` 文件夹。

## 功能

- 支持 OpenAI 兼容接口检测：`/v1/chat/completions`、`/v1/models`
- 支持多个 Provider 和多个模型
- 支持全局并发与单 Provider 并发限制
- 支持跳过指定模型
- 支持正常、较慢、异常状态分类
- 支持历史条、24h 平均延迟、统计窗口成功率
- 输出静态 Web 仪表盘：`web/index.html`、`web/assets/*`
- 通过 `/api/status` 和 `/api/events` 实时刷新页面，不依赖 `status.json`
- 内置 HTTP 服务和手动检测接口
- 使用 `.env` 管理配置和密钥

## 快速开始

1. 复制配置文件：

```bash
cp .env.example .env
```

2. 编辑 `.env`，至少配置一个 Provider：

```env
PROVIDER_1_ID=openai-main
PROVIDER_1_NAME=OpenAI
PROVIDER_1_TYPE=openai
PROVIDER_1_BASE_URL=https://api.openai.com/v1
PROVIDER_1_API_KEY=sk-xxx
PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
PROVIDER_1_ENABLED=true
```

3. 启动服务：

```bash
go run ./cmd/cg
```

4. 打开网页：

```text
http://127.0.0.1:8080/
```

5. 手动触发检测：

```bash
curl -X POST http://127.0.0.1:8080/api/check
```

也可以只运行一次检测并更新本地最新报告：

```bash
go run ./cmd/cg check
```

## 配置说明

主要配置见 `.env.example`。

### 服务配置

- `APP_HOST`：监听地址，默认 `127.0.0.1`
- `APP_PORT`：监听端口，默认 `8080`
- `WEB_DIR`：Web 输出目录，默认 `web`
- `DATA_DIR`：历史和最新报告存储目录，默认 `data`
- `DASHBOARD_TITLE`：状态页标题
- `ADMIN_TOKEN`：本地使用可选。设置后，`POST /api/check` 需要 `Authorization: Bearer <token>`；当 `APP_HOST` 绑定到 `0.0.0.0` / `::` 等公开接口时必须设置。

### 探测配置

- `TIMEOUT_SECONDS`：单模型检测超时秒数
- `MODEL_LIST_TIMEOUT_SECONDS`：获取模型列表超时秒数
- `SLOW_THRESHOLD_MS`：超过该耗时但调用成功时标记为“较慢”
- `CONCURRENCY`：全局最大并发探测数
- `PROVIDER_CONCURRENCY`：单个 Provider 最大并发探测数
- `MAX_MODELS_PER_PROVIDER`：每个 Provider 最多检测模型数，`0` 表示不限制
- `SKIP_MODELS`：跳过模型，支持 `model`、`provider/model`、`provider::model`
- `PROBE_PROMPT`：检测时发送的用户消息
- `PROBE_SYSTEM_PROMPT`：检测时发送的 system prompt

### 历史与主题

- `ENABLE_HISTORY`：是否保存历史记录
- `SHOW_CURVE_CHART`：是否在前端显示延迟曲线
- `STATS_WINDOW_DAYS`：统计窗口天数
- `HISTORY_SIZE`：状态历史条长度
- `MAX_HISTORY_RECORDS`：每个模型最多保留多少条历史记录
- `SHOW_ERROR_DETAIL`：是否显示错误详情
- `THEME_MODE`：`auto`、`dark`、`light`
- `DAY_MODE_START_HOUR` / `DAY_MODE_END_HOUR`：自动白天模式时间段

### 定时检测

- `AUTO_CHECK_INTERVAL_MIN_HOURS`：定时检测最小间隔小时，`0` 表示关闭
- `AUTO_CHECK_INTERVAL_MAX_HOURS`：定时检测最大间隔小时
- `AUTO_CHECK_RUN_ON_START`：启动后是否立即检测一次

例如：

```env
AUTO_CHECK_INTERVAL_MIN_HOURS=2
AUTO_CHECK_INTERVAL_MAX_HOURS=5
AUTO_CHECK_RUN_ON_START=true
```

## Provider 配置

使用编号变量配置多个 Provider：

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

如果 `PROVIDER_N_MODELS` 为空，程序会尝试请求 `{BASE_URL}/models` 获取模型列表。

## API

- `GET /health`：健康检查
- `GET /api/status`：读取最新状态
- `POST /api/check`：触发一次检测，更新最新报告，并向 `/api/events` 客户端推送
- `GET /api/events`：SSE 实时事件流，检测完成后推送最新报告
- `GET /`：Web 仪表盘

## 输出文件

检测后会生成：

```text
web/index.html
web/assets/app.js
web/assets/style.css
data/latest_report.json
data/probe_history.json
```

不会生成 PNG/JPG 等图片文件。

## 注意

检测会真实调用模型接口，可能产生额度消耗。请不要提交 `.env`，其中通常包含 API Key。公开部署时请设置 `ADMIN_TOKEN`，避免他人触发 `/api/check` 消耗额度。
