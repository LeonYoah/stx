# 作业列表多选对齐：批量条原语收敛与全选 merge 语义

## 背景

巡检记录列表的批量删除（上一个任务）把「勾选列 + 悬浮批量条」收敛成了共享样式（`list-detail.module.css` 的 `.batchBarWrap/.batchBar`）。但作业列表（`JobListTab`，控制台作业表）走的是 `job-table/` 自成体系的实现，交互不一致：

- 批量条在 toolbar 内联（顶部），样式是散装 `border-primary/30 bg-primary/5`；我们的是表格底部悬浮 + 共享样式。
- Checkbox 用 `@/components/ui/checkbox`，我们用 `@/components/arc/checkbox`。
- 清空是文字按钮；我们的是 ghost X。
- **全选语义 bug**：`SelectAllHeader` 用 `new Set(当前页)` 直接覆盖，跨页勾选会被全选/反选清掉；且 `selectedJobs` 只解析当前页 items，跨页勾选的 id 会计数但不进动作载荷（潜在 bug）。

## 需求

A. 作业列表对齐：
- 批量条移到表格底部悬浮（复用 `.batchBarWrap`），样式统一 `layout.batchBar`。
- Checkbox 换 arc；清空改 ghost X。
- 全选/反选改 merge 语义（只切本页，不丢跨页勾选）。
- `selectedJobs` 跨页解析修复（用 `mergeKnownJobs` 快照累积）。

B. 抽公共原语：
- `mergeIdsIntoSelection` 纯函数 + `usePagedSelection` hook + `BatchBar` 组件（common/layout）。
- 三个面板（巡检/告警/作业列表）共用；各面板保留自己的动作按钮与结果文案。

## 非目标

- 不把 JobTable 迁到 list-detail 行原语。
- DataSyncStudio 的 JobTable 无多选列，不动。
- 不改批量动作的业务语义（stop/recover、串行汇总不变）。

## 验收

- 作业列表全选不丢跨页勾选；跨页选中的作业能进入批量动作。
- 三个面板批量条视觉/清空一致；`tsc --noEmit`、相关 vitest 全过。
