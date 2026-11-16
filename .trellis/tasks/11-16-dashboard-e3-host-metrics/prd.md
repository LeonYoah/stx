# 增强：服务器主机指标采集

父任务：`11-14-home-monitoring-dashboard`。排期：P1–P4 验收完成之后。

## 已确认事实（2026-11-16 代码复核）

- **Agent 当前是纯出向进程**：`agent/` 是独立 Go module，只用 gRPC client 拨号到 STX（`Register` / `Heartbeat` / `CommandStream` /
  `LogStream` / `GetDiagnosticsLogCursors` / `DownloadPackage`），**没有任何监听端口**，依赖里没有 `client_golang`，也没有 gopsutil。
- 采集全是手写：`agent/internal/collector/collector.go` 读 `/proc/stat`、`/proc/meminfo`，调 `df -k /`、`top`、`vm_stat`、`wmic`；
  只产出 CPU / 内存 / 根分区三个百分比 + 可用内存 / 可用磁盘（`ResourceUsage`），心跳间隔默认 10s（`grpc.heartbeat_interval`）。
- 落库是覆盖写：`hosts.cpu_usage / memory_usage / disk_usage / last_check`，**无历史**。
- **控制面已有一套「自己采集、自己导出」的成熟范式**：`internal/apps/sync/job_prometheus.go` 用 `client_golang` 建私有 Registry，
  在 `/api/v1/monitoring/prometheus/jobs/metrics` 暴露 `stx_job_*`；`setCounterAbs` 已解决「引擎给绝对累计值 → 导出成 Counter」
  和回绕问题；终态作业 `DeletePartialMatch` 删序列。
- **HTTP SD 已能注册控制面自身**：`remote_integration.go:buildSTXSyncJobsSDTarget()` 用 `config.GetExternalURL()` 生成
  `job=stx_sync_jobs` 单目标组（含 `__metrics_path__` / `__scheme__`）；`prometheus.yml.tpl` 里 `seatunnel_engine_http` 与
  `stx_sync_jobs` 两个 job 都走 `__STX_SD_URL__`。
- 大盘后端范式：`internal/pkg/promquery`（`BaseURL()` → `source: prometheus|none` + `reason`、`Range` / `Instant`、
  `ResolveWindow` / `RangeSteps`），`internal/apps/dashboard/cluster.go` 并发查询 + `partial_errors`。
- 主机有三种类型（`bare_metal` / `docker` / `kubernetes`），**只有装了 Agent 的 `bare_metal` 能拿到整机指标**。
- 策略中心现有三类构建器：`platform_health`（可用）、`metrics_template`（可用）、`custom_promql`（能力卡片 + 入口已存在）。
- **一台主机可以承载多个集群的节点**：`nodes.host_id` 只有普通索引、无唯一约束（`internal/apps/cluster/model.go:227`），
  所以 `cluster` 不能做主机指标的 label（会重复序列），集群维度只能在后端按 host↔node 关联过滤。
- proto 改动要跑 `make proto`（`scripts/proto.sh` 会同时生成 `internal/proto/agent/` 与 agent module 两份）。
- 首页现有三个页签：集群 / 作业 / 作业列表（`frontend/components/common/dashboard/HomeDashboard.tsx`），
  头部已有时间窗 + 自定义区间 + 集群 scope 下拉，页签共用。

## Locked Decisions（用户确认）

1. **链路 = B：Agent 采集 → gRPC 上报 → 控制面统一导出**（替换原「Agent 内置导出器 + 每主机 HTTP SD 目标」决定）。
   Agent 保持零监听端口；控制面复用 `job_prometheus.go` 范式导出 `stx_host_*`，Prometheus 只多抓一个控制面目标。
   注：若内嵌可观测任务落地（单模式，进程内 tsdb），「导出给 Prometheus 抓」改为「控制面直接写入内嵌 tsdb」，链路更短。
2. **固定指标集与固定面板**：监控面板由产品内置定义，不开放用户自定义面板 / 指标选择；本任务不新增自定义 PromQL 入口。
   （`AlertPolicyBuilderKindCustomPromQL` 能力本身的存废归「内嵌可观测」任务，不在本任务处理。）
3. 本任务范围 = **采集 + 导出 + 首页主机视图**，不含主机类告警（是否补 `metrics_template` 内置模板留到后续任务）。

## Requirements

- 采集改用成熟库（gopsutil），替换现有手写 `/proc` + `df` 实现；心跳那三个当前值复用同一实现，心跳协议保持兼容。
- 固定指标集（产品内置，不可配置），语义对齐 node_exporter 习惯：
  - CPU：按 mode 的占用（user / system / iowait / steal / idle）、核数、load1 / 5 / 15。
  - 内存：total / available / used%、cached / buffers；Swap total / used%。
  - 文件系统：各挂载点 size / avail / used% 与 inode used%。
  - 磁盘 IO：各设备读写字节、IOPS、io 忙碌时间。
  - 网络：各网卡收发字节 / 包 / 错误 / 丢包。
  - 其他：TCP established 连接数、启动时间 / 运行时长、进程打开 FD。
- **基数必须有硬上限**（固定策略，不做配置项）：过滤伪文件系统（tmpfs / overlay / devtmpfs 等）与虚拟网卡（lo / veth / docker*），
  挂载点与块设备各取 Top N（按使用率 / 吞吐），超出合并为 `other`。
- 首页新增主机视图（页签或集群页签分组），按主机展示上述指标；非 `bare_metal` 主机与未装 Agent 的主机明确标注「不支持」而不是空图。
- 无 Prometheus 时该区块走统一空态 + 开启指引（与 P1–P4 一致），心跳当前值仍可见。
- 接口 + CLI 同步；proto 若变更需重新生成 `agent.pb.go` / `agent_grpc.pb.go`。

## Acceptance Criteria

- [ ] Linux 上各指标与 `vmstat` / `iostat` / `df -i` / `ip -s link` 抽样一致。
- [ ] Prometheus 能查到全部在线 `bare_metal` 主机的 `stx_host_*`（或等效）序列，标签可按集群 / 主机过滤。
- [ ] 单主机序列数有上限，挂载点 / 网卡 / 块设备很多的机器不会撑爆基数（给出实测序列数）。
- [ ] `observability.enabled=false` 时主机视图不报错，显示空态与指引。
- [ ] 非 `bare_metal` / 未装 Agent 的主机在视图里是明确的「不支持 / 未采集」态。
- [ ] 后端查询并发执行、单条失败记 `partial_errors`；CLI 与接口同步。
- [ ] 没有引入任何用户可编辑的 PromQL / 自定义面板入口。

## Out of Scope

- 自定义 PromQL 策略存废、用户自定义面板 / 指标选择（归「内嵌可观测」任务）。
- 托管或分发 node_exporter。
- 主机指标写入 STX 数据库做时序（维持「不保存时序」的既有决定；时序走 Prometheus / 内嵌 tsdb）。
- Docker / K8s 主机类型的整机指标。
