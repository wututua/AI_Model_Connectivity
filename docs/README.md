# 文档索引

AI_Model_Connectivity（简称 CG，Go module 名 `cg`）是一个用于检测 **OpenAI 兼容接口**连通性的独立 Web 服务：Go 后端负责探测、聚合、告警与 API，React 前端负责仪表盘与管理面板，数据全部落在本机 SQLite。

本目录按主题拆分文档，可从下面任意一篇开始：

| 文档 | 内容 |
|------|------|
| [architecture.md](architecture.md) | 系统架构、目录结构、数据流、状态机（推荐先读） |
| [configuration.md](configuration.md) | 全部环境变量、Provider 配置、校验规则与热加载语义 |
| [api.md](api.md) | 完整 HTTP API 参考（含 curl 示例、错误码） |
| [backend-api.md](backend-api.md) | 前后端对接文档（前端视角的 API / SSE / 数据结构） |
| [data-storage.md](data-storage.md) | SQLite 表结构、历史/用量聚合、迁移与文件权限 |
| [frontend.md](frontend.md) | React + shadcn/ui 技术栈、页面路由、主题组件与构建产物 |
| [deployment.md](deployment.md) | Docker / Compose / 二进制 / 反向代理 / CI 发布流程 |
| [security.md](security.md) | 认证与授权、限流、SSRF 防护、凭据保护 |
| [operations.md](operations.md) | Prometheus 指标、告警配置、备份、故障排查 |
| [development.md](development.md) | 本地开发、测试、Makefile、流水线说明 |

## 一分钟上手

```bash
go run ./cmd/cg                 # 启动（默认 127.0.0.1:8080）
# 终端会打印自动生成的 ADMIN_TOKEN
```

- 仪表盘：<http://127.0.0.1:8080>
- 管理面板：<http://127.0.0.1:8080/admin>
- 首次进入管理面板强制改密（至少 16 位），随后在 **Provider** 标签页添加 Provider，在 **检测控制** 触发一次检测。

完整快速开始、Provider 示例与镜像说明见仓库根目录 [README.md](../README.md)。

## 文档约定

- 配置变量名以 `.env` / 环境变量为准，运行时可在管理面板或 `PUT /api/admin/settings` 修改并持久化到 SQLite。
- 状态使用三态：`ok`（正常）/ `slow`（较慢）/ `error`（异常），暂停探测的 Provider 显示为 `paused`。
- 时间：报告与任务时间格式为 `2006-01-02 15:04:05`（本地时区），历史记录与用量按 RFC3339 / UTC 自然日处理。
