# 集群告警规则目录与默认通知

> 适用于 `internal/apps/monitoring/cluster_rule_catalog.go`、`cluster_rule_config.go`、`cluster_rule_api.go`、`cluster_rule_handler.go`、`availability_evaluator.go`、`alert_rule_catalog.go`、`internal/db/migrator/alert_rule_catalog_migration.go` 及 `internal/cmd/monitoring_write.go`。作业级规则见 [同步作业告警](./sync-alerts.md)。

## 1. 范围 / 触发条件

- 新增或修改集群级告警规则目录、全局层 / 集群例外层配置解析、可用性评估（`node_offline` / `agent_offline` / `master_unavailable` / `worker_insufficient`）、告警级别与外部通知校验、默认通知或 `scope` 筛选时。
- 原则：
  - **两层用户范围**：用户面对的告警只分 **集群级（`cluster`）** 与 **作业级（`job`）**；判定引擎（`availability` / `event` / `tsdb` / `sync`）属于目录内部属性，不暴露给用户分类。
  - **代码即规则目录**：集群级规则以代码常量目录为唯一来源，取代旧内置 YAML 告警组（`rules/seatunnel-default-rules.yml` 仅保留 recording 组）、非作业策略模板与 `monitoring_alert_rules` 种子。
  - **只存偏离值**：数据库只持久化偏离目录默认值的配置，调整目录默认值可自动惠及未覆盖的规则。

## 2. 签名

```go
// internal/apps/monitoring/cluster_rule_catalog.go
type ClusterRuleDefinition struct {
    Key            string
    Name, NameEN   string
    Description, DescriptionEN string
    Engine         clusterRuleEngine // availability | event | tsdb | sync
    Severity       AlertSeverity     // 目录默认均为 warning，用户可编辑
    DefaultEnabled bool
    Threshold      *ThresholdSpec    // nil 表示无阈值
    DefaultFor     time.Duration
    ForEditable    bool
    Expr           func(cfg *resolvedRuleConfig, clusterMatcher string) string
}

// internal/apps/monitoring/cluster_rule_config.go
func (s *Service) ResolveClusterRule(ctx context.Context, ruleKey string, clusterID uint) (*resolvedRuleConfig, error)
func (s *Service) SaveClusterRuleConfig(ctx context.Context, ruleKey string, fields *clusterRuleFields) (*ClusterRuleConfig, error)
func (s *Service) DeleteClusterRuleConfig(ctx context.Context, ruleKey string) error
func (s *Service) SaveClusterRuleOverride(ctx context.Context, ruleKey string, clusterID uint, fields *clusterRuleFields) (*ClusterRuleOverride, error)
func (s *Service) DeleteClusterRuleOverride(ctx context.Context, ruleKey string, clusterID uint) error
func (s *Service) SaveAlertNotificationDefaults(ctx context.Context, channelIDs []uint, receiverUserIDs []uint64, cooldownMinutes int, sendRecovery bool) (*AlertNotificationDefaults, error)

// internal/db/migrator/alert_rule_catalog_migration.go
const AlertRuleCatalogMigrationName = "alert_rule_catalog_v1"
func MigrateAlertRuleCatalog(database *gorm.DB) error
```

| 方法 | 路由 | 说明 | CLI |
| --- | --- | --- | --- |
| GET | `/api/v1/monitoring/cluster-rules` | 列出集群规则目录、全局原值、例外与生效结果 | `stx monitoring cluster-rule list` |
| GET | `/api/v1/monitoring/cluster-rules/:key` | 查询单条规则详情 | `stx monitoring cluster-rule get <key>` |
| PUT | `/api/v1/monitoring/cluster-rules/:key` | 整层替换全局配置（字段传 `null` 恢复继承） | `stx monitoring cluster-rule update <key> --request-file … --confirm` |
| DELETE | `/api/v1/monitoring/cluster-rules/:key` | 删除全局配置恢复目录默认 | 不生成 CLI（`cli-guidelines.md` §11） |
| PUT | `/api/v1/monitoring/cluster-rules/:key/clusters/:clusterId` | 整层替换单集群例外 | `stx monitoring cluster-rule override set <key> <cluster-id> --request-file … --confirm` |
| DELETE | `/api/v1/monitoring/cluster-rules/:key/clusters/:clusterId` | 删除单集群例外 | 不生成 CLI（`cli-guidelines.md` §11） |
| GET | `/api/v1/monitoring/alert-notification-defaults` | 查询默认通知（未配置时 `configured=false`） | `stx monitoring notification-defaults get` |
| PUT | `/api/v1/monitoring/alert-notification-defaults` | 整行替换默认通知 | `stx monitoring notification-defaults update --request-file … --confirm` |
| POST | `/api/v1/monitoring/notification-channels/test` | 草稿渠道发送测试（不入库） | `stx monitoring notification-channel test-draft --request-file … --confirm` |
| POST | `/api/v1/monitoring/notification-channels/test-connection` | SMTP 连接测试 | `stx monitoring notification-channel test-connection --request-file … --confirm` |

数据库新表（已加入 `migrator.go` `managedModels()`）：
- `monitoring_cluster_rule_configs`（`ClusterRuleConfig`，`rule_key` 唯一索引）
- `monitoring_cluster_rule_overrides`（`ClusterRuleOverride`，`(rule_key, cluster_id)` 联合唯一索引 `ux_monitoring_cluster_rule_override`）
- `monitoring_alert_notification_defaults`（`AlertNotificationDefaults`，单行）

## 3. 契约

### 规则目录与默认开关（21 条集群级规则）
- **默认开启（13 条）**：
  - `availability`：`node_offline`（2m）、`master_unavailable`（1m）、`worker_insufficient`（阈值默认=部署 worker 数，2m）、`agent_offline`（2m）
  - `event`：`process_crashed`、`process_restart_failed`、`process_restart_limit_reached`
  - `sync`：`sync_status_unavailable`（5m）
  - `tsdb`：`split_brain_risk`（2m）、`deadlocked_threads_detected`（1m）、`job_thread_pool_rejection_high`（>0 / 5m）、`memory_usage_high`（>85% / 10m）、`fd_usage_high`（>80% / 10m）
- **默认关闭（8 条）**：
  - `tsdb`：`cpu_usage_high`（>80% / 10m）、`gc_time_high`（>20% / 10m）、`job_thread_pool_queue_backlog_high`（>100 / 5m）、`coordinator_backlog_growing`（>50 / 10m）、`failed_jobs_high`（>0 / 1m）
  - `sync`：`sync_event_channel_degraded`
  - `event`：`cluster_restart_requested`、`node_stop_requested`
- 阈值统一使用用户可读单位（`percent` / `count` / `workers`），`tsdb` 表达式内部自动做百分比到 ratio 换算。
- **`AlertRuleCatalog`（`/alert-rules/catalog`）派生契约**：集群级按 `clusterRuleCatalog` 顺序输出（名称取 `Name`/`NameEN`，`category` 由 `Engine` 派生：`tsdb→metric`、其余→`platform_health`，`scope=cluster`），作业级追加 `syncalert.TaskRules()`（`category=job`，`scope=job`）；不再从策略模板摘要拼装，5 条 `stx_*` 平台自监控模板不在目录内。诊断自动巡检 `ALERT_FIRING` 条件引用目录外规则键时 `InvalidReason=unknown_alert_rule_key`（`invalid_detail` 为首个未知键），存量策略不改写、仅提示。

### 三层配置解析与缓存
- 解析顺序：**目录默认 → `ClusterRuleConfig`（全局）→ `ClusterRuleOverride`（集群例外）**。
- 指针字段为 `nil` 表示继承上一层；`PUT` 整层替换，传入 `null` 即清除该字段回退继承。
- `ResolveClusterRule` 带短 TTL 进程内缓存；保存 / 删除配置、例外或默认通知时立即失效缓存，并向 `MetricEvaluator.reloadCh` 发送重载信号。
- `buildManagedRuleGroups` 为每条启用的 `tsdb` 规则生成一条全局 rule（`cluster_id!~"<例外集群列表>"`）和每个有效例外的集群 rule（`cluster_id="<id>"`），标签携带 `managed_by=stx` 与 `rule_key`。

### 可用性评估器（`availability_evaluator.go`）
- 按 `eval_interval`（默认 15s）评估全部受管集群，内存中维护 pending 起点实现 `for` 连续判定语义，单次恢复即清零。
- **`node_offline` 合并与去重边界**：合并进程原因（`process_stopped` / `process_missing` / `node_error` / `host_visibility_lost`）与 TSDB 即时查询（`up{job="seatunnel_engine_http"}==0` → `metrics_endpoint_down`、`node_state==0` → `node_state_down`）；**`host_offline`（Agent 心跳超时）不再归入 `node_offline`**，改由 `agent_offline` 按主机聚合，避免主机断连触发双重节点告警。
- `master_unavailable`：role ∈ `{master, master_worker}` 的健康节点数 == 0。
- `worker_insufficient`：role ∈ `{worker, master_worker}` 的健康节点数 < 阈值；`Threshold == nil` 表示等于部署 worker 数（部署数为 0 的集群跳过）。

### 级别语义与默认通知
- **级别即是否外发**：
  - `warning`（目录默认）：告警只进平台告警中心，**不发送外部通知**。
  - `critical`：告警进告警中心并发往外部渠道；保存规则或例外时校验生效通知中**至少包含一个启用的外部渠道**（且邮件渠道必须存在可达接收人），否则返回 400 拒绝写入。
- **不变式与退化标识**：渠道后续被停用 / 删除时**不自动降级**规则级别（停用是临时态，恢复后补发）；接口在 `ClusterRuleView.external_channel_missing` / `effective.external_channel_missing` 标记 `true`，由前端展示警告标识。
- 通知来源：`""` / `default` 沿用默认通知；`custom` 使用规则自定义渠道与接收人；`none` 保留字段兼容迁移且等价于不外发（前端不暴露）。冷却与恢复通知设置取自默认通知。

### Scope 与已移除的旧契约
- 存储 `source_type`（`platform_health` / `metric` / `job`）不变，DTO 与筛选统一派生 `scope`：`job → job`，其余 → `cluster`。告警实例列表同时接受 `scope=cluster|job` 与旧 `source_type`。
- **已移除接口与字段**：
  - 删除 `/notification-routes*`、`/clusters/:id/rules*`、`/alert-policies/bootstrap` 路由与对应 CLI；
  - `/alert-policies` 的 `POST` / `PUT` 只接受 `sync_job`，其余类型返回 `400`（`ErrAlertPolicyUseClusterRules`）；
  - `promql` 字段从策略请求结构移除，不接受也不存储。

### 一次性升级迁移（`alert_rule_catalog_v1`）
- 在 `migrator.go` `AutoMigrate` 之后幂等执行一次（`SchemaFlag`）：
  - 将启用的 `metrics_template` / `platform_health` 策略按全局 / 集群转为 `ClusterRuleConfig` / `ClusterRuleOverride`（ratio→percent 换算；有启用渠道者设为 `critical`，无渠道者降为 `warning` 以满足不变式；多条冲突取最近更新的启用策略）；
  - 将 `monitoring_alert_rules` 中偏离默认开关 / 窗口的行转为集群例外；
  - 将 `NotificationRoute` 渠道并入默认通知或对应规则 `custom` 渠道；若迁移后默认通知仍为空且存在启用策略，取策略渠道 / 接收人并集兜底；
  - 旧表旧行**保留不删**（仅把已迁移非作业策略置 `Enabled=false`），`sync_job` 策略保持不动，迁移摘要以中英双语输出。

## 4. 校验与错误对应表

| 情况 | 结果 |
| --- | --- |
| 规则键不在目录中 | 404 `ErrClusterRuleNotFound` |
| 非法级别、非法 `notify_mode`、阈值超范围、不可编辑的 `for_seconds` 或非法集群 ID | 400（`ErrClusterRuleInvalid*` / `ErrClusterRuleUnsupportedField`） |
| 级别为 `critical` 但生效通知无启用外部渠道 | 400 `ErrClusterRuleCriticalChannelRequired` |
| 引用的渠道不存在或停用、邮件渠道无可达接收人 | 400（`ErrAlertPolicyNotificationChannelNotFound` / `ErrClusterRuleChannelDisabled` / `ErrClusterRuleEmailRecipientMissing`） |
| 向 `POST/PUT /alert-policies` 提交非 `sync_job` 类型 | 400 `ErrAlertPolicyUseClusterRules` |

## 5. Good / Base / Bad

- Good：新装环境不做任何策略配置，节点进程退出后 2 分钟内告警中心出现一条 `node_offline`；将 `cpu_usage_high` 设为 `critical` 且未配默认通知或自定义渠道时，保存返回 400 不落库。
- Base：规则全局保持 `warning` 仅进告警中心，只为特定关键集群添加 `critical` + 自定义渠道的例外覆盖。
- Bad：同时保留 YAML 告警组与策略模板导致同一异常发两条告警；或把 `host_offline` 塞进 `node_offline` 导致 Agent 掉线一次产生两条节点与主机重复告警。

## 6. 必须有的测试

- 目录与配置（`cluster_rule_config_test.go`）：目录 21 条默认值断言、三层继承与 `nil` 复位、缓存失效、critical 无外部渠道拒绝 400、渠道停用后 `external_channel_missing=true` 且不降级。
- 评估器（`metric_evaluator_test.go`、`node_offline_evaluator_test.go`）：TSDB 全局排除例外与例外生成、表达式 `parser.ParseExpr` 校验；`node_offline` 多原因合并、`host_offline` 只出 `agent_offline`、`master_unavailable` 与 `worker_insufficient` 触发及恢复、启动抑制。
- 迁移（`internal/db/migrator/alert_rule_catalog_migration_test.go`）：全局 / 例外策略、冲突取舍、ratio→percent、`AlertRule` 偏离行、三种 `NotificationRoute`、默认通知并集兜底、`sync_job` 不动、幂等重跑。
- 接口与 CLI（`alert_policy_service_test.go`、`internal/cmd/monitoring_write_test.go`、`internal/operation/validate_test.go`）：非 `sync_job` 返回 400、`promql` 忽略、`cluster-rule` / `notification-defaults` / `test-draft` / `test-connection` 命令与路由登记校验。

## 7. 错误与正确示例

#### 错误
```go
// 把 Agent 心跳超时计入 node_offline，同时又评估 agent_offline：一台主机掉线会发两条告警。
if !node.IsOnline {
    reasons = append(reasons, "host_offline")
}
```

#### 正确
```go
// host_offline 归入 agent_offline 按主机聚合；node_offline 只看进程侧异常与 TSDB up/node_state。
// host_offline belongs to agent_offline (per-host); node_offline only inspects process state and TSDB up/node_state.
if isHostOffline(node) {
    recordAgentOffline(hostID)
}
```
