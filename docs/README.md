# 项目文档

> [项目主页](../README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) · [GitHub Releases](https://github.com/wututua/AI_Model_Connectivity/releases) · [CNB Releases](https://cnb.cool/ligzs/AI_Model_Connectivity/-/releases)

AI Model Connectivity（简称 CG，Go module 名为 `cg`）是一个用于检测 OpenAI 兼容接口连通性的独立 Web 服务。Go 后端负责探测、聚合、告警和 API，React 前端提供公开仪表盘与管理面板，运行数据保存在本地 SQLite。

## 选择阅读路径

| 读者 | 建议顺序 |
|------|----------|
| 初次使用 | [根 README](../README.md) → [配置参考](configuration.md) → [部署指南](deployment.md) |
| 运维人员 | [部署指南](deployment.md) → [安全说明](security.md) → [运维指南](operations.md) → [数据存储](data-storage.md) |
| 后端/API 开发 | [系统架构](architecture.md) → [HTTP API](api.md) → [数据存储](data-storage.md) → [开发指南](development.md) |
| 前端开发 | [前端说明](frontend.md) → [前后端对接](backend-api.md) → [HTTP API](api.md) → [开发指南](development.md) |
| 发布维护 | [仓库与发布渠道](repositories.md) → [部署指南](deployment.md#8-发布流水线) → [开发指南](development.md#7-发布) |

## 文档目录

| 文档 | 内容 |
|------|------|
| [仓库与发布渠道](repositories.md) | GitHub/CNB 地址、同步方向、CNB 特性、CI/CD 和 Release |
| [系统架构](architecture.md) | 模块边界、目录结构、启动流程、检测时序和状态模型 |
| [配置参考](configuration.md) | 环境变量、Provider 配置、校验规则和热加载语义 |
| [HTTP API](api.md) | 认证、错误码、全部端点、curl 示例和数据结构索引 |
| [前后端对接](backend-api.md) | 前端视角的 API、SSE、权限状态和 TypeScript 数据契约 |
| [数据存储](data-storage.md) | SQLite 表结构、事务、历史/用量聚合、迁移和备份 |
| [前端说明](frontend.md) | React + shadcn/ui 结构、路由、主题组件和构建产物 |
| [部署指南](deployment.md) | Docker、Compose、二进制、systemd、反向代理和 CI |
| [安全说明](security.md) | 认证授权、限流、SSRF、防泄漏和部署加固 |
| [运维指南](operations.md) | 健康检查、Prometheus、日志、告警、备份和故障排查 |
| [开发指南](development.md) | 本地环境、常用命令、测试约定和发布流程 |

## 一分钟上手

```bash
go run ./cmd/cg
```

终端会打印自动生成的 `ADMIN_TOKEN`。随后访问：

- 仪表盘：<http://127.0.0.1:8080>
- 管理面板：<http://127.0.0.1:8080/admin>

首次登录需要修改管理密钥。然后在 **Provider** 页面添加 OpenAI 兼容服务，并在 **运行概览** 中触发检测。公开监听或容器部署必须预先显式设置至少 16 字符的 `ADMIN_TOKEN`。

## 仓库与版本

| 渠道 | 地址 | 说明 |
|------|------|------|
| GitHub | <https://github.com/wututua/AI_Model_Connectivity> | 上游源码、Issue、Pull Request、GitHub Actions 和首发 Release |
| CNB | <https://cnb.cool/ligzs/AI_Model_Connectivity> | 国内同步镜像、CNB 云原生构建和独立 Release |

CNB 每天北京时间 01:00、09:00、17:00 从 GitHub 同步源码与标签。标签同步后，CNB 会自动测试、构建前端、并行编译 6 个平台目标并创建 Release。详见 [仓库与发布渠道](repositories.md)。

## 文档约定

- 命令默认从仓库根目录执行；需要切换目录时会在代码块中明确写出。
- 配置名以 `.env.example` 和 `internal/config` 为准；SQLite 中已保存的运行时配置会覆盖同名运行时初始值。
- 状态统一使用 `ok`（正常）、`slow`（较慢）、`error`（异常）和 `paused`（暂停）。
- 报告与任务展示时间使用服务本地时区，历史记录使用 RFC3339，用量按 UTC 自然日聚合。
- 涉及 API Key、管理密钥、Webhook 和日志的示例均为占位值，提交 Issue 前必须脱敏。

发现文档与代码不一致时，请以当前分支源码和自动化测试为准，并在 [GitHub Issues](https://github.com/wututua/AI_Model_Connectivity/issues) 提交可复现的问题。
