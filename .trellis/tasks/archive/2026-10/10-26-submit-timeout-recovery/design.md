# 设计：提交与停止结果未知的登记、确认和超时配置

## 1. 现状（代码位置）

- **提交入口** `submitTaskInstanceWithPayload`（`internal/apps/sync/service.go` 约 1580 行）。手动运行、定时、恢复都会走到这里。
  - 先调 `engineClient.Submit`，成功后才 `CreateJobInstance`。
  - 出错直接返回，只有 Dolphin 会调 `recordFailedDolphinSubmit`（`task_schedule.go:172`）。
- **jobId 由 STX 生成**：`nextJobID()` 生成后，作为 `jobId` 参数传给引擎（`engine_client.go` 的 `Submit` 和 `submitV1`），所以之后能按这个 ID 去查。
- **HTTP 客户端** `http.Client{Timeout: 15s}`（`engine_client.go:160`），提交、停止、列表、job-info 共用。
- **v1 回退**：`Submit` 遇到任何传输错误时，只要配置了 `LegacyURL`，就改用 `submitV1` 再提交一次。状态码为 404（no context）、502、503、504 时，也会经 `trySubmitLegacy` 回退。
- **请求 ctx**：handler 传入的是 `c.Request.Context()`，客户端断开时会被取消。`runWithExecution` 的后续步骤用的也是这个 ctx。
- **对账** `reconcileClusterJobs`（`job_reconciler.go` 约 259 行）：
  - 作业在 running 或 finished 列表里：拉 job-info 后写入。
  - 都不在：`UpdatedAt` 超过 15 秒才拉 job-info。返回空壳（`engineJobInfoLooksOrphaned`），且 `CreatedAt` 已超过 30 秒，就判 FAILED。
- **状态写入** `applyEngineJobInfoUpdate`（`job_engine_sync.go:60`）：
  - 以上一状态为条件执行 `UpdateJobStatus`，只写 status、result_preview、error_message、finished_at 和 sync 字段，**不写 submit_spec**。
  - job-info 返回空壳（状态为空）时保持原状态。
- **停止** `CancelJob`（`service.go` 约 1665 行）：
  - pending 或 running 先改为 `cancel_requested`，再调 `StopJob`。
  - **任何错误**都会调 `restoreJobAfterCancelFailure`，恢复原状态，并写"cancel request failed"。
  - 成功后改为 `cancelling`，等对账看到终态。
  - 没有时间上限。
- **定时去重**：`HasScheduledJobInstanceInWindow` 判断窗口内是否已有记录。
- **事件预期**：`registerEventExpectation`（`engine_events.go:733`）。
- **任务设置**：`task.Definition`，可观测性用的是同样的写法（`observability_inject.go:33`）。CLI 通过 `stx sync task create|update --definition-file` 写入（`internal/cmd/sync.go:351`、`:454`）。
- **前端**：工作台在有运行中作业时会拦下新的运行（`DataSyncStudio.tsx` 的 `hasActiveRun`），但这只是前端拦截。

## 2. 方案取舍

| 方案 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| A. 结果不明再登记（**采用**） | 引擎调用返回"结果未知"类错误后，才写待确认记录 | 改动集中；提交过程中不会有两方并发写同一行 | STX 进程在提交过程中崩溃时，仍可能留不下记录 |
| B. 先登记再提交 | 提交前就写 pending，结束后改为成功、失败或未知 | 不变量最强 | 提交期间对账可能已经改写这一行，需要额外的版本号；确定失败后还要删掉这条记录 |
| C. 完全异步 | 先返回，后台再提交 | 接口返回快 | 改变接口契约，配置错误不能再即时报错 |

## 3. 错误分类（`engine_client.go`）

```go
// ErrEngineCallOutcomeUnknown 表示写操作（提交/停止）可能已到达引擎但没有拿到回复。
// ErrEngineCallOutcomeUnknown means a write call (submit/stop) may have reached the engine but no reply was received.
var ErrEngineCallOutcomeUnknown = errors.New("sync: engine call outcome unknown")

// EngineCallUnknownError 记录结果未知时实际请求的端点，供对账按 jobId 查询。
// EngineCallUnknownError records the endpoint actually called so reconcile can look the job up by id.
type EngineCallUnknownError struct {
    Cause     error
    APIMode   string // "v2" | "v1"
    BaseURL   string // v2: endpoint.BaseURL；v1: endpoint.LegacyURL
    EngineURL string
}
func (e *EngineCallUnknownError) Error() string
func (e *EngineCallUnknownError) Unwrap() error
func (e *EngineCallUnknownError) Is(target error) bool // == ErrEngineCallOutcomeUnknown
```

`engineTransportErrorIsAmbiguous(err error) bool` 的判定规则：

| 错误 | 判定 |
|---|---|
| `net.Error.Timeout()`、`context.DeadlineExceeded`、`context.Canceled` | 未知 |
| `io.EOF`、`io.ErrUnexpectedEOF`、`syscall.ECONNRESET`、`syscall.EPIPE` | 未知。Go 不会自动重试非幂等的 POST，出现这些错误说明请求已经写出去了 |
| `syscall.ECONNREFUSED`、`*net.DNSError`、TLS 或 x509 错误、其他 | 确定失败。拿不准的按确定失败处理，与现在的行为一致 |

各接口的处理：
- **Submit（v2）**：
  - `Do` 出错时，未知就返回 `EngineCallUnknownError`，**不回退**；确定失败保持现有的 v1 回退。
  - 504 按未知处理，`shouldFallbackToLegacy` 里去掉 504。
  - `ReadAll` 出错按未知处理。
  - 2xx 但 JSON 解析失败时按成功处理，jobId 用请求里传的那个。
- **submitV1**：用同一套判定，未知时 `APIMode` 写 `"v1"`。
- **StopJob（v2 和 v1）**：用同一套判定，未知时返回 `EngineCallUnknownError`。stop 不带 jobName 和格式，也没有回退逻辑，所以只需要套用分类。

## 4. 生效超时的计算（新文件 `submit_outcome.go`）

```go
// effectiveSubmitTimeout 返回本次运行使用的提交超时：任务快照覆盖 > 全局配置 > 默认 5 分钟。
// effectiveSubmitTimeout returns the submit timeout for this run: task snapshot override > global config > 5 minutes.
func effectiveSubmitTimeout(task *Task) time.Duration
func effectiveStopTimeout(task *Task) time.Duration // 同上，作用于停止

const (
    defaultSubmitTimeout = 5 * time.Minute
    defaultStopTimeout   = 5 * time.Minute
    taskTimeoutMinMinutes = 1
    taskTimeoutMaxMinutes = 120
    submitTimeoutSettingKey = "submit_timeout_minutes"
    stopTimeoutSettingKey   = "stop_timeout_minutes"
)
```

- **全局配置**：`config.Config.Sync.SubmitTimeoutSeconds`、`StopTimeoutSeconds`，`<=0` 时用默认值。
- **任务覆盖**：`intValue(task.Definition, key)`，落在 1 到 120 之间才采用，否则忽略并回到全局值。
- **校验**：`validateTaskDefinition` 发现值不在范围内时返回 `ErrInvalidTaskDefinition`（沿用现有的错误类型），前端和 CLI 都会收到 400。
- **停止时任务的来源**：停止时没有任务快照，按 `instance.TaskID` 读当前任务。为了和提交时用的版本保持一致，提交时就把 `stop_timeout_seconds` 写进 `submit_spec`，停止时优先读这个值。这样停止超时也跟着运行时的版本走。

## 5. 提交：登记待确认作业

`submitTaskInstanceWithPayload` 在 Submit 出错时的分支：

```go
var unknownErr *EngineCallUnknownError
if errors.As(err, &unknownErr) && strings.TrimSpace(platformJobID) != "" {
    return s.recordUnknownSubmitOutcome(ctx, task, instance, endpoint, unknownErr)
}
```

`recordUnknownSubmitOutcome` 的处理：
- `Status=pending`，`EngineJobID=platformJobID`。
- 端点字段按 `unknownErr` 写入，写法与成功分支一致：
  - `engine_api_mode`、`engine_base_url`；
  - v2 时写 `engine_context_path`，v1 时写 `engine_legacy_base_url`。
- 写入 `submit_spec`：
  - `submit_outcome="unknown"`
  - `submit_outcome_since`：当前 UTC 时间
  - `submit_timeout_seconds`：取 `effectiveSubmitTimeout`
  - `submit_deadline=since+timeout`
  - `submit_error`：经 `audit.RedactText` 脱敏
  - `engine_url`
- 执行 `CreateJobInstance`；**不调用** `registerEventExpectation`；按 `logging-guidelines.md` 打 warning 日志。
- 返回 `instance, nil`。
- 成功分支也把 `stop_timeout_seconds` 写进 `submit_spec`（见 §4）。

辅助函数：
- `submitOutcomeIsUnknown(job) bool`
- `submitOutcomeDeadline(job) time.Time`：先读 `submit_deadline`。解析失败时用 `submit_outcome_since + submit_timeout_seconds`；再失败就用 `CreatedAt + 全局值`。
- `submitOutcomePending(job, now) bool`：`unknown && now < deadline`。

## 6. ctx 解绑

- 在 `runWithExecution` 里，`sync.task.submit`、`sync.job.recover` 和停止相关的 `sync.job.cancel` 先做 `ctx = context.WithoutCancel(ctx)`，去掉取消信号，保留 value。
- 不经过 `runWithExecution` 的路径，在 `submitTaskInstanceWithPayload` 和 `CancelJob` 内部也做一次解绑，覆盖范围是引擎调用和写库。
- `attachExternalRef` 里等待取消完成的循环仍然用原来的 ctx。
- 耗时上限：一次 HTTP 调用 15 秒；确定失败后回退 v1 时，最多再加 15 秒。

## 7. 对账确认提交结果

### 7.1 `applyEngineJobInfoUpdate` 开头

```go
if submitOutcomeIsUnknown(instance) {
    status := strings.TrimSpace(info.JobStatus)
    if status == "" {
        return instance, nil // 空壳：交给对账的孤儿分支判断
    }
    if isRecoverRun(instance) && isFinalEngineJobStatus(status) && submitOutcomePending(instance, now) {
        return instance, nil // 恢复提交：截止前不采信可能属于上一轮的终态
    }
    markSubmitOutcomeConfirmed(instance) // confirmed + submit_outcome_at
    // 本次 updates 额外带 "submit_spec"
}
```

- `isRecoverRun` 的判断：`RunType == RunTypeRecover`，或者 `submit_spec.start_with_savepoint == true`。
- 并发确认靠 `UpdateJobStatus` 的 CAS 保证：对账和事件刷新可能同时确认，但两边写入的内容相同，谁先谁后结果都一样。

### 7.2 对账的孤儿分支

```go
if engineJobInfoLooksOrphaned(detail) {
    if submitOutcomeIsUnknown(job) {
        if submitOutcomePending(job, time.Now()) {
            continue
        }
        // 先 mark expired（写 submit_spec），再以 FAILED 和提交超时专用文案调用 apply
        continue
    }
    // 现有逻辑
}
```

- 提交超时的专用文案：

  > 提交请求超时，引擎在 N 分钟内一直没有出现这个作业，提交可能已被拒绝或丢失；请查看引擎日志后重新提交 / submit timed out and the engine never registered this job within N minutes; check engine logs before resubmitting

  其中 N 取 `submit_timeout_seconds` 换算成的分钟数。
- finished 列表命中时，恢复提交仍走 §7.1 的规则，这一轮直接跳过。
- `refreshJobInstance`（事件刷新、`?fresh=1`）调用的也是 `applyEngineJobInfoUpdate`，但**只在对账里判断是否过期**，避免读接口带副作用。
- **给子任务"作业告警"的接入点**：过期分支 `onSubmitOutcomeExpired(job)`，以及 confirmed 分支。本任务只把函数位置留好，不实现告警。

## 8. 重复提交拦截

- 新错误 `ErrSubmitOutcomePending`，`getStatusCodeForError` 映射为 409。返回时包上带上下文的提示：

  > 上一次提交（作业 #123）结果还在确认中，最晚 15:04 出结论，请勿重复提交 / previous submit (job #123) is still being confirmed until 15:04; do not resubmit

- 仓储新增 `ListPendingJobInstancesByTaskID(ctx, taskID, limit)`，查询条件是 `task_id=? AND status=?`，兼容三种数据库。
- `guardSubmitOutcomePending(ctx, taskID)` 在三处调用：
  - `submitNewTask`：在 `getTaskForExecution` 之后、`nextJobID` 之前；
  - `RecoverJob`：取到 source 和 task 之后；
  - `submitScheduledTask`。

## 9. 停止

### 9.1 每次停止都记录截止时间

`CancelJob` 把状态 CAS 到 `cancel_requested` 时，同时写入 `result_preview`：
- `cancel_requested_at`：当前 UTC 时间；
- `cancel_timeout_seconds`：先读 `submit_spec.stop_timeout_seconds`，没有就用 `effectiveStopTimeout(当前任务)`；
- `cancel_deadline`。

本地执行模式也写这几项，但本地停止是同步的，基本用不上。

### 9.2 停止结果未知

```go
if err := s.engineClient.StopJob(ctx, endpoint, instance.EngineJobID, stopWithSavepoint); err != nil {
    if errors.Is(err, ErrEngineCallOutcomeUnknown) {
        // 不 restore：按成功路径进入 cancelling，额外写 cancel_outcome=unknown、cancel_error
    } else {
        s.restoreJobAfterCancelFailure(ctx, instance, previousStatus)
        return nil, err
    }
}
```

- 确认：对账看到终态，或者 `applyEngineJobInfoUpdate` 拿到 CANCELED、SAVEPOINT_DONE，就按现有映射写入终态，同时写 `cancel_outcome=confirmed`。不需要单独的确认分支。
- 引擎上报的状态是 RUNNING 时，现有逻辑会保持 `cancelling` 不变（`job_engine_sync.go:78`）。
- 过了截止时间：状态不变。作业响应里加计算字段 `cancel_overdue`，前端据此显示"停止超时"。这是子任务"作业告警"的接入点 `onCancelOverdue(job)`。由对账判断是否越过截止时间，每个作业只触发一次：触发后在 `result_preview` 写 `cancel_overdue_at`。

## 10. 配置与前端

- `internal/config/model.go` 的 `SyncConfig` 增加两个字段：
  - `SubmitTimeoutSeconds int \`mapstructure:"submit_timeout_seconds"\``
  - `StopTimeoutSeconds int \`mapstructure:"stop_timeout_seconds"\``
  
  注释中英双语。`config.example.yaml` 加上示例值 300，并写明说明。
- 前端：
  - `SettingsSidebarPanel.tsx` 新增"超时"分组，包含两个分钟输入框。占位提示是"默认 5 分钟（全局）"；全局值不同时，后端 bootstrap 接口会带上全局值。超出范围时就地报错。
  - 草稿保存时写入 `definition.submit_timeout_minutes` 和 `definition.stop_timeout_minutes`；留空就删除这两个键。
  - `sync-studio-utils.ts` 新增四个函数：
    - `isSubmitOutcomeUnknown`
    - `submitOutcomeDeadline`（读 `submit_deadline`）
    - `isCancelOutcomeUnknown`
    - `isCancelOverdue`
  - `DataSyncStudio.tsx`：提交和停止返回"结果未知"时，弹出警告提示。
  - 作业状态单元格增加两个徽标："提交确认中"（悬停显示截止时间）、"停止超时"。
  - i18n：`sync.submitOutcome.*`、`sync.cancelOutcome.*`、`sync.timeoutSettings.*`，中英文各一份。
- **全局默认值的来源**：
  - 任务详情响应里加一个不落库的字段 `timeout_defaults`（`gorm:"-"`），内容为 `{submit_timeout_seconds, stop_timeout_seconds}`，取当前全局生效值。
  - 前端占位提示用它显示"默认 N 分钟（全局）"。
  - **不新增路由**。`/api/v1/capabilities` 是给 CLI 用的能力清单，不往里面放运行配置。

## 11. CLI

- `syncJobWaitResult` 增加 `SubmitSpec map[string]any \`json:"submit_spec,omitempty"\``。
- submit 和 recover 遇到 `submit_outcome=unknown`，在 stderr 输出 warning；`--wait` 继续等待。
- cancel 命令遇到 `result_preview.cancel_outcome=unknown`，输出 warning。
- 409 走现有的冲突退出码。
- 超时字段通过 `--definition-file` 写入，补一个例子和一条单测。

## 12. 兼容与回滚

- **数据**：不加列。旧记录没有这些键，按"已确认"或"没有截止时间"处理。
- **API**：
  - 提交和恢复接口新增 409。
  - 提交、恢复和停止接口在结果未知时返回 200。
  - Swagger 补 `@Failure 409`。
- **行为变化**：
  - 超时不再报错，而是返回 pending 或 cancelling 的作业。
  - v2 超时或 504 时不再回退到 v1。
  - 截止时间内拦截重复提交。
  - 停止超时后不再恢复成运行中。
- **回滚**：
  - 回滚代码后，残留的 unknown 记录会被旧的孤儿逻辑在 30 秒后判为 FAILED，或者正常确认。
  - `cancelling` 中的作业照常等对账看到终态。

## 13. 测试要点

| 层 | 用例 | 断言 |
|---|---|---|
| engine_client | 超时（客户端超时设为 50ms） | `ErrEngineCallOutcomeUnknown`；legacy 收到的请求数为 0 |
| engine_client | 连接被拒绝，且配置了 legacy | 回退 v1 并成功 |
| engine_client | 504 / 503 / 2xx 但不是 JSON | 依次为未知 / 回退 / 成功 |
| engine_client | StopJob 超时、连接被拒绝 | 未知 / 确定失败 |
| engine_client | 分类函数，表驱动 | 覆盖各类错误 |
| service | 生效超时：覆盖值、超出范围、只有全局、都没有 | 120 以内的覆盖值生效；0 和 121 退回全局值；什么都没配时为 5 分钟 |
| service | 提交结果未知 | pending；字段齐全；`deadline = since + 生效超时`；没有登记事件预期 |
| service | 普通错误 | 不建记录（非 Dolphin） |
| service | ctx 已取消 | 记录写入成功 |
| service | 截止时间内再次 submit 或 recover | 409；过了截止时间放行；同一个 ref 走接管 |
| service | 停止结果未知 | 状态为 cancelling，`cancel_outcome=unknown`，没有执行 restore |
| service | 停止确定失败 | 执行 restore |
| reconciler | running 命中 | 状态为 running，outcome 为 confirmed |
| reconciler | 截止时间内返回空壳 / 超过截止时间返回空壳 | 状态保持 pending / 状态 failed 并标记 expired，使用专用文案 |
| reconciler | 恢复提交，finished 列表中有终态，在截止时间内 / 已超过 | 状态不变 / 落到终态 |
| reconciler | 处于 cancelling 且已过 `cancel_deadline` | 写入 `cancel_overdue_at`，只触发一次 |
| handler | `ErrSubmitOutcomePending` | 409 |
| CLI | 返回体带 unknown | warning |
| 前端 | 4 个 util 函数、超时输入框的校验 | 单测 |
