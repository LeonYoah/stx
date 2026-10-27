# SeaTunnel 实时可观测性与事件上报

> 适用于 `internal/apps/sync` 的实时指标代理（`/metrics/realtime`）、提交时注入 `env.engine.observability`，以及 SeaTunnel `event-report-http` 事件通道。引擎侧事实见 `.trellis/tasks/archive/*/10-25-realtime-observability-events/research/engine-capabilities.md`（未归档时在 `.trellis/tasks/10-25-realtime-observability-events/`）。

## 场景一：提交时注入实时可观测性

### 1. 范围 / 触发条件
- 集群模式提交（运行、恢复、定时）都经过 `submitTaskInstanceWithPayload`，在这里决定是否往提交内容里注入 `env.engine.observability`。
- 3.0.0 的实时指标是**作业级开关**，默认关闭；不注入的话，引擎自带 Web UI 的背压视图也是空的。

### 2. 签名
```go
func (s *Service) applyObservabilityInjection(ctx context.Context, task *Task, body []byte, format string) ([]byte, observabilityInjection)
// 任务设置：task.Definition["observability"] = "auto" | "on" | "off"（缺省 auto）
// 结果写入：SubmitSpec["observability"] = {injected, mode, reason, streaming, version, supported}
```

### 3. 契约
- HOCON：在内容最前面加注入标记行和 `env { engine { observability { enabled = true, bucket_ms = 5000, retention_minutes = 10 } } }`。HOCON 同名对象合并、后写覆盖先写，所以用户配置优先。根对象带花括号时，插到第一个 `{` 后面。
- JSON：只在 `env.engine.observability` 缺失时补上，用 `UseNumber` 解码，避免大整数变成浮点数。
- `supported` 由后端按集群版本判定（≥3.0.0）。前端靠它决定是否显示入口，**前端不做版本比较**。

### 4. 校验与原因对应表
| 情况 | reason | 是否注入 |
| --- | --- | --- |
| 内容里已有注入标记（恢复作业复用了提交内容） | `already_injected` | 否（injected=true） |
| 任务设为 `off` | `disabled_by_task` | 否 |
| `auto` 且不是流式作业（优先看内容里的 `job.mode`，其次看 `Task.Mode`） | `batch_job` | 否 |
| 取不到版本 / 版本 <3.0.0 | `version_unknown` / `version_unsupported` | 否 |
| 用户内容里已经出现 `observability` | `user_configured` | 否 |
| SQL 格式 / JSON 解析失败 | `format_unsupported` / `invalid_content` | 否，原样提交 |
| 其他情况 | `injected` | 是 |

### 5. Good / Base / Bad
- Good：3.0.0 集群上的流式任务提交后，`submitted_content` 以标记行开头，引擎 `/metrics/realtime/jobs` 里能看到该作业，且 `enabled=true`。
- Base：2.3.x 集群提交的内容逐字不变，`reason=version_unsupported`。
- Bad：把片段追加到末尾，覆盖了用户显式写的 `enabled=false`。

### 6. 必须有的测试
- 上表每一行都要有用例：HOCON 前置合并、花括号根对象、JSON 合并、大整数保留、重复注入识别。
- 取版本失败时退回 `Task.EngineVersion`。

### 7. 错误与正确示例
```hocon
# 错误：追加在末尾，会覆盖用户的 enabled = false
env { parallelism = 1, engine { observability { enabled = false } } }
env { engine { observability { enabled = true } } }
# 正确：放在最前面，用户配置在后、优先生效（用户已写 observability 时直接不注入）
```

## 场景二：实时指标代理 `/api/v1/sync/jobs/:id/realtime`

### 1. 范围 / 触发条件
- 代理引擎 v2 的 `/metrics/realtime/jobs[/{id}/vertices|edges]`，并计算瓶颈提示。
- **不自建背压 DAG 和曲线**：3.0.0 自带的 Web UI 作业详情页（`#/jobs/:jobId`）已经有了，前端用深链跳过去。

### 2. 签名
```go
type EngineRealtimeClient interface { GetRealtimeJobs; GetRealtimeVertices; GetRealtimeEdges } // 可选接口，类型断言获取
type ClusterEngineEndpointLister interface { ListEngineEndpoints(ctx, clusterID, def) ([]*EngineEndpoint, error) }
func (s *Service) GetJobRealtimeForActor(ctx, actor, id uint, windowMs int64) (*JobRealtimeResult, error)
```
CLI：`stx sync job realtime <id> [--window_ms 180000]`

### 3. 契约
- **只有 active master 能回答**：非 master 返回 404，响应体包含 `Not a master node`（映射为 `ErrEngineNotMaster`）；服务未启动返回 503（映射为 `ErrEngineRealtimeDisabled`）。
- 依次探测候选节点的 `/metrics/realtime/jobs`，按集群缓存 master 60 秒；遇到非 master 时作废缓存并重试一次。
- 同一 `(jobID, windowMs)` 缓存 2 秒，并发请求合并，多人查看只打一次引擎。
- `window_ms` 取值范围是 (0, 600000]，0 或负数取默认值 180000。
- 引擎侧的任何失败都返回 **HTTP 200 + `available=false` + `reason`**；只有作业不存在返回 404，权限沿用 `getJobInstanceForActor`。
- 数组字段（`vertices`、`edges`、`hints`）必须是 `[]`，不能是 `null`。节点类型和名称从 `result_preview.job_dag` 取：STX 预览 DAG 用 `connectorType`，引擎 job-info 用 `vertexName`。
- 调用这个接口、`GetJob`、`GetJobLogs` 都会给作业打"查看"标记（`jobWatchedRecently`，30 秒窗口），供自适应轮询使用（自适应轮询默认关闭，见 [job-status-reconciler](./job-status-reconciler.md)）。

### 4. 原因对应表
`version_unsupported`、`not_cluster_mode`、`not_running`、`not_enabled`（引擎返回 enabled=false）、`master_unreachable`、`no_engine_job`、`collecting`（作业未知，或还没有数据点）、`client_unsupported`。

### 5. Good / Base / Bad
- Good：多节点集群里第一个节点返回 404 `Not a master node`，第二个返回 200，结果来自第二个节点，并且缓存下来。
- Base：作业刚提交，引擎返回 `{}`，结果为 `reason=collecting`。
- Bad：直接用 `ResolveEngineEndpoint`（第一个在线节点）请求实时接口，多 master 集群里会随机失败。

### 6. 必须有的测试
- 用 httptest 模拟节点：200、404 加 `Not a master node`、503、`{}`。
- master 缓存过期后会刷新；8 个并发请求只打 1 次引擎；每一种 reason 都有用例。
- 瓶颈提示规则：每个 code 一正一反，外加"只连续 2 个桶不命中"。

### 补充：集群观测地址 `GET /api/v1/clusters/:id/webui-endpoints`
- 返回 `{available, reason, proxy_base_url, native_base_url, node_id, host_name, job_path_template:"#/jobs/{job_id}"}`，CLI 是 `stx cluster webui endpoints <id>`。
- `reason` 取值：`version_unsupported`（<2.3.9）、`http_disabled`（`config.runtime.enable_http=false`）、`no_webui_node`、`node_address_missing`。
- 节点选择和 `ProxyWebUI` 共用 `webUIMasterSelector`：多个在线 master 时探测 `/metrics/realtime/jobs`，结果缓存 60 秒；探测全部失败时退回第一个在线节点。

### 7. 错误与正确示例
- 错误：引擎请求失败时返回 502，前端每 5 秒轮询一次，全局错误拦截器会不停弹 toast。
- 正确：返回 200 和 `available=false`、`reason=master_unreachable`，前端显示原因，并退回快照指标。

## 场景三：事件通道（`event-report-http` → STX）

### 1. 范围 / 触发条件
- 集群级开关会改写 seatunnel.yaml，需要滚动重启后生效。
- 引擎每 10 秒 POST 一批事件（JSON 数组），收到 2xx 才前进；**只要返回非 2xx，就会一直重试同一批，后面的事件全被堵住**。

### 2. 签名
```text
GET  /api/v1/sync/event-channels/:cluster_id             (R0, CLI: stx sync event-channel get)
PUT  /api/v1/sync/event-channels/:cluster_id             (R2, 仅管理员；CLI: stx sync event-channel update <id> --enabled=true|false [--rotate-token] --confirm)
POST /api/v1/sync/engine-events/:cluster_id              (公开路由，登记为 ModeServerOnly；鉴权用 Authorization: Bearer <token>)
GET  /api/v1/sync/jobs/:id/events?types=&page=&page_size= (R0, CLI: stx sync job events)
表：sync_engine_event_channels（cluster_id 主键）、sync_engine_events（dedupe_key 唯一）
```

### 3. 契约
- **token**：`base64url(HMAC-SHA256(app.session_secret, "engine-events:{cluster}:{nonce}"))`。库里只存 nonce，轮换就是换 nonce；接口只返回 `token_set`。
- **写入 seatunnel.yaml**：`seatunnel.engine.event-report-http.{url, headers.Authorization}`；≥3.0.0 时加 `report-non-terminal-job-state: true`，**这个键必须写在 `event-report-http` 下面**。
  - url 为 `{app.external_url}/api/v1/sync/engine-events/{cluster_id}`。
  - 通过 `config.Service.ApplyEventReportConfig` 对模板和每个节点**逐个**用 yaml.Node 打补丁（有待推送内容时基于待推送内容），不用模板覆盖节点。
- **接收**：
  - token 错误返回 401。
  - 通道未开启、JSON 非法、cluster_id 非法、请求体超过 4MB、未知作业：一律返回 200。
  - 落库用 `context.WithoutCancel`，引擎断开连接也要写完。
- **清洗**：
  - 丢弃 `STAIN_TRACE`。
  - `READER_MESSAGE_DELAYED` 只保留 `delay_ms`，**删除 `record`（整行数据）**，按分钟桶去重，分钟点保存本批最大值。
  - `SCHEMA_CHANGE_*` 递归删除 `changeAfter`（整表 schema）。
  - 所有 payload 都经过 `audit.RedactDetails` 脱敏，单条超过 16KB 时只保留标量字段。
- **去重**：sha256(cluster|jobId|type|status|createdTime|payload)。先查已有的去重键，再用 `ON CONFLICT DO NOTHING` 插入。换主后整批重放全部算重复，**不再触发刷新**。
- **分发**：
  - `JOB_STATUS` 按作业防抖 1 秒后调 `RefreshJobByID`。**事件里的状态本身不直接写进作业**，状态的准数始终来自引擎 job-info。
  - CDC 延迟只在内存里聚合，按样本原始时间单调更新，通过 `JobInstance.CDCDelay`（`gorm:"-"`）在 `GetJob`/`ListJobs` 里带出；`fresh` 表示 60 秒内收到过样本。
  - 作业按 `engine_job_id` 找最新实例，并校验属于该集群，属于别的集群视为未知作业。
- **健康**：
  - 状态取值 `disabled | pending_restart | healthy | degraded`。
  - 3.x 集群在通道已收到过事件的前提下，作业提交后 45 秒内没收到状态事件，就记 `expect_missed_at`，判定为降级。
  - 提交**结果未知**（`submit_outcome=unknown`，见 [作业状态对账器 · 写操作结果未知](./job-status-reconciler.md)）的作业不登记这个预期：引擎可能根本没有这个作业，登记了会把健康的通道误判为降级。

### 4. 校验与错误对应表
| 情况 | 行为 |
| --- | --- |
| 非管理员调用 PUT | 403 `ErrEventChannelPermissionDenied` |
| 版本 <2.3.12 / 没有 `app.external_url` / 没有 `session_secret` | 400 |
| 集群在 STX 里没有 seatunnel.yaml 记录 | `config.ErrSeatunnelConfigMissing` |
| 接收端 token 错误 | 401，`auth_fail_count++` |
| 接收端内容非法 | 200 `{"ignored":"invalid_payload"}`，`parse_error_count++` |
| 整批重放 | 200，`accepted=0`，不刷新 |

### 5. Good / Base / Bad
- Good（t1 实测）：提交后约 7.6 秒收到 PENDING/SCHEDULED/RUNNING；取消后 8.2 秒收到 CANCELED，9.6 秒时作业已是终态；STX 停机期间的事件恢复后整批补发；PG CDC 的 ADD COLUMN 产生 `SCHEMA_CHANGE_UPDATE_COLUMNS`，批量写入产生延迟事件（2362 ms）。
- Base：2.3.12 只推终态，通道可以开，但不进入自适应轮询。
- Bad：内容解析失败时返回 400，引擎会无限重试这一批，后面所有作业的事件都收不到。

### 6. 必须有的测试
- token：区分集群和 nonce、大小写不敏感的 Bearer、没有 secret 时失效。
- 开关：权限、版本、external_url、2.3.12 不写非终态键、关闭时移除配置并清空 token。
- 接收：401/200 的 HTTP 契约、批内去重与整批重放、`record` 不入库、跨集群作业不关联、schema payload 去掉 `changeAfter` 后脱敏。
- 防抖、预期事件、状态计算；config 补丁的新增、覆盖、删除、保留注释和节点内容。

### 7. 错误与正确示例
```go
// 错误：直接把事件里的状态写进作业，被伪造或重放的事件都能篡改状态。
instance.Status = normalizeJobStatus(event.JobStatus)
// 正确：事件只用来触发刷新，状态的准数来自引擎 job-info。
s.enqueueEventRefresh(job.ID) // → RefreshJobByID → applyEngineJobInfoUpdate
```

## 已知限制
- token 明文会出现在 seatunnel.yaml 里，而 config 接口返回配置正文时不做脱敏（这是已有行为，checkpoint 密钥也一样）。影响有限：这个 token 只能往该集群的接收接口发事件，伪造的状态事件只会触发一次从引擎拉真实状态。
- `STAIN_TRACE` 和 `JOB_STATUS` 共用一个容量 2000 的 ringbuffer。打开 stain trace 前要压低采样率，否则状态事件会被挤掉。
