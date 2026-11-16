# P3 作业大盘

父任务：`11-14-home-monitoring-dashboard`。依赖 P1 首页骨架。

## Goal

在「作业」页签给出全局作业运行态：总吞吐、谁最快、谁延迟高、数据从哪流到哪、Checkpoint 与算子瓶颈。

## Requirements

- 全局吞吐：读 / 写合计，行 / 秒与字节 / 秒切换；叠加事件标注（作业失败、重启、表结构变更），来自 `sync_engine_events` 与作业状态变更。
- 写入速率 Top 8、CDC 延迟 Top 8（阈值线取 CDC 延迟告警配置，未配置时 60 秒）；点击条目打开行预览抽屉（复用 P1）。
- 数据流向：源连接器 → 目标连接器，按时间窗内写入行数（后端按 job_id 关联提交配置的插件名聚合）。
- 运行日历：近 16 周每日运行 / 失败次数（DB，无需 Prometheus）。
- Checkpoint：时间窗内各作业最大耗时与失败次数。
- 算子热点：忙碌度 / 背压最高的算子（仅引擎可观测性开启的作业，表头注明）。
- 后端 `GET /monitoring/dashboard/jobs`、`GET /sync/jobs/daily-stats` + CLI。
- 聚合数据所有登录用户看全局；条目带 `viewable`，无权查看的作业不可点开（父设计 §4）。

## Acceptance Criteria

- [ ] 全局吞吐与各作业详情趋势之和一致（抽样核对）。
- [ ] Top N 中的作业名、任务名、集群名由后端补齐，前端不再逐个请求。
- [ ] 无 Prometheus 时仅运行日历出图，其余为统一空态。
- [ ] 非管理员视角下聚合为全局数据，无权查看的 Top N 条目 `viewable=false` 且不可点开（有测试）。
- [ ] `stx monitoring dashboard jobs --range 6h`、`stx sync job daily-stats --weeks 16` 可用。
