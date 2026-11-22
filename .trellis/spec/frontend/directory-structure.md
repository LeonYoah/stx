# 前端目录结构

> 本项目中前端代码的组织方式。

---

## 概述

前端为 **Next.js 15**、**React 19** 应用，使用 **App Router**。路由位于 `app/` 下，路由组包括 `(main)`（需登录的控制台）和 `(auth)`（登录、回调）。UI 组件层以 `components/arc/`（Arc UI 免费注册表）与 `components/common/layout/`（公共布局与分栏原语）为核心，领域业务组件按域组织在 `components/common/<domain>/`；API 访问与共享工具在 `lib/` 下；hooks 位于 `hooks/`。

---

## 目录布局

```
frontend/
├── app/
│   ├── (main)/                 # 需登录的控制台路由
│   │   ├── layout.tsx          # 主布局（常驻 Dock 栏、路由进度条、自适应容器）
│   │   ├── dashboard/
│   │   ├── hosts/              # 主机列表与详情 [id]
│   │   ├── clusters/           # 集群列表、详情 [id] 与三阶段升级 [id]/upgrade/{prepare,config,execute}
│   │   ├── packages/           # 包/安装器管理
│   │   ├── plugins/
│   │   ├── commands/           # 命令日志
│   │   ├── audit-logs/
│   │   ├── diagnostics/        # 诊断与巡检
│   │   ├── monitoring/         # 监控中心（/monitoring 三面板；旧 sub-routes 重定向）
│   │   ├── admin/users/
│   │   ├── sync/               # 数据同步工作台
│   │   ├── workbench/          # 运行工作台
│   │   ├── e2e-lab/            # 不进入导航的 E2E 挂载页
│   │   └── ...
│   ├── (auth)/                 # 登录、OAuth 回调
│   │   ├── login/
│   │   └── callback/
│   ├── layout.tsx              # 根布局（引入 @/components/arc/foundation.css）
│   ├── globals.css             # 全局变量与 .arc-compact 紧凑尺度
│   └── page.tsx                # 入口重定向
├── components/
│   ├── arc/                    # 主力控制台基础控件（Button、Badge、Dialog、Drawer、Tabs、SegmentedControl、SearchField、Select、Combobox、Switch、Checkbox、DropdownMenu、Alert、EmptyState 等）
│   ├── ui/                     # 存量基础组件与 Arc 暂无替代的底座（AlertDialog、Textarea、Pagination、Tooltip 等）
│   ├── common/                 # 领域组件
│   │   ├── ViewToggle.tsx      # 通用纯图标分段视图切换（基于 Arc SegmentedControl）
│   │   ├── auth/               # LoginForm、CallbackHandler、LoginBrandPanel
│   │   ├── cluster/            # 集群列表、速览栏、详情子模块（detail/）、升级流（upgrade/）、向导（wizard/）、扩缩容（scale/）与管理弹窗
│   │   ├── diagnostics/        # 诊断与巡检界面（DiagnosticsWorkspace、errors/、inspections/、policies/）
│   │   ├── host/               # HostMain、HostTable、HostDetail、HostInstallGuideDialog
│   │   ├── installer/          # PackageMain、InstallWizard、步骤向导
│   │   ├── markdown/           # Markdown 展示
│   │   ├── marketplace/        # 市场共享组件（MorphCatalog、MarketplaceViewToggle、SelectionToolbar 等）
│   │   ├── plugin/             # PluginMain、PluginCard、单层画布管理弹窗
│   │   ├── monitoring/         # MonitoringWorkspace（alerts/、rules/、notify/、status-chips）
│   │   ├── audit/              # CommandMain、CommandTable、AuditLogMain、AuditLogTable
│   │   ├── config/             # 配置相关 UI
│   │   ├── schedule/           # 调度配置
│   │   ├── sync/               # 数据同步工作台组件（含 StudioFileTabs、VariableDialogForm、job-detail/）
│   │   ├── layout/             # ManagementBar、WorkspaceHeader、ModuleNavTabs、TableCard、TableLoadingBar、TableSkeletonRows、DetailHost、DirtyCloseGuard、UnsavedChangesDialog、BatchBar、SoftChip、ArcDateRangePicker、CompactTimeFilter、ExpandableTextPanel、ThemeProvider、list-detail.module.css
│   │   └── ...
│   ├── animate-ui/             # 动效、radix 变体
│   ├── motion/                 # beUI motion 控件（File Tree、Table、SharedLayoutBg）
│   ├── loading/                # PageLoading
│   └── icons/
├── lib/
│   ├── animations/             # 表卡共享形变（view-morph.ts）等动效工具
│   ├── services/               # API 服务层
│   │   ├── core/               # api-client、base.service、types
│   │   ├── auth/
│   │   ├── cluster/
│   │   ├── host/
│   │   ├── dashboard/
│   │   ├── audit/
│   │   ├── installer/
│   │   ├── plugin/
│   │   ├── config/
│   │   ├── diagnostics/
│   │   ├── monitor/
│   │   ├── monitoring/
│   │   ├── discovery/
│   │   ├── project/
│   │   ├── st-upgrade/
│   │   ├── sync/
│   │   └── admin/
│   ├── i18n/                   # 语言包（zh、en）、provider、配置
│   ├── ease.ts                 # beUI 弹簧与缓动 token（勿与 Arc motion-tokens 混用）
│   └── utils.ts
├── hooks/                      # 跨页面/领域复用 hooks（如 use-host、use-audit 等）
└── ...
```

---

## 模块组织

- **页面**：`app/(main)/...` 下每条路由通常有一个 `page.tsx`，渲染**页面级组件**（根容器挂 `.arc-compact`，组合 `WorkspaceHeader` 与对应领域主组件如 `ClusterMain`、`HostMain`、`MonitoringWorkspace`、`DiagnosticsWorkspace`）。全局布局由 `(main)/layout.tsx` 提供常驻 Dock 栏；单页严禁额外增加冲突的外层 `max-w-*` 容器或重复的大 padding。
- **功能组件**：在 `components/common/<domain>/` 下（如 `cluster/`、`diagnostics/`、`monitoring/`、`sync/`）。大型详情页按子域拆入子目录（如 `detail/`、`errors/`、`rules/`、`upgrade/`），禁止长期堆成数千行单文件。新开发与重构页面**优先组合 `components/arc/` 与 `components/common/layout/` 原语**；`components/ui/` 仅用于未迁移页面及 `AlertDialog` / `Textarea` / `Pagination` 等互补组件。
- **Arc 控件**：免费注册表组件放在 `components/arc/`，不要写回 `components/ui/`。约定见 [UI 约定](./ui-conventions.md) 第 8 节。
- **beUI motion**：工作台资源树与趋势图使用 `components/motion/` / `components/charts/`（注册表 `@beui`）。不要覆盖 `lib/i18n/`、`lib/utils.ts` 或 `lib/ease.ts`。
- **Services**：按后端领域在 `lib/services/` 下分目录（如 `cluster/`、`host/`）。每个目录有继承 `BaseService` 的 `*.service.ts`、可选的 `types.ts` 以及统一导出的 `index.ts`。后端基础路径为 `/api/v1`；services 使用相对路径（如 `/hosts`、`/clusters/:id/nodes`）。
- **Hooks**：数据拉取或领域状态 hooks（如列表查询、生命周期操作封装），调用 services 并处理请求竞态与 loading/error 状态，供页面或 common 组件复用。

---

## 命名约定

- **路由**：目录使用 kebab-case（`audit-logs`、`monitoring`）；动态段为 `[id]`。
- **组件**：PascalCase；功能主入口常命名为 `XxxMain` / `XxxWorkspace`（如 `ClusterMain`、`MonitoringWorkspace`）。
- **Services**：类名为 `XxxService`，文件为 `xxx.service.ts`；API 调用为静态方法；`basePath` 与后端路由一致。
- **类型**：放在 service 同目录的 `types.ts` 或 `lib/services/core/types.ts` 中；API 请求/响应优先用 interface。

---

## 示例

- **页面 + 主组件**：`app/(main)/clusters/page.tsx` 渲染 `ClusterMain`；布局来自 `(main)/layout.tsx`。
- **Service**：`lib/services/cluster/cluster.service.ts` 继承 `BaseService`，`basePath = '/clusters'`，定义 `getClustersSafe()`、`getClusterSafe()` 等，返回类型化结果。
- **标准列表页骨架**：根容器 `.arc-compact` + `WorkspaceHeader` + `layout.toolbar`（Arc `SegmentedControl` + Arc `SearchField` + `ViewToggle`）+ `TableCard` + `DetailHost`（见 [UI 约定](./ui-conventions.md) 第 4 节）。
