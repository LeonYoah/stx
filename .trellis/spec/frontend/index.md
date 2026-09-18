# 前端开发规范

> 适用于 `frontend/` 下的 Next.js 控制台。

---

## 概述

本目录包含前端开发相关规范。STX 控制台为 **Next.js 15**（App Router）全栈控制台应用，使用 **React 19**、**TypeScript**、**Tailwind CSS v4** 及 **Radix UI / shadcn/ui** 风格组件体系。

### 核心架构与视觉系统
- **主导航框架**：macOS 风格底部常驻 Dock 栏（`ManagementBar`），配合轻量无阻塞路由进度条（`RouteProgressBar`）与主工作区容器查询（`@container/main`）。
- **标准工作区结构**：统一使用标准化 `WorkspaceHeader`（内聚标题、描述与主操作），搭配 `StatPillsBar` 状态聚合统计与一键快速过滤。
- **一体化表格与分页**：搜索筛选、状态标签、数据列表、底部总计与分页器整合于统一 `Card` 容器内。
- **全局多语言与字体**：内置中英文（`zh-CN` / `en`）即时切换；排版采用 `Inter`（西文）+ `Noto Sans SC`（中文）+ `JetBrains Mono`（代码）。
- **API 通信规范**：统一通过 `lib/services/` 下的领域服务与 `BaseService` 封装，通过共享 `apiClient` 消费 `{ error_msg, data }` 结构。

---

## 规范索引

| 文档 | 说明 | 状态 |
| --- | --- | --- |
| [UI 约定](./ui-conventions.md) | Dock 导航、WorkspaceHeader、StatPillsBar、零塌陷加载、表格与分页卡片、弹窗与引导、凭证保密规范 | 已填写 |
| [目录结构](./directory-structure.md) | App Router、Dock 架构、公共 UI 模块化、lib 与 hooks 组织 | 已填写 |
| [API 与 Services](./api-and-services.md) | API 客户端、BaseService 服务层、错误拦截、类型与运行时兜底 | 已填写 |
| [E2E 测试](./e2e-testing.md) | Playwright 目录、夹具策略、功能流样例、CI 门禁过滤规则 | 已填写 |

---

## 核心设计与交互原则

1. **零塌陷视觉加载（Zero-Collapse Visual Loading Architecture）**
   - 首次加载使用骨架屏行（`TableSkeletonRows`），静默刷新使用表格顶置进度条（`TableLoadingBar`）配合数据行半透明（`opacity-60`）。
   - 严禁在表格卡片内使用全白屏替换的大 Spinner，避免高度塌陷和页面剧烈跳动。
2. **反噪音与高信息密度（Anti-Noise & Actionable UI）**
   - 严禁在列表/表格卡片顶部放置大段无交互价值的静态长说明；操作指导一律内收至 Tooltip、空状态引导或操作向导中。
   - 对需要后续操作的新增动作（如添加主机），提供一键复制安装命令与实时心跳感知的引导弹窗。
3. **一体化卡片容器（Integrated Table Card）**
   - 搜索栏、筛选条件、数据表格、底部统计条与紧凑分页器统一定义在同一个 Card 内，严禁外部悬浮孤立分页条。
4. **凭证安全保密契约（Masked Secret Credentials）**
   - 敏感信息（如全局变量中的 Secret/密码类型）严禁明文回显；列表一律掩码显示，编辑弹窗默认留空（不修改保持原密码），防二次展示泄露。
5. **规范化弹窗层级（Framed & Scalable Dialogs）**
   - 弹窗必须具备明显的层级边框（`border border-border/80 dark:border-border/60 shadow-2xl`）；根据内容选择宽度规格（`sm` / `md` / `lg` / `max-w-4xl~5xl`），复杂内容由内部容器独立滚动。

---

## 开发前检查

- **布局与导航**：页面放在 `app/(main)/...` 下；底部留出 `pb-28` 避免被 Dock 栏遮挡；新业务组件置于 `components/common/<domain>/`。
- **页面头部**：统一使用 `<WorkspaceHeader />`，配置语义化图标、标题、说明文案及主操作区，严禁手写样式不一致的 flex 标题栏。
- **状态统计与筛选**：列表页存在按状态分类（如 全部 / 在线 / 离线 / 警告）时，优先采用 `<StatPillsBar />` 提供统揽和联动。
- **加载态平滑度**：严格执行“零塌陷加载规范”，配备 `TableSkeletonRows` 和 `TableLoadingBar`。
- **分页设计**：分页组件放置于表格卡片底部 footer，包含页码指示、跳页及每页条数切换器。
- **弹窗尺寸与边框**：弹窗宽度契合内容复杂度，配置层级边框与内滚动容器。
- **安全性**：涉及密码/密钥的输入与编辑严格执行掩码和留空保密机制。
- **接口与服务**：仅通过 `lib/services/` 调用后端；确保数组类型做好 `null` 兜底（`?? []`）。
- **E2E 覆盖**：新增或修改功能代码时，涉及核心流程、多步骤向导、核心增删改动作需同步补充 Playwright 测试或夹具参考（参见 [E2E 测试](./e2e-testing.md)）。

---

## 构建与检查

- 需要生成**可部署前端产物**时，统一使用 `cd frontend && pnpm run pack:standalone`。
  - 项目的 Docker、CI 和 PM2 发布链路依赖 `dist-standalone/`，不只依赖 `.next`。
  - `pnpm build` 仅用于排查 Next.js 原始构建，不作为默认交付命令。
- 需要**本地重启 / 发布前后端服务**时，优先使用仓库根目录 `./scripts/restart.sh`。
  - 该脚本已包含后端构建、前端构建、standalone 组装、PM2 重启与保存。
  - 执行前不要再额外运行 `pnpm run pack:standalone`，除非正在单独排查 standalone 构建。
- TypeScript 类型检查默认可复用增量缓存：`frontend/tsconfig.json` 已启用 `"incremental": true`。
  - 常规检查运行 `cd frontend && pnpm exec tsc --noEmit`。
- 单元测试运行 `cd frontend && pnpm test`；涉及用户操作流程时，再运行对应 Playwright 用例。

---

## 注释约定

- **自己新增或修改的前端代码注释，要求中英双语。** 适用于组件说明、复杂交互说明、状态管理说明、边界条件注释等。
- **默认顺序为“中文在前，英文在后”。** 两种语言应表达同一语义，不要写成两套不一致的说明。
- **第三方 / 生成 / 历史镜像代码不强制回填双语。** 但只要本次新增了注释，就应按双语写法补齐。
- **仍然遵循“少而准”的原则。** 不要为了满足双语要求去逐行翻译显而易见的代码。

---

## 中英文切换约定

- **面向用户的前端界面仍要求支持中英文切换。** 例如 diagnostics、巡检中心、诊断报告、任务中心等模块都应能随当前语言切换展示。
- **这里的“双语支持”指“可切换的单语言展示”**，不是同屏同时展示 `中文 / English`，也不是把两种语言拼在一条文案里。

---

**语言**：本目录下所有文档均使用**中文**。
