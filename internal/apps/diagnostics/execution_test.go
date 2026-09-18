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
	"errors"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

func newDiagnosticExecutionService(t *testing.T) (*Service, *executionapp.Service) {
	t.Helper()
	repo := newDiagnosticTaskServiceRepository(t)
	if err := repo.db.AutoMigrate(&executionapp.Execution{}, &executionapp.Confirmation{}); err != nil {
		t.Fatalf("auto migrate execution models: %v", err)
	}
	service := NewServiceWithRepository(
		repo,
		&fakeDiagnosticTaskClusterReader{cluster: &cluster.Cluster{
			ID:         7,
			Name:       "demo-cluster",
			InstallDir: "/opt/seatunnel",
			Nodes: []cluster.ClusterNode{
				{ID: 1, HostID: 11, Role: cluster.NodeRoleWorker, InstallDir: "/opt/seatunnel"},
			},
		}},
		nil,
		nil,
	)
	providers := executionapp.NewProviderRegistry()
	executionService := executionapp.NewService(executionapp.NewRepository(repo.db), providers)
	service.SetExecutionService(executionService)
	providers.Register(diagnosticExecutionModule, service)
	return service, executionService
}

func TestDiagnosticTaskActorScope(t *testing.T) {
	service, _ := newDiagnosticExecutionService(t)
	ctx := t.Context()
	taskIDs := make(map[uint]uint)
	for _, owner := range []uint{11, 22} {
		task, err := service.CreateDiagnosticTask(ctx, &CreateDiagnosticTaskRequest{
			ClusterID:     7,
			TriggerSource: DiagnosticTaskSourceManual,
		}, owner, "user")
		if err != nil {
			t.Fatalf("create diagnostic task for owner %d: %v", owner, err)
		}
		taskIDs[owner] = task.ID
	}

	items, total, err := service.ListDiagnosticTasksForActor(ctx, executionapp.Actor{UserID: 11}, &DiagnosticTaskListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list owned diagnostic tasks: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].CreatedBy != 11 {
		t.Fatalf("unexpected owned task list: total=%d items=%+v", total, items)
	}

	allItems, allTotal, err := service.ListDiagnosticTasksForActor(ctx, executionapp.Actor{UserID: 99, IsAdmin: true}, &DiagnosticTaskListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list all diagnostic tasks as administrator: %v", err)
	}
	if allTotal != 2 || len(allItems) != 2 {
		t.Fatalf("expected administrator to see both tasks, total=%d len=%d", allTotal, len(allItems))
	}

	if _, err := service.GetDiagnosticTaskForActor(ctx, executionapp.Actor{UserID: 11}, taskIDs[22]); !errors.Is(err, ErrDiagnosticTaskNotFound) {
		t.Fatalf("expected foreign task to be hidden, got %v", err)
	}
}

func TestCreateDiagnosticTaskWithExecutionIsIdempotent(t *testing.T) {
	service, _ := newDiagnosticExecutionService(t)
	ctx := t.Context()
	req := &CreateDiagnosticTaskRequest{ClusterID: 7, TriggerSource: DiagnosticTaskSourceManual}
	metadata := DiagnosticExecutionRequest{
		RequestID:      "request-1",
		IdempotencyKey: "diagnostic-idempotency-1",
		RequestHash:    executionapp.HashString("diagnostic-request-1"),
	}

	first, err := service.CreateDiagnosticTaskWithExecution(ctx, req, 11, "user-a", metadata)
	if err != nil {
		t.Fatalf("create first diagnostic task: %v", err)
	}
	second, err := service.CreateDiagnosticTaskWithExecution(ctx, req, 11, "user-a", metadata)
	if err != nil {
		t.Fatalf("create idempotent diagnostic task: %v", err)
	}
	if first.ID != second.ID || first.ExecutionID == "" || first.ExecutionID != second.ExecutionID {
		t.Fatalf("expected the same task and execution, first=%+v second=%+v", first, second)
	}

	items, total, err := service.ListDiagnosticTasksForActor(ctx, executionapp.Actor{UserID: 11}, &DiagnosticTaskListFilter{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list diagnostic tasks: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one diagnostic task after duplicate submission, total=%d len=%d", total, len(items))
	}
}

func TestCancelReadyDiagnosticTaskStopsImmediately(t *testing.T) {
	service, executionService := newDiagnosticExecutionService(t)
	task, err := service.CreateDiagnosticTaskWithExecution(t.Context(), &CreateDiagnosticTaskRequest{
		ClusterID:     7,
		TriggerSource: DiagnosticTaskSourceManual,
	}, 11, "user-a", DiagnosticExecutionRequest{
		IdempotencyKey: "diagnostic-cancel-ready",
		RequestHash:    executionapp.HashString("diagnostic-cancel-ready"),
	})
	if err != nil {
		t.Fatalf("create diagnostic task: %v", err)
	}

	item, err := executionService.Cancel(t.Context(), executionapp.Actor{UserID: 11}, task.ExecutionID)
	if err != nil {
		t.Fatalf("cancel ready diagnostic task: %v", err)
	}
	if item.Status != executionapp.StatusCancelled {
		t.Fatalf("expected shared execution cancelled, got %s", item.Status)
	}
	stored, err := service.repo.GetDiagnosticTaskByID(t.Context(), task.ID)
	if err != nil {
		t.Fatalf("load cancelled diagnostic task: %v", err)
	}
	if stored.Status != DiagnosticTaskStatusCancelled {
		t.Fatalf("expected diagnostic task cancelled, got %s", stored.Status)
	}
	for _, step := range stored.Steps {
		if step.Status != DiagnosticTaskStatusSkipped {
			t.Fatalf("expected pending step skipped after cancellation, got %s", step.Status)
		}
	}
}

func TestCancelRunningDiagnosticTaskWaitsForStepBoundary(t *testing.T) {
	service, executionService := newDiagnosticExecutionService(t)
	ctx := context.Background()
	task, err := service.CreateDiagnosticTaskWithExecution(ctx, &CreateDiagnosticTaskRequest{
		ClusterID:     7,
		TriggerSource: DiagnosticTaskSourceManual,
	}, 11, "user-a", DiagnosticExecutionRequest{
		IdempotencyKey: "diagnostic-cancel-running",
		RequestHash:    executionapp.HashString("diagnostic-cancel-running"),
	})
	if err != nil {
		t.Fatalf("create diagnostic task: %v", err)
	}
	now := time.Now().UTC()
	task.Status = DiagnosticTaskStatusRunning
	task.StartedAt = &now
	if err := service.repo.UpdateDiagnosticTask(ctx, task); err != nil {
		t.Fatalf("mark diagnostic task running: %v", err)
	}
	if err := executionService.Transition(ctx, task.ExecutionID, executionapp.StatusPending, executionapp.StatusRunning, nil); err != nil {
		t.Fatalf("mark shared execution running: %v", err)
	}

	item, err := executionService.Cancel(ctx, executionapp.Actor{UserID: 11}, task.ExecutionID)
	if err != nil {
		t.Fatalf("request diagnostic cancellation: %v", err)
	}
	if item.Status != executionapp.StatusCancelling {
		t.Fatalf("expected shared execution cancelling before boundary, got %s", item.Status)
	}
	requested, err := service.repo.GetDiagnosticTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("load cancellation-requested task: %v", err)
	}
	if requested.Status != DiagnosticTaskStatusCancelRequested {
		t.Fatalf("expected task cancel_requested before boundary, got %s", requested.Status)
	}

	cancelled, err := service.cancelDiagnosticTaskAtBoundary(ctx, task.ID)
	if err != nil {
		t.Fatalf("cancel diagnostic task at boundary: %v", err)
	}
	if !cancelled {
		t.Fatal("expected cancellation to complete at the step boundary")
	}
	finalExecution, err := executionService.Get(ctx, executionapp.Actor{UserID: 11}, task.ExecutionID)
	if err != nil {
		t.Fatalf("load final shared execution: %v", err)
	}
	if finalExecution.Status != executionapp.StatusCancelled {
		t.Fatalf("expected shared execution cancelled after boundary, got %s", finalExecution.Status)
	}
}

func TestJVMDumpDiagnosticTaskRequiresAdministrator(t *testing.T) {
	service, _ := newDiagnosticExecutionService(t)
	_, err := service.CreateDiagnosticTaskWithExecution(t.Context(), &CreateDiagnosticTaskRequest{
		ClusterID:     7,
		TriggerSource: DiagnosticTaskSourceManual,
		Options:       DiagnosticTaskOptions{IncludeJVMDump: true},
	}, 11, "user-a", DiagnosticExecutionRequest{
		IdempotencyKey: "diagnostic-jvm-dump",
		RequestHash:    executionapp.HashString("diagnostic-jvm-dump"),
	})
	if !errors.Is(err, executionapp.ErrAdminRequired) {
		t.Fatalf("expected administrator requirement, got %v", err)
	}
}
