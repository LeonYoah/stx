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

package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newExecutionTestService 创建每个测试独立使用的内存数据库和执行服务。
// newExecutionTestService creates an isolated in-memory database and execution service for each test.
func newExecutionTestService(t *testing.T) (*Service, *ProviderRegistry) {
	t.Helper()
	dsn := fmt.Sprintf("file:execution-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := database.AutoMigrate(&Execution{}, &Confirmation{}); err != nil {
		t.Fatalf("迁移公共执行表失败: %v", err)
	}
	providers := NewProviderRegistry()
	return NewService(NewRepository(database), providers), providers
}

func TestCanTransitionProtectsTerminalStates(t *testing.T) {
	allowed := []struct {
		from Status
		to   Status
	}{
		{StatusPending, StatusRunning},
		{StatusRunning, StatusCancelRequested},
		{StatusCancelRequested, StatusCancelling},
		{StatusCancelling, StatusCancelled},
	}
	for _, item := range allowed {
		if !CanTransition(item.from, item.to) {
			t.Fatalf("应允许状态迁移 %s -> %s", item.from, item.to)
		}
	}
	for _, terminal := range []Status{StatusCancelled, StatusSucceeded, StatusFailed, StatusTimedOut} {
		if CanTransition(terminal, StatusRunning) {
			t.Fatalf("终态不应回到运行状态: %s", terminal)
		}
	}
}

func TestCreateUsesIdempotencyKeyAndRejectsDifferentRequest(t *testing.T) {
	service, _ := newExecutionTestService(t)
	ctx := context.Background()
	input := CreateInput{
		OperationID:    "diagnostics.task.start",
		OwnerUserID:    7,
		Module:         "diagnostics",
		ModuleRef:      "10",
		IdempotencyKey: "same-key",
		RequestHash:    HashString("request-a"),
	}

	first, created, err := service.Create(ctx, input)
	if err != nil || !created {
		t.Fatalf("首次创建失败: created=%t err=%v", created, err)
	}
	second, created, err := service.Create(ctx, input)
	if err != nil || created {
		t.Fatalf("重复请求应返回已有记录: created=%t err=%v", created, err)
	}
	if second.ExecutionID != first.ExecutionID {
		t.Fatalf("重复请求产生了不同执行编号: %s != %s", second.ExecutionID, first.ExecutionID)
	}

	input.RequestHash = HashString("request-b")
	if _, _, err := service.Create(ctx, input); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("不同请求复用幂等键应冲突，得到: %v", err)
	}
}

func TestGetEnforcesOwnerAndAdminAccess(t *testing.T) {
	service, _ := newExecutionTestService(t)
	item, _, err := service.Create(context.Background(), CreateInput{
		OperationID: "sync.submit",
		OwnerUserID: 11,
		Module:      "sync",
	})
	if err != nil {
		t.Fatalf("创建执行失败: %v", err)
	}
	if _, err := service.Get(context.Background(), Actor{UserID: 12}, item.ExecutionID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("其他用户应被拒绝，得到: %v", err)
	}
	if _, err := service.Get(context.Background(), Actor{UserID: 11}, item.ExecutionID); err != nil {
		t.Fatalf("所有者读取失败: %v", err)
	}
	if _, err := service.Get(context.Background(), Actor{UserID: 99, IsAdmin: true}, item.ExecutionID); err != nil {
		t.Fatalf("管理员读取失败: %v", err)
	}

	systemItem, _, err := service.Create(context.Background(), CreateInput{
		OperationID: "diagnostics.auto",
		OwnerUserID: 0,
		ActorType:   ActorTypeSystem,
		Module:      "diagnostics",
	})
	if err != nil {
		t.Fatalf("创建系统执行失败: %v", err)
	}
	if _, err := service.Get(context.Background(), Actor{UserID: 11}, systemItem.ExecutionID); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("普通用户不应读取系统执行，得到: %v", err)
	}
}

func TestTransitionUsesExpectedStateAndProtectsTerminalResult(t *testing.T) {
	service, _ := newExecutionTestService(t)
	ctx := context.Background()
	item, _, err := service.Create(ctx, CreateInput{OperationID: "upgrade.execute", OwnerUserID: 2, Module: "stupgrade"})
	if err != nil {
		t.Fatalf("创建执行失败: %v", err)
	}
	if err := service.Transition(ctx, item.ExecutionID, StatusPending, StatusRunning, nil); err != nil {
		t.Fatalf("进入运行状态失败: %v", err)
	}
	if err := service.Transition(ctx, item.ExecutionID, StatusPending, StatusFailed, nil); !errors.Is(err, ErrConcurrentUpdate) {
		t.Fatalf("旧状态条件不匹配时应拒绝，得到: %v", err)
	}
	if err := service.Transition(ctx, item.ExecutionID, StatusRunning, StatusSucceeded, nil); err != nil {
		t.Fatalf("完成执行失败: %v", err)
	}
	if err := service.Transition(ctx, item.ExecutionID, StatusSucceeded, StatusRunning, nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("终态回到运行状态应被拒绝，得到: %v", err)
	}
}

func TestExecutionLifecycleWritesCreateStartAndResultAudit(t *testing.T) {
	dsn := fmt.Sprintf("file:execution-audit-%s?mode=memory&cache=shared", t.Name())
	database, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("创建测试数据库失败: %v", err)
	}
	if err := database.AutoMigrate(&Execution{}, &Confirmation{}, &audit.AuditLog{}); err != nil {
		t.Fatalf("迁移执行和审计表失败: %v", err)
	}
	auditRepo := audit.NewRepository(database)
	service := NewService(NewRepository(database), NewProviderRegistry())
	service.SetAuditRepository(auditRepo)
	ctx := context.Background()
	item, created, err := service.Create(ctx, CreateInput{
		OperationID: "sync.task.submit",
		OwnerUserID: 8,
		Module:      "sync",
		RequestID:   "request-audit-1",
		RiskLevel:   RiskLevelR1,
		ClientType:  "cli",
		Cancellable: true,
	})
	if err != nil || !created {
		t.Fatalf("创建执行失败: created=%t err=%v", created, err)
	}
	if err := service.Transition(ctx, item.ExecutionID, StatusPending, StatusRunning, nil); err != nil {
		t.Fatalf("启动执行失败: %v", err)
	}
	if err := service.Transition(ctx, item.ExecutionID, StatusRunning, StatusSucceeded, map[string]any{"cancellable": false}); err != nil {
		t.Fatalf("完成执行失败: %v", err)
	}

	logs, total, err := auditRepo.ListAuditLogs(ctx, &audit.AuditLogFilter{ExecutionID: item.ExecutionID, IncludeAll: true})
	if err != nil {
		t.Fatalf("查询执行审计失败: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("应写入创建、启动和结果三条审计: total=%d len=%d", total, len(logs))
	}
	actions := make(map[string]*audit.AuditLog, len(logs))
	for _, logItem := range logs {
		actions[logItem.Action] = logItem
		if logItem.UserID == nil || *logItem.UserID != 8 || logItem.RequestID != "request-audit-1" || logItem.RiskLevel != string(RiskLevelR1) {
			t.Fatalf("执行审计关联字段不完整: %+v", logItem)
		}
	}
	if actions["execution.create"] == nil || actions["execution.start"] == nil || actions["execution.result"] == nil {
		t.Fatalf("执行审计动作不完整: %#v", actions)
	}
	if actions["execution.create"].ClientType != "cli" || actions["execution.start"].ClientType != "cli" || actions["execution.result"].ClientType != "cli" || actions["execution.result"].ResultStatus != string(StatusSucceeded) {
		t.Fatalf("执行审计来源或结果错误: create=%+v result=%+v", actions["execution.create"], actions["execution.result"])
	}
}

type countingCancellationProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingCancellationProvider) RequestCancel(_ context.Context, _ *Execution, _ Actor) (CancelResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	return CancelResult{Status: StatusCancelling, Cancellable: false, CancellableReason: "cancel_in_progress"}, nil
}

func TestCancelIsIdempotentAndDoesNotClaimCancelledEarly(t *testing.T) {
	service, providers := newExecutionTestService(t)
	provider := &countingCancellationProvider{}
	providers.Register("diagnostics", provider)
	item, _, err := service.Create(context.Background(), CreateInput{
		OperationID: "diagnostics.task.start",
		OwnerUserID: 3,
		Module:      "diagnostics",
		Status:      StatusRunning,
		Cancellable: true,
	})
	if err != nil {
		t.Fatalf("创建执行失败: %v", err)
	}
	actor := Actor{UserID: 3}
	first, err := service.Cancel(context.Background(), actor, item.ExecutionID)
	if err != nil {
		t.Fatalf("首次取消失败: %v", err)
	}
	if first.Status != StatusCancelling {
		t.Fatalf("业务模块未确认停止时不应返回 cancelled，得到: %s", first.Status)
	}
	second, err := service.Cancel(context.Background(), actor, item.ExecutionID)
	if err != nil {
		t.Fatalf("重复取消失败: %v", err)
	}
	if second.Status != StatusCancelling || provider.calls != 1 {
		t.Fatalf("重复取消不应再次调用业务模块: status=%s calls=%d", second.Status, provider.calls)
	}
}

func TestAuthorizeConfirmationExpiresAndCanOnlyBeUsedOnce(t *testing.T) {
	service, _ := newExecutionTestService(t)
	baseTime := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return baseTime }
	actor := Actor{UserID: 8, IsAdmin: true}
	input := AuthorizationInput{
		OperationID:    "diagnostics.dump",
		RiskLevel:      RiskLevelR3,
		Impact:         "生成 dump 可能影响集群性能",
		IdempotencyKey: "dump-key",
		RequestHash:    HashString("dump-request"),
	}

	err := service.Authorize(context.Background(), actor, input)
	var required *ConfirmationRequiredError
	if !errors.As(err, &required) || required.ConfirmationID == "" {
		t.Fatalf("首次请求应返回一次性确认编号，得到: %v", err)
	}
	input.ConfirmationID = required.ConfirmationID
	if err := service.Authorize(context.Background(), actor, input); err != nil {
		t.Fatalf("有效确认编号应通过: %v", err)
	}
	if err := service.Authorize(context.Background(), actor, input); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("确认编号不能重复使用，得到: %v", err)
	}

	input.IdempotencyKey = "expired-key"
	input.ConfirmationID = ""
	err = service.Authorize(context.Background(), actor, input)
	if !errors.As(err, &required) {
		t.Fatalf("创建过期测试确认失败: %v", err)
	}
	input.ConfirmationID = required.ConfirmationID
	service.now = func() time.Time { return baseTime.Add(defaultConfirmationTTL + time.Second) }
	if err := service.Authorize(context.Background(), actor, input); !errors.Is(err, ErrConfirmationInvalid) {
		t.Fatalf("过期确认编号应被拒绝，得到: %v", err)
	}
}

func TestAuthorizeRequiresAdminForR3(t *testing.T) {
	service, _ := newExecutionTestService(t)
	err := service.Authorize(context.Background(), Actor{UserID: 6}, AuthorizationInput{
		OperationID:    "diagnostics.dump",
		RiskLevel:      RiskLevelR3,
		IdempotencyKey: "key",
		RequestHash:    HashString("request"),
	})
	if !errors.Is(err, ErrAdminRequired) {
		t.Fatalf("R3 应要求管理员权限，得到: %v", err)
	}
}
