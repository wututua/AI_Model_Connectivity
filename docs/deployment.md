# 部署指南

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

## 1. 前置要求

| 方式 | 要求 |
|------|------|
| 源码运行 | Go 1.25（与 `go.mod` 一致） |
| 源码运行（前端） | Node 20+ |
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

镜像三阶段构建：Node 20 构建前端 → Go 编译（CGO_ENABLED=0）→ `gcr.io/distroless/static-debian12:nonroot` 运行时（无 shell、非 root）。

GitHub Actions 会把多架构镜像推送到 `<DOCKERHUB_USERNAME>/model-connectivity`，其中用户名来自仓库 Secret。项目流水线未配置 GHCR 发布，因此本文不使用旧的 `ghcr.io` 地址。

## 4. 二进制部署

1. 从 [GitHub Releases](https://github.com/wututua/AI_Model_Connectivity/releases) 下载对应平台压缩包并解压（内含平台命名的二进制、`.env.example`、`README.md`、`web/`）。
2. `cp .env.example .env` 并填写 Provider 与初始管理员 `ADMIN_USERNAME` / `ADMIN_PASSWORD`（也可使用启动日志生成的密码）。
3. 启动：`./model-connectivity`（Windows：`model-connectivity.exe`）。

常用子命令：

| 命令 | 说明 |
|------|------|
| `model-connectivity` / `serve` | 常驻服务（默认） |
| `model-connectivity check` / `once` | 跑一次检测后退出，适合 cron |
| `model-connectivity healthcheck` | 请求本机 `/health`，用于容器健康检查 |

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
EnvironmentFile=/opt/cg/.env
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
2. 替换二进制或镜像 tag，重启。
3. 首次启动日志出现 `server started` 且 `/health` 返回 `{"ok":true}` 即成功。
4. 旧版本升级账号系统：符合密码规则的旧管理 Token 迁移为初始管理员密码，否则生成新密码并打印到日志；首次登录必须改密。
5. 旧 Bearer 和只读分享密钥停止工作；为只读访问者创建普通用户。Prometheus 抓取也需改为维护有效登录会话。
6. 回滚：恢复旧二进制 + 升级前一致性数据库备份。账号迁移会删除旧 Token KV，不能只替换旧二进制。

`ADMIN_PASSWORD` 不会重置已存在的管理员；日常密码维护使用账户安全或其他管理员的用户管理页面。

## 8. 发布流水线

### GitHub Actions

| 工作流 | 触发 | 内容 |
|--------|------|------|
| `ci.yml` | push / PR（排除 `v*` tag） | `go vet`、`go test -race`、前端单元测试和构建 |
| `release.yml` | tag `v*` / main / 手动 | Linux 竞态测试、Windows 单元测试、前端单元测试和构建通过后，允许 6 平台交叉编译与镜像发布；打 tag 时发布 Release，tag 或 `main` 推送 Docker Hub 多架构镜像 |

## 9. 容量与性能建议

- 默认 `CONCURRENCY=1` 串行探测；Provider 较多可适当提高，但注意上游限流。
- 定时检测建议 6–12 小时一次，避免不必要的 token 消耗。
- 每个模型最多保留 `MAX_HISTORY_RECORDS`（默认 500）条，`probe_results` 另有 90 天保留策略；SQLite 文件通常仅几 MB。
