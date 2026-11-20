# 设计

## 后端
- `inspection_models.go`：`ClusterInspectionReport.AutoPolicyID uint \`json:"auto_policy_id,omitempty" gorm:"index;not null;default:0"\``；filter 加 `AutoPolicyID uint`。API 响应模型（若有独立 Info 结构）同步该字段。
- `auto_policy_repository.go`：`UpdateInspectionReportAutoTriggerReason` 扩为同时写 `auto_policy_id`（改名 `SetInspectionReportAutoTrigger(ctx, reportID, policyID uint, reason string)`，checker 同步改）；新增 `AutoPolicyTriggerStats(ctx) (map[uint]AutoPolicyTriggerStats, error)` —— `SELECT auto_policy_id, MAX(created_at) FROM reports WHERE auto_policy_id>0 GROUP BY` + 近 7 天 count 同组查询；`AutoPolicyTriggerStats{LastTriggeredAt time.Time; TriggerCount7d int}`。
- 策略列表响应项（`auto_policy_models.go` 的响应结构，和 `invalid_reason` 同一出口）加 `last_triggered_at,omitempty` / `trigger_count_7d`；service 装饰时填。
- `handler.go`：report list 解析 `auto_policy_id`（uint>0，非数字沿用现有 400 风格）。
- `repository.go` `applyInspectionReportFilters`：`auto_policy_id>0` → Where。
- `migrator.go`：`backfillDiagnosticsReportAutoPolicyIDs(database)`，参照 `deleteLegacyCustomPromQLAlertPolicies` 风格放 MigrateWithDB 调用链里：查 `auto_trigger_reason LIKE 'policy:%' AND auto_policy_id=0`（LIMIT 500 批处理），`strings.TrimPrefix` + `strconv.Atoi` 后 Update。
- `operation/registry.go`：report list op 的 inputs 加 `auto_policy_id`；检查 `trigger_source` 是否已暴露，没有则补。

## 前端
- `DiagnosticsClusterFilter.tsx`：`components/arc/combobox`（抄 AlertsPanel 的 options 组装），宽度 ~160px，「全部集群」选项在前；受控 props。
- Workspace：`resolveTab` 里 `policies→inspections`（并在 effect 里把 `view` 补 `auto`）；`DiagnosticsTabKey` 去掉 `policies`；`updateQuery` 切 tab 时带 `view`。删除上下文条 JSX、`entrySourceLabel`/`hasWorkspaceContext`/`selectedClusterName` 等仅其使用的代码；`selectedClusterId`/`updateQuery`/`rememberPreferredClusterId`/`isSoleDefaultCluster` 保留给各面板。`onClusterChange` 传给面板。
- `InspectionsPanel`：props 加 `view`/`policyId`/`autoPolicyFilter` + 回调（或内部读 URL——统一沿用 workspace prop 模式）；工具栏最左 `SegmentedControl`（layoutGroup 与 header tabs 互斥时注意：不在同一挂载点，检查需不需要 `layoutGroup`）；records 视图工具栏 = 状态段 + 严重度 + 集群 + auto_policy 筛选徽章 + 发起巡检 + 刷新；auto 视图渲染 `policies/PoliciesPanel`（props 加 `focusPolicyId`、`onViewRecords`）。
- `PoliciesPanel`：行重排为 名称+句 / 范围 / 最近触发 / 近7天 / 开关；`data-mcols='policies'` 列模板随之改；DetailHost 底部加「查看触发记录」按钮调 `onViewRecords(id)`；`focusPolicyId` 到达时 requestEdit(id)。
- `policy-describe.ts`：`describePolicy(policy, {templates, ruleCatalog, t})` 返回 `{conditionText, scopeText, bundleText}`；模板名用 `tpl` 的本地化字段（先确认 `InspectionConditionTemplate` 的展示字段名）；ALERT_FIRING 用 `rule_keys` 反查 catalog 名称；SCHEDULED 取 cron 表达式。
- i18n 键放 `diagnosticsCenter.inspections.view*`、`autoPolicies.*`、`common` 复用。

## 测试
- Go：`auto_policy_stats`（repo 聚合）、filter/handler、backfill（migrator 单测风格对齐）、registry。
- Vitest：`policy-describe` 各分支、InspectionsPanel 视图切换/深链/auto chip、PoliciesPanel 行渲染（最近触发/7天/句子）。
- E2E：三个诊断 spec 更新 + 新增覆盖点（见 PRD F6）；helper 加 `auto_policy_id`、`last_triggered_at` 桩数据。
