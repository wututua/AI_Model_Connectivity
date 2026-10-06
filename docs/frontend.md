# 前端开发

[项目首页](../README.md) · [文档索引](README.md) · [开发指南](development.md) · [前端集成](backend-api.md)

## 目录

- [技术栈](#技术栈)
- [本地开发](#本地开发)
- [目录结构](#目录结构)
- [页面与路由](#页面与路由)
- [状态与交互](#状态与交互)
- [组件与主题](#组件与主题)
- [字体与静态资源](#字体与静态资源)
- [构建](#构建)
- [浏览器回归](#浏览器回归)

## 技术栈

| 层 | 选择 |
| --- | --- |
| 框架 | React 18、TypeScript、Vite 7 |
| 路由 | React Router 7，`createBrowserRouter` |
| 组件 | shadcn/ui 源码组件、Radix UI |
| 样式 | Tailwind CSS 4、HSL CSS 变量 |
| 图标 | Lucide React |
| 测试 | Node.js test runner、Playwright |

准确依赖版本见 [package.json](../frontend/package.json) 与锁文件。shadcn/ui 采用源码组件模式，组件保存在仓库内，并非独立的黑盒组件包。

## 本地开发

从仓库根目录打开两个终端。后端：

```bash
go run ./cmd/cg
```

前端：

```bash
npm ci --prefix frontend
npm run dev --prefix frontend -- --host 127.0.0.1
```

访问 Vite 输出的地址，默认 `http://127.0.0.1:5173`；端口被占用时以终端输出为准。`/api` 和 `/health` 代理到 `http://localhost:8080`，不要在组件中硬编码后端地址。

## 目录结构

```text
frontend/
  components.json          shadcn/ui 生成配置
  public/fonts/            原始字体、许可与声明
  tests/                   单元与浏览器回归
  src/
    App.tsx                路由
    api.ts                 请求与错误处理
    types.ts               前端数据类型
    index.css              主题、样式与动画
    fonts.css              字体声明
    hooks/
      useAuth.tsx          会话与授权
      useTheme.ts          主题偏好
    utils/
      liveStatus.ts        SSE 与请求生命周期
      status.ts            状态与时间展示
      models.ts            模型解析与去重
      usageChart.ts        用量图表计算
    components/
      ui/                  基础控件
      ModelPicker.tsx      模型选择
      ProviderCard.tsx     Provider 展示
      ModelRow.tsx         模型结果
      UsageTrend.tsx       用量趋势
    pages/
      Dashboard.tsx        状态监控
      Login.tsx            登录与首次改密
      Admin.tsx            后台工作台
      admin/               后台各页面
```

TypeScript 与 Vite 均使用 `@/*` 映射到 `src/*`。复用现有组件、工具和别名，不另建一套基础组件目录。

## 页面与路由

| 路径 | 内容 |
| --- | --- |
| `/` | 状态监控，是否公开由配置决定 |
| `/login` | 登录与首次改密，按 `next` 返回 |
| `/admin/overview` | 实时进度、检测控制、失败项重测、请求预算与输出预算参考 |
| `/admin/providers` | 分组/标签筛选、批量管理、复制、探测参数与模型重测 |
| `/admin/settings` | 检测、历史、调度、通知与访问控制 |
| `/admin/tasks` | 任务筛选、详情与分页 |
| `/admin/notifications` | 管理员通知历史、测试与手动重试 |
| `/admin/billing` | Token 汇总、趋势与模型明细 |
| `/admin/config` | JSON 导入导出 |
| `/admin/operations` | 管理员 CSV / 诊断导出与指标凭据管理 |
| `/admin/users` | 用户、角色、禁用与密码重置 |
| `/admin/account` | 当前账号改密 |

后台路径支持深链。普通用户不显示共享数据写操作和管理员栏目，后端同时执行授权检查。会话、CSRF 和初始改密流程见[前端集成](backend-api.md#会话生命周期)。

## 状态与交互

### 状态监控

首页通过统一控制器加载 `/api/status`、订阅 SSE 并在断线时退避轮询。每 30 秒更新相对时间，按 `stale_after_seconds` 标记过期；旧版缺失或无时区的时间视为待更新。

- 支持 Provider、ID 和模型搜索，以及状态筛选、排序。
- Provider 分组可折叠，详细与紧凑视图共享结果；详情展开时才挂载，收起动画结束后释放。
- 展示延迟、成功率、历史、P50/P95/P99、样本数、曲线与允许公开的错误。
- 首次安装、等待检测、无 Provider、无搜索结果和请求失败分别处理，未检测不能冒充服务已故障。
- 视图和排序偏好可持久化，搜索与筛选不持久化。

### 表单与模型

Provider 的 API Key 留空保留原值，显式清除才删除；更换 Base URL 时需重新填写或清除 Key。通知凭据采用相同的保留/清除语义。

模型选择支持标签、搜索勾选、批量粘贴和同步。同步不保存配置、不调用聊天接口，失败不丢弃已选模型；保存时合并未添加的手动输入。清空全部模型恢复检测时自动发现的语义，需保留成本提示。

设置页采用分组表单与吸顶保存栏，待提交条目参与保存及离开确认。保存中锁定表单；栏目切换、浏览器导航和关闭页面分别处理未保存编辑。临时会话刷新失败保留挂载中的表单，认证失效或角色变化仍执行访问控制，密钥草稿不进入浏览器存储。

### 任务与请求顺序

概览跟踪已接受或首次加载发现的任务。任务终态后继续重试报告刷新，直到成功再结束跟踪；切页不取消后台检测。

监控首页、概览、任务列表和用量范围查询均需忽略迟到响应。不要依赖报告时间排序，也不要让旧的整体刷新覆盖刚完成的任务摘要。详细约定见[请求顺序](backend-api.md#请求顺序)。

### 用量与移动端

Provider 探测配置由 `ProbeFields` 管理，预设仅填充可编辑字段，不进行模型能力推断。`ModelCheckDialog` 发送明确 Provider/模型组合；复制不读取或复用已有密钥。批量操作通过单个事务 API 提交，不能用多个独立请求模拟原子操作。

检测进度只读取 detection 的 `progress`，不能把活动模型当成正式结果。`OperationsTab` 的凭据原文只留在当前内存，切页不持久化；导出使用同源 Cookie，非成功响应必须作为错误处理，不能下载成 CSV。

趋势图支持鼠标、触摸与键盘选择日期（方向键、Home、End），单点和空数据单独处理。模型明细在桌面使用表格，手机使用紧凑列表；Provider 移动端操作集中在操作对话框，模型标签改为单列。

### 通知记录

管理员可查看分页发送记录、按状态筛选并发送测试通知；失败和结果未知的记录经确认后可重发历史摘要。成功显示为“平台已接受”，不表示最终用户送达。平台拒绝的记录可能仍有 HTTP 200，必须以记录 `status` 判断结果。

测试只使用已保存配置。列表请求取消并忽略迟到响应，进行中的记录自动刷新，写操作防止重复点击，切页后不更新已卸载页面。桌面使用表格，手机使用纵向列表；普通用户既不显示入口，也无后端访问权限。协议见[通知 API](api.md#通知记录)。

## 组件与主题

基础控件复用 `components/ui/` 中的 Button、Input、Dialog、Select、Switch、Tabs、Table 等。Radix UI 提供焦点管理、键盘交互和无障碍语义，业务组件不应复制基础控件样式。

`index.css` 定义 HSL 语义令牌：`background`、`foreground`、`primary`、`muted`、`destructive`、`success`、`warning` 等。主操作使用蓝色，健康、较慢、异常分别为绿、琥珀和红，未检测为中性灰；不要用品牌色替代状态语义。

`body[data-theme]` 控制深浅色。`useTheme` 接受 `dark` / `light` / `auto`，由 `ThemeToggle` 切换；`auto` 跟随浏览器 `prefers-color-scheme`，不使用服务端报告的昼夜时间表。

主题存于 `localStorage.theme`，存储不可用时回退默认深色，并在当前页面生命周期保留选择。HTML 启动脚本与 React 使用一致的容错规则。

动画以 CSS 实现，尊重 `prefers-reduced-motion: reduce`，此时跳过主题 View Transition 并缩短动画。折叠内容设为 `inert`，避免键盘焦点进入隐藏区域。

如需新增 shadcn/ui 组件，在 `frontend/` 目录生成后审查源码、依赖和锁文件，保留 [components.json](../frontend/components.json) 中的 `new-york`、CSS 变量模式与别名约定。

## 字体与静态资源

页面使用本地 HarmonyOS Sans SC。Regular 对应 400–500，Bold 对应 600–900；Medium 原文件保留但不额外请求。数字沿用组件的 `tabular-nums`。

- `font-display: swap` 保证加载时可阅读，HTML 预加载常规字重。
- 字体 URL 包含文件 SHA-256，后端验证后提供一年不可变缓存与 ETag。
- HTML 和固定名称的脚本、样式在复用前重新验证。
- 构建生成 Regular/Bold 的内容寻址 gzip 副本，解压字节与原文件一致，不裁剪或转换字体。
- Go 根据 `Accept-Encoding` 返回压缩副本及对应 ETag，范围请求回退原文件；没有副本时仍能使用原字体。

原文件与独立许可位于 `frontend/public/fonts/harmonyos-sans/`，构建时复制到 `web/fonts/`。字体不属于项目 MIT 授权，必须保留[来源声明](../frontend/public/fonts/harmonyos-sans/NOTICE.md)与许可证。

## 构建

```bash
npm test --prefix frontend
npm run build --prefix frontend
```

构建先执行 `tsc -b`，再由 Vite 将 `app.js`、`vendor.js`、`icons.js` 和 `index.css` 写入 `web/assets/`。Tailwind 4 通过 `@tailwindcss/postcss` 编译。

发布包与后端依赖预构建 `web/`，前端源码与产物需要同步提交。不要把本地数据库、凭据或测试截图放入静态目录。

## 浏览器回归

浏览器测试直接读取构建后的 `web/` 并模拟 API，不启动真实检测、不读写业务数据库。先构建，再使用本机 Chrome：

```bash
npm run build --prefix frontend
npm run test:browser --prefix frontend
```

使用 Playwright Chromium 时，先在 `frontend/` 安装浏览器：

```bash
cd frontend
npx playwright install chromium
```

然后从仓库根目录运行。Bash：

```bash
PLAYWRIGHT_CHANNEL=chromium npm run test:browser --prefix frontend
```

PowerShell：

```powershell
$env:PLAYWRIGHT_CHANNEL = 'chromium'
npm run test:browser --prefix frontend
```

截图默认保存在临时目录，可通过 `BROWSER_ARTIFACT_DIR` 指定。CI 执行同一组测试，覆盖会话恢复、表单、导航拦截、请求竞态、任务完成刷新、移动端操作、状态文案、按需挂载、字体请求及主题存储异常。
