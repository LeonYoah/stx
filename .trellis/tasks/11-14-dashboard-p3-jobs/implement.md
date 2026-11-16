# P3 执行计划

1. [x] 前端：首页时间选择器（`start` / `end` URL 状态、分段控件联动、自定义窗口不轮询）；集群大盘带上 `start` / `end`。
2. [x] sync：`DescribeJobsForDashboard`（名称、任务、集群、源 / 目标插件、viewable）、`JobCalendar`、`JobAnnotations` + 测试。
3. [x] dashboard：`jobs.go` 服务 + handler（`GET /dashboard/home/jobs`）+ 测试；路由、operation、基线、swagger。
4. [x] 前端：vendor beUI bar-chart / sankey-chart / heat-calendar；`JobsTab`；i18n；单测。
5. [x] 全量测试、许可证；本地构建验证（跑演示作业看吞吐 / Top N / 流向）；spec；提交；开叠加 PR（base = `feat/home-dashboard-p2`）。

## 实施记录（2026-11-16）

- 运行日历并入 `/dashboard/home/jobs`，不单独做 daily-stats 接口。
- vendored beUI sankey 本地适配：几何同时作为静态属性（motion 未写入时整图空白）。
- 本地验证：跑 FakeSource 演示作业，用自定义时间窗看历史段：吞吐、写入 Top 8、流向、Checkpoint、热点、日历均出图；预设 / 自定义切换写 URL 并带入请求。
- 未验证：CDC Top 8（本地无 CDC 作业）、失败标注（本机引擎时区错位，`finished_at` 偏 8 小时落在窗口外）、关闭可观测性的真实环境。

