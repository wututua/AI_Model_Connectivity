# 部署指南

[项目首页](../README.md) · [文档索引](README.md) · [配置参考](configuration.md) · [运维指南](operations.md)

本文说明发布包、容器和 Linux 服务部署。首次登录与添加 Provider 的步骤见[快速开始](../README.md#首次使用)。

## 目录

- [环境要求](#环境要求)
- [Linux 安装脚本](#linux-安装脚本)
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

## Linux 安装脚本

[install-model-connectivity.sh](../install-model-connectivity.sh) 面向使用 systemd 的 Linux，支持 amd64 / arm64。它需要 root 权限、Bash 4+、Python 3.8+、curl、CA 证书、util-linux（`flock`）、coreutils、tar 和用户管理工具。缺少依赖时脚本会停止，不自动修改系统软件源、安装软件包或开放防火墙；不支持 OpenRC、无 systemd 的容器、Windows 或 macOS。

从仓库下载脚本并检查内容后执行：

```bash
curl -fL --proto '=https' --proto-redir '=https' \
  -o install-model-connectivity.sh \
  https://raw.githubusercontent.com/wututua/AI_Model_Connectivity/main/install-model-connectivity.sh

sudo bash install-model-connectivity.sh
```

请使用 `bash`，不要使用 `sh` 或直接将网络响应管道传给 root shell。也可使用相应 Release 附带的脚本，并用同一版本的 `SHA256SUMS.txt` 核对；旧 Release 不会补写脚本附件。

### 界面语言

在终端启动脚本后，首先显示：

```text
1. 简体中文
2. English
```

输入 `1` 或直接回车使用简体中文，输入 `2` 使用 English。所选语言用于本次运行的菜单、帮助、操作提示、警告和安装器校验错误，下次运行会重新选择，不写入服务配置。无参数时，选择语言后进入管理菜单；带命令运行时，选择语言后执行该命令。`systemctl`、`journalctl`、`curl` 等外部工具仍保留其原始输出。

标准输入不是终端时，不显示语言选择、不读取输入，默认使用 English，避免自动化任务等待。`--yes` 只跳过操作确认，在终端运行时仍会询问语言，以及安装时尚未指定的地址、端口；无人值守时应重定向标准输入，例如 `sudo bash install-model-connectivity.sh upgrade --channel preview --yes < /dev/null`。

### 通道与安装

- 不带参数显示安装、升级、备份、状态、日志、启动、停止、重启和卸载菜单。
- `stable` 是默认通道，只安装正式版；没有正式版时明确报错，不自动退回预发布版。
- `preview` 从 GitHub 最近 100 条公开 Release 中按发布时间选择最新版本，包括 beta / RC 或正式版，不使用分支快照。
- `--version` 指定确切标签，可明确选择预发布版本。下载和校验失败不会停止现有服务。

```bash
# 评估预发布版本
sudo bash install-model-connectivity.sh install --channel preview

# 固定一个已发布版本；自动化运行时显式确认
sudo bash install-model-connectivity.sh install --version v1.0.0-beta.4 --yes

# 指定监听地址和端口；不要直接暴露尚未加固的管理入口
sudo bash install-model-connectivity.sh install \
  --channel preview --host 127.0.0.1 --port 8081
```

`--host`、`--port` 和 `--secure-cookies` 仅用于首次安装或卸载后的重新安装。HTTPS 代理部署可加 `--secure-cookies`，但纯 HTTP 本地访问不应启用。脚本不修改防火墙，也不配置反向代理。

### 自定义地址与端口

无论从菜单选择安装，还是直接运行 `install`，终端中都会询问未通过 `--host`、`--port` 指定的值：

```text
监听 IP [127.0.0.1]：
监听端口（1-65535）[8080]：
```

回车保留默认值，格式无效时可重新输入。安装确认前会显示最终监听地址，升级不会重新询问或覆盖这些设置。已通过参数指定的值不再询问；非交互安装直接使用参数或默认值。

- `127.0.0.1`：仅监听本机 IPv4，适用于本地访问或同机反向代理。
- `0.0.0.0`：监听所有 IPv4 接口，可用于远程访问；安装器会提醒配置防火墙和 HTTPS。
- `::1`、`::` 或本机其他 IPv6 地址：支持数字形式的 IPv6，不带方括号输入；摘要使用 `[地址]:端口` 显示。
- 地址不填写域名、URL 或 `IP:端口`；端口单独填写。绑定地址不可用或端口被占用时，安装会失败，不会覆盖已有数据。低端口还取决于系统对非 root 服务的绑定限制，通常建议使用 `1024-65535`。

| 内容 | 路径 / 行为 |
| --- | --- |
| 程序和完整 `web/` | `/opt/model-connectivity`，root 管理 |
| SQLite 与运行数据 | `/var/lib/model-connectivity`，专用服务账户拥有，目录权限 `0700` |
| 离线备份 | `/var/backups/model-connectivity/<时间>-<随机后缀>`，root 私有，不自动清理 |
| systemd 服务 | `/etc/systemd/system/model-connectivity.service` |
| 服务账户 | `model-connectivity`，非 root、不可交互登录 |
| 首次监听 / 登录 | `127.0.0.1:8080`，首次初始化时要求状态页登录 |

启动变量直接写在 systemd 单元中，不创建应用配置文件。程序目录里的 `.installer.json` 仅记录安装器版本、监听设置和服务文件校验值，不是应用运行配置；Provider、密码、通知与运行设置仍保存在 SQLite。已有数据库的运行设置不会被安装默认值覆盖。

### 升级与卸载

管理员后台也可[检查版本和提交更新](system-updates.md)。一键更新仅限脚本管理的 Linux，首次需由服务器管理员运行 `sudo bash install-model-connectivity.sh enable-updates`；不会默认授予 Web 程序 root 权限。

```bash
sudo bash install-model-connectivity.sh upgrade --channel preview --yes
sudo bash install-model-connectivity.sh backup --yes
sudo bash install-model-connectivity.sh status
sudo bash install-model-connectivity.sh logs
sudo bash install-model-connectivity.sh restart
sudo bash install-model-connectivity.sh uninstall --yes
```

升级先下载并验证完整发布包及 SHA-256，拒绝路径穿越、链接、设备文件、不完整前端和架构不匹配的压缩包，然后停服备份程序、数据目录及服务文件。正常升级保留端口、启动配置、账户、历史和用量；原来停止的服务升级后仍保持停止。运行中的服务升级后须通过本机健康检查，否则尝试从同一份备份恢复程序、数据库和服务配置。健康检查不调用模型，但服务启动后原有定时任务会继续运行。

备份和升级期间，必须确保没有其他手工进程或容器写入同一数据目录。备份包含凭据且可能较大，操作前检查剩余磁盘空间。默认不会删除历史备份，也不会自动降级当前版本；明确指定旧标签属于管理员选择，操作前应阅读对应版本的数据兼容性说明。

卸载同样先停服备份，然后删除脚本管理的服务和程序；数据库、备份和专用账户保留。重新安装时可识别卸载保留的数据，不重置已有用户密码。脚本不会接管手动创建的安装，也不会覆盖带自定义修改或 drop-in 的服务。需要改变脚本安装的监听设置时，可先备份、卸载，再固定原版本重新安装并传入新参数；需要更多 systemd 定制时，改用[手工部署](#systemd)并自行维护升级。

### 失败恢复

普通下载、校验失败发生在停服前；备份失败会尝试恢复原服务运行状态。升级失败后的自动恢复会先停止新服务，再恢复旧数据库，绝不会仅把旧二进制放到已迁移的数据库上。

断电、`SIGKILL`、磁盘耗尽或系统服务管理失败时，自动恢复不一定能够完成。脚本会保留备份，不宣称这类情况已经恢复。完整备份带有 `complete` 文件，其中 `program/`、`data/` 和 `service` 分别对应程序目录、数据目录和 systemd 单元；首次安装前不存在的项目不会出现在备份中。手工恢复必须停服，并同时恢复该快照的程序、数据和服务，重新执行 `daemon-reload` 后再启动。不要将多份快照混用，也不要直接启动其他写入同一数据库的进程。一般备份原则见[备份与恢复](operations.md#备份与恢复)。

首次密码通过以下命令查看，日志请勿公开：

```bash
sudo journalctl -u model-connectivity.service -n 50 --no-pager
```

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

旧账号系统升级会删除旧 Token KV。旧管理 Bearer 和只读分享密钥不再有效；只读访问需创建普通用户，指标采集可创建仅限 `/metrics` 的新指标凭据或维护有效登录会话。`ADMIN_PASSWORD` 不会重置已有账号，忘记密码请使用[离线恢复命令](operations.md#管理员密码恢复)。

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
