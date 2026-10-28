# 设计：作业级监控面板

## 1. 数据：STX 导出作业指标

- 引入 `github.com/prometheus/client_golang`（成熟库，不手写 exposition），新增 `GET /api/v1/monitoring/prometheus/jobs/metrics`
  （受 `observability.enabled` 控制），HTTP SD 增加一个 target group：`job="stx_sync_jobs"`，target 为 STX 自身。
- 只导出运行中作业；作业进入终态后删除其序列（Prometheus 保留历史）。采集在对账写入 `result_preview.metrics` 时顺带更新
  （不新增引擎调用）。
- 标签：`cluster_id`、`task_id`、`job_id`（STX 作业实例 ID）；算子级加 `vertex`；表级加 `table`（每作业前 50 张，其余 `other`）。

| 指标 | 类型 | 来源 |
|---|---|---|
| `stx_job_source_received_rows_total` / `_bytes_total` | counter | `SourceReceivedCount/Bytes` |
| `stx_job_sink_write_rows_total` / `_bytes_total` | counter | `SinkWriteCount/Bytes` |
| `stx_job_sink_committed_rows_total` | counter | `SinkCommittedCount` |
| `stx_job_table_rows_total{direction="read|write",table}` | counter | `TableSourceReceivedCount` / `TableSinkWriteCount` |
| `stx_job_cdc_delay_seconds` | gauge | 事件通道 `READER_MESSAGE_DELAYED`（内存最新值） |
| `stx_job_checkpoint_duration_seconds` / `stx_job_checkpoint_failed_total` | gauge / counter | 第二期：后台低频拉 checkpoint overview |
| `stx_job_vertex_busy_ratio{vertex}` / `stx_job_edge_backpressure_ratio{vertex}` | gauge | 第二期：后台拉 `/metrics/realtime`（仅 3.x 且开启可观测性） |
| `stx_job_up{status}` | gauge | 作业状态 |

注意：引擎给的是累计值，作业恢复（新实例）会从 0 开始，`job_id` 不同所以不会污染 `rate()`。

## 2. 查询

- `GET /api/v1/sync/jobs/:id/timeseries?range=1h&step=auto&metrics=throughput,rows,cdc_delay,...`，R0，权限同 `/realtime`。
- 服务端按 metric key 选 PromQL 模板（例：吞吐 `sum by (job_id) (rate(stx_job_source_received_rows_total{job_id="$id"}[1m]))`），
  调 Prometheus `query_range`；`step` 按范围自动（1h→15s，6h→1m，24h→5m，7d→30m）。
- 未接 Prometheus 或请求失败：返回进程内环形缓冲（每作业每 30 秒一个点，保留 1 小时，最多 N 个运行中作业），
  响应带 `source: "prometheus" | "local"` 与 `retention_seconds`，前端据此提示。
- 响应结构：`{source, step_seconds, series:[{key, label, unit, points:[[ts_ms, value]]}], annotations:[{ts_ms, kind, summary, event_id}]}`；
  annotations 取 `sync_engine_events`（状态、表结构变更）与重启。
- CLI：`stx sync job timeseries <id> --range 1h --metrics throughput`（registry 生成）。

## 3. 前端：详情「监控」页签

```
┌ 时间范围 [15m 1h 6h 24h 7d]  自动刷新 ⟳   来源: Prometheus / 本地 1h   [在 Grafana 打开] ┐
├ 读 QPS │ 写 QPS │ 读写差(积压) │ CDC 延迟 │ 最近 Checkpoint ┤   ← 关键数字（hero number，非图）
├ 吞吐（读/写 两条线，records/s）      │ 累计条数（读/写/已提交）              ┤
├ CDC 延迟（秒，单线）                  │ Checkpoint 耗时（柱，失败红点）        ┤
├ 算子忙碌度（每算子一线）              │ 背压（每条边一线）                     ┤
└ 按表（多表作业）：表名 │ 迷你趋势 │ 当前 QPS │ 累计，可搜索、排序，前 50                    ┘
所有折线图共享 x 轴与十字准线；事件以竖线标记，悬停显示摘要，点击打开事件原始数据。
```

- 图表用 recharts（shadcn chart），遵循 dataviz 规范：一个 y 轴、2px 线、悬停十字准线 + tooltip、≥2 条序列有图例、
  文字用文本色不用序列色。暗色模式序列色：读 `#3987e5`、写 `#d95926`、已提交 `#199e70`（暗底 `#0f0f11` 上校验通过，
  `validate_palette.js` 全部 PASS）；状态色 good `#0ca30c` / warning `#fab219` / critical `#d03b3b` 只用于状态。
- 只在「监控」页签可见时请求；运行中作业每 30 秒刷新一次，终态作业不轮询。
- 拓扑节点抽屉「指标」页签加该算子的忙碌度 / 背压迷你趋势（第二期）。

## 4. 分期

1. 导出 + HTTP SD + 时序接口（含本地 1h 兜底）+「监控」页签（吞吐、条数、CDC 延迟、事件标记）+ CLI。
2. 后台采集 Checkpoint 与忙碌度 / 背压（注意引擎调用成本：每运行中作业 ≤1 次/30s，可配置关闭）。
3. 按表趋势 + 作业级 Grafana 面板 JSON（随 `deps/grafana_config/dashboards` 发布）。

## 5. 风险

- 标签基数：表级序列必须封顶；`job_id` 随作业结束删除。
- 本地兜底占内存：按“运行中作业数 × 序列数 × 120 点”估算并设上限。
- 设计阶段未出可视原型（会话额度用尽前已校验配色）；实现前可先出原型请用户确认布局。
