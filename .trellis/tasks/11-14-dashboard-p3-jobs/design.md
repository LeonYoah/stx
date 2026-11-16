# P3 设计：作业大盘 + 首页自定义时间窗

依赖 P2；分支 `feat/home-dashboard-p3` 叠在 `feat/home-dashboard-p2`（PR #138）之上。

## 1. 首页时间选择器（集群 / 作业页签共用）

- 头部：时间窗分段控件（15 分钟 ~ 7 天）+ `ArcDateRangePicker withTime`（精确到分钟），与作业详情趋势一致。
- URL：预设写 `range`；自定义写 `start` / `end`（毫秒），此时分段控件无选中项；点预设清掉 `start` / `end`。跨度上限 31 天（前端截取靠近结束的一段，后端同样校验）。
- 自定义窗口固定不变，不自动刷新（`usePolledResource` 间隔为 0 时只拉一次）。
- 两个大盘接口都收 `start` / `end`（`promquery.ParseWindow`）。

## 2. 接口

`GET /api/v1/dashboard/home/jobs?cluster_id=&range=&start=&end=&weeks=16`（CLI `stx dashboard home jobs`）。

```jsonc
{
  "source": "prometheus" | "none", "reason": "", "range": "1h", "start_ms": 0, "end_ms": 0, "step_seconds": 15, "window_seconds": 3600,
  "throughput": [ { "key": "read|write|read_bytes|write_bytes", "points": [[ms, v]] } ],
  "annotations": [ { "ts_ms": 0, "kind": "job_failed|job_recovered|schema_change", "job_id": 1, "task_id": 2, "title": "...", "viewable": true } ],
  "top_write": [ { "job_id": 1, "task_id": 2, "name": "...", "cluster_id": 1, "value": 123.4, "viewable": true } ],  // 时间窗内平均写入速率
  "top_cdc":   [ ... ],                                                                                                // 时间窗内最大 CDC 延迟（秒）
  "flow": [ { "source": "MySQL-CDC", "sink": "Doris", "value": 1000 } ],                                               // 时间窗内写入行数
  "checkpoints": [ { ...JobRank, "max_seconds": 3.2, "failures": 0 } ],
  "hotspots": [ { ...JobRank, "kind": "busy|backpressure", "vertex": "Sink[0]", "value": 0.82 } ],
  "calendar": { "weeks": 16, "end_date": "2026-11-15", "runs": [[7 个数] x weeks], "failed": [[...]] },             // DB，始终返回
  "partial_errors": []
}
```

- Top N / Checkpoint / 热点用**窗口聚合**（`increase(...[span])/span`、`max_over_time(...[span])`），而不是终点瞬时值：
  自定义历史窗口与已结束作业也能出数（作业终态后序列删除，但窗口内的历史样本仍在）。
- 数据流向：`sum by (job_id) (increase(stx_job_sink_write_rows_total[span]))`，按 job_id 关联 `result_preview.job_dag.vertexInfoMap`
  的顶点名（`pipeline-1 [Source[0]-MySQL-CDC]`、`[Sink[0]-Doris-MultiTableSink]`）解析源 / 目标插件；一个作业多个目标时平均分摊。
  解析不到时归入「未知」。
- 运行日历：DB 统计近 `weeks` 周（≤52）每日运行 / 失败次数，按显示时区分天，排除预览运行。
- 事件标注：窗口内失败的作业（按结束时间）、恢复运行（`run_type=recover` 按开始时间）、表结构变更事件（`sync_engine_events` 的 `SCHEMA_CHANGE*`），各最多 50 条。
- 聚合为全局口径；作业条目带 `viewable`（父设计 §4）。
- 放在 `internal/apps/dashboard/jobs.go`；作业名、插件、日历、标注由 sync 提供（接口注入）。

## 3. 前端 `JobsTab`

- 全局吞吐：Arc `BrushChart`（行 / 秒与字节 / 秒分段切换，事件标注）。BrushChart 只画一条序列，读 / 写用分段切换（默认写入）。
- 写入 Top 8、CDC 延迟 Top 8：beUI `BarChart` horizontal；CDC 阈值写在标题说明里。条目可点（viewable）→ 跳到作业列表并搜索该引擎作业。
- 数据流向：beUI `SankeyChart`；运行日历：beUI `HeatCalendar`（运行 / 失败分段切换）。
- Checkpoint：Arc `BarChart`（最大耗时，失败次数写进 label）；算子热点：小表格。
- 无 Prometheus：只有运行日历出图，其余用 `TrendEmptyState`。

## 4. 测试

Go：顶点名解析、流向分摊、窗口聚合查询拼装、日历分天、标注、无 Prometheus；前端：JobsTab 有 / 无 Prometheus、时间选择器写 URL 与清除。
