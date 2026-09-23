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
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

const installerExecutionModule = "installer"

// ExecutionRequest 保存安装包写操作的公共执行参数。
// ExecutionRequest stores shared execution parameters for package write operations.
type ExecutionRequest struct {
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	Confirmed      bool
	ConfirmationID string
	ClientType     string
}

// chunkExecutionResult 保存可安全放入公共执行记录的分片结果摘要。
// chunkExecutionResult stores a compact chunk result safe for a shared execution record.
type chunkExecutionResult struct {
	UploadID       string `json:"upload_id"`
	Completed      bool   `json:"completed"`
	ReceivedChunks int    `json:"received_chunks"`
	TotalChunks    int    `json:"total_chunks"`
	PackageVersion string `json:"package_version,omitempty"`
}

// UploadPackageWithExecution 在公共确认和幂等规则下上传安装包。
// UploadPackageWithExecution uploads a package under shared confirmation and idempotency rules.
func (s *Service) UploadPackageWithExecution(ctx context.Context, actor executionapp.Actor, version string, file *multipart.FileHeader, request ExecutionRequest) (*PackageInfo, error) {
	return s.UploadPackageBundleWithExecution(ctx, actor, version, file, nil, request)
}

// UploadPackageBundleWithExecution 上传运行包，并可在同一次请求中附带源码包。
// UploadPackageBundleWithExecution uploads the runtime package with an optional source archive in the same request.
func (s *Service) UploadPackageBundleWithExecution(ctx context.Context, actor executionapp.Actor, version string, file, sourceFile *multipart.FileHeader, request ExecutionRequest) (*PackageInfo, error) {
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.upload", version, executionapp.RiskLevelR1,
		"上传会把安装包写入 STX 本地存储；同版本文件不会被覆盖。", request, false)
	if err != nil {
		return nil, err
	}
	if existing {
		return s.packageResultForExecution(ctx, item)
	}
	result, runErr := s.UploadPackageBundle(ctx, version, file, sourceFile)
	s.finishPackageOperation(ctx, item, runErr, version)
	return result, runErr
}

// UploadSourcePackageWithExecution 为已有运行包补充或替换源码包。
// UploadSourcePackageWithExecution supplements or replaces source for an existing runtime package.
func (s *Service) UploadSourcePackageWithExecution(ctx context.Context, actor executionapp.Actor, version string, file *multipart.FileHeader, request ExecutionRequest) (*PackageInfo, error) {
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.source.upload", version, executionapp.RiskLevelR1,
		"上传会写入或替换该版本的源码包，不会修改运行包。", request, false)
	if err != nil {
		return nil, err
	}
	if existing {
		return s.packageResultForExecution(ctx, item)
	}
	result, runErr := s.UploadSourcePackage(ctx, version, file)
	s.finishPackageOperation(ctx, item, runErr, version)
	return result, runErr
}

// FetchSourcePackageWithExecution 从镜像为已有运行包补充源码包。
// FetchSourcePackageWithExecution fetches source from a mirror for an existing runtime package.
func (s *Service) FetchSourcePackageWithExecution(ctx context.Context, actor executionapp.Actor, version string, mirror MirrorSource, request ExecutionRequest) (*PackageInfo, error) {
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.source.fetch", version, executionapp.RiskLevelR1,
		"下载源码会占用 STX 服务器网络带宽和磁盘空间，但不会修改运行包。", request, false)
	if err != nil {
		return nil, err
	}
	if existing {
		return s.packageResultForExecution(ctx, item)
	}
	result, runErr := s.FetchSourcePackage(ctx, version, mirror)
	s.finishPackageOperation(ctx, item, runErr, version)
	return result, runErr
}

// UploadPackageChunkWithExecution 在公共确认和幂等规则下上传一个分片。
// UploadPackageChunkWithExecution uploads one chunk under shared confirmation and idempotency rules.
func (s *Service) UploadPackageChunkWithExecution(ctx context.Context, actor executionapp.Actor, upload *PackageChunkUploadRequest, file *multipart.FileHeader, request ExecutionRequest) (*PackageChunkUploadResult, error) {
	moduleRef := strings.TrimSpace(upload.UploadID)
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.upload.chunk", moduleRef, executionapp.RiskLevelR1,
		"上传分片会写入 STX 临时目录，最后一个分片会生成本地安装包。", request, false)
	if err != nil {
		return nil, err
	}
	if existing {
		var stored chunkExecutionResult
		if decodeErr := json.Unmarshal([]byte(item.ResultRef), &stored); decodeErr != nil {
			return nil, executionapp.ErrConcurrentUpdate
		}
		result := &PackageChunkUploadResult{
			UploadID: stored.UploadID, Completed: stored.Completed,
			ReceivedChunks: stored.ReceivedChunks, TotalChunks: stored.TotalChunks,
		}
		if stored.Completed && stored.PackageVersion != "" {
			result.Package, _ = s.GetPackageInfo(ctx, stored.PackageVersion)
		}
		return result, nil
	}
	result, runErr := s.UploadPackageChunk(ctx, upload, file)
	resultRef := ""
	if result != nil {
		stored := chunkExecutionResult{
			UploadID: result.UploadID, Completed: result.Completed,
			ReceivedChunks: result.ReceivedChunks, TotalChunks: result.TotalChunks,
		}
		if result.Package != nil {
			stored.PackageVersion = result.Package.Version
		}
		if content, marshalErr := json.Marshal(stored); marshalErr == nil {
			resultRef = string(content)
		}
	}
	s.finishPackageOperation(ctx, item, runErr, resultRef)
	return result, runErr
}

// DeletePackageWithExecution 在公共确认和幂等规则下删除安装包。
// DeletePackageWithExecution deletes a package under shared confirmation and idempotency rules.
func (s *Service) DeletePackageWithExecution(ctx context.Context, actor executionapp.Actor, version string, request ExecutionRequest) error {
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.delete", version, executionapp.RiskLevelR1,
		"删除后本地安装包无法恢复，后续安装或升级可能需要重新上传或下载。", request, false)
	if err != nil || existing {
		return err
	}
	runErr := s.DeletePackage(ctx, version)
	s.finishPackageOperation(ctx, item, runErr, version)
	return runErr
}

// StartDownloadWithExecution 在公共确认和幂等规则下启动安装包下载。
// StartDownloadWithExecution starts a package download under shared confirmation and idempotency rules.
func (s *Service) StartDownloadWithExecution(ctx context.Context, actor executionapp.Actor, download *DownloadRequest, request ExecutionRequest) (*DownloadTask, error) {
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.download.start", download.Version, executionapp.RiskLevelR1,
		"下载会占用 STX 服务器网络带宽和磁盘空间。", request, true)
	if err != nil {
		return nil, err
	}
	if existing {
		return s.GetDownloadStatusForActor(ctx, actor, item.ModuleRef)
	}
	task, runErr := s.startDownload(ctx, download, item.ExecutionID, actor.UserID)
	if runErr != nil {
		s.finishPackageOperation(ctx, item, runErr, download.Version)
		return task, runErr
	}
	return task, nil
}

// CancelDownloadWithExecution 在公共确认和幂等规则下请求真实停止下载。
// CancelDownloadWithExecution requests a real download stop under shared confirmation and idempotency rules.
func (s *Service) CancelDownloadWithExecution(ctx context.Context, actor executionapp.Actor, version string, request ExecutionRequest) (*DownloadTask, error) {
	item, existing, err := s.beginPackageExecution(ctx, actor, "package.download.cancel", version, executionapp.RiskLevelR1,
		"取消会停止当前网络下载并删除未完成的临时文件。", request, false)
	if err != nil {
		return nil, err
	}
	if existing {
		return s.GetDownloadStatusForActor(ctx, actor, version)
	}
	task, loadErr := s.GetDownloadStatusForActor(ctx, actor, version)
	if loadErr != nil {
		s.finishPackageOperation(ctx, item, loadErr, version)
		return nil, loadErr
	}
	if strings.TrimSpace(task.ExecutionID) == "" {
		runErr := executionapp.ErrNotCancellable
		s.finishPackageOperation(ctx, item, runErr, version)
		return nil, runErr
	}
	_, runErr := s.executionService.Cancel(ctx, actor, task.ExecutionID)
	result, resultErr := s.GetDownloadStatusForActor(ctx, actor, version)
	if runErr == nil {
		runErr = resultErr
	}
	s.finishPackageOperation(ctx, item, runErr, version)
	return result, runErr
}

func (s *Service) beginPackageExecution(ctx context.Context, actor executionapp.Actor, operationID, moduleRef string, risk executionapp.RiskLevel, impact string, request ExecutionRequest, cancellable bool) (*executionapp.Execution, bool, error) {
	if s.executionService == nil {
		return nil, false, fmt.Errorf("shared execution service is unavailable")
	}
	if existing, found, err := s.executionService.FindIdempotent(ctx, actor, operationID, request.IdempotencyKey, request.RequestHash); err != nil || found {
		if err != nil {
			return nil, false, err
		}
		if existing.Status == executionapp.StatusFailed {
			return nil, false, fmt.Errorf("previous request failed: %s", existing.ErrorMessage)
		}
		return existing, true, nil
	}
	if err := s.executionService.Authorize(ctx, actor, executionapp.AuthorizationInput{
		OperationID: operationID, RiskLevel: risk, Impact: impact,
		IdempotencyKey: request.IdempotencyKey, RequestHash: request.RequestHash,
		Confirmed: request.Confirmed, ConfirmationID: request.ConfirmationID,
	}); err != nil {
		return nil, false, err
	}
	item, created, err := s.executionService.Create(ctx, executionapp.CreateInput{
		OperationID: operationID, OwnerUserID: actor.UserID, ActorType: executionapp.ActorTypeUser,
		Module: installerExecutionModule, ModuleRef: strings.TrimSpace(moduleRef), RequestID: request.RequestID,
		IdempotencyKey: request.IdempotencyKey, RequestHash: request.RequestHash, RiskLevel: risk,
		Status: executionapp.StatusRunning, Cancellable: cancellable, ClientType: request.ClientType,
	})
	if err != nil {
		return nil, false, err
	}
	return item, !created, nil
}

func (s *Service) packageResultForExecution(ctx context.Context, item *executionapp.Execution) (*PackageInfo, error) {
	if item == nil || item.Status != executionapp.StatusSucceeded {
		return nil, executionapp.ErrConcurrentUpdate
	}
	return s.GetPackageInfo(ctx, item.ModuleRef)
}

func (s *Service) finishPackageOperation(ctx context.Context, item *executionapp.Execution, runErr error, resultRef string) {
	if s.executionService == nil || item == nil {
		return
	}
	// 请求断开后仍必须写入最终执行状态，避免审计和执行记录永久停在 running。
	// The final execution state must be written even after the request disconnects, instead of leaving audit and execution records stuck in running.
	finishCtx := context.WithoutCancel(ctx)
	target := executionapp.StatusSucceeded
	updates := map[string]any{"result_ref": strings.TrimSpace(resultRef), "cancellable": false}
	if runErr != nil {
		target = executionapp.StatusFailed
		updates["error_message"] = runErr.Error()
	}
	_ = s.executionService.Transition(finishCtx, item.ExecutionID, item.Status, target, updates)
}

// GetDownloadStatusForActor 仅向任务所有者或管理员返回下载任务。
// GetDownloadStatusForActor returns a download only to its owner or an administrator.
func (s *Service) GetDownloadStatusForActor(ctx context.Context, actor executionapp.Actor, version string) (*DownloadTask, error) {
	task, err := s.GetDownloadStatus(ctx, version)
	if err != nil {
		return nil, err
	}
	if task.OwnerUserID != 0 && task.OwnerUserID != actor.UserID && !actor.IsAdmin {
		return nil, executionapp.ErrPermissionDenied
	}
	return task, nil
}

// ListDownloadsForActor 返回当前用户的下载任务，管理员可以查看全部任务。
// ListDownloadsForActor returns owned downloads, while administrators can inspect all downloads.
func (s *Service) ListDownloadsForActor(ctx context.Context, actor executionapp.Actor) []*DownloadTask {
	tasks := s.ListDownloads(ctx)
	if actor.IsAdmin {
		return tasks
	}
	result := make([]*DownloadTask, 0, len(tasks))
	for _, task := range tasks {
		if task.OwnerUserID == 0 || task.OwnerUserID == actor.UserID {
			result = append(result, task)
		}
	}
	return result
}
