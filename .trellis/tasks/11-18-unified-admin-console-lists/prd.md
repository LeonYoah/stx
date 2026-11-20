# 全局 UI 统一：管理列表迁移 + 全站交互组件对齐

## 背景

作业列表已在 `11-17-unified-job-table` / `11-17-job-detail-host` 统一。本任务要把**仍缺列表骨架**的管理页迁到共享 `TableCard` + 查询 hook，并把全站确认 / 下拉 / 溢出菜单对齐到 Arc 选型。

2026-11-20 产品收口（LeonYoah + stx UI 检查）：告警中心与诊断中心（#170）的列表 + 常驻详情已落地；大盘布局由 #168 完成。本任务范围相应收缩——不再重做这些页的骨架/布局，只补交互组件对齐与仍缺的审计 / 用户中心完整迁移。

## 已确认事实（2026-11-20，基于 main `918bfdc1` = #170 合入后）

### 管理列表与相关页

| 页面 | 文件（行数） | 骨架 / 详情现状 | 本期剩余 |
|---|---|---|---|
| 告警中心 | `monitoring/alerts/AlertsPanel.tsx`（1798） | 已是 list + `DetailHost`（J/K、窄屏 Drawer）；`TableCard`；状态 `SegmentedControl` **带服务端全集计数**；旧 `MonitoringAlertsCenter.tsx`（1384）已不存在 | 交互组件对齐；关闭告警行内 `ConfirmMorph`；可选接入 `useServerListQuery`（不重做布局） |
| 错误中心 | `diagnostics/errors/ErrorsPanel.tsx`（497） | #170：list + `DetailHost` + `TableCard`；热度 `SegmentedControl` **无分桶计数**（频次走服务端 min/max_occurrences） | 同上：组件对齐，非骨架迁移 |
| 巡检 | `diagnostics/inspections/InspectionsPanel.tsx`（1062） | #170：list + `DetailHost` + `TableCard`；状态用 Arc `Select`，**无当前页桶计数** | 同上 |
| 巡检策略 | `diagnostics/policies/PoliciesPanel.tsx`（697） | #170：list + `DetailHost` + `TableCard` + `DirtyCloseDialog`；状态分段计数来自**已加载全集**；部分弹窗已用 Arc `Dialog` | 组件对齐；复核确认路径（勿假设仍有 `window.confirm`） |
| 审计日志 | `AuditLogMain.tsx`（368）+ `AuditLogTable.tsx`（464）+ `AuditTraceSheet.tsx`（263） | 仍自建卡片/分页；追踪用 `ui/sheet`（临时覆盖，保留） | **完整列表骨架迁移**（试点） |
| 用户中心 | `UserManagement.tsx`（728）；令牌 `TokenManagement.tsx`（621，仍用 `StatPillsBar`） | 仍自建卡片/分页 | **完整列表骨架迁移** |

- **`TableCard` 已存在**：`common/layout/TableCard.tsx`（208 行，注释引用 design §4.4），告警 / 错误 / 巡检 / 策略 / 通知 / 规则等已在用。**禁止重建**。
- **`useServerListQuery` 尚不存在**；本期仍要抽出（审计 / 用户优先接入；已迁页可选接线）。
- **公共布局原语（#170 已升到 `common/layout/`）**：`DetailHost.tsx`（222）、`DirtyCloseGuard.tsx`（含 `DirtyCloseDialog`，65）、`UnsavedChangesDialog.tsx`（199）、`SoftChip.tsx`（56）、`list-detail.module.css`（623）。当作既有资产复用，不另造第二套。
- **通知 / 规则面板**（`NotifyPanel` 788、`DeliveriesPanel` 343、`RulesPanel` 1251、`JobRulesPanel` 682）已是 `TableCard` + `DetailHost`；本期不改布局，只随「同一区域一套组件」做必要对齐。
- `/sync` 仍是带深链参数重定向到 `/workbench`；回归时检查跳转即可。

### 大盘（#168 已合入，布局出本期范围）

| 资产 | 行数 | 说明 |
|---|---|---|
| `KpiStrip.tsx` | 190 | 7 格指标条 |
| `ThroughputTabs.tsx` | 178 | 吞吐格驱动图表，无分段控件 |
| `DashboardPanel.tsx` | 113 | 主/次面板 + 空态矮条 |
| `ClusterScopeStrip.tsx` | 150 | 替代 `RadioCards` |
| `HomeDashboard.tsx` | 361 | 头部集群范围 Arc `Combobox`；仍用 `[&_label]:sr-only` 临时隐藏 label（Combobox 在 main 上 label 已可选，Phase 6 清掉兜底并保证 aria-label） |
| `JobListTab.tsx` | 418 | 详情 `JobDetailHost`；**已删除**「全部/正式/预览」运行类型分段——**不要恢复** |

### `StatPillsBar` 存量（组件 178 行，已弃用）

仍在用：`TokenManagement`、`JobTableToolbar`（×2）、`ClusterMain`、`HostMain`、`ClusterConfigs`、`ClusterDetail`（×2）、`PluginMain`、`PackageMain`、`TroubleshootingMemoryCenter`。告警 / 错误 / 巡检管理列表已不再使用。本期迁走令牌 + 作业工具栏；其余随下期页面。

### 确认弹窗

- 文档曾列的 3 处 `window.confirm`（`TaskAlertSection` / 旧 `MonitoringPolicyCenter`）在当前 main **已不存在**（全库仅 `PackageMain` 注释提及）。开工前再 grep 复核；巡检策略删除 / 脏关闭已走 `DirtyCloseDialog` / Arc 确认，勿重复「替换 window.confirm」。
- 仍要清理：用普通 `Dialog` 承载的纯确认 → `AlertDialog`；告警「关闭」补 `ConfirmMorph`。

## Locked Decisions（含 2026-11-20 收口，不再重开）

1. **范围分两层**：
   - **完整列表骨架迁移**（本期）：审计（试点）、用户中心。
   - **仅交互组件对齐**（不改布局 / 数据流）：工作台（含底部控制台）、首页大盘、告警中心、错误 / 巡检 / 巡检策略（及已 list+detail 的监控子面板）。
2. **管理列表详情标准 = `DetailHost` list + 常驻详情**（J/K、窄屏 Drawer）。`ui/sheet` 仅临时覆盖（如审计追踪）。吸收 #170 的 `common/layout` 原语，不发明第二套。
3. **Phase 4 / 5 不再做骨架迁移**：错误 / 巡检 / 策略（随 #170）与告警（已在 main）的 list+detail 已完成；剩余是 Select / Combobox / DropdownMenu / 确认与 `StatPillsBar` 政策下的对齐，可选接线 `useServerListQuery`。
4. **大盘布局出范围**（#168）。Phase 6 剩：确认 / 下拉 / 溢出菜单；去掉头部 `sr-only` 并保证 Combobox 可访问名称；规范已收录指标条 / 吞吐页签 / 集群范围条。不恢复「全部/正式/预览」分段。
5. **筛选计数**：错误 / 巡检不恢复当前页桶计数；无服务端（或客户端全集）分桶则分段可保留但不显示计数。告警仅在已有服务端全集统计时显示计数。
6. **回归用 Egolite**；现有 CI 保持通过；不强制为本任务补 Playwright。
7. **以 `ui-conventions.md` 为起点**；推翻已有决定须改规范。
8. **不做**：移动端专用 Drawer 方案（窄屏走 `DetailHost` 已有 Drawer）、审计批量操作、「万能表格」。
9. **Badge**：保留 shadcn `ui/badge`，不做向 Arc `Badge` 批量迁移；§8/§14 已用 Arc `Badge` 维持现状。
10. **视觉**：Arc 桌面后台收紧版 Token（8–10px / 32–28px）；防框框堆叠；同一区域不混用 Arc 与 shadcn（`ui/badge` 豁免）。
11. **`StatPillsBar` 全站退役**；本期迁令牌 + 作业工具栏 2 处，其余下期。
12. **a11y（stx UI 检查）**：Combobox / Select 标签可可选，但去掉可见标签后必须仍有可见标签或 `aria-label`；控件不得缺少可访问名称。

## Requirements

### R1 交互组件统一选型（全局）
- 场景 → 组件表（确认、表单弹窗、行内轻确认、下拉、可搜索下拉、更多菜单、状态标签、页签、详情宿主、分页、日期）写回 `ui-conventions.md`（已在 §3/§5/§8/§9/§10）。
- Badge 决策见 Locked #9。

### R2 管理列表骨架（收缩后）
- **新建**：仅 `useServerListQuery`（`TableCard` 已存在，禁止重建）。
- **完整迁移**：审计、用户（含令牌 `StatPillsBar` → `SegmentedControl`；计数仅在有可靠全集统计时显示）。
- **已 list+detail 页**：不重做骨架；按需接线 hook + 组件对齐。
- 行操作：≤2 直接按钮；更多进 `DropdownMenu`；危险走 `AlertDialog` / `ConfirmMorph`（§3.4 规则）。
- 业务逻辑、接口、权限不变。

### R3 交互组件对齐层
- 工作台、大盘、告警、错误 / 巡检 / 策略：确认、下拉、溢出菜单按选型对齐；大盘清 `sr-only` 并保证 aria 名称。
- 作业工具栏 `StatPillsBar` → `chip-group` 多选 + 计数（`with_counts`）；URL 多值兼容旧单值。
- 其他页面（集群 / 主机等）本期只处理若仍存在的 `window.confirm`；表单整页 Arc 化下期。
- 单独阶段 / PR，不与 R2 混交。

### R4 回归
- 每阶段 PR 附 Egolite 记录；CI 通过。

## Acceptance Criteria

- [ ] Phase 0 差异清单与选型表完成；开放问题已拍板并写回 `ui-conventions.md`。
- [ ] 审计、用户使用共享 `TableCard` + `useServerListQuery`；筛选变化回第 1 页。
- [ ] 告警 / 错误 / 巡检 / 策略保持 list+`DetailHost`，无布局回退；其交互组件符合选型。
- [ ] 全站无 `window.confirm`；纯确认用 `AlertDialog`；管理列表常驻详情不走 `ui/sheet`。
- [ ] `ui/badge` 未做批量 Arc 化；`StatPillsBar` 本期约定 6 处中管理列表侧已清、令牌 + 作业工具栏已换。
- [ ] 错误 / 巡检筛选无当前页桶计数；告警计数仅来自服务端全集统计。
- [ ] 大盘布局未被本任务改动；头部 Combobox 有可见标签或 `aria-label`；未恢复运行类型分段。
- [ ] 工作台 / 大盘确认、下拉、更多菜单符合选型；布局与数据流无变化。
- [ ] 本期涉及区域内不混用 Arc 与 shadcn（`ui/badge` 豁免）。
- [ ] 每阶段 Egolite 通过；现有 CI 通过；文案走 next-intl，可点击控件有可读可访问名称。

## Out of Scope

- 大盘 / 工作台布局与列表骨架重做；告警 / 错误 / 巡检 / 策略的 list+detail 骨架重做。
- 业务逻辑、后端接口、权限模型修改（错误频次服务端筛选等 #170 已做部分除外，本任务不扩展）。
- 移动端专用方案、审计批量操作、万能表格。
- 集群 / 主机 / 安装包 / 插件等表单与对话框整页 Arc 化；对话框外框是否统一 Arc `Dialog` 另行评估。
- 其余 `StatPillsBar`（集群、主机、插件、安装包、排障记忆等）随下期迁移后删除组件。
