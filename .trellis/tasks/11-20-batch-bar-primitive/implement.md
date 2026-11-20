# Implement

1. `batch-bar.tsx` + `common` i18n 两键 + `index.ts` 导出；`usePagedSelection`/`mergeIdsIntoSelection` 单测。
2. `InspectionsPanel`、`AlertsPanel` 迁移到原语（语义不变）。
3. `JobTable.bulkBar` prop + `job-table-columns` arc checkbox/merge 全选/indeterminate/aria-label。
4. `JobTableToolbar` 去 bulk；`JobListTab` 迁移（usePagedSelection + mergeKnownJobs 解析 selectedJobs + BatchBar 注入）。
5. 测试：job-table 全选 merge 用例、JobListTab 跨页 selectedJobs 用例；跑 diagnostics/monitoring/job-table/JobListTab vitest + `tsc --noEmit` + go 侧无需动。
6. 死 i18n 键清理（仅确定无引用的）。
