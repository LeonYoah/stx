# 实施清单（交接）

> 状态：只有规划（prd / design / implement），**未写代码**。按顺序推进，每步可独立验证与回滚。
> 开工前：`trellis-before-dev` 读 backend / frontend spec，重点 `embedded-tsdb-self-obs.md`、`embedded-tsdb-capacity.md`、
> `cli-guidelines.md`、`sync-alerts.md`、`ui-conventions.md`。

## 1. Server 采集器（后端）
- [ ] 新包 `internal/pkg/selfmetrics`：HTTP / gRPC 计数器与直方图、`agents_connected` GaugeFunc（注入回调）、`build_info`。
- [ ] gin 中间件：`route=c.FullPath()`，空则 `unmatched`；`code` 按类；SSE / 长连接只计数不进直方图。
- [ ] gRPC unary + stream 拦截器，挂到 `internal/grpc/server.go` `buildServerOptions()`。
- [ ] `embedts.RegisterSelfCollectors(...)`；在 `router.go` 里 `embedts.Start` 成功后注册
  （selfmetrics 采集器 + `collectors.NewDBStatsCollector(sqlDB, "stx")`）。
- [ ] 单测：路由模板与 `unmatched`、状态码分类、拦截器计数；`SelfScrapeOnce` 后能在 tsdb 查到 `stx_server_*`。
- 验证：`go test ./internal/pkg/selfmetrics/... ./internal/pkg/embedts/...`

## 2. Agent 自身指标
- [ ] `agent/internal/collector/host_metrics.go` 加 `collectAgentSelf`（见 design §3 指标清单）。
- [ ] 指令分发处累加 `stx_agent_commands_total{type,result}`；心跳失败处累加 `stx_agent_heartbeat_failures_total`。
- [ ] Server `internal/apps/host/metrics.go` 白名单放开 `stx_agent_` 前缀 + 单测（旧前缀仍过、非法前缀仍拒、保留标签仍不可覆盖）。
- 验证：`(cd agent && go test ./...)`、`go test ./internal/apps/host/...`

## 3. 快照接口 + CLI
- [ ] monitoring service `GetPlatformSnapshot(ctx)` + handler `GET /api/v1/monitoring/platform`（路由注册在 `router.go` monitoring 组）。
- [ ] Agent 列表以 hosts 表为主合并指标；不可用时返回 `available=false` + `reason`。
- [ ] CLI `stx monitoring platform`（`internal/cmd/monitor.go`），表格与 `--json`；补 `monitor_test.go`。
- 验证：`go test ./internal/apps/monitoring/... ./internal/cmd/...`

## 4. 内置告警
- [ ] 5 条模板（design §5），中文名 / 描述加进 `metric_evaluator.go` 的 switch；英文在 `policy_center_service.go`。
- [ ] 表达式生成单测（含版本号注入、阈值替换、转义）。

## 5. 前端
- [ ] `lib/services/monitoring` 加 `getPlatformSnapshot` + 类型（数组字段 `?? []` 兜底）。
- [ ] 监控中心「平台」页签：指标卡 + 趋势 + Agent 表（design §6）；i18n zh/en。
- [ ] 单测：不可用空态、旧 Agent 指标为空显示「—」、版本不一致高亮。
- [ ] E2E 评估（`.trellis/spec/frontend/e2e-testing.md`）。
- 验证：`cd frontend && pnpm exec tsc --noEmit && pnpm test`（8G 机器上 tsc 可能被 OOM，换大内存机器或 CI 跑）。

## 6. 收尾
- [ ] spec：`backend/` 新增「平台自监控」文档（指标清单、基数规则、注册时序、白名单前缀）；`backend/index.md` 登记；
  `frontend/ui-conventions.md` 写「平台」页签约定。
- [ ] 文档说明外部探活 `/api/v1/health` 的必要性（进程内自监控的盲区）。
- [ ] 全量：`go test ./...`、`(cd agent && go test ./...)`、`pnpm test`、`tsc --noEmit`。
