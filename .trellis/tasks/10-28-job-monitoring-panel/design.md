# 设计：作业级监控面板

> 锁定决策见 prd.md。Mockups（只读）：`/workspace/stx-mockups/job-metrics*.png`、`job-metrics.html`。

## 0. SeaTunnel 调研结论 → 为何 STX 导出 + HTTP SD

- 引擎不能 push 指标；事件通道无条数 / CP / busy。
- Prom 端点只有集群 / 节点维度，无 job。
- Busy/BP：3.0+、作业 observability 开、master 内存短窗、`/metrics/realtime`；Push Mode 未实现。
- Checkpoint：`/jobs/checkpoints` 轮询（核心 REST，不依赖 `engine.observability`）。
- Rows/QPS：复用对账 job-info，不加额外引擎调用。
→ **STX 导出作业指标 + HTTP SD 是正确路径。**

## 1. 数据：STX 导出作业指标

- 引入 `github.com/prometheus/client_golang`，新增
  `GET /api/v1/monitoring/prometheus/jobs/metrics`（受 **STX** `observability.enabled` 控制；关闭时 503）。
- HTTP SD（`BuildPrometheusSDTargets`）增加 target group：`job="stx_sync_jobs"`，
  target 为 `app.external_url` 的 host:port，`__metrics_path__=/api/v1/monitoring/prometheus/jobs/metrics`。
  Prometheus 配置增加对应 scrape job（或 relabel keep `job=stx_sync_jobs`），与 `seatunnel_engine_http` 并列。
- **仅当** `observability.enabled=true` 时更新 / 导出序列；关闭时不写 registry、不后台采集。
- 只导出运行中作业；进入终态后 `DeletePartialMatch` 删除其序列（Prometheus 保留历史）。
- 吞吐 / 条数 / 表级：在对账写入 `result_preview.metrics` 时顺带更新（不新增引擎调用）。
- CDC 延迟：事件通道 `READER_MESSAGE_DELAYED` 内存最新值写入 gauge。
- Checkpoint + vertex busy / edge backpressure：**一期就做**，后台低频采集（约 30–60s）。
  Checkpoint 只要求 STX `observability.enabled`（Zeta `/jobs/checkpoints` 为核心 REST，与 `engine.observability` 无关）；
  busy / backpressure **仅当该作业引擎可观测性开启时**采集，关闭则不调 realtime、不导出这两类指标。
- 标签：`cluster_id`、`task_id`、`job_id`（STX 作业实例 ID）；算子级加 `vertex`；边加 `edge`；
  表级加 `table`（每作业前 50 张，其余并入 `other`）。

| 指标 | 类型 | 来源 | 采集条件 |
|---|---|---|---|
| `stx_job_source_received_rows_total` / `_bytes_total` | counter | job-info metrics | STX obs on |
| `stx_job_sink_write_rows_total` / `_bytes_total` | counter | 同上 | STX obs on |
| `stx_job_sink_committed_rows_total` | counter | SinkCommittedCount | STX obs on |
| `stx_job_table_rows_total{direction="read\|write",table}` | counter | Table* maps，前 50 + other | STX obs on |
| `stx_job_cdc_delay_seconds` | gauge | 事件通道最新值 | STX obs on |
| `stx_job_checkpoint_duration_seconds` | gauge | `/jobs/checkpoints` overview | STX obs |
| `stx_job_checkpoint_failed_total` | counter | 同上（失败次数累计） | STX obs |
| `stx_job_vertex_busy_ratio{vertex}` | gauge | `/metrics/realtime` vertices | STX obs + 引擎 obs |
| `stx_job_edge_backpressure_ratio{edge}` | gauge | `/metrics/realtime` edges | STX obs + 引擎 obs |
| `stx_job_up{status}` | gauge | 作业状态 | STX obs on |

注意：引擎累计值在作业恢复（新实例）从 0 开始，`job_id` 不同，不污染 `rate()`。

### 引擎可观测性判定

读 `submit_spec.observability`：`injected=true`，或 `reason` ∈ {`user_configured`,`already_injected`} → 视为开启；
`disabled_by_task` / `batch_job` / `version_*` 等 → 关闭。前端忙碌度 / 背压空态用同一判定（可从 job 详情带出）。

## 2. 查询

- `GET /api/v1/sync/jobs/:id/timeseries?range=1h&step=auto&metrics=throughput,rows,cdc_delay,checkpoint,busy,backpressure`
  权限同 `/realtime`（R0）。
- **STX observability 关闭或 Prometheus URL 不可用**：返回 HTTP 200 +
  `{source:"none", reason:"observability_disabled"|"prometheus_unavailable", series:[], annotations:[]}`，
  前端展示主空态（不 5xx，避免 toast 风暴）。
- 服务端按 metric key 选 PromQL 模板，调 Prometheus `query_range`；
  `step` 自动：15m→10s，1h→15s，6h→1m，24h→5m，7d→30m。
- **无 local / 内存环形缓冲**；`source` 仅为 `"prometheus"` 或 `"none"`。
- 响应：
  ```
  {source, step_seconds, range, engine_observability_enabled,
   series:[{key, label, unit, points:[[ts_ms, value]]}],
   annotations:[{ts_ms, kind, summary, event_id}]}
  ```
  annotations 取 `sync_engine_events`（状态、表结构变更）与重启类生命周期。
- CLI：`stx sync job timeseries <id> --range 1h --metrics throughput`（registry 生成）。

## 3. 前端：「指标」页签 快照 | 趋势

```
页签仍为：拓扑 | 指标 | Checkpoint | ...
指标页顶栏 segment：[快照 | 趋势]

快照 = 现有 MetricsDialogContent（不变）

趋势（Prometheus 可用）：
┌ 时间范围 [15m 1h 6h 24h 7d]  自动刷新 ⟳   来源: Prometheus   [在 Grafana 打开] ┐
├ 读 QPS │ 写 QPS │ 读写差 │ CDC 延迟 │ 最近 Checkpoint ┤  ← hero，当前值来自 job / checkpoint 轮询
├ 吞吐（读/写）          │ 累计条数（读/写/已提交）     ┤
├ CDC 延迟               │ Checkpoint 耗时（柱，失败红点）┤
├ 算子忙碌度             │ 背压（按边）                  ┤
└ 按表（第三期占位 / 可隐藏）                              ┘
```

- **主空态**（STX obs 关）：文案区分「需要开启可观测性并接入 Prometheus」；说明未开启时不采集不保存时序；
  CTA「去开启」→ 监控集成页；提示当前数值仍可在「快照」查看。（勿再提「本地 1 小时」。）
- **次空态**（仅忙碌度 / 背压卡）：引擎可观测性关 →「该作业未开启引擎可观测性」。
- 图表：recharts + shadcn chart；配色与 mockup 一致（读 `#3987e5`、写 `#d95926`、已提交 `#199e70`）。
- 只在趋势视图可见时请求 timeseries；运行中 30s 刷新，终态不轮询。
- 文案 zh / en。

## 4. 分期

1. **本期**：导出 + HTTP SD + 时序接口（仅 Prometheus）+ 指标「快照|趋势」
   （吞吐、条数、CDC、Checkpoint、忙碌度、背压、事件标记）+ CLI + 双开关空态。
2. （原二期内容已并入一期采集。）
3. 按表趋势 UI + 作业级 Grafana 面板 JSON（随 `deps/grafana_config/dashboards` 发布）。

## 5. 风险

- 标签基数：表级必须封顶；终态删除 `job_id` 序列。
- 引擎调用成本：Checkpoint 后台采集对「运行中」作业（仅需 STX obs）；realtime（busy/bp）仅对「运行中 + 引擎 obs 开」的作业；间隔 ≥30s，可配置关闭（后续）。
- 抓取端点公开模型：`/monitoring/prometheus/jobs/metrics` 与现有 HTTP SD / webhook 一样**刻意不加 LoginRequired**（Prometheus 抓取通常无 Bearer）；勿给该路径或 discovery 加登录。
- Prometheus 未抓到数据时曲线为空：前端用空 series + 来源徽章提示，不假装有本地数据。
