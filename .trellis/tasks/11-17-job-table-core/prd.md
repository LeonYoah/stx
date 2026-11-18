# 抽取 JobTable 与 useJobList，迁移大盘作业列表

> 父任务：`11-17-unified-job-table`。列清单、类型、预设差异以父任务 design.md 为准，本文件只写本子任务的范围和验收标准。

## 目标

建立 `frontend/components/common/sync/job-table/` 共享模块，把大盘「作业」页签（`dashboard/JobListTab.tsx`）迁移上去。迁移后大盘使用 **global 预设**，并按父 design §4 补齐新列（运行方式、发起人、时间线合并、结果提示）。

## 范围

做：

- 父 design §2 中除 `JobDetailHost` / `use-job-logs` / `use-job-checkpoint` 以外的全部文件
- `JobTable` 同时支持 `global` 和 `task` 两个预设（task 预设由本子任务的单测覆盖，接入工作台在 D）
- `JobListTab` 改为「宿主」：只保留 URL 查询状态、批量确认对话框、预览抽屉、集群名映射和操作回调
- 行操作改为 `JobRowActions`：详情（C 完成前暂时打开预览抽屉，C 完成后改为 `JobDetailHost`）、停止 / 恢复 / savepoint 停止（带确认）、更多菜单中的「在工作台打开」
- 预览抽屉头部加「完整详情」按钮（C 完成前跳工作台深链，C 完成后打开 `JobDetailHost`）
- 搬迁 `dashboard/job-list-utils.ts`、`dashboard/use-job-bulk-actions.ts` 到 `sync/job-table/`，并修改所有引用
- 新 i18n 命名空间 `jobTable`（zh/en）

不做：

- 工作台接入（D）
- 后端（A）

## 需求要点

1. 列和单元格严格按父 design §4.1。`job-table-columns.tsx` 是唯一实现，`JobTable` 不允许按列 id 写特殊分支（复选框列除外）。
2. `useJobList` 实现父 design §3.2 的全部参数，包括 `refreshKey` / `pinnedJobId` / `overlay`。这三个大盘用不上，但必须实现并有单测，D 直接使用。
3. 速率 / CDC 排序在非运行中组时的回退逻辑放进 `useJobList`（D8），从 `JobListTab` 里删掉。
4. 零塌陷加载：首屏显示骨架行；刷新时显示 `TableLoadingBar` 并把行变暗；错误和空状态占满整行（沿用现有实现）。
5. 原子信息防换行：运行方式、速率、条数、时间这几格都 `whitespace-nowrap`，标识符用 truncate + title。
6. 保留现有 testid：`home-job-list`、`home-job-row-{id}`、`home-job-pill-*`、`home-job-search`、`home-job-bulk`、`home-job-preview`。

## 验收标准

- [ ] 大盘作业列表的外观和交互符合父 design §4 / §5 的 global 列
- [ ] URL 筛选、状态胶囊计数、分页、排序、迷你曲线、批量停止 / 恢复全部保持原有行为
- [ ] 行内停止 / 恢复有确认对话框和结果 toast，完成后列表刷新
- [ ] `dashboard/JobListTab.tsx` 不再包含任何 `<StudioTableCell>`
- [ ] `dashboard/job-list-utils.ts`、`dashboard/use-job-bulk-actions.ts` 已删除，没有残留引用
- [ ] 单测：`job-table-columns`（两个预设的列可见性、平均速率显示、多表徽章、CDC 颜色阈值）、`job-table-format`（搬迁过来的原有测试 + 时长）、`use-job-list`（排序回退、overlay 合并时 updated_at 新者胜出、pinned 置顶只在 page=1 且无筛选时生效、refreshKey 触发重拉）
- [ ] 原有 `JobListTab.test.tsx` 通过（必要时只改 import 路径）
- [ ] `pnpm exec tsc --noEmit` 和相关 `pnpm test` 通过
