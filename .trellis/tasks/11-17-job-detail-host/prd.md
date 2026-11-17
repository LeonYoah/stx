# 抽取 JobDetailHost，大盘打开完整作业详情

> 父任务：`11-17-unified-job-table`（全局契约见父任务 design.md §7）

## 目标

让 `JobDetailDialog`（拓扑、指标、Checkpoint、日志、事件、配置、脚本）可以脱离工作台使用：大盘点击「详情」就地打开同一个完整详情，不再跳转到工作台。

## 现状

- `JobDetailDialog` 本身是自包含的，只有 `logsSlot` / `checkpointSlot` 由工作台注入（`DataSyncStudio.tsx:6308`）。
- 日志加载（`loadSelectedJobLogs`，约 `DataSyncStudio.tsx:1845`：增量 offset、自适应分块、abort、请求版本号、展开模式 `all`）和 Checkpoint 加载（`loadCheckpointSnapshot` / `loadCheckpointFiles`，约 `:1532` / `:1562`）都内联在 `DataSyncStudio` 中，并且绑定 `selectedJobId`。
- Checkpoint 文件「检查」（`handleInspectCheckpointFile`）依赖编辑器内容和自定义变量，只能在工作台中使用。

## 需求

1. 抽出 `sync/job-table/use-job-logs.ts`：
   ```ts
   useJobLogs({jobId, enabled, keyword, level, expanded?}) → {result, loading, offset, reload, reset}
   ```
   行为和现在的 `loadSelectedJobLogs` 逐项一致：增量拼接、`nextLogChunkSize` 自适应、abort、版本号防串、错误时回到空结果并重置 offset、`enabled` 时 3s 轮询。jobId、keyword、level 变化时 reset。普通模式和展开模式（`all`）用两个 hook 实例，不再共享一组 ref。
2. 抽出 `sync/job-table/use-job-checkpoint.ts`：
   ```ts
   useJobCheckpoint({job, enabled}) → {snapshot, loading, files, filesLoading, reload}
   ```
   行为和现在的两个 load 函数加 3s 轮询（作业 pending/running 且 enabled 时）一致。
3. **工作台改用这两个 hook**，行为不变：底部面板和详情弹窗继续共享 `selectedJobId` 这一份数据；可见性判断 `logsVisible` / `checkpointVisible` 作为 `enabled` 传入。
4. 新增 `sync/job-table/JobDetailHost.tsx`（签名见父 design §7）：
   - 内部 `useJob(jobId)`：作业活动时每 5s 调 `getJob` 刷新；其余时间使用传入的 job
   - `enabled` 只在对应页签打开时为真（`tab==='logs'` / `tab==='checkpoint'`）
   - 日志插槽：`ConsolePanel`；`onExpand` 打开 Host 自带的大日志弹窗（复用工作台的日志弹窗组件，找不到可复用组件时，本期先改为可选属性并隐藏展开按钮，在本文件记录这一决定）
   - Checkpoint 插槽：`CheckpointWorkspacePanel`，不传 `onInspectCheckpointFile`
   - 操作：`onAction` 透传给宿主（大盘负责确认和 toast）
5. `CheckpointWorkspacePanel` 的 `onInspectCheckpointFile` 改为可选：缺省时隐藏「检查」，改为显示「在工作台中检查」链接（`jobWorkbenchHref(job)`）。
6. 大盘接线（B 完成后进行，由 B、C 中后完成的那个负责）：行「详情」、抽屉「完整详情」→ `JobDetailHost`；多表徽章 → `JobDetailHost` 的 `metrics` 页签。

## 验收标准

- [ ] 工作台日志行为不回退：增量追加、级别筛选、关键字搜索、展开弹窗、切换作业后清空、运行中 3s 刷新
- [ ] 工作台 Checkpoint 行为不回退：快照、文件列表、文件检查、运行中刷新
- [ ] 大盘打开任意作业详情，七个页签都能正常渲染；日志和 Checkpoint 有数据；运行中作业状态 5s 内更新
- [ ] 详情关闭后不再有任何轮询请求（在网络面板确认）
- [ ] `use-job-logs` 单测：增量拼接、错误重置、jobId 切换时 reset、abort 后的旧响应被丢弃
- [ ] `DataSyncStudio.tsx` 中 `loadSelectedJobLogs` / `loadCheckpointSnapshot` / `loadCheckpointFiles` 已删除
- [ ] `pnpm exec tsc --noEmit` 和相关测试通过
