# brainstorm: analyze alerting and notification completeness

## Goal

分析 STX 当前“告警 + 通知”能力的完成度，明确：

1. **已经具备什么**；
2. **哪些只是 MVP / 半成品**；
3. **与 Doris Manager / SelectDB 的成熟能力相比还缺什么**；
4. **后续应该按什么顺序推进**，避免做成一套“有表有页面但不会真正发通知”的系统。

---

## What I already know

### User intent

- 用户希望：**新切一个分支，分析告警和通知功能完整度，以及后面如何做**。
- 用户给了对标参考：
  - `https://docs.selectdb.com/enterprise/2.1/management-guide/monitor-and-alerting/doris-alerting/`

### Repo facts discovered from inspection

#### 1) 代码库里已经存在 monitoring 域，不是从零开始

后端目录：

- `internal/apps/monitoring/`
- `internal/apps/monitor/`

前端目录：

- `frontend/components/common/monitoring/`
- `frontend/lib/services/monitoring/`
- `frontend/app/(main)/monitoring/`

#### 2) 当前后端已经有持久化模型

- `AlertRule`
- `AlertEventState`
- `NotificationChannel`
- `RemoteAlertRecord`

见：

- `internal/apps/monitoring/entity.go`
- `internal/db/migrator/migrator.go`

#### 3) 当前后端已经有一组监控 / 告警 API

- 本地告警：
  - `GET /api/v1/monitoring/alerts`
  - `POST /api/v1/monitoring/alerts/:eventId/ack`
  - `POST /api/v1/monitoring/alerts/:eventId/silence`
- 规则管理：
  - `GET /api/v1/monitoring/clusters/:id/rules`
  - `PUT /api/v1/monitoring/clusters/:id/rules/:ruleId`
- 远程可观测集成：
  - `GET /api/v1/monitoring/prometheus/discovery`
  - `POST /api/v1/monitoring/alertmanager/webhook`
  - `GET /api/v1/monitoring/remote-alerts`
  - `GET /api/v1/monitoring/integration/status`
- 通知渠道：
  - `GET /api/v1/monitoring/notification-channels`
  - `POST /api/v1/monitoring/notification-channels`
  - `PUT /api/v1/monitoring/notification-channels/:id`
  - `DELETE /api/v1/monitoring/notification-channels/:id`

#### 4) 当前前端已经有监控中心页面

- `MonitoringAlertsCenter.tsx`
- `MonitoringRulesPanel.tsx`
- `MonitoringIntegrationsPanel.tsx`
- `MonitoringOverview.tsx`

但这些页面的完成度并不一致。

---

## Assumptions (temporary)

- 这次任务以**产品能力完整度分析 + 路线设计**为主，不做大规模实现。
- 评估维度以“是否形成可用闭环”为准，而不是“是否已经有表 / API / 页面”。
- 告警与通知最终应覆盖两类来源：
  - STX 自身进程 / 集群事件；
  - 外部 Prometheus / Alertmanager 回流的远程告警。

---

## Open Questions

当前无阻塞性问题；后续真正开始实现前，需要补做一个产品决策：

- **长期路线更偏向 Alertmanager-first，还是自研本地 rule engine？**

这不阻塞本次分析，但会影响后续架构分期。

---

## Research Notes

### What similar tools do

#### Doris Manager / SelectDB 的典型能力（从公开文档与发布说明归纳）

公开文档显示，Doris 的告警体系围绕 **Prometheus + Alertmanager + Grafana** 展开，强调：

- 告警规则管理；
- 多种通知方式；
- 历史告警 / 通知记录；
- 监控、告警、可视化三件套联动。

参考：

- Doris 告警文档：<https://docs.selectdb.com/enterprise/2.1/management-guide/monitor-and-alerting/doris-alerting/>
- Doris Monitor 文档：<https://docs.selectdb.com/enterprise/management-guide/monitor-and-alerting/doris-monitor/>

从 SelectDB 发布说明还能看到其告警能力继续往“产品化”方向演进，例如：

- 支持自定义 PromQL 告警；
- 支持告警恢复通知；
- 支持告警配置导入导出；
- 支持 Webhook 自定义参数。

参考：

- v24.1.0：<https://docs.selectdb.com/enterprise/releasenotes/release-notes-24.1.0>
- v24.0.0：<https://docs.selectdb.com/enterprise/releasenotes/release-notes-24.0.0>

#### 结论

成熟的“告警中心”通常不止是：

- 能看到告警；
- 能配一个 channel。

而是至少要形成以下闭环：

1. **规则产生告警**；
2. **告警路由到接收对象 / 接收组**；
3. **通知被真正发出去**；
4. **有历史和投递结果可查**；
5. **支持恢复通知、静默、测试发送、模板 / 参数化**；
6. **前端视图与后端模型是统一的，而不是两套割裂的告警世界。**

### Constraints from our repo/project

#### A. 当前 STX 实际上存在“两套告警域”

**本地告警域**：

- 来源：`internal/apps/monitor` 产生的进程事件；
- 典型事件：`crashed` / `restart_failed` / `restart_limit_reached`；
- 可做动作：ack / silence；
- API：`/monitoring/alerts`。

**远程告警域**：

- 来源：Alertmanager webhook；
- 持久化：`RemoteAlertRecord`；
- API：`/monitoring/remote-alerts`；
- 当前前端告警中心主要展示这套数据。

这两套模型目前**没有统一成一个 alert instance / lifecycle 模型**。

#### B. 远程可观测接入做得比“通知产品化”更完整

仓库已经具备：

- Prometheus HTTP SD；
- Alertmanager webhook 入库；
- 远程告警查询；
- 平台 / 集群健康聚合；
- 文档 + smoke 脚本。

也就是说：**“外部三件套接入”这一段已经接近完整 MVP**。

但“平台内的通知中心能力”明显还没形成闭环。

#### C. Notification Channel 目前是 CRUD，不是 Delivery System

当前只看到了：

- 建表；
- 列表 / 新增 / 更新 / 删除接口；
- 前端新增 / 开关 / 删除页面。

**没有发现真正的通知发送器 / 派发器 / 重试 / 结果落库**。

因此目前它更像“通知渠道配置占位”，而不是一个真的通知系统。

#### D. 告警规则是固定模板 + 基础可编辑，不是通用规则系统

当前默认规则只有 3 条：

- `process_crashed`
- `process_restart_failed`
- `process_restart_limit_reached`

只能更新：

- `severity`
- `enabled`
- `threshold`
- `window_seconds`
- 以及后端支持但 UI 未暴露编辑的 `rule_name` / `description`

没有：

- 新建规则；
- 删除规则；
- 自定义表达式 / PromQL；
- 多条件组合；
- 规则分组 / 模板；
- 规则导入导出。

#### E. `threshold` / `window_seconds` 当前大概率只是“可存储配置”

通过全局检索，`threshold` / `window_seconds` 的使用点只出现在：

- 数据模型；
- DTO 映射；
- 更新请求；
- 默认值创建。

**没有看到真正参与告警判定的运行时逻辑。**

这意味着当前规则管理更像“为后续扩展预留字段”，并没有完全生效。

#### F. 前端监控中心与后端能力并不对齐

1. `MonitoringAlertsCenter.tsx` 当前只消费 `getRemoteAlertsSafe()`，即只看远程告警；
2. 本地告警 API `getAlerts / acknowledgeAlert / silenceAlert` 已存在，但前端没有实际使用；
3. `MonitoringIntegrationsPanel.tsx` 实际只做 notification channel CRUD，并没有消费 `getIntegrationStatusSafe()`；
4. `MonitoringOverview.tsx` 存在，但不在监控中心 tab 中，而是在 dashboard 页面中使用。

结论：**UI 呈现出来的是一套“偏远程告警列表”的视角，而不是完整的告警 / 通知中心。**

---

## Feasible approaches here

### Approach A: Alertmanager-first（推荐）

#### How it works

- 继续把 Prometheus / Alertmanager / Grafana 当成主监控引擎；
- STX 专注做：
  - 受管集群指标发现；
  - 规则配置托管（或规则模板管理）；
  - 告警回流；
  - 通知渠道 / 联系点管理；
  - 路由策略 / 订阅策略；
  - 告警历史 / 通知投递记录；
  - UI 统一聚合。

#### Pros

- 与当前 repo 已有“远程可观测性 MVP”方向一致；
- 可复用 Alertmanager 已有的 grouping / dedup / inhibition / route 能力；
- 实现成本明显低于自研完整告警引擎；
- 更接近 Doris Manager 这类成熟系统的落地方式。

#### Cons

- 需要处理 STX 自身“本地进程事件告警”如何并入统一模型；
- 平台内 UI / 配置模型需要重构，不是简单补几个接口就够。

### Approach B: Hybrid unify（短中期最现实）

#### How it works

- 保留当前本地 process-event 告警；
- 保留当前远程 Alertmanager webhook 回流；
- 在 STX 内部抽象出统一的：
  - `AlertInstance`
  - `AlertSource`
  - `AlertRoute`
  - `NotificationDelivery`
- 然后把“本地告警”和“远程告警”都映射到统一中心。

#### Pros

- 最适合当前代码现状，增量改造路径清晰；
- 可以先把前端 / 后端模型统一，再逐步补通知派发。

#### Cons

- 一段时间内会存在“双来源、单视图”的复杂度；
- 设计不好容易继续出现“字段看起来统一，语义实际不统一”的问题。

### Approach C: Native in-app alert engine（不建议近期走）

#### How it works

- STX 自己做规则执行、窗口聚合、告警生成、通知派发、历史记录、静默策略。

#### Pros

- 完全可控，不依赖外部 Alertmanager。

#### Cons

- 成本最高；
- 需要自己处理 dedup、grouping、重试、escalation、silence、恢复通知等一整套复杂问题；
- 与当前仓库已有的 remote observability 基础方向不一致。

### Recommendation

**推荐采用 A + B 的组合路线：**

- **底座上坚持 Alertmanager-first；**
- **产品层面做本地 / 远程告警统一视图与统一通知域。**

---

## Expansion Sweep (DIVERGE)

### Future evolution

- 后续大概率会从“单机通知渠道”扩展到：
  - 团队 / 成员订阅；
  - 值班 / 升级策略；
  - 维护窗口 / 静默计划；
  - 更多告警对象（任务、资源、插件、升级流程、审计异常等）。

### Related scenarios

- Dashboard、Cluster Health、Alert Center、Rule Center、Notification Center 应共享同一份告警状态语义；
- “触发 / 恢复 / 确认 / 静默 / 发送失败”需要贯穿 UI、API、存储、审计。

### Failure & edge cases

- webhook 重复推送 / 乱序；
- 通知发送失败后的重试与退避；
- 恢复通知是否发送；
- 渠道密钥存储与脱敏；
- 公网 webhook 的鉴权 / 签名校验；
- 同一告警多渠道投递去重；
- 静默与 ack 对通知链路的影响；
- 渠道不可达时的 fallback / escalation。

---

## Current completeness assessment

> 以“能否形成产品闭环”为标准，而不是“有没有表 / API / 页面”。

### 总体判断

- **远程可观测接入：70/100**
- **告警中心（统一视角）：40/100**
- **通知系统（真正能发）：15/100**
- **综合：约 35~45 / 100，属于“基础骨架已搭好，但离产品完成态还有明显距离”。**

### Scorecard

| 能力项 | 当前状态 | 判断 |
|---|---|---|
| Prometheus / Alertmanager / Grafana 远程接入 | 已有 HTTP SD、Webhook、健康探测、文档、脚本 | **较完整 MVP** |
| 本地进程事件告警识别 | 仅 3 类关键进程事件 | **部分完成** |
| 告警规则管理 | 仅默认规则可编辑 | **部分完成** |
| 规则真实生效逻辑 | `threshold/window` 未见运行时使用 | **明显缺失** |
| 告警中心 UI | 只看远程告警，未统一本地告警 | **部分完成** |
| 本地告警动作（ack/silence） | 后端有 API，前端未接入 | **后端有，前端缺** |
| 通知渠道配置 | CRUD 已有 | **部分完成** |
| 通知真实派发 | 未发现 sender / dispatcher / retry | **缺失** |
| 路由 / 策略 / 接收组绑定 | 未发现 | **缺失** |
| 测试发送（test send） | 未发现 | **缺失** |
| 通知历史 / 投递记录 | 未发现 | **缺失** |
| 恢复通知 | 远程告警有 resolved 记录，但无通知闭环 | **部分完成** |
| 静默 / 确认 | 仅本地告警支持，远程告警未统一 | **部分完成** |
| 集成状态展示 | 后端有 API，但当前 UI 未用 | **后端有，前端缺** |
| 安全性 | 公网 webhook 无签名校验；channel secret 明文建模并回传 | **需补强** |

---

## What is already complete enough to build on

### 1) 外部三件套接入底座

这是当前最成熟的一块：

- 公开 HTTP SD 接口；
- Alertmanager webhook 入库；
- 远程告警查询；
- 平台 / 集群健康汇总；
- 文档和 smoke 脚本都齐。

### 2) 基础数据模型已经铺好

虽然不完整，但这些表说明方向已经明确：

- 规则；
- 告警动作状态；
- 通知渠道；
- 远程告警记录。

### 3) 已经具备“最小监控前端骨架”

至少已经有：

- Alerts 页；
- Rules 页；
- Integrations / Notifications 页；
- Dashboard / Overview 组件。

这意味着后续不需要从零设计页面框架。

---

## Major gaps

### Gap 1：没有统一的 Alert Domain

当前至少有 3 套视角：

1. `GetOverview / GetClusterOverview`：偏本地 process events；
2. `ListAlerts / ack / silence`：本地告警；
3. `remote-alerts / platform-health / clusters-health / current alert UI`：偏远程告警。

这会导致：

- 不同页面看到的“活跃告警数”语义不同；
- 有些告警能 ack / silence，有些不能；
- 有些告警出现在 Dashboard，不出现在 Alert Center；
- 用户无法建立一致认知。

### Gap 2：Notification Channel 只有“配置”，没有“发送”

这应该是当前最大的完整度缺口。

没有看到：

- dispatcher / sender；
- 类型化发送器（Webhook / Email / WeCom / DingTalk / Feishu）；
- 重试与失败状态；
- delivery record；
- test send；
- route / receiver binding。

因此从产品角度，当前“通知中心”还不能算完成。

### Gap 3：规则管理是壳，非完整规则系统

目前规则只有固定 3 条，且：

- 不能新增；
- 不能删除；
- 不能自定义表达式；
- 阈值 / 窗口参数看起来未参与运行时判定。

这意味着“规则页”更像配置占位，而不是实际的规则引擎控制台。

### Gap 4：前后端能力不对齐

- 后端有本地告警 ack / silence，前端没接；
- 后端有 integration status，前端没展示；
- 后端支持更新 channel 全字段，前端只做 toggle enabled；
- 后端支持更新 rule name / description，前端未暴露编辑。

### Gap 5：没有通知治理能力

对标成熟系统，缺的不是一两个 API，而是一整层治理能力：

- contact point / receiver group；
- route policy；
- escalation；
- grouping / dedup；
- mute windows；
- notification template；
- delivery audit。

### Gap 6：安全与运维性不足

当前观察到的风险点：

- Alertmanager webhook 是公开入口，但未见签名 / token / allowlist 校验；
- `NotificationChannel.Secret` 目前是明文模型，并通过 DTO 返回到前端；
- 渠道配置是“通用 endpoint + secret”结构，不足以支撑生产级 email / IM 配置校验。

---

## Recommended phased roadmap

## Phase 0：先统一模型，不急着堆功能（建议优先级最高）

### Goal

把“本地告警”和“远程告警”统一成一个产品视图和一个生命周期语义。

### 建议动作

1. 定义统一概念：
   - `AlertSource`（local_process / remote_alertmanager / future_task / future_capacity）
   - `AlertInstance`
   - `AlertLifecycleStatus`（firing / resolved / acknowledged / silenced）
2. 明确各页面的单一数据来源；
3. 统一 Dashboard、Platform Health、Alerts Center 的告警计数口径；
4. 明确 ack / silence 对不同来源的语义：
   - 本地：直接落库状态；
   - 远程：至少支持 STX 侧本地状态覆盖，后续可选同步到 Alertmanager silence。

### 产出

- 一个统一的后端 DTO / API 设计；
- 一个统一的前端 alerts 列表，而不是 remote-only。

---

## Phase 1：补齐“能发通知”的最小闭环（MVP）

### Goal

让通知中心从“配置页”升级为“真正可投递”。

### 最小闭环建议

1. 新增 `NotificationDelivery` / `NotificationAttempt` 持久化；
2. 实现最小 sender：
   - Webhook（优先）
   - WeCom / DingTalk / Feishu（本质仍可抽象为 webhook sender）
3. Email 暂缓到 Phase 2，除非已有统一 SMTP 基础；
4. 支持 **Test Send**；
5. 支持 **按 rule / severity / cluster 绑定 channel**；
6. 发送失败可见：
   - success / failed / last_error / retry_count / next_retry_at

### 为什么这样排

- Webhook / IM webhook 成本低；
- 先有 delivery log，后续失败排查才有抓手；
- 先做 rule-to-channel 绑定，比一上来做复杂 route engine 更稳。

---

## Phase 2：把规则系统做“可用”

### Goal

让规则页从“默认三条可改参数”进化成真正的告警规则能力。

### 建议动作

1. 先把现有 `threshold` / `window_seconds` 真正接入运行时判定；
2. 支持规则启停、阈值、窗口、severity 的真实生效；
3. 允许新增有限类型规则：
   - 进程崩溃次数超阈值
   - 重启失败次数超阈值
   - 重启上限触发
   - 节点离线持续 N 分钟
4. 若走 Alertmanager-first，可增加：
   - 托管 PrometheusRule 模板；
   - STX 侧做表单化配置，最终渲染为 PromQL / rule group。

### 不建议 Phase 2 就做的事

- 自由表达式编辑器；
- 复杂多条件编排；
- 可视化 route builder。

---

## Phase 3：补治理能力，向 Doris Manager 靠拢

### Goal

把“通知系统”升级为“告警治理系统”。

### 建议动作

1. Contact Point / Receiver Group；
2. Notification Policy / Route Tree；
3. Grouping / Dedup / Inhibition；
4. Mute Time Interval / Maintenance Window；
5. Recovery Notification；
6. 模板 / 参数化消息；
7. 配置导入导出；
8. Delivery audit / notification history 检索。

---

## Phase 4：高级产品化能力（后续）

- 值班 / on-call；
- 升级策略（escalation）；
- 用户 / 团队订阅；
- 多租户 / 项目级通知隔离；
- 审计联动；
- 外部 IM / Email 配置管理台；
- 告警噪音治理与智能聚合。

---

## Suggested implementation order (practical)

如果按工程投入 / 业务收益比排序，建议是：

1. **统一 alert domain 与 UI 口径**
2. **补 NotificationDelivery 与 Webhook sender**
3. **补 rule-channel 绑定 + test send**
4. **让现有 threshold/window 真正生效**
5. **再做 contact point / route policy / history / recovery notification**

不建议一开始就：

- 自研完整 rule engine；
- 自研复杂 route tree；
- 一次性支持所有渠道的全部高级能力。

---

## Acceptance Criteria (for this analysis task)

- [x] 已梳理现有后端 / 前端 / 数据模型能力；
- [x] 已识别本地告警与远程告警割裂问题；
- [x] 已确认 notification channel 当前缺乏真实派发闭环；
- [x] 已对照 Doris / SelectDB 的产品化能力形成差距分析；
- [x] 已输出分阶段推进建议。

---

## Out of Scope (explicit)

- 本次不直接实现新的通知发送器；
- 本次不直接改造前端监控中心；
- 本次不直接设计完整数据库迁移方案；
- 本次不直接接入 Alertmanager silence API；
- 本次不直接决定最终是否完全托管 PrometheusRule / Alertmanager route 配置。

---

## Technical Notes

### Key files inspected

#### Backend

- `internal/apps/monitoring/entity.go`
- `internal/apps/monitoring/model.go`
- `internal/apps/monitoring/service.go`
- `internal/apps/monitoring/repository.go`
- `internal/apps/monitoring/remote_integration.go`
- `internal/router/router.go`
- `internal/db/migrator/migrator.go`
- `internal/config/model.go`
- `config.example.yaml`

#### Frontend

- `frontend/lib/services/monitoring/types.ts`
- `frontend/lib/services/monitoring/monitoring.service.ts`
- `frontend/components/common/monitoring/MonitoringAlertsCenter.tsx`
- `frontend/components/common/monitoring/MonitoringRulesPanel.tsx`
- `frontend/components/common/monitoring/MonitoringIntegrationsPanel.tsx`
- `frontend/components/common/monitoring/MonitoringOverview.tsx`
- `frontend/components/common/monitoring/MonitoringCenterWorkspace.tsx`

#### Docs / scripts

- `docs/observability-remote-integration-guide.md`
- `docs/observability-remote-design.md`
- `docs/observability-remote-mvp-plan.md`
- `docs/可观测性三件套一键接入说明.md`
- `scripts/observability-remote-smoke.sh`
- `scripts/observability-enabled-switch-regression.sh`

### Concrete findings worth carrying into implementation

1. `threshold` / `window_seconds` 当前未见运行时使用，应视为“未闭环字段”；
2. 本地告警 API 已有，但前端未接；
3. `integration/status` API 已有，但前端未使用；
4. `NotificationChannel.Secret` 当前建模与 DTO 返回方式不适合直接进入生产态；
5. 当前健康汇总更多基于 remote alert，而 overview 又偏 process event，统计口径存在割裂；
6. 如果后续要做统一告警中心，优先要解决的是**模型统一**，不是“再多加几个页面按钮”。

---

## Replan after product clarification (2026-03-08)

### New product principle

用户侧不应感知“本地告警”和“远程告警”是两套割裂系统。

最终产品视图应当是：

- 一个统一的 **Alerts Center / Alert Policy Center**；
- 一套统一的告警生命周期（firing / recovered / acknowledged / silenced / notified）；
- 一套统一的通知体验（站内、Webhook、IM、邮件等）；
- 告警来源由系统内部管理，而不是让用户先理解内部架构差异。

### Internal architecture principle

虽然用户视图统一，但后端仍然必须保留 **source-aware capability model**，至少区分：

1. **Platform health / managed runtime signals**
   - 由 STX 自身生成或聚合；
   - 例如：master 不可用、worker 不足、agent 离线、进程 crash/restart 失败、升级失败、配置下发失败；
   - 这些能力不依赖 Prometheus，也应在无监控组件时继续工作。

2. **Prometheus / Alertmanager powered signals**
   - 依赖监控组件与指标链路；
   - 例如：CPU、内存、FD、失败作业数、磁盘容量、脑裂相关指标、JVM/线程/QPS 等；
   - 支持静态模板指标与自定义 PromQL；
   - 若监控组件不可用，则这些能力应自动降级或隐藏。

结论：

- **产品层统一**；
- **能力层分源**；
- **展示层按 capability 自动裁剪**。

### Revised answer to "do we need to distinguish local and remote?"

对用户：

- **不作为主概念区分**；
- 用户应该操作“告警策略”“通知策略”“通知历史”，而不是在两个告警系统之间切换。

对系统内部：

- **必须区分**；
- 因为不同来源的采集方式、可用性检测、触发引擎、恢复语义、数据字段、依赖组件都不同。

因此更合适的产品设计是：

- UI 中以 **策略类型 / 数据来源能力** 呈现；
- 而不是直接暴露“本地告警页面 / 远程告警页面”。

### Capability gating principle

Prometheus / Alertmanager 相关能力应采用 capability gating：

- 若 Prometheus / Alertmanager 已安装、已配置、可联通：
  - 显示 metrics-based policy builder；
  - 显示静态指标模板；
  - 显示自定义 PromQL；
  - 显示远程 webhook / remote alert ingest 状态。
- 若相关组件未安装、未配置或不可用：
  - 仍保留平台健康类告警与通知能力；
  - 隐藏或禁用依赖指标链路的高级能力；
  - 在页面中给出 capability 提示，而不是报错暴露内部异常。

### Revised task goal

本任务不再只是“补本地规则”或“补远程通知”，而是重规划为：

**设计并逐步实现一个统一的 STX 告警策略中心：**

- 面向用户是一体化的告警产品；
- 面向系统内部是“平台健康信号 + Prometheus 指标信号”的双引擎；
- 在监控组件不可用时自动降级，而不是让产品形态断裂。

### Revised product scope

#### Track A: Unified alerting product layer

统一这些概念：

- Alert Policy
- Alert Instance
- Notification Target / Receiver Group
- Notification Delivery
- Notification History
- Recovery Notification
- Cooldown / minimum notify interval

#### Track B: Platform health engine

负责不依赖 Prometheus 的平台健康告警，包括但不限于：

- master unavailable
- worker insufficient
- node offline
- agent offline
- process crashed / restart failed / restart limit reached
- deploy failed / upgrade failed / config apply failed

#### Track C: Metrics-based engine (Prometheus-powered)

负责依赖 Prometheus / Alertmanager 的指标类告警，包括：

- CPU / memory / disk / fd / threads
- failed jobs / queue backlog / latency / errors
- split brain / consensus / role inconsistency related metrics if observable
- static metric templates
- custom PromQL

### Revised non-goals

当前不把这些作为第一阶段必做：

- 值班排班 / on-call escalation
- 复杂 route tree / inhibition / grouping tree editor
- 多租户通知隔离
- 完整的模板脚本语言
- 与第三方 IM 的所有高级交互特性

### Revised phased roadmap

#### Phase 1: Unified policy domain and capability-aware UX

目标：

- 不再让前端表现成“两套告警世界”；
- 统一策略、统一告警列表、统一通知历史；
- 引入 capability 检测与页面裁剪。

交付：

- unified alert policy model
- integration/capability status API
- frontend capability-aware policy builder shell
- unified alert center list contract

#### Phase 2: Platform health policy engine

目标：

- STX 在没有 Prometheus 时，仍能独立提供有价值的集群健康告警。

交付：

- platform-health policy templates
- runtime evaluation for local/managed signals
- cooldown / recovery / notification integration
- cluster health oriented alert semantics

#### Phase 3: Prometheus-powered policy engine

目标：

- 在 Prometheus 可用时，把指标告警真正产品化，而不是只有 webhook 回流。

交付：

- static metrics templates
- custom PromQL policies
- metrics preview / capability checks
- Alertmanager / Prometheus integration alignment

#### Phase 4: Notification center productization

目标：

- 把渠道能力从“channel CRUD”升级成产品级通知系统。

交付：

- receiver groups
- in-app notification center
- recovery notification delivery
- delivery history search / filter / retention
- webhook custom parameters / richer IM payloads

### Revised implementation recommendation

基于当前仓库状态，建议先把 PR-4 改成：

**PR-4: unify alert policy domain and add capability-aware alerting architecture**

其重点不再是单纯“让 threshold 生效”，而是先建立后续演进不会推翻的产品骨架：

1. 统一策略域模型；
2. 保留 source-aware 后端能力模型；
3. 前端统一显示，但按 capability 自动裁剪；
4. 为后续平台健康引擎和 PromQL 引擎留出稳定扩展点。

### References used for this replanning

- Doris 集群告警（站内通知、邮箱、IM、Webhook、策略配置）：
  https://docs.selectdb.com/enterprise/management-guide/monitor-and-alerting/doris-alerting/
- Doris 集群监控（Prometheus + Grafana + AlertManager 集成、预制指标、主机指标）：
  https://docs.selectdb.com/docs/enterprise/doris-manager-guide/monitor-and-alerting/doris-monitor
- Enterprise Manager release notes（自定义 PromQL、Webhook 自定义参数、告警恢复内容优化、日志告警等）：
  https://docs.selectdb.com/enterprise/release-notes/enterprisemanager/
