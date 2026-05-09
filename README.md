# Model Connectivity

一个用于检测 OpenAI 兼容模型接口连通性的独立 Web 状态页。

## 功能

- 检测 `/v1/chat/completions` 和 `/v1/models`
- 支持多个 Provider 和多个模型
- 支持全局并发、单 Provider 并发、跳过指定模型
- 展示正常、较慢、异常状态
- 记录历史、24h 平均延迟、统计窗口可用率
- 提供 Web 仪表盘、状态 API 和 SSE 实时更新

## 快速开始

```bash
cp .env.example .env
```

编辑 `.env`，至少配置一个 Provider：

```env
PROVIDER_1_ID=openai-main
PROVIDER_1_NAME=OpenAI
PROVIDER_1_TYPE=openai
PROVIDER_1_BASE_URL=https://api.openai.com/v1
PROVIDER_1_API_KEY=sk-xxx
PROVIDER_1_MODELS=gpt-4o-mini,gpt-4.1-mini
PROVIDER_1_ENABLED=true
```

启动服务：

```bash
go run ./cmd/cg
```

打开：

```text
http://127.0.0.1:8080/
```

手动检测：

```bash
curl -X POST http://127.0.0.1:8080/api/check
```

只运行一次检测：

```bash
go run ./cmd/cg check
```

## 配置

主要配置见 `.env.example`。

### 服务

- `APP_HOST`：监听地址，默认 `127.0.0.1`
- `APP_PORT`：监听端口，默认 `8080`
- `WEB_DIR`：Web 文件目录，默认 `web`
- `DATA_DIR`：报告和历史数据目录，默认 `data`
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
- `PROBE_PROMPT` / `PROBE_SYSTEM_PROMPT`：探测提示词

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

示例：

```env
AUTO_CHECK_INTERVAL_MIN_HOURS=2
AUTO_CHECK_INTERVAL_MAX_HOURS=5
AUTO_CHECK_RUN_ON_START=true
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

## 数据文件

```text
web/index.html
web/assets/app.js
web/assets/style.css
data/latest_report.json
data/probe_history.json
```

## 安全提示

检测会真实调用模型接口，可能产生额度消耗。公开部署时请设置 `ADMIN_TOKEN`。
