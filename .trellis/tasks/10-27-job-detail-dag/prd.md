# 作业详情页：Flink 风格 DAG 与全部操作入口

## Goal

运行记录里的作业，点击任意位置即可打开「作业详情」，在一个界面里看清执行拓扑（参考 Flink Web UI）、
每个节点的表结构 / 建表计划 / 指标 / 实时忙碌度与背压，并能完成该作业的所有操作。
解决当前运行 DAG 看不到列信息和 DDL、长表名被截断无法查看、已有指标没有展示在图上的问题。

## Confirmed Facts（已查证）

- 编辑器「DAG」按钮与运行记录「查看执行拓扑 (DAG)」共用 `frontend/components/common/sync/WebUiDagPreview.tsx`
  （1191 行，手写绝对定位 + SVG 连线，节点固定宽 220px，表名单行截断仅 `title` 悬停）。
- 运行 DAG 数据来自引擎 `job-info`，存于 `result_preview.job_dag`（`internal/apps/sync/service.go` `mergeJobRuntimeInfo`）。
  线上作业 #26 实测：`vertexInfoMap` 为数组，每个节点只有 `tablePaths`、`type`、`vertexId`、`vertexName`，
  **没有** `tableColumns` / `tableSchemas` / `saveModePreviews`，因此列详情为空、「预览 DDL」按钮不出现。
- 编辑器 DAG 走 config tool（`config_tool_client.go`），节点带 `tableColumns` / `tableSchemas` / `saveModePreviews`。
- `result_preview.metrics` 已有按节点 / 按表的指标（`SourceReceivedCount/QPS`、`SinkWriteCount/QPS`、字节数、
  `TableSink*`、`IntermediateQueueSize` 等），图上未展示。
- 实时忙碌度 / 背压 / 瓶颈提示由 `GET /api/v1/sync/jobs/:id/realtime` 提供（`RealtimeInsightPanel.tsx`），
  目前是图上方的独立面板，未画在节点和连线上。
- 前端无任何图布局库。
- 运行记录（`ConsolePanel.tsx` `JobRunsPanel`）每行现有操作：SeaTunnel 观测菜单、作业事件（3.x）、查看执行拓扑、
  查看执行脚本、查看指标（含多表）、恢复、保存点停止、停止。点击行目前只做「选中」，驱动底部「日志」「Checkpoint」页签。
- 作业出现的位置：工作台运行记录；告警中心 / 通知通过深链 `/sync?task=<id>&job=<id>` 跳到工作台并选中作业
  （`app/(main)/sync/page.tsx` 重定向到 `/workbench`，`DataSyncStudio` 解析深链）。
- `submit_spec.submitted_content` 保存了提交时的完整配置（已含 STX 注入片段），可直接作为 config tool
  `InspectWebUIDAG` 的输入；集群取 `submit_spec.cluster_id`。config tool 读的是当前库表结构，不是提交瞬间的快照。
- 前端规范：重内容弹窗用 `w-[92vw] max-w-[1400px]` / `h-[88vh]`，内部滚动；重型明细放二级 Tab 或侧边抽屉。

## Requirements

### R1 入口：查看详情（已定：工作台内全屏弹层）
- 详情形态为工作台内的全屏级弹层（规范的重内容尺寸 `w-[92vw] max-w-[1400px]` / `h-[88vh]`），不做独立路由页。
- 运行记录中的「查看执行拓扑 (DAG)」改为「查看详情」。
- 作业行任意位置可点击打开详情，同时保持选中该作业（底部日志 / Checkpoint 页签仍跟随）；行内按钮自身的点击不受影响。
- 告警深链 `/sync?task=<id>&job=<id>` 打开工作台后直接打开该作业详情。
- 详情界面包含该作业的所有操作：停止、保存点停止、恢复、SeaTunnel 观测，以及拓扑、指标、日志、事件、Checkpoint、执行脚本。
- 行内按钮精简（已定）：只保留「详情」「恢复」「保存点停止」「停止」（后三者按现有条件显示 / 禁用）。
  事件、执行脚本、指标、SeaTunnel 观测从行内移除，统一收进详情；多表徽标点击打开详情并定位到「指标」页签。

### R2 执行拓扑（Flink 风格）
- 引入 `@xyflow/react` + `dagre` 自动布局，支持拖拽、缩放、小地图、适应窗口；按需加载，不进主包。
- 节点宽度随内容自适应，显示连接器与主表名；长表名中间省略，可一键复制，完整名称在详情中可见。
- 运行中作业：节点按忙碌度着色，连线标注背压百分比与每秒条数。

### R3 节点详情
- 点击节点打开右侧抽屉，页签：概览（连接器、可搜索的表列表）、表结构（列 / 类型 / 主键）、
  建表计划（运行时 DAG 标为「提交时的建表计划」只读展示）、指标（按表条数与 QPS；运行中加忙碌度与背压）。

### R4 补齐运行 DAG 的表结构数据
- 后端新增只读接口，用作业提交时的配置（`submit_spec.submitted_content`）调用 config tool 得到计划 DAG；
  前端按节点 ID（兜底按类型 + 表路径）把表结构与建表计划合并进运行时 DAG。新增接口须补 CLI 能力（AGENTS.md CLI 约定）。
- 已实测作业 #26：两套 DAG 的节点 ID 与表路径一致，config tool 耗时 0.2s。后端不缓存，前端在一次详情会话内缓存并可手动刷新。
- 表结构按当前库表解析，建表计划由提交时配置推导，界面需明确标注，且只读。
- 无法得到计划 DAG（旧作业、本地执行、集群不可用、解析失败）时拓扑照常显示，抽屉给出原因，不报错。

### R7 性能、多算子、多表（用户强调）
- 多算子：多 pipeline、多 Source / Transform / Sink、扇入扇出都能正确布局，节点不重叠，连线不穿过节点。
- 多表：单节点几百张表时节点只显示首表 + 数量，不撑高；抽屉表列表可搜索、虚拟化或分页渲染；按表指标表格可排序（条数 / QPS）。
- 性能：布局只依赖拓扑结构，实时数据刷新不触发重新布局；实时轮询只在「拓扑」页签可见、作业运行中且页面可见时进行；
  计划 DAG 在打开详情时后台加载，不阻塞拓扑首屏；React Flow 只渲染可视区域元素。
- 日志、Checkpoint 作为详情页签（用户确认），与底部面板共用数据，不重复请求。

### R8 参考 Flink Web UI 的细节
- 拓扑页：上方图、下方算子列表（名称、类型、表数、读入 / 写出条数、QPS、字节、忙碌度、背压），点行与点节点等价。
- 节点上直接显示忙碌度 / 背压最大值（运行中），颜色分档与 SeaTunnel Web UI 一致。
- 头部显示作业 ID、引擎作业 ID、状态、开始时间、耗时；失败作业显示错误摘要（可展开全文）。
- 「配置」页签：envOptions（并行度、checkpoint 间隔、作业模式等）与提交关键字段（对应 Flink Configuration）。
- 依据 Flink 源码（`apache/flink@eead57e` web-dashboard）对照，见 `research/flink-web-ui.md`；SeaTunnel 没有的数据
  （子任务、并行度 / 时间按算子、Data Skew、Watermark、FlameGraph）不做。

### R6 编辑器 DAG 同步升级
- 编辑器「DAG」按钮的预览改用同一个新画布（无指标），列信息与 DDL 预览能力不回退；旧 `WebUiDagPreview` 删除。

### R5 一并提交的文案修复
- 上一轮已修改的 `observabilityHint`（zh / en）随本任务一起提交，不单独提交。
- 文案中对入口的描述需与 R1 的新入口名称保持一致（指向「作业详情」而非「DAG 页」）。

## Acceptance Criteria

- [ ] 运行记录中不再出现「查看执行拓扑 (DAG)」，点击作业行任意空白处打开作业详情。
- [ ] 线上类似 #26 的运行中作业，在详情中能看到每张表的列信息与提交时的建表计划。
- [ ] 表名超长时节点内中间省略、可复制，抽屉内显示完整名称，无横向遮挡。
- [ ] 运行中 3.x 作业的节点与连线显示忙碌度 / 背压；未开启可观测性时有明确说明，不报错。
- [ ] 编辑器「DAG」预览不回退（列信息、DDL 预览仍可用）。
- [ ] `observabilityHint` zh / en 文案与新入口一致，i18n 覆盖测试通过。
- [ ] 新增后端接口有对应 CLI 命令（`stx sync job plan-dag <id>`）。
- [ ] 行内只剩「详情」「恢复」「保存点停止」「停止」；多表徽标打开详情的「指标」页签。
- [ ] 告警深链 `/sync?task=&job=` 打开后直接显示该作业详情。
- [ ] 详情内停止 / 保存点停止 / 恢复与行内行为、权限、禁用条件一致；日志、Checkpoint、事件、脚本、指标页签可用。
- [ ] 旧作业或计划 DAG 不可用时，拓扑仍可查看，抽屉显示原因而不报错。
- [ ] React Flow 与 dagre 只在按需加载的 chunk 中，不进入主包。

## Out of Scope

- 自建完整背压历史曲线（SeaTunnel Web UI 已提供，保留跳转）。
- 修改引擎侧 `job-info` 返回结构。

## Decisions

- 2026-10-27 详情形态：工作台内全屏弹层（用户选择）。
- 2026-10-27 行内按钮：只留快捷操作（用户选择）。
- 2026-10-27 图库：`@xyflow/react` + `@dagrejs/dagre`，运行 DAG 的 DDL 标为「提交时的建表计划」只读（用户认可方案）。
- 2026-10-27 上一轮 `observabilityHint` 文案修复并入本任务提交（用户要求）。

## Open Questions

- 无。规划待用户审阅 `prd.md` / `design.md` / `implement.md` 后进入实现。
