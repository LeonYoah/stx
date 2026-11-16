# P2 执行计划

1. [x] 时间窗解析迁入 `promquery`（`ParseWindow` / `ResolveRange` / `AutoStep`），sync 改用，原测试全绿。
2. [x] sync：`RunningJobCountsByCluster`（排除预览）+ 测试。
3. [x] dashboard：`cluster.go` 服务 + handler（`GET /dashboard/home/clusters`）+ 测试；路由、operation、路由基线、swagger。
4. [x] 前端：vendor beUI status-bar / composition-chart（+ tooltip 依赖链），`@floating-ui/dom` 直接依赖，MIT 白名单。
5. [x] 前端：`home.service` 加 `getClusters`；`ClusterTab`、`TrendEmptyState`；头部时间窗；移除 Grafana iframe；i18n。
6. [x] 单测、tsc、eslint、`pnpm test`、`go test ./...`、许可证检查。
7. [ ] 本地环境构建验证（有 Prometheus），截图；spec 更新；中文提交；开叠加 PR（base = `feat/home-dashboard-p1`）并链接线程。

回退：首页集群页签只换组件；接口为新增，回退 PR 即可。

## 实施记录（2026-11-15）

- 接口路径改为 `/dashboard/home/clusters`（放在 dashboard 模块，与首页汇总同处）。
- 卡片始终返回全部集群，节点 / 趋势 / 信号按范围过滤（否则选中单个集群后无法切回）。
- 集群卡片用 Arc `radio-cards`（新装）。
- 顺带修复：集群健康把已在告警中心关闭的远程告警仍算作活跃（首页「活跃告警 3」与告警列表为空矛盾）。
- 本地验证：有 Prometheus 时各区块出图、时间窗 / 集群卡片写 URL 并带入请求、节点表与信号无截断；无 Prometheus 分支只有单测覆盖。

