# E3 设计：服务器主机指标采集（链路 B：Agent 采集 → gRPC 上报 → 控制面直写内嵌 tsdb）

## 链路

```
agent (gopsutil 采集, 与心跳同周期默认10s)
  → ReportHostMetrics RPC（新 proto 消息，捎带式：独立 RPC 但随心跳循环调用）
  → 控制面 metrics_ingest：展平为 stx_host_* 样本，时间戳=控制面接收时刻
  → embedts.Appender 直写（job="stx_host", host_id, host_name, instance=host:port）
  → 主机离线：心跳超时 reconcile 时对 last-seen 序列写 stale marker
```

选独立 RPC 而非心跳捎带：心跳消息结构已稳定且字段是三个百分比，硬塞指标会污染兼容面；独立 RPC 老 agent 不调用即不上报（控制面容忍缺省），新 agent 调旧控制面返回 Unimplemented 可忽略——双向兼容。

## proto（需 `make proto` 重新生成两份 pb）

`agent.proto` 增 `rpc ReportHostMetrics(ReportHostMetricsRequest) returns (ReportHostMetricsResponse)`：

```
message HostMetricSample { string name=1; map<string,string> labels=2; double value=3; }
message ReportHostMetricsRequest { string agent_id=1; uint32 host_id=2; repeated HostMetricSample samples=3; }
message ReportHostMetricsResponse {}
```

样本为平面（name+labels+value），控制面不做二次命名转换，直接追加。

## Agent 采集（agent/ module）

`agent/internal/collector/` 新增 `host_metrics.go`，用 **gopsutil v4** 采集（PRD 锁定项；
跨 linux/darwin/windows，优于手写 /proc）：
- cpu: `cpu.Times(false)` per mode → `stx_host_cpu_seconds_total{mode}`（累计值，rate 交给 PromQL）；`cpu.Counts` → `stx_host_cpu_cores`；load1/5/15 → `stx_host_load{1,5,15}`
- mem: `mem.VirtualMemory` → total/available/used/cached/buffers；`mem.SwapMemory` → swap total/used
- fs: `disk.Partitions(false)` + `disk.Usage` → per-mount `stx_host_fs_{size,avail,used_ratio,ino_used_ratio}{mountpoint}`；伪 fs 过滤 + Top8 合并 other
- diskio: `disk.IOCounters` → per-device `stx_host_disk_{read,write}_bytes_total`、`stx_host_disk_{reads,writes}_total`、`stx_host_disk_io_time_seconds_total`；Top8
- net: `net.IOCounters(false)` → per-iface `stx_host_net_{receive,transmit}_bytes_total`、packets/errors/dropped；lo/veth/docker*/br-* 过滤 + Top8
- misc: `net.Connections("tcp")` established → `stx_host_tcp_established`；`host.BootTime` → `stx_host_boot_time_seconds`；`/proc/sys/fs/file-nr`（仅 Linux）→ `stx_host_fds_allocated`

基数硬上限：fs/disk/net 各 Top8 按使用率/吞吐，其余合并 `other`；单主机理论上限 ~240 序列。

## 控制面写入

`internal/apps/host/` 新增 `metrics.go`：
- `ReportHostMetrics` handler：校验 agent 注册态 → `embedts.Appender` 追加（时间戳=now，不用 agent 时钟）
- labels：`job="stx_host"`, `host_id`, `host_name`, `instance=<host:agent端口语义>`；样本自带维度标签（mode/mountpoint/device/iface/state）
- 主机离线（心跳 last_check 超 3×interval）→ reconcile 写 stale marker（复用 embedts P2 的 stale 函数）

## 前端主机视图

集群页签内加「主机」分组段（不进集群卡片区）：`GET /dashboard/home/hosts?cluster_id` 返回主机列表（bare_metal+agent=有指标；其它标注不支持）+ 每主机小图组（CPU mode stacked、mem used%、fs Top、net rx/tx、load、disk io）。
- `HomeService.getHomeHosts()`；每主机 4 面板网格，实例标签 = host_name。
- 非 bare_metal / 无 agent → 「不支持」徽标；source=none → 统一空态；心跳当前值照显。

## CLI

`stx dashboard home hosts`（operation `dashboard.home.hosts`）。

## 边界与风险

- agent 是独立 go module：gopsutil 依赖加在 agent/go.mod（用 `v3.24.x` 老版本线避免新发布风险）。
- proto 变更跑 `make proto`（scripts/proto.sh 同时生成两份）。
- stale marker 复用 `embedts` 导出函数（P2 已实现，查函数名后接）。
- 不写 STX 业务库做时序；不引入自定义 PromQL 入口。
