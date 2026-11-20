# 设计：平台自监控

## 1. 数据流

```
STX server 进程
  gin 中间件 / gRPC 拦截器 / DBStats / AgentManager ──► selfmetrics 采集器
                                                        │ 注册进 embedts 的 registry
  embedts.SelfScrapeOnce（每个抓取周期，已存在）◄────────┘  Gather → Flatten → Append（job=stx_embedded）

Agent 进程
  collector.CollectHostMetrics()（已存在，stx_host_*） + 新增 collectAgentSelf()（stx_agent_*）
      └─► ReportHostMetrics RPC（已存在，不改 proto）
            └─► host.WriteHostMetrics（放开 stx_agent_ 前缀）──► 内嵌 tsdb（job=stx_host，带 host_id/host_name）

监控中心「平台」页签 / CLI ──► GET /api/v1/monitoring/platform ──► promquery 即时查询拼快照
```

## 2. Server 指标

### 2.1 挂载方式

- 新包 `internal/pkg/selfmetrics`：定义采集器（`prometheus.Collector`），**不自己建 registry、不自己抓取**。
- `embedts` 增加 `RegisterSelfCollectors(cs ...prometheus.Collector) error`：把采集器注册进 `instance.registry`，
  由已有的 `SelfScrapeOnce` 写回 tsdb（`job=stx_embedded, instance=stx`），与 #154 的 Go runtime / tsdb 指标同源。
  - 坑：registry 在 `embedts.Start` 里建。router 里 HTTP 中间件比 embedts 启动早挂上，所以采集器对象要先创建、
    中间件先计数，`embedts.Start` 成功后再调用 `RegisterSelfCollectors` 注册（计数器不依赖 registry 存在）。
    `observability.enabled=false` 时只计数不注册，开销可忽略。
  - 依赖方向：`selfmetrics` 只依赖 client_golang；`embedts` 不反向依赖 apps（见 embedts 包注释）。
- 不新增依赖：gRPC 拦截器手写（一个 unary + 一个 stream，计数 + 直方图，几十行），不引 go-grpc-middleware。

### 2.2 指标清单

| 指标 | 类型 | 标签 | 来源 |
|---|---|---|---|
| `stx_server_http_requests_total` | counter | `method`, `route`, `code`（`2xx/3xx/4xx/5xx`） | gin 中间件 |
| `stx_server_http_request_duration_seconds` | histogram | `method`, `route` | gin 中间件 |
| `stx_server_grpc_requests_total` | counter | `method`（全名）, `code`（gRPC code） | gRPC 拦截器 |
| `stx_server_grpc_request_duration_seconds` | histogram | `method` | gRPC 拦截器 |
| `stx_server_agents_connected` | gauge | 无 | AgentManager 在线数（GaugeFunc） |
| `go_sql_*`（`collectors.NewDBStatsCollector(sqlDB, "stx")`） | 多个 | `db_name` | `db.GetDB(ctx).DB()` |
| `stx_server_build_info` | gauge=1 | `version` | 版本常量 |

- **基数**：`route = c.FullPath()`（路由模板）；为空（404 / 未匹配）记 `unmatched`；`code` 按类。
  直方图桶用少量固定桶（如 `.005,.025,.1,.25,.5,1,2.5,5,10`）。STX 路由数百条 × 4 方法以内，可控。
- 排除：`/api/v1/health`、静态资源、SSE / 长连接（日志流、`--wait` 轮询类）不计延迟直方图（会把 p99 拉成分钟级），
  只计数；具体路由清单实现时 grep `c.Stream` / `text/event-stream`。
- 挂载点：`internal/router/router.go:158` 一带 `r.Use(...)`；gRPC 拦截器在 `internal/grpc/server.go` 的
  `buildServerOptions()` 里加 `grpc.ChainUnaryInterceptor` / `grpc.ChainStreamInterceptor`。

## 3. Agent 指标

- 复用 `ReportHostMetrics`（`internal/proto/agent/agent.proto:37`）。**不改 proto**，不用重新生成 pb。
- Agent 侧 `agent/internal/collector/host_metrics.go` 增加 `collectAgentSelf(put)`：
  - `stx_agent_process_resident_memory_bytes`、`stx_agent_process_cpu_seconds_total`（gopsutil `process.NewProcess(os.Getpid())`，agent 已依赖 gopsutil v4）
  - `stx_agent_go_goroutines`、`stx_agent_go_heap_inuse_bytes`（`runtime`）
  - `stx_agent_uptime_seconds`
  - `stx_agent_build_info{version}` = 1
  - `stx_agent_commands_total{type, result}`（指令执行结果计数；在指令分发处累加，`type` 用 CommandType 枚举名）
  - `stx_agent_heartbeat_failures_total`
- Server 侧 `internal/apps/host/metrics.go`：`hostMetricNameRe` 放开 `stx_agent_` 前缀（身份标签注入逻辑不变，
  仍是 `job=stx_host` + `host_id/host_name/instance`）；`hostMetricsSampleCap=512` 足够。
- 兼容：旧 Agent 不报 `stx_agent_*`，页面该列显示「—」并提示升级 Agent；新 Agent 连旧 server 时这些样本会被旧白名单
  丢弃（只跳过不报错，见 `WriteHostMetrics` 注释），无害。

## 4. 快照接口与 CLI

- `GET /api/v1/monitoring/platform`（monitoring handler），返回：
  ```json
  {
    "available": true,
    "server": {"version":"…","uptime_seconds":0,"rss_bytes":0,"heap_inuse_bytes":0,"goroutines":0,
               "http_rps":0,"http_5xx_ratio":0,"http_p95_seconds":0,"db_open":0,"db_in_use":0,"db_wait_count_rate":0,
               "agents_connected":0,"tsdb":{"head_series":0,"blocks_bytes":0,"compactions_failed":0}},
    "agents": [{"host_id":1,"host_name":"…","version":"…","rss_bytes":0,"cpu_ratio":0,"goroutines":0,
                "uptime_seconds":0,"command_failures_1h":0,"last_report_at":"…","online":true}]
  }
  ```
  - 用 `promquery.Instant` 拼；`available=false` 时 `reason` 沿用 `observability_disabled / prometheus_unavailable`。
  - tsdb 段直接用 `embedts.GetStorageStatus()`（已有）。
  - Agent 列表以 hosts 表为主（离线 / 旧版也要列出来），指标按 `host_id` 合并；版本取注册时上报的 `agent_version`。
- CLI：`stx monitoring platform`（`internal/cmd/monitor.go`），表格 + `--json`，遵循 `.trellis/spec/backend/cli-guidelines.md`。

## 5. 内置告警

在策略中心模板（`internal/apps/monitoring/policy_center_service.go` 的 platform_health 列表，及
`metric_evaluator.go` 的中文名 / 描述 switch）新增：

| key | 表达式（示意） | 默认阈值 / for |
|---|---|---|
| `stx_server_memory_high` | `process_resident_memory_bytes{job="stx_embedded"} > <阈值>` | 可配，默认 2GiB，5m |
| `stx_server_http_5xx_ratio_high` | `sum(rate(stx_server_http_requests_total{code="5xx"}[5m])) / sum(rate(stx_server_http_requests_total[5m])) > 0.05` | 5%，10m |
| `stx_tsdb_compaction_failed` | `increase(prometheus_tsdb_compactions_failed_total{job="stx_embedded"}[1h]) > 0` | 0，0m |
| `stx_agent_memory_high` | `stx_agent_process_resident_memory_bytes > <阈值>` | 默认 512MiB，10m |
| `stx_agent_version_mismatch` | `stx_agent_build_info{version!="<server 版本>"} == 1` | 表达式由 server 生成时填入当前版本；30m |

- 实现时先读 `.trellis/spec/backend/sync-alerts.md` 与已有内置规则（如 `agent_offline`、`cpu_usage_high`）的生成路径照抄，
  规则走 #144 起的 `rules.Manager` 内嵌评估。

## 6. 前端

- `frontend/components/common/monitoring/MonitoringCenterWorkspace.tsx` 的 `ModuleNavTabs` 新增「平台」页签。
- 上段：STX server 一行指标卡（Arc `MetricCard`：内存、接口 QPS、5xx 比例、p95、DB 连接、在线 Agent、tsdb 序列）
  + 两张小趋势（Arc `LineChart`：RSS/heap、QPS/5xx），时间窗复用首页的分段控件。
- 下段：Agent 表（`StudioDataTable`）：主机、版本（与 server 不一致标黄）、RSS、CPU、goroutine、运行时长、1h 指令失败、最近上报。
- 遵循 `ui-conventions.md`：零塌陷加载、监控不可用空态、长说明进 HelpHint、i18n 走 next-intl。

## 7. 取舍与风险

- **为什么不用心跳带 Prometheus 文本**：`ReportHostMetrics` 已是结构化样本通道，带白名单、上限和 stale 处理，复用即可，省掉 proto 变更与 Agent/Server 版本协同。
- **为什么不开 `/metrics`**：与 #146 的安全决策一致；内嵌 tsdb 已能查询。
- **进程内自监控的盲区**：STX 挂了就没有指标也没有告警。文档写清楚用外部探活 `/api/v1/health`（systemd / LB / 云监控）。
- **开销**：中间件每请求两次原子操作 + 一次直方图 observe，可忽略；新增序列约 `路由数 × 方法 × 4 + 桶数`，千级，远低于 #154 的容量基准。

## 8. 回滚

- 各部分独立：去掉中间件 / 拦截器注册、Agent 不再报 `stx_agent_*` 即回到现状；无 DB 迁移、无 proto 变更。
