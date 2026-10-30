# API 与 Services

> 前端如何调用后端以及如何统一处理错误。

---

## 概述

所有后端请求通过 `lib/services/core/api-client.ts` 的 **axios** 客户端发出。客户端使用 `baseURL: '/api/v1'`、`withCredentials: true` 和 60 秒超时。响应格式为 **ApiResponse&lt;T&gt;**，包含 `error_msg` 与 `data`。`lib/services/core/base.service.ts` 中的 **BaseService** 封装 get/post/put/delete，成功时仅返回 `data`；错误由响应拦截器处理，以 `Error` 形式 reject，消息为后端返回或通用提示。

---

## API 响应结构

- **成功**：后端返回 `{ "error_msg": "", "data": <载荷> }`。Services 返回类型为 `T` 的 `data`。
- **错误**：后端返回 HTTP 4xx/5xx，通常带 `{ "error_msg": "消息" }`。拦截器用 `Error` reject，若存在 `error_msg` 则 `message` 为该值。

来自 `lib/services/core/types.ts`：

```ts
export interface ApiResponse<T = unknown> {
  error_msg: string;
  data: T;
}

export interface ApiError {
  error_msg: string;
  data?: unknown;
}
```

---

## 服务层

- **BaseService**：抽象类，提供静态方法 `get<T>`、`post<T>`、`put<T>`、`delete<T>`，以及按需透传查询参数的 `postWithParams<T>`、`putWithParams<T>`。各方法调用 `apiClient.*`，期望 `ApiResponse<T>`，并返回 `response.data.data`。子类设置 `protected static basePath`，通过 `getFullPath(path)` 拼 URL。
- **约定**：
  - 每个领域一个 service 类（如 `ClusterService`、`HostService`）。
  - 方法对应后端接口（如 `list(params)`、`getById(id)`、`create(body)`）。
  - 返回类型为 `data` 的类型（如 `ClusterInfo`、`PaginatedResponse<ClusterInfo>`）；不向调用方暴露原始 `ApiResponse`。
  - 登录页这类**能力开关由后端决定**的界面，不要硬编码展示第三方登录入口。应先调用服务（如 `auth.getEnabledOAuthProviders()` → `GET /api/v1/oauth/providers`），再按返回结果渲染 GitHub / Google 按钮。
  - 安装向导 / 创建集群这类**版本敏感配置**，不要在前端硬编码版本比较逻辑来猜哪些字段可用。应优先消费后端返回的能力矩阵（如 `installer.listPackages()` 返回的 `version_capabilities`），再决定是否展示 `history_job_expire_minutes`、`scheduled_deletion_enable`、`job_schedule_strategy` 等高级配置。
  - 若后端来自 Go / GORM 等实现，**不要假定数组字段一定是 `[]`**；对于会被 UI 直接 `.map()` / `.filter()` 的字段，优先在 service 或 session 恢复边界统一做 normalize，把 `null` 兜底成 `[]`。
  - 若接口返回**后端本地化文案**（如 diagnostics 摘要、诊断报告、SSE 事件、模板说明），service 应显式附带当前语言参数 `lang`，避免后端回落到默认语言或双语拼接结果。
  - 后端写接口接入公共执行协议后，Web service 必须同步接入 `createWebExecutionHeaders`。不能只验证 CLI，也不能在全局 axios 拦截器里给所有 POST/PUT/DELETE 自动加执行请求头，因为登录、查询型 POST 和未接入公共执行协议的接口不需要这些请求头。
  - R1 请求必须发送稳定的 `Idempotency-Key` 和 `X-STX-Confirm: true`；R2/R3 首次请求取得 `confirmation_id` 后，必须复用同一个幂等键，并通过 `X-STX-Confirmation-ID` 只重试一次。上传分片应为每个分片生成稳定且互不相同的幂等键，使单个分片可以安全重试。
  - 在线补充 SeaTunnel 源码属于长耗时写请求，service 要为该请求设置独立的 10 分钟超时，不能沿用普通 API 客户端的 60 秒超时。
  - 同一业务动作的 CLI 与 Web 调用方必须一起检查。后端新增或提高风险等级时，至少验证 Web 正常请求、幂等重试、一次性确认和错误结构透传。
- **使用**：组件或 hooks 从 `lib/services/<domain>` 导入 service，调用静态方法；错误用 try/catch 捕获，消息在 UI 中展示（toast 或行内错误状态）。

---

## 错误处理

- **401**：拦截器重定向到 `/login`（或 `/login?redirect=...`），`/auth/login` 自身除外，以便登录失败能展示后端错误。
- **403**：以「权限不足」消息 reject。
- **5xx**：以「服务器内部错误，请稍后重试」reject。
- **超时 (ECONNABORTED)**：以「请求超时，请检查网络连接」reject。
- **后端 error_msg**：若存在 `error.response?.data?.error_msg`，用该消息 reject；否则使用通用提示。
- **结构化确认数据**：响应拦截器应把后端 `data` 保留在抛出的 `ApiError.data` 中，R2/R3 service 据此读取 `confirmation_required` 和 `confirmation_id`。不能只保留错误文案，否则 Web 无法完成第二次确认请求。
- **在组件中**：对 service 调用包一层 try/catch；用 toast（如 sonner）或行内错误状态展示 `error.message`。不要向用户暴露堆栈或打日志。

---

## 新增一个 Service

1. 在 `lib/services/<domain>/` 下创建 `<domain>.service.ts`，可选 `types.ts` 和 `index.ts`。
2. 继承 `BaseService`，设置 `basePath` 为后端前缀（如 `'/clusters'`）。
3. 添加方法，调用 `this.get/post/put/delete` 并指定路径与返回类型 `T`（即 `data` 类型）。
4. 从 `lib/services/<domain>/index.ts` 导出，如需再从 `lib/services/index.ts` 导出。
5. 与后端契约一致：列表接口常返回 `{ data: { total, clusters } }` 等；类型 `T` 与之对应。

---

## 常见错误

- 在共享 `apiClient` 之外使用 `fetch` 或裸 axios（会绕过 401 重定向与错误统一处理）。
- 假定成功时一定存在 `data`；拦截器只透传响应，BaseService 假定为 `ApiResponse<T>` 并返回 `data` — 需保证后端 2xx 时始终返回 `data`。
- 基于会话的认证忘记 `withCredentials: true`；客户端与请求拦截器中已配置。
- 在调用处定义包含 `error_msg` 的响应类型；应把成功视为 `T`，错误视为抛出的 `Error`。
- 直接信任后端数组字段永远不是 `null`；尤其是 Go 的 nil slice 可能在 JSON 中变成 `null`，会让 UI 在 `.map()` / `.filter()` / `.length` 处崩溃。对这类字段要在边界统一 normalize。
- 后端给某个写接口加上 `Authorize` 后，只修改 CLI，不检查 Web service。这样浏览器请求会缺少 `Idempotency-Key` 或确认请求头，并直接收到 4xx。
- 在全局请求拦截器里无条件给全部写请求添加确认头。这样会把公共执行语义错误地扩散到登录、查询型 POST 和普通配置接口。
- R2/R3 第二次确认请求重新生成幂等键。确认编号与原幂等键、请求摘要绑定，换键会导致确认失败。

## Scenario: 实时诊断与事件入口（3.x）

- **入口判定只看后端记录**：`getRealtimeEntry(job)` 读取 `submit_spec.observability.supported / injected` 和作业状态，**不在前端比较 SeaTunnel 版本**。
- **跳转 SeaTunnel 只有一个入口**：`SeaTunnelObserveMenu`（`components/common/cluster/`）是唯一入口，作业详情头部和集群详情都复用它。
  忙碌度 / 背压已直接画在作业详情的拓扑上（见下方「作业详情与 Flink 风格拓扑」），完整背压历史曲线仍跳转引擎 Web UI，不自建。
  - 可选代理地址或原生地址，地址取自 `GET /api/v1/clusters/:id/webui-endpoints`。前端**不自己拼直连地址**，也不判断版本或 `enable_http`，这些都由后端返回的 `available/reason` 决定。
  - 带作业时追加 `job_path_template`（引擎 UI 是 hash 路由）。
  - 实时和离线作业都显示。拓扑上方的提示条（`JobTopology` 内的 `RealtimeHintsBar`）只显示状态、瓶颈提示和不可用原因，节点与连线承载忙碌度 / 背压数值。
- **轮询**：实时数据每 5 秒一次（和引擎采集桶对齐，`useJobRealtime`），只在详情「拓扑」页签可见时请求，页面不可见时暂停，作业到终态时停止；事件时间线每 10 秒一次（和引擎上报批次对齐），只在「事件」页签可见时请求。
- **数组兜底**：`getJobRealtime` 在 service 层把 `vertices/edges/hints/points` 归一成 `[]`，`listJobEvents` 把 `events` 归一成 `[]`。
- **CDC 延迟**：只在 `job.cdc_delay?.fresh` 为真时显示徽标。引擎只在延迟 ≥1 秒时上报，60 秒内没有样本就视为已恢复。
- **事件通道开关**：网页端直接 PUT，不带执行请求头，后端对 web 客户端跳过执行协议（和 config 写接口一致）；确认交互用 AlertDialog，并提示需要滚动重启。

## Scenario: 作业详情与 Flink 风格拓扑

### 1. Scope / Trigger
- 运行记录的作业详情（`components/common/sync/job-detail/`）与编辑器「DAG」预览共用一个拓扑组件。设计对照 Flink web-dashboard
  源码（`apache/flink@eead57e`，记录在 `.trellis/tasks/*/10-27-job-detail-dag/research/flink-web-ui.md`）。
- 修改作业行操作、详情页签、拓扑数据合并、节点配色或布局时，必须同步本节。

### 2. Signatures
```ts
// 入口：DataSyncStudio 持有 detailJobId / detailTab；作业对象从 jobs 取，随轮询刷新
openJobDetail(job: SyncJobInstance, tab?: JobDetailTab): void      // 同时 setSelectedJobId(job.id)
type JobDetailTab = 'topology' | 'metrics' | 'checkpoint' | 'logs' | 'events' | 'config' | 'script';
getJobActionState(job, {isAdmin, isOwner, canRun, currentUserId, disableRecover})
  // → {showRecover, canRecover, showStop, canStop}，作业行与详情头部唯一的判断来源
<JobTopology mode='runtime' | 'plan' jobDag metrics? jobId? realtimeEnabled? running? warnings? />
services.sync.getJobPlanDag(jobId)                                   // GET /sync/jobs/:id/plan-dag，warnings 兜底 []
```

### 3. Contracts
- **入口**：点击运行记录行任意处 = 选中作业 + 打开详情；行内只留「详情 / 恢复 / 保存点停止 / 停止」；多表徽标打开「指标」页签；
  告警深链 `/sync?task=&job=` 选中作业后直接打开详情。行内按钮必须 `stopPropagation`。
- **页签**：拓扑｜指标｜Checkpoint｜日志｜事件｜配置｜脚本，只渲染当前页签。日志与 Checkpoint 由工作台以插槽传入，加载条件为
  `bottomConsoleTab === X || (detailJob && detailTab === X)`，与底部面板共用同一份数据，不重复请求。
- **数据合并**（`dag-model.ts`）：拓扑以运行时 `result_preview.job_dag` 为准；计划 DAG（`webui_job.jobDag`）按 `vertexId + type`
  → `type + tablePaths 集合` → `type + 指标前缀` 依次匹配补表结构，匹配不上标 `planMatched=false`。指标按 `Source[0].<表>` 前缀一次索引。
- **实时叠加**：节点忙碌度取 `latestVertexBusy`；连线背压按 `target_vertex_id` 取最新 `bpRatio`；节点“被反压” = 出边背压最大值。
  节点配色照 Flink：默认 `#1890ff`，按忙碌度向 `#ee2222` 混合，再按被反压向 `#888888` 混合（`nodeColors`）。连线只标背压 %（引擎没有按边的吞吐）。
- **布局与性能**：`dag-layout.ts` 用 dagre，参数照 Flink（`LR` / `network-simplex` / `DL` / `ranksep 150` / `edgesep 150`），节点固定
  260px 宽；布局只按 `structureKey` 缓存，轮询刷新只改节点 data。React Flow、dagre 只能在 `job-detail/JobTopology` 动态导入的 chunk 里，
  `dag-model.ts`、`JobDetailDialog.tsx` 等会被工作台静态导入的文件不得引入它们（复制函数在 `copy-text.ts`）。
- **多表**：节点只显示首表 + 数量（`shortTablePath`：去掉库名再中间省略，保证表名结尾可见；可复制）；抽屉表列表 / 选择器 /
  按表指标每次最多渲染 200 行，搜索收窄。
- **卡片配色**（参照 Flink `node.component`）：每个节点都铺同一色相的浅色底（`nodeSurface`，默认 Flink 蓝），颜色代表“算子”而不是
  高亮；运行模式下按忙碌度连续混红、按被反压程度连续混灰，**不设阈值**，所以颜色不会突然出现。底色始终是浅色调（100% 忙碌也只是浅玫红底
  + 红边）。两种主题都从卡片底色向色相混合成实色：浅色从白色混（避免白卡片融进白画布），暗色从 `--card`（#18181b）混（画布点阵不透出）。类型只靠小图标 + 灰色元信息，不加类型色条；Sink 不显示 BP。
- **抽屉状态**：作业列表轮询会换掉 `tablePaths` 数组引用，抽屉的选中表 / 搜索词 / 分页只能按 `vertexId + tablePaths 内容` 重置，
  不能按数组引用（回归测试 `DagNodeDrawer.test.tsx`）。
- **语义文案**：表结构与建表计划都由提交时配置对照数据库**当前**状态推导，界面必须写明；目标表已存在时建表计划显示无需建表，
  不代表作业启动时没有建表。
- **指标页**：引擎原始扩展指标里全为 0 的（如 `FlushSignal*`、`IntermediateQueueSize`，相关能力未触发或没有积压）用
  `isZeroMetricValue` 识别后折叠成一行，可展开；非零的照常分组。不删除这些指标，非零时它们有诊断价值。
- **日志加载**：`logsResult === null`（还没拿到第一份结果）显示骨架，只有拿到空结果才显示“暂无日志 / 切换日志模式”引导；
  自动刷新时保留已显示内容，只用 `TableLoadingBar` 提示，头部固定显示上次更新时间；刷新失败不清空已有日志（回归测试 `ConsoleLogs.test.tsx`）。
- **事件原始数据**：时间线每条事件（含生命周期分组里的子事件）点击展开 `ExpandableTextPanel` 显示完整 JSON（服务端已脱敏），
  格式化摘要只是速览。schema change 摘要：增 / 改列的 `column` 是对象（显示 `name sourceType`），删列的 `column` 是字符串（MySQL CDC 实测）。
- **动态码**：计划 DAG 原因码、实时 reason / hint 码都来自后端，翻译前用 `t.has()`，未知值显示原码。

### 4. Validation & Error Matrix
| 情况 | 表现 |
|---|---|
| 运行时 `job_dag` 缺失 | 拓扑页签显示“引擎还没有返回该作业的执行拓扑” |
| 计划 DAG 加载中 / `available=false` / 请求失败 | 拓扑照常显示；抽屉“表结构 / 建表计划”给一行原因（可刷新） |
| 节点未对应上计划 DAG | 抽屉提示“未能对应到提交配置中的节点” |
| 非 3.x 或非运行中 | 不显示提示条，节点 Busy / BP 显示 `N/A`；「事件」页签在不支持时禁用 |
| 选中节点 | 视口适配“选中节点 + 上下游”（等抽屉改变画布宽度后再 fit），其余节点与连线淡化 |

### 5. Good/Base/Bad Cases
- Good：运行中的 CDC 作业，Source 忙碌 100% 显示为红色，连线 `BP 0%`，提示条给出“读取占用 100% 且下游无背压”，点 Sink 看到建表计划。
- Base：已结束的作业，拓扑与按表指标正常，Busy / BP 为 `N/A`，表结构来自计划 DAG。
- Bad：在画布上 `setCenter` 选中节点却不考虑抽屉，导致下游节点被挤出视口、`onlyRenderVisibleElements` 把它移出 DOM。

### 6. Tests Required
- `job-detail/__tests__/dag-model.test.ts`：作业 #26 真实结构归一化与合并、多 pipeline 去重、500 表索引、实时叠加、Flink 配色、扇入扇出布局不重叠、`structureKey` 稳定。
- `job-detail/__tests__/table-schema.test.ts`：列 / 主键 / 唯一键 / 可空解析，`__default__` 建表计划兜底。
- `job-detail/__tests__/DagNodeDrawer.test.tsx`：同样的表集合、新的对象引用重新渲染后，选中表与搜索词保留。
- `job-detail/__tests__/JobRunsPanel.test.tsx`：点行打开详情并选中、行内按钮数量与禁用、按钮不冒泡、多表徽标打开指标页签、`getJobActionState` 权限矩阵。
- `pnpm build` 后确认 `react-flow__` / `network-simplex` 只出现在非 `/workbench` 首屏 chunk 中。

### 7. Wrong vs Correct
#### Wrong
```ts
// 布局依赖 graph 对象：作业列表每次轮询都会换掉 job_dag 引用，dagre 每几秒重排一次，视口还会跳。
const positions = useMemo(() => layoutDag(graph, size), [graph]);
```
#### Correct
```ts
// 只在结构签名变化时重排，指标与实时数据只更新节点 data。
const layoutKey = `${structureKey(graph)}|${mode}`;
const positions = useMemo(() => layoutDag(graph, size), [layoutKey]);
```

## Scenario: Sync 工作台 Config DAG 兼容 WebUI `jobDag`

### 1. Scope / Trigger
- Trigger：Data Sync Studio 需要展示配置预览 DAG，但不希望长期维护一套与 SeaTunnel Web 完全分叉的 DAG 数据模型。

### 2. Signatures
- 前端 service：
  - `POST /api/v1/sync/tasks/:id/dag`
- config tool（由后端调用，不直接暴露给浏览器）：
  - `POST /api/v1/config/webui-dag`

### 3. Contracts
- `/api/v1/sync/tasks/:id/dag` 返回仍保持 `data` 包裹，但 `data` 至少包含：
  - `nodes`
  - `edges`
  - `webui_job`
  - `warnings`
  - `simple_graph`
- `webui_job` 必须是 **WebUI-compatible pseudo job detail**，最少包含：
  - `jobId`
  - `jobName`
  - `jobStatus`
  - `jobDag.pipelineEdges`
  - `jobDag.vertexInfoMap`
  - `metrics`
  - `pluginJarsUrls`
- `metrics` 即使没有 runtime 指标也不能省；必须提供空壳对象，避免前端 DAG 详情区再做特殊分支。

### 4. Validation & Error Matrix
- `config tool` 支持 `/api/v1/config/webui-dag`：
  - 返回 `webui_job`，前端用兼容 DAG 视图渲染。
- `config tool` 不支持 `/api/v1/config/webui-dag` 或返回非 2xx：
  - `/sync` DAG 预览直接报错，提示后端/agent/proxy 版本不满足要求。
- 只有在用户显式查看调试数据时，才允许展示原始 JSON；它不是主链路 fallback。

### 5. Good/Base/Bad Cases
- Good：
  - `webui_job.jobDag.vertexInfoMap` 与 `pipelineEdges` 完整，前端直接渲染兼容 DAG。
- Base：
  - 返回 `webui_job`，但 `metrics` 是空壳对象；前端仍可稳定渲染。
- Bad：
  - 后端只返回旧 `nodes/edges` 而没有 `webui_job`，前端被迫长期维护两套 DAG 语义。

### 6. Tests Required
- Backend：
  - java-proxy `webui-dag` 映射测试：断言 `vertexInfoMap`、`pipelineEdges`、`metrics` 空壳存在。
  - Go service 测试：当 `InspectWebUIDAG` 可用时，`/sync` DAG 结果必须带 `webui_job`。
  - Go service 测试：当 `InspectWebUIDAG` 失败时，错误应直接向上返回，不再偷偷回退旧接口。
- Frontend：
  - TypeScript 类型检查，确保 `SyncDagResult.webui_job` 与 DAG 视图组件契约一致。

### 7. Wrong vs Correct
#### Wrong
- 直接把 config DAG 当成一份自定义节点列表，前端单独维护新的展示模型。

#### Correct
- 后端统一适配成 WebUI-compatible `jobDag`，前端只消费该兼容结构；旧 config DAG 不再作为 DAG 主链路 fallback 契约。
