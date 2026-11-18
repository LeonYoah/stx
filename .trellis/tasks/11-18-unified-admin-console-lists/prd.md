# 全局 UI 统一：管理列表迁移 + 全站交互组件对齐

## 背景

作业列表已经在 `11-17-unified-job-table` 和 `11-17-job-detail-host` 中统一完成：`JobTable`、`JobTableToolbar`、`StatPillsBar`、
服务端分页，以及统一的行操作和详情宿主。其他管理列表虽然遵守了 `ui-conventions.md` §10 的表格卡片约定，
但每个页面各写一份；确认弹窗、下拉框、状态标签这类小组件在全站也有多种实现并存，视觉和交互不一致。

## 已确认事实（2026-11-18，基于 main `88628080`）

### 管理列表（5 个页面）
| 页面 | 文件（行数） | 已用的共享件 | 交互组件 |
|---|---|---|---|
| 告警中心 | `MonitoringAlertsCenter.tsx`（1384） | `StatPillsBar`、`TableLoadingBar`、`TableSkeletonRows`、`ui/pagination` | `ui/badge`、`ui/select`、`ui/sheet` |
| 错误中心 | `DiagnosticsErrorCenter.tsx`（959） | 同上 | `ui/badge`、`ui/sheet` |
| 巡检中心 | `DiagnosticsInspectionCenter.tsx`（1496） | 同上 | `ui/badge`、`ui/select`、`ui/dialog`、`ui/sheet` |
| 审计日志 | `AuditLogMain.tsx`（368）+ `AuditLogTable.tsx`（464）+ `AuditTraceSheet.tsx` | `TableLoadingBar`、`TableSkeletonRows`、`ui/pagination`、`ArcDateRangePicker` | `ui/badge`、`ui/select`、`ui/sheet` |
| 用户中心 | `UserManagement.tsx`（728） | `WorkspaceHeader`、`TableLoadingBar`、`TableSkeletonRows`、`ui/pagination` | `ui/badge`、`ui/dialog`、`ui/alert-dialog` |

- 5 个页面**各自重复**同一套表格卡片：§10 的保底高度容器、`mt-auto border-t` 分页底栏、加载条、骨架行，共 5 份拷贝。
- "筛选变化时回到第 1 页"也是各写各的：告警 11 处、巡检 4 处、错误中心 2 处，审计和用户 0 处（需在 Phase 0 核对是否漏了）。
- 5 个页面的行操作都是直接摆按钮，没有使用 `DropdownMenu`。
- `StudioDataTable` 不是带卡片、分页、加载的表格壳。它是 HeroUI 解剖结构的表格原语，外加客户端排序工具
  （`applyStudioSort`），用于工作台任务表；仓库里没有 `ServerPagination`。

### 全站交互组件现状
- **确认**：
  - 3 处仍用浏览器原生的 `window.confirm`：`TaskAlertSection.tsx:90`、`MonitoringPolicyCenter.tsx:1680`、`MonitoringPolicyCenter.tsx:1846`；
  - 20 个文件用 `ui/alert-dialog`，1 处用 Arc `confirm-morph`（工作台定时侧栏的解除绑定，规范 §14 指定）；
  - sync 下有 5 个文件用普通 `ui/dialog` 承载确认，需要逐个判断是否属于纯确认。
- **下拉选择**：工作台、作业控制台、大盘里，`ui/select` 7 处、Arc `combobox` 2 处、`ui/command` 3 处；没有原生 `<select>`。
- **更多菜单**：`ui/dropdown-menu` 4 处，`ui/popover` 3 处（需核对是否被当菜单用）。
- **状态标签**：全站 `ui/badge` 78 个文件、172 处用法，其中 91 处手写了颜色 class，同一状态的配色、字号、有无底色都不一致，
  部分写法缺少深色适配。Arc `badge` 有 5 个文件，是规范 §14 指定的位置：工作台侧栏、作业详情、升级向导。
- **页签**：
  - 页面级导航用 `ModuleNavTabs`；
  - 详情页签按规范用 Arc `Tabs`；
  - 工具栏里的互斥筛选按规范用 `SegmentedControl`。

## Locked Decisions（用户确认）

1. **范围分两层**：
   - 5 个管理列表做**完整迁移**：列表骨架、筛选栏、行操作、交互组件。
   - 工作台、作业控制台、首页大盘及其他页面**只对齐交互组件**：确认弹窗、下拉框和选择器、行内更多菜单、状态标签。
     不动布局、列表骨架、数据流、轮询和深链。
2. **回归工具用 Egolite**（Mac 上的点击回归工具，可替代 Playwright，不局限于 CI）。
   每个阶段的 PR 附 Egolite 回归记录；现有 CI（vitest、`tsc`、已有 Playwright E2E）保持通过。不要求为本任务补 Playwright 用例。
3. **以 `ui-conventions.md` 已有的选型为起点**。规范没定的部分在 Phase 0 拍板后写回规范；要推翻规范已有的决定，必须写明理由并同步修改规范。
4. **不做**：移动端 Drawer 方案、审计批量操作、给 `confirm-morph` 开新用途、"万能表格"。
5. **状态标签用 Arc `badge` 的 `tone`**；非状态的中性标签用 `ui/badge` 的 `outline` / `secondary`；全站不再手写颜色 class。

## Requirements

### R1 交互组件统一选型（全局）
- 产出一张"场景 → 组件"的选型表：破坏性确认、表单弹窗、行内轻确认、下拉选择（少量 / 可搜索）、行内更多菜单、状态标签、
  页签（页面级 / 详情 / 工具栏筛选）、详情面板、分页、日期范围。每一项写明组件路径和规范出处。
- 状态标签已定（2026-11-18 用户确认）：表示状态的用 Arc `badge` 的 `tone`，非状态的中性标签用 `ui/badge` 的 `outline` / `secondary`，
  不再手写颜色 class。对照表见 design §3，规范见 `ui-conventions.md` §17。
- 选型结论写回 `ui-conventions.md`。

### R2 管理列表骨架统一（5 个页面）
- 抽出公共的表格卡片（容器、保底高度、加载条、骨架或空状态、分页底栏），替换 5 份拷贝，视觉与 §10 一致。
- 抽出受控查询状态：页码、每页条数、筛选条件；筛选变化统一回到第 1 页。审计和用户如确实漏了重置，一并修复。
- 行操作仍以按钮为主；单行动作超过 2 个时，次要动作收进 `DropdownMenu`，危险动作走 `AlertDialog`。
- 业务逻辑、接口、权限约束（如用户中心"不能操作自己"、告警状态流转）保持不变。

### R3 交互组件对齐（工作台、作业控制台、首页大盘及其他页面）
- 3 处 `window.confirm` 换成 `AlertDialog`。
- 用普通 `Dialog` 承载的纯确认改为 `AlertDialog`。带表单的保留 `Dialog`。
- 下拉框、更多菜单、状态标签按 R1 的选型替换。
- 这一层作为**单独的阶段和 PR** 交付，不和 R2 混在一起。

### R4 回归
- 每个阶段的 PR 附 Egolite 回归记录（格式见 implement.md），用例来自 design §6 的回归矩阵。
- 现有 CI 全部通过。

## Acceptance Criteria

- [ ] Phase 0 差异清单与选型表完成，开放问题全部拍板，选型已写回 `ui-conventions.md`。
- [ ] 5 个管理列表使用同一个表格卡片和查询状态 hook，不再有各自拷贝的卡片与分页底栏；筛选变化都回到第 1 页。
- [ ] 全站不再有 `window.confirm`；纯确认都用 `AlertDialog`。
- [ ] 全站没有手写颜色 class 的 `Badge`；状态都用 Arc `badge` 的 `tone`，映射符合 design §3 的对照表。
- [ ] 工作台、作业控制台、大盘的确认弹窗、下拉框、更多菜单、状态标签符合选型表；布局与数据流没有变化。
- [ ] 每个阶段的 PR 都附有 Egolite 回归记录且全部通过，现有 CI 通过。
- [ ] 新增和修改的文案都走 next-intl（中英文），可点击元素都有可读的 `aria-label`。

## Out of Scope

- 工作台、作业控制台、首页大盘的布局和列表骨架重做（刚完成统一）。
- 业务逻辑、后端接口、权限模型的修改。
- 移动端适配、审计批量操作。
- 主机、命令历史、安装包等其他列表的骨架迁移。这些页面本期只对齐交互组件，骨架是否迁移在试点完成后另行评估。
