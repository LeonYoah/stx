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
	"errors"
	"io"
	"testing"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUploadPackageWithExecutionRequiresConfirmAndIsIdempotent(t *testing.T) {
	service := newInstallerExecutionTestService(t)
	actor := executionapp.Actor{UserID: 7}
	file := createUploadFileHeader(t, "file", "apache-seatunnel-9.9.91-bin.tar.gz", []byte("package-data"))
	request := ExecutionRequest{RequestID: "req-1", IdempotencyKey: "upload-key", RequestHash: "hash-1", ClientType: "cli"}

	if _, err := service.UploadPackageWithExecution(context.Background(), actor, "9.9.91", file, request); !errors.Is(err, executionapp.ErrExplicitConfirmNeeded) {
		t.Fatalf("缺少确认时错误不正确 / unexpected error without confirmation: %v", err)
	}
	request.Confirmed = true
	first, err := service.UploadPackageWithExecution(context.Background(), actor, "9.9.91", file, request)
	if err != nil || first == nil || !first.IsLocal {
		t.Fatalf("首次上传失败 / first upload failed: result=%#v err=%v", first, err)
	}
	second, err := service.UploadPackageWithExecution(context.Background(), actor, "9.9.91", file, request)
	if err != nil || second == nil || second.Checksum != first.Checksum {
		t.Fatalf("幂等重试没有返回原结果 / idempotent retry did not return original result: result=%#v err=%v", second, err)
	}

	request.RequestHash = "different-hash"
	if _, err := service.UploadPackageWithExecution(context.Background(), actor, "9.9.91", file, request); !errors.Is(err, executionapp.ErrIdempotencyConflict) {
		t.Fatalf("复用幂等键时没有拒绝不同请求 / different request was not rejected: %v", err)
	}
}

func TestHashMultipartFileDistinguishesEqualSizedContentAndKeepsFileReadable(t *testing.T) {
	first := createUploadFileHeader(t, "file", "apache-seatunnel-9.9.91-bin.tar.gz", []byte("content-a"))
	second := createUploadFileHeader(t, "file", "apache-seatunnel-9.9.91-bin.tar.gz", []byte("content-b"))

	firstHash, err := hashMultipartFile(first)
	if err != nil {
		t.Fatalf("计算第一个文件摘要失败 / hash first file: %v", err)
	}
	secondHash, err := hashMultipartFile(second)
	if err != nil {
		t.Fatalf("计算第二个文件摘要失败 / hash second file: %v", err)
	}
	if first.Size != second.Size || firstHash == secondHash {
		t.Fatalf("同大小不同内容必须生成不同摘要 / equal-sized different content must have different hashes: first=%s second=%s", firstHash, secondHash)
	}

	reader, err := first.Open()
	if err != nil {
		t.Fatalf("摘要计算后重新打开文件失败 / reopen file after hashing: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil || string(content) != "content-a" {
		t.Fatalf("摘要计算后文件内容不可读 / file unreadable after hashing: content=%q err=%v", content, err)
	}
}

func TestFinishPackageOperationCompletesAfterRequestCancellation(t *testing.T) {
	service := newInstallerExecutionTestService(t)
	item, created, err := service.executionService.Create(context.Background(), executionapp.CreateInput{
		OperationID:    "package.source.fetch",
		OwnerUserID:    7,
		ActorType:      executionapp.ActorTypeUser,
		Module:         installerExecutionModule,
		ModuleRef:      "2.3.13",
		RequestID:      "request-cancelled",
		IdempotencyKey: "request-cancelled-key",
		RequestHash:    "request-cancelled-hash",
		RiskLevel:      executionapp.RiskLevelR1,
		Status:         executionapp.StatusRunning,
		Cancellable:    false,
		ClientType:     "web",
	})
	if err != nil || !created || item == nil {
		t.Fatalf("创建测试执行记录失败 / failed to create execution record: item=%#v created=%t err=%v", item, created, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service.finishPackageOperation(ctx, item, errors.New("source download cancelled"), "2.3.13")

	finished, err := service.executionService.Get(context.Background(), executionapp.Actor{UserID: 7}, item.ExecutionID)
	if err != nil {
		t.Fatalf("读取收尾后的执行记录失败 / failed to read finished execution: %v", err)
	}
	if finished.Status != executionapp.StatusFailed || finished.ErrorMessage != "source download cancelled" {
		t.Fatalf("请求断开后没有写入失败终态 / final failed state was not persisted after request cancellation: status=%s error=%q", finished.Status, finished.ErrorMessage)
	}
}

func newInstallerExecutionTestService(t *testing.T) *Service {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("创建测试数据库失败 / open test database failed: %v", err)
	}
	if err := database.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("迁移测试数据库失败 / migrate test database failed: %v", err)
	}
	providers := executionapp.NewProviderRegistry()
	executionService := executionapp.NewService(executionapp.NewRepository(database), providers)
	service := NewService(t.TempDir(), nil)
	service.tempDir = t.TempDir()
	service.SetExecutionService(executionService)
	providers.Register(installerExecutionModule, service)
	return service
}
