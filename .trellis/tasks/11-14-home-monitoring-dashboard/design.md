# 设计：首页监控大盘 + 全局作业列表

本文是四个子任务共享的技术设计：边界、接口契约、数据来源、组件映射与上线 / 回退方式。
各子任务的执行步骤写在各自的 `implement.md`。

## 1. 边界与数据流

> 2026-11-16 更新：可观测改为内嵌单模式（`11-16-embedded-observability`）。下图中的「Prometheus」现为 STX 进程内的
> `tsdb` + `promql` 引擎，抓取由 STX 自研循环完成（`job="seatunnel_engine_http"` 标签不变），`stx_job_*` 直写 tsdb（`job="stx_sync_jobs"`）。
> 大盘代码只依赖 `promquery`，签名不变。下文出现的「Prometheus」均按此理解。

```
浏览器 ──► STX API ──┬─► DB（sync_job_instances / sync_tasks / clusters / hosts / alerts）   当前值、列表、日历
                    └─► Prometheus /api/v1/query(_range)                               趋势、Top N、节点指标
                          ▲ 抓取：seatunnel_engine_http（cluster_id 标签）+ stx_sync_jobs（STX 导出 stx_job_*）
```

- 前端只调用 STX API；PromQL 模板全部在后端（与 `job_timeseries.go` 一致），前端传指标键、范围、集群，不传 PromQL。
- 公共 Prometheus 访问抽到一个小包（建议 `internal/pkg/promquery`）：`QueryRange`、`QueryInstant`、样本解析、`source/reason` 判定。
  `job_timeseries.go` 现有的 `queryPrometheusRange` / `coercePromSample` 迁过去复用，行为不变。
- 不新增表；时序只在内嵌 tsdb 中。大盘接口做 ≤15 秒的进程内查询缓存（只缓存 PromQL 结果，逐用户字段在缓存外装饰；P4 落地），
  内嵌后查询消耗的是 STX 自身 CPU，避免多人同时打开首页时放大计算。

## 2. 统一的响应外壳（大盘类接口）

```jsonc
{
  "error_msg": "",
  "data": {
    "source": "prometheus" | "none",          // none = 未开启或不可达
    "reason": "" | "observability_disabled" | "prometheus_unavailable" | "prometheus_error",
    "range": "1h", "start_ms": 0, "end_ms": 0, "step_seconds": 60,
    "current": { ... },                       // 当前值：始终有（来自 DB / 心跳 / 对账）
    "series": [ { "key": "heap", "label": "10.66.0.2:5801", "group": "node", "points": [[ts_ms, value], ...] } ],
    "rankings": { "write_rate": [ { "job_id": 1, "task_id": 2, "name": "...", "value": 123.4 } ] },
    "annotations": [ { "ts_ms": 0, "kind": "job_failed", "job_id": 1, "label": "..." } ]
  }
}
```

- `source:"none"` 时 `series / rankings` 为空数组（不是 null），`current` 仍完整；前端据 `reason` 显示统一空态。
- 时间窗参数与作业趋势接口一致：`range=15m|1h|6h|24h|7d` 或 `start/end`（RFC3339 / 秒 / 毫秒，跨度 ≤31 天），`step` 缺省自动。
  直接复用 `ParseJobTimeseriesWindow` 与 `autoTimeseriesStep`（迁入公共包）。
- 单个查询失败不让整个接口失败：该序列缺失并在 `data.partial_errors[]` 记录键名，HTTP 仍 200。

## 3. 接口清单

| 子任务 | 方法 / 路径 | 说明 | CLI |
|---|---|---|---|
| P1 | `GET /api/v1/sync/jobs`（扩展） | 新增 `status`（逗号分隔）、`mode`、`cluster_id`、`keyword`、`sort`（`attention`\|`started_at`\|`rate`\|`total`\|`cdc_delay`）、`order`；响应项增加 `task_name`、`cluster_id`、`cluster_name`、`live`（当前读写速率、累计、CDC 延迟、最近 checkpoint）。旧参数与默认排序保持兼容。 | `stx sync job list` 自动多出新参数 |
| P1 | `GET /api/v1/sync/jobs/sparklines` | `ids=1,2,3`（≤100）`&range=1h`；一条 `sum by(job_id)(rate(stx_job_sink_write_rows_total{job_id=~"..."}[1m]))` 批量返回 | `stx sync job sparklines --ids 1,2 --range 1h` |
| P1 | `GET /api/v1/dashboard/home/summary` | 顶部指标卡与「需要处理」条：DB 计数 + 告警 + 对账当前值；有 Prometheus 时附带写入速率 / CDC 延迟的 1h 迷你序列 | `stx dashboard home summary` |
| P2 | `GET /api/v1/monitoring/dashboard/cluster` | `cluster_id`（可空 = 全部）+ 时间窗；`current` = 集群卡片 + 节点表 + 异常信号，`series` = 可用性 / 堆 / CPU / GC / 作业状态构成 | `stx monitoring dashboard cluster` |
| P3 | `GET /api/v1/monitoring/dashboard/jobs` | `cluster_id` + 时间窗；全局吞吐、Top N、流向、Checkpoint、算子热点、事件标注 | `stx monitoring dashboard jobs` |
| P3 | `GET /api/v1/sync/jobs/daily-stats` | `weeks=16`（≤52）、`cluster_id`；按天统计运行 / 失败次数（DB） | `stx sync job daily-stats` |

路由前缀遵循现有分组：作业类挂在 `syncJobRouter`（注意 `/sparklines`、`/daily-stats` 必须注册在 `/:id` 之前），
监控类挂在 `monitoringRouter`，首页汇总挂在现有 `/dashboard/overview` 旁的 `/dashboard/home`。

## 4. 权限（R8）

- 作业列表：沿用 `ListJobsForActor`。管理员看全部；非管理员看 `created_by = self` 以及其有权查看的任务下的作业（与 `getJobInstanceForActor` 判定一致，P1 中把列表的可见范围补齐到同一规则）。
- 作业类聚合（summary 计数与速率、作业大盘、daily-stats）：所有登录用户看全局，不按用户过滤（用户确认）。
  Top N / 流向等返回的作业条目附 `viewable: bool`（按 actor 判定），前端对 `false` 的条目只展示名称与数值，不提供点开预览。
- 集群大盘：登录即可看（与集群列表一致）。
- `sparklines` 的 `ids` 先与可见集合求交集，越权 id 静默丢弃。

## 5. PromQL 模板（服务端常量）

集群（`{cluster}` = `cluster_id="N"` 或省略）：

| 键 | 查询 | 来源面板 |
|---|---|---|
| availability | `min by(instance)(up{job="seatunnel_engine_http",{cluster}})` 与 `min by(instance)(node_state{...})` 合成 | P0 Up / 节点状态 |
| heap | `seatunnel:jvm_heap_usage_percent{{cluster}}`（by instance） | P1 |
| cpu | `rate(process_cpu_seconds_total{job="seatunnel_engine_http",{cluster}}[5m])` | P1 |
| gc | `seatunnel:gc_time_ratio_percent5m{{cluster}}` | P1 |
| job_state | `sum by(type)(job_count{job="seatunnel_engine_http",{cluster}})` | P2 |
| signals（instant） | `hazelcast_partition_isClusterSafe`、`jvm_threads_deadlocked`、`seatunnel:fd_usage_percent`、`seatunnel:job_thread_pool_reject_rate5m`、`seatunnel:hazelcast_executor_queue_util_percent` | P0 / P3 / P4 |

实现上一律写原始表达式，不依赖 `seatunnel:*` 录制规则（内嵌方案不实现录制规则；表中录制名只表示语义来源）。

作业（`{scope}` = `cluster_id` 与可见 `job_id` 过滤）：

| 键 | 查询 |
|---|---|
| throughput_read / write | `sum(rate(stx_job_source_received_rows_total{{scope}}[1m]))` / `..._sink_write_rows_total...` |
| bytes_read / write | 同上换 `_bytes_total` |
| top_write | `topk(8, sum by(job_id)(rate(stx_job_sink_write_rows_total{{scope}}[1m])))`（instant） |
| top_cdc | `topk(8, max by(job_id)(stx_job_cdc_delay_seconds{{scope}}))`（instant） |
| flow | `sum by(job_id)(increase(stx_job_sink_write_rows_total{{scope}}[$range]))`，后端按 job_id 关联提交配置中的源 / 目标插件名再聚合 |
| checkpoint | `max by(job_id)(max_over_time(stx_job_checkpoint_duration_seconds{{scope}}[$range]))` + `increase(stx_job_checkpoint_failed_total[$range])` |
| hotspots | `topk(5, stx_job_vertex_busy_ratio{{scope}})`、`topk(5, stx_job_edge_backpressure_ratio{{scope}})`（instant） |

Top N 返回 `job_id` 后，后端用 DB 补作业名 / 任务名 / 集群名，前端不再二次请求。

## 6. 前端结构

```
app/(main)/dashboard/page.tsx                 // 只做页面装配
components/common/dashboard/
  HomeDashboard.tsx                           // 顶部 + 页签容器，范围 / 时间窗状态放 URL（?tab=&cluster=&range=）
  DashboardKpiRow.tsx  AttentionStrip.tsx
  cluster/ClusterTab.tsx ...                  // P2
  jobs/JobsTab.tsx ...                        // P3
  list/JobListTab.tsx  JobPreviewDrawer.tsx   // P1
  TrendEmptyState.tsx                         // 统一「需要 Prometheus」空态
lib/services/dashboard/home.service.ts        // 走 BaseService / apiClient
```

- 范围与时间窗写入 URL query，刷新与分享不丢；页签切换只渲染当前页签（与作业详情一致）。
- 刷新：默认 30 秒轮询当前页签数据，页面不可见时暂停；时间窗 ≥24h 时降到 60 秒。
- 「打开作业详情」跳 `/workbench?task=<task_id>&job=<job_id>`（已有深链）。`JobDetailDialog` 依赖工作台插槽（日志、Checkpoint），首页不复刻。
- 遵循 ui-conventions：`WorkspaceHeader`、零塌陷加载（骨架高度与成品一致）、原子信息不换行、`pb-28` 让出 Dock。
  作业列表的状态筛选按规范用 `StatPillsBar`（原型里画成了胶囊组，实现时以规范为准）。

### 组件映射

| 区域 | 组件 | 状态 |
|---|---|---|
| 指标卡 | Arc `metric-card` + beUI `sparkline` | 待安装 |
| 时间窗 / 单位切换 | Arc `segmented-control` | 已安装 |
| 范围选择 | Arc `combobox` | 已安装 |
| 节点可用性 | beUI `status-bar` | 待安装 |
| 堆 / CPU / GC / 吞吐 | Arc `line-chart`；全局吞吐用 Arc `brush-chart`（事件标注） | 已安装 |
| 作业状态构成 | beUI `composition-chart` | 待安装 |
| Top N | beUI `bar-chart`（horizontal） | 待安装 |
| 数据流向 | beUI `sankey-chart` | 待安装 |
| 运行日历 | beUI `heat-calendar` | 待安装 |
| Checkpoint | Arc `bar-chart` | 已安装 |
| 作业列表 | 现有 `StudioDataTable`（`components/common/sync/StudioDataTable.tsx`：HeroUI Table 解剖 + Arc token，粘性表头、列排序、表内空态、底栏分页）+ beUI `sparkline` | 表格已有；sparkline 待安装 |
| 空态 | Arc `empty-state` | 待安装 |
| 状态筛选 | 现有 `StatPillsBar` | 已有 |

新装组件按 License 门禁处理：保留上游 MIT 头并登记 `licenses/third_party_mit_files.txt`（同 PR #135）。
beUI 组件依赖 `motion`（已在依赖中）。安装后先确认是否引入新的运行时依赖，有则在 PR 中列出。

## 7. 配色

图表按 dataviz 规范：分类色固定顺序，状态色（good / warning / serious / critical）只表示状态并配图标与文字；
节点 / 作业等实体颜色跟随实体而非排名。暗色与亮色各自取色，不做自动反转。
落地时把分类色映射到 `globals.css` 的 `--chart-*` 变量，先跑 dataviz 验色脚本再定稿。

## 8. 兼容、上线与回退

- `GET /sync/jobs` 只加可选参数与字段，旧调用方（工作台、CLI、引导）不受影响；默认排序仍是 `created_at DESC`。
- P1 上线后首页新骨架与旧四宫格并存一个版本：旧内容仍在「集群」页签位置之前展示（P2 再替换），回退只需还原 `dashboard/page.tsx`。
- P2 移除首页 Grafana iframe；P4 删除代理、`observability.grafana` 配置与前端残留，安装器 / docker 中的 Grafana 随三件套由内嵌任务删除。
- 按内嵌任务的破坏性升级决定，`observability.grafana` 直接删除、不打废弃 warn；配置加载非 Exact，旧文件残留该段不影响启动。
