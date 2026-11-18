# 设计：管理列表迁移 + 全站交互组件对齐

> 关联：`11-17-unified-job-table`、`11-17-job-detail-host`、PR #160（预览表 beUI Table 的边界）
> 规范：`.trellis/spec/frontend/ui-conventions.md`（§6 筛选栏、§7 弹窗、§10 表格卡片与分页、§14 Arc 组件）

## 1. 结论

统一的是**框架与交互组件的选型**，不是同一个领域表格。

- 5 个管理列表都是服务端驱动的列表：分页、筛选、排序以后端为准。它们已经遵守 §10，问题是**同一套卡片和分页底栏拷贝了 5 份**，
  查询状态也各写各的。所以只抽两个公共件（§4），不建"万能壳"。
- 作业域的 `JobTable` 不外溢到告警、审计、用户；它的列定义、轮询、批量逻辑留在作业域。
- 交互组件（确认、下拉、菜单、标签、页签）的选型是**全局性的**，先定选型（§3），再分两层落地：
  - 管理列表随骨架迁移一起换；
  - 工作台、作业控制台、大盘只换组件，不动布局。

## 2. 现有资产（代码事实）

| 资产 | 位置 | 实际能力 | 本任务中的用法 |
|---|---|---|---|
| 页头 / 统计条 / 模块页签 | `common/layout/`：`WorkspaceHeader`、`StatPillsBar`、`ModuleNavTabs` | 统一页头、可点击的统计胶囊、页面级页签 | 直接复用 |
| 加载与骨架 | `common/layout/`：`TableLoadingBar`、`TableSkeletonRows` | 刷新时的顶部加载条、首屏骨架行 | 收进 §4.1 的表格卡片 |
| 分页 | `ui/pagination` | §10 指定的分页组件 | 收进表格卡片底栏 |
| `StudioDataTable` | `common/sync/StudioDataTable.tsx` | HeroUI 解剖结构的表格原语（`StudioTable`/`Header`/`Column`/`SortableColumnHeader`/`Row`/`Cell`/`EmptyRow`/`Footer`）+ **客户端排序**（`applyStudioSort`） | 只用于工作台任务表这类本地数据，**不作为**服务端列表的基座 |
| beUI Table | `components/motion/table/` | 虚拟滚动、列宽拖动、本地排序 | 只用于预览这类客户端大样本，不用于管理列表 |
| 作业表 | `common/sync/job-table/` | 作业域的列、工具栏、行操作、详情宿主 | 作为模式参考，不复用到其他实体 |

## 3. 交互组件统一选型

状态列：**已定** = 规范已有结论，直接执行；**待定** = Phase 0 拍板后写回规范。

| 场景 | 组件 | 出处 / 状态 | 说明 |
|---|---|---|---|
| 破坏性 / 不可逆确认（删除、停用、强制停止） | `ui/alert-dialog` | §7，**已定** | 写清后果，危险按钮用 destructive；全站禁止 `window.confirm` |
| 表单创建 / 编辑 | `ui/dialog` | §7，**已定** | 宽度按 §7.2 分级 |
| 行内轻确认 | Arc `confirm-morph` | §14，**已定** | 只用于规范已列的场景（解除绑定），不扩大用途 |
| 少量选项的下拉选择 | `ui/select` | **待定** | 现状以它为主 |
| 可搜索 / 长列表选择 | Arc `combobox` | §14 已在模板插件列表使用；全局**待定** | 替代 `ui/command` 自拼的可搜索下拉 |
| 行内"更多"菜单 | `ui/dropdown-menu` | **待定** | `ui/popover` 不当菜单用；单行动作 ≤2 个时直接摆按钮 |
| 状态 / 级别标签 | Arc `badge`（`tone`） | **已定**（2026-11-18 用户确认），写入规范 §17 | 成功、失败、警告、进行中这类状态都用它 |
| 非状态的中性标签（版本、数量、分类、类型） | `ui/badge` 的 `outline` / `secondary` | **已定**，写入规范 §17 | 不带语义色，不手写颜色 class |
| 页面级页签 | `ModuleNavTabs` | **已定** | 监控中心、诊断中心、账号子导航 |
| 详情内页签 | Arc `Tabs` | §14，**已定** | |
| 工具栏互斥筛选 / 视图切换 | Arc `SegmentedControl` | §14，**已定** | |
| 详情 / 追踪面板 | `ui/sheet`（右侧） | **已定**（现状一致） | |
| 分页 | `ui/pagination` | §10，**已定** | |
| 日期范围 | `ArcDateRangePicker` | §14，**已定** | |

**状态标签的决定（已定）**：

- **按用途分工**：表示状态的标签用 Arc `badge` 的 `tone`；非状态的中性标签继续用 `ui/badge` 的 `outline` / `secondary`。
- **理由**：
  - 全站 172 处 Badge 用法中，有 91 处给 `ui/badge` 手写了颜色 class（`text-green-600`、`bg-amber-500/10` 等）。
    `ui/badge` 只有 default / secondary / destructive / outline 四种，没有语义色。
  - 手写的结果是：同一状态的配色、字号、有无底色都不一致；部分写法没有 `dark:` 变体，深色主题下对比度不足。
  - 这 91 处无论选哪个方案都要改。Arc `badge` 自带 `neutral/success/info/warning/danger` 五种 tone，深浅色都调过，
    还有 `dot`、`outline`、`size`；规范 §14 也已在作业详情、工作台、升级向导中用它表示状态。
  - 备选方案"给 `ui/badge` 补语义 variant"颜色效果接近，但要改 shadcn 源码，也没有 dot / size 变体，因此不采用。
- **状态 → tone 对照**，沿用 `JobDetailDialog.jobStatusTone` 的既有约定，各域在 `*-format.ts` 里各提供一个映射函数：

| tone | 含义 | 例子 |
|---|---|---|
| `success` | 成功、已完成、健康、已启用 | SUCCESS、FINISHED、SAVEPOINT_DONE、在线 |
| `info` | 进行中、运行中、已调度 | RUNNING、执行中、巡检中 |
| `warning` | 等待、即将超限、需要注意、未知中间态 | PENDING、排队、告警级别 warning |
| `danger` | 失败、错误、离线、严重 | FAILED、离线、告警级别 critical |
| `neutral` | 已取消、已停止、已禁用 | CANCELED、已停用 |

- **列表里的状态列**统一用 `size='sm'` 加 `dot`，各页之间不再混用不同尺寸。

## 4. 管理列表骨架

### 4.1 只抽两个公共件

| 公共件 | 落点 | 职责 | 依据 |
|---|---|---|---|
| `TableCard`（暂名） | `common/layout/`，与 `TableLoadingBar` 同目录 | §10 的卡片容器与保底高度；刷新时的加载条；首屏骨架 / 空状态 / 错误状态插槽；分页底栏（左侧总数与每页条数，右侧 `ui/pagination`） | 5 个页面各有一份拷贝 |
| `useServerListQuery`（暂名） | `common/layout/` 或 `lib/hooks/` | 持有 page / pageSize / filters；`patch(filters)` 时回到第 1 页；生成请求参数；可选同步到 URL | 告警、巡检、错误中心各自实现了页码重置，审计和用户没有 |

不抽 `ServerListShell`、工具栏原语、`RowActionsMenu`、`StatusBadge`、`FilterSelect`。迁移过程中如果某个模式在 ≥3 个页面重复，
再按同样的标准提出。

### 4.2 页面结构

```
<Domain>Page（取数、权限、增删改）
  ├─ WorkspaceHeader / ModuleNavTabs
  ├─ StatPillsBar（可选）
  ├─ 筛选栏（按 §6，各域自己的筛选项）
  ├─ TableCard
  │    ├─ 各域自己的列定义（每个域一个唯一入口）
  │    └─ 行操作（按钮；>2 个时次要动作进 DropdownMenu）
  └─ 覆盖层：Sheet 详情 / Dialog 表单 / AlertDialog 确认
```

### 4.3 各页面迁移要点

| 页面 | 要点 | 风险 |
|---|---|---|
| 审计（试点） | `AuditLogMain` 持有查询状态，`AuditLogTable` 只负责渲染；接入 `TableCard` | 日期时区换算（`lib/date-range.ts`）与追踪面板链路不能回退 |
| 用户 | 新建 / 编辑用 `Dialog`，删除 / 停用用 `AlertDialog`（现状已是）；接入两个公共件 | "不能操作自己"等约束保持不变 |
| 错误中心、巡检 | 两页共用同一套结构；巡检的启动对话框保留 `Dialog` | 与告警、作业、集群的跳转参数 |
| 告警（最后做） | 文件 1384 行，先换卡片和查询状态，再处理行操作与详情面板 | 状态流转、深链参数、排障面板 |

## 5. 交互组件对齐层（工作台、作业控制台、大盘及其他页面）

- 只替换组件，不改布局、列表骨架、数据流、轮询和深链。
- 必改：3 处 `window.confirm` → `AlertDialog`：`TaskAlertSection.tsx:90`、`MonitoringPolicyCenter.tsx:1680`、`:1846`。
- 逐个判断：sync 下 5 个用普通 `Dialog` 承载确认的文件（`TaskAlertSection`、`CuratedTemplatesManagePanel`、`DataSyncStudio`、
  `DolphinPushDialog`、`StudioDialogs`）。纯确认改成 `AlertDialog`，带表单的保留。
- 状态标签：把手写颜色 class 的 `ui/badge`（全站 91 处）换成 Arc `badge` 的 `tone`；非状态的 `outline` / `secondary` 标签不动。
- 下拉框、更多菜单按 §3 选型替换；具体替换清单来自 Phase 0 的差异清单。

## 6. Phase 0 差异清单与回归矩阵

### 6.1 差异清单格式

两层页面分开列，每格写"当前实现 → 目标实现"，没有差异的写"一致"。

- **管理列表**：卡片容器、分页底栏、加载 / 空 / 错态、筛选栏、页码重置、行操作、确认、下拉、标签、详情面板。
- **工作台、作业控制台、大盘及其他页面**：只列确认、下拉、更多菜单、状态标签四类，并精确到文件和行号。

### 6.2 Egolite 回归矩阵

每条用例写成可以逐条勾选的步骤，并写明失败判定，例如"筛选后表格未刷新"或"页码未回到 1"。

| 页面 | 必须覆盖 |
|---|---|
| 审计 `/audit-logs` | 来源 / 动作 / 资源 / 日期筛选；翻页与每页条数；筛选后页码回到 1；打开追踪面板；时区显示 |
| 用户 `/admin/users` | 搜索与翻页；新建 / 编辑对话框提交；删除 / 停用确认；不能操作自己；令牌页签 |
| 错误中心 `/diagnostics` | 热度筛选；翻页；详情面板；跳转到作业或集群 |
| 巡检 `/diagnostics` | 启动巡检对话框；报告筛选与翻页；报告详情；下载或查看 |
| 告警 `/monitoring?tab=alerts` | 状态 / 级别 / 时间筛选；翻页；行操作；详情面板；深链参数 |
| 交互组件对齐层 | 每一处被替换的组件：确认（取消与确认两条路径）、下拉（选中、清空、键盘操作）、更多菜单（每个菜单项）、标签显示 |

## 7. 风险

| 风险 | 缓解 |
|---|---|
| 公共件抽象过度，难以适配告警等复杂页面 | 只抽 §4.1 两个件，用插槽注入，告警最后迁移 |
| 交互组件对齐层误改了布局或数据流 | 单独 PR；回归矩阵覆盖每一处替换；评审时检查 diff 只涉及组件替换 |
| 状态标签迁移 91 处，可能漏改或映射不一致 | 各域只提供一个 `xxxStatusTone` 映射函数；完成后 grep 确认没有手写颜色 class 的 `Badge` |
| Arc `badge` 文字变化时有动画，大列表频繁刷新时可能卡顿 | 审计试点时用最大页（100 行）加自动刷新验证；有问题再评估在列表里关闭动画 |
| 服务端列表误用客户端排序语义（`StudioDataTable` / beUI Table） | 在规范和评审清单中写明边界 |
| vendored 的 Arc / beUI 组件升级 | 项目适配集中在封装层；改动在文件头注明；同步 `licenses/third_party_mit_files.txt` |
