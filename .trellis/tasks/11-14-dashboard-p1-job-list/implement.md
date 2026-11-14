# P1 执行计划

## 0. 前置

- [x] 父 PRD Q1–Q3 已回答（2026-11-14）。
- [ ] 把 `local/pr-134-135` 上未进 main 的 PR #135 后续提交整理成 PR 合入 main（独立于本任务，需用户确认推送）。
- [ ] 从最新 `main` 拉分支 `feat/home-dashboard-p1`。
- [ ] 加载 `trellis-before-dev`：backend（database / error-handling / cli-guidelines / quality / job-status-reconciler）、frontend（ui-conventions / api-and-services / directory-structure）、guides/cross-layer-thinking-guide。

## 1. 后端：作业表两列 + 回填（回退点 A）

- [ ] `JobInstance` 加 `ClusterID`、`JobMode`；创建作业实例的三处（提交 / 恢复 / 接管）写入。
- [ ] 启动回填（批量、幂等）+ 单测。
- [ ] `go test ./internal/apps/sync/...`；`./scripts/test_db_compat.sh`。

## 2. 后端：`GET /sync/jobs` 扩展

- [ ] `JobFilter` 新字段 + handler 解析与校验（非法 `sort` / `status_group` → 400）。
- [ ] repository：状态组、模式、集群、关键字（LEFT JOIN + `LOWER LIKE`）、attention 排序、`with_counts`。
- [ ] 运行中组内存排序（rate / cdc_delay）。
- [ ] 响应装饰：`task_name`、`cluster_name`、`live`（批量取名，无 N+1）。
- [ ] 非管理员可见范围补齐 + 测试。
- [ ] 旧参数回归测试（不传新参数时结果与顺序不变）。
- [ ] operation 注册表补 `inputs`（CLI 自动生成）；swagger 注释。

## 3. 后端：sparklines 与 home summary

- [ ] Prometheus 访问抽公共包（迁移 `queryPrometheusRange` / `coercePromSample` / 窗口解析），`job_timeseries` 改用，原测试全绿。
- [ ] `GET /sync/jobs/sparklines` + 测试（交集、上限、无 Prometheus）。
- [ ] `internal/apps/dashboard/home.go`：summary + 测试（无 Prometheus 分支、attention 排序与上限）。
- [ ] 路由注册（`/sparklines` 在 `/:id` 前）、operation 注册（`sync.job.sparklines`、`dashboard.home.summary`）、`route_baseline.json`。
- [ ] `go build ./... && go test ./...`。

## 4. 前端：骨架（回退点 B）

- [ ] 安装 `@uiarc/metric-card @uiarc/empty-state @beui/table @beui/sparkline`；MIT 头 + `licenses/third_party_mit_files.txt`。
- [ ] 图标候选列表 → 开发者确认。
- [ ] `HomeDashboard`、`useDashboardQueryState`、`DashboardKpiRow`、`AttentionStrip`、`TrendEmptyState`；`dashboard/page.tsx` 精简，删除旧四宫格。
- [ ] `home.service.ts`、i18n zh / en。

## 5. 前端：作业列表页签

- [ ] `sync.service.ts`：`listJobs` 新参数 / 新字段类型、`getJobSparklines`。
- [ ] `JobListTab`：StatPillsBar、模式 / 集群 / 关键字、beUI table、服务端分页与排序、sparkline 批量取。
- [ ] `JobPreviewDrawer`（Sheet + 1h 趋势 + 失败原因 + 跳工作台深链）。
- [ ] `useJobBulkActions`（停止 / 恢复，复用确认交互，逐条结果）。
- [ ] 单测；`pnpm test`、`pnpm exec tsc --noEmit`、改动文件 eslint。

## 6. 验证

- [ ] 本地构建前后端（t1 只在 nice 19 下单独构建，见 memory），在 `~/stx-local` 环境：
  - 关闭 observability：首页三页签可用、sparkline 列隐藏。
  - 开启 observability：跑一个 CDC demo 作业，核对写入速率、sparkline、CDC 延迟、需要处理条。
  - 非管理员账号核对可见范围。
  - 390px / 中英文截图存入任务目录。
- [ ] CLI：`stx sync job list --status_group failed`、`stx sync job sparklines --ids ...`、`stx dashboard home summary`。

## 7. 收尾

- [ ] trellis-check 全量自检（spec checklist）。
- [ ] spec 更新：frontend/ui-conventions（首页结构、大盘空态）、backend 新增「首页大盘接口」一节或并入 engine-events-and-realtime。
- [ ] 中文提交，开 PR，链接到 T3 线程。

## 回退

- A：作业表新增列为可空 / 默认值，回退代码不影响旧版本读取。
- B：首页只改 `dashboard/page.tsx` 装配，回退还原该文件即恢复旧首页。
