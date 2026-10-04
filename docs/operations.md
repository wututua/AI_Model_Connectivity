# 运维与可观测

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

## 1. 健康检查

| 方式 | 说明 |
|------|------|
| `GET /health` | 返回 `{"ok": true}`，无需认证 |
| `model-connectivity healthcheck` | 请求 `http://127.0.0.1:<APP_PORT>/health`，非 2xx 退出码 1（容器健康检查用） |
| `GET /api/status` | 有报告说明已完成过检测；`404` 表示尚未检测；启用登录开关后需要账号会话 |

## 2. Prometheus 指标

`GET /metrics` 需携带已完成初始改密的普通用户或管理员会话 Cookie，未启用时返回 `404`。注册在自己的 `prometheus.Registry`（含 Go/进程收集器）。

| 指标 | 类型 | 标签 | 说明 |
|------|------|------|------|
| `cg_probe_latency_ms` | Histogram | `provider`,`model`,`status` | 单次探测延迟，桶 50/100/250/500/1000/2000/5000/10000/30000 ms |
| `cg_probe_total` | Counter | `provider`,`model`,`status` | 探测次数 |
| `cg_probe_tokens_total` | Counter | `provider`,`model`,`kind` | token 消耗，`kind` 为 `prompt`/`completion`；上游未上报时不计 |
| `cg_check_runs_total` | Counter | `kind`,`status` | 检测任务次数，`status` 为 `success`/`error`/`canceled` |
| `cg_check_duration_seconds` | Histogram | `kind`,`status` | 一次检测耗时，桶 1/5/10/30/60/120/300/600 s |

`kind` 取值：`manual`（手动）、`scheduled`（定时）、`startup`（启动）、`provider`（单 Provider 重跑）。

手动查询（登录方式见 [API 参考](api.md#1-认证)）：

```bash
curl -b cookies.txt http://127.0.0.1:8080/metrics
```

**兼容性变化**：旧版 `authorization: Bearer` 配置已失效。当前没有永久指标密钥，自动抓取需要由受信任的本地采集程序登录普通用户账号、保存 Cookie 并在 24 小时会话过期时重新登录；不要直接填写一个不会更新的 Cookie，也不要公开暴露无认证的代理端点。

建议告警规则：

```yaml
- alert: ModelConnectivityProbeFailing
  expr: increase(cg_probe_total{status="error"}[30m]) > 5
```

`cg_check_runs_total` 是计数器，不包含最后报告时间，不能直接用于 stale-report 告警。报告过期检测应由外部探针读取 `/api/status` 的 `generated_at`，或根据部署计划监控长时间没有新增 `cg_check_runs_total` 的情况。

## 3. 日志

`slog` JSON 输出到 stdout，关键事件：

| 日志 | 含义 |
|------|------|
| `server started` | 监听地址与 `web_dir` |
| `Administrator account: …` / `Initial administrator password: …` | 初始管理员和随机生成密码；请保护启动日志 |
| `next scheduled check` | 下一次定时检测时间与间隔 |
| `check finished` | ok/slow/error/total 计数 |
| `send notify failed` | 告警发送失败（不影响检测结果） |
| `scheduled check skipped` | 上一轮未完成，被 `ErrCheckAlreadyRunning` 跳过 |

上游错误中的 API Key 会脱敏；首次启动生成密码会显示于终端，日志应限制读取权限。

## 4. 告警配置与验证

1. 在管理面板 **设置** 填写平台与 Webhook；Telegram 需 token + chat id。
2. 冷却时间建议 30 分钟，避免抖动刷屏。
3. 验证：临时把某 Provider 的 API Key 改错后触发检测，应收到 `DEGRADED` 通知；恢复后（开启 `NOTIFY_ON_RECOVERY`）收到恢复通知。
4. 无通知的常见原因：平台设为 `disabled` / Webhook 为空 / 状态未变化 / 首次启动即正常 / 处于冷却期。

## 5. 任务历史排查

```bash
curl -b cookies.txt \
  'http://127.0.0.1:8080/api/admin/tasks?status=error&limit=20'
```

`error_message` 与 `status=canceled` 可区分失败与被手动停止；`elapsed_ms` 用于定位整体变慢。

## 6. 备份与恢复

```bash
# 在线一致性备份（需 sqlite3 CLI）
sqlite3 data/cg.sqlite ".backup /backup/cg-$(date +%F).sqlite"
# 或停服后复制整个 data 目录
systemctl stop model-connectivity && cp -a data data.bak && systemctl start model-connectivity
```

备份文件含账号密码哈希、会话记录、Provider API Key 和通知凭据，权限应设为 `0600` 或存入加密介质。账号系统升级会删除旧 Token KV，回滚必须恢复升级前备份。

## 7. 常见故障

| 现象 | 排查 |
|------|------|
| 仪表盘一直空 | 尚未检测过；在管理面板触发一次，或设置 `AUTO_CHECK_RUN_ON_START=true` |
| 模型列表为空 | `PROVIDER_N_MODELS` 留空时依赖 `/models`；检查 base_url 与鉴权，或显式指定模型 |
| 全部 `slow` | 调高 `SLOW_THRESHOLD_MS`，或检查上游/网络延迟 |
| 触发检测返回 409 | 上一轮仍在进行；等待或调用 `detection/stop` |
| 登录失败 / 401 | 检查账号密码、账号是否禁用以及会话是否过期；首次使用可查启动日志或旧 Token 迁移说明，已有账号不会被 `ADMIN_PASSWORD` 重置 |
| 写操作 403 | 普通用户无修改权限；检查初始改密、CSRF、反向代理 Host 和 `SECURE_COOKIES` 是否与 HTTPS 部署一致 |
| 429 | 一分钟内失败 10 次；等待 `Retry-After` 或重启服务清空计数 |
| 通知收不到 | 见第 4 节检查清单 |
| 数据库 locked | 单写者设计，确认未用同一文件的多实例；必要时检查是否有外部进程占用 |
| 上游返回 401/403 | API Key 无效或 base_url 不匹配；错误详情在仪表盘（需 `SHOW_ERROR_DETAIL=true`） |

## 8. 成本提示

探测会真实消耗 token。默认提示词下每模型约 40 token；建议：

- 使用 `SKIP_MODELS` / `MAX_MODELS_PER_PROVIDER` 缩小探测面；
- 定时检测间隔设为 6–12 小时；
- 在管理面板 **检测控制** 查看「Token 消耗估算」，在 **用量** 页查看实际统计。
