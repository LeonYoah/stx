# 执行计划

按顺序执行。每个检查点都要能独立编译、测试通过。先做 P1，跑通 t1 实测后再做 P2。

## 0. 准备
- [x] `task.py start` 之后，运行 `trellis-before-dev`，读这些 spec：
  - backend：index、job-status-reconciler、cli-guidelines、database、error-handling、execution-security、logging、quality
  - frontend：index、ui-conventions、api-and-services
  - guides：cross-layer
- [x] 提交前新建特性分支 `feat/realtime-observability-events`。当前分支上的 admin 修复已经以 #67 合进 origin/main，开 PR 前 rebase 掉。

## P1 —— 不需要重启集群

### 1. 能力门控
- [x] `internal/seatunnel/capabilities.go`：加 `SupportsRealtimeObservability`、`SupportsJobStateEvents`、`SupportsNonTerminalJobStateEvents`，并写表驱动测试（2.3.11、2.3.12、3.0.0、3.0.1-SNAPSHOT、空字符串）。

### 2. 提交时注入
- [x] `internal/apps/sync/observability_inject.go`：
  - `resolveObservabilityMode`、`detectStreamingJob`
  - `injectObservabilityHOCON`、`injectObservabilityJSON`
  - `applyObservabilityInjection(ctx, task, body, format) (newBody, record)`
- [x] 在 `submitTaskInstanceWithPayload` 里调用，把结果记到 `SubmitSpec.observability`。
- [x] 测试，覆盖这些情况：
  - 2.3.x 不变
  - 3.0.0 流式注入
  - 批作业不注入
  - `on` 让批作业也注入
  - `off`
  - 内容里已有 observability
  - JSON 合并
  - JSON 无法解析时原样提交
  - 取版本失败
- [x] 现有提交相关测试保持通过。

### 3. 实时代理
- [x] `engine_client_realtime.go`：
  - `EngineRealtimeClient` 接口和类型
  - `SeaTunnelEngineClient` 实现（v2 地址拼接参照现有方法）
  - `ErrEngineNotMaster`、`ErrEngineRealtimeDisabled`
  - 用 httptest 测试：200、404 加 "Not a master node"、503、`{}`
- [x] `engine_client.go`：`DefaultClusterRuntimeResolver.ListEngineEndpoints`，并声明 `ClusterEngineEndpointLister` 接口。
- [x] `realtime.go`：
  - master 缓存和探测
  - `GetJobRealtime`：判断顺序和各个 `reason` 见 design 4.3
  - 合并请求：2 秒缓存加并发合并
  - `jobWatch`
  - `realtimeHints`
- [x] 测试：
  - 多个节点里，第一个返回 404，第二个是 master
  - 缓存失效
  - 每一种 `reason`
  - 合并：并发 N 个请求只请求引擎一次
  - hints 表驱动（5 个 code 各一正一反，再加"只连续 2 个桶不命中"）
- [x] `handler.go`：`GetJobRealtime` handler，`window_ms` 解析和截断。
- [x] `router.go`：注册 `syncJobRouter.GET("/:id/realtime", ...)`。
- [x] `registry_sync.go`：加 `sync.job.realtime` 操作（R0），CLI 是 `stx sync job realtime <id> --window-ms`。
- [x] `GetJob`、`GetJobLogs` 里打上查看标记。

### 4. 前端：轻量提示、深链、任务设置（不自建背压 DAG）
- [x] `types.ts`、`sync.service.ts`：`SyncJobRealtime` 等类型和 `getJobRealtime`。
- [x] `RealtimeInsightPanel.tsx`：轮询、hints、不可用原因、深链到引擎 Web UI 的 `#/jobs/{engine_job_id}`。
- [x] `DataSyncStudio.tsx`：DAG 弹窗里接入提示面板（只对运行中的作业）；作业行加深链入口。
- [x] `internal/apps/cluster`：`ProxyWebUI` 优先选 active master（探测加缓存，失败时退回原逻辑），并写测试。
- [x] `SettingsSidebarPanel.tsx`：可观测性选择框，写入 `definition.observability`。
- [x] `zh.json`、`en.json` 文案。
- [x] 跑 `pnpm exec tsc --noEmit`、`pnpm test`、`pnpm lint`；必要时给新组件写 vitest（工具函数：阈值配色、hint 文案映射）。

### ✅ 检查点 P1
- [x] `go test ./internal/...`，以及前端检查。
- [x] **t1 实测**：
  1. 重建并部署 STX：按 memory `stx-deploy-upgrade` 的步骤升级 `/opt/stx`。
  2. 提交一个 FakeSource → Console 的 STREAMING 任务，带一个 Sql transform 和 `async_boundaries`。
  3. 比较 `curl STX /api/v1/sync/jobs/:id/realtime` 和 `curl localhost:8080/metrics/realtime/jobs/<id>/vertices`。
  4. 浏览器从 STX 作业的深链打开引擎 Web UI，确认背压着色出现；同时看 STX 面板里的提示。
  5. 批作业确认不注入；在任务设置里设为 `off` 后，确认实时视图显示不可用及原因。

## P2 —— 事件通道和自适应轮询

### 5. 模型和仓储
- [x] `engine_events_model.go`：两个模型，加上 `TableName`；在 `migrator.go` 里注册。
- [x] `repository.go`：
  - 通道：获取、更新插入、计数原子递增
  - 事件：`InsertEngineEventsIgnoreDuplicates`、`ListEngineEventsByJob`（分页、按类型过滤）、`PurgeEngineEventsBefore`
  - `FindJobInstanceByEngineJobID`
- [x] 用 SQLite 写仓储测试：去重插入、清理、分页；`./scripts/test_db_compat.sh`。

### 6. config 应用：写入 event-report-http
- [x] `internal/apps/config/event_report.go`：`PatchEventReportHTTP(content, settings|nil) (string, error)`，用 yaml.Node 写入或删除，保留其他键。
  - 测试：新增、覆盖已有配置、删除、原来没有 `engine` 节点、带注释的内容、非法 YAML。
- [x] `Service.ApplyEventReportConfig(ctx, clusterID, settings, userID)`：模板和每个节点配置逐个 `Update`，汇总每个节点的推送结果。测试用假的 agentClient，覆盖推送失败时标记待推送。
- [x] `router.go`：把 config 服务适配成 `syncapp.EventReportConfigApplier`。

### 7. 事件通道：开关、接收、分发
- [x] `engine_events.go`：
  - `buildEngineEventToken`、`validateEngineEventToken`
  - `GetEventChannel`、`UpdateEventChannel`：前置检查、下发、返回结构
  - `IngestEngineEvents`：解析、清洗、去重、计数、更新 CDC 延迟
  - 防抖刷新 worker、预期事件登记与检查、清理（都由 `StartEngineEventRuntime(ctx)` 启动）
  - `ListJobEvents`
- [x] 在 `submitTaskInstanceWithPayload` 提交成功后登记预期事件。
- [x] `err.go`：新哨兵错误，并加进 handler 的状态码映射。
- [x] `handler.go`：`IngestEngineEvents`（MaxBytesReader，401 和 200 的语义见 design 5.4）、`GetEventChannel`、`UpdateEventChannel`（审计）、`ListJobEvents`。
- [x] `router.go`：
  - 公开路由 `apiV1Router.POST("/sync/engine-events/:cluster_id", ...)`
  - `syncRouter` 下的 `/event-channels/:cluster_id`（GET/PUT）
  - `syncJobRouter.GET("/:id/events")`
  - 启动 `StartEngineEventRuntime(ctx)`
- [x] 登记表：
  - `sync.event-channel.get`（R0）
  - `sync.event-channel.update`（R2，需要 `--confirm`，风险说明写清楚"会改 seatunnel.yaml，需要滚动重启"）
  - `sync.job.events`（R0）
  - 接收路由在 `registry.go` 标为 `ModeServerOnly`
- [x] 测试：
  - token 正确与错误
  - 通道未开启时返回 200 并忽略
  - 非法 JSON 返回 200
  - 整批重复只入库一次
  - `record` 被删除
  - STAIN_TRACE 被丢弃
  - 每分钟只记一行延迟
  - JOB_STATUS 防抖：1 秒内多条只刷新一次（用可注入的时钟或调用计数）
  - 预期事件超时后变成 degraded，满足后恢复 healthy
  - 未知作业的事件只入库
  - 响应和日志里没有 token 原文

### 8. 自适应轮询
- [x] `job_reconciler.go`：
  - `eventModeForCluster`、`lastFull` 记录
  - `reconcileClusterJobsLight`
  - 读取配置项 `sync.job_reconciler_event_mode_interval_seconds`（加进 config model 和示例配置）
- [x] 测试：
  - 通道健康时 60 秒内不拉列表
  - 有人查看的作业 10 秒拉一次，没人查看的 30 秒拉一次
  - 降级后立即回到完整对账
  - 版本 <3.0.0 时总是完整对账
  - 配置为 0 时关闭自适应

### 9. 前端：时间线、CDC 延迟、通道开关
- [x] 类型和服务：`listJobEvents`、`getEventChannel`、`updateEventChannel`。
- [x] 作业事件时间线（弹窗或抽屉）、CDC 延迟徽标。
- [x] 集群事件通道卡片：先找到集群详情页里放运维配置的现有位置。显示状态、开关、轮换按钮、重启提示，带二次确认。
- [x] 中英文文案；`tsc`、test、lint。

### ✅ 检查点 P2
- [x] 全部单测，以及 `./scripts/test_db_compat.sh`。
- [x] **t1 实测**：
  1. 开启事件通道，确认 `/opt/seatunnel/config/seatunnel.yaml` 里的配置块正确。
  2. 重启集群：t1 是单节点，直接重启引擎服务即可，具体操作前先确认现有的重启入口。
  3. 提交、取消、制造失败，量出从事件到状态刷新的延迟。
  4. 停掉 STX 60 秒再恢复，看补发和去重。
  5. 把 token 改错，确认收到 401，通道变成 degraded，对账回到 10 秒。
  6. 恢复后，用 `/data/workspace/pg-to-pg.conf` 跑 CDC 并做一次 DDL，看时间线和延迟。

## 收尾
- [x] 对全部改动跑 `trellis-check`。
- [x] 更新 spec：
  - `backend/job-status-reconciler.md`：补充事件模式和自适应轮询
  - 新的事件接收契约：总是返回 2xx、去重、敏感字段处理
  - realtime 只能找 master 的约定
  - 前端实时面板约定
- [x] 用中文写提交信息，末尾带上 Co-Authored-By。
- [x] 按 `execution-security-guidelines.md` 的要求，给 6 个新增路由补 Swagger 注解，重新生成 `docs/` 并更新 `route_baseline.json`（开 PR 前补做）。

## 执行结果（2026-10-25）

- 单测：根模块 `go test ./...` 只有 installer 的 2 个用例失败，原因是本机 `/data` 与 tmpfs 跨设备重命名加上旧的上传残留；同样的用例在 `/tmp` 下的干净 worktree 里能通过，与本任务无关。agent 模块 1 个用例失败，原因是 `/tmp/seatunnel-plugins` 目录权限，同样与本任务无关。前端 tsc 通过，40 个测试文件、261 个用例通过，改动的文件没有新增 lint 问题；`test_db_compat.sh` 在 SQLite 上通过，本机没有 PG/MySQL 实例，这两项跳过。
- t1 实测（3.0.0 单节点）：
  - **注入**：流式作业的提交内容以标记行开头，STX `/realtime` 返回的数据与引擎原始数据一致。
  - **事件延迟**：提交后约 7.6 秒收到状态事件；取消后 8.2 秒收到 CANCELED，9.6 秒时已是终态。
  - **停机补发**：STX 停机期间的事件恢复后整批补发，去重正常。
  - **PG CDC**：`wal_level` 已改成 logical；ADD COLUMN 产生 `SCHEMA_CHANGE_UPDATE_COLUMNS`，30 万行批量写入产生延迟事件（2362 ms），`cdc_delay` 显示为 fresh。
  - **实测中修掉的问题**：引擎 job-info 的节点名字段是 `vertexName`；schema payload 每层都带整表 `changeAfter`，已去掉并脱敏。
- 没有在线上验证（由单测覆盖）：改错 token 后进入降级的路径（恢复需要再重启一次集群）、浏览器里的视觉效果（需要网页登录会话）。
- 说明：集群重启只执行了一次（`stx cluster restart 1`）。之后同一幂等键的重复调用被 auto mode 拦截，没有再执行。
