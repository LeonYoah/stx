# 告警语义统一

随 PR #146（移除外部告警接入口与外部监控栈兼容面）一起落地。#146 拆掉了外部接入，但告警来源、告警规则、巡检三处
仍各说各话；本任务把它们收敛到同一套名词。

## 已确认事实（2026-11-17，基于 #146 head `8321f84d` 复核）

- **告警来源三类，但名字与实际不符**：`AlertSourceType` = `local_process_event` / `remote_alertmanager` / `sync`
  （`internal/apps/monitoring/model.go:176-192`），界面文案「本地进程事件 / 指标告警 / 作业告警」。
  - `local_process_event` 实际覆盖节点离线、进程崩溃 / 重启失败、集群重启、节点停止等平台健康事件，不只是"进程"。
  - `remote_alertmanager` 现在全部由内嵌评估器产生（`metric_evaluator.go`），已无"远端 Alertmanager"。
  - `sync` 混了两种东西：任务级作业告警（`syncalert.IsTaskRule`），以及集群级的「作业状态获取失败
    `sync_status_unavailable`」「事件通道降级 `sync_event_channel_degraded`」——后两条在策略中心归在
    **平台健康**构建器（`policy_center_service.go:280-297`），落库却是 sync 阶段（`sync_alerts.go:219`），
    告警中心按「作业告警」显示。同一条规则在两处属于不同类别。
- **规则 / 策略混用**：界面里同一个"告警策略"实体（`AlertPolicy`：模板 + 范围 + 级别 + 渠道）有时叫「告警规则」
  （`zh.json` 3132-3136、3510、3568-3625、3857、3898），有时叫「告警策略」（3148-3190、3349-3372）；而真正的
  「告警规则」`AlertRule`（按集群的检测开关 / 阈值 / 窗口，`zh.json` 3112-3126）也叫「告警规则」。
- **巡检"指标信号"是假分类**：巡检自动策略 `CheckPrometheusPolicies`（`internal/apps/diagnostics/auto_policy_checker.go:136`）
  不查任何指标，只是扫当前 firing 告警，再用英文别名对告警的 RuleKey / 名称 / 摘要 / 描述做子串匹配
  （`metricAlertMatchesCondition`，同文件 641-662）：
  - 指标告警的 `RuleKey` 为空（`alert_instance_service.go:676` 未赋值，`template_key` 只在 `labels_json`），
    名称是用户起的策略名，摘要 / 描述是中文模板 → `PROM_CPU_HIGH` / `PROM_HEAP_HIGH` 基本匹配不上；
  - `PROM_GC_FREQUENT` / `PROM_HEAP_RISING` 在告警模板里没有对应规则 → 永远不触发；
  - 这几个条件在界面上可填的阈值 / 窗口（如 GC 默认 10 / 5 分钟）完全不被读取；
  - `NODE_UNHEALTHY` 同样靠 `node_offline` / `node offline` 等别名子串识别；
  - `ALERT_FIRING` 不填关键词时，集群内任意 firing 告警都会触发巡检。
- **巡检条件存储**：`InspectionAutoPolicy.Conditions` 是 JSON 列（`InspectionConditionItems`），条目字段
  `template_code / enabled / threshold_override / window_minutes_override / cron_expr_override / extra_keywords`。
- **CLI**：`stx monitoring alert list --source_type`（`internal/operation/registry.go:2077`，样例值 2174）；
  `stx diagnostics auto-policy create|update --condition <CODE>`（`internal/cmd/diagnostics.go:201/261`）。

## Locked Decisions（用户确认）

1. **在 #146 内完成**，不另开 PR。
2. **破坏性变更，不做旧数据兼容**：不写数据迁移、API 不接受旧枚举值作别名。历史行保留原值即可（见 design「已知影响」）。
3. **阈值只在告警策略里定义一次**：巡检、通知只消费告警，不再自带指标阈值或文本匹配。
4. 主机指标（`stx_host_*`）告警模板**不在本任务**，后续单独补到「指标」类模板。

## 统一后的名词表

| 层 | 中文 | 英文 / 代码 | 说明 |
|---|---|---|---|
| 告警类别 | 平台健康 | `platform_health` | 节点 / 进程 / 集群事件 + 集群级作业通道规则 |
| 告警类别 | 指标告警 | `metric` | 内嵌评估器按指标模板产生 |
| 告警类别 | 作业告警 | `job` | 同步任务级规则，在任务设置里配置 |
| 实体 | 告警策略 | `AlertPolicy` | 规则模板 + 范围 + 级别 + 冷却 + 渠道；用户在「告警策略」页配置 |
| 实体 | 检测规则 | `AlertRule` | 按集群的内置检测开关 / 阈值 / 窗口（原「告警规则」页） |
| 标识 | 规则键 | `rule_key` | 一条告警"检测的是什么"；策略模板 key、作业规则 key、平台事件 key 共用一个命名空间 |
| 响应 | 巡检触发条件 | `InspectionConditionItem` | 只有四类：告警触发 / Java 致命错误 / 错误突增 / 定时 |

界面、API、CLI、文档、spec 一律使用上表名词；用户可见文案不再出现 "Prometheus"、"Alertmanager"、"本地进程事件"。

## Requirements

### R1 告警类别

- `source_type` 取值改为 `platform_health | metric | job`，告警中心筛选、告警列表 / 详情、通知路由、投递记录、
  webhook 负载、CLI 全部使用新值。
- 类别按**规则归属**判定，不按存储表判定：集群级作业规则（`syncalert.ClusterRules()`）的阶段归 `platform_health`，
  任务级规则的阶段归 `job`。
- 告警 ID（`local:event:N` / `remote:<fp>:<ts>` / `sync:episode:N`）格式不变。

### R2 规则键

- 每条告警实例都必须带 `rule_key`：指标告警取策略的 `template_key`。
- 新增只读接口「告警规则目录」，按类别列出全部可选 `rule_key` 与显示名，作为巡检条件、通知路由 `rule_key`、CLI 的唯一来源。

### R3 巡检触发条件

- 条件类别收敛为 `alert_firing`（告警触发）/ `java_error` / `error_rate` / `schedule`；删除 `prometheus`、`node_unhealthy`
  两类及 `PROM_*`、`NODE_UNHEALTHY` 四 + 一个条件码。
- `ALERT_FIRING` 必须选择至少一个 `rule_key`，按**精确相等**匹配告警的 `rule_key`，不再做关键词 / 别名子串匹配。
- `ALERT_FIRING` 支持「至少持续触发 N 分钟」与「至少 N 个不同对象同时触发」，覆盖原 `NODE_UNHEALTHY` 语义。

### R4 术语

- 「告警策略」统一指 `AlertPolicy`；「检测规则」指 `AlertRule`；按名词表改 i18n、文档与 spec。

## Acceptance Criteria

- [ ] 告警中心来源筛选显示「平台健康 / 指标告警 / 作业告警」，`?source_type=platform_health` 能查到节点离线告警与
      「作业状态获取失败」告警；`?source_type=job` 只返回任务级作业告警；旧值 `local_process_event` 返回 400。
- [ ] 指标告警实例的 `rule_key` 等于其策略的 `template_key`（如 `cpu_usage_high`）。
- [ ] 告警规则目录接口返回三类规则，CLI `stx monitoring alert-rule catalog` 可用。
- [ ] 巡检自动策略配置「告警触发：`cpu_usage_high`，持续 ≥5 分钟」，在 CPU 指标告警 firing 满 5 分钟后触发巡检；
      不满 5 分钟、或 firing 的是其他规则时不触发。
- [ ] 「告警触发：`node_offline`，≥2 个对象」仅在两个不同节点同时离线时触发。
- [ ] 创建 / 更新巡检策略时，`ALERT_FIRING` 未选 `rule_key`、选了目录外的 `rule_key`、或使用已删除条件码，均返回 400。
- [ ] 代码与用户可见文案中不再有 `metricAlertMatchesCondition`、`isNodeUnhealthyAlert`、别名表、`CheckPrometheusPolicies`；
      `zh.json` / `en.json` 不再出现"本地进程事件""Prometheus"作为告警或巡检分类名。
- [ ] 同一个 `AlertPolicy` 实体在所有页面统一叫「告警策略」。

## Out of Scope

- 主机指标告警模板（磁盘 / inode / CPU / 内存）。
- 旧数据迁移、旧枚举值别名。
- 告警 ID 格式、告警存储表结构调整（除巡检条件 JSON 新增 `rule_keys` 字段外）。
