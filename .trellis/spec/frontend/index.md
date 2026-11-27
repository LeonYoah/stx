# 前端开发规范

> 适用于 `frontend/` 下的 Next.js 控制台。本文档与 [UI 约定](./ui-conventions.md)、[目录结构](./directory-structure.md) 构成当前唯一有效的前端设计基线。

---

## 概述

STX 控制台为 **Next.js 15**（App Router）应用，使用 **React 19**、**TypeScript**、**Tailwind CSS v4**，界面体系以 **Arc UI（`.arc-compact` 半胶囊紧凑密度）** + `components/common/layout/` 公共布局原语为核心（`components/ui/` 仅保留给未迁移页面及 `AlertDialog` / `Textarea` / `Pagination`）。

### 核心架构与视觉系统
- **主导航框架**：`app/(main)/layout.tsx` 提供底部常驻 Dock 栏（`ManagementBar`）、顶部轻量路由进度条（`RouteProgressBar`）与主工作区容器查询（`@container/main`），并统一承担底部避让留白（`pb-28`）。
- **标准工作区外壳**：根容器挂 `.arc-compact`，页头统一使用 `WorkspaceHeader`（内聚标题、面包屑、同域 `ModuleNavTabs` 与主操作 / 二级 Arc `TabsList`）。
- **单行工具栏 + 唯一主卡片（`TableCard`）**：状态/短枚举 `SegmentedControl` + Arc `SearchField` + `ViewToggle` 同行排布；数据列表、骨架、顶部静默刷新条、错误重试、定制 `EmptyState` 与底部分页条统一收口在 `TableCard` 内。
- **列表 + 常驻详情分栏（`DetailHost`）**：桌面端 ≥1080px 采用列表 + 右侧常驻栏与 #178 克制联动（零间距、列表退为底层、凹圆角相接、名称飞入、J/K 导航），<1080px 自动退化为贴边 Arc `Drawer`。
- **单层扁平画布 + `0.5px` 细线**：详情页、右侧速览栏、Tab 主面板与管理类弹窗一律压平为单层卡片，去掉内部多余的嵌套 `<Card>`、边框与阴影，通过留白 + `0.5px` 细线（`border-t-[0.5px] border-border/80`）自然分隔各组信息。
- **全局多语言与字体**：支持中英文（`zh-CN` / `en`）单语切换；排版采用 `Inter` + `Noto Sans SC` + `JetBrains Mono`，数值与度量统一 `tabular-nums`。
- **API 通信规范**：统一通过 `lib/services/` 下的领域服务与 `BaseService` 封装，通过共享 `apiClient` 消费 `{ error_msg, data }` 结构。

---

## 规范索引

| 文档 | 说明 | 状态 |
| --- | --- | --- |
| [UI 约定](./ui-conventions.md) | `.arc-compact` 体系、单层画布 `0.5px` 压平、控件与浮层语义、`TableCard` 零塌陷加载、`DetailHost` #178 联动、`ViewToggle` + `useViewMorph`、Arc / beUI 契约 | 已更新 |
| [目录结构](./directory-structure.md) | App Router、`components/arc/` 与 `components/common/layout/` 模块组织、services 与 hooks 约定 | 已更新 |
| [API 与 Services](./api-and-services.md) | API 客户端、BaseService 服务层、执行协议、错误拦截、类型与运行时兜底、作业拓扑契约 | 已填写 |
| [E2E 测试](./e2e-testing.md) | Playwright 目录、夹具策略、功能流样例、CI 门禁过滤规则 | 已填写 |
| [全局部署模式模型](../guides/deployment-mode-model.md) | Standalone / K8s Session / K8s Application / 规划 YARN 术语、集群架构命名、`@/lib/infra` 单一真源与 ESLint 防线 | 已填写 |

---

## 核心设计与交互原则

1. **收敛复杂度，按需披露（设计第一优先级，Decision Reduction First）**
   - 任何面向用户的交互与流程，先在全局视角替用户收敛决策，再呈现界面：**能推断的不询问、能安全默认的不留空、值域封闭的用枚举选择而非自由输入、低频高级选项收进下一层**。
   - 能推断的不询问：系统已持有或可推导的值（上下文已选对象、可继承的地址与配置）自动回填，不重复追问。
   - 能安全默认的不留空：默认值取多数场景的最佳答案且选错代价低；删除、付费、对外暴露、秘钥等无安全默认的决策必须显式选择，不得预勾选。
   - 值域封闭不自由输入：枚举与受控集合一律用选择控件，自由文本只留给开放值域；控件按选项数量选型（详见 [UI 约定](./ui-conventions.md) §3）。
   - 低频高级项不进主干：高级/罕见配置收进折叠区、二级弹窗或向导后续步骤；每个屏幕只保留完成当前这一步所需的最少决策点。
   - 理论依据：渐进式披露（Progressive Disclosure）、识别优于回忆（Recognition over Recall）、智能默认值（Smart Defaults）、希克定律（Hick's Law）。
2. **单层画布 + `0.5px` 细线压平（Flat Single-Canvas Architecture）**
   - 详情页、右侧详情栏、管理类弹窗只保留唯一外层表面；严禁在画布内再嵌套 `<Card>`、`rounded-lg border shadow-xs` 或虚线占位盒。
   - 各信息组以留白（`pb-5` / `py-5`）与 `border-t-[0.5px] border-border/80` 自然分隔；内嵌表格直接落在画布上并用 `[&_tr]:border-b-[0.5px] [&_tr]:border-border/70` 分行。
3. **零塌陷且克制的加载反馈（Zero-Collapse & Single-Indicator Loading）**
   - 列表页统一通过 `TableCard` 实现：**300ms** 延迟显现骨架、显现后至少驻留 **500ms** 防闪烁；刷新/翻页保留旧数据与可交互性，只亮顶部 `TableLoadingBar`（不压暗、不禁点整表）。
   - 独立表单/详情区首次加载用同构骨架屏，严禁居中单个 Spinner 替换整卡；请求失败保留旧数据并在卡片内给出错误原因与重试入口。
4. **语义准确的控件与浮层分工（Semantic Controls & Overlays）**
   - `Switch` 仅用于拨动即刻生效的设置；随表单「保存」提交的布尔项用 `Checkbox`。
   - 2–5 个短选项用 Arc `SegmentedControl`（不塞进 `Select`）；≥6 项或动态增长项用 Arc `Select` / `Combobox`；动作集合用 `DropdownMenu`。
   - 高危/不可逆确认用居中 `AlertDialog`（写清影响对象与具名动作按钮）；编辑表单离开统一走 `DirtyCloseDialog` / `UnsavedChangesDialog` 脏态拦截。
5. **反噪音与高信噪比展示（Anti-Noise Display，原则 1 在展示侧的延伸）**
   - 严禁在列表或卡片顶部堆砌无交互价值的静态长说明；帮助文案收进 `Tooltip`、`HelpHint` 或场景化 `EmptyState`。
   - 禁止在单屏内无脑平铺过多平级 Tab 或重型子表；一级 Tab 控制在合理数量，关联子域用内部 `SegmentedControl` 分段隔离。
   - 禁止仅按当前页数据拼凑与总数矛盾的状态计数。
6. **原子信息完整与数值稳态（Atomic Integrity & Tabular Numerics）**
   - 技术标识符、路径、代码、URL 禁止 `break-all` 硬折；采用单行 `truncate` + Tooltip + 复制，或独立横向滚动容器。
   - 计数、版本号、端口、PID、内存、进度百分比与时间统一启用 `tabular-nums`，刷新时列宽不抖动。
7. **凭证安全保密契约（Masked Secret Credentials）**
   - 敏感凭据（Secret / Password / Token）列表与详情严禁明文回显；编辑框默认留空表示保留原值，防二次展示泄露。

---

## 开发前检查

- **外壳与布局**：页面放在 `app/(main)/...` 下，根容器挂 `.arc-compact`；严禁单页再套自带 `max-w-*` 居中或大 padding 的外层 `container`；新业务组件置于 `components/common/<domain>/`。
- **页面头部与图标**：统一使用 `<WorkspaceHeader />`，图标与 Dock 对齐；换/加图标前先在 `lucide-react`、`@tabler/icons-react` 按实际尺寸列出至少 3 个候选并经确认（详见 [UI 约定](./ui-conventions.md) 第 1 节）。
- **列表页标准**：单行工具栏（Arc `SegmentedControl` + Arc `SearchField`，无冗余「搜索」按钮）+ `TableCard` + 底部分页；支持 URL 参数同步、筛选重置 `page=1` 与请求序号竞态保护；表/卡切换用 `ViewToggle` + `useViewMorph`；常驻详情用 `DetailHost`；多选批量条用底部浮动 `BatchBar`。
- **详情与弹窗标准**：单层画布 + `0.5px` 细线压平；Arc `Dialog` / `Drawer` Portal 内容节点必挂 `.arc-compact`；编辑态离开接入 `DirtyCloseDialog`。
- **接口与服务**：仅通过 `lib/services/` 调用后端；数组字段在边界做好 `null` 兜底（`?? []`）。Dolphin 工作流的 `tasks`/`relations`/`recent_jobs` 同样必须归一成数组，成环预检不能用假边。
- **CLI 令牌管理**：仅管理员可访问；与用户管理同页双 Tab。签发时展示原文一次，列表只显示前缀加掩码。有效期预设为 7 天 / 30 天 / 90 天 / 永久，并允许自定义 1–3650 天。审计日志必须展示令牌列（名称或前缀；Web 会话显示「会话」）。Dolphin Worker 的 `STX_TOKEN` 使用这里签发的 `stx_` 登录令牌，不再单独签发调度令牌。
- **推送不绑定 STX 登录令牌**：任务侧栏和推送弹窗只负责把 SHELL 节点推到 Dolphin，不得放「打开令牌管理」或任何签发/绑定 `stx_` 的入口。Worker 令牌由用户在令牌管理页签发后，自行 `stx login --token` 或写入 Worker 环境。
- **推送绑定已发布版本**：未发布或已推送版本没有更新时，推送预演/确认按钮必须禁用并说明原因。作业列表要把 Dolphin 触发（`trigger_source=dolphin` / `external_ref=ds:...`）与内置定时、手动运行区分开，并展示任务版本。
- **Dolphin Open API Token**：推送弹窗必须把该字段做成带标签的密码必填项（请求头 `token`），无连接时直接展开新建表单；不得只靠折叠面板里的 placeholder。它与 Worker 上的 `STX_TOKEN`（管理员签发的 `stx_` 登录令牌）不是同一张令牌。
- **作业告警**：任务设置侧栏的「告警」分组未配置时只显示一行和「配置」按钮，编辑放在弹窗里；表单 `TaskAlertForm`（Arc 控件）与监控中心「规则 → 作业级」抽屉共用，保存走 `services.sync.updateTaskAlert`（不跟随任务版本，不要放进任务定义或发布流程）。告警通知和告警中心的作业链接是 `/sync?task=<id>&job=<id>`，`/sync` 页面必须把这两个参数带到 `/workbench`，工作台据此打开任务并选中作业（不在最近 50 条时单独读取）。后端契约见 [同步作业告警](../backend/sync-alerts.md)。
- **作业详情与拓扑**：运行记录行点击即打开作业详情，行内只留详情 / 恢复 / 保存点停止 / 停止，操作可用性只用 `getJobActionState`；拓扑（React Flow + dagre）只能在 `job-detail/JobTopology` 的动态 chunk 中加载，布局按结构签名缓存。契约见 [作业详情与 Flink 风格拓扑](./api-and-services.md#scenario-作业详情与-flink-风格拓扑)，后端见 [作业计划 DAG](../backend/job-plan-dag.md)。
- **E2E 覆盖**：新增或修改核心流程、多步骤向导、增删改动作时同步补充 Playwright 测试或夹具参考（参见 [E2E 测试](./e2e-testing.md)）。

---

## 构建与检查

- 需要生成**可部署前端产物**时，统一使用 `cd frontend && pnpm run pack:standalone`。
  - 项目的 Docker、CI 和 PM2 发布链路依赖 `dist-standalone/`，不只依赖 `.next`。
  - `pnpm build` 仅用于排查 Next.js 原始构建，不作为默认交付命令。
- 需要**本地重启 / 发布前后端服务**时，优先使用仓库根目录 `./scripts/restart.sh`。
  - 该脚本已包含后端构建、前端构建、standalone 组装、PM2 重启与保存。
  - 执行前不要再额外运行 `pnpm run pack:standalone`，除非正在单独排查 standalone 构建。
- TypeScript 类型检查默认复用增量缓存：`frontend/tsconfig.json` 已启用 `"incremental": true`，常规检查运行 `cd frontend && pnpm exec tsc --noEmit`。
- 单元测试运行 `cd frontend && pnpm test`；改动语言包后运行 `cd frontend && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts`；涉及用户操作流程时运行对应 Playwright 用例。

---

## 注释约定

- 注释只写代码表达不了的 why（约束、坑、业务规则来源），不复述代码逻辑、不记录改动过程、不写面向用户的解释。
- **自己新增或修改的前后端代码注释，要求中英双语，中文在前，英文在后。** 两种语言表达同一语义，不要为了凑双语去注释显而易见的代码。

---

## 中英文切换约定

- **面向用户的前端界面全部支持中英文切换。**
- **「双语支持」指「可切换的单语言展示」**，严禁同屏同时展示 `中文 / English` 或把两种语言拼在一条文案里。

---

**语言**：本目录下所有文档均使用**中文**。
