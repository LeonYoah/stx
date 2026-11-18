# 实施计划：告警模型收拢

单任务完成（D7），按阶段推进；每阶段结束跑该阶段的验证命令，通过后再进入下一阶段。

## 开工前

- [ ] `task.py start 11-18-alert-model-consolidation`
- [ ] 确认工作区：当前分支 `feat/list-ui-unification` 上有未提交的 `TaskAlertForm.tsx` / `TaskAlertSection.tsx` 改动，开工前与用户确认是否先提交或另开分支。
- [ ] 读 spec：`backend/index.md`、`database-guidelines.md`、`error-handling.md`、`cli-guidelines.md`、`quality-guidelines.md`、`logging-guidelines.md`、`sync-alerts.md`、`embedded-tsdb-*.md`；`frontend/index.md`、`ui-conventions.md`、`api-and-services.md`；`guides/cross-layer-thinking-guide.md`。

## 阶段 1：规则目录与配置解析（后端，无行为变化）

- [ ] `cluster_rule_catalog.go`：`ClusterRuleDefinition` 与目录（design §2），单测断言默认开关 / 阈值 / 持续时间与 PRD D4、D6 一致。
- [ ] `cluster_rule_config.go`：三张表实体 + repository；加入 `migrator.go` `managedModels()`。
- [ ] `ResolveClusterRule` + 缓存 + 失效；单测覆盖“目录默认 → 全局 → 集群例外”逐字段继承、`nil` 重置。
- [ ] `resolveRuleNotification` + `resolveRuleSeverity`；单测覆盖 default / custom / none / 默认通知为空，以及 warning 不投外部、critical 校验规则（无启用渠道拒绝保存，含例外层）。
- 验证：`go test ./internal/apps/monitoring/... ./internal/db/...`

## 阶段 2：判定接线与去重

- [ ] tsdb：`buildManagedRuleGroups` 从目录生成全局 rule + 例外 rule；删除 YAML `seatunnel-default-alerts` 组（保留 recording 组）。单测：例外集群被全局 rule 排除；阈值换算正确；未启用不生成。
- [ ] event：`DispatchAlertPolicyEvent` / `DispatchActiveNodeOfflineReminder` 改用 `ResolveClusterRule`。
- [ ] sync 集群级：`SyncAlertHub.clusterRule` 改用解析结果；`sync_alerts_test.go` 保持通过。
- [ ] 投递：托管告警按 `rule_key` label 解析；删除路由回落 `deliverRemoteAlertNotifications`。
- 验证：`go test ./internal/apps/monitoring/...`

## 阶段 3：可用性评估器

- [ ] `availability_evaluator.go`：节点健康（进程侧原因 + `up` / `node_state` 即时查询）、`for` 语义、恢复。
- [ ] `node_offline` 原因合并；`host_offline` 移到 `agent_offline`（按主机聚合）。
- [ ] `master_unavailable`、`worker_insufficient`（阈值 nil = 部署 worker 数；separated / hybrid 角色计数）。
- [ ] 单测：三原因同时成立只一条；Agent 掉线 + `up==1` 只出 Agent 离线；Master 全挂触发与恢复；Worker 少一台触发；持续时间未满不触发、中途恢复清零；启动抑制沿用 `shouldSuppressNodeOfflineDuringStartup`。
- 验证：`go test ./internal/apps/monitoring/...`

## 阶段 4：接口、scope 与 CLI

- [ ] handler + router：design §8 新接口；错误按 `error-handling.md` 映射 400/404。
- [ ] `scope` 派生字段：告警实例 DTO / 列表筛选、规则目录。
- [ ] `/alert-policies` POST/PUT 仅接受 `sync_job`；删除 `promql` 请求字段；删除 `notification-routes`、`clusters/:id/rules`、`alert-policies/bootstrap` 路由及 service / repository 中无调用方的代码；删除 legacy bridge。
- [ ] CLI：`internal/cmd/monitoring_write.go`、`internal/operation/registry.go` 增删；`monitoring_write_test.go`、`validate_test.go` 同步；`stx monitoring --help` 自检。
- 验证：`go test ./...`

## 阶段 5：迁移

- [ ] `internal/db/migrator/alert_rule_catalog_migration.go`（`SchemaFlag` `alert_rule_catalog_v1`，事务），按 design §10；在 `migrator.go` 中于 AutoMigrate 之后调用。
- [ ] 迁移单测：全局 / 集群策略、冲突取舍、ratio→percent、`AlertRule` 改动转例外、路由三种情形、默认通知并集兜底、`sync_job` 不动、重复执行幂等。
- 验证：`go test ./internal/db/...`；`./scripts/test_db_compat.sh`

## 阶段 6：前端（Arc 优先，直接替换）

- [ ] 读 `.claude/skills/arc/SKILL.md`、`components.md`、`composition.md`、`copy.md`、`checklist.md`；每个组件先 `get_component` 核对 props。
- [ ] 补装 Arc 组件（design §9.5），核对 License 头与 `licenses/third_party_mit_files.txt`。
- [ ] `lib/services/monitoring`：新增 cluster-rules / notification-defaults 方法与类型；删除 routes、cluster rules、`promql`、bootstrap 相关。
- [ ] 新建组件（`components/common/monitoring/`）：`MonitoringWorkspace`（Arc tabs + 深链映射）、`alerts/`（指标卡、工具栏、表格、详情 drawer）、`rules/`（集群级表格、规则编辑 drawer、集群例外、作业级表格）、`notify/`（默认通知卡、渠道表格与 drawer）。
- [ ] 删除旧实现：`MonitoringCenterWorkspace.tsx`、`MonitoringPolicyCenter.tsx`、`MonitoringAlertsCenter.tsx` 及其仅被它们引用的工具与测试；旧路由页改为重定向到新 tab。
- [ ] `TaskAlertForm` 内部控件换 Arc，props 与保存接口不变；回归任务设置页（注意当前分支上该文件有未提交改动，开工前处理）。
- [ ] i18n zh / en；遵循 Arc copy 规则与 `ui-conventions.md`（零塌陷、防被迫换行、渐进披露）。
- [ ] 单测：`__tests__` 覆盖深链映射、继承显示（全局 / 例外）、通知模式切换、例外增删、告警筛选参数构建与 page 重置。
- [ ] Arc `checklist.md` 自检；Egolite 回归矩阵（design §9.6）逐项执行并记录。
- 验证：`cd frontend && pnpm test && pnpm exec tsc --noEmit`

## 阶段 7：文档与收尾

- [ ] `docs/内置监控说明.md`：告警类别改为集群级 / 作业级；规则目录与默认开关；通知层级。
- [ ] `docs/release-notes/unreleased.md`：升级行为变化（design §11）。
- [ ] `.trellis/spec/backend/sync-alerts.md` 与新增规则目录 spec（经 `trellis-update-spec`）。
- [ ] 全量验证：`go test ./...`、`(cd agent && go test ./...)`、`./scripts/test_db_compat.sh`、`cd frontend && pnpm test && pnpm exec tsc --noEmit`。
- [ ] 中文提交。

## 风险与回滚点

| 风险 | 位置 | 缓解 |
|---|---|---|
| 迁移误转或丢配置 | 阶段 5 | 旧表旧行保留；事务 + SchemaFlag；迁移摘要日志；单测覆盖每种来源 |
| TSDB 规则组生成错误导致全部指标告警失效 | 阶段 2 | 沿用“重建失败沿用上一轮规则集”；单测校验每条规则表达式可被 `parser.ParseExpr` 解析 |
| 可用性评估误报（启动期 / 安装中节点） | 阶段 3 | 沿用 `shouldSkipNodeOfflineEvaluation` 与启动抑制；单测 |
| 删接口影响外部脚本 | 阶段 4 | release notes 列出移除的接口与 CLI 及替代命令 |
| 前端大文件改动冲突 | 阶段 6 | 拆分新组件，`MonitoringPolicyCenter.tsx` 只做删除与组装 |
