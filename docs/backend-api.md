# 前端集成

[项目首页](../README.md) · [文档索引](README.md) · [HTTP API](api.md) · [前端开发](frontend.md)

本文说明跨请求的行为约定和主要数据类型。端点、状态码与 curl 示例统一维护在 [HTTP API](api.md)，不在这里重复列出。

## 代码入口

| 文件 | 用途 |
| --- | --- |
| [frontend/src/api.ts](../frontend/src/api.ts) | 同源请求、CSRF 与错误处理 |
| [frontend/src/types.ts](../frontend/src/types.ts) | 前端使用的 TypeScript 类型 |
| [frontend/src/hooks/useAuth.tsx](../frontend/src/hooks/useAuth.tsx) | 会话与路由授权 |
| [frontend/src/utils/liveStatus.ts](../frontend/src/utils/liveStatus.ts) | SSE、重连、轮询与请求顺序 |
| [internal/web/server.go](../internal/web/server.go) | 后端路由与响应 |
| [internal/report/report.go](../internal/report/report.go) | 报告结构与聚合 |

生产环境由后端托管 `web/`，开发环境由 Vite 代理 `/api` 和 `/health`。接口字段统一使用 snake_case；前端类型只列出界面使用的字段，完整响应以 Go 的 JSON 定义为准，客户端应容忍额外字段。

## 会话生命周期

1. 启动时查询 `GET /api/auth/session`。匿名响应的 `user` 为 `null`，仍包含状态页登录策略。
2. 登录成功后接受完整 `AuthSession`。Cookie 由服务端设置，CSRF 值仅保存在内存。
3. `must_change_password=true` 时先进入改密流程，不能继续访问受保护功能。
4. 改密会撤销旧会话并签发新会话，更新 Cookie 和 CSRF 后再继续请求。
5. 注销或会话失效后清理身份；临时网络失败不能直接当作退出登录并丢弃编辑中的表单。

`api.ts` 使用 `credentials: 'same-origin'`，写请求附带 `X-CSRF-Token`。不要将密码、会话值或 CSRF 写入 localStorage。登录、改密与会话刷新可能并发，迟到的旧响应不能覆盖刚建立的身份。

`AuthProvider` 在启动、窗口聚焦及每 30 秒刷新会话。普通用户只能读取允许的共享数据并修改自己的密码；隐藏按钮不能替代后端授权。

| 响应 | 前端处理 |
| --- | --- |
| `401` | 清理失效身份并重新验证 |
| `403` + `password_change_required` | 刷新会话并进入改密 |
| 其他 `403` | 显示权限、来源或 CSRF 错误，不盲目重试写请求 |
| `429` | 按 `Retry-After` 等待 |
| 网络错误 / `503` | 显示可重试状态，保留尚未提交的表单 |

错误体以 `error` 字段为准，`ok` 不保证总是存在。非 JSON 错误回退显示 `HTTP <status>`。

## 状态订阅

`/api/status` 与 `/api/events` 都提供 `Report`，受相同的 `status_login_required` 策略控制。

- `unconfigured`：没有启用的 Provider。
- `pending`：等待检测，`generated_at=""`。
- `ready`：有报告，但仍需检查每个模型的状态与时间。

建立 SSE 后先收到当前快照，之后检测完成或配置变化时推送新快照，每 5 秒有保活注释。`auth-required` 事件表示访问权限需要重新确认，前端应关闭连接并刷新身份。

EventSource 不可用或连接失败时，退避轮询间隔为 30、60、120 秒，并继续尝试恢复 SSE。收到有效推送后停止轮询，离开页面时清理连接和定时器。

### 请求顺序

首屏请求、手动刷新、轮询与 SSE 必须共享顺序控制。新请求或推送使更早的响应失效，包括迟到的错误；不能只按 `generated_at` 排序，因为配置变化可能不改变检测时间。

连接参数变化后，当前结果变为 `unknown`，清空当前检测时间、延迟与错误；历史和用量保留。`connection_revision` 是随机配置版本，不是密钥哈希或可排序时间戳。

## 后台任务

检测启动与 Provider 重测返回 `202` 和 `AcceptedCheck`，只是接受任务，不代表完成。

1. 保存返回的 `task.id`，进入运行态。
2. 查询 detection 与任务详情，任务状态为 `running` / `success` / `error` / `canceled`。
3. 达到终态后刷新报告；刷新失败时保留任务 ID 并继续重试。
4. 只有对应任务的报告刷新成功后才结束跟踪，旧任务响应不能清除新任务状态。

概览首次加载时也需跟踪已在运行的任务，避免它在下次轮询前结束而被遗漏。运行状态、摘要和提示消息分别维护请求版本，较早的整体刷新不能覆盖刚完成任务的摘要。

任务最长 30 分钟，不随浏览器切页或断网取消。`409` 表示已有任务运行。任务 `total` 只统计本任务实际探测，不包括合并报告中保留的其他 Provider 和发现失败补出的 `unknown` 样本。

模型级启动使用 `CheckSelection`（明确 `targets` 或 `failed_only`），任务类型为 `models` / `failed`。未重测模型保留原时间和历史，不刷新 Provider 的完整检测时间；仅完整全量检查参与正式告警。`progress` 与正式报告独立，阶段包括模型发现、检测、保存及通知。预算耗尽返回 `429`，处理中耗尽则任务以错误结束，已确认用量仍保存。

## 配置编辑

### 敏感字段

| 字段 | 读取 | 留空提交 | 显式清除 |
| --- | --- | --- | --- |
| Provider API Key | `api_key_set` | 地址不变时保留旧值 | `clear_api_key=true` |
| 通知地址 | `notify_webhook_url_set` | 保留旧值 | `clear_notify_webhook_url=true` |
| Telegram Token / Chat ID | 对应 `*_set` | 保留旧值 | 对应 `clear_*=true` |

已有 Provider 更换 Base URL 时必须重新填写 Key 或显式清除，不能把旧 Key 自动发送到新地址。规则同样适用于模型同步与配置导入，校验失败时整次更新不生效。

设置接口提交完整 `RuntimeSettings`，不是部分 PATCH。导入整体替换 settings 与 providers；导出不含凭据、用户和会话，不是数据库备份。

Provider 新增 `group`、`tags`、`probe`，运行设置新增预算、防抖与维护窗口字段，详见[检测与运维功能](monitoring-features.md)。旧客户端的完整 PUT 若省略新字段，会恢复其默认值。指标凭据是单独的服务级授权，仅可读取 `/metrics`，不能作为管理 API 的会话。

### 模型选择

`ModelDiscoveryRequest` 使用尚未保存的连接草稿读取 `{base_url}/models`，不保存 Provider、不创建检测任务，也不调用聊天接口。`provider_id` 可引用已保存配置以复用密钥；超时不超过 30 秒。

模型选择器需保留手动模型和当前选择，同步结果合并去重。保存时合并尚未按 Enter 或点击添加的输入，失败后保留草稿；清空时同时清除待添加输入。空 `models` 表示每次检测自动发现，可能扩大调用范围。

变更连接参数或关闭弹窗时取消同步；保存期间锁定表单。Provider ID 拼接到 URL 路径前使用 `encodeURIComponent`，用于查找的模型身份键视为不透明值，不自行按 `::` 拆分。

## 数据类型

以下表格解释语义，字段定义见 [types.ts](../frontend/src/types.ts)；统计口径见[数据存储](data-storage.md#统计口径)。

### 报告

| 类型 / 字段 | 语义 |
| --- | --- |
| `Report.state` | `unconfigured` / `pending` / `ready` |
| `generated_at` / `elapsed_ms` | 报告时间与检测耗时，不代表所有 Provider 刚被检测 |
| `global_concurrency` / `provider_concurrency` | 检测并发配置 |
| `total` / `*_count` / `provider_count` | 报告范围内的模型与 Provider 计数 |
| `providers` / `provider_errors` | `ProviderReport[]` 与模型发现错误 |
| `overall_status` / `overall_class` | 后端聚合状态，空态应优先按 `state` 展示 |
| `stale_after_seconds` | 判断结果是否过期的阈值 |
| `history_size` / `stats_window_days` | 展示条数与统计窗口 |
| `theme` / `theme_label` | 服务端报告主题；Web 界面主题由浏览器偏好独立控制 |

### Provider 与模型

| 字段 | 语义 |
| --- | --- |
| `provider_id` / `provider_type` / `provider_name` / `provider_logo` | Provider 标识与展示 |
| `ProviderReport.results` | `ModelResult[]`，不是模型名字符串数组 |
| `connection_revision` | 可选配置版本，用于防止旧连接结果恢复当前状态 |
| `checked_at` | 带时区的检测时间；单 Provider 重测不刷新其他 Provider 时间 |
| `status` / `status_label` / `status_class` | 机器状态与展示文案 |
| `model_count` / `*_count` | Provider 范围内计数 |
| `model` / `current_model` / `is_current` | 模型名、当前模型及选择标记 |
| `latency_ms` / `response_preview` / `error` | 本次结果；预览已剥离思考标签，错误受展示开关控制 |
| `history` | 状态灯序列，可能包含用于补齐的 `empty` |
| `show_curve_chart` / `svg_path_line` / `svg_path_area` / `time_labels` | 后端生成的曲线与时间轴 |
| `avg_latency_24h` / `p50_latency_24h` / `p95_latency_24h` / `p99_latency_24h` | 格式化后的延迟文案，无有效样本为 `N/A` |
| `latency_samples_24h` | 24 小时有效延迟样本数 |
| `weekly_success_text` / `availability` | 配置窗口内采样成功率，不是连续在线时长 |

`unknown` 表示没有当前有效检测证据，包括新增配置、连接变更或发现失败。它不等于已证实的聊天接口故障。暂停 Provider 使用 `paused`，详见[状态模型](architecture.md#状态模型)。

### 配置与身份

| 类型 | 内容 |
| --- | --- |
| `RuntimeSettings` | 可管理运行参数，字段与优先级见[配置参考](configuration.md) |
| `SafeProviderConfig` | `id`、`name`、`type`、`base_url`、`models`、`enabled`、`probe_enabled`、`api_key_set` |
| `ProviderUpdate` | Provider 写入字段，使用 `api_key` 和 `clear_api_key`，不回传真实密钥 |
| `AdminConfig` / `ConfigExport` | settings 与安全 Provider 列表 |
| `ConfigImport` | settings 与 `ProviderUpdate[]` |
| `AuthSession` | `user`、`csrf_token`、Unix 秒 `expires_at`、`status_login_required` |
| `User` | ID、用户名、角色、启用状态、初始改密标记和创建时间，不含密码哈希 |
| `UserInput` | 用户名、密码、角色与启用状态；编辑时密码留空表示不变 |

### 任务与用量

| 类型 | 内容 |
| --- | --- |
| `RunningState` | 是否运行、任务 ID、类型、Provider、调度区间与 `read_only` |
| `AcceptedCheck` | `ok` 与已接受的 `CheckTask` |
| `CheckTask` | ID、类型、状态、时间、计数、错误与报告时间，完整字段见[任务历史](api.md#任务历史) |
| `BillingSummary` | 查询范围、总 Token、总探测次数、`per_model` 与 `daily` |
| `BillingItem` | Provider、模型、输入/输出/总 Token 与探测次数 |
| `BillingDaily` | UTC 日期 `day`、输入/输出/总 Token 与探测次数 |
| `NotificationDelivery` | 通知发送尝试，含类型、重试来源、平台类型、结果、时间、HTTP 状态与安全摘要；仅管理员可读 |

用量最多保留 365 天，包含失败响应中上游明确返回的有效 usage；未上报的费用无法推算，不能作为供应商账单。

通知发送接口返回 200 仅表示尝试结果已记录，需检查 `NotificationDelivery.status`，不能套用检测任务的 202 语义。记录状态为 `sending` / `success` / `error` / `unknown`，其中 `success` 仅表示平台接受。测试和重试不改变正式告警状态，详见[通知 API](api.md#通知记录)。
