# 安全与访问控制

[项目首页](../README.md) · [文档索引](README.md) · [安全报告](../SECURITY.md) · [部署指南](deployment.md)

本文说明当前实现的安全边界，不是安全认证或无漏洞保证。报告漏洞请遵循根目录 [SECURITY.md](../SECURITY.md)，不要公开凭据或可利用细节。

## 威胁模型

服务会**真实调用**上游模型接口并持有 API Key，同时对外暴露管理 API。主要风险：管理接口被爆破、Provider/通知目标被用于 SSRF、数据库文件泄露凭据。

管理员可以配置出站目标，应视为可信操作人员。私网 Provider 是支持的使用场景，不能把应用本身当作管理员之间的网络隔离边界。

## 认证

### 用户角色

| 用户组 | 权限 | 存储 |
|------|------|------|
| 管理员 `admin` | 配置、检测控制、用户管理、全部只读数据 | SQLite `users` |
| 普通用户 `user` | 只读：监控、`detection`、`providers`、`tasks`、`billing`、`/metrics`；可修改自己的密码 | SQLite `users` |

密码至少 8 位，包含 ASCII 大写字母、小写字母、数字，不强制特殊符号；输入最多 1024 字节。用户名为 3–32 位 ASCII 字母、数字、点、下划线或短横线，以字母或数字开头，不区分大小写。

### 首次启动

1. 仅在 `users` 为空时初始化管理员：用户名来自 `ADMIN_USERNAME`（默认 `admin`）；密码优先使用 `ADMIN_PASSWORD`。
2. 未指定密码时，尝试迁移旧 `ADMIN_TOKEN` 或 `admin_stored_token`；若不符合新规则则用 `crypto/rand` 生成密码并打印到终端。创建管理员和删除旧 Token KV 在一个事务中完成。
3. 初始账号、新建账号和管理员重置密码后均标记 `must_change_password=true`；后端仅允许会话查询、退出和修改密码，不能绕过前端继续访问后台。
4. 已有账号时不再读取环境密码作为登录凭据，重启不会覆盖用户密码。旧 Bearer、管理 Token 和只读分享 Token 接口已移除。

### 密码与会话

- 使用 Go 标准库 PBKDF2-SHA256，600,000 次迭代、随机 16 字节盐，保存 32 字节派生密钥；不存储明文密码。
- 登录凭据为随机 32 字节会话值，数据库仅保存 SHA-256 摘要；Cookie 设置 HttpOnly、SameSite=Strict、Path=/，24 小时绝对有效期，每账号最多 10 个会话。
- HTTPS 直连自动使用 Secure Cookie；TLS 反向代理必须配置 `SECURE_COOKIES=true` 并保留原始 Host。不信任客户端提供的转发协议头。
- 写请求验证 Origin、Sec-Fetch-Site 和与会话绑定的 `X-CSRF-Token`。登录必须为 JSON 请求，防止跨站表单登录。
- 退出、禁用、删除、编辑用户和重置密码会撤销相关会话；自己改密后仅保留新会话。普通用户不可变更任何角色或共享配置。
- 最后一个启用管理员不能被删除、禁用或降级；用户管理不允许操作当前管理员自身，密码通过账户安全页修改。

### 限流

- 哈希与 CSRF 凭据采用常量时间比较；不存在的用户也执行同等密码派生计算。
- 登录与改密按连接 IP 使用 1 分钟窗口，累计 10 次尝试后返回 `429` 和 `Retry-After`；成功认证清除计数，全局最多 4 个并行密码认证。
- 普通用户访问管理员接口返回 `403`，不计入密码失败次数。
- 不信任 `X-Forwarded-For`；反向代理后所有用户共享代理 IP 的限额。

## 授权边界

管理员可在「运维工具」创建服务级指标凭据。该凭据为 32 字节安全随机值，仅在创建/轮换时返回一次，SQLite 仅保存 SHA-256 摘要。只允许通过请求头 Bearer 读取 `GET /metrics`，不能访问其他 API。轮换、撤销即时生效，不随用户改密或注销自动失效；旧管理 Bearer 仍无效。使用 HTTPS 并保护采集端凭据文件。

| 接口 | 管理员 | 普通用户 |
|------|----------|----------|
| `GET /health` | 公开 | 公开 |
| `GET /api/status`、`/api/events` | 允许；登录要求由开关控制 | 同左 |
| `GET /api/admin/detection`、`providers`、`tasks`、`tasks/{id}`、`billing`、`/metrics` | 允许 | 允许 |
| 其他 `/api/admin/*`（config、settings、providers 写、users、config/import、检测触发） | 允许 | 拒绝，返回 403 |

`/api/admin/config` 与 `/api/admin/config/export` 即使管理员访问也**不返回** Provider API Key 与通知凭据，只返回 `api_key_set` / `*_set` 布尔。状态页开关同时保护 REST 与 SSE；SSE 在每次发送报告前及每 5 秒保活时重新检查会话和策略，失效发送 `auth-required` 并断开。已发送或下载的数据无法追溯撤回。

## 网络访问

[internal/httpclient](../internal/httpclient/client.go) 提供统一出站客户端：

- 每次请求前解析目标主机（DNS），命中链路本地、未指定、组播地址直接拒绝；
- 拨号时逐个尝试解析结果并固定 IP，避免 DNS rebinding；
- `CheckRedirect` 返回 `http.ErrUseLastResponse`，**不跟随重定向**；
- 配置层 `validateProviderURL` 额外限制：仅 `http`/`https`、必须有 host、禁止 userinfo/query（Webhook 允许 query）/fragment。

> 使用外部 HTTP 代理时，地址策略由代理执行，应在可信代理上配置同等策略。

私网和回环地址没有被全面禁止，便于连接本地模型或内部网关。因此这些检查不能替代出站防火墙、允许列表或可信代理；不要向不可信用户授予管理员权限。

## 凭据与日志脱敏

- 上游请求错误中的 API Key 会被替换为 `[redacted]`（`redactError`）。
- 错误信息截断到 300 字符；`sanitizeErrorText` 进一步裁剪 URL 与 DNS 细节，避免在仪表盘暴露完整内网拓扑。
- `SHOW_ERROR_DETAIL=false` 时，公开 `/api/status`、SSE 初始快照和后续推送均按当前设置清空模型与 Provider 错误文本，旧报告和单 Provider 合并结果也不例外。此开关控制展示，不删除数据库历史、日志或既有备份中的错误记录。
- 令牌与用量统计不含密钥；导出配置不含 API Key 与通知凭据。
- 诊断导出使用明确字段白名单，不含地址、模型提示词、用户、凭据或原始错误。历史 CSV 不含响应正文，危险公式前缀被转义；下载后应按监控数据保护。
- Provider 复制不带密钥且副本默认停用；预算在实际请求前持久化预留，不依赖可能缺失的上游用量数据。

## 文件与数据

- 数据目录 `0700`；数据库与 `-wal`/`-shm`/`-journal` 在 Unix 为 `0600`，Windows 为仅当前账户 + SYSTEM 的 ACL。
- 这是文件访问控制，不是加密：服务账户仍可读取其中凭据，备份需同等保护。
- 建议定期更新用户密码与 Provider API Key；备份可能含旧版本的明文 Token，升级不会改写已有备份。

## 部署加固

- 业务 REST 接口使用 `Content-Type: application/json`，受认证接口设置 `Cache-Control: no-store`，避免代理缓存敏感响应。
- 请求体上限 1 MiB，禁止多 JSON 值，降低解析类攻击面。
- HTTP 服务器设置 `ReadHeaderTimeout=5s`、`ReadTimeout=15s`、`IdleTimeout=60s`。
- 容器以非 root（65532）运行，镜像为 distroless（无 shell / 包管理器）。
- 公网使用 HTTPS，在代理后设置 `SECURE_COOKIES=true`；检查 SQLite 中实际生效的状态页登录策略，而非只检查环境变量。
- 保护启动日志与备份，不将数据目录置于静态资源目录内。步骤见[公网访问](deployment.md#公网访问)和[备份与恢复](operations.md#备份与恢复)。

## 已知限制

- 单实例单写者：SQLite 连接池固定 1，水平扩展需独立数据目录。
- 认证限流按连接 IP，代理后粒度较粗。
- Provider API Key 以明文存于 SQLite，依赖文件权限保护。
- Token 统计基于上游上报的 `usage` 字段；未上报的服务会记为 0，不能作为账单。
- 已经公开或下载的状态数据不能通过后续开启登录要求追溯撤回。
