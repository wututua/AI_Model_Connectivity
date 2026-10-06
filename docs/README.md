# 项目文档

> [项目主页](../README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [GitHub Releases](https://github.com/wututua/AI_Model_Connectivity/releases)

AI Model Connectivity（简称 CG，Go module 名为 `cg`）是一个用于检测 OpenAI 兼容接口连通性的独立 Web 服务。Go 后端负责探测、聚合、告警和 API，React 前端提供可选登录保护的仪表盘与账号管理面板，运行数据保存在本地 SQLite。

## 选择阅读路径

| 读者 | 建议顺序 |
|------|----------|
| 初次使用 | [根 README](../README.md) → [配置参考](configuration.md) → [部署指南](deployment.md) |
| 运维人员 | [部署指南](deployment.md) → [安全说明](security.md) → [运维指南](operations.md) → [数据存储](data-storage.md) |
| 后端/API 开发 | [系统架构](architecture.md) → [HTTP API](api.md) → [数据存储](data-storage.md) → [开发指南](development.md) |
| 前端开发 | [前端说明](frontend.md) → [前后端对接](backend-api.md) → [HTTP API](api.md) → [开发指南](development.md) |
| 发布维护 | [部署指南](deployment.md#8-发布流水线) → [开发指南](development.md#7-发布) |

## 文档目录

| 文档 | 内容 |
|------|------|
| [系统架构](architecture.md) | 模块边界、目录结构、启动流程、检测时序和状态模型 |
| [配置参考](configuration.md) | 环境变量、Provider 配置、校验规则和配置生效方式 |
| [HTTP API](api.md) | 认证、错误码、全部端点、curl 示例和数据结构索引 |
| [前后端对接](backend-api.md) | 前端视角的 API、SSE、权限状态和 TypeScript 数据契约 |
| [数据存储](data-storage.md) | SQLite 表结构、事务、历史/用量聚合、迁移和备份 |
| [前端说明](frontend.md) | React + shadcn/ui 结构、路由、主题组件和构建产物 |
| [部署指南](deployment.md) | Docker、Compose、二进制、systemd、反向代理和 CI |
| [安全说明](security.md) | 认证授权、限流、SSRF、防泄漏和部署加固 |
| [运维指南](operations.md) | 健康检查、Prometheus、日志、告警、备份和故障排查 |
| [开发指南](development.md) | 本地环境、常用命令、测试约定和发布流程 |
| [v1.0.0-beta.2](releases/v1.0.0-beta.2.md) | 第二个预发布版本的修复、验证范围和升级注意事项 |

## 一分钟上手

```bash
go run ./cmd/cg
```

首次启动会创建管理员（默认账号 `admin`）；未设置 `ADMIN_PASSWORD` 且没有可迁移的旧密码时，终端会打印随机初始密码。随后访问：

- 仪表盘：<http://127.0.0.1:8080>
- 管理面板：<http://127.0.0.1:8080/admin>

首次登录需要修改初始密码，至少 8 位且包含大写字母、小写字母和数字。然后在 **Provider** 页面添加 OpenAI 兼容服务，并在 **运行概览** 中触发检测。管理员可在 **用户管理** 添加管理员/普通用户，在 **系统设置 → 访问控制** 开启监控页登录保护。普通用户只能查看共享数据和修改自己的密码。

## 文档约定

- 命令默认从仓库根目录执行；需要切换目录时会在代码块中明确写出。
- 配置名以 [配置参考](configuration.md) 和 `internal/config` 为准；SQLite 中已保存的运行时配置会覆盖同名运行时初始值。
- 状态统一使用 `ok`（正常）、`slow`（较慢）、`error`（异常）、`unknown`（未检测）和 `paused`（暂停）。
- 报告、模型和历史记录时间使用带时区的 RFC3339，前端按浏览器本地时区展示，用量按 UTC 自然日聚合。
- 涉及 API Key、账号密码、会话 Cookie、Webhook 和日志的示例均为占位值，提交 Issue 前必须脱敏。

发现文档与代码不一致时，请以当前分支源码和自动化测试为准，并在 [GitHub Issues](https://github.com/wututua/AI_Model_Connectivity/issues) 提交可复现的问题。
