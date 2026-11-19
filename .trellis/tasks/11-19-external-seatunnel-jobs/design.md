# 技术设计：纳入非 STX 提交的 SeaTunnel 外部作业与统一作业权限模型

## 1. 总体架构

```text
  SeaTunnel Cluster (Engine REST / event-report-http)
     │                                    │
     │ (A) 推: POST /engine-events/:cid   │ (B) 拉: StartJobReconciler (10s tick)
     ▼                                    ▼
  IngestEngineEvents              reconcileActiveJobs
   └─ 未知 engine_job_id (JOB_STATUS)   └─ 合并待巡检集群 (有活跃作业 ∪ 已开启外部发现的在线集群)
      └─ 校验开关 + 防抖队列                └─ tryDispatch (并发=4 / 退避 / 止损)
         └─ GetJobInfo 验证                   └─ reconcileClusterJobs:
            └─ ensureExternalJob                 1. 一次 ListRunningJobs + ListFinishedJobs
                                                 2. 推进已知作业 (原有逻辑)
                                                 3. 差集识别未知 engine_job_id → 按配额补 GetJobInfo → ensureExternalJob
                                                    │
                                                    ▼
                                            sync_job_instances
                                       (source = "stx" | "external", task_id = 0)
                                                    │
                     ┌──────────────────────────────┴──────────────────────────────┐
                     ▼                                                             ▼
         统一作业权限模型 (JobPermissions)                               功能复用与降级边界
         - CanView / CanStop / CanRecover                       - 复用: 列表/汇总/指标/趋势/日志/Checkpoint/事件/停止
         - 外部: 全员只读可见, 仅管理员可停, 不可恢复                 - 降级: PlanDAG(no_submitted_content), 禁 Recover/脚本/Preview
```

## 2. 数据模型与配置设计

### 2.1 `JobInstance` 扩展（`internal/apps/sync/model.go`）
- 新增常量与字段：
  ```go
  type JobSource string

  const (
      JobSourceSTX      JobSource = "stx"
      JobSourceExternal JobSource = "external"
  )

  const RunTypeExternal RunType = "external"
  ```
- `JobInstance` 字段调整：
  - `TaskID uint`：去掉 `gorm:"not null"`（外部作业为 `0`；兼容 SQLite / MySQL / PostgreSQL，GORM 默认零值 `0` 可写入，保持现有非空语义的内部作业继续为正整数）。
  - `Source JobSource`：`gorm:"size:20;not null;default:'stx';index"`（数据库迁移与 `BeforeCreate` 兜底：若为空且 `RunType == RunTypeExternal` 或 `TaskID == 0 && PlatformJobID == ""` 则设为 `external`，否则设为 `stx`）。
  - 响应边界权限字段（`gorm:"-"`）：
    ```go
    CanStop    bool `json:"can_stop" gorm:"-"`
    CanRecover bool `json:"can_recover" gorm:"-"`
    ```
    （`CanView` 在返回列表中恒为 `true`，在首页 Attention/Dashboard 条目中继续通过 `Viewable` 体现。）
- **判重策略（防重复导入）**：
  - 因为 STX `RecoverJob` 会沿用原 `engine_job_id` 创建新 `JobInstance`，所以数据库**不能**对 `(cluster_id, engine_job_id)` 建唯一索引。
  - 改为增加复合普通索引 `idx_cluster_engine_job (cluster_id, engine_job_id)`，在导入前做库内存在性检查（`ExistsJobInstanceByClusterAndEngineJobID`），若已存在任意行（无论 `stx` 还是 `external`、无论活跃还是终态）则不再创建外部实例。
  - 同时，若极端并发下「定时对账」与「事件推发现」同时发现同一个 `(cluster_id, engine_job_id)`，在内存中用 `singleflight` / 集群发现锁串行化 `ensureExternalJobInstance(clusterID, engineJobID, ...)`，确保零并发重复插入。

### 2.2 外部发现开关（全局 + 集群级）
1. **全局开关**：`internal/config/config.go` 的 `SyncConfig` 新增：
   - `ExternalJobDiscoveryEnabled *bool`（yaml: `external_job_discovery_enabled`，未配置时默认 `true`，遵循 `database-guidelines.md` `*bool` 区分省略与显式 `false`）。
   - `ExternalJobDiscoveryInfoBudget int`（每轮每集群新发现作业最多拉取 `GetJobInfo` 的条数，默认 `20`，`<=0` 使用默认值）。
2. **集群级开关**：
   - `ClusterConfig`（`clusters.config` JSON）新增键 `external_job_discovery_enabled`（`bool` / 省略表示继承全局，显式 `false` 关闭该集群，显式 `true` 开启；若全局为 `false` 则全局优先关闭，或定义为 `effective = globalEnabled && (clusterOverride == nil || *clusterOverride)`）。
   - 在 `cluster` 模块的更新配置接口 / 集群信息结构体中暴露读取与更新能力，并在 `sync` 通过最小接口（`ClusterDiscoveryProvider`）读取需要巡检的集群列表与开关状态，保持 `sync` 不直接依赖 `cluster` 模型实现（延续 `ClusterVersionProvider` / `ClusterRuntimeResolver` 解耦模式）。

## 3. 采集、对账与事件加速契约

### 3.1 巡检集群范围与零重复拉取（`job_reconciler.go`）
1. `reconcileActiveJobs` 每轮：
   - 查询非终态作业并按 `cluster_id` 分组（原有逻辑）。
   - 若全局外部作业发现开启，调用 `clusterDiscoveryProvider.ListExternalDiscoveryClusterIDs(ctx)` 获取**状态可访问且开启外部发现的集群 ID 列表**（例如：集群存在在线节点或 `api_port` 节点，且 `external_job_discovery_enabled != false`）。
   - 对这些集群，即使当前 `grouped[clusterID]` 为空，也通过同一个 `s.reconcile.tryDispatch(ctx, clusterID, ...)` 派发（共用同一份并发信号量 `4`、单轮跳过 `inFlight`、指数退避 `10..120s`）。
   - **重要**：事件模式自适应轮询（`useEventModeReconcile`）下，无活跃作业的集群只有到 `fullReconcileDue` 时才跑完整列表；有事件通道时新作业靠事件推秒级发现，未开启事件模式时每 10 秒跑 `reconcileClusterJobs`。
2. `reconcileClusterJobs(ctx, clusterID, activeJobs)` 单轮执行顺序：
   - Step 1：解析端点（`ResolveEngineEndpoint`），调用 `ListRunningJobs` 与 `ListFinishedJobs`（各状态），遇到传输层错误立即止损并返回错误触发退避。
   - Step 2：推进已有 `activeJobs` 的状态与指标（与现有完全一致）。
   - Step 3（仅当该集群开启外部发现）：将 `runningMap` 与 `finishedMap` 中的全部 `engine_job_id` 与本地库批量比对（一次 `IN` 查询 `FindExistingEngineJobIDs(ctx, clusterID, allEngineIDs)`，避免 N+1 查询）：
     - 先处理 `runningMap` 中的未知作业，再处理 `finishedMap` 中的未知作业；
     - 计入 `GetJobInfo` 配额（每轮每集群最多 `budget` 次，例如 `20` 次）：
       - 在配额内调用 `GetJobInfo` 获取 `jobDag`、`metrics`、`errorMsg` 并创建 `JobInstance`（立即写入 `last_synced_at`、`sync_state=ok`、终态 `metrics_log` 与 Prometheus 观察）；若 `GetJobInfo` 遇传输错误则立即止损返回（保留已落库记录，剩余留给下一轮，触发退避）；
       - 超出配额的：
         - 若在 `runningMap` 中，可先用列表摘要（`engineListItemToInfo`）建出非终态行（下轮自然在 `activeJobs` 中补 `GetJobInfo`）；
         - 若在 `finishedMap` 中，先用列表摘要建出终态行，或留到下一轮列表差集（因为列表结果在引擎保留窗口内仍会返回——若用摘要先落库，需确保无 `metrics` 时保持 `hasFinalMetricsLog=false` 以便后续 `startTerminalMetricsBackfill` 异步补齐终态指标与 `job_dag`）。**推荐**：`finishedMap` 超出配额的先用列表摘要落库（列表立即可见且不会丢），依靠现有 `startTerminalMetricsBackfill`（或后续低优先补详情）渐进补 `job_dag` 与 `metrics`，且同样受传输止损与退避保护。

### 3.2 `JobInstance` 构造契约（外部作业）
- `TaskID = 0`, `TaskVersion = 0`, `CreatedBy = 0`
- `Source = JobSourceExternal`, `RunType = RunTypeExternal`
- `PlatformJobID = ""`（无平台生成 ID）
- `EngineJobID = strings.TrimSpace(item.JobID)`
- `ClusterID = clusterID`
- `JobMode = ""`（未知；`BeforeCreate` 不强填为 `streaming`）
- `Status = normalizeJobStatus(rawStatus)`（若终态且从 `finishedMap` 来，`FinishedAt` 用 `parseEngineJobTime` 解析引擎 `finishTime`；`StartedAt` 用 `parseEngineJobTime` 解析引擎 `createTime`，均为空则回退当前 UTC）
- `SubmitSpec`：
  ```json
  {
    "source": "external",
    "trigger_source": "external",
    "cluster_id": 1,
    "job_name": "引擎 jobName",
    "engine_api_mode": "v2",
    "engine_base_url": "http://...",
    "engine_legacy_base_url": "http://...",
    "engine_context_path": ""
  }
  ```
- **端点解析兜底**：抽取公共 helper `resolveJobEngineEndpoint(ctx, job)`：优先 `endpointFromSubmitSpec(job.SubmitSpec)`；若缺失或集群内需动态主备路由，当 `clusterID := clusterIDForJob(...) > 0` 且 `s.runtimeResolver != nil` 时调用 `ResolveEngineEndpoint(ctx, clusterID, nil)`。在 `refreshJobInstance`、`CancelJob`、`GetJobCheckpointSnapshot` 中统一使用该 helper。

### 3.3 事件通道加速发现（`engine_events.go`）
- **职责定位**（推只做触发加速与细粒度事件源，拉做主干）：SeaTunnel `event-report-http` 推送的 `JOB_STATUS` 仅含 `jobId / jobName / jobStatus / createdTime`，不含 `jobDag`、`metrics`、`errorMsg`，且 2.3.12 仅推终态、2.3.11 及以下不推 `JOB_STATUS`、ringbuffer（2000 条）高负载可覆盖；因此推送仅作 3.0+ 集群的唤醒触发器与 CDC 延迟 / Schema 变更来源，发现主干仍以 `reconcileClusterJobs` 为准。
- 在 `IngestEngineEvents` 中：
  - `sanitizeEngineEvent` 仍先做常规清洗与脱敏；遇到 `findClusterJob == nil` 的未知 `engine_job_id` 时，事件仍正常落库（`JobInstanceID = nil`），并记录 `(clusterID, engineJobID)` 到新增的轻量防抖队列 `pendingExternalDiscoveries`（仅针对 `EventType == EngineEventTypeJobStatus` 且该集群开启了外部发现）。
  - 在 `StartEngineEventRuntime` 的 `refreshTicker` 中处理到期的外部发现：调用 `discoverExternalJobFromEvent(ctx, clusterID, engineJobID)` -> 检查库内是否已存在 -> 检查集群不在退避期 -> 调用 `GetJobInfo` 验证真实存在 -> 落库 `JobInstance` 并回填该 `(clusterID, engineJobID)` 最近已入库事件的 `job_instance_id`（`BackfillEngineEventJobInstanceID`），确保详情「事件」页签完整不漏首条状态事件。

### 3.4 `JobStatus`、`JobMode` 与 `HomeJobSummary` 契约
- `effectiveJobMode(job)`：外部作业 `JobMode == ""` 且 `submit_spec.mode` 为空时，返回 `""`。
- `home_summary.go`：
  - `out.Running` 计入所有非预览运行中作业（含外部作业）；
  - `switch effectiveJobMode(job) { case string(TaskModeBatch): out.RunningBatch++; case string(TaskModeStreaming): out.RunningStreaming++ }`，未知模式不再错误累加到 `RunningStreaming`；可按需保持 `Running == RunningBatch + RunningStreaming + 未知外部数`（不破坏原有字段语义）。
  - `WriteQPS` / `ReadQPS` / `Failed24h` / `Attention` 自然计入外部作业。
- `alert_signals.go`：`alertSignalForJob` 保持 `if job == nil || job.RunType == RunTypePreview || job.IsExternal() || job.TaskID == 0 { return ..., false }`，外部作业不产生平台告警信号。

## 4. 统一作业权限模型（Single Source of Truth）

### 4.1 领域模型与裁决函数（`internal/apps/sync/job_permissions.go`）
定义统一权限判定（对应 `Task.CanUserView/Edit/Run`）：
```go
type JobPermissions struct {
    CanView    bool
    CanStop    bool
    CanRecover bool
}
```
判定规则（接收 `job *JobInstance`, `task *Task`（可 nil）, `actor executionapp.Actor`）：
1. **`CanView`**：
   - `actor.IsAdmin` → `true`
   - `job.IsExternal()` → `true`（所有登录用户只读可见）
   - `actor.UserID > 0 && job.CreatedBy == uint(actor.UserID)` → `true`
   - `task != nil && task.CanUserView(uint(actor.UserID), false)` → `true`
2. **`CanStop`**：
   - `actor.IsAdmin` → `true`
   - `job.IsExternal()` → `false`（外部作业仅管理员可停）
   - `actor.UserID > 0 && job.CreatedBy == uint(actor.UserID)` → `true`
   - `task != nil && task.CreatedBy != 0 && task.CreatedBy == uint(actor.UserID)` → `true`
3. **`CanRecover`**：
   - `job.IsExternal() || job.TaskID == 0 || strings.TrimSpace(job.PlatformJobID) == "" || job.RunType == RunTypePreview` → `false`
   - `actor.IsAdmin` → `true`
   - `task != nil && task.CanUserRun(uint(actor.UserID), false)` → `true`

### 4.2 后端所有入口收敛
- **SQL 过滤统一**：`scopeJobFilterForActor` 设置 `IncludeExternalRead = true`（或在 `applyJobFilter` 中非管理员条件统一为 `(source = 'external' OR created_by = ? OR task_id IN ?)`，当 `VisibleTaskIDs` 为空时为 `(source = 'external' OR created_by = ?)`），使 `ListJobsWithOptions`、`ListJobsForActor`、`GetJobSparklinesForActor` 自动包含外部作业且严格与 `CanView` 等价。
- **内存过滤统一**：`jobViewableFunc` 直接调用同一权限判定逻辑（外部作业对登录用户 `viewable = true`），覆盖 `GetHomeJobSummary`、`DescribeJobsForDashboard`、`JobAnnotationsBetween`。
- **单条读取统一**：`getJobInstanceForActor` 加载作业及关联 `Task`（当 `TaskID > 0`），调用 `EvaluateJobPermissions(job, task, actor)` 检查 `CanView`，覆盖 `GetJobForActor`、`GetJobLogsForActor`、`GetPreviewSnapshotForActor`、`GetJobCheckpointSnapshotForActor`、`GetJobTimeseriesForActor`、`GetJobRealtimeForActor`、`GetJobPlanDAGForActor`、`ListJobEventsForActor`。
- **控制操作统一**：
  - `CancelJobForActor`：调用 `EvaluateJobPermissions`，`!perms.CanStop` 返回 `ErrTaskPermissionDenied`（403）。
  - `RecoverJobForActor`（新增 Service 层统一入口替代 `handler.go` 里的散落校验）：外部作业或不可恢复返回明确业务错误（如 `ErrExternalJobRecoverUnsupported` → 400 / `ErrTaskPermissionDenied` → 403）。
- **响应边界统一装饰**：在 `GetJobForActor` 与 `decorateJobsForList`（列表）中批量加载关联 `Task`（一次查询）并写入 `job.CanStop` / `job.CanRecover`，不再让前端猜测权限。

### 4.3 前端统一消费
- `SyncJobInstance` 类型增加 `source?: 'stx' | 'external'`、`can_stop?: boolean`、`can_recover?: boolean`。
- `getJobActionState(job, ctx)` 优先信任后端返回的 `job.can_stop` / `job.can_recover`（当存在时直接叠加生命周期状态与 `disableRecover`；向后兼容未传时退回原 `ctx` 逻辑）。
- `JobListTab` 不再硬编码 `canRun: true`，直接使用每行返回的权限字段。
- 作业列表/详情 UI：
  - 新增「来源」徽标（`STX` / `外部`）与列表筛选条件 `source`；
  - 外部作业不展示 `#task_id` 任务引用，不展示「在工作台打开」；
  - 详情页「脚本」页签对外部作业隐藏或展示空状态说明“外部提交作业不包含脚本快照”；「拓扑」抽屉里的表结构/建表计划由 `/plan-dag` 返回 `no_submitted_content` 正常提示。

## 5. API、CLI 与兼容性

- **新增/扩展 Query 参数**：
  - `GET /api/v1/sync/jobs` 新增可选参数 `source`（`stx | external`），未知值返回 `400 ErrInvalidJobListQuery`。
- **新增/扩展配置与集群接口**：
  - 集群配置/详情读写支持 `external_job_discovery_enabled`（缺省继承全局默认开启）。
  - 按 `cli-guidelines.md` 检查 `stx sync job list`（自动透传或补 `--source`）及集群配置 CLI、运行 `swag init` 与 `contract-report` 保持基线同步。
- **兼容性与回滚**：
  - 数据库仅新增可为空/带默认值的列（`source`）并放宽 `task_id` 非空限制，老数据向后兼容（`source` 默为 `stx`）；
  - 发现逻辑完全由 `sync.external_job_discovery_enabled` 开关控制，随时可通过配置关闭回滚且不影响现有 STX 作业对账。
