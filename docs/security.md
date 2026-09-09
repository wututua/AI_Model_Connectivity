# 安全说明

## 1. 威胁模型

服务会**真实调用**上游模型接口并持有 API Key，同时对外暴露管理 API。主要风险：管理接口被爆破、Provider/通知目标被用于 SSRF、数据库文件泄露凭据。

## 2. 认证

### 2.1 两种密钥

| 密钥 | 权限 | 存储 |
|------|------|------|
| 管理密钥 `ADMIN_TOKEN` | 全部 `/api/admin/*` | 环境变量 / `.env`，或自动生成后存 SQLite |
| 只读分享密钥 | 只读：`detection`、`providers`、`tasks`、`tasks/{id}`、`billing`、`/metrics` | SQLite `admin_view_token` |

格式要求：16–256 位，可打印 ASCII（33–126），不含空格；两者不能相同。

### 2.2 首次启动

1. 未设置 `ADMIN_TOKEN`：读 `admin_stored_token`；为空则用 `crypto/rand` 生成 18 字节 → base64url（24 字符），写库并打印到终端，标记 `first_use=true`。
2. 首次进入管理面板强制改密（≥16 位，两次确认），改完自动进入。
3. 外部已显式配置 `ADMIN_TOKEN` 时，**拒绝**通过网页临时覆盖（`ChangeAdminToken` 返回错误），避免重启后旧密钥重新生效。

### 2.3 公开监听

`IsPublicBindHost`：除 `localhost`、`127.0.0.0/8`、`::1` 外都视为公开（含局域网地址与自定义主机名）。

- 公开监听 + 未设置 `ADMIN_TOKEN` → 进程拒绝启动；
- 若运行期 `ADMIN_TOKEN` 为空 → 管理接口返回 `403`；
- 非回环地址必须显式配置管理密钥。

### 2.4 比较与限流

- 使用 `subtle.ConstantTimeCompare` 比较 `Bearer` 头，避免时序侧信道。
- 每来源 IP 滑动窗口：1 分钟内 10 次认证失败 → `429` + `Retry-After`；成功访问重置计数。
- 只读密钥访问写接口返回 `403` 且**不计入**失败计数。
- 不信任 `X-Forwarded-For`；反向代理后所有用户共享代理 IP 的限额。

## 3. 授权边界

| 接口 | 管理密钥 | 只读密钥 |
|------|----------|----------|
| `GET /api/status`、`/api/events`、`/health` | 公开 | 公开 |
| `GET /api/admin/detection`、`providers`、`tasks`、`tasks/{id}`、`billing`、`/metrics` | ✅ | ✅ |
| 其他 `/api/admin/*`（config、settings、providers 写、token、config/import、reload、检测触发） | ✅ | ❌ 403 |

`/api/admin/config` 与 `/api/admin/config/export` 即使使用管理密钥也**不返回** Provider API Key 与通知凭据，只返回 `api_key_set` / `*_set` 布尔。

## 4. SSRF 防护

`internal/httpclient` 提供统一安全客户端：

- 每次请求前解析目标主机（DNS），命中链路本地、未指定、组播地址直接拒绝；
- 拨号时逐个尝试解析结果并固定 IP，避免 DNS rebinding；
- `CheckRedirect` 返回 `http.ErrUseLastResponse`，**不跟随重定向**；
- 配置层 `validateProviderURL` 额外限制：仅 `http`/`https`、必须有 host、禁止 userinfo/query（Webhook 允许 query）/fragment。

> 使用外部 HTTP 代理时，地址策略由代理执行，应在可信代理上配置同等策略。

## 5. 凭据与日志脱敏

- 上游请求错误中的 API Key 会被替换为 `[redacted]`（`redactError`）。
- 错误信息截断到 300 字符；`sanitizeErrorText` 进一步裁剪 URL 与 DNS 细节，避免在仪表盘暴露完整内网拓扑。
- `SHOW_ERROR_DETAIL=false` 时报告中的错误文本整体清空。
- 令牌与用量统计不含密钥；导出配置不含 API Key 与通知凭据。

## 6. 文件与数据

- 数据目录 `0700`；数据库与 `-wal`/`-shm`/`-journal` 在 Unix 为 `0600`，Windows 为仅当前账户 + SYSTEM 的 ACL。
- 这是文件访问控制，不是加密：服务账户仍可读取其中凭据，备份需同等保护。
- 建议定期轮换 `ADMIN_TOKEN`、只读密钥与 Provider API Key；吊销只读密钥后立即失效。

## 7. 其他加固

- `Content-Type: application/json` + `Cache-Control: no-store`，避免代理缓存敏感响应。
- 请求体上限 1 MiB，禁止多 JSON 值，降低解析类攻击面。
- HTTP 服务器设置 `ReadHeaderTimeout=5s`、`ReadTimeout=15s`、`IdleTimeout=60s`。
- 容器以非 root（65532）运行，镜像为 distroless（无 shell / 包管理器）。

## 8. 已知限制

- 单实例单写者：SQLite 连接池固定 1，水平扩展需独立数据目录。
- 认证限流按连接 IP，代理后粒度较粗。
- Provider API Key 以明文存于 SQLite，依赖文件权限保护。
- Token 统计基于上游上报的 `usage` 字段；未上报的服务会记为 0，不能作为账单。
