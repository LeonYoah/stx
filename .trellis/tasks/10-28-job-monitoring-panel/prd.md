# 作业级监控面板（Prometheus 时序 + 指标「快照 | 趋势」）

## Goal

在作业详情的「指标」页签查看作业级历史趋势（吞吐、条数、CDC 延迟、Checkpoint、算子忙碌度 / 背压），而不只是当前快照；
接入 Prometheus（`observability.enabled`）后可看长历史并跳转 Grafana。未接入时 **不采集、不保存任何时序**，趋势页展示空态引导开启。

## Confirmed Facts（2026-10-28 在 t1 实测 + SeaTunnel 调研）

- SeaTunnel 3.0.0 的 Prometheus 端点（节点 `:5801/hazelcast/rest/instance/metrics`）只有集群 / 节点 / JVM / 线程池 /
  状态存储指标，标签只有 `cluster`、`address`、`type`、`pool` 等，**没有任何作业维度**（`job_count` 只按状态计数）。
  → 作业级时序必须由 STX 自己导出。
- SeaTunnel **不能** push 指标；事件通道（已集成）只有 JOB_STATUS / SCHEMA_CHANGE_* / LIFECYCLE_* / READER_MESSAGE_DELAYED，
  没有条数 / Checkpoint / busy 事件。无 Pushgateway / OTel / 作业 JMX。
- Busy / backpressure：ST 3.0+ 且作业 `engine.observability` 开启；master 内存 3–10 分钟；`GET /metrics/realtime/...`；
  Push Mode 未实现。STX 在引擎可观测性开启时约 60s 轮询（windowMs）。
- Checkpoint：引擎可观测性开启时后台约 30–60s 拉 `/jobs/checkpoints`；当前值始终可按需轮询。
- Rows / QPS：顺带现有约 10s job-info 对账写入，不另增引擎调用。
- STX 已有的作业数据都是“当前值”：对账每 10 秒拉 `job-info`（`result_preview.metrics`）；
  `/sync/jobs/:id/realtime` 有算子忙碌度 / 边背压；`sync_engine_events` 有 CDC 延迟、表结构变更、状态变化；
  Checkpoint 可按需请求。
- 可观测性按 `docs/observability-remote-design.md`：STX 不托管 Prometheus，通过 HTTP SD 让外部 Prometheus 抓取；
  Grafana 面板 JSON 随版本发布。
- 前端已有 `recharts`（shadcn `components/ui/chart.tsx`）。作业详情页签：拓扑 / 指标 / Checkpoint / 日志 / 事件 / 配置 / 脚本。

## Locked Decisions（2026-10-28，覆盖旧稿）

1. **无 Prometheus → 无进程内 1h 内存兜底，不存储任何时序。**
2. **不新增「监控」页签。** 现有「指标」页签顶部 segment「快照 | 趋势」；快照 = 现有指标 UI；趋势 = 图表。
3. **Busy / backpressure / Checkpoint 的采集**跟随作业 `engine.observability.enabled`：开则采，关则不采。

### 两个开关

| 开关 | 作用 |
|---|---|
| STX `observability.enabled`（接上 Prometheus） | 为真时趋势才能查图；否则空态提示需开启 STX 可观测性 / 接入 Prometheus。快照始终可用。关闭时 STX **不采集、不导出**作业时序。 |
| 作业 `engine.observability.enabled` | **只**约束忙碌度 / 背压卡片：关时这两张卡显示「该作业未开启引擎可观测性」。吞吐 / 条数 / CDC / Checkpoint 在 Prometheus 开启时仍可出图（Checkpoint 历史依赖采集，采集亦跟随引擎可观测性）。 |

Hero 当前值（读/写 QPS、CDC、最近 Checkpoint）始终来自 job-info / checkpoints 轮询；历史曲线依赖 STX 导出 + Prometheus。

## Requirements

- R1 STX 导出作业级指标（仅运行中作业），登记到现有 HTTP SD（`job="stx_sync_jobs"`），供外部 Prometheus 抓取。
- R2 后端时序查询接口，服务端持有 PromQL 模板；响应 `source: "prometheus"` + annotations；**无 local source**。
- R3 「指标」页签「快照 | 趋势」；布局与空态见 design.md / mockups。
- R4 表级标签基数上限（默认每作业前 50 张表，其余 `other`）；按表趋势 UI = 第三期（可占位）。
- R5 新接口补 CLI；所有新文案 zh / en；swagger 与路由基线更新。

## Acceptance Criteria

- [ ] `observability.enabled=true` 且 Prometheus 已抓取时，运行中作业的读 / 写 QPS、累计条数、CDC 延迟在 15m / 1h / 6h / 24h / 7d 可查。
- [ ] `observability.enabled=false` 时趋势空态（需 Prometheus），不报错；STX 不导出 `/jobs/metrics`（404/503），不写任何时序。
- [ ] 引擎可观测性关闭时，忙碌度 / 背压卡显示「该作业未开启引擎可观测性」；吞吐等其他卡在 Prometheus 可用时仍出图。
- [ ] 图上叠加事件标记（状态变化、表结构变更、重启），点击可看事件原始数据。
- [ ] 标签基数有上限；作业终态后删除该作业序列。
- [ ] `stx sync job timeseries <id> --range 1h` 可用。

## Open Questions

（已全部锁定，无待决项。）
