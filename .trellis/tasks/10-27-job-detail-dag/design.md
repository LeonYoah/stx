# 设计：作业详情页与 Flink 风格执行拓扑

Flink 对照依据见 `research/flink-web-ui.md`（源码 `apache/flink@eead57e`）。

## 1. 边界与模块

```
后端 internal/apps/sync
  └─ plan_dag.go（新）     GET /api/v1/sync/jobs/:id/plan-dag  → JobPlanDAGResult
     复用 configToolClient.InspectWebUIDAG / configToolResolver / extractWebUIDAGNodes|Edges / getJobInstanceForActor
  └─ operation/registry_sync.go  登记 sync.job.plan-dag（R0），生成 `stx sync job plan-dag <id>`

前端 frontend/components/common/sync/job-detail/（新目录）
  ├─ dag-model.ts          纯函数：归一化、运行时 × 计划合并、指标映射、实时映射、中间省略、dagre 布局
  ├─ JobDagCanvas.tsx      React Flow 画布（next/dynamic 按需加载，ssr:false），plan / runtime 两种模式
  ├─ DagNodeDrawer.tsx     节点抽屉：概览 / 表结构 / 建表计划 / 指标
  └─ JobDetailDialog.tsx   全屏弹层：头部操作 + 页签（拓扑 / 指标 / 日志 / 事件 / Checkpoint / 脚本）
改动：ConsolePanel.tsx（行内按钮精简、行点击打开详情）、DataSyncStudio.tsx（详情状态、深链、日志与 Checkpoint 加载条件）、
      JobEventTimelineDialog.tsx（拆出 JobEventTimelineContent）、编辑器「DAG」弹窗改用 JobDagCanvas（plan 模式）、
      zh/en 语言包。WebUiDagPreview.tsx 两处使用都迁走后删除。
```

`DataSyncStudio.tsx` 已有 6332 行，新逻辑全部放进 `job-detail/`，工作台只负责状态接线，不再往里堆渲染代码。

## 2. 后端：运行作业的计划 DAG

### 签名
```go
// GET /api/v1/sync/jobs/:id/plan-dag
func (s *Service) GetJobPlanDAGForActor(ctx context.Context, actor executionapp.Actor, id uint) (*JobPlanDAGResult, error)

type JobPlanDAGResult struct {
    JobID     uint      `json:"job_id"`
    Available bool      `json:"available"`
    Reason    string    `json:"reason"`              // 见原因表；available=true 时为空
    Message   string    `json:"message,omitempty"`   // inspect_failed 时的脱敏错误摘要
    WebUIJob  JSONMap   `json:"webui_job,omitempty"` // 与 /tasks/:id/dag 的 webui_job 同构（WebUI-compatible jobDag，api-and-services.md 契约）
    Warnings  []string  `json:"warnings"`            // 永远是数组，不为 null
}
```

### 流程
1. `getJobInstanceForActor` 校验可见权限（与 `/realtime` 一致，不可见 → 403）。
2. 从 `submit_spec` 读 `submitted_content`、`submitted_format`（缺省 `format`）、`cluster_id`。
3. `configToolResolver.ResolveConfigToolEndpoint(cluster_id, nil)` → `InspectWebUIDAG(ConfigToolContentRequest{Content, ContentFormat})`。
4. 与 `BuildTaskDAG` 一样用 `structToJSONMap(webuiResp)` 输出 `webui_job`，前端只消费 WebUI 兼容 `jobDag`（`api-and-services.md`「Config DAG 兼容 WebUI jobDag」契约），与编辑器 DAG 一套解析。

### 原因表（都返回 200 + `available=false`，前端显示说明而不是报错）
| 情况 | reason |
|---|---|
| `submitted_content` 为空（旧作业 / 提交前失败） | `no_submitted_content` |
| 本地执行作业（`submitSpecExecutionMode == local`） | `local_mode` |
| config tool 未配置 / 集群无法解析 endpoint（含集群已删除） | `config_tool_unavailable` |
| config tool 调用失败 | `inspect_failed`，`message` 经 `audit.RedactText` 脱敏并截断 500 字符 |

### 不缓存（后端）
- 输入（提交内容）不可变；线上实测 config tool 调用 0.2s。后端不加缓存，避免与对账器并发写 `result_preview`。
- 前端在一次详情会话内按作业 ID 缓存，抽屉「建表计划 / 表结构」提供手动刷新。
- 语义说明：config tool 读的是**当前**库表结构；界面标注「按当前库表结构解析，建表计划为提交时配置推导，只读」。

### 已验证的假设
- 作业 #26：运行时 DAG 与 config tool DAG 的 `vertexId` 一致（1=Source、2=Sink），`tablePaths` 一致；`pipelineEdges` 的 pipeline 键不同（"1" vs "0"），所以**按顶点 ID 合并，不按 pipeline 键**。

## 3. 前端数据模型（dag-model.ts）

```ts
interface DagVertex {            // 画布与抽屉的唯一节点模型
  vertexId: number; type: 'source'|'transform'|'sink'|string;
  displayName: string;           // 去掉 "pipeline-1 [...]" 外壳后的 Source[0]-Postgres-CDC
  metricPrefix?: string;         // Source[0] / Sink[0] / Transform[0]，从 connectorType 或 vertexName 解析
  tablePaths: string[];
  tableColumns?: Record<string,string[]>; tableSchemas?: Record<string,SyncJSON>;
  saveModePreviews?: Record<string,SyncSinkSaveModePreviewTable>; saveModeWarnings?: string[];
}
interface DagEdge { source: number; target: number }
```

- `normalizeDag(jobDag)`：兼容 `vertexInfoMap` 为对象或数组、ID 为字符串或数字（现有 `normalizeVertices/normalizeEdges` 的逻辑迁入）。
- `mergeRuntimeWithPlan(runtime, plan)`：拓扑以运行时为准；按 `vertexId` + `type` 匹配补表结构；ID 匹配不上时按
  （`type` + `tablePaths` 集合相等）兜底；仍匹配不上的节点保持无表结构，抽屉显示"未能对应到提交配置"。
- `vertexTableMetrics(vertex, metrics)`：用 `${metricPrefix}.${tablePath}` 取 `TableSourceReceived*` / `TableSinkWrite*` /
  `TableSinkCommitted*`；节点汇总用 `*PerVertex` 与作业级字段兜底。
- `realtimeOverlay(realtime)`：`vertex_id` → 最新忙碌度（复用 `latestVertexBusy`）；边按 `target_vertex_id` 取最新背压
  （复用 `backpressureTone`）；轮询沿用 5s。
- `middleEllipsis(text, maxChars)`：保留库名前缀与表名结尾，中间 `…`。
- `layoutDag(vertices, edges)`：`@dagrejs/dagre`，参数照抄 Flink `graph.ts`：`rankdir LR`、`ranker network-simplex`、`align DL`、
  `ranksep 150`、`edgesep 150`、`marginx/y 20`。节点**固定宽 260px、高度由模式决定的固定行数**（Flink 也是固定宽 205px），
  布局完全确定、不需要实测回写；长表名靠中间省略 + 悬停全名 + 复制 + 抽屉完整展示解决。
- `nodeColors(busy, backpressured)`：照 Flink `node.component.ts` `update()`：默认色按 busy% 向红混合，再按被反压% 向灰混合；
  颜色值用 CSS 变量定义亮 / 暗两套。被反压% = 该节点所有出边 `bpRatio` 的最大值。

## 4. 画布与抽屉

- `JobDagCanvas`：`@xyflow/react` 自定义节点；`fitView`、`MiniMap`、`Controls`；节点只读（不可连线、不可删）。
  - 节点（对照 Flink 节点文字）：类型徽标 + `displayName` + 主表（多表时“首表 + 共 N 张表”，表名 `middleEllipsis`，悬停全名，
    复制按钮）；runtime 模式再加 `Busy (max)`、`Backpressured (max)`、读 / 写条数，无值显示 `N/A`；节点底色按 `nodeColors` 混合。
  - 边：runtime 模式标签显示背压 %（颜色同 SeaTunnel Web UI 分档），plan 模式不显示标签。
  - 聚焦（照 Flink `dagre.component.ts clickNode`）：点节点后视口平移使节点位于画布宽度 1/4 处给抽屉让位，其余节点与边淡化，
    选中节点、相邻节点与相连边高亮；关闭抽屉恢复原视口。
  - 顶部紧凑条：瓶颈提示（原 `RealtimeInsightPanel` 的提示与 SeaTunnel Web UI 入口），不可用时显示原因而不报错。
- `DagNodeDrawer`（右侧，默认宽 45%，有「关闭」与「铺满」两个把手，同 Flink 抽屉；内部滚动）：
  - 概览：完整连接器名、类型、可搜索表列表（完整表名，可复制）。
  - 表结构：选中表的列 / 类型 / 可空 / 注释 / 主键（迁移现有 `extractSchemaColumnDetails` 与列表渲染）。
  - 建表计划：现有 `SaveModePreviewDetail` 内容；runtime 模式标题为「提交时的建表计划（只读）」。
  - 指标：按表的读 / 写条数、QPS、字节；runtime 且可观测时加忙碌度 / 背压。plan 模式隐藏本页签。

## 4.1 拓扑页布局（参考 Flink）

```
┌ 瓶颈提示条（运行中 3.x；不可用时一行原因）──────────────────────────────┐
│ React Flow 画布（左→右）                          │ 节点抽屉（选中时） │
│   [Source[0] PG-CDC  busy 12%] ─42%·3.9k/s→ [Sink]│ 概览/表结构/      │
├──────────────────────────────────────────────────┤ 建表计划/指标      │
│ 算子列表：# │ 名称 │ 类型 │ 表数 │ 读入 │ 写出 │ QPS │ 字节 │ 忙碌 │ 背压 │                │
└──────────────────────────────────────────────────┴───────────────────┘
```
- 图与算子列表上下分栏，中间可拖动分隔（同 Flink `flink-resize`），图区初始高度 `max(280, min(可用高度*0.55, 520))`；
  列表内部滚动、各列可排序、选中行高亮；点列表行 = 点节点（选中 + 打开抽屉 + 聚焦）。
- 列表列（对照 Flink 列表，去掉 SeaTunnel 没有的子任务 / 并行度 / 每算子时间）：名称（左固定、悬停全名）、类型、表数、
  读入条数、读入字节、写出条数、写出字节、QPS、Busy、Backpressured。Transform 无指标显示 `-`。
- 节点最大显示：类型、名称、首表 + “共 N 张表”、运行中 busy / 背压最大值徽标。

## 4.2 性能策略
- 布局 `useMemo` 依赖“结构签名”（顶点 ID / 类型 / 边列表的字符串），实时与指标刷新只更新节点 `data`，不重新布局。
- React Flow `onlyRenderVisibleElements`；节点组件 `memo`；边标签只在运行模式显示。
- 抽屉表列表：搜索过滤 + 每次渲染最多 200 行（“显示更多”），按表指标表格可按条数 / QPS 排序。
- 实时轮询：仅 `detailTab === 'topology'` 且作业运行中且 `document.visibilityState === 'visible'` 时轮询 5s；切页签或隐藏即停。
- 计划 DAG：打开详情即后台请求（会话缓存），返回后合并进节点数据；请求中拓扑照常可用。
- 多表指标：按 `metricPrefix` 一次性把 `Table*` 指标索引成 `Map<prefix, Map<table, stats>>`，避免每节点重复扫描。

## 5. 作业详情弹层

- 尺寸：`w-[92vw] max-w-[1400px] h-[88vh] flex flex-col overflow-hidden`（ui-conventions §7.2）。
- 头部：`#id`、状态徽标、模式 / 版本 / 通道、开始时间与耗时；操作区：恢复、保存点停止、停止（复用行内的显示 / 禁用条件，
  抽成 `getJobActionState(job, ctx)` 两处共用）、SeaTunnel 观测菜单。
- 头部字段对照 Flink `job-status`：作业 ID、引擎作业 ID、状态、开始时间、结束时间（结束才显示）、耗时；失败时一行错误摘要，
  点开看全文（对应 Flink Exceptions，不单独做页签）。停止类操作沿用现有确认弹窗。
- 页签：拓扑（默认）｜指标（`MetricsDialogContent`）｜Checkpoint（工作台传入插槽）｜日志（工作台传入插槽）
  ｜事件（`JobEventTimelineContent`，非 3.x 时禁用并提示）｜配置（`envOptions` 与 `submit_spec` 关键字段键值表，对应 Flink Configuration）
  ｜脚本（`JobScriptDialogContent`）。只渲染当前页签（渐进式披露）。
- 作业对象：弹层只持有 `detailJobId`，作业从工作台 `jobs` 列表取（随轮询刷新状态）；深链作业已被插入列表。
- 日志 / Checkpoint：打开详情时 `setSelectedJobId(detailJobId)`；工作台现有加载条件 `bottomConsoleTab === 'logs'` 扩展为
  `bottomConsoleTab === 'logs' || (detailOpen && detailTab === 'logs')`，Checkpoint 同理。

## 6. 入口变化

- `JobRunsPanel`：行点击 → `onSelectJob(id)` + `onOpenDetail(job)`；行内保留「详情」「恢复」「保存点停止」「停止」；
  多表徽标 → `onOpenDetail(job, 'metrics')`。移除事件 / 脚本 / 指标 / SeaTunnel 观测按钮与 `viewExecutionDag`。
- 深链 `/sync?task=&job=`：选中作业后直接打开详情（拓扑页签）。
- 编辑器「DAG」按钮：弹窗内容换成 `JobDagCanvas mode="plan"`（列信息、DDL 预览保留在抽屉里）。
- `observabilityHint` 文案改为指向作业详情（zh / en）。

## 7. 依赖与兼容

- 新增：`@xyflow/react`（12.x，peer react>=17，项目为 React 19 / Next 15）、`@dagrejs/dagre`（3.1.x）。仅详情 / DAG 弹窗内
  `next/dynamic` 加载，不进主包；引入 `@xyflow/react/dist/style.css` 只在该组件内。
- 后端无表结构变更、无迁移；新接口只读（R0）。
- 旧作业没有 `submitted_content` 时拓扑照常显示，只是抽屉提示无表结构。

## 8. 回滚

- 前端：回退 `job-detail/`、`ConsolePanel.tsx`、`DataSyncStudio.tsx` 与依赖即可恢复旧弹窗；后端新接口无人调用时无副作用，可独立保留或回退。
