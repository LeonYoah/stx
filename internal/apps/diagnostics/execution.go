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

package diagnostics

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

const diagnosticExecutionModule = "diagnostics"

// DiagnosticExecutionRequest 保存诊断写操作的非敏感请求元数据。
// DiagnosticExecutionRequest stores non-sensitive request metadata for a diagnostic write operation.
type DiagnosticExecutionRequest struct {
	OperationID    string
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	Confirmed      bool
	ConfirmationID string
	IsAdmin        bool
	ClientType     string
}

// ListDiagnosticTasksForActor 按当前用户归属列出诊断任务，管理员可查看全部。
// ListDiagnosticTasksForActor lists diagnostic tasks owned by the current user, while administrators may view all tasks.
func (s *Service) ListDiagnosticTasksForActor(ctx context.Context, actor executionapp.Actor, filter *DiagnosticTaskListFilter) ([]*DiagnosticTaskSummary, int64, error) {
	if err := validateDiagnosticActor(actor); err != nil {
		return nil, 0, err
	}
	if filter == nil {
		filter = &DiagnosticTaskListFilter{}
	} else {
		clone := *filter
		filter = &clone
	}
	filter.OwnerUserID = uint(actor.UserID)
	filter.IncludeAll = actor.IsAdmin
	return s.ListDiagnosticTasks(ctx, filter)
}

// GetDiagnosticTaskForActor 按当前用户归属读取诊断任务。
// GetDiagnosticTaskForActor loads a diagnostic task under the current user's ownership scope.
func (s *Service) GetDiagnosticTaskForActor(ctx context.Context, actor executionapp.Actor, taskID uint) (*DiagnosticTask, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if err := validateDiagnosticActor(actor); err != nil {
		return nil, err
	}
	return s.repo.GetDiagnosticTaskByIDForOwner(ctx, taskID, uint(actor.UserID), actor.IsAdmin)
}

// ListDiagnosticTaskStepsForActor 在归属检查后读取诊断步骤。
// ListDiagnosticTaskStepsForActor reads diagnostic task steps after enforcing ownership.
func (s *Service) ListDiagnosticTaskStepsForActor(ctx context.Context, actor executionapp.Actor, taskID uint) ([]*DiagnosticTaskStep, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if err := validateDiagnosticActor(actor); err != nil {
		return nil, err
	}
	return s.repo.ListDiagnosticTaskStepsForOwner(ctx, taskID, uint(actor.UserID), actor.IsAdmin)
}

// ListDiagnosticStepLogsForActor 在归属检查后读取诊断日志。
// ListDiagnosticStepLogsForActor reads diagnostic task logs after enforcing ownership.
func (s *Service) ListDiagnosticStepLogsForActor(ctx context.Context, actor executionapp.Actor, filter *DiagnosticTaskLogFilter) ([]*DiagnosticStepLog, int64, error) {
	if err := validateDiagnosticActor(actor); err != nil {
		return nil, 0, err
	}
	if filter == nil {
		filter = &DiagnosticTaskLogFilter{}
	} else {
		clone := *filter
		filter = &clone
	}
	if filter.TaskID > 0 {
		if _, err := s.GetDiagnosticTaskForActor(ctx, actor, filter.TaskID); err != nil {
			return nil, 0, err
		}
	}
	filter.OwnerUserID = uint(actor.UserID)
	filter.IncludeAll = actor.IsAdmin
	return s.ListDiagnosticStepLogs(ctx, filter)
}

// StartDiagnosticTaskForActor 在归属检查后启动诊断任务。
// StartDiagnosticTaskForActor starts a diagnostic task after enforcing ownership.
func (s *Service) StartDiagnosticTaskForActor(ctx context.Context, actor executionapp.Actor, taskID uint) error {
	if err := validateDiagnosticActor(actor); err != nil {
		return err
	}
	if _, err := s.GetDiagnosticTaskForActor(ctx, actor, taskID); err != nil {
		return err
	}
	return s.StartDiagnosticTask(ctx, taskID)
}

// CreateDiagnosticTaskWithExecution 创建公共执行记录后创建诊断任务。
// CreateDiagnosticTaskWithExecution creates a diagnostic task after creating its shared execution record.
func (s *Service) CreateDiagnosticTaskWithExecution(ctx context.Context, req *CreateDiagnosticTaskRequest, createdBy uint, createdByName string, request DiagnosticExecutionRequest) (*DiagnosticTask, error) {
	if s == nil || s.executionService == nil {
		return s.CreateDiagnosticTask(ctx, req, createdBy, createdByName)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidDiagnosticTaskRequest)
	}
	operationID := strings.TrimSpace(request.OperationID)
	if operationID == "" {
		operationID = "diagnostics.task.create"
	}
	riskLevel := diagnosticTaskRiskLevel(req)
	actor := executionapp.Actor{UserID: uint64(createdBy), IsAdmin: request.IsAdmin || createdBy == 0}
	if riskLevel == executionapp.RiskLevelR3 && !actor.IsAdmin {
		return nil, executionapp.ErrAdminRequired
	}
	if existing, found, err := s.executionService.FindIdempotent(ctx, actor, operationID, request.IdempotencyKey, request.RequestHash); err != nil {
		return nil, err
	} else if found {
		return s.diagnosticTaskForExistingExecution(ctx, existing)
	}
	if createdBy != 0 {
		if err := s.executionService.Authorize(ctx, actor, executionapp.AuthorizationInput{
			OperationID:    operationID,
			RiskLevel:      riskLevel,
			Impact:         diagnosticTaskImpact(req),
			IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
			RequestHash:    strings.TrimSpace(request.RequestHash),
			Confirmed:      request.Confirmed,
			ConfirmationID: strings.TrimSpace(request.ConfirmationID),
		}); err != nil {
			return nil, err
		}
	}
	item, created, err := s.executionService.Create(ctx, executionapp.CreateInput{
		OperationID:    operationID,
		OwnerUserID:    uint64(createdBy),
		ActorType:      diagnosticExecutionActorType(createdBy),
		Module:         diagnosticExecutionModule,
		RequestID:      strings.TrimSpace(request.RequestID),
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		RequestHash:    strings.TrimSpace(request.RequestHash),
		RiskLevel:      riskLevel,
		Status:         executionapp.StatusPending,
		Cancellable:    true,
		ClientType:     strings.TrimSpace(request.ClientType),
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return s.diagnosticTaskForExistingExecution(ctx, item)
	}

	requestCopy := *req
	autoStart := requestCopy.AutoStart
	requestCopy.AutoStart = false
	task, createErr := s.CreateDiagnosticTask(ctx, &requestCopy, createdBy, createdByName)
	if createErr != nil {
		_ = s.executionService.Transition(ctx, item.ExecutionID, executionapp.StatusPending, executionapp.StatusFailed, map[string]any{
			"cancellable":   false,
			"error_code":    "diagnostic_task_create_failed",
			"error_message": "diagnostic task creation failed",
		})
		return nil, createErr
	}

	task.ExecutionID = item.ExecutionID
	if err := s.repo.UpdateDiagnosticTask(ctx, task); err != nil {
		return nil, err
	}
	if err := s.executionService.BindModuleRef(ctx, item.ExecutionID, strconv.FormatUint(uint64(task.ID), 10)); err != nil {
		return nil, err
	}
	if autoStart {
		if err := s.StartDiagnosticTask(ctx, task.ID); err != nil {
			return nil, err
		}
	}
	return s.repo.GetDiagnosticTaskByID(ctx, task.ID)
}

func (s *Service) diagnosticTaskForExistingExecution(ctx context.Context, item *executionapp.Execution) (*DiagnosticTask, error) {
	if item == nil || strings.TrimSpace(item.ModuleRef) == "" {
		return nil, executionapp.ErrConcurrentUpdate
	}
	taskID, err := strconv.ParseUint(item.ModuleRef, 10, 64)
	if err != nil {
		return nil, ErrDiagnosticTaskNotFound
	}
	return s.repo.GetDiagnosticTaskByID(ctx, uint(taskID))
}

func diagnosticExecutionActorType(ownerUserID uint) executionapp.ActorType {
	if ownerUserID == 0 {
		return executionapp.ActorTypeSystem
	}
	return executionapp.ActorTypeUser
}

func validateDiagnosticActor(actor executionapp.Actor) error {
	if actor.UserID == 0 && !actor.IsAdmin {
		return executionapp.ErrPermissionDenied
	}
	return nil
}

func diagnosticTaskRiskLevel(req *CreateDiagnosticTaskRequest) executionapp.RiskLevel {
	if req == nil {
		return executionapp.RiskLevelR0
	}
	options := req.Options.Normalize()
	if options.IncludeJVMDump {
		return executionapp.RiskLevelR3
	}
	if options.IncludeThreadDump {
		return executionapp.RiskLevelR1
	}
	return executionapp.RiskLevelR0
}

// diagnosticTaskImpact 返回与实际所选资源一致的确认提示。
// diagnosticTaskImpact returns a confirmation message that matches the selected resources.
func diagnosticTaskImpact(req *CreateDiagnosticTaskRequest) string {
	if req == nil {
		return "诊断任务会读取集群和节点信息。"
	}
	options := req.Options.Normalize()
	if options.IncludeJVMDump {
		return "JVM Dump 可能触发 Full GC、暂停目标 Java 进程并产生较大的文件，只允许管理员确认后执行。"
	}
	if options.IncludeThreadDump {
		return "线程快照会执行 jcmd 或 jstack，可能短暂增加目标 JVM 和主机负载。"
	}
	return "诊断任务会读取集群、配置、日志或监控信息，不修改 SeaTunnel 运行状态。"
}

// RequestCancel 在当前步骤结束后停止诊断任务，不中断已经发给 Agent 的命令。
// RequestCancel stops a diagnostic task after the current step without interrupting a command already sent to an Agent.
func (s *Service) RequestCancel(ctx context.Context, item *executionapp.Execution, actor executionapp.Actor) (executionapp.CancelResult, error) {
	if s == nil || s.repo == nil || item == nil {
		return executionapp.CancelResult{}, ErrDiagnosticsRepositoryUnavailable
	}
	taskID, err := strconv.ParseUint(strings.TrimSpace(item.ModuleRef), 10, 64)
	if err != nil || taskID == 0 {
		return executionapp.CancelResult{}, ErrDiagnosticTaskNotFound
	}
	task, err := s.repo.GetDiagnosticTaskByIDForOwner(ctx, uint(taskID), uint(item.OwnerUserID), actor.IsAdmin)
	if err != nil {
		return executionapp.CancelResult{}, err
	}

	switch task.Status {
	case DiagnosticTaskStatusPending, DiagnosticTaskStatusReady:
		if err := s.cancelDiagnosticTaskImmediately(ctx, task); err != nil {
			return executionapp.CancelResult{}, err
		}
		return executionapp.CancelResult{Status: executionapp.StatusCancelled, Cancellable: false, CancellableReason: "diagnostic task cancelled"}, nil
	case DiagnosticTaskStatusRunning:
		updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusRunning}, map[string]any{
			"status":     DiagnosticTaskStatusCancelRequested,
			"summary":    bilingualText("已请求取消，将在当前步骤完成后停止。", "Cancellation requested; the task will stop after the current step."),
			"updated_at": time.Now().UTC(),
		})
		if err != nil {
			return executionapp.CancelResult{}, err
		}
		if updated {
			s.publishDiagnosticTaskByID(ctx, task.ID)
		}
		return executionapp.CancelResult{Status: executionapp.StatusCancelling, Cancellable: false, CancellableReason: "waiting for the current diagnostic step to finish"}, nil
	case DiagnosticTaskStatusCancelRequested, DiagnosticTaskStatusCancelling:
		return executionapp.CancelResult{Status: executionapp.StatusCancelling, Cancellable: false, CancellableReason: "waiting for the current diagnostic step to finish"}, nil
	case DiagnosticTaskStatusCancelled:
		return executionapp.CancelResult{Status: executionapp.StatusCancelled, Cancellable: false, CancellableReason: "diagnostic task cancelled"}, nil
	case DiagnosticTaskStatusSucceeded:
		return executionapp.CancelResult{Status: executionapp.StatusSucceeded, Cancellable: false, CancellableReason: "diagnostic task already succeeded"}, nil
	case DiagnosticTaskStatusFailed:
		return executionapp.CancelResult{Status: executionapp.StatusFailed, Cancellable: false, CancellableReason: "diagnostic task already failed"}, nil
	default:
		return executionapp.CancelResult{}, fmt.Errorf("%w: unsupported status %s", ErrInvalidDiagnosticTaskRequest, task.Status)
	}
}

func (s *Service) cancelDiagnosticTaskImmediately(ctx context.Context, task *DiagnosticTask) error {
	now := time.Now().UTC()
	updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusPending, DiagnosticTaskStatusReady}, map[string]any{
		"status":         DiagnosticTaskStatusCancelled,
		"summary":        bilingualText("诊断任务已在开始前取消。", "Diagnostic task cancelled before it started."),
		"completed_at":   now,
		"updated_at":     now,
		"failure_step":   "",
		"failure_reason": "",
	})
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	if err := s.skipPendingDiagnosticTaskWork(ctx, task.ID, now); err != nil {
		return err
	}
	s.publishDiagnosticTaskByID(ctx, task.ID)
	return nil
}

func (s *Service) skipPendingDiagnosticTaskWork(ctx context.Context, taskID uint, completedAt time.Time) error {
	steps, err := s.repo.ListDiagnosticTaskSteps(ctx, taskID)
	if err != nil {
		return err
	}
	message := bilingualText("任务取消，未执行此步骤。", "Task cancelled before this step started.")
	for _, step := range steps {
		if step == nil || step.Status != DiagnosticTaskStatusPending {
			continue
		}
		step.Status = DiagnosticTaskStatusSkipped
		step.Message = message
		step.CompletedAt = &completedAt
		if err := s.UpdateDiagnosticTaskStep(ctx, step); err != nil {
			return err
		}
	}
	return s.repo.SkipPendingDiagnosticNodeExecutions(ctx, taskID, message, completedAt)
}

// cancelDiagnosticTaskAtBoundary 在安全步骤边界确认取消并停止后续步骤。
// cancelDiagnosticTaskAtBoundary confirms cancellation at a safe step boundary and prevents later steps from starting.
func (s *Service) cancelDiagnosticTaskAtBoundary(ctx context.Context, taskID uint) (bool, error) {
	task, err := s.repo.GetDiagnosticTaskByID(ctx, taskID)
	if err != nil {
		return false, err
	}
	if task.Status != DiagnosticTaskStatusCancelRequested && task.Status != DiagnosticTaskStatusCancelling {
		return false, nil
	}

	now := time.Now().UTC()
	if task.Status == DiagnosticTaskStatusCancelRequested {
		updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusCancelRequested}, map[string]any{
			"status":     DiagnosticTaskStatusCancelling,
			"summary":    bilingualText("当前步骤已结束，正在停止后续诊断步骤。", "The current step has finished and later diagnostic steps are being stopped."),
			"updated_at": now,
		})
		if err != nil {
			return false, err
		}
		if updated {
			task.Status = DiagnosticTaskStatusCancelling
			s.publishDiagnosticTaskByID(ctx, task.ID)
		} else {
			task, err = s.repo.GetDiagnosticTaskByID(ctx, task.ID)
			if err != nil {
				return false, err
			}
			if task.Status == DiagnosticTaskStatusCancelled {
				return true, s.syncExecutionFromDiagnosticTask(ctx, task)
			}
			if task.Status != DiagnosticTaskStatusCancelling {
				return false, nil
			}
		}
	}

	if err := s.skipPendingDiagnosticTaskWork(ctx, task.ID, now); err != nil {
		return false, err
	}
	updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusCancelling, DiagnosticTaskStatusCancelRequested}, map[string]any{
		"status":         DiagnosticTaskStatusCancelled,
		"summary":        bilingualText("诊断任务已取消，未再执行后续步骤。", "Diagnostic task cancelled; later steps were not executed."),
		"completed_at":   now,
		"updated_at":     now,
		"failure_step":   "",
		"failure_reason": "",
	})
	if err != nil {
		return false, err
	}
	if !updated {
		latest, loadErr := s.repo.GetDiagnosticTaskByID(ctx, task.ID)
		if loadErr != nil {
			return false, loadErr
		}
		if latest.Status != DiagnosticTaskStatusCancelled {
			return false, nil
		}
		task = latest
	} else {
		task.Status = DiagnosticTaskStatusCancelled
		task.CompletedAt = &now
		task.UpdatedAt = now
		task.Summary = bilingualText("诊断任务已取消，未再执行后续步骤。", "Diagnostic task cancelled; later steps were not executed.")
		s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	}
	return true, s.syncExecutionFromDiagnosticTask(ctx, task)
}

func (s *Service) publishDiagnosticTaskByID(ctx context.Context, taskID uint) {
	task, err := s.repo.GetDiagnosticTaskByID(ctx, taskID)
	if err == nil {
		s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	}
}

func (s *Service) syncExecutionFromDiagnosticTask(ctx context.Context, task *DiagnosticTask) error {
	if s == nil || s.executionService == nil || task == nil || strings.TrimSpace(task.ExecutionID) == "" {
		return nil
	}
	actor := executionapp.Actor{UserID: uint64(task.CreatedBy), IsAdmin: task.CreatedBy == 0}
	item, err := s.executionService.Get(ctx, actor, task.ExecutionID)
	if err != nil {
		return err
	}
	target := diagnosticExecutionStatus(task.Status)
	if item.Status == target || executionapp.IsTerminal(item.Status) {
		return nil
	}
	if (item.Status == executionapp.StatusCancelRequested || item.Status == executionapp.StatusCancelling) && target == executionapp.StatusRunning {
		return nil
	}
	updates := map[string]any{
		"progress":           diagnosticExecutionProgress(task),
		"cancellable":        !isTerminalDiagnosticTaskStatus(task.Status),
		"cancellable_reason": "",
		"result_ref":         fmt.Sprintf("diagnostics/tasks/%d", task.ID),
	}
	if task.Status == DiagnosticTaskStatusFailed {
		updates["error_code"] = "diagnostic_task_failed"
		updates["error_message"] = "diagnostic task failed"
	}
	for item.Status != target {
		next, ok := nextDiagnosticExecutionStatus(item.Status, target)
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

func diagnosticExecutionStatus(status DiagnosticTaskStatus) executionapp.Status {
	switch status {
	case DiagnosticTaskStatusPending, DiagnosticTaskStatusReady:
		return executionapp.StatusPending
	case DiagnosticTaskStatusRunning:
		return executionapp.StatusRunning
	case DiagnosticTaskStatusCancelRequested:
		return executionapp.StatusCancelRequested
	case DiagnosticTaskStatusCancelling:
		return executionapp.StatusCancelling
	case DiagnosticTaskStatusCancelled:
		return executionapp.StatusCancelled
	case DiagnosticTaskStatusSucceeded:
		return executionapp.StatusSucceeded
	case DiagnosticTaskStatusFailed:
		return executionapp.StatusFailed
	default:
		return executionapp.StatusFailed
	}
}

func nextDiagnosticExecutionStatus(current, target executionapp.Status) (executionapp.Status, bool) {
	if executionapp.CanTransition(current, target) {
		return target, true
	}
	switch current {
	case executionapp.StatusPending:
		if target == executionapp.StatusCancelling || target == executionapp.StatusCancelled {
			return executionapp.StatusCancelRequested, true
		}
		if target == executionapp.StatusSucceeded {
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

func diagnosticExecutionProgress(task *DiagnosticTask) int {
	if task == nil {
		return 0
	}
	if isTerminalDiagnosticTaskStatus(task.Status) {
		return 100
	}
	steps := DefaultDiagnosticTaskSteps()
	for index, step := range steps {
		if step.Code == task.CurrentStep {
			return index * 100 / len(steps)
		}
	}
	return 0
}

func isTerminalDiagnosticTaskStatus(status DiagnosticTaskStatus) bool {
	switch status {
	case DiagnosticTaskStatusCancelled, DiagnosticTaskStatusSucceeded, DiagnosticTaskStatusFailed:
		return true
	default:
		return false
	}
}
