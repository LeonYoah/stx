# 设计：全局 UI 换新与统一管理控制台框架

> 状态：设计草案（Trellis design start）  
> 关联参考：`11-17-unified-job-table`（作业列表统一）、PR #160（预览表接入 beUI Motion Table 的边界与 vendoring 约定）  
> 规范：`.trellis/spec/frontend/ui-conventions.md`、`.trellis/spec/frontend/directory-structure.md`、`.trellis/spec/guides/cross-layer-thinking-guide.md`

## 1. 结论（可行性与边界）

**结论：可以统一，但应统一“列表框架与交互原语”，而非统一为同一个领域表格组件。**

- 告警中心、错误巡检/错误中心、审计、用户中心均为 **服务端驱动（server-driven）** 的管理列表：筛选/分页/（部分）排序由后端或受控请求参数决定，适合共享同一套 **List Shell + Toolbar + Pagination + State** 骨架。
- “工作台任务列表 / 控制台作业列表”已统一的核心资产是 `frontend/components/common/sync/job-table/`（领域：作业实例）。其 **数据契约、列语义、轮询/overlay/bulk** 不应泄漏到告警/审计/用户模块；应抽取其中 **通用模式**（受控 query、统计胶囊、集成卡片表、零塌陷 loading、服务端分页、行操作区、详情宿主分层）作为跨域模板。
- **Tabs / 标签 / 下拉 / 弹窗(操作按钮) / 抽屉** 可以也应该全局统一选型与使用规范，但需区分“页面级导航 Tab”“内容筛选 Tab/Segment”“状态标签”“行内动作菜单”“确认/表单弹窗”“详情抽屉”的层级，避免一个组件承载所有语义。
- 全局大换新不等于全量重写：以“扩展已有组件/壳”为主，保留业务逻辑与数据契约；逐步迁移并允许并存期。

## 2. 全局页面清单（当前 `frontend/app/(main)`）

| 路由 | 页面 | 级别 | 说明 |
|---|---|---|---|
| `/dashboard` | 仪表盘 | P1 | 指标/概览，组件对齐 |
| `/workbench` | 工作台 | P0 | 任务/作业列表与详情链路 |
| `/sync` | 数据同步 | P0 | 控制台作业列表与详情 |
| `/monitoring` | 监控中心 | P0/P1 | 告警中心 P0；其他 P1 |
| `/monitoring/rules` | 监控规则 | P1 | 规则列表/编辑 |
| `/monitoring/history` | 历史 | P1 | 历史记录 |
| `/monitoring/integrations` | 集成 | P1 | 集成配置 |
| `/diagnostics` | 诊断中心 | P0 | 错误中心/巡检中心 |
| `/diagnostics/inspections/[id]` | 巡检报告 | P0 | 报告详情 |
| `/clusters` | 集群管理 | P1 | 列表/详情 |
| `/clusters/[id]` | 集群详情 | P1 | 详情/升级入口 |
| `/clusters/[id]/upgrade/*` | 升级向导 | P1 | 向导流程 |
| `/hosts` | 主机管理 | P1 | 列表/详情 |
| `/hosts/[id]` | 主机详情 | P1 | 详情 |
| `/packages` | 安装包管理 | P1 | 包列表/上传 |
| `/plugins` | 插件市场 | P1 | 插件列表 |
| `/audit-logs` | 审计日志 | P0 | 审计列表/追踪 |
| `/commands` | 命令历史 | P1 | 命令记录 |
| `/admin/users` | 用户中心 | P0 | 账号/令牌管理 |
| `/system-upgrade` | 系统升级 | P1 | 升级流程 |
| `/e2e-lab/install-wizard` | E2E 实验室 | P2 | 测试辅助 |
| `/login`、`/callback` | 鉴权 | P2 | 登录/回调 |

## 3. 当前状态快照（代码事实）

### 3.1 已有统一/可复用资产

| 资产 | 位置 | 能力 | 适用性 |
|---|---|---|---|
| 作业统一表（领域实现） | `frontend/components/common/sync/job-table/` | `JobTable`/`JobTableToolbar`/`JobRowActions`、`useJobList`、`StatPillsBar`、服务端分页、bulk、详情宿主（`JobDetailHost`） | 作业域复用；**模式**可抽象，**列/类型**不可直接复用 |
| 通用工作台表格壳 | `frontend/components/common/sync/StudioDataTable.tsx` | 集成卡片、`TableLoadingBar`、`TableSkeletonRows`、`ServerPagination`、`updatedAt` 刷新提示、空/错态插槽 | server-driven 表格首选基座候选之一 |
| 状态胶囊/头部/分页等 | `frontend/components/common/layout/`（`StatPillsBar`, `WorkspaceHeader`, `TableLoadingBar`, `TableSkeletonRows`, `ModuleNavTabs` 等） | 统一头部、统计条、加载与骨架、模块 Tab | 直接复用/扩展 |
| shadcn/ui 原语 | `frontend/components/ui/` | `table`, `tabs`, `badge`, `dropdown-menu`, `dialog`, `alert-dialog`, `sheet`, `drawer`, `pagination`, `select`, `input`, `button` 等 | 默认基础原语层（全局一致性基线） |
| Arc UI（vendored） | `frontend/components/arc/`，registry `@uiarc` | `tabs`, `badge`, `segmented-control`, `date-range-picker`, `button-group`, `combobox`, `search-field`, `empty-state`, `confirm-morph` 等 | 按需引入；须遵循 Arc skill 与 beUI 导入约束、`foundation.css`/token 约定 |
| beUI Motion（vendored/registry） | `frontend/components/motion/table`（PR #160 方向）、registry `@beui` | Motion Table（虚拟滚动、列交互）等 | **仅限本地/样本大数据网格**（如预览）；server-driven 管理列表默认不用其本地分页/排序语义 |

### 3.2 目标模块现状（待统一点）

| 模块 | 头部/统计 | 工具栏/筛选 | 表格/分页 | 标签/状态 | 下拉/操作 | 弹窗 | 抽屉/详情 |
|---|---|---|---|---|---|---|---|
| 告警中心 `MonitoringAlertsCenter` | `StatPillsBar`/筛选区 | 状态/级别/来源/时间/搜索等（Select/Input/时间） | `Table` + `TableLoadingBar`/`TableSkeletonRows` + `Pagination` | `Badge` 等级/状态 | 行操作按钮/菜单（视实现） | 确认/处理类（按需） | `Sheet` 详情 + 排障/记忆卡片 |
| 错误中心 `DiagnosticsErrorCenter` | `StatPillsBar` | 搜索/热度筛选等 | `Table` + loading/skeleton + `Pagination` | 热度/计数 Badge | 行内查看/跳转等 | - | `Sheet` 详情 + 事件/记忆 |
| 巡检中心 `DiagnosticsInspectionCenter` | `StatPillsBar` | 触发巡检/筛选 | `Table` + loading/skeleton + `Pagination` | 状态 Badge | 查看/下载等 | `Dialog` 启动巡检配置 | `Sheet` 报告详情 |
| 审计 `AuditLogMain`+`AuditLogTable` | `WorkspaceHeader` | 来源/动作组/资源/用户/日期范围（Arc `DateRangePicker`） | 独立 table 组件 + 分页 | 动作/结果/来源标签 | 行查看追踪 | - | `AuditTraceSheet` |
| 用户中心 `UserManagement` | `WorkspaceHeader` + `AccountsNavTabs` | 搜索/刷新 | `Table` + loading/skeleton + `Pagination` | 角色/状态 Badge/Switch 语义 | 编辑/删除等 | `Dialog` 新建/编辑，`AlertDialog` 删除/停用 | - |

> 注：上表为设计启动时的代码观察摘要，实施前需按各文件逐项核对（含移动端与权限分支）。

## 4. 目标架构（分层）

```
Domain Page / Workspace
  └─ <Domain>ListContainer        (fetch/query state, permissions, mutations)
       ├─ WorkspaceHeader / ModuleNavTabs          (页面级头部与模块 Tab)
       ├─ StatPillsBar?                            (可选统计胶囊)
       ├─ ListToolbar                              (搜索/筛选/Segment/DateRange/Bulk/刷新)
       ├─ DataTableCard | ServerDataTable          (统一卡片+表格壳：loading/empty/error/pagination/footer)
       │    ├─ columns renderer (domain)           (列唯一来源，按域维护)
       │    └─ RowActions (domain)                 (下拉菜单/按钮，权限与确认在域层注入)
       └─ Overlays                                 (Sheet 详情 / Dialog 表单 / AlertDialog 确认)
```

### 4.1 统一原则

1. **受控查询单一事实源**：`query` 状态（page/pageSize/keyword/filters/sort）由容器持有；URL 同步仅在需要深链的页面启用（参考 `JobListTab`/监控告警 URL 参数模式），并提供 patch 语义且筛选变化重置 page=1。
2. **Server-driven 不变量**：分页/排序/筛选结果以后端响应为准；前端不做跨页本地过滤排序作为权威结果（可仅做当前页展示优化，默认不做）。
3. **零塌陷加载**：刷新时保留表头与内容高度，使用 `TableLoadingBar`/遮罩或骨架策略；避免布局跳动（遵循 ui-conventions）。
4. **列与动作域内聚**：像 `job-table-columns.tsx` 一样，每个域一个“唯一列定义/渲染入口”；共享框架只提供列槽位与通用单元格原语（时间、拷贝、省略、状态徽章适配器）。
5. **覆盖层分工**：破坏性/不可逆操作 → `AlertDialog`；表单创建编辑 → `Dialog`；只读详情/追踪/上下文面板 → `Sheet`（右侧，桌面优先）；`Drawer` 仅在移动端特定场景按需引入并与 `Sheet` 行为对齐。

## 5. 交互原语统一选型（Tabs / Tags / Dropdowns / Dialogs / Drawers）

### 5.1 Tabs 与分段

| 场景 | 推荐原语 | 说明 | 参考 |
|---|---|---|---|
| 页面/模块级主导航 Tab（监控中心、诊断工作台、账号子导航） | `ModuleNavTabs`（`frontend/components/common/layout/ModuleNavTabs.tsx`）或统一封装；视觉风格与 Arc `Tabs` 对齐评估 | 保持路由/查询参数联动一致；避免各 workspace 自造 tab 样式 | 现有 `MonitoringCenterWorkspace` / `DiagnosticsWorkspace` / `AccountsNavTabs` |
| 工具栏内互斥筛选（状态组/模式/运行类型/视图切换） | `SegmentedControl`（`@/components/arc/segmented-control/segmented-control`）或 `StatPillsBar` 选中态 | 紧凑、单选、适合少量选项；与 `JobTableToolbar` 一致 | `frontend/components/common/sync/job-table/JobTableToolbar.tsx` |
| 内容区内多面板 Tab（详情内子页签等） | shadcn `Tabs`（`@/components/ui/tabs`）或 Arc `Tabs`（`@/components/arc/tabs/tabs`）二选一并全局收敛 | **全局建议**：存量 shadcn `Tabs` 为基线，新视觉/动效需求评估 Arc Tabs；同一产品层级避免混用两套动画与焦点样式 | `frontend/components/arc/tabs/tabs.tsx`, `frontend/components/ui/tabs.tsx` |

### 5.2 Tags / Badges（状态、严重级别、来源、计数）

| 场景 | 推荐原语 | 说明 |
|---|---|---|
| 表格内状态/级别/类型标签 | shadcn `Badge`（`@/components/ui/badge`）+ 统一语义 variant 映射（success/warning/danger/info/neutral） | 各域提供 `mapXToBadgeVariant`，颜色与文案走 i18n/主题 token |
| 统计胶囊与可点击筛选计数 | `StatPillsBar` + `StatPillItem.variant/pulse` | 已在 Job/告警/诊断使用，作为“可筛选标签条”标准 |
| 轻量计数/热度/多表等 inline tag | `Badge` 或 Arc `Badge`（`@/components/arc/badge/badge`）按密度选型 | Arc Badge 适合更紧凑/图标胶囊风；需统一圆角与字号层级 |

### 5.3 Dropdowns / Action Buttons

| 场景 | 推荐原语 | 说明 |
|---|---|---|
| 行内“更多操作” | shadcn `DropdownMenu`（`@/components/ui/dropdown-menu`） | 与 `JobRowActions` 模式一致：主操作按钮 + 更多菜单；危险项分组并二次确认 |
| 工具栏批量操作入口 | `Button` + `DropdownMenu` 或显式批量条（参考 `JobTableToolbar` bulk） | 多选列启用时才出现；server-driven 批量需处理分页选择语义（明确“仅本页/跨页”） |
| 筛选下拉 | shadcn `Select` / Arc `Combobox`（`@/components/arc/combobox/combobox`） | 选项少用 `Select`；可搜索/长列表用 `Combobox` |

### 5.4 Dialogs（弹窗）与确认

| 场景 | 推荐原语 | 说明 |
|---|---|---|
| 不可逆/高风险确认（删除、停用、强制停止等） | `AlertDialog`（`@/components/ui/alert-dialog`） | 文案必须明确后果；破坏性按钮样式区分 |
| 表单创建/编辑（用户创建编辑、巡检启动参数等） | `Dialog`（`@/components/ui/dialog`） | 表单校验与提交 loading；成功后刷新列表并 toast |
| 轻确认/快捷操作 | 评估 Arc `confirm-morph`（`@/components/arc/confirm-morph/confirm-morph`） | 仅用于低风险快捷确认，避免替代 AlertDialog |

### 5.5 Drawers / Sheets（详情与追踪）

| 场景 | 推荐原语 | 说明 |
|---|---|---|
| 记录详情、审计追踪、告警详情/排障上下文 | `Sheet`（`@/components/ui/sheet`，右侧） | 作为桌面端默认详情容器；内部可分节滚动、复用 `animateSheetSections` |
| 移动端或需要从底部滑出的补充面板 | `Drawer`（`@/components/ui/drawer`） | 按需使用；与 Sheet 信息架构对齐，不复制业务逻辑 |
| 工作台内嵌面板 | 既有面板/分栏模式（非强制 Drawer） | 遵循 workbench 布局约束 |

## 6. 共享组件建议（可能引入 / 调整）

> 原则：先复用现有 `ui/`、`common/layout/`、`sync/StudioDataTable`；确有重复再抽象。所有新组件需在 `frontend/components/common/**` 或对应域内落点并遵循目录规范。

| 候选组件 | 建议落点 | 职责 | 依赖/基础 | 优先级 |
|---|---|---|---|---|
| `ServerListShell` / `AdminListPageShell` | `common/list-shell/` 或 `common/data-table/` | 组合 Header + StatPills + Toolbar slot + TableCard slot + Pagination；统一间距与 loading/empty/error 插槽 | `WorkspaceHeader`, `Card`, slots | P1 |
| `ServerDataTableCard`（可由 `StudioDataTable` 泛化或并行提供） | `common/data-table/` 或增强 `sync/StudioDataTable` | 卡片化表格、sticky header、`TableLoadingBar`、`TableSkeletonRows`、`ServerPagination`、刷新时间 | `ui/table`, `ui/pagination` | P0 |
| `DataTableToolbar` primitives | `common/data-table/` | 搜索输入、筛选组布局、刷新按钮、批量条容器、左右槽位；筛选变化统一回调 `onQueryPatch` | `Input`, `Button`, `SegmentedControl` | P0 |
| `useServerTableQuery` / `usePaginatedResource` | `common/data-table/hooks/` 或复用 `job-table/use-job-table-query` 模式 | 受控 query（page reset 规则）、请求参数构建、loading/refreshing/error、可选轮询 | fetcher 注入 | P0 |
| `RowActionsMenu`（通用壳） | `common/data-table/` | 统一“主按钮 + DropdownMenu + 危险项确认”渲染协议 | `DropdownMenu`, `Button`, `AlertDialog` | P1 |
| `StatusBadge` / severity mappers（按域或共享适配器） | 各域 `*-format` / `common/list-shell` | 状态/严重级别到 `Badge` variant 与 i18n key 的映射 | `ui/badge` | P1 |
| 统一 `FilterSelect` / `FilterCombobox` 包装（可选） | `common/data-table/` | 统一 label、clear、宽度与 a11y | `ui/select`, `arc/combobox` | P2 |

**不建议首期引入/扩展的方向：**
- 不抽象“万能超级表格”覆盖 job/alert/audit/user 所有差异；避免配置爆炸。
- 不在 server-driven 列表中默认引入 beUI Motion Table 替代 `ServerDataTableCard`（除非明确转为 client-side 大样本场景并单独评审）。

## 7. 目标模块映射（现状 → 目标）

| 模块 | 目标骨架 | 重点改造 | 详情/覆盖层 | 风险/注意 |
|---|---|---|---|---|
| 告警中心 `MonitoringAlertsCenter` | `ServerListShell` + 统计胶囊 + toolbar + server table | 抽离筛选 query/patch 逻辑；行操作菜单化并统一确认；保留告警域状态转换与排障能力 | `Sheet` 详情保留，内部组件复用 | 状态机/批量操作复杂，需分步迁移并补测试 |
| 错误中心 `DiagnosticsErrorCenter` | 同上 | 热度筛选与跳转链路纳入 toolbar/filter 契约；统一空态/错误态 | `Sheet` 详情 + memory | 与巡检/告警跳转参数联动需回归 |
| 巡检中心 `DiagnosticsInspectionCenter` | 同上 | 启动巡检 `Dialog` 与列表刷新解耦；报告状态筛选统一 | `Sheet` 报告详情，`Dialog` 启动 | 运行中状态刷新策略需明确（轮询/手动） |
| 审计 `AuditLogMain`/`AuditLogTable` | Shell +（可选）统计 + toolbar（含 `ArcDateRangePicker`）+ server table | 合并 main/table 的加载与分页职责边界；筛选项组件化；追踪入口统一 | `AuditTraceSheet` 保留并对齐 Sheet header/section 规范 | 日期/时区（RFC3339 wallTime 转换）与导出/追踪链路不能回归 |
| 用户中心 `UserManagement` | Shell + toolbar + server table | CRUD 动作收敛为行菜单 + `Dialog`/`AlertDialog`；分页与搜索 patch 统一 | 表单 `Dialog`、危险 `AlertDialog` | 权限与“不可操作自己”等约束必须保留；乐观/悲观刷新一致 |
| 工作台 `Workbench` | 任务/作业列表壳 + 详情宿主 | 任务列表与作业列表对齐 `JobTable` 模式；运行详情/拓扑/恢复链路不回归 | 详情面板/抽屉 | 深链参数与轮询/overlay 语义 |
| 控制台 `Sync` | 作业列表壳 + 详情宿主 | 与工作台一致的作业表模式；Dolphin 触发链路与版本展示 | 作业详情/拓扑 | 版本/触发来源展示与筛选 |

## 8. Egolite 功能回归矩阵（每页必备）

<|channel|> 页面 | 必须覆盖的交互 |
|---|---|
| `/workbench` | 任务列表加载/筛选/分页；打开任务详情；选中作业；跳转到控制台作业详情；恢复/停止确认；深链 `task/job` 参数 |
| `/sync` | 作业列表加载/筛选/分页；打开详情；拓扑加载；恢复/保存点/停止；Dolphin 触发标识与版本展示 |
| `/monitoring?tab=alerts` | 状态/级别/时间筛选；分页；行操作；详情 Sheet；确认/静音/处理链路 |
| `/diagnostics`（错误中心） | 热度筛选；跳转链路；详情 Sheet；记忆/事件 |
| `/diagnostics`（巡检中心） | 启动巡检 Dialog；报告筛选；报告详情 Sheet；下载/查看 |
| `/audit-logs` | 来源/动作/资源/日期筛选；分页；追踪抽屉；时区转换 |
| `/admin/users` | 搜索/分页；创建/编辑 Dialog；删除/停用 AlertDialog；令牌 Tab |
| 其他 P1 页面 | 至少覆盖“列表加载 + 主要筛选/分页 + 主操作/详情打开” |

## 9. Arc UI / beUI 调研与链接（供后续核验）

> 说明：外部站点页面结构可能变动，实施前请以 registry 与本地 vendored 代码为准逐项核对 API。仓库已配置 shadcn registries（见 `frontend/components.json`）。

### 9.1 Registry 与文档入口

- **Arc UI 官网 / Docs**
  - Home: https://uiarc.dev
  - AI / Agent docs（skill 指引）: https://uiarc.dev/docs/ai
  - Docs root（核验导航与组件清单）: https://uiarc.dev/docs
  - Registry（项目已配置 `@uiarc`）: `https://uiarc.dev/r/{name}.json`（例：`https://uiarc.dev/r/tabs.json`, `https://uiarc.dev/r/badge.json`, `https://uiarc.dev/r/segmented-control.json`, `https://uiarc.dev/r/combobox.json`, `https://uiarc.dev/r/date-range-picker.json`, `https://uiarc.dev/r/empty-state.json`）
- **beUI 官网 / Docs**
  - Home: https://beui.dev
  - Docs root（核验组件清单）: https://beui.dev/docs
  - Registry（项目已配置 `@beui`）: `https://beui.dev/r/{name}.json`（例：`https://beui.dev/r/table.json`，以实际 registry 名称为准）

### 9.2 本仓库已安装/相关组件（核验基线）

- Arc（`frontend/components/arc/`）：`tabs`, `badge`, `segmented-control`, `date-range-picker`, `button-group`, `combobox`, `search-field`, `empty-state`, `confirm-morph`, `input` 等（以目录为准）
- shadcn/ui（`frontend/components/ui/`）：`table`, `tabs`, `badge`, `dropdown-menu`, `dialog`, `alert-dialog`, `sheet`, `drawer`, `pagination`, `select`, `command` 等
- beUI Motion：PR #160 规划/引入的 `frontend/components/motion/table/`（本地预览大样本网格）；server-driven 管理列表默认不采用其本地状态语义

### 9.3 与本任务强相关的候选组件映射

| 需求 | 首选（现有/基线） | Arc/beUI 候选（按需） | 备注 |
|---|---|---|---|
| 模块 Tab | `ModuleNavTabs` / `ui/tabs` | Arc `tabs`（`@/components/arc/tabs/tabs`） | 全局选一套主实现，避免双轨动画/焦点差异 |
| 状态标签 | `ui/badge` + `StatPillsBar` | Arc `badge` | 领域统一 variant 映射 |
| 工具栏分段筛选 | `arc/segmented-control`（已用） | - | 与 JobToolbar 对齐 |
| 日期范围筛选 | `arc/date-range-picker`（审计已用） | - | 统一时区与清空语义 |
| 可搜索下拉 | `ui/select` / `ui/command` 组合 | Arc `combobox` | 长列表/搜索场景 |
| 行操作菜单 | `ui/dropdown-menu` | - | 统一危险项与确认 |
| 表单/确认弹窗 | `ui/dialog`, `ui/alert-dialog` | Arc `confirm-morph`（轻确认） | 高风险必须 AlertDialog |
| 详情抽屉 | `ui/sheet`（已有 `AuditTraceSheet`） | `ui/drawer`（移动端补充） | 统一标题区/分节/关闭行为 |
| 本地大样本表格 | - | beUI Motion `table`（vendored） | **仅 client-side 场景**（如预览）；需依赖与 i18n/a11y/许可合规 |

## 10. 实施策略（建议分阶段）

1. **Phase 0 - 设计冻结**：评审本设计，拍板开放问题（Tab 主实现、Shell 落点、Drawer 策略、批量能力范围、Egolite 覆盖粒度）。
2. **Phase 1 - 抽取通用骨架**：在不改变业务行为前提下提炼/引入 `ServerDataTableCard`、toolbar primitives、query hook（可从 `StudioDataTable`/`job-table` 模式抽象），补 Story/用例或最小单测（如 query patch reset）。
3. **Phase 2 - 试点迁移**：优先迁移结构最接近的模块（建议 **审计** 或 **用户中心**）验证壳与 Dialog/Sheet 模式；再迁移 **告警中心**（复杂度最高，最后或拆分）。
4. **Phase 3 - 诊断模块收敛**：统一 `DiagnosticsErrorCenter` / `DiagnosticsInspectionCenter` 到同一 shell，复用巡检 Dialog 与详情 Sheet 规范。
5. **Phase 4 - 工作台/控制台换新**：任务/作业列表与详情链路对齐；深链与轮询不回归。
6. **Phase 5 - P1 页面换新**：监控其它页、集群、主机、包/插件、命令、系统升级、仪表盘。
7. **Phase 6 - 清理与规范回写**：删除重复样板，更新 `.trellis/spec/frontend/ui-conventions.md`（如新增“管理列表壳”约定）与必要组件文档。

## 11. 验证与回归清单（实施时）

- [ ] 各列表：筛选变更 `page` 重置为 1；分页 `totalPages` 已知/未知分支展示正确；空数据与末页行为正确。
- [ ] 加载态：刷新/翻页不出现表格高度塌陷；首屏 skeleton 与增量刷新 loading bar 符合约定。
- [ ] i18n：所有用户可见文案、`aria-label`、空态/错误态、Dialog/Sheet 标题走 next-intl。
- [ ] a11y：键盘可达（Tab/菜单/弹窗焦点陷阱与返回）、表头语义、操作按钮可读标签。
- [ ] 交互一致：行操作危险项统一二次确认；Sheet 关闭与路由/状态清理无泄漏。
- [ ] 权限与边界：用户中心自我保护、告警状态转换、巡检触发权限等原有约束不回退。
- [ ] Egolite：按第 8 节矩阵逐页执行点击回归并记录结果。
- [ ] 质量门禁：按仓库约定执行相关 `pnpm test` / `pnpm exec tsc --noEmit` / lint（注意既有 lint 基线问题需区分）。

## 12. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 过度抽象导致壳组件难以适配告警/诊断复杂交互 | 交付慢、回归多 | 保持 slot/renderer 注入；首期只抽“必同项”（卡片、分页、loading、query patch） |
| Tab/Badge 双实现并存造成视觉漂移 | 体验不一致 | 设计评审拍板主实现并制定迁移顺序；新增代码禁用未选方案 |
| server-driven 列表误用 beUI Table 本地状态 | 排序/分页与后端不一致 | 在规范与 code review checklist 明确边界；仅 client-side 场景使用 |
| vendored Arc/beUI 升级成本 | 维护负担 | 集中 adapter/wrapper，记录差异；升级用 diff merge 并同步 license 清单 |
| 大模块一次性重写风险高（MonitoringAlertsCenter） | 回归 | 分阶段、按子 PR/子任务拆分，保留行为对照测试 |
| 全局范围扩大导致回归面激增 | 交付风险 | 先 P0，再 P1/P2；Egolite 回归矩阵作为门禁 |
