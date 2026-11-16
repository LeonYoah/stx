# P4 设计：Grafana 应用层摘除 + 内嵌口径对齐

需求见 `prd.md`；内嵌架构见 `11-16-embedded-observability/design.md`。本文只写大盘任务这边的落地方式。

## 1. Grafana 应用层摘除

后端（全部是死路径，删除即可，无替代实现）：

| 位置 | 处理 |
|---|---|
| `internal/apps/monitoring/handler.go` `ProxyGrafana` / `isGrafanaLiveWSRequest` / 只为它服务的 `parseProxyTarget` 分支 | 删除（`parseProxyTarget` 若还有别的调用方则保留） |
| `internal/router/router.go` `/monitoring/proxy/grafana` 路由组 | 删除 |
| `internal/router/middlewares.go` `isGrafana*Path` 日志降噪分支 | 删除 |
| `internal/config/model.go` `ObservabilityGrafanaConfig` 与 `Grafana` 字段 | 删除 |
| `internal/config/config.go` `STX_OBSERVABILITY_GRAFANA_URL` 覆盖、默认值、`validateOptionalHTTPURL("observability.grafana.url")` | 删除；对应测试同步 |
| `config.example.yaml`、`config.e2e*.yaml` 的 `grafana:` 段 | 删除 |
| `internal/operation/testdata/route_baseline.json`、`docs/` swagger | 重新生成 |

兼容性：配置加载为 `viper.Unmarshal`（非 Exact），旧文件残留 `observability.grafana` 被忽略。按内嵌任务 Locked Decision 7（破坏性升级）
不打废弃 warn，release note 记一条即可。补一个配置加载测试：含 `grafana:` 段的 yaml 能正常加载。

前端：`components/common/monitoring/MonitoringOverview.tsx` 在 P2 之后已无引用（`ClusterDetail` 只引用同名类型
`ClusterMonitoringOverviewData`，不受影响），整文件删除；逐个检查它用到的 i18n 键，只被它用的键从 zh / en 删除；
`HomeDashboard.tsx` 顶部注释去掉「P1 集群页签暂放 Grafana 折叠面板」过渡说明。

不在本任务处理：integration status 组件（内嵌 P2 / P3）、`install/observability/grafana_config/`、
`deploy/docker/observability/grafana/`、安装 / 打包脚本、三件套文档（内嵌 P4）。

## 2. 文案与空态

现状（zh）：`clusterTab.noProm.{title,desc}`、`jobsTab.noPromDesc`、`preview.noTrend` 四处写「接入 Prometheus」；
`TrendEmptyState` 不区分 reason。

改为按 `reason` 分两套文案（键名保留 `noProm*` 不改，避免无意义的大面积重命名）：

| reason | 标题 | 描述要点 |
|---|---|---|
| `observability_disabled` | 可观测性未开启 | 在配置中开启 `observability.enabled` 后显示趋势；卡片 / 节点表 / 运行日历照常 |
| `prometheus_unavailable` / `prometheus_error` | 监控数据暂不可用 | 内嵌监控存储未就绪或查询失败，稍后自动重试 |

`TrendEmptyState` 增加 `reason` 入参选择文案；集群 / 作业页签与预览抽屉把接口返回的 `reason` 透传。en 同步。

## 3. 查询负载保护：大盘响应缓存

问题：内嵌后 PromQL 在 STX 进程内执行。集群大盘一次约 19 条并发查询、作业大盘约 15 条，每个打开的首页 30 秒轮询一次，
N 个人同时开首页就是 N 倍 CPU。

方案：在 `internal/apps/dashboard` 加一个包内的查询缓存 `promCache`，包住集群 / 作业两个服务里对 `promquery.Range/Instant` 的调用：

- 键：`kind | expr | start | end | step`。预设窗口（`range=1h` 等）的 `end` 先按 15 秒向下取整再算 `start`，
  这样 15 秒内的同参数请求键相同；自定义窗口本来就固定。
- 值：查询结果（`[]Series` / `[]Sample`）与错误；TTL 15 秒；错误只缓存 5 秒，避免抖动放大。
- 并发合并：`golang.org/x/sync/singleflight`（仓库已有则复用），同键同时到达只算一次。
- 上限：最多 256 个键，过期懒删除 + 超限时整体清空（键空间小，不值得上 LRU）。
- 只缓存 Prometheus 结果；DB 部分（集群卡片、节点表、作业名、`viewable`、运行日历、标注）每次请求照常计算，
  所以逐用户字段不会串号。
- `home summary` 与 `sparklines` 查询量小，本期不接缓存。

内嵌引擎层的并发上限（`promql.ActiveQueryTracker` 或 `promquery` 内信号量）属于内嵌任务，已在评审意见中提给内嵌 P2。

## 4. spec 更新

- `frontend/ui-conventions.md` §16：「未接入 Prometheus」改为「可观测性关闭 / 监控数据不可用」，并写明空态按 `reason` 分文案。
- `backend/job-status-reconciler.md` 首页大盘一节：
  - 「节点与 Prometheus 对应靠 `instance = HostIP:HazelcastPort`（与 HTTP SD 一致）」改为「与内嵌抓取目标一致」。
  - 「不依赖 `seatunnel:*` 录制规则（外部 Prometheus 可能没加载）」改为「内嵌方案不实现录制规则，一律写原始表达式」。
  - 新增大盘响应缓存的约定（键、TTL、只缓存时序结果）。

## 5. 复验与抽样核对

- 复验：父 PRD 的跨子任务验收逐条在内嵌环境重跑（本地 `~/stx-local`，observability 开启、无外部 Prometheus 进程查询）。
- 抽样核对（内嵌 P2 合并后、P3 合并前，外部 Prometheus 仍在抓）：同一自定义窗口分别用内嵌 `promquery` 与外部 `curl :9090/api/v1/query_range`
  跑全局写入吞吐、某节点堆使用率、写入 Top 8，记录差异。预期差异来自两边抓取时刻不同（≤ 一个抓取周期的错位），
  超出则回报内嵌任务排查（标签缺失、staleness 处理）。

## 6. 回退

- 代理与配置删除是纯删除，回退 = revert 本 PR。
- 缓存可单独回退：`promCache` 只是包装层，去掉后服务直接调 `promquery`。
