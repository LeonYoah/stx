# E3 主机指标 — 实现记录

## 状态：已实现并通过真实链路验证（随 E1+E2 修正同一 PR）

## 链路

`agent(gopsutil) → ReportHostMetrics(gRPC) → 控制面按 agent→host 绑定写内嵌 tsdb → /dashboard/home/hosts → 主机页签`

## 关键文件

| 层 | 文件 | 说明 |
|---|---|---|
| proto | `internal/proto/agent/agent.proto` + 双端生成物 | `ReportHostMetrics` RPC + `HostMetricSample` |
| agent | `agent/internal/collector/host_metrics.go` | gopsutil v4 采集：CPU 分模式/核数、负载、内存/swap、文件系统、盘 IO、网卡 IO、TCP、fd、开机时间；Top8 合并 `other` + 虚拟设备过滤；分类独立失败不互相阻断 |
| agent | `agent/internal/grpc/client.go` `agent/cmd/main.go` | `ReportHostMetrics` 客户端；心跳成功后同周期上报，Unimplemented 静默、其他错误仅 warn 不重试 |
| 控制面 | `internal/grpc/host_metrics.go` | 校验 agent 连接 → `conn.HostID` 归属 → `host.WriteHostMetrics` |
| 控制面 | `internal/apps/host/metrics.go` | 注入 job/host_id/host_name/instance 身份标签（防 agent 注入），维度白名单，SeriesTracker 差分补消失序列 stale |
| 控制面 | `internal/router/router.go` | `MarkHostOffline` 前调 `host.MarkHostMetricsStale` |
| 大盘 | `internal/apps/dashboard/hosts.go` | `/dashboard/home/hosts`：清单 + 每台 10 面板并发查询；docker/k8s 无 agent → `metrics_supported=false` 不出面板；`cluster_nodes.host_id` 过滤 |
| 前端 | `HostsTab.tsx` + `home.service.ts` + i18n | 「主机」页签逐台卡片 + 面板；字节面板用新增 `formatBytes`（IEC） |
| CLI | `internal/operation/registry.go` | `stx dashboard home hosts --cluster_id 1 --range 6h` |

## 兼容性

- 旧 agent 不调新 RPC：控制面无感知。
- 新 agent 对旧控制面：收到 `codes.Unimplemented`，静默跳过不影响心跳。
- 样本维度不允许覆盖身份标签（`__name__/job/instance/host_id/host_name`）。

## 真实验证（本机）

- 新版 agent（root，配置同线上）注册 → 每 10s 心跳后上报。
- `GET /api/v1/dashboard/home/hosts?range=15m`：host t1 `metrics_supported=true`，10 个面板全部有真实序列与点（cpu 8 序列、memory 5、load 3、net 4、disk_io 4、disk_util 2、fs_used 2、fs_inodes 2、tcp 1、fds 1），`partial_errors` 空。
- 主机页签浏览器实渲染：CPU 分模式折线、内存 GB 量纲、Agent 状态徽章、心跳时间。
- CLI `stx dashboard home hosts --help` 生成正常。
- 单元：`metrics_test.go` 真实 tsdb 往返（写入→查询→消失序列 stale→整机离线 stale）；`hosts_test.go` 面板组装/集群过滤/可观测关闭；`host_metrics_test.go`(agent) 真机采集冒烟。
