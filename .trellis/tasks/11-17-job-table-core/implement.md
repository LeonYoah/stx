# 实施步骤：job-table-core

1. **搬迁工具函数**（纯重构，可以单独提交）
   - `dashboard/job-list-utils.ts` → `sync/job-table/job-table-format.ts`；同时搬入 `ConsolePanel.tsx` 中私有的 `resolveJobInitiatorName` 并导出（`ConsolePanel` 改为 import）
   - `dashboard/use-job-bulk-actions.ts` → `sync/job-table/use-job-bulk-actions.ts`
   - 已有测试跟着搬；`grep -rn "job-list-utils\|use-job-bulk-actions" frontend` 改完所有引用
   - 检查：`pnpm exec tsc --noEmit`，`pnpm test -- dashboard`
2. **类型与格式化**：`types.ts`；在 `job-table-format.ts` 中补 `jobDurationSeconds`、`formatRatePair`、`formatTotalPair`，并写单测
3. **数据 hook**：`use-job-list.ts`、`use-job-sparklines.ts`、`use-job-table-query.ts`，并写单测（排序回退、overlay、pinned、refreshKey）
4. **列注册表**：`job-table-columns.tsx`，并写单测（两个预设的列集合、各格的边界值）
5. **外壳**：`JobRowActions.tsx` → `JobTable.tsx` → `JobTableToolbar.tsx`
6. **i18n**：zh.json / en.json 加 `jobTable` 命名空间
7. **迁移大盘**：按 design 中的结构重写 `JobListTab.tsx`；`JobPreviewDrawer` 加 `onOpenFull` 属性
8. **自检**
   - `cd frontend && pnpm test -- job-table dashboard && pnpm exec tsc --noEmit`
   - 本地起服务，在浏览器里检查大盘作业页：筛选、排序、翻页、批量、行操作、抽屉，以及深浅色两种主题
   - 对照 `.trellis/spec/frontend/ui-conventions.md` 的检查清单
9. 中文提交，例如 `refactor(job-table): 抽取共享作业表并迁移大盘作业列表`

## 回滚点

- 步骤 1 完成后可以单独提交；之后任何一步出问题，都可以回退到这个提交
- 步骤 7 之前，大盘仍在使用旧实现，新模块只有单测覆盖
