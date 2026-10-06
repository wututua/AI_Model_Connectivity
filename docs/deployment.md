# 部署指南

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

## 1. 前置要求

| 方式 | 要求 |
|------|------|
| 源码运行 | Go 1.26.8（与 `go.mod` 一致） |
| 源码运行（前端） | Node 24 |
| 二进制 | 无需依赖，发布包内含 `web/` |
| Docker | Docker 20.10+（Compose v2 可选） |

## 2. Docker Compose（推荐）

```bash
mkdir -p data
export ADMIN_USERNAME=admin
export ADMIN_PASSWORD='<设置至少8位且含大写、小写字母和数字的密码>'
docker compose up -d
```

- 镜像内 `APP_HOST=0.0.0.0`。未提供初始密码时会生成并打印到启动日志，首次登录要求修改。
- 数据卷 `./data:/app/data` 持久化 SQLite。
- 启动变量通过 Compose 的 `environment` 配置；未列出的变量需显式添加，Provider 等日常配置通过后台维护。
- 健康检查：容器内执行 `model-connectivity healthcheck`（30s 间隔，start_period 10s）。

Linux 绑定挂载前请准备目录权限（容器以 `65532:65532` 运行）：

```bash
mkdir -p data && sudo chown 65532:65532 data && sudo chmod 0700 data
```

也可使用命名卷：`-v model-connectivity-data:/app/data`。

## 3. docker run

先从当前源码构建本地镜像：

```bash
docker build -t model-connectivity:local .
```

```bash
docker run -d -p 8080:8080 \
  -e ADMIN_USERNAME -e ADMIN_PASSWORD \
  -e PROVIDER_1_ID=openai \
  -e PROVIDER_1_BASE_URL=https://api.openai.com/v1 \
  -e PROVIDER_1_API_KEY=sk-xxx \
  -e PROVIDER_1_MODELS=gpt-4o-mini \
  -v $(pwd)/data:/app/data \
  --name model-connectivity \
  model-connectivity:local
```

镜像三阶段构建：Node 24 构建前端 → Go 1.26.8 编译（CGO_ENABLED=0）→ `gcr.io/distroless/static-debian12:nonroot` 运行时（无 shell、非 root）。

GitHub Actions 会把多架构镜像推送到 `<DOCKERHUB_USERNAME>/model-connectivity`，其中用户名来自仓库 Secret。项目流水线未配置 GHCR 发布，因此本文不使用旧的 `ghcr.io` 地址。

## 4. 二进制部署

1. 从 [GitHub Releases](https://github.com/wututua/AI_Model_Connectivity/releases) 下载对应平台压缩包，核对 `SHA256SUMS.txt` 后解压（可执行文件统一命名 `model-connectivity`，Windows 加 `.exe`；另含 `README.md`、`LICENSE`、`docs/`、`assets/`、`web/` 及字体许可）。
2. 可选：通过进程环境变量设置 `ADMIN_USERNAME` / `ADMIN_PASSWORD`；未指定密码时使用启动日志生成的密码。无需创建配置文件，Provider 稍后在管理面板添加。
3. 启动：`./model-connectivity`（Windows：`model-connectivity.exe`）。

常用子命令：

| 命令 | 说明 |
|------|------|
| `model-connectivity` / `serve` | 常驻服务（默认） |
| `model-connectivity check` / `once` | 跑一次检测后退出，适合 cron |
| `model-connectivity healthcheck` | 请求本机 `/health`，用于容器健康检查 |
| `model-connectivity --version` | 输出版本、提交、Go 工具链和目标平台，不打开数据库 |
| `model-connectivity recover-admin <用户名>` | 停服后恢复已有管理员，生成临时密码并吊销该账号会话 |

## 5. systemd 示例

```ini
[Unit]
Description=AI Model Connectivity
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/cg
ExecStart=/opt/cg/model-connectivity
Restart=always
RestartSec=5
Environment=APP_HOST=127.0.0.1
Environment=APP_PORT=8080
Environment=DATA_DIR=/opt/cg/data
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/opt/cg/data

[Install]
WantedBy=multi-user.target
```

## 6. 反向代理

SSE 需要禁用缓冲（以 Nginx 为例）：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header X-Real-IP $remote_addr;
}

location /api/events {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
    chunked_transfer_encoding on;
}
```

注意事项：

- 应用**不信任**客户端 `X-Forwarded-For`，认证限流以连接来源 IP 为准；反向代理后的用户会共享代理 IP 的限额。若要按真实 IP 限流，请在受信任的代理层实现。
- 正式部署应强制 HTTPS，设置 `SECURE_COOKIES=true` 后重启，并为密码、Cookie 与 CSRF 值保密。纯 HTTP 本地预览保持 false。
- 代理必须保留原始 Host（含端口），否则同源校验会拒绝浏览器写请求。不要缓存 `/api/`；登录状态依赖同源 Cookie。
- 后端已返回 `X-Accel-Buffering: no`，Nginx 会据此关闭缓冲。

## 7. 升级与回滚

1. 备份 `data/`（含 SQLite 与 WAL）。
2. 将旧的文件式启动配置迁移到进程环境变量（Compose 使用 `environment`，systemd 使用 `Environment=`），确认 `DATA_DIR`、`DATABASE_PATH`、监听地址和 `SECURE_COOKIES` 保持原值，再替换二进制或镜像 tag 并重启。Provider 和运行设置继续读取原 SQLite 数据库，通过后台修改。
3. 首次启动日志出现 `server started` 且 `/health` 返回 `{"ok":true}` 即成功。
4. 旧版本升级账号系统：符合密码规则的旧管理 Token 迁移为初始管理员密码，否则生成新密码并打印到日志；首次登录必须改密。
5. 旧 Bearer 和只读分享密钥停止工作；为只读访问者创建普通用户。Prometheus 抓取也需改为维护有效登录会话。
6. 回滚：恢复旧二进制 + 升级前一致性数据库备份。账号迁移会删除旧 Token KV，不能只替换旧二进制。

`ADMIN_PASSWORD` 不会重置已存在的管理员；日常密码维护使用账户安全或其他管理员的用户管理页面。忘记密码使用[离线恢复命令](operations.md#9-管理员密码恢复)，不要删除数据库。

## 8. 发布流水线

### GitHub Actions

| 工作流 | 触发 | 内容 |
|--------|------|------|
| `ci.yml` | push / PR（排除 `v*` tag） | Go 静态检查、竞态测试、govulncheck、npm audit、前端单元测试、构建和浏览器回归 |
| `release.yml` | tag `v*` / main / 手动 | Linux 竞态、Windows 单元、安全扫描、前端测试/构建/浏览器回归通过后允许发布；tag 产出 6 平台压缩包、校验文件与镜像 |

带 `-` 的预发布 tag（如 `v2.0.0-rc.1`）标记为 GitHub prerelease，只发布对应 Docker 版本，不更新 `latest` 或 major.minor 别名。正式版本更新 `latest`；`main` 分支仅更新 `main` 镜像标签。版本与提交写入可执行文件和镜像，源码本地构建默认为 `dev`。实际版本以发布 tag 为准，前端私有包版本不作为发布号。

## 9. 容量与性能建议

- 默认 `CONCURRENCY=1` 串行探测；Provider 较多可适当提高，但注意上游限流。
- 定时检测建议 6–12 小时一次，避免不必要的 token 消耗。
- 每个模型最多保留 `MAX_HISTORY_RECORDS`（默认 500）条，`probe_results` 最长保留 90 天；有效统计窗口同时受天数和记录条数限制，不保证有完整 365 天探测历史。用量按日最多保留 365 天，数据库大小随模型数、频率与错误详情增长。
- 一个数据库只能由一个服务实例或单次检测进程运行；不要让 cron `check` 与常驻服务共用同一数据库并行执行。
