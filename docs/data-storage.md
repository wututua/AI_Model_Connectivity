# 数据存储

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

所有状态保存在单个 SQLite 文件（默认 `data/cg.sqlite`，驱动 `modernc.org/sqlite`，纯 Go 无 CGO）。

## 1. 连接与 PRAGMA

`internal/storage/sqlite.go` 打开时执行：

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;      -- 64 MB
PRAGMA temp_store = MEMORY;
PRAGMA mmap_size = 30000000;
```

连接池 `SetMaxOpenConns(1)`：本进程读写均排队使用一个连接，避免多写入者争用；WAL 支持其他连接上的并发读取，但不会让此连接池内部的查询并行。

## 2. 表结构

### `probe_results`（探测历史）

| 列 | 说明 |
|----|------|
| `id` | 自增主键 |
| `provider` / `provider_type` / `provider_name` | Provider 标识 |
| `model` | 模型名 |
| `result` | `ok` / `slow` / `error` / `unknown`（发现失败导致未检测） |
| `latency_ms` | 延迟 |
| `checked_at` | 实际探测结束时间，UTC RFC3339；旧记录的时区偏移仍可读取 |
| `error_type` | `timeout` / `dns` / `auth` / `rate_limit` / `server` / `unknown` |
| `error_message`、`response_preview` | 错误与响应预览 |
| `history_key` | `url.QueryEscape(providerID) + "::" + model`；客户端应视为不透明标识 |
| `prompt_tokens` / `completion_tokens` / `total_tokens` | 本次用量 |

索引：`(provider, model, checked_at DESC)`、`(history_key, checked_at DESC)`、`(checked_at DESC)`。
旧库缺 token 列时通过 `PRAGMA table_info` 检测并 `ALTER TABLE ADD COLUMN`。

探测去重和报告查找使用同一身份键。Provider 部分经过百分号编码，例如 `a::b` + `c` 对应 `a%3A%3Ab::c`，不会与 `a` + `b::c` 混淆。历史查询和裁剪直接使用独立的 `provider`、`model` 列；旧 SQLite 记录无需重写，也不会因旧键相同而合并。

### `latest_report`

单列主键 `id = 1`，存 `generated_at` 与完整报告 JSON。

### `notify_state`

单行存告警状态 `status` 与 `sent_at`（RFC3339）。

### `runtime_config`

`key` 主键 KV 表，存：

| key | 值 |
|-----|-----|
| `runtime` | `{"settings":…,"providers":[…]}` 完整运行时配置 |
| `usage_daily_migrated` | 用量表迁移标记 |

旧版管理/只读 Token KV 在首次迁移到账号系统时删除；新的登录凭据不再写入此表。

### `users`

`id` 自增主键，`username` 大小写不敏感且唯一；`password_hash` 保存加盐的 PBKDF2-SHA256 哈希；`role` 为 `admin` 或 `user`；另有 `enabled`、`must_change_password`、`created_at`。

### `sessions`

`token_hash` 主键，仅保存随机 Cookie 的 SHA-256 哈希；`user_id` 外键关联用户并级联删除；`csrf_token` 为会话 CSRF 校验值；`expires_at` 为 Unix 秒。按用户与过期时间建索引。

每个账号最多保留 10 个会话，登录时清理过期记录；请求读取时也检查有效期与账号状态。修改账号或密码会在同一事务中撤销旧会话；最后一个启用管理员的保护也在事务内执行。

### `check_tasks`

任务记录（字段见 [api.md](api.md#8-任务历史)），索引：`(started_at DESC)`、`(status)`、`(status, provider_id)`。

服务启动时将遗留 `running` 任务改为 `canceled`，写入结束时间和重启中断原因。单 Provider 任务不统计合并报告中其他 Provider 的结果。

### `usage_daily`（按日用量聚合）

主键 `(day, provider, model)`，列为 `provider_name`、`provider_type`、`prompt_tokens`、`completion_tokens`、`total_tokens`、`probe_count`。

## 3. 写入事务

`RecordCheck` 在一个事务内完成：

1. 写 `usage_daily`（`ON CONFLICT` 累加，实际调用无论成功与否都计，`unknown` 不计）；
2. `ENABLE_HISTORY=true` 时批量插入 `probe_results`；
3. 删除 90 天前记录（`sqliteRetentionDays`）；
4. 按独立的 `(provider, model)` 保留最近 `MAX_HISTORY_RECORDS` 条；
5. 若传入 `latest`，写入 `latest_report`；
6. 提交。

任一步失败即回滚：**历史、用量与最新报告要么一起成功，要么一起失败**（测试 `TestReportWriteFailureRollsBackHistoryAndUsage`）。

应用层在批次取消或主事务失败后，会用独立的 5 秒收尾上下文尝试单独保存已确认探测的用量，不替换报告或历史，也不重复保存已提交的批次。数据库不可写或进程被强杀时无法保证收尾成功；未开始及取消中未收到响应的探测不估算用量。

## 4. 历史读取与裁剪

- `LoadHistory(limitPerKey, statsWindowDays)`：按时间戳代表的实际时刻过滤统计窗口，通过 SQL 窗口函数限制每 `(provider, model)` 最近 `limitPerKey` 条（默认 `MAX_HISTORY_RECORDS`），再生成身份键；时间相同按自增 ID 排序。
- `report.Build` 追加本次结果后再次按 `MaxHistoryRecords` 截断。
- 有效窗口是配置天数、90 天保留策略及每模型条数上限的交集；扩大显示窗口不能恢复已裁剪样本。
- `pruneHistory` 按统计窗口裁剪，但若裁剪后少于 `HISTORY_SIZE`，会回退保留最近 `HISTORY_SIZE` 条，保证曲线和状态灯仍有足够数据点。

## 5. 统计口径

| 指标 | 口径 |
|------|------|
| `avg_latency_24h` | 24h 内 `ok`/`slow` 样本算术平均 |
| `p50/p95/p99_latency_24h` | 升序后 Type-7 线性插值分位数（同 numpy / Excel `PERCENTILE`） |
| `latency_samples_24h` | 24h 内有效样本数 |
| `weekly_success_text` / `availability` | `STATS_WINDOW_DAYS` 窗口内 `(ok+slow)/(ok+slow+error+unknown)`，表示检测成功率，不是连续在线时长比例 |
| `history` | 最近 `HISTORY_SIZE` 条状态，左侧补 `empty` |
| `svg_path_line` / `svg_path_area` | 100×40 视图内归一化后的三次贝塞尔平滑曲线，峰值按 max(1000, 最大延迟) 归一 |

历史被裁剪或关闭不影响 `usage_daily`，因此关闭历史后用量统计仍完整。

HTTP 错误、成功 HTTP 状态中的错误信封，以及回复为空、缺失消息或因 token 上限被截断时，探测记为失败，但保留上游返回的有效 usage；未返回 usage 的消耗仍无法估算。

## 6. 迁移

启动时 `importLegacy` 在数据目录存在旧版 JSON 且目标表为空时自动导入（旧文件保留不删）：

| 旧文件 | 目标 |
|--------|------|
| `data/probe_history.json` | `probe_results`（按 key 排序插入，补全 provider/model） |
| `data/latest_report.json` | `latest_report` |
| `data/notify_state.txt` | `notify_state`（支持纯文本与 JSON 两种格式） |

`usage_daily` 首次初始化时由 `probe_results` 聚合回填一次，并用 `usage_daily_migrated` 标记避免重复。

升级后建议执行一次完整检测，以重建旧快照中的历史统计。已因旧身份键冲突而漏测、裁剪的样本无法恢复；仅保存拼接键的旧 JSON 无法可靠区分本来就有歧义的 Provider/模型组合，仍按首次 `::` 分隔导入。

账号系统首次启动创建 `users`、`sessions`。用户表为空时，管理员密码优先取 `ADMIN_PASSWORD`；否则迁移符合新规则的旧管理 Token，不符合时生成新密码。创建初始管理员和删除旧 Token KV 在同一事务提交，所有初始账号要求首次改密。已有账号不因重启被重置。迁移前应备份，回滚须恢复旧数据库备份。

Provider 的随机 `connection_revision` 随运行配置及最新报告 JSON 保存，不需要新增 SQL 表。升级时，为没有版本的 Provider 分配新版本，旧快照的当前状态保守地变为“未检测”，下一次检测后恢复；历史与用量不会清除。版本随连接或凭据变更轮换，重启与仅名称修改不轮换。

> 用量只能迁移数据库中仍存在的记录，已删除或取消检测产生的实际消耗无法还原，因此该统计**不是**供应商账单。

## 7. 文件权限

- 数据目录：不存在时以 `0700` 创建。
- 数据库文件及 `-wal` / `-shm` / `-journal`：
  - Unix → `chmod 0600`；
  - Windows → 受限 ACL，仅当前服务账户与 SYSTEM 可访问。

这属于文件访问控制而非加密；服务账户仍可读取其中凭据，备份文件应使用同等级权限或加密介质。

## 8. 备份

```bash
# 推荐：在线一致性备份
sqlite3 data/cg.sqlite ".backup backup.sqlite"   # 需要 sqlite3 CLI
# 或停服后直接复制整目录
cp -a data data.bak
```

备份包含账号密码哈希、会话记录、Provider API Key 和通知凭据。密码不存明文，Provider API Key 与通知凭据没有静态加密，请按第 7 节保护备份。配置导出不包含用户、密码或会话，不能替代数据库备份。
