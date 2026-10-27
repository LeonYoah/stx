# 作业状态读路径与集群对账器

> 适用于 `internal/apps/sync` 中 SeaTunnel 作业实例的观察面与后台对账。

## 原则

- **读路径**：`GET /api/v1/sync/jobs`、`GET /api/v1/sync/jobs/:id` 默认只读 `sync_job_instances`，禁止同步调用 SeaTunnel `/job-info`。
- **刷路径**：`StartJobReconciler` 按 `cluster_id` 分组，**各集群在后台并行对账**（见下文"集群并行对账、止损与退避"），调用 `ListRunningJobs` / `ListFinishedJobs`，仅在状态变迁或必要时调用 `/job-info`。
- **手动刷新**：`GET /api/v1/sync/jobs/:id?fresh=1` 仍可对单条作业调用 `RefreshJobByID`（同步 `/job-info`），供运维或调试；CLI `--wait` 仍只轮询默认读路径。
- **失败语义**：问引擎失败只更新 `sync_state` / `sync_fail_count`，不得因一次 `/job-info` 失败把取消中作业写成 `canceled`。

## 观察字段

| 字段 | 含义 |
| --- | --- |
| `last_synced_at` | 最近一次成功对齐引擎的时间 |
| `sync_state` | `ok` / `degraded` / `stale` / `unknown` |
| `sync_fail_count` | 连续失败次数 |
| `sync_lag_ms` | API 派生：`now - last_synced_at` |

## 配置

`config.yaml` → `sync.job_reconciler_interval_seconds`：默认 10；设为 `-1` 关闭对账器。

`config.yaml` → `sync.job_reconciler_event_mode_interval_seconds`：事件模式下完整对账的最长间隔。**默认 0，表示关闭自适应**；配置正数（例如 60）才开启。

`config.yaml` → `sync.job_reconciler_cluster_concurrency`（默认 4）、`sync.job_reconciler_max_backoff_seconds`（默认 120）、`sync.engine_request_timeout_seconds`（默认 15，读写请求共用，实时指标接口固定 3 秒）：0 或不填都取默认值。见下文"集群并行对账、止损与退避"。

`config.yaml` → `sync.submit_timeout_seconds` / `sync.stop_timeout_seconds`：提交确认、停止确认的截止时长，默认都是 300 秒（0 或不填也是 300）。任务可以在 `definition.submit_timeout_minutes` / `definition.stop_timeout_minutes` 里覆盖（1~120 的整数分钟，跟随任务版本，重新发布后生效）。见下文"写操作结果未知"。

## 写操作结果未知（提交 / 停止）

> 提交过期会触发作业告警 A2（不重复报 B1），停止越过截止时间会触发 C1，见[同步作业告警](./sync-alerts.md)。

### 1. 适用范围 / 触发条件

- SeaTunnel 很忙时，`submit-job`、`stop-job` 可能超过 HTTP 超时（`engine_request_timeout_seconds`，默认 15 秒）；超时后引擎通常仍在处理。把它当普通失败会留下 STX 不知道的作业，重试还会换新 jobId，同一份数据被两个作业同时写。
- 凡是改动 `engine_client.go` 的写请求、`submitTaskInstanceWithPayload`、`CancelJob`、`applyEngineJobInfoUpdate` 或对账孤儿分支时，必须遵守本节。

### 2. 签名

```go
var ErrEngineCallOutcomeUnknown = errors.New("sync: engine call outcome unknown")
type EngineCallUnknownError struct{ Cause error; APIMode, BaseURL, EngineURL string } // errors.Is(err, ErrEngineCallOutcomeUnknown)
func engineTransportErrorIsAmbiguous(err error) bool                                    // engine_call_outcome.go
func (s *Service) recordUnknownSubmitOutcome(ctx, task, instance, endpoint, unknownErr) (*JobInstance, error) // submit_outcome.go
func (s *Service) guardSubmitOutcomePending(ctx context.Context, taskID uint) error    // 返回 ErrSubmitOutcomePending（409）
func (r *Repository) ListPendingJobInstancesByTaskID(ctx context.Context, taskID uint, limit int) ([]*JobInstance, error)
func effectiveSubmitTimeout(task *Task) time.Duration / effectiveStopTimeout(task *Task) time.Duration
```

### 3. 契约

- **错误分类**：
  - 结果未知：已建立连接后的超时、`context` 到期或取消、EOF、连接重置、broken pipe、HTTP 504、读响应体出错。
  - 确定失败：拨号阶段失败（连接被拒、DNS、拨号超时）、TLS 错误、504 以外的非 2xx、其他拿不准的错误。
  - 结果未知时**不回退 REST v1 再提交一次**；确定失败保持原有 v1 回退。2xx 但正文不是 JSON 按成功处理，jobId 用请求里的。
- **提交结果未知**：写一条 `status=pending` 的作业，`engine_job_id` 等于本次 jobId，端点字段按实际请求的端点写；`submit_spec` 写：

  | 键 | 含义 |
  | --- | --- |
  | `submit_outcome` | `unknown` → `confirmed`（引擎给出真实状态）或 `expired`（过截止时间仍是空壳） |
  | `submit_outcome_since` / `submit_outcome_at` | 登记时间 / 结论时间（UTC RFC3339） |
  | `submit_timeout_seconds` / `submit_deadline` | 登记时算好的超时和截止时间；之后改配置不影响已登记的作业 |
  | `submit_error` | 脱敏后的原始错误 |
  | `stop_timeout_seconds` | 每次提交都写，停止时直接用，保证跟随运行时的任务版本 |

  接口返回 200 和这条作业；**不登记**事件通道的"45 秒内应收到状态事件"预期。
- **确认**（`applyEngineJobInfoUpdate` 开头）：unknown 作业拿到空壳直接返回不改；拿到真实状态即 `confirmed` 并随 `UpdateJobStatus` 写回 `submit_spec`。savepoint 恢复（`run_type=recover` 或 `start_with_savepoint=true`）沿用旧 jobId，截止前**不采信终态**。
- **过期**：只在对账的孤儿分支判断（引擎可达且 job-info 是空壳）。截止前继续等；截止后写 `expired`，以 FAILED 落库，`error_message` 是固定英文文案 `submit timed out: ...`，前端按 `submit_outcome=expired` 本地化展示。引擎不可达时只更新 `sync_state`，不判失败。
- **拦截重复提交**：同一任务有截止时间内的 unknown pending 作业时，`submitNewTask`、`RecoverJob`、`submitScheduledTask` 返回 `ErrSubmitOutcomePending`（409，文案带作业 ID 和截止时间）。同一 `external_ref` 的重试先走 `attachExternalRef` 接管，不会被拦。
- **ctx 解绑**：`SubmitTaskWithExecutionOptions`、`RecoverJobWithExecution`、`submitTaskInstanceWithPayload`、`CancelJob` 用 `context.WithoutCancel`；`attachExternalRef` 的等待仍可被调用方取消。
- **停止**：进入 `cancel_requested` 时在 `result_preview` 写 `cancel_requested_at`、`cancel_timeout_seconds`、`cancel_deadline`。`stop-job` 结果未知时**不恢复成运行中**，按成功路径进入 `cancelling`，写 `cancel_outcome=unknown`、`cancel_error`；到终态时改为 `confirmed`。确定失败保持原有恢复和"cancel request failed"。
- **停止超时**：`reconcileActiveJobs` 只看本地截止时间，越过时写一次 `cancel_overdue_at`，状态不变；作业响应带派生字段 `cancel_overdue`。

### 4. 校验与错误对应表

| 情况 | 结果 |
| --- | --- |
| 提交超时 / 504 / 连接中途断开 | 200，pending + `submit_outcome=unknown` |
| 提交连接被拒、引擎 500 | 原样返回错误，不建记录（Dolphin 仍留 FAILED） |
| unknown 作业出现在 running/finished 列表或 job-info 有状态 | 按引擎状态更新，`confirmed` |
| 截止前 job-info 是空壳 | 保持 pending |
| 截止后 job-info 是空壳 | failed + `expired` |
| 引擎不可达 | 只更新 `sync_state` |
| 截止前同一任务再次提交 / 恢复 | 409 `ErrSubmitOutcomePending` |
| 任务超时覆盖不在 1~120 的整数分钟 | 400 `ErrInvalidTaskTimeout` |
| 停止超时 / 504 | 200，`cancelling` + `cancel_outcome=unknown` |
| 停止确定失败 | 恢复原状态 + "cancel request failed" |

### 5. Good / Base / Bad

- Good：引擎压力大，提交 15 秒没回复；作业显示"提交确认中"，20 秒后对账在 running 列表里看到它，转为运行中；期间用户再点运行收到 409。
- Base：引擎正常回复，行为和以前完全一致，只多写一个 `stop_timeout_seconds`。
- Bad：超时后报"提交失败"，用户重试又提交了一个新 jobId，两个作业同时写目标表；或者超时后改走 v1 用同一 jobId 再提交一次。

### 6. 必须有的测试

- `engine_client_outcome_test.go`：分类函数表驱动；超时不回退 v1；连接被拒仍回退；504 未知、503 回退；2xx 非 JSON 成功；停止超时未知。
- `submit_outcome_test.go`：登记字段和截止时间（全局 / 任务覆盖）；确定失败不建记录；调用方取消后仍落库；拦截与过期后放行；同一 ref 接管；不登记事件预期；对账确认 / 截止前等待 / 过期判失败；恢复提交截止前不采信旧终态；停止未知保持 cancelling 与终态确认；停止确定失败恢复；停止超时只标记一次；超时解析与生效值。
- CLI：unknown 提交和停止打印 warning；409 走冲突退出码 6。

### 7. 错误与正确示例

```go
// 错误：任何传输错误都回退 v1，同一 jobId 被提交两次。
// Wrong: fall back to v1 on any transport error, submitting the same jobId twice.
resp, err := c.httpClient.Do(httpReq)
if err != nil && legacyURL != "" {
    return c.submitV1(ctx, req)
}

// 正确：请求可能已送达时返回结果未知，交给对账按 jobId 确认。
// Correct: when the request may have arrived, return an unknown outcome for reconcile to confirm by jobId.
if engineTransportErrorIsAmbiguous(err) {
    return nil, newEngineCallUnknownError(err, "v2", req.Endpoint.BaseURL, targetURL)
}
```

## 集群并行对账、止损与退避

### 1. 适用范围 / 触发条件
- 改动 `reconcileActiveJobs`、`reconcileClusterJobs(Light)`、`job_reconciler_scheduler.go`、`refreshJobInstance` 的引擎调用，或引擎客户端的 HTTP 超时时。
- 目标：一个 SeaTunnel 集群很慢或连不上时，其他集群照常按对账间隔更新；不再每轮对这个集群发十几次请求；恢复后立即追上。

### 2. 签名
```go
func (s *Service) reconcileActiveJobs(ctx context.Context) error            // 只分派，不等集群对账结束
func (s *Service) reconcileClusterJobs(ctx context.Context, clusterID uint, jobs []*JobInstance) error
func (s *Service) reconcileClusterJobsLight(ctx context.Context, clusterID uint, jobs []*JobInstance) error
func (r *clusterReconcileScheduler) tryDispatch(ctx context.Context, key uint, run func(context.Context) error, done func(error)) bool
func (r *clusterReconcileScheduler) finish(key uint, err error, interval, maxBackoff time.Duration) clusterReconcileOutcome
func (r *clusterReconcileScheduler) inBackoff(key uint) bool
func isEngineTransportError(err error) bool                                  // engine_call_outcome.go
```

### 3. 契约
- **分派**：tick 协程只做查库、停止超时标记、本地作业刷新，然后每个集群调用一次 `tryDispatch`。集群对账在后台协程里跑，最多 `job_reconciler_cluster_concurrency` 个同时执行（信号量，首次分派时按配置创建）。`reconcileActiveJobs` 不等它们结束。
- **跳过**：集群上一轮还没结束（`inFlight`），或者还在退避期（`now < nextAttemptAt`）时，本轮直接跳过，不排队。
- **直连作业**（没有 `cluster_id`、只有 `submit_spec.engine_base_url`）：共用调度键 `0`，作为一个单元在后台逐个对账，彼此不止损，**不退避**（可能分属不同地址）。
- **止损**：`reconcileClusterJobs` 遇到以下情况立即返回非 nil 错误，本轮不再请求这个集群：
  - 找不到端点：`errReconcileNoEndpoint`；
  - running 列表失败（任何错误）；
  - 任意一次 finished 列表或 `/job-info` 遇到传输层失败（`isEngineTransportError`）。

  没处理到的作业各记一次同步失败（`recordRemainingSyncFailures`，跳过没有 `engine_job_id` 的）。已经处理完的作业保持不变；正在处理的 running 作业 `/job-info` 超时时，照旧先用列表里的状态更新它。
- **不止损**：finished 列表或 `/job-info` 返回非 2xx、响应解析失败——引擎还能回话，只是这个接口不可用，保持原来的 `continue`。
- **列表路径回退**：`listEngineJobs` 遇到连接已建立后的超时或中断（`engineTransportErrorIsAmbiguous`）直接返回，不再试 v1 兼容路径；连接被拒等拨号失败仍尝试下一个路径。
- **退避**：`finish` 收到错误时 `consecutiveAborts++`，推迟 `min(interval × 2^(n-1), max_backoff)`，即 10、20、40、80、120、120 秒；`firstFailureAt` 记录这一段连续失败的开始时间。成功一次立即复位，并带回失败了几轮、从何时开始（`Recovered`）。
- **刷新路径**：`refreshJobInstance`（`?fresh=1`、事件触发刷新、执行状态查询、`external_ref` 接管）在作业所在集群处于退避期时**不请求引擎**，直接返回库里的数据，也不另记同步失败。用户主动的写操作（提交、停止）不受退避影响。
- **终态指标补拉**：`startTerminalMetricsBackfill` 在后台运行，上一次没结束时不再启动；跳过退避中的集群；某个地址遇到传输层失败后，本次不再请求这个地址。
- **日志**：每个集群只在进入退避时打一条 warn（集群、原因、下一次时间），恢复时打一条 info（失败轮数、开始时间），不每轮都打。
- **告警信号**：`finishClusterReconcile` 在提前结束时发 D1 `Observe`（`Since=firstFailureAt`），在恢复或进程启动后首次成功时发 `Resolve`；退避期内跳过的轮次不发信号、不刷新首次失败时间。见[同步作业告警](./sync-alerts.md)。
- **HTTP 超时**：`NewSeaTunnelEngineClient` 在构造时读取 `engine_request_timeout_seconds`，运行中修改要重启生效；并发上限同理。

### 4. 校验与错误对应表
| 情况 | 本轮 | 下一轮 | 作业 |
|---|---|---|---|
| 找不到端点 | 结束 | 退避 | 全部记一次同步失败 |
| running 列表超时 / 500 | 结束 | 退避 | 全部记一次同步失败 |
| finished 列表超时 | 结束，不再请求剩下的列表和 job-info | 退避 | 全部记一次同步失败 |
| finished 某个状态 404 | 继续下一个状态 | 正常 | 不受影响 |
| `/job-info` 超时 | 结束 | 退避 | 当前作业按原逻辑处理，后面的各记一次同步失败 |
| `/job-info` 非 2xx | 继续下一个作业 | 正常 | 当前作业记一次同步失败 |
| 退避期内 `?fresh=1` | 不请求引擎 | — | 返回库里的值 |

### 5. Good / Base / Bad
- Good：集群 A 的 running 列表卡 15 秒；集群 B 的作业仍每 10 秒更新；A 进入退避，10、20、40 秒后重试，期间页面上 A 的作业显示"同步延迟"。
- Base：所有集群正常，每轮每个集群各跑一次完整对账，行为和串行时一致。
- Bad：在 tick 协程里同步调用 `reconcileClusterJobs`，一个慢集群让所有集群停止更新。

### 6. 必须有的测试
- 慢集群不拖慢其他集群；上一轮没结束的集群本轮跳过（`TestReconcileSlowClusterDoesNotBlockOtherClusters`）。
- 止损：job-info 超时后不再请求后面的作业、finished 列表超时后不再请求剩下的列表、非 2xx 不止损、没有端点返回 `errReconcileNoEndpoint`（断言调用次数和 `sync_fail_count`）。
- 退避序列 10→20→40→80→120→120、成功复位、直连键不退避。
- 退避期内对账跳过、刷新不调引擎；退避结束后重试并翻倍。
- `isEngineTransportError` 分类；`engine_request_timeout_seconds` 生效；列表超时不回退 v1、连接被拒仍回退。
- `go test -race ./internal/apps/sync/`。测试库用 `:memory:` 时必须 `SetMaxOpenConns(1)`，否则并发协程可能拿到另一个空库。

### 7. 错误与正确示例
#### 错误
```go
for clusterID, items := range grouped {
    s.reconcileClusterJobs(ctx, clusterID, items) // 串行：一个集群卡 400 秒，所有集群都不更新
}
```
#### 正确
```go
for clusterID, items := range grouped {
    s.reconcile.tryDispatch(ctx, clusterID, func(ctx context.Context) error {
        return s.reconcileClusterJobs(ctx, clusterID, items)
    }, func(err error) { s.finishClusterReconcile(ctx, clusterID, err) })
}
```

## 事件模式（自适应轮询）

> **默认关闭。** 事件平均晚到约 8 秒（t1 实测取消后 8.2 秒收到），不比 10 秒轮询快；开启只换来更少的引擎调用，代价是事件悄悄丢失时状态最长滞后一个完整对账间隔。通道健康只在提交后 45 秒内验证，长期不提交的集群发现不了通道失效。只有需要减轻引擎压力的大集群才配置正数开启。默认情况下状态事件只是额外触发一次 `RefreshJobByID`，轮询频率不变。

- 条件：集群事件通道为 `healthy`、`report_non_terminal=true`，并且集群版本 ≥3.0.0（见 [实时可观测性与事件上报](./engine-events-and-realtime.md)）。
- 满足时，在完整对账间隔内每轮只做 `reconcileClusterJobsLight`：**不拉 running/finished 列表**，只给 `running` 作业中"30 秒内被读过"（`GetJob`/`GetJobLogs`/`/realtime` 会打标记）或 `last_synced_at` 超过 30 秒的作业拉 `/job-info`。
- 状态变化靠事件触发 `RefreshJobByID`（防抖 1 秒）；非 running 的非终态作业最多等一个完整对账间隔，由完整对账兜底。
- 通道降级、未开启、2.3.x、或配置为 0 / 负数（默认）时，每轮都是完整对账，行为和原来一致。作业从引擎消失时置为 failed、终态补拉终态指标等语义**只在完整对账里发生**。
- 完整对账提前结束（止损）时不更新 `lastFull`，下一轮仍然做完整对账。
- `lastFull` 只存在内存里：STX 重启后第一轮必然是完整对账。CLI `--wait` 轮询 `GetJob` 会让作业保持高频拉取。

## CLI

- 读作业：`stx sync job get <id>`（走默认读路径）
- 强制刷新：`GET ...?fresh=1`（codegen 路由可带 query）
- **`--wait`（submit/preview/recover）**：轮询 `GET /api/v1/sync/jobs/{id}`（读库，禁止 `fresh=1`）；默认 **follow 日志**，按 `offset`/`limit_bytes` 调 `GET .../logs`（Agent 读文件，不打 SeaTunnel REST）。
- stderr NDJSON 事件：`job_wait`、`job_status`（含 `sync_state`/`sync_lag_ms`）、`log_chunk`、`log_notice`、`job_observation_lost`；stdout 仍为终态 JSON。可用 `--no-follow-logs` 关闭日志跟随。
- **`--observation-timeout`**（默认 **3m**）：非终态且 `sync_state=stale` 持续超过该窗口（以 `sync_lag_ms` 或本地 stale 起始时间为准）时，`--wait` 以 timeout 退出并发出 `job_observation_lost`；设为 `0` 关闭。
- **集群不可用**：对账器抬高 `sync_fail_count` 并将 `sync_state` 置为 `degraded`/`stale`，**不**擅自把 `status` 改为 failed；Dolphin 应结合本参数与 DS `--timeout` 判定失败。
- **提交结果未知**：`stx sync task submit` / `stx sync job recover` 收到 `submit_spec.submit_outcome=unknown` 时在 stderr 输出 `warning`（`code=outcome_unknown`），`--wait` 继续等到确认；`stx sync job cancel` 收到 `result_preview.cancel_outcome=unknown` 时同样输出 warning。截止前重复提交收到 409，按冲突退出码 6 退出。任务超时覆盖通过 `--definition-file` 写入 `submit_timeout_minutes` / `stop_timeout_minutes`。
- **作业从引擎消失**：集群 API 可达，但 running/finished 列表都没有该 `engine_job_id`，且 `/job-info` 仅返回空壳 `{jobId}`（常见于集群重启或历史过期）时，对账器将作业置为 `failed`，避免永久 `running` 僵尸。手动提交、内置 schedule、Dolphin `external_ref` 只要落了 `engine_job_id` 都走同一条路径。
- **日志换行**：Agent/REST/CLI 传输路径必须保留真实换行；`log_chunk` 同时带 `lines`，并额外发 `log_line` 便于调度器任务日志按行阅读。
- **指标入日志流**：对账写入 `result_preview.metrics` 后，每约 30s（及终态）追加可读指标块；`GET .../logs` 返回 `metrics_log`；CLI `--wait` 同步打印。
