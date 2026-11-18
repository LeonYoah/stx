# 平台自监控：STX server 与 Agent 进程指标

## 背景

STX 监控了 SeaTunnel 集群、作业和主机，但没有监控**自己**：

- 已有：内嵌 tsdb 自观测（#154，`job=stx_embedded`，含 STX 进程的 Go runtime / process 采集器与 `prometheus_tsdb_*`）；
  Agent 心跳里的主机资源与 SeaTunnel 进程状态；`stx_host_*` 主机细粒度指标（`ReportHostMetrics`）；
  `agent_offline` 告警；按需 pprof；OTel 链路追踪。
- 缺：STX server 的接口量 / 延迟 / 5xx、数据库连接池、gRPC、在线 Agent 数；Agent 进程自身的 RSS / CPU /
  goroutine / 版本 / 指令失败；面向运维的「平台自监控」视图与内置告警。

## 需求

1. **Server 指标**写入内嵌 tsdb：HTTP（按路由模板）、数据库连接池、gRPC、在线 Agent 数。不对外开放 `/metrics`
   （与 #146 删除无鉴权指标端点的决策一致）。
2. **Agent 指标**：Agent 进程自身指标随现有 `ReportHostMetrics` 上报写入 tsdb，**不改 proto**。
3. **视图**：监控中心新增「平台」页签：STX server 概况 + Agent 列表。
4. **接口与 CLI**：新增平台自监控快照接口，CLI 同步（AGENTS.md CLI 约定）。
5. **内置告警**（策略中心 `platform_health` 分类）：STX 内存过高、接口 5xx 比例、tsdb compaction 失败、Agent 内存过高、
   Agent 版本与 server 不一致。
6. 文档说明固有局限：进程内自监控发现不了 STX 自己挂掉，需外部探活 `/api/v1/health`。

## 非目标

- 不开放 Prometheus `/metrics` 给外部抓取（以后要接企业监控另开 issue，走带鉴权 federate / remote_write）。
- 不做分布式追踪改造（OTel 链路已有）。
- 不做多 STX 实例 HA 视角。

## 验收标准

- AC1：PromQL 能查到 `stx_server_http_requests_total`、`stx_server_http_request_duration_seconds_bucket`、
  `go_sql_*`（db 连接池）、`stx_server_grpc_requests_total`、`stx_server_agents_connected`；HTTP `route` 标签是
  路由模板（如 `/api/v1/sync/jobs/:id`），不是原始路径。
- AC2：PromQL 能查到 `stx_agent_process_resident_memory_bytes{host_id=...}` 等 Agent 指标；旧版 Agent 不报这些指标时
  server 无报错。
- AC3：监控中心「平台」页签展示 server 概况与 Agent 列表；监控不可用时走既有空态降级，不塌陷。
- AC4：`GET /api/v1/monitoring/platform` 与 `stx monitoring platform` 返回同一份快照。
- AC5：5 条内置告警在策略中心可见可启用，规则表达式有单测。
- AC6：基数受控：route 用模板、状态码按类（2xx/4xx/5xx）、未匹配路由归为 `unmatched`；单测锁住。
- AC7：`go test ./...`、`(cd agent && go test ./...)`、前端 tsc / test 通过；spec 更新。
