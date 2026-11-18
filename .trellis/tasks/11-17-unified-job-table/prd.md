# 作业列表统一：大盘与工作台共用 JobTable

## 背景

系统里有两张作业实例列表，功能高度重叠，但呈现和交互各做各的：

| | 大盘「作业」页签 `dashboard/JobListTab.tsx` | 工作台底部「作业」面板 `sync/ConsolePanel.tsx#JobRunsPanel` |
|---|---|---|
| 范围 | 全局（排除预览运行） | 当前任务（含预览运行） |
| 数据 | `GET /sync/jobs`，服务端分页 / 筛选 / 排序，`with_live=1`，30s 轮询 | **同一个** `GET /sync/jobs?task_id=`，只取最近 50 条，前端内存排序；活动作业 3s 逐个 `getJob` 刷新 |
| 指标取数 | 后端压平的 `job.live` | 前端 `extractJobMetricSummary` 解析 `result_preview.metrics` |
| 长处 | 服务端分页、状态胶囊计数、迷你曲线、CDC 延迟列、注意力排序、批量停止 / 恢复、行预览抽屉 | 发起人列（含「你」）、时间线列、运行方式·版本·通道、结果提示 `JobOutcomeHints`、多表徽章、完整详情弹窗 `JobDetailDialog`、行内恢复 / savepoint 停止 / 停止 |
| 短板 | 「详情」跳工作台，没有就地完整详情；没有发起人、运行方式 | 超过 50 条的运行被**静默截断**；没有迷你曲线；CDC 只是徽章；不能筛选 |

另外发现一个口径问题：后端 `buildJobLiveMetrics` 只读全局指标；前端 `extractJobMetricSummary` 还会回退 `SinkCommittedCount`，并汇总 SeaTunnel 3.0 的表级指标。**同一个多表作业，两处可能显示不同的条数。**

## 目标

1. 两处作业列表用**同一个组件、同一套列定义、同一套格式化和操作判定**。用户在两处看到的作业行长得一样、含义一样、操作一样。
2. 两处差异只来自**范围**（全局 / 当前任务）和**宿主**（整页 / 底部面板），并通过显式的预设（preset）表达，不允许出现第二份列渲染代码。
3. 作业指标只有一个口径：后端 `live`。前端列表不再自行解析 `result_preview.metrics`。
4. 作业详情只有一套：两处都能就地打开 `JobDetailDialog`（全页签和操作）。
5. 工作台作业列表不再被截断，可以分页、筛选、服务端排序。

## 非目标

- 不重做 `JobDetailDialog` 的页签内容。
- 不改动工作台「运行 / 停止 / 预览」等主操作的判定来源（`activeJobs` / `previewJob` / `recoverableRunJobs` 继续取自工作台现有的任务运行状态，见 design.md D5）。
- 不改大盘其他页签（集群、主机、趋势）。
- 不新增后端接口。只增强现有 `GET /sync/jobs` 的 `live` 字段。
- 大盘预览抽屉的内容不重做，只调整入口（见 design.md D7）。

## 子任务映射

| 顺序 | 子任务 | 交付物 | 依赖 |
|---|---|---|---|
| A | `11-17-job-live-metrics-parity` | 后端 `live` 与工作台口径对齐，并新增 `table_count`；补 CLI 说明 | 无 |
| B | `11-17-job-table-core` | `sync/job-table/` 共享模块（列注册表、`JobTable`、工具栏、`useJobList`、行操作）；大盘迁移完成 | A 合入后再做「条数 / 多表」列；其他部分可先行 |
| C | `11-17-job-detail-host` | `useJobLogs` / `useJobCheckpoint` 从工作台抽出；`JobDetailHost`；大盘「详情」就地打开完整详情 | 可与 B 并行；接线到大盘行操作时依赖 B |
| D | `11-17-job-table-workbench` | 工作台底部作业面板换成 `JobTable`（task 预设），删除 `JobRunsPanel` | A、B、C |

## 跨子任务验收标准

- [ ] 仓库里只剩一份作业列渲染代码（`sync/job-table/job-table-columns.tsx`）。`JobRunsPanel` 和 `JobListTab` 内联的行渲染全部删除。
- [ ] 同一个作业在大盘和工作台显示的状态、发起人、速率、条数、CDC 延迟、时间线**一致**（含多表 3.0 作业、已结束作业的平均速率）。
- [ ] 两处的「详情」按钮都打开同一个 `JobDetailDialog`。大盘中日志、Checkpoint 页签可用。
- [ ] 工作台作业超过 20 条时可以翻页。状态筛选和排序走服务端。
- [ ] 工作台原有行为不回退：点击行会选中作业，底部日志 / Checkpoint 跟随；告警深链 `?task=&job=` 能定位并打开详情；运行 / 恢复 / 停止后列表立即刷新；运行中状态 3–5 秒内更新。
- [ ] 大盘原有行为不回退：URL 筛选、状态胶囊计数、批量停止 / 恢复、迷你曲线、速率 / CDC 排序仅在「运行中」可用。
- [ ] `go test ./...`、`cd frontend && pnpm test`、`pnpm exec tsc --noEmit` 通过；新增文案中英文齐全。
- [ ] 新增或修改的注释中英双语（中文在前）。

## 待确认（实现前由 owner 拍板，默认值已写入 design）

- 工作台每页条数默认 20（底部面板高度有限）。
- task 预设默认不显示多选列（工作台暂不开放批量操作）。
