<div align="center">
  <img src="assets/gpt_uptime_v2.png" alt="AI Model Connectivity" width="120">
  <h1>AI Model Connectivity</h1>
  <p>自托管的 OpenAI 兼容接口连通性监控工具</p>
  <p>
    <a href="https://github.com/wututua/AI_Model_Connectivity/actions/workflows/ci.yml"><img src="https://github.com/wututua/AI_Model_Connectivity/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26.8-00ADD8?logo=go&amp;logoColor=white" alt="Go 1.26.8"></a>
    <a href="LICENSE"><img src="https://img.shields.io/github/license/wututua/AI_Model_Connectivity" alt="MIT License"></a>
  </p>
  <p>
    <a href="#快速开始">快速开始</a> ·
    <a href="docs/README.md">文档</a> ·
    <a href="https://github.com/wututua/AI_Model_Connectivity/releases">下载</a> ·
    <a href="CHANGELOG.md">更新日志</a> ·
    <a href="CONTRIBUTING.md">参与贡献</a>
  </p>
</div>

AI Model Connectivity（简称 CG）定期检测多个服务商及其模型，集中展示检测状态、延迟、历史记录和 Token 用量，并在状态变化时发送通知。Go 后端、React 界面和 SQLite 数据库运行在一个自托管服务中，无需额外部署数据库。

> **版本状态**
>
> 当前版本为 [v1.0.0-beta.4](docs/releases/v1.0.0-beta.4.md)，适合评估和试用，不是稳定版。升级前请备份数据库，并阅读对应版本的 Release 说明。检测会调用真实模型接口并消耗 Token。

## 功能

beta.4 新增[监控中心](docs/monitoring-center.md)：诊断详情、模型清单提醒、定期备份、独立告警、事件、Provider 调度、能力断言及费用估算。

- **多服务商管理**：分组、标签、批量启停与暂停，无密钥复制配置，支持单模型和失败项重测。
- **探测兼容性**：按 Provider 配置提示词、输出上限、temperature 和超时，支持 Chat Completions、Responses 及流式首段文本延迟。
- **模型选择**：手动指定模型或自动发现，支持全局与单 Provider 并发限制。
- **状态监控**：实时状态、历史状态灯、延迟曲线、P50/P95/P99 和检测成功率。
- **任务与用量**：实时检测进度、每日请求预算、自动发现确认阈值，支持历史与用量 CSV 导出。
- **通知**：支持 Webhook、Telegram、Discord、Bark、企业微信和钉钉，提供范围过滤、恢复通知、冷却、防抖、维护窗口、测试、发送历史和手动重试。
- **运维**：脱敏诊断导出、可轮换与撤销的 Prometheus 指标专用凭据。
- **系统更新**：检查稳定版与预发布版本；脚本安装的 Linux 可显式启用管理员确认的一键更新、备份及失败恢复。
- **账号权限**：管理员和只读用户，可为状态页开启登录保护。
- **部署**：支持 Linux、Windows、macOS 二进制，以及 Docker 和 Docker Compose。

## 快速开始

选择一种安装方式即可。日常设置通过管理面板维护，保存到 SQLite 后重启仍然生效。

### Linux 安装脚本

适用于使用 systemd 的 Linux amd64 / arm64。先下载并检查[安装脚本](install-model-connectivity.sh)，再运行；需要 Bash 4+、Python 3.8+、curl 和系统管理工具。

```bash
sudo bash install-model-connectivity.sh
```

在终端运行时先选择语言：`1. 简体中文`、`2. English`，回车默认简体中文；后续菜单、提示和帮助使用所选语言。无参数显示管理菜单。当前评估预发布版本时，显式选择 `preview` 通道：

```bash
sudo bash install-model-connectivity.sh install --channel preview
```

安装时可输入自定义监听 IP 和端口，回车默认 `127.0.0.1:8080`；升级保留已有设置。使用独立服务账户，首次初始化开启状态页登录要求。脚本校验发布包、保留完整前端，升级前停服备份；卸载保留数据库。参数、升级与恢复限制见[安装脚本指南](docs/deployment.md#linux-安装脚本)。

### 使用发布包

无需安装 Go 或 Node.js。

1. 在 [Releases](https://github.com/wututua/AI_Model_Connectivity/releases) 下载与你的系统和架构匹配的压缩包。
2. 使用随版本提供的 `SHA256SUMS.txt` 核对文件，完整解压，保留 `web/` 目录。
3. 在解压目录启动服务。

Linux / macOS：

```bash
./model-connectivity
```

Windows PowerShell：

```powershell
.\model-connectivity.exe
```

### 使用 Docker Compose

需要 Git、Docker 和 Compose v2。仓库中的 Compose 配置会从源码构建镜像：

```bash
git clone https://github.com/wututua/AI_Model_Connectivity.git
cd AI_Model_Connectivity
```

Linux 使用绑定挂载前，先准备容器账户可写的数据目录：

```bash
mkdir -p data
sudo chown 65532:65532 data
sudo chmod 0700 data
```

启动并查看首次启动日志：

```bash
docker compose up -d --build
docker compose logs model-connectivity
```

数据保存在仓库下的 `data/`。日志可能包含初始管理员密码，请勿公开分享。Compose 默认发布主机的 `8080` 端口；公网部署前请完成[访问加固](docs/deployment.md#公网访问)。

### 从源码运行

需要 Go 1.26.8，具体要求见 [go.mod](go.mod)。仓库已提交前端构建产物，仅运行服务不需要 Node.js。

```bash
git clone https://github.com/wututua/AI_Model_Connectivity.git
cd AI_Model_Connectivity
go run ./cmd/cg
```

### 首次使用

1. 打开管理面板：<http://127.0.0.1:8080/admin>。
2. 使用初始化的管理员账号登录。默认用户名为 `admin`；未预设密码且没有可迁移的旧凭据时，随机密码会显示在启动终端或容器日志中。
3. 按提示修改初始密码，至少 8 位，包含大写字母、小写字母和数字。
4. 在 **Provider** 页面填写接口地址、密钥和模型，在 **运行概览** 发起检测。
5. 打开状态页 <http://127.0.0.1:8080> 查看结果；按需配置定时检测、通知和状态页登录保护。

## 配置

| 配置类型 | 修改方式 |
| --- | --- |
| Provider、检测参数、历史、通知、状态页登录要求 | 管理面板或 API，写入 SQLite 后立即生效 |
| 监听地址、数据路径、静态资源路径、安全 Cookie | 进程环境变量，修改后重启 |
| 初始管理员账号与密码 | 仅首次创建账号时读取环境变量 |

例如在 Bash 中修改监听端口：

```bash
APP_PORT=8081 ./model-connectivity
```

**已有数据库中的运行设置优先于同名环境变量。** 修改 `ADMIN_PASSWORD` 不会重置已有账号，忘记密码请使用[管理员恢复流程](docs/operations.md#管理员密码恢复)，不要删除数据库。

完整变量与示例见[配置参考](docs/configuration.md)；容器、反向代理、systemd 和升级步骤见[部署指南](docs/deployment.md)。

## 兼容范围与限制

- 支持 OpenAI 兼容的 Chat Completions / Responses 文本及流式探测，以及非流式工具调用结构与 Embedding 探测；不直接支持原生 Anthropic/Gemini、图像或音频探测。
- 旧配置默认仍为 `temperature=0`、`max_tokens=16`；参数支持取决于上游，兼容预设不能保证所有模型可用。详见[检测与运维功能](docs/monitoring-features.md)。
- 未指定模型时会自动发现模型，可能扩大探测范围和 Token 消耗。
- 检测成功率反映采样请求结果，不等于连续在线时间；用量仅来自上游返回的数据，不能替代供应商账单。
- 采用单实例 SQLite 存储，不要让多个服务或单次检测进程并发使用同一数据库。
- Provider API Key 和通知凭据未进行静态加密，请保护数据目录和备份。

## 文档

| 我想要 | 阅读 |
| --- | --- |
| 安装、升级或部署到服务器 | [部署指南](docs/deployment.md) |
| 检查版本或启用后台更新 | [系统更新](docs/system-updates.md) |
| 设置 Provider、通知或检测周期 | [配置参考](docs/configuration.md) |
| 配置探测、预算、批量管理或数据导出 | [检测与运维功能](docs/monitoring-features.md) |
| 备份、恢复密码或排查故障 | [运维指南](docs/operations.md) |
| 调用 API 或集成状态数据 | [HTTP API](docs/api.md) |
| 了解架构并参与开发 | [开发指南](docs/development.md) |
| 查看全部文档 | [文档索引](docs/README.md) |

## 参与贡献

欢迎提交问题、改进文档或贡献代码。提交前请先搜索已有 [Issues](https://github.com/wututua/AI_Model_Connectivity/issues)，并阅读[贡献指南](CONTRIBUTING.md)。

漏洞或凭据泄露请按[安全报告指南](SECURITY.md)处理，不要在公开 Issue 中提交敏感信息。

## 许可证

项目代码使用 [MIT License](LICENSE)。

HarmonyOS Sans 字体使用独立的[字体许可证](web/fonts/harmonyos-sans/LICENSE.txt)，不属于项目的 MIT 授权范围。来源和再分发说明见[字体声明](web/fonts/harmonyos-sans/NOTICE.md)。
