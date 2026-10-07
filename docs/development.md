# 开发指南

[项目首页](../README.md) · [文档索引](README.md) · [贡献指南](../CONTRIBUTING.md) · [系统架构](architecture.md)

本文面向代码贡献者。安装和日常使用见[部署指南](deployment.md)，前端交互与组件约定见[前端开发](frontend.md)。

## 环境准备

- Go 1.26.8，与 [go.mod](../go.mod) 一致；常规构建无需 CGO。
- Node.js 24，仅修改或重新构建前端时需要。
- Git；Make 为可选工具，Windows 可使用下文的直接命令。
- 运行 Go race 测试需要受支持的平台及 CGO/C 编译环境。

建议使用独立开发数据库与测试 Provider，不要让开发实例连接生产数据目录。

## 本地运行

仓库已经提交 `web/` 构建产物，只启动后端即可使用：

```bash
go run ./cmd/cg
```

修改前端时，在另一终端运行：

```bash
npm ci --prefix frontend
npm run dev --prefix frontend -- --host 127.0.0.1
```

打开 Vite 输出的地址，默认端口为 `5173`。`/api` 和 `/health` 代理到 `http://localhost:8080`；修改后端端口时同步调整 [vite.config.ts](../frontend/vite.config.ts)。首次登录流程与发布版一致。

## 构建与测试

从仓库根目录执行：

```bash
go vet ./...
go test ./...
go test -race ./...
npm test --prefix frontend
npm run build --prefix frontend
go build -o model-connectivity ./cmd/cg
```

Windows 构建输出可改为 `model-connectivity.exe`。二进制运行时仍需配套的 `web/`；前端构建会执行 `tsc -b` 并更新此目录，源码与构建产物应同步提交。

界面变更还应运行[浏览器回归](frontend.md#浏览器回归)。浏览器测试模拟 API，不会调用真实模型。

**单次真实检测不是测试套件的一部分。** 需要手动验证上游兼容性时，可在独立数据目录运行 `go run ./cmd/cg check`，它会消耗 Token；不要与常驻实例共用数据库并行执行。

### Make 快捷命令

| 命令 | 用途 |
| --- | --- |
| `make dev-backend` | 启动后端 |
| `make dev-frontend` | 启动 Vite，需先安装依赖 |
| `make build` | 安装前端依赖并构建前后端，输出到 `dist/` 与 `web/` |
| `make build-backend` | 仅构建后端 |
| `make test` | Go 详细输出与 race 测试 |
| `make lint` | `go vet ./...` |
| `make clean` | 清理 `dist/` 和 `frontend/dist/`，不清理数据库 |

## 模块边界

| 目录 | 职责 |
| --- | --- |
| `cmd/cg` | 应用生命周期、配置更新、检测事务与调度 |
| `internal/config` | 环境变量解析、配置模型和校验 |
| `internal/auth` | 账号规则、密码哈希与校验 |
| `internal/provider` | Provider 抽象、OpenAI 兼容与 Anthropic / Gemini 原生客户端 |
| `internal/probe` | 目标收集、并发与探测，不直接写库 |
| `internal/report` | 报告聚合、投影、历史和统计 |
| `internal/storage` | SQL、迁移与持久化领域操作 |
| `internal/web` | HTTP、SSE、认证授权和静态资源 |
| `internal/notify` | 通知范围、状态变化和发送 |
| `internal/httpclient` | 出站请求地址校验与连接 |

`application` 实现 `web.AdminController`。新增业务能力应遵循现有模块边界，避免在路由处理器中直接写 SQL，或在基础 UI 组件中加入业务请求。

## 修改配置与 API

新增可持久化配置时：

1. 增加字段，更新 `SettingsFromConfig` 与 `ApplyRuntimeSettings` 映射。
2. 补充 `ValidateRuntimeSettings` 校验、默认值及旧数据兼容测试。
3. 同步前端类型、表单、导入导出和敏感字段语义。
4. 更新[配置参考](configuration.md)，明确即时生效还是重启后生效。

修改 API 时：

1. 在 `internal/web` 注册路由并区分 `requireAdmin` / `requireAuth`，检查 CSRF 与请求体边界。
2. 按需更新 `AdminController` 及 `cmd/cg` 实现。
3. 同步 [types.ts](../frontend/src/types.ts) 与 [api.ts](../frontend/src/api.ts)。
4. 在 [HTTP API](api.md) 维护端点约定，在[前端集成](backend-api.md)维护跨请求行为。
5. 覆盖匿名、普通用户、管理员、初始改密、失败及取消路径。

## 前端约定

- 接口调用复用 `api.ts`；状态订阅复用现有 `liveStatus` 控制器，避免独立请求互相覆盖。
- 基础控件复用 `components/ui/`；跨目录导入优先使用 `@/*` 别名。
- 字段使用 snake_case 并同步后端；保持 TypeScript 未使用变量检查通过。
- 语义颜色、主题、键盘访问与减弱动画沿用现有组件规范。
- 迟到请求、保存失败、权限变化及未保存编辑都应有明确处理，敏感草稿不写入浏览器存储。

## 验证范围

已有测试覆盖配置边界、密码与会话、HTTP 权限、Provider 错误与用量、探测去重、报告投影、通知范围、数据库事务和前端请求竞态。具体用例以各目录的 `*_test.go` 与 `frontend/tests/` 为准。

CI 执行 Go 静态检查、Linux race 测试、govulncheck、npm audit、前端单元测试、构建和浏览器回归；发布工作流另有 Windows 单元测试。跨平台编译不代表已在每个目标平台运行测试。

文档改动需检查链接、锚点和命令，不必重建应用。无法在本地执行的检查请在 PR 中说明，不能把“未运行”写成“通过”。

### 发布包升级验收

完整解压旧版与候选发布包后，可用 Node.js 24 执行真实 HTTP 升级检查：

```bash
node scripts/tests/test_release_upgrade.cjs /path/to/previous/model-connectivity /path/to/candidate/model-connectivity
```

Windows 使用对应的 `.exe` 路径。源码构建可用第三个参数指定候选 `web/` 的路径。脚本使用临时数据库、随机本地端口和模拟上游，检查账号 / 会话、配置、历史、用量、事件、备份以及新增原生协议和审计；完成后关闭自己的进程并删除临时数据，不读写部署数据或调用付费模型。此脚本面向 beta.5 到 beta.6 的升级，不替代 systemd、容器或生产数据的恢复演练。

## 发布

- 先确认目标提交通过检查，更新 [CHANGELOG](../CHANGELOG.md) 与对应 `docs/releases/<tag>.md`。
- Release 说明先放完整中文，再放完整英文；不要逐段交错或重写已发布版本事实。
- 推送 `v*` tag 后由工作流生成发布包、校验文件、Release 与镜像；不要覆盖已有标签或发布附件。
- 发布触发条件、预发布标签与产物见[发布流水线](deployment.md#发布流水线)。
