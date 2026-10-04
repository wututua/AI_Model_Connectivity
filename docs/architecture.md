# 系统架构

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

## 1. 概览

```
┌──────────────────────────────────────────────────────────────┐
│                        cmd/cg (main.go)                      │
│  配置加载 → 账号初始化 → 调度器 goroutine → HTTP 服务器       │
│  application 实现 AdminController，串联探测/存储/通知/推送      │
└───────────┬───────────────────────────────────┬──────────────┘
            │                                   │
     ┌──────▼────────┐                   ┌──────▼─────────┐
     │ internal/web  │  HTTP + SSE       │ internal/probe │  并发探测
     │  Server/Broker│◄─────────────────►│    Runner      │
     └──────┬────────┘                   └──────┬─────────┘
            │                                   │
     ┌──────▼────────┐                   ┌──────▼─────────┐
     │internal/report│  报告聚合/统计    │internal/provider│ OpenAI 兼容客户端
     └──────┬────────┘                   └──────┬─────────┘
            │                                   │
     ┌──────▼───────────────────────────────────▼─────────┐
     │ internal/storage (SQLite) · internal/notify        │
     │ internal/metrics (Prometheus)                       │
     └────────────────────────────────────────────────────┘
```

运行在单个进程内，无外部依赖（数据库为本地 SQLite 文件）。

## 2. 目录结构

```
cmd/cg/              程序入口
  main.go            application：配置、调度、HTTP 生命周期
  users.go           初始管理员与旧版 Token 迁移
internal/auth/       用户名/密码规则、密码哈希与校验
internal/config/     .env 解析、运行时配置模型与校验
internal/provider/   Provider 抽象与 OpenAI 兼容实现、图标映射
internal/probe/      探测编排（收集目标 → 并发探测 → 结果）
internal/report/     报告构建、历史裁剪、延迟统计、SVG 曲线
internal/notify/     告警客户端与过滤、告警状态
internal/storage/    SQLite 持久化、账号/会话、用量聚合、文件权限
internal/web/        HTTP 服务、SSE Broker、Cookie/CSRF/角色授权、静态托管
internal/metrics/    Prometheus 注册表与指标
internal/httpclient/ 安全 HTTP 客户端（DNS 校验 / 禁止重定向）
frontend/            React + TypeScript + Vite 源码
web/                 前端构建产物（Go 直接托管，已提交到仓库）
docs/                本目录
```

## 3. 核心概念

| 概念 | 说明 |
|------|------|
| Provider | 一个 OpenAI 兼容服务（base_url + api_key + 模型列表） |
| Model | Provider 下的一个模型，探测的最小单位 |
| Target | `Provider × Model` 组合，探测任务单元 |
| Result | 单次探测结果（状态、延迟、预览、错误、token 用量） |
| Report | 一次检测的完整快照，按 Provider 分组并附统计 |
| Task | 一次检测运行记录（manual / scheduled / startup / provider） |

## 4. 启动流程（`cmd/cg/main.go`）

1. `config.Load(".env")` — 读 `.env` 再叠加真实环境变量（环境变量优先）。
2. `healthcheck` 子命令：请求 `127.0.0.1:<port>/health`，成功退出 0（供容器健康检查使用）。
3. 打开 SQLite，将上次异常退出遗留的 `running` 任务标记为 `canceled`。
4. 若已有运行时配置则用其覆盖 `.env` 默认值，否则把当前配置写入。
5. 校验持久化的运行时参数与 Provider，任一非法即退出（避免脏配置带病启动）。
6. `initializeUsers`：用户表非空则保留现有账号；否则按 `ADMIN_PASSWORD`、符合规则的旧管理 Token、随机密码的优先级创建初始管理员，并标记首次改密。创建账号和删除旧 Token KV 在同一事务内完成；后续认证只接受账号会话。
7. 按子命令分叉：`check` / `once` 跑一次就退出；`serve`（默认）常驻。
8. 常驻模式：`AUTO_CHECK_RUN_ON_START=true` 时异步跑一次启动检测；启动调度器 goroutine；启动 HTTP 服务。
9. 收到 SIGINT/SIGTERM：拒绝新检测，取消并等待正在运行的任务，在 5 秒总宽限期内关闭 HTTP、等待启动任务和调度器退出，最后关闭数据库。

## 5. 一次检测的时序

```
触发源：HTTP(manual) / 调度器(scheduled) / 启动(startup) / 单 Provider(provider)
        │
        ▼
application.checkWithOptions
   ├─ 抢占 running 标志（已有任务 → ErrCheckAlreadyRunning → HTTP 409）
   ├─ 创建 check_tasks 记录（status=running）
   ├─ probe.Runner.Run
   │     ├─ collectTargets：逐 Provider 拉 /models（超时 MODEL_LIST_TIMEOUT_SECONDS）
   │     │     └─ 失败/空列表 → ProviderError（仍生成 DEGRADED 报告）
   │     ├─ 去重、应用 SKIP_MODELS、MAX_MODELS_PER_PROVIDER 截断
   │     └─ probeTargets：先获取单 Provider 信号量，再获取全局信号量（CONCURRENCY）
   │           └─ probeOne：POST /chat/completions，超时 TIMEOUT_SECONDS
   │                 ├─ 延迟 ≥ SLOW_THRESHOLD_MS → slow
   │                 └─ 剥离 <think>/<thinking> 后校验非空；拒绝缺失消息与长度截断，成功时取前 80 字符为预览
   ├─ 模型发现失败：为上次报告中仍符合配置的已知模型补 unknown 记录
   ├─ 载入历史（EnableHistory 时）→ report.Build 生成报告
   ├─ 单 Provider 重跑：report.MergeProvider 合并进上一次完整报告
   ├─ 写库（历史 + 用量 + 最新报告，同一事务）
   ├─ Broker.Publish（SSE 推送）
   ├─ notify.SendIfNeeded（状态变化才发）
   └─ 完成 check_tasks（success / error / canceled）
```

取消（`POST /api/admin/detection/stop`）：任务标记 `canceled`，**不**更新最新报告、**不**写历史、**不**发告警。

## 6. 状态模型

| 状态 | 触发条件 | 展示 |
|------|----------|------|
| `ok` | 请求成功且延迟 < `SLOW_THRESHOLD_MS` | 正常（绿） |
| `slow` | 请求成功且延迟 ≥ `SLOW_THRESHOLD_MS` | 较慢（黄） |
| `error` | 请求失败/超时/解析失败 | 异常（红） |
| `unknown` | 模型发现失败，无法检测上次已知的模型 | 未检测（历史灯为灰） |
| `paused` | `ENABLED=true` 且 `PROBE_ENABLED=false` | 已暂停（黄） |

- Provider 状态：有 error 或 Provider 级错误（模型列表失败）→ `error`；否则有 unknown → `unknown`；否则有 slow → `slow`；否则 `ok`；暂停探测 → `paused`。
- 全局状态：存在 error、unknown 或 Provider 级错误 → `DEGRADED`，否则 `OPERATIONAL`。
- `unknown` 说明本轮未能验证响应，不代表聊天接口已证实故障；计入检测成功率分母，但不计入实际调用次数或用量。首次发现失败且没有已知模型时，仅记录 Provider 错误。
- 报告 `generated_at` 与模型/Provider `checked_at` 使用 UTC RFC3339。单 Provider 重测保留其他 Provider 原时间，任务计数只统计实际重测目标。

## 7. 实时推送

`web.Broker` 维护订阅者 channel 集合（`internal/web/server.go`）：

- `GET /api/events` 建立 SSE，先补发一次当前最新报告；
- 每次检测完成 `Publish`，各订阅者非阻塞投递（缓冲满则用新报告替换旧报告，避免慢客户端拖慢检测）；
- 公开读取和 SSE 推送都按当前错误详情开关过滤；修改设置后会重新推送已有报告，无需等待检测；
- 每 5 秒发 `: keep-alive` 注释帧；每次发送报告和心跳前按当前 `status_login_required` 检查会话，失效则推送 `auth-required` 后断开；
- 前端 `EventSource` 不可用或出错时，指数退避轮询 `/api/status`（30s → 60s → 120s）。

## 8. 配置优先级

```
管理面板 / PUT /api/admin/settings  ──►  SQLite runtime_config（持久）
导入配置 / POST /api/admin/config/import ─┘        │
                                                  ▼
.env 文件 ──► 环境变量（覆盖 .env）──► 基础配置 ──► 生效配置
```

监听地址、静态/数据路径和 `SECURE_COOKIES` 变更需重启；探测提示词可从 `.env` 热加载，`AUTO_CHECK_RUN_ON_START` 只在启动时判断，其余可管理参数（含监控登录开关）由 SQLite 持久化并即时生效。环境账号密码仅在初次创建管理员时使用。详见 [configuration.md](configuration.md)。
