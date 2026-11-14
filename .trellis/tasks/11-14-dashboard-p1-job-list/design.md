# P1 设计：作业列表与顶部指标卡

总体契约见父任务 `design.md`（§2 响应外壳、§3 接口、§4 权限、§6 前端结构）。本文只写 P1 的落地细节。

## 1. 数据模型：作业表补两列

现状：作业的模式与集群只存在 `submit_spec`（JSON）里（`sanitizeJobForResponse` 用 `submitSpecMode` 现算），
三种数据库对 JSON 查询语法不同，不能可靠地在 SQL 里按模式 / 集群过滤。

- `sync_job_instances` 新增 `cluster_id uint`（index）、`job_mode string size:20`（index）。字段名用 `job_mode`，避开已有的非持久化字段 `Mode`（`gorm:"-"`）；
  JSON 仍输出 `mode`（`Mode` 为空时由 `JobMode` 填），不改响应契约。
- 写入：提交 / 恢复 / 接管（attach）创建作业实例时从 `submit_spec` 取值写入。
- 回填：迁移后启动时一次性批量回填 `cluster_id = 0 OR job_mode = ''` 的历史行（每批 500，Go 侧解析 `submit_spec`），幂等，失败只记 warn。
- 迁移走 `AutoMigrate`；提交前跑 `./scripts/test_db_compat.sh`。

## 2. `GET /sync/jobs` 扩展

`JobFilter` 新增：`Statuses []JobStatus`、`Mode string`、`ClusterID uint`、`Keyword string`、`Sort string`、`Order string`。

查询：
- `status`：`IN (...)`。前端「失败」胶囊映射 `failed,failing`；「运行中」映射 `running,doing_savepoint`；「停止中」映射 `cancel_requested,cancelling`；
  「已完成」映射 `success,savepoint_done`。映射表放后端常量（`status_group` 参数），前端只传组名，CLI 也可直接传原始 `status`。
- `keyword`：`LEFT JOIN sync_tasks t ON t.id = sync_job_instances.task_id`，
  `LOWER(t.name) LIKE LOWER(?) OR engine_job_id = ? OR platform_job_id = ?`（按规范统一 `LOWER(...) LIKE LOWER(?)`，不用反引号）。
- 排序：
  - `attention`（列表页默认）：`ORDER BY CASE status WHEN 'failed' THEN 0 WHEN 'failing' THEN 0 WHEN 'running' THEN 1 ... END, started_at DESC`，三库通用。
  - `started_at` / `created_at`：直接排序。
  - `rate` / `cdc_delay`：值在 `result_preview.metrics` / 事件运行时内存中，无法 SQL 排序。仅当状态筛选只含运行中组时允许
    （运行中作业数有限）：取全部匹配行后在内存排序再分页；其他情况返回 400 `sort requires status_group=running`。
    前端只在「运行中」胶囊选中时让这两列表头可点。
  - 缺省（不传 `sort`）：保持 `created_at DESC`，保证旧调用方不变。
- 状态计数：响应新增 `data.status_counts`（按状态组计数，受范围 / 模式 / 关键字筛选影响、不受状态筛选影响），供 `StatPillsBar`；
  仅当请求带 `with_counts=1` 时计算，旧调用方无额外开销。
- 响应项新增（只读装饰，不落库）：`task_name`、`cluster_name`、`live`：
  ```jsonc
  "live": { "read_qps": 0, "write_qps": 0, "read_total": 0, "write_total": 0,
            "cdc_delay_seconds": null, "last_checkpoint_seconds": null, "last_checkpoint_ok": null }
  ```
  `read/write_*` 从 `result_preview.metrics`（`SourceReceivedQPS` / `SinkWriteQPS` / `SourceReceivedCount` / `SinkWriteCount`）解析，
  与 `job_prometheus.go` 用同一组解析函数；CDC 延迟复用 `decorateJobRuntime`。Checkpoint 只在已有缓存时填，不为列表额外请求引擎。
- 任务名 / 集群名批量取（一次 `IN` 查询），不做 N+1。

可见范围：把 `ListJobsForActor` 的非管理员条件从 `created_by = self` 扩为
`created_by = self OR task_id IN (该用户可查看的任务)`，与 `getJobInstanceForActor` 一致；有 `task_id` 时保持现有逻辑。

## 3. `GET /sync/jobs/sparklines`

- 参数：`ids`（逗号分隔，≤100，超出 400）、`range`（默认 1h，取值同趋势接口）、`metric`（`write` 默认 | `read`）。
- 逐作业数据：先与 actor 可见集合求交；一条 PromQL：`sum by(job_id)(rate(stx_job_sink_write_rows_total{job_id=~"1|2|3"}[1m]))`，step 按 range 自动。
- 响应：`{source, reason, step_seconds, items: {"<job_id>": [[ts_ms, v], ...]}}`；无数据的 id 不出现。
- 路由注册在 `/:id` 之前。

## 4. `GET /dashboard/home/summary`

参数：`cluster_id`（可空）。返回：
```jsonc
{
  "jobs": { "running": 9, "running_streaming": 7, "running_batch": 2, "failed_24h": 2, "failed_prev_24h": 1 },
  "throughput": { "write_qps": 27200, "read_qps": 28000, "trend": [[ts,v]...] | [] },   // 当前值来自对账；trend 来自 Prometheus 1h
  "cdc": { "max_delay_seconds": 74.2, "job_id": 12, "job_name": "...", "trend": [] },
  "clusters": { "total": 2, "healthy": 1, "nodes_online": 4, "nodes_total": 4 },
  "alerts": { "active": 3, "critical": 3 },
  "attention": [ { "kind": "job_failed"|"cdc_delay"|"alert"|"backpressure", "severity": "critical"|"warning",
                   "title": "...", "target": { "type": "job"|"alert"|"cluster", "id": 1, "task_id": 2, "viewable": true }, "at": "RFC3339" } ]
}
```
- 计数与速率对所有登录用户为全局口径（父 PRD Locked Decision 6）；`attention` 中当前用户无权查看的作业 `viewable=false`，前端不可点开。
- 集群 / 节点 / 告警计数复用 `dashboard.OverviewService` 与 `monitoring.Service`，不另写统计。
- `attention` 最多 10 条，严重在前；CDC 阈值取 CDC 延迟告警配置（无则 60 秒）；背压项只在 Prometheus 可用时出现。
- 服务放在 `internal/apps/dashboard`（新增 `home.go`），依赖以接口注入 sync / monitoring，避免循环引用。

## 5. 前端

- `app/(main)/dashboard/page.tsx` 精简为装配 `HomeDashboard`；原四宫格删除，`MonitoringOverview compact` 暂放在「集群」页签里（P2 替换）。
- 状态：`useDashboardQueryState()` 读写 URL（`tab`、`cluster`、`range`、列表的 `status`、`mode`、`q`、`sort`、`page`）。
- 数据：`lib/services/dashboard/home.service.ts`（summary），`sync.service.ts` 扩展 `listJobs` 参数与 `getJobSparklines`。
  轮询 30 秒，`document.hidden` 时暂停；切页签不重复请求未显示的数据。
- 组件安装：`@uiarc/metric-card`、`@uiarc/empty-state`、`@beui/table`、`@beui/sparkline`。安装后检查新增依赖，登记 MIT 白名单。
- 行预览抽屉：用现有 `Sheet`（`components/ui/sheet.tsx`），趋势复用 `getJobTimeseries(range=1h, metrics=throughput)` 与 Arc line-chart。
- 批量停止 / 恢复：复用工作台的确认与调用逻辑（抽成 hook `useJobBulkActions`），逐条结果用表格行内状态 + 汇总提示呈现，不只靠 toast。
- 图标按 ui-conventions「先列候选、确认后再改」：指标卡、需要处理条用到的图标在实现前列出候选给开发者确认。

## 6. 测试

- Go：repository 筛选 / 排序 / 计数（SQLite 单测 + `test_db_compat.sh` 三库）、回填幂等、可见范围、sparklines 交集与 PromQL 拼装（`httptest` 假 Prometheus）、summary 无 Prometheus 分支。
- 前端：`JobListTab`（筛选写 URL、排序可用性、无 Prometheus 隐藏 sparkline）、`DashboardKpiRow` 空 / 加载 / 错误态、批量操作结果呈现。
- CLI：operation 注册表测试 + 路由基线 `internal/operation/testdata/route_baseline.json` 更新。
