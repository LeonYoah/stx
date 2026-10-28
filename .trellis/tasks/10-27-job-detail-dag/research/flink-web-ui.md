# Flink Web UI 作业详情实现参考

来源：`apache/flink` 主分支 `eead57e`（2026-10-27），稀疏检出于 `/data/workspace/refs/flink/flink-runtime-web/web-dashboard/src/app`。
以下结论都对应具体源码文件，不凭印象。

## 1. 作业级结构
- `pages/job/job.config.ts`：作业页签 Overview / Exceptions / Data Skew / TimeLine / Checkpoints / Configuration / Rescales。
- `pages/job/job-detail/status/job-status.component.html`：头部用描述列表，字段 Job ID、Job State（状态徽标 + 各状态任务数）、
  Job Type、Start Time、End Time（结束才显示）、Duration；右上角只有一个危险操作「Cancel Job」，带二次确认 popconfirm，
  仅 RUNNING / CREATED / RESTARTING 显示。

## 2. Overview 页：图 + 算子列表 + 抽屉
- `pages/job/overview/job-overview.component.html`：上方 `flink-dagre` 图，下方 `flink-job-overview-list` 算子表，中间
  `flink-resize` 可拖动分隔；图区高度初值 `max(280, min(innerHeight*0.4, 500))`（`job-overview.component.ts:59`）。
- 点图节点和点列表行走同一个 `onNodeClick` → 打开右侧抽屉（路由子页）。
- 抽屉 `drawer/job-overview-drawer.component.*`：宽 60%，有「关闭」和「铺满」两个把手；页签 Detail / SubTasks /
  TaskManagers / Watermarks / Accumulators / BackPressure / Metrics / FlameGraph（`job-overview.config.ts`）。
- 算子列表 `list/job-overview-list.component.html`：Name（左固定，悬停看全名）、Status、Bytes Received、Records Received、
  Bytes Sent、Records Sent、Parallelism、Start Time、Duration、End Time、Tasks；各列可排序，选中行高亮。

## 3. 图布局（`components/dagre/graph.ts`）
- dagre：`rankdir: 'LR'`，`ranker: 'network-simplex'`，`align: 'DL'`，`ranksep: 150`，`edgesep: 150`，`marginx/y: 20`。
- 节点固定宽 205px（`node.component.html` foreignObject width），高度按内容实测后回写再布局（`dagre.component.ts:220`）。
- 边标签为分发策略文字，沿路径居中（textPath startOffset 50%）。

## 4. 指标刷新不重排（`job-overview.component.ts:87-109, 156-205`）
- 节点数或作业变化 → `refreshGraph()` 重新布局；否则只 `refreshNodesWithMetrics()` → `dagreComponent.updateNode(id, node)`，
  沿用缓存的宽高，不重新布局。
- 每个顶点并行拉 `backPressuredTimeMsPerSecond`、`busyTimeMsPerSecond`、`numRecordsInPerSecond`（含聚合 max / skew），
  百分比 = `min(round(max / 10), 100)`；单个顶点失败用 `catchError` 回退原节点，不影响整体。

## 5. 节点外观（`components/dagre/components/node/node.component.*`）
- 文字：算子名、描述（>300 字截断）、`Parallelism`、`Backpressured (max)`、`Busy (max)`、`Data Skew`、`Low Watermark`、`Operation`；
  无值显示 `N/A`，百分比四舍五入。
- 颜色：默认底色 `#5db1ff` / 边框 `#1890ff`；按 busy% 向 `#ee6464`/`#ee2222` 线性混合，再按 backpressured% 向 `#888888`/`#000000`
  混合（`update()` 中两次 `blend`）。即“越忙越红、越被反压越灰”，一眼看出瓶颈在红色节点、被它拖住的上游是灰色。
- 背压状态徽标 `components/backpressure-badge`：ok `#52c41a` / low `#faad14` / high `#f5222d`。

## 6. 聚焦交互（`dagre.component.ts:283-335`）
- 点节点：记住原视口 → 平移缩放使节点位于宽度 1/xCenter 处（overview 传 `xCenter=4`，给右侧抽屉让位）→ 遮罩淡化其余元素，
  选中节点、相邻节点（circularNodeIds）和相连边（focusedLinkIds）提到遮罩之上并加粗。
- 关闭抽屉 → `redrawGraph()` 恢复原视口。

## 7. Checkpoints 页（`pages/job/checkpoints/job-checkpoints.component.html`）
- 子页签 Overview / History / Summary / Configuration；History 表列 ID、Status、Acknowledged、Trigger Time、
  Latest Acknowledgement、End to End Duration、Checkpointed Data Size、Full Checkpoint Data Size、Processed in-flight data。

## 8. 映射到 STX / SeaTunnel 数据

| Flink | STX 可用数据 | 处理 |
|---|---|---|
| Busy (max) | `/realtime` 顶点最新 `sourceReadRatio` / `busyRatio`（`latestVertexBusy`） | 采用，同样四舍五入 % |
| Backpressured (max) | `/realtime` 边 `bpRatio`（按 `target_vertex_id`）；上游节点的被反压 = 其出边 bp 最大值 | 采用；边上也标 % |
| 节点混色 | 同上 | 采用 Flink 的“蓝→红（忙）→灰（被反压）”混合，暗色主题用对应 token |
| Records/Bytes Received/Sent | `metrics.TableSourceReceived*` / `TableSinkWrite*` 按 `Source[0]`/`Sink[0]` 前缀聚合 | 采用；Transform 无指标显示 `-` |
| Parallelism | 只有作业级 `envOptions.parallelism` | 放到「配置」，节点不显示 |
| Data Skew / Watermark / SubTasks / FlameGraph | 无对应数据 | 不做 |
| Status / Start / Duration（每算子） | 无 | 列表只用作业级 |
| Exceptions | `job.error_message` | 头部失败摘要，可展开全文 |
| Configuration | `job_dag.envOptions` + `submit_spec` 关键字段 | 新增「配置」页签（键值表） |
| Checkpoints | 现有 `CheckpointWorkspacePanel` | 作为页签 |
| Cancel Job（popconfirm） | 停止 / 保存点停止 / 恢复 | 头部操作，沿用现有确认与权限 |
