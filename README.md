<div align="center">
  <img src="assets/gpt_uptime_v2.png" alt="AI Model Connectivity" width="160">
  <h1>AI Model Connectivity</h1>
  <p>面向 OpenAI 兼容接口的轻量级、自托管模型可用性监控平台</p>
  <p>
    <a href="https://github.com/wututua/AI_Model_Connectivity/actions/workflows/ci.yml"><img src="https://github.com/wututua/AI_Model_Connectivity/actions/workflows/ci.yml/badge.svg?branch=main" alt="GitHub CI"></a>
    <a href="https://cnb.cool/ligzs/AI_Model_Connectivity"><img src="https://img.shields.io/badge/CNB-镜像仓库-00B578" alt="CNB"></a>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&amp;logoColor=white" alt="Go 1.25"></a>
    <a href="https://react.dev/"><img src="https://img.shields.io/badge/React-18.3-61DAFB?logo=react&amp;logoColor=20232A" alt="React 18.3"></a>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/wututua/AI_Model_Connectivity" alt="MIT License"></a>
  </p>
  <p>
    <a href="https://github.com/wututua/AI_Model_Connectivity">GitHub 仓库</a>
    <span> · </span>
    <a href="https://cnb.cool/ligzs/AI_Model_Connectivity">CNB 仓库</a>
    <span> · </span>
    <a href="docs/README.md">项目文档</a>
    <span> · </span>
    <a href="https://github.com/wututua/AI_Model_Connectivity/releases">GitHub Releases</a>
    <span> · </span>
    <a href="https://cnb.cool/ligzs/AI_Model_Connectivity/-/releases">CNB Releases</a>
  </p>
</div>

> 项目同时托管在 [GitHub](https://github.com/wututua/AI_Model_Connectivity) 和 [CNB](https://cnb.cool/ligzs/AI_Model_Connectivity)。GitHub 是上游源码仓库，CNB 提供国内访问友好的同步镜像、流水线和独立 Release；同一份 README 在两个仓库中都可以直接跳转到另一端。

AI Model Connectivity（简称 CG）会定期探测多个 Provider 及其模型，集中展示可用率、延迟、历史状态和 Token 用量，并在状态变化时发送告警。服务由单个 Go 进程、React 管理界面和本地 SQLite 组成，适合个人、团队或内网环境自托管。

## 目录

- [主要特性](#主要特性)
- [快速开始](#快速开始)
- [部署](#部署)
- [配置 Provider](#配置-provider)
- [管理与监控](#管理与监控)
- [CNB 支持](#cnb-支持)
- [项目文档](#项目文档)
- [本地开发](#本地开发)
- [安全说明](#安全说明)
- [参与贡献](#参与贡献)
- [许可证](#许可证)

## 主要特性

- **OpenAI 兼容探测**：检查 `/v1/models` 与 `/v1/chat/completions`，支持显式模型列表或自动发现。
- **多 Provider 管理**：通过 Web 管理面板增删、编辑、暂停或单独重测 Provider，无需重启。
- **可靠的并发控制**：全局与单 Provider 两级并发限制，避免集中探测触发上游限流。
- **状态与趋势**：展示正常、较慢、异常、暂停四种状态，以及历史灯、24 小时延迟分位数和统计窗口可用率。
- **实时更新**：优先使用 SSE 推送，连接失败时自动降级为 30、60、120 秒退避轮询。
- **推理模型兼容**：自动剥离 `<think>` / `<thinking>` 内容，兼容 DeepSeek-R1、QwQ 等模型。
- **告警通知**：支持 Telegram、Discord、Bark、企业微信、钉钉和通用 Webhook，提供恢复通知、冷却与 Provider/模型过滤。
- **权限分离**：管理密钥拥有完整权限，只读分享密钥仅可查看状态、任务、用量和 Prometheus 指标。
- **本地持久化**：使用纯 Go SQLite 驱动，无需部署外部数据库；支持旧版 JSON 数据自动迁移。
- **轻量部署**：支持源码、跨平台二进制、Docker 和 Docker Compose；容器使用非 root 的 distroless 运行时。

## 快速开始

### 获取源码

任选一个仓库克隆。CNB 镜像会定时从 GitHub 同步源码和标签。

```bash
# GitHub
git clone https://github.com/wututua/AI_Model_Connectivity.git

# 或 CNB
git clone https://cnb.cool/ligzs/AI_Model_Connectivity.git

cd AI_Model_Connectivity
```

### 启动服务

需要 Go 1.25。仓库已包含构建后的前端资源，因此首次体验不需要安装 Node.js，也不要求预先创建 `.env`。

```bash
go run ./cmd/cg
```

启动后打开：

- 仪表盘：<http://127.0.0.1:8080>
- 管理面板：<http://127.0.0.1:8080/admin>
- 健康检查：<http://127.0.0.1:8080/health>

未设置 `ADMIN_TOKEN` 时，服务会在首次启动时生成 24 字符的随机管理密钥并打印到终端。使用该密钥登录后，管理面板会要求立即修改。

```text
Auto-generated ADMIN_TOKEN: aB3xZ9mK2p...
Please change it on first login.
```

进入管理面板的 **Provider** 页面添加服务，然后在 **运行概览** 中触发第一次检测。

## 部署

### Docker Compose

Docker Compose 会从当前源码构建镜像。容器监听公开地址，因此必须显式设置至少 16 字符的 `ADMIN_TOKEN`。

```bash
cp .env.example .env
# 编辑 .env，至少填写 ADMIN_TOKEN；Provider 也可以稍后在管理面板添加
docker compose up -d --build
```

Windows PowerShell：

```powershell
Copy-Item .env.example .env
# 编辑 .env 后启动
docker compose up -d --build
```

Linux 使用仓库中的绑定目录前，需要让容器账户可以写入：

```bash
mkdir -p data
sudo chown 65532:65532 data
sudo chmod 0700 data
```

### 本地构建镜像

```bash
docker build -t model-connectivity:local .
docker run -d \
  --name model-connectivity \
  -p 8080:8080 \
  -e ADMIN_TOKEN='replace-with-a-secret-at-least-16-characters' \
  -v model-connectivity-data:/app/data \
  model-connectivity:local
```

### 预编译二进制

发布流水线提供 Linux、Windows、macOS 的 amd64/arm64 压缩包，包内包含二进制、`.env.example`、README 和预构建的 `web/`：

- [GitHub Releases](https://github.com/wututua/AI_Model_Connectivity/releases)
- [CNB Releases](https://cnb.cool/ligzs/AI_Model_Connectivity/-/releases)

解压后直接运行 `model-connectivity`，Windows 使用 `model-connectivity.exe`。完整的 Docker、systemd、反向代理、升级和回滚说明见 [部署指南](docs/deployment.md)。

## 配置 Provider

推荐在管理面板中维护 Provider。也可以复制 `.env.example` 并使用环境变量初始化：

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

`PROVIDER_N_MODELS` 留空时，服务会从 `{BASE_URL}/models` 自动发现模型。`ENABLED=false` 会完全隐藏并停用 Provider；`PROBE_ENABLED=false` 会保留展示但暂停探测。

常用配置：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `APP_PORT` | `8080` | HTTP 监听端口 |
| `ADMIN_TOKEN` | 本地自动生成 | 管理密钥；非回环监听时必须显式设置 |
| `TIMEOUT_SECONDS` | `30` | 单模型探测超时 |
| `SLOW_THRESHOLD_MS` | `800` | 较慢状态阈值 |
| `CONCURRENCY` | `1` | 全局探测并发上限 |
| `PROVIDER_CONCURRENCY` | `1` | 单 Provider 并发上限 |
| `AUTO_CHECK_INTERVAL_MIN_HOURS` | `0` | 自动检测最短间隔，`0` 表示关闭 |
| `AUTO_CHECK_INTERVAL_MAX_HOURS` | `0` | 自动检测最长间隔 |

完整变量、校验规则和热加载语义见 [配置参考](docs/configuration.md)。

## 管理与监控

| 页面 | 路径 | 用途 |
|------|------|------|
| 运行概览 | `/admin/overview` | 查看运行状态，触发或停止检测 |
| Provider | `/admin/providers` | 搜索、新增、编辑、暂停、删除和重测 Provider |
| 系统设置 | `/admin/settings` | 修改探测、历史、调度、告警和访问控制配置 |
| 任务历史 | `/admin/tasks` | 筛选检测任务并查看结果明细 |
| Token 用量 | `/admin/billing` | 查看汇总、每日趋势和模型用量 |
| 配置管理 | `/admin/config` | 导入导出 JSON 配置、热加载 `.env` |

公开端点包括 `/health`、`/api/status` 和 `/api/events`。管理 API 使用 `Authorization: Bearer <token>`；Prometheus 指标位于 `/metrics`，接受管理密钥或只读分享密钥。接口细节见 [HTTP API 参考](docs/api.md)。

## CNB 支持

[CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) 不只是代码镜像，仓库内的 [`.cnb.yml`](.cnb.yml) 还提供完整的云原生构建与发布流程：

| 能力 | 当前行为 |
|------|----------|
| 国内镜像 | 每天北京时间 01:00、09:00、17:00 从 GitHub 拉取源码和标签 |
| 构建缓存 | 复用 Go module 与 Go build 缓存卷，缩短重复构建时间 |
| 自动测试 | 发布前在 Go 1.25 容器中运行全部单元测试 |
| 前端构建 | 使用 Node 20 和 `npm ci` 生成可随二进制分发的 `web/` |
| 并行跨平台构建 | 同时产出 Linux、Windows、macOS 的 amd64/arm64，共 6 个目标 |
| 自动 Release | `tag_push` 或 `tag_deploy.release` 触发打包并发布 CNB Release |

同步方向是 **GitHub → CNB**。若两个仓库短时间内显示的提交不同，请以 GitHub 的 `main` 为准，或等待下一次定时同步。完整说明见 [仓库与发布渠道](docs/repositories.md)。

## 项目文档

| 文档 | 内容 |
|------|------|
| [文档索引](docs/README.md) | 按使用者、运维者和开发者分类的阅读入口 |
| [仓库与发布渠道](docs/repositories.md) | GitHub/CNB 地址、同步机制、流水线和 Release |
| [系统架构](docs/architecture.md) | 模块边界、启动流程、检测时序和状态模型 |
| [配置参考](docs/configuration.md) | 环境变量、Provider、校验与热加载 |
| [HTTP API](docs/api.md) | 认证、错误码、端点和数据结构 |
| [前后端对接](docs/backend-api.md) | 前端视角的 API、SSE 和类型契约 |
| [数据存储](docs/data-storage.md) | SQLite 表、事务、统计、迁移和备份 |
| [前端说明](docs/frontend.md) | React 结构、页面、主题和构建产物 |
| [部署指南](docs/deployment.md) | Compose、二进制、systemd、反向代理和 CI |
| [安全说明](docs/security.md) | 认证授权、SSRF、防泄漏和加固建议 |
| [运维指南](docs/operations.md) | 健康检查、Prometheus、日志、告警和排障 |
| [开发指南](docs/development.md) | 开发环境、测试约定和发布流程 |

## 本地开发

后端直接运行：

```bash
go run ./cmd/cg
```

前端热更新需要 Node.js 20+：

```bash
cd frontend
npm ci
npm run dev
```

Vite 默认运行在 <http://127.0.0.1:5173>，并将 `/api` 和 `/health` 代理到 `http://localhost:8080`。

提交前建议执行：

```bash
go vet ./...
go test -race ./...
cd frontend && npm run build
```

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.25、SQLite、SSE、Prometheus |
| 前端 | React 18、TypeScript、Vite 5 |
| UI | shadcn/ui、Radix UI、Tailwind CSS、Lucide |
| 部署 | Docker、Docker Compose、distroless、跨平台二进制 |
| CI/CD | GitHub Actions、CNB 云原生构建 |

## 项目结构

```text
cmd/cg/              程序入口与应用编排
internal/config/     配置解析、运行时配置和校验
internal/provider/   OpenAI 兼容 Provider 客户端
internal/probe/      探测目标收集与并发执行
internal/report/     报告、统计和延迟曲线
internal/storage/    SQLite 持久化与迁移
internal/notify/     状态告警与平台适配
internal/metrics/    Prometheus 指标
internal/web/        HTTP API、认证、SSE 和静态托管
frontend/            React + TypeScript 源码
web/                 已构建的前端静态资源
docs/                项目文档
```

## 安全说明

- 非回环监听（包括 `0.0.0.0`、局域网地址和自定义主机名）必须显式配置 `ADMIN_TOKEN`，否则服务拒绝启动。
- Provider API Key、自动生成的管理密钥和通知凭据保存在本地 SQLite 中，依赖文件权限保护，并未进行静态加密。
- 配置导出和管理 API 不返回 API Key 或通知凭据明文。
- 部署到公网时应使用 HTTPS 反向代理、限制管理入口并定期轮换密钥。
- 探测会真实请求上游并消耗 Token，请根据模型数量合理设置检测周期与并发。

更多威胁模型和加固建议见 [安全说明](docs/security.md)。安全问题请不要公开披露敏感凭据或可直接利用的细节。

## 参与贡献

1. 从 [GitHub 上游仓库](https://github.com/wututua/AI_Model_Connectivity) Fork 并创建功能分支。
2. 保持改动聚焦，新增或修改行为时补充相应测试与文档。
3. 提交前运行后端测试、静态检查和前端构建。
4. 通过 GitHub Pull Request 提交改动；CNB 主要用于同步访问和自动发布。

提交问题前请先搜索现有 [GitHub Issues](https://github.com/wututua/AI_Model_Connectivity/issues)，并附上版本、部署方式、复现步骤和已脱敏日志。

## 许可证

本项目基于 [MIT License](LICENSE) 开源。
