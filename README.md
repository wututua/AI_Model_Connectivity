# AI_Model_Connectivity

一个用于检测 OpenAI 兼容模型接口连通性的独立 Web 状态页，配套基于 React 与 shadcn/ui 的可视化管理面板。

## 功能

- 检测 `/v1/chat/completions` 和 `/v1/models` 接口
- 支持多个 Provider 和多个模型，双层并发控制（全局 + 单 Provider）
- 三态展示：正常、较慢、异常；记录历史、24h 平均延迟和统计窗口可用率
- SSE 实时推送，仪表盘自动刷新；无实时推送时自动降级为指数退避轮询（30s → 60s → 120s）
- 自动剥离响应中的 `<think>` / `<thinking>` 思考标签，兼容 DeepSeek-R1、QwQ 等推理模型
- 仪表盘全局搜索 + 状态过滤（正常 / 较慢 / 异常），多维排序（状态 / 名称 / 延迟 / 模型数），简洁 / 详细卡片视图切换
- 仪表盘展示每次历史检测的圆形 LED 状态灯、每个模型的当前检测状态指示灯及延迟曲线
- React + TypeScript 前端，使用 shadcn/ui 源码组件、Radix UI、Tailwind CSS 和 Lucide 图标
- Web 管理面板：在浏览器中动态增删 Provider、调整检测参数、查看任务历史与 Token 用量、导入导出配置
- 每个 Provider 独立检测开关（`probe_enabled`）：可保留配置但暂停探测，不影响仪表盘展示
- 支持 Telegram、Discord、Bark、企业微信、钉钉和通用 Webhook 告警通知
- 天蓝色主视觉，健康、较慢、异常状态分别使用独立语义色；支持跟随系统 / 深色 / 浅色三态主题
- **无配置启动**：不需要 `.env` 文件，首次运行自动生成密码学随机管理密钥，管理面板引导修改

## 文档

完整文档位于 [docs/](docs/README.md)：

| 文档 | 内容 |
|------|------|
| [架构](docs/architecture.md) | 系统架构、目录结构、检测时序、状态模型 |
| [配置](docs/configuration.md) | 全部环境变量、Provider 配置与校验规则 |
| [API](docs/api.md) | 完整 HTTP API 参考（含示例与错误码） |
| [前后端对接](docs/backend-api.md) | 前端视角的接口、SSE 与数据结构 |
| [数据存储](docs/data-storage.md) | SQLite 表结构、统计口径、迁移与备份 |
| [前端](docs/frontend.md) | 前端结构、页面组件与开发方式 |
| [部署](docs/deployment.md) | Docker / Compose / 二进制 / 反向代理 / CI |
| [安全](docs/security.md) | 认证授权、限流、SSRF 与凭据保护 |
| [运维](docs/operations.md) | 指标、日志、告警与故障排查 |
| [开发](docs/development.md) | 本地开发、测试与发布流程 |

## 快速开始

```bash
go run ./cmd/cg
```

打开 [http://127.0.0.1:8080](http://127.0.0.1:8080) 查看仪表盘，点击右上角 **管理** 进入管理面板。

**首次启动**时若未设置 `ADMIN_TOKEN`，服务会自动生成一个随机密钥并打印到终端：

```
Auto-generated ADMIN_TOKEN: aB3xZ9mK2p...
Please change it on first login.
```

首次进入管理面板时，系统会强制要求修改密钥（至少 16 位），修改完成后自动进入。

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

| 页面 | 路径 | 功能 |
|------|------|------|
| 运行概览 | `/admin/overview` | 查看运行状态、手动触发或停止检测、最近检测摘要和 Token 消耗估算 |
| Provider | `/admin/providers` | 搜索、新增、编辑、删除、暂停和单独重测 Provider |
| 系统设置 | `/admin/settings` | 修改检测、历史、调度、告警和访问控制配置（仅管理员） |
| 任务历史 | `/admin/tasks` | 筛选并分页查看检测任务及结果明细 |
| Token 用量 | `/admin/billing` | 按时间范围查看汇总、每日趋势和模型用量 |
| 配置管理 | `/admin/config` | 导出或导入 JSON 配置、热加载 `.env`（仅管理员） |

后台路径可直接访问和刷新。使用只读分享密钥登录时，仅显示运行概览、Provider、任务历史和 Token 用量，并隐藏所有写操作。

## 部署

### Docker（推荐）

```bash
# 构建镜像
docker build -t model-connectivity .

# 运行（挂载数据目录和配置文件）
docker run -d \
  --name model-connectivity \
  -p 8080:8080 \
  -e ADMIN_TOKEN \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/.env:/app/.env \
  model-connectivity
```

镜像采用三阶段构建：Node 20 构建前端 → Go 编译后端 → distroless/static 最小运行时（非 root），最终镜像不含 shell 和包管理器。

### 二进制部署

1. 从 [Releases](../../releases) 下载对应平台的压缩包并解压（内含预构建的 `web/` 目录）
2. 直接启动：`./model-connectivity`（Windows 运行 `model-connectivity.exe`）
3. 首次启动时终端会打印自动生成的管理密钥，进入管理面板后强制修改

可选：复制 `.env.example` 为 `.env` 并填写 Provider 信息，配置自动检测间隔和告警。

### Docker

容器默认监听 `0.0.0.0`，启动前必须设置至少 16 字符的 `ADMIN_TOKEN`，例如通过 `.env` 或 `export ADMIN_TOKEN=...` 提供。Docker Compose 会在缺失密钥时直接报错。

Linux 使用下列绑定目录前，先创建 `data` 并将其所有者设置为容器账户 `65532:65532`，权限设置为 `0700`。全新部署也可以使用命名卷 `-v model-connectivity-data:/app/data`；已有数据请继续使用原目录。

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e ADMIN_TOKEN \
  --name model-connectivity \
  ghcr.io/wututua/ai_model_connectivity:latest
```

或通过环境变量传入配置：

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e ADMIN_TOKEN \
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

也可以使用 Makefile：

```bash
make dev-backend # 启动后端
make build     # 完整构建（含前端）
make test      # 运行全部测试
make lint      # 运行 go vet
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
| `ADMIN_TOKEN` | 自动生成 | 保护管理接口；未设置时自动生成至少 16 字符的随机密钥；公开监听时必须手动设置 |

#### 管理密钥说明

- **未设置 `ADMIN_TOKEN`**：服务启动时自动生成一个密码学随机密钥，打印到终端，并持久化到 SQLite。首次进入管理面板时会强制要求修改。
- **已设置 `ADMIN_TOKEN`**：直接使用配置值，不触发首次修改流程。修改时需更新环境变量或 `.env` 并重启；网页接口会拒绝临时覆盖，避免重启后旧密钥重新生效。
- **公开部署**（监听 `0.0.0.0` / `::`）：**必须**通过环境变量显式设置 `ADMIN_TOKEN`，自动生成的密钥不足以保障公开暴露的安全。
- 管理密钥和只读分享密钥必须为 16–256 位无空格的可打印 ASCII 字符，且两者不能相同；撤销只读密钥后立即失效。只读登录仅展示状态、Provider、任务和用量，不显示写操作或敏感配置入口。

非回环监听（包括局域网地址、通配地址和自定义主机名）必须显式配置管理密钥。`localhost`、`127.0.0.0/8` 和 `::1` 可以使用自动生成的密钥。

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
| `MAX_HISTORY_RECORDS` | `500` | 每个模型在数据库中最多保留的记录数；写入时执行清理 |
| `SHOW_ERROR_DETAIL` | `true` | 显示错误详情 |
| `THEME_MODE` | `auto` | 主题初始模式：`auto`、`dark`、`light`；前端可实时切换 |
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
PROVIDER_1_PROBE_ENABLED=true

PROVIDER_2_ID=ollama-local
PROVIDER_2_NAME=Ollama
PROVIDER_2_TYPE=ollama
PROVIDER_2_BASE_URL=http://127.0.0.1:11434/v1
PROVIDER_2_API_KEY=
PROVIDER_2_MODELS=llama3.1
PROVIDER_2_ENABLED=true
PROVIDER_2_PROBE_ENABLED=true
```

| 字段 | 说明 |
|------|------|
| `PROVIDER_N_ID` | 唯一标识，用于 API 和告警过滤 |
| `PROVIDER_N_NAME` | 显示名称 |
| `PROVIDER_N_TYPE` | 类型，用于自动匹配图标 |
| `PROVIDER_N_BASE_URL` | API 根地址，须以 `http://` 或 `https://` 开头 |
| `PROVIDER_N_API_KEY` | API Key，留空表示无需鉴权 |
| `PROVIDER_N_MODELS` | 模型列表，逗号分隔；留空时自动请求 `{BASE_URL}/models` 获取 |
| `PROVIDER_N_ENABLED` | `true` 启用，`false` 完全禁用（不展示、不探测） |
| `PROVIDER_N_PROBE_ENABLED` | `true` 参与探测（默认），`false` 保留配置但不发送探测请求 |

`ENABLED=false` 会将 Provider 从仪表盘和探测中完全移除；`PROBE_ENABLED=false`（`ENABLED=true`）则保留仪表盘展示但跳过探测，适合临时暂停某个 Provider 的检测而不删除配置。

Provider ID 最长 128 字符，不允许路径分隔符、控制字符、`?`、`#`，也不能是 `.` 或 `..`。Provider Base URL 不允许用户信息、查询参数或片段，API Key 请使用独立字段；通知 Webhook 可以包含平台要求的查询参数。

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

管理员接口请求时携带：`Authorization: Bearer <ADMIN_TOKEN>`。只读接口也接受管理面板生成的分享密钥。

> 本地监听且未设置 `ADMIN_TOKEN` 时，使用终端打印的自动生成密钥登录，不能留空。

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/admin/detection` | 查看检测运行状态和自动检测配置 |
| `POST` | `/api/admin/detection/start` | 开始检测 |
| `POST` | `/api/admin/detection/stop` | 停止当前检测 |
| `POST` | `/api/admin/check` | 触发一次完整检测 |
| `POST` | `/api/admin/token` | 修改管理密钥 `{"token":"new-token"}` |
| `GET` | `/api/admin/config` | 查看当前配置（通知凭据只返回是否已设置；仅管理员） |
| `PUT` | `/api/admin/settings` | 修改阈值、检测参数、自动检测间隔 |
| `GET` | `/api/admin/providers` | 查看 Provider 列表（不含 API Key） |
| `POST` | `/api/admin/providers` | 新增 Provider |
| `PUT` | `/api/admin/providers/{id}` | 修改 Provider；不传 `api_key` 时保留旧值 |
| `DELETE` | `/api/admin/providers/{id}` | 删除 Provider |
| `POST` | `/api/admin/providers/{id}/rerun` | 单独重跑某个 Provider |
| `GET` | `/api/admin/tasks` | 查看历史检测任务 |
| `GET` | `/api/admin/tasks/{id}` | 查看任务详情 |
| `GET` | `/api/admin/config/export` | 导出配置（不含 Provider/API/通知密钥；仅管理员） |
| `POST` | `/api/admin/config/import` | 导入配置并保存到 SQLite |
| `POST` | `/api/admin/config/reload` | 重新读取 `.env` 并热加载 |

`GET /api/admin/detection`、`GET /api/admin/providers`、`GET /api/admin/tasks`、`GET /api/admin/tasks/{id}`、`GET /api/admin/billing` 和 `/metrics` 接受只读分享密钥；其他 `/api/admin/*` 接口只接受 `ADMIN_TOKEN`。`/api/admin/config` 与 `/api/admin/config/export` 即使使用管理员密钥，也不会返回 Provider API Key 或通知凭据明文。

`/api/admin/detection` 通过 `read_only` 标识当前权限。有效只读密钥访问管理员专属接口返回 `403`，不会计入认证失败限流。

管理 API 的 JSON 请求体上限为 1 MiB，超限返回 `413`，多个 JSON 值或格式错误返回 `400`。同一连接来源 IP 在一分钟内累计 10 次认证失败后返回 `429`，等待 `Retry-After` 指定的时间后重试。应用不信任客户端提供的 `X-Forwarded-For`；反向代理后的用户共享代理 IP 的限额，需要按真实 IP 限流时请在受信任代理上配置。

超时必须是有限正数且不超过 86400 秒，自动检测间隔必须为有限非负数且不超过 8760 小时；并发数上限为 1024。停止检测后，未完成任务标记为 `canceled`，不更新最新报告、历史记录，也不触发告警。模型列表请求失败则会生成 `DEGRADED` 报告，并保留失败 Provider。

## 数据文件

```
web/index.html
web/assets/app.js
web/assets/index.css
data/cg.sqlite
```

`web/` 目录由 Vite 构建生成，发布包内已包含预构建产物，无需手动构建即可运行。

历史检测结果、最新报告、告警状态和管理密钥均保存在 SQLite。首次启动时若存在旧版 JSON 文件（`data/latest_report.json`、`data/probe_history.json`、`data/notify_state.txt`），会自动迁移到 SQLite，旧文件不会被删除。

Token 用量按 UTC 自然日单独汇总，统计区间含当天，最多保留 365 天；关闭历史记录或清理模型历史不会减少已记录用量。升级时仅能迁移数据库中仍存在的记录，之前已删除或取消检测产生的实际消耗无法还原，因此该统计不是供应商账单。历史、用量与最新报告通过同一事务保存。

热加载仅更新运行参数和 Provider。监听地址、静态资源/数据库路径或外部管理密钥变更需要重启。直连请求在实际连接时校验并使用解析后的 IP，禁止链路本地地址及自动重定向；使用 HTTP 代理时，应由可信代理执行相同目标地址策略。

数据库和已存在的 WAL、SHM、journal 文件在打开时收紧权限：Unix 使用 `0600`，Windows 使用受保护 ACL，只授予当前服务账户和 SYSTEM 访问权限。新建的数据目录在 Unix 使用 `0700`。这属于文件访问控制，不是数据库加密；服务账户仍可读取其中的凭据，备份文件应采用同等级权限或存入加密介质。

## 安全提示

- 该项目会真实调用模型接口并消耗 token，建议使用最小化探测提示词（默认已优化），并适当拉长检测间隔。
- **公开部署必须显式配置至少 16 字符的 `ADMIN_TOKEN`**，否则服务拒绝启动。自动生成的密钥仅用于回环监听。
- 管理面板首次登录后强制要求修改密钥，后续密钥持久化存储在 SQLite，重启后无需重新配置。
- 只读分享密钥仅用于只读状态、任务、计费和指标接口，不能读取运行配置或导出配置。
- Provider `base_url` 和通知 Webhook 会校验 scheme（须为 `http://` 或 `https://`）、主机和用户信息，并拦截链路本地地址；实际请求还会检查 DNS 结果且禁止跟随重定向，降低 SSRF 风险。

发布工作流在 Windows 和 Linux 上执行后端测试，并在 Linux 上执行竞态检测、前端构建、Compose 校验、容器认证及健康检查。容器验证通过后才构建发布二进制或推送镜像。

## License

[MIT](LICENSE)
