# 部署指南

[项目首页](../README.md) · [文档索引](README.md) · [配置参考](configuration.md) · [运维指南](operations.md)

本文说明发布包、容器和 Linux 服务部署。首次登录与添加 Provider 的步骤见[快速开始](../README.md#首次使用)。

## 目录

- [环境要求](#环境要求)
- [二进制部署](#二进制部署)
- [Docker Compose](#docker-compose)
- [Docker Run](#docker-run)
- [Systemd](#systemd)
- [公网访问](#公网访问)
- [升级与回滚](#升级与回滚)
- [容量规划](#容量规划)
- [发布流水线](#发布流水线)

## 环境要求

| 方式 | 要求 |
| --- | --- |
| 发布包 | 对应系统和架构的压缩包，无需 Go 或 Node.js |
| Docker | Docker 与 Compose v2；源码构建需能访问构建依赖 |
| 源码运行 | Go 1.26.8，使用仓库已提交的 `web/` |
| 修改前端 | Node.js 24，使用 `npm ci` 安装锁定依赖 |

默认监听 `127.0.0.1:8080`，数据库为工作目录下的 `data/cg.sqlite`。更换工作目录或数据路径会连接到不同数据库，请固定启动目录。

## 二进制部署

1. 从 [Releases](https://github.com/wututua/AI_Model_Connectivity/releases) 下载对应平台的压缩包和 `SHA256SUMS.txt`。
2. 核对压缩包的 SHA-256，再完整解压。保留 `web/`、文档、资源和许可证，不要只复制可执行文件。
3. 在解压目录启动。未预设初始密码时，请查看启动日志并完成首次改密。

Linux / macOS：

```bash
./model-connectivity
```

Windows PowerShell：

```powershell
.\model-connectivity.exe
```

| 命令 | 用途 |
| --- | --- |
| `model-connectivity` / `serve` | 启动常驻服务 |
| `model-connectivity check` / `once` | 执行一次真实检测后退出，会消耗 Token |
| `model-connectivity healthcheck` | 请求本机 `/health`，不检测上游模型 |
| `model-connectivity --version` | 输出版本、提交、工具链和目标平台，不打开数据库 |
| `model-connectivity recover-admin <用户名>` | 停服后恢复已有管理员，详见[恢复流程](operations.md#管理员密码恢复) |

发布包不需要额外创建配置文件。Provider、通知与运行设置通过管理面板维护。

## Docker Compose

仓库中的 [docker-compose.yml](../docker-compose.yml) 从源码构建镜像：

```bash
git clone https://github.com/wututua/AI_Model_Connectivity.git
cd AI_Model_Connectivity
```

Linux 使用绑定挂载前，先准备数据目录。容器以 `65532:65532` 运行：

```bash
mkdir -p data
sudo chown 65532:65532 data
sudo chmod 0700 data
```

然后启动：

```bash
docker compose up -d --build
docker compose logs model-connectivity
```

- `./data:/app/data` 持久化 SQLite；不要在升级时删除此目录。
- 容器内监听 `0.0.0.0:8080`，Compose 默认发布主机所有接口的 `8080` 端口。
- Compose 通过 `environment` 注入启动变量；未列出的变量需要显式加入配置。
- 不预设管理员密码时会生成随机密码并打印到日志，请限制日志访问。
- 健康检查每 30 秒执行一次 `model-connectivity healthcheck`，启动宽限期为 10 秒。

如由同机反向代理提供外部访问，将 `ports` 改为 `"127.0.0.1:8080:8080"`。HTTPS 部署还需设置 `SECURE_COOKIES=true`，详见[公网访问](#公网访问)。

## Docker Run

先从仓库根目录构建本地镜像：

```bash
docker build -t model-connectivity:local .
```

以下示例使用 Docker 命名卷，并仅向本机发布端口：

```bash
docker run -d \
  --name model-connectivity \
  --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -v model-connectivity-data:/app/data \
  model-connectivity:local
docker logs model-connectivity
```

首次登录后在后台添加 Provider。也可用 `-e` 注入[启动变量](configuration.md#配置优先级)；它们不会覆盖 SQLite 中已有的运行设置。

镜像使用 Node.js 构建前端、Go 编译后端，运行层为 `gcr.io/distroless/static-debian12:nonroot`，不包含 shell。需要维护数据时使用应用提供的命令或受控的宿主机工具，不要依赖 `docker exec ... sh`。

发布流水线推送到 `<DOCKERHUB_USERNAME>/model-connectivity`，命名空间取自仓库 Secret，本文不假设固定的公共镜像地址。

## Systemd

以下示例适用于使用 systemd 的 Linux。将发布包完整解压到 `/opt/model-connectivity`，二进制和 `web/` 放在同一目录；程序目录由 root 管理，服务只写入数据目录。

创建专用账户和数据目录（若账户已存在则跳过创建）：

```bash
sudo useradd --system --user-group --home-dir /var/lib/model-connectivity \
  --shell /usr/sbin/nologin model-connectivity
sudo install -d -o model-connectivity -g model-connectivity \
  -m 0700 /var/lib/model-connectivity
```

在 `/etc/systemd/system/model-connectivity.service` 配置：

```ini
[Unit]
Description=AI Model Connectivity
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=model-connectivity
Group=model-connectivity
WorkingDirectory=/opt/model-connectivity
ExecStart=/opt/model-connectivity/model-connectivity
Environment=APP_HOST=127.0.0.1
Environment=APP_PORT=8080
Environment=WEB_DIR=/opt/model-connectivity/web
Environment=DATA_DIR=/var/lib/model-connectivity
Restart=on-failure
RestartSec=5
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/model-connectivity

[Install]
WantedBy=multi-user.target
```

启动并查看日志：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now model-connectivity
sudo journalctl -u model-connectivity -n 50 --no-pager
```

日志可能包含首次生成的密码。部署到 HTTPS 代理后，在服务配置中加入 `Environment=SECURE_COOKIES=true`，执行 `daemon-reload` 并重启。

## 公网访问

对外开放前请完成：

- 使用 HTTPS，设置 `SECURE_COOKIES=true` 后重启。
- 修改初始密码，在 **系统设置 → 访问控制** 确认实际生效的状态页登录要求。
- 通过防火墙或反向代理限制管理入口，不直接暴露数据目录和启动日志。
- 代理保留原始 Host（含端口），不缓存 `/api/` 响应，关闭 SSE 缓冲。

以下 Nginx 片段放入已配置证书的 HTTPS `server` 中，不是完整 TLS 配置：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
}

location /api/events {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header Connection '';
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 3600s;
}
```

应用不信任客户端的 `X-Forwarded-For` 和转发协议头，认证限流按连接来源 IP 计算。代理后所有用户可能共享限额；需要按真实 IP 限流时，在可信代理层实现。纯 HTTP 本地访问不要开启 Secure Cookie，否则浏览器可能无法保持会话。

## 升级与回滚

1. 阅读目标版本的 [Release 说明](releases/README.md)，记录当前版本、启动参数与数据路径。
2. 按[备份与恢复](operations.md#备份与恢复)生成一致性数据库备份，并保留旧版本文件。不要在服务写入时直接复制数据库主文件。
3. 停服，替换二进制及配套 `web/`，或更新容器镜像。保留原 `DATA_DIR`、`DATABASE_PATH`、监听配置与 `SECURE_COOKIES`。
4. 重启后检查日志和 `/health`，再验证登录、Provider、运行设置、历史与用量。健康检查仅证明进程可响应，不代表升级或上游检测全部成功。
5. 在可接受真实调用成本的前提下执行一次完整检测，独立确认状态与通知范围。

回滚时停止所有写入者，恢复旧版本程序和**升级前的一致性数据库备份**。数据库迁移不保证可逆，仅替换旧二进制可能无法回滚。

旧账号系统升级会删除旧 Token KV。旧 Bearer 和只读分享密钥不再有效；只读访问需创建普通用户，指标采集需维护有效登录会话。`ADMIN_PASSWORD` 不会重置已有账号，忘记密码请使用[离线恢复命令](operations.md#管理员密码恢复)。

## 容量规划

- 默认串行探测；提高并发前确认上游限额、网络资源与调用预算。
- 需要低成本巡检时可从 6–12 小时间隔开始，根据监控需求调整。
- 探测历史同时受 90 天保留期及每模型 `MAX_HISTORY_RECORDS` 限制；用量按日最多保留 365 天。
- 一个数据库只供一个服务或单次检测进程使用，不要让 cron `check` 与常驻服务并行写入同一数据库。

## 发布流水线

| 工作流 | 触发 | 检查与产物 |
| --- | --- | --- |
| [ci.yml](../.github/workflows/ci.yml) | push / PR，排除 `v*` tag | Go 静态检查、竞态测试、安全扫描、前端测试、构建与浏览器回归 |
| [release.yml](../.github/workflows/release.yml) | `v*` tag / `main` / 手动 | Linux 与 Windows 检查、前端检查；tag 产出六平台压缩包、校验文件及镜像 |

带 `-` 的预发布 tag 不更新 Docker `latest` 或 major.minor 别名；正式版本更新 `latest`，`main` 只更新 `main` 镜像标签。版本和提交写入二进制与镜像，本地源码构建默认 `dev`。发布号以 Git tag 为准，不使用前端私有包的版本号。
