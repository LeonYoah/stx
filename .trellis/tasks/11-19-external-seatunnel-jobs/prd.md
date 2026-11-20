# 纳入非 STX 提交的 SeaTunnel 外部作业

## Goal

将通过外部方式（SeaTunnel CLI、REST API、其他调度器等）提交到 STX 所管理 SeaTunnel 集群的作业统一纳入 STX 作业体系，复用现有作业列表、状态对账、详情观测（DAG / 指标 / 趋势 / Checkpoint / 日志 / 事件时间线）、首页汇总与控制能力（停止）；同时**统一平台作业权限模型**，明确区分内部（STX）与外部（External）提交主体，并对强依赖脚本内容或 STX 任务实体的能力（计划 DAG、历史恢复、脚本查看、数据预览）做明确禁用或优雅降级。

## Confirmed Facts (from Codebase Inspection)

- **作业对账器**：`internal/apps/sync/job_reconciler.go` 的 `reconcileActiveJobs` 每 10 秒按 `cluster_id` 通过 `clusterReconcileScheduler` 并行对账（并发上限 4、单轮传输失败止损、指数退避 10→20→40→80→120s、HTTP 超时 15s）。`reconcileClusterJobs` 已调用 `ListRunningJobs`（`/running-jobs`）与 `ListFinishedJobs`（`/finished-jobs` 及各终态路径）获取全量作业列表。
- **事件通道**：`internal/apps/sync/engine_events.go` 的 `IngestEngineEvents` 已支持接收 `event-report-http` 推送（2.3.12+ 终态、3.0.0+ 非终态）；当前遇到未知 `engine_job_id` 仅入库事件，不建作业记录。
- **当前任务与作业权限现状**：
  - `Task` 已有完善的三层权限：`is_public`（默认 `true`）、`collaborators`/`collaborator_ids`、`created_by` 与 `is_admin`（`CanUserView` / `CanUserEdit` / `CanUserRun` / `canManageTaskPermissions`）。
  - `JobInstance` 当前**没有**统一权限模型：查看分散在 `scopeJobFilterForActor` / `getJobInstanceForActor` / `jobViewableFunc` 三处；停止在 `CancelJobForActor`；恢复在 `handler.go` 里查 `Task.CanUserRun`；前端 `JobListTab` 全局列表硬编码 `canRun: true` 且只能靠 `created_by` 猜权限。
  - `Cluster` 对所有登录用户全局可见，无按集群的读隔离。
- **无脚本降级基础**：
  - `/plan-dag` 已对空 `submitted_content` 返回 `200 + available=false (reason=no_submitted_content)`。
  - `getClusterJobLogs` 按 `cluster_id` + `engine_job_id` 经 Agent 读节点日志文件，不依赖脚本或 `TaskID`。
  - `refreshJobInstance`、`CancelJob`、`GetJobCheckpointSnapshot` 目前仅从 `submit_spec` 读取端点，尚未在缺失时回退 `runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil)`。
- **当前对账的引擎调用量**（每轮默认 10 秒、每个有非终态作业的集群一次）：`/running-jobs` 1 次 + `/finished-jobs` 与 `/finished-jobs/{FINISHED,FAILED,CANCELED,CANCELLED,SAVEPOINT_DONE}` 共 6 次（v2 非 2xx 时各自再试 hazelcast 旧路径）+ 每个匹配到的活跃作业 1 次 `/job-info`。`reconcileActiveJobs` 在没有非终态作业时直接返回，`reconcileClusterJobs` 在 `len(jobs)==0` 时直接返回。`job_reconciler_event_mode_interval_seconds` 默认 `0`（自适应关闭）。
- **提交顺序**：`submitTaskInstanceWithPayload` 先调引擎 `Submit`，成功后才 `CreateJobInstance`；对账在后台协程并行执行。
- **删除任务**：`DeleteTaskForActor` 会连带 `DeleteJobInstancesByTaskIDs`，且不检查是否仍有运行中作业。
- **已有可复用的拒绝路径**：`RecoverJob` 对 `TaskID == 0 || PlatformJobID == ""` 返回 `ErrRecoverSourceRequired`；`alertSignalForJob` 与 CDC 延迟告警已跳过 `TaskID == 0`。

## Confirmed Facts (from SeaTunnel Source, 2.3.11 / 2.3.12 / 2.3.13 / 3.1.0 / dev)

- **finished 列表**：不带状态的 `/finished-jobs`（v2 与 `/hazelcast/rest/maps/finished-jobs` 都一样）返回保留期内全部已结束作业；`/finished-jobs/{state}` 按 `JobStatus.name()` 精确匹配，`CANCELLED` 不在枚举里，恒返回 `[]`。不带 `page` 参数时不分页。
- **finished 列表项 = 终态 job-info**：列表每一项都带 `jobDag`、`metrics`、`errorMsg`、`createTime`、`finishTime`，与 `/job-info` 查已结束作业走的是同一个 `getJobInfoJson(JobState, metrics, dag)`。pipeline 结束时先写 `IMAP_FINISHED_JOB_METRICS`，作业终态最后写，所以列表出现终态时终态指标已就绪。
- **running 列表**：`/running-jobs` 每一项带完整 `jobDag`、`metrics`、`envOptions`（引擎为每个作业还原一次 DAG、取一次指标）；3.1.0 起另有轻量的 `/running-jobs/summary`（仅 jobId / jobName / jobStatus / createTime）。
- **作业模式可得**：`jobDag.envOptions` 是用户 env 块原样；缺 `job.mode` 时引擎按默认 `BATCH` 运行。
- **保留期**：`history-job-expire-minutes` 默认 `1440`（24 小时）。
- **提交可见性**：`CoordinatorService.submitJob` 在 REST 提交返回之前就已 `runningJobInfoIMap.put` 并进入 PENDING，作业会先于 STX 落库出现在 `/running-jobs`，开启非终态上报时 PENDING 事件也可能先到。
- **jobId 唯一性**：同一 jobId 非 savepoint 恢复时重复提交会被拒绝（"has already been submitted"）；3.1.0 起 `restoreMode` + `restoreSourceJobId` 会以新 jobId 从别的作业 checkpoint 恢复。
- **未知作业**：`/job-info` 对不存在的作业返回 200 + 仅含 `jobId` 的空壳。
- **`JOB_STATUS` 事件**：只含 `jobId / jobName / jobStatus / createdTime`，`createdTime` 是事件产生时间而非作业创建时间；非终态仅在 `report-non-terminal-job-state=true` 时上报。

## Requirements

1. **采集与防重复拉取（拉为主，推为加速）**
   - 拉取侧直接复用 `StartJobReconciler` + `SeaTunnelEngineClient`，不新增独立轮询线程。先做引擎调用去重（对现有 STX 作业同样生效，可作为前置 PR）：
     - finished 列表每轮只调 1 次不带状态的 `/finished-jobs`，删除按状态的 5 次子集请求；
     - 解析列表项里的 `jobDag`、`metrics`、`errorMsg` 直接推进作业状态，已结束作业不再逐个调 `/job-info`；运行中作业用 `/running-jobs` 项里的 `metrics` 与 `jobDag` 更新，`/job-info` 只保留给详情页 `fresh` 刷新、列表里找不到需要确认消失、以及事件触发的单作业刷新。
     - 目标调用量：每轮每集群 2 次列表请求（running + finished），与已纳管作业对账共用，外部发现不额外请求列表。
   - 没有非终态作业但开启外部发现的集群也要被巡检，但使用独立的较低频率（新增 `sync.external_job_discovery_interval_seconds`，默认 `60`），不依赖事件模式开关。
   - 推送侧复用 `event-report-http`：收到未知 `engine_job_id` 的 `JOB_STATUS` 事件后入防抖队列（有上限、按 `(cluster_id, engine_job_id)` 去重），到期重新查库，仍不存在才调一次 `/job-info`，用 `engineJobInfoLooksOrphaned` 排除空壳后落库。
   - **防止把 STX 自己的提交识别成外部作业**：提交前把 `platformJobID` 登记到进程内「提交中」集合，落库后移除，发现时跳过；另设宽限期，未知作业需在连续两轮列表中出现，或其 `createTime` 早于当前时间超过 `submit_timeout_seconds`，才允许导入。
   - **首次接入的写库上限**：已结束外部作业直接用 finished 列表项入库，无需额外请求；保留期默认只有 24 小时，但仍限制每轮每集群新建行数（`sync.external_job_discovery_max_new_per_round`，默认 `200`），运行中优先，其余留给下一轮。库内判重的 `IN` 查询按 500 个一批。
   - **删除任务不再删除作业记录**：`DeleteTaskForActor` 保留 `sync_job_instances`，作业行写入 `task_deleted_at`（或等价标记），这样引擎保留期内的历史作业不会被重新导入，运行中作业仍可对账与停止。展示为「任务已删除」，不再给工作台链接；Recover 返回明确错误；非管理员仅发起人可见。需另定已删除任务作业的清理策略。

2. **内外区分、未知模式与首页/大盘统计口径**
   - 在 `JobInstance` 上显式区分提交来源（`source = "stx" | "external"`，现有作业默认 `stx`），外部作业允许 `TaskID = 0`、`CreatedBy = 0`。
   - 外部作业的模式取自引擎返回的 `jobDag.envOptions["job.mode"]`（`STREAMING` / `BATCH`），缺失时按引擎默认 `batch`；仅在拿不到 `jobDag` 时 `job_mode` 才为空串，前端展示为“-”，按 `mode=batch|streaming` 筛选只匹配已知模式作业。
   - 首页汇总（`HomeJobSummary`）与大盘统计（`JobDashboard` / `RunningJobCountsByCluster`）**包含外部作业**（真实反映集群负载、总 QPS 与失败状态）；`RunningStreaming` 与 `RunningBatch` 只统计已知模式作业，不得把未知模式混入 `RunningStreaming`。

3. **统一作业权限模型（Backend + Frontend）**
   - 收敛后端分散在列表、单条、大盘、首页、取消、恢复各处的权限逻辑，在领域层建立统一的 `JobPermissions` / 判定函数（`CanView` / `CanStop` / `CanRecover`），并在 API 响应边界派生输出（`can_stop`、`can_recover`、`can_view` 或等价统一结构）。
   - 外部作业适用 **方案 B**：所有登录用户对外部作业**只读可见**（列表、详情、拓扑、指标、趋势、Sparklines、日志、Checkpoint、事件）；**仅管理员**可停止（Cancel / Stop with Savepoint）；不可恢复（Recover 恒禁用）。
   - 内部作业保持既有权限语义不变：查看沿用“管理员 / 发起人 / 可见任务下作业”；停止沿用“管理员 / 发起人 / 任务所有者”；恢复沿用“管理员 / 任务可运行权限”。
   - 前端全局列表与详情直接消费后端派生的作业权限与统一 `getJobActionState`，不再由 `JobListTab` 硬编码 `canRun: true` 猜测权限。

4. **开关与告警边界**
   - 外部作业发现**默认开启**：提供全局配置开关（`config.yaml`）与集群级覆盖开关（集群配置 + 接口/UI），仅当全局和集群最终判定为开启时才对该集群执行外部发现。
   - 外部作业失败或停止超时**暂不触发**作业告警（`alert_signals.go` 显式跳过外部作业），仅做状态与指标观测。

5. **脚本依赖能力降级**
   - 复用：作业列表（支持按 `source` 筛选与来源徽标）、对账与刷新、拓扑（基于 `job_dag`）、指标、Prometheus 趋势与 Sparklines、集群日志、Checkpoint、事件时间线、CDC 延迟、停止（补齐 `runtimeResolver` 端点兜底）。
   - 禁用/降级：计划 DAG（返回 `reason=no_submitted_content`）、历史恢复（后端明确拒绝 + 前端隐藏/禁用）、脚本页签（展示“外部提交，无脚本快照”）、数据预览（不适用）。

## Acceptance Criteria

- [ ] **不重复拉取**：`reconcileActiveJobs` 每轮对每个集群最多执行一轮 `reconcileClusterJobs`；已纳管 STX 作业与外部作业发现共享同一次 `ListRunningJobs` / `ListFinishedJobs` 调用；事件模式下仍在完整对账周期中发现外部作业。
- [ ] **全量状态覆盖与限流保护**：运行中及已结束（FINISHED/FAILED/CANCELED/SAVEPOINT_DONE）的外部作业均可被发现；同 `(cluster_id, engine_job_id)` 不重复建表行（包括已有 `recover` 历史行、STX 提交与对账并发、已删除任务的历史作业）；每轮新建行数受上限控制，传输失败立即止损并触发退避。
- [ ] **引擎调用去重**：每轮每集群最多 1 次 `/running-jobs` + 1 次 `/finished-jobs`；已结束作业不再逐个调 `/job-info`；没有活跃作业的集群按 `external_job_discovery_interval_seconds` 巡检。
- [ ] **模式识别**：带 `job.mode` 的外部作业按其模式统计；未写 `job.mode` 的按 `batch`。
- [ ] **事件加速发现**：3.0+ 开启事件通道与发现开关的集群，收到未知 `engine_job_id` 的 `JOB_STATUS` 事件后能防抖调用 `GetJobInfo` 建出外部作业并把同批/后续事件关联到 `job_instance_id`。
- [ ] **开关控制**：全局关闭或单集群显式关闭后，不再为该集群发现新外部作业，已有历史记录保持可读；默认情况下集群外部发现开启。
- [ ] **统一作业权限模型**：
  - 后端所有作业入口（列表、单条、日志、Checkpoint、实时、计划 DAG、时序、Sparklines、事件、首页、大盘、取消、恢复）共用同一套权限裁决；
  - 普通登录用户可只读查看外部作业，调用 `Cancel` 返回 403/权限错误；管理员可停止外部作业；外部作业调用 `Recover` 返回明确拒绝错误；
  - 作业列表/详情响应携带权威权限标记，前端 `JobListTab` 与 `JobDetailDialog` 行为一致且与后端裁决一致。
- [ ] **统计与展示正确**：未知 `mode` 的外部作业不被算进 `HomeJobSummary.RunningStreaming`；列表支持区分并筛选 `stx` / `external`；无脚本外部作业的 `/plan-dag` 返回 `no_submitted_content`，日志与 Checkpoint 在端点回退后可正常读取。
- [ ] **质量门禁**：根 Go 模块单元测试（含 `-race`）、三库迁移兼容测试、前端 `tsc` 与 Vitest、CLI 契约与路由基线/Swagger 校验全部通过。

## Out of Scope

- 外部作业失败告警（B1 / A1 / A2 / C1）及任务级告警策略绑定
- 从外部作业反向生成/导入为可编辑的 STX 工作台 `Task`
- 集群级别的读取权限隔离（按用户隔离集群可见性）
