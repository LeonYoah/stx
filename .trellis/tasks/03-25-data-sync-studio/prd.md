# data-sync-studio

## Goal
在 STX 平台内实现一个数据同步工作台（Data Sync Studio）MVP，支持任务编辑、校验、DAG 展示、预览运行、正式提交与运行状态查看。

## Requirements
- 支持创建与编辑数据同步任务定义。
- 支持对任务定义做编译/校验，并返回 errors / warnings。
- 支持生成并展示同步 DAG。
- 支持预览模式运行，基于现有 http sink / mock sink 方案获取样例结果。
- 支持通过 SeaTunnel Engine 官方接口提交任务。
- 支持查看任务运行实例状态、基础日志与错误信息。
- 支持保存任务版本快照，为后续回滚预留模型。
- 前期优先参考 SeaTunnel 官方文档中的 REST API / CLI 提交方式，不照搬 Dinky 的 savepoint 表设计。
- DAG 预览优先复用 SeaTunnel Web 现有 `jobDag` 展示语义，而不是长期维护一套独立前端 DAG 数据模型。
- 后端需把 config DAG 适配成 SeaTunnel Web 兼容的“伪 runtime jobDag”，前端直接消费该结构做配置预览。

## Config DAG Preview（WebUI Compatible）

### Scope
- 这是**配置预览 DAG**，不是 runtime DAG。
- 目标是在工作台里展示“当前配置解析后会形成怎样的 source / transform / sink DAG”，并尽量保持与 SeaTunnel Web 现有 DAG 风格一致。

### Backend Contract
- `stx-java-proxy` 新增：
  - `POST /api/v1/config/webui-dag`
- 输入：
  - `content`
  - `contentFormat`，当前仅支持 `hocon`
  - `filePath`（与 `content` 二选一）
  - `variables`
- 输出：
  - 直接返回一个 **WebUI compatible pseudo job detail**，最少包含：
    - `jobId`
    - `jobName`
    - `jobStatus`
    - `jobDag`
    - `metrics`
    - `pluginJarsUrls`

### Mapping Rules
- `ProxyNode -> vertexInfoMap`
  - `SOURCE -> source`
  - `TRANSFORM -> transform`
  - `SINK -> sink`
- `ProxyEdge -> pipelineEdges`
  - 第一阶段统一塞入伪 pipeline `0`
- `vertexId`
  - 通过稳定顺序编号，优先按拓扑顺序生成 `1..N`
- `connectorType`
  - 形如 `Source[0]-FakeSource` / `Transform[1]-Sql` / `Sink[0]-Console`
- `metrics`
  - 即使没有运行时指标，也返回空壳结构，避免前端兼容分支过多

### Frontend Strategy
- `/sync` 的 DAG 弹窗不再只展示原始 `nodes/edges` JSON。
- 前端改为消费上面的 pseudo job detail / `jobDag`。
- 第一阶段保持：
  - 继续在工作台弹窗内展示
  - 尽量复用 SeaTunnel Web 的 DAG 展示方式与字段语义
- 若后续发现分支/合流布局不理想，再单独增强 `ConfigDagPreview`，但**不改后端契约**。
- 工作台 DAG 预览**只认最新 `webui-dag` 接口与结构**，不再兼容旧 `/api/v1/config/dag` 作为 fallback 契约。

## Acceptance Criteria
- [ ] 后端存在 sync task / sync job instance 的基础模型与 API。
- [ ] 可以对一个同步任务执行 validate 并返回结构化结果。
- [ ] 可以返回该任务对应的 DAG 数据。
- [ ] 可以发起 preview，并能读取到 preview 结果或状态。
- [ ] 可以发起正式 submit，并记录 engine job id 与运行状态。
- [ ] 任务版本快照模型建立完成。
- [ ] 架构与接口设计明确区分 validate / preview / submit / runtime tracking。
- [ ] `stx-java-proxy` 提供 `POST /api/v1/config/webui-dag`，并能将 config DAG 转成 WebUI compatible pseudo job detail。
- [ ] `/api/v1/sync/tasks/:id/dag` 返回包含 WebUI compatible `jobDag` 的 DAG preview 结果。
- [ ] `/sync` 页面中的 DAG 弹窗优先展示 WebUI-compatible DAG 视图，而不是纯 JSON 列表。
- [ ] 至少完成一轮后端单测、前端类型检查，以及工作台手工/自动化自测。

## Technical Notes
- 初期聚焦 SeaTunnel Engine cluster / REST API V2 路径。
- 恢复任务先按“恢复行为/策略”建模，不额外设计统一 savepoint 表。
- 复用现有 catalog columns 解析、DAG 解析、preview sink 改写能力。
- 这是一个跨层特性，涉及后端 API、任务模型、运行时适配以及后续前端 Studio 页面。
- WebUI-compatible DAG 的目标是**复用官方字段语义与展示习惯**，不是冒充真实 runtime DAG。
