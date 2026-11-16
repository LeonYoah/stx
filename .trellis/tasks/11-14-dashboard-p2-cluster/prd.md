# P2 集群大盘（替换 Grafana iframe）

父任务：`11-14-home-monitoring-dashboard`。依赖 P1 的首页骨架（页签容器、范围 / 时间窗状态、`TrendEmptyState`）。

## Goal

用 STX 自己的集群页签替代首页底部的 Grafana iframe，覆盖原 Grafana P0–P4 面板中的关键信号。

## Requirements

- 集群卡片：每个集群一张（名称、版本、健康状态、节点在线 / 总数、运行作业数、最高堆使用率、告警数），外加「全部集群」卡；点击切换范围（与顶部范围选择联动）。
- 节点可用性：每个节点一条 beUI status-bar，按时间窗切 48 格，抓取失败或节点非 ACTIVE 标红。
- 趋势：JVM 堆使用率（85% 参考线）、进程 CPU、GC 时间占比、引擎作业状态构成。
- 节点表：节点、集群、角色、状态、堆、CPU、线程、FD、运行时长（DB 节点表 + Prometheus instant 值；无 Prometheus 时只显示 DB 字段）。
- 异常信号：分区安全、死锁线程、FD 使用率、线程池拒绝、Hazelcast 队列利用率、指标端点可达性；全部正常时折叠为一行。
- 后端 `GET /monitoring/dashboard/cluster` + CLI；`seatunnel:*` 录制规则缺失时回退原始表达式。
- 首页移除 `MonitoringOverview` 中的 Grafana iframe（代理与安装组件留到 P4）。

## Acceptance Criteria

- [ ] 与 Grafana 原面板同一时间窗对比，堆 / CPU / GC / 作业构成曲线一致（截图对比记录在任务目录）。
- [ ] 外部 Prometheus 未加载录制规则时，堆 / GC 仍能出图。
- [ ] 集群指标端点不可达时，该集群卡片与异常信号给出明确提示，不影响其他集群。
- [ ] 无 Prometheus 时集群卡片与节点表仍显示 DB / 心跳数据，趋势显示统一空态。
- [ ] `stx monitoring dashboard cluster --cluster_id 1 --range 1h` 可用。
