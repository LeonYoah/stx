# Design

## 公共原语（components/common/layout/batch-bar.tsx，经 index.ts 导出）

```ts
mergeIdsIntoSelection<T>(current: ReadonlySet<T>, ids: readonly T[], on: boolean): Set<T>
usePagedSelection<T>(pageIds: readonly T[]): {
  ids, setIds, toggle, togglePage, clear, pageAllSelected, pageSomeSelected
}
```

`BatchBar` 渲染 `layout.batchBarWrap > layout.batchBar`（role=status）：
- props：`selectedCount`、`children`（动作按钮区）、`result?: ReactNode`（结果行，parent 组好文案）、`busy?`、`onClear`、`selectedText?`/`clearLabel?`（默认 common 键）。
- `selectedCount>0 || result` 时才渲染，宿主只负责放到相对定位容器里。

i18n：`common.selectedCount`（zh 已选 {count} 条 / en {count} selected）、`common.clearSelection`（清空选择 / Clear selection）。作业列表传 `t('selected', {count})` 保留「个作业」措辞。

## 作业表接入

- `JobTable` 新增 `bulkBar?: ReactNode`，直接渲在外层 `relative` div 内（`bottom:58px` 的 wrap 正落在 `StudioTableFooter` 上方）。
- `JobTableToolbar` 删掉 `bulk` prop 与 bulkBar JSX。
- `job-table-columns.tsx`：换 `@/components/arc/checkbox`；`SelectAllHeader` 用 `mergeIdsIntoSelection` + indeterminate；`SelectAllHeader` 的 aria-label 改为全选语义。
- `JobListTab`：`usePagedSelection`（pageIds=list.items）；`selectedJobs` 改走 `mergeKnownJobs` 累积快照按 id 解析（跨页可命中）；批量条以 `<BatchBar>` 传进 `JobTable.bulkBar`，按钮/结果文案沿用 `jobTable` 命名空间。

## 面板迁移

- `InspectionsPanel`、`AlertsPanel`：删内联选择逻辑 → `usePagedSelection`；批量条 JSX → `<BatchBar>`，各自动作按钮与结果文本作为 props 传入。语义不变，纯收敛。

## 兼容

- `BatchBar` 默认文案与巡检/告警现有文案逐字一致（已选 N 条/清空选择），测试断言不变。
- `usePagedSelection.togglePage` = 现有两个面板的 merge 实现原样上移；作业表因此获得相同行为。
