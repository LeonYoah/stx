# 诊断中心对齐告警规则目录

## 背景

告警中心重构（`feat/alert-model-consolidation`）后，集群级规则改以 `cluster_rule_catalog.go` 代码目录为唯一来源，作业级规则仍来自 `syncalert.TaskRules()`；旧 `metrics_template` / `platform_health` 策略在 `alert_rule_catalog_v1` 迁移后全部停用，`ListEnabledManagedMetricAlertPolicies` 已无生产调用方。

诊断中心自动巡检的 `ALERT_FIRING` 条件仍从 `GET /monitoring/alert-rules/catalog`（`AlertRuleCatalog`）取规则键，而该目录还在用旧策略模板 `allAlertPolicyTemplateSummaries()` 拼装。

## 已确认问题（代码证据）

1. **目录漂移**：`AlertRuleCatalog` 与集群规则目录不一致（本地对比测试输出）。
   - 缺 `gc_time_high`、`coordinator_backlog_growing`：告警中心可开启，但巡检条件选不到。
   - 多出 5 条 `stx_*` 平台自监控模板（`stx_server_memory_high` 等）：迁移后不再评估，且无集群维度，`CheckAlertFiringPolicies` 按集群拉告警永远匹配不到——选了等于死条件。
   - 中文名走 `alertPolicyTemplateDisplayNameZH` / sync 名，与告警中心规则页使用的目录 `Name/NameEN` 来源不同。
2. **存量死条件无提示**：已保存的 `ALERT_FIRING.rule_keys` 若含目录外键（如上述 `stx_*`），`InvalidReason()` 不识别，列表不提示；用户编辑保存时才被 `validateAutoPolicyConditions` 以 `unknown alert rule key` 拒绝。
3. **分类口径不一致**：巡检规则选择器按 `platform_health / metric / job` 三类分组，而告警中心面向用户只分 **集群级 / 作业级**（`ui-conventions.md` §7、`cluster-alert-rules.md` §1）。
4. **前端残留失效原因码**：`AutoPolicyConfigPanel` 白名单仍含后端已不再产出的 `legacy_source_type`，语言包同名文案为死键。
5. 已选规则以原始 `rule_key` 徽章展示，未知键与有效键无法区分。

## 需求

- R1 `AlertRuleCatalog` 改为由 `clusterRuleCatalog`（集群级）+ `syncalert.TaskRules()`（作业级）生成，按目录顺序输出，名称取目录 `Name/NameEN` 与 sync 规则名；`category` 字段保留兼容（`tsdb` 引擎 → `metric`，其余集群引擎 → `platform_health`，作业 → `job`），`scope` 为 `cluster` / `job`。
- R2 新增失效原因码 `unknown_alert_rule_key`：`ALERT_FIRING` 条件含目录外规则键时返回，`invalid_detail` 为首个未知键；启动 WARN 扫描自动覆盖。
- R3 巡检规则选择器按 `scope` 分两组（集群级 / 作业级，复用 `monitoring.scopeCluster/scopeJob`）；已选徽章显示目录名称（`title` 带规则键），目录外键以警示样式显示原始键并可取消勾选。
- R4 前端失效原因白名单与语言包：移除 `legacy_source_type`，新增 `unknown_alert_rule_key`（中英）。
- R5 规范同步：`cluster-alert-rules.md` 记录 `AlertRuleCatalog` 派生契约；`platform-self-monitoring.md` §4 去掉“模板摘要随 catalog 暴露”的过期表述。

## 验收标准

- [ ] `AlertRuleCatalog("")` 的键集合 == 集群目录 21 键 ∪ `syncalert.TaskRules()`，不含任何 `stx_*`；有单测断言。
- [ ] 含 `stx_server_memory_high` 的存量 `ALERT_FIRING` 策略 `invalid_reason=unknown_alert_rule_key`、`invalid_detail=stx_server_memory_high`；有单测。
- [ ] 创建/更新策略选择 `gc_time_high` 通过校验。
- [ ] 前端选择器只出现“集群级 / 作业级”两组；未知键徽章可见且可移除；有组件单测。
- [ ] `translation-coverage` 测试、`tsc`、相关 vitest、`go test ./internal/apps/monitoring/... ./internal/apps/diagnostics/... ./internal/cmd/...` 通过。
- [ ] CLI：`stx monitoring alert-rule catalog` 端点不变，无需新增命令；确认 `stx diagnostics` 帮助文案无过期引用。

## 不在范围

- 不自动改写存量策略数据（沿用“只提示、引导编辑”约定）。
- 不处理“规则在该集群被关闭导致巡检永不触发”的提示（后续可单独做）。
- 不改 `matchAlertFiringCondition` 的匹配语义与告警中心 UI。
- 平台自监控 `stx_*` 告警是否恢复评估属于告警中心议题，不在本任务。
