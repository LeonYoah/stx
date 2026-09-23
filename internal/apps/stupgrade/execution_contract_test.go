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
	"errors"
	"fmt"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newUpgradeExecutionContractService(t *testing.T) (*Service, *executionapp.Service) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&UpgradePlanRecord{}, &UpgradeTask{}, &UpgradeTaskStep{}, &UpgradeNodeExecution{}, &UpgradeStepLog{}, &executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("auto migrate execution models: %v", err)
	}
	repo := NewRepository(database)
	service := NewService(repo)
	providers := executionapp.NewProviderRegistry()
	executionService := executionapp.NewService(executionapp.NewRepository(database), providers)
	service.SetExecutionService(executionService)
	providers.Register(upgradeExecutionModule, service)
	return service, executionService
}

func createUpgradeTaskWithSharedExecution(t *testing.T, service *Service, executionService *executionapp.Service, owner uint, key string) *UpgradeTask {
	t.Helper()
	ctx := t.Context()
	plan := &UpgradePlanRecord{
		ClusterID:     1,
		SourceVersion: "2.3.11",
		TargetVersion: "2.3.12",
		Status:        PlanStatusReady,
		CreatedBy:     owner,
		Snapshot: UpgradePlanSnapshot{
			ClusterID:     1,
			SourceVersion: "2.3.11",
			TargetVersion: "2.3.12",
			Steps:         DefaultExecutionSteps(),
		},
	}
	if err := service.repo.CreatePlan(ctx, plan); err != nil {
		t.Fatalf("create upgrade plan: %v", err)
	}
	task, err := service.CreateTaskFromPlan(ctx, plan.ID, owner)
	if err != nil {
		t.Fatalf("create upgrade task: %v", err)
	}
	item, created, err := executionService.Create(ctx, executionapp.CreateInput{
		OperationID:    "stupgrade.plan.execute",
		OwnerUserID:    uint64(owner),
		ActorType:      executionapp.ActorTypeUser,
		Module:         upgradeExecutionModule,
		IdempotencyKey: key,
		RequestHash:    executionapp.HashString(key),
		RiskLevel:      executionapp.RiskLevelR2,
		Status:         executionapp.StatusPending,
		Cancellable:    true,
	})
	if err != nil || !created {
		t.Fatalf("create shared execution: created=%t err=%v", created, err)
	}
	task.ExecutionID = item.ExecutionID
	if err := service.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("attach shared execution: %v", err)
	}
	if err := executionService.BindModuleRef(ctx, item.ExecutionID, stringUint(task.ID)); err != nil {
		t.Fatalf("bind shared execution: %v", err)
	}
	return task
}

func TestUpgradeTaskActorScope(t *testing.T) {
	service, executionService := newUpgradeExecutionContractService(t)
	first := createUpgradeTaskWithSharedExecution(t, service, executionService, 11, "upgrade-owner-11")
	second := createUpgradeTaskWithSharedExecution(t, service, executionService, 22, "upgrade-owner-22")

	items, total, err := service.ListTasksForActor(t.Context(), executionapp.Actor{UserID: 11}, &TaskListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list owned upgrade tasks: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("unexpected owned upgrade tasks: total=%d items=%+v", total, items)
	}
	if _, err := service.GetTaskForActor(t.Context(), executionapp.Actor{UserID: 11}, second.ID); !errors.Is(err, ErrUpgradeTaskNotFound) {
		t.Fatalf("expected foreign upgrade task to be hidden, got %v", err)
	}

	adminItems, adminTotal, err := service.ListTasksForActor(t.Context(), executionapp.Actor{UserID: 99, IsAdmin: true}, &TaskListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list upgrade tasks as administrator: %v", err)
	}
	if adminTotal != 2 || len(adminItems) != 2 {
		t.Fatalf("expected administrator to see both tasks, total=%d len=%d", adminTotal, len(adminItems))
	}
}

func TestCancelPendingUpgradeTaskStopsBeforeExecution(t *testing.T) {
	service, executionService := newUpgradeExecutionContractService(t)
	task := createUpgradeTaskWithSharedExecution(t, service, executionService, 11, "upgrade-cancel-pending")

	item, err := executionService.Cancel(t.Context(), executionapp.Actor{UserID: 11}, task.ExecutionID)
	if err != nil {
		t.Fatalf("cancel pending upgrade: %v", err)
	}
	if item.Status != executionapp.StatusCancelled {
		t.Fatalf("expected shared execution cancelled, got %s", item.Status)
	}
	stored, err := service.repo.GetTaskByID(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("load cancelled upgrade task: %v", err)
	}
	if stored.Status != ExecutionStatusCancelled {
		t.Fatalf("expected upgrade task cancelled, got %s", stored.Status)
	}
	for _, step := range stored.Steps {
		if step.Status != ExecutionStatusSkipped {
			t.Fatalf("expected pending step skipped after cancellation, got %s", step.Status)
		}
	}

	executed, err := service.executeTask(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("load cancelled task through executeTask: %v", err)
	}
	if executed.Status != ExecutionStatusCancelled {
		t.Fatalf("expected cancelled task to remain cancelled, got %s", executed.Status)
	}
}

func TestCancelRunningUpgradeReturnsNotCancellableState(t *testing.T) {
	service, executionService := newUpgradeExecutionContractService(t)
	task := createUpgradeTaskWithSharedExecution(t, service, executionService, 11, "upgrade-cancel-running")
	now := time.Now()
	task.Status = ExecutionStatusRunning
	task.CurrentStep = StepCodeStopCluster
	task.StartedAt = &now
	if err := service.repo.UpdateTask(t.Context(), task); err != nil {
		t.Fatalf("mark upgrade running: %v", err)
	}
	if err := executionService.Transition(t.Context(), task.ExecutionID, executionapp.StatusPending, executionapp.StatusRunning, map[string]any{"cancellable": true}); err != nil {
		t.Fatalf("mark shared execution running: %v", err)
	}

	item, err := executionService.Cancel(t.Context(), executionapp.Actor{UserID: 11}, task.ExecutionID)
	if err != nil {
		t.Fatalf("request running upgrade cancellation: %v", err)
	}
	if item.Status != executionapp.StatusRunning || item.Cancellable {
		t.Fatalf("expected running non-cancellable execution, got status=%s cancellable=%t", item.Status, item.Cancellable)
	}
	if item.CancellableReason == "" {
		t.Fatal("expected a stable non-cancellable reason")
	}
	stored, err := service.repo.GetTaskByID(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("load running upgrade task: %v", err)
	}
	if stored.Status != ExecutionStatusRunning {
		t.Fatalf("expected business task to remain running, got %s", stored.Status)
	}
}

func TestStartPlanExecutionWithExecutionRequiresConfirmationAndIsIdempotent(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&UpgradePlanRecord{}, &UpgradeTask{}, &UpgradeTaskStep{}, &UpgradeNodeExecution{}, &UpgradeStepLog{}, &executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("auto migrate execution models: %v", err)
	}
	repo := NewRepository(database)
	service := newExecutionService(t, repo, &stubClusterOperator{}, &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}})
	providers := executionapp.NewProviderRegistry()
	executionService := executionapp.NewService(executionapp.NewRepository(database), providers)
	service.SetExecutionService(executionService)
	providers.Register(upgradeExecutionModule, service)
	planID := mustCreateReadyPlan(t, service)
	request := ExecutionRequest{
		RequestID:      "upgrade-request-1",
		IdempotencyKey: "upgrade-idempotency-1",
		RequestHash:    executionapp.HashString("upgrade-plan-1"),
	}

	_, err = service.StartPlanExecutionWithExecution(t.Context(), planID, 7, request)
	var confirmationErr *executionapp.ConfirmationRequiredError
	if !errors.As(err, &confirmationErr) {
		t.Fatalf("expected upgrade confirmation requirement, got %v", err)
	}
	request.ConfirmationID = confirmationErr.ConfirmationID
	first, err := service.StartPlanExecutionWithExecution(t.Context(), planID, 7, request)
	if err != nil {
		t.Fatalf("start confirmed upgrade: %v", err)
	}
	if first.ExecutionID == "" {
		t.Fatal("expected shared execution id on upgrade task")
	}

	second, err := service.StartPlanExecutionWithExecution(t.Context(), planID, 7, request)
	if err != nil {
		t.Fatalf("repeat confirmed upgrade with the same idempotency key: %v", err)
	}
	if second.ID != first.ID || second.ExecutionID != first.ExecutionID {
		t.Fatalf("expected idempotent upgrade task, first=%+v second=%+v", first, second)
	}
}

func stringUint(value uint) string {
	return fmt.Sprintf("%d", value)
}
