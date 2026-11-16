# P4 执行计划

依赖：内嵌可观测 P2 已合并（见 `prd.md` Dependencies）。分支从届时的内嵌 P2 末端拉 `feat/home-dashboard-p4`，PR base 跟随。

## 0. 前置

- [ ] 确认内嵌 P2 已合并、本地 `~/stx-local` 换成内嵌版二进制后大盘有数据。
- [ ] 加载 `trellis-before-dev`：backend（error-handling / cli-guidelines / quality / job-status-reconciler）、frontend（ui-conventions / api-and-services）。

## 1. Grafana 应用层摘除（回退点 A）

- [ ] 后端：代理 handler、路由组、中间件分支、配置结构 / 默认值 / 校验 / 环境变量、example 与 e2e 配置。
- [ ] 配置测试：含 `observability.grafana` 段的 yaml 正常加载。
- [ ] 前端：删除 `MonitoringOverview.tsx` 与专用 i18n / 测试；清理 `HomeDashboard` 注释。
- [ ] `go run ./internal/operation/cmd/contract-report -write`（不写入就再跑一次）；`swag init -o docs --parseDependency --parseInternal`。
- [ ] `go build ./... && go test ./internal/config/... ./internal/apps/monitoring/... ./internal/router/... ./internal/operation/...`。

## 2. 大盘响应缓存（回退点 B）

- [ ] `internal/apps/dashboard/prom_cache.go`：键、15 秒 TTL、错误 5 秒、singleflight、256 键上限；中英双语注释。
- [ ] 集群 / 作业服务改走缓存；预设窗口 `end` 按 15 秒取整。
- [ ] 测试：同参数并发 N 次只打一轮假 Prometheus；TTL 过期后重查；错误缓存 5 秒；`viewable` 按 actor 不串号（`-race`）。

## 3. 文案与空态

- [ ] `TrendEmptyState` 按 `reason` 选文案；集群 / 作业页签、预览抽屉透传 `reason`。
- [ ] zh / en 改写四处「接入 Prometheus」文案，新增 `prometheus_unavailable` 文案。
- [ ] 前端单测更新；`pnpm test`、`pnpm exec tsc --noEmit`、改动文件 eslint。

## 4. 验证

- [ ] 跨子任务验收在内嵌环境逐条重跑：时间窗 15m–7d 与自定义、集群范围、非管理员、`observability.enabled=false`、内嵌存储未就绪（`storage.dir` 指到不可写目录）、390px / 英文。
- [ ] 抽样一致性（须在内嵌 P3 合并前）：吞吐 / 堆 / Top 8 内嵌 vs 外部 9090，数值写进本文件实施记录。
- [ ] 两个浏览器会话同时开首页，后端日志确认 15 秒内同参数只算一轮。
- [ ] `grep -ri grafana internal/ frontend/app frontend/components frontend/lib` 无命中。

## 5. 收尾

- [ ] spec：`ui-conventions.md` §16、`job-status-reconciler.md` 首页大盘一节（见 design §4）。
- [ ] 父任务 PRD 跨子任务验收勾选；中文提交、开 PR、链接 T3 线程。

## 回退

- A：代理 / 配置删除，revert 即恢复（届时 Grafana 进程已由内嵌任务移除，恢复代理也无意义，只作代码回退）。
- B：缓存为包装层，单独 revert 不影响接口契约。
