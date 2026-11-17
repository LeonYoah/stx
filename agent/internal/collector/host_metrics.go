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
// Host fine-grained metric collection (E3). Every sample carries the stx_host_ prefix;
// the control plane writes them verbatim into the embedded tsdb (job="stx_host").
// Collection goes through gopsutil (cross linux/darwin/windows); the only fallback is
// /proc/sys/fs/file-nr, which exists on Linux only and is skipped silently elsewhere.
package collector

import (
	"os"
	"sort"
	"strconv"
	"strings"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// hostMetricCap 限定单类对象（挂载点 / 磁盘 / 网卡）上报的条数，超出的合并为 "other"，控制 tsdb 基数。
// hostMetricCap bounds how many objects (mounts / disks / interfaces) are reported per class;
// the rest are merged into "other" to cap tsdb cardinality.
const hostMetricCap = 8

// pseudoFilesystems 是不需要容量视角的文件系统类型（gopsutil all=false 已滤一遍，这里兜底）。
// pseudoFilesystems are fstypes carrying no capacity signal (gopsutil's all=false filters most;
// this is a safety net).
var pseudoFilesystems = map[string]bool{
	"proc": true, "sysfs": true, "devpts": true, "devtmpfs": true,
	"cgroup": true, "cgroup2": true, "tmpfs": true, "overlay": true,
	"squashfs": true, "autofs": true, "debugfs": true, "tracefs": true,
	"securityfs": true, "pstore": true, "bpf": true, "configfs": true,
	"fusectl": true, "hugetlbfs": true, "mqueue": true, "ramfs": true,
	"nsfs": true, "efivarfs": true, "binfmt_misc": true, "sunrpc": true,
}

// pseudoInterface 是虚拟网卡前缀：它们不反映物理网络流量。
// pseudoInterface are virtual interface prefixes that do not reflect physical traffic.
var pseudoInterface = []string{
	"lo", "veth", "docker", "br-", "virbr", "kube-", "cni", "flannel", "cali", "tunl", "wg", "utun", "gif", "stf", "awdl", "llw",
}

// CollectHostMetrics 采集一轮 stx_host_* 指标样本。
// 单个采集源失败不阻断其余指标（每步独立容错），返回 nil 表示整轮不可用。
// CollectHostMetrics gathers one round of stx_host_* samples.
// A single failing source does not abort the others (each step tolerates independently);
// nil means the whole round failed.
func (c *MetricsCollector) CollectHostMetrics() []*pb.HostMetricSample {
	var out []*pb.HostMetricSample
	put := func(name string, value float64, dims ...string) {
		labels := map[string]string{}
		for i := 0; i+1 < len(dims); i += 2 {
			labels[dims[i]] = dims[i+1]
		}
		out = append(out, &pb.HostMetricSample{Name: name, Labels: labels, Value: value})
	}

	c.collectCPUTimes(put)
	c.collectLoadAvg(put)
	c.collectMemory(put)
	c.collectFilesystems(put)
	c.collectDiskIO(put)
	c.collectNetIO(put)
	c.collectTCPEstablished(put)
	c.collectBootAndFDs(put)
	return out
}

// hostPut 是指标落点函数签名：name + value + 偶数个维度键值。
// hostPut is the sample sink signature: name + value + an even number of dimension pairs.
type hostPut func(name string, value float64, dims ...string)

// collectCPUTimes 输出按 mode 拆分的累计 CPU 秒数（rate 交给 PromQL）与核数。
// collectCPUTimes emits cumulative CPU seconds per mode (rates are left to PromQL) plus
// the core count.
func (c *MetricsCollector) collectCPUTimes(put hostPut) {
	times, err := cpu.Times(false)
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
	if cores, err := cpu.Counts(true); err == nil && cores > 0 {
		put("stx_host_cpu_cores", float64(cores))
	}
}

// collectLoadAvg 输出 1m/5m/15m 负载。
// collectLoadAvg emits the 1m/5m/15m load averages.
func (c *MetricsCollector) collectLoadAvg(put hostPut) {
	avg, err := load.Avg()
	if err != nil {
		return
	}
	put("stx_host_load", avg.Load1, "kind", "1m")
	put("stx_host_load", avg.Load5, "kind", "5m")
	put("stx_host_load", avg.Load15, "kind", "15m")
}

// collectMemory 输出内存与交换区（gopsutil 已统一为 bytes）。
// collectMemory emits memory and swap figures (gopsutil already normalizes to bytes).
func (c *MetricsCollector) collectMemory(put hostPut) {
	if vm, err := mem.VirtualMemory(); err == nil {
		put("stx_host_memory_bytes", float64(vm.Total), "kind", "total")
		put("stx_host_memory_bytes", float64(vm.Free), "kind", "free")
		put("stx_host_memory_bytes", float64(vm.Available), "kind", "available")
		put("stx_host_memory_bytes", float64(vm.Buffers), "kind", "buffers")
		put("stx_host_memory_bytes", float64(vm.Cached), "kind", "cached")
	}
	if sw, err := mem.SwapMemory(); err == nil {
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

// collectFilesystems 枚举真实分区（伪 fs 双重过滤），按使用率取前 hostMetricCap，
// 其余合并为 "other"。
// collectFilesystems enumerates real partitions (pseudo fstypes double-filtered), keeps the
// top hostMetricCap by usage and merges the rest into "other".
func (c *MetricsCollector) collectFilesystems(put hostPut) {
	parts, err := disk.Partitions(false)
	if err != nil {
		return
	}
	var stats []fsStat
	for _, part := range parts {
		if pseudoFilesystems[part.Fstype] {
			continue
		}
		usage, err := disk.Usage(part.Mountpoint)
		if err != nil {
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

// collectDiskIO 输出整盘设备的读写次数 / 字节 / IO 耗时；超出 hostMetricCap 的按吞吐合并 "other"。
// collectDiskIO emits whole-disk read/write counts, bytes and time-in-io; devices beyond
// hostMetricCap merge into "other" by throughput.
func (c *MetricsCollector) collectDiskIO(put hostPut) {
	counters, err := disk.IOCounters()
	if err != nil {
		return
	}
	var disks []diskStat
	for name, io := range counters {
		if !wholeDisk(name) {
			continue
		}
		disks = append(disks, diskStat{
			name: name, reads: float64(io.ReadCount), readBytes: float64(io.ReadBytes),
			writes: float64(io.WriteCount), writeBytes: float64(io.WriteBytes),
			ioTimeMillis: float64(io.IoTime),
		})
	}
	sort.Slice(disks, func(i, j int) bool {
		return disks[i].readBytes+disks[i].writeBytes > disks[j].readBytes+disks[j].writeBytes
	})
	emit := func(d diskStat) {
		put("stx_host_disk_reads_total", d.reads, "device", d.name)
		put("stx_host_disk_read_bytes_total", d.readBytes, "device", d.name)
		put("stx_host_disk_writes_total", d.writes, "device", d.name)
		put("stx_host_disk_write_bytes_total", d.writeBytes, "device", d.name)
		put("stx_host_disk_io_time_seconds_total", d.ioTimeMillis/1000, "device", d.name)
	}
	var other diskStat
	other.name = "other"
	for i, d := range disks {
		if i >= hostMetricCap {
			other.reads += d.reads
			other.readBytes += d.readBytes
			other.writes += d.writes
			other.writeBytes += d.writeBytes
			other.ioTimeMillis += d.ioTimeMillis
			continue
		}
		emit(d)
	}
	if len(disks) > hostMetricCap {
		emit(other)
	}
}

// netStat 是一块网卡的收发计数。
// netStat is the rx/tx counters of one interface.
type netStat struct {
	name      string
	rxBytes   float64
	rxPackets float64
	rxErrors  float64
	rxDropped float64
	txBytes   float64
	txPackets float64
	txErrors  float64
	txDropped float64
}

// collectNetIO 输出真实网卡的收发计数；虚拟网卡跳过，超出 hostMetricCap 的合并 "other"。
// collectNetIO emits per-interface counters for real NICs; virtual interfaces are skipped
// and interfaces beyond hostMetricCap merge into "other".
func (c *MetricsCollector) collectNetIO(put hostPut) {
	counters, err := net.IOCounters(true)
	if err != nil {
		return
	}
	var nets []netStat
	for _, io := range counters {
		skip := false
		for _, prefix := range pseudoInterface {
			if strings.HasPrefix(io.Name, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		nets = append(nets, netStat{
			name: io.Name, rxBytes: float64(io.BytesRecv), rxPackets: float64(io.PacketsRecv),
			rxErrors: float64(io.Errin), rxDropped: float64(io.Dropin),
			txBytes: float64(io.BytesSent), txPackets: float64(io.PacketsSent),
			txErrors: float64(io.Errout), txDropped: float64(io.Dropout),
		})
	}
	sort.Slice(nets, func(i, j int) bool {
		return nets[i].rxBytes+nets[i].txBytes > nets[j].rxBytes+nets[j].txBytes
	})
	emit := func(n netStat) {
		put("stx_host_net_receive_bytes_total", n.rxBytes, "iface", n.name)
		put("stx_host_net_receive_packets_total", n.rxPackets, "iface", n.name)
		put("stx_host_net_receive_errors_total", n.rxErrors, "iface", n.name)
		put("stx_host_net_receive_dropped_total", n.rxDropped, "iface", n.name)
		put("stx_host_net_transmit_bytes_total", n.txBytes, "iface", n.name)
		put("stx_host_net_transmit_packets_total", n.txPackets, "iface", n.name)
		put("stx_host_net_transmit_errors_total", n.txErrors, "iface", n.name)
		put("stx_host_net_transmit_dropped_total", n.txDropped, "iface", n.name)
	}
	var other netStat
	other.name = "other"
	for i, n := range nets {
		if i >= hostMetricCap {
			other.rxBytes += n.rxBytes
			other.rxPackets += n.rxPackets
			other.rxErrors += n.rxErrors
			other.rxDropped += n.rxDropped
			other.txBytes += n.txBytes
			other.txPackets += n.txPackets
			other.txErrors += n.txErrors
			other.txDropped += n.txDropped
			continue
		}
		emit(n)
	}
	if len(nets) > hostMetricCap {
		emit(other)
	}
}

// collectTCPEstablished 统计 ESTABLISHED 状态的 TCP 连接数。
// collectTCPEstablished counts TCP connections in ESTABLISHED state.
func (c *MetricsCollector) collectTCPEstablished(put hostPut) {
	conns, err := net.Connections("tcp")
	if err != nil {
		return
	}
	total := 0
	for _, conn := range conns {
		if conn.Status == "ESTABLISHED" {
			total++
		}
	}
	put("stx_host_tcp_established", float64(total))
}

// collectBootAndFDs 输出开机时间与全系统已分配 fd（file-nr 仅 Linux 有，读不到跳过）。
// collectBootAndFDs emits boot time and system-wide allocated fds (file-nr exists on
// Linux only and is skipped elsewhere).
func (c *MetricsCollector) collectBootAndFDs(put hostPut) {
	if boot, err := host.BootTime(); err == nil && boot > 0 {
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
