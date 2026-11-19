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
                                                 3. 差集识别未知 engine_job_id → 过提交守卫 → 用列表项 ensureExternalJob
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
  - `TaskID uint`：保持 `gorm:"index;not null"` 不变。外部作业写 `0`，`0` 满足 NOT NULL；改列的可空性在 SQLite 上会重建整张作业表，没有必要。
  - `Source JobSource`：`gorm:"size:20;not null;default:'stx';index"`。外部作业在构造时显式写 `external`，其余路径不写，靠列默认值得到 `stx`；`BeforeCreate` 只在为空时补 `stx`，不再按 `TaskID` / `PlatformJobID` 猜。
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
  - 发现路径之外，还要挡住 STX 自己的提交：引擎在 REST 提交返回之前就把作业放进 running 列表（见 PRD 引擎事实），而 STX 在提交返回后才落库。处理方式：
    - `submitTaskInstanceWithPayload` 与恢复路径在调 `Submit` 前把 `platformJobID` 放进进程内 `inflightSubmits`（`sync.Map`），`CreateJobInstance` 或失败后移除；发现时命中即跳过。
    - 宽限期兜底多实例与进程重启：未知作业需在连续两轮列表中出现（内存记录首次看到的时间），或其 `createTime` 早于当前时间超过 `submit_timeout_seconds`，才导入。
  - 已删除任务的作业行保留（见 §6），判重查得到，不会被重新导入。

### 2.2 外部发现开关（全局 + 集群级）
1. **全局开关**：`internal/config/model.go` 的 `SyncConfig` 新增：
   - `ExternalJobDiscoveryEnabled *bool`（`mapstructure:"external_job_discovery_enabled"`，未配置时默认 `true`，`*bool` 区分省略与显式 `false`）。
   - `ExternalJobDiscoveryIntervalSeconds int`（`external_job_discovery_interval_seconds`，没有非终态作业的集群的巡检间隔，默认 `60`，`<=0` 用默认值）。
   - `ExternalJobDiscoveryMaxNewPerRound int`（`external_job_discovery_max_new_per_round`，每轮每集群最多新建的外部作业行数，默认 `200`，`<=0` 用默认值）。
2. **集群级开关**：
   - `ClusterConfig`（`clusters.config` JSON）新增键 `external_job_discovery_enabled`：省略表示继承全局，显式 `false` 关闭该集群。生效规则固定为 `effective = globalEnabled && (clusterOverride == nil || *clusterOverride)`，全局关闭时集群显式 `true` 也不生效。
   - 在 `cluster` 模块的更新配置接口 / 集群信息结构体中暴露读取与更新能力，并在 `sync` 通过最小接口（`ClusterDiscoveryProvider`）读取需要巡检的集群列表与开关状态，保持 `sync` 不直接依赖 `cluster` 模型实现（延续 `ClusterVersionProvider` / `ClusterRuntimeResolver` 解耦模式）。

## 3. 采集、对账与事件加速契约

### 3.1 巡检范围与引擎调用去重（`job_reconciler.go`）
1. **先做引擎调用去重**（对现有 STX 作业同样生效，建议作为前置 PR 单独合入）：
   - `reconcileClusterJobs` 的 finished 列表只调一次不带状态的 `ListFinishedJobs(ctx, endpoint, "")`，删除 `FINISHED / FAILED / CANCELED / CANCELLED / SAVEPOINT_DONE` 五次子集请求。
   - `EngineJobListItem` 增加 `JobDag map[string]any`、`Metrics map[string]any` 字段，`engineListItemToInfo` 原样带过去。
   - 已结束作业直接用列表项 `applyEngineJobInfoUpdate`，不再逐个调 `GetJobInfo`；列表项与终态 `/job-info` 是同一份数据。保留现有「终态但缺终态指标」的放弃策略作为兜底。
   - 运行中作业用 `/running-jobs` 项更新状态与指标；`GetJobInfo` 只用于：详情页 `fresh` 刷新、作业不在两个列表里需要确认是否消失（`engineJobInfoLooksOrphaned`）、事件触发的单作业刷新。
   - 去重后每轮每集群固定 2 次列表请求。
2. `reconcileActiveJobs` 每轮：
   - 去掉「没有非终态作业就 return」的早退，改为只跳过非终态分组那一段。
   - 全局开关打开时，调 `clusterDiscoveryProvider.ListExternalDiscoveryClusterIDs(ctx)` 取开启外部发现、且能解析到引擎端点的集群。
   - 有非终态作业的集群照旧每轮派发；只做发现的集群按 `external_job_discovery_interval_seconds` 派发（按集群记录上次发现时间）。两者都走同一个 `tryDispatch`，共用并发信号量、`inFlight` 跳过与指数退避。
   - `reconcileClusterJobs` 去掉 `len(jobs)==0` 早退；端点只用 `ResolveEngineEndpoint`，解析不到时返回 `errReconcileNoEndpoint` 进入退避。
3. `reconcileClusterJobs` 在推进已有作业之后（仅当该集群开启外部发现）：
   - 把 running 与 finished 列表的全部 `engine_job_id` 和库里比对，`FindExistingEngineJobIDs(ctx, clusterID, ids)` 按 500 个一批做 `IN` 查询。
   - 未知作业先过提交守卫（§2.1），再按「运行中优先、已结束其后」直接用列表项 `ensureExternalJobInstance`，每轮最多 `external_job_discovery_max_new_per_round` 行，其余留给下一轮。
   - 这一步不发任何额外引擎请求。

### 3.2 `JobInstance` 构造契约（外部作业）
- `TaskID = 0`, `TaskVersion = 0`, `CreatedBy = 0`
- `Source = JobSourceExternal`, `RunType = RunTypeExternal`
- `PlatformJobID = ""`（无平台生成 ID）
- `EngineJobID = strings.TrimSpace(item.JobID)`
- `ClusterID = clusterID`
- `JobMode = normalizeJobMode(jobDag.envOptions["job.mode"])`，取不到 `job.mode` 但有 `jobDag` 时按引擎默认 `batch`；连 `jobDag` 都没有时为空串。`SubmitSpec.mode` 同步写入同一个值，`effectiveJobMode` 无需改动。
- `Status = normalizeJobStatus(rawStatus)`（若终态且从 `finishedMap` 来，`FinishedAt` 用 `parseEngineJobTime` 解析引擎 `finishTime`；`StartedAt` 用 `parseEngineJobTime` 解析引擎 `createTime`，均为空则回退当前 UTC；不要用事件的 `createdTime`，它是事件产生时间）
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

- 防抖与校验细节：
  - 队列按 `(clusterID, engineJobID)` 去重，总长度有上限（例如 1000），满了丢弃最旧的项（拉取主干会兜底）。
  - 防抖延迟要长于一次提交的往返（例如 15 秒），到期先查 `inflightSubmits` 与库，命中任意一个就丢弃。
  - `GetJobInfo` 的结果先过 `engineJobInfoLooksOrphaned`，空壳不落库。
  - 落库同样走 §2.1 的判重与 §3.2 的构造契约（模式取自返回的 `jobDag.envOptions`）。

### 3.4 `JobStatus`、`JobMode` 与 `HomeJobSummary` 契约
- `effectiveJobMode(job)`：外部作业按 §3.2 已落 `job_mode`，与内部作业同一套取值；只有缺 `jobDag` 的外部作业返回 `""`。
- `BackfillJobDimensions`：查询条件排除 `source = 'external'`，否则每次启动都要扫一遍 `job_mode` 为空的外部作业行。
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
  - `RecoverJobForActor`（新增 Service 层统一入口替代 `handler.go` 里的散落校验）：外部作业复用已有的 `ErrRecoverSourceRequired`（`RecoverJob` 已对 `TaskID == 0 || PlatformJobID == ""` 返回它），权限不足返回 `ErrTaskPermissionDenied`（403），不新增错误码。
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
  - 数据库只新增带默认值的 `source` 列与 `(cluster_id, engine_job_id)` 复合索引，`task_id` 约束不变，老数据向后兼容（`source` 默认为 `stx`）；
  - 发现逻辑完全由 `sync.external_job_discovery_enabled` 开关控制，随时可通过配置关闭回滚且不影响现有 STX 作业对账。

## 6. 删除任务保留作业记录

- `DeleteTaskForActor` 不再调用 `DeleteJobInstancesByTaskIDs`，改为给这些作业行写 `task_deleted_at`（新增可空列）。预览数据、任务版本等仍按原逻辑删除。
- 作用：判重能查到这些作业，引擎保留期内的历史作业不会被重新导入为外部作业；删除时仍在运行的作业继续被对账，管理员与发起人仍可停止。
- 权限与展示：
  - `CanView`：管理员、发起人；任务已删除时不再走任务可见性。
  - `CanStop`：管理员、发起人。
  - `CanRecover`：`false`，`RecoverJob` 查不到任务时返回明确错误。
  - 列表与详情显示「任务已删除」，不给工作台链接。
- 不能只靠 `task_id` 指向不存在的任务来判断「已删除」：MySQL 5.7 重启后自增计数会重置为 `max(id)+1`，新任务可能复用被删任务的 ID。所以必须落 `task_deleted_at` 标记，`VisibleTaskIDs` 计算与 `decorateJobsForList` 都要排除已标记的行。
- 清理：已删除任务的作业行按单独的保留期清理（与外部作业共用同一个清理配置，实施时再定默认值）。
