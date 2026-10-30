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

// Package installer provides SeaTunnel installation management for the Agent.
// installer 包提供 Agent 的 SeaTunnel 安装管理功能。
package installer

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// CheckStatus represents the status of a precheck item
// CheckStatus 表示预检查项的状态
type CheckStatus string

const (
	// CheckStatusPassed indicates the check passed
	// CheckStatusPassed 表示检查通过
	CheckStatusPassed CheckStatus = "passed"

	// CheckStatusFailed indicates the check failed
	// CheckStatusFailed 表示检查失败
	CheckStatusFailed CheckStatus = "failed"

	// CheckStatusWarning indicates the check passed with warnings
	// CheckStatusWarning 表示检查通过但有警告
	CheckStatusWarning CheckStatus = "warning"
)

// CheckName represents the name of a precheck item
// CheckName 表示预检查项的名称
type CheckName string

const (
	// CheckNameMemory is the memory check name
	// CheckNameMemory 是内存检查名称
	CheckNameMemory CheckName = "memory"

	// CheckNameCPU is the CPU check name
	// CheckNameCPU 是 CPU 检查名称
	CheckNameCPU CheckName = "cpu"

	// CheckNameDisk is the disk space check name
	// CheckNameDisk 是磁盘空间检查名称
	CheckNameDisk CheckName = "disk"

	// CheckNamePorts is the ports check name
	// CheckNamePorts 是端口检查名称
	CheckNamePorts CheckName = "ports"

	// CheckNameJava is the Java environment check name
	// CheckNameJava 是 Java 环境检查名称
	CheckNameJava CheckName = "java"
)

// AllCheckNames returns all check names in order
// AllCheckNames 返回所有检查名称（按顺序）
func AllCheckNames() []CheckName {
	return []CheckName{
		CheckNameMemory,
		CheckNameCPU,
		CheckNameDisk,
		CheckNamePorts,
		CheckNameJava,
	}
}

// PrecheckItem represents a single precheck result item
// PrecheckItem 表示单个预检查结果项
type PrecheckItem struct {
	// Name is the name of the check
	// Name 是检查的名称
	Name CheckName `json:"name"`

	// Status is the check status (passed/failed/warning)
	// Status 是检查状态（通过/失败/警告）
	Status CheckStatus `json:"status"`

	// Message is a human-readable description of the result
	// Message 是结果的人类可读描述
	Message string `json:"message"`

	// Details contains additional information about the check
	// Details 包含检查的附加信息
	Details map[string]interface{} `json:"details,omitempty"`
}

// PrecheckResult contains all precheck results
// PrecheckResult 包含所有预检查结果
type PrecheckResult struct {
	// Items is the list of precheck items
	// Items 是预检查项列表
	Items []PrecheckItem `json:"items"`

	// OverallStatus is the overall status (failed if any check failed)
	// OverallStatus 是总体状态（如果任何检查失败则为失败）
	OverallStatus CheckStatus `json:"overall_status"`

	// Summary is a brief summary of the precheck results
	// Summary 是预检查结果的简要摘要
	Summary string `json:"summary"`
}

// PrecheckParams contains parameters for precheck execution
// PrecheckParams 包含预检查执行的参数
type PrecheckParams struct {
	// MinMemoryMB is the minimum required memory in MB
	// MinMemoryMB 是最小所需内存（MB）
	MinMemoryMB int64 `json:"min_memory_mb"`

	// MinCPUCores is the minimum required CPU cores
	// MinCPUCores 是最小所需 CPU 核心数
	MinCPUCores int `json:"min_cpu_cores"`

	// MinDiskSpaceMB is the minimum required disk space in MB
	// MinDiskSpaceMB 是最小所需磁盘空间（MB）
	MinDiskSpaceMB int64 `json:"min_disk_space_mb"`

	// InstallDir is the installation directory to check disk space
	// InstallDir 是用于检查磁盘空间的安装目录
	InstallDir string `json:"install_dir"`

	// Ports is the list of ports to check availability
	// Ports 是要检查可用性的端口列表
	Ports []int `json:"ports"`

	// Architecture is the CPU architecture (amd64, arm64) for Java download info
	// Architecture 是 CPU 架构（amd64、arm64），用于 Java 下载信息
	Architecture string `json:"architecture"`
}

// DefaultPrecheckParams returns default precheck parameters
// DefaultPrecheckParams 返回默认预检查参数
func DefaultPrecheckParams() *PrecheckParams {
	return &PrecheckParams{
		MinMemoryMB:    2048, // 2GB minimum / 最小 2GB
		MinCPUCores:    2,    // 2 cores minimum / 最小 2 核
		MinDiskSpaceMB: 5120, // 5GB minimum / 最小 5GB
		InstallDir:     "/opt/seatunnel",
		Ports:          []int{5801, 5802, 8080}, // Default SeaTunnel ports / 默认 SeaTunnel 端口
		Architecture:   "amd64",                 // Default architecture / 默认架构
	}
}

// Prechecker performs environment prechecks before SeaTunnel installation
// Prechecker 在 SeaTunnel 安装前执行环境预检查
type Prechecker struct {
	// params contains the precheck parameters
	// params 包含预检查参数
	params *PrecheckParams

	// systemInfoProvider provides system information (for testing)
	// systemInfoProvider 提供系统信息（用于测试）
	systemInfoProvider SystemInfoProvider
}

// SystemInfoProvider is an interface for getting system information
// SystemInfoProvider 是获取系统信息的接口
type SystemInfoProvider interface {
	// GetAvailableMemoryMB returns available memory in MB
	// GetAvailableMemoryMB 返回可用内存（MB）
	GetAvailableMemoryMB() (int64, error)

	// GetCPUCores returns the number of CPU cores
	// GetCPUCores 返回 CPU 核心数
	GetCPUCores() int

	// GetAvailableDiskSpaceMB returns available disk space in MB for the given path
	// GetAvailableDiskSpaceMB 返回给定路径的可用磁盘空间（MB）
	GetAvailableDiskSpaceMB(path string) (int64, error)

	// IsPortAvailable checks if a port is available
	// IsPortAvailable 检查端口是否可用
	IsPortAvailable(port int) bool

	// GetJavaVersion returns the installed Java version (major version number)
	// GetJavaVersion 返回已安装的 Java 版本（主版本号）
	GetJavaVersion() (int, string, error)
}

// DefaultSystemInfoProvider is the default implementation of SystemInfoProvider
// DefaultSystemInfoProvider 是 SystemInfoProvider 的默认实现
type DefaultSystemInfoProvider struct{}

// NewPrechecker creates a new Prechecker instance
// NewPrechecker 创建一个新的 Prechecker 实例
func NewPrechecker(params *PrecheckParams) *Prechecker {
	if params == nil {
		params = DefaultPrecheckParams()
	}
	return &Prechecker{
		params:             params,
		systemInfoProvider: &DefaultSystemInfoProvider{},
	}
}

// NewPrecheckerWithProvider creates a new Prechecker with a custom SystemInfoProvider
// NewPrecheckerWithProvider 使用自定义 SystemInfoProvider 创建新的 Prechecker
func NewPrecheckerWithProvider(params *PrecheckParams, provider SystemInfoProvider) *Prechecker {
	if params == nil {
		params = DefaultPrecheckParams()
	}
	return &Prechecker{
		params:             params,
		systemInfoProvider: provider,
	}
}

// RunAll executes all prechecks and returns the results
// RunAll 执行所有预检查并返回结果
func (p *Prechecker) RunAll(ctx context.Context) (*PrecheckResult, error) {
	result := &PrecheckResult{
		Items:         make([]PrecheckItem, 0, 5),
		OverallStatus: CheckStatusPassed,
	}

	// Run all checks / 运行所有检查
	checks := []func(context.Context) PrecheckItem{
		p.CheckMemory,
		p.CheckCPU,
		p.CheckDisk,
		p.CheckPorts,
		p.CheckJava,
	}

	passedCount := 0
	failedCount := 0
	warningCount := 0

	for _, check := range checks {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			item := check(ctx)
			result.Items = append(result.Items, item)

			switch item.Status {
			case CheckStatusPassed:
				passedCount++
			case CheckStatusFailed:
				failedCount++
				result.OverallStatus = CheckStatusFailed
			case CheckStatusWarning:
				warningCount++
				if result.OverallStatus == CheckStatusPassed {
					result.OverallStatus = CheckStatusWarning
				}
			}
		}
	}

	// Generate summary / 生成摘要
	result.Summary = fmt.Sprintf(
		"Precheck completed: %d passed, %d failed, %d warnings / 预检查完成：%d 通过，%d 失败，%d 警告",
		passedCount, failedCount, warningCount,
		passedCount, failedCount, warningCount,
	)

	return result, nil
}

// CheckMemory checks if available memory meets the minimum requirement
// CheckMemory 检查可用内存是否满足最低要求
func (p *Prechecker) CheckMemory(ctx context.Context) PrecheckItem {
	item := PrecheckItem{
		Name:    CheckNameMemory,
		Details: make(map[string]interface{}),
	}

	availableMB, err := p.systemInfoProvider.GetAvailableMemoryMB()
	if err != nil {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf("Failed to get memory info: %v / 获取内存信息失败：%v", err, err)
		return item
	}

	item.Details["available_mb"] = availableMB
	item.Details["required_mb"] = p.params.MinMemoryMB

	if availableMB >= p.params.MinMemoryMB {
		item.Status = CheckStatusPassed
		item.Message = fmt.Sprintf(
			"Available memory %d MB >= required %d MB / 可用内存 %d MB >= 所需 %d MB",
			availableMB, p.params.MinMemoryMB, availableMB, p.params.MinMemoryMB,
		)
	} else {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf(
			"Available memory %d MB < required %d MB / 可用内存 %d MB < 所需 %d MB",
			availableMB, p.params.MinMemoryMB, availableMB, p.params.MinMemoryMB,
		)
	}

	return item
}

// CheckCPU checks if CPU cores meet the minimum requirement
// CheckCPU 检查 CPU 核心数是否满足最低要求
func (p *Prechecker) CheckCPU(ctx context.Context) PrecheckItem {
	item := PrecheckItem{
		Name:    CheckNameCPU,
		Details: make(map[string]interface{}),
	}

	cpuCores := p.systemInfoProvider.GetCPUCores()
	item.Details["available_cores"] = cpuCores
	item.Details["required_cores"] = p.params.MinCPUCores

	if cpuCores >= p.params.MinCPUCores {
		item.Status = CheckStatusPassed
		item.Message = fmt.Sprintf(
			"CPU cores %d >= required %d / CPU 核心数 %d >= 所需 %d",
			cpuCores, p.params.MinCPUCores, cpuCores, p.params.MinCPUCores,
		)
	} else {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf(
			"CPU cores %d < required %d / CPU 核心数 %d < 所需 %d",
			cpuCores, p.params.MinCPUCores, cpuCores, p.params.MinCPUCores,
		)
	}

	return item
}

// CheckDisk checks if available disk space meets the minimum requirement
// CheckDisk 检查可用磁盘空间是否满足最低要求
func (p *Prechecker) CheckDisk(ctx context.Context) PrecheckItem {
	item := PrecheckItem{
		Name:    CheckNameDisk,
		Details: make(map[string]interface{}),
	}

	item.Details["install_dir"] = p.params.InstallDir
	item.Details["required_mb"] = p.params.MinDiskSpaceMB

	availableMB, err := p.systemInfoProvider.GetAvailableDiskSpaceMB(p.params.InstallDir)
	if err != nil {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf("Failed to get disk info for %s: %v / 获取 %s 磁盘信息失败：%v",
			p.params.InstallDir, err, p.params.InstallDir, err)
		return item
	}

	item.Details["available_mb"] = availableMB

	if availableMB >= p.params.MinDiskSpaceMB {
		item.Status = CheckStatusPassed
		item.Message = fmt.Sprintf(
			"Available disk space %d MB >= required %d MB / 可用磁盘空间 %d MB >= 所需 %d MB",
			availableMB, p.params.MinDiskSpaceMB, availableMB, p.params.MinDiskSpaceMB,
		)
	} else {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf(
			"Available disk space %d MB < required %d MB / 可用磁盘空间 %d MB < 所需 %d MB",
			availableMB, p.params.MinDiskSpaceMB, availableMB, p.params.MinDiskSpaceMB,
		)
	}

	return item
}

// CheckPorts checks if required ports are available
// CheckPorts 检查所需端口是否可用
func (p *Prechecker) CheckPorts(ctx context.Context) PrecheckItem {
	item := PrecheckItem{
		Name:    CheckNamePorts,
		Details: make(map[string]interface{}),
	}

	item.Details["ports_to_check"] = p.params.Ports

	if len(p.params.Ports) == 0 {
		item.Status = CheckStatusPassed
		item.Message = "No ports to check / 无需检查端口"
		return item
	}

	unavailablePorts := make([]int, 0)
	availablePorts := make([]int, 0)

	for _, port := range p.params.Ports {
		if p.systemInfoProvider.IsPortAvailable(port) {
			availablePorts = append(availablePorts, port)
		} else {
			unavailablePorts = append(unavailablePorts, port)
		}
	}

	item.Details["available_ports"] = availablePorts
	item.Details["unavailable_ports"] = unavailablePorts

	if len(unavailablePorts) == 0 {
		item.Status = CheckStatusPassed
		item.Message = fmt.Sprintf(
			"All ports are available: %v / 所有端口可用：%v",
			p.params.Ports, p.params.Ports,
		)
	} else {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf(
			"Ports in use: %v / 端口被占用：%v",
			unavailablePorts, unavailablePorts,
		)
	}

	return item
}

// Java version constants / Java 版本常量
const (
	// JavaMinVersion is the minimum supported Java version
	// JavaMinVersion 是最低支持的 Java 版本
	JavaMinVersion = 8

	// JavaMaxRecommendedVersion is the maximum recommended Java version
	// JavaMaxRecommendedVersion 是最高推荐的 Java 版本
	JavaMaxRecommendedVersion = 11
)

// JavaDownloadInfo contains download information for Java
// JavaDownloadInfo 包含 Java 下载信息
type JavaDownloadInfo struct {
	Version     int    `json:"version"`
	PackageName string `json:"package_name"`
	DownloadURL string `json:"download_url"`
	MirrorURL   string `json:"mirror_url"`
	InstallDir  string `json:"install_dir"`
}

// GetJavaDownloadInfo returns download information for the specified Java version and architecture
// GetJavaDownloadInfo 返回指定 Java 版本和架构的下载信息
func GetJavaDownloadInfo(version int, arch string) *JavaDownloadInfo {
	// Normalize architecture / 规范化架构
	archSuffix := "x64"
	if arch == "arm64" || arch == "aarch64" {
		archSuffix = "aarch64"
	}

	switch version {
	case 8:
		return &JavaDownloadInfo{
			Version:     8,
			PackageName: fmt.Sprintf("jdk-8u202-linux-%s.tar.gz", archSuffix),
			DownloadURL: fmt.Sprintf("https://repo.huaweicloud.com/java/jdk/8u202-b08/jdk-8u202-linux-%s.tar.gz", archSuffix),
			MirrorURL:   fmt.Sprintf("https://mirrors.tuna.tsinghua.edu.cn/Adoptium/8/jdk/%s/linux/OpenJDK8U-jdk_%s_linux_hotspot_8u432b06.tar.gz", archSuffix, archSuffix),
			InstallDir:  "jdk1.8.0_202",
		}
	case 11:
		return &JavaDownloadInfo{
			Version:     11,
			PackageName: fmt.Sprintf("jdk-11.0.2_linux-%s_bin.tar.gz", archSuffix),
			DownloadURL: fmt.Sprintf("https://repo.huaweicloud.com/java/jdk/11.0.2+9/jdk-11.0.2_linux-%s_bin.tar.gz", archSuffix),
			MirrorURL:   fmt.Sprintf("https://mirrors.tuna.tsinghua.edu.cn/Adoptium/11/jdk/%s/linux/OpenJDK11U-jdk_%s_linux_hotspot_11.0.25_9.tar.gz", archSuffix, archSuffix),
			InstallDir:  "jdk-11.0.2",
		}
	default:
		return nil
	}
}

// CheckJava checks if Java is installed and meets the version requirements
// CheckJava 检查 Java 是否已安装并满足版本要求
// Supported versions: 8, 11 (recommended)
// Versions > 11 will show a warning as they may have compatibility issues
// 支持的版本：8、11（推荐）
// 版本 > 11 将显示警告，因为可能存在兼容性问题
func (p *Prechecker) CheckJava(ctx context.Context) PrecheckItem {
	return p.checkJavaWith(p.systemInfoProvider.GetJavaVersion)
}

// CheckJavaAt 用指定的 java 可执行文件（节点配置的 <java_home>/bin/java）检查版本，规则与 CheckJava 相同。
// CheckJavaAt checks the version of a specific java executable (the node's configured <java_home>/bin/java) with
// the same rules as CheckJava.
func (p *Prechecker) CheckJavaAt(ctx context.Context, javaBin string) PrecheckItem {
	return p.checkJavaWith(func() (int, string, error) {
		return JavaVersionFromBinary(ctx, javaBin)
	})
}

// JavaVersionFromBinary 执行 `<javaBin> -version` 并解析主版本号。
// JavaVersionFromBinary runs `<javaBin> -version` and parses the major version.
func JavaVersionFromBinary(ctx context.Context, javaBin string) (int, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	output, err := exec.CommandContext(ctx, javaBin, "-version").CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("%s -version failed: %w", javaBin, err)
	}
	return parseJavaVersion(string(output))
}

// checkJavaWith 按统一的版本规则生成 Java 检查项。
// checkJavaWith builds the Java check item with the shared version rules.
func (p *Prechecker) checkJavaWith(getVersion func() (int, string, error)) PrecheckItem {
	item := PrecheckItem{
		Name:    CheckNameJava,
		Details: make(map[string]interface{}),
	}

	item.Details["min_version"] = JavaMinVersion
	item.Details["max_recommended_version"] = JavaMaxRecommendedVersion
	item.Details["recommended_versions"] = []int{8, 11}

	version, versionStr, err := getVersion()
	if err != nil {
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf(
			"Java not found: %v. Recommended versions: 8 or 11 / Java 未找到：%v。推荐版本：8 或 11",
			err, err,
		)
		// Add download information / 添加下载信息
		item.Details["download_info"] = map[string]interface{}{
			"huaweicloud_java8":  "https://repo.huaweicloud.com/java/jdk/8u202-b08/",
			"huaweicloud_java11": "https://repo.huaweicloud.com/java/jdk/11.0.2+9/",
			"tsinghua_java8":     "https://mirrors.tuna.tsinghua.edu.cn/Adoptium/8/jdk/",
			"tsinghua_java11":    "https://mirrors.tuna.tsinghua.edu.cn/Adoptium/11/jdk/",
		}
		item.Details["install_command_linux"] = `# Install Java 8 / 安装 Java 8
apt-get install openjdk-8-jdk  # Debian/Ubuntu
yum install java-1.8.0-openjdk-devel  # CentOS/RHEL

# Install Java 11 (Recommended) / 安装 Java 11（推荐）
apt-get install openjdk-11-jdk  # Debian/Ubuntu
yum install java-11-openjdk-devel  # CentOS/RHEL`
		return item
	}

	item.Details["installed_version"] = version
	item.Details["version_string"] = versionStr

	// Check version requirements / 检查版本要求
	if version < JavaMinVersion {
		// Version too low / 版本过低
		item.Status = CheckStatusFailed
		item.Message = fmt.Sprintf(
			"Java version %d (%s) < minimum required %d. Please install Java 8 or 11. / Java 版本 %d (%s) < 最低要求 %d。请安装 Java 8 或 11。",
			version, versionStr, JavaMinVersion,
			version, versionStr, JavaMinVersion,
		)
		item.Details["download_info"] = map[string]interface{}{
			"huaweicloud_java8":  "https://repo.huaweicloud.com/java/jdk/8u202-b08/",
			"huaweicloud_java11": "https://repo.huaweicloud.com/java/jdk/11.0.2+9/",
		}
	} else if version > JavaMaxRecommendedVersion {
		// Version higher than recommended - warning / 版本高于推荐 - 警告
		item.Status = CheckStatusWarning
		item.Message = fmt.Sprintf(
			"Java version %d (%s) > recommended max %d. May have compatibility issues. Recommended: Java 8 or 11. / Java 版本 %d (%s) > 推荐最高版本 %d。可能存在兼容性问题。推荐：Java 8 或 11。",
			version, versionStr, JavaMaxRecommendedVersion,
			version, versionStr, JavaMaxRecommendedVersion,
		)
		item.Details["warning"] = "Higher Java versions may cause compatibility issues with SeaTunnel / 较高的 Java 版本可能导致与 SeaTunnel 的兼容性问题"
	} else {
		// Version is within recommended range (8-11) / 版本在推荐范围内（8-11）
		item.Status = CheckStatusPassed
		item.Message = fmt.Sprintf(
			"Java version %d (%s) is supported (recommended: 8 or 11) / Java 版本 %d (%s) 受支持（推荐：8 或 11）",
			version, versionStr, version, versionStr,
		)
	}

	return item
}

// ToJSON converts the precheck result to JSON string
// ToJSON 将预检查结果转换为 JSON 字符串
func (r *PrecheckResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// HasCheck returns true if the result contains a check with the given name
// HasCheck 如果结果包含给定名称的检查则返回 true
func (r *PrecheckResult) HasCheck(name CheckName) bool {
	for _, item := range r.Items {
		if item.Name == name {
			return true
		}
	}
	return false
}

// GetCheck returns the check item with the given name, or nil if not found
// GetCheck 返回给定名称的检查项，如果未找到则返回 nil
func (r *PrecheckResult) GetCheck(name CheckName) *PrecheckItem {
	for i := range r.Items {
		if r.Items[i].Name == name {
			return &r.Items[i]
		}
	}
	return nil
}

// IsComplete returns true if all expected checks are present
// IsComplete 如果所有预期检查都存在则返回 true
func (r *PrecheckResult) IsComplete() bool {
	expectedChecks := AllCheckNames()
	for _, name := range expectedChecks {
		if !r.HasCheck(name) {
			return false
		}
	}
	return true
}

// ============================================================================
// DefaultSystemInfoProvider implementations
// DefaultSystemInfoProvider 实现
// ============================================================================

// GetAvailableMemoryMB returns available memory in MB
// GetAvailableMemoryMB 返回可用内存（MB）
func (d *DefaultSystemInfoProvider) GetAvailableMemoryMB() (int64, error) {
	switch runtime.GOOS {
	case "linux":
		return d.getLinuxAvailableMemory()
	case "darwin":
		return d.getDarwinAvailableMemory()
	case "windows":
		return d.getWindowsAvailableMemory()
	default:
		return 0, fmt.Errorf("unsupported OS: %s / 不支持的操作系统：%s", runtime.GOOS, runtime.GOOS)
	}
}

// getLinuxAvailableMemory gets available memory on Linux
// getLinuxAvailableMemory 获取 Linux 上的可用内存
func (d *DefaultSystemInfoProvider) getLinuxAvailableMemory() (int64, error) {
	// Read from /proc/meminfo / 从 /proc/meminfo 读取
	cmd := exec.Command("cat", "/proc/meminfo")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to read /proc/meminfo: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemAvailable:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return 0, fmt.Errorf("failed to parse MemAvailable: %w", err)
				}
				return kb / 1024, nil // Convert KB to MB / 将 KB 转换为 MB
			}
		}
	}

	// Fallback to MemFree + Buffers + Cached / 回退到 MemFree + Buffers + Cached
	var memFree, buffers, cached int64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseInt(fields[1], 10, 64)
		switch {
		case strings.HasPrefix(line, "MemFree:"):
			memFree = value
		case strings.HasPrefix(line, "Buffers:"):
			buffers = value
		case strings.HasPrefix(line, "Cached:"):
			cached = value
		}
	}

	return (memFree + buffers + cached) / 1024, nil
}

// getDarwinAvailableMemory gets available memory on macOS
// getDarwinAvailableMemory 获取 macOS 上的可用内存
func (d *DefaultSystemInfoProvider) getDarwinAvailableMemory() (int64, error) {
	cmd := exec.Command("vm_stat")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run vm_stat: %w", err)
	}

	// Parse vm_stat output / 解析 vm_stat 输出
	lines := strings.Split(string(output), "\n")
	pageSize := int64(4096) // Default page size / 默认页面大小
	var freePages, inactivePages int64

	for _, line := range lines {
		if strings.Contains(line, "page size of") {
			re := regexp.MustCompile(`page size of (\d+) bytes`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				pageSize, _ = strconv.ParseInt(matches[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "Pages free:") {
			freePages = parseVMStatValue(line)
		} else if strings.HasPrefix(line, "Pages inactive:") {
			inactivePages = parseVMStatValue(line)
		}
	}

	availableBytes := (freePages + inactivePages) * pageSize
	return availableBytes / (1024 * 1024), nil
}

// parseVMStatValue parses a value from vm_stat output
// parseVMStatValue 从 vm_stat 输出解析值
func parseVMStatValue(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	valueStr := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
	value, _ := strconv.ParseInt(valueStr, 10, 64)
	return value
}

// getWindowsAvailableMemory gets available memory on Windows
// getWindowsAvailableMemory 获取 Windows 上的可用内存
func (d *DefaultSystemInfoProvider) getWindowsAvailableMemory() (int64, error) {
	cmd := exec.Command("wmic", "OS", "get", "FreePhysicalMemory", "/Value")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run wmic: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "FreePhysicalMemory=") {
			valueStr := strings.TrimPrefix(line, "FreePhysicalMemory=")
			valueStr = strings.TrimSpace(valueStr)
			kb, err := strconv.ParseInt(valueStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse FreePhysicalMemory: %w", err)
			}
			return kb / 1024, nil // Convert KB to MB / 将 KB 转换为 MB
		}
	}

	return 0, fmt.Errorf("FreePhysicalMemory not found in wmic output")
}

// GetCPUCores returns the number of CPU cores
// GetCPUCores 返回 CPU 核心数
func (d *DefaultSystemInfoProvider) GetCPUCores() int {
	return runtime.NumCPU()
}

// GetAvailableDiskSpaceMB returns available disk space in MB for the given path
// GetAvailableDiskSpaceMB 返回给定路径的可用磁盘空间（MB）
func (d *DefaultSystemInfoProvider) GetAvailableDiskSpaceMB(path string) (int64, error) {
	// Use syscall.Statfs for Unix-like systems / 对类 Unix 系统使用 syscall.Statfs
	switch runtime.GOOS {
	case "linux", "darwin":
		return d.getUnixDiskSpace(path)
	case "windows":
		return d.getWindowsDiskSpace(path)
	default:
		return 0, fmt.Errorf("unsupported OS: %s / 不支持的操作系统：%s", runtime.GOOS, runtime.GOOS)
	}
}

// getUnixDiskSpace gets disk space on Unix-like systems using df command
// getUnixDiskSpace 使用 df 命令获取类 Unix 系统上的磁盘空间
func (d *DefaultSystemInfoProvider) getUnixDiskSpace(path string) (int64, error) {
	// Use df command to get disk space / 使用 df 命令获取磁盘空间
	// -k outputs in 1K blocks / -k 以 1K 块为单位输出
	cmd := exec.Command("df", "-k", path)
	output, err := cmd.Output()
	if err != nil {
		// Try parent directory if path doesn't exist / 如果路径不存在，尝试父目录
		parentPath := getParentPath(path)
		if parentPath != path {
			cmd = exec.Command("df", "-k", parentPath)
			output, err = cmd.Output()
			if err != nil {
				return 0, fmt.Errorf("failed to get disk stats for %s: %w", path, err)
			}
		} else {
			return 0, fmt.Errorf("failed to get disk stats for %s: %w", path, err)
		}
	}

	// Parse df output / 解析 df 输出
	// Format: Filesystem 1K-blocks Used Available Use% Mounted on
	// 格式：文件系统 1K-块 已用 可用 使用% 挂载点
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected df output format")
	}

	// Parse the second line (first line is header) / 解析第二行（第一行是标题）
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return 0, fmt.Errorf("unexpected df output format: not enough fields")
	}

	// Available is the 4th field (index 3) / 可用空间是第 4 个字段（索引 3）
	availableKB, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse available space: %w", err)
	}

	return availableKB / 1024, nil // Convert KB to MB / 将 KB 转换为 MB
}

// getWindowsDiskSpace gets disk space on Windows
// getWindowsDiskSpace 获取 Windows 上的磁盘空间
func (d *DefaultSystemInfoProvider) getWindowsDiskSpace(path string) (int64, error) {
	// Extract drive letter / 提取驱动器字母
	drive := path
	if len(path) >= 2 && path[1] == ':' {
		drive = path[:2]
	} else {
		drive = "C:"
	}

	cmd := exec.Command("wmic", "logicaldisk", "where", fmt.Sprintf("DeviceID='%s'", drive), "get", "FreeSpace", "/Value")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to run wmic: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "FreeSpace=") {
			valueStr := strings.TrimPrefix(line, "FreeSpace=")
			valueStr = strings.TrimSpace(valueStr)
			bytes, err := strconv.ParseInt(valueStr, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("failed to parse FreeSpace: %w", err)
			}
			return bytes / (1024 * 1024), nil
		}
	}

	return 0, fmt.Errorf("FreeSpace not found for drive %s", drive)
}

// getParentPath returns the parent directory path
// getParentPath 返回父目录路径
func getParentPath(path string) string {
	// Simple implementation - find last separator / 简单实现 - 找到最后一个分隔符
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "/"
}

// IsPortAvailable checks if a port is available
// IsPortAvailable 检查端口是否可用
func (d *DefaultSystemInfoProvider) IsPortAvailable(port int) bool {
	// Try to listen on the port / 尝试监听端口
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// GetJavaVersion returns the installed Java version (major version number)
// GetJavaVersion 返回已安装的 Java 版本（主版本号）
func (d *DefaultSystemInfoProvider) GetJavaVersion() (int, string, error) {
	// Try java -version / 尝试 java -version
	cmd := exec.Command("java", "-version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("java command failed: %w", err)
	}

	versionStr := string(output)
	return parseJavaVersion(versionStr)
}

// parseJavaVersion parses Java version from java -version output
// parseJavaVersion 从 java -version 输出解析 Java 版本
func parseJavaVersion(output string) (int, string, error) {
	// Java version output formats:
	// Java 版本输出格式：
	// - java version "1.8.0_xxx" (Java 8)
	// - java version "11.0.x" (Java 11+)
	// - openjdk version "17.0.x" (OpenJDK)

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.ToLower(line)
		if strings.Contains(line, "version") {
			// Extract version string / 提取版本字符串
			re := regexp.MustCompile(`"([^"]+)"`)
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				versionStr := matches[1]
				majorVersion := extractMajorVersion(versionStr)
				return majorVersion, versionStr, nil
			}
		}
	}

	return 0, "", fmt.Errorf("could not parse Java version from output")
}

// extractMajorVersion extracts the major version number from a version string
// extractMajorVersion 从版本字符串中提取主版本号
func extractMajorVersion(versionStr string) int {
	// Handle "1.8.0_xxx" format (Java 8 and earlier)
	// 处理 "1.8.0_xxx" 格式（Java 8 及更早版本）
	if strings.HasPrefix(versionStr, "1.") {
		parts := strings.Split(versionStr, ".")
		if len(parts) >= 2 {
			version, _ := strconv.Atoi(parts[1])
			return version
		}
	}

	// Handle "11.0.x", "17.0.x" format (Java 9+)
	// 处理 "11.0.x"、"17.0.x" 格式（Java 9+）
	parts := strings.Split(versionStr, ".")
	if len(parts) >= 1 {
		// Remove any non-numeric suffix / 移除任何非数字后缀
		numStr := strings.TrimFunc(parts[0], func(r rune) bool {
			return r < '0' || r > '9'
		})
		version, _ := strconv.Atoi(numStr)
		return version
	}

	return 0
}
