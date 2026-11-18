# 设计补充：job-table-core

全局契约见父任务 design.md。这里只写实现层面的细节。

## 组件签名

```ts
// JobTable.tsx
interface JobTableProps {
  preset: JobTablePreset;                       // 'global' | 'task'
  data: Pick<UseJobListResult, 'items' | 'total' | 'loading' | 'refreshing' | 'error' | 'updatedAt'>;
  query: JobListQuery;
  onQueryChange: (patch: Partial<JobListQuery>) => void;   // 排序、翻页都走这里
  pageSize: number;
  sparklines?: SyncJobSparklines | null;
  clusterNames?: Map<number, string>;
  actor: JobActorContext | ((job: SyncJobInstance) => JobActorContext); // 大盘逐行，工作台固定
  initiatorUsers?: NotificationRecipientUser[];
  selection?: {ids: Set<number>; onChange: (ids: Set<number>) => void}; // 不传 = 没有多选列
  activeJobId?: number | null;
  onRowActivate: (job: SyncJobInstance) => void;
  onOpenDetail: (job: SyncJobInstance, tab?: JobDetailTab) => void;
  onAction: (action: JobRowAction, job: SyncJobInstance) => void;
  testIdPrefix: string;                          // 'home-job' | 'studio-job'
  className?: string;
}

// JobTableToolbar.tsx
interface JobTableToolbarProps {
  preset: JobTablePreset;
  query: JobListQuery;
  onQueryChange: (patch: Partial<JobListQuery>) => void;  // 筛选变化时宿主负责把 page 重置为 1
  statusCounts: Partial<Record<SyncJobStatusGroup, number>>;
  bulk?: {selectedCount; running; result; onStop; onRecover; onClear};  // 不传 = 没有批量条
  testIdPrefix: string;
}
```

- `onQueryChange` 统一采用 patch 语义。大盘宿主把它映射到现有的 `update()`（URL，值为 `undefined` 时删除参数）；工作台由 `useJobTableQuery` 提供。
- 筛选项一旦变化，`page` 必须重置为 1。这条在**工具栏内部**处理（调用 `onQueryChange({...patch, page: 1})`），不要依赖宿主。

## 列注册表实现提示

- `JOB_COLUMNS: JobColumnDef[]` 按显示顺序排列；`JobTable` 先按 `presets.includes(preset) && (visibleWhen?.(items, ctx) ?? true)` 过滤，再算 `colSpan`。
- 列表头的排序：`sortKey` 存在、并且 `sortableWhen?.(query) ?? true` 时渲染 `StudioSortableColumnHeader`；否则渲染纯文本，并在 title 里放 `jobTable.sortNeedsRunning`。
- 排序切换规则沿用现状：点击当前降序的列 → 变为升序，其余情况一律降序；同时把 `page` 置为 1。
- 单元格里的点击（复选框、多表徽章、操作区）必须 `stopPropagation`，避免触发 `onRowActivate`。

## useJobList 实现提示

- 用 `usePolledResource` 实现。listKey 由 `scope + 规范化后的 query` 序列化得到；`refreshKey` 变化时调用 `reload()`。
- 轮询间隔：`items.some(isActive) && activePollMs ? activePollMs : pollMs`。`isActive` 用 `isJobLifecycleActive(getDisplayJobLifecycleStatus(job))`。
- `overlay` 合并：先建 `Map(id → job)`，overlay 里的记录 `Date.parse(updated_at)` 更大时覆盖。**注意：overlay 中的作业不带 `live`**（工作台用 `getJob` 刷新，没有 `with_live`），覆盖时保留原行的 `live`，只覆盖状态和时间类字段。实现方式为 `{...overlayJob, live: row.live, task_name: row.task_name}`。
- `pinnedJobId`：条件是 `page === 1`、`statusGroup === ''`、`keyword === ''`、`runType === 'all'`，并且本页不包含该作业。满足时 `getJob` 取回作业并插到最前面；`total` 不变。取不到时静默忽略（工作台会自己 toast）。
- 返回值中的 `statusCounts` 取自 `data.status_counts`。

## 大盘宿主（JobListTab）改造后结构

```
JobListTab
 ├─ useJobList({scope:{kind:'all'}, query: fromUrl(query), pageSize:50, pollMs:30_000})
 ├─ useJobSparklines(items, updatedAt)
 ├─ useJobBulkActions(reload)
 ├─ useNotifiableUsers()           // 新增小 hook（或放进 job-table），给发起人列用
 ├─ <JobTableToolbar preset='global' bulk={...}/>
 ├─ <JobTable preset='global' selection={...} onRowActivate={setPreview} .../>
 ├─ <AlertDialog/>（批量 + 单行确认共用一个，状态改为 {action, jobs}）
 └─ <JobPreviewDrawer onOpenFull={...}/>
```

## i18n

把 `dashboard.home.list` 中列、单位、提示相关的键复制到 `jobTable`（`columns.*`、`units.*`、`avgPrefix`、`avgRateHint`、`ranFor`、`tookFor`、`sortNeedsRunning`、`sparklineLabel`、`groups.*`、`mode.*`、`bulk.*` 等），再加上工作台带过来的 `you`、`multiTableBadge`、`viewMultiTableMetricsHint`、`cdcDelayTooltip`、运行方式相关键。旧键的清理留到 D 完成后统一做（父 implement）。
