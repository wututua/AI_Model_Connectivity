# 运维指南

[项目首页](../README.md) · [文档索引](README.md) · [部署指南](deployment.md) · [配置参考](configuration.md)

## 目录

- [健康检查](#健康检查)
- [Prometheus 指标](#prometheus-指标)
- [日志](#日志)
- [告警验证](#告警验证)
- [任务历史](#任务历史)
- [备份与恢复](#备份与恢复)
- [管理员密码恢复](#管理员密码恢复)
- [常见故障](#常见故障)
- [成本控制](#成本控制)

## 健康检查

| 入口 | 含义 |
| --- | --- |
| `GET /health` | 无需认证，返回 `{"ok":true}`，仅证明 HTTP 进程可响应 |
| `model-connectivity healthcheck` | 请求 `http://127.0.0.1:<APP_PORT>/health`，失败时非零退出 |
| `GET /api/status` | 最新报告；开启登录要求后需要有效会话 |

首次使用的 `unconfigured`（无启用 Provider）与 `pending`（等待检测）均为正常空态，返回 200。`ready` 不代表结果健康，还需检查模型状态、Provider 错误、`generated_at` 与 `checked_at`。

## Prometheus 指标

常规服务启动会注册 `GET /metrics`，需要已完成初始改密的普通用户/管理员会话，或管理员创建的独立指标凭据。仅自定义嵌入服务未调用 `SetMetrics` 时返回 404，没有对应的运行时启用开关。

| 指标 | 类型 | 标签 | 说明 |
| --- | --- | --- | --- |
| `cg_probe_latency_ms` | Histogram | `provider`,`model`,`status` | 探测延迟；桶为 50/100/250/500/1000/2000/5000/10000/30000 ms |
| `cg_probe_total` | Counter | `provider`,`model`,`status` | 探测次数 |
| `cg_probe_tokens_total` | Counter | `provider`,`model`,`kind` | 上游返回的 Token 数，`kind` 为 `prompt` / `completion` |
| `cg_check_runs_total` | Counter | `kind`,`status` | 任务数，状态为 `success` / `error` / `canceled` |
| `cg_check_duration_seconds` | Histogram | `kind`,`status` | 任务耗时；桶为 1/5/10/30/60/120/300/600 秒 |

任务 `kind` 为 `manual`、`scheduled`、`startup`、`provider`、`models` 或 `failed`。服务使用独立 Registry，并包含 Go 与进程收集器。

完成 [API 登录](api.md#认证)后可手动查询：

```bash
curl -b cookies.txt http://127.0.0.1:8080/metrics
```

自动采集建议使用「运维工具 → 指标凭据」创建的专用凭据；旧管理 Bearer 仍然无效。将新凭据保存为采集端受限文件，勿提交到仓库。Prometheus 示例：

```yaml
scrape_configs:
  - job_name: model-connectivity
    scheme: https
    static_configs:
      - targets: ["monitor.example.com"]
    authorization:
      type: Bearer
      credentials_file: /run/secrets/cg_metrics_token
```

轮换后更新采集端文件；撤销后立即失效。凭据仅授权读取指标，不能用于管理 API；无认证代理不应公开指标。会话方式仍可使用，但必须维护 24 小时有效期。

以下规则片段可加入 Prometheus 规则组的 `rules` 列表：

```yaml
- alert: ModelConnectivityProbeFailing
  expr: increase(cg_probe_total{status="error"}[30m]) > 5
```

计数器不是报告更新时间。过期告警应读取 `/api/status` 的时间戳，或结合调度计划检查长时间没有新增检测的情况。

## 日志

应用通过 `slog` 输出 JSON 日志；初始化账号和密码提示可能另外写入终端。关键事件：

| 事件 | 含义 |
| --- | --- |
| `server started` | 监听地址与静态资源路径 |
| `Administrator account: ...` / `Initial administrator password: ...` | 初始化账号与随机密码，请勿公开 |
| `next scheduled check` | 下次定时检测时间与间隔 |
| `check finished` | 正常、较慢、异常与总数 |
| `send notify failed` | 通知失败，不改变检测结果 |
| `scheduled check skipped` | 已有任务运行，本轮调度跳过 |

上游错误会做凭据脱敏，但这不代表所有日志都可公开。提交 Issue 前仍需检查密码、Cookie、CSRF、通知地址与业务信息。

## 告警验证

1. 在设置中选择通知平台，填写 Webhook；Telegram 使用 Bot Token 与 Chat ID。
2. 保存配置后，在 **通知记录** 页面发送测试通知，并在接收端确认。此操作不调用模型，也不改变正式告警状态。
3. 如需验证过滤与恢复规则，使用独立测试 Provider 和通知范围模拟状态变化，不要更改生产凭据。
4. 测试结束后还原正式范围与冷却时间。

没有通知时检查平台是否禁用、凭据是否完整、过滤范围是否匹配、状态是否变化，以及是否仍在冷却期。首次检测即正常不会发送恢复通知；未检测、空范围或全部暂停也不能作为恢复证据。

企业微信与钉钉要求 `errcode=0`，Telegram 要求 `ok=true`，Bark 要求 `code=200`；通用 Webhook 与 Discord 按 HTTP 2xx 判断。平台拒绝时不会推进已发送状态，后续检测会再次尝试，但没有独立的自动重试队列。具体范围规则见[告警通知](configuration.md#告警通知)。

**通知记录** 可按发送中、平台已接受、发送失败和结果未知筛选，查看时间、渠道类型、摘要、HTTP 状态、耗时与安全错误信息。被过滤或处于冷却期的通知不会产生发送记录。

失败或结果未知的记录可手动重试，使用当前已保存的通知渠道，仅重发历史摘要并建立新记录，不改变原记录和正式告警状态。重启中断、超时或断线可能发生在平台已接收之后，请先核对接收端，避免重复消息。历史中的“平台已接受”不是最终用户送达证明。

## 任务历史

```bash
curl -b cookies.txt \
  'http://127.0.0.1:8080/api/admin/tasks?status=error&limit=20'
```

结合 `status`、`error_message` 和 `elapsed_ms` 判断失败、取消或变慢。后台任务不因离开页面而取消；API 返回 202 仅表示已接受，需查询任务终态。

正常取消会保留已确认的用量，强杀进程或数据库不可写仍可能丢失未落库数据。任务次数与报告中保留的模型总数不同，字段语义见[任务 API](api.md#任务历史)。

## 备份与恢复

备份包含密码哈希、会话记录、Provider API Key 和通知凭据。配置导出不含这些数据，也不含历史，**不能替代数据库备份**。

### 在线备份

需要 `sqlite3` CLI，以有权访问数据库的服务账户执行。以下 Bash 示例从原工作目录备份默认数据库；自定义路径请相应替换：

```bash
umask 077
mkdir -p backups
sqlite3 data/cg.sqlite ".backup 'backups/cg-$(date +%F-%H%M%S).sqlite'"
```

`.backup` 生成一致性快照。不要在写入期间只复制 `cg.sqlite`，也不要分别复制不断变化的 WAL 文件来拼接备份。

### 停服备份

停止所有服务、单次检测与其他写入者后，复制完整数据目录。以[本文档的 systemd 部署](deployment.md#systemd)为例：

```bash
sudo systemctl stop model-connectivity
sudo install -d -m 0700 /var/backups/model-connectivity
sudo cp -a /var/lib/model-connectivity \
  "/var/backups/model-connectivity/data-$(date +%F-%H%M%S)"
sudo systemctl start model-connectivity
```

Compose 部署先 `docker compose stop`，复制绑定挂载的数据目录后再启动。命名卷需使用具备该卷访问权限的备份方式；distroless 应用容器没有 shell。

### 恢复检查

1. 停止所有写入者，保留当前数据副本供排查。
2. 将备份恢复到干净的目标目录，不混入另一份数据库遗留的 `-wal` / `-shm`。
3. 恢复服务账户的所有权和权限；Unix 数据库与备份使用 `0600`，目录使用 `0700`，Windows 使用受限 ACL。
4. 使用匹配的应用版本启动，检查登录、配置、历史与用量，必要时轮换备份中的旧凭据。

升级回滚需恢复升级前备份。备份应保存在受限或加密介质，并定期在隔离环境验证可恢复性。

## 管理员密码恢复

1. 停止所有使用该数据库的进程，并备份数据。
2. 使用原工作目录、原 `DATA_DIR` / `DATABASE_PATH` 和有访问权限的服务账户运行恢复命令。
3. 重启，用生成的临时密码登录，完成强制改密。

发布包示例，`admin` 替换为实际管理员用户名：

```bash
./model-connectivity recover-admin admin
```

Windows 使用 `.\model-connectivity.exe recover-admin admin`，源码使用 `go run ./cmd/cg recover-admin admin`。

Compose 使用同一数据卷和服务账户：

```bash
docker compose stop
docker compose run --rm model-connectivity recover-admin admin
docker compose up -d
```

命令只恢复已有管理员，重新启用账号并撤销其会话，不提升普通用户权限，不清除配置、历史或其他账号。输出包含临时密码，不要录屏、上传或写入共享日志。修改 `ADMIN_PASSWORD` 不能替代此流程。

## 常见故障

| 现象 | 排查方向 |
| --- | --- |
| 状态页一直为空 | 确认有启用的 Provider，并执行首次检测 |
| 模型列表为空 | 检查 `{base_url}/models` 与鉴权，或手动指定模型 |
| 全部显示较慢 | 检查网络、上游延迟和 `SLOW_THRESHOLD_MS` |
| 检测返回 409 | 已有任务运行，查询其状态；确需取消时使用管理员停止 API |
| 登录失败或 401 | 检查账号是否禁用、密码与会话；已有账号不会被环境密码重置 |
| 写操作返回 403 | 检查角色、初始改密、CSRF、代理 Host 与 Secure Cookie 配置 |
| 认证返回 429 | 按 `Retry-After` 等待，检查失败尝试与代理共享 IP；不要通过重启绕过限流 |
| 收不到通知 | 检查[告警验证](#告警验证)与通知过滤条件 |
| 数据库锁定或不可写 | 确认只有一个实例，检查服务账户、挂载与磁盘权限 |
| 上游返回 401/403 | 检查 API Key、接口地址和账号权限，不要将真实密钥粘贴到 Issue |
| 修改环境变量未生效 | 检查是否属于[SQLite 持久化运行设置](configuration.md#配置优先级) |

## 成本控制

探测会真实消耗 Token，短提示词不保证固定费用。概览按 Provider 的输出上限和平均调度周期计算输出预算参考，不包括输入或额外推理费用。可设置每日请求预算和自动发现确认阈值，详见[请求预算](monitoring-features.md#请求预算)。

- 显式选择模型，或使用 `SKIP_MODELS` / `MAX_MODELS_PER_PROVIDER` 限制探测范围。
- 根据监控目标设置检测周期，避免不必要的高频轮询上游。
- 用量页仅统计上游已返回的数据，最终费用以供应商账单为准。
