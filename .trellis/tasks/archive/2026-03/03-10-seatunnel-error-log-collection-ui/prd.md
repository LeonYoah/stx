# Seatunnel 错误日志收集与界面查看

## Goal

为已深度集成 Seatunnel 集群的 Agent 增加一套“仅采集 ERROR 日志”的收集与查看能力，支持在 STX 控制台中按集群 / 节点 / Job 查看错误日志与错误聚合结果，并为后续 AI 定时分析提供稳定、低成本的数据输入。

本任务优先遵循以下已确认原则：

- 主链路基于 Agent 侧直接采集，不依赖 Seatunnel REST API；
- 不采集全量原始日志，仅关注 `ERROR`；
- 需要兼容 Seatunnel `routingAppender` 场景，即错误可能落在 `job-*.log` 中，而不只在 `seatunnel-engine-worker.log` 中；
- 设计保持简洁，不引入额外日志中间件或复杂组件。

---

## Requirements

### R1. 采集链路基于 Agent

- 利用现有 Agent 对 Seatunnel 集群的深度集成能力，在 Agent 端直接扫描日志目录；
- 默认扫描：
  - `seatunnel-engine-*.log`
  - `job-*.log`
- 需要支持日志轮转与增量读取（inode + offset 或等价机制）。

### R2. 仅采集 ERROR

- 初版仅采集 `ERROR` 级别日志；
- `WARN` 暂不采集；
- 不保存全量原始日志，只保存错误事件及其必要上下文。

### R3. 兼容 routingAppender

- 不能假设错误都在 engine worker/master 主日志中；
- 当启用 log4j2 `routingAppender` 时，必须支持从 `job-*.log` 中识别并采集错误；
- 若无法直接从文件名获得 jobId，也应支持从日志内容中提取或留空。

### R4. 后端只存错误事件与聚合结果

- 至少需要区分：
  - 单条错误事件（event）
  - 错误聚合组（group / fingerprint）
  - 文件读取游标（cursor）
- 错误详情页若需查看更多上下文，应优先使用“事件附近内容”或有限样本，而不是全量原文归档。

### R5. 前端提供错误查看界面

- 提供错误列表页：按集群、节点、时间、jobId、异常类等维度筛选；
- 提供错误详情页：展示错误摘要、堆栈、首次/最近出现时间、次数、影响节点范围；
- 页面设计应以“错误定位”和“给 AI 提供结构化输入”为目标，而不是做通用日志平台。

### R6. 面向后续 AI 分析的数据准备

- 需要为后续定时 AI 分析保留稳定结构：
  - 错误指纹
  - 代表样本
  - 关联集群 / 节点 / job
  - 时间窗口聚合结果
- 不要求本任务内直接实现 AI 分析，但数据结构必须兼容后续接入。

### R7. 保持低复杂度与低性能开销

- 不引入额外中间件；
- Agent 批量上报，后端批量写入；
- 控制台默认优先展示错误组，不直接扫描海量原始日志；
- 必须有基础的去重与保留策略。

---

## Acceptance Criteria

- [ ] Trellis 任务明确记录：主链路为 Agent 采集，不依赖 Seatunnel API
- [ ] 文档明确约束：初版只采集 ERROR，不采 WARN，不存全量原始日志
- [ ] 文档明确兼容 `routingAppender` 与 `job-*.log`
- [ ] 文档给出后端最小存储模型（event / group / cursor）
- [ ] 文档给出前端最小页面结构（列表 / 详情）
- [ ] 文档给出后续 AI 分析所需的最小输入字段
- [ ] 方案不依赖新增外部日志组件

---

## Technical Notes

### Confirmed constraints from discussion

- Agent 已深度集成 Seatunnel 集群，因此日志采集应直接走 Agent 侧文件读取；
- 不需要将 Seatunnel REST API 作为主链路；
- Seatunnel 的日志模式存在 `routingAppender` 场景，错误会写入 `job-xxxx.log`；
- 设计重点是错误日志收集与界面查看，而不是通用全文日志平台。

### Likely related areas

- `agent/`：日志采集与上报
- `internal/apps/`：错误日志接收、存储、查询接口
- `frontend/components/common/`：错误日志列表与详情页面
- `docs/`：方案与实施文档

### Repo research notes

- 现有节点日志查看链路已存在：`internal/apps/cluster/service.go:GetNodeLogs` + `frontend/components/common/cluster/ClusterDetail.tsx`
  - 当前实现是 **按节点临时读取单个日志文件**；
  - 文件名按 `DeploymentMode + role` 硬编码推导；
  - 不兼容 `routingAppender` 下的 `job-*.log` 全量错误定位需求。
- Agent 侧已有 `get_logs` 命令实现：`agent/cmd/main.go:handleCollectLogsCommand`
  - 当前更像“人工排查工具”；
  - 支持 tail/head/all、filter、date；
  - 不适合作为后续 AI 分析的数据主链路。
- gRPC 协议里已经存在 `LogStream(stream LogEntry)`：`internal/proto/agent/agent.proto`
  - 服务端处理器已存在：`internal/grpc/handlers.go:LogStream`；
  - 但当前仅落到 `audit_logs`，且仓库中暂无 Agent 侧实际调用代码。
- 升级中心已实现一套成熟的“步骤/日志/节点执行”前后端模式：
  - 后端：`internal/apps/stupgrade/*`
  - 前端：`frontend/components/common/cluster/upgrade/ClusterUpgradeExecute.tsx`
  - 适合作为后续“错误分析任务 / AI 分析任务”的 UI 复用模板。
- 审计日志页已实现成熟的“筛选 + 列表 + 分页”模式：
  - `frontend/components/common/audit/AuditLogMain.tsx`
  - 适合作为错误事件列表页的交互基线。
