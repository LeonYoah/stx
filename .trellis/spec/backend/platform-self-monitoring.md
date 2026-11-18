# 平台自监控（STX server 与 Agent 进程指标）

> 适用于 `internal/pkg/selfmetrics`、`internal/apps/monitoring/platform_snapshot.go`、`agent/internal/collector` 的自身指标，以及 `GET /api/v1/monitoring/platform` 与 `stx monitoring platform`。

## 场景：让 STX 观测自己的控制面与 Agent 进程

### 1. 范围 / 触发条件

- Server 侧指标由 `selfmetrics.Collector` 在进程内采集，注册进 `embedts` 自抓取 registry（`job="stx_embedded"`，`instance="stx"`）。
- Agent 进程指标随 `ReportHostMetrics` 上报，走 `stx_host` 注入标签（`host_id` / `host_name` / `instance`），不改 protobuf。
- 平台快照接口 `GET /api/v1/monitoring/platform` 与 CLI `stx monitoring platform` 返回同一份 `PlatformSnapshot`。

### 2. 签名与指标名

```go
selfmetrics.New() *Collector
Collector.GinMiddleware() gin.HandlerFunc
Collector.UnaryServerInterceptor() grpc.UnaryServerInterceptor
Collector.StreamServerInterceptor() grpc.StreamServerInterceptor
Collector.SetAgentCountFn(func() int)
embedts.RegisterSelfCollectors(reg ...prometheus.Collector) error
```

- HTTP：`stx_server_http_requests_total{method,route,code}`（`route` 用 Gin 路由模板防基数爆炸，`code` 为 `2xx/3xx/4xx/5xx`）；`stx_server_http_request_duration_seconds{method,route}`。SSE / 长轮询 / `/api/v1/health` 计数但不进直方图。
- gRPC：`stx_server_grpc_requests_total{method,code}`、`stx_server_grpc_request_duration_seconds{method}`，method 为完整 gRPC 方法名。
- 其他：`stx_server_agents_connected`（GaugeFunc）、`collectors.NewDBStatsCollector(sqlDB, "stx")`、`stx_server_build_info{version}`。
- Agent（`stx_agent_` 前缀）：`process_resident_memory_bytes`、`process_cpu_seconds_total`、`goroutines`、`heap_inuse_bytes`、`uptime_seconds`、`build_info{version}`、`commands_total{type,result}`（result 只取 `success`/`failed`）、`heartbeat_failures_total`。

### 3. 契约

- 采集器必须在 `embedts.Start` 成功后才 `RegisterSelfCollectors`；注册失败只 WARN，不阻断业务 API。observability 关闭时进程内计数照常，只是不写 tsdb。
- `host` 指标白名单同时接受 `stx_host_` 与 `stx_agent_` 前缀；保留样本/标签上限、有限值校验与保留标签（`job`/`instance`/`host_id`/`host_name`/`__name__`）保护。
- 快照 `available=false` 时 `reason` 复用 `promquery.UnavailableReason()` 取值（`observability_disabled` / `prometheus_unavailable`），`server`/`trends` 省略，`agents` 恒为数组。
- 快照 `trends` 为固定四条序列（RSS / heap / HTTP QPS / HTTP 5xx 比例），最近 1h、60s 步长，PromQL 固定不外露任意查询；单条失败只置空该序列。
- Agent 快照的指标字段（`rss_bytes` / `cpu_ratio` / `goroutines` / `uptime_seconds` / `command_failures_1h` / `last_report_at`）在旧版 Agent 未上报时为 `null`，前端渲染 `—`；`version` 优先取 hosts 表 `agent_version`，回退 `stx_agent_build_info` 标签，版本不一致徽标由前端对比 `server.version` 得出。

### 4. 平台健康告警模板

- 五条内置模板归类 `platform_health`：`stx_server_memory_high`、`stx_server_http_5xx_ratio_high`、`stx_tsdb_compaction_failed`、`stx_agent_memory_high`、`stx_agent_version_mismatch`。
- 走 metrics 模板求值器，`metric_source="stx_platform"`；PromQL 只带 `job="stx_embedded"` 或 Agent 上报序列，严禁混入 `job="seatunnel_engine_http"`。
- 版本不一致模板用 `fmt.Sprintf` 注入 server 版本时必须先做 PromQL 字符串转义。

### 5. 边界：进程内自监控的盲区

- **进程内自监控无法发现 STX Server 自身宕机**：进程退出后所有自指标停止更新，页面与 CLI 只能看到“无新数据”。
- 生产环境必须在外部（负载均衡 / 巡检脚本 / 外部 Prometheus）周期性探测 `GET /api/v1/health`；该说明同时写在前端降级卡与文档中。

### 6. 校验与错误

| 情况 | 行为 |
| --- | --- |
| observability 关闭 | 快照 `available=false, reason=observability_disabled`；自指标不注册不写库 |
| tsdb 未就绪 | `reason=prometheus_unavailable`；单条 instant/range 失败只置空对应字段 |
| Agent 从未上报指标 | 仍出现在列表（来自 host 注册表），指标字段为空由前端 `—` 兜底 |

### 7. 必须有的测试

- `selfmetrics`：中间件状态分类与路由模板标签、gRPC 拦截器、build_info、agents_connected GaugeFunc。
- `embedts.RegisterSelfCollectors`：重复注册幂等（AlreadyRegisteredError 容忍）。
- `agent` collector：`collectAgentSelf` 指标名/样本数、心跳失败与指令结果计数。
- `host/metrics`：`stx_agent_` 前缀放行、保留标签不可覆写。
- `platform_snapshot`：snapshot 拼装、reason 降级、trends 序列。
- `platform_alert_rules`：五条模板注册、表达式可解析、版本注入转义、无引擎 matcher。
