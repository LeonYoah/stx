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
    ResolveReasonRuleChanged = "rule_changed"        // 告警中改了阈值/持续时间，按新参数未超标
)

// Signal 新增 CDCDelay *CDCDelayDetail：结构化明细（状态、当前/窗口最大延迟、阈值、持续时间、超标起点、
// 最后样本时间、无法确认原因与起点、是否停止提醒、恢复依据），monitoring 存进阶段的 last_detail。

// MaxMessageLength 从 2000 改为 500：存储和展示统一一个上限（企业微信/钉钉单条约 2048 字节）。

// TaskConfig 新增（GET/PUT /api/v1/sync/tasks/:id/alert 同步变化）：
CDCDelayThresholdSeconds int `json:"cdc_delay_threshold_seconds"` // 默认 60，范围 10~86400
CDCDelayDurationMinutes  int `json:"cdc_delay_duration_minutes"`  // 默认 5，范围 1~60
```

- `Normalize`：两个字段为 0 时补默认值。
- PUT 请求体里两个字段是 `*int`：**不填时保留已保存的值**（老客户端不认识这两个字段，保存一次不会把自定义的 300 / 10 冲回 60 / 5）；新建时不填、或明确填 0，才用默认值。
- `Validate`：两个字段超出范围时返回 `ErrInvalidConfig`（400）。不管有没有勾选 B4 都校验，因为补默认值后一定有值。

## 3. 存储

策略不改表。B4 写成一条策略条件：

```json
{"metric_key": "sync_cdc_delay_high", "operator": ">=", "threshold": "60", "window_minutes": 5}
```

- `taskConfigToPolicyRequest` 和 `syncJobPolicyToTaskConfig` 双向转换。没有 B4 条件时，返回默认的 60 和 5。
- `UpsertAlertPolicyRequest.Validate` 在 sync_job 分支里校验 B4 条件：threshold 必须是 10~86400 的整数，window_minutes 必须在 1~60。这样直接调策略中心 API 也绕不过校验。
- 已有策略不受影响：不会自动加上 B4。
- 阶段表 `monitoring_sync_alert_episodes` 新增 `last_detail`（text，JSON）。只做加法，AutoMigrate 自动加列，旧版本忽略。
  Refresh / Fire 时写入，模板和 Webhook 从它读结构化字段；"停止提醒"也从它判断。

## 4. 判定器（sync，新文件 `internal/apps/sync/cdc_delay_alert.go`）

### 4.1 样本接入
- `recordCDCDelay` 把**每一条**样本放进判定器本轮的样本列表（每作业每轮最多 256 条；锁顺序 events → cdcAlerts），样本时间记为 STX 收到的时间。
  - 多源作业有多个 reader 各自上报，引擎时间会交错、不递增，所以**不能**按"时间不前进就丢弃"处理。延迟摘要（徽标）用同一个计入结论，窗口和"最近值"按 STX 收到时间。
- 旧样本按**时间差** `offset = STX 收到时间 − 引擎 createdTime` 判断（评审第二轮 N5：之前用"作业见过的最新引擎时间"，一条快 10 分钟的样本会让之后所有样本都被当成旧样本；worker 之间时钟差 >2 分钟时，卡住的 reader 的样本会被丢掉）：
  - 基线 = 该作业最近 10 分钟内**计入**样本的最大时间差；10 分钟没有计入的样本，基线过期，下一批重建。
  - `offset > max(基线, 0) + 2 分钟` 的是重投的积压旧样本，不计。`IngestEngineEvents` 在作业没有基线时，先用一批里最新的引擎时间建立基线，同一批里更早的重投样本才能识别。
  - 被判为旧样本的情况连续 5 分钟以上（不是一次性的重投，而是某个 worker 时钟长期偏慢），改为计入并抬高基线。
  - 效果：单条快 10 分钟的样本时间差是 −10 分钟，不会抬高基线，后面的样本照常计入；时钟快 3 分钟的 worker（时间差 −3 分钟）不影响时钟正常的 reader；时钟慢 3 分钟的 worker 上卡住的 reader 最多晚 5 分钟计入。
  - 时间差超过 2 分钟（时钟不同步）的样本照常计入，并按作业每 10 分钟最多打一条 warning（"时钟不同步或积压重投"）。
  - 代价：STX 重启后第一批如果全是积压的旧样本（没有新样本可比），会被计入；超过 5 分钟的积压重投也会被计入（见 PR 已知后续）。
  - 一轮里收到过样本、但全部被判为旧样本、一条都没计入（评审第三轮 M1：例如告警中引擎时钟回拨 5 分钟）：这一轮按**无数据**处理，归为 unknown（`stale_samples`），**不按读取条数推断为 low**——延迟事件还在来，说明读取延迟仍 ≥ 1 秒。之前会误发一次"恢复"，5 分钟后旧样本被改为计入时再告警一次。说明文字：`最近收到的延迟数据引擎时间比 STX 收到时间早 2 分钟以上（引擎时钟回拨或积压重投），暂不计入，无法确认读取延迟是否恢复……请检查 SeaTunnel 节点与 STX 的时钟是否同步。`

### 4.2 每 30 秒一轮
- 在 `StartEngineEventRuntime` 里加一个定时器。只有同时注入了 `alertReporter` 和 `alertStore` 才运行。
- **候选作业**：运行中、非批模式（`jobIsBatch`，#79 已合并）、非数据预览，并且任务配置里启用了 B4。
  - 一次查询列出运行中的作业；
  - 同一轮内，每个集群的通道状态只查一次；
  - 任务配置按任务缓存 60 秒。

**本轮分类**（N = 阈值）：

| 条件 | 分类 |
|---|---|
| 通道未开启、待重启，或集群不支持 | 退出：阶段静默关闭（`channel_unavailable`），清掉状态 |
| 本轮有样本，**最大值** ≥ N | high |
| 本轮有样本，最大值 < N | low |
| 无样本，通道降级 | unknown |
| 无样本，最近 60 秒内 `SourceReceivedCount` 增长了 | low（延迟已低于 1 秒） |
| 无样本，读取条数没有增长 | unknown |

- 高低按本轮**最大值**判断：多源作业里两个源 5 秒、一个源卡在 900 秒，本轮按 900 秒算 high（之前按中位数 5 秒，会完全漏掉）。作业的当前延迟也取本轮最大值。**中位数只用作追赶判定的基线**（见下）。

**状态转移**（每个作业一份：`phase ∈ {none, firing, maybe, rearm}`、`aboveSince`、`lowStreak`、最近 M 分钟内各轮 high 的中位数与最大值、`catchUpSince`、`unknownSince`）：
- **high**：
  - `lowStreak` 清零；`aboveSince` 没值时设为本轮时间。
  - `phase=firing`：发 Refresh。
  - `phase ∈ {maybe, rearm}` 且还在计时或追赶：发 Refresh（已有阶段按当前值更新，不用旧值提醒）。
  - 满足 `now − aboveSince ≥ M` 并且**不在追赶**（或追赶已满 30 分钟），就发 Fire，`phase` 变为 firing。
- **low**：`lowStreak++`。和恢复一样，**连续两轮低才算数**：
  - 单独一轮低（`lowStreak=1`）**不重新计时**，`aboveSince`、窗口、追赶起点都保留。间歇性高延迟（例如 3 轮 200 秒 + 1 轮 30 秒循环）照样能累计到持续时间并告警。`phase≠none` 时发 Refresh（状态 `recovering`，带当前延迟和当前阈值），阶段内容和之后的提醒都是最新的；rearm 期间的低轮也一样。
  - `lowStreak ≥ 2`（至少 60 秒）：重新计时（清 `aboveSince`、窗口、追赶起点）。`phase ∈ {firing, maybe}` 时发 Resolve（recovered，带恢复依据：样本 / 读取条数），`phase` 变为 none；`phase=rearm` 时发 Resolve（`rule_changed`），静默关闭，不发恢复通知。
  - 根据读取条数推断的低 / 恢复没有当前延迟：明细 `current_delay_ms=0`、`delay_inferred=true`，webhook 的 `current_delay_ms` 输出 null（不再带最后一个旧样本，例如 192000）。
- **两轮 high 之间隔了两轮以上非 high**（评审第二轮 N1 / Codex P2，第三轮 H1 修正）：`phase≠firing` 时，`nonHighRounds ≥ 2`（low 和 unknown 都算）的下一轮 high：
  - 本轮最大延迟 `D ≥ N + (now − lastHighAt)`（`lastHighAt` = 上一轮 high 的判定时间）：**照常累计**。读取位置只会前进：现在读到的是 now − D 时刻的数据，间隔内任一时刻 t 的读取位置都不超过它，延迟 ≥ t − (now − D) ≥ D − (now − lastHighAt) ≥ N，即间隔内从未低于阈值。下游反压、sink flush 卡住时，引擎只在真正发出（延迟 ≥ 1 秒的）记录时才上报事件，中间会连续几轮没有事件；第二轮的做法会让延迟从 300 秒涨到 3900 秒的作业（隔 2～3 轮上报一次）1 小时都不告警（f57f2a57 会告警）。
  - 否则从本轮重新计时（清 `aboveSince`、窗口、追赶起点）。孤立尖峰仍不告警：例如阈值 60 秒、隔 10 分钟的 200 秒尖峰（200 < 60 + 600），H、L、无数据 ×N、H 或 H、无数据 ×N、H 都不会立刻告警，"已持续"也不会是几百分钟（Codex P2 仍满足）。
  - 只隔一轮非 high 的间歇性高延迟（3 轮 200 秒 + 1 轮 30 秒）照样累计。firing 时不受影响（阶段是连续的）。
  - 注意：持续时间很短（例如 1 分钟）时，恢复后 200 秒、隔 2 轮（90 秒）再来 200 秒，200 ≥ 60 + 90，会按"间隔内一直超标"累计并告警，这是符合定义的。
- **unknown**：
  - `aboveSince` 暂时不变，`lowStreak` 清零，`nonHighRounds` 加一（见上一条）。
  - `phase≠none` 时发 Refresh，消息注明无法确认（带起始日期+时区、可能原因、作业链接）。只在刚进入 unknown 时发，之后每 5 分钟发一次。
  - unknown 持续 ≥24 小时：Refresh 带 `reminders_paused=true`，monitoring 的 sweep 不再提醒，阶段保留。
- **追赶判定**（选定方案：**每轮中位数**）：窗口 `[now−M, now]` 内至少两轮 high；取最早一轮的中位数作基线 `base`，和最新一轮的中位数 `last` 比较，`last ≤ 0.9 × base` 就算正在追赶；**同时**要求最新一轮最大值 ≤ 最早一轮最大值 × 90%（评审第二轮 N2：多源作业里一个 reader 卡住越来越慢、其他 reader 在追赶时，混在一起的中位数在降，之前要到第 12 分钟才告警）。
  - 为什么不用第一轮最大值：一次大事务尖峰会把基线抬高，之后正常的高延迟都显得"在下降"，漏报。
  - 为什么不用"连续下降"：引擎 10 秒一批、样本抖动大，逐轮单调下降的要求太脆，正常追赶也会被打断。
  - 追赶期最长 30 分钟（`cdcDelayCatchUpCap`），从第一次判为追赶算起，到点照常 Fire。追赶最早在超标满 M 分钟时开始判定，所以**持续超标的作业最晚在首次超标后 M + 30 分钟告警**（再加一轮 30 秒的判定间隔）。
- **改参数**：阈值或持续时间变化时重新计时（清 `aboveSince`、窗口、追赶起点）；`phase ∈ {firing, maybe}` 转为 `rearm`。之后按新参数满足条件会再 Fire（合并到已有阶段），回落则 `rule_changed` 静默关闭。
- **重启**：STX 启动后第一次看到某个作业时，`phase=maybe`。
  - 如果持续高：计时期间发 Refresh；满足条件后按正常规则再发一次 Fire。monitoring 会合并到已有阶段，不会重复通知。
  - 如果连续两轮 low：补发一次 Resolve（recovered），关掉重启前留下的阶段；没有阶段时 monitoring 什么也不做。
  - Refresh 只作用于告警中、未人工关闭的阶段；没有阶段时忽略，不建阶段。

### 4.3 其他关闭点
- **作业结束**：`reportJobTransition` 里，非批作业进入终态时，发 Resolve（scope `cdc_delay`，原因 `job_ended`，带 JobID），并清掉判定器状态。monitoring 没有对应阶段时什么也不做。
- **配置变化**：去掉 B4、停用策略、删除策略时，`phase≠none` 的作业发 Resolve（`rule_removed`）。monitoring 在保存策略（去掉 B4 或停用）时也会直接关闭告警中的 B4 阶段（`closeSyncTaskEpisodesByRules`），删除策略时本来就会关闭，sync 这条 Resolve 重复也没有影响。sync 侧任务配置缓存在保存/删除时失效。

### 4.4 消息（中文，已脱敏，最长 500 字，统一用"读取延迟"）
- **high**：`CDC 读取延迟 3分12秒（近 5 分钟最大 3分40秒），阈值 1分，已持续 6 分钟`
- **unknown**：
  - `最近 1 分钟没有收到延迟数据，读取条数也没有增长，无法确认读取延迟是否恢复（自 2026-10-29 10:20:00 +08:00 (Asia/Shanghai) 起）；最后一次读取延迟 3分12秒（2026-10-29 10:19:30 +08:00 (Asia/Shanghai)）。可能原因：源库暂时没有写入；作业读取卡住；事件通道中断。作业：{console_url}/sync?job=…&task=…`
  - 通道降级时：`事件通道降级，最近没有收到延迟数据，无法确认读取延迟是否恢复……`
  - 样本全部被判为旧样本时（`stale_samples`）：`最近收到的延迟数据引擎时间比 STX 收到时间早 2 分钟以上（引擎时钟回拨或积压重投），暂不计入，无法确认读取延迟是否恢复……请检查 SeaTunnel 节点与 STX 的时钟是否同步。`
  - 停止提醒后追加：`无法确认已超过 24 小时，已停止提醒，告警保留，请人工确认后关闭。`
- **恢复**：样本：`读取延迟已连续两轮低于阈值 1分（最近一次 12秒）`；读取条数：`根据读取条数推断已恢复：……`
- **recovering**（告警中的单独一轮低）：`读取延迟已回落到 12秒，低于阈值 1分；再确认一轮后自动关闭`；读取条数推断时说明"根据读取条数推断"。
- 时间用 `timex.DisplayLocation()`，带日期和时区；没有配置控制台地址时不带链接。

### 4.5 Refresh 只在内容变化时发送
- 每个作业记下最近一次发出的 Fire / Refresh 的展示内容（状态、恢复依据、阈值、持续时间、停止提醒、是否推断、说明文字）。内容没变的 Refresh 不发。说明文字里已有格式化后的延迟和"已持续 N 分钟"，所以延迟稳定的作业大约每分钟才刷新一次，而不是每 30 秒一次。
- 内容没变也每 5 分钟重发一次（`cdcDelayRefreshResendInterval`）。发送记录在信号**被接收之后**才更新：`syncalert.AcceptingReporter.TryReportSyncAlert` 返回是否进了队列，被丢弃的 Refresh 下一轮重发（unknown 自己的 5 分钟节流在被丢弃时也清掉）。
- monitoring 侧再兜底：队列（容量 1024）里已有 512 条信号时直接丢弃普通 Refresh，剩下一半留给 Fire / Resolve（例如 B1），大量高延迟作业不会把它们挤掉。`Signal.StateChange` 的 Refresh 不受这个限制，只在缓冲区全满时才丢。评审第三轮 L1 之后**只有进入 unknown 和停止提醒**算 StateChange（`cdcDelayRefreshIsStateChange`）：这两种之后状态机不会每轮再发（unknown 每 5 分钟才刷新一次，停止提醒后不再提醒），被丢弃就要等很久。high ↔ recovering 的切换不算：延迟抖动的作业每轮都在切换（实测 20 分钟 41 条 Refresh 全部绕过上限），全部绕过会让上限失效；这些 Refresh 内容变了，被丢弃后下一轮就重发，recovering 之后的恢复是 Resolve，不受上限影响。没有选"每个作业每 5 分钟最多绕过一次"：要多记一份状态，且抖动时绕过的仍是意义不大的 high / recovering。
- 代价：只在亚秒级变化的 `current_delay_ms` 不会单独触发 Refresh，提醒里的 webhook 数值可能差不到 1 秒。

### 4.6 配置缓存
- 任务配置按任务缓存 60 秒，保存 / 删除时失效。失效时递增该任务的代数；读取期间代数变了（读的同时配置被改）就不缓存读到的结果，避免把改之前的配置缓存 60 秒。

## 5. monitoring 变更

- **规则目录**：加入 B4，级别 warning，名称"CDC 延迟过高"。告警中心的来源和标签沿用"作业告警"。
- **`handle`**：
  - `KindRefresh` 交给 `refreshTask`：找到这个任务的 B4 告警中阶段，用 `applySignalToEpisode` 更新后保存，不计次、不通知。
  - B4 的 `KindFire` 走现有的 `fireTask`（"不配不发"、合并、首次通知）。
- **`resolve`**：
  - 新增 `ResolveCDCDelay` 分支。
  - `signal.ResolveReason` 为空或 recovered 时，走 `syncAlertResolveRecovered`，按策略发恢复通知；其余原因原样写进 `resolve_reason`，只关闭、不通知。
- **告警中心摘要**：`buildSyncAlertInstanceSummary` 对已解决、并且原因是 `job_ended`、`channel_unavailable`、`rule_removed`、`rule_changed` 或 `policy_deleted` 的阶段，追加"（作业已结束，自动关闭）"之类的说明；停止提醒的阶段追加"无法确认已超过 24 小时，已停止提醒"。
  `source_ref` 新增 `resolve_reason`、`reminders_paused`，前端按它显示中英文的关闭原因。
- **冷却提醒**：沿用 sweep，跳过 `reminders_paused` 的阶段。提醒内容用阶段的 `LastMessage` 和 `last_detail`；Refresh 会让它保持最新。
  - 30 秒判定和 1 分钟巡检的定时器是对齐的。巡检前先处理队列里已经排着的信号（同一时刻的 Refresh / 静默关闭），提醒用的是最新内容。
  - 每个阶段按 ID 重读阶段和策略的**检查**和策略保存 / 删除（会关闭阶段）用同一把锁互斥；**网络投递不持锁**（评审第二轮 N4：之前 webhook 卡住时保存策略要等 9.7 秒）。投递前再核对一次策略版本（`syncAlertPolicyVersion`，每次保存 / 删除加一）和阶段状态：检查之后策略被保存 / 删除或阶段被关闭的，这次不发。停用告警的那一刻不会再发出旧提醒（例如"无法确认、阈值 60 秒"）；已经在网络上发出的提醒无法撤回。
  - SMTP：连上之后、读服务器问候语之前就设置连接截止时间（15 秒）。之前截止时间在 `smtp.NewClient` 之后才设置，只接受 TCP、不发问候语的服务器会让投递一直卡住。
- **模板**：
  - 标题：`CDC 延迟过高（3分12秒）`；提醒追加 ` [提醒] 已持续 X 分钟`；无法确认 `（无法确认是否恢复）`；读取条数推断的恢复 `（根据读取条数推断）`。
  - B4 不用"错误摘要"字段，改为：当前读取延迟、近 M 分钟最大读取延迟、告警条件、已持续、说明（仅无法确认）、恢复依据。
  - Webhook JSON 追加 `resolve_reason`、`reminder` 和 B4 扁平字段：`delay_state`（high / unknown / recovering / recovered）、`current_delay_ms`（推断时为 null）、`delay_inferred`、`window_max_delay_ms`、`threshold_seconds`、`duration_minutes`、`above_since`、`last_sample_at`、`unknown_reason`、`unknown_since`、`reminders_paused`、`recovery_basis`。
  - recovering：标题 `（已回落，待确认）`，字段"当前读取延迟""说明：已低于阈值，再确认一轮后自动关闭"。

## 6. API、CLI、前端

- **API**：路由不变，只是 TaskConfig 多了两个字段。重新生成 swagger；route baseline 只核对，不应该有变化。
- **CLI**：`stx sync task alert set` 新增 `--cdc-delay-threshold <秒>` 和 `--cdc-delay-duration <分钟>`，`--rules` 的帮助里加上 `sync_cdc_delay_high`。
  - 没有给这两个参数时，先 GET 当前配置并保留原值；没有配置时用默认值。
  - 超出范围是用法错误。
  - `get` 是生成的命令，输出自动带上新字段。
  - 同步更新 `registry_sync.go` 里的参数说明。
- **前端**：
  - 类型、`task-alert-utils.ts`：规则列表加上 B4。新配置默认**不勾选** B4，因为它有前置条件，需要用户主动开启。表单状态新增两个字段，带校验和默认值。
  - `TaskAlertForm.tsx`：勾选 B4 后才展开两个参数输入框（渐进披露），并显示提示："建议 ≥30 秒；数据库与 SeaTunnel 节点时间不同步会让延迟偏大"和"仅对 SeaTunnel 2.3.12 及以上、已开启事件通道的集群上的 CDC 实时作业生效；多源作业按最慢的源判断"。版本门槛见 §9。
  - 能拿到集群 ID 时，查询集群的事件通道状态；未开启或不支持时，显示琥珀色提示"该集群未开启事件通道，这条规则不会触发"。
  - 摘要里的规则名包含 B4。
  - 告警中心：作业告警显示自动关闭原因（`closeReason_*`）和"已停止提醒"。
  - i18n 中英文都补齐（`syncAlert` 命名空间）。

## 7. 兼容与回滚

- 只做加法：新规则键、新信号类型和新解除范围、两个可选字段、阶段表一个新列 `last_detail`。
- 旧版本会忽略 B4（已核实）：`syncJobPolicyRules` 只保留 `syncalert.IsTaskRule` 认识的规则，不认识的条件直接跳过，不会报错。旧版本在表单里保存一次，就会把 B4 条件去掉。
- 回滚：revert PR 即可。残留的 B4 阶段可以在告警中心手动关闭。

## 8. 已知限制（写进 spec 和界面提示）

- 发现不了完全卡住的作业（那时没有延迟事件），由将来的 B3 负责。
- 通道悄悄断开、数据却在走时，可能误判恢复。
- 数据库和 worker 的时钟差会直接算进延迟。
- 源库空闲时，恢复通知会推迟到下一次有数据写入。
- STX 重启后，持续时间和"无法确认"的起始时间从零开始计算（判定状态只在内存）。
- 多源作业按最慢的源判断（本轮最大值），但不区分是哪个 source 慢。

## 9. 版本门槛：SeaTunnel ≥ 2.3.12（后端、前端、文档一致）

- 依据（apache/seatunnel 源码，按 release tag / 分支核对）：
  - `READER_MESSAGE_DELAYED` 由 CDC 的 `IncrementalSourceRecordEmitter.reportMetrics` 发出（`MessageDelayedEvent` + `MessageDelayedEventLimiter`），2.3.5 引入（#6634 "Support listening for message delayed events in cdc source"，2024-04）；同一版本 #6419 引入 event-report-http 的 `JobEventHttpReportHandler`。2.3.4 没有这个 handler。
  - 2.3.12 到 `3.0.0-release` 之间，`MessageDelayedEvent`、`IncrementalSourceRecordEmitter`、`MessageDelayedEventLimiter` 没有任何提交；`JobEventHttpReportHandler` 只多了本地缓冲（不影响事件内容）。
  - 所以引擎侧 ≥2.3.5 就会上报；但 STX 只允许在 ≥2.3.12 的集群上开启事件通道（`SupportsJobStateEvents`，2.3.12 起才有 JOB_STATUS），B4 又依赖事件通道，实际生效的门槛就是 **2.3.12**。
- 代码：`seatunnel.SupportsCDCDelayEvents`（= 事件通道门槛）用于判定器的通道判断；前端的"不支持"提示来自后端的 `supported`，文案改为"SeaTunnel 2.3.12 及以上"；CLI 帮助同步。之前写的"仅 3.x"是错的。
