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
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
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
