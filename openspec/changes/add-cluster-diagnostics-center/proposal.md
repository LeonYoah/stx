## Why

STX 目前已有监控中心、告警中心、进程事件、升级执行等能力，但“错误日志收集、集群巡检、诊断包”仍然分散且缺少统一入口，运维闭环不完整。随着后续要接入 AI 定时分析，需要尽快建立一套以 Agent 为主执行面、以结构化诊断证据为核心的数据与交互模型。

## What Changes

- 新增“诊断中心”产品域，提供全局入口并统一承载错误中心、巡检、诊断任务/诊断包能力。
- 新增 Seatunnel 错误中心能力：Agent 仅采集 ERROR 日志，兼容 `job-*.log` / routingAppender，并在控制台提供按集群、节点、Job、错误组的查看能力。
- 新增集群巡检能力：基于现有集群、监控、告警、进程事件数据执行手动/后续可扩展定时巡检，输出结构化巡检报告与发现项。
- 新增诊断包任务能力：围绕错误组、巡检失败项或人工触发，编排日志样本、线程栈、JVM dump、运行态快照等采集步骤，并以任务视图实时展示执行日志。
- 新增诊断联动：从告警、集群详情、错误详情、巡检结果之间可相互跳转，并沉淀供 AI 分析消费的统一诊断证据。

## Capabilities

### New Capabilities
- `cluster-error-center`: 面向 Seatunnel 集群的 ERROR 事件采集、错误组聚合、全局/集群级错误查看与跳转联动。
- `cluster-inspection`: 面向 Seatunnel 集群的手动巡检、巡检报告、问题发现与整改建议展示。
- `diagnostic-bundle-tasks`: 面向错误/巡检/人工触发的诊断任务与诊断包编排、执行日志、产物登记与来源回溯。

### Modified Capabilities
- None.

## Impact

- 后端：新增 `internal/apps/diagnostics/*` 领域模块，复用/扩展 gRPC `LogStream`、Agent 诊断命令、监控与告警服务。
- Agent：新增 Seatunnel ERROR 日志扫描与增量上报能力，复用已存在的 `COLLECT_LOGS`、`THREAD_DUMP`、`JVM_DUMP` 诊断命令。
- 前端：新增诊断中心工作台页面，并在集群详情页、告警中心中增加诊断快捷入口与联动跳转。
- 数据：新增错误事件/错误组/采集游标、巡检报告/发现项、诊断任务/步骤/日志/产物等模型。
- 约束：不引入额外日志组件；主链路依赖 Agent 直接采集，不以 SeaTunnel REST API 作为核心依赖。
