# AI_Model_Connectivity

一个用于检测 OpenAI 兼容模型接口连通性的独立 Web 状态页，配套可视化管理面板。

## 功能

- 检测 `/v1/chat/completions` 和 `/v1/models` 接口
- 支持多个 Provider 和多个模型，双层并发控制（全局 + 单 Provider）
- 三态展示：正常、较慢、异常；记录历史、24h 平均延迟和统计窗口可用率
- SSE 实时推送，仪表盘自动刷新；无实时推送时自动降级为 30 秒轮询
- 自动剥离响应中的 `<think>` / `<thinking>` 思考标签，兼容 DeepSeek-R1、QwQ 等推理模型
- 仪表盘展示每次历史检测的圆形 LED 状态灯及每个模型的当前检测状态指示灯
- Web 管理面板：在浏览器中动态增删 Provider、调整检测参数、查看任务历史、导入导出配置；内置 Token 消耗估算
- 支持 Telegram、Discord、Bark、企业微信、钉钉和通用 Webhook 告警通知
- **无配置启动**：不需要 `.env` 文件，首次运行自动生成 10 位管理密钥，管理面板引导修改

## 快速开始

```bash
go run ./cmd/cg
```

打开 [http://127.0.0.1:8080](http://127.0.0.1:8080) 查看仪表盘，点击右上角 **管理** 进入管理面板。

**首次启动**时若未设置 `ADMIN_TOKEN`，服务会自动生成一个随机密钥并打印到终端：

```
╔══════════════════════════════════════════╗
║  Auto-generated ADMIN_TOKEN: aB3xZ9mK2p  ║
║  Please change it on first login         ║
╚══════════════════════════════════════════╝
```

首次进入管理面板时，系统会强制要求修改密钥（至少 6 位），修改完成后自动进入。

若需要自定义 Provider，复制并编辑配置文件后再启动：

```bash
cp .env.example .env
# 编辑 .env，填写 Provider 信息
go run ./cmd/cg
```

首次启动若仪表盘为空，先手动触发一次检测：

```bash
curl -X POST -H "Authorization: Bearer <your-token>" http://127.0.0.1:8080/api/admin/check
```

## 管理面板

访问 [http://127.0.0.1:8080/admin](http://127.0.0.1:8080/admin) 打开管理面板。

| 标签页 | 功能 |
|--------|------|
| 检测控制 | 查看运行状态、手动触发检测、停止检测；Token 消耗估算 |
| Provider | 新增、编辑、删除、单独重跑 Provider |
| 设置 | 修改检测参数、历史配置、告警通知 |
| 任务历史 | 分页查看历史检测任务及结果 |
| 配置管理 | 导出/导入 JSON 配置、热加载 `.env` |

## 部署

### 二进制部署

1. 从 [Releases](../../releases) 下载对应平台的压缩包并解压（内含预构建的 `web/` 目录）
2. 直接启动：`./model-connectivity`（Windows 运行 `model-connectivity.exe`）
3. 首次启动时终端会打印自动生成的管理密钥，进入管理面板后强制修改

可选：复制 `.env.example` 为 `.env` 并填写 Provider 信息，配置自动检测间隔和告警。

### Docker

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  --name model-connectivity \
  ghcr.io/wututua/ai_model_connectivity:latest
```

或通过环境变量传入配置：

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e ADMIN_TOKEN=your-token \
  -e PROVIDER_1_ID=openai \
  -e PROVIDER_1_BASE_URL=https://api.openai.com/v1 \
  -e PROVIDER_1_API_KEY=sk-xxx \
  -e PROVIDER_1_MODELS=gpt-4o-mini \
  --name model-connectivity \
  ghcr.io/wututua/ai_model_connectivity:latest
```

### 源码运行

```bash
# 首次运行前需要构建前端
cd frontend && npm install && npm run build && cd ..

go run ./cmd/cg          # 持续服务模式
go run ./cmd/cg check    # 只运行一次检测后退出
```

### 前端开发模式

```bash
# 终端 1：启动后端
go run ./cmd/cg

# 终端 2：启动前端开发服务器（热更新，代理到 :8080）
cd frontend && npm run dev
```

访问 [http://127.0.0.1:5173](http://127.0.0.1:5173) 即可。修改 `frontend/src/` 下的文件后浏览器自动刷新。

开发完成后执行 `npm run build` 将产物写入 `web/`，Go 服务端直接提供。

## 配置

所有配置通过 `.env` 文件或环境变量设置，`.env` 不存在时也可正常启动。后台 API 修改的参数写入 SQLite，重启后继续生效；`.env` 仍作为初始配置来源。

### 服务

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_HOST` | `127.0.0.1` | 监听地址 |
| `APP_PORT` | `8080` | 监听端口 |
| `WEB_DIR` | `web` | Web 静态文件目录 |
| `DATA_DIR` | `data` | 数据目录 |
| `DATABASE_PATH` | `DATA_DIR/cg.sqlite` | SQLite 路径，留空取默认值 |
| `DASHBOARD_TITLE` | `模型连通性` | 页面标题 |
| `ADMIN_TOKEN` | 自动生成 | 保护管理接口；未设置时自动生成随机密钥；公开监听时**强烈建议手动设置** |

#### 管理密钥说明

- **未设置 `ADMIN_TOKEN`**：服务启动时自动生成一个 10 位随机密钥（大小写字母 + 数字），打印到终端，并持久化到 SQLite。首次进入管理面板时会强制要求修改。
- **已设置 `ADMIN_TOKEN`**：直接使用环境变量中的值，不触发首次修改流程。
- **公开部署**（监听 `0.0.0.0` / `::`）：**必须**通过环境变量显式设置 `ADMIN_TOKEN`，自动生成的密钥不足以保障公开暴露的安全。

### 探测

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `TIMEOUT_SECONDS` | `30` | 单模型检测超时（秒） |
| `MODEL_LIST_TIMEOUT_SECONDS` | `20` | 获取模型列表超时（秒） |
| `SLOW_THRESHOLD_MS` | `800` | 超过此延迟标记为"较慢"（毫秒） |
| `CONCURRENCY` | `1` | 全局最大并发数；默认 `1` 表示所有模型严格逐个检测 |
| `PROVIDER_CONCURRENCY` | `1` | 单 Provider 最大并发数 |
| `MAX_MODELS_PER_PROVIDER` | `0` | 每个 Provider 最多检测模型数，`0` 不限制 |
| `SKIP_MODELS` | — | 跳过的模型，支持 `model`、`provider/model`、`provider::model`，逗号分隔 |
| `PROBE_PROMPT` | `ping` | 探测用提示词 |
| `PROBE_SYSTEM_PROMPT` | `No thinking. Respond only with exactly: pang. No extra words.` | 探测用系统提示词 |

> **推理模型兼容**：对于 DeepSeek-R1、QwQ 等会在响应中输出 `<think>…</think>` 或 `<thinking>…</thinking>` 思考过程的模型，后端会在解析时自动剥离这些标签，仅保留实际回复内容用于状态判断。系统提示词默认已要求禁止思考输出，减少 token 消耗。

### 历史与展示

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `ENABLE_HISTORY` | `true` | 启用历史记录 |
| `SHOW_CURVE_CHART` | `true` | 显示延迟曲线 |
| `STATS_WINDOW_DAYS` | `7` | 统计窗口天数 |
| `HISTORY_SIZE` | `30` | 历史条长度（仪表盘展示） |
| `MAX_HISTORY_RECORDS` | `500` | 每个模型在数据库中最多保留的记录数 |
| `SHOW_ERROR_DETAIL` | `true` | 显示错误详情 |
| `THEME_MODE` | `auto` | 主题模式：`auto`、`dark`、`light` |
| `DAY_MODE_START_HOUR` | `8` | `auto` 主题下亮色模式起始小时（0–23） |
| `DAY_MODE_END_HOUR` | `18` | `auto` 主题下亮色模式结束小时（0–23） |

### 定时检测

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `AUTO_CHECK_INTERVAL_MIN_HOURS` | `0` | 最小检测间隔（小时），`0` 关闭定时检测 |
| `AUTO_CHECK_INTERVAL_MAX_HOURS` | `0` | 最大检测间隔（小时） |
| `AUTO_CHECK_RUN_ON_START` | `false` | 启动后立即执行一次检测 |

实际检测间隔在 min–max 之间随机取值，可以错开多实例同时检测。建议适当拉长周期，减少不必要的 token 消耗：

```env
AUTO_CHECK_INTERVAL_MIN_HOURS=6
AUTO_CHECK_INTERVAL_MAX_HOURS=12
AUTO_CHECK_RUN_ON_START=true
```

### 告警通知

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `NOTIFY_PLATFORM` | `webhook` | 平台：`webhook`、`discord`、`bark`、`wecom`、`wechat_work`、`dingtalk`、`telegram` |
| `NOTIFY_WEBHOOK_URL` | — | Webhook 地址（Discord、Bark、企业微信、钉钉及通用 Webhook 使用） |
| `NOTIFY_TELEGRAM_BOT_TOKEN` | — | Telegram Bot Token |
| `NOTIFY_TELEGRAM_CHAT_ID` | — | Telegram Chat ID |
| `NOTIFY_ON_RECOVERY` | `true` | 从异常/较慢恢复正常时是否发送通知 |
| `NOTIFY_COOLDOWN_MINUTES` | `0` | 告警冷却时间（分钟），`0` 关闭 |
| `NOTIFY_PROVIDERS` | — | 只对指定 Provider 告警，支持 ID 或 Name，留空表示全部 |
| `NOTIFY_MODELS` | — | 只对指定模型告警，支持 `model`、`provider/model`、`provider::model`，留空表示全部 |

告警在筛选后的整体状态发生变化时触发；首次启动且状态正常时不发送通知。

示例：

```env
NOTIFY_PLATFORM=dingtalk
NOTIFY_WEBHOOK_URL=https://example.com/robot/send?access_token=xxx
NOTIFY_ON_RECOVERY=true
NOTIFY_COOLDOWN_MINUTES=30
NOTIFY_PROVIDERS=openai-main,ollama-local
NOTIFY_MODELS=openai-main/gpt-4o-mini,llama3.1
```

## Provider 配置

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

`PROVIDER_N_MODELS` 留空时，自动请求 `{BASE_URL}/models` 获取模型列表。

Provider 也可以在管理面板的 **Provider** 标签页中通过界面增删，无需重启服务。

Provider 图标根据 `PROVIDER_N_ID`、`PROVIDER_N_TYPE`、`PROVIDER_N_NAME` 自动匹配，优先级依次降低，支持前缀及按 `_`、`-`、空格拆分后的关键词匹配。`PROVIDER_N_TYPE` 支持以下内置图标键：

```
openai  azure  xai  anthropic  ollama  google  deepseek  modelscope  zhipu  nvidia
siliconflow  moonshot  kimi  kimi-code  longcat  ppio  dify  coze  dashscope
deerflow  fastgpt  lm_studio  fishaudio  minimax  minimax-token-plan  mimo
302ai  microsoft  vllm  groq  aihubmix  openrouter  tokenpony  compshare
xinference  bailian  volcengine
```

## API

### 公开接口

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/health` | 健康检查 |
| `GET` | `/api/status` | 获取最新状态报告 |
| `GET` | `/api/events` | SSE 实时推送 |
| `GET` | `/` | Web 仪表盘 |
| `GET` | `/admin` | Web 管理面板 |

### 管理接口

请求时携带：`Authorization: Bearer <ADMIN_TOKEN>`

> 本地监听 `127.0.0.1` 且未设置 `ADMIN_TOKEN` 时，允许无 token 访问。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/admin/detection` | 查看检测运行状态和自动检测配置 |
| `POST` | `/api/admin/detection/start` | 开始检测 |
| `POST` | `/api/admin/detection/stop` | 停止当前检测 |
| `POST` | `/api/admin/check` | 触发一次完整检测 |
| `POST` | `/api/admin/token` | 修改管理密钥 `{"token":"new-token"}` |
| `GET` | `/api/admin/config` | 查看当前配置（不含密钥） |
| `PUT` | `/api/admin/settings` | 修改阈值、检测参数、自动检测间隔 |
| `GET` | `/api/admin/providers` | 查看 Provider 列表（不含 API Key） |
| `POST` | `/api/admin/providers` | 新增 Provider |
| `PUT` | `/api/admin/providers/{id}` | 修改 Provider；不传 `api_key` 时保留旧值 |
| `DELETE` | `/api/admin/providers/{id}` | 删除 Provider |
| `POST` | `/api/admin/providers/{id}/rerun` | 单独重跑某个 Provider |
| `GET` | `/api/admin/tasks` | 查看历史检测任务 |
| `GET` | `/api/admin/tasks/{id}` | 查看任务详情 |
| `GET` | `/api/admin/config/export` | 导出配置（不含密钥） |
| `POST` | `/api/admin/config/import` | 导入配置并保存到 SQLite |
| `POST` | `/api/admin/config/reload` | 重新读取 `.env` 并热加载 |

## 数据文件

```
web/index.html
web/assets/app.js
web/assets/index.css
data/cg.sqlite
```

`web/` 目录由 Vite 构建生成，发布包内已包含预构建产物，无需手动构建即可运行。

历史检测结果、最新报告、告警状态和管理密钥均保存在 SQLite。首次启动时若存在旧版 JSON 文件（`data/latest_report.json`、`data/probe_history.json`、`data/notify_state.txt`），会自动迁移到 SQLite，旧文件不会被删除。

## 安全提示

- 该项目会真实调用模型接口并消耗 token，建议使用最小化探测提示词（默认已优化），并适当拉长检测间隔。
- **公开部署（监听 `0.0.0.0` / `::`）时必须通过 `ADMIN_TOKEN` 环境变量显式设置密钥**，否则任何人均可触发检测或修改配置。自动生成的密钥仅适合本地使用场景。
- 管理面板首次登录后强制要求修改密钥，后续密钥持久化存储在 SQLite，重启后无需重新配置。

## License

[MIT](LICENSE)
