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

package sync

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

const syncExecutionModule = "sync"

// ExecutionRequest 保存一次同步写操作的非敏感请求元数据。
// ExecutionRequest stores non-sensitive request metadata for one sync write operation.
type ExecutionRequest struct {
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	ClientType     string
}

// ListJobsForActor 按当前用户归属列出作业，管理员可查看全部。
// ListJobsForActor lists jobs owned by the current user, while administrators may view all jobs.
func (s *Service) ListJobsForActor(ctx context.Context, actor executionapp.Actor, filter *JobFilter) ([]*JobInstance, int64, error) {
	if filter == nil {
		filter = &JobFilter{}
	} else {
		clone := *filter
		filter = &clone
	}
	filter.OwnerUserID = uint(actor.UserID)
	filter.IncludeAll = actor.IsAdmin
	return s.ListJobs(ctx, filter)
}

// GetJobForActor 按当前用户归属读取一个作业。
// GetJobForActor loads one job under the current user's ownership scope.
func (s *Service) GetJobForActor(ctx context.Context, actor executionapp.Actor, id uint) (*JobInstance, error) {
	instance, err := s.repo.GetJobInstanceByIDForOwner(ctx, id, uint(actor.UserID), actor.IsAdmin)
	if err != nil {
		return nil, err
	}
	return s.refreshJobInstance(ctx, instance)
}

// GetJobLogsForActor 在归属检查后读取作业日志。
// GetJobLogsForActor reads job logs after enforcing ownership.
func (s *Service) GetJobLogsForActor(ctx context.Context, actor executionapp.Actor, id uint, offset string, limitBytes int, keyword, level string) (*JobLogsResult, error) {
	if _, err := s.repo.GetJobInstanceByIDForOwner(ctx, id, uint(actor.UserID), actor.IsAdmin); err != nil {
		return nil, err
	}
	return s.GetJobLogs(ctx, id, offset, limitBytes, keyword, level)
}

// GetPreviewSnapshotForActor 在归属检查后读取预览结果。
// GetPreviewSnapshotForActor reads a preview snapshot after enforcing ownership.
func (s *Service) GetPreviewSnapshotForActor(ctx context.Context, actor executionapp.Actor, id uint, tablePath string) (*PreviewSnapshot, error) {
	if _, err := s.repo.GetJobInstanceByIDForOwner(ctx, id, uint(actor.UserID), actor.IsAdmin); err != nil {
		return nil, err
	}
	return s.GetPreviewSnapshot(ctx, id, tablePath)
}

// GetJobCheckpointSnapshotForActor 在归属检查后读取 Checkpoint 信息。
// GetJobCheckpointSnapshotForActor reads checkpoint data after enforcing ownership.
func (s *Service) GetJobCheckpointSnapshotForActor(ctx context.Context, actor executionapp.Actor, id uint, pipelineID *int, limit int, status string) (*CheckpointSnapshot, error) {
	if _, err := s.repo.GetJobInstanceByIDForOwner(ctx, id, uint(actor.UserID), actor.IsAdmin); err != nil {
		return nil, err
	}
	return s.GetJobCheckpointSnapshot(ctx, id, pipelineID, limit, status)
}

// CancelJobForActor 在归属检查后请求取消作业。
// CancelJobForActor requests cancellation after enforcing ownership.
func (s *Service) CancelJobForActor(ctx context.Context, actor executionapp.Actor, id uint, stopWithSavepoint bool) (*JobInstance, error) {
	if _, err := s.repo.GetJobInstanceByIDForOwner(ctx, id, uint(actor.UserID), actor.IsAdmin); err != nil {
		return nil, err
	}
	return s.CancelJob(ctx, id, stopWithSavepoint)
}

// PreviewTaskWithExecution 创建公共执行记录后启动预览任务。
// PreviewTaskWithExecution starts a preview task after creating its shared execution record.
func (s *Service) PreviewTaskWithExecution(ctx context.Context, id, createdBy uint, opts *PreviewTaskRequest, request ExecutionRequest) (*JobInstance, error) {
	return s.runWithExecution(ctx, createdBy, "sync.task.preview", executionapp.RiskLevelR0, request, func(runCtx context.Context) (*JobInstance, error) {
		return s.PreviewTask(runCtx, id, createdBy, opts)
	})
}

// SubmitTaskWithExecution 创建公共执行记录后提交运行任务。
// SubmitTaskWithExecution submits a run task after creating its shared execution record.
func (s *Service) SubmitTaskWithExecution(ctx context.Context, id, createdBy uint, draft *TaskDraftPayload, request ExecutionRequest) (*JobInstance, error) {
	return s.runWithExecution(ctx, createdBy, "sync.task.submit", executionapp.RiskLevelR1, request, func(runCtx context.Context) (*JobInstance, error) {
		return s.SubmitTask(runCtx, id, createdBy, draft)
	})
}

// RecoverJobWithExecution 创建公共执行记录后恢复作业。
// RecoverJobWithExecution recovers a job after creating its shared execution record.
func (s *Service) RecoverJobWithExecution(ctx context.Context, sourceJobID, createdBy uint, draft *TaskDraftPayload, request ExecutionRequest) (*JobInstance, error) {
	return s.runWithExecution(ctx, createdBy, "sync.job.recover", executionapp.RiskLevelR1, request, func(runCtx context.Context) (*JobInstance, error) {
		return s.RecoverJob(runCtx, sourceJobID, createdBy, draft)
	})
}

func (s *Service) runWithExecution(ctx context.Context, ownerUserID uint, operationID string, risk executionapp.RiskLevel, request ExecutionRequest, run func(context.Context) (*JobInstance, error)) (*JobInstance, error) {
	if s.executionService == nil {
		return run(ctx)
	}
	item, created, err := s.executionService.Create(ctx, executionapp.CreateInput{
		OperationID:    operationID,
		OwnerUserID:    uint64(ownerUserID),
		ActorType:      executionActorType(ownerUserID),
		Module:         syncExecutionModule,
		RequestID:      strings.TrimSpace(request.RequestID),
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		RequestHash:    strings.TrimSpace(request.RequestHash),
		RiskLevel:      risk,
		Status:         executionapp.StatusPending,
		Cancellable:    true,
		ClientType:     strings.TrimSpace(request.ClientType),
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return s.jobForExistingExecution(ctx, item, ownerUserID)
	}

	runCtx := audit.WithCommandMetadata(ctx, audit.CommandMetadata{
		RequestID:   request.RequestID,
		ExecutionID: item.ExecutionID,
		OwnerUserID: ownerUserID,
	})
	job, runErr := run(runCtx)
	if runErr != nil {
		_ = s.executionService.Transition(ctx, item.ExecutionID, executionapp.StatusPending, executionapp.StatusFailed, map[string]any{
			"cancellable":   false,
			"error_code":    "sync_operation_failed",
			"error_message": "sync operation failed",
		})
		return nil, runErr
	}
	if job == nil {
		return nil, ErrJobInstanceNotFound
	}
	job.ExecutionID = item.ExecutionID
	if err := s.repo.UpdateJobInstance(ctx, job); err != nil {
		return nil, err
	}
	if err := s.executionService.BindModuleRef(ctx, item.ExecutionID, strconv.FormatUint(uint64(job.ID), 10)); err != nil {
		return nil, err
	}
	if err := s.syncExecutionFromJob(ctx, job); err != nil {
		return nil, err
	}
	return job, nil
}

func (s *Service) jobForExistingExecution(ctx context.Context, item *executionapp.Execution, ownerUserID uint) (*JobInstance, error) {
	if item == nil || strings.TrimSpace(item.ModuleRef) == "" {
		return nil, ErrJobStatusChanged
	}
	id, err := strconv.ParseUint(item.ModuleRef, 10, 64)
	if err != nil {
		return nil, ErrJobInstanceNotFound
	}
	return s.repo.GetJobInstanceByIDForOwner(ctx, uint(id), ownerUserID, false)
}

func executionActorType(ownerUserID uint) executionapp.ActorType {
	if ownerUserID == 0 {
		return executionapp.ActorTypeSystem
	}
	return executionapp.ActorTypeUser
}

func (s *Service) syncExecutionFromJob(ctx context.Context, job *JobInstance) error {
	if s.executionService == nil || job == nil || strings.TrimSpace(job.ExecutionID) == "" {
		return nil
	}
	actor := executionapp.Actor{UserID: uint64(job.CreatedBy), IsAdmin: job.CreatedBy == 0}
	item, err := s.executionService.Get(ctx, actor, job.ExecutionID)
	if err != nil {
		return err
	}
	target := executionStatusFromJob(job.Status)
	if item.Status == target {
		return nil
	}
	updates := map[string]any{
		"progress":           executionProgressFromJob(job.Status),
		"cancellable":        !isFinalNormalizedJobStatus(job.Status),
		"cancellable_reason": "",
		"result_ref":         fmt.Sprintf("sync/jobs/%d", job.ID),
	}
	if job.Status == JobStatusFailed {
		updates["error_code"] = "sync_job_failed"
		updates["error_message"] = "sync job failed"
	}
	for item.Status != target {
		next, ok := nextExecutionStatus(item.Status, target)
		if !ok {
			return executionapp.ErrInvalidTransition
		}
		if err := s.executionService.Transition(ctx, item.ExecutionID, item.Status, next, updates); err != nil {
			return err
		}
		item.Status = next
	}
	return nil
}

func nextExecutionStatus(current, target executionapp.Status) (executionapp.Status, bool) {
	if executionapp.CanTransition(current, target) {
		return target, true
	}
	switch current {
	case executionapp.StatusPending:
		if target == executionapp.StatusCancelling || target == executionapp.StatusCancelled {
			return executionapp.StatusCancelRequested, true
		}
		if target == executionapp.StatusSucceeded || target == executionapp.StatusTimedOut {
			return executionapp.StatusRunning, true
		}
	case executionapp.StatusRunning:
		if target == executionapp.StatusCancelling || target == executionapp.StatusCancelled {
			return executionapp.StatusCancelRequested, true
		}
	case executionapp.StatusCancelRequested:
		if target == executionapp.StatusCancelled {
			return executionapp.StatusCancelling, true
		}
	}
	return "", false
}

func executionStatusFromJob(status JobStatus) executionapp.Status {
	switch status {
	case JobStatusPending:
		return executionapp.StatusPending
	case JobStatusCancelRequested:
		return executionapp.StatusCancelRequested
	case JobStatusCancelling:
		return executionapp.StatusCancelling
	case JobStatusCanceled:
		return executionapp.StatusCancelled
	case JobStatusSuccess:
		return executionapp.StatusSucceeded
	case JobStatusFailed:
		return executionapp.StatusFailed
	default:
		return executionapp.StatusRunning
	}
}

func executionProgressFromJob(status JobStatus) int {
	switch status {
	case JobStatusPending:
		return 0
	case JobStatusSuccess, JobStatusFailed, JobStatusCanceled:
		return 100
	case JobStatusCancelRequested:
		return 50
	case JobStatusCancelling:
		return 75
	default:
		return 10
	}
}

// RequestCancel 实现公共执行协议的同步任务真实取消入口。
// RequestCancel implements the real cancellation entry for sync jobs under the shared execution contract.
func (s *Service) RequestCancel(ctx context.Context, item *executionapp.Execution, _ executionapp.Actor) (executionapp.CancelResult, error) {
	if item == nil || strings.TrimSpace(item.ModuleRef) == "" {
		return executionapp.CancelResult{}, ErrJobInstanceNotFound
	}
	id, err := strconv.ParseUint(item.ModuleRef, 10, 64)
	if err != nil {
		return executionapp.CancelResult{}, ErrJobInstanceNotFound
	}
	job, err := s.CancelJob(ctx, uint(id), false)
	if err != nil {
		return executionapp.CancelResult{}, err
	}
	return executionapp.CancelResult{
		Status:            executionStatusFromJob(job.Status),
		Cancellable:       !isFinalNormalizedJobStatus(job.Status),
		CancellableReason: "",
	}, nil
}
