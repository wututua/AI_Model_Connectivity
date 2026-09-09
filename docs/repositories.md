# 仓库与发布渠道

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity)

项目同时托管在 GitHub 与 CNB。两个仓库使用同一份 README 和文档，因此从任一平台都可以跳转到另一端。

## 仓库地址

| 渠道 | 地址 | 定位 |
|------|------|------|
| GitHub | <https://github.com/wututua/AI_Model_Connectivity> | 上游源码、Issue、Pull Request、GitHub Actions 和 GitHub Release |
| CNB | <https://cnb.cool/ligzs/AI_Model_Connectivity> | 国内访问镜像、CNB 云原生构建和 CNB Release |

克隆时任选一个地址：

```bash
git clone https://github.com/wututua/AI_Model_Connectivity.git
# 或
git clone https://cnb.cool/ligzs/AI_Model_Connectivity.git
```

需要同时保留两个远端时：

```bash
git remote add github https://github.com/wututua/AI_Model_Connectivity.git
git remote add cnb https://cnb.cool/ligzs/AI_Model_Connectivity.git
git remote -v
```

## 同步关系

同步方向为 **GitHub → CNB**。`.cnb.yml` 在 CNB 的 `main` 分支上配置 `tencentcom/git-sync`，每天 UTC 01:00、09:00、17:00 拉取 GitHub 源码和标签，对应北京时间 09:00、17:00、次日 01:00。

这意味着两个仓库可能短暂显示不同的最新提交。排查版本差异时应比较完整 commit SHA；需要提交代码、Issue 或 Pull Request 时，以 GitHub 上游仓库为准。

## CNB 云原生构建

CNB 流水线定义在仓库根目录 [`.cnb.yml`](../.cnb.yml)，发布环境定义在 [`.cnb/tag_deploy.yml`](../.cnb/tag_deploy.yml)。

| 能力 | 实现 |
|------|------|
| 隔离构建环境 | 后端使用 `golang:1.25`，前端使用 `node:20-slim` |
| 依赖缓存 | 挂载 `go-modules` 与 `go-build` 缓存卷 |
| 发布门禁 | 先运行 `go test -v ./...`，测试失败时不进入平台构建 |
| 前端产物复用 | `npm ci` + `npm run build` 只构建一次，6 个后端任务复用 `web/` |
| 跨平台产物 | Linux、Windows、macOS × amd64、arm64，共 6 个目标 |
| 可追溯版本 | 编译时写入 `$CNB_BRANCH` 与 `$CNB_COMMIT_SHA` |
| 自动打包 | Windows 生成 `.zip`，其他平台生成 `.tar.gz` |
| 自动发布 | 汇总各构建任务产物并创建 CNB Release |

### 触发方式

| 事件 | 行为 |
|------|------|
| `tag_push` | 测试、构建前端、并行交叉编译、打包并发布 Release |
| `tag_deploy.release` | 使用同一套发布流水线，支持 CNB 发布环境触发 |
| `main` 定时任务 | 从 GitHub 拉取源码和标签；新标签进入 CNB 后触发 `tag_push` |

CNB 流水线目前发布二进制压缩包，不负责推送容器镜像。

## GitHub Actions

| 工作流 | 触发 | 作用 |
|--------|------|------|
| `ci.yml` | 非 `v*` tag 的 push、Pull Request | 运行 `go vet`、`go test -race` 和 TypeScript 检查 |
| `release.yml` | `main`、手动运行、`v*` tag（以 GitHub 当前 workflow 为准） | 构建前端和 6 平台二进制；标签发布 GitHub Release，`main` 或标签推送 Docker Hub 多架构镜像。不同镜像同步时可能包含额外的测试/容器校验步骤 |

Docker Hub 镜像名由仓库 Secret `DOCKERHUB_USERNAME` 决定，工作流目标为 `<DOCKERHUB_USERNAME>/model-connectivity`。文档不使用未经流水线配置的 GHCR 地址；本地部署可以直接用 `docker compose up -d --build` 从源码构建。

## Release 渠道

- GitHub：<https://github.com/wututua/AI_Model_Connectivity/releases>
- CNB：<https://cnb.cool/ligzs/AI_Model_Connectivity/-/releases>

两个渠道都提供 Linux、Windows、macOS 的 amd64/arm64 包。由于 CNB 依赖定时同步 GitHub tag，CNB Release 可能比 GitHub Release 稍晚出现。

发布包包含：

```text
model-connectivity[-<os>-<arch>][.exe]
.env.example
README.md
web/
```

校验下载版本时，优先比对 Release tag、commit SHA 和压缩包中的文件结构。部署步骤见 [deployment.md](deployment.md)，发布操作见 [development.md](development.md#7-发布)。
