# 前端目录结构

> 本项目中前端代码的组织方式。

---

## 概述

前端为 **Next.js 15**、**React 19** 应用，使用 **App Router**。路由位于 `app/` 下，路由组包括 `(main)`（需登录的控制台）和 `(auth)`（登录、回调）。可复用 UI 与按功能划分的组件在 `components/` 下；API 访问与共享工具在 `lib/` 下；hooks 位于 `hooks/`。

---

## 目录布局

```
frontend/
├── app/
│   ├── (main)/                 # 需登录的控制台路由
│   │   ├── layout.tsx          # 主布局（常驻 Dock 栏、路由进度条、自适应容器）
│   │   ├── dashboard/
│   │   ├── hosts/              # 主机列表与详情 [id]
│   │   ├── clusters/           # 集群列表与详情 [id]
│   │   ├── packages/           # 包/安装器管理
│   │   ├── plugins/
│   │   ├── commands/           # 命令日志
│   │   ├── audit-logs/
│   │   ├── diagnostics/        # 诊断与巡检
│   │   ├── monitoring/         # 监控、告警、集成、规则
│   │   ├── admin/users/
│   │   ├── sync/               # 数据同步工作台
│   │   ├── workbench/          # 运行工作台
│   │   ├── e2e-lab/            # 不进入导航的 E2E 挂载页
│   │   └── ...
│   ├── (auth)/                 # 登录、OAuth 回调
│   │   ├── login/
│   │   └── callback/
│   ├── layout.tsx              # 根布局
│   ├── globals.css
│   └── page.tsx                # 入口重定向
├── components/
│   ├── ui/                     # 基础组件（Button、Card、Select、Table、Pagination 等）
│   ├── common/                 # 领域组件
│   │   ├── auth/               # LoginForm、CallbackHandler、LoginBrandPanel
│   │   ├── cluster/            # ClusterMain、ClusterCard、弹窗、配置
│   │   ├── diagnostics/        # 诊断与巡检界面（Inspection、ErrorCenter、TaskCenter）
│   │   ├── host/               # HostTable、HostDetail、HostInstallGuideDialog
│   │   ├── installer/          # PackageMain、InstallWizard、步骤向导
│   │   ├── markdown/           # Markdown 展示
│   │   ├── plugin/             # PluginMain、PluginCard、弹窗
│   │   ├── monitoring/         # MonitoringOverview、告警中心、规则面板
│   │   ├── audit/              # CommandMain、CommandTable、AuditLogMain、AuditLogTable
│   │   ├── config/             # 配置相关 UI
│   │   ├── schedule/           # 调度配置
│   │   ├── sync/               # 数据同步工作台组件
│   │   ├── layout/             # ManagementBar(常驻Dock)、RouteProgressBar、WorkspaceHeader、StatPillsBar、TableLoadingBar、TableSkeletonRows、EmptyState、ThemeProvider
│   │   └── ...
│   ├── animate-ui/             # 动效、radix 变体
│   ├── loading/                # PageLoading
│   └── icons/
├── lib/
│   ├── animations/             # GSAP 声明式微动效库（gsap-motion.ts）
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
│   └── utils.ts
├── hooks/                      # use-host、use-audit 等
└── ...
```

---

## 模块组织

- **页面**：`app/(main)/...` 下每条路由通常有一个 `page.tsx`，渲染**页面级组件**（组合统一的 `WorkspaceHeader` 与对应领域的主组件如 `ClusterMain`、`HostMain`、`PackageMain`）。全局布局由 `(main)/layout.tsx` 提供常驻 Dock 栏；单页严禁额外增加冲突的外层 `container` 或重复的大 padding。
- **功能组件**：在 `components/common/<domain>/` 下（如 `cluster/`、`diagnostics/`、`installer/`、`sync/`）。每个目录可有主列表或详情组件、卡片、弹窗和表格。优先组合 `components/ui/` 中的小组件。
- **Services**：按后端领域在 `lib/services/` 下分目录（如 `cluster/`、`host/`）。每个目录有继承 `BaseService` 的 `*.service.ts`、可选的 `types.ts` 以及统一导出的 `index.ts`。后端基础路径为 `/api/v1`；services 使用相对路径（如 `/hosts`、`/clusters/:id/nodes`）。
- **Hooks**：数据拉取或领域相关 hooks（如 `use-host.ts`、`use-audit.ts`），调用 services 并暴露 loading/error 状态，供页面或 common 组件使用。

---

## 命名约定

- **路由**：目录使用 kebab-case（`audit-logs`、`monitoring`）；动态段为 `[id]`。
- **组件**：PascalCase；功能主入口常命名为 `XxxMain`（如 `ClusterMain`、`PackageMain`）。
- **Services**：类名为 `XxxService`，文件为 `xxx.service.ts`；API 调用为静态方法；`basePath` 与路径与后端路由一致。
- **类型**：放在 service 同目录的 `types.ts` 或 `lib/services/core/types.ts` 中；API 请求/响应优先用 interface。

---

## 示例

- **页面 + 主组件**：`app/(main)/clusters/page.tsx` 渲染 `ClusterMain`；布局来自 `(main)/layout.tsx`。
- **Service**：`lib/services/cluster/cluster.service.ts` 继承 `BaseService`，`basePath = '/clusters'`，定义 `list()`、`getById()`、`create()` 等，返回后端 `ApiResponse<T>` 中的类型化 `data`。
- **UI 约定**：`ClusterMain` 中的筛选栏采用左对齐布局，搜索框使用 `flex-1 min-w-[200px] max-w-sm`（见 [UI 约定](./ui-conventions.md)）。
