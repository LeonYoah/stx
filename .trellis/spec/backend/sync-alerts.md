# 同步作业告警

> 适用于 `internal/pkg/syncalert`（契约）、`internal/apps/sync/alert_signals.go`、`task_alert.go`（信号与任务告警接口）、`internal/apps/monitoring/sync_alert*.go`（判定、阶段、投递）。超时与写操作结果未知见[作业状态对账器](./job-status-reconciler.md)。

## 1. 适用范围 / 触发条件

- 新增或修改作业告警规则、信号产生位置、告警阶段的生命周期、通知内容、任务告警配置接口时。
- 修改 `applyEngineJobInfoUpdate`、`submitTaskInstanceWithPayload`、`submitLocalTaskInstance`、`expireUnknownSubmit`、`markCancelOverdue`、集群对账结束（`finishClusterReconcile`）或事件通道状态写入时，必须确认信号仍然在同样的时机发出。
- 原则：
  - **作业级告警只在用户配置后生效**：任务没有 `sync_job` 策略、策略停用或没勾选这条规则时，信号直接丢弃，不建阶段，不发通知。
  - **集群级规则默认开启**：作为每个集群的默认规则（与"节点离线"一样）；没有策略时只在告警中心显示，不发通知。
  - **只用现有信号**，不新增引擎调用；数据预览（`run_type=preview`）不产生信号。
  - **sync 不 import monitoring**：两边只依赖 `internal/pkg/syncalert`，在 `router.go` 里注入；没注入时 sync 照常工作。

## 2. 签名

```go
// internal/pkg/syncalert
type Reporter interface{ ReportSyncAlert(signal Signal) }          // 实现必须非阻塞
type TaskPolicyStore interface {
    GetTaskAlertConfig(ctx context.Context, taskID uint) (*TaskConfig, error)
    SaveTaskAlertConfig(ctx context.Context, task TaskRef, cfg *TaskConfig) (*TaskConfig, error)
    DeleteTaskAlertConfig(ctx context.Context, taskID uint) error
}
var ErrInvalidConfig, ErrConflict error                               // → 400 / 409

// internal/apps/monitoring
func (s *Service) NewSyncAlertHub() *SyncAlertHub                     // 实现 Reporter
func (h *SyncAlertHub) Start(ctx context.Context)                     // 单 worker：信号 + 每分钟提醒巡检
func (s *Service) SetSyncTaskNameResolver(r SyncTaskNameResolver)     // 策略列表显示任务名

// internal/apps/sync
func (s *Service) SetAlertReporter(reporter syncalert.Reporter)
func (s *Service) SetTaskAlertPolicyStore(store syncalert.TaskPolicyStore)
```

| 方法 | 路由 | 权限 | 风险 | CLI |
|---|---|---|---|---|
| GET | `/api/v1/sync/tasks/:id/alert` | 任务可见 | R0 | `stx sync task alert get <id>`（登记表生成） |
| PUT | `/api/v1/sync/tasks/:id/alert` | 任务可编辑 | R1 | `stx sync task alert set <id> --rules … --channel … [--receiver …] [--notify-owner] [--cooldown N] [--no-recovery] [--disable] --confirm` |
| DELETE | `/api/v1/sync/tasks/:id/alert` | 任务可编辑 | R1 | 不提供（`cli-guidelines.md` §11）；停用用 `set --disable` |
| GET | `/api/v1/monitoring/alert-policies?policy_type=sync_job&task_id=` | 登录 | R0 | `stx monitoring alert-policy list --policy_type sync_job --task_id 1` |

数据库：
- 新表 `monitoring_sync_alert_episodes`（`SyncAlertEpisode`），普通复合索引 `idx_sync_alert_episode_key(scope, cluster_id, task_id, rule_key)`，不用部分唯一索引。
- `monitoring_alert_policies` 新增 `task_id`（`not null default 0`）、`notify_task_owner`（`not null default false`）。

## 3. 契约

### 规则目录

| 规则键 | 场景 | 范围 | 级别 | 触发 | 恢复（Resolve 范围） |
|---|---|---|---|---|---|
| `sync_submit_failed` | A1 | 任务 | critical | 找不到引擎端点、引擎明确拒绝（含本地执行失败） | `task_run_healthy` |
| `sync_submit_timeout` | A2 | 任务 | critical | 提交结果未知过了截止时间被判失败 | `task_run_healthy` |
| `sync_job_failed` | B1 | 任务 | critical | 状态从非 failed 变为 failed；`submit_outcome=expired` 的不报 | `task_run_healthy` |
| `sync_stop_timeout` | C1 | 任务 | warning | 首次越过 `cancel_deadline`（`markCancelOverdue` 写入成功后） | `job_terminal`（同一作业） |
| `sync_cdc_delay_high` | B4 | 任务 | warning | sync 判定器：本轮最大读取延迟 ≥ 阈值持续 M 分钟且不在追赶（中位数只做追赶基线）（见 §3 B4） | `cdc_delay`（带 `ResolveReason`，见 §3 B4） |
| `sync_status_unavailable` | D1 | 集群 | 规则配置（默认 warning） | 集群对账提前结束时发 `Observe`，持续时间 ≥ 规则 `WindowSeconds`（默认 300）才成立 | 恢复或进程启动后首次成功：`cluster_rule` |
| `sync_event_channel_degraded` | D2 | 集群 | 规则配置（默认 warning） | 通道状态从非降级变为 `degraded` | 离开降级（healthy / disabled）：`cluster_rule` |

`task_run_healthy`：批作业状态变为 `success`；实时作业状态变为 `running`，或实时作业提交成功（提交时就记为 running）。模式由 `jobRunMode` 决定：优先取 `submit_spec.submitted_content` 里引擎实际使用的 `env.job.mode`，其次 `job.Mode`、`submit_spec.mode`，仍为空按实时作业处理（提交成功时再参考任务模式）。任务元数据可能与内容不一致（线上作业 #26：`submit_spec.mode=batch`、内容 `STREAMING`），以内容为准，否则实时作业的 B1 只会在“成功”时恢复。

### 信号
- 作业级信号的快照字段：`TaskID`、`TaskName`、`TaskCreatorID`、`ClusterID`（`submit_spec.cluster_id`，没有时取任务）、`JobID`、`EngineJobID`、`TriggerSource`（`submit_spec.trigger_source`：manual / schedule / dolphin，Dolphin 触发见 [DolphinScheduler SHELL 接入](./dolphin-scheduler-shell.md)）、`Message`（`audit.RedactText` 脱敏后截断 `syncalert.MaxMessageLength` = **500** 字符；存储和展示统一这一个上限，企业微信 / 钉钉单条约 2048 字节）。
- `KindRefresh`：只更新告警中、未人工关闭阶段的展示字段（消息、最近作业、`last_detail`），不计次、不通知；没有阶段时忽略。
- `Signal.ResolveReason`：空或 `recovered` 按策略发恢复通知；`job_ended` / `channel_unavailable` / `rule_removed` / `rule_changed` 只关闭、不通知，原样写进 `resolve_reason`。
- `Signal.CDCDelay`（`*CDCDelayDetail`）：B4 的结构化明细，存进阶段的 `last_detail`（text JSON）。B4 阶段以恢复以外的原因关闭（`job_ended` / `channel_unavailable` / `rule_removed` / `rule_changed` / `policy_deleted` / `closed`）时，monitoring 把 `last_detail.state` 改成 `closed` 并写 `last_detail.resolve_reason`（人工关闭写 `manual`，阶段的 `resolve_reason` 仍是 `closed`；`closed` 不会出现在 webhook 的 `delay_state` 里），不留关闭前的 `high` / `recovering` / `unknown`；恢复关闭保留 sync 送来的 `recovered` 明细（t1 实测 B3）。
- D2 的通道状态（`healthy` / `degraded` 等）定义见[实时可观测性与事件上报](./engine-events-and-realtime.md)。
- 集群名由 hub 用 `clusterService.Get` 补齐，sync 不传。
- `OccurredAt` 为空时按接收时间。

### 告警阶段（SyncAlertEpisode）
- 键：`scope + cluster_id + task_id + rule_key`（集群级 `task_id=0`）。同一个键同一时间最多一个 `firing` 阶段：hub 只有一个 worker，串行"先查后写"。
- 第一次出问题：建阶段，按策略投递告警通知，策略 `match_count+1`。
- 再出问题：`occurrence_count+1`，更新最近的作业、触发来源、错误信息，**不再发通知**。
- 冷却提醒：每分钟巡检 firing 阶段，复用投递去重（`source_key + channel + event_type`），过了策略 `cooldown_minutes` 才重发；`cooldown_minutes=0` 不提醒。
- 恢复：`status=resolved`、`resolve_reason=recovered`，按策略 `send_recovery` 投递恢复（要求对应告警通知发送成功过）。
- 人工处理：`source_key=sync:episode:<id>`，确认、静默、关闭复用 `AlertState`。静默期间不发告警和提醒；关闭后再出问题时，旧阶段记 `resolve_reason=closed`，开新阶段并重新通知。
- 删除任务告警策略：任务的 firing 阶段记 `resolve_reason=policy_deleted`，不发通知。
- 自动关闭的阶段在告警中心摘要追加原因（"作业已结束，自动关闭"等），`source_ref.resolve_reason` / `reminders_paused` 供前端显示中英文。

### B4 CDC 延迟（`sync_cdc_delay_high`）
- **判定在 sync**（`internal/apps/sync/cdc_delay_alert.go`），monitoring 只管阶段和投递；两边只依赖 `syncalert`。
- 版本门槛：SeaTunnel **≥ 2.3.12**（`seatunnel.SupportsCDCDelayEvents`，与事件通道同一门槛；引擎 2.3.5 起就上报 `READER_MESSAGE_DELAYED`，但 STX 的事件通道要求 2.3.12）。前端提示、CLI 帮助、文档都写 2.3.12，不写"3.x"。
- 输入：事件通道 `READER_MESSAGE_DELAYED` 样本（`recordCDCDelay` 接入**每一条**，样本时间记 STX 收到时间）、对账写入的 `result_preview.metrics.SourceReceivedCount`、通道状态、作业状态。
  - 旧样本按**时间差**（STX 收到时间 − 引擎 createdTime）判断：超过 `max(基线, 0) + 2 分钟` 的是重投的积压，不计。基线 = 该作业最近 10 分钟内计入样本的最大时间差，10 分钟没有计入的样本就过期重建；作业没有基线时，`IngestEngineEvents` 先用整批里最新的引擎时间建立基线（同一批里更早的重投样本才能识别）。
  - 被判为旧样本的情况连续 5 分钟以上（不是一次性的重投，而是某个 worker 时钟长期偏慢），改为计入并抬高基线。所以一条时间不准的样本（例如快 10 分钟）不会让后面的样本一直被丢；worker 之间时钟差 >2 分钟时，卡住的 reader 也照常计入（时钟偏慢的节点最多晚 5 分钟）。
  - 时间差 >2 分钟（时钟不同步）的样本照常计入，按作业每 10 分钟最多一条 warning。
  - 多个 reader 交错上报、时间不递增的样本都计入。延迟摘要（最近值、1 分钟窗口最大值）用同一个计入结论，窗口和"最近值"按 STX 收到时间，不按引擎时间。
- 候选：运行中、`!jobIsBatch`、非数据预览、`TaskID>0`、任务配置启用了 B4（配置缓存 60 秒，保存 / 删除时失效；读取期间被改过的结果不缓存）。
- 每 30 秒一轮，按本轮样本**最大值**分类（中位数只用作追赶基线）：

| 条件 | 分类 / 动作 |
|---|---|
| 去掉 B4 或停用，且 phase≠none | Resolve `rule_removed` |
| 通道 disabled / pending_restart，或版本明确不支持（版本取不到不算） | Resolve `channel_unavailable`（首次看到的作业也发一次） |
| 有样本，最大值 ≥ N | high |
| 有样本，最大值 < N | low（依据：样本） |
| 收到过样本但全部被判为旧样本、一条都没计入（例如引擎时钟回拨） | unknown（`stale_samples`），**不**按读取条数推断为 low |
| 无样本，通道降级 | unknown（`channel_degraded`） |
| 无样本，60 秒内读取条数增长 | low（依据：读取条数） |
| 其他 | unknown（`no_progress`） |

- phase：`none` / `firing` / `maybe`（STX 启动后首次看到）/ `rearm`（告警中改了阈值或持续时间）。
  - high：firing → Refresh；maybe / rearm 计时或追赶期间 → Refresh；持续 ≥M 且不在追赶 → Fire。
  - phase≠firing 时，和上一轮 high 之间隔了**两轮以上非 high**（low 和 unknown 都算）的 high：本轮最大延迟 **≥ N + (now − lastHighAt)** 时照常累计，否则从本轮重新计时。读取位置只会前进，现在读到 now − D 时刻的数据，间隔内任一时刻的延迟都 ≥ D − (now − lastHighAt) ≥ N，即间隔内从未低于阈值；下游反压 / sink flush 卡住时引擎只在真正发出记录时上报事件，延迟一直在涨的稀疏上报也能告警。孤立尖峰（H、L、无数据 ×N、H 或 H、无数据 ×N、H，例如隔 10 分钟的 200 秒 < 60 秒 + 600 秒）仍然重新计时，不会告警，也不会出现"已持续几百分钟"。只隔一轮非 high 的间歇性高延迟照样累计。`lastHighAt` 是最近一轮 high 的判定时间。
  - low 第一轮：**不重新计时**（`aboveSince`、窗口、追赶起点保留）；phase≠none 时 Refresh（状态 `recovering`，当前延迟 + 当前阈值，rearm 期间同样刷新）。
  - low 连续两轮：重新计时；firing / maybe → Resolve `recovered`（带恢复依据）；rearm → Resolve `rule_changed`（**不发恢复**）。
  - 读取条数推断的 low / 恢复：明细 `current_delay_ms=0` + `delay_inferred=true`，webhook `current_delay_ms` 为 null。
  - unknown：phase≠none 时进入即 Refresh，之后每 5 分钟一次；持续 ≥24h 带 `reminders_paused`，sweep 不再提醒，阶段保留。
- 追赶：窗口 `[now−M, now]` 内至少两轮 high，最新一轮中位数 ≤ 最早一轮中位数 × 90%，**并且**最新一轮最大值 ≤ 最早一轮最大值 × 90%（多源作业里其他 reader 在追赶、一个 reader 卡住越来越慢时，混在一起的中位数在降，但不算追赶）；从第一次判为追赶起最多压 30 分钟。基线**不用第一轮最大值**（单次大事务尖峰会抬高基线导致漏报）。持续超标的作业**最晚在首次超标后 M + 30 分钟告警**（+ 一轮 30 秒判定间隔）。
- Refresh 只在展示内容（状态、依据、阈值、时长、停止提醒、是否推断、说明文字）变化时发送，内容不变也每 5 分钟重发一次。发送记录在信号**被接收之后**才更新（`syncalert.AcceptingReporter.TryReportSyncAlert` 返回是否进了队列），被丢弃的下一轮重发（unknown 的 5 分钟节流也会清掉）。
- hub 队列 ≥512 时丢弃普通 Refresh，留一半给 Fire / Resolve；`Signal.StateChange` 的 Refresh 不受这个限制，只在缓冲区全满时才丢。**只有进入 unknown 和停止提醒**算 StateChange（这两种之后状态机不会每轮再发，丢了要等很久）；high ↔ recovering 的切换不算：延迟抖动的作业每轮都在切换，全部绕过会让上限失效，而它们内容变了、被丢后下一轮就重发，recovering 之后的恢复是 Resolve，不受上限影响。
- 冷却提醒：巡检前先处理队列里已有的信号；按 ID 重读阶段和策略的**检查**与策略保存 / 删除互斥（`syncAlertEpisodeMu`），**网络投递不持锁**（渠道卡住不会拖住保存策略）；投递前再核对一次策略版本（`syncAlertPolicyVersion`，每次保存 / 删除加一）和阶段状态，检查之后策略被保存 / 删除或阶段被关闭的，这次不发。
- SMTP：连上之后、读服务器问候语之前就设置连接截止时间（15 秒），不发问候语的服务器不会让投递一直卡住。
- 作业进入终态：`reportJobTransition` → Resolve `job_ended`（带 JobID，只关同一作业的阶段）。
- 模板：标题 `CDC 延迟过高（3分12秒）`（延迟四舍五入到秒）；提醒追加 ` [提醒] 已持续 X 分钟`（含无法确认区间时注明「含无法确认 X 分钟」）；恢复标题带当时延迟如 `已恢复（20秒）`；B4 不展示「触发来源」；时间本地+UTC 合成一行；不用"错误摘要"，改为当前 / 窗口最大读取延迟、告警条件、已持续、说明、恢复依据；读取条数推断的恢复注明"根据读取条数推断"；webhook 追加 `current_delay_ms`（推断或 unknown 时 null）、`last_delay_ms`（unknown）、`delay_inferred`、`threshold_seconds` 等与 `resolve_reason`、`reminder`；recovering 标题 `（已回落，待确认）`。通用 Webhook 未配置 secret 时不带签名头（有 secret 才签）。
- 文案统一说"读取延迟"；"无法确认"带日期 + 时区（`timex.DisplayLocation`）、可能原因和作业链接。
- 已知限制：时钟偏慢超过 2 分钟的 worker 上卡住的 reader 最多晚 5 分钟计入；超过 5 分钟的积压重投会被计入；发现不了完全卡住（B3）；通道悄悄断开可能误判恢复；时钟差计入延迟；源库空闲时恢复推迟；重启后计时从零开始（状态只在内存）；多源作业按最慢的源判断，但不区分是哪个源；重启后第一批全是积压旧样本时会计入一轮。

### 投递
- 与来源无关的 `alertMessage`（来源类型、集群、告警名、额外收件人、按渠道拼消息体），本地进程事件和作业告警共用 `dispatchAlertPolicyChannels` / `dispatchAlertDelivery`。
- 标题：`[STX][告警|恢复][级别] 任务 <名称> · <规则名>`（集群级为"集群 <名称>"）。
- 正文：状态、规则、集群、任务、作业 `#id`、引擎作业 ID、触发来源、累计次数、首次 / 最近发生时间、恢复时间、错误摘要（500 字符）、策略、链接。
- webhook 带结构化 `alert`：`source_type=sync`、`source_key`、`status`、`scope`、`rule_key`、`task_id`、`job_id`、`occurrence_count`、`link` 等。
- 链接：`{app.console_url}/sync?task=<id>&job=<id>`，`console_url` 为空时不带链接；告警中心用相对链接 `/sync?task=&job=`。**不要用 `external_url`**：它是 Agent、Alertmanager 回调的后端地址（默认 17800），默认安装里前端在另一个端口（17880），拼出来的链接会 404。
- 邮件收件人：策略接收人 + `notify_task_owner` 时的任务创建人邮箱（用户停用或没有邮箱时忽略，不让告警失败），按小写去重。

### 任务告警配置（TaskConfig）
- `rules ⊆ {sync_submit_failed, sync_submit_timeout, sync_job_failed, sync_stop_timeout, sync_cdc_delay_high}`，至少一条；新配置前端默认不勾选 B4；`channel_ids` 至少一个、必须存在且启用；`cooldown_minutes` 0–1440；PUT 时 `enabled`、`send_recovery` 默认 true，`cooldown_minutes` 默认 60。
- `cdc_delay_threshold_seconds`（默认 60，10–86400）、`cdc_delay_duration_minutes`（默认 5，1–60）：PUT **不填时保留已保存的值**（老客户端不会把自定义值冲回默认），新建时不填或明确填 0 按默认，超范围 400；不论是否勾选 B4 都校验。B4 条件写成 `threshold=<秒> window_minutes=<分钟>`，通用策略接口同样校验（`ErrAlertPolicySyncJobInvalidCDCDelay`）。没勾选 B4 时参数存进一条 `metric_key=sync_cdc_delay_params` 的只保存参数的条件（不是规则：不出现在 `rules`，不能单独存在、最多一条、不能和 B4 条件并存、同样校验范围；读取先认 B4 条件），所以去掉 B4 再勾回仍是原来的阈值和持续时间（t1 实测 B1：此前去掉 B4 时参数随条件一起丢失，读回变成默认 5 分钟）。CLI `--cdc-delay-threshold` / `--cdc-delay-duration`，勾选 B4 而没给参数时先 GET 保留当前值；只改这两个参数时不要求 `--rules` / `--channel`，先 GET 再 PUT 保留其余字段。
- 底层是 `policy_type=sync_job` 的告警策略：规则写在 `conditions[].metric_key`（`operator=">=" threshold="1"`），`cluster_id` 只用于展示，匹配按 `task_id`；同一任务只能有一条。
- 通用策略接口也能改 `sync_job`：必须带 `task_id`（创建后不可改，以库里为准），不能在 `sync_job` 与其他类型之间互转。
- 不跟随任务版本，保存即生效。

### 不阻塞
- `ReportSyncAlert` 用 `select` 非阻塞写入容量 1024 的缓冲区；满了丢弃并计数，warning 日志每分钟最多一条。B4 Refresh 在队列 ≥512 时就丢弃（只计数，不打日志）。

## 4. 校验与错误对应表

| 情况 | 结果 |
|---|---|
| 告警模块未注入（`alertStore == nil`） | 任务告警接口 503 `ErrTaskAlertUnavailable` |
| 任务不可见 / 不可编辑 | 403 `ErrTaskPermissionDenied` |
| 任务不是文件节点 | 400 `ErrTaskNotFile` |
| 规则为空或未知、渠道为空 / 不存在 / 停用、冷却超范围、B4 阈值 / 持续时间超范围 | 400（`syncalert.ErrInvalidConfig`） |
| 通用策略接口的 B4 条件非法 | 400 `ErrAlertPolicySyncJobInvalidCDCDelay` |
| 选了邮件渠道，但没有接收人、没勾选通知创建人、渠道也没配收件人 | 400 |
| 通用接口给同一任务建第二条 `sync_job` | 409 `ErrAlertPolicySyncJobConflict` |
| 通用接口在 `sync_job` 与其他类型之间转换 | 400 `ErrAlertPolicySyncJobTypeChange` |
| CLI / 直接 API 的 PUT、DELETE 缺少确认 | 428（R1 确认） |
| 缓冲区满 | 信号丢弃，warning（每分钟一条） |
| 投递失败 | 记 `notification_deliveries.status=failed`；阶段照常存在，下一次巡检重试 |

## 5. Good / Base / Bad

- Good：任务配置了"作业失败 + webhook"。作业失败三次，webhook 只收到一条通知，告警中心显示累计 3 次；下一次批作业成功后收到恢复通知。
- Base：任务没有配置告警，作业失败时不建阶段、不发通知；集群 running 列表连续失败 6 分钟，告警中心出现"作业状态获取失败"，没有策略所以不发通知。
- Bad：在对账循环里同步调用投递，渠道超时拖慢所有集群的对账；或者每一轮实时作业 running 都发一次 Resolve。

## 6. 必须有的测试

- sync（`alert_signals_test.go`）：A1 只在引擎失败时发（结果未知、409 不发），消息脱敏；B1 只在状态变化时发一次；批作业成功、实时作业转为运行中 / 提交成功时 Resolve；提交过期只报 A2 不报 B1；C1 只发一次、作业终态时 Resolve；D1 Observe 带首次失败时间、启动后首次成功和恢复时 Resolve、正常轮次不发；D2 只在状态变化时发；任务告警接口的权限、默认值、显式 false / 0、503。
- monitoring（`sync_alerts_test.go`）：没配置 / 没勾选规则时丢弃；首次通知一次、后续只累计；告警中心条目字段；Resolve 发恢复通知；冷却提醒与静默；关闭后开新阶段；C1 只解除对应作业；集群规则（无策略只建阶段、D1 窗口、规则停用丢弃）；缓冲区满不阻塞；任务配置存取（创建、更新、409、校验包装、邮件收件人、筛选、删除关闭阶段）；创建人邮箱合并。
- B4 sync（`cdc_delay_alert_test.go`）：持续高于阈值 ≤60 秒内 Fire、两轮低 ≤90 秒恢复、短尖峰不触发、300→260 不触发 / 300→280 触发 / 持平 / 上涨、尖峰不抬高基线、追赶满 30 分钟触发、读取条数推断恢复、unknown 刷新与 24h 停止提醒、重启 maybe Refresh 且只补发一次 Resolve、改参数重新计时不误发恢复、未配置不发、批 / 预览 / 旧样本不计、静默关闭、`-race`；间歇性高延迟（3×200 秒 + 1×30 秒）告警、多源按最大值、多 reader 交错样本不丢、时钟差照常计入并限频 warning、rearm 低轮刷新内容、读配置竞态不缓存、Refresh 只在内容变化时发、推断恢复清掉当前延迟、PUT 不带 `cdc_delay_*` 保留原值（含去掉 B4 再勾回）、2.3.12 版本门槛；H、L、无数据 ×N、H 不告警且"已持续"不含空白时间（Codex P2）；稀疏上报（隔 2/3/5 轮）延迟上涨时告警、孤立尖峰（含阈值 + 间隔 − 1 秒边界）不告警；告警中引擎时钟回拨（样本全判为旧样本、读取条数在涨）不误发恢复、不重复告警；抖动作业的 Refresh 不带 StateChange；一个 reader 卡住、其他在追赶时按时告警；被丢弃的 Refresh 下一轮重发、StateChange、5 分钟重发；快 10 分钟的单条样本不影响后续样本；节点间时钟差 ±3 分钟时卡住的 reader 照常告警。
- B4 monitoring（`sync_alert_cdc_delay_test.go`）：配置往返与 400、标题 / 字段 / webhook / 提醒、Refresh 不计次不通知、停止提醒与读取条数恢复文案、静默关闭显示原因；推断恢复的 webhook `current_delay_ms=null`、队列过半丢 Refresh、停用时不发提醒（锁 + 按 ID 重读）、巡检前先处理排队信号；StateChange Refresh 不受队列过半限制、TryReportSyncAlert 返回结果；webhook 卡住时保存策略不被阻塞、检查后策略被保存则不发；不发问候语的 SMTP 服务器按超时返回；去掉 B4 再勾回参数不变、只保存参数的条件的校验与策略中心原样回写；非恢复关闭后 `last_detail.state=closed`（job_ended / channel_unavailable / rule_changed / rule_removed / policy_deleted；人工关闭明细原因为 `manual`），恢复保留 `recovered`；B4 条件与只保存参数的条件并存被拒绝、读取先认 B4 条件。sync 侧另有接真实 monitoring 存储的去掉 B4 再勾回测试（`task_alert_monitoring_store_test.go`）。
- 原有本地事件投递测试必须保持通过（投递重构的回归）。
- CLI：`set` 的正文和安全头、去掉 B4 且不带参数时请求里不带 `cdc_delay_*`、`--disable` 先读后写、校验在网络前、只有 get / set 没有 delete、`alert-policy list` 带筛选参数。
- 前端：表单校验、摘要、深链解析、侧栏组件。

## 7. 错误与正确示例

#### 错误
```go
// 在对账里直接查策略并发通知：慢渠道会拖住对账，还会让 sync 依赖 monitoring。
if instance.Status == JobStatusFailed {
    monitoringService.DispatchJobFailed(ctx, instance)
}
```

#### 正确
```go
// 写入成功后只发一条非阻塞信号，判定和投递在告警模块的 worker 里完成。
s.reportJobTransition(ctx, instance, previousStatus)
```

#### 错误（B4 追赶基线）
```go
// 用第一轮最大值做基线：一次大事务尖峰（例如 900 秒）之后，持续 300 秒的延迟也被当成"在追赶"，一直不告警。
catchingUp := latestMax <= firstRoundMax*9/10
```

#### 正确（B4 追赶基线）
```go
// 每轮取中位数，窗口内最早一轮作基线；追赶最多压 30 分钟。
catchingUp := len(history) >= 2 && latest.median*100 <= history[0].median*90
```

#### 错误（B4 改参数）
```go
// 告警中改了阈值，新阈值下没超标就发"恢复"：用户会收到一条并非真实恢复的通知。
if lowStreak >= 2 { resolve(syncalert.ResolveReasonRecovered) }
```

#### 正确（B4 改参数）
```go
// 改参数后转 rearm 重新计时，回落时静默关闭。
if phase == cdcPhaseRearm { resolve(syncalert.ResolveReasonRuleChanged) }
```

#### 错误（B4 高低分类与重新计时）
```go
// 按中位数分类：多源作业两个源 5 秒、一个源 900 秒，中位数 5 秒，完全漏报。
high := median(values) >= thresholdMs
// 任意一轮低就重新计时：3 轮 200 秒 + 1 轮 30 秒循环，永远攒不满持续时间。
if !high { aboveSince = time.Time{} }
```

#### 正确（B4 高低分类与重新计时）
```go
// 按本轮最大值分类（中位数只做追赶基线）；连续两轮低才重新计时。
high := maxMs >= thresholdMs
if !high && lowStreak+1 >= cdcDelayLowRoundsToRecover { resetTiming() }
```

#### 错误（B4 旧样本）
```go
// 拿引擎时间和 STX 时钟比：两边时差超过 2 分钟时，所有样本都被静默丢弃。
if now.Sub(time.UnixMilli(sampleMs)) > 2*time.Minute { return }
// 和"作业见过的最新引擎时间"比：一条快 10 分钟的样本之后，所有样本都被当成旧样本；节点间时钟差 >2 分钟时卡住的 reader 被丢。
if time.Duration(newestMs-sampleMs)*time.Millisecond > cdcDelayStaleSample { return }
```

#### 正确（B4 旧样本）
```go
// 按时间差（收到时间 − 引擎时间）对比作业最近 10 分钟计入样本的最大时间差；持续 5 分钟的"旧样本"改为计入。
offset := now.Sub(time.UnixMilli(sampleMs))
if !state.acceptCDCDelayOffset(jobID, offset.Milliseconds(), now) { return false }
```

#### 错误（B4 孤立尖峰）
```go
// 无法确认只清 lowStreak，不清超标计时：H、L、10 分钟无数据、H 会立刻告警，说"已持续几百分钟"。
func (st *cdcDelayJobState) onUnknown(...) { st.lowStreak = 0 }
```

#### 错误（B4 稀疏上报）
```go
// 隔两轮以上非 high 一律重新计时：反压时引擎隔 2～3 轮才上报一次，延迟从 300 秒涨到 1 小时也永远不告警。
if st.phase != cdcPhaseFiring && st.nonHighRounds >= cdcDelayLowRoundsToRecover { st.resetTiming() }
```

#### 正确（B4 孤立尖峰 / 稀疏上报）
```go
// 两轮 high 之间隔了两轮以上非 high（low、unknown 都算）：本轮最大延迟 ≥ 阈值 + 间隔时证明间隔内一直超标，照常累计；
// 否则（孤立尖峰）从后一轮重新计时。
if st.phase != cdcPhaseFiring && st.nonHighRounds >= cdcDelayLowRoundsToRecover {
	if !(point.max >= thresholdMs+now.Sub(st.lastHighAt).Milliseconds()) { st.resetTiming() }
}
```

#### 错误（B4 时钟回拨误恢复）
```go
// 样本全被判为旧样本后本轮"没有样本"，读取条数在涨就推断为 low：告警中引擎时钟回拨 5 分钟会误发恢复、5 分钟后再告警。
case !st.progressAt.IsZero() && now.Sub(st.progressAt) <= cdcDelayProgressWindow: return st.onLow(byReadCount)
```

#### 正确（B4 时钟回拨）
```go
// 收到过样本、全部判为旧样本、一条没计入：按无数据（unknown stale_samples）处理，放在读取条数推断之前。
case in.staleOnly: return st.onUnknown(now, syncalert.CDCDelayUnknownStaleSamples)
```

#### 错误（冷却提醒持锁投递）
```go
// 锁住整个网络投递：webhook 卡住时保存策略一直等。
mu.Lock(); defer mu.Unlock(); check(); dispatch()
```

#### 正确（冷却提醒持锁投递）
```go
// 锁只覆盖检查；投递前核对策略版本，检查之后被保存 / 删除的不发。
episode, policies, version := h.remindCheck(ctx, id)
if h.remindStillValid(ctx, id, version) { h.dispatchEpisode(...) }
```
