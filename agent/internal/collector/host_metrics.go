/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package collector - 主机细粒度指标采集（E3）。
// 全部样本带 stx_host_ 前缀，控制面经 gRPC 收齐后原样直写内嵌 tsdb（job="stx_host"）。
// 采集走 gopsutil（跨 linux/darwin/windows）；唯二兜底：/proc/sys/fs/file-nr 只在 Linux 存在，
// 读不到就静默跳过。
// 防心跳阻塞三件套：所有 gopsutil 调用走 *WithContext；statfs（disk.Usage）对每个挂载点
// 单独放进带超时的 goroutine（挂死的网络盘在内核态阻塞，ctx 中断不了，只能跳过）；
// 网络盘 / 用户态 fs 类型直接不进枚举。
// Host fine-grained metric collection (E3). Every sample carries the stx_host_ prefix;
// the control plane writes them verbatim into the embedded tsdb (job="stx_host").
// Collection goes through gopsutil (cross linux/darwin/windows); the only fallback is
// /proc/sys/fs/file-nr, which exists on Linux only and is skipped silently elsewhere.
// Heartbeat-safety measures: every gopsutil call goes through its *WithContext variant;
// each statfs (disk.Usage) runs in its own timeout-guarded goroutine (a hung network mount
// blocks in kernel space where ctx cannot reach, so the call is abandoned instead);
// network and userspace fstypes are excluded from enumeration entirely.
package collector

import (
	"context"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// hostMetricCap 限定单类对象（挂载点 / 磁盘 / 网卡）上报的条数上限，控制 tsdb 基数。
// 计数器类（盘 IO / 网卡 IO）按设备名排序取前 N —— 成员集合稳定，避免设备进出集合时
// 独立序列与聚合序列互换产生 rate() 假尖峰；瞬时值类（fs 容量）按使用率取前 N 其余合并 other。
// hostMetricCap bounds how many objects (mounts / disks / interfaces) are reported per class.
// Counter classes (disk IO / NIC IO) take the first N sorted by device name — a stable
// membership so devices cannot oscillate between a dedicated series and an aggregate one,
// which would fabricate rate() spikes; instantaneous fs capacity keeps top-N-by-usage plus
// an "other" rollup.
const hostMetricCap = 8

// statfsTimeout 是单个挂载点 statfs 的放行时限；超时直接跳过该挂载点。
// statfsTimeout is how long a single mount's statfs may take before it is skipped.
const statfsTimeout = 2 * time.Second

// pseudoFilesystems 是不需要容量视角或可能挂死采集的文件系统类型（gopsutil all=false 已滤一遍，
// 这里兜底）；nfs/cifs/ceph 等网络盘与 fuse/sshfs 用户态文件系统直接排除 —— statfs 阻塞属于
// 内核态 D 状态，超时都救不回来，从源头不进枚举。
// pseudoFilesystems are fstypes carrying no capacity signal or capable of hanging collection
// (gopsutil's all=false filters most; this is the safety net). Network filesystems
// (nfs/cifs/ceph/…) and userspace filesystems (fuse/sshfs/…) are excluded outright — a statfs
// hang lives in uninterruptible kernel sleep that no timeout can abort, so these mounts never
// enter enumeration.
var pseudoFilesystems = map[string]bool{
	"proc": true, "sysfs": true, "devpts": true, "devtmpfs": true,
	"cgroup": true, "cgroup2": true, "tmpfs": true, "overlay": true,
	"squashfs": true, "autofs": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "pstore": true, "bpf": true, "configfs": true,
	"fusectl": true, "hugetlbfs": true, "mqueue": true, "ramfs": true,
	"nsfs": true, "efivarfs": true, "binfmt_misc": true, "sunrpc": true,
	// 网络盘：挂死时 statfs 在内核态不可中断。/ Network filesystems hang statfs in the kernel.
	"nfs": true, "nfs4": true, "cifs": true, "smbfs": true, "smb3": true,
	"ceph": true, "glusterfs": true, "davfs": true, "lustre": true,
	// 用户态文件系统同理。/ Userspace filesystems hang the same way.
	"fuse": true, "sshfs": true, "fuseblk": true, "gvfs": true,
}

// pseudoInterface 是虚拟网卡前缀：它们不反映物理网络流量。WireGuard（wg*）刻意保留 —
// 用 WireGuard 组网的部署主要流量就走它。Cilium/VXLAN/TAP/TUN 等容器与隧道网卡必须
// 在列：按名字序取前 hostMetricCap 时它们排在 eth0 之前，会把真实网卡挤出集合。
// pseudoInterface are virtual interface prefixes that do not reflect physical traffic.
// WireGuard (wg*) is deliberately kept — on WireGuard-joined deployments it carries the
// real traffic. Cilium/VXLAN/TAP/TUN and friends must stay filtered: they sort before
// eth0 lexically and would push the real NICs out of the stable top-N set.
var pseudoInterface = []string{
	"lo", "veth", "docker", "br-", "virbr", "kube-", "cni", "flannel", "cali", "utun", "gif", "stf", "awdl", "llw",
	// 容器网络与隧道：Cilium 的 host/net/vxlan、各 CNI 的 tap/tun、IP 隧道。
	// Container networking and tunnels: cilium_* (host/net/vxlan), CNI tap/tun, IP tunnels.
	"cilium", "vxlan", "tap", "tun", "tunl", "ip6tnl", "sit", "gre", "gretap", "erspan", "ipip", "nodelocaldns", "dummy",
}

// blockedFstype 精确匹配 + fuse.*/nfs.* 前缀式排除。
// blockedFstype matches the map plus fuse.*/nfs.* style prefixes.
func blockedFstype(fstype string) bool {
	if pseudoFilesystems[fstype] {
		return true
	}
	for _, prefix := range []string{"fuse.", "nfs", "smbfs"} {
		if strings.HasPrefix(fstype, prefix) {
			return true
		}
	}
	return false
}

// CollectHostMetrics 采集一轮 stx_host_* 指标样本，顺带输出 stx_agent_* 进程自指标。
// 单个采集源失败不阻断其余指标（每步独立容错），返回 nil 表示整轮不可用；
// ctx 取消提前终止整轮，调用方须带超时。
// CollectHostMetrics gathers one round of stx_host_* samples plus the stx_agent_*
// process self-metrics. A single failing source does not abort the others (each step
// tolerates independently); nil means the whole round failed. A cancelled ctx aborts
// the round early — callers are expected to pass a deadline.
func (c *MetricsCollector) CollectHostMetrics(ctx context.Context) []*pb.HostMetricSample {
	var out []*pb.HostMetricSample
	put := func(name string, value float64, dims ...string) {
		labels := map[string]string{}
		for i := 0; i+1 < len(dims); i += 2 {
			labels[dims[i]] = dims[i+1]
		}
		out = append(out, &pb.HostMetricSample{Name: name, Labels: labels, Value: value})
	}

	c.collectCPUTimes(ctx, put)
	c.collectLoadAvg(ctx, put)
	c.collectMemory(ctx, put)
	c.collectDiskIO(ctx, put)
	c.collectNetIO(ctx, put)
	c.collectTCPEstablished(ctx, put)
	c.collectBootAndFDs(ctx, put)
	// 文件系统采集放最后：挂载点是串行 statfs（每个至多 statfsTimeout），遇到坏盘/
	// 卡死设备时耗掉的预算不挤占前面的采集项。
	// Filesystem collection runs last: mounts are stafs'd serially (each bounded by
	// statfsTimeout), so a bad disk cannot starve the collectors above of ctx budget.
	c.collectFilesystems(ctx, put)
	// 自指标随同一轮上报，纯 runtime 读取不占 ctx 预算，放在文件系统之后。
	// Self-metrics ride the same round; pure runtime reads don't contend for ctx
	// budget, so they run after the filesystem pass.
	c.collectAgentSelf(put)
	return out
}

// collectAgentSelf 输出 Agent 进程自身指标（stx_agent_ 前缀）：
// RSS / CPU / goroutine / heap / uptime / build_info，以及进程内累计的命令结果与
// 心跳失败计数。控制面收到后与主机指标走同一通道写入 tsdb，并注入 host_id。
// collectAgentSelf emits the Agent process self-metrics (stx_agent_ prefix):
// RSS / CPU / goroutines / heap / uptime / build_info plus the in-process command
// outcome and heartbeat-failure counters. The control plane writes them through the
// same channel as host metrics and stamps host_id onto each series.
func (c *MetricsCollector) collectAgentSelf(put hostPut) {
	if proc, err := process.NewProcess(int32(os.Getpid())); err == nil {
		if minfo, merr := proc.MemoryInfo(); merr == nil && minfo != nil {
			put("stx_agent_process_resident_memory_bytes", float64(minfo.RSS))
		}
		if times, terr := proc.Times(); terr == nil && times != nil {
			put("stx_agent_process_cpu_seconds_total", times.User+times.System)
		}
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	put("stx_agent_heap_inuse_bytes", float64(ms.HeapInuse))
	put("stx_agent_goroutines", float64(runtime.NumGoroutine()))
	put("stx_agent_uptime_seconds", time.Since(c.selfStartedAt).Seconds())
	put("stx_agent_build_info", 1, "version", c.selfVersion)
	put("stx_agent_heartbeat_failures_total", float64(c.heartbeatFailures.Load()))

	// 命令结果计数：type × result 固定维度，避免命令 ID 等高基数信息泄漏进标签。
	// Command outcome counters: fixed type × result dimensions — no high-cardinality
	// data such as command IDs leaks into labels.
	c.selfMu.Lock()
	for key, count := range c.commandCounts {
		put("stx_agent_commands_total", float64(*count), "type", key[0], "result", key[1])
	}
	c.selfMu.Unlock()
}

// hostPut 是指标落点函数签名：name + value + 偶数个维度键值。
// hostPut is the sample sink signature: name + value + an even number of dimension pairs.
type hostPut func(name string, value float64, dims ...string)

// collectCPUTimes 输出按 mode 拆分的累计 CPU 秒数（rate 交给 PromQL）与核数。
// collectCPUTimes emits cumulative CPU seconds per mode (rates are left to PromQL) plus
// the core count.
func (c *MetricsCollector) collectCPUTimes(ctx context.Context, put hostPut) {
	times, err := cpu.TimesWithContext(ctx, false)
	if err == nil && len(times) > 0 {
		t := times[0]
		modes := map[string]float64{
			"user": t.User, "nice": t.Nice, "system": t.System, "idle": t.Idle,
			"iowait": t.Iowait, "irq": t.Irq, "softirq": t.Softirq, "steal": t.Steal,
		}
		for mode, v := range modes {
			put("stx_host_cpu_seconds_total", v, "mode", mode)
		}
	}
	if cores, err := cpu.CountsWithContext(ctx, true); err == nil && cores > 0 {
		put("stx_host_cpu_cores", float64(cores))
	}
}

// collectLoadAvg 输出 1m/5m/15m 负载。
// collectLoadAvg emits the 1m/5m/15m load averages.
func (c *MetricsCollector) collectLoadAvg(ctx context.Context, put hostPut) {
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return
	}
	put("stx_host_load", avg.Load1, "kind", "1m")
	put("stx_host_load", avg.Load5, "kind", "5m")
	put("stx_host_load", avg.Load15, "kind", "15m")
}

// collectMemory 输出内存与交换区（gopsutil 已统一为 bytes）。
// collectMemory emits memory and swap figures (gopsutil already normalizes to bytes).
func (c *MetricsCollector) collectMemory(ctx context.Context, put hostPut) {
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		put("stx_host_memory_bytes", float64(vm.Total), "kind", "total")
		put("stx_host_memory_bytes", float64(vm.Free), "kind", "free")
		put("stx_host_memory_bytes", float64(vm.Available), "kind", "available")
		put("stx_host_memory_bytes", float64(vm.Buffers), "kind", "buffers")
		put("stx_host_memory_bytes", float64(vm.Cached), "kind", "cached")
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		put("stx_host_swap_bytes", float64(sw.Total), "kind", "total")
		put("stx_host_swap_bytes", float64(sw.Free), "kind", "free")
	}
}

// fsStat 是一个真实挂载点的容量快照。
// fsStat is one real mount's capacity snapshot.
type fsStat struct {
	mountpoint string
	sizeBytes  float64
	availBytes float64
	files      float64
	filesFree  float64
}

// usageWithTimeout 把单个挂载点的 statfs 放进带超时的 goroutine：
// 挂死的 statfs 走内核 D 状态，context 中断不了，唯一可行的做法是到点就丢。
// 超时后记入 fsStuck：该 goroutine 仍占着一个 OS 线程在内核里等，解除前不再对
// 这个挂载点发起新 statfs，防止每轮各泄一个撞上 Go 的线程上限；goroutine 一旦
// 返回（盘恢复或内核报错）自清标记，下一轮正常重试。
// usageWithTimeout runs one mount's statfs inside a timeout-guarded goroutine: a hung
// statfs blocks in uninterruptible kernel sleep which no context can cancel, so the only
// workable strategy is to abandon the call. On timeout the mount is recorded in fsStuck —
// its goroutine still owns an OS thread inside the kernel, and issuing further statfs for
// the same mount would leak one goroutine+thread per cycle until Go's thread limit kills
// the process. The mark clears itself once the goroutine returns (disk recovered or the
// kernel errored out) and the next round retries normally.
func (c *MetricsCollector) usageWithTimeout(mountpoint string) (*disk.UsageStat, bool) {
	type result struct {
		usage *disk.UsageStat
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		usage, err := disk.Usage(mountpoint)
		ch <- result{usage: usage, err: err}
		// 清标记必须是 goroutine 的最后一个动作：保证「标记还在」⇔「statfs 还没返回」，
		// 不会出现 goroutine 已结束却留下永不重试的假 stuck。
		// Clearing the mark is the goroutine's last act, so "still marked" reliably means
		// "statfs hasn't returned" — a finished goroutine can never leave a stale mark.
		c.fsStuck.Delete(mountpoint)
	}()
	select {
	case res := <-ch:
		return res.usage, res.err == nil
	case <-time.After(statfsTimeout):
		c.fsStuck.Store(mountpoint, struct{}{})
		// 超时边界上 goroutine 可能刚把结果写进缓冲通道：非阻塞再看一眼，采到就用。
		// At the timeout boundary the goroutine may have just delivered into the
		// buffered channel — peek once more and use a result that did arrive.
		select {
		case res := <-ch:
			c.fsStuck.Delete(mountpoint)
			return res.usage, res.err == nil
		default:
		}
		return nil, false
	}
}

// collectFilesystems 枚举真实分区（伪 fs 与网络/用户态 fs 双重过滤），按使用率取前 hostMetricCap，
// 其余合并为 "other"。
// collectFilesystems enumerates real partitions (pseudo and network/userspace fstypes
// double-filtered), keeps the top hostMetricCap by usage and merges the rest into "other".
func (c *MetricsCollector) collectFilesystems(ctx context.Context, put hostPut) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return
	}
	var stats []fsStat
	for _, part := range parts {
		if ctx.Err() != nil {
			// 超预算就停，但已经采到的挂载点照常输出（fs 采集排在最后，
			// 半截结果也有价值）。/ Out of budget — stop enumerating but keep the
			// mounts already collected (fs runs last; a partial result still counts).
			break
		}
		if blockedFstype(part.Fstype) {
			continue
		}
		if _, stuck := c.fsStuck.Load(part.Mountpoint); stuck {
			// 上一个 statfs 还卡在内核里，跳过不再泄 goroutine。
			// The previous statfs is still stuck in the kernel — skip it instead of
			// leaking another goroutine.
			continue
		}
		usage, ok := c.usageWithTimeout(part.Mountpoint)
		if !ok {
			continue
		}
		stats = append(stats, fsStat{
			mountpoint: part.Mountpoint,
			sizeBytes:  float64(usage.Total),
			availBytes: float64(usage.Free),
			files:      float64(usage.InodesTotal),
			filesFree:  float64(usage.InodesFree),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		ui := 1 - stats[i].availBytes/maxFloat(stats[i].sizeBytes, 1)
		uj := 1 - stats[j].availBytes/maxFloat(stats[j].sizeBytes, 1)
		return ui > uj
	})
	emit := func(s fsStat) {
		put("stx_host_fs_size_bytes", s.sizeBytes, "mountpoint", s.mountpoint)
		put("stx_host_fs_avail_bytes", s.availBytes, "mountpoint", s.mountpoint)
		put("stx_host_fs_files", s.files, "mountpoint", s.mountpoint)
		put("stx_host_fs_files_free", s.filesFree, "mountpoint", s.mountpoint)
	}
	var other fsStat
	other.mountpoint = "other"
	for i, s := range stats {
		if i >= hostMetricCap {
			other.sizeBytes += s.sizeBytes
			other.availBytes += s.availBytes
			other.files += s.files
			other.filesFree += s.filesFree
			continue
		}
		emit(s)
	}
	if len(stats) > hostMetricCap {
		emit(other)
	}
}

// diskStat 是一块整盘设备的 IO 计数。
// diskStat is the IO counters of one whole-disk device.
type diskStat struct {
	name         string
	reads        float64
	readBytes    float64
	writes       float64
	writeBytes   float64
	ioTimeMillis float64
}

// wholeDisk 判定整盘设备名（排除 loop/ram/dm/zram/光盘与各分区）；非 Linux 平台设备名
// 不匹配这些规则的默认保留。
// wholeDisk reports whether name is a whole-disk device (loops/ram/dm/zram/optical and
// per-partition rows excluded); non-Linux device names that match none of these rules are kept.
func wholeDisk(name string) bool {
	for _, prefix := range []string{"loop", "ram", "dm-", "zram", "sr", "fd"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	// nvme0n1p1 / mmcblk0p2 的分区段以 pN 结尾。/ nvme0n1p1 / mmcblk0p2 partitions end with pN.
	if i := strings.LastIndex(name, "p"); i > 0 {
		if _, err := strconv.Atoi(name[i+1:]); err == nil &&
			(strings.Contains(name[:i], "nvme") || strings.Contains(name[:i], "mmcblk")) {
			return false
		}
	}
	// sda1 / vda2 / disk0s1 这类分区以数字结尾但纯字母基座除外（如 disk0 是整盘）。
	// sda1 / vda2 / disk0s1 style partitions end with a digit; pure-letter bases like
	// disk0 are whole disks — handled by the trailing-digit check below on Linux names only.
	last := name[len(name)-1]
	if last < '0' || last > '9' {
		return true
	}
	// nvme0n1 / mmcblk0 / disk0 这类整盘名本身以数字结尾，按基座判定放行。
	// Whole-disk names like nvme0n1 / mmcblk0 / disk0 also end with a digit; let the
	// container-style bases through.
	if strings.Contains(name, "nvme") || strings.Contains(name, "mmcblk") || strings.HasPrefix(name, "disk") {
		return true
	}
	return false
}

// stableTopN 按名字排序取前 hostMetricCap 个，构成稳定的成员集合：
// 计数器类指标（rate 消费）若按吞吐排名+other 合并，设备进出集合会让 other 突增/回落
// 产生假尖峰；固定取名字序前 N 后成员只在硬件变更时变化，由控制面 stale marker 兜底。
// 注意：设备数超过上限时排在后面的会被静默丢弃（虚拟设备尽量在过滤阶段就排掉）。
// stableTopN keeps the first hostMetricCap names sorted lexically — a stable membership:
// counter metrics (consumed via rate) would fabricate spikes when devices oscillate
// between dedicated series and an "other" aggregate; with name order the set only changes
// when hardware does, covered by control-plane stale markers.
// Caveat: devices sorting beyond the cap are silently dropped — virtual devices should
// be filtered out before this point wherever possible.
func stableTopN(names []string) []string {
	sort.Strings(names)
	if len(names) > hostMetricCap {
		names = names[:hostMetricCap]
	}
	return names
}

// collectDiskIO 输出整盘设备的读写次数 / 字节 / IO 耗时；设备集合按名字序固定前 N。
// collectDiskIO emits whole-disk read/write counts, bytes and time-in-io; the device set is
// the name-ordered stable top N.
func (c *MetricsCollector) collectDiskIO(ctx context.Context, put hostPut) {
	counters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return
	}
	var names []string
	for name := range counters {
		if wholeDisk(name) {
			names = append(names, name)
		}
	}
	for _, name := range stableTopN(names) {
		io := counters[name]
		put("stx_host_disk_reads_total", float64(io.ReadCount), "device", name)
		put("stx_host_disk_read_bytes_total", float64(io.ReadBytes), "device", name)
		put("stx_host_disk_writes_total", float64(io.WriteCount), "device", name)
		put("stx_host_disk_write_bytes_total", float64(io.WriteBytes), "device", name)
		put("stx_host_disk_io_time_seconds_total", float64(io.IoTime)/1000, "device", name)
	}
}

// collectNetIO 输出真实网卡的收发计数；虚拟网卡跳过，接口集合按名字序固定前 N。
// collectNetIO emits per-interface counters for real NICs; virtual interfaces are skipped
// and the interface set is the name-ordered stable top N.
func (c *MetricsCollector) collectNetIO(ctx context.Context, put hostPut) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return
	}
	var names []string
	for _, io := range counters {
		skip := false
		for _, prefix := range pseudoInterface {
			if strings.HasPrefix(io.Name, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			names = append(names, io.Name)
		}
	}
	byName := map[string]net.IOCountersStat{}
	for _, io := range counters {
		byName[io.Name] = io
	}
	for _, name := range stableTopN(names) {
		io := byName[name]
		put("stx_host_net_receive_bytes_total", float64(io.BytesRecv), "iface", name)
		put("stx_host_net_receive_packets_total", float64(io.PacketsRecv), "iface", name)
		put("stx_host_net_receive_errors_total", float64(io.Errin), "iface", name)
		put("stx_host_net_receive_dropped_total", float64(io.Dropin), "iface", name)
		put("stx_host_net_transmit_bytes_total", float64(io.BytesSent), "iface", name)
		put("stx_host_net_transmit_packets_total", float64(io.PacketsSent), "iface", name)
		put("stx_host_net_transmit_errors_total", float64(io.Errout), "iface", name)
		put("stx_host_net_transmit_dropped_total", float64(io.Dropout), "iface", name)
	}
}

// collectTCPEstablished 输出 ESTABLISHED 状态的 TCP 连接数：读 /proc/net/snmp
// （ProtoCounters 的 CurrEstab，与 node_exporter 口径一致），不扫全进程 fd。
// collectTCPEstablished emits the ESTABLISHED TCP connection count from /proc/net/snmp
// (ProtoCounters' CurrEstab, same source as node_exporter) — no per-process fd sweep.
func (c *MetricsCollector) collectTCPEstablished(ctx context.Context, put hostPut) {
	counters, err := net.ProtoCountersWithContext(ctx, []string{"tcp"})
	if err != nil {
		return
	}
	for _, counter := range counters {
		if counter.Protocol == "tcp" {
			put("stx_host_tcp_established", float64(counter.Stats["CurrEstab"]))
			return
		}
	}
}

// collectBootAndFDs 输出开机时间与全系统已分配 fd（file-nr 仅 Linux 有，读不到跳过）。
// collectBootAndFDs emits boot time and system-wide allocated fds (file-nr exists on
// Linux only and is skipped elsewhere).
func (c *MetricsCollector) collectBootAndFDs(ctx context.Context, put hostPut) {
	if boot, err := host.BootTimeWithContext(ctx); err == nil && boot > 0 {
		put("stx_host_boot_time_seconds", float64(boot))
	}
	if data, err := os.ReadFile("/proc/sys/fs/file-nr"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			if v, err := strconv.ParseFloat(fields[0], 64); err == nil {
				put("stx_host_fds_allocated", v)
			}
		}
	}
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
