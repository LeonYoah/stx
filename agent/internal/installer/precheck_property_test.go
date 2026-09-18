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

package installer

import (
	"context"
	"fmt"
	"testing"

	"pgregory.net/rapid"
)

// **Feature: stx-agent, Property 7: Precheck Result Completeness**
// **Validates: Requirements 4.1, 4.2, 4.3, 4.4, 4.5, 4.6**
//
// Property: For any precheck execution, the returned result SHALL contain
// check items for all configured checks (memory, CPU, disk, ports, Java),
// each with a status (passed/failed/warning) and descriptive message.
// 属性：对于任何预检查执行，返回的结果应该包含所有配置检查的检查项
// （内存、CPU、磁盘、端口、Java），每个都有状态（通过/失败/警告）和描述性消息。
func TestProperty_PrecheckResultCompleteness(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate random precheck parameters / 生成随机预检查参数
		params := generatePrecheckParams(rt)

		// Generate a mock system info provider with random values
		// 生成具有随机值的模拟系统信息提供者
		mockProvider := generateMockSystemInfoProvider(rt)

		// Create prechecker with mock provider / 使用模拟提供者创建预检查器
		prechecker := NewPrecheckerWithProvider(params, mockProvider)

		// Run all prechecks / 运行所有预检查
		ctx := context.Background()
		result, err := prechecker.RunAll(ctx)

		// Verify no error occurred / 验证没有发生错误
		if err != nil {
			rt.Fatalf("Precheck execution failed: %v", err)
		}

		// Verify result is not nil / 验证结果不为空
		if result == nil {
			rt.Fatal("Precheck result is nil")
		}

		// Property 1: Result must contain all expected check items
		// 属性 1：结果必须包含所有预期的检查项
		expectedChecks := AllCheckNames()
		if len(result.Items) != len(expectedChecks) {
			rt.Fatalf("Expected %d check items, got %d", len(expectedChecks), len(result.Items))
		}

		// Property 2: Each expected check must be present
		// 属性 2：每个预期检查必须存在
		for _, checkName := range expectedChecks {
			if !result.HasCheck(checkName) {
				rt.Fatalf("Missing check: %s", checkName)
			}
		}

		// Property 3: Each check item must have valid status and message
		// 属性 3：每个检查项必须有有效的状态和消息
		for _, item := range result.Items {
			// Verify status is valid / 验证状态有效
			if item.Status != CheckStatusPassed &&
				item.Status != CheckStatusFailed &&
				item.Status != CheckStatusWarning {
				rt.Fatalf("Check %s has invalid status: %s", item.Name, item.Status)
			}

			// Verify message is not empty / 验证消息不为空
			if item.Message == "" {
				rt.Fatalf("Check %s has empty message", item.Name)
			}

			// Verify name is valid / 验证名称有效
			if item.Name == "" {
				rt.Fatal("Check item has empty name")
			}
		}

		// Property 4: Overall status must be consistent with individual checks
		// 属性 4：总体状态必须与各个检查一致
		hasFailure := false
		hasWarning := false
		for _, item := range result.Items {
			if item.Status == CheckStatusFailed {
				hasFailure = true
			}
			if item.Status == CheckStatusWarning {
				hasWarning = true
			}
		}

		if hasFailure && result.OverallStatus != CheckStatusFailed {
			rt.Fatalf("Overall status should be 'failed' when any check fails, got: %s", result.OverallStatus)
		}
		if !hasFailure && hasWarning && result.OverallStatus != CheckStatusWarning {
			rt.Fatalf("Overall status should be 'warning' when no failures but has warnings, got: %s", result.OverallStatus)
		}
		if !hasFailure && !hasWarning && result.OverallStatus != CheckStatusPassed {
			rt.Fatalf("Overall status should be 'passed' when all checks pass, got: %s", result.OverallStatus)
		}

		// Property 5: Summary must not be empty / 属性 5：摘要不能为空
		if result.Summary == "" {
			rt.Fatal("Precheck result summary is empty")
		}

		// Property 6: IsComplete() must return true / 属性 6：IsComplete() 必须返回 true
		if !result.IsComplete() {
			rt.Fatal("IsComplete() returned false for a complete precheck result")
		}
	})
}

// generatePrecheckParams generates random precheck parameters for testing
// generatePrecheckParams 为测试生成随机预检查参数
func generatePrecheckParams(rt *rapid.T) *PrecheckParams {
	// Generate reasonable parameter ranges / 生成合理的参数范围
	minMemoryMB := int64(rapid.IntRange(512, 8192).Draw(rt, "minMemoryMB"))
	minCPUCores := rapid.IntRange(1, 16).Draw(rt, "minCPUCores")
	minDiskSpaceMB := int64(rapid.IntRange(1024, 102400).Draw(rt, "minDiskSpaceMB"))

	// Generate install directory / 生成安装目录
	installDir := "/" + rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "installDir")

	// Generate ports to check (0-5 ports) / 生成要检查的端口（0-5 个端口）
	numPorts := rapid.IntRange(0, 5).Draw(rt, "numPorts")
	ports := make([]int, numPorts)
	for i := 0; i < numPorts; i++ {
		ports[i] = rapid.IntRange(1024, 65535).Draw(rt, "port")
	}

	// Generate architecture / 生成架构
	arch := rapid.SampledFrom([]string{"amd64", "arm64"}).Draw(rt, "arch")

	return &PrecheckParams{
		MinMemoryMB:    minMemoryMB,
		MinCPUCores:    minCPUCores,
		MinDiskSpaceMB: minDiskSpaceMB,
		InstallDir:     installDir,
		Ports:          ports,
		Architecture:   arch,
	}
}

// MockSystemInfoProvider is a mock implementation for testing
// MockSystemInfoProvider 是用于测试的模拟实现
type MockSystemInfoProvider struct {
	AvailableMemoryMB    int64
	CPUCores             int
	AvailableDiskSpaceMB int64
	AvailablePorts       map[int]bool
	JavaVersion          int
	JavaVersionString    string
	JavaError            error
	MemoryError          error
	DiskError            error
}

// GetAvailableMemoryMB returns the mock available memory
// GetAvailableMemoryMB 返回模拟的可用内存
func (m *MockSystemInfoProvider) GetAvailableMemoryMB() (int64, error) {
	if m.MemoryError != nil {
		return 0, m.MemoryError
	}
	return m.AvailableMemoryMB, nil
}

// GetCPUCores returns the mock CPU cores
// GetCPUCores 返回模拟的 CPU 核心数
func (m *MockSystemInfoProvider) GetCPUCores() int {
	return m.CPUCores
}

// GetAvailableDiskSpaceMB returns the mock available disk space
// GetAvailableDiskSpaceMB 返回模拟的可用磁盘空间
func (m *MockSystemInfoProvider) GetAvailableDiskSpaceMB(path string) (int64, error) {
	if m.DiskError != nil {
		return 0, m.DiskError
	}
	return m.AvailableDiskSpaceMB, nil
}

// IsPortAvailable returns whether the mock port is available
// IsPortAvailable 返回模拟端口是否可用
func (m *MockSystemInfoProvider) IsPortAvailable(port int) bool {
	if m.AvailablePorts == nil {
		return true
	}
	available, exists := m.AvailablePorts[port]
	if !exists {
		return true // Default to available / 默认可用
	}
	return available
}

// GetJavaVersion returns the mock Java version
// GetJavaVersion 返回模拟的 Java 版本
func (m *MockSystemInfoProvider) GetJavaVersion() (int, string, error) {
	if m.JavaError != nil {
		return 0, "", m.JavaError
	}
	return m.JavaVersion, m.JavaVersionString, nil
}

// generateMockSystemInfoProvider generates a mock provider with random values
// generateMockSystemInfoProvider 生成具有随机值的模拟提供者
func generateMockSystemInfoProvider(rt *rapid.T) *MockSystemInfoProvider {
	// Generate random system values / 生成随机系统值
	availableMemoryMB := int64(rapid.IntRange(256, 32768).Draw(rt, "availableMemoryMB"))
	cpuCores := rapid.IntRange(1, 64).Draw(rt, "cpuCores")
	availableDiskSpaceMB := int64(rapid.IntRange(512, 1048576).Draw(rt, "availableDiskSpaceMB"))

	// Generate Java version (0 means not installed) / 生成 Java 版本（0 表示未安装）
	hasJava := rapid.Bool().Draw(rt, "hasJava")
	var javaVersion int
	var javaVersionString string
	var javaError error
	if hasJava {
		javaVersion = rapid.IntRange(8, 21).Draw(rt, "javaVersion")
		javaVersionString = rapid.SampledFrom([]string{
			"1.8.0_301",
			"11.0.12",
			"17.0.1",
			"21.0.1",
		}).Draw(rt, "javaVersionString")
	} else {
		javaError = fmt.Errorf("java command not found")
	}

	// Generate port availability / 生成端口可用性
	availablePorts := make(map[int]bool)
	numPortsToSet := rapid.IntRange(0, 10).Draw(rt, "numPortsToSet")
	for i := 0; i < numPortsToSet; i++ {
		port := rapid.IntRange(1024, 65535).Draw(rt, "portToSet")
		available := rapid.Bool().Draw(rt, "portAvailable")
		availablePorts[port] = available
	}

	return &MockSystemInfoProvider{
		AvailableMemoryMB:    availableMemoryMB,
		CPUCores:             cpuCores,
		AvailableDiskSpaceMB: availableDiskSpaceMB,
		AvailablePorts:       availablePorts,
		JavaVersion:          javaVersion,
		JavaVersionString:    javaVersionString,
		JavaError:            javaError,
	}
}
