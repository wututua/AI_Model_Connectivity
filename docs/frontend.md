# 前端说明

> [项目主页](../README.md) · [文档索引](README.md) · [GitHub 仓库](https://github.com/wututua/AI_Model_Connectivity)

## 技术栈

| 项 | 选择 |
|---|---|
| 框架 | React 18 + TypeScript + Vite 7 + React Router 7 |
| 路由 | React Router 7（`createBrowserRouter`，支持未保存更改拦截） |
| 组件 | shadcn/ui 源码组件 + Radix UI 基础件 |
| 样式 | Tailwind CSS 4 + HSL CSS 变量主题 |
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
    useAuth.tsx              账号会话、角色、登录状态刷新
  utils/
    status.ts                状态、时间和样式映射
    liveStatus.ts            SSE 重连与退避轮询生命周期
    usageChart.ts            Token 趋势坐标、刻度与日期格式
    models.ts                模型 ID 解析与有序去重
  components/
    ui/                      shadcn/ui 基础组件
    ThemeToggle.tsx
    SummaryCard.tsx
    ProviderCard.tsx
    ModelRow.tsx
    ModelPicker.tsx           模型标签、多选搜索、同步与手动添加
    CurveChart.tsx
    UsageTrend.tsx
    StatusLights.tsx
    StatusPill.tsx
  pages/
    Dashboard.tsx            公开状态页
    Admin.tsx                认证与后台工作台
    Login.tsx                登录与首次修改密码
    admin/
      OverviewTab.tsx
      ProvidersTab.tsx
      SettingsTab.tsx
      TasksTab.tsx
      BillingTab.tsx
      ConfigTab.tsx
      UsersTab.tsx
      shared.tsx
```

## 公开状态页

`/` 是面向日常监控的状态运营台：

- 首次通过 `GET /api/status` 加载，随后订阅 `/api/events` SSE；连接不可用时按 30、60、120 秒指数退避轮询。
- 显示整体状态、数据时效、实时连接、检测成功率、模型和 Provider 指标；成功包含正常和较慢响应，无样本显示 N/A。
- 支持 Provider、ID 和模型搜索，按状态筛选，按状态、名称、延迟或模型数排序。
- Provider 使用单列分组列表，可独立折叠；详细视图默认显示延迟、成功率和历史状态，紧凑视图收起历史状态。
- 两种视图均可展开查看 P50/P95/P99、样本数、延迟曲线、响应和允许公开的错误信息。
- 模型详情首次展开时挂载，收起动画完成后释放；排序和视图偏好保存在本地，搜索与状态筛选不持久化。
- 总览文案区分未检测、模型异常、Provider 请求失败和较慢响应，未检测不会单独触发“服务异常”标题。
- Provider 级请求错误、首次加载、无报告、无 Provider 和无搜索结果均有独立状态。

页面每 30 秒刷新相对时间，使用报告中的 `stale_after_seconds` 判定过期；旧版无时区或缺失时间记录视为待更新。SSE 断线后继续尝试重连，同时退避轮询；收到有效推送后停止轮询，离开页面清理连接和定时器。

## 管理后台

后台使用可深链路径：

| 路径 | 功能 |
|---|---|
| `/admin/overview` | 运行状态、触发检测、最近结果和 Token 估算 |
| `/admin/providers` | Provider 搜索、新增、编辑、删除和单独重测 |
| `/admin/settings` | 检测、历史、调度、通知和访问控制 |
| `/admin/tasks` | 任务状态筛选、结果明细和分页 |
| `/admin/billing` | Token 汇总、每日趋势和模型明细 |
| `/admin/config` | JSON 配置导入导出 |
| `/admin/users` | 管理员创建、编辑、禁用、删除用户和重置密码 |
| `/admin/account` | 修改自己的密码 |
| `/login` | 共用登录页，按 next 返回目标页面 |

登录凭据改为后端设置的 HttpOnly Cookie，不再存于 localStorage；旧 `cg_admin_token` 会被清除。CSRF 值仅保存在内存，写请求通过 `X-CSRF-Token` 携带。`AuthProvider` 首次及每 30 秒、窗口重新聚焦时查询 `/api/auth/session`；401 清理身份并重新验证，403 的 `password_change_required` 引导改密。普通用户隐藏设置、配置、用户管理和共享数据写操作，仅能修改自己的密码。

开启 `status_login_required` 后，公开状态页未登录访问跳转 `/login?next=/`，登录后返回状态页。后端状态 API 和 SSE 同样检查会话，`auth-required` 事件使前端立即卸载状态列表并重新验证。

Provider 表单中 API Key 留空表示保留现有值；只有开启“清除现有 API Key”才会删除。设置页中的通知凭据采用相同语义。

模型列表使用双列可移除标签（手机为单列），支持候选模型搜索/勾选、数量显示、清空、手动输入与多行/逗号分隔粘贴。点击“同步模型”调用 `/api/admin/provider-models`，只读取上游列表并与当前选择合并去重；失败不影响已有选择。变更连接参数或关闭弹窗取消同步请求，保存期间锁定表单。同步不调用聊天接口、不保存配置；点击保存才提交 `models` 数组，空数组沿用自动发现全部模型的语义。

设置页使用分组表单和吸顶保存栏，显示未保存状态；保存中锁定表单，避免响应覆盖新编辑的内容。修改后刷新或关闭页面会触发浏览器离开提醒。

Token 趋势支持鼠标、触摸和键盘选择日期（方向键、Home、End），显示对应的精确用量；单点和空数据有独立呈现。快速切换时间范围时忽略旧请求结果。模型明细在桌面端使用表格，在手机端使用紧凑列表。

## 主题与组件

页面字体统一为本地托管的 HarmonyOS Sans（SC 中文版），覆盖正文、表单、模型名称与数字。`src/fonts.css` 使用 Regular 承担 400–500、Bold 承担 600–900，减少一份约 8.2 MB 的 Medium 字体请求；原始文件不作裁剪或转换。`font-display: swap` 保证字体加载期间可正常阅读；`index.html` 预加载常规字重。字体 URL 带文件 SHA-256 版本，Go 静态服务核验版本后提供一年不可变缓存和 ETag；HTML 与固定名称的脚本、样式每次复用前重新验证。原始字体与独立许可保存在 `public/fonts/harmonyos-sans/`，构建时复制到 `web/fonts/`，不再请求 Google Fonts。Tailwind 的 `sans` 与既有 `mono` 字体入口统一指向 HarmonyOS Sans，数字仍按原组件保留 `tabular-nums`。

系统设置使用标签式列表输入，支持分隔符、批量粘贴和手动删除。未提交条目同样参与保存和离开确认；切换栏目或浏览器前进后退会提示未保存更改，刷新或关闭页面使用浏览器原生确认。后台会话刷新短暂失败时保留挂载中的表单，仅显示重试提示；认证失效或权限变化仍执行原有访问控制，不把密码或密钥草稿写入浏览器存储。任务历史会取消过期请求并忽略迟到结果。Provider 在手机上显示为纵向列表，操作集中在“更多操作”对话框。

### 浏览器回归

在 `frontend` 下运行 `npm run build` 后，执行 `npm run test:browser`。默认使用本机 Chrome，也可先执行 `npx playwright install chromium`，再设置 `PLAYWRIGHT_CHANNEL=chromium`。测试直接读取 `web/` 构建产物，拦截所有网络与 API 请求，不启动真实检测、不读写业务数据库。截图默认保存到临时目录，可用 `BROWSER_ARTIFACT_DIR` 指定输出位置。CI 同样执行这组测试，覆盖设置编辑、会话恢复、导航拦截、请求竞态、移动端操作、状态文案、详情按需挂载、字体请求与视图偏好，以及切页后任务快速完成、完成后报告请求失败重试。

`frontend/src/index.css` 定义 shadcn/ui 兼容的 HSL 颜色令牌，包括 `background`、`foreground`、`card`、`primary`、`muted`、`destructive`、`success` 和 `warning`。界面以中性灰为基础，浅色主题使用高对比度蓝色主色；健康、较慢、异常分别使用绿色、琥珀色、红色，未检测使用中性灰色，不要用主色替代状态语义。

`body[data-theme]` 控制深浅色，`useTheme` 在 `dark`、`light`、`auto` 之间循环并写入 `localStorage.theme`。`auto` 跟随浏览器 `prefers-color-scheme`，主题切换按钮由 `ThemeToggle` 统一提供。

入场、分组与详情展开、指标更新和图表揭示采用 CSS 动画，不依赖额外动画框架。`prefers-reduced-motion: reduce` 下取消延迟并将动画、过渡缩至近乎即时，同时跳过主题 View Transition。折叠的 Provider 设为 `inert`，避免键盘焦点进入隐藏内容。

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
npm test
npm run build
```

构建会先运行 TypeScript 检查，再将 `app.js`、`vendor.js`、`icons.js` 和 `index.css` 写入 `web/assets/`。Tailwind 4 通过 `@tailwindcss/postcss` 编译，显式加载已有主题配置。发布包依赖这些预构建文件，因此前端源码和 `web/` 产物需要同步提交。

构建还为 Regular/Bold 生成内容寻址的 gzip 副本；Go 根据 `Accept-Encoding` 协商，提供 `Vary` 与分离的 ETag，范围请求回退原文件。解压字节与原始字体相同，没有裁剪或格式转换。未运行新构建时可安全回退原字体。

首次安装的 `unconfigured/pending` 展示中性空态，不显示请求失败。后台检测返回任务 ID 后轮询运行状态和终态，任务历史中的运行记录自动刷新；接受请求不等于检测完成，切页不会取消任务。

运行概览首次加载时即跟踪正在执行的任务，因此任务在下一次轮询前结束也能显示终态。任务完成后若报告请求暂时失败，保留任务 ID 并继续重试；只有报告刷新成功才结束跟踪，迟到的旧任务响应不会清除新任务的跟踪状态。

监控页的首次请求、轮询及手动刷新共用 `startStatusUpdates` 控制器。新的 SSE 或请求会使更早的请求失效（包括错误响应），不依赖 `generated_at` 排序，因此同一检测时间的配置变更也不会被旧响应覆盖。

主题只接受 `dark/light/auto`。HTML 启动脚本与 React 均容忍本地存储访问异常，默认深色；存储不可用时，用户选择在页面内导航期间仍保留，`auto` 继续响应系统主题变化。`npm run test:browser` 包含这些异常及刷新竞态回归。
