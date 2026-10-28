# 设计：引擎查询韧性

## 1. 现状
- `StartJobReconciler`（`job_reconciler.go:41`）用 `time.Ticker` 按间隔调 `reconcileActiveJobs`。上一轮没跑完时，到期的 tick 会被丢掉。
- `reconcileActiveJobs` 先按集群分组，再对每个集群**串行**执行 `reconcileClusterJobsLight` 或 `reconcileClusterJobs`，最后执行 `backfillTerminalJobFinalMetrics`。
- `reconcileClusterJobs` 的失败处理：
  - running 列表失败：所有作业 `recordJobSyncFailure`，然后 return。
  - finished 列表失败：`continue`，接着请求下一个状态。
  - job-info 失败：记录这一个作业的失败，继续处理下一个作业。
- 事件刷新：`engine_events.go:852` 用并发上限 4 的 limiter 调 `RefreshJobByID`。`?fresh=1` 走的也是 `RefreshJobByID`。最终都会进入 `refreshJobInstance`，由它调 `GetJobInfo`。
- 引擎客户端：`NewSeaTunnelEngineClient()` 写死 `Timeout: 15s`。

## 2. 集群调度器（新文件 `job_reconciler_scheduler.go`）

```go
// clusterReconcileState 记录单个集群的对账进度与退避。
// clusterReconcileState tracks one cluster's reconcile progress and backoff.
type clusterReconcileState struct {
    inFlight            bool
    consecutiveAborts   int
    nextAttemptAt       time.Time
    firstFailureAt      time.Time // 给 D1 Observe 用；成功时清零
}

type clusterReconcileScheduler struct {
    mu       sync.Mutex
    states   map[uint]*clusterReconcileState
    sem      chan struct{} // 容量 = cluster_concurrency
    now      func() time.Time // 测试可注入
    wg       sync.WaitGroup  // 只给测试 Wait 用
}
```

- `reconcileActiveJobs` 分组以后，对每个集群调用 `scheduler.tryDispatch(clusterID, fn)`。
  - 以下两种情况直接跳过这个集群：`inFlight` 为真，或者 `now < nextAttemptAt`。
  - 否则把 `inFlight` 置为真，启动一个 goroutine：先获取信号量，执行 fn，然后更新状态，最后释放信号量。
  - **`reconcileActiveJobs` 不等这些 goroutine 结束**，所以慢集群不会占着整个 tick。
- goroutine 里用的是 reconciler 自己的 ctx，服务退出时一起取消。
- `backfillTerminalJobFinalMetrics` 仍然在 tick 末尾执行一次，不做并行化：它每次最多 100 行，而且只处理已经结束的作业。
- 集群 ID 为 0 的直连兜底（`endpointFromSubmitSpec`）沿用现有逻辑，单独作为一个调度键，键值取 0 加上端点，或者直接串行执行。实现时选一种，并写进测试。

## 3. 一轮内止损

`reconcileClusterJobs` 改为返回 `clusterReconcileOutcome`：

```go
type clusterReconcileOutcome int
const (
    clusterReconcileOK clusterReconcileOutcome = iota
    clusterReconcileAborted   // 传输层失败（超时/连接错误）或无端点
)
```

- 判定函数 `isEngineTransportError(err)`：`net.Error`、`*url.Error`，以及 `context.DeadlineExceeded`、`ECONNREFUSED` 等。HTTP 状态错误（`status=…`）**不算**传输错误。分类函数和子任务 1 的 `engineTransportErrorIsAmbiguous` 共用一套底层判断。
- 遇到传输错误时，剩下还没处理的作业统一 `recordJobSyncFailure`，然后返回 `Aborted`。已经处理完的作业保持不变。
- finished 列表的循环：遇到传输错误就止损；遇到 HTTP 状态错误，保持现在的 `continue`。
- `reconcileClusterJobsLight` 同样处理。

## 4. 退避

```go
func (s *clusterReconcileScheduler) finish(clusterID uint, outcome clusterReconcileOutcome, interval, maxBackoff time.Duration)
```

- `OK`：`consecutiveAborts` 清零，`nextAttemptAt` 清零，`firstFailureAt` 清零。如果之前处于失败状态，发出 D1 的 Resolve 信号，前提是 reporter 已经注入。
- `Aborted`：
  - `consecutiveAborts++`
  - `delay = min(interval << (consecutiveAborts-1), maxBackoff)`
  - `nextAttemptAt = now + delay`
  - `firstFailureAt` 为零时，设成 now
  - 发出 D1 的 Observe 信号，`Since=firstFailureAt`
  - 退避从开始变成持续时打一条日志；成功恢复时再打一条
- `clusterInBackoff(clusterID) bool`：`now < nextAttemptAt`。

### 刷新路径接入
`refreshJobInstance` 在调 `GetJobInfo` 之前，先取作业所在的集群：优先 `submit_spec.cluster_id`，其次 `clusterIDForJob`。如果这个集群正在退避，**直接返回库里的 instance**，不记同步失败（对账那边已经记过了）。

## 5. 配置

在 `SyncConfig` 中新增三项，注释中英双语，并同步到 `config.example.yaml`：

| 键 | 默认 | 说明 |
|---|---|---|
| `job_reconciler_cluster_concurrency` | 4 | 同时对账的集群数上限，`<=0` 时使用默认值 |
| `job_reconciler_max_backoff_seconds` | 120 | 退避上限，`<=0` 时使用默认值 |
| `engine_request_timeout_seconds` | 15 | 引擎 HTTP 超时，`<=0` 时使用默认值 |

`NewSeaTunnelEngineClient()` 在构造时读取 `engine_request_timeout_seconds`。如果 `config.Config` 为 nil（单测场景），使用 15 秒。

## 6. 与其他子任务的接口
- **D1 信号**：如果"作业告警"先合并，它在 `reconcileClusterJobs` 里发出的 Observe/Resolve 要**迁到 scheduler.finish**，集中发出，原位置删除；如果本任务先合并，"作业告警"直接接 `finish`。两种顺序都要在对应子任务的 implement.md 里写清楚。
- **子任务 1**：写请求（submit/stop）走同一个客户端，同样受 `engine_request_timeout_seconds` 控制；结果未知的判定逻辑不变。

## 7. 测试
- fake 引擎客户端：按集群注入延迟和错误（阻塞到 ctx 结束，或者返回 `net.Error` 超时）。
- 可注入时钟，加上 `scheduler.wg.Wait()` 测试钩子。
- 用例清单：
  - 慢集群不拖慢其他集群；
  - in-flight 时跳过；
  - 止损后不再发后续请求（检查调用计数）；
  - 退避序列，以及成功后复位；
  - 刷新路径在退避期间不调引擎；
  - 配置生效；
  - `-race`。
