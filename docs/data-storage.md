# 数据存储

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) · [仓库与发布](repositories.md)

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

连接池 `SetMaxOpenConns(1)`：SQLite 单写者模型，串行化写入避免 `database is locked`；WAL 仍支持并发读。

## 2. 表结构

### `probe_results`（探测历史）

| 列 | 说明 |
|----|------|
| `id` | 自增主键 |
| `provider` / `provider_type` / `provider_name` | Provider 标识 |
| `model` | 模型名 |
| `result` | `ok` / `slow` / `error` |
| `latency_ms` | 延迟 |
| `checked_at` | RFC3339 时间 |
| `error_type` | `timeout` / `dns` / `auth` / `rate_limit` / `server` / `unknown` |
| `error_message`、`response_preview` | 错误与响应预览 |
| `history_key` | `provider::model` |
| `prompt_tokens` / `completion_tokens` / `total_tokens` | 本次用量 |

索引：`(history_key, checked_at DESC)`、`(checked_at DESC)`。
旧库缺 token 列时通过 `PRAGMA table_info` 检测并 `ALTER TABLE ADD COLUMN`。

### `latest_report`

单列主键 `id = 1`，存 `generated_at` 与完整报告 JSON。

### `notify_state`

单行存告警状态 `status` 与 `sent_at`（RFC3339）。

### `runtime_config`

`key` 主键 KV 表，存：

| key | 值 |
|-----|-----|
| `runtime` | `{"settings":…,"providers":[…]}` 完整运行时配置 |
| `admin_stored_token` | 自动生成的管理密钥 |
| `admin_token_first_use` | `true` / `false` |
| `admin_view_token` | 只读分享密钥（空串表示未设置） |
| `usage_daily_migrated` | 用量表迁移标记 |

### `check_tasks`

任务记录（字段见 [api.md](api.md#8-任务历史)），索引：`(started_at DESC)`、`(status)`、`(status, provider_id)`。

### `usage_daily`（按日用量聚合）

主键 `(day, provider, model)`，列为 `provider_name`、`provider_type`、`prompt_tokens`、`completion_tokens`、`total_tokens`、`probe_count`。

## 3. 写入事务

`RecordCheck` 在一个事务内完成：

1. 写 `usage_daily`（`ON CONFLICT` 累加，无论探测成功与否都计）；
2. `ENABLE_HISTORY=true` 时批量插入 `probe_results`；
3. 删除 90 天前记录（`sqliteRetentionDays`）；
4. 按 `history_key` 保留最近 `MAX_HISTORY_RECORDS` 条；
5. 若传入 `latest`，写入 `latest_report`；
6. 提交。

任一步失败即回滚：**历史、用量与最新报告要么一起成功，要么一起失败**（测试 `TestReportWriteFailureRollsBackHistoryAndUsage`）。

## 4. 历史读取与裁剪

- `LoadHistory(limitPerKey, statsWindowDays)`：只取 `checked_at >= now - statsWindowDays` 的记录，每 key 保留最近 `limitPerKey` 条（默认 `MAX_HISTORY_RECORDS`）。
- `report.Build` 追加本次结果后再次按 `MaxHistoryRecords` 截断。
- `pruneHistory` 按统计窗口裁剪，但若裁剪后少于 `HISTORY_SIZE`，会回退保留最近 `HISTORY_SIZE` 条，保证曲线和状态灯仍有足够数据点。

## 5. 统计口径

| 指标 | 口径 |
|------|------|
| `avg_latency_24h` | 24h 内 `ok`/`slow` 样本算术平均 |
| `p50/p95/p99_latency_24h` | 升序后 Type-7 线性插值分位数（同 numpy / Excel `PERCENTILE`） |
| `latency_samples_24h` | 24h 内有效样本数 |
| `weekly_success_text` / `availability` | `STATS_WINDOW_DAYS` 窗口内 `(ok+slow)/总数` |
| `history` | 最近 `HISTORY_SIZE` 条状态，左侧补 `empty` |
| `svg_path_line` / `svg_path_area` | 100×40 视图内归一化后的三次贝塞尔平滑曲线，峰值按 max(1000, 最大延迟) 归一 |

历史被裁剪或关闭不影响 `usage_daily`，因此关闭历史后用量统计仍完整。

## 6. 迁移

启动时 `importLegacy` 在数据目录存在旧版 JSON 且目标表为空时自动导入（旧文件保留不删）：

| 旧文件 | 目标 |
|--------|------|
| `data/probe_history.json` | `probe_results`（按 key 排序插入，补全 provider/model） |
| `data/latest_report.json` | `latest_report` |
| `data/notify_state.txt` | `notify_state`（支持纯文本与 JSON 两种格式） |

`usage_daily` 首次初始化时由 `probe_results` 聚合回填一次，并用 `usage_daily_migrated` 标记避免重复。

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

备份包含自动生成的管理密钥、只读分享密钥、Provider API Key 和通知凭据。这些敏感值以可供服务读取的形式保存，并未进行静态加密，请按第 7 节的要求保护。
