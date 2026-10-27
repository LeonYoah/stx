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
| `sync_status_unavailable` | D1 | 集群 | 规则配置（默认 warning） | 集群对账提前结束时发 `Observe`，持续时间 ≥ 规则 `WindowSeconds`（默认 300）才成立 | 恢复或进程启动后首次成功：`cluster_rule` |
| `sync_event_channel_degraded` | D2 | 集群 | 规则配置（默认 warning） | 通道状态从非降级变为 `degraded` | 离开降级（healthy / disabled）：`cluster_rule` |

`task_run_healthy`：批作业状态变为 `success`；实时作业状态变为 `running`，或实时作业提交成功（提交时就记为 running）。模式取 `job.Mode`，为空时取 `submit_spec.mode`，仍为空按实时作业处理。

### 信号
- 作业级信号的快照字段：`TaskID`、`TaskName`、`TaskCreatorID`、`ClusterID`（`submit_spec.cluster_id`，没有时取任务）、`JobID`、`EngineJobID`、`TriggerSource`（`submit_spec.trigger_source`：manual / schedule / dolphin）、`Message`（`audit.RedactText` 脱敏后截断 2000 字符）。
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

### 投递
- 与来源无关的 `alertMessage`（来源类型、集群、告警名、额外收件人、按渠道拼消息体），本地进程事件和作业告警共用 `dispatchAlertPolicyChannels` / `dispatchAlertDelivery`。
- 标题：`[STX][告警|恢复][级别] 任务 <名称> · <规则名>`（集群级为"集群 <名称>"）。
- 正文：状态、规则、集群、任务、作业 `#id`、引擎作业 ID、触发来源、累计次数、首次 / 最近发生时间、恢复时间、错误摘要（500 字符）、策略、链接。
- webhook 带结构化 `alert`：`source_type=sync`、`source_key`、`status`、`scope`、`rule_key`、`task_id`、`job_id`、`occurrence_count`、`link` 等。
- 链接：`{app.console_url}/sync?task=<id>&job=<id>`，`console_url` 为空时不带链接；告警中心用相对链接 `/sync?task=&job=`。**不要用 `external_url`**：它是 Agent、Alertmanager 回调的后端地址（默认 17800），默认安装里前端在另一个端口（17880），拼出来的链接会 404。
- 邮件收件人：策略接收人 + `notify_task_owner` 时的任务创建人邮箱（用户停用或没有邮箱时忽略，不让告警失败），按小写去重。

### 任务告警配置（TaskConfig）
- `rules ⊆ {sync_submit_failed, sync_submit_timeout, sync_job_failed, sync_stop_timeout}`，至少一条；`channel_ids` 至少一个、必须存在且启用；`cooldown_minutes` 0–1440；PUT 时 `enabled`、`send_recovery` 默认 true，`cooldown_minutes` 默认 60。
- 底层是 `policy_type=sync_job` 的告警策略：规则写在 `conditions[].metric_key`（`operator=">=" threshold="1"`），`cluster_id` 只用于展示，匹配按 `task_id`；同一任务只能有一条。
- 通用策略接口也能改 `sync_job`：必须带 `task_id`（创建后不可改，以库里为准），不能在 `sync_job` 与其他类型之间互转。
- 不跟随任务版本，保存即生效。

### 不阻塞
- `ReportSyncAlert` 用 `select` 非阻塞写入容量 1024 的缓冲区；满了丢弃并计数，warning 日志每分钟最多一条。

## 4. 校验与错误对应表

| 情况 | 结果 |
|---|---|
| 告警模块未注入（`alertStore == nil`） | 任务告警接口 503 `ErrTaskAlertUnavailable` |
| 任务不可见 / 不可编辑 | 403 `ErrTaskPermissionDenied` |
| 任务不是文件节点 | 400 `ErrTaskNotFile` |
| 规则为空或未知、渠道为空 / 不存在 / 停用、冷却超范围 | 400（`syncalert.ErrInvalidConfig`） |
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
- 原有本地事件投递测试必须保持通过（投递重构的回归）。
- CLI：`set` 的正文和安全头、`--disable` 先读后写、校验在网络前、只有 get / set 没有 delete、`alert-policy list` 带筛选参数。
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
