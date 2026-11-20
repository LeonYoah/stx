# 实施：巡检记录清理

工作目录：`/Users/mac/Documents/projects/stx-pr-alert-model`（分支
`fix/diagnostics-alert-catalog-align`，PR #170 继续叠加）。

## 步骤

### 1. 设置模型与存储

- [ ] `internal/apps/diagnostics/inspection_models.go`（或新
      `cleanup_models.go`）：`InspectionCleanupSettings`（`diagnostics_cleanup_settings`
      单行表：retention_days / max_reports / updated_at）。
- [ ] `internal/db/migrator/migrator.go` `managedModels()` 注册。
- [ ] repository：`GetInspectionCleanupSettings`（无行返回零值）、
      `SaveInspectionCleanupSettings`（id=1 upsert）。
- [ ] service：`Get/SaveInspectionCleanupSettings` + 非负/上限校验。

验证：`go test ./internal/apps/diagnostics/ -run CleanupSettings`

### 2. 级联删除

- [ ] repository：`ListDiagnosticTasksByInspectionReportIDs(ids)`（方言三套，
      复用 `GetLatestDiagnosticTaskByInspectionReportID` 的拼接）；
      `DeleteInspectionReportsCascade(ctx, reportIDs, taskIDs)` 事务内按序删
      logs→nodes→steps→tasks→findings→reports。
- [ ] service `DeleteInspectionReports(ctx, ids)`：逐条判定
      not_found / skipped_running / skipped_task_running / deleted；
      汇总可删集合走事务；提交后 RemoveAll bundle 目录（前缀校验）；
      返回 `[]InspectionDeleteResult`。
- [ ] 单测：级联完整性、两个守护分支、bundle 目录移除（t.TempDir +
      Storage.BaseDir 覆盖）。

验证：`go test ./internal/apps/diagnostics/ -run 'DeleteInspection|Cascade'`

### 3. sweep + 僵尸收尾

- [ ] `auto_policy_runtime.go`：`cleanupSweepInterval = 10*time.Minute`；
      `Service.lastCleanupSweep`；每轮评估尾部按需调
      `sweepExpiredInspectionReports`（读设置→到期∪超量→去重→限量→
      `DeleteInspectionReports`；跳过原因记 debug 日志，失败记 warn 不中断）。
- [ ] `ReconcileInterruptedDiagnostics(ctx)`：runtime goroutine 第一轮评估前调一次；
      按 design §3.5 的条件更新把非终态报告置 failed、非终态任务（含 steps /
      node executions）置 failed/cancelled。
- [ ] 单测：僵尸报告/任务被收尾且 updated_at 新的不受影响。
- [ ] 单测：用可注入 now 或直接调内部方法验证候选集合计算。

验证：`go test ./internal/apps/diagnostics/ -run Sweep`

### 4. API + 登记 + 审计 + 路由

- [ ] handler：`GetInspectionCleanupSettings` / `PutInspectionCleanupSettings` /
      `BatchDeleteInspectionReports`（ids 非空、≤500）；后两个走
      `beginDiagnosticsSynchronousWrite`（module `diagnostics_inspection_cleanup`，
      RiskR1 + impact 文案常量）+ `finishDiagnosticsSynchronousWrite`；
      `writeExistingDiagnosticsSynchronousWrite` 补两个 op 的幂等返回分支；
      batch-delete 加 `audit.RecordFromGin` 一条。
- [ ] `internal/operation/registry.go`：`diagnostics.inspection-cleanup.get`（读）、
      `diagnostics.inspection-cleanup.update`（PUT，GeneratedCLI=true + InputBody）、
      `diagnostics.inspection.batch-delete`（POST，GeneratedCLI=false、无
      CommandPath/Example）。
- [ ] router.go diagnostics group：GET/PUT `inspection-cleanup-settings`，
      POST `inspections/batch-delete`。

验证：`go build ./...` + curl 三个端点（本地 17800）+ 
`stx diagnostics inspection cleanup get` 生成命令可用。

### 5. CLI

- [ ] 登记表生成即得 `cleanup get|set`，无 cobra 代码；跑
      `internal/cmd` 的登记表一致性测试确认登记合法。

验证：`go test ./internal/cmd/ -run Diagnostics`、`go test ./internal/operation/`。

### 6. 前端

- [ ] `services/diagnostics`：types + `getCleanupSettings` / `putCleanupSettings` /
      `batchDeleteInspectionReports`。
- [ ] `InspectionsPanel.tsx`：Checkbox 列（页内全选 indeterminate）、
      `Set<number>` 选中态、批量条（删除 + 清除）、确认弹窗、结果 toast、
      删后刷新与详情清理。
- [ ] `list-detail.module.css`：batchBar* 三样式（照搬 monitoring.module.css 结构）。
- [ ] 新 `CleanupSettingsDialog`（或内联小 Dialog）：两个数值输入 + 保存。
- [ ] i18n：zh/en 新 key（cleanup*、selected、batchDelete、结果文案、确认文案、
      设置表单）。

验证：`pnpm exec tsc --noEmit`、`pnpm test` 相关文件、
`npx playwright test e2e/diagnostics-inspections-template.spec.ts`。

### 7. 收尾

- [ ] 新增关键逻辑按注释约定写中英注释（只写 why）。
- [ ] spec 自检（backend/frontend checklist）；如有「巡检记录不可删除」类旧约定要同步。
- [ ] `go test ./...`（仓库根）全量回归。
- [ ] 提交（中文 message，参照最近 commit 风格），推到 #170 分支。

## 回滚点

- 每步独立可回滚；级联删除是全功能核心，若上线前发现风险可只保留设置 API 与 UI、
  摘掉 sweep 挂载（settings 为 0/0 时 sweep 本来也是 no-op）。
- 表迁移是增表，回滚不需要 down migration（新表可遗留）。

## 风险

- 方言 SQL 拼接错误 → repo 单测覆盖三种 dialect 分支中最常用的 sqlite/mysql 两种拼法
  （测试库是 sqlite，postgres 分支仅靠代码走查，与现有
  `GetLatestDiagnosticTaskByInspectionReportID` 同级别风险）。
- bundle 目录误删 → RemoveAll 前强制路径前缀校验。
- 长事务 → 单批限量（接口 ≤500，sweep ≤200）。
