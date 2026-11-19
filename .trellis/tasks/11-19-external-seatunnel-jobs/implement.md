# 实施计划：纳入非 STX 提交的 SeaTunnel 外部作业

> 严格在评审确认 `prd.md`、`design.md` 和本文件后，再执行 `python3 ./.trellis/scripts/task.py start 11-19-external-seatunnel-jobs` 进入 Phase 2。

## 1. Phase 2 准备（Spec 加载）
- 加载 `trellis-before-dev`，重读：
  - `.trellis/spec/backend/database-guidelines.md`
  - `.trellis/spec/backend/job-status-reconciler.md`
  - `.trellis/spec/backend/engine-events-and-realtime.md`
  - `.trellis/spec/backend/job-plan-dag.md`
  - `.trellis/spec/backend/cli-guidelines.md`
  - `.trellis/spec/frontend/api-and-services.md`
  - `.trellis/spec/guides/cross-layer-thinking-guide.md`

## 2. 分步实施清单

### Step 1：配置与数据模型基础
1. `internal/config/config.go` & `config.example.yaml`：
   - 添加 `Sync.ExternalJobDiscoveryEnabled *bool`（默认 `true`）、`Sync.ExternalJobDiscoveryIntervalSeconds int`（默认 `60`）与 `Sync.ExternalJobDiscoveryMaxNewPerRound int`（默认 `200`），定义在 `internal/config/model.go`。
2. `internal/apps/cluster/`：
   - 在 `ClusterConfig` 规范与更新/读取中支持 `external_job_discovery_enabled`（`*bool` / 可缺省）；
   - 为 `sync` 提供集群发现列表查询方法（返回开启发现的在线/配置端口集群 ID）。
3. `internal/apps/sync/model.go` & `job_dimensions.go`：
   - 定义 `JobSource`（`stx` / `external`）与 `RunTypeExternal`；
   - `JobInstance` 增加 `Source`（索引）、`CanStop` / `CanRecover`（`gorm:"-"`）、放宽 `TaskID` 非空约束，新增 `(cluster_id, engine_job_id)` 复合索引；
   - `BeforeCreate` 填充 `Source` 默认值并保证外部作业不强行填 `JobMode`。
   - 检查 `internal/db/migrator/migrator.go` 与三库（SQLite / MySQL / PostgreSQL）迁移兼容性。

### Step 2：统一作业权限模型（Single Source of Truth）
1. 新增 `internal/apps/sync/job_permissions.go`：
   - 实现 `JobPermissions`、`EvaluateJobPermissions(job, task, actor)` 与 `decorateJobPermissions` 批量装饰 helper。
2. 收敛 `internal/apps/sync/` 所有作业入口：
   - `repository.go` (`applyJobFilter`)：新增 `Source` 筛选；非管理员权限条件统一包含 `source = 'external'`。
   - `job_list.go` (`scopeJobFilterForActor`, `decorateJobsForList`)、`execution.go` (`getJobInstanceForActor`, `GetJobForActor`, `CancelJobForActor`)、`home_summary.go` (`jobViewableFunc`, `GetHomeJobSummary`)、`dashboard_jobs.go`、`job_sparklines.go`、`handler.go` (`RecoverJob`) 统一走 `EvaluateJobPermissions` / `RecoverJobForActor`。
   - `RecoverJob` 对外部作业复用已有的 `ErrRecoverSourceRequired`，不新增错误码。
   - `DeleteTaskForActor` 改为保留作业行并写 `task_deleted_at`（见 design §6），补对应的可见性、展示与 Recover 测试。
3. 编写单元测试：覆盖管理员/普通用户/所有者/协作者在 STX 公开任务作业、私有任务作业、外部作业上的 `View / Stop / Recover` 全矩阵（含列表、单条、sparklines、timeseries、logs、checkpoint、events、home summary）。

### Step 3：复用对账器与事件通道的外部作业发现
1. 端点回退与判重持久层：
   - `service.go` 提取 `resolveJobEngineEndpoint(ctx, job)`，接入 `refreshJobInstance`、`CancelJob`、`GetJobCheckpointSnapshot`；
   - `repository.go` 新增 `FindExistingEngineJobIDsByCluster`、`BackfillEngineEventJobInstanceID`。
2. `job_reconciler.go` & `external_discovery.go`：
   - 在 `reconcileActiveJobs` 合并活跃作业集群与外部发现集群，复用 `clusterReconcileScheduler.tryDispatch`；
   - 先做引擎调用去重（finished 只调一次不带状态的列表；解析列表项的 `jobDag` / `metrics` / `errorMsg`，已结束作业不再逐个调 `GetJobInfo`），再在 `reconcileClusterJobs` 用同一份列表结果识别未知 `engine_job_id`，过提交守卫后直接用列表项建行，每轮受 `MaxNewPerRound` 限制，遇传输错误立即止损；
   - 保护并发判重（`singleflight` / 集群发现锁）。
3. `engine_events.go`：
   - 在 `IngestEngineEvents` 遇到未知 `engine_job_id` 的 `JOB_STATUS` 事件时入防抖发现队列；
   - `StartEngineEventRuntime` 防抖回调 `discoverExternalJobFromEvent`，经 `GetJobInfo` 验证建库并回填 `sync_engine_events.job_instance_id`。
4. `home_summary.go` & `alert_signals.go`：
   - 修复 `HomeJobSummary` 对未知 `mode` 的计数，不误计入 `RunningStreaming`；
   - 确认 `alertSignalForJob` 显式跳过外部作业（不发出告警）。
5. 编写集成/竞态测试（`go test -race ./internal/apps/sync/`）：
   - 确认列表只请求一次（无重复拉取）；
   - 覆盖运行中/各种终态发现、Recover 复用同 `engine_job_id` 不重复建行、每轮新建上限、STX 提交与对账并发不重复建行、删除任务后不重新导入、每轮每集群只发 2 次列表请求、传输错误止损与退避、事件通道发现与事件回填、未知 mode 统计。

### Step 4：Handler、CLI 与契约同步
1. `internal/apps/sync/handler.go` & `types.go`：支持 `GET /api/v1/sync/jobs?source=stx|external` 校验与透传；更新错误码映射。
2. `internal/apps/cluster/` handler & CLI：暴露集群级 `external_job_discovery_enabled` 开关（如涉及生成式 CLI 同步检查 `internal/operation/`）。
3. 检查 `internal/operation/`、`internal/cli/` 中 `stx sync job list` 对 `--source` 的支持，更新 Swagger 与路由基线（`swag init`、`contract-report`）。

### Step 5：前端展示与交互统一
1. `frontend/lib/services/sync/`：扩展 `SyncJobInstance`（`source`、`can_stop`、`can_recover`）与 `ListJobsParams`（`source`）。
2. `frontend/components/common/sync/sync-studio-utils.ts`：更新 `getJobActionState` 优先消费后端权威权限 `can_stop` / `can_recover`，对外部作业禁用 Recover。
3. `JobTable` / `JobTableToolbar` / `JobListTab` / `JobDetailDialog` & i18n（中英）：
   - 列表支持 `来源` 筛选与徽标（`STX` / `外部`），外部作业隐去 `#task_id` 与“工作台打开”，未知 `mode` 显示 `-`；
   - `JobListTab` 移除硬编码 `canRun: true`；
   - `JobDetailDialog` 对外部作业适配“脚本”页签与操作按钮状态。
4. 补齐前端 Vitest 单元测试。

## 3. 验证命令

```bash
# 后端单元测试与竞态检测
go test -race ./internal/apps/sync/... ./internal/apps/cluster/... ./internal/operation/...

# 三库迁移与方言兼容测试
./scripts/test_db_compat.sh

# 契约与 Swagger 检查
go run ./internal/operation/cmd/contract-report -check

# 前端类型检查与单测
cd frontend && pnpm exec tsc --noEmit && pnpm test
```

## 4. 高风险点与回滚边界
- **MySQL/PG/SQLite 列变更**：`TaskID` 去除 `NOT NULL` 与新增 `Source` 列需通过 `./scripts/test_db_compat.sh` 实测 GORM `AutoMigrate` 对已有数据的迁移兼容性（尤其 SQLite 改表约束）。
- **引擎历史作业风暴**：首次上线时若集群保留大量历史终态作业，引擎默认只保留 24 小时，且已结束作业直接用列表项入库、不额外请求；仍靠 `ExternalJobDiscoveryMaxNewPerRound` 限制每轮写库行数 + 已有传输止损退避兜底；如遇异常可通过配置 `sync.external_job_discovery_enabled: false` 即刻关闭发现，不影响既有作业对账。
