# 设计：巡检记录清理

## 0. 现状锚点

- 巡检记录：`ClusterInspectionReport`（`diagnostics_inspection_reports`），
  `trigger_source` ∈ manual / cluster_detail / diagnostics_workspace / auto；
  `auto_policy_id` 记录来源策略（0 = 非自动）。
- 发现项：`ClusterInspectionFinding.report_id` → report。
- 诊断任务绑定：`DiagnosticTask.SourceRef.InspectionReportID`（JSON 字段，无 FK）。
  反查已有方言实现：`Repository.GetLatestDiagnosticTaskByInspectionReportID`
  （`task_repository.go`，sqlite `json_extract` / mysql `CAST(JSON_UNQUOTE…)` /
  postgres `->>…::bigint`），本任务扩展为 `IN (ids)` 批量版本。
- 任务子表：`diagnostics_task_steps` / `diagnostics_task_nodes` /
  `diagnostics_task_logs`，均按 `task_id` 关联。
- bundle 磁盘目录：`diagnosticTaskBundleDir(taskID)` =
  `config.Storage.BaseDir/diagnostics/tasks/<taskID>`（默认 `./data/storage`）。
- 后台循环：`StartAutoPolicyRuntime` → `runAutoPolicyEvaluationRound` 每 60s 一轮，
  挂在 `Service` 上，是唯一可复用的常驻循环。
- 单行全局设置先例：`monitoring.AlertNotificationDefaults`
  （`monitoring_alert_notification_defaults`，单 id 行 upsert）。
- 批量条 UI 先例：`alerts/AlertsPanel.tsx` 的 Checkbox + `Set<string>` +
  `batchBarWrap/batchBar`（`monitoring.module.css`）。
- 面板布局原语：`common/layout/list-detail.module.css`（row / rowHead / listCol）。
- 路由：`/api/v1/diagnostics` group（router.go:608-），inspections 现有
  POST/GET，无 DELETE。
- CLI：`internal/cmd/diagnostics.go`，`diagnostics inspection` 子命令组目前只有 `run`；
  `auto-policy` 组有 create/update（204/205 行附近为 update 写命令模式参考）。
- 审计：handler 持有 `auditRepo`，写操作用 `audit.RecordFromGin`（handler.go:707-722）。

## 1. 数据模型

新表 `diagnostics_cleanup_settings`（单行，同 AlertNotificationDefaults 模式）：

```go
type InspectionCleanupSettings struct {
    ID            uint      `json:"id" gorm:"primaryKey;autoIncrement"`
    RetentionDays int       `json:"retention_days"` // 0 = 不按时间清理
    MaxReports    int       `json:"max_reports"`    // 0 = 不按数量清理
    UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}
```

- 进 `managedModels()` 注册 AutoMigrate。
- 读取：无行时返回 `{0,0}`；更新：upsert id=1（首次插入）。
- 校验：两字段非负整数；加一个合理上限（如 retention_days ≤ 3650，max_reports ≤ 100000），
  防止误输天文数字。

## 2. API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/diagnostics/inspection-cleanup-settings` | 返回当前设置 |
| PUT | 同上 | `{retention_days, max_reports}`，校验失败 4xx |
| POST | `/api/v1/diagnostics/inspections/batch-delete` | `{ids: []uint}` → 逐条结果 |

批量删除响应用现有 `Response` 包装，data 形如：

```json
{
  "results": [
    {"id": 12, "result": "deleted"},
    {"id": 15, "result": "skipped_running", "reason": "report is running"},
    {"id": 18, "result": "skipped_task_running", "reason": "diagnostic task 77 is running"},
    {"id": 99, "result": "not_found"}
  ],
  "deleted": 1,
  "skipped": 3
}
```

- 逐条结果而不是整批失败：列表勾选常混合状态，前端要按条提示。
- 用 POST 而非 DELETE+body：项目里 DELETE 只用于路径参数单资源（DeleteAutoPolicy），
  批量走 POST 动作路由，与 `tasks/:id/start` 风格一致。
- **登记表**（`internal/operation/registry.go` diagnosticsReadOperationSpecs 里新增）：
  - `diagnostics.inspection-cleanup.get`：`diagnosticsReadOperation`，GeneratedCLI=true
    （免费得到 `stx diagnostics inspection cleanup get` 生成命令）。
  - `diagnostics.inspection-cleanup.update`：PUT 写操作，**GeneratedCLI=true**、
    `InputBody` → 生成 `--request-file`（照 `cluster.java-proxy.config` 先例），
    RiskR1（Idempotency-Key + X-STX-Confirm）→ 得到
    `stx diagnostics inspection cleanup set --request-file x.json --confirm`。
  - `diagnostics.inspection.batch-delete`：POST 写操作，**GeneratedCLI=false、
    CommandPath=nil、Example=""**——cli-guidelines §11 禁止 CLI 提供删除入口
    （改名绕过也违规），此操作只登记供服务端/网页/能力查询用。RiskR1 + Impact。
- **写操作过公共执行记录**：batch-delete 与 settings PUT 的 handler 都走
  `beginDiagnosticsSynchronousWrite`（module `diagnostics_inspection_cleanup`，
  RiskR1）+ `finishDiagnosticsSynchronousWrite`——web 请求自动 no-op
  （`ClientType=="web"` 跳过），CLI/直连 API 得到幂等 + 确认 + 执行记录；
  `writeExistingDiagnosticsSynchronousWrite` 补 batch-delete/settings 分支返回安全结果。
- **审计**：批量删除 handler 里 `audit.RecordFromGin` 一条
  （action=`diagnostics.inspection.batch-delete`，details 记 ids 与
  deleted/skipped 计数），模式同 `recordDiagnosticResourceAccess`。

## 3. 级联删除（核心）

新 service 方法 `DeleteInspectionReports(ctx, ids []uint) []InspectionDeleteResult` +
repo `DeleteInspectionReportsCascade(ctx, ids)`。流程：

1. 加载目标报告；不存在 → `not_found`；status ∈ {pending, running} → `skipped_running`。
2. 查绑定任务：`ListDiagnosticTasksByInspectionReportIDs(ids)`（方言批量反查，
   复用现有三套 SQL 的拼接方式）。任一绑定任务非终态
   （∉ succeeded/failed/cancelled/skipped）→ `skipped_task_running`。
3. 单事务删除（`Repository.Transaction` 已有）：
   `diagnostics_task_logs` → `diagnostics_task_nodes` → `diagnostics_task_steps` →
   `diagnostics_tasks`（绑定集合）→ `diagnostics_inspection_findings` →
   `diagnostics_inspection_reports`（可删集合）。
4. 收集被删任务的 `bundle_dir`，**事务提交后** `os.RemoveAll`，失败只记 WARN
   （行已删，目录残留由下次运维清理，不回滚事务）。

注意点：

- 绑定查询与删除必须在同一事务里做（先 SELECT 再 DELETE），否则 SELECT 到的任务集合
  与实际删除不一致。
- bundle 目录用 `filepath.Clean` + 前缀校验（必须落在
  `storage/diagnostics/tasks/` 下）再删，防止 DB 里 bundle_dir 被脏数据污染时误删。
- `executions` 行保留（操作历史，result_ref 悬挂可接受）。
- 删除 report 时若正被某个请求并发读取，允许读到半截状态——无 FK 约束，属既有并发语义，
  不加锁。

## 3.5 僵尸状态收尾（顺带修）

巡检评估是进程内同步执行（`StartInspection` 内联跑完）、诊断任务是
`go executeDiagnosticTask` 异步执行——进程一死，两类非终态对象都永久卡死，无人收尾
（sync 域有 `job_reconciler` 先例，diagnostics 没有）。

- 新增 `ReconcileInterruptedDiagnostics(ctx)`，在 `StartAutoPolicyRuntime` 的
  goroutine 里第一轮评估前调一次。
- 判定僵尸：`status` 非终态 **且**（`updated_at` 早于进程启动时刻，**或**
  `updated_at` 早于 now-24h）。前者收重启孤儿；后者兜底本进程内评估挂死
  超过一天的行。`updated_at` 用条件更新写回，避免误收尾新在途对象。
  - 报告：pending / running → failed，`error_message` 写「进程重启，巡检被中断 /
    Inspection interrupted by restart」（走 bilingualText 双语存储）。
  - 任务：pending / ready / running → failed（`failure_reason` 同上语义）；
    cancel_requested / cancelling → cancelled（取消意图本来就到了终态方向）。
    任务的 steps / node executions 中非终态行同步置为与任务一致的终态。
- 收尾后这些行进入终态，自动清理与手动删除的守护逻辑自然覆盖。
- 并发安全：reconcile 用条件更新（`WHERE status IN (...) AND updated_at < ?`），
  启动后新创建的在途行不满足 updated_at 条件，不会被误收尾。
- **源头止血**：僵尸的另一个来源是请求取消——`StartInspection` 拿着
  `c.Request.Context()` 跑同步评估，浏览器断开/代理超时会让最终状态写不进去
  （跨层指南错误八）。巡检执行本体改走 `context.WithoutCancel(ctx)`，
  与 `finishDiagnosticsSynchronousWrite` 的收尾上下文同一模式；
  `executeDiagnosticTask` 已用 `context.Background()`，无需再改。

## 4. 后台 sweep

- 挂在 `runAutoPolicyEvaluationRound` 尾部：`Service` 记 `lastCleanupSweep`，
  距上次 ≥ `cleanupSweepInterval`（10min，包级常量）才执行；启动后第一轮即跑。
- 步骤：读设置 → retention_days>0 时查 `created_at < now-retention` 的终态报告 id →
  max_reports>0 时按 created_at desc 取第 N 名之后的终态报告 id → 合并去重 →
  走同一 `DeleteInspectionReports`（自动带上运行中守护：sweep 撞上就跳过下轮再说）。
- sweep 失败只记日志不阻塞策略评估。

## 5. 前端

`inspections/InspectionsPanel.tsx`：

- 勾选列：`layout.rowHead` 与每行首格加 Arc `Checkbox`；head 为页内全选
  （indeterminate 同 AlertsPanel:935）；行内点击 `stopPropagation`，键盘可达性照
  AlertsPanel 做法（checkbox 参与 `data-mrow` 的交互元素豁免列表）。
- `selected: Set<number>`，换页/刷新/过滤变化后清掉不在当前页的选中
  （照 AlertsPanel：选择按 id 存，跨页保留更简单且已验证，直接沿用其语义——
  本处沿用 AlertsPanel 的「跨页保留选中」语义，批量条显示总数）。
- 浮动批量条：把 `batchBarWrap/batchBar/batchActions` 样式搬进共享
  `list-detail.module.css`（AlertsPanel 的留在 monitoring.module.css 不动，
  两套样式并存可接受；不抽公共组件，避免本任务顺手重构监控面板）。
  条内：已选 N 条、删除按钮、清除选择。
- 删除确认：复用 `common` 下确认弹窗模式（AlertsPanel 批量动作怎么确认就怎么确认，
  文案含级联说明「连带删除绑定的诊断任务与诊断包文件」）。
- 结果 toast：沿用 `@/lib/toast`；汇总 `deleted` 条已删、`skipped` 条跳过，
  skipped_task_running / skipped_running 原因行内列出前几条。
- 删除后：`loadReports()` 刷新；若被删的含 `selectedReportId`，清详情栏。
- 设置入口：toolbarRight 加「巡检记录清理」icon 按钮（Settings/Trash 类 Arc 图标），
  开一个小 Dialog 表单：两个 number-field（保留天数 / 最多份数，0=不清理）+ 保存。
  表单值 GET 拉、PUT 存；保存成功 toast 提示「下一轮自动清理生效」。
- i18n：`diagnosticsCenter.inspections.cleanup*` 一批新 key（中英两份）。

## 6. CLI

按 cli-guidelines §11：**CLI 不提供任何删除入口**（含改名绕过），批量删除只有
服务端 API + 网页。本任务 CLI 全部由登记表生成，不写 cobra：

- `stx diagnostics inspection cleanup get`（GET 生成命令，免费）。
- `stx diagnostics inspection cleanup set --request-file settings.json --confirm`
  （PUT + InputBody 生成命令，自动带 --confirm/--idempotency-key/--confirmation-id）。
- `diagnostics.inspection.batch-delete` 登记为 GeneratedCLI=false 且无
  CommandPath/Example，CLI 无对应命令；`diagnostics_test.go` 里有遍历登记表断言
  DELETE/删除类不生成命令的测试，登记方式与它兼容。

「每次新增接口要补充 cli 能力」的仓库约定在此处的合规形态就是：读/写接口登记进
registry 生成命令，删除接口登记为服务端专属。

## 7. 测试

- 后端单测（sqlite，沿用 *_test.go 既有 repo 测试套）：
  - cascade：报告+findings+绑定任务（含子表）全删，未绑定任务不动；
  - 守护：running 报告 skipped_running、绑定 running 任务 skipped_task_running；
  - sweep：过期/超量两个分支、0/0 不动、终态过滤；
  - 设置：默认 0/0、upsert、非法值拒绝。
- 前端：InspectionsPanel 勾选/全选/批量条/结果 toast 单测；
  设置 Dialog 读写单测；`e2e/diagnostics-inspections-template.spec.ts`
  补一条勾选删除用例。
- 手动验证（E2E 环境）：设 retention_days=1 造旧记录，等 sweep 或重启后查库 + 磁盘目录。

## 8. 边界与兼容

- 旧库升级：新表由 AutoMigrate 建，无需数据回填。
- bundle_dir 为空/目录已不存在的任务照常删行（RemoveAll 对不存在路径不报错）。
- 大批量：单次删除 id 数量加上限（如 ≤500），sweep 每轮也限批（如 ≤200），
  避免长事务。
- `InspectionDeleteResult` 的 `result` 枚举是 API 契约，前端按值分支显示。

## 9. 影响面清单（提交前自检）

- 后端：inspection_models.go（settings 模型）、repository.go（级联+方言查询）、
  inspection_report_service.go / 新 cleanup_service.go（编排+sweep 守护+reconcile）、
  handler.go（3 个 handler + 审计 + 公共执行记录接线）、write_handler.go
  （existing 分支）、router.go（路由注册）、auto_policy_runtime.go（sweep+reconcile
  挂载）、migrator.go（注册模型）、operation/registry.go（3 条登记）。
- CLI：registry 生成，无 cobra 代码。
- 前端：InspectionsPanel.tsx、list-detail.module.css、（设置 Dialog 新文件）、
  services/diagnostics 客户端与 types、zh.json/en.json、相关单测与 E2E spec。
- spec：`.trellis/spec/backend/`、`frontend/` 相关文档若有「不可删除」类约定需同步。
