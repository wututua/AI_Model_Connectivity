# 开发指南

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) · [仓库与发布](repositories.md)

## 1. 环境

- Go 1.25（见 `go.mod`，无 CGO 依赖）
- Node 20+（前端）
- 首次源码运行需先构建前端，或直接使用仓库已提交的 `web/`

## 2. 常用命令

```bash
make dev-backend     # go run ./cmd/cg
make dev-frontend    # cd frontend && npm run dev（Vite 代理 /api 到 :8080）
make build           # 构建前端 + 后端 → dist/model-connectivity
make build-backend   # 仅后端
make test            # go test -v -race ./...
make lint            # go vet ./...
make clean
```

手动等价命令：

```bash
cd frontend && npm ci && npm run build && cd ..
go run ./cmd/cg            # 常驻
go run ./cmd/cg check      # 单次检测后退出
go test ./... -race
go vet ./...
cd frontend && npx tsc --noEmit
```

## 3. 代码结构约定

| 包 | 职责边界 |
|----|----------|
| `internal/config` | 只做解析、模型定义与纯函数校验，不碰 IO 之外的业务 |
| `internal/provider` | Provider 抽象；目前只有 OpenAI 兼容实现（`New` 的 switch 预留扩展点） |
| `internal/probe` | 并发编排，不直接写库 |
| `internal/report` | 纯函数式报告构建（输入配置+结果+历史，输出报告与新历史） |
| `internal/storage` | 所有 SQL；对外只暴露领域方法 |
| `internal/web` | HTTP 与 SSE；`AdminController` 接口隔离业务逻辑 |
| `cmd/cg` | `application` 实现 `web.AdminController`，负责事务编排 |

新增配置项的三步：加字段 → `SettingsFromConfig` / `ApplyRuntimeSettings` 映射 → `ValidateRuntimeSettings` 校验（必要时同步 `frontend/src/types.ts` 与管理面板表单）。

## 4. 测试

已覆盖（见 `*_test.go`）：

| 包 | 重点 |
|----|------|
| `cmd/cg` | 取消检测不覆盖报告/不发告警；单 Provider 重跑失败保留其他 Provider；密钥轮换与并发一致性 |
| `internal/config` | URL/Provider ID 校验、危险数值拒绝、管理配置不泄露通知凭据 |
| `internal/httpclient` | 拨号前拒绝不安全地址、固定解析结果、DNS 变化后重查 |
| `internal/notify` | 不跟随重定向、拒绝链路本地 webhook、冷却期不丢弃待发告警 |
| `internal/probe` | 跳过规则、去重、截断、错误文本清理 |
| `internal/provider` | 错误响应解析与凭据脱敏 |
| `internal/report` | 合并 Provider、暂停/失败 Provider 空数组、错误可见性不污染入参 |
| `internal/storage` | 最新报告/历史/通知状态/运行时配置/任务生命周期、用量在历史裁剪后仍保留、写入失败回滚、文件权限 |
| `internal/web` | 公开监听判定、认证限流与过期、JSON 边界（空/多值/超限）、token 角色边界 |

CI（`.github/workflows/ci.yml`）跑 `go vet`、`go test -race`、`tsc --noEmit`；发布工作流额外在 Windows 上跑测试、在 Linux 上跑竞态与容器验证。

提交前建议：

```bash
go vet ./... && go test ./... -race
cd frontend && npx tsc --noEmit && npm run build && cd ..
```

## 5. 前端开发约定

- 所有请求走 `src/api.ts`，不要直接使用裸 `fetch`（除 `/api/status`）。
- 新增后端字段需同步 `src/types.ts`；字段为 snake_case。
- 基础控件优先复用 `src/components/ui/` 中的 shadcn/ui 源码组件；新增组件通过 `frontend/components.json` 约束生成位置和风格。
- 颜色使用 `index.css` 中的 HSL 语义令牌和对应 Tailwind 类；品牌操作使用 `primary` 天蓝色，健康、较慢、异常分别使用 `success`、`warning`、`destructive`。
- 数据获取和页面级状态集中在 `pages/`，可复用展示逻辑放入业务组件；不要在 shadcn/ui 基础组件中耦合接口请求。
- `@/*` 映射到 `src/*`，新增跨目录模块优先使用该别名。
- `noUnusedLocals` / `noUnusedParameters` 已开启，注意清理未使用变量。

## 6. 修改 API 时的检查清单

1. `internal/web/server.go` 注册路由并区分 `requireAdmin` / `requireAuth`；
2. 更新 `internal/web` 的 `AdminController` 接口（如新增能力）并在 `cmd/cg` 中实现；
3. 更新 `frontend/src/types.ts` 与 `api.ts`；
4. 更新 `docs/backend-api.md`（前端对接文档）与 `docs/api.md`；
5. 补充测试。

## 7. 发布

- 在 GitHub 上游仓库推送 `v*` tag 后，GitHub Actions 立即产出 6 平台压缩包、发布 GitHub Release，并推送 Docker Hub 多架构镜像。
- CNB 仓库每天北京时间 01:00、09:00、17:00 从 GitHub 同步源码与 tag；新 tag 同步到 CNB 后触发测试、6 平台构建和 CNB Release。
- CNB 也支持 `tag_deploy.release` 发布环境事件，执行与 `tag_push` 相同的二进制发布流程。
- 前端产物 `web/` 已提交到仓库，发布包内自带，无需用户本地构建。
- 双仓库定位、触发器与产物说明见 [repositories.md](repositories.md)。
