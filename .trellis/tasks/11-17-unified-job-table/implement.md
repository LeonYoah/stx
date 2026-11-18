# 实施计划（父任务：集成与验收）

父任务本身不写业务代码。它负责排定顺序、把关子任务之间的契约，以及做最终集成验收。

## 顺序

```
A job-live-metrics-parity ──┐
                            ├──► B job-table-core ──┐
C job-detail-host ──────────┴───────────────────────┴──► D job-table-workbench ──► 父任务集成验收
```

- A 和 C 可以并行。B 的「条数 / 多表」列等 A 合入后再接 `live.table_count`；在此之前先渲染 `table_count` 为空的情况，不要临时回退到 `extractJobMetricSummary`。
- C 先交付 hooks 和 `JobDetailHost`。大盘行操作接到 `JobDetailHost` 这一步在 B 完成后进行（由后完成的那个子任务负责接线）。
- D 必须在 A、B、C 全部合入后开始。

## 每个子任务的通用要求

1. 开工前：`trellis-before-dev` 读取 `.trellis/spec/frontend/index.md`（`ui-conventions.md` 中的零塌陷加载、原子信息防换行、一体化表格卡片）；后端子任务读 `.trellis/spec/backend/index.md`。
2. 新增或修改的注释中英双语，中文在前。
3. 文案只走 next-intl，新键放进 `jobTable` 命名空间（zh.json / en.json 同时加）。
4. 自检命令：
   - 后端：`go test ./internal/apps/sync/...`
   - 前端：`cd frontend && pnpm test -- job-table dashboard sync` 以及 `pnpm exec tsc --noEmit`
5. 中文提交信息，每个子任务单独提交。

## 集成验收（D 合入后由父任务执行）

- [ ] 在本地起 API + Web（`go run main.go api`，`cd frontend && pnpm dev`），用 `admin/admin123` 登录
- [ ] 找一个多表 CDC 作业，对比大盘行和工作台行：状态、发起人、速率、条数、多表徽章、CDC、时间线逐项一致
- [ ] 已结束作业：两处都显示「均」速率，数值一致
- [ ] 大盘：点击行打开抽屉；抽屉里「完整详情」以及行上「详情」都打开 `JobDetailDialog`；日志、Checkpoint 页签有数据；停止 / 恢复有确认和 toast
- [ ] 工作台：任务运行超过 20 次时可以翻页；筛选「预览」只显示预览运行；点击行后底部日志跟随；运行一次后列表 5s 内出现新行并在运行中刷新
- [ ] 深链 `/workbench?task=<id>&job=<旧作业>`（作业不在第一页）能打开详情，列表置顶显示该作业
- [ ] `grep -rn "JobRunsPanel\|extractJobMetricSummary" frontend/components/common/sync/job-table frontend/components/common/dashboard` 无列表侧引用
- [ ] 清理 `dashboard.home.list` / `workbenchStudio` 中不再使用的 i18n 键
- [ ] 在 `.trellis/spec/frontend/` 记录「作业列表统一使用 `sync/job-table`」的约定（3.3 规范更新）
