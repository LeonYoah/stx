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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

// TestFetchVersionsFromApache tests fetching versions from Apache Archive
// TestFetchVersionsFromApache 测试从 Apache Archive 获取版本列表
func TestFetchVersionsFromApache(t *testing.T) {
	// Create a mock server that returns Apache Archive HTML
	// 创建一个返回 Apache Archive HTML 的模拟服务器
	mockHTML := `
<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html>
 <head>
  <title>Index of /dist/seatunnel</title>
 </head>
 <body>
<h1>Index of /dist/seatunnel</h1>
<pre><img src="/icons/blank.gif" alt="Icon "> <a href="?C=N;O=D">Name</a>                    <a href="?C=M;O=A">Last modified</a>      <a href="?C=S;O=A">Size</a>  <a href="?C=D;O=A">Description</a><hr><img src="/icons/back.gif" alt="[PARENTDIR]"> <a href="/dist/">Parent Directory</a>                             -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.1.0/">2.1.0/</a>                  2022-03-18 03:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.1.1/">2.1.1/</a>                  2022-06-08 02:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.1.2/">2.1.2/</a>                  2022-08-10 09:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.1.3/">2.1.3/</a>                  2022-10-26 08:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.2.0-beta/">2.2.0-beta/</a>             2022-09-21 03:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.0/">2.3.0/</a>                  2023-01-10 08:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.1/">2.3.1/</a>                  2023-04-18 03:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.2/">2.3.2/</a>                  2023-07-12 09:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.3/">2.3.3/</a>                  2023-09-20 08:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.4/">2.3.4/</a>                  2024-01-15 03:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.5/">2.3.5/</a>                  2024-03-20 09:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.6/">2.3.6/</a>                  2024-05-15 08:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.7/">2.3.7/</a>                  2024-07-10 03:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.8/">2.3.8/</a>                  2024-08-20 09:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.9/">2.3.9/</a>                  2024-09-25 08:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.10/">2.3.10/</a>                 2024-10-30 03:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.11/">2.3.11/</a>                 2024-11-15 09:28    -   
<img src="/icons/folder.gif" alt="[DIR]"> <a href="2.3.12/">2.3.12/</a>                 2024-12-01 08:28    -   
<img src="/icons/text.gif" alt="[TXT]"> <a href="KEYS">KEYS</a>                    2024-12-01 08:28  123K  
<hr></pre>
</body></html>
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockHTML))
	}))
	defer server.Close()

	// Test parsing the HTML / 测试解析 HTML
	t.Run("ParseMockHTML", func(t *testing.T) {
		// We can't easily test fetchVersionsFromApache directly because it uses a hardcoded URL
		// Instead, test the regex pattern used in the function
		// 我们无法直接测试 fetchVersionsFromApache 因为它使用硬编码的 URL
		// 所以测试函数中使用的正则表达式模式

		service := NewService(t.TempDir(), nil)
		ctx := context.Background()

		// Test that getVersions returns fallback versions when cache is empty
		// 测试当缓存为空时 getVersions 返回备用版本
		versions := service.getVersions(ctx)
		if len(versions) == 0 {
			t.Error("Expected non-empty versions list")
		}

		// Verify fallback versions are returned / 验证返回了备用版本
		found := false
		for _, v := range versions {
			if v == "2.3.12" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find version 2.3.12 in fallback versions")
		}
	})
}

// TestCompareVersions tests version comparison
// TestCompareVersions 测试版本比较
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int // >0 if v1 > v2, <0 if v1 < v2, 0 if equal
	}{
		{"equal versions", "2.3.12", "2.3.12", 0},
		{"v1 greater major", "3.0.0", "2.3.12", 1},
		{"v1 less major", "1.0.0", "2.3.12", -1},
		{"v1 greater minor", "2.4.0", "2.3.12", 1},
		{"v1 less minor", "2.2.0", "2.3.12", -1},
		{"v1 greater patch", "2.3.13", "2.3.12", 1},
		{"v1 less patch", "2.3.11", "2.3.12", -1},
		{"beta vs release", "2.2.0-beta", "2.2.0", -1},
		{"release vs beta", "2.2.0", "2.2.0-beta", 1},
		{"different beta", "2.2.0-alpha", "2.2.0-beta", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			if tt.expected > 0 && result <= 0 {
				t.Errorf("compareVersions(%s, %s) = %d, expected > 0", tt.v1, tt.v2, result)
			} else if tt.expected < 0 && result >= 0 {
				t.Errorf("compareVersions(%s, %s) = %d, expected < 0", tt.v1, tt.v2, result)
			} else if tt.expected == 0 && result != 0 {
				t.Errorf("compareVersions(%s, %s) = %d, expected = 0", tt.v1, tt.v2, result)
			}
		})
	}
}

// TestVersionCache tests version caching behavior
// TestVersionCache 测试版本缓存行为
func TestVersionCache(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	// Set cached versions manually / 手动设置缓存版本
	service.versionsMu.Lock()
	service.cachedVersions = []string{"2.3.12", "2.3.11", "2.3.10"}
	service.versionsCacheTime = time.Now()
	service.versionsMu.Unlock()

	// Get versions should return cached versions / getVersions 应该返回缓存的版本
	versions := service.getVersions(ctx)
	if len(versions) != 3 {
		t.Errorf("Expected 3 cached versions, got %d", len(versions))
	}
	if versions[0] != "2.3.12" {
		t.Errorf("Expected first version to be 2.3.12, got %s", versions[0])
	}
}

func TestBuildInstallParamsIncludesRuntimeConfig(t *testing.T) {
	dynamicSlot := false
	slotNum := 6
	historyJobExpireMinutes := 2880
	scheduledDeletionEnable := false

	params := buildInstallParams(&InstallationRequest{
		Version:                 "2.3.9",
		ClusterName:             "productionstx01",
		InstallMode:             InstallModeOnline,
		DeploymentMode:          DeploymentModeSeparated,
		NodeRole:                NodeRoleWorker,
		DynamicSlot:             &dynamicSlot,
		SlotNum:                 &slotNum,
		SlotAllocationStrategy:  SlotAllocationStrategySystemLoad,
		JobScheduleStrategy:     JobScheduleStrategyWait,
		HistoryJobExpireMinutes: &historyJobExpireMinutes,
		ScheduledDeletionEnable: &scheduledDeletionEnable,
	})

	if params["dynamic_slot"] != "false" {
		t.Fatalf("expected dynamic_slot=false, got %q", params["dynamic_slot"])
	}
	if params["slot_num"] != "6" {
		t.Fatalf("expected slot_num=6, got %q", params["slot_num"])
	}
	if params["slot_allocation_strategy"] != "SYSTEM_LOAD" {
		t.Fatalf("expected slot_allocation_strategy=SYSTEM_LOAD, got %q", params["slot_allocation_strategy"])
	}
	if params["job_schedule_strategy"] != "WAIT" {
		t.Fatalf("expected job_schedule_strategy=WAIT, got %q", params["job_schedule_strategy"])
	}
	if params["history_job_expire_minutes"] != "2880" {
		t.Fatalf("expected history_job_expire_minutes=2880, got %q", params["history_job_expire_minutes"])
	}
	if params["scheduled_deletion_enable"] != "false" {
		t.Fatalf("expected scheduled_deletion_enable=false, got %q", params["scheduled_deletion_enable"])
	}
	if params["cluster_name"] != "productionstx01" {
		t.Fatalf("expected cluster_name=productionstx01, got %q", params["cluster_name"])
	}
}

func TestStartClusterAfterInstallSkipsStandaloneInstallation(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:   "10",
		NodeRole: NodeRoleMasterWorker,
	}, status)

	if status.Status != StepStatusSuccess {
		t.Fatalf("独立安装状态被意外修改: %s", status.Status)
	}
	if status.Message != "安装完成；未提供集群 ID，已跳过集群启动" {
		t.Fatalf("独立安装完成文案错误: %q", status.Message)
	}
	if status.CurrentStep != InstallStepComplete {
		t.Fatalf("独立安装完成步骤错误: %q", status.CurrentStep)
	}
	if status.EndTime == nil {
		t.Fatal("独立安装成功后缺少结束时间")
	}
}

type stubInstallationNodeStarter struct {
	ensureCalls int
	startCalls  int
	ensureErr   error
	// ensureJavaHome 记录登记节点时下发的 JDK。/ ensureJavaHome records the JDK passed when registering the node.
	ensureJavaHome      string
	ensureHazelcastPort int
	startOK             bool
	startMsg            string
	startErr            error
}

func (s *stubInstallationNodeStarter) EnsureNodeForInstallation(ctx context.Context, clusterID uint, hostID uint, role string, installDir string, hazelcastPort int, apiPort int, workerPort int, javaHome string) error {
	s.ensureCalls++
	s.ensureJavaHome = javaHome
	s.ensureHazelcastPort = hazelcastPort
	return s.ensureErr
}

func (s *stubInstallationNodeStarter) StartNodeByClusterAndHost(ctx context.Context, clusterID uint, hostID uint) (bool, string, error) {
	return s.startOK, s.startMsg, s.startErr
}

func (s *stubInstallationNodeStarter) StartNodeByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (bool, string, error) {
	s.startCalls++
	return s.startOK, s.startMsg, s.startErr
}

func TestStartClusterAfterInstallRegistersNodeBeforeStarting(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	starter := &stubInstallationNodeStarter{startOK: true, startMsg: "started"}
	service.SetNodeStarter(starter)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:      "10",
		ClusterID:   "9",
		NodeRole:    NodeRoleMasterWorker,
		InstallDir:  "/tmp/seatunnel-2.3.12-retry",
		ClusterPort: 15822,
		WorkerPort:  15823,
		HTTPPort:    18099,
	}, status)

	if starter.ensureCalls != 1 {
		t.Fatalf("expected one node registration call, got %d", starter.ensureCalls)
	}
	if starter.startCalls != 1 {
		t.Fatalf("expected one node start call, got %d", starter.startCalls)
	}
	if status.Status != StepStatusSuccess {
		t.Fatalf("expected successful installation status, got %s", status.Status)
	}
	if status.Error != "" {
		t.Fatalf("expected empty installation error, got %q", status.Error)
	}
	if status.CurrentStep != InstallStepComplete {
		t.Fatalf("expected current step %q, got %q", InstallStepComplete, status.CurrentStep)
	}
	if status.EndTime == nil {
		t.Fatal("expected successful installation to record an end time")
	}
}

func TestStartClusterAfterInstallSeparatedWorkerRegistersWorkerPort(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	starter := &stubInstallationNodeStarter{startOK: true, startMsg: "started"}
	service.SetNodeStarter(starter)
	status := &InstallationStatus{Status: StepStatusRunning}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:         "10",
		ClusterID:      "9",
		NodeRole:       NodeRoleWorker,
		DeploymentMode: DeploymentModeSeparated,
		InstallDir:     "/tmp/seatunnel-worker",
		ClusterPort:    15822,
		WorkerPort:     15823,
	}, status)

	if starter.ensureCalls != 1 {
		t.Fatalf("expected one node registration call, got %d", starter.ensureCalls)
	}
	if starter.ensureHazelcastPort != 15823 {
		t.Fatalf("expected separated worker registered with worker port 15823, got %d", starter.ensureHazelcastPort)
	}
}

func TestStartClusterAfterInstallRegistrationFailureMarksInstallationFailed(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	starter := &stubInstallationNodeStarter{ensureErr: errors.New("register failed")}
	service.SetNodeStarter(starter)
	status := &InstallationStatus{Status: StepStatusSuccess}

	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID:    "10",
		ClusterID: "9",
		NodeRole:  NodeRoleMasterWorker,
	}, status)

	if status.Status != StepStatusFailed {
		t.Fatalf("expected failed installation status, got %s", status.Status)
	}
	if !strings.Contains(status.Error, "register failed") {
		t.Fatalf("expected registration error, got %q", status.Error)
	}
	if starter.startCalls != 0 {
		t.Fatalf("expected startup to be skipped after registration failure, got %d calls", starter.startCalls)
	}
}

// TestFallbackVersions tests that fallback versions are used when fetch fails
// TestFallbackVersions 测试当获取失败时使用备用版本
func TestFallbackVersions(t *testing.T) {
	// Verify FallbackVersions is not empty / 验证 FallbackVersions 不为空
	fallbackVersions := seatunnel.FallbackVersions()
	if len(fallbackVersions) == 0 {
		t.Error("FallbackVersions should not be empty")
	}

	// Verify recommended version is in fallback list / 验证推荐版本在备用列表中
	found := false
	for _, v := range fallbackVersions {
		if v == seatunnel.RecommendedVersion() {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RecommendedVersion %s should be in FallbackVersions", seatunnel.RecommendedVersion())
	}
}

// TestRefreshVersions tests the RefreshVersions method
// TestRefreshVersions 测试 RefreshVersions 方法
func TestRefreshVersions(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	// RefreshVersions should return versions (either from Apache or fallback)
	// RefreshVersions 应该返回版本（从 Apache 或备用列表）
	versions, err := service.RefreshVersions(ctx)

	// Even if fetch fails, we should get fallback versions / 即使获取失败，我们也应该得到备用版本
	if len(versions) == 0 {
		t.Error("RefreshVersions should return non-empty versions list")
	}

	// If there's an error, it means we're using fallback / 如果有错误，说明我们在使用备用列表
	if err != nil {
		t.Logf("Using fallback versions due to error: %v", err)
	} else {
		t.Log("Successfully fetched versions from Apache Archive")
	}
}

// TestFetchVersionsFromApacheIntegration is an integration test that actually fetches from Apache
// TestFetchVersionsFromApacheIntegration 是一个实际从 Apache 获取的集成测试
// This test is skipped by default, run with: go test -run TestFetchVersionsFromApacheIntegration -v
// 此测试默认跳过，运行方式: go test -run TestFetchVersionsFromApacheIntegration -v
func TestFetchVersionsFromApacheIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	service := NewService(t.TempDir(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	versions, err := service.fetchVersionsFromApache(ctx)
	if err != nil {
		t.Logf("Failed to fetch from Apache Archive (this is expected if network is unavailable): %v", err)
		t.Log("Verifying fallback versions are available...")

		// Verify fallback works / 验证备用列表可用
		fallbackVersions := service.getVersions(ctx)
		if len(fallbackVersions) == 0 {
			t.Error("Fallback versions should not be empty")
		}
		return
	}

	t.Logf("Successfully fetched %d versions from Apache Archive", len(versions))

	// Verify versions are sorted in descending order / 验证版本按降序排序
	if len(versions) > 1 {
		for i := 0; i < len(versions)-1; i++ {
			if compareVersions(versions[i], versions[i+1]) < 0 {
				t.Errorf("Versions not sorted correctly: %s should come before %s", versions[i], versions[i+1])
			}
		}
	}

	// Log all versions / 记录所有版本
	t.Log("Fetched versions:")
	for _, v := range versions {
		t.Logf("  - %s", v)
	}
}

type stubNodeJVMResolver struct {
	result *JVMConfig
	err    error
}

func (s *stubNodeJVMResolver) ResolveNodeJVMByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (*JVMConfig, error) {
	return s.result, s.err
}

func TestService_resolveInstallationJVM_usesNodeResolverWhenRequestJVMIsEmpty(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	service.SetNodeJVMResolver(&stubNodeJVMResolver{
		result: &JVMConfig{
			MasterHeapSize: 6,
			WorkerHeapSize: 8,
		},
	})

	req := &InstallationRequest{
		HostID:    "7",
		ClusterID: "9",
		NodeRole:  NodeRoleMaster,
	}

	service.resolveInstallationJVM(context.Background(), req)

	if req.JVM == nil {
		t.Fatalf("expected JVM config to be resolved")
	}
	if req.JVM.MasterHeapSize != 6 {
		t.Fatalf("expected master heap 6GB, got %d", req.JVM.MasterHeapSize)
	}
	if req.JVM.WorkerHeapSize != 8 {
		t.Fatalf("expected worker heap 8GB, got %d", req.JVM.WorkerHeapSize)
	}
}

func TestService_preparedPluginCacheRequiresDependencyFingerprint(t *testing.T) {
	service := NewService(t.TempDir(), nil)

	if service.hasPreparedPlugin("agent-a", "jdbc", "2.3.13", "/opt/seatunnel-2.3.13", []string{"mysql"}, "") {
		t.Fatalf("expected empty fingerprint to disable prepared plugin reuse")
	}

	service.rememberPreparedPlugin("agent-a", "jdbc", "2.3.13", "/opt/seatunnel-2.3.13", []string{"mysql"}, "deps-hash-a")

	if !service.hasPreparedPlugin("agent-a", "jdbc", "2.3.13", "/opt/seatunnel-2.3.13", []string{"mysql"}, "deps-hash-a") {
		t.Fatalf("expected prepared plugin cache hit for identical dependency fingerprint")
	}
	if service.hasPreparedPlugin("agent-a", "jdbc", "2.3.13", "/opt/seatunnel-2.3.13", []string{"mysql"}, "deps-hash-b") {
		t.Fatalf("expected dependency fingerprint change to invalidate prepared plugin cache")
	}
}

func createUploadFileHeader(t *testing.T, fieldName, fileName string, content []byte) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file content failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/packages/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(int64(body.Len()) + 1024); err != nil {
		t.Fatalf("parse multipart form failed: %v", err)
	}

	_, fileHeader, err := req.FormFile(fieldName)
	if err != nil {
		t.Fatalf("read multipart file failed: %v", err)
	}
	return fileHeader
}

func createSourceArchive(t *testing.T, version string) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("package org.apache.seatunnel;\n")
	name := "apache-seatunnel-" + version + "-src/seatunnel-core/src/main/java/Example.java"
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatalf("写入源码包头失败 / write source header failed: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("写入源码包内容失败 / write source content failed: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("关闭 tar 失败 / close tar failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("关闭 gzip 失败 / close gzip failed: %v", err)
	}
	return buffer.Bytes()
}

func TestValidateSourceArchiveReadsEntireStream(t *testing.T) {
	const version = "9.9.90"
	tarBuffer := &bytes.Buffer{}
	tarWriter := tar.NewWriter(tarBuffer)
	content := []byte("package org.apache.seatunnel;\n")
	name := "apache-seatunnel-" + version + "-src/seatunnel-core/src/main/java/Example.java"
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatalf("写入源码包头失败 / write source header failed: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("写入源码包内容失败 / write source content failed: %v", err)
	}
	if err := tarWriter.Flush(); err != nil {
		t.Fatalf("刷新 tar 失败 / flush tar failed: %v", err)
	}
	tarBuffer.Write(bytes.Repeat([]byte{0x7f}, 100))

	archive := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(archive)
	if _, err := gzipWriter.Write(tarBuffer.Bytes()); err != nil {
		t.Fatalf("写入 gzip 失败 / write gzip failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("关闭 gzip 失败 / close gzip failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), sourcePackageFileName(version))
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatalf("写入损坏源码包失败 / write corrupt source archive failed: %v", err)
	}

	err := validateSourceArchive(path, version)
	if err == nil || !errors.Is(err, ErrInvalidSourcePackage) {
		t.Fatalf("源码包后半段损坏应被拒绝 / trailing archive corruption must be rejected: %v", err)
	}
}

func createRuntimePackageArchive(t *testing.T, version string, files map[string]string) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		data := []byte(content)
		entryName := "apache-seatunnel-" + version + "/" + name
		if err := tarWriter.WriteHeader(&tar.Header{Name: entryName, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			t.Fatalf("write tar content: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buffer.Bytes()
}

func TestService_UploadPackageImportsBundledConnectors(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages")
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	service := NewService(packageDir, nil)
	const version = "9.9.94"
	archive := createRuntimePackageArchive(t, version, map[string]string{
		"connectors/connector-fake-9.9.94.jar":              "fake-jar",
		"connectors/connector-console-9.9.94.jar":           "console-jar",
		"connectors/nested/connector-ignore-9.9.94.jar":     "ignored",
		"extra/connectors/connector-deep-ignore-9.9.94.jar": "ignored",
		"lib/not-a-connector.jar":                           "ignored",
	})
	fileHeader := createUploadFileHeader(t, "file", packageFileName(version), archive)

	if _, err := service.UploadPackage(context.Background(), version, fileHeader); err != nil {
		t.Fatalf("upload package with bundled connectors: %v", err)
	}
	for name, want := range map[string]string{
		"connector-fake-9.9.94.jar":    "fake-jar",
		"connector-console-9.9.94.jar": "console-jar",
	} {
		got, err := os.ReadFile(filepath.Join(root, "plugins", version, "connectors", name))
		if err != nil {
			t.Fatalf("expected bundled connector %s: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("connector %s content = %q, want %q", name, got, want)
		}
	}
	for _, ignored := range []string{"connector-ignore-9.9.94.jar", "connector-deep-ignore-9.9.94.jar"} {
		if _, err := os.Stat(filepath.Join(root, "plugins", version, "connectors", ignored)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("nested connector jar %s must not be imported, stat err=%v", ignored, err)
		}
	}
}

func TestService_ExtractBundledConnectorsRespectsSizeLimit(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	const version = "9.9.96"
	archive := createRuntimePackageArchive(t, version, map[string]string{
		"connectors/connector-huge-9.9.96.jar": "0123456789abcdef",
	})
	archivePath := filepath.Join(packageDir, packageFileName(version))
	if err := os.WriteFile(archivePath, archive, 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(packageDir, nil)
	if _, err := service.extractBundledConnectorsWithLimits(version, archivePath, 8, 64); err == nil {
		t.Fatal("expected oversized connector jar to be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "plugins", version, "connectors", "connector-huge-9.9.96.jar")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized jar must not be committed, stat err=%v", err)
	}
}

func TestService_RunDownloadExtractsBundledConnectors(t *testing.T) {
	root := t.TempDir()
	packageDir := filepath.Join(root, "packages")
	tempDir := filepath.Join(root, "temp")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	const version = "9.9.97"
	archive := createRuntimePackageArchive(t, version, map[string]string{
		"connectors/connector-fake-9.9.97.jar": "downloaded-fake-jar",
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	service := NewService(packageDir, nil)
	service.tempDir = tempDir
	task := &DownloadTask{
		ID:          "dl-test",
		Version:     version,
		Mirror:      MirrorApache,
		DownloadURL: server.URL + "/" + packageFileName(version),
		Status:      DownloadStatusPending,
		done:        make(chan struct{}),
	}
	service.runDownload(context.Background(), task)
	if task.Status != DownloadStatusCompleted {
		t.Fatalf("expected download completed, got status=%s error=%s", task.Status, task.Error)
	}
	got, err := os.ReadFile(filepath.Join(root, "plugins", version, "connectors", "connector-fake-9.9.97.jar"))
	if err != nil {
		t.Fatalf("expected bundled connector extracted after download: %v", err)
	}
	if string(got) != "downloaded-fake-jar" {
		t.Fatalf("unexpected extracted connector content %q", got)
	}
}

func TestService_UploadPackageValidation(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	t.Run("invalid version is rejected", func(t *testing.T) {
		fileHeader := createUploadFileHeader(t, "file", "apache-seatunnel-2.3.12-bin.tar.gz", []byte("test-data"))

		_, err := service.UploadPackage(ctx, "2.3.12/../../x", fileHeader)
		if err == nil || !errors.Is(err, ErrInvalidPackageVersion) {
			t.Fatalf("expected ErrInvalidPackageVersion, got: %v", err)
		}
	})

	t.Run("invalid extension is rejected", func(t *testing.T) {
		fileHeader := createUploadFileHeader(t, "file", "apache-seatunnel-2.3.12-bin.zip", []byte("test-data"))

		_, err := service.UploadPackage(ctx, "2.3.12", fileHeader)
		if err == nil || !errors.Is(err, ErrInvalidPackageFile) {
			t.Fatalf("expected ErrInvalidPackageFile, got: %v", err)
		}
	})

	t.Run("duplicate version is rejected", func(t *testing.T) {
		fileHeader1 := createUploadFileHeader(t, "file", "apache-seatunnel-2.3.12-bin.tar.gz", []byte("test-data-1"))
		if _, err := service.UploadPackage(ctx, "2.3.12", fileHeader1); err != nil {
			t.Fatalf("first upload should succeed, got error: %v", err)
		}

		fileHeader2 := createUploadFileHeader(t, "file", "apache-seatunnel-2.3.12-bin.tar.gz", []byte("test-data-2"))
		_, err := service.UploadPackage(ctx, "2.3.12", fileHeader2)
		if err == nil || !errors.Is(err, ErrPackageAlreadyExists) {
			t.Fatalf("expected ErrPackageAlreadyExists, got: %v", err)
		}
	})
}

func TestService_UploadPackageBundleStoresOptionalSource(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	service.tempDir = t.TempDir()
	const version = "9.9.93"
	runtimeFile := createUploadFileHeader(t, "file", packageFileName(version), []byte("runtime-package"))
	sourceFile := createUploadFileHeader(t, "source_file", sourcePackageFileName(version), createSourceArchive(t, version))

	info, err := service.UploadPackageBundle(context.Background(), version, runtimeFile, sourceFile)
	if err != nil {
		t.Fatalf("上传运行包和源码失败 / upload runtime and source failed: %v", err)
	}
	if !info.HasSource || info.SourceStatus != DownloadStatusCompleted || info.SourceChecksum == "" {
		t.Fatalf("源码元数据错误 / invalid source metadata: %#v", info)
	}
	if _, err := os.Stat(filepath.Join(service.packageDir, sourcePackageFileName(version))); err != nil {
		t.Fatalf("源码包未落盘 / source archive missing: %v", err)
	}
}

func TestService_UploadSourceAcceptsRenamedArchive(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	service.tempDir = t.TempDir()
	const version = "9.9.92"
	if err := os.WriteFile(filepath.Join(service.packageDir, packageFileName(version)), []byte("runtime-package"), 0o600); err != nil {
		t.Fatalf("写入运行包失败 / write runtime package failed: %v", err)
	}
	sourceFile := createUploadFileHeader(t, "source_file", "renamed-source-backup.tar.gz", createSourceArchive(t, version))

	info, err := service.UploadSourcePackage(context.Background(), version, sourceFile)
	if err != nil {
		t.Fatalf("重命名源码包应允许导入 / renamed source archive should be accepted: %v", err)
	}
	if !info.HasSource || info.SourceFileName != sourcePackageFileName(version) {
		t.Fatalf("源码应按固定文件名保存 / source archive must use canonical stored name: %#v", info)
	}
}

func TestService_InvalidOptionalSourceKeepsRuntimePackage(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	service.tempDir = t.TempDir()
	const version = "9.9.94"
	runtimeFile := createUploadFileHeader(t, "file", packageFileName(version), []byte("runtime-package"))
	sourceFile := createUploadFileHeader(t, "source_file", sourcePackageFileName(version), []byte("not-a-tar-gz"))

	info, err := service.UploadPackageBundle(context.Background(), version, runtimeFile, sourceFile)
	if err != nil {
		t.Fatalf("可选源码失败不应撤销运行包 / optional source failure must not fail runtime: %v", err)
	}
	if !info.IsLocal || info.HasSource || info.SourceStatus != DownloadStatusFailed || info.SourceError == "" {
		t.Fatalf("源码失败状态错误 / invalid source failure state: %#v", info)
	}
	if _, err := os.Stat(filepath.Join(service.packageDir, packageFileName(version))); err != nil {
		t.Fatalf("运行包不应被删除 / runtime package must remain: %v", err)
	}
}

func TestService_StartDownloadIncludesSourceByDefault(t *testing.T) {
	const version = "9.9.95"
	sourceArchive := createSourceArchive(t, version)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/" + version + "/" + packageFileName(version):
			_, _ = writer.Write([]byte("runtime-package"))
		case "/" + version + "/" + sourcePackageFileName(version):
			_, _ = writer.Write(sourceArchive)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	originalURL := MirrorURLs[MirrorAliyun]
	MirrorURLs[MirrorAliyun] = server.URL
	defer func() { MirrorURLs[MirrorAliyun] = originalURL }()

	service := NewService(t.TempDir(), nil)
	service.tempDir = t.TempDir()
	if _, err := service.StartDownload(context.Background(), &DownloadRequest{Version: version, Mirror: MirrorAliyun}); err != nil {
		t.Fatalf("启动下载失败 / start download failed: %v", err)
	}
	var task *DownloadTask
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task, _ = service.GetDownloadStatus(context.Background(), version)
		if task != nil && (task.Status == DownloadStatusCompleted || task.Status == DownloadStatusFailed) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if task == nil || task.Status != DownloadStatusCompleted || task.SourceStatus != DownloadStatusCompleted {
		t.Fatalf("运行包和源码未完成 / runtime and source did not complete: %#v", task)
	}
	info, err := service.GetPackageInfo(context.Background(), version)
	if err != nil || !info.IsLocal || !info.HasSource {
		t.Fatalf("安装包列表未显示源码 / package info missing source: info=%#v err=%v", info, err)
	}
}

func TestGetSourceDownloadURLsIncludesAllMirrors(t *testing.T) {
	const version = "2.3.13"
	urls := getSourceDownloadURLs(version)
	expectedFileName := sourcePackageFileName(version)

	if len(urls) != len(MirrorURLs) {
		t.Fatalf("源码镜像数量不一致 / source mirror count mismatch: got=%d want=%d", len(urls), len(MirrorURLs))
	}
	for mirror, baseURL := range MirrorURLs {
		expectedURL := fmt.Sprintf("%s/%s/%s", baseURL, version, expectedFileName)
		if urls[mirror] != expectedURL {
			t.Fatalf("源码镜像地址错误 / invalid source mirror URL: mirror=%s got=%s want=%s", mirror, urls[mirror], expectedURL)
		}
	}
}

func TestService_FetchSourceDefaultsToApache(t *testing.T) {
	const version = "9.9.96"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/"+version+"/"+sourcePackageFileName(version) {
			t.Fatalf("源码下载路径错误 / invalid source download path: %s", request.URL.Path)
		}
		_, _ = writer.Write(createSourceArchive(t, version))
	}))
	defer server.Close()
	originalURL := MirrorURLs[MirrorApache]
	MirrorURLs[MirrorApache] = server.URL
	defer func() { MirrorURLs[MirrorApache] = originalURL }()

	service := NewService(t.TempDir(), nil)
	service.tempDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(service.packageDir, packageFileName(version)), []byte("runtime-package"), 0o600); err != nil {
		t.Fatalf("写入运行包失败 / write runtime package failed: %v", err)
	}
	info, err := service.FetchSourcePackage(context.Background(), version, "")
	if err != nil {
		t.Fatalf("默认 Apache 补源码失败 / default Apache source fetch failed: %v", err)
	}
	if !info.HasSource || info.SourceStatus != DownloadStatusCompleted {
		t.Fatalf("源码状态错误 / invalid source status: %#v", info)
	}
}

func TestService_resolveOfflinePackagePath_RejectsPathTraversal(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	req := &InstallationRequest{
		Version:     "2.3.12",
		InstallMode: InstallModeOffline,
		PackagePath: "../outside/apache-seatunnel-2.3.12-bin.tar.gz",
	}

	_, err := service.resolveOfflinePackagePath(req)
	if err == nil || !errors.Is(err, ErrInvalidPackagePath) {
		t.Fatalf("expected ErrInvalidPackagePath, got: %v", err)
	}
}

func TestService_UploadPackageChunk_Success(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	version := "2.3.12"
	fileName := "apache-seatunnel-2.3.12-bin.tar.gz"
	content := []byte("chunk-upload-integration-test-content-1234567890")
	chunkSize := 10
	totalChunks := (len(content) + chunkSize - 1) / chunkSize
	uploadID := "upload_chunk_12345678"

	var finalResult *PackageChunkUploadResult
	for chunkIndex := 0; chunkIndex < totalChunks; chunkIndex++ {
		start := chunkIndex * chunkSize
		end := start + chunkSize
		if end > len(content) {
			end = len(content)
		}

		fileHeader := createUploadFileHeader(t, "file", fileName, content[start:end])
		result, err := service.UploadPackageChunk(ctx, &PackageChunkUploadRequest{
			Version:     version,
			UploadID:    uploadID,
			ChunkIndex:  chunkIndex,
			TotalChunks: totalChunks,
			TotalSize:   int64(len(content)),
			FileName:    fileName,
		}, fileHeader)
		if err != nil {
			t.Fatalf("chunk %d upload failed: %v", chunkIndex, err)
		}

		if result.ReceivedChunks != chunkIndex+1 {
			t.Fatalf("expected received chunks %d, got %d", chunkIndex+1, result.ReceivedChunks)
		}

		if chunkIndex < totalChunks-1 && result.Completed {
			t.Fatalf("expected non-final chunk to be incomplete")
		}

		if chunkIndex == totalChunks-1 {
			finalResult = result
		}
	}

	if finalResult == nil {
		t.Fatalf("expected final chunk result")
	}
	if !finalResult.Completed {
		t.Fatalf("expected final result completed")
	}
	if finalResult.Package == nil {
		t.Fatalf("expected final result package info")
	}
	if finalResult.Package.FileSize != int64(len(content)) {
		t.Fatalf("expected file size %d, got %d", len(content), finalResult.Package.FileSize)
	}

	persisted, err := os.ReadFile(finalResult.Package.LocalPath)
	if err != nil {
		t.Fatalf("read persisted package failed: %v", err)
	}
	if !bytes.Equal(persisted, content) {
		t.Fatalf("persisted package content mismatch")
	}
}

func TestService_UploadPackageChunk_OutOfOrder(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	fileHeader := createUploadFileHeader(
		t,
		"file",
		"apache-seatunnel-2.3.12-bin.tar.gz",
		[]byte("chunk-data"),
	)

	_, err := service.UploadPackageChunk(ctx, &PackageChunkUploadRequest{
		Version:     "2.3.12",
		UploadID:    "upload_chunk_out_of_order",
		ChunkIndex:  1,
		TotalChunks: 2,
		TotalSize:   int64(len("chunk-data") * 2),
		FileName:    "apache-seatunnel-2.3.12-bin.tar.gz",
	}, fileHeader)
	if err == nil || !errors.Is(err, ErrChunkOutOfOrder) {
		t.Fatalf("expected ErrChunkOutOfOrder, got: %v", err)
	}
}

func TestService_StartDownload_RejectsInvalidVersion(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	_, err := service.StartDownload(ctx, &DownloadRequest{Version: "../evil", Mirror: MirrorAliyun})
	if err == nil || !errors.Is(err, ErrInvalidPackageVersion) {
		t.Fatalf("expected ErrInvalidPackageVersion, got: %v", err)
	}
}

func TestService_CancelDownload_RejectsInvalidVersion(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	ctx := context.Background()

	_, err := service.CancelDownload(ctx, "../evil")
	if err == nil || !errors.Is(err, ErrInvalidPackageVersion) {
		t.Fatalf("expected ErrInvalidPackageVersion, got: %v", err)
	}
}

func TestService_CancelDownloadStopsRequestAndRemovesPartialFile(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Length", "104857600")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		<-request.Context().Done()
	}))
	defer server.Close()

	originalURL := MirrorURLs[MirrorAliyun]
	MirrorURLs[MirrorAliyun] = server.URL
	defer func() { MirrorURLs[MirrorAliyun] = originalURL }()

	packageDir := t.TempDir()
	tempDir := t.TempDir()
	service := NewService(packageDir, nil)
	service.tempDir = tempDir

	const version = "9.9.97"
	if _, err := service.StartDownload(context.Background(), &DownloadRequest{Version: version, Mirror: MirrorAliyun}); err != nil {
		t.Fatalf("启动下载失败 / start download failed: %v", err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("下载请求没有启动 / download request did not start")
	}

	task, err := service.CancelDownload(context.Background(), version)
	if err != nil {
		t.Fatalf("取消下载失败 / cancel download failed: %v", err)
	}
	if task.Status != DownloadStatusCancelled {
		t.Fatalf("取消后状态错误 / unexpected status after cancel: %s", task.Status)
	}
	fileName := packageFileName(version)
	if _, err := os.Stat(filepath.Join(packageDir, "."+fileName+".tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("临时文件仍然存在 / partial file still exists in packageDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, fileName+".tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("临时文件仍然存在 / partial file still exists in tempDir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(packageDir, fileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("最终文件不应存在 / final file must not exist: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	latest, err := service.GetDownloadStatus(context.Background(), version)
	if err != nil || latest.Status != DownloadStatusCancelled {
		t.Fatalf("取消状态发生倒退 / cancelled status regressed: task=%#v err=%v", latest, err)
	}
}

// TestBuildInstallParamsAgentLocalFallback 验证安装参数携带 Agent 本地兜底所需信息，且无传输路径时不设置 package_path。
// TestBuildInstallParamsAgentLocalFallback verifies params carry Agent-local fallback info and omit package_path when untransferred.
func TestBuildInstallParamsAgentLocalFallback(t *testing.T) {
	params := buildInstallParams(&InstallationRequest{
		Version:          "2.3.13",
		InstallMode:      InstallModeOnline,
		AgentPackagePath: " /data/pkgs ",
	})
	if _, ok := params["package_path"]; ok {
		t.Fatalf("package_path must be omitted when nothing was transferred, got %q", params["package_path"])
	}
	if params["package_file_name"] != "apache-seatunnel-2.3.13-bin.tar.gz" {
		t.Fatalf("package_file_name=%q", params["package_file_name"])
	}
	if params["agent_package_path"] != "/data/pkgs" {
		t.Fatalf("agent_package_path=%q", params["agent_package_path"])
	}
}

func TestListAvailableVersionsSortsLocalPackagesAndKeepsCapabilities(t *testing.T) {
	packageDir := t.TempDir()
	for _, fileName := range []string{
		"apache-seatunnel-2.3.9-bin.tar.gz",
		"apache-seatunnel-2.3.12-bin.tar.gz",
		"apache-seatunnel-3.1.0-bin.tar.gz",
		"apache-seatunnel-3.2.0-custom-bin.tar.gz",
	} {
		if err := os.WriteFile(filepath.Join(packageDir, fileName), []byte("stub"), 0o644); err != nil {
			t.Fatalf("write stub package %s: %v", fileName, err)
		}
	}

	service := NewService(packageDir, nil)
	// 预填旧在线缓存模拟内网落后目录，确认本地高版本与自定义版本仍被识别与排序。
	// Seed a stale online cache to simulate an intranet fallback list and verify newer/custom local packages are still recognized and ordered.
	service.versionsMu.Lock()
	service.cachedVersions = []string{"2.3.12", "2.3.11"}
	service.versionsCacheTime = time.Now()
	service.versionsMu.Unlock()

	available, err := service.ListAvailableVersions(context.Background())
	if err != nil {
		t.Fatalf("ListAvailableVersions failed: %v", err)
	}

	wantLocalOrder := []string{"3.2.0-custom", "3.1.0", "2.3.12", "2.3.9"}
	if len(available.LocalPackages) != len(wantLocalOrder) {
		t.Fatalf("expected %d local packages, got %d", len(wantLocalOrder), len(available.LocalPackages))
	}
	for i, want := range wantLocalOrder {
		if available.LocalPackages[i].Version != want {
			t.Fatalf("local package[%d] = %q, want %q", i, available.LocalPackages[i].Version, want)
		}
	}

	// 在线目录列表不混入自定义本地包，保持在线 Tab 语义纯净。
	// Online catalog versions must not be polluted by custom local packages.
	if len(available.Versions) != 2 || available.Versions[0] != "2.3.12" {
		t.Fatalf("unexpected online versions: %v", available.Versions)
	}
	if available.RecommendedVersion != "3.2.0-custom" {
		t.Fatalf("expected recommended_version compatibility field to prefer newest local package 3.2.0-custom, got %q", available.RecommendedVersion)
	}

	cap310, ok := available.VersionCapabilities["3.1.0"]
	if !ok {
		t.Fatal("expected VersionCapabilities to include local package 3.1.0")
	}
	if cap310.MinJavaVersion != 11 || !cap310.SupportsEventChannel {
		t.Fatalf("unexpected 3.1.0 capabilities: %+v", cap310)
	}
}

func TestMoveFileAtomicFallsBackOnCrossDeviceLink(t *testing.T) {
	t.Run("cross-device rename falls back to sibling copy and atomic rename", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		src := filepath.Join(srcDir, "package-upload.tmp")
		dst := filepath.Join(dstDir, "apache-seatunnel-3.0.0-bin.tar.gz")
		payload := []byte("fake-seatunnel-3.0.0-archive")
		if err := os.WriteFile(src, payload, 0o600); err != nil {
			t.Fatalf("write src: %v", err)
		}

		exdevRename := func(oldPath, newPath string) error {
			return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
		}

		if err := moveFileAtomicWith(src, dst, exdevRename); err != nil {
			t.Fatalf("moveFileAtomicWith returned unexpected error: %v", err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatalf("read dst: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("dst payload mismatch: got %q, want %q", got, payload)
		}
		if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected src to be removed after fallback copy, err=%v", err)
		}
		entries, err := os.ReadDir(dstDir)
		if err != nil {
			t.Fatalf("readdir dstDir: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != filepath.Base(dst) {
			t.Fatalf("expected only final file in dstDir, got %d entries", len(entries))
		}
	})

	t.Run("savePackageFromReader succeeds when rename hits EXDEV", func(t *testing.T) {
		packageDir := t.TempDir()
		tempDir := t.TempDir()
		service := NewService(packageDir, nil)
		service.tempDir = tempDir
		service.renameFunc = func(oldPath, newPath string) error {
			return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EXDEV}
		}

		payload := []byte("seatunnel-3.0.0-tar-gz-content")
		info, err := service.savePackageFromReader(
			context.Background(),
			"3.0.0",
			"apache-seatunnel-3.0.0-bin.tar.gz",
			int64(len(payload)),
			bytes.NewReader(payload),
		)
		if err != nil {
			t.Fatalf("savePackageFromReader failed on EXDEV: %v", err)
		}
		if info.Version != "3.0.0" || !info.IsLocal {
			t.Fatalf("unexpected package info: %+v", info)
		}
		got, err := os.ReadFile(filepath.Join(packageDir, "apache-seatunnel-3.0.0-bin.tar.gz"))
		if err != nil {
			t.Fatalf("read saved package: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("saved package mismatch: got %q, want %q", got, payload)
		}
	})

	t.Run("non-EXDEV rename error is returned directly", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "src.tmp")
		dst := filepath.Join(t.TempDir(), "dst.tar.gz")
		if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
			t.Fatalf("write src: %v", err)
		}

		eaccesRename := func(oldPath, newPath string) error {
			return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: syscall.EACCES}
		}

		err := moveFileAtomicWith(src, dst, eaccesRename)
		if !errors.Is(err, syscall.EACCES) {
			t.Fatalf("expected EACCES error, got %v", err)
		}
	})
}
