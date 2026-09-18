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

package stupgrade

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

const upgradeExecutionModule = "stupgrade"

// ExecutionRequest 保存升级执行的非敏感请求元数据。
// ExecutionRequest stores non-sensitive request metadata for an upgrade execution.
type ExecutionRequest struct {
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	Confirmed      bool
	ConfirmationID string
	IsAdmin        bool
	ClientType     string
}

// GetPlanForActor 按用户归属读取升级计划。
// GetPlanForActor loads an upgrade plan under the current user's ownership scope.
func (s *Service) GetPlanForActor(ctx context.Context, actor executionapp.Actor, planID uint) (*UpgradePlanRecord, error) {
	if err := validateUpgradeActor(actor); err != nil {
		return nil, err
	}
	return s.repo.GetPlanByIDForOwner(ctx, planID, uint(actor.UserID), actor.IsAdmin)
}

// GetTaskForActor 按用户归属读取升级任务。
// GetTaskForActor loads an upgrade task under the current user's ownership scope.
func (s *Service) GetTaskForActor(ctx context.Context, actor executionapp.Actor, taskID uint) (*UpgradeTask, error) {
	if err := validateUpgradeActor(actor); err != nil {
		return nil, err
	}
	return s.repo.GetTaskByIDForOwner(ctx, taskID, uint(actor.UserID), actor.IsAdmin)
}

// ListTasksForActor 按用户归属列出升级任务，管理员可查看全部。
// ListTasksForActor lists upgrade tasks owned by the current user, while administrators may view all tasks.
func (s *Service) ListTasksForActor(ctx context.Context, actor executionapp.Actor, filter *TaskListFilter) ([]*UpgradeTaskSummary, int64, error) {
	if err := validateUpgradeActor(actor); err != nil {
		return nil, 0, err
	}
	if filter == nil {
		filter = &TaskListFilter{}
	} else {
		clone := *filter
		filter = &clone
	}
	filter.OwnerUserID = uint(actor.UserID)
	filter.IncludeAll = actor.IsAdmin
	return s.ListTasks(ctx, filter)
}

// ListTaskStepsForActor 在归属检查后读取升级步骤。
// ListTaskStepsForActor reads upgrade steps after enforcing ownership.
func (s *Service) ListTaskStepsForActor(ctx context.Context, actor executionapp.Actor, taskID uint) ([]*UpgradeTaskStep, error) {
	if err := validateUpgradeActor(actor); err != nil {
		return nil, err
	}
	return s.repo.ListTaskStepsForOwner(ctx, taskID, uint(actor.UserID), actor.IsAdmin)
}

// ListNodeExecutionsForActor 在归属检查后读取节点执行状态。
// ListNodeExecutionsForActor reads node execution state after enforcing ownership.
func (s *Service) ListNodeExecutionsForActor(ctx context.Context, actor executionapp.Actor, taskID uint) ([]*UpgradeNodeExecution, error) {
	if _, err := s.GetTaskForActor(ctx, actor, taskID); err != nil {
		return nil, err
	}
	return s.ListNodeExecutions(ctx, taskID)
}

// ListStepLogsForActor 在归属检查后读取升级日志。
// ListStepLogsForActor reads upgrade logs after enforcing ownership.
func (s *Service) ListStepLogsForActor(ctx context.Context, actor executionapp.Actor, filter *StepLogFilter) ([]*UpgradeStepLog, int64, error) {
	if err := validateUpgradeActor(actor); err != nil {
		return nil, 0, err
	}
	if filter == nil {
		filter = &StepLogFilter{}
	} else {
		clone := *filter
		filter = &clone
	}
	if filter.TaskID > 0 {
		if _, err := s.GetTaskForActor(ctx, actor, filter.TaskID); err != nil {
			return nil, 0, err
		}
	}
	filter.OwnerUserID = uint(actor.UserID)
	filter.IncludeAll = actor.IsAdmin
	return s.ListStepLogs(ctx, filter)
}

// StartPlanExecutionWithExecution 创建公共执行记录后异步启动升级。
// StartPlanExecutionWithExecution starts an upgrade asynchronously after creating its shared execution record.
func (s *Service) StartPlanExecutionWithExecution(ctx context.Context, planID, createdBy uint, request ExecutionRequest) (*UpgradeTask, error) {
	if s.executionService == nil {
		return s.StartPlanExecution(ctx, planID, createdBy)
	}
	if _, err := s.repo.GetPlanByIDForOwner(ctx, planID, createdBy, request.IsAdmin); err != nil {
		return nil, err
	}
	actor := executionapp.Actor{UserID: uint64(createdBy), IsAdmin: request.IsAdmin}
	if existing, found, err := s.executionService.FindIdempotent(ctx, actor, "stupgrade.plan.execute", request.IdempotencyKey, request.RequestHash); err != nil {
		return nil, err
	} else if found {
		return s.taskForExistingExecution(ctx, existing)
	}
	if err := s.executionService.Authorize(ctx, actor, executionapp.AuthorizationInput{
		OperationID:    "stupgrade.plan.execute",
		RiskLevel:      executionapp.RiskLevelR2,
		Impact:         "升级会停止 SeaTunnel 集群、切换运行目录，并可能在失败时执行回滚。",
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		RequestHash:    strings.TrimSpace(request.RequestHash),
		Confirmed:      request.Confirmed,
		ConfirmationID: strings.TrimSpace(request.ConfirmationID),
	}); err != nil {
		return nil, err
	}
	item, created, err := s.executionService.Create(ctx, executionapp.CreateInput{
		OperationID:    "stupgrade.plan.execute",
		OwnerUserID:    uint64(createdBy),
		ActorType:      executionapp.ActorTypeUser,
		Module:         upgradeExecutionModule,
		RequestID:      strings.TrimSpace(request.RequestID),
		IdempotencyKey: strings.TrimSpace(request.IdempotencyKey),
		RequestHash:    strings.TrimSpace(request.RequestHash),
		RiskLevel:      executionapp.RiskLevelR2,
		Status:         executionapp.StatusPending,
		Cancellable:    true,
		ClientType:     strings.TrimSpace(request.ClientType),
	})
	if err != nil {
		return nil, err
	}
	if !created {
		return s.taskForExistingExecution(ctx, item)
	}

	task, err := s.prepareExecutionTask(ctx, planID, createdBy)
	if err != nil {
		_ = s.executionService.Transition(ctx, item.ExecutionID, executionapp.StatusPending, executionapp.StatusFailed, map[string]any{
			"cancellable":   false,
			"error_code":    "upgrade_task_create_failed",
			"error_message": "upgrade task creation failed",
		})
		return nil, err
	}
	task.ExecutionID = item.ExecutionID
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	if err := s.executionService.BindModuleRef(ctx, item.ExecutionID, strconv.FormatUint(uint64(task.ID), 10)); err != nil {
		return nil, err
	}

	go func(taskID uint) {
		_, _ = s.executeTask(context.Background(), taskID)
	}(task.ID)
	return s.GetTaskDetail(ctx, task.ID)
}

func (s *Service) taskForExistingExecution(ctx context.Context, item *executionapp.Execution) (*UpgradeTask, error) {
	if item == nil || strings.TrimSpace(item.ModuleRef) == "" {
		return nil, executionapp.ErrConcurrentUpdate
	}
	taskID, err := strconv.ParseUint(item.ModuleRef, 10, 64)
	if err != nil {
		return nil, ErrUpgradeTaskNotFound
	}
	return s.repo.GetTaskByID(ctx, uint(taskID))
}

// RequestCancel 仅在升级尚未进入实际执行时取消任务。
// RequestCancel cancels an upgrade task only before actual execution begins.
func (s *Service) RequestCancel(ctx context.Context, item *executionapp.Execution, actor executionapp.Actor) (executionapp.CancelResult, error) {
	if item == nil {
		return executionapp.CancelResult{}, ErrUpgradeTaskNotFound
	}
	taskID, err := strconv.ParseUint(strings.TrimSpace(item.ModuleRef), 10, 64)
	if err != nil || taskID == 0 {
		return executionapp.CancelResult{}, ErrUpgradeTaskNotFound
	}
	task, err := s.repo.GetTaskByIDForOwner(ctx, uint(taskID), uint(item.OwnerUserID), actor.IsAdmin)
	if err != nil {
		return executionapp.CancelResult{}, err
	}

	if task.RollbackStatus == ExecutionStatusRollbackRunning {
		return executionapp.CancelResult{Status: executionapp.StatusRunning, Cancellable: false, CancellableReason: upgradeNotCancellableReason(task)}, nil
	}

	switch task.Status {
	case ExecutionStatusPending, ExecutionStatusReady:
		now := time.Now()
		updated, err := s.repo.UpdateTaskFields(ctx, task.ID, []ExecutionStatus{ExecutionStatusPending, ExecutionStatusReady}, map[string]any{
			"status":       ExecutionStatusCancelled,
			"completed_at": now,
			"updated_at":   now,
		})
		if err != nil {
			return executionapp.CancelResult{}, err
		}
		if updated {
			task.Status = ExecutionStatusCancelled
			task.CompletedAt = &now
			s.publishTaskEvent(newTaskUpdatedEvent(task))
			_ = s.skipPendingUpgradeSteps(ctx, task.ID, now)
			return executionapp.CancelResult{Status: executionapp.StatusCancelled, Cancellable: false, CancellableReason: "upgrade cancelled before execution"}, nil
		}
		latest, loadErr := s.repo.GetTaskByID(ctx, task.ID)
		if loadErr != nil {
			return executionapp.CancelResult{}, loadErr
		}
		if latest.Status == ExecutionStatusRunning || latest.RollbackStatus == ExecutionStatusRollbackRunning {
			return executionapp.CancelResult{Status: executionapp.StatusRunning, Cancellable: false, CancellableReason: upgradeNotCancellableReason(latest)}, nil
		}
		if latest.Status == ExecutionStatusCancelled {
			return executionapp.CancelResult{Status: executionapp.StatusCancelled, Cancellable: false, CancellableReason: "upgrade already cancelled"}, nil
		}
		return executionapp.CancelResult{Status: upgradeExecutionStatus(latest), Cancellable: false, CancellableReason: "upgrade status changed before cancellation"}, nil
	case ExecutionStatusRunning, ExecutionStatusRollbackRunning:
		return executionapp.CancelResult{Status: executionapp.StatusRunning, Cancellable: false, CancellableReason: upgradeNotCancellableReason(task)}, nil
	case ExecutionStatusCancelled:
		return executionapp.CancelResult{Status: executionapp.StatusCancelled, Cancellable: false, CancellableReason: "upgrade already cancelled"}, nil
	case ExecutionStatusSucceeded:
		return executionapp.CancelResult{Status: executionapp.StatusSucceeded, Cancellable: false, CancellableReason: "upgrade already succeeded"}, nil
	default:
		return executionapp.CancelResult{Status: executionapp.StatusFailed, Cancellable: false, CancellableReason: "upgrade already finished"}, nil
	}
}

func (s *Service) skipPendingUpgradeSteps(ctx context.Context, taskID uint, completedAt time.Time) error {
	steps, err := s.repo.ListTaskSteps(ctx, taskID)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if step == nil || step.Status != ExecutionStatusPending {
			continue
		}
		step.Status = ExecutionStatusSkipped
		step.Message = "任务在升级开始前取消。 / Task cancelled before upgrade execution started."
		step.CompletedAt = &completedAt
		if err := s.UpdateTaskStep(ctx, step); err != nil {
			return err
		}
	}
	nodes, err := s.repo.ListNodeExecutions(ctx, taskID)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if node == nil || node.Status != ExecutionStatusPending {
			continue
		}
		node.Status = ExecutionStatusSkipped
		node.Message = "任务在升级开始前取消。 / Task cancelled before upgrade execution started."
		node.CompletedAt = &completedAt
		if err := s.UpdateNodeExecution(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

func upgradeNotCancellableReason(task *UpgradeTask) string {
	if task != nil && task.RollbackStatus == ExecutionStatusRollbackRunning {
		return "upgrade rollback is running and cannot be cancelled safely"
	}
	if task != nil && task.CurrentStep != "" {
		return fmt.Sprintf("upgrade step %s is running and cannot be cancelled safely", task.CurrentStep)
	}
	return "upgrade execution has started and cannot be cancelled safely"
}

func (s *Service) syncExecutionFromTask(ctx context.Context, task *UpgradeTask) error {
	if s.executionService == nil || task == nil || strings.TrimSpace(task.ExecutionID) == "" {
		return nil
	}
	item, err := s.executionService.Get(ctx, executionapp.Actor{UserID: uint64(task.CreatedBy)}, task.ExecutionID)
	if err != nil {
		return err
	}
	target := upgradeExecutionStatus(task)
	if item.Status == target || executionapp.IsTerminal(item.Status) {
		return nil
	}
	updates := map[string]any{
		"progress":           upgradeExecutionProgress(task),
		"cancellable":        task.Status == ExecutionStatusPending || task.Status == ExecutionStatusReady,
		"cancellable_reason": "",
		"result_ref":         fmt.Sprintf("st-upgrade/tasks/%d", task.ID),
	}
	if task.Status == ExecutionStatusRunning || task.Status == ExecutionStatusRollbackRunning {
		updates["cancellable"] = false
		updates["cancellable_reason"] = upgradeNotCancellableReason(task)
	}
	if target == executionapp.StatusFailed {
		updates["error_code"] = "upgrade_failed"
		updates["error_message"] = "upgrade failed"
	}
	if err := s.executionService.Transition(ctx, item.ExecutionID, item.Status, target, updates); err != nil {
		return err
	}
	return nil
}

func upgradeExecutionStatus(task *UpgradeTask) executionapp.Status {
	if task == nil {
		return executionapp.StatusFailed
	}
	if task.RollbackStatus == ExecutionStatusRollbackRunning {
		return executionapp.StatusRunning
	}
	switch task.Status {
	case ExecutionStatusPending, ExecutionStatusReady:
		return executionapp.StatusPending
	case ExecutionStatusRunning, ExecutionStatusRollbackRunning:
		return executionapp.StatusRunning
	case ExecutionStatusCancelled:
		return executionapp.StatusCancelled
	case ExecutionStatusSucceeded:
		return executionapp.StatusSucceeded
	default:
		return executionapp.StatusFailed
	}
}

func upgradeExecutionProgress(task *UpgradeTask) int {
	if task == nil {
		return 0
	}
	switch task.Status {
	case ExecutionStatusSucceeded, ExecutionStatusFailed, ExecutionStatusCancelled:
		return 100
	}
	steps := DefaultExecutionSteps()
	for index, step := range steps {
		if step.Code == task.CurrentStep {
			return index * 100 / len(steps)
		}
	}
	return 0
}

func validateUpgradeActor(actor executionapp.Actor) error {
	if actor.UserID == 0 && !actor.IsAdmin {
		return executionapp.ErrPermissionDenied
	}
	return nil
}
