# E1 设计：集群指标细化分组

## 接口

新增 `GET /api/v1/dashboard/home/clusters/detail`（不复用 `/clusters` 加参数——两种响应形状放同一接口是坏契约；「接口加 group 参数」即此接口的必传 `group`）。

参数：`cluster_id`、`range|start&end`（复用 promquery 窗口）、`group` ∈ `jvm|gc|sched|store|proc`（必传，懒加载：前端只在展开分组时请求该组）。

响应 `ClusterDetail`：`source/reason/range/start_ms/end_ms/step_seconds/group/panels/events/partial_errors`。

```go
DetailSeries   {instance, label, points}   // instance 决定颜色；label=维度名（pool/gc/state/type/store/direction）
DetailPanel    {key, unit, series}         // unit ∈ bytes|percent|count|per_second|ms
DetailEvent    {ts_ms, kind, label}        // kind ∈ node_restart|members_change
ClusterDetail  {…公共字段, group, panels, events}
```

事件由 Go 侧对 range 序列做差分检测：`process_start_time_seconds` 值上升 = 节点重启；`node_count` 变化 = 成员数变化。原始表达式，不新增录制规则。前端把事件渲染为详情区分组标题旁的事件条 + LineChart 图例说明。

## 分组 → 查询（`{sel}` = `job="seatunnel_engine_http"` + 可选 `cluster_id`）

**jvm**（4 面板）：`heap_pools` `sum by (instance,pool)(jvm_memory_pool_bytes_used{pool=~".*(Eden|Survivor|Old|Tenured).*"})`；`nonheap` `…{pool=~"Metaspace|Code.*|Compressed Class Space"}`；`buffer` `sum by (instance,pool)(jvm_buffer_pool_used_bytes)`；`heap` `sum by (instance)(jvm_memory_bytes_used{area="heap"})` + `…committed`。

**gc**（5 面板）：`gc_rate` `sum by (instance,gc)(rate(jvm_gc_collection_seconds_count[5m]))`；`gc_pause` `100*sum by (instance,gc)(rate(jvm_gc_collection_seconds_sum[5m]))`；`threads` current/daemon/peak 三条合并为维度；`thread_states` `sum by (instance,state)(jvm_threads_state{state=~"BLOCKED|WAITING|TIMED_WAITING"})`；`deadlocked` `jvm_threads_deadlocked` + `_monitor`。

**sched**（6 面板）：`pool` `job_thread_pool_{poolSize,activeCount,queueTaskCount}` 三查询合并；`reject` `rate(job_thread_pool_rejection_total[5m])`；`op_latency` `request_slot_operation_last_invocation_latency_ms` + `report_metrics_operation_last_invocation_latency_ms`（ms）；`hz_queue` `sum by (instance,type)(hazelcast_executor_queueSize)`；`partitions` `hazelcast_partition_partitionCount` + `_activePartition`；`members` `max by (cluster_id)(node_count)`，参考线 = DB 节点数。

**store**（3 面板）：`store_owned` `sum by (instance,store)(engine_state_store_local_owned_entries)`；`store_backup` `…_local_backup_entries`；`store_heap` `…_local_heap_cost_bytes`（bytes）。
（真实环境核实：`engine_state_store_entries` 不存在；本地引擎有 `_local_owned_entries` / `_local_backup_entries` / `_local_heap_cost_bytes` 三个真实序列。）

**proc**（4 面板）：`offheap` `process_resident_memory_bytes` 与 `rss - heap`（`on(instance)` 减法）；`fds` `process_open_fds` + `process_max_fds`；`classes` `jvm_classes_currently_loaded`；`class_load_rate` `rate(jvm_classes_loaded_total[5m])`。
（真实环境核实：`process_network_*` 系列在现引擎版本中不存在，网络面板已移除。）

## 执行模型

复用 `promTask` 并发查询 + `dashboardPromCache` + `partial_errors` 降级；`source=none` 时返回空壳（与概览一致）。事件检测查询与其他查询同批并发。

## 前端

ClusterTab 尾部加「节点指标细节」折叠区：SegmentedControl 选组 → `getClustersDetail` 懒加载；每组面板网格 `md:grid-cols-2 xl:grid-cols-3`，LineChart 高 160px，实例色沿用 `colorByName`，`instance · dim` 图例。事件条在组头部。`source=none` 走 `TrendEmptyState`；空组显「该集群未上报此组指标」。

## CLI

`stx dashboard home cluster-detail --group jvm`（operation `dashboard.home.cluster-detail`）。
