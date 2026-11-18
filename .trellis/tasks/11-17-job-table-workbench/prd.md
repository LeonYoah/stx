# 工作台作业列表迁移到 JobTable

> 父任务：`11-17-unified-job-table`。**前置条件：A、B、C 已合入。** 全局契约见父 design.md，特别是 §5 的 task 预设和 D5。

## 目标

把工作台底部「作业」面板（`DataSyncStudio.tsx:5804` 的 `<JobRunsPanel>`）换成 `JobTable` 的 task 预设：支持服务端分页、筛选和排序，列和大盘一致，同时不破坏工作台现有的业务判定和联动。

## 需求

1. **数据双轨（D5）**
   - 保留现有 `jobs` + `loadJobs` + 活动作业 3s `getJob` 轮询，作为「任务运行状态」，继续驱动 `activeJobs` / `hasActiveRun` / `hasActivePreview` / `previewJob` / `runJobs` / `recoverableRunJobs` / QuickOpen 最近作业。**这些派生逻辑不改。**
   - 表格单独使用 `useJobList({scope:{kind:'task', taskId}, query, pageSize:20, pollMs:30_000, activePollMs:5_000, refreshKey, pinnedJobId, overlay: jobs})`。
   - `refreshKey`：每次 `loadJobs` 完成后递增（覆盖运行、恢复、停止、切换任务、深链等全部入口）。
   - 切换任务时，用 `useJobTableQuery` 重置查询状态（page=1，清空筛选）。
2. **选中 / 详情作业的解析（新风险，必须处理）**
   - 现在 `selectedJob = jobs.find(id) || jobs[0]`，`detailJob = jobs.find(id)`。表格可以翻到 `jobs` 以外的作业，这时选中会错误地回退到 `jobs[0]`，导致底部日志跟错作业，或者详情打不开。
   - 改为从 `knownJobs` 查找，它按以下优先级合并：`jobs`（最新）→ 表格当前页 `items` → `retainedJobs`。`retainedJobs` 是一个 ref Map，在表格中选中或打开详情时，把该行的快照放进去。这样即使翻页离开，选中的作业仍然有记录。
   - `retainedJobs` 中的作业如果处于活动状态，需要纳入现有的 3s 刷新。实现方式：只刷新 `selectedJobId` / `detailJobId` 对应的作业，**不要把它们加入 `jobs`**，以免影响 `activeJobs` 等业务判定。
3. **交互保持现状**：点击行时执行 `setSelectedJobId` 并打开详情（`openJobDetail`）；`activeJobId={selectedJobId}` 用于高亮；行操作接现有的 `handleCancelJob` / `handleRecoverFromHistory`，`actor` 使用任务级上下文（父 design §6）。
4. **深链**：保留 `loadJobs` 里现有的深链逻辑（单独取作业并打开详情）；同时把 `pinnedJobId` 设为深链作业 ID，保证表格第一页置顶显示它。
5. **工具栏**：task 预设——紧凑状态胶囊（带计数）+ 运行类型（全部 / 正式 / 预览）+ 可展开的关键字搜索。底部面板高度有限，工具栏占一行，高度 ≤ 36px。
6. **CDC 列**：本页任意一行有 `live.cdc_delay_seconds` 时才显示（父 design §4.1）。
7. 删除 `JobRunsPanel` 及其独有的辅助代码；`job-detail/__tests__/JobRunsPanel.test.tsx` 中的用例迁移为 `JobTable` task 预设的测试（覆盖的行为点不能少）。

## 验收标准

- [ ] 任务超过 20 次运行时可以翻页；状态筛选、运行类型筛选、排序都走服务端
- [ ] 翻到第 2 页选中某个作业：底部日志 / Checkpoint 跟随该作业；打开的详情是这个作业；翻回第 1 页后选中状态不丢
- [ ] 运行按钮的禁用状态（存在活动运行或预览时）不受表格翻页和筛选影响
- [ ] 运行一次：表格 5s 内出现新行并自动选中（沿用现有行为）；运行中速率和条数刷新
- [ ] 停止 / savepoint 停止 / 恢复后，表格立即刷新；恢复成功后选中新作业
- [ ] 深链 `/workbench?task=&job=`（作业不在最近 50 条内）：打开详情，表格第一页置顶显示该作业
- [ ] 列、格式、行操作和大盘一致（父任务集成验收逐项对比）
- [ ] 工作台底部面板在 1280×800 视口下，表格可以横向滚动，工具栏不换行
- [ ] `ConsolePanel.tsx` 中已没有 `JobRunsPanel`；`pnpm exec tsc --noEmit` 和相关测试通过
