# 作业级监控面板（Prometheus 时序 + 详情「监控」页签）

## Goal

在作业详情中查看作业级的历史趋势（吞吐、条数、CDC 延迟、Checkpoint、算子忙碌度 / 背压），而不只是当前快照；
接入 Prometheus 时可看长历史并跳转 Grafana，未接入时仍能看最近 1 小时。

## Confirmed Facts（2026-10-28 在 t1 实测）

- SeaTunnel 3.0.0 的 Prometheus 端点（节点 `:5801/hazelcast/rest/instance/metrics`）只有集群 / 节点 / JVM / 线程池 /
  状态存储指标，标签只有 `cluster`、`address`、`type`、`pool` 等，**没有任何作业维度**（`job_count` 只按状态计数）。
  → 作业级时序必须由 STX 自己导出。
- STX 已有的作业数据都是“当前值”：对账每 10 秒拉 `job-info`（按表读写条数 / QPS / 字节，`result_preview.metrics`）；
  `/sync/jobs/:id/realtime` 有算子忙碌度 / 边背压（引擎只保留 10 分钟）；`sync_engine_events` 有 CDC 延迟、表结构变更、
  状态变化（保留 7 天）；Checkpoint 只在查看时请求。
- 可观测性按 `docs/observability-remote-design.md`：STX 不托管 Prometheus，通过 HTTP SD 让外部 Prometheus 抓 SeaTunnel；
  Grafana 面板 JSON 随版本发布。t1 上 `observability.enabled=false`，没有运行 Prometheus。
- STX 未引入 `prometheus/client_golang`，没有自己的 `/metrics`；前端已有 `recharts`（shadcn `components/ui/chart.tsx`），未使用。
- 作业详情（`frontend/components/common/sync/job-detail/`）已有页签：拓扑 / 指标 / Checkpoint / 日志 / 事件 / 配置 / 脚本。

## Requirements

- R1 STX 导出作业级指标（仅运行中作业），并登记到现有 HTTP SD，供外部 Prometheus 抓取。
- R2 后端时序查询接口，服务端持有 PromQL 模板；未接 Prometheus 时用进程内最近 1 小时环形缓冲兜底，并在界面提示。
- R3 详情新增「监控」页签（现有「指标」保留为当前快照），布局见 design.md。
- R4 多表作业按表趋势受基数上限约束（默认每作业前 50 张表，其余并入 `other`）。
- R5 新接口补 CLI（AGENTS.md 约定）；所有新文案 zh / en。

## Acceptance Criteria

- [ ] 接入 Prometheus 时，运行中作业的读 / 写 QPS、累计条数、CDC 延迟在 1h / 6h / 24h / 7d 范围内可查。
- [ ] 未接入时显示最近 1 小时曲线与“接入 Prometheus 可看更长历史”提示，不报错。
- [ ] 图上叠加事件标记（状态变化、表结构变更、重启），点击可看事件原始数据。
- [ ] 标签基数有上限；作业结束后不再导出该作业的序列。
- [ ] `stx sync job timeseries <id> --range 1h` 可用。

## Open Questions（待用户决定）

1. 未接 Prometheus 时是否做 1 小时内存兜底（建议做）。
2. 新增「监控」页签，还是与「指标」合并为“快照 / 趋势”两个视图。
3. 忙碌度 / 背压与 Checkpoint 是否需要后台常驻采集（有引擎调用成本），还是只在第二期做。
