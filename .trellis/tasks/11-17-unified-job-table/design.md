# 设计：统一作业列表（JobTable）

本文是全局契约。各子任务的 design/implement 只补充本子任务的细节，**不得与本文冲突**；需要偏离时先改本文。

## 1. 架构总览

```
                         ┌────────────────────────── sync/job-table/（共享模块）──────────────────────────┐
                         │                                                                               │
  dashboard/JobListTab ──┤  useJobList(scope, query)  ──►  JobTableToolbar   JobTable(preset)            │
   (URL 查询状态)          │  useJobSparklines(items)        │                  ├─ job-table-columns（唯一列定义）│
                         │  useJobTableQuery(本地状态)       │                  ├─ JobRowActions            │
  sync/DataSyncStudio ───┤                                  │                  └─ job-table-format          │
   (本地查询状态 + 任务运行状态) │  JobDetailHost ──► JobDetailDialog（已有）                                   │
                         │    ├─ useJobLogs   ├─ useJobCheckpoint                                          │
                         └───────────────────────────────────────────────────────────────────────────────┘
                                         │
                                GET /sync/jobs?with_live=1&with_counts=1   GET /sync/jobs/sparklines
                                         │
                              后端 buildJobLiveMetrics（唯一指标口径，子任务 A）
```

分层原则：

- **数据层**（`useJobList` / `useJobSparklines`）：只负责取数、轮询和合并，不含 JSX。
- **呈现层**（`JobTable` / 列注册表 / 工具栏）：纯受控组件，不发请求、不读 URL、不持有业务状态。
- **宿主层**（`JobListTab` / `DataSyncStudio`）：持有查询状态（URL 或本地），注入操作回调、权限上下文和点击行的行为。

## 2. 目录与文件

全部放在 `frontend/components/common/sync/job-table/`（作业属于 sync 领域，大盘已经从 sync 引 `StudioDataTable`）：

| 文件 | 职责 |
|---|---|
| `types.ts` | `JobTablePreset`、`JobColumnId`、`JobTableScope`、`JobListQuery`、`JobRowAction`、`JobActorContext` |
| `job-table-format.ts` | 从 `dashboard/job-list-utils.ts` **搬入**：`jobDisplayName`、`jobWorkbenchHref`、`isRunningStatus`、`jobTimeBounds`、`jobListRates`；从 `ConsolePanel.tsx` 搬入目前是私有函数的 `resolveJobInitiatorName` 并导出；新增 `jobDurationSeconds`、`formatRatePair`、`formatTotalPair`。`dashboard/job-list-utils.ts` 删除，引用处改路径 |
| `job-table-columns.tsx` | **唯一**的列定义（见 §4） |
| `JobTable.tsx` | 表格外壳：表头、行、首屏骨架、错误 / 空状态、页脚分页、刷新条 |
| `JobTableToolbar.tsx` | 状态胶囊、关键字、模式 / 运行类型筛选、批量操作条 |
| `JobRowActions.tsx` | 行内操作（详情 + 主操作 + 更多菜单） |
| `use-job-list.ts` | 列表取数 hook（见 §3） |
| `use-job-sparklines.ts` | 从 `JobListTab` 抽出的批量迷你曲线 |
| `use-job-table-query.ts` | 本地查询状态（工作台用）；和 URL 版的 patch 语义一致 |
| `use-job-bulk-actions.ts` | 从 `dashboard/` **搬入**，逻辑不变 |
| `JobDetailHost.tsx`、`use-job-logs.ts`、`use-job-checkpoint.ts` | 子任务 C 产出（见 §7） |
| `index.ts` | 公共导出 |
| `__tests__/` | 列、格式化、hook 的单测 |

## 3. 数据契约

### 3.1 类型

```ts
/** 列表范围 / List scope */
type JobTableScope =
  | {kind: 'all'}                      // 大盘：全局，固定 exclude_run_type=preview
  | {kind: 'task'; taskId: number};    // 工作台：当前任务，含预览运行

/** 受控查询状态（URL 或本地都用这个形状）/ Controlled query state */
interface JobListQuery {
  statusGroup: SyncJobStatusGroup | '';   // '' = 全部
  mode: 'streaming' | 'batch' | '';       // 仅 all 预设暴露
  runType: 'all' | 'formal' | 'preview';  // 仅 task 预设暴露；formal → exclude_run_type=preview，preview → run_type=preview
  keyword: string;
  clusterId?: number;                     // 仅 all
  sort: SyncJobListSort;                  // 默认 started_at
  order: 'asc' | 'desc';                  // 默认 desc
  page: number;                           // 从 1 开始
}
```

### 3.2 `useJobList`

```ts
useJobList({
  scope, query, pageSize,
  pollMs,          // 无活动作业时的轮询间隔：all=30_000，task=30_000
  activePollMs,    // 本页有活动作业时的间隔：all=不设（沿用 pollMs），task=5_000
  refreshKey?,     // 宿主外部变化（运行 / 停止 / 恢复后）递增，立即重拉
  pinnedJobId?,    // 深链作业：不在本页时单独 getJob，置顶插入（只在 page=1 且无筛选时插入）
  overlay?,        // 宿主手里更新的作业副本（见 D5），按 id 覆盖，updated_at 新者胜出
}) → {
  items, total, statusCounts, loading, refreshing, error, updatedAt, reload,
}
```

- 请求固定携带 `with_live=1&with_counts=1`。
- **速率 / CDC 排序约束**（后端只在运行中组支持内存排序）：`statusGroup !== 'running'` 时，`rate` / `cdc_delay` 回退到 `started_at`。这条逻辑放在 hook 里，不放在宿主。
- 基于现有 `dashboard/use-polled-resource.ts`。该文件保留在 dashboard，可以直接引用，也可以搬到 `sync/job-table/`（子任务 B 决定，二选一，不复制）。

### 3.3 指标口径（子任务 A）

前端列表**只读 `job.live`**：

```ts
interface SyncJobLiveMetrics {
  read_qps?: number; write_qps?: number;      // 仅运行中
  read_total?: number; write_total?: number;
  cdc_delay_seconds?: number;                 // 仅运行中且 fresh
  table_count?: number;                       // 新增：逻辑表数量，>1 显示多表徽章
}
```

后端回退顺序和 `extractJobMetricSummary` 对齐：`SinkWriteCount → SinkCommittedCount`、`SinkWriteQPS → SinkCommittedQPS`；全局指标缺失时汇总表级 `Table*` 映射。`table_count` 按逻辑表去重（`Source[0].t` 与 `Sink[0].t` 算同一张表）。

`extractJobMetricSummary` 保留给详情页使用，列表代码里不再调用。

## 4. 列定义（唯一来源）

列注册表的形状：

```ts
interface JobColumnDef {
  id: JobColumnId;
  header: (ctx) => ReactNode;
  sortKey?: SyncJobListSort;                    // 有则可排序
  sortableWhen?: (query) => boolean;            // rate/cdc_delay：statusGroup==='running'
  align?: 'start' | 'end';
  minWidth?: number;
  presets: JobTablePreset[];                    // 出现在哪些预设里
  visibleWhen?: (items, ctx) => boolean;        // 动态显示（cdc 在 task 预设里用）
  cell: (job, ctx: JobCellContext) => ReactNode;
}
```

`JobCellContext` 包括 preset、`updatedAt`（运行中时长基准）、sparklines、`clusterNames`、`actor`、`t`、`format`、`onOpenDetail`、`onAction`。

### 4.1 列清单（按显示顺序）

| id | 表头 | global | task | 单元格内容 | 排序 |
|---|---|---|---|---|---|
| `select` | ☐ | ✅ | ❌（默认，见 PRD 待确认） | 复选框；点击不冒泡到行 | – |
| `job` | 作业 | ✅ | ✅ | **global**：主行 `jobDisplayName`；副行 mono `#id · engine_job_id`，hover 时 title 显示 `任务 #task_id`。**task**：主行 `#id`；副行 mono `engine_job_id \|\| platform_job_id`。都用 truncate + title | – |
| `cluster` | 集群 | ✅ | ❌ | `clusterNames.get(id) ?? #id`，无则 `-` | – |
| `run` | 运行方式 | ✅ | ✅ | 运行方式徽章（运行 / 恢复 / 调度 / 预览，`getRunModeLabel`）+ 模式徽章（流 / 批）+ mono `v{task_version}` + 通道（Local / V1 / REST V2）。整组 `whitespace-nowrap` | – |
| `status` | 状态 | ✅ | ✅ | 状态徽章 + `JobOutcomeHints` | `attention` |
| `initiator` | 发起人 | ✅ | ✅ | `resolveJobInitiatorName`，本人加「你」徽章；用户列表来自 `services.monitoring.listNotifiableUsers()`，没有时回退 `#uid` | – |
| `rates` | 读 / 写速率 | ✅ | ✅ | 迷你曲线（`sparklines.source==='prometheus'` 时，w-24）+ `读 / 写 /s`；已结束显示平均值：浅色 + 「均」前缀 + title 提示（`jobListRates`） | `rate`（仅运行中） |
| `totals` | 读 / 写条数 | ✅ | ✅ | `读 / 写`（compact 数字）；`live.table_count>1` 时加多表徽章，点击 → `onOpenDetail(job,'metrics')` | – |
| `cdc` | CDC 延迟 | ✅ | 动态：本页任一行有值时显示 | `x.xs`；≥60s 红色，>0 琥珀色；有 `job.cdc_delay` 时 tooltip 显示 1 分钟内最大值和采样时间 | `cdc_delay`（仅运行中） |
| `timeline` | 时间 | ✅ | ✅ | 合并原来的「开始 / 结束」两列：主行开始时间（`formatDateTimeMinuteTz`）；副行浅色「已运行 3h12m」或「耗时 5m」；tooltip 显示开始、结束、时长全量。时间边界一律用 `jobTimeBounds`（兼容引擎时区错位） | `started_at` |
| `actions` | 操作 | ✅ | ✅ | `JobRowActions`（§6） | – |

两个预设的表格最小宽度：global 约 1180px，task 约 1000px。底部面板横向滚动由 `StudioTableScroll` 处理。

### 4.2 格式化统一

- 数字：`formatCompactNumber`（从 `dashboard/dashboard-format.ts` 引用或搬入 `job-table-format.ts`），**不再使用** `formatMetricValue`。
- 时长：`formatDurationSeconds` + `jobDurationSeconds(job, nowMs)`，**不再使用** `formatJobDuration`。
- 时间：`formatDateTimeMinuteTz`；tooltip 里用秒级 `formatJobDateTime`。

## 5. 预设与宿主差异（只允许这些差异）

| 维度 | global（大盘） | task（工作台） |
|---|---|---|
| scope | `{kind:'all'}` | `{kind:'task', taskId}` |
| 查询状态存放 | URL（现有 `useDashboardQuery`） | 本地 `useJobTableQuery`，切换任务时重置 |
| 每页条数 | 50 | 20 |
| 工具栏 | 状态胶囊（标准）+ 关键字 + 模式 | 状态胶囊（紧凑）+ 运行类型（全部 / 正式 / 预览）；关键字收进图标按钮展开 |
| 多选 / 批量 | 有 | 无 |
| 点击行 | 打开预览抽屉（D7） | 选中作业 + 打开详情（保持现状） |
| 选中高亮 | 多选行 | `activeJobId`（工作台当前选中的作业） |
| 轮询 | 30s | 本页有活动作业时 5s，否则 30s；`refreshKey` 立即重拉 |
| 更多菜单 | 含「在工作台打开」 | 不含 |
| testid 前缀 | `home-job`（保持现有 e2e / 单测选择器） | `studio-job` |

## 6. 行操作（`JobRowActions`）

固定结构，两处一样：

1. **详情**（ghost 按钮，图标 `PanelRightOpen`）→ `onOpenDetail(job)`
2. **主操作**（最多一个）：活动作业 → 「停止」；可恢复 → 「恢复」。可见性和禁用状态全部由 `getJobActionState(job, actorCtx)` 判定；禁用时 tooltip 说明原因（沿用 `readOnlyCancelTooltip` / `historyRecoverTooltip`）
3. **更多菜单**（`⋯`，`DropdownMenu`）：「Savepoint 停止」（活动作业）、「在工作台打开」（global）、「复制作业 ID」

```ts
type JobRowAction = 'stop' | 'savepointStop' | 'recover';
onAction(action: JobRowAction, job: SyncJobInstance): void   // 宿主实现
```

宿主实现：

- **工作台**：沿用 `handleCancelJob` / `handleRecoverFromHistory`（包括恢复后选中新作业、诊断弹窗）。
- **大盘**：停止 / savepoint 停止前先 `AlertDialog` 确认，然后直接调 `services.sync.cancelJob` / `recoverJob`，toast 提示结果后 `reload()`。

权限上下文 `JobActorContext`：

| 字段 | 工作台 | 大盘 |
|---|---|---|
| isAdmin | `currentUser.is_admin` | 同 |
| isOwner | `editor.isOwner`（任务级） | `job.created_by === currentUser.id`（逐行；后端仍是最终裁决，失败 toast） |
| canRun | `editor.canRun` | `true`（后端裁决） |
| disableRecover | 本任务有活动运行或预览 | `false`（后端会拒绝同任务并发，错误 toast） |

`getJobActionState` 的签名不改。大盘逐行构造 ctx。

## 7. 作业详情（子任务 C）

- `JobDetailDialog` 不改签名。它的 `logsSlot` / `checkpointSlot` 现在依赖工作台的状态。
- 从 `DataSyncStudio` 抽出：
  - `useJobLogs({jobId, enabled, keyword, level})`：增量 offset、分块大小、abort、版本号防串，3s 轮询（enabled 时）
  - `useJobCheckpoint({job, enabled})`：快照（limit 20）+ 运行时存储文件列表，作业 pending/running 时 3s 轮询
- 工作台用这两个 hook 替换原有内联实现，行为不变（底部面板和详情弹窗继续共享 `selectedJobId` 这一份数据）。
- 新增 `JobDetailHost({job, open, onOpenChange, tab, onTabChange, actor, onAction, monacoTheme})`：内部实例化两个 hook，组装 `ConsolePanel` / `CheckpointWorkspacePanel` 插槽，再渲染 `JobDetailDialog`；还自带 `useJob(jobId)` 轮询（活动时 5s），保证弹窗里的作业是最新的。**大盘用它，工作台不用**（工作台要和底部面板共享数据）。
- Checkpoint 文件「检查」依赖工作台的编辑器内容和自定义变量。`JobDetailHost` 里不传 `onInspectCheckpointFile`，`CheckpointWorkspacePanel` 需要支持该属性可选：缺省时隐藏「检查」按钮，改为显示「在工作台中检查」链接（`jobWorkbenchHref`）。

## 8. 关键决策

| # | 决策 | 理由 |
|---|---|---|
| D1 | 一个 `JobTable`，用预设区分，不做两个组件 | 两边数据接口相同，差异只在范围和宿主；两份列代码必然再次漂移 |
| D2 | 列表指标只认后端 `live` | 消除多表作业两边数字不一致的问题；前端不重复解析引擎指标 |
| D3 | 合并「开始 / 结束」为一列「时间」 | 省出约 160px 给新增的运行方式、发起人两列；工作台已验证这种形式可用 |
| D4 | 呈现层纯受控，查询状态由宿主持有 | 大盘要 URL 可分享，工作台要随任务切换重置，两者不能共用存储 |
| D5 | **工作台的「任务运行状态」和「列表分页」分开** | `DataSyncStudio` 的 `jobs` 数组还驱动 `activeJobs`→运行按钮禁用、`previewJob`、`recoverableRunJobs`、`selectedJob`、QuickOpen 最近作业。如果把它换成「当前页」，翻页或筛选会把这些状态弄错。所以：`jobs` 继续由现有 `loadJobs`（最近 50 条 + 活动作业 3s 刷新）维护，**只用于业务判定**；表格用自己的 `useJobList`，通过 `overlay={jobs}` 合并更新的副本（行状态和底部面板一致），通过 `refreshKey` 在运行 / 停止 / 恢复后同步刷新。**附带风险**：`selectedJob` / `detailJob` 现在只在 `jobs` 中查找，翻页后会解析失败。改为从 `knownJobs`（jobs → 当前页 → retainedJobs）中查找，详见子任务 D 的 prd |
| D6 | task 预设先不开放批量 | 底部面板空间有限，单任务很少需要批量操作；以后要开只需改预设开关 |
| D7 | 大盘点击行仍打开预览抽屉；「详情」按钮打开完整详情；抽屉头部加「完整详情」按钮 | 全局巡检需要不打断上下文的速览，完整详情是深入查看。两处的「详情」按钮语义一致（都打开 `JobDetailDialog`） |
| D8 | `rate` / `cdc_delay` 排序回退逻辑放进 `useJobList` | 这是后端约束，两个宿主都要遵守，放在宿主里会写两遍 |

## 9. 兼容性与回滚

- 不改后端接口签名，只给 `live` 加字段（`omitempty`），老前端不受影响。
- 每个子任务单独提交，可以单独回滚。D 回滚后工作台恢复使用 `JobRunsPanel`（D 在最终提交前保留旧组件，验收通过后再删）。
- i18n：新建命名空间 `jobTable`（zh/en）。旧命名空间 `dashboard.home.list` / `workbenchStudio` 里不再使用的键在 D 完成后清理，清理前用 grep 确认没有引用。

## 10. CLI

- `GET /sync/jobs` 没有新增参数。子任务 A 更新 `internal/operation/registry_sync.go` 中 `with_live` 的说明（加上条数和 table_count），并在 Swagger 注释变化时重新生成 `docs/docs.go`。
- 不新增接口，所以不需要新增 CLI 命令。
