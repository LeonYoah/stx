# P2 执行计划

1. [ ] 时间窗解析迁入 `promquery`（`ParseWindow` / `ResolveRange` / `AutoStep`），sync 改用，原测试全绿。
2. [ ] sync：`RunningJobCountsByCluster`（排除预览）+ 测试。
3. [ ] dashboard：`cluster.go` 服务 + handler（`GET /dashboard/home/clusters`）+ 测试；路由、operation、路由基线、swagger。
4. [ ] 前端：vendor beUI status-bar / composition-chart（+ tooltip 依赖链），`@floating-ui/dom` 直接依赖，MIT 白名单。
5. [ ] 前端：`home.service` 加 `getClusters`；`ClusterTab`、`TrendEmptyState`；头部时间窗；移除 Grafana iframe；i18n。
6. [ ] 单测、tsc、eslint、`pnpm test`、`go test ./...`、许可证检查。
7. [ ] 本地环境构建验证（有 Prometheus），截图；spec 更新；中文提交；开叠加 PR（base = `feat/home-dashboard-p1`）并链接线程。

回退：首页集群页签只换组件；接口为新增，回退 PR 即可。
