<div align="center">
  <img src="assets/gpt_uptime_v2.png" alt="AI Model Connectivity" width="160">
  <h1>AI Model Connectivity</h1>
  <p>面向 OpenAI 兼容接口的轻量级、自托管模型可用性监控平台</p>
  <p>
    <a href="https://github.com/wututua/AI_Model_Connectivity/actions/workflows/ci.yml"><img src="https://github.com/wututua/AI_Model_Connectivity/actions/workflows/ci.yml/badge.svg?branch=main" alt="GitHub CI"></a>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26.8-00ADD8?logo=go&amp;logoColor=white" alt="Go 1.26.8"></a>
    <a href="https://react.dev/"><img src="https://img.shields.io/badge/React-18.3-61DAFB?logo=react&amp;logoColor=20232A" alt="React 18.3"></a>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/wututua/AI_Model_Connectivity" alt="MIT License"></a>
  </p>
  <p>
    <a href="https://github.com/wututua/AI_Model_Connectivity">GitHub 仓库</a>
    <span> · </span>
    <a href="docs/README.md">项目文档</a>
    <span> · </span>
    <a href="https://github.com/wututua/AI_Model_Connectivity/releases">GitHub Releases</a>
  </p>
</div>

AI Model Connectivity（简称 CG）会定期探测多个 Provider 及其模型，集中展示可用率、延迟、历史状态和 Token 用量，并在状态变化时发送告警。服务由单个 Go 进程、React 管理界面和本地 SQLite 组成，适合个人、团队或内网环境自托管。

## 目录

- [主要特性](#主要特性)
- [快速开始](#快速开始)
- [部署](#部署)
- [配置 Provider](#配置-provider)
- [管理与监控](#管理与监控)
- [项目文档](#项目文档)
- [本地开发](#本地开发)
- [安全说明](#安全说明)
- [参与贡献](#参与贡献)
- [许可证](#许可证)

## 主要特性

- **OpenAI 兼容探测**：检查 `/v1/models` 与 `/v1/chat/completions`，支持显式模型列表或自动发现。
- **多 Provider 管理**：通过 Web 管理面板增删、编辑、暂停或单独重测 Provider，无需重启。
- **可靠的并发控制**：全局与单 Provider 两级并发限制，避免集中探测触发上游限流。
- **状态与趋势**：展示正常、较慢、异常、未检测、暂停状态，以及历史灯、24 小时延迟分位数和统计窗口检测成功率。
- **实时更新**：优先使用 SSE 推送，连接失败时自动降级为 30、60、120 秒退避轮询。
- **响应校验**：剥离 `<think>` / `<thinking>` 内容后检查实际回复；不保证所有推理模型兼容，详见下方兼容范围。
- **告警通知**：支持 Telegram、Discord、Bark、企业微信、钉钉和通用 Webhook，提供恢复通知、冷却与 Provider/模型过滤。
- **账号与权限**：账号密码登录；管理员管理用户与配置，普通用户只读；可开启状态监控页登录限制。
- **本地持久化**：使用纯 Go SQLite 驱动，无需部署外部数据库；支持旧版 JSON 数据自动迁移。
- **轻量部署**：支持源码、跨平台二进制、Docker 和 Docker Compose；容器使用非 root 的 distroless 运行时。

## 快速开始

### 获取源码

从 GitHub 克隆仓库：

```bash
git clone https://github.com/wututua/AI_Model_Connectivity.git
cd AI_Model_Connectivity
```

### 启动服务

需要 Go 1.26.8。仓库已包含构建后的前端资源，因此首次体验不需要安装 Node.js 或准备配置文件。日常配置通过管理面板保存到 SQLite，启动参数可通过进程环境变量指定。

```bash
go run ./cmd/cg
```

启动后打开：

- 仪表盘：<http://127.0.0.1:8080>
- 管理面板：<http://127.0.0.1:8080/admin>
- 健康检查：<http://127.0.0.1:8080/health>

首次启动会创建 `admin` 账号。可通过 `ADMIN_USERNAME`、`ADMIN_PASSWORD` 指定初始账号；未设置密码时随机生成并打印到终端。首次登录必须修改密码，规则为至少 8 位且包含大写字母、小写字母和数字，不要求特殊符号。

```text
Administrator account: admin
Initial administrator password: <随机生成的密码>
```

进入管理面板的 **Provider** 页面添加服务，然后在 **运行概览** 中触发第一次检测。

## 部署

### Docker Compose

Docker Compose 会从当前源码构建镜像。建议预设 `ADMIN_PASSWORD` 并通过 HTTPS 反向代理访问；HTTPS 部署设置 `SECURE_COOKIES=true`。

```bash
export ADMIN_PASSWORD='<替换为符合规则的初始密码>'
# Provider 在管理面板添加
docker compose up -d --build
```

Windows PowerShell：

```powershell
$env:ADMIN_PASSWORD = '<替换为符合规则的初始密码>'
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
  -e ADMIN_PASSWORD='<替换为符合规则的初始密码>' \
  -v model-connectivity-data:/app/data \
  model-connectivity:local
```

### 预编译二进制

发布流水线提供 Linux、Windows、macOS 的 amd64/arm64 压缩包，包内包含固定名称的可执行文件、README、LICENSE、`docs/`、`assets/` 和预构建的 `web/`（含字体许可），另提供 SHA-256 校验文件：

- [GitHub Releases](https://github.com/wututua/AI_Model_Connectivity/releases)

解压后直接运行 `model-connectivity`，Windows 使用 `model-connectivity.exe`。完整的 Docker、systemd、反向代理、升级和回滚说明见 [部署指南](docs/deployment.md)。

## 配置 Provider

推荐在管理面板中维护 Provider。仅首次初始化空数据库时，也可以通过进程环境变量提供初始值，例如 Bash：

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

数据库已有配置时，环境变量不会覆盖已保存的 Provider 和运行设置；请使用后台编辑或 JSON 配置导入。

`PROVIDER_N_MODELS` 留空时，服务会从 `{BASE_URL}/models` 自动发现模型。`ENABLED=false` 会完全隐藏并停用 Provider；`PROBE_ENABLED=false` 会保留展示但暂停探测。

**兼容范围**：当前使用 Chat Completions 非流式文本请求，固定 `max_tokens=16`、`temperature=0`。不支持仅 Responses、原生 Anthropic/Gemini、图像、音频或嵌入端点；要求不同参数或较大推理预算的模型可能失败。模型返回非空有效回复才算成功，用量以接口返回为准，不等于供应商账单。

常用配置：

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `APP_HOST` | `127.0.0.1` | HTTP 监听地址 |
| `APP_PORT` | `8080` | HTTP 监听端口 |
| `ADMIN_USERNAME` | `admin` | 首次创建的管理员账号 |
| `ADMIN_PASSWORD` | 随机生成 | 初始密码，已有账号时不再覆盖 |
| `STATUS_LOGIN_REQUIRED` | `false` | 状态页需登录；可在后台修改并持久化 |
| `SECURE_COOKIES` | `false` | HTTPS 反向代理部署时开启 |
| `TIMEOUT_SECONDS` | `30` | 单模型探测超时 |
| `SLOW_THRESHOLD_MS` | `800` | 较慢状态阈值 |
| `CONCURRENCY` | `1` | 全局探测并发上限 |
| `PROVIDER_CONCURRENCY` | `1` | 单 Provider 并发上限 |
| `AUTO_CHECK_INTERVAL_MIN_HOURS` | `0` | 自动检测最短间隔，`0` 表示关闭 |
| `AUTO_CHECK_INTERVAL_MAX_HOURS` | `0` | 自动检测最长间隔 |

完整变量、校验规则和配置生效方式见 [配置参考](docs/configuration.md)。

## 管理与监控

| 页面 | 路径 | 用途 |
|------|------|------|
| 运行概览 | `/admin/overview` | 查看运行状态，启动后台检测并跟踪完成状态 |
| Provider | `/admin/providers` | 搜索、新增、编辑、暂停、删除和重测 Provider |
| 系统设置 | `/admin/settings` | 修改探测、历史、调度、告警和访问控制配置 |
| 任务历史 | `/admin/tasks` | 筛选检测任务并查看结果明细 |
| Token 用量 | `/admin/billing` | 查看汇总、每日趋势和模型用量 |
| 配置管理 | `/admin/config` | 导入导出 JSON 配置 |
| 用户管理 | `/admin/users` | 管理员新增、编辑、禁用、删除用户及重置密码 |
| 账户安全 | `/admin/account` | 修改自己的密码 |
| 登录 | `/login` | 管理员和普通用户共用登录入口 |

`/health` 始终公开。开启 **系统设置 → 访问控制 → 查看状态监控需要登录** 后，`/api/status` 与 `/api/events` 也要求有效账号会话。管理 API 使用 HttpOnly 会话 Cookie，写请求额外校验 CSRF；普通用户不能修改配置或运行检测。`/metrics` 同样需要账号会话。接口细节见 [HTTP API 参考](docs/api.md)。

旧版本升级时，符合密码规则的管理 Token 会迁移为初始管理员密码，否则生成新密码；旧 Bearer 和只读分享密钥不再被接受。已有账号不会因重启或修改 `ADMIN_PASSWORD` 被重置。

忘记管理员密码时，先停止服务，在相同工作目录与数据配置下运行 `model-connectivity recover-admin admin`，获得临时密码并在登录后修改；其他数据不变。操作细节见 [运维指南](docs/operations.md#9-管理员密码恢复)。

## 项目文档

| 文档 | 内容 |
|------|------|
| [文档索引](docs/README.md) | 按使用者、运维者和开发者分类的阅读入口 |
| [系统架构](docs/architecture.md) | 模块边界、启动流程、检测时序和状态模型 |
| [配置参考](docs/configuration.md) | 环境变量、Provider、校验与配置持久化 |
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

前端开发与 CI 使用 Node.js 24：

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
cd frontend && npm test && npm run build
```

## 技术栈

| 层级 | 技术 |
|------|------|
| 后端 | Go 1.26.8、SQLite、SSE、Prometheus |
| 前端 | React 18、TypeScript、Vite 7、React Router 7 |
| UI | shadcn/ui、Radix UI、Tailwind CSS、Lucide |
| 部署 | Docker、Docker Compose、distroless、跨平台二进制 |
| CI/CD | GitHub Actions |

## 项目结构

```text
cmd/cg/              程序入口与应用编排
internal/config/     配置解析、运行时配置和校验
internal/auth/       密码校验与加盐哈希
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

- 所有后台接口必须登录，管理员权限在后端校验；新账号和重置密码后首次登录必须改密。
- 密码以 PBKDF2-SHA256 加盐哈希存储，会话凭据仅保存摘要；Provider API Key 和通知凭据仍依赖 SQLite 文件权限保护，未进行静态加密。
- 配置导出和管理 API 不返回 API Key 或通知凭据明文。
- 部署到公网时使用 HTTPS 反向代理并设置 `SECURE_COOKIES=true`，限制管理入口并定期更新密码和上游密钥。
- 探测会真实请求上游并消耗 Token，请根据模型数量合理设置检测周期与并发。

更多威胁模型和加固建议见 [安全说明](docs/security.md)。安全问题请不要公开披露敏感凭据或可直接利用的细节。

## 参与贡献

1. 从 [GitHub 上游仓库](https://github.com/wututua/AI_Model_Connectivity) Fork 并创建功能分支。
2. 保持改动聚焦，新增或修改行为时补充相应测试与文档。
3. 提交前运行后端测试、静态检查和前端构建。
4. 通过 GitHub Pull Request 提交改动。

提交问题前请先搜索现有 [GitHub Issues](https://github.com/wututua/AI_Model_Connectivity/issues)，并附上版本、部署方式、复现步骤和已脱敏日志。

## 许可证

本项目基于 [MIT License](LICENSE) 开源。

**本项目使用 HarmonyOS Sans 字体。** 字体版权归 Huawei Device Co., Ltd. 所有，适用独立的 [HarmonyOS Sans 字体许可](web/fonts/harmonyos-sans/LICENSE.txt)，不属于项目的 MIT 授权范围。字体来源见 [字体声明](web/fonts/harmonyos-sans/NOTICE.md)。
