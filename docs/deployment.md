# 部署指南

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
export ADMIN_TOKEN='至少16位的随机字符串'
docker compose up -d
```

- 镜像内 `APP_HOST=0.0.0.0`，Compose 会在 `ADMIN_TOKEN` 缺失时直接报错（`${ADMIN_TOKEN:?...}`）。
- 数据卷 `./data:/app/data` 持久化 SQLite。
- 健康检查：容器内执行 `model-connectivity healthcheck`（30s 间隔，start_period 10s）。

Linux 绑定挂载前请准备目录权限（容器以 `65532:65532` 运行）：

```bash
mkdir -p data && sudo chown 65532:65532 data && sudo chmod 0700 data
```

也可使用命名卷：`-v model-connectivity-data:/app/data`。

## 3. docker run

```bash
docker run -d -p 8080:8080 \
  -e ADMIN_TOKEN \
  -e PROVIDER_1_ID=openai \
  -e PROVIDER_1_BASE_URL=https://api.openai.com/v1 \
  -e PROVIDER_1_API_KEY=sk-xxx \
  -e PROVIDER_1_MODELS=gpt-4o-mini \
  -v $(pwd)/data:/app/data \
  --name model-connectivity \
  ghcr.io/wututua/ai_model_connectivity:latest
```

镜像三阶段构建：Node 20 构建前端 → Go 编译（CGO_ENABLED=0）→ `gcr.io/distroless/static-debian12:nonroot` 运行时（无 shell、非 root）。

## 4. 二进制部署

1. 从 Releases 下载对应平台压缩包并解压（内含 `model-connectivity`、`.env.example`、`README.md`、`web/`）。
2. `cp .env.example .env` 并填写 Provider 与 `ADMIN_TOKEN`。
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
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}

location /api/events {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
    chunked_transfer_encoding on;
}
```

注意事项：

- 应用**不信任**客户端 `X-Forwarded-For`，认证限流以连接来源 IP 为准；反向代理后的用户会共享代理 IP 的限额。若要按真实 IP 限流，请在受信任的代理层实现。
- 建议强制 HTTPS 并为 `Authorization` 头保密。
- 后端已返回 `X-Accel-Buffering: no`，Nginx 会据此关闭缓冲。

## 7. 升级与回滚

1. 备份 `data/`（含 SQLite 与 WAL）。
2. 替换二进制或镜像 tag，重启。
3. 首次启动日志出现 `server started` 且 `/health` 返回 `{"ok":true}` 即成功。
4. 回滚：恢复旧二进制 + 旧 `data/`；数据库 schema 变更是向后兼容的 `ALTER TABLE ADD COLUMN`。

## 8. 发布流水线

### CNB（`.cnb.yml`）

- `tag_push` 与 `tag_deploy.release` 触发：单元测试 → 构建前端 → 6 个平台交叉编译（linux/windows/darwin × amd64/arm64）→ 打包（tar.gz / zip）→ 发布 CNB Release。
- `main` 分支每日 UTC 1/9/17 点从 GitHub 同步源码与 tag，tag 同步后自动触发发版。

### GitHub Actions

| 工作流 | 触发 | 内容 |
|--------|------|------|
| `ci.yml` | push / PR（排除 `v*` tag） | `go vet`、`go test -race`、前端 `tsc --noEmit` |
| `release.yml` | tag `v*` / main / PR / 手动 | Windows+Linux 后端测试、Linux 竞态检测、前端构建、Compose 校验、容器启动与健康检查、`/api/admin/config` 鉴权验证；打 tag 时发布 Release 并推送多架构镜像 |

## 9. 容量与性能建议

- 默认 `CONCURRENCY=1` 串行探测；Provider 较多可适当提高，但注意上游限流。
- 定时检测建议 6–12 小时一次，避免不必要的 token 消耗。
- 每个模型最多保留 `MAX_HISTORY_RECORDS`（默认 500）条，`probe_results` 另有 90 天保留策略；SQLite 文件通常仅几 MB。
