# 前端说明

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity) · [CNB 仓库](https://cnb.cool/ligzs/AI_Model_Connectivity) · [仓库与发布](repositories.md)

## 技术栈

| 项 | 选择 |
|---|---|
| 框架 | React 18 + TypeScript + Vite 5 |
| 路由 | React Router 6（`BrowserRouter`） |
| 组件 | shadcn/ui 源码组件 + Radix UI 基础件 |
| 样式 | Tailwind CSS 3 + HSL CSS 变量主题 |
| 图标 | Lucide React |
| 构建 | `npm run build`，产物输出到仓库根目录 `web/` |

项目采用 shadcn/ui 的源码组件模式，不依赖黑盒组件包。组件源码位于 `frontend/src/components/ui/`，配置位于 `frontend/components.json`。Radix UI 提供 Dialog、Alert Dialog、Select、Switch、Tabs 与 Tooltip 的键盘操作、焦点管理和无障碍语义。

TypeScript 与 Vite 均配置了 `@/*` → `src/*` 路径别名，shadcn CLI 通过该别名定位组件、工具函数和样式文件。业务代码现有相对导入可以逐步迁移，新组件应保持同一别名配置，避免创建第二套组件目录。

## 目录结构

```text
frontend/src/
  main.tsx
  App.tsx
  api.ts
  types.ts
  index.css
  lib/
    utils.ts                 className 合并工具
  hooks/
    useTheme.ts              深色、浅色、跟随系统
  utils/
    status.ts                状态、时间和样式映射
  components/
    ui/                      shadcn/ui 基础组件
    ThemeToggle.tsx
    SummaryCard.tsx
    ProviderCard.tsx
    ModelRow.tsx
    CurveChart.tsx
    StatusLights.tsx
    StatusPill.tsx
  pages/
    Dashboard.tsx            公开状态页
    Admin.tsx                认证与后台工作台
    admin/
      OverviewTab.tsx
      ProvidersTab.tsx
      SettingsTab.tsx
      TasksTab.tsx
      BillingTab.tsx
      ConfigTab.tsx
      shared.tsx
```

## 公开状态页

`/` 是面向日常监控的状态运营台：

- 首次通过 `GET /api/status` 加载，随后订阅 `/api/events` SSE；连接不可用时按 30、60、120 秒指数退避轮询。
- 显示整体状态、数据时效、实时连接、可用率、模型和 Provider 指标。
- 支持 Provider、ID 和模型搜索，按状态筛选，按状态、名称、延迟或模型数排序。
- 支持详细与紧凑两种模型视图；详细视图包含 P50/P95/P99、历史状态、延迟曲线和可展开错误信息。
- Provider 级请求错误、首次加载、无报告、无 Provider 和无搜索结果均有独立状态。

## 管理后台

后台使用可深链路径：

| 路径 | 功能 |
|---|---|
| `/admin/overview` | 运行状态、触发/停止检测、最近结果和 Token 估算 |
| `/admin/providers` | Provider 搜索、新增、编辑、删除和单独重测 |
| `/admin/settings` | 检测、历史、调度、通知和访问控制 |
| `/admin/tasks` | 任务状态筛选、结果明细和分页 |
| `/admin/billing` | Token 汇总、每日趋势和模型明细 |
| `/admin/config` | JSON 导入导出和 `.env` 热加载 |

当前登录使用的 Admin Token 或 View Token 存储于 `localStorage.cg_admin_token`。收到 401 时，API 层会清除 Token 并通知后台立即返回认证页，不需要整页刷新。只读会话隐藏系统设置、配置管理和所有写操作。

Provider 表单中 API Key 留空表示保留现有值；只有开启“清除现有 API Key”才会删除。设置页中的通知凭据采用相同语义。

## 主题与组件

`frontend/src/index.css` 定义 shadcn/ui 兼容的 HSL 颜色令牌，包括 `background`、`foreground`、`card`、`primary`、`muted`、`destructive`、`success` 和 `warning`。主色 `primary` 使用天蓝色；健康状态继续使用 `success` 绿色，较慢与异常分别使用 `warning` 和 `destructive`，不要用主色替代状态语义。

`body[data-theme]` 控制深浅色，`useTheme` 在 `dark`、`light`、`auto` 之间循环并写入 `localStorage.theme`。`auto` 跟随浏览器 `prefers-color-scheme`，主题切换按钮由 `ThemeToggle` 统一提供。

新增界面应优先组合 `components/ui/` 中的组件。业务组件可以扩展布局和状态语义，但不应复制 Button、Input、Dialog、Select、Switch、Tabs 或 Table 的基础样式。

需要引入新的 shadcn/ui 组件时，在 `frontend/` 目录执行：

```bash
npx shadcn@latest add <component>
```

生成后检查 `components.json` 中的 `new-york` 风格、CSS 变量模式和 `@/components` 别名是否保持不变，再按项目现有交互和尺寸规范调整组件源码。

## 开发与构建

```bash
# 终端 1
go run ./cmd/cg

# 终端 2
cd frontend
npm install
npm run dev
```

Vite 开发服务器监听 `http://127.0.0.1:5173`，并将 `/api` 和 `/health` 代理到 `http://localhost:8080`。

提交前执行：

```bash
cd frontend
npm run build
```

构建会先运行 TypeScript 检查，再将 `app.js`、`vendor.js`、`icons.js` 和 `index.css` 写入 `web/assets/`。发布包依赖这些预构建文件，因此前端源码和 `web/` 产物需要同步提交。
