# 实施步骤：job-detail-host

1. 通读 `DataSyncStudio.tsx` 中与日志相关的状态：`jobLogs`、`logsLoading`、`jobLogsOffset`、`expandedJobLogs*`、`logSearchTerm`、`logFilterMode`、`logsDialogOpen`，以及所有 ref 和使用点（`grep -n "jobLogs\|expandedJobLogs\|LogChunk" DataSyncStudio.tsx`）。逐个列出，并写进本文件末尾的「迁移对照表」
2. 实现 `use-job-logs.ts` 和单测（用 fake timers 和 mock 的 `services.sync.getJobLogs`）
3. 工作台替换日志实现，然后手动验证日志的全部行为 → **单独提交**（回滚点 1）
4. 实现 `use-job-checkpoint.ts` 并在工作台替换，手动验证 → **单独提交**（回滚点 2）
5. `CheckpointWorkspacePanel` 的检查按钮改为可选
6. 实现 `JobDetailHost.tsx` 和 `useJob`
7. 大盘接线（如果 B 已经合入）
8. 自检：`pnpm test -- job-table sync`、`pnpm exec tsc --noEmit`；浏览器中验证工作台和大盘两侧
9. 中文提交

## 风险

- 日志 hook 的 ref 和版本号逻辑容易写错，表现为切换作业后串入上一个作业的日志。单测必须覆盖：慢请求返回时 jobId 已经变化，这个响应要被丢弃。
- 工作台 `logsVisible` 依赖 `detailTab` 和 `bottomConsoleTab`。替换时保持原有判定表达式不变。

## 迁移对照表

（步骤 1 完成时填写）
