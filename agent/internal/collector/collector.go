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

// Package collector provides metrics collection functionality for the Agent.
// collector 包提供 Agent 的指标采集功能。
//
// This package provides:
// 此包提供：
// - CPU usage collection / CPU 使用率采集
// - Memory usage collection / 内存使用率采集
// - Disk usage collection / 磁盘使用率采集
// - Process status collection / 进程状态采集
package collector

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/process"
)

// MetricsCollector collects system and process metrics
// MetricsCollector 采集系统和进程指标
type MetricsCollector struct {
	// processManager is used to get process status
	// processManager 用于获取进程状态
	processManager *process.ProcessManager

	// lastCPUStats stores the last CPU stats for calculating usage
	// lastCPUStats 存储上次 CPU 统计信息用于计算使用率
	lastCPUStats *cpuStats

	// mu protects lastCPUStats
	// mu 保护 lastCPUStats
	mu sync.Mutex
}

// cpuStats stores CPU statistics for usage calculation
// cpuStats 存储 CPU 统计信息用于使用率计算
type cpuStats struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
	steal   uint64
	total   uint64
	time    time.Time
}

// NewMetricsCollector creates a new MetricsCollector instance
// NewMetricsCollector 创建新的 MetricsCollector 实例
func NewMetricsCollector(pm *process.ProcessManager) *MetricsCollector {
	return &MetricsCollector{
		processManager: pm,
	}
}

// CollectResourceUsage collects current system resource usage
// CollectResourceUsage 采集当前系统资源使用情况
// Requirements 1.3: Heartbeat message includes node resource usage
// 需求 1.3：心跳消息包含节点资源使用率
func (c *MetricsCollector) CollectResourceUsage() *pb.ResourceUsage {
	cpuUsage := c.collectCPUUsage()
	memUsage, availMem := c.collectMemoryUsage()
	diskUsage, availDisk := c.collectDiskUsage()

	return &pb.ResourceUsage{
		CpuUsage:        cpuUsage,
		MemoryUsage:     memUsage,
		DiskUsage:       diskUsage,
		AvailableMemory: availMem,
		AvailableDisk:   availDisk,
	}
}

// CollectProcessStatus collects status of all managed processes
// CollectProcessStatus 采集所有托管进程的状态
// Requirements 6.5: Return process PID, uptime, CPU usage, memory usage, start time
// 需求 6.5：返回进程 PID、运行时长、CPU 使用率、内存使用量、启动时间
func (c *MetricsCollector) CollectProcessStatus() []*pb.ProcessStatus {
	if c.processManager == nil {
		return nil
	}

	processes := c.processManager.ListProcesses()
	result := make([]*pb.ProcessStatus, 0, len(processes))

	for _, proc := range processes {
		status := &pb.ProcessStatus{
			Name:        proc.Name,
			Pid:         int32(proc.PID),
			Status:      string(proc.Status),
			Uptime:      int64(proc.Uptime.Seconds()),
			CpuUsage:    proc.CPUUsage,
			MemoryUsage: proc.MemoryUsage,
		}
		result = append(result, status)
	}

	return result
}

// Collect collects both resource usage and process status
// Collect 同时采集资源使用情况和进程状态
func (c *MetricsCollector) Collect() (*pb.ResourceUsage, []*pb.ProcessStatus) {
	return c.CollectResourceUsage(), c.CollectProcessStatus()
}

// collectCPUUsage collects CPU usage percentage
// collectCPUUsage 采集 CPU 使用率百分比
func (c *MetricsCollector) collectCPUUsage() float64 {
	switch runtime.GOOS {
	case "linux":
		return c.collectCPUUsageLinux()
	case "darwin":
		return c.collectCPUUsageDarwin()
	case "windows":
		return c.collectCPUUsageWindows()
	default:
		return 0
	}
}

// collectCPUUsageLinux collects CPU usage on Linux by reading /proc/stat
// collectCPUUsageLinux 通过读取 /proc/stat 在 Linux 上采集 CPU 使用率
func (c *MetricsCollector) collectCPUUsageLinux() float64 {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0
	}

	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0
	}

	fields := strings.Fields(line)
	if len(fields) < 8 {
		return 0
	}

	// Parse CPU stats / 解析 CPU 统计信息
	user, _ := strconv.ParseUint(fields[1], 10, 64)
	nice, _ := strconv.ParseUint(fields[2], 10, 64)
	system, _ := strconv.ParseUint(fields[3], 10, 64)
	idle, _ := strconv.ParseUint(fields[4], 10, 64)
	iowait, _ := strconv.ParseUint(fields[5], 10, 64)
	irq, _ := strconv.ParseUint(fields[6], 10, 64)
	softirq, _ := strconv.ParseUint(fields[7], 10, 64)

	var steal uint64
	if len(fields) > 8 {
		steal, _ = strconv.ParseUint(fields[8], 10, 64)
	}

	total := user + nice + system + idle + iowait + irq + softirq + steal

	currentStats := &cpuStats{
		user:    user,
		nice:    nice,
		system:  system,
		idle:    idle,
		iowait:  iowait,
		irq:     irq,
		softirq: softirq,
		steal:   steal,
		total:   total,
		time:    time.Now(),
	}

	c.mu.Lock()
	lastStats := c.lastCPUStats
	c.lastCPUStats = currentStats
	c.mu.Unlock()

	// If no previous stats, return 0 / 如果没有之前的统计信息，返回 0
	if lastStats == nil {
		return 0
	}

	// Calculate CPU usage / 计算 CPU 使用率
	totalDiff := float64(currentStats.total - lastStats.total)
	if totalDiff == 0 {
		return 0
	}

	idleDiff := float64(currentStats.idle - lastStats.idle)
	usage := (1 - idleDiff/totalDiff) * 100

	// Clamp to 0-100 / 限制在 0-100 范围内
	if usage < 0 {
		usage = 0
	}
	if usage > 100 {
		usage = 100
	}

	return usage
}

// collectCPUUsageDarwin collects CPU usage on macOS using top command
// collectCPUUsageDarwin 使用 top 命令在 macOS 上采集 CPU 使用率
func (c *MetricsCollector) collectCPUUsageDarwin() float64 {
	// Use top command to get CPU usage / 使用 top 命令获取 CPU 使用率
	cmd := exec.Command("top", "-l", "1", "-n", "0")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "CPU usage:") {
			// Parse: CPU usage: 5.26% user, 10.52% sys, 84.21% idle
			// 解析：CPU usage: 5.26% user, 10.52% sys, 84.21% idle
			parts := strings.Split(line, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.Contains(part, "idle") {
					fields := strings.Fields(part)
					if len(fields) >= 1 {
						idleStr := strings.TrimSuffix(fields[0], "%")
						idle, err := strconv.ParseFloat(idleStr, 64)
						if err == nil {
							return 100 - idle
						}
					}
				}
			}
		}
	}

	return 0
}

// collectCPUUsageWindows collects CPU usage on Windows using wmic
// collectCPUUsageWindows 使用 wmic 在 Windows 上采集 CPU 使用率
func (c *MetricsCollector) collectCPUUsageWindows() float64 {
	cmd := exec.Command("wmic", "cpu", "get", "loadpercentage", "/value")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "LoadPercentage=") {
			valueStr := strings.TrimPrefix(line, "LoadPercentage=")
			valueStr = strings.TrimSpace(valueStr)
			value, err := strconv.ParseFloat(valueStr, 64)
			if err == nil {
				return value
			}
		}
	}

	return 0
}

// collectMemoryUsage collects memory usage percentage and available memory
// collectMemoryUsage 采集内存使用率百分比和可用内存
func (c *MetricsCollector) collectMemoryUsage() (usagePercent float64, availableBytes int64) {
	switch runtime.GOOS {
	case "linux":
		return c.collectMemoryUsageLinux()
	case "darwin":
		return c.collectMemoryUsageDarwin()
	case "windows":
		return c.collectMemoryUsageWindows()
	default:
		return 0, 0
	}
}

// collectMemoryUsageLinux collects memory usage on Linux by reading /proc/meminfo
// collectMemoryUsageLinux 通过读取 /proc/meminfo 在 Linux 上采集内存使用情况
func (c *MetricsCollector) collectMemoryUsageLinux() (usagePercent float64, availableBytes int64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var memTotal, memAvailable, memFree, buffers, cached int64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}

		// Values in /proc/meminfo are in KB / /proc/meminfo 中的值以 KB 为单位
		switch fields[0] {
		case "MemTotal:":
			memTotal = value * 1024
		case "MemAvailable:":
			memAvailable = value * 1024
		case "MemFree:":
			memFree = value * 1024
		case "Buffers:":
			buffers = value * 1024
		case "Cached:":
			cached = value * 1024
		}
	}

	// If MemAvailable is not present (older kernels), calculate it
	// 如果 MemAvailable 不存在（旧内核），则计算它
	if memAvailable == 0 {
		memAvailable = memFree + buffers + cached
	}

	if memTotal > 0 {
		usagePercent = float64(memTotal-memAvailable) / float64(memTotal) * 100
	}

	return usagePercent, memAvailable
}

// collectMemoryUsageDarwin collects memory usage on macOS using vm_stat
// collectMemoryUsageDarwin 使用 vm_stat 在 macOS 上采集内存使用情况
func (c *MetricsCollector) collectMemoryUsageDarwin() (usagePercent float64, availableBytes int64) {
	// Get page size / 获取页面大小
	pageSizeCmd := exec.Command("pagesize")
	pageSizeOutput, err := pageSizeCmd.Output()
	if err != nil {
		return 0, 0
	}
	pageSize, err := strconv.ParseInt(strings.TrimSpace(string(pageSizeOutput)), 10, 64)
	if err != nil {
		pageSize = 4096 // Default page size / 默认页面大小
	}

	// Get vm_stat output / 获取 vm_stat 输出
	cmd := exec.Command("vm_stat")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	var pagesFree, pagesActive, pagesInactive, pagesSpeculative, pagesWired int64

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Pages free:") {
			pagesFree = parseVMStatValue(line)
		} else if strings.Contains(line, "Pages active:") {
			pagesActive = parseVMStatValue(line)
		} else if strings.Contains(line, "Pages inactive:") {
			pagesInactive = parseVMStatValue(line)
		} else if strings.Contains(line, "Pages speculative:") {
			pagesSpeculative = parseVMStatValue(line)
		} else if strings.Contains(line, "Pages wired down:") {
			pagesWired = parseVMStatValue(line)
		}
	}

	// Calculate memory / 计算内存
	freePages := pagesFree + pagesInactive + pagesSpeculative
	usedPages := pagesActive + pagesWired
	totalPages := freePages + usedPages

	availableBytes = freePages * pageSize

	if totalPages > 0 {
		usagePercent = float64(usedPages) / float64(totalPages) * 100
	}

	return usagePercent, availableBytes
}

// parseVMStatValue parses a value from vm_stat output line
// parseVMStatValue 从 vm_stat 输出行解析值
func parseVMStatValue(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	valueStr := strings.TrimSpace(parts[1])
	valueStr = strings.TrimSuffix(valueStr, ".")
	value, _ := strconv.ParseInt(valueStr, 10, 64)
	return value
}

// collectMemoryUsageWindows collects memory usage on Windows using wmic
// collectMemoryUsageWindows 使用 wmic 在 Windows 上采集内存使用情况
func (c *MetricsCollector) collectMemoryUsageWindows() (usagePercent float64, availableBytes int64) {
	// Get total physical memory / 获取总物理内存
	totalCmd := exec.Command("wmic", "ComputerSystem", "get", "TotalPhysicalMemory", "/value")
	totalOutput, err := totalCmd.Output()
	if err != nil {
		return 0, 0
	}

	var totalMemory int64
	lines := strings.Split(string(totalOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TotalPhysicalMemory=") {
			valueStr := strings.TrimPrefix(line, "TotalPhysicalMemory=")
			valueStr = strings.TrimSpace(valueStr)
			totalMemory, _ = strconv.ParseInt(valueStr, 10, 64)
		}
	}

	// Get free physical memory / 获取空闲物理内存
	freeCmd := exec.Command("wmic", "OS", "get", "FreePhysicalMemory", "/value")
	freeOutput, err := freeCmd.Output()
	if err != nil {
		return 0, 0
	}

	var freeMemory int64
	lines = strings.Split(string(freeOutput), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "FreePhysicalMemory=") {
			valueStr := strings.TrimPrefix(line, "FreePhysicalMemory=")
			valueStr = strings.TrimSpace(valueStr)
			// FreePhysicalMemory is in KB / FreePhysicalMemory 以 KB 为单位
			freeKB, _ := strconv.ParseInt(valueStr, 10, 64)
			freeMemory = freeKB * 1024
		}
	}

	availableBytes = freeMemory

	if totalMemory > 0 {
		usagePercent = float64(totalMemory-freeMemory) / float64(totalMemory) * 100
	}

	return usagePercent, availableBytes
}

// collectDiskUsage collects disk usage percentage and available disk space
// collectDiskUsage 采集磁盘使用率百分比和可用磁盘空间
func (c *MetricsCollector) collectDiskUsage() (usagePercent float64, availableBytes int64) {
	switch runtime.GOOS {
	case "linux", "darwin":
		return c.collectDiskUsageUnix()
	case "windows":
		return c.collectDiskUsageWindows()
	default:
		return 0, 0
	}
}

// collectDiskUsageUnix collects disk usage on Unix-like systems using df
// collectDiskUsageUnix 使用 df 在类 Unix 系统上采集磁盘使用情况
func (c *MetricsCollector) collectDiskUsageUnix() (usagePercent float64, availableBytes int64) {
	// Get disk usage for root filesystem / 获取根文件系统的磁盘使用情况
	cmd := exec.Command("df", "-k", "/")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, 0
	}

	// Parse the second line (first line is header)
	// 解析第二行（第一行是标题）
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, 0
	}

	// Fields: Filesystem, 1K-blocks, Used, Available, Use%, Mounted on
	// 字段：文件系统、1K 块、已用、可用、使用率、挂载点
	total, _ := strconv.ParseInt(fields[1], 10, 64)
	available, _ := strconv.ParseInt(fields[3], 10, 64)

	// Convert from KB to bytes / 从 KB 转换为字节
	availableBytes = available * 1024
	totalBytes := total * 1024

	if totalBytes > 0 {
		usagePercent = float64(totalBytes-availableBytes) / float64(totalBytes) * 100
	}

	return usagePercent, availableBytes
}

// collectDiskUsageWindows collects disk usage on Windows using wmic
// collectDiskUsageWindows 使用 wmic 在 Windows 上采集磁盘使用情况
func (c *MetricsCollector) collectDiskUsageWindows() (usagePercent float64, availableBytes int64) {
	// Get disk info for C: drive / 获取 C: 盘的磁盘信息
	cmd := exec.Command("wmic", "logicaldisk", "where", "DeviceID='C:'", "get", "Size,FreeSpace", "/value")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	var totalSize, freeSpace int64

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Size=") {
			valueStr := strings.TrimPrefix(line, "Size=")
			valueStr = strings.TrimSpace(valueStr)
			totalSize, _ = strconv.ParseInt(valueStr, 10, 64)
		} else if strings.HasPrefix(line, "FreeSpace=") {
			valueStr := strings.TrimPrefix(line, "FreeSpace=")
			valueStr = strings.TrimSpace(valueStr)
			freeSpace, _ = strconv.ParseInt(valueStr, 10, 64)
		}
	}

	availableBytes = freeSpace

	if totalSize > 0 {
		usagePercent = float64(totalSize-freeSpace) / float64(totalSize) * 100
	}

	return usagePercent, availableBytes
}

// GetSystemInfo collects basic system information
// GetSystemInfo 采集基本系统信息
func (c *MetricsCollector) GetSystemInfo() *pb.SystemInfo {
	cpuCores := runtime.NumCPU()
	_, totalMem := c.getTotalMemory()
	_, totalDisk := c.getTotalDisk()
	kernelVersion := c.getKernelVersion()

	return &pb.SystemInfo{
		CpuCores:      int32(cpuCores),
		TotalMemory:   totalMem,
		TotalDisk:     totalDisk,
		KernelVersion: kernelVersion,
	}
}

// getTotalMemory returns total memory in bytes
// getTotalMemory 返回总内存（字节）
func (c *MetricsCollector) getTotalMemory() (usagePercent float64, totalBytes int64) {
	switch runtime.GOOS {
	case "linux":
		return c.getTotalMemoryLinux()
	case "darwin":
		return c.getTotalMemoryDarwin()
	case "windows":
		return c.getTotalMemoryWindows()
	default:
		return 0, 0
	}
}

// getTotalMemoryLinux gets total memory on Linux
// getTotalMemoryLinux 在 Linux 上获取总内存
func (c *MetricsCollector) getTotalMemoryLinux() (usagePercent float64, totalBytes int64) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				value, _ := strconv.ParseInt(fields[1], 10, 64)
				totalBytes = value * 1024 // KB to bytes
				return 0, totalBytes
			}
		}
	}
	return 0, 0
}

// getTotalMemoryDarwin gets total memory on macOS
// getTotalMemoryDarwin 在 macOS 上获取总内存
func (c *MetricsCollector) getTotalMemoryDarwin() (usagePercent float64, totalBytes int64) {
	cmd := exec.Command("sysctl", "-n", "hw.memsize")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	totalBytes, _ = strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
	return 0, totalBytes
}

// getTotalMemoryWindows gets total memory on Windows
// getTotalMemoryWindows 在 Windows 上获取总内存
func (c *MetricsCollector) getTotalMemoryWindows() (usagePercent float64, totalBytes int64) {
	cmd := exec.Command("wmic", "ComputerSystem", "get", "TotalPhysicalMemory", "/value")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "TotalPhysicalMemory=") {
			valueStr := strings.TrimPrefix(line, "TotalPhysicalMemory=")
			valueStr = strings.TrimSpace(valueStr)
			totalBytes, _ = strconv.ParseInt(valueStr, 10, 64)
			return 0, totalBytes
		}
	}
	return 0, 0
}

// getTotalDisk returns total disk space in bytes
// getTotalDisk 返回总磁盘空间（字节）
func (c *MetricsCollector) getTotalDisk() (usagePercent float64, totalBytes int64) {
	switch runtime.GOOS {
	case "linux", "darwin":
		return c.getTotalDiskUnix()
	case "windows":
		return c.getTotalDiskWindows()
	default:
		return 0, 0
	}
}

// getTotalDiskUnix gets total disk space on Unix-like systems
// getTotalDiskUnix 在类 Unix 系统上获取总磁盘空间
func (c *MetricsCollector) getTotalDiskUnix() (usagePercent float64, totalBytes int64) {
	cmd := exec.Command("df", "-k", "/")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, 0
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return 0, 0
	}

	total, _ := strconv.ParseInt(fields[1], 10, 64)
	totalBytes = total * 1024 // KB to bytes
	return 0, totalBytes
}

// getTotalDiskWindows gets total disk space on Windows
// getTotalDiskWindows 在 Windows 上获取总磁盘空间
func (c *MetricsCollector) getTotalDiskWindows() (usagePercent float64, totalBytes int64) {
	cmd := exec.Command("wmic", "logicaldisk", "where", "DeviceID='C:'", "get", "Size", "/value")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Size=") {
			valueStr := strings.TrimPrefix(line, "Size=")
			valueStr = strings.TrimSpace(valueStr)
			totalBytes, _ = strconv.ParseInt(valueStr, 10, 64)
			return 0, totalBytes
		}
	}
	return 0, 0
}

// getKernelVersion returns the kernel version
// getKernelVersion 返回内核版本
func (c *MetricsCollector) getKernelVersion() string {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/version")
		if err != nil {
			return ""
		}
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			return fields[2]
		}
		return ""
	case "darwin":
		cmd := exec.Command("uname", "-r")
		output, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	case "windows":
		cmd := exec.Command("cmd", "/c", "ver")
		output, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	default:
		return ""
	}
}

// GetHostname returns the hostname
// GetHostname 返回主机名
func (c *MetricsCollector) GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return hostname
}

// GetOSType returns the operating system type
// GetOSType 返回操作系统类型
func (c *MetricsCollector) GetOSType() string {
	return runtime.GOOS
}

// GetArch returns the CPU architecture
// GetArch 返回 CPU 架构
func (c *MetricsCollector) GetArch() string {
	return runtime.GOARCH
}

// GetIPAddress returns the primary IP address
// GetIPAddress 返回主 IP 地址
func (c *MetricsCollector) GetIPAddress() string {
	switch runtime.GOOS {
	case "linux", "darwin":
		return c.getIPAddressUnix()
	case "windows":
		return c.getIPAddressWindows()
	default:
		return ""
	}
}

// ListLocalIPAddresses 返回本机全部非回环、非链路本地网卡 IP（去重）。
// 用于控制面校验用户填写的主机 IP 是否属于当前机器。
// ListLocalIPAddresses returns all non-loopback, non-link-local interface IPs (deduplicated).
// Used by Control Plane to verify whether the user-entered host IP belongs to this machine.
func (c *MetricsCollector) ListLocalIPAddresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			s := ip.String()
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			ips = append(ips, s)
		}
	}
	return ips
}

// GetOutboundIP 获取用于访问目标地址的本地出口 IP 地址。
// 它通过查询操作系统内核路由表获取，不会发送实际的网络数据包。
// GetOutboundIP determines the local outbound IP address used to reach the target address.
// It queries the OS routing table without sending any actual network packets.
func (c *MetricsCollector) GetOutboundIP(targetAddr string) string {
	targetAddr = strings.TrimSpace(targetAddr)
	if targetAddr == "" {
		return ""
	}

	// 确保目标地址包含端口以便进行 UDP 拨号
	// Ensure target has a port for UDP dial
	host, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		host = targetAddr
		port = "80"
	}
	if host == "" {
		return ""
	}

	conn, err := net.DialTimeout("udp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr == nil || localAddr.IP == nil {
		return ""
	}

	// 忽略未指定地址 (0.0.0.0, ::)
	// Ignore unspecified addresses (0.0.0.0, ::)
	if localAddr.IP.IsUnspecified() {
		return ""
	}

	return localAddr.IP.String()
}

// getIPAddressUnix gets IP address on Unix-like systems
// getIPAddressUnix 在类 Unix 系统上获取 IP 地址
func (c *MetricsCollector) getIPAddressUnix() string {
	cmd := exec.Command("hostname", "-I")
	output, err := cmd.Output()
	if err != nil {
		// Try alternative method for macOS / 尝试 macOS 的替代方法
		cmd = exec.Command("ipconfig", "getifaddr", "en0")
		output, err = cmd.Output()
		if err != nil {
			return ""
		}
	}

	ips := strings.Fields(string(output))
	if len(ips) > 0 {
		return ips[0]
	}
	return ""
}

// getIPAddressWindows gets IP address on Windows
// getIPAddressWindows 在 Windows 上获取 IP 地址
func (c *MetricsCollector) getIPAddressWindows() string {
	cmd := exec.Command("powershell", "-Command",
		"(Get-NetIPAddress -AddressFamily IPv4 | Where-Object {$_.InterfaceAlias -notlike '*Loopback*'} | Select-Object -First 1).IPAddress")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// CollectProcessStatusByPID collects status for a specific process by PID
// CollectProcessStatusByPID 通过 PID 采集特定进程的状态
func (c *MetricsCollector) CollectProcessStatusByPID(pid int, name string) *pb.ProcessStatus {
	if pid <= 0 {
		return nil
	}

	cpuUsage, memUsage := getProcessMetrics(pid)
	status := "unknown"
	if isProcessAlive(pid) {
		status = "running"
	} else {
		status = "stopped"
	}

	return &pb.ProcessStatus{
		Name:        name,
		Pid:         int32(pid),
		Status:      status,
		CpuUsage:    cpuUsage,
		MemoryUsage: memUsage,
	}
}

// isProcessAlive checks if a process with the given PID is alive
// isProcessAlive 检查给定 PID 的进程是否存活
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to send signal 0 to check
	// 在 Unix 上，FindProcess 总是成功，所以我们需要发送信号 0 来检查
	if runtime.GOOS != "windows" {
		err = process.Signal(os.Signal(nil))
		// Signal(nil) returns nil if process exists
		// 如果进程存在，Signal(nil) 返回 nil
		return err == nil
	}

	// On Windows, use tasklist / 在 Windows 上使用 tasklist
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), strconv.Itoa(pid))
}

// getProcessMetrics gets CPU and memory usage for a process
// getProcessMetrics 获取进程的 CPU 和内存使用率
func getProcessMetrics(pid int) (cpuUsage float64, memoryUsage int64) {
	switch runtime.GOOS {
	case "linux":
		return getProcessMetricsLinux(pid)
	case "darwin":
		return getProcessMetricsDarwin(pid)
	case "windows":
		return getProcessMetricsWindows(pid)
	default:
		return 0, 0
	}
}

// getProcessMetricsLinux gets process metrics on Linux
// getProcessMetricsLinux 在 Linux 上获取进程指标
func getProcessMetricsLinux(pid int) (cpuUsage float64, memoryUsage int64) {
	// Read /proc/[pid]/statm for memory info
	// 读取 /proc/[pid]/statm 获取内存信息
	statmPath := fmt.Sprintf("/proc/%d/statm", pid)
	statmData, err := os.ReadFile(statmPath)
	if err != nil {
		return 0, 0
	}

	statmFields := strings.Fields(string(statmData))
	if len(statmFields) >= 2 {
		// RSS is in pages, convert to bytes (assuming 4KB pages)
		// RSS 以页为单位，转换为字节（假设 4KB 页）
		rss, _ := strconv.ParseInt(statmFields[1], 10, 64)
		memoryUsage = rss * 4096
	}

	return 0, memoryUsage
}

// getProcessMetricsDarwin gets process metrics on macOS
// getProcessMetricsDarwin 在 macOS 上获取进程指标
func getProcessMetricsDarwin(pid int) (cpuUsage float64, memoryUsage int64) {
	cmd := exec.Command("ps", "-o", "rss=,pcpu=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	fields := strings.Fields(string(output))
	if len(fields) >= 2 {
		// RSS is in KB, convert to bytes
		// RSS 以 KB 为单位，转换为字节
		rss, _ := strconv.ParseInt(fields[0], 10, 64)
		memoryUsage = rss * 1024

		// CPU percentage
		// CPU 百分比
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		cpuUsage = cpu
	}

	return cpuUsage, memoryUsage
}

// getProcessMetricsWindows gets process metrics on Windows
// getProcessMetricsWindows 在 Windows 上获取进程指标
func getProcessMetricsWindows(pid int) (cpuUsage float64, memoryUsage int64) {
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "WorkingSetSize", "/value")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "WorkingSetSize=") {
			value := strings.TrimPrefix(line, "WorkingSetSize=")
			value = strings.TrimSpace(value)
			mem, _ := strconv.ParseInt(value, 10, 64)
			memoryUsage = mem
		}
	}

	return 0, memoryUsage
}

// JVMMetrics contains JVM-specific metrics
// JVMMetrics 包含 JVM 特定的指标
type JVMMetrics struct {
	// PID is the JVM process ID
	// PID 是 JVM 进程 ID
	PID int `json:"pid"`

	// Name is the process name or main class
	// Name 是进程名称或主类
	Name string `json:"name"`

	// HeapUsed is the used heap memory in bytes
	// HeapUsed 是已使用的堆内存（字节）
	HeapUsed int64 `json:"heap_used"`

	// HeapMax is the maximum heap memory in bytes
	// HeapMax 是最大堆内存（字节）
	HeapMax int64 `json:"heap_max"`

	// HeapUsagePercent is the heap usage percentage (0-100)
	// HeapUsagePercent 是堆使用率百分比（0-100）
	HeapUsagePercent float64 `json:"heap_usage_percent"`

	// NonHeapUsed is the used non-heap memory in bytes
	// NonHeapUsed 是已使用的非堆内存（字节）
	NonHeapUsed int64 `json:"non_heap_used"`

	// GCCount is the total GC count
	// GCCount 是总 GC 次数
	GCCount int64 `json:"gc_count"`

	// GCTime is the total GC time in milliseconds
	// GCTime 是总 GC 时间（毫秒）
	GCTime int64 `json:"gc_time"`

	// ThreadCount is the current thread count
	// ThreadCount 是当前线程数
	ThreadCount int `json:"thread_count"`

	// ClassLoaded is the number of loaded classes
	// ClassLoaded 是已加载的类数量
	ClassLoaded int `json:"class_loaded"`

	// Uptime is the JVM uptime in seconds
	// Uptime 是 JVM 运行时间（秒）
	Uptime int64 `json:"uptime"`
}

// CollectJVMMetrics collects JVM metrics for SeaTunnel processes
// CollectJVMMetrics 采集 SeaTunnel 进程的 JVM 指标
// This uses jstat and jps commands which require JDK to be installed
// 这使用 jstat 和 jps 命令，需要安装 JDK
func (c *MetricsCollector) CollectJVMMetrics() []*JVMMetrics {
	// Find SeaTunnel JVM processes using jps
	// 使用 jps 查找 SeaTunnel JVM 进程
	pids := c.findSeaTunnelJVMProcesses()
	if len(pids) == 0 {
		return nil
	}

	var metrics []*JVMMetrics
	for _, pidInfo := range pids {
		jvmMetrics := c.collectJVMMetricsForPID(pidInfo.pid, pidInfo.name)
		if jvmMetrics != nil {
			metrics = append(metrics, jvmMetrics)
		}
	}

	return metrics
}

// jvmProcessInfo contains JVM process information
// jvmProcessInfo 包含 JVM 进程信息
type jvmProcessInfo struct {
	pid  int
	name string
}

// findSeaTunnelJVMProcesses finds SeaTunnel JVM processes using jps
// findSeaTunnelJVMProcesses 使用 jps 查找 SeaTunnel JVM 进程
func (c *MetricsCollector) findSeaTunnelJVMProcesses() []jvmProcessInfo {
	// Try to run jps command / 尝试运行 jps 命令
	cmd := exec.Command("jps", "-l")
	output, err := cmd.Output()
	if err != nil {
		// jps not available, try alternative method
		// jps 不可用，尝试替代方法
		return c.findSeaTunnelProcessesByName()
	}

	var processes []jvmProcessInfo
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		name := fields[1]
		// Check if this is a SeaTunnel process
		// 检查是否是 SeaTunnel 进程
		if strings.Contains(strings.ToLower(name), "seatunnel") ||
			strings.Contains(strings.ToLower(name), "hazelcast") {
			processes = append(processes, jvmProcessInfo{pid: pid, name: name})
		}
	}

	return processes
}

// findSeaTunnelProcessesByName finds SeaTunnel processes by searching process names
// findSeaTunnelProcessesByName 通过搜索进程名称查找 SeaTunnel 进程
func (c *MetricsCollector) findSeaTunnelProcessesByName() []jvmProcessInfo {
	var processes []jvmProcessInfo

	switch runtime.GOOS {
	case "linux":
		// Use ps to find java processes with seatunnel in command line
		// 使用 ps 查找命令行中包含 seatunnel 的 java 进程
		cmd := exec.Command("ps", "-eo", "pid,args")
		output, err := cmd.Output()
		if err != nil {
			return nil
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(strings.ToLower(line), "seatunnel") &&
				strings.Contains(strings.ToLower(line), "java") {
				fields := strings.Fields(line)
				if len(fields) >= 1 {
					pid, err := strconv.Atoi(fields[0])
					if err == nil {
						processes = append(processes, jvmProcessInfo{pid: pid, name: "SeaTunnel"})
					}
				}
			}
		}

	case "windows":
		// Use wmic to find java processes
		// 使用 wmic 查找 java 进程
		cmd := exec.Command("wmic", "process", "where", "name like '%java%'", "get", "ProcessId,CommandLine", "/value")
		output, err := cmd.Output()
		if err != nil {
			return nil
		}

		lines := strings.Split(string(output), "\n")
		var currentPID int
		var currentCmd string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ProcessId=") {
				pidStr := strings.TrimPrefix(line, "ProcessId=")
				currentPID, _ = strconv.Atoi(strings.TrimSpace(pidStr))
			} else if strings.HasPrefix(line, "CommandLine=") {
				currentCmd = strings.TrimPrefix(line, "CommandLine=")
				if currentPID > 0 && strings.Contains(strings.ToLower(currentCmd), "seatunnel") {
					processes = append(processes, jvmProcessInfo{pid: currentPID, name: "SeaTunnel"})
				}
				currentPID = 0
				currentCmd = ""
			}
		}
	}

	return processes
}

// collectJVMMetricsForPID collects JVM metrics for a specific PID using jstat
// collectJVMMetricsForPID 使用 jstat 采集特定 PID 的 JVM 指标
func (c *MetricsCollector) collectJVMMetricsForPID(pid int, name string) *JVMMetrics {
	metrics := &JVMMetrics{
		PID:  pid,
		Name: name,
	}

	// Collect GC stats using jstat -gc
	// 使用 jstat -gc 采集 GC 统计信息
	c.collectJStatGC(pid, metrics)

	// Collect class loading stats using jstat -class
	// 使用 jstat -class 采集类加载统计信息
	c.collectJStatClass(pid, metrics)

	// Collect thread count
	// 采集线程数
	metrics.ThreadCount = c.getJVMThreadCount(pid)

	return metrics
}

// collectJStatGC collects GC statistics using jstat -gc
// collectJStatGC 使用 jstat -gc 采集 GC 统计信息
func (c *MetricsCollector) collectJStatGC(pid int, metrics *JVMMetrics) {
	cmd := exec.Command("jstat", "-gc", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return
	}

	// Parse header and values
	// 解析标题和值
	headers := strings.Fields(lines[0])
	values := strings.Fields(lines[1])

	if len(headers) != len(values) {
		return
	}

	// Create a map of header -> value
	// 创建标题 -> 值的映射
	data := make(map[string]float64)
	for i, header := range headers {
		val, err := strconv.ParseFloat(values[i], 64)
		if err == nil {
			data[header] = val
		}
	}

	// Calculate heap usage
	// 计算堆使用情况
	// S0C, S1C: Survivor space 0/1 capacity (KB)
	// S0U, S1U: Survivor space 0/1 used (KB)
	// EC: Eden space capacity (KB)
	// EU: Eden space used (KB)
	// OC: Old space capacity (KB)
	// OU: Old space used (KB)
	// MC: Metaspace capacity (KB)
	// MU: Metaspace used (KB)
	// YGC: Young GC count
	// YGCT: Young GC time (seconds)
	// FGC: Full GC count
	// FGCT: Full GC time (seconds)

	heapCapacity := (data["S0C"] + data["S1C"] + data["EC"] + data["OC"]) * 1024 // Convert KB to bytes
	heapUsed := (data["S0U"] + data["S1U"] + data["EU"] + data["OU"]) * 1024

	metrics.HeapMax = int64(heapCapacity)
	metrics.HeapUsed = int64(heapUsed)
	if heapCapacity > 0 {
		metrics.HeapUsagePercent = (heapUsed / heapCapacity) * 100
	}

	// Non-heap (Metaspace)
	// 非堆（元空间）
	metrics.NonHeapUsed = int64(data["MU"] * 1024)

	// GC stats
	// GC 统计
	metrics.GCCount = int64(data["YGC"] + data["FGC"])
	metrics.GCTime = int64((data["YGCT"] + data["FGCT"]) * 1000) // Convert seconds to milliseconds
}

// collectJStatClass collects class loading statistics using jstat -class
// collectJStatClass 使用 jstat -class 采集类加载统计信息
func (c *MetricsCollector) collectJStatClass(pid int, metrics *JVMMetrics) {
	cmd := exec.Command("jstat", "-class", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return
	}

	// Parse values (Loaded, Bytes, Unloaded, Bytes, Time)
	// 解析值（已加载、字节、已卸载、字节、时间）
	values := strings.Fields(lines[1])
	if len(values) >= 1 {
		loaded, err := strconv.Atoi(values[0])
		if err == nil {
			metrics.ClassLoaded = loaded
		}
	}
}

// getJVMThreadCount gets the thread count for a JVM process
// getJVMThreadCount 获取 JVM 进程的线程数
func (c *MetricsCollector) getJVMThreadCount(pid int) int {
	switch runtime.GOOS {
	case "linux":
		// Read /proc/[pid]/status for thread count
		// 读取 /proc/[pid]/status 获取线程数
		statusPath := fmt.Sprintf("/proc/%d/status", pid)
		data, err := os.ReadFile(statusPath)
		if err != nil {
			return 0
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Threads:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					count, _ := strconv.Atoi(fields[1])
					return count
				}
			}
		}

	case "darwin":
		// Use ps to get thread count
		// 使用 ps 获取线程数
		cmd := exec.Command("ps", "-M", "-p", strconv.Itoa(pid))
		output, err := cmd.Output()
		if err != nil {
			return 0
		}
		// Count lines minus header
		// 计算行数减去标题
		lines := strings.Split(string(output), "\n")
		count := 0
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				count++
			}
		}
		return count - 1 // Subtract header line / 减去标题行

	case "windows":
		// Use wmic to get thread count
		// 使用 wmic 获取线程数
		cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("ProcessId=%d", pid), "get", "ThreadCount", "/value")
		output, err := cmd.Output()
		if err != nil {
			return 0
		}

		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ThreadCount=") {
				countStr := strings.TrimPrefix(line, "ThreadCount=")
				count, _ := strconv.Atoi(strings.TrimSpace(countStr))
				return count
			}
		}
	}

	return 0
}

// GetJVMMetricsSummary returns a summary string of JVM metrics
// GetJVMMetricsSummary 返回 JVM 指标的摘要字符串
func (c *MetricsCollector) GetJVMMetricsSummary() string {
	metrics := c.CollectJVMMetrics()
	if len(metrics) == 0 {
		return "No SeaTunnel JVM processes found / 未找到 SeaTunnel JVM 进程"
	}

	var sb strings.Builder
	for _, m := range metrics {
		sb.WriteString(fmt.Sprintf("PID: %d, Name: %s\n", m.PID, m.Name))
		sb.WriteString(fmt.Sprintf("  Heap: %d MB / %d MB (%.1f%%)\n",
			m.HeapUsed/(1024*1024), m.HeapMax/(1024*1024), m.HeapUsagePercent))
		sb.WriteString(fmt.Sprintf("  Non-Heap: %d MB\n", m.NonHeapUsed/(1024*1024)))
		sb.WriteString(fmt.Sprintf("  GC: %d times, %d ms\n", m.GCCount, m.GCTime))
		sb.WriteString(fmt.Sprintf("  Threads: %d, Classes: %d\n", m.ThreadCount, m.ClassLoaded))
	}

	return sb.String()
}
