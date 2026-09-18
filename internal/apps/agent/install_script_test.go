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

// Package agent provides Agent distribution and management for the STX Control Plane.
// agent 包提供 STX Control Plane 的 Agent 分发和管理功能。
package agent

import (
	"strings"
	"testing"

	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

// TestFormatInstallCommand verifies the one-click install command shape.
// TestFormatInstallCommand 验证一键安装命令形态。
func TestFormatInstallCommand(t *testing.T) {
	cmd := FormatInstallCommand("192.168.1.10:8000")
	if !strings.Contains(cmd, "http://192.168.1.10:8000/api/v1/agent/install.sh") {
		t.Fatalf("unexpected control plane URL in command: %s", cmd)
	}
	if !strings.Contains(cmd, "bash -s -- --install-dir=$HOME/.stx/agent/") {
		t.Fatalf("expected bash -s with default install-dir, got: %s", cmd)
	}
}

// TestNewInstallScriptGenerator tests the creation of InstallScriptGenerator.
// TestNewInstallScriptGenerator 测试 InstallScriptGenerator 的创建。
func TestNewInstallScriptGenerator(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *InstallScriptConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			cfg:     nil,
			wantErr: false,
		},
		{
			name: "custom config",
			cfg: &InstallScriptConfig{
				ControlPlaneAddr: "http://example.com:8080",
				GRPCAddr:         "example.com:50051",
			},
			wantErr: false,
		},
		{
			name:    "empty config uses defaults",
			cfg:     &InstallScriptConfig{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen, err := NewInstallScriptGenerator(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewInstallScriptGenerator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && gen == nil {
				t.Error("NewInstallScriptGenerator() returned nil generator")
			}
		})
	}
}

// TestInstallScriptGenerate tests the script generation.
// TestInstallScriptGenerate 测试脚本生成。
func TestInstallScriptGenerate(t *testing.T) {
	gen, err := NewInstallScriptGenerator(&InstallScriptConfig{
		ControlPlaneAddr: "http://test-server:8080",
		GRPCAddr:         "test-server:50051",
	})
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	script, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Verify script contains expected content
	// 验证脚本包含预期内容
	expectedContents := []string{
		"#!/bin/bash",
		"CONTROL_PLANE_ADDR=\"http://test-server:8080\"",
		"GRPC_ADDR=\"test-server:50051\"",
		"DEFAULT_INSTALL_DIR=\"$HOME/.stx/agent\"",
		"--install-dir",
		"detect_os()",
		"detect_arch()",
		"download_agent",
		"download_support_assets",
		"install_support_assets",
		"install_agent",
		"create_systemd_service",
		"create_launchd_service",
		"start_agent",
		"systemctl --user",
		"launchctl",
		"$HOME/.stx/agent",
		"CAPABILITY_PROXY_VERSION=\"" + seatunnelmeta.DefaultSTXJavaProxyVersion + "\"",
		"/api/v1/agent/assets/stx-java-proxy.jar?version=${CAPABILITY_PROXY_VERSION}",
		"/api/v1/agent/assets/stx-java-proxy.sh",
		"STX_JAVA_PROXY_HOME",
		"STX_JAVA_PROXY_SCRIPT",
		"stx-agent",
		seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion),
	}

	for _, expected := range expectedContents {
		if !strings.Contains(script, expected) {
			t.Errorf("Generated script missing expected content: %s", expected)
		}
	}
}

// TestInstallScriptGenerateWithData tests script generation with custom data.
// TestInstallScriptGenerateWithData 测试使用自定义数据生成脚本。
func TestInstallScriptGenerateWithData(t *testing.T) {
	gen, err := NewInstallScriptGenerator(nil)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	data := &InstallScriptData{
		ControlPlaneAddr: "http://custom-server:9090",
		GRPCAddr:         "custom-server:60000",
		InstallDir:       "/opt/custom/stx-agent",
		ConfigDir:        "/opt/custom/config",
		AgentBinary:      "custom-agent",
		ServiceName:      "custom-service",
		SupportDir:       "/opt/custom/support",
	}

	script, err := gen.GenerateWithData(data)
	if err != nil {
		t.Fatalf("GenerateWithData() error = %v", err)
	}

	// Verify custom values are in the script
	// 验证自定义值在脚本中
	expectedContents := []string{
		"http://custom-server:9090",
		"custom-server:60000",
		"DEFAULT_INSTALL_DIR=\"/opt/custom/stx-agent\"",
		"custom-agent",
		"custom-service",
	}

	for _, expected := range expectedContents {
		if !strings.Contains(script, expected) {
			t.Errorf("Generated script missing expected content: %s", expected)
		}
	}
}

// TestInstallScriptGenerateWithNilData tests error handling for nil data.
// TestInstallScriptGenerateWithNilData 测试 nil 数据的错误处理。
func TestInstallScriptGenerateWithNilData(t *testing.T) {
	gen, err := NewInstallScriptGenerator(nil)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	_, err = gen.GenerateWithData(nil)
	if err == nil {
		t.Error("GenerateWithData(nil) should return error")
	}
}

// TestIsPlatformSupported tests platform support checking.
// TestIsPlatformSupported 测试平台支持检查。
func TestIsPlatformSupported(t *testing.T) {
	tests := []struct {
		os   string
		arch string
		want bool
	}{
		{"linux", "amd64", true},
		{"linux", "arm64", true},
		{"darwin", "amd64", true},
		{"darwin", "arm64", true},
		{"LINUX", "AMD64", true}, // Case insensitive
		{"Linux", "Arm64", true}, // Case insensitive
		{"windows", "amd64", false},
		{"linux", "386", false},
		{"freebsd", "amd64", false},
	}

	for _, tt := range tests {
		t.Run(tt.os+"-"+tt.arch, func(t *testing.T) {
			if got := IsPlatformSupported(tt.os, tt.arch); got != tt.want {
				t.Errorf("IsPlatformSupported(%q, %q) = %v, want %v", tt.os, tt.arch, got, tt.want)
			}
		})
	}
}

// TestGetBinaryName tests binary name retrieval.
// TestGetBinaryName 测试二进制文件名称获取。
func TestGetBinaryName(t *testing.T) {
	tests := []struct {
		os       string
		arch     string
		wantName string
		wantOK   bool
	}{
		{"linux", "amd64", "stx-agent-linux-amd64", true},
		{"linux", "arm64", "stx-agent-linux-arm64", true},
		{"darwin", "amd64", "stx-agent-darwin-amd64", true},
		{"darwin", "arm64", "stx-agent-darwin-arm64", true},
		{"LINUX", "AMD64", "stx-agent-linux-amd64", true}, // Case insensitive
		{"windows", "amd64", "", false},
		{"linux", "386", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.os+"-"+tt.arch, func(t *testing.T) {
			gotName, gotOK := GetBinaryName(tt.os, tt.arch)
			if gotName != tt.wantName || gotOK != tt.wantOK {
				t.Errorf("GetBinaryName(%q, %q) = (%q, %v), want (%q, %v)",
					tt.os, tt.arch, gotName, gotOK, tt.wantName, tt.wantOK)
			}
		})
	}
}

// TestNormalizeArch tests architecture normalization.
// TestNormalizeArch 测试架构标准化。
func TestNormalizeArch(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"x86_64", "amd64"},
		{"amd64", "amd64"},
		{"AMD64", "amd64"},
		{"X86_64", "amd64"},
		{"aarch64", "arm64"},
		{"arm64", "arm64"},
		{"ARM64", "arm64"},
		{"AARCH64", "arm64"},
		{"386", "386"}, // Unknown, returned as-is (lowercase)
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeArch(tt.input); got != tt.want {
				t.Errorf("NormalizeArch(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizeOS tests OS normalization.
// TestNormalizeOS 测试操作系统标准化。
func TestNormalizeOS(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Linux", "linux"},
		{"LINUX", "linux"},
		{"linux", "linux"},
		{"Darwin", "darwin"},
		{"DARWIN", "darwin"},
		{"darwin", "darwin"},
		{"Windows", "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := NormalizeOS(tt.input); got != tt.want {
				t.Errorf("NormalizeOS(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestGetSupportedPlatforms tests retrieval of supported platforms.
// TestGetSupportedPlatforms 测试获取支持的平台。
func TestGetSupportedPlatforms(t *testing.T) {
	platforms := GetSupportedPlatforms()

	if len(platforms) != 4 {
		t.Errorf("Expected 4 supported platforms, got %d", len(platforms))
	}

	// Verify all expected platforms are present
	// 验证所有预期平台都存在
	expectedPlatforms := map[string]bool{
		"linux-amd64":  false,
		"linux-arm64":  false,
		"darwin-amd64": false,
		"darwin-arm64": false,
	}

	for _, p := range platforms {
		key := p.OS + "-" + p.Arch
		if _, ok := expectedPlatforms[key]; ok {
			expectedPlatforms[key] = true
		}
	}

	for key, found := range expectedPlatforms {
		if !found {
			t.Errorf("Expected platform %s not found", key)
		}
	}
}

// TestInstallScriptGenerate_TLSEnabled verifies CA download and Agent TLS config when CP TLS is on.
// TestInstallScriptGenerate_TLSEnabled 验证 Control Plane 开启 TLS 时脚本会下载 CA 并启用 Agent TLS。
func TestInstallScriptGenerate_TLSEnabled(t *testing.T) {
	gen, err := NewInstallScriptGenerator(&InstallScriptConfig{
		ControlPlaneAddr: "http://cp.example.com:8000",
		GRPCAddr:         "cp.example.com:9000",
		TLSEnabled:       true,
	})
	if err != nil {
		t.Fatalf("NewInstallScriptGenerator: %v", err)
	}

	script, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantSnippets := []string{
		`GRPC_TLS_ENABLED="true"`,
		"/api/v1/agent/ca.crt",
		"download_ca",
		`AGENT_CA_FILE="${CONFIG_DIR}/certs/ca.crt"`,
		"enabled: true",
		`ca_file: "${ca_file_value}"`,
	}
	for _, want := range wantSnippets {
		if !strings.Contains(script, want) {
			t.Errorf("TLS-enabled script missing %q", want)
		}
	}
}

// TestInstallScriptGenerate_TLSDisabled verifies CA download is skipped when CP TLS is off.
// TestInstallScriptGenerate_TLSDisabled 验证 Control Plane 未开 TLS 时脚本跳过 CA 下载。
func TestInstallScriptGenerate_TLSDisabled(t *testing.T) {
	gen, err := NewInstallScriptGenerator(&InstallScriptConfig{
		ControlPlaneAddr: "http://cp.example.com:8000",
		GRPCAddr:         "cp.example.com:9000",
		TLSEnabled:       false,
	})
	if err != nil {
		t.Fatalf("NewInstallScriptGenerator: %v", err)
	}

	script, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !strings.Contains(script, `GRPC_TLS_ENABLED="false"`) {
		t.Error("expected GRPC_TLS_ENABLED=false")
	}
	if !strings.Contains(script, "enabled: false") {
		t.Error("expected tls.enabled: false in generated config")
	}
	// download_ca function still exists but skips when GRPC_TLS_ENABLED != true
	// download_ca 函数仍存在，但在 GRPC_TLS_ENABLED != true 时会跳过
	if !strings.Contains(script, "download_ca") {
		t.Error("expected download_ca helper to remain in script")
	}
}

// TestInstallScriptContainsRequirements tests that the script implements all requirements.
// TestInstallScriptContainsRequirements 测试脚本实现了所有需求。
func TestInstallScriptContainsRequirements(t *testing.T) {
	gen, err := NewInstallScriptGenerator(nil)
	if err != nil {
		t.Fatalf("Failed to create generator: %v", err)
	}

	script, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Requirement 2.1: Auto-detection of OS and architecture
	// 需求 2.1: 自动检测操作系统和架构
	if !strings.Contains(script, "detect_os") || !strings.Contains(script, "detect_arch") {
		t.Error("Script missing OS/architecture detection (Requirement 2.1)")
	}

	// Requirement 2.2: Download from Control Plane
	// 需求 2.2: 从 Control Plane 下载
	if !strings.Contains(script, "download_agent") || !strings.Contains(script, "/api/v1/agent/download") {
		t.Error("Script missing download functionality (Requirement 2.2)")
	}
	if !strings.Contains(script, "download_support_assets") ||
		!strings.Contains(script, "CAPABILITY_PROXY_VERSION=\""+seatunnelmeta.DefaultSTXJavaProxyVersion+"\"") ||
		!strings.Contains(script, "/api/v1/agent/assets/stx-java-proxy.jar?version=${CAPABILITY_PROXY_VERSION}") {
		t.Error("Script missing stx-java-proxy asset download functionality")
	}

	// Requirement 2.3: Install under Agent home ($HOME/.stx/agent by default) and create config
	// 需求 2.3: 安装到 Agent 主目录（默认 $HOME/.stx/agent）并创建配置
	if !strings.Contains(script, "$HOME/.stx/agent") || !strings.Contains(script, "--install-dir") {
		t.Error("Script missing default Agent home / --install-dir (Requirement 2.3)")
	}
	if !strings.Contains(script, "config.yaml") {
		t.Error("Script missing config file creation (Requirement 2.3)")
	}
	if !strings.Contains(script, "SUPPORT_LIB_DIR") && !strings.Contains(script, "/lib") {
		t.Error("Script missing support asset installation path")
	}

	// Requirement 2.4: Create service with auto-start (systemd or launchd)
	// 需求 2.4: 创建服务并自动启动（systemd 或 launchd）
	if !strings.Contains(script, "create_systemd_service") || !strings.Contains(script, "systemctl") {
		t.Error("Script missing systemd service creation (Requirement 2.4)")
	}
	if !strings.Contains(script, "create_launchd_service") || !strings.Contains(script, "launchctl") {
		t.Error("Script missing launchd service creation for macOS")
	}

	// Requirement 2.5: Start Agent and wait for registration
	// 需求 2.5: 启动 Agent 并等待注册
	if !strings.Contains(script, "start_agent") {
		t.Error("Script missing Agent start functionality (Requirement 2.5)")
	}

	// Requirement 2.6: Error handling with cleanup
	// 需求 2.6: 错误处理和清理
	if !strings.Contains(script, "cleanup") || !strings.Contains(script, "trap") {
		t.Error("Script missing error handling/cleanup (Requirement 2.6)")
	}
}
