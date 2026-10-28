# CDC 延迟告警（B4）技术设计

## 1. 边界：谁判定、谁投递

- **判定放在 sync**。B4 的输入全在 sync 里：
  - 延迟样本：事件接收端；
  - 读取条数：对账写入的 `result_preview.metrics`；
  - 通道状态；
  - 作业状态。

  阈值和持续时间存在 monitoring 的任务策略里，sync 通过已有的 `syncalert.TaskPolicyStore.GetTaskAlertConfig` 读取（按任务缓存 60 秒）。
- **monitoring 只管告警阶段和投递**，和 A1/A2/B1/C1 一样：
  - Fire 建阶段并通知；
  - 新增 Refresh，只更新展示内容；
  - Resolve 按原因决定发不发恢复通知。
- **没有选 D1 的做法**（D1 是 sync 发 Observe，由 monitoring 按窗口判定）。如果 B4 也这样，monitoring 就得拿到读取条数、通道状态和"安静"心跳，判定所需的状态会散在两个模块里。
- 两边仍然只依赖 `internal/pkg/syncalert`，不新增互相调用。

## 2. 契约变更（`internal/pkg/syncalert`，只做加法）

```go
const RuleCDCDelayHigh = "sync_cdc_delay_high" // B4，作业级，排在 TaskRules 最后

// KindRefresh：只更新告警中阶段的展示字段（消息、最近作业、最近发生时间），不计次、不通知；没有告警中的阶段时忽略。
const KindRefresh SignalKind = "refresh"

// ResolveCDCDelay：关闭任务的 B4 阶段；JobID > 0 时只关闭 LastJobID 相同的阶段。
const ResolveCDCDelay ResolveScope = "cdc_delay"

// Signal 新增 ResolveReason：空或 recovered 表示恢复，按策略发恢复通知；其余原因只关闭、不通知。
const (
    ResolveReasonRecovered   = "recovered"
    ResolveReasonJobEnded    = "job_ended"           // 作业停止、失败、完成
    ResolveReasonChannelOff  = "channel_unavailable" // 通道关闭、待重启，或集群不支持
    ResolveReasonRuleRemoved = "rule_removed"        // 任务告警配置去掉了 B4，或被停用
)

// TaskConfig 新增（GET/PUT /api/v1/sync/tasks/:id/alert 同步变化）：
CDCDelayThresholdSeconds int `json:"cdc_delay_threshold_seconds"` // 默认 60，范围 10~86400
CDCDelayDurationMinutes  int `json:"cdc_delay_duration_minutes"`  // 默认 5，范围 1~60
```

- `Normalize`：两个字段为 0 时补默认值。这样旧客户端不带这两个字段时，PUT 也能通过。
- `Validate`：两个字段超出范围时返回 `ErrInvalidConfig`（400）。不管有没有勾选 B4 都校验，因为补默认值后一定有值。

## 3. 存储

不改表。B4 写成一条策略条件：

```json
{"metric_key": "sync_cdc_delay_high", "operator": ">=", "threshold": "60", "window_minutes": 5}
```

- `taskConfigToPolicyRequest` 和 `syncJobPolicyToTaskConfig` 双向转换。没有 B4 条件时，返回默认的 60 和 5。
- `UpsertAlertPolicyRequest.Validate` 在 sync_job 分支里校验 B4 条件：threshold 必须是 10~86400 的整数，window_minutes 必须在 1~60。这样直接调策略中心 API 也绕不过校验。
- 已有策略不受影响：不会自动加上 B4。

## 4. 判定器（sync，新文件 `internal/apps/sync/cdc_delay_alert.go`）

### 4.1 样本接入
- `recordCDCDelay` 通过单调时间检查后，把样本累计进判定器：本轮最大值、最小值、条数、最后一条的值和时间。
- 样本时间早于"现在 − 2 分钟"的不计入 B4。STX 停机后，引擎会重投积压的旧批次，这些旧值不代表当前延迟。延迟徽标和时间线照旧使用它们。

### 4.2 每 30 秒一轮
- 在 `StartEngineEventRuntime` 里加一个定时器。只有同时注入了 `alertReporter` 和 `alertStore` 才运行。
- **候选作业**：运行中、非批模式、非数据预览，并且任务配置里启用了 B4。
  - 一次查询列出运行中的作业；
  - 同一轮内，每个集群的通道状态只查一次；
  - 任务配置按任务缓存 60 秒。

**本轮分类**（N = 阈值）：

| 条件 | 分类 |
|---|---|
| 通道未开启、待重启，或集群不支持 | 退出：阶段静默关闭（`channel_unavailable`），清掉状态 |
| 本轮有样本，最大值 ≥ N | high |
| 本轮有样本，最大值 < N | low |
| 无样本，通道降级 | unknown |
| 无样本，最近 60 秒内 `SourceReceivedCount` 增长了 | low（延迟已低于 1 秒） |
| 无样本，读取条数没有增长 | unknown |

**状态转移**（每个作业一份：`phase ∈ {none, firing, maybe}`、`aboveSince`、`lowStreak`、最近 M 分钟内各轮的 high 最大值）：
- **high**：
  - `lowStreak` 清零；`aboveSince` 没值时设为本轮时间。
  - `phase=firing`：发 Refresh。
  - 其他情况：满足 `now − aboveSince ≥ M` 并且**不在追赶**，就发 Fire，`phase` 变为 firing。
- **low**：
  - `aboveSince` 清空，`lowStreak++`。
  - `lowStreak ≥ 2`（至少 60 秒）并且 `phase ∈ {firing, maybe}` 时，发 Resolve（recovered），`phase` 变为 none。
- **unknown**：
  - `aboveSince` 不变，`lowStreak` 清零。
  - `phase=firing` 时发 Refresh，消息注明无法确认。只在刚进入 unknown 时发，之后每 5 分钟发一次。
- **追赶判定**：取窗口 `[now−M, now]` 内最早一个 high 轮的最大值 `first`，和本轮最大值 `last`。`last ≤ 0.9 × first` 就算正在追赶。
- **重启**：STX 启动后第一次看到某个作业时，`phase=maybe`。
  - 如果持续高：按正常规则再发一次 Fire。monitoring 会合并到已有阶段，不会重复通知。
  - 如果连续两轮 low：补发一次 Resolve（recovered），关掉重启前留下的阶段；没有阶段时 monitoring 什么也不做。

### 4.3 其他关闭点
- **作业结束**：`reportJobTransition` 里，非批作业进入终态时，发 Resolve（scope `cdc_delay`，原因 `job_ended`，带 JobID），并清掉判定器状态。monitoring 没有对应阶段时什么也不做。
- **配置变化**：去掉 B4、停用策略、删除策略时，`phase ∈ {firing, maybe}` 的作业发 Resolve（`rule_removed`）。删除策略时 monitoring 本来就会关闭阶段，这条 Resolve 重复也没有影响。

### 4.4 消息（中文，已脱敏，最长 2000 字）
- **high**：`CDC 延迟 3 分 12 秒（近 5 分钟最大 3 分 40 秒），阈值 60 秒，已持续 6 分钟`
- **unknown**：
  - `最近 1 分钟没有收到延迟数据，读取条数也没有增长，无法确认是否恢复；最后一次延迟 3 分 12 秒（10:21:30）`
  - 通道降级时：`事件通道降级，无法确认是否恢复；……`

## 5. monitoring 变更

- **规则目录**：加入 B4，级别 warning，名称"CDC 延迟过高"。告警中心的来源和标签沿用"作业告警"。
- **`handle`**：
  - `KindRefresh` 交给 `refreshTask`：找到这个任务的 B4 告警中阶段，用 `applySignalToEpisode` 更新后保存，不计次、不通知。
  - B4 的 `KindFire` 走现有的 `fireTask`（"不配不发"、合并、首次通知）。
- **`resolve`**：
  - 新增 `ResolveCDCDelay` 分支。
  - `signal.ResolveReason` 为空或 recovered 时，走 `syncAlertResolveRecovered`，按策略发恢复通知；其余原因原样写进 `resolve_reason`，只关闭、不通知。
- **告警中心摘要**：`buildSyncAlertInstanceSummary` 对已解决、并且原因是 `job_ended`、`channel_unavailable` 或 `rule_removed` 的阶段，追加"（作业已结束，自动关闭）"之类的说明。
- **冷却提醒**：沿用 sweep，不改。提醒内容用阶段的 `LastMessage`；Refresh 会让它保持最新，"无法确认"的说明也随之带上。

## 6. API、CLI、前端

- **API**：路由不变，只是 TaskConfig 多了两个字段。重新生成 swagger；route baseline 只核对，不应该有变化。
- **CLI**：`stx sync task alert set` 新增 `--cdc-delay-threshold <秒>` 和 `--cdc-delay-duration <分钟>`，`--rules` 的帮助里加上 `sync_cdc_delay_high`。
  - 没有给这两个参数时，先 GET 当前配置并保留原值；没有配置时用默认值。
  - 超出范围是用法错误。
  - `get` 是生成的命令，输出自动带上新字段。
  - 同步更新 `registry_sync.go` 里的参数说明。
- **前端**：
  - 类型、`task-alert-utils.ts`：规则列表加上 B4。新配置默认**不勾选** B4，因为它有前置条件，需要用户主动开启。表单状态新增两个字段，带校验和默认值。
  - `TaskAlertForm.tsx`：勾选 B4 后才展开两个参数输入框（渐进披露），并显示提示："仅对 3.x 集群、已开启事件通道的 CDC 实时作业生效；阈值需大于数据库与 SeaTunnel 节点的时钟差"。
  - 能拿到集群 ID 时，查询集群的事件通道状态；未开启或不支持时，显示琥珀色提示"该集群未开启事件通道，这条规则不会触发"。
  - 摘要里的规则名包含 B4。
  - i18n 中英文都补齐（`syncAlert` 命名空间，以及告警中心的规则名）。

## 7. 兼容与回滚

- 只做加法：新规则键、新信号类型和新解除范围、两个可选字段。不改表。
- 旧版本会忽略 B4（已核实）：`syncJobPolicyRules` 只保留 `syncalert.IsTaskRule` 认识的规则，不认识的条件直接跳过，不会报错。旧版本在表单里保存一次，就会把 B4 条件去掉。
- 回滚：revert PR 即可。残留的 B4 阶段可以在告警中心手动关闭。

## 8. 已知限制（写进 spec 和界面提示）

- 发现不了完全卡住的作业（那时没有延迟事件），由将来的 B3 负责。
- 通道悄悄断开、数据却在走时，可能误判恢复。
- 数据库和 worker 的时钟差会直接算进延迟。
- 源库空闲时，恢复通知会推迟到下一次有数据写入。
- STX 重启后，持续时间从零开始计算。
