# 增强：集群指标细化

父任务：`11-14-home-monitoring-dashboard`。排期：P1–P4 验收完成之后（2026-11-16 用户确认：这些是增强）。

**依赖**：内嵌可观测 P2 已合并（`11-16-embedded-observability`）。数据来自内嵌抓取器写入 tsdb 的 SeaTunnel 全量 exposition；
Summary / Histogram 已展平为 `_bucket` / `_sum` / `_count`，抓取间隔默认 15 秒。

## 背景

集群页签目前只用了堆、CPU、GC、可用性、引擎作业状态。SeaTunnel 引擎暴露的 90+ 指标由内嵌抓取器全部写入 tsdb（`job="seatunnel_engine_http"`，标签同原 HTTP SD：
`instance` / `cluster_id` / `cluster_name` / `cluster` / `env`），运维除排障外主要靠看监控，细节不能缺。**不需要新增采集**。

## Requirements

- 集群页签首屏保持现有概览；下方加分段切换的细节分组，每组按节点 4–6 张图，关键图带参考线：
  - JVM 内存：各内存池（Eden / Old / Survivor / Metaspace / Code Cache）、非堆、直接内存与 mapped 缓冲区（`jvm_buffer_pool_*`）。
  - GC 与线程：按回收器拆分的次数速率与停顿占比；线程当前 / 守护 / 峰值、按状态分布（BLOCKED / WAITING）、死锁。
  - 调度与 Hazelcast：协调器线程池活跃 / 容量 / 队列 / 拒绝速率；`request_slot_operation_*`、`report_metrics_operation_*` 延迟；
    Hazelcast 执行器队列（按类型）、分区数 / 活跃分区、成员数（`node_count`）与 DB 节点数是否一致。
  - 状态存储：进行中 Checkpoint、保留历史、IMap 本地堆占用（`engine_state_store_local_heap_cost_bytes`）、已结束作业记录数。
  - 进程与网络：RSS 与堆差值（堆外增长）、进程网络收发（`process_network_*_bytes_total`）、FD、类加载数。
- 趋势叠加事件：节点重启（`process_start_time_seconds` 变化）、集群成员数变化。
- 继续遵循：一律原始表达式（内嵌方案不实现录制规则）；可观测性关闭或内嵌存储不可用时该区块按 `reason` 统一空态；
  时间窗 / 自定义区间与头部联动；新增查询接入 P4 的大盘查询缓存。
- 分组按需加载：只在展开对应分组时请求该组查询（接口加 `group` 参数），避免集群页签单次查询数从 ~19 条翻倍。

## Acceptance Criteria

- [ ] 每个分组至少覆盖上表指标，按节点出图，颜色跟实例名。
- [ ] 节点重启与成员数变化在图上可见（用测试数据或真实重启验证）。
- [ ] 后端新增查询仍并发执行、单条失败记 `partial_errors`；CLI 与接口同步。
