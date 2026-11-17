# Dashboard E1/E2/E3 评审修复（PR #148 合后评审 16 条）

来源：PR #148 合并后 owner 追加的 16 条评审意见，逐条落地。

## 功能 bug

| # | 问题 | 修法 |
|---|---|---|
| 1 | CPU 面板是各 mode 速率（含 idle、单位 /s），非使用率 | `100 * sum by (host_id, mode) (rate(stx_host_cpu_seconds_total{mode!="idle"}[5m])) / on(host_id) group_left() max by (host_id) (stx_host_cpu_cores)`，unit=percent |
| 2 | 读写量亚日桶标签只截 HH:mm，跨天/24h 首尾残缺桶重名 | 前端：全部桶同日才截 HH:mm，跨天保留 `MM-DD HH:mm`；partial 仍标 `*` |
| 3 | today/month 按自定义窗口 end 计算，查历史窗口谎报 | totals 改按 `s.now()` 计算（window 仍按窗口） |
| 4 | 计数器 top-N + other 合并产生 rate 假尖峰 | agent：disk/net 改「按设备名排序取前 8」稳定集合，不再合并 other；fs（瞬时值）保留 topN+other |
| 5 | 全部集群下 members 序列无集群身份、成员事件说不清是哪个集群 | members 序列 Instance 填集群名；`members_change` 事件标签加 `集群名: old → new` |
| 6 | offheap 用 heap used | 改 `RSS − jvm_memory_bytes_committed{area="heap"}` |
| 7 | 字节速率单位缺 B、格式不统一 | 新增 `bytes_per_second` 单位；formatBytes + `/s`；volume 字节指标统一走 formatBytes（IEC） |
| 8 | month 粒度在 31 天窗口上限下不可达 | `ParseWindowWithSpan` 放开 volume 端点到 400 天（前端 UI 窗口不变，CLI/API 可达） |
| 9 | 可观测关闭时主机清单整个隐藏 | 横幅原因 + 清单照列，卡片指标位一句说明 |

## 可靠性 / 性能

| # | 问题 | 修法 |
|---|---|---|
| 10 | 采集挂在心跳循环、statfs 可永久阻塞、RPC 无超时 | 独立 goroutine+ticker；全部 gopsutil 走 WithContext（采集 8s 上限）；每个 statfs 单独 goroutine 2s 超时丢弃；nfs/cifs/ceph/fuse/sshfs 等网络/用户态 fs 不进枚举；ReportHostMetrics 5s ctx 超时 |
| 11 | `net.Connections("tcp")` 扫全进程 fd | 改 `net.ProtoCounters(["tcp"])` 读 /proc/net/snmp 的 CurrEstab（node_exporter 同口径） |
| 12 | 主机页签 N×13 查询且不走缓存 | 每面板一条 `host_id=~"…"` 查询（固定 ~13 条），结果按 host_id 标签拆回；全走 wrapRange 缓存 |
| 13 | 读写量上百条即时查询无并发上限 | 等宽满桶（刻钟/小时）合并每指标一条 range 查询（点落桶 EndMs）；残缺/日月桶仍即时查询；统一并发上限 4 |
| 14 | 未绑定主机的 agent 每次上报 NotFound 双端刷 warn | 控制面回 success 丢弃；agent 侧同时静默 NotFound |
| 15 | `wg` 接口被误滤 | 从 pseudoInterface 移除（WireGuard 组网的主要流量） |
| 16 | 与 #146（移除 BaseURL/外部栈）冲突 | #146 仍未合；本 PR 基于 main 现状。待 #146 合入后由其作者/后续 rebase 处理冲突 |

## 验证

- `go test ./internal/apps/dashboard/ ./internal/grpc/ ./internal/pkg/promquery/ ./internal/apps/host/ ./internal/operation/` 全绿
- `agent go test ./internal/collector/ ./cmd/` 全绿
- `pnpm vitest run` HostsTab/JobsTab 定向全过（含新增跨天标签 / metricsOff 用例）
- 实机：重编 API+agent 重启 → `Host metrics loop started with interval: 10s`；
  `/dashboard/home/hosts` 回 cpu=percent 7 系列（无 idle）、net/disk_io=bytes_per_second、
  `rx/tx wg0` 出现、无 other；`/volume?granularity=auto` 1h 窗 5 桶（2 残缺+3 满）正常，
  today_start 为显示时区当日零点。
