# 设计：作业告警第一期

## 1. 现有告警链路（代码位置）

- **事件来源**：`monitor.ProcessEvent`（表 `process_events`）。`monitorService.RecordEvent` 写入后，回调 `monitoring.DispatchAlertPolicyEvent`（`router.go:378`）。
- **匹配策略**：`ListEnabledAlertPoliciesByClusterAndLegacyRuleKey`（`alert_policy_repository.go:41`）。条件是 `cluster_id` 精确等于事件所在集群，并且 `legacy_rule_key` 或 `template_key` 等于规则键。
- **投递**：`dispatchLocalAlertPolicyDelivery`（`alert_policy_execution_service.go:483`）。
  - `NotificationDelivery` 按 `(source_key, channel_id, event_type)` 去重。
  - 已发送过的投递，只有过了冷却时间才重发（`shouldResendLocalAlertPolicyDelivery`）。
  - 遇到 `AlertState` 为关闭或静默时跳过。
  - 恢复通知要求对应的告警通知已发送成功（`requireFiringSent`）。
- **消息**：`buildLocalAlertPolicyPayload` 按渠道类型拼消息体，绑定的是 ProcessEvent。真正发送用 `sendNotification(ctx, channel, payload)`，这一步与来源无关。
- **接收人**：`resolveAlertPolicyReceiverEmails(policy)`，从 `ReceiverUserIDsJSON` 查用户邮箱。
- **持续状态类告警的样板**：
  - `EvaluateNodeHealthAlerts` 发出 `node_offline` 和 `node_recovered`；
  - `DispatchActiveNodeOfflineReminder` 负责冷却后重发提醒；
  - `dispatchLocalResolvedAlertPolicyEvent` 负责恢复通知。
- **默认规则**：`defaultClusterRules` 加上 `ensureDefaultRules`，后者只补齐缺失的规则，所以已有集群也能自动补上新规则。
- **告警中心**：`ListAlertInstances` 合并两类来源：本地事件（`buildLocalAlertInstances`）和远程 Alertmanager 告警（`buildRemoteAlertInstances`）。人工处理状态存在 `AlertState`，按 `source_key` 关联。
- **策略中心前端**：`MonitoringPolicyCenter.tsx`。模板目录在 `allAlertPolicyTemplateSummaries`（`policy_center_service.go:230`）。
- **依赖方向**：sync 不 import monitoring。服务之间的连接都在 `router.go` 里用 setter 完成，例如 `syncService.SetXxx`，在 730 行一带。

## 2. 总体结构

```
sync（信号产生）                       monitoring（判定 + 投递）
───────────────                       ──────────────────────────
提交/对账/停止/事件通道 ──Report──▶  SyncAlertHub（有界 chan + 1 个 worker）
                                         │ 按信号 upsert 告警阶段（sync_alert_episodes）
                                         │ 查策略：任务级（task_id）/ 集群级（cluster_id+rule）
                                         ▼
                                      投递（复用 NotificationDelivery / AlertState / sendNotification）
                                         ▲
                       每分钟 sweeper ──┘ 冷却提醒、D1 窗口成熟判定
任务设置侧栏 ─API─▶ sync handler ─TaskAlertPolicyStore─▶ monitoring（读写 sync_job 策略）
```

两个接口都在 sync 包里定义，由 monitoring 实现，在 `router.go` 里注入：

```go
// SyncAlertReporter 接收 sync 产生的告警信号；实现必须非阻塞。
// SyncAlertReporter receives alert signals produced by sync; implementations must not block.
type SyncAlertReporter interface {
    ReportSyncAlert(signal SyncAlertSignal)
}

// TaskAlertPolicyStore 读写以任务为范围的作业告警策略。
// TaskAlertPolicyStore reads and writes task-scoped job alert policies.
type TaskAlertPolicyStore interface {
    GetTaskAlertPolicy(ctx context.Context, taskID uint) (*TaskAlertConfig, error)
    UpsertTaskAlertPolicy(ctx context.Context, taskID uint, cfg *TaskAlertConfig) (*TaskAlertConfig, error)
    DeleteTaskAlertPolicy(ctx context.Context, taskID uint) error
}
```

两个接口都没注入时（测试，或者告警模块没有初始化），sync 里的调用什么都不做。

## 3. 信号契约

```go
type SyncAlertSignalKind string
const (
    SyncAlertFire     SyncAlertSignalKind = "fire"      // 发生一次（作业级）或状态成立（集群级 D2）
    SyncAlertResolve  SyncAlertSignalKind = "resolve"   // 按 ResolveScope 解除
    SyncAlertObserve  SyncAlertSignalKind = "observe"   // D1：持续失败观测，带 Since；由 monitoring 按窗口判定成熟
)

type SyncAlertSignal struct {
    Kind          SyncAlertSignalKind
    RuleKey       string    // 见 prd R1；Resolve 时可为空，表示按 ResolveScope 解除
    ResolveScope  string    // "task_run_healthy" | "job_terminal" | "cluster_rule"
    ClusterID     uint
    ClusterName   string
    TaskID        uint
    TaskName      string
    TaskCreatorID uint
    JobID         uint
    EngineJobID   string
    TriggerSource string    // manual | schedule | dolphin | cli …（取自 submit_spec.trigger_source）
    RunType       string
    Message       string    // 已脱敏，截断 2000 字符；落库和展示时再截断到 500
    Since         time.Time // Observe 用
    OccurredAt    time.Time
}
```

### 产生信号的位置

| 规则 | 位置 | 说明 |
|---|---|---|
| A1 | `submitTaskInstanceWithPayload`：`ResolveEngineEndpoint` 出错，或 Submit 确定失败 | 本地执行模式下 `submitLocalTaskInstance` 失败也算。`run_type=preview` 不发 |
| A2 | 子任务 1 的接入点 `onSubmitOutcomeExpired` | |
| B1 | `applyEngineJobInfoUpdate` 写入成功、状态从非失败变为 `failed` | 如果 `submit_outcome=expired` 就不发，因为这种情况 A2 已经报过 |
| C1 | 子任务 1 的接入点 `onCancelOverdue` | |
| 恢复 A1/A2/B1 | `applyEngineJobInfoUpdate` 写入成功、状态从非 success 变为 `success`（批作业）或从其他状态变为 `running`（实时作业） | 发 `Resolve` 信号，`ResolveScope=task_run_healthy` |
| 恢复 C1 | `applyEngineJobInfoUpdate` 进入终态时，如果 `result_preview.cancel_overdue_at` 存在 | `ResolveScope=job_terminal`，并带上 JobID |
| D1 | `reconcileClusterJobs`：找不到端点或者 `ListRunningJobs` 出错时发 `Observe(Since=首次失败时间)`；成功时如果之前失败过，发 `Resolve(cluster_rule)` | 每个集群的首次失败时间放在内存 map 里 |
| D2 | 事件通道状态改变的地方：设置或清除 `ExpectMissedAt`、`ExpectSatisfiedAt`，以及关闭通道 | 状态从 healthy 变为 degraded 时发 Fire；从 degraded 变为 healthy 或 disabled 时发 Resolve |

- **判断作业的批或流模式**：用 `instance.Mode`，取不到时用 `submit_spec.mode`。
- **实时作业重复上报**：实时作业每一轮都是 running，只在状态**发生变化**时发信号，所以不会每一轮都重复发送。
- **工具函数**：`syncAlertTriggerSource(job)` 和 `syncAlertSignalFromJob(job, task)`，放在新文件 `internal/apps/sync/alert_signals.go`。

## 4. 数据模型（monitoring）

### 4.1 新表 `monitoring_sync_alert_episodes`

```go
type SyncAlertEpisode struct {
    ID              uint      `gorm:"primaryKey;autoIncrement"`
    Scope           string    `gorm:"size:20;not null;index:idx_sync_alert_episode_key,priority:1"`  // task | cluster
    ClusterID       uint      `gorm:"not null;default:0;index:idx_sync_alert_episode_key,priority:2"`
    TaskID          uint      `gorm:"not null;default:0;index:idx_sync_alert_episode_key,priority:3"`
    RuleKey         string    `gorm:"size:80;not null;index:idx_sync_alert_episode_key,priority:4"`
    Status          string    `gorm:"size:20;not null;index"`  // firing | resolved
    Severity        AlertSeverity `gorm:"size:20"`
    ClusterName     string    `gorm:"size:255"`
    TaskName        string    `gorm:"size:255"`
    TaskCreatorID   uint
    OccurrenceCount int       `gorm:"not null;default:1"`
    FirstFiredAt    time.Time `gorm:"index"`
    LastOccurredAt  time.Time
    LastJobID       uint
    LastEngineJobID string    `gorm:"size:64"`
    LastTriggerSource string  `gorm:"size:40"`
    LastMessage     string    `gorm:"type:text"`
    ResolvedAt      *time.Time `gorm:"index"`
    ResolveReason   string    `gorm:"size:40"`
    CreatedAt, UpdatedAt time.Time
}
```

- 普通索引，加上"先查后写"，由 hub 的单个 worker 串行处理，不使用部分唯一索引，三种数据库都兼容。
- AutoMigrate 注册在 `migrator.go`。按 `database-guidelines.md` 自检，并跑 `check_db_compatibility.py`。

### 4.2 `monitoring_alert_policies` 新增两列
- `TaskID uint`：`gorm:"not null;default:0;index"`，值为 0 表示不是任务策略。
- `NotifyTaskOwner bool`：`gorm:"not null;default:false"`。

`PolicyType` 新增 `AlertPolicyBuilderKindSyncJob = "sync_job"`。

### 4.3 来源类型
- `AlertSourceTypeSync = "sync"`。
- `source_key` 的格式为 `sync:<episode_id>`，`parseAlertSourceKey` 需要支持这种格式。
- 确认、静默、关闭沿用 `AlertState`。

## 5. 判定与投递（新文件 `monitoring/sync_alerts.go`）

### 5.1 SyncAlertHub
- 有界 channel，容量 1024，一个 worker。
- `ReportSyncAlert` 用 `select` 非阻塞写入；写不进去时计数并打 warning 日志，同一分钟只打一条。
- 收到信号后：
  1. **Fire**（作业级）：
     - 先检查任务有没有已启用的 `sync_job` 策略，并且策略条件里包含这条规则；没有就**直接丢弃**，不建阶段（对应 prd 的"不配不发"）。
     - 找到"告警中"的阶段：次数加一，更新最近一次的信息，不发送通知。
     - 没找到：新建阶段，然后投递告警通知。
  2. **Fire**（集群级 D2）：
     - 先检查集群的这条默认规则（`AlertRule`）是否启用；没启用就丢弃。
     - 建阶段或累计次数的做法同上。
     - 投递时按集群和规则查策略，查不到策略就只建阶段、不发通知，告警中心照样能看到。
  3. **Observe**（D1）：读集群规则的 `WindowSeconds`（默认 300）。`now - Since >= window` 并且当前没有告警中的阶段，就按集群级 Fire 处理。
  4. **Resolve**：
     - `task_run_healthy`：关闭该任务 A1、A2、B1 的告警中阶段；
     - `job_terminal`：关闭该任务 `LastJobID=JobID` 的 C1 阶段；
     - `cluster_rule`：关闭该集群这条规则的阶段。
     
     阶段关闭后，按策略的 `SendRecovery` 投递恢复通知。

### 5.2 投递
- 抽出与来源无关的 `dispatchAlertDelivery(ctx, alertMessage, policy, channel, eventType, sourceKey, requireFiringSent)`。原来的 `dispatchLocalAlertPolicyDelivery` 改成先组装 `alertMessage`，再调用它。重构时**本地事件的已有测试必须全部保持通过**。
- `alertMessage` 包含标题、纯文本、HTML、结构化 alert map、集群 ID 和名称、告警名称、来源类型。
- 接收人：`resolveAlertPolicyReceiverEmails(policy)`；如果 `NotifyTaskOwner=true`，再加上 `TaskCreatorID` 对应用户的邮箱，去重。
- 消息字段见 prd R6。链接通过 `config.GetExternalURL()` 拼出：`/sync?task=<id>&job=<id>`。

### 5.3 Sweeper
- 每分钟运行一次，跟 `StartNodeHealthEvaluator` 一起启动。
- 任务：给告警中的阶段发冷却提醒。规则是按策略找到渠道，复用 `shouldResend`，投递类型用 firing，同一行投递的 `AttemptCount` 加一。

## 6. 任务告警配置 API（sync）

| 方法 | 路由 | 权限 | 说明 |
|---|---|---|---|
| GET | `/api/v1/sync/tasks/:id/alert` | `CanUserView` | 未配置时返回 `{configured:false}` |
| PUT | `/api/v1/sync/tasks/:id/alert` | `CanUserEdit` | 写入配置，需要确认头，风险等级 R1，记审计 |
| DELETE | `/api/v1/sync/tasks/:id/alert` | `CanUserEdit` | 删除配置，同时关闭这个任务的告警中阶段，原因记为 `policy_deleted` |

```go
type TaskAlertConfig struct {
    Configured      bool     `json:"configured"`
    PolicyID        uint     `json:"policy_id,omitempty"`
    Enabled         bool     `json:"enabled"`
    Rules           []string `json:"rules"`              // ⊆ {sync_submit_failed, sync_submit_timeout, sync_job_failed, sync_stop_timeout}
    ChannelIDs      []uint   `json:"channel_ids"`        // ≥1，必须存在且启用
    ReceiverUserIDs []uint   `json:"receiver_user_ids"`
    NotifyTaskOwner bool     `json:"notify_task_owner"`
    CooldownMinutes int      `json:"cooldown_minutes"`   // 0..1440，默认 60
    SendRecovery    bool     `json:"send_recovery"`      // 默认 true
}
```

- 策略的其他字段：名称为 `任务 <name> 作业告警`；`ClusterID` 写当前任务的集群，只用于展示，匹配按 `TaskID`；`TemplateKey="sync_job"`；`Severity` 写 critical，只用于展示，实际级别按规则目录的默认值。
- 校验错误返回 400：规则为空或未知、渠道为空或不存在、选了邮件渠道却没有任何收件人（接收人为空，且没勾选通知创建人，且渠道本身也没配收件人）。
- 按 `execution-security-guidelines.md`，PUT 和 DELETE 都要求确认头和幂等键，并且登记操作。
- `registry.go` 和 `route_baseline.json` 新增 3 条路由，Swagger 也要补上。

策略中心：
- `ListAlertPolicies` 支持 `policy_type=sync_job&task_id=` 筛选，DTO 里带 `task_id`、`task_name`。`task_name` 由 hub 通过一个可选的 `TaskNameResolver` 接口查询，接口由 sync 实现。
- 用通用的 `UpsertAlertPolicyRequest` 新建 `sync_job` 策略时，必须带 `task_id`，同一个任务只能有一条，否则返回 409。

## 7. 前端

- **任务设置侧栏**（`SettingsSidebarPanel.tsx`）新增"告警"分组。
  - 未配置时只显示一行文字和一个按钮；已配置时显示摘要，例如"失败、提交超时 · 钉钉运维群 · 冷却 60 分钟"，点击后展开表单。
  - 表单字段对应 `TaskAlertConfig`。渠道和用户的列表复用 monitoring service 里现有的查询接口。
- **策略中心**（`MonitoringPolicyCenter.tsx`）：
  - 新增"作业告警"筛选，列表显示任务名并能跳转到任务。
  - 编辑时复用一个独立出来的 `TaskAlertForm` 组件。
  - 模板目录里新增两个集群级模板，归到"作业观测"类别。
- **告警中心**（`MonitoringAlertsCenter.tsx`）：sync 来源的告警显示任务、作业、次数，并带跳转链接。
- **深链**：`/sync` 页面读取 `task`、`job` 两个查询参数，调用现有的打开任务、选中作业逻辑。
- **i18n**：`monitoring.syncAlerts.*`、`sync.taskAlert.*`，中英文各一份。遵循 `ui-conventions.md`：渐进披露、原子信息不换行、加载时不塌陷。

## 8. CLI

- `internal/cmd/sync_alert.go`：
  - `get`：读取并输出配置；
  - `set`：把参数组装成 PUT 请求，走 `prepareSecureWrite`；
  - **不提供 `delete`**：`cli-guidelines.md` §11 禁止 CLI 删除资源。停用告警用 `set --disable`，对应 PUT 请求里的 `enabled=false`；删除只在网页和 API 里提供。
- `stx monitoring alert-policy list` 支持 `--policy-type`、`--task-id` 两个筛选参数（如果现有命令没有，就补上）。

## 9. 兼容与回滚

- 新表和新列都通过 AutoMigrate 创建，回滚后旧版本会忽略它们。
- 现有告警的行为不变：本地事件投递那部分重构，要靠现有测试兜底。
- 新的默认规则通过 `ensureDefaultRules` 自动补到已有集群。没有策略时只在告警中心显示，不会发出任何通知。所以升级后不会突然多出通知。

## 10. 测试要点

| 层 | 用例 |
|---|---|
| sync | 各信号的产生位置：A1 的确定失败和校验错误、B1 的状态变化和过期例外、恢复信号、preview 排除、D1 的首次失败时间与恢复、D2 的状态变化 |
| monitoring hub | 没有策略时丢弃；首次 Fire 建阶段并投递；再次 Fire 只累计；Resolve 各种范围；集群规则禁用时丢弃；D1 的窗口判定 |
| 投递 | 通知创建人时的收件人合并；webhook、钉钉、邮件的消息内容；静默时不发；关闭后再 Fire 开新阶段 |
| sweeper | 冷却提醒，用短冷却时间加上可注入的时钟 |
| 重构回归 | 现有 monitoring 测试全部通过 |
| API | 权限（看与改）、校验、409、确认头 |
| CLI | get / set / set --disable；确认没有 delete 子命令 |
| 前端 | 表单校验、摘要文案、深链解析 |
