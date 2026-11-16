# P2 设计：集群大盘

总体契约见父任务 `design.md`；依赖 P1 的首页骨架（`HomeDashboard`、`usePolledResource`、`useDashboardQuery`、`promquery`）。
分支 `feat/home-dashboard-p2` 叠在 `feat/home-dashboard-p1`（PR #137）之上。

## 1. 接口

`GET /api/v1/dashboard/home/clusters?cluster_id=&range=1h|...&start=&end=`（CLI `stx dashboard home clusters`）。

放在 `internal/apps/dashboard`（与首页汇总同处）：它已经依赖 cluster 模块，集群健康 / 节点来自 monitoring 与 cluster service，
运行中作业数来自 sync（接口注入，与 P1 的 `HomeJobSummaryProvider` 同法）。

```jsonc
{
  "source": "prometheus" | "none", "reason": "...", "range": "1h", "start_ms": 0, "end_ms": 0, "step_seconds": 75,
  "clusters": [ { "id": 1, "name": "...", "version": "3.0.0", "health_status": "healthy|degraded|unhealthy|unknown",
                  "nodes_online": 1, "nodes_total": 1, "running_jobs": 2, "active_alerts": 3, "critical_alerts": 3,
                  "max_heap_percent": 34.5, "endpoints_down": 0 } ],
  "nodes": [ { "cluster_id": 1, "cluster_name": "...", "node_id": 3, "instance": "10.66.0.2:5801", "role": "hybrid",
               "status": "running", "agent_online": true, "master": true,
               "scrape_up": true, "node_active": true, "heap_percent": 34.5, "cpu_cores": 0.12, "threads": 141,
               "fd_percent": 0.07, "uptime_seconds": 86400 } ],
  "signals": { "clusters_safe": 1, "clusters_total": 1, "deadlocked_threads": 0, "fd_percent_max": 0.07,
               "pool_reject_rate": 0, "hz_queue_util_max": 0, "endpoints_down": 0 },
  "series": [ { "key": "availability|heap|cpu|gc|job_state", "label": "10.66.0.2:5801 | running", "points": [[ms, v]] } ],
  "partial_errors": []
}
```

- 节点 ↔ Prometheus 对应：`instance = HostIP:HazelcastPort`，与 HTTP SD 生成目标的规则一致（`collectManagedMetricsTargets`）。
  `master` 取 `cluster_info{master=...}`。
- 无 Prometheus：`clusters` / `nodes` 只含 DB / 心跳字段，指标字段缺省，`series` 为空数组，`signals` 为 null。
- `cluster_id` 为空时汇总全部集群。

## 2. PromQL（不依赖 STX 录制规则，直接用原始表达式；外部 Prometheus 未加载规则也能出图）

`{sel}` = `job="seatunnel_engine_http"` + 可选 `cluster_id="N"`。

| 键 | 查询 |
|---|---|
| availability | `min by (instance) (min_over_time(up{sel}[$cell]))` 与 `min by (instance) (min_over_time(node_state{sel}[$cell]))`，后端合成：up≥1 且（node_state 缺失或 =1）为 1，否则 0 |
| heap | `100 * max by (instance) (jvm_memory_bytes_used{sel,area="heap"}) / max by (instance) (clamp_min(jvm_memory_bytes_max{sel,area="heap"},1))` |
| cpu | `sum by (instance) (rate(process_cpu_seconds_total{sel}[5m]))`（核） |
| gc | `100 * sum by (instance) (rate(jvm_gc_collection_seconds_sum{sel}[5m]))` |
| job_state | `sum by (type) (job_count{sel})` |
| 节点即时值 | heap / cpu 同上 instant；`jvm_threads_current`、`100*process_open_fds/clamp_min(process_max_fds,1)`、`time()-process_start_time_seconds`、`up`、`node_state`、`cluster_info` |
| signals | `min by (cluster_id) (hazelcast_partition_isClusterSafe)`、`max(jvm_threads_deadlocked)`、`sum(rate(job_thread_pool_rejection_total[5m]))`、`max(100*hazelcast_executor_queueSize/clamp_min(queueSize+queueRemainingCapacity,1))`、`count(up==0)` |

可用性的 `$cell` = 窗口 / 48，step 同值，前端每个点对应一格，不再二次分桶。

时间窗解析（`range` 或 `start/end`、跨度上限、自动 step）从 sync 迁到 `promquery`，作业趋势与集群大盘共用。

## 3. 前端

- 首页头部加时间窗分段控件（15 分钟 / 1 小时 / 6 小时 / 24 小时 / 7 天），写 URL `range`；只在集群（及 P3 作业）页签显示。
- `ClusterTab`：集群卡片（全部 + 每个集群，点击切换范围，与头部范围联动）→ 节点可用性（beUI `StatusBar`，每节点一行）→
  2×2 趋势（Arc `LineChart`：堆 85% 参考线 / CPU / GC；beUI `CompositionChart`：引擎作业状态构成）→ 节点表（`StudioDataTable`）+ 异常信号。
- 无 Prometheus：卡片与节点表照常（DB 字段），趋势块统一 `TrendEmptyState`（Arc `EmptyState` + 开启指引）。
- 首页移除 `MonitoringOverview` 的 Grafana iframe（代理与安装组件 P4 再删）。
- 引入 beUI `status-bar` / `composition-chart` 及共用 tooltip 依赖链（`components/motion/tooltip*`、`lib/hooks/use-dismiss|use-hover-gesture|use-tap-gesture`、`lib/touch`），
  `@floating-ui/dom` 改为直接依赖（已在锁文件里随 Radix 存在）；i18n 指向 `components/charts/beui-i18n.tsx`；MIT 头 + 白名单。
- 配色：节点按实例名固定取 `--series-*`（跟实体不跟排名）；作业状态构成用状态语义色（运行中 info、完成 success、失败 danger、取消 muted）。

## 4. 测试

- Go：可用性合成、节点 ↔ instance 映射与 master 判定、无 Prometheus 分支、集群范围过滤、单查询失败记 `partial_errors`（`httptest` 假 Prometheus）。
- 前端：`ClusterTab` 有 / 无 Prometheus、集群卡片切换范围、节点表空态。
