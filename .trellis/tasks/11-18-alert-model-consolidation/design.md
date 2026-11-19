# 设计：告警模型收拢

## 1. 目标模型

```
告警
├── 集群级：一份内置规则目录（代码内定义）
│     每条规则 = 目录默认值  ←  全局配置（可选，一行）  ←  集群例外（可选，按集群）
│     通知 = 规则自定义 ｜ 沿用默认通知 ｜ 仅告警中心
└── 作业级：每任务一份配置（现有 sync_job 策略，不变）
```

判定机制（进程事件、内嵌 TSDB 规则、sync 阶段、可用性评估器）只是规则目录里每条规则的内部属性，不对用户暴露。

## 2. 规则目录（代码，单一来源）

新文件 `internal/apps/monitoring/cluster_rule_catalog.go`，取代：
- `rules/seatunnel-default-rules.yml` 中的 `seatunnel-default-alerts` 组（recording 组保留，供大盘与规则表达式复用）；
- `allAlertPolicyTemplateSummaries()` 中的 `platform_health` / `metrics_template` 模板；
- `defaultClusterRules()`（`monitoring_alert_rules` 种子）。

```go
type ClusterRuleDefinition struct {
    Key            string            // 稳定键，沿用现有 rule_key / template_key，便于迁移与 CLI 兼容
    Engine         clusterRuleEngine // event | tsdb | sync | availability（内部）
    Severity       AlertSeverity     // 目录默认值：全部 warning；用户可编辑（D9）
    DefaultEnabled bool
    Threshold      *ThresholdSpec    // nil 表示无阈值（如进程崩溃）
    DefaultFor     time.Duration     // 持续时间；事件类为 0
    ForEditable    bool
    Expr           func(cfg resolvedRuleConfig, clusterMatcher string) string // 仅 tsdb
}

type ThresholdSpec struct {
    Operator string  // 固定，如 ">"；不开放编辑
    Unit     string  // percent | count | ratio | workers
    Default  float64
    Min, Max float64
}
```

### 目录内容与键

| Key（沿用） | 名称 | Engine | 默认 | 阈值 / 持续 |
|---|---|---|---|---|
| `node_offline` | 节点离线 | availability | 开 | — / 2m |
| `master_unavailable` | Master 不可用 | availability | 开 | — / 1m |
| `worker_insufficient` | 可用 Worker 不足 | availability | 开 | 健康 worker 数 < N（默认 = 部署 worker 数，见 §5） / 2m |
| `agent_offline` | Agent 离线 | availability | 开 | — / 2m |
| `process_crashed` | 进程崩溃 | event | 开 | — |
| `process_restart_failed` | 进程重启失败 | event | 开 | — |
| `process_restart_limit_reached` | 达到重启上限 | event | 开 | — |
| `sync_status_unavailable` | 作业状态获取失败 | sync | 开 | — / 5m（沿用 300s） |
| `split_brain_risk` | 分区不安全 | tsdb | 开 | 采用内置表达式（clusterSafe==0 或 localMemberSafe==0） / 2m |
| `deadlocked_threads_detected` | 线程死锁 | tsdb | 开 | > 0 / 1m |
| `job_thread_pool_rejection_high` | 任务被拒绝 | tsdb | 开 | reject rate5m > 0 / 5m |
| `memory_usage_high` | JVM 堆内存高 | tsdb | 开 | > 85% / 10m |
| `fd_usage_high` | FD 使用率高 | tsdb | 开 | > 80% / 10m |
| `cpu_usage_high` | CPU 高 | tsdb | 关 | > 80% / 10m |
| `gc_time_high`（新键，原 SeaTunnelGCTimeHigh） | GC 耗时高 | tsdb | 关 | > 20% / 10m |
| `job_thread_pool_queue_backlog_high` | 线程池队列积压 | tsdb | 关 | > 100 / 5m |
| `coordinator_backlog_growing`（新键） | 积压持续增长 | tsdb | 关 | 队列 > 50 且提交>完成 / 10m（阈值 = 队列深度） |
| `failed_jobs_high` | 失败作业数过高 | tsdb | 关 | > 0 / 1m |
| `sync_event_channel_degraded` | 作业事件通道降级 | sync | 关 | 实时 |
| `cluster_restart_requested` | 集群重启请求 | event | 关 | — |
| `node_stop_requested` | 节点停止请求 | event | 关 | — |

阈值单位统一用用户可读的百分比（目录内部换算为 ratio 拼表达式），消除现模板 `0.8` 与内置 `85` 的单位不一致。

## 3. 持久化

不新增“策略”概念，只存**偏离默认值的配置**。三张新表（加入 `migrator.go` 的 `managedModels()`）：

```go
// monitoring_cluster_rule_configs：每条规则最多一行，全局配置
type ClusterRuleConfig struct {
    ID uint; RuleKey string `uniqueIndex`
    Enabled *bool; Threshold *float64; ForSeconds *int
    Severity *string             // nil 继承；取值 warning | critical
    NotifyMode string            // ""(沿用默认) | default | custom | none
    ChannelIDsJSON, ReceiverUserIDsJSON string
    CreatedAt, UpdatedAt time.Time
}

// monitoring_cluster_rule_overrides：集群例外
type ClusterRuleOverride struct {
    ID uint; RuleKey string `uniqueIndex:ux_rule_cluster`; ClusterID uint `uniqueIndex:ux_rule_cluster`
    Enabled *bool; Threshold *float64; ForSeconds *int
    Severity *string
    NotifyMode string; ChannelIDsJSON, ReceiverUserIDsJSON string
    CreatedAt, UpdatedAt time.Time
}

// monitoring_alert_notification_defaults：单行
type AlertNotificationDefaults struct {
    ID uint; ChannelIDsJSON, ReceiverUserIDsJSON string
    CooldownMinutes int; SendRecovery bool; UpdatedAt time.Time
}
```

字段为指针：`nil` 表示继承上一层。解析顺序：目录默认 → 全局配置 → 集群例外，逐字段覆盖。

`ResolveClusterRule(ctx, ruleKey, clusterID) resolvedRuleConfig` 是唯一读取入口，所有判定与投递都只调用它；带短 TTL 进程内缓存，写配置时失效并触发 TSDB 规则组 reload（复用 `MetricEvaluator.reloadCh`）。

## 4. 判定接线

| Engine | 现状入口 | 改为 |
|---|---|---|
| tsdb | YAML 内置组 + `buildManagedRuleGroups` 读策略 | `buildManagedRuleGroups` 从目录生成：每条启用规则一条全局 rule（`cluster_id!~"<例外集群>"`）+ 每个有效例外一条 rule（`cluster_id="<id>"`）；labels 带 `managed_by=stx`、`rule_key`。删除 YAML alerts 组 |
| event | `DispatchAlertPolicyEvent` 按 `LegacyRuleKey` 查策略 | 按事件 rule_key 调 `ResolveClusterRule`，未启用即丢弃 |
| sync（集群级） | `SyncAlertHub.clusterRule` 读 `monitoring_alert_rules` | 读 `ResolveClusterRule`（`ForSeconds` 即原 `WindowSeconds`） |
| availability | `node_offline` 在 `service.go` 内按 `NodeInfo` 判定；其余三条无实现 | 新 `availability_evaluator.go`，见 §5 |

托管 TSDB 告警的投递路径 `deliverManagedRemoteAlertPolicyNotifications` 改为按 `rule_key` label 解析规则与通知；`deliverRemoteAlertNotifications`（路由回落）删除。

## 5. 可用性评估器

每 `eval_interval`（默认 15s）对每个受管集群计算节点健康，内存中维护 pending 起点实现 `for` 语义（与 Prometheus 一致：一次不满足即清零）。STX 重启后的计时延续沿用现 `node_offline` episode 机制。

节点不健康原因（任一成立）：
- 现有 `evaluateNodeOfflineState` 的进程侧原因：`process_stopped` / `process_missing` / `node_error` / `host_visibility_lost`；
- 内嵌 TSDB 最新样本：`up{job="seatunnel_engine_http"}==0` → `metrics_endpoint_down`；`node_state==0` → `node_state_down`（经 `internal/pkg/promquery` 即时查询）。

**去重边界**：
- `host_offline`（`NodeInfo.IsOnline=false`，即 Agent 心跳超时）**不再**算节点离线原因，归入 `agent_offline`（按主机聚合，一台主机一条）。若同时 `up==0`，则两条都触发：Agent 离线 + 节点离线，原因不同，这是两件事。
- 同一节点多个原因只产生一条 `node_offline`，原因列表写入告警内容。

集群级：
- `master_unavailable`：集群内 role ∈ {master, master_worker} 的健康节点数 == 0。
- `worker_insufficient`：role ∈ {worker, master_worker}（separated 模式只计 worker）的健康节点数 < 阈值。阈值未配置时默认 = 当前部署的 worker 节点数（`Threshold=nil` 表示“等于部署数”）；用户可设固定数。部署数为 0 的集群跳过。

## 6. 通知投递与级别语义（D9）

级别即"是否送到平台外"：

- `warning`（默认）：告警只进平台告警中心，**不发外部通知**。
- `critical`：告警进告警中心并发往外部渠道。保存规则时校验：该规则（或其集群例外）的生效通知中**至少有一个启用的外部渠道**（webhook / 邮件 / 企微 / 钉钉 / 飞书），否则保存返回 400，错误提示引导去配"默认通知"或规则自定义渠道；邮件渠道还要求存在可达接收人（渠道收件人或配置接收人，复用抽出的邮件校验）。

通知来源解析（仅对 critical 生效）：
- `custom` → 规则自己的渠道与接收人；
- `""` / `default` → 默认通知；
- `none` → 等价于 warning（保留字段以兼容迁移，前端不暴露）。

由此产生不变式：**critical ⇔ 生效通知中必有外部渠道**。保存校验保证写入成立；渠道停用 / 删除不自动降级级别（停用是临时态，恢复后补发），但规则列表对"critical 且生效通知已无可用外部渠道"的规则显示警告标识（`alert` 提示 + 行内 badge），提醒用户补配或降级。

冷却与恢复通知取默认通知的值，仅在存在外部投递时才有意义。底层复用 `NotificationDelivery` 去重键、退避重试、渠道停用暂停 / 补发；`PolicyID` 字段不再写入（保留列）。

## ## 7. 范围（scope）

存储中的 `source_type` 不改。新增派生字段 `scope`：`job → job`，其余 → `cluster`。
- `AlertInstance` DTO、`AlertRuleCatalogItem` 增加 `scope`；
- 告警实例列表接收 `scope=cluster|job`，与旧 `source_type` 并存；
- 前端告警中心筛选只显示“全部 / 集群级 / 作业级”。

## 8. 接口与 CLI

新增（`/api/v1/monitoring`）：

| 方法 | 路径 | CLI |
|---|---|---|
| GET | `/cluster-rules` | `stx monitoring cluster-rule list` |
| GET | `/cluster-rules/:key` | `stx monitoring cluster-rule get <key>` |
| PUT | `/cluster-rules/:key` | `stx monitoring cluster-rule update <key> --request-file … --confirm` |
| PUT | `/cluster-rules/:key/clusters/:clusterId` | `stx monitoring cluster-rule override set <key> <cluster-id> …` |
| DELETE | `/cluster-rules/:key/clusters/:clusterId` | `stx monitoring cluster-rule override delete <key> <cluster-id> --confirm` |
| GET / PUT | `/alert-notification-defaults` | `stx monitoring notification-defaults get / update` |

`PUT` 带字段级 `reset`（如 `{"threshold": null}` 表示恢复继承），校验阈值范围、渠道存在且启用、邮件渠道有收件人（复用 `validateSyncJobPolicy` 中的邮件校验，抽为共享函数）。

移除：`/notification-routes*`、`/clusters/:id/rules*`、`/alert-policies/bootstrap`；`/alert-policies` 的 POST/PUT 只接受 `sync_job`，其余类型返回 400 并提示改用 `cluster-rules`；`promql` 字段从请求结构删除（未知字段忽略，不存储）。对应 CLI 子命令与 `internal/operation/registry.go` 登记同步删除 / 新增。

## 9. 前端（Arc 优先，直接替换旧界面）

遵循 D8 与 PR #163 的列表分层（页头 / 工具栏 / 表格卡片 / 覆盖层、server-driven）。监控模块内**Arc 组件优先**，旧的 shadcn `Tabs` / `Sheet` / `Select` / `Badge` 与 `StatPillsBar` 在本模块内不再使用。选型按 `.claude/skills/arc/SKILL.md` 的 Quick decisions；交付前跑 Arc `checklist.md`。

### 9.1 信息架构（页面合并与省略）

`/monitoring` 一个页面，Arc `tabs` 三个同级面板：**告警 / 规则 / 通知**。

| 旧页面 / 入口 | 去向 |
|---|---|
| `tab=alerts` 告警中心 | 告警 |
| `tab=policies` 策略中心“告警规则”段（模板建策略） | **省略**，由“规则 → 集群级”替代 |
| 策略中心中的 `sync_job` 策略列表 | 规则 → 作业级 |
| 策略中心“通知通道”段 | 通知 |
| 策略中心“投递记录”段 | **合并**到告警详情（该告警的投递）与渠道详情（该渠道最近投递）；全局投递列表省略，CLI `stx monitoring notification-delivery list` 保留 |
| `/monitoring/alerts`、`/rules`、`/history`、`/integrations` | 重定向到 `/monitoring?tab=alerts|rules|notify` |
| 旧深链 `tab=policies`、`tab=history`、`section=*` | 映射到 `rules` / `notify` |
| 外部入口：集群详情 `/monitoring?tab=alerts&cluster_id=<id>`（`ClusterDetail.tsx:2008/2035`）、仪表盘 / Dock / 诊断页 `/monitoring` | 保持可用：`cluster_id` 作为告警筛选初值生效 |

### 9.2 告警

- 指标行：Arc `metric-card` ×3（触发中、其中严重、近 24 小时新增），点击即应用对应筛选。
- 页头：沿用全局 `WorkspaceHeader` + `ModuleNavTabs`（告警中心 / 诊断中心，`reorderGroupId='monitoring-diagnostics'`），菜单「告警与诊断」下两个模块的切换入口不能丢；告警 / 规则 / 通知三个面板 tabs 放在页头 actions 位。
- 工具栏：`segmented-control`（全部 / 集群级 / 作业级）+ 行内 Arc `select`（状态、级别、集群，与相邻控件同高）+ `search-field`；有任一筛选时显示「清除筛选」文字按钮。筛选变化重置 page=1，参数同步 URL。不用 `filter-toolbar`：它自带面板边框与内边距，适合独立一行承载 chips，塞进同一行会变成胶囊套胶囊且高度不齐（用户评审否决）。
- 列表：`sortable-data-table`（server-driven：每页 rows 由接口返回，排序以受控 `defaultSort` + 请求参数实现，后端不支持的列 `sortable:false`）+ Arc `pagination`；状态 / 级别 / 范围用 `badge`（tone）。首屏 `skeleton` 占位、刷新保持高度（零塌陷）。
- 行操作：`dropdown-menu`（确认、静默 30 分钟、关闭）；结果在行内更新，不只靠 toast。
- 详情：Arc `drawer`。内容依次：标题与 badge → “命中规则：xxx（全局 / 集群例外），生效阈值 / 持续时间” → `timeline`（触发、通知、确认、恢复）→ 该告警的投递记录 → 现有排障 / 记忆面板。
- 空态：`empty-state`（无告警时一个动作：查看规则）。

### 9.3 规则

- `segmented-control`：集群级 / 作业级。
- 默认通知未配置时，顶部 Arc `alert`（tone warning）一行文案 + “去配置”按钮（切到通知 Tab）。
- **集群级**：`chip-group`（全部 / 已开启 / 已关闭 / 有例外）+ `sortable-data-table`（21 行，客户端排序即可）。列：规则名、级别 badge、启用 `switch`（行内即时生效，失败回滚并在行内提示）、阈值 / 持续时间、通知方式 badge、例外数。点击行打开编辑 `drawer`。
- **规则编辑 drawer**（长表单放 drawer）：
  - 启用 `switch`；阈值 `number-field`（单位后缀、上下限）；持续时间 `select`（1 / 2 / 5 / 10 / 15 / 30 分钟）。
  - 级别 `radio-cards` 二选一（D9）：**警告**（描述：只进入告警中心）/ **严重**（描述：同时发送外部通知）。选"严重"展开通知方式 `radio-cards`：沿用默认通知（显示其摘要）/ 自定义（展开 `multi-select` 渠道 + `multi-select` 接收人）；若两者都为空则保存被禁并给出内联提示，引导先去"通知"页配置默认通知。选"警告"不显示渠道选择。
  - 集群例外 `accordion`：例外列表 + “添加集群例外”（`combobox` 选集群）；每条例外可设开关、阈值、通知，未设字段显示“继承全局”；删除例外用 `confirm-morph`。
  - 底部：主按钮“保存”（按钮内 loading → 已保存），次按钮“恢复默认”（`confirm-morph`）。
- **作业级**：`sortable-data-table`（任务名、已选规则 badge、通知摘要、启用 `switch`），编辑打开 `drawer` 承载 `TaskAlertForm`。`TaskAlertForm` 同时用于任务设置页，本次将其内部控件换为 Arc（`switch`、`number-field`、`multi-select`、`checkbox`），保持 props 与保存接口不变，两处一起换新。

### 9.4 通知

- 默认通知：Arc `card`，`multi-select` 渠道、`multi-select` 接收人、`number-field` 冷却（分钟）、`switch` 恢复通知；保存在按钮内确认。
- 渠道：`sortable-data-table` + 新建 / 编辑 `drawer`（渠道表单按类型有多组字段，属长表单）；“测试”用 `action-button` 并在行内显示结果；删除用 `confirm-morph`。渠道详情 drawer 底部显示该渠道最近投递记录。

### 9.5 待安装 Arc 组件

已安装：`tabs`、`badge`、`switch`、`segmented-control`、`metric-card`、`radio-cards`、`empty-state`、`confirm-morph`、`chip-group`、`search-field`、`combobox`、`action-button`、`button`、`input`。

需补装（`cd frontend && pnpm dlx shadcn@latest add @uiarc/<id>`）：`drawer`、`select`、`multi-select`、`number-field`、`dropdown-menu`、`alert`、`pagination`、`sortable-data-table`、`filter-toolbar`、`skeleton`、`card`、`timeline`、`accordion`、`checkbox`、`tooltip`。装后按 PR #163 的受控 vendoring 约定核对 License 头与 `licenses/third_party_mit_files.txt`。

### 9.7 视觉规范（2026-11-19 改定：对齐 PR #163 拍板的「8–10px 桌面后台收紧版」）

本模块视觉以 `ui-conventions.md §18`（控制台控件统一）与
`11-18-unified-admin-console-lists/design.md §3.1 / §3.2 / §3.6` 为准，不再另行自创版式规则。要点与本模块映射：

- **容器级 Token 覆盖（不改 Arc 源码圆角）**：`/monitoring` 根容器 + 所有 Portal 表面（抽屉内容、弹层内容若可传 class）统一覆盖
  `--radius-control: 0.5rem`（8px）、`--radius-panel: 0.625rem`（10px）、`--control-height-md: 2rem`（32px）、`--control-height-sm: 1.75rem`（28px）。
  禁止再逐个组件手写 `h-*` / `rounded-*` / `shadow-*`。
- **防框框堆叠**：每个列表页只保留一层主卡片外框；筛选栏、搜索框、状态分段直接落页面画布，不外包带框工具条；
  卡片内部先间距分组、其次 1px `--border-subtle` 分隔线；相邻汇总指标（告警统计）合并单框 + 内分割线。
- **弹层收紧（vendored 本地改，文件头注明）**：`foundation.css :root` 新增 `--radius-popover: .625rem`；
  `DropdownMenu` / `Select` / `Combobox` 弹层外框用 `var(--radius-popover)`、内边距 4px、选项圆角 `calc(var(--radius-popover) - 4px)`、选项行高 32px、高亮项图标 `--foreground`。
- **选型（按 §18 表）**：状态筛选 `SegmentedControl` + Arc `Badge sm` 计数；少量选项 `Select`（标签可选，不显示必须 `aria-label`）；
  >10 项 / 可搜索用 `Combobox`；行内「更多」Arc `Button ghost iconOnly` + Arc `DropdownMenu`，≤2 个行内动作直接摆无框 ghost 按钮；
  单条不可撤销行内确认 `ConfirmMorph`（原版胶囊）；批量 / 破坏性确认 `ui/alert-dialog`；`Button` 全部 Arc，不加 `ActionButton`。
- **表格卡片**：按 §10 / 163 design §4.4 做 `TableCard`（`common/layout/`，含加载条、`TableSkeletonRows` 首屏、空 / 错误插槽、
  `border-t bg-muted/10` 分页底栏：左总数与每页条数，右 `ui/pagination`）。
- **详情宿主**：沿用本模块已实现的 Arc `Drawer`（贴边、附着侧直角）；内部为「先间距、后 `--border-subtle` 分隔行」的设置分组，
  底部固定操作栏；未保存拦截保留。
- **本地改登记**（升级 Arc 时 diff 合并）：`foundation.css` 的 `--radius-popover`；三种弹层样式；`Button.iconOnly`；
  `Select`/`Combobox` 可选标签与 `aria-label`；`chip-group` count（本模块不用可暂缓）。

### 9.6 回归（Egolite 矩阵，`/monitoring`）

| 面板 | 必测交互 |
|---|---|
| 告警 | 指标卡筛选；范围切换；状态 / 级别 / 集群下拉筛选与清除；搜索；分页；确认 / 静默 / 关闭；详情 drawer（命中规则、时间线、投递）；URL 深链恢复筛选 |
| 规则 · 集群级 | chip 筛选；行内开关；编辑阈值 / 持续时间并保存；三种通知模式；添加 / 删除集群例外；恢复默认；未配置默认通知时的提示与跳转 |
| 规则 · 作业级 | 列表；行内开关；drawer 中编辑任务告警并保存；任务设置页的同一表单行为一致 |
| 通知 | 保存默认通知；新建 / 编辑 / 测试 / 删除渠道；渠道详情中的投递记录 |
| 旧链接 | `/monitoring/rules|history|integrations|alerts` 与旧 `tab` / `section` 参数正确落到新面板 |

文案全部走 i18n（`frontend/lib/i18n/locales/zh.json` / `en.json`），遵循 Arc copy 规则（句式大小写、无 eyebrow、无破折号）。

## 10. 迁移（`SchemaFlag`：`alert_rule_catalog_v1`，事务内一次执行）

| 旧数据 | 去向 |
|---|---|
| `metrics_template` / `platform_health` 策略，`cluster_id` 为空 / all | 对应规则的全局配置：Enabled、阈值（ratio→percent 换算）、持续时间；渠道 / 接收人非空 → `NotifyMode=custom` | 
| 策略的 severity | 有启用渠道的策略 → critical；无渠道 → warning（满足 D9 不变式：critical ⇔ 必有外部渠道）；因无渠道被降为 warning 的记入迁移摘要 |
| 同上，指定集群 | 对应规则的集群例外 |
| 同一规则多条策略冲突 | 取最近更新的启用策略，其余记入迁移摘要日志 |
| `master_unavailable` 等三条无判定模板的策略 | 同上迁移；摘要中注明“升级后开始生效” |
| `monitoring_alert_rules` 中 Enabled / WindowSeconds 与种子默认不同的行 | 对应规则的集群例外 |
| `NotificationRoute`：无过滤条件或仅 source_type 过滤 | 渠道并入默认通知 |
| `NotificationRoute`：带 cluster / rule_key 过滤 | 写入对应规则的全局配置或集群例外 `custom` 渠道；severity 过滤无法表达，并入默认通知并记入摘要 |
| 默认通知仍为空且存在启用策略 | 取所有迁移策略渠道 / 接收人的并集作为默认通知，避免默认开启规则升级后无处可发 |
| `sync_job` 策略 | 不动 |

旧表与旧行**不删除**（代码不再读取），便于回滚与排查；迁移后将已迁移的非作业策略 `Enabled=false`。迁移摘要用 `log.Printf` 中英双语输出（参考 `legacy_alerts_migration.go`）。

## 11. 兼容与回滚

- 回滚到旧版本：旧表数据完整（仅非作业策略被置为停用），需手动重新启用；在迁移说明中写明。
- 升级行为变化（写入 `docs/release-notes/unreleased.md` 与 `docs/内置监控说明.md`）：
  - CPU 高、GC 耗时高、线程池队列积压、积压持续增长改为默认关闭；
  - Master 不可用、可用 Worker 不足、Agent 离线开始生效；
  - 节点离线不再包含“Agent 心跳超时”这一原因，改由 Agent 离线承担；
  - 通知路由移除，渠道并入默认通知或规则自定义。

## 12. 主要取舍

- 规则目录放在代码而非数据库：规则的判定逻辑本就在代码里，放数据库只会出现“有配置无实现”（正是当前三条模板的问题）。
- 只存偏离值：升级时调整目录默认值能自动惠及未改动的用户。
- 冷却 / 恢复只在默认通知上配置：按规则配置冷却的需求未出现，避免编辑抽屉过重；以后需要可在 `ClusterRuleConfig` 增加可空字段，不破坏模型。
- 级别开放编辑但语义收敛为"是否外发"：warning = 仅告警中心，critical = 必须外发（保存校验 + 失效渠道警告标识），避免"标了严重但没人收到"的假严重。
