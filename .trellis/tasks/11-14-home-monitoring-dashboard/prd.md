# 首页监控大盘（替代 Grafana）+ 全局作业列表

## Goal

不再依赖 Grafana。在控制台首页自建监控大盘（集群 / 作业）和一个全局作业列表，
让用户在 STX 内完成「集群是否健康、作业跑得怎样、哪里要处理」的判断，而不用跳到外部面板。

原型：`prototype/dashboard.html`（可交互，模拟数据）与 `prototype/p-*.webp` 截图。

## Background / Confirmed Facts（2026-11-14 调研）

- 现首页 `/dashboard`（`app/(main)/dashboard/page.tsx`，879 行）：StatPillsBar + 告警事件 / 待办 / 最近活动 / 资源统计四宫格，
  底部 `MonitoringOverview compact` 以 iframe 嵌入 Grafana（经 `/api/v1/monitoring/proxy/grafana` 代理，默认收起）。
- Grafana 随版本发布两份面板 JSON：`deploy/docker/observability/grafana/dashboards/seatunnel-overview-{zh,en}.json`，
  面板分 P0 可用性 / P1 资源压力 / P2 调度吞吐 / P3 Hazelcast / P4 异常信号，全部基于 `job="seatunnel_engine_http"` 指标及
  `seatunnel:*` 录制规则。
- Prometheus 中 SeaTunnel 目标带 `cluster_id`、`cluster_name`、`instance` 标签（HTTP SD 下发），可直接按 STX 集群过滤。
- STX 自导出作业指标 `stx_job_*`（PR #135 / 作业级监控面板任务），标签 `cluster_id`、`task_id`、`job_id`（另有 `table`、`direction`、`vertex` 等）。
  作业终态后序列删除。指标里**没有**源 / 目标连接器标签。
- 已有作业趋势接口 `GET /sync/jobs/:id/timeseries`：服务端持有 PromQL 模板，`observability.enabled=false` 时返回 `source:"none"`。
- `GET /sync/jobs` 现只支持 `task_id / run_type / platform_job_id / engine_job_id` 过滤，按 `created_at DESC`，非管理员只看自己创建的作业；
  任务名、集群、模式在 `sync_tasks` 表（`name / cluster_id / mode`），需要关联查询。
- 运行中作业的当前读写速率与累计条数在 `result_preview.metrics`（10 秒对账写入），无需 Prometheus。
- **2026-11-16 起，可观测改为内嵌单模式**（任务 `11-16-embedded-observability`，PR #140 起）：STX 进程内嵌 Prometheus 的
  `tsdb` + `promql` 引擎，抓取 SeaTunnel 指标与写入 `stx_job_*` 由 STX 自己完成，告警评估自研，外部 Prometheus / Alertmanager / Grafana
  全部退役；HTTP SD 与 `jobs/metrics` 端点删除。`promquery.Range/Instant` 签名与 `source/reason` 取值不变，本任务已写的大盘后端无需改动。
  保留期默认 31 天、存储上限默认 30GiB，均可配。
- 组件：Arc 已装 line-chart / brush-chart / bar-chart / donut-chart / segmented-control / chip-group / badge 等；
  beUI 已在 `components.json` 注册（`@beui`），已用过 file tree；仓库内**未**引入 HeroUI。

## Locked Decisions

1. **去掉 Grafana；时序存储与查询由内嵌可观测任务负责**。STX 后端持有 PromQL 模板，经 `promquery` 查询进程内引擎，前端不接触 PromQL。
   （原决定为「保留外部 Prometheus、STX 进程内不存时序」，2026-11-16 随内嵌可观测任务改为内嵌单模式。本任务不实现存储，只消费 `promquery`。）
2. **未接入 Prometheus 时首页仍可用**：顶部指标卡、集群卡片、节点表、作业列表的当前值来自 DB / Agent 心跳 / 10 秒对账；
   仅趋势面板显示引导空态。
3. 首页结构：顶部（范围选择 + 时间窗 + 指标卡 + 「需要处理」条）三个页签共享；页签 = 集群 / 作业 / 作业列表。
4. 图表与列表优先用 Arc 与 beUI 现成组件，不手写图表（详见 design.md 组件映射）。
5. 原首页「最近活动」「资源统计」移除：最近活动由审计日志承接，资源统计进入工作台（2026-11-14 用户确认）。
6. 作业类聚合（全局吞吐、Top N、数据流向、Checkpoint、算子热点、运行日历、指标卡计数）对所有登录用户展示全局数据；
   作业列表、行预览、sparklines 等逐个作业的数据仍按作业可见范围过滤（2026-11-14 用户确认）。
7. 实现基线：先把 PR #135 后续提交（`local/pr-134-135` 上未进 main 的部分）合入 main，各子任务从 main 拉分支（2026-11-14 用户确认）。

## Requirements

- R1 顶部指标卡：运行中作业（流 / 批）、全局写入速率、最大 CDC 延迟、24 小时失败作业、集群健康（集群 / 节点）、活跃告警；点击跳到对应页签或告警中心。
- R2 「需要处理」条：失败作业、CDC 延迟超阈值、严重告警、高背压算子，点击直达对象。合并原「告警事件」「待办提醒」两块。
- R3 集群页签：集群卡片（可切换范围）、节点可用性状态条、JVM 堆 / CPU / GC / 引擎作业状态构成趋势、节点表、异常信号；覆盖原 Grafana P0–P4 的关键信号。
- R4 作业页签：全局吞吐（叠加事件标记）、写入速率 Top N、CDC 延迟 Top N（阈值线）、数据流向（源 → 目标连接器）、运行日历、Checkpoint 耗时、算子热点。
- R5 作业列表页签：跨任务的全局作业列表，支持状态 / 模式 / 集群 / 关键字筛选、排序（默认失败优先）、行内近 1 小时写入迷你曲线、多选批量停止 / 恢复、行预览抽屉、进入现有作业详情。
- R6 移除 Grafana：首页 iframe（P2 已完成）、Grafana 代理与配置、前端残留（P4）；安装器 / docker 中的 Grafana 组件与面板 JSON
  随三件套一起由内嵌可观测任务删除。
- R7 每个新增接口补 CLI（经 `internal/operation` 注册表生成）；新文案 zh / en；swagger 与路由基线更新。
- R8 权限：聚合类数据所有登录用户看全局；作业列表与逐作业数据按作业可见范围过滤。Top N 中当前用户无权查看的作业只显示名称与数值，不可点开。

## Task Map

| 子任务 | 交付 | 依赖 Prometheus |
|---|---|---|
| `11-14-dashboard-p1-job-list` | 首页新骨架（顶部 + 页签框架）、指标卡、需要处理条、作业列表页签、`/sync/jobs` 扩展、sparkline 批量接口 | 仅 sparkline 与写入速率卡的历史曲线 |
| `11-14-dashboard-p2-cluster` | 集群页签、集群大盘接口；首页移除 Grafana iframe | 是（集群卡片与节点表当前值除外） |
| `11-14-dashboard-p3-jobs` | 作业页签、作业大盘接口、运行日历接口 | 是（运行日历除外） |
| `11-14-dashboard-p4-remove-grafana` | 大盘收尾：Grafana 代理 / 配置 / 前端残留摘除、按内嵌口径改文案与 spec、大盘响应缓存、内嵌环境复验 | 是（依赖内嵌 P2） |

顺序：P1 → P2 → P3 → P4。P4 依赖内嵌可观测 **P2** 合并（大盘要有数据才能复验）；其中抽样一致性核对须在内嵌 P3（外部 Prometheus 退役）之前做。
PR 叠加：#137 → #138 → #139 → #140（内嵌 P1）→ 内嵌 P2 → P4。内嵌 P1 单独合并会让大盘全部空态（查询已切到空 tsdb），P1 与 P2 须连续合并、中间不发版。

## Enhancements（P1–P4 验收完成后再做，2026-11-16 用户确认）

| 子任务 | 内容 | 已定决策 |
|---|---|---|
| `11-16-dashboard-e1-cluster-metrics` | 集群指标细化：JVM 内存池 / 直接内存、GC 分回收器、线程状态、调度与 slot 延迟、Hazelcast、状态存储、进程网络；重启 / 成员变化标注 | 不新增采集：内嵌抓取器写入的 SeaTunnel 全量指标；依赖内嵌 P2 |
| `11-16-dashboard-e2-data-volume` | 每日 / 每月 / 指定区间读写量：指标卡 + 日 / 月柱状图 | 只用内嵌 tsdb，保留期默认 31 天（受 30GiB 上限约束，以实际最早样本为准）；依赖内嵌 P2 |
| `11-16-dashboard-e3-host-metrics` | 服务器 CPU / 内存 / 磁盘 / IO / 网络 / 负载 | Agent（gopsutil）经 gRPC 上报 → 控制面直写内嵌 tsdb；依赖内嵌 P2 |

增强排期建议：P4 → E2（直接受益于 31 天保留期）→ E1 → E3（涉及 Agent 与 proto，最大）。

## Cross-child Acceptance Criteria

- [ ] `observability.enabled=false`：首页三个页签都能打开，不报错；当前值正常，趋势面板显示统一空态与开启指引。
- [ ] 内嵌存储未就绪（打开失败 / 查询失败）：趋势面板显示「监控数据暂不可用」，不误导为「未开启」。
- [ ] `observability.enabled=true` 且内嵌抓取已有数据：15m / 1h / 6h / 24h / 7d 时间窗切换后所有趋势面板同步刷新。
- [ ] 范围选择切到单个集群后，指标卡、集群页签、作业页签、作业列表都按该集群过滤。
- [ ] 非管理员在作业列表 / 行预览 / sparklines 中看不到别人的作业；聚合图显示全局，越权条目不可点开。
- [ ] 所有新增接口都有 CLI；`stx` 帮助里可见。
- [ ] 中英文切换无遗漏文案；390px 宽度下不溢出。
- [ ] Grafana 代理与配置删除后，监控中心、告警中心功能不受影响（安装 / 升级冒烟归内嵌可观测任务）。

## Out of Scope

- 时序存储、抓取、告警评估的实现（归内嵌可观测任务）；本任务只消费 `promquery`。
- 自定义大盘（用户拖拽面板、保存视图）。
- 告警规则编辑（仍在告警中心）。

## Resolved Questions

- Q1 首页旧内容 → 移除（见 Locked Decision 5）。
- Q2 非管理员的作业类聚合 → 所有人看全局（见 Locked Decision 6）。
- Q3 实现基线 → 先合入 main（见 Locked Decision 7）。
