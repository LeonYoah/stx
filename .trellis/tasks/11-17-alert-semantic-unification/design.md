# 告警语义统一 · 技术设计

前置阅读：同目录 `prd.md`（事实、名词表、验收）。本设计只写"怎么改"。

## 0. 总原则

- **对外一个"类别"，对内一个"存储种类"，两者分开**。
  - 对外：`source_type` = `platform_health | metric | job`，给界面、API、CLI、webhook、通知路由用。
  - 对内：告警 ID 前缀（`local:` / `remote:` / `sync:`）决定去哪张表取数、确认 / 静默 / 关闭走哪个分支。
    这层**不对外暴露、不改格式**。
  - 现在 `AlertSourceType` 一个枚举身兼两职，所以 sync 阶段既是"存储种类"又被当成"作业告警类别"，
    集群级作业规则才会归错类。拆开后各管各的。
- 不做迁移、不留别名（用户已确认）。

## 1. 后端 · 告警类别（R1）

### 1.1 枚举

`internal/apps/monitoring/model.go:176-192` 改为：

```go
// AlertSourceType 是告警类别（对外语义），按规则归属判定，与存储表无关。
// AlertSourceType is the public alert category, decided by which rule fired rather than which table stores it.
type AlertSourceType string

const (
	AlertSourceTypePlatformHealth AlertSourceType = "platform_health"
	AlertSourceTypeMetric         AlertSourceType = "metric"
	AlertSourceTypeJob            AlertSourceType = "job"
)
```

JSON 字段名保持 `source_type`（只换值，降低前端 / CLI 改动面）。删除三个旧常量，编译器会把所有引用点列出来。

### 1.2 内部存储种类

在 `alert_instance_service.go` 新增**未导出**类型，替换 `parsedAlertSourceKey.SourceType`：

```go
// alertStoreKind 是告警的存储种类，只由告警 ID 前缀决定，仅用于内部分发。
// alertStoreKind is where an alert is stored, derived from the alert ID prefix; internal dispatch only.
type alertStoreKind string

const (
	alertStoreLocalEvent   alertStoreKind = "local_event"   // local:event:<id>
	alertStoreMetricRecord alertStoreKind = "metric_record" // remote:<fp>:<startsAt>
	alertStoreSyncEpisode  alertStoreKind = "sync_episode"  // sync:episode:<id>
)
```

- `parseAlertSourceKey`（`alert_instance_service.go:806`）返回 `Kind alertStoreKind`；
  确认 / 静默 / 关闭 / 详情里 `switch parsed.SourceType`（182、281、376 行附近）改为 `switch parsed.Kind`。
- `alert_policy_execution_service.go:362` 的派发消息结构里 `SourceType` 字段保留（写投递记录用），
  **另加** `Kind alertStoreKind`；580 / 601 行"同步告警走退避重试 / 未送达补说明"的判断改为
  `message.Kind == alertStoreSyncEpisode`。这两处是存储语义（只有 sync 阶段有退避重试），不能用类别判断，
  否则集群级作业规则改成 `platform_health` 后会丢掉退避重试。

### 1.3 类别判定

```go
// syncEpisodeCategory 返回同步告警阶段的类别：集群级规则归平台健康，任务级归作业告警。
// syncEpisodeCategory maps a sync episode to its category: cluster rules are platform health, task rules are job alerts.
func syncEpisodeCategory(ruleKey string) AlertSourceType {
	if syncalert.IsClusterRule(ruleKey) {
		return AlertSourceTypePlatformHealth
	}
	return AlertSourceTypeJob
}
```

| 构造点 | 新类别 |
|---|---|
| `buildLocalAlertInstances`（`alert_instance_service.go:543`、763） | `platform_health` |
| `buildRemoteAlertInstances`（678） | `metric` |
| `buildSyncAlertInstances`（`sync_alert_episode.go:333`） | `syncEpisodeCategory(episode.RuleKey)` |
| sync 投递 / webhook（`sync_alerts.go:811`、865） | `syncEpisodeCategory(...)` |
| 指标告警投递 / webhook（`notification_delivery_service.go:175`、193、342；`managed_remote_alert_delivery.go:241`、366） | `metric` |

### 1.4 列表过滤

`ListAlertInstances`（`alert_instance_service.go:63-87`）不再用三个 include 布尔对应三张表，改为：

```go
includeLocal  := filter.SourceType == "" || filter.SourceType == AlertSourceTypePlatformHealth
includeRemote := filter.SourceType == "" || filter.SourceType == AlertSourceTypeMetric
includeSync   := filter.SourceType == "" || filter.SourceType == AlertSourceTypePlatformHealth || filter.SourceType == AlertSourceTypeJob
```

`buildSyncAlertInstances` 内按 `syncEpisodeCategory` 再过滤一次（filter 为空时不过滤）。统计接口若也按 source_type 过滤，同样处理。

### 1.5 校验点

- `alert_instance_handler.go:39`：只接受三个新值，否则 400 `invalid source_type`。
- 通知路由 `UpsertNotificationRouteRequest.Validate`（`model.go:1215`）：现在只允许 local / remote 两值。
  改为允许 `platform_health | metric | job` 三值。
- `matchRemoteAlertRoute`（`notification_delivery_service.go:534`）比较值改为 `AlertSourceTypeMetric`。
  另外核对平台健康、作业告警走的路由匹配函数是否也读 `route.SourceType`；不读的话补上同样的过滤，保证三类对称。

## 2. 后端 · 规则键（R2）

### 2.1 指标告警补 `rule_key`

`buildRemoteAlertInstances` 构造 `AlertInstance` 时，从 `row.LabelsJSON` 解出 `template_key` 赋给 `RuleKey`
（评估器已把它写进规则 labels：`metric_evaluator.go:615-617`）。抽一个 `remoteRecordRuleKey(row) string`，解析失败返回空串。
不改表结构。

### 2.2 告警规则目录

新增只读接口：

```
GET /api/v1/monitoring/alert-rules/catalog
→ { "data": [ { "rule_key": "node_offline", "name": "节点离线", "category": "platform_health" }, ... ] }
```

- 数据来源（monitoring 包内一个函数 `AlertRuleCatalog(lang) []*AlertRuleCatalogItem`）：
  - 策略模板列表（`policy_center_service.go:193` 起的模板）：`SourceKind=platform_health` → `platform_health`；
    `metrics_template` → `metric`；
  - 任务级作业规则 `syncalert.TaskRules()`（`internal/pkg/syncalert/syncalert.go:66`）→ `job`；
    显示名复用 `syncAlertRuleName`。
  - 按 `rule_key` 去重。集群级作业规则已经以平台健康模板出现，不重复。
- 名称走现有 zh / en 显示名函数，与告警中心保持一致。
- 路由注册在 monitoring 只读组，权限同 `alert-policies/bootstrap`。
- CLI（AGENTS.md「新增接口要补 CLI」）：`internal/operation/registry.go` 增加
  `monitoring.alert-rule.catalog`，命令 `stx monitoring alert-rule catalog`，样例同名。

巡检条件校验、前端巡检编辑器、通知路由的 `rule_key` 下拉都只从这里取值。

## 3. 后端 · 巡检触发条件（R3）

### 3.1 模型（`internal/apps/diagnostics/auto_policy_models.go`）

- 删除条件码：`PROM_GC_FREQUENT`、`PROM_HEAP_RISING`、`PROM_HEAP_HIGH`、`PROM_CPU_HIGH`、`NODE_UNHEALTHY`。
- 删除类别：`prometheus`、`node_unhealthy`。保留 `java_error`、`error_rate`、`alert_firing`、`schedule`。
- `InspectionConditionItem` 增加：

```go
// RuleKeys 是告警触发条件要监听的规则键，取自告警规则目录，按精确相等匹配。
// RuleKeys lists the rule keys an alert-firing condition watches, taken from the alert rule catalog; exact match.
RuleKeys []string `json:"rule_keys,omitempty"`
```

- `ALERT_FIRING` 模板改为：`DefaultThreshold: 1`（至少 N 个不同对象），`DefaultWindowMinutes: 0`（至少持续触发 N 分钟，0 = 立即）。
  沿用已有 `threshold_override` / `window_minutes_override` 字段承载这两个含义，不新增字段。
  模板描述改成"所选告警规则触发时发起巡检；可要求持续时长与同时触发的对象数"。
- `ExtraKeywords` 只保留给 `java_error`（`matchJavaErrorCondition`，checker 297 行）和 `error_rate`（`errorSpikeBurstMatchesKeywords`），`alert_firing` 不再读取。

### 3.2 校验（`auto_policy_service.go` 创建 / 更新）

- 条件码必须在 `BuiltinConditionTemplates` 内，否则 400。已删除的码也走这个分支，不另做特判。
- `ALERT_FIRING`：`rule_keys` 去空、去重后至少 1 个，且每个都在告警规则目录内，否则 400。错误文案写明非法的 key。
- `threshold_override >= 1`，`window_minutes_override` 在 0..1440。

### 3.3 检查器（`auto_policy_checker.go`）

- `CheckPrometheusPolicies` 改名为 `CheckAlertFiringPolicies`，调用点 `auto_policy_runtime.go:74` 同步改。
  日志 / 错误前缀里的 "metric-signal" 改为 "alert-firing"。
- 只处理 `ALERT_FIRING`；循环里的 `switch` 整个删掉，换成：

```go
// matchAlertFiringCondition 只认规则键精确相等；持续时长按 FiringAt 计算，对象数按 alertTargetKey 去重。
// matchAlertFiringCondition matches rule keys exactly; duration uses FiringAt and targets are deduped by alertTargetKey.
func matchAlertFiringCondition(cond InspectionConditionItem, alerts []*monitoringapp.AlertInstance, now time.Time) (*monitoringapp.AlertInstance, int, bool)
```

  - 命中条件：`alert.Status == firing` 且 `alert.RuleKey ∈ cond.RuleKeys`，且 `now - FiringAt >= window`。
  - 对象去重键 `alertTargetKey`：`SourceRef.Hostname` → `SourceRef.TaskID`（>0 时）→ `SourceRef.Fingerprint` → `AlertID`，
    取第一个非空值（指标告警每个 instance 一条序列，指纹已经是按对象区分的）。
  - 去重后的对象数 `>= threshold` 才算命中。trigger reason 写成
    `<prefix>|ALERT_FIRING rules=<k1,k2> targets=<n> window=<m>m`。
- 删除：`matchMetricAlertCondition`、`metricAlertMatchesCondition`、`containsAnyMetricAlertAlias`、
  `matchNodeUnhealthyCondition`、`isNodeUnhealthyAlert`、`normalizeAlertNodeKey`、`alertMatchesKeywords`
  （只被告警 / 节点两个匹配器调用，java_error / error_rate 用的是各自的关键词逻辑）、`normalizeMetricAlertReasonKey`。
- 拉取告警时的 `PageSize: 200` 保留；列表仍只取 firing。
- 库里残留的旧条件码：检查器遇到未知码直接 `continue`，不报错、不打刷屏日志。

### 3.4 本地化（`internal/apps/diagnostics/localization.go:504`）

`localizeBuiltinConditionTemplates` 删掉旧码的翻译，补 `ALERT_FIRING` 新描述的 zh / en。

### 3.5 CLI（`internal/cmd/diagnostics.go`）

- `auto-policy create|update` 增加可重复的 `--alert-rule-key`。有值时生成 / 覆盖 `ALERT_FIRING` 条件的 `rule_keys`；
  只给了 `--condition ALERT_FIRING` 而没给 `--alert-rule-key`（且没有 `--request-file`）时报 usage 错误。
- `diagnosticsConditionBody`（289 行）同步调整。
- `--condition` 帮助文本列出当前合法码：`JAVA_OOM, JAVA_STACKOVERFLOW, JAVA_METASPACE, ERROR_SPIKE, ALERT_FIRING, SCHEDULED`。

## 4. 前端

### 4.1 告警中心

- `frontend/lib/services/monitoring/types.ts:152`：`AlertSourceType = 'platform_health' | 'metric' | 'job'`。
- `MonitoringAlertsCenter.tsx`：
  - 368-378 的标签函数、770-778 的筛选项换成三个新值；
  - 893 / 901 / 918 / 1211 / 1219 / 1222 行那些阶段专属展示（发生次数、自动关闭原因、暂停提醒）现在用
    `source_type === 'sync'` 判断。集群级作业规则改成 `platform_health` 后，这样判断会丢展示。改为按存储种类判断：
    `alert.alert_id.startsWith('sync:')`。抽一个 `isSyncEpisode(alert)` 工具函数，不要散落字符串。
- 通知路由表单如有 source_type 选择，同样换成三值。

### 4.2 巡检自动策略（`frontend/components/common/diagnostics/AutoPolicyConfigPanel.tsx`）

- 类别标签（201-216）只剩四类；删掉 `prometheus`、`node_unhealthy` 分支。
- `alert_firing` 条件的编辑区：
  - 关键词输入（993-1004 的 `extra_keywords`）换成**按类别分组的规则多选**，数据来自 `GET /api/v1/monitoring/alert-rules/catalog`；
  - 阈值输入的标签改成「至少 N 个对象同时触发」，窗口输入改成「至少持续触发 N 分钟（0 = 立即）」。
- 序列化（96-115）：`alert_firing` 输出 `rule_keys`、不输出 `extra_keywords`；`error_rate` 继续输出 `extra_keywords`。
- 加载已有策略时，丢掉模板列表里不存在的条件码。下次保存即为干净数据，无需提示。
- 遵循 `.trellis/spec/frontend/ui-conventions.md`：多选用项目已有组件，选中项用 chip 展示，不换行截断。

### 4.3 i18n（`frontend/lib/i18n/locales/zh.json` / `en.json`）

- `alerts.sourceTypes`：`platform_health: 平台健康 / Platform health`，`metric: 指标告警 / Metric alerts`，
  `job: 作业告警 / Job alerts`。删除旧三键。
- 巡检 `categories.prometheus`（4246 行，「指标信号」）、`node_unhealthy` 删除；`alert_firing` 文案改为「告警触发」。
- 术语（R4）：
  - 指 `AlertPolicy` 的文案统一为「告警策略 / Alert policy」：3132、3136、3510、3568、3570、3575-3578、3611-3625、
    3725、3857、3898 等处的「告警规则」改为「告警策略」；
  - 3112-3126 那组（按集群配置开关 / 阈值 / 窗口，对应 `AlertRule`）改为「检测规则 / Detection rules」；
  - `en.json` 对应键同步改。
- `frontend` 的 i18n 动态键测试（#146 新增的 `test(i18n)`）会校验键存在。删键后跑一遍，确认没有引用残留。

## 5. 文档与 spec

- `.trellis/spec/backend/` 中涉及告警来源 / 巡检条件的文档，`.trellis/spec/frontend/ui-conventions.md` 的告警相关段落，
  以及 `docs/内置监控说明.md`，改用 prd 名词表；补一段「类别按规则归属判定，存储种类只在内部使用」的约定。
- `internal/operation/registry.go:2077` 的 `source_type` 描述改为 `Alert category: platform_health, metric or job`，
  2175 行样例值改为 `platform_health`；`validate_test.go` 里操作总数随新增目录接口 +1。

## 6. 已知影响（不兼容，按用户决定不处理）

| 现象 | 原因 | 用户侧动作 |
|---|---|---|
| 历史投递记录 / 告警状态行的 `source_type` 显示旧值 | 不迁移 | 无，仅显示问题。前端标签函数对未知值回退显示原值即可 |
| 已有通知路由若设置了 `source_type`，升级后**不再匹配** | 路由存的是旧值 | 重新编辑保存该路由。release note 写明 |
| 外部 webhook 接收方若按 `source_type` 分流，需要改判断 | 负载值变化 | release note 写明新旧值对照 |
| 已有巡检自动策略里的 `PROM_*` / `NODE_UNHEALTHY` 条件**失效** | 条件码删除 | 重新配置为「告警触发」+ 规则。这些条件此前本就基本不触发 |
| CLI 脚本里用旧 `--source_type` 值会得到 400 | 不留别名 | 改用新值 |

release note 草稿放在 PR 描述的「破坏性变更」小节，按上表写。

## 7. 测试要点

- `alert_instance_service_test.go`：三类过滤，尤其 `platform_health` 同时包含本地事件和集群级 sync 阶段，
  `job` 不包含集群级阶段；旧值 400；指标告警 `rule_key` 来自 labels。
- `alert_policy_execution_service` 相关测试：集群级 sync 阶段的类别改成 `platform_health` 后，仍然走退避重试分支（`Kind` 判定）。
- `auto_policy_checker_test.go`：规则键精确匹配、持续时长边界（差 1 秒不触发）、多对象阈值、未知旧码跳过；
  删除所有别名匹配用例。
- 新增 `auto_policy_service_test.go`：校验的 400 分支。
- 告警规则目录：三类都有、无重复 key。
- 前端：`AutoPolicyConfigPanel` 序列化 / 加载丢弃旧码；告警中心 `isSyncEpisode`。
