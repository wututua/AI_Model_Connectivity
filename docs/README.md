# 文档

[项目首页](../README.md) · [更新日志](../CHANGELOG.md) · [参与贡献](../CONTRIBUTING.md)

这里是 AI Model Connectivity 的使用和开发文档。首次使用请从项目首页的[快速开始](../README.md#快速开始)进入；以下文档按当前分支维护，已发布版本的行为请结合对应的 [Release 说明](releases/README.md)阅读。

## 使用与部署

| 文档 | 内容 |
| --- | --- |
| [部署指南](deployment.md) | 发布包、容器、systemd、反向代理、升级与回滚 |
| [配置参考](configuration.md) | 启动变量、运行时设置、Provider 和通知规则 |
| [运维指南](operations.md) | 健康检查、任务、日志、备份、密码恢复与排障 |
| [安全与访问控制](security.md) | 认证、权限、网络访问和数据保护边界 |

## API 与开发

| 文档 | 内容 |
| --- | --- |
| [HTTP API](api.md) | 认证、端点、错误码和请求示例 |
| [前端集成](backend-api.md) | 会话、SSE、后台任务及响应类型 |
| [开发指南](development.md) | 本地环境、测试、代码约定和发布流程 |
| [系统架构](architecture.md) | 模块职责、启动流程、检测流程和状态模型 |
| [前端开发](frontend.md) | 路由、组件、主题、构建和浏览器测试 |
| [数据存储](data-storage.md) | SQLite 表结构、事务、统计、迁移及备份 |

## 项目协作

- [贡献指南](../CONTRIBUTING.md)：提交 Issue、Pull Request 和文档改进。
- [安全报告](../SECURITY.md)：如何报告漏洞而不公开敏感信息。
- [更新日志](../CHANGELOG.md)：版本摘要与升级注意事项。
- [Release 说明](releases/README.md)：按版本归档的完整中文和英文说明。
- [许可证](../LICENSE)：项目代码的许可条款。

## 常用入口

| 问题 | 入口 |
| --- | --- |
| 环境变量修改后为什么没有生效？ | [配置优先级](configuration.md#配置优先级) |
| 如何限制模型数量和探测成本？ | [探测参数](configuration.md#探测) |
| 如何给状态页加登录保护？ | [运行时修改与重启](configuration.md#运行时修改与重启) |
| 管理员密码忘了怎么办？ | [管理员密码恢复](operations.md#管理员密码恢复) |
| 如何正确备份和回滚？ | [备份与恢复](operations.md#备份与恢复) |
| 收不到通知或检测结果异常？ | [常见故障](operations.md#常见故障) |

## 阅读约定

- 命令默认从仓库根目录运行；发布包命令从解压目录运行。
- 发布包不包含开发源码；查看源码链接时，请在 GitHub 对应版本标签下阅读文档。
- Shell 示例默认使用 Bash，PowerShell 示例会单独标注。
- 示例中的密码、密钥、URL 和模型 ID 请替换为自己的值，不要直接用于生产。
- `ok`、`slow`、`error`、`unknown`、`paused` 分别表示正常、较慢、异常、未检测和暂停。
- 时间戳使用带时区的 RFC3339，用量按 UTC 自然日聚合；界面按浏览器本地时区展示时间。
- 文档示例与源码不一致时，请附上版本及最小复现步骤提交 Issue，勿附真实凭据。
