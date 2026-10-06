# 系统更新

[项目首页](../README.md) · [部署指南](deployment.md) · [运维指南](operations.md)

管理后台的 **系统更新** 页面显示当前版本、运行平台、发布说明及最近一次更新结果。进入页面或切换通道会检查 GitHub Release，同一通道成功响应在进程内缓存 5 分钟；也可点击“检查更新”。没有后台自动安装、自动重试或定时重启。

## 支持范围

| 部署 | 检查版本 | 后台安装 |
| --- | --- | --- |
| Linux amd64 / arm64，脚本安装 | 支持 | 显式启用独立更新服务后支持 |
| Docker | 支持 | 不支持，在宿主机更新镜像并重建容器 |
| 手动部署、Windows、macOS | 支持 | 下载完整发布包手动更新 |
| `dev` 等非发布构建 | 可查看 Release | 不支持自动比较与安装 |

仅管理员可访问这些接口。写请求仍需要管理员会话、同源校验及 CSRF。指标凭据和普通用户不能检查或提交系统更新。

## 启用一键更新

先使用新版安装脚本安装包含此功能的发布版本。旧版程序不支持更新协议时，须先通过脚本升级，不能仅下载新版脚本就直接启用。

```bash
sudo bash install-model-connectivity.sh enable-updates
```

也可在安装管理菜单中选择“启用后台更新”。启用期间会停服备份、更新 systemd 配置并恢复原运行状态；已停止的服务不会被启动。

启用操作会安装：

| 文件 | 用途 |
| --- | --- |
| `/usr/local/lib/model-connectivity-updater/model-connectivity` | 独立更新执行器副本，由 root 管理 |
| `/usr/local/lib/model-connectivity-updater/install.sh` | 固定的安装器副本，由 root 管理 |
| `model-connectivity-update.path` | 监听固定请求文件 |
| `model-connectivity-update.service` | 独立于应用服务执行更新 |
| `/var/lib/model-connectivity-updater/requests/request.json` | 应用原子提交的单次请求 |
| `/var/lib/model-connectivity-updater/status.json` | root 写入的最近一次任务状态，不在应用数据库内 |

请求目录由 root 拥有，服务组可写；应用不能替换其父目录、执行器、安装脚本或状态文件。应用继续使用非 root 账户，不挂载 Docker socket，不开放任意命令、路径或 URL 参数。独立更新服务需要 root 权限，只处理固定仓库 `wututua/AI_Model_Connectivity` 的已发布版本。

更新执行器与安装器副本不会随应用更新自动替换。需要更新它们时，由服务器管理员审核新脚本后重新执行 `enable-updates`。存在未处理请求时会拒绝重新启用。

禁用新的后台更新：

```bash
sudo bash install-model-connectivity.sh disable-updates
```

执行器正在运行时不允许禁用或卸载；请等待任务结束。禁用和卸载均保留更新记录，禁用不清除已排队请求。

## 更新流程

1. 选择 `stable` 或 `preview`。稳定通道不自动回退到预发布；预发布通道从最近 100 条公开 Release 中按发布时间选择，包含正式版。
2. 查看原始发布说明，核对目标版本。仅允许升级到所选通道检查到的更高版本，不支持后台降级。
3. 点击“安装更新”并确认停服、备份和预发布风险。检测正在执行时需要先等待或停止检测。
4. 独立执行器重新查询官方 Release，并核对当前安装版本。版本已变化、附件缺失或网络失败时，不继续安装。
5. 安装器下载完整压缩包及 `SHA256SUMS.txt`，检查哈希、归档路径、文件类型、完整前端及二进制架构。
6. 停止服务并备份程序、数据目录和服务配置，再替换程序及 `web/`，保留端口、地址及数据库。
7. 启动服务并进行本机健康检查。失败时尝试同时恢复旧程序、数据库和服务配置；不能只回滚二进制。

更新任务存在时，新的检测任务暂停受理。更新期间网页连接中断是可能发生的，页面会轮询重连；提交安装请求时也继续查询状态。提交响应超过 25 秒、断线或返回服务器错误时，页面会确认该次提交结果，不会自动重复提交。确认接口会使尚未入队的旧请求失效，防止延迟请求随后启动安装；未入队或已结束的请求确认后可重新操作，已排队的任务则继续显示进度。确认接口暂不可用时保持禁止重复提交，恢复连接后继续确认。成功后重新加载页面以使用新前端。

请确保没有其他进程写入同一数据库，且磁盘空间足够存放完整备份。健康检查不调用模型，但服务启动后原有定时任务会恢复。数据目录中的运行设置优先级不因更新而改变。

## 异常恢复

```bash
sudo systemctl status model-connectivity-update.service --no-pager
sudo journalctl -u model-connectivity-update.service -n 100 --no-pager
sudo systemctl status model-connectivity.service --no-pager
```

- `succeeded`：安装器已完成更新；原来运行的服务通过本机健康检查，原来停止的服务仍保持停止。
- `rolled_back`：安装失败，安装器报告旧程序、数据库和服务已恢复。
- `failed`：核对、下载、校验或替换前的步骤失败；先查日志及应用状态。
- `recovery_required`：替换后的恢复结果未确认，阻止再次更新和新检测任务，需要服务器管理员处理。
- 长时间停留在 `pending` / `running`：可能是网络等待、更新服务未启动、进程被终止或机器断电，不能据此认为成功或已回滚。

执行器先持久化任务状态，再消费请求，然后执行安装。任务一旦开始，不会因为网页刷新或机器重启而自动重放。突然断电、`SIGKILL`、磁盘耗尽、systemd 故障不保证自动恢复成功。

对未确认任务，先禁用请求监听 `sudo systemctl disable --now model-connectivity-update.path`，确认更新执行器及安装器子进程都已退出，再检查备份的 `complete` 标记，按[备份与恢复](operations.md#备份与恢复)恢复一致的程序、数据库和服务。核对运行状态后，由 root 将 `status.json` 及仍存在的 `requests/request.json` 移到私有归档目录，最后重新运行 `enable-updates`。不要在任务运行时删除状态或强行开始另一个更新。

## 安全与限制

- 下载通过 HTTPS，校验使用同一官方 Release 的 SHA-256 文件。当前没有独立发布签名验证；哈希校验不能替代发布者身份验证或防御发布账户被攻破。
- Release 说明以纯文本显示，不执行其中的 HTML。
- 当前只保存最近一次更新状态，备份目录和 systemd 日志可保留进一步排障证据；不是完整更新审计历史。
- 不自动开放防火墙，不支持 Docker 内部自更新，不承诺零停机或跨版本数据库兼容。
- 自动化测试覆盖隔离执行器、权限 API、备份恢复模拟和浏览器交互；仍需要在真实 Linux/systemd 测试环境验证启用、升级、失败恢复及断电后的人工恢复流程。
