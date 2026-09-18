# Alerting & Notification Technical Plan

## 1. Objective

在不推翻现有 `monitor` / `monitoring` 基础的前提下，把 STX 的告警与通知能力从：

- 远程可观测性接入 MVP
- 告警/通知配置骨架

演进为：

- **统一告警中心**（本地告警 + 远程告警同一视图）
- **最小可用通知闭环**（能路由、能发送、能查历史、能测试发送）
- **后续可平滑扩展到 Doris Manager 类能力**（恢复通知、策略、导入导出、PromQL 托管等）

---

## 2. Design Principles

### 2.1 Recommended direction

采用：**Alertmanager-first + Unified Alert Domain**

- 监控/规则执行底座优先复用 Prometheus / Alertmanager / Grafana；
- STX 负责：
  - 受管集群目标发现；
  - 本地关键事件告警归一化；
  - 告警聚合与统一展示；
  - 通知渠道、路由与投递记录；
  - 后续的策略和产品化能力。

### 2.2 Incremental migration, not rewrite

不建议一次性重写为全新系统，应采用增量演进：

1. 新增统一模型与统一 API；
2. 保持现有 `/monitoring/alerts` 和 `/monitoring/remote-alerts` 暂时兼容；
3. 前端逐步切换到 unified endpoint；
4. 再补通知路由与 delivery worker。

### 2.3 Split lifecycle and handling state

当前最大问题之一是：

- 本地告警 `status` = `firing | acknowledged | silenced`
- 远程告警 `status` = `firing | resolved`

语义冲突。

**新的统一模型必须拆分成两类状态：**

- `lifecycle_status`: `firing | resolved`
- `handling_status`: `pending | acknowledged | silenced`

这样才可统一展示与统计。

---

## 3. Target Capability Map

### 3.1 Phase 0 target

- 统一 Alert DTO / API / UI 口径
- 本地 / 远程告警都能在同一列表中展示
- ack / silence 统一作用于 AlertInstance
- Dashboard / Health / Alert Center 统计口径统一

### 3.2 Phase 1 target

- Notification channel 不再只是 CRUD
- 支持 Webhook / WeCom / DingTalk / Feishu 实际投递
- 支持 test send
- 支持 rule/severity/cluster 到 channel 的绑定
- 支持 delivery log

### 3.3 Phase 2 target

- 现有 threshold/window 真正生效
- 扩展更多内建规则
- 支持恢复通知

### 3.4 Phase 3 target

- Contact Point / Receiver Group
- Notification Policy / Route Tree
- 模板、导入导出、静默计划、历史清理

---

## 4. Unified Domain Model

## 4.1 New normalized concepts

### AlertSourceType

```txt
local_process_event
remote_alertmanager
future_task
future_capacity
future_inspection
```

### AlertInstance

统一视图对象，不一定要求第一阶段就单独建一张实体表；
第一阶段可以先做 **read model + state overlay**。

字段建议：

- `id`: string
- `source_type`: enum
- `source_key`: string
- `cluster_id`: string
- `cluster_name`: string
- `severity`: string
- `alert_name`: string
- `rule_key`: string
- `summary`: string
- `description`: string
- `labels`: map[string]string
- `annotations`: map[string]string
- `lifecycle_status`: `firing | resolved`
- `handling_status`: `pending | acknowledged | silenced`
- `firing_at`: time
- `resolved_at`: time?
- `last_seen_at`: time
- `acknowledged_by/at`
- `silenced_by/until`
- `source_ref`: json

### AlertStateOverlay

统一保存“人工处理状态”，与原始告警数据解耦。

#### Why

- 本地告警和远程告警都需要 ack/silence
- 不能把 handling 状态硬塞进 remote alert 原始记录
- 不能继续沿用只支持 event_id 的 `AlertEventState`

---

## 5. Data Model Proposal

## 5.1 Phase 0 schema changes

### Table A: `monitoring_alert_states`

> 新表。用于替代只支持本地事件的 `monitoring_alert_event_states`。

字段建议：

- `id` bigint pk
- `source_type` varchar(32) not null
- `source_key` varchar(255) not null unique
- `cluster_id` varchar(64) null
- `handling_status` varchar(20) not null default `pending`
- `acknowledged_by` varchar(100) null
- `acknowledged_at` datetime null
- `silenced_by` varchar(100) null
- `silenced_until` datetime null
- `note` text null
- `created_at`
- `updated_at`

#### source_key encoding

- local event: `local:event:<event_id>`
- remote alert: `remote:<fingerprint>:<starts_at>`

#### Migration strategy

- V1：保留 `monitoring_alert_event_states` 不删；
- 启动时迁移旧本地状态到新表；
- 服务优先读新表；
- 等稳定后再考虑弃用旧表。

### Table B: `monitoring_alert_snapshots` (optional, Phase 0b)

第一阶段**可不建**，先由服务层动态拼装 unified list。

只有当以下问题明显时再引入：

- 联表 / 合并查询性能差；
- 多来源统计逻辑太分散；
- 需要高频筛选和全文检索。

当前建议：**先不建**，避免过早复杂化。

---

## 5.2 Phase 1 schema changes

### Table C: `monitoring_notification_routes`

MVP 不直接做复杂 route tree，先做简单绑定表。

字段建议：

- `id` bigint pk
- `name` varchar(120) not null
- `enabled` bool default true
- `source_type` varchar(32) null
- `cluster_id` varchar(64) null
- `severity` varchar(20) null
- `rule_key` varchar(80) null
- `channel_id` bigint not null
- `send_resolved` bool default true
- `mute_if_acknowledged` bool default true
- `mute_if_silenced` bool default true
- `created_at`
- `updated_at`

#### Matching semantics

一条 route 表示：

> 当告警满足 `source_type + cluster_id + severity + rule_key` 条件时，投递到指定 channel。

#### Why not tree first

- 当前系统尚未进入多层级复杂路由阶段；
- 先做线性匹配规则足够完成 MVP；
- 后续 route tree 可兼容扩展。

### Table D: `monitoring_notification_deliveries`

字段建议：

- `id` bigint pk
- `alert_id` varchar(255) not null
- `source_type` varchar(32) not null
- `source_key` varchar(255) not null
- `channel_id` bigint not null
- `event_type` varchar(20) not null
  - `firing`
  - `resolved`
  - `test`
- `status` varchar(20) not null
  - `pending`
  - `sending`
  - `sent`
  - `failed`
  - `retrying`
  - `canceled`
- `attempt_count` int default 0
- `last_error` text null
- `request_payload` text null
- `response_status_code` int null
- `response_body_excerpt` text null
- `scheduled_at` datetime null
- `sent_at` datetime null
- `created_at`
- `updated_at`

#### Unique key suggestion

避免重复发送：

- unique(`source_key`, `channel_id`, `event_type`)

对于同一告警同一渠道：
- firing 只发一次
- resolved 只发一次

### Table E: evolve `monitoring_notification_channels`

当前字段：
- `name`
- `type`
- `enabled`
- `endpoint`
- `secret`
- `description`

建议扩展为：

- `config_json` text
- `secret_json` text or encrypted blob
- `last_test_status` varchar(20)
- `last_test_at` datetime
- `last_error` text

#### Compatibility

- Phase 1 保留 `endpoint` / `secret` 兼容 webhook 类 sender；
- 新增 `config_json` 用于未来 email / 参数化 webhook / 签名策略；
- 前端不再回显真实 secret，只回显是否已配置。

---

## 6. Backend Architecture

## 6.1 New backend modules

建议在 `internal/apps/monitoring/` 下新增：

- `unified_alert_service.go`
- `alert_state_service.go`
- `notification_route_service.go`
- `notification_delivery_service.go`
- `notification_dispatcher.go`
- `delivery_worker.go`
- `sender_webhook.go`
- `sender_im.go`
- `sender_email.go`（Phase 2）

## 6.2 Unified alert read path

### Inputs

- local: `monitor.ProcessEvent`
- remote: `RemoteAlertRecord`
- overlay state: `monitoring_alert_states`

### Output

- `UnifiedAlertListData`
- `UnifiedAlertStats`
- `UnifiedAlertItem[]`

### Merge rule

#### local event -> unified alert

- `source_type = local_process_event`
- `source_key = local:event:<event_id>`
- `lifecycle_status = firing`
  - Phase 0 对本地事件不做自动 resolved，保持 firing-only
- `handling_status` 从 overlay state 推导

#### remote alert -> unified alert

- `source_type = remote_alertmanager`
- `source_key = remote:<fingerprint>:<starts_at>`
- `lifecycle_status` = firing/resolved
- `handling_status` 从 overlay state 推导

### Sorting

排序建议：

1. firing before resolved
2. critical before warning
3. unsilenced before silenced
4. latest `last_seen_at` desc

---

## 6.3 Notification flow

### Producer

#### remote alert

`HandleAlertmanagerWebhook()` 写入 `RemoteAlertRecord` 后：

1. 归一化出 unified alert envelope
2. 匹配 route
3. 为每个命中的 channel 创建 `notification_deliveries`（pending）

#### local alert

第一阶段不建议强耦合 monitor 事件写入链路，采用 **scanner + outbox** 更稳：

- 后台 worker 每 30s 扫描最近未处理的 critical process events
- 归一化后匹配 route
- 通过 unique key 去重创建 delivery

#### Why scanner first

- 减少对 `internal/apps/monitor` 的侵入性改动；
- 本地事件已有持久化表，扫描成本可控；
- 用 delivery unique key 可以天然防重复。

### Dispatcher worker

循环流程：

1. 拉取 `pending/retrying` deliveries
2. 按 channel type 选择 sender
3. 执行发送
4. 记录 request/response/last_error
5. 更新为 `sent` 或 `retrying/failed`

### Retry policy

MVP 建议：

- 最多 3 次
- 指数退避：1m / 5m / 15m
- 失败后标记 `failed`

---

## 7. API Proposal

## 7.1 Phase 0 new unified alert APIs

### `GET /api/v1/monitoring/alert-instances`

查询统一告警列表。

#### Query params

- `source_type?`
- `cluster_id?`
- `severity?`
- `lifecycle_status?`
- `handling_status?`
- `keyword?`
- `start_time?`
- `end_time?`
- `page?`
- `page_size?`

#### Response core fields

- `generated_at`
- `page`
- `page_size`
- `total`
- `stats`
  - `firing`
  - `resolved`
  - `pending`
  - `acknowledged`
  - `silenced`
- `alerts[]`

### `POST /api/v1/monitoring/alert-instances/:id/ack`

统一确认告警。

### `POST /api/v1/monitoring/alert-instances/:id/silence`

统一静默告警。

### `POST /api/v1/monitoring/alert-instances/:id/unsilence` (optional)

建议一并设计，哪怕 Phase 0.1 再做。

### Compatibility APIs kept temporarily

- `GET /api/v1/monitoring/alerts`
- `GET /api/v1/monitoring/remote-alerts`
- `POST /api/v1/monitoring/alerts/:eventId/ack`
- `POST /api/v1/monitoring/alerts/:eventId/silence`

这些在前端全部切到 unified API 后再考虑废弃。

---

## 7.2 Phase 1 notification APIs

### Channel APIs

#### Keep existing

- `GET /notification-channels`
- `POST /notification-channels`
- `PUT /notification-channels/:id`
- `DELETE /notification-channels/:id`

#### Add

- `POST /notification-channels/:id/test`
- `GET /notification-channels/:id/deliveries`

### Route APIs

新增：

- `GET /notification-routes`
- `POST /notification-routes`
- `PUT /notification-routes/:id`
- `DELETE /notification-routes/:id`

### Delivery APIs

新增：

- `GET /notification-deliveries`
- `GET /notification-deliveries/:id`
- `POST /notification-deliveries/:id/retry` (optional)

### Suggested request model for route

```json
{
  "name": "critical cluster 1 alerts to feishu",
  "enabled": true,
  "source_type": "remote_alertmanager",
  "cluster_id": "1",
  "severity": "critical",
  "rule_key": "process_crashed",
  "channel_id": 12,
  "send_resolved": true,
  "mute_if_acknowledged": true,
  "mute_if_silenced": true
}
```

---

## 8. Frontend Proposal

## 8.1 Monitoring Center IA refactor

当前 tabs：
- Alerts
- Rules
- Integrations

建议演进为：
- Alerts
- Rules
- Notifications
- History（Phase 1）

其中：
- `Notifications` = channels + routes + stack status
- `History` = delivery log

## 8.2 Alerts page refactor

当前 `MonitoringAlertsCenter.tsx` 只展示 remote alerts。

### 改造目标

改成统一告警列表：

- 数据源切到 `getAlertInstancesSafe()`
- 展示字段：
  - cluster
  - alert name
  - source type
  - severity
  - lifecycle status
  - handling status
  - summary
  - first seen / last seen
- action：
  - ack
  - silence
  - unsilence（可后补）

### UI notes

建议双 badge：
- 红/灰：firing/resolved
- 蓝/黄：pending/acknowledged/silenced

## 8.3 Rules page refactor

当前规则页问题：
- 只能编辑 severity/enabled/threshold/window
- rule_name / description 后端支持但前端没开
- threshold/window 可能未生效

### Phase 0 change

- 先补 rule_name / description 编辑能力
- 标记哪些规则是 `built-in`
- 若 threshold/window 尚未生效，UI 加轻提示，避免误导

### Phase 2 change

- 支持新增有限类型内建规则
- 支持 send_resolved 开关（若规则层需要）

## 8.4 Notifications page split

当前 `MonitoringIntegrationsPanel.tsx` 只做 channel CRUD。

建议拆为 3 个 card / section：

### Section A: Stack Status

使用已有 `getIntegrationStatusSafe()`：
- Prometheus
- Alertmanager
- Grafana
- seatunnel_metrics

### Section B: Channels

- list/create/update/delete
- enabled toggle
- test send
- last test result

### Section C: Routes

- route 列表
- 新建 route
- 编辑 route
- 删除 route

## 8.5 Delivery history page

新增 `MonitoringNotificationHistoryPanel.tsx`

字段建议：
- created_at
- alert_name
- cluster
- channel
- event_type
- status
- attempt_count
- last_error
- sent_at

支持过滤：
- channel
- status
- event_type
- cluster
- time range

---

## 9. Detailed Implementation Sequence

## 9.1 Milestone A — Unified Alert Domain (recommended first PR set)

### Backend

1. 新增 `monitoring_alert_states`
2. 新增 unified alert DTO / service / handler
3. 将 local + remote merge 到 `GET /alert-instances`
4. 新增统一 ack / silence API
5. 保持旧 API 不删

### Frontend

1. 新增 `UnifiedAlert*` types
2. 新增 `getAlertInstances()` / `ackAlertInstance()` / `silenceAlertInstance()`
3. 重写 `MonitoringAlertsCenter.tsx` 使用 unified endpoint
4. 增加 source/lifecycle/handling filters

### Acceptance

- 本地 critical event 能出现在 Alerts Center
- remote alert 能出现在 Alerts Center
- 两类告警都能 ack / silence
- 页面统计与后端统一口径

## 9.2 Milestone B — Notification delivery MVP

### Backend

1. 新增 route / delivery 表
2. 新增 route CRUD API
3. 新增 delivery query API
4. 实现 sender：
   - webhook
   - wecom/dingtalk/feishu（可共用 webhook sender with formatter）
5. 实现 delivery worker
6. 实现 remote webhook producer
7. 实现 local event scanner producer
8. 实现 channel test API

### Frontend

1. Integrations 页增加 stack status
2. Notifications 页支持 route CRUD
3. Channels 页支持 test send
4. 新增 delivery history panel

### Acceptance

- 远程 firing 告警触发后可命中 channel 并生成 sent/failed delivery
- 本地 critical event 可命中 channel 并生成 sent/failed delivery
- test send 可用
- failed delivery 可查错误原因

## 9.3 Milestone C — Rule engine closure

### Backend

1. 让 threshold/window 真正参与本地告警判定
2. 增加更多 built-in rules
3. resolved notification support
4. 统一 route match 对 resolved/firing 生效

### Frontend

1. Rules 页去掉“看起来能配但实际不生效”的误导
2. 增加规则能力提示和 validation

---

## 10. Code Change Map

## 10.1 Backend files likely to change

### Existing files

- `internal/apps/monitoring/entity.go`
- `internal/apps/monitoring/model.go`
- `internal/apps/monitoring/repository.go`
- `internal/apps/monitoring/service.go`
- `internal/apps/monitoring/remote_integration.go`
- `internal/router/router.go`
- `internal/db/migrator/migrator.go`

### New files suggested

- `internal/apps/monitoring/unified_alert_service.go`
- `internal/apps/monitoring/unified_alert_handler.go` (or fold into handler)
- `internal/apps/monitoring/notification_route_service.go`
- `internal/apps/monitoring/notification_dispatcher.go`
- `internal/apps/monitoring/delivery_worker.go`
- `internal/apps/monitoring/sender_webhook.go`
- `internal/apps/monitoring/sender_im.go`

### Cross-domain touchpoints

- `internal/apps/monitor/service.go`
  - 若未来从 scanner 改为事件驱动，这里要增加 hook
- `internal/config/model.go`
  - 可能新增 dispatcher worker interval / retry policy config

## 10.2 Frontend files likely to change

### Existing files

- `frontend/lib/services/monitoring/types.ts`
- `frontend/lib/services/monitoring/monitoring.service.ts`
- `frontend/components/common/monitoring/MonitoringAlertsCenter.tsx`
- `frontend/components/common/monitoring/MonitoringRulesPanel.tsx`
- `frontend/components/common/monitoring/MonitoringIntegrationsPanel.tsx`
- `frontend/components/common/monitoring/MonitoringCenterWorkspace.tsx`

### New files suggested

- `frontend/components/common/monitoring/MonitoringNotificationRoutesPanel.tsx`
- `frontend/components/common/monitoring/MonitoringNotificationHistoryPanel.tsx`
- `frontend/components/common/monitoring/MonitoringStackStatusCard.tsx`

---

## 11. Testing Strategy

## 11.1 Backend tests

### Unit tests

- unified alert normalization
- lifecycle/handling state resolution
- route matcher
- delivery dedup
- sender payload formatters
- retry scheduling

### Integration tests

- Alertmanager webhook -> remote record -> route match -> delivery created
- local process event -> scanner -> delivery created
- ack/silence on remote alert
- ack/silence on local alert
- test send API

### Regression tests

- existing remote observability smoke remains green
- observability enabled true/false path unchanged

## 11.2 Frontend tests / checks

- unified alert filters work
- ack/silence refreshes item state
- route create/update/delete happy path
- test send toast + state refresh
- delivery history filters and pagination

---

## 12. Rollout / Migration Plan

## Step 1

只上线 unified alert read path，不启用新通知逻辑。

## Step 2

上线 alert state overlay 与 unified ack/silence。

## Step 3

上线 notification route + delivery tables，但先只打开 test send。

## Step 4

开启 remote alert delivery。

## Step 5

开启 local scanner delivery。

## Step 6

前端切到 unified alert UI，并补 Notifications / History 页面。

## Step 7

评估旧 API / 旧状态表是否废弃。

---

## 13. Risks and Mitigations

### Risk 1: 双模型迁移期间口径混乱

**Mitigation**
- 新旧 API 并存但前端只切一处；
- 统计口径明确写到 DTO 和 UI 文案中。

### Risk 2: 本地事件扫描重复投递

**Mitigation**
- `notification_deliveries` 使用 unique(source_key, channel_id, event_type)
- worker 幂等处理

### Risk 3: webhook / IM secret 暴露

**Mitigation**
- Secret 不再原样返回前端
- 后端做脱敏 DTO
- 后续补加密存储

### Risk 4: 当前 rule 字段看起来可配但不生效

**Mitigation**
- 在 Phase 0 UI 明示“仅部分规则参数已生效”或临时收敛字段
- Phase 2 前不要过度宣传规则能力

### Risk 5: 过早做复杂 route tree

**Mitigation**
- 先做线性 route binding
- 有实际复杂度需求后再升级为树形 policy

---

## 14. Recommended Task Breakdown

### PR-1: Unified alert state + unified alert query

- DB: `monitoring_alert_states`
- API: `GET /alert-instances`
- API: `POST /alert-instances/:id/ack`
- API: `POST /alert-instances/:id/silence`
- FE: Alerts Center switched to unified API

### PR-2: Notification routes + delivery schema + test send

- DB: `monitoring_notification_routes`
- DB: `monitoring_notification_deliveries`
- API: route CRUD
- API: channel test send
- FE: Notifications page route section + test send

### PR-3: Delivery worker + remote delivery path

- webhook/im senders
- delivery worker
- remote webhook dispatch
- FE: delivery history

### PR-4: Local scanner delivery + rule closure

- local critical event scanner
- threshold/window effective logic
- more built-in rules

---

## 15. Recommendation Summary

### What to do first

**最先做 PR-1。**

因为如果不先统一 alert domain：
- 页面继续割裂；
- 通知 route 无法定义在统一对象之上；
- delivery 历史也会陷入“到底是本地告警还是远程告警”的建模混乱。

### What not to do first

不要第一步就做：
- PromQL 编辑器
- 树形通知策略
- Email 完整配置中心
- Alertmanager silence 双向同步

这些都应该放在统一 alert domain 与 delivery MVP 之后。

---

## 16. External references

- Doris 集群告警：<https://docs.selectdb.com/enterprise/management-guide/monitor-and-alerting/doris-alerting/>
- Doris 集群监控：<https://docs.selectdb.com/docs/enterprise/doris-manager-guide/monitor-and-alerting/doris-monitor>
- Enterprise Manager Release Notes：<https://docs.selectdb.com/enterprise/release-notes/enterprisemanager/>

### Reference takeaways used in this plan

从 SelectDB 公开资料可确认：
- Doris Manager 已支持邮件、IM 工具、Webhook 等告警通知方式；
- 其后续版本继续增加了 webhook 自定义请求参数、自定义 PromQL 告警、恢复通知、告警策略导入导出等产品化能力；
- 这验证了 STX 的演进方向应先完成“统一告警 + 实际通知闭环”，再逐步走向策略、模板和高级治理。
