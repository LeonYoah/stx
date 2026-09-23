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

// Package agent provides Agent distribution and management tests.
// agent 包提供 Agent 分发和管理的测试。
package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/gin-gonic/gin"
)

// setupTestRouter creates a test Gin router with the Agent handler.
// setupTestRouter 创建带有 Agent 处理器的测试 Gin 路由器。
func setupTestRouter(handler *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/agent/install.sh", handler.GetInstallScript)
	r.GET("/api/v1/agent/ca.crt", handler.DownloadCA)
	r.GET("/api/v1/agent/download", handler.DownloadAgent)
	r.GET("/api/v1/agent/assets/stx-java-proxy.jar", handler.DownloadSTXJavaProxyJar)
	r.GET("/api/v1/agent/assets/stx-java-proxy.sh", handler.DownloadSTXJavaProxyScript)
	return r
}

// TestNewHandler tests the Handler creation with various configurations.
// TestNewHandler 测试使用各种配置创建 Handler。
func TestNewHandler(t *testing.T) {
	// Test with nil config (should use defaults)
	// 使用 nil 配置测试（应使用默认值）
	h := NewHandler(nil)
	if h == nil {
		t.Fatal("Expected non-nil handler")
	}
	if h.agentBinaryDir != "./lib/agent" {
		t.Errorf("Expected default binary dir './lib/agent', got '%s'", h.agentBinaryDir)
	}
	expectedDefaultJarPath := filepath.Join("./lib", seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion))
	if h.stxJavaProxyJarPath != expectedDefaultJarPath {
		t.Errorf("Expected default stx-java-proxy jar path %q, got %q", expectedDefaultJarPath, h.stxJavaProxyJarPath)
	}
	expectedDefaultScriptPath := filepath.Join("./scripts", seatunnelmeta.STXJavaProxyScriptFileName)
	if h.stxJavaProxyScriptPath != expectedDefaultScriptPath {
		t.Errorf("Expected default stx-java-proxy script path %q, got %q", expectedDefaultScriptPath, h.stxJavaProxyScriptPath)
	}
	if h.grpcPort != "50051" {
		t.Errorf("Expected default gRPC port '50051', got '%s'", h.grpcPort)
	}

	// Test with custom config
	// 使用自定义配置测试
	customConfig := &HandlerConfig{
		ControlPlaneAddr:       "http://custom-host:8080",
		AgentBinaryDir:         "/custom/path",
		STXJavaProxyJarPath:    "/custom/lib/stx-java-proxy-v2.jar",
		STXJavaProxyScriptPath: "/custom/scripts/stx-java-proxy.sh",
		GRPCPort:               "50052",
	}
	h2 := NewHandler(customConfig)
	if h2.controlPlaneAddr != "http://custom-host:8080" {
		t.Errorf("Expected custom control plane addr, got '%s'", h2.controlPlaneAddr)
	}
	if h2.agentBinaryDir != "/custom/path" {
		t.Errorf("Expected custom binary dir '/custom/path', got '%s'", h2.agentBinaryDir)
	}
	if h2.stxJavaProxyJarPath != "/custom/lib/stx-java-proxy-v2.jar" {
		t.Errorf("Expected custom stx-java-proxy jar path, got '%s'", h2.stxJavaProxyJarPath)
	}
	if h2.stxJavaProxyScriptPath != "/custom/scripts/stx-java-proxy.sh" {
		t.Errorf("Expected custom stx-java-proxy script path, got '%s'", h2.stxJavaProxyScriptPath)
	}
	if h2.grpcPort != "50052" {
		t.Errorf("Expected custom gRPC port '50052', got '%s'", h2.grpcPort)
	}
}

// TestDownloadCA_TLSDisabled returns 404 when Control Plane TLS is off.
// TestDownloadCA_TLSDisabled 在 Control Plane 未开 TLS 时返回 404。
func TestDownloadCA_TLSDisabled(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		TLSEnabled: false,
		CAFile:     "/tmp/does-not-matter.crt",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/ca.crt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestDownloadCA_Success serves the CA PEM when TLS is enabled and the file exists.
// TestDownloadCA_Success 在 TLS 开启且 CA 文件存在时返回 PEM。
func TestDownloadCA_Success(t *testing.T) {
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	caPEM := []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler(&HandlerConfig{
		TLSEnabled: true,
		CAFile:     caPath,
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/ca.crt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-pem-file") {
		t.Errorf("unexpected content type: %s", ct)
	}
	if !strings.Contains(w.Body.String(), "BEGIN CERTIFICATE") {
		t.Errorf("expected PEM body, got %q", w.Body.String())
	}
}

// TestGetInstallScript_TLSEnabled embeds CA download into the install script.
// TestGetInstallScript_TLSEnabled 验证安装脚本在 TLS 开启时包含 CA 下载。
func TestGetInstallScript_TLSEnabled(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		ControlPlaneAddr: "http://test-server:8080",
		GRPCPort:         "50051",
		TLSEnabled:       true,
		CAFile:           "/etc/stx/certs/ca.crt",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/install.sh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`GRPC_TLS_ENABLED="true"`, "/api/v1/agent/ca.crt", "enabled: true"} {
		if !strings.Contains(body, want) {
			t.Errorf("install script missing %q", want)
		}
	}
}

// TestGetInstallScript tests the install script endpoint.
// TestGetInstallScript 测试安装脚本端点。
// Requirements: 2.1 - Returns shell script with auto-detection logic.
func TestGetInstallScript(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		ControlPlaneAddr: "http://test-server:8080",
		GRPCPort:         "50051",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/install.sh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify status code
	// 验证状态码
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify content type
	// 验证内容类型
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/x-shellscript") {
		t.Errorf("Expected content type 'text/x-shellscript', got '%s'", contentType)
	}

	// Verify content disposition
	// 验证内容处置
	contentDisposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "install.sh") {
		t.Errorf("Expected content disposition with 'install.sh', got '%s'", contentDisposition)
	}

	// Verify script content contains key elements
	// 验证脚本内容包含关键元素
	body := w.Body.String()

	// Check for shebang
	// 检查 shebang
	if !strings.HasPrefix(body, "#!/bin/bash") {
		t.Error("Expected script to start with '#!/bin/bash'")
	}

	// Check for Control Plane address
	// 检查 Control Plane 地址
	if !strings.Contains(body, "http://test-server:8080") {
		t.Error("Expected script to contain Control Plane address")
	}

	// Check for gRPC address
	// 检查 gRPC 地址
	if !strings.Contains(body, "test-server:50051") {
		t.Error("Expected script to contain gRPC address")
	}

	// Check for OS detection function
	// 检查操作系统检测函数
	if !strings.Contains(body, "detect_os") {
		t.Error("Expected script to contain OS detection function")
	}

	// Check for architecture detection function
	// 检查架构检测函数
	if !strings.Contains(body, "detect_arch") {
		t.Error("Expected script to contain architecture detection function")
	}

	// Check for systemd service creation
	// 检查 systemd 服务创建
	if !strings.Contains(body, "systemd") {
		t.Error("Expected script to contain systemd service creation")
	}

	// Check for cleanup function (Requirements 2.6)
	// 检查清理函数（需求 2.6）
	if !strings.Contains(body, "cleanup") {
		t.Error("Expected script to contain cleanup function")
	}

	if !strings.Contains(body, "CAPABILITY_PROXY_VERSION=\""+seatunnelmeta.DefaultSTXJavaProxyVersion+"\"") {
		t.Error("Expected script to contain stx-java-proxy version variable")
	}

	if !strings.Contains(body, "/api/v1/agent/assets/stx-java-proxy.jar?version=${CAPABILITY_PROXY_VERSION}") {
		t.Error("Expected script to contain stx-java-proxy jar download URL")
	}

	if !strings.Contains(body, "/api/v1/agent/assets/stx-java-proxy.sh") {
		t.Error("Expected script to contain stx-java-proxy script download URL")
	}
}

// TestDownloadAgentMissingParams tests download with missing parameters.
// TestDownloadAgentMissingParams 测试缺少参数的下载。
func TestDownloadAgentMissingParams(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: "./test-binaries",
	})
	router := setupTestRouter(handler)

	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "Missing both parameters",
			url:      "/api/v1/agent/download",
			expected: "Missing required parameters",
		},
		{
			name:     "Missing arch parameter",
			url:      "/api/v1/agent/download?os=linux",
			expected: "Missing required parameters",
		},
		{
			name:     "Missing os parameter",
			url:      "/api/v1/agent/download?arch=amd64",
			expected: "Missing required parameters",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tc.url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}

			var resp ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if !strings.Contains(resp.ErrorMsg, tc.expected) {
				t.Errorf("Expected error message to contain '%s', got '%s'", tc.expected, resp.ErrorMsg)
			}
		})
	}
}

// TestDownloadAgentUnsupportedOS tests download with unsupported OS.
// TestDownloadAgentUnsupportedOS 测试不支持的操作系统的下载。
func TestDownloadAgentUnsupportedOS(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: "./test-binaries",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/download?os=windows&arch=amd64", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !strings.Contains(resp.ErrorMsg, "Unsupported operating system") {
		t.Errorf("Expected error about unsupported OS, got '%s'", resp.ErrorMsg)
	}
}

// TestDownloadAgentUnsupportedArch tests download with unsupported architecture.
// TestDownloadAgentUnsupportedArch 测试不支持的架构的下载。
func TestDownloadAgentUnsupportedArch(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: "./test-binaries",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/download?os=linux&arch=386", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !strings.Contains(resp.ErrorMsg, "Unsupported architecture") {
		t.Errorf("Expected error about unsupported architecture, got '%s'", resp.ErrorMsg)
	}
}

// TestDownloadAgentBinaryNotFound tests download when binary file doesn't exist.
// TestDownloadAgentBinaryNotFound 测试二进制文件不存在时的下载。
func TestDownloadAgentBinaryNotFound(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: "./non-existent-dir",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/download?os=linux&arch=amd64", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if !strings.Contains(resp.ErrorMsg, "not found") {
		t.Errorf("Expected error about binary not found, got '%s'", resp.ErrorMsg)
	}
}

// TestDownloadAgentSuccess tests successful binary download.
// TestDownloadAgentSuccess 测试成功的二进制下载。
// Requirements: 2.2 - Downloads Agent binary for specified OS and architecture.
func TestDownloadAgentSuccess(t *testing.T) {
	// Create temporary directory with test binary
	// 创建带有测试二进制文件的临时目录
	tempDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test binary file
	// 创建测试二进制文件
	testBinaryContent := []byte("test binary content")
	testBinaryPath := filepath.Join(tempDir, "stx-agent-linux-amd64")
	if err := os.WriteFile(testBinaryPath, testBinaryContent, 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: tempDir,
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/download?os=linux&arch=amd64", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify content type
	// 验证内容类型
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/octet-stream") {
		t.Errorf("Expected content type 'application/octet-stream', got '%s'", contentType)
	}

	// Verify content disposition
	// 验证内容处置
	contentDisposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, "stx-agent-linux-amd64") {
		t.Errorf("Expected content disposition with 'stx-agent-linux-amd64', got '%s'", contentDisposition)
	}

	// Verify content
	// 验证内容
	if w.Body.String() != string(testBinaryContent) {
		t.Error("Downloaded content doesn't match expected binary content")
	}
}

func TestDownloadSTXJavaProxyJarSuccess(t *testing.T) {
	tempDir := t.TempDir()
	jarPath := filepath.Join(tempDir, seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion))
	expectedContent := []byte("proxy-jar")
	if err := os.WriteFile(jarPath, expectedContent, 0o644); err != nil {
		t.Fatalf("Failed to create proxy jar: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		STXJavaProxyJarPath: jarPath,
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/assets/stx-java-proxy.jar", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion)) {
		t.Fatalf("Expected jar attachment header, got %q", w.Header().Get("Content-Disposition"))
	}
	if w.Body.String() != string(expectedContent) {
		t.Fatalf("Unexpected jar content: %q", w.Body.String())
	}
}

func TestDownloadSTXJavaProxyJarFallsBackToDefaultVersion(t *testing.T) {
	tempDir := t.TempDir()
	defaultJarPath := filepath.Join(tempDir, seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion))
	expectedContent := []byte("fallback-jar")
	if err := os.WriteFile(defaultJarPath, expectedContent, 0o644); err != nil {
		t.Fatalf("Failed to create default proxy jar: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		STXJavaProxyJarPath: defaultJarPath,
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/assets/stx-java-proxy.jar?version=2.3.99", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion)) {
		t.Fatalf("Expected fallback jar attachment header, got %q", w.Header().Get("Content-Disposition"))
	}
	if w.Body.String() != string(expectedContent) {
		t.Fatalf("Unexpected fallback jar content: %q", w.Body.String())
	}
}

func TestDownloadSTXJavaProxyJarRejectsInvalidVersion(t *testing.T) {
	tempDir := t.TempDir()
	defaultJarPath := filepath.Join(tempDir, seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion))
	if err := os.WriteFile(defaultJarPath, []byte("default-jar"), 0o644); err != nil {
		t.Fatalf("Failed to create default proxy jar: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		STXJavaProxyJarPath: defaultJarPath,
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/assets/stx-java-proxy.jar?version=../../../tmp/evil", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if !strings.Contains(resp.ErrorMsg, "invalid version parameter") {
		t.Fatalf("Unexpected error message: %q", resp.ErrorMsg)
	}
}

func TestDownloadSTXJavaProxyScriptNotFound(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		STXJavaProxyScriptPath: filepath.Join(t.TempDir(), "missing-stx-java-proxy.sh"),
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/agent/assets/stx-java-proxy.sh", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected status 404, got %d", w.Code)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}
	if !strings.Contains(resp.ErrorMsg, "Capability proxy script not found") {
		t.Fatalf("Unexpected error message: %q", resp.ErrorMsg)
	}
}

// TestDownloadAgentAllArchitectures tests download for all supported architectures.
// TestDownloadAgentAllArchitectures 测试所有支持架构的下载。
func TestDownloadAgentAllArchitectures(t *testing.T) {
	// Create temporary directory with test binaries
	// 创建带有测试二进制文件的临时目录
	tempDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test binaries for all supported combinations
	// 为所有支持的组合创建测试二进制文件
	testCases := []struct {
		os       string
		arch     string
		filename string
	}{
		{"linux", "amd64", "stx-agent-linux-amd64"},
		{"linux", "arm64", "stx-agent-linux-arm64"},
		{"darwin", "amd64", "stx-agent-darwin-amd64"},
		{"darwin", "arm64", "stx-agent-darwin-arm64"},
	}

	for _, tc := range testCases {
		binaryPath := filepath.Join(tempDir, tc.filename)
		content := []byte("binary-" + tc.os + "-" + tc.arch)
		if err := os.WriteFile(binaryPath, content, 0755); err != nil {
			t.Fatalf("Failed to create test binary %s: %v", tc.filename, err)
		}
	}

	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: tempDir,
	})
	router := setupTestRouter(handler)

	for _, tc := range testCases {
		t.Run(tc.os+"-"+tc.arch, func(t *testing.T) {
			url := "/api/v1/agent/download?os=" + tc.os + "&arch=" + tc.arch
			req, _ := http.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200 for %s-%s, got %d", tc.os, tc.arch, w.Code)
			}

			expectedContent := "binary-" + tc.os + "-" + tc.arch
			if w.Body.String() != expectedContent {
				t.Errorf("Expected content '%s', got '%s'", expectedContent, w.Body.String())
			}
		})
	}
}

// TestGetControlPlaneURL tests the URL generation helper.
// TestGetControlPlaneURL 测试 URL 生成辅助函数。
func TestGetControlPlaneURL(t *testing.T) {
	testCases := []struct {
		name     string
		addr     string
		expected string
	}{
		{
			name:     "With http prefix",
			addr:     "http://localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "With https prefix",
			addr:     "https://secure-host:8443",
			expected: "https://secure-host:8443",
		},
		{
			name:     "Without prefix",
			addr:     "localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "Empty address",
			addr:     "",
			expected: "http://localhost:8080",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &Handler{controlPlaneAddr: tc.addr}
			result := handler.getControlPlaneURL()
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

// TestGetGRPCAddr tests the gRPC address generation helper.
// TestGetGRPCAddr 测试 gRPC 地址生成辅助函数。
func TestGetGRPCAddr(t *testing.T) {
	testCases := []struct {
		name     string
		addr     string
		grpcPort string
		expected string
	}{
		{
			name:     "With http prefix and port",
			addr:     "http://localhost:8080",
			grpcPort: "50051",
			expected: "localhost:50051",
		},
		{
			name:     "With https prefix and port",
			addr:     "https://secure-host:8443",
			grpcPort: "50052",
			expected: "secure-host:50052",
		},
		{
			name:     "Without prefix",
			addr:     "myhost:8080",
			grpcPort: "50051",
			expected: "myhost:50051",
		},
		{
			name:     "Without port",
			addr:     "http://myhost",
			grpcPort: "50051",
			expected: "myhost:50051",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &Handler{
				controlPlaneAddr: tc.addr,
				grpcPort:         tc.grpcPort,
			}
			result := handler.getGRPCAddr()
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

// TestDownloadAgentCaseInsensitive tests that OS and arch parameters are case-insensitive.
// TestDownloadAgentCaseInsensitive 测试 OS 和 arch 参数不区分大小写。
func TestDownloadAgentCaseInsensitive(t *testing.T) {
	// Create temporary directory with test binary
	// 创建带有测试二进制文件的临时目录
	tempDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test binary
	// 创建测试二进制文件
	testBinaryPath := filepath.Join(tempDir, "stx-agent-linux-amd64")
	if err := os.WriteFile(testBinaryPath, []byte("test"), 0755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		AgentBinaryDir: tempDir,
	})
	router := setupTestRouter(handler)

	testCases := []struct {
		os   string
		arch string
	}{
		{"LINUX", "AMD64"},
		{"Linux", "Amd64"},
		{"linux", "amd64"},
		{"LINUX", "amd64"},
		{"linux", "AMD64"},
	}

	for _, tc := range testCases {
		t.Run(tc.os+"-"+tc.arch, func(t *testing.T) {
			url := "/api/v1/agent/download?os=" + tc.os + "&arch=" + tc.arch
			req, _ := http.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status 200 for %s-%s, got %d", tc.os, tc.arch, w.Code)
			}
		})
	}
}
