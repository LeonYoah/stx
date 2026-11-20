# 设计：管理列表迁移 + 全站交互组件对齐

> 关联：`11-17-unified-job-table`、`11-17-job-detail-host`、PR #160、PR #168（大盘视觉）、PR #170（诊断对齐告警中心）
> 规范：`.trellis/spec/frontend/ui-conventions.md`（§3 列表/DetailHost、§5 弹窗、§8 Arc、§9 Badge、§10 大盘）
> 收口：2026-11-20（按 #168/#170 与告警中心现状收缩范围）

## 1. 结论

统一的是**框架与交互组件的选型**，不是同一个领域表格。

- **仍缺骨架的管理列表**（审计、用户）是服务端驱动列表：抽 / 复用 `TableCard`（已存在）与 `useServerListQuery`（待建），不建「万能壳」。
- **告警、错误、巡检、巡检策略**的 list + 常驻详情已完成（告警在 main；其余随 #170）。本期只做交互组件对齐，可选接线查询 hook。
- **大盘布局**由 #168 完成，本期不改；只对齐确认 / 下拉 / 溢出菜单并清理 Combobox a11y 兜底。
- 作业域 `JobTable` 不外溢到告警 / 审计 / 用户。

## 2. 现有资产（2026-11-20，main `918bfdc1`）

| 资产 | 位置 | 实际能力 | 本任务中的用法 |
|---|---|---|---|
| 页头 / 模块页签 | `common/layout/`：`WorkspaceHeader`、`ModuleNavTabs` | 统一页头、页面级页签 | 直接复用 |
| **`TableCard`** | `common/layout/TableCard.tsx`（208） | §3/§4.4 卡片、加载条、骨架/空/错、分页底栏 | **已存在，禁止重建**；审计 / 用户接入 |
| **`DetailHost` + list-detail CSS** | `DetailHost.tsx`（222）、`list-detail.module.css`（623） | 常驻 rail / 窄屏 Drawer / J/K | 管理列表详情标准；已用于告警 / 错误 / 巡检 / 策略 / 通知 / 规则 |
| **脏关闭** | `DirtyCloseGuard.tsx`（`DirtyCloseDialog`）、`UnsavedChangesDialog.tsx` | 未保存确认 + beforeunload | 策略等编辑页已用；迁移页复用 |
| **`SoftChip`** | `SoftChip.tsx`（56） | 轻量状态芯片 | 诊断 / 告警行内已用 |
| 状态胶囊 | `StatPillsBar`（178） | 可点击状态计数 | **已弃用**；本期迁令牌 + 作业工具栏 |
| 加载与骨架 | `TableLoadingBar`、`TableSkeletonRows` | 刷新条、首屏骨架 | 已收进 `TableCard` |
| 分页 | `ui/pagination` | §3 指定 | `TableCard` 底栏 |
| `StudioDataTable` | `common/sync/StudioDataTable.tsx` | 客户端排序表格原语 | 只用于工作台任务表等本地数据 |
| beUI Table | `components/motion/table/` | 虚拟滚动等 | 只用于预览大样本 |
| 作业表 | `common/sync/job-table/` | 列、工具栏、`JobDetailHost` | 模式参考；工具栏 `StatPillsBar` 在对齐层替换 |
| 大盘件（#168） | `KpiStrip` / `ThroughputTabs` / `DashboardPanel` / `ClusterScopeStrip` | 指标条、吞吐格、面板、集群范围条 | **布局冻结**；规范见 ui-conventions §10 |

**已不存在（勿再按旧库存盘）**：`MonitoringAlertsCenter.tsx`、`DiagnosticsErrorCenter.tsx`、`DiagnosticsInspectionCenter.tsx`、`MonitoringPolicyCenter.tsx` 单体文件。

## 3. 交互组件统一选型

| 场景 | 组件 | 出处 / 状态 | 说明 |
|---|---|---|---|
| 破坏性 / 不可逆确认 | `ui/alert-dialog` | §5，**已定** | 禁止 `window.confirm` |
| 表单创建 / 编辑 | `ui/dialog` 或已迁页的 Arc `Dialog` + 内部 Arc 控件 | §5，**已定** | 本期不强制换外框；#170 部分策略弹窗已用 Arc `Dialog` |
| 行内轻确认 | Arc `confirm-morph` | §5 / §8，**已定** | 单条、不可撤回、只涉及自己、有按钮可锚定（§3.4） |
| 少量选项下拉 | Arc `Select`（源码补丁见 §3.2） | **已定** | 2–5 互斥筛选用 `SegmentedControl` |
| 可搜索 / 长列表 | Arc `Combobox` | **已定** | main 上 `label` **已可选**；必须可见标签或 `aria-label`（§3.2 a11y） |
| 行内「更多」 | Arc `DropdownMenu` + `Button` `iconOnly` | **已定** | `ui/popover` 不当菜单 |
| 状态 / 中性标签 | shadcn `ui/badge` | §9，**已定** | 不做 Arc Badge 批量迁移 |
| 页面级页签 | `ModuleNavTabs` | **已定** | |
| 详情内页签 | Arc `Tabs` | **已定** | |
| 工具栏互斥筛选 | Arc `SegmentedControl` | **已定** | 计数规则见 §3.3 |
| **管理列表常驻详情** | **`DetailHost`** | §3，**已定（2026-11-20）** | 取代「详情一律 `ui/sheet`」；Sheet 仅临时覆盖（审计追踪） |
| 状态筛选多选 | Arc `chip-group` + 计数 | **已定** | 作业状态；计数来自 `with_counts` |
| 分页 | `ui/pagination` | **已定** | |
| 日期范围 | `ArcDateRangePicker` | **已定** | |

### 3.1 视觉方向：8–10px 桌面后台收紧版（2026-11-19 拍板）

- 控制台外层覆盖 Token：`--radius-control: 0.5rem`、`--radius-panel: 0.625rem`、`--control-height-md: 2rem`、`--control-height-sm: 1.75rem`；`--radius-popover: 0.625rem`（`:root`）；不魔改组件圆角源码；`ConfirmMorph` / `Switch` 保持胶囊；不启 `superellipse`。
- **避免框框堆叠**：只留 `TableCard` 一层主外框；筛选落画布；行内以 ghost / 菜单为主。
- **同一块区域只用一套**：Arc 与 shadcn 不混用（`ui/badge` 豁免）。
- **分两期**：本期 = 审计 / 用户完整迁移 + 告警 / 诊断 / 工作台 / 大盘组件对齐；其他页面表单下期。对话框外框不强制本期统一。

### 3.2 Arc 源码改动与 a11y

| 组件 | 现状（main） | 本期仍要做 |
|---|---|---|
| `Combobox` | `label` 已可选；缺 label 且无 `aria-label` 时 dev warn | 大盘头部去掉 `[&_label]:sr-only`，改为不传 label + `aria-label`（或可见 label）；**验收：控件必有可访问名称** |
| `Select` | 已安装 | 标签可选时同样强制 `aria-label`；选项图标 / 分组按 Phase 0 清单补 |
| `DropdownMenu` / `Button` `iconOnly` / `chip-group` `count` | 已安装 | 按缺口补自定义触发器、`iconOnly`、`count`；文件头注明 Local modification |
| 弹层样式 | 随 Token 收紧 | `:root` `--radius-popover` + 三种弹层 10/6/4/32 |

**a11y 硬约束（stx UI 检查）**：标签可可选，但「无可见标签且无 `aria-label`」一律不合格。

### 3.3 `StatPillsBar` 退役与筛选计数

- 全站退役；计数用 Arc `Badge size='sm'` 放在 `accessory` / chip `count`。
- **计数政策（2026-11-20）**：
  - **禁止**当前页分桶计数（与分页总数矛盾）——错误 / 巡检已去掉，**不要加回**。
  - 分段控件可保留；无可靠全集统计则**不显示数字**。
  - 告警：继续用一次未过滤轻量请求的服务端统计。
  - 策略：客户端已持有全集时可显示与 scoped 列表一致的计数。
  - 作业：`with_counts` 服务端计数 → `chip-group`。
- **本期迁移**：

| 位置 | 换成 | 阶段 |
|---|---|---|
| `JobTableToolbar` 两处 `StatPillsBar` | `chip-group` 多选 + 计数；URL 多值兼容 | Phase 6 |
| `TokenManagement` | `SegmentedControl`（有可靠计数才显示） | Phase 3 |
| 告警 / 错误 / 巡检 | 已不用 `StatPillsBar` | Phase 4/5 仅组件对齐 |

- **下期**：`ClusterMain`、`HostMain`、`ClusterConfigs`、`PluginMain`、`PackageMain`、`ClusterDetail`×2、`TroubleshootingMemoryCenter` 等。

### 3.4 确认方式按操作分类（2026-11-19 方案 A）

| 性质 | 方式 | 例 |
|---|---|---|
| 能撤回 | 不确认 + toast（可带撤销） | 静默告警 |
| 单条、不可撤回、只涉及自己、有锚点 | `ConfirmMorph` | 关闭单条告警 |
| 影响他人 / 批量 / 范围大 | `AlertDialog` | 删用户、吊销令牌 |

- 告警「关闭」现状无确认 → 本期补 `ConfirmMorph`。
- 巡检策略删除 / 脏关闭：#170 已改，Phase 6 复核即可，勿按旧 `MonitoringPolicyCenter` 行号改。

### 3.5 出错与成功提示

- 表单字段错误落在字段 `error`；列表加载失败留在 `TableCard` 错误插槽 + 重试；toast 只用于用户刚完成的操作结果。

### 3.6 弹层数值（2026-11-19）

- 外框 10px、选项 6px、内边距 4px、行高 32px；不启连续圆角。

## 4. 管理列表骨架

### 4.1 公共件

| 公共件 | 落点 | 职责 | 状态 |
|---|---|---|---|
| `TableCard` | `common/layout/TableCard.tsx` | 卡片、保底高度、加载、骨架/空/错、分页 | **已有，复用** |
| `useServerListQuery` | `common/layout/` 或 `lib/hooks/` | page / pageSize / filters；`patch` 回第 1 页；可选 URL 同步 | **待建** |

不抽万能 `ServerListShell` / `RowActionsMenu` / `StatusBadge`。详情宿主用既有 `DetailHost`，不另造。

### 4.2 页面结构（完整迁移页）

```
<Domain>Page
  ├─ WorkspaceHeader / ModuleNavTabs
  ├─ 筛选栏（SegmentedControl / Select / Combobox；计数见 §3.3）
  ├─ TableCard（域内列 + 行操作）
  └─ DetailHost 常驻详情 或 临时 Sheet（仅追踪类）
       + Dialog / AlertDialog / ConfirmMorph
```

### 4.3 各页要点（收缩后）

| 页面 | 要点 | 风险 |
|---|---|---|
| 审计（试点） | 接入 `TableCard` + hook；`AuditTraceSheet` 保持 Sheet | 时区与追踪链路 |
| 用户 | 接入两公共件；令牌换分段；删除/停用 `AlertDialog` | 「不能操作自己」 |
| 错误 / 巡检 / 策略 | **不重做骨架**；组件对齐；可选 hook | 勿加回页内桶计数 |
| 告警 | **不重做骨架**；关闭补 ConfirmMorph；组件对齐 | 深链、状态流转 |

### 4.4 `TableCard` 加载约定

已由现组件实现：一次等待一种提示；骨架仅首屏（300ms / 500ms）；行入场只播一次；数字列 `tabular-nums`；`aria-busy`。新接入页不要旁路这些约定。

## 5. 交互组件对齐层（工作台、大盘、已 list+detail 页）

- 只换组件，不改布局 / 骨架 / 数据流 / 轮询 / 深链。
- 大盘：确认、Select/Combobox/DropdownMenu；清 `sr-only` + 保证 aria 名称；**不**改 `KpiStrip` / `ThroughputTabs` / `ClusterScopeStrip` / 作业列表结构；**不**恢复运行类型分段。
- 告警 / 诊断：在现有 `DetailHost` 壳内对齐控件。
- 作业工具栏：`StatPillsBar` → `chip-group`。
- 复核全库 `window.confirm` 与 Dialog-vs-AlertDialog；以 grep 现状为准。

## 6. Phase 0 差异清单与回归矩阵

### 6.1 差异清单

- **完整迁移页**（审计、用户）：卡片、分页、加载态、筛选、页码重置、行操作、确认、下拉、详情宿主。
- **对齐层页**（工作台、大盘、告警、错误、巡检、策略）：只列确认、下拉、更多菜单、状态筛选；精确到文件。

### 6.2 Egolite 回归矩阵

| 页面 | 必须覆盖 |
|---|---|
| 审计 `/audit-logs` | 筛选、翻页、页码回 1、追踪 Sheet、时区 |
| 用户 `/admin/users` | 搜索翻页、表单字段错误、删/停确认、不能操作自己、令牌分段 |
| 错误 / 巡检 `/diagnostics` | 筛选（无错误的页内桶计数）、翻页、DetailHost J/K、跳转；巡检启动对话框 |
| 告警 `/monitoring?tab=alerts` | 状态分段计数与列表一致、翻页、关闭 ConfirmMorph、DetailHost、深链 |
| 大盘 | 布局无回归；头部 Combobox 可访问名称；确认/下拉/菜单替换点 |
| 作业列表 | 多选状态 URL 新旧兼容；只选「运行中」时速率排序 |
| 加载态（审计/用户） | 刷新不熄屏；错误在卡片内 |

## 7. 风险

| 风险 | 缓解 |
|---|---|
| 误把告警/诊断当骨架迁移重做 | Phase 4/5 清单明示「仅组件对齐」；评审拒布局 diff |
| 误改大盘布局 | Phase 6 单独 PR；diff 白名单组件替换 |
| 加回当前页桶计数 | 规范 §3 + 本设计 §3.3；回归断言 |
| Combobox 去掉 sr-only 后无名 | Phase 1/6 AC 强制可见标签或 aria-label |
| Token 收紧影响已有 Arc 页 | Egolite 覆盖升级向导 / 作业详情 |
| `StatPillsBar` 跨期残留 | 规范弃用；新代码禁用 |
