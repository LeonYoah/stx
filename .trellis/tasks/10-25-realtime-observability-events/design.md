# 技术设计：3.x 实时作业事件 + 实时可观测性

需求见 `prd.md`，引擎事实见 `research/engine-capabilities.md`，STX 现状见 `research/stx-current-state.md`。

## 1. 边界与模块

全部放在根模块的 `internal/apps/sync`，不改 agent 和 proto。只有"改写 seatunnel.yaml"一项借用 `internal/apps/config`，经接口在 `router.go` 里组装。

```
internal/seatunnel/capabilities.go      + 3 个版本能力
internal/apps/sync/
  observability_inject.go   (新) 提交时注入 env.engine.observability
  realtime.go               (新) master 定位、实时代理、合并请求、瓶颈提示
  engine_client_realtime.go (新) EngineRealtimeClient 可选接口与 SeaTunnelEngineClient 实现
  engine_events.go          (新) 事件接收、清洗、去重、分发、通道健康、清理
  engine_events_model.go    (新) EngineEventChannel / EngineEvent 模型
  job_reconciler.go         (改) 自适应轮询
  service.go / handler.go / repository.go / err.go (改)
internal/apps/config/event_report.go     (新) 按 yaml.Node 写入或移除 event-report-http
internal/router/router.go                (改) 路由与依赖注入
internal/operation/registry_sync.go      (改) CLI 操作；registry.go 加 ServerOnly 条目
internal/db/migrator/migrator.go         (改) 注册两张新表
frontend/...                             见第 7 节
```

## 2. 能力门控

```go
// internal/seatunnel/capabilities.go
SupportsRealtimeObservability(version) bool      // >= 3.0.0
SupportsJobStateEvents(version) bool             // >= 2.3.12
SupportsNonTerminalJobStateEvents(version) bool  // >= 3.0.0
```

- 版本来源：`ClusterVersionProvider.GetClusterVersion(ctx, clusterID)`。
- 取版本失败或版本为空时，按"不支持"处理：保守，等于没开新功能。

## 3. 提交时注入（R2）

- **注入点**：`submitTaskInstanceWithPayload` 一开始（运行、恢复、定时都会经过），在写入 `SubmitSpec.submitted_content` 之前完成。本地执行走 `submitLocalTaskInstance`，不经过这里，所以天然不注入。
- **判定**：
  - `definition.observability` 取 `on` → 开；`off` → 关；缺省或 `auto` → 判断是否流式。
  - 流式判定：先在内容里匹配 `job.mode`。正则大致是 `job\.mode\s*[:=]\s*"?STREAMING`，再加上 JSON 形式 `"job.mode"\s*:\s*"STREAMING"`，忽略大小写。
  - 内容里匹配不到时，用 `Task.Mode == TaskModeStreaming`。
- **HOCON**：在内容最前面加一段 `env { engine { observability { enabled = true, bucket_ms = 5000, retention_minutes = 10 } } }` 并换行。
  - HOCON 同名对象会合并，后写的同名键覆盖先写的，所以用户自己写的配置（包括 `enabled=false`）优先。
  - 用户内容里已经出现 `observability` 关键字时，**不注入**：避免和 `enabled` 的自动推断规则打架，交给用户的配置决定。
- **JSON**：解析成 map，在 `env.engine.observability` 下只补缺失的键；用户已经有 `observability` 对象时不动；解析失败就原样提交。
- **记录**：`SubmitSpec.observability = {"injected": bool, "mode": "auto|on|off", "reason": "...", "streaming": bool}`。
  - `reason` 取值：`injected`、`user_configured`、`disabled_by_task`、`batch_job`、`version_unsupported`、`version_unknown`。
  - `/realtime` 借这个记录区分 `not_enabled` 的具体原因。

## 4. 实时代理（R3）

### 4.1 引擎客户端（可选接口，不扩展 `EngineClient`）

```go
type EngineRealtimeClient interface {
    GetRealtimeJobs(ctx, ep *EngineEndpoint) (*EngineRealtimeJobs, error)
    GetRealtimeVertices(ctx, ep *EngineEndpoint, jobID string, windowMs int64) (*EngineRealtimeSeries, error)
    GetRealtimeEdges(ctx, ep *EngineEndpoint, jobID string, windowMs int64) (*EngineRealtimeSeries, error)
}
var ErrEngineNotMaster = errors.New(...)         // 引擎返回 404 且 body 含 "Not a master node"
var ErrEngineRealtimeDisabled = errors.New(...)  // 引擎返回 503
```

- 请求走 v2 的 `BaseURL+ContextPath`，复用 `SeaTunnelEngineClient.httpClient`，单次超时 3 秒。
- point 用 `map[string]any` 原样透传：保留引擎的 camelCase 字段名，引擎以后加字段也不用改代码。

### 4.2 定位 master

- 新增可选接口 `ClusterEngineEndpointLister.ListEngineEndpoints(ctx, clusterID, def)`，返回全部在线节点，顺序和 `ResolveEngineEndpoint` 一致。由 `DefaultClusterRuntimeResolver` 实现。
- `realtimeMasterCache`：按集群缓存 `{endpoint, expiresAt=60s}`。
  1. 先用缓存；遇到 `ErrEngineNotMaster` 或网络错误就作废缓存。
  2. 依次对候选节点调 `GetRealtimeJobs`，第一个返回 200 的当作 master。
  3. 全部失败时返回 `master_unreachable`。

### 4.3 Service 与返回结构

`GetJobRealtime(ctx, jobID uint, windowMs int64)` 的判断顺序：

1. 作业是否存在：不存在 → 404 `ErrJobNotFound`。
2. 是否本地模式：是 → `not_cluster_mode`。
3. 有没有 `EngineJobID`：没有 → `no_engine_job`。
4. 集群版本是否 ≥3.0.0：不是 → `version_unsupported`。
5. 作业是否运行中（`running`/`cancel_requested`/`cancelling`/`doing_savepoint`）：不是 → `not_running`。
6. 定位 master，并行拉节点和边的数据。
7. 引擎 `enabled=false` 或作业不在列表里时，结合 `submit_spec.observability.reason` 返回 `not_enabled`。
8. 两边都还没有数据点 → `collecting`。

```jsonc
// GET /api/v1/sync/jobs/:id/realtime?window_ms=180000
{"data": {
  "available": true, "reason": "",
  "engine_job_id": "1234", "bucket_ms": 5000, "window_ms": 180000,
  "from_ms": 0, "to_ms": 0, "fetched_at_ms": 0,
  "observability": {"injected": true, "reason": "injected"},
  "vertices": [{"vertex_id": 1, "points": [{"ts": 0, "sourceReadRatio": 0.4}]}],
  "edges":    [{"queue_id": -5, "target_vertex_id": 3, "points": [{"ts": 0, "bpRatio": 0.6, "queueFillRatio": 0.9}]}],
  "hints":    [{"code": "sink_slow", "severity": "warning", "vertex_id": 3, "queue_id": -5, "buckets": 3}]
}}
```

- `window_ms`：默认 180000，最大 600000，越界时截断；0 取默认值。
- **合并请求**：key 为 `(jobID, windowMs)`，结果缓存 2 秒；同一时刻的并发请求只发一次（singleflight，用 mutex 加 map 实现，不引新依赖）。
- **查看标记**：`GetJobRealtime`、`GetJob`、`GetJobLogs` 都会更新 `jobWatch[jobID]=now`（内存 `sync.Map`），供第 6 节使用。
- **错误语义**：引擎相关的失败都转成 `available=false` 加 `reason`，HTTP 200；只有作业不存在返回 404。这样前端轮询不会刷屏报错。

### 4.4 瓶颈提示（Go，`realtimeHints`）

- 只看最近 3 个桶，3 个桶都满足才算命中。阈值是常量。
- 边和节点的对应关系：边的 `target_vertex_id` 就是它指向的节点；节点的下游关系从作业的 `result_preview.job_dag.pipelineEdges` 得到。

| code | 条件 | 定位 |
| --- | --- | --- |
| `sink_slow` | sink 节点 `sinkBusyRatio≥0.8`，并且输入边 `bpRatio≥0.2` 且 `queueFillRatio≥0.8` | 该 sink |
| `transform_busy` | transform 节点 `transformBusyRatio≥0.8`，并且输入边同样有背压 | 该 transform |
| `downstream_backpressure` | 边 `bpRatio≥0.5` 且 `queueFillRatio≥0.8`，但目标节点忙碌度 <0.8 | 该边（可能是外部系统或 GC） |
| `source_idle` | source `sourceIdleRatio≥0.8`，并且它所有可观测的下游边 `queueFillRatio≤0.1` | 该 source（上游没数据） |
| `source_busy` | source `sourceReadRatio≥0.8`，并且第一条可观测的下游边 `bpRatio<0.05` | 该 source |

- `severity`：`bpRatio≥0.5` 为 `critical`，其余为 `warning`。
- 前端按 `code` 显示本地化文案，后端不返回用户文案（遵守"后端不拼双语"的约定）。

## 5. 事件通道（R4）

### 5.1 模型（AutoMigrate）

```go
// sync_engine_event_channels：每个集群一行
type EngineEventChannel struct {
    ClusterID          uint `gorm:"primaryKey"`
    Enabled            bool `gorm:"not null"`
    TokenNonce         string `gorm:"size:64"`   // HMAC 盐，轮换 token 时改它；不是 token 本身
    ReportNonTerminal  bool `gorm:"not null"`
    ConfigAppliedAt    *time.Time
    ConfigApplyError   string `gorm:"type:text"`
    LastEventAt        *time.Time
    LastBatchAt        *time.Time
    ReceivedCount      int64
    DuplicateCount     int64
    DroppedCount       int64   // STAIN_TRACE 或无法解析的条目
    ParseErrorCount    int64   // 整批 JSON 无法解析
    ExpectMissedAt     *time.Time
    ExpectSatisfiedAt  *time.Time
    CreatedAt, UpdatedAt time.Time
}
// sync_engine_events：清洗后的事件
type EngineEvent struct {
    ID            uint
    ClusterID     uint   `gorm:"index:idx_see_cluster_time"`
    EngineJobID   string `gorm:"size:64;index"`
    JobInstanceID *uint  `gorm:"index"`
    EventType     string `gorm:"size:64;index"`
    JobStatus     string `gorm:"size:32"`
    EventTimeMs   int64  `gorm:"index:idx_see_cluster_time"`
    Payload       JSONMap `gorm:"type:json"` // 已清洗，单条上限 16KB
    DedupeKey     string `gorm:"size:64;uniqueIndex"`
    ReceivedAt    time.Time `gorm:"index"`
}
```

### 5.2 token

- token = `base64url(HMAC-SHA256(SessionSecret, "engine-events:" + clusterID + ":" + nonce))`，写法和 `buildPreviewCollectToken` 一致。
- 校验时重新计算，用 `hmac.Equal` 比较。
- **token 本身不落库**，API 只返回 `token_set: true`。明文只出现在下发到节点的 seatunnel.yaml 里，和现有的 checkpoint 密钥同级。
- `SessionSecret` 为空时，开启返回 `ErrEventChannelSecretMissing`（400）。

### 5.3 开关与下发

- `PUT /api/v1/sync/event-channels/:cluster_id`，请求体 `{"enabled": bool, "rotate_token": bool}`。写操作风险等级 R2，**仅管理员**；CLI 与直接 API 调用走 `BeginSynchronous`（网页请求跳过，与 config 写接口一致），并记审计。
- 开启的前置检查：
  - 集群版本 ≥2.3.12，否则 `ErrEventChannelVersionUnsupported`（400）。
  - `config.GetExternalURL()` 非空，否则 `ErrEventChannelExternalURLMissing`（400）。
- url 为 `strings.TrimRight(externalURL,"/") + "/api/v1/sync/engine-events/" + clusterID`。
- ≥3.0.0 时 `report-non-terminal-job-state: true`。
- 通过 sync 包内声明的接口 `EventReportConfigApplier` 调用 config 应用：

  ```go
  ApplyEventReportConfig(ctx, clusterID uint, settings *EventReportSettings /*nil 表示移除*/, userID uint) (*EventReportApplyResult, error)
  ```

  config 应用里的实现：对集群的 seatunnel.yaml 模板和每个节点配置**逐个**用 yaml.Node 写入或删除 `seatunnel.engine.event-report-http`，然后走现有的 `Update`（节点配置先推送再提交新版本，推送失败时标记待推送，agent 重连后自动重推）。**不用 SyncTemplateToAllNodes**，以免覆盖节点上各自的配置。
- 返回结构：

  ```jsonc
  {"enabled": true, "state": "pending_restart", "token_set": true, "url": "...",
   "report_non_terminal": true, "restart_required": true,
   "nodes": [{"host_id": 1, "pushed": true, "error": ""}]}
  ```

- 关闭：移除配置块，`Enabled=false`，提示需要重启。

### 5.4 接收

- 路由：`POST /api/v1/sync/engine-events/:cluster_id` 注册在 `apiV1Router` 下，不要求登录；登记表里标为 `ModeServerOnly`。
- 请求体用 `http.MaxBytesReader` 限制在 4MB（每批最多 1000 条）。
- 处理顺序：
  1. 集群通道不存在或未开启 → **200**，`{"accepted":0,"ignored":"channel_disabled"}`。不返回 4xx，避免关闭后、重启前的旧配置把引擎堵住。
  2. Authorization 不匹配 → **401**，`AuthFailCount++`（只记计数，不打印请求头）。
  3. JSON 解析失败 → 200，`ParseErrorCount++`，并打一次 warn 日志，只记长度不记正文。
  4. 逐条处理，见 5.5。
  5. 更新 `LastEventAt` / `LastBatchAt` / 计数，返回 200 `{"accepted":n,"duplicates":m,"dropped":k}`。
- 处理是同步的：落库和去重都在请求内完成，所以返回 2xx 时已经持久化。分发（刷新作业）异步进行，不阻塞响应。

### 5.5 清洗、去重、分发

| eventType | 入库 | 处理 |
| --- | --- | --- |
| `JOB_STATUS` | ✓ | 按 `jobId` 找作业实例（`engine_job_id`，并校验集群）→ 防抖 1 秒后调 `RefreshJobByID`；满足"预期事件" |
| `SCHEMA_CHANGE_*` | ✓ | 只写时间线；payload 超过 16KB 时截断，并标 `truncated: true` |
| `LIFECYCLE_*` | ✓ | 低频（每个 reader/writer 一条）；时间线默认折叠成计数 |
| `READER_MESSAGE_DELAYED` | 每个作业每分钟 1 行（取本批最大值） | **先删除 `record`**；按样本原始时间单调更新内存里的 `{latest_ms, max_1m_ms, at}`（重放的旧样本被忽略）；去重 key 用分钟桶 |
| `STAIN_TRACE` | ✗ | `DroppedCount++` |
| 其他 / 未知 | ✓（payload 只保留标量字段） | 只写时间线 |

- **去重 key**：对 `clusterID|jobId|eventType|jobStatus|createdTime|规范化 payload` 取 sha256；`READER_MESSAGE_DELAYED` 改用 `clusterID|jobId|READER_MESSAGE_DELAYED|floor(createdTime/60000)`。
- 插入用 `ON CONFLICT DO NOTHING`：GORM 的 `clause.OnConflict{DoNothing:true}`，三种数据库都支持。
- **找不到作业**（STX 之外提交的作业）：照样入库，`JobInstanceID=nil`，不分发。
- **防抖与刷新 worker**：`StartEngineEventRuntime(ctx)` 启动一个协程：
  - 维护 `pending map[jobID]time`，每 250ms 扫描一次，把超过 1 秒的条目交给 `RefreshJobByID`，并发上限 4。
  - 每 5 秒检查一次"预期事件"是否超时。
  - 每小时清理一次 7 天前的事件。
- **CDC 延迟的展示语义**：只有延迟 ≥1 秒时才会有事件。`cdc_delay.at` 距今超过 60 秒时 `fresh=false`，前端不显示徽标，不当作当前延迟。
- **CDC 延迟不写 `result_preview`**（实现时调整）：对账器会整体改写 `result_preview`，两边并发写会互相覆盖。改为内存聚合，通过 `JobInstance.CDCDelay`（`gorm:"-"`，写法同 `sync_lag_ms`）在 `GetJob`/`ListJobs` 响应里带出；STX 重启后在下一条延迟事件到来时恢复。

### 5.6 通道健康

- 状态取值：`disabled | pending_restart | healthy | degraded`。
  - 开启后、收到第一批事件前为 `pending_restart`。
  - 收到过事件，并且最近一次"预期未满足"晚于"最近一次满足"时为 `degraded`。
  - 其余为 `healthy`。
- **预期事件**：版本 ≥3.0.0 且已开启非终态上报时，作业提交成功后在内存里记一条 `{jobID, deadline=now+45s}`（10 秒批量间隔加上余量）。
  - 45 秒内收到该作业任意一条 `JOB_STATUS` → `ExpectSatisfiedAt=now`。
  - 超时 → `ExpectMissedAt=now`。
  - 2.3.12 只有终态事件，不登记预期，所以永远不进入自适应轮询（第 6 节）。
- STX 重启后预期会丢失：保守处理，等下一个作业重新验证。

## 6. 自适应轮询（R5）

`reconcileActiveJobs` 按集群分组后：

```
eventMode := channel.healthy && SupportsNonTerminalJobStateEvents(version)
if !eventMode || now-lastFull[cluster] >= eventModeInterval(默认 60s):
    reconcileClusterJobs(...)                 // 原逻辑不变
    lastFull[cluster] = now
else:
    reconcileClusterJobsLight(...)            // 不拉列表
        对 status=running 的作业:
            watched(≤30s) 或 last_synced_at 早于 30s → GetJobInfo → applyEngineJobInfoUpdate
        其他非终态作业（pending/submitting/cancelling 等）:
            不处理，靠事件刷新；最长 60 秒后由完整对账兜底
```

- 新增配置项 `sync.job_reconciler_event_mode_interval_seconds`：0 表示默认 60 秒，小于 0 关闭自适应（与 `job_reconciler_interval_seconds` 的取值规则一致）。
- `backfillTerminalJobFinalMetrics` 不变。
- `lastFull` 存在内存里，STX 重启后第一轮必然是完整对账。
- 所有原有语义（作业从引擎消失时置为 failed、终态补指标、`recordJobSyncFailure`）都只在完整对账里发生，与现在一致。

## 7. 前端

- `lib/services/sync/types.ts`、`sync.service.ts`：
  - 新增 `SyncJobRealtime`、`SyncRealtimeHint`、`SyncEngineEvent`、`SyncEventChannelStatus` 类型。
  - 新增 `getJobRealtime`、`listJobEvents`、`getEventChannel`、`updateEventChannel`；数组字段统一 `?? []`。
- **不自建背压 DAG 和曲线**（2026-10-25 调整）：3.0.0 自带的 Web UI（`ui/assets/detail-*.js`）已经请求 `/metrics/realtime`，用忙碌度、`bpRatio`、`queueFillRatio` 着色，并用 echarts 画曲线；路由是 hash 模式，作业详情为 `#/jobs/:jobId`。STX 已经能通过 `/api/v1/clusters/:id/webui/` 反向代理访问它（`cluster/handler.go` `ProxyWebUI`，会改写 HTML/JS 路径）。
- 新组件 `components/common/sync/RealtimeInsightPanel.tsx`（轻量）：
  - 轮询 `/realtime`：5 秒一次，`document.visibilityState` 不可见时暂停，作业到终态时停止。
  - 显示瓶颈提示（按 `code` 映射本地化文案，标出节点/边）和不可用原因。
  - 右上角使用通用的 `SeaTunnelObserveMenu`（见下）。
- **通用观测入口 `components/common/cluster/SeaTunnelObserveMenu.tsx`**（2026-10-25 调整）：
  - 下拉菜单，可选"通过 STX 代理打开"或"直连原生地址打开"。
  - 菜单展开时才请求 `GET /api/v1/clusters/:id/webui-endpoints`，同一集群缓存 60 秒。
  - 传了 `engineJobId` 就拼 `job_path_template`（`#/jobs/{job_id}`）。
  - 用在作业行（所有集群模式作业，含批作业）、实时诊断面板和集群详情 Web UI 标签页（替换原来前端自己拼的直连地址）。
  - 后端判断：版本 ≥2.3.9（`SupportsHTTPService`），并且 `config.runtime.enable_http` 不为 false；节点用 `webUIMasterSelector.pick`，和代理选同一个节点。
  - 采用零塌陷加载：首次用骨架，刷新时用顶部细进度条。
- `WebUiDagPreview.tsx` 不改。
- **Web UI 代理指向 active master**：`ProxyWebUI` 目前取第一个在线的 master 角色节点。改为先用一个短时缓存的探测结果：依次请求候选节点的 `GET /metrics/realtime/jobs`，返回 200 的就是 active master；探测失败时退回原来的选法。探测逻辑写在 cluster 应用里（不依赖 sync 包），只用于选择代理目标。
- `DataSyncStudio.tsx`：
  - `onViewJobDag` 时把 STX 作业 ID 传进 DAG 弹窗，作业运行中时挂上实时面板。
  - 作业行或详情显示 CDC 延迟徽标（读 `result_preview.cdc_delay`）。
  - 加"事件时间线"入口（弹窗或抽屉，按时间倒序，生命周期事件折叠）。
- `SettingsSidebarPanel.tsx`：集群模式下加"实时可观测性"选择框（`auto/on/off`），写入 `definition.observability`。
- 集群事件通道入口：在集群详情现有的配置或运维区域加一张卡片，显示状态、开关、轮换 token、需要滚动重启的提示。实现前先找到集群详情页的现有位置。
- 文案放进 `lib/i18n/locales/{zh,en}.json`；后端返回的 `reason` / `code` 只在前端映射成文案。

## 8. 兼容、上线与回滚

- **2.3.x 集群**：不注入、没有实时视图（`version_unsupported`）。事件通道在 2.3.12 可以开，但只有终态事件，不进入自适应轮询。
- **注入的风险**：前置 HOCON 片段理论上可能和极少数写法冲突，比如顶层 `env = null`。缓解办法：
  - 用户内容里出现 `observability` 时不注入。
  - 任务可以设为 `off`。
  - 提交失败时错误原样透传；如有需要，后续可以加"注入失败自动重试不注入"，本轮不做。
- **回滚**：
  - 关闭事件通道会移除配置（需要重启集群）。
  - 所有任务设为 `off`，或设置 `sync.job_reconciler_event_mode_interval_seconds=-1`，就能完全回到现在的行为。
  - 两张新表留着不影响旧版本运行。
- **安全**：
  - 接收端 token 用常量时间比较；不打印 Authorization；`record` 字段落库前就删除。
  - payload 大小有上限；未知字段只保留标量。

## 9. 取舍记录

- 生命周期事件改为"入库，界面折叠"，没有按 PRD 早期写的"只计数"：它们数量少，入库后时间线更完整，实现也更简单。
- 实时数据**不落库**：引擎窗口最长 10 分钟，持久化放到 P3。
- 引擎各种失败都在 HTTP 200 里用 `available=false` 表达：前端每 5 秒轮询一次，用错误状态码会让全局错误拦截器刷屏。
