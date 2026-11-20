# 设计

## 后端

- `internal/apps/monitoring/alert_rule_catalog.go`
  - `AlertRuleCatalog(lang)`：遍历 `clusterRuleCatalog`（英文取 `NameEN`，否则 `Name`），`category` 由 `Engine` 派生（`tsdb` → `AlertSourceTypeMetric`，其余 → `AlertSourceTypePlatformHealth`），`Scope=clusterRuleScope`；随后追加 `syncalert.TaskRules()`（去重，`Category=job`，`Scope="job"`，名称沿用 `syncAlertRuleName/EN`）。
  - 修正文档注释：不再提“通知路由 rule_key”，说明来源为集群目录 + sync 任务规则。
  - 若 `allAlertPolicyTemplateSummaries` 仍被 `metric_evaluator.findAlertPolicyTemplateSummary` 等引用则保留，本任务不删。
- `internal/apps/diagnostics/auto_policy_models.go`
  - 新常量 `AutoPolicyInvalidUnknownAlertRuleKey = "unknown_alert_rule_key"`。
  - `invalidInfo()`：在 `alert_firing_without_rule_keys` 判断之后，对 `ALERT_FIRING` 规则键逐个查 `alertRuleCatalogKeySet()`，首个未知键返回 `(unknown_alert_rule_key, key)`；更新 `InvalidReason` 字段与函数的双语注释。
  - `alertRuleCatalogKeySet` 被多次调用，可接受（目录约 28 项）；无需缓存。

## 前端

- `lib/services/monitoring/types.ts`：`AlertRuleCatalogItem` 增加 `scope: 'cluster' | 'job'`。
- `AutoPolicyConfigPanel.tsx`
  - 分组改为 `(['cluster','job'] as const)`，按 `item.scope` 过滤，标题用 `useTranslations('monitoring')` 的 `scopeCluster/scopeJob`；删除只服务三类分组的 `getRuleCategoryLabel` 与不再使用的 `tAlerts`/`AlertSourceType` 导入（若无其他用途）。
  - 已选徽章：按 `rule_key` 查目录，命中显示 `name`（`title=rule_key`）；未命中显示原始键 + 警示色（`variant='destructive'` 或现有警示 token）并带移除按钮/可点击取消（调用 `handleConditionRuleKeyToggle(..., false)`），`aria-label` 走 i18n。
  - `INVALID_REASON_CODES`：去掉 `legacy_source_type`，加入 `unknown_alert_rule_key`。
- 语言包 `zh/en`：删除 `legacy_source_type` 文案，新增 `unknown_alert_rule_key`（如“告警规则 "{detail}" 已不在规则目录，请编辑并重新选择。”/ "Alert rule \"{detail}\" is no longer in the rule catalog; edit and reselect."），以及未知键移除按钮的 aria 文案。

## 兼容性

- API 字段只增不减（`category` 保留）；旧客户端按 category 分组仍可用。
- 存量含 `stx_*` 的策略：之前静默不触发，现在显式标失效，行为未变但可见。
