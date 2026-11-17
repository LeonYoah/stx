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

## 2025-11-17 第二轮：#152 追加评审（代码 8 条 + 图表重做）

### 代码修复（LeonYoah + Codex 评审）

- `volume.go` today/month 求值时刻改 `alignPresetNow(now, nil)`，恢复 15s 缓存命中（原来毫秒级 now 永远 miss，31 天全量扫描是最重查询）
- `host_metrics.go` statfs 卡死治理：`fsStuck` sync.Map 记账，卡死的挂载点后续周期直接跳过不再开新 goroutine；goroutine 返回即清标记；fs 采集挪到最后避免串行 statfs 吃光 8s 预算；ctx 过期 `break` 保留已采样本
- `cluster_detail.go` offheap 改 `nonheap_used + buffer_pool_used`（原 `RSS−committed` 大 -Xms 未 PreTouch 时可为负）；store_heap 改按节点汇总
- CLI `--end` 说明改 400 天；虚拟网卡前缀补 cilium/vxlan/tap/tun/sit/gre/erspan/nodelocaldns/dummy 等（wg 放行）
- 心跳/主机指标两条循环的间隔改 `heartbeatIntervalNs atomic.Int64` 中转：applyRemoteConfig 原子写、循环每 tick 重读并 `ticker.Reset`，同时消掉原字段直读的数据竞争
- 主机卡 `metricsOff` 文案移除（顶部横幅已说明原因）；集群名回退 `#<id>`；桶标签分隔符统一 `MM-DD`
- `HomeHostEntry.BootTimeMs` + `stx_host_boot_time_seconds` instant 查询 → 卡头「已运行 N 天」；内存面板改 used(total−available)/cached/available

### 图表重做（一个指标一种图 + 语义色表）

- `dashboard-format.ts`：共享 `SERIES_PALETTE` + `DIMENSION_COLORS` 语义表 + `DIMENSION_RULES`（JVM 池/类空间正则）+ `dimensionColor` + `colorForLabels`（语义命中调色、同语义多实例 65%/45%/30% 渐淡、未知名按名字序稳定取色）；JobsTab `VOLUME_COLORS` 并入
- `metrics-trend-data.ts`：`NamedSeries.dashed` 透传
- HostsTab 10→8 格：fs_used+fs_inodes 合成按容量排序的 `Progress` 列表（容量/inode 两条，≥85% warning、≥95% danger，占两列）；tcp+fds 合成一格 `MetricCard`+`Sparkline`；负载加核数虚线、磁盘使用率加 100% 虚线
- ClusterDetailGroups：deadlocked/reject → 每节点 MetricCard（>0 经包层 `--foreground: var(--danger)` 染红，reject 带 Sparkline）；partitions → Donut（各节点 active 分布）；store_owned+backup → 横向分组 BarChart；fds → Progress（open/max ≥80% warning）；classes 只画 loaded（速率同线斜率）；hz_queue 全 0 类型不画；heap committed / threads peak / gc_pause 10% 虚线；proc 组改名「进程与类加载」
- `vitest.setup.ts` 补 IntersectionObserver/ResizeObserver 桩；`Progress` 加 `indicatorClassName`
- `ui-conventions.md` §16 加主机页签/集群细节/图表选型原则 + 3 条自检项
- 合并槽位防护：伙伴面板缺失时 fs_inodes/store_backup/class_load_rate/fds 各自照常渲染

### 验证
- `go test ./internal/apps/dashboard ./internal/grpc ./internal/pkg/promquery ./internal/operation` 全过；agent `go test ./...` 全过
- `pnpm test` 630 全过；`tsc --noEmit` 干净；swagger 已重生成（boot_time_ms）
- 注：`internal/apps/installer` 两个用例本地环境失败（chunks 计数断言），与本 diff 无关，CI 绿

## 2025-11-17 第三轮：embedts 测试 flake 根因修复

- CI 失败 `TestGetJobSparklinesDropsInvisibleIDs` panic：`chunks_head: no such file`
- 根因：`t.Cleanup` 后进先出——helper 先注册 `embedts.Close()`、后在 `Start(Dir: t.TempDir())` 里触发 TempDir 的删除清理；测试结束时**先删目录再关库**，`Head.Close()` 的 `mmapHeadChunks` 往已删 `chunks_head` 写盘 panic
- 触发条件：序列有 ≥2 head chunk 才走 `WriteChunk`——样本 `now-75s..now` 恰跨 2h chunk 边界（偶数整点）时必现，约 1%/run 的 flake
- 修复：`withEmbeddedObservability`（sync/job_list_test）与 `withPrometheus`（dashboard/cluster_test）把 `dir := t.TempDir()` 提到 `t.Cleanup` 注册之前；审查其余 4 个 embedts 测试点顺序本就正确
