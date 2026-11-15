# P3 执行计划

1. [ ] 前端：首页时间选择器（`start` / `end` URL 状态、分段控件联动、自定义窗口不轮询）；集群大盘带上 `start` / `end`。
2. [ ] sync：`DescribeJobsForDashboard`（名称、任务、集群、源 / 目标插件、viewable）、`JobCalendar`、`JobAnnotations` + 测试。
3. [ ] dashboard：`jobs.go` 服务 + handler（`GET /dashboard/home/jobs`）+ 测试；路由、operation、基线、swagger。
4. [ ] 前端：vendor beUI bar-chart / sankey-chart / heat-calendar；`JobsTab`；i18n；单测。
5. [ ] 全量测试、许可证；本地构建验证（跑演示作业看吞吐 / Top N / 流向）；spec；提交；开叠加 PR（base = `feat/home-dashboard-p2`）。
