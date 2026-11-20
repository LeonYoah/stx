# 巡检与自动巡检合并、移除旧上下文条

## 背景（用户反馈）

1. 「巡检」和「巡检策略」看不出关系：自动巡检策略存在的意义是自动发现问题，但现在的呈现像一份普通配置列表。用户拍板：**合并为一个巡检标签**，内部用分段切换「巡检记录 | 自动巡检」（同通知页渠道/投递记录）。
2. 「清除上下文」「返回集群详情」、来源/告警徽章是旧交互产物，整行上下文条移除；集群筛选改为各面板工具栏里的可搜索 Combobox（与告警中心一致）。

## 需求

### 后端
- B1 `ClusterInspectionReport` 增加 `auto_policy_id`（index，默认 0），AutoMigrate 建列；`triggerAutoPolicyInspection` 处与 `auto_trigger_reason` 一起落库；`migrator.go` 加一次性回填（解析存量 `auto_trigger_reason LIKE 'policy:%'` 的 `policy:<id>` 前缀）。
- B2 报告列表新增 `auto_policy_id` 过滤（`>0` 生效），handler 解析 + `applyInspectionReportFilters`；CLI registry 给 report list 加 `auto_policy_id` `InputQuery`（`trigger_source` 若未暴露也补上）。
- B3 自动策略列表响应每项补 `last_triggered_at`（从未触发为 null）与 `trigger_count_7d`；repo 一次聚合查询（按 `auto_policy_id` 分组取 max(created_at) 与近 7 天 count），不逐条查询。
- B4 Go 测试：回填、过滤、聚合统计、registry 覆盖。

### 前端
- F1 `DiagnosticsWorkspace`：标签变为 错误 / 巡检 / 经验库；旧 `?tab=policies` 自动跳 `?tab=inspections&view=auto`；整行上下文条删除（集群下拉、徽章、返回集群详情、清除上下文及相关代码）。`cluster_id` URL 参数保留为唯一筛选真相；脏表单拦截改为 `activeTab==='inspections'` 时生效。
- F2 新增 `diagnostics/DiagnosticsClusterFilter.tsx`：Arc Combobox（仿 `monitoring/alerts/AlertsPanel.tsx` 用法），`value`/`options`/`onChange` 由 props 传入；各面板工具栏末端挂载（errors / inspections / memories / auto 视图）。经验库若已有自筛选则对齐复用，没有则通过可选 prop 挂载，不破坏其他页面复用。
- F3 `InspectionsPanel` 顶部加分段切换「巡检记录 | 自动巡检」（`view` URL 参数，默认 records 省略）；视图切换遇表单脏态走 `DirtyCloseDialog`；`policy_id` 深链可直接打开规则详情。
- F4 巡检记录行：`trigger_source==='auto'` 的行显示 `自动 · <规则名>` SoftChip，点击跳自动巡检视图并打开该规则详情；`auto_policy_id` 过滤生效时工具栏显示可移除的筛选徽章。面板加载策略名映射（`listAutoPoliciesSafe`）。
- F5 自动巡检列表行改为"读得懂的作用"：名称（+失效徽标）/ 一句话描述（`当 <条件> 时，自动巡检 <范围>`；`SCHEDULED` 为 `按 <cron>`；`ALERT_FIRING` 用规则名；`auto_create_task` 时追加 `，并生成诊断包`）/ 最近触发（ago，从未触发显示 `-`）/ 近 7 天 / 启用开关。详情底部加「查看触发记录」。描述生成器放 `policy-describe.ts`（或并入 `policy-form-utils`）并单测。
- F6 i18n 中英；删死键；E2E：更新 policies spec 的 URL 到 `tab=inspections&view=auto`，inspections spec 补视图切换 + 自动行跳规则 + 无上下文条断言，错误面板 spec 补工具栏集群筛选。

## 验收标准
- [ ] `/diagnostics?tab=policies` 落点为巡检-自动巡检视图；页面无上下文条。
- [ ] 规则行能读出"何时自动巡检什么 + 上次什么时候触发了"；记录行能看出哪条规则触发的。
- [ ] go test / tsc / vitest / eslint / translation-coverage / 三个诊断 E2E 通过。

## 不在范围
- `InspectionDetailPage` 独立页；策略表单内容本身不改；CLI 交互输出格式。
