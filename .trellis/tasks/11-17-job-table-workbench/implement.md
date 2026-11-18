# 实施步骤：job-table-workbench

1. **盘点 `jobs` 的使用点**：`grep -n "\bjobs\b\|selectedJob\b\|detailJob\b" DataSyncStudio.tsx`，把每个使用点归类为「业务判定」（保持 `jobs`）或「列表展示」（改用表格）。结果写在本文件末尾
2. **knownJobs / retainedJobs**：先改 `selectedJob` / `detailJob` 的解析方式，并补上对 retained 活动作业的刷新；这时还在用旧面板，行为应该完全不变 → **单独提交**（回滚点 1）
3. **接入 `useJobList` 和 `useJobTableQuery`**：实现 `refreshKey` 递增、切换任务时重置、`pinnedJobId`
4. **替换面板**：`<JobRunsPanel>` → `<JobTableToolbar preset='task'/>` + `<JobTable preset='task'/>`，外层容器保持 `overflow-hidden`，表格区域 `flex-1` 内部滚动
5. **迁移测试**：把 `JobRunsPanel.test.tsx` 的用例改写到 `job-table/__tests__/`；新增 knownJobs 解析的单测（翻页后选中作业仍然可以解析）
6. **删除 `JobRunsPanel`**，清理不再使用的 import 和 i18n 键（只删确认无引用的键）
7. **自检**
   - `pnpm test -- job-table sync && pnpm exec tsc --noEmit`
   - 浏览器中逐条走一遍 prd 验收标准；找一个运行次数超过 20 的任务（没有的话用 CLI 多跑几次批作业来造数据）
8. 中文提交，例如 `refactor(workbench): 作业面板改用共享 JobTable，支持分页与筛选`
9. 通知父任务开始集成验收

## jobs 使用点盘点

（`DataSyncStudio.tsx`，行号为迁移后位置）

### 业务判定 —— 保持 `jobs`（最新 50 条，3s 活动轮询）

- `previewJob`（~972）：`run_type === 'preview'` 的预览作业，驱动预览面板 / 快照拉取（~1848、~1925）
- `runJobs` / `recoverableRunJobs`（~976、~986）：运行作业集合与恢复来源候选
- `activeJobs`（~1130）→ `hasActiveRun` / `hasActivePreview`（~1137）：运行 / 预览按钮禁用、`jobTableActor.disableRecover`、3s 轮询主集合（~1879）
- QuickOpen 最近作业（~859）：`jobs.slice(0, 50)` 命令面板条目
- `recentJobs`（~6574）：工作台快捷入口
- `loadJobs`（~1562）：唯一写入 `jobs` 的入口；完成后 `jobTableRefreshKey + 1` 通知表格重拉
- `overlay={jobs}`：3s 轮询出的新副本按 id 覆盖表格行显示（不反向改 `jobs`）

### 列表展示 —— 改用 `jobTableList`（useJobList，服务端分页 20 / 筛选 / 排序）

- 底部「作业」页签渲染（~5671）：`JobRunsPanel` → `JobTableToolbar preset='task'` + `JobTable preset='task'`
- 状态计数 `statusCounts`、当前页 `items`、`updatedAt` 均由 `useJobList` 提供；`useJobSparklines` 画速率
- 未打开任务时 `enabled: false`，不发请求

### 解析与轮询交汇（D5）

- `knownJobs = mergeKnownJobs(jobs, items, retainedJobs)`（~1029）：`selectedJob`（~1046）与 `detailJob`（~1053）经它解析，优先级 `jobs` > 当前页 > retained；`selectedJobId === null` 或无解析时仍回退 `jobs[0]`（与旧行为一致）
- `retainJob`：行激活 `openJobDetail`、行操作 `handleJobTableAction`、提交 / 预览 / 恢复 / 定位 / 深链成功后各留一份快照
- `retainedPollJobIds`（~1148）：不在 `jobs` 且仍活动的 selected/detail 作业并入 3s 轮询，结果只回写 retained
- `pinnedJobId`（~1605）：深链作业在表格第一页无筛选时置顶
- `selectedNodeId` 切换 effect（~1821）：重置表格查询、pin、retained，并显式关闭详情弹窗
