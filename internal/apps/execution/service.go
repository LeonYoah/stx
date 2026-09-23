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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/google/uuid"
)

const defaultConfirmationTTL = 5 * time.Minute

// CreateInput 保存创建公共执行记录所需的非敏感字段。
// CreateInput stores the non-sensitive fields needed to create a shared execution record.
type CreateInput struct {
	OperationID       string
	OwnerUserID       uint64
	ActorType         ActorType
	Module            string
	ModuleRef         string
	RequestID         string
	IdempotencyKey    string
	RequestHash       string
	RiskLevel         RiskLevel
	Status            Status
	Cancellable       bool
	CancellableReason string
	ClientType        string
}

// AuthorizationInput 保存风险确认校验参数。
// AuthorizationInput stores risk-confirmation validation parameters.
type AuthorizationInput struct {
	OperationID    string
	RiskLevel      RiskLevel
	Impact         string
	IdempotencyKey string
	RequestHash    string
	Confirmed      bool
	ConfirmationID string
}

// SynchronousInput 保存同步写操作开始执行所需的安全字段。
// SynchronousInput stores the safe fields required to begin a synchronous write operation.
type SynchronousInput struct {
	OperationID    string
	Module         string
	ModuleRef      string
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	RiskLevel      RiskLevel
	Impact         string
	Confirmed      bool
	ConfirmationID string
	ClientType     string
}

// ConfirmationRequiredError 返回一次性确认编号和影响说明。
// ConfirmationRequiredError returns a one-time confirmation ID and impact description.
type ConfirmationRequiredError struct {
	ConfirmationID string
	RiskLevel      RiskLevel
	Impact         string
	ExpiresAt      time.Time
}

func (e *ConfirmationRequiredError) Error() string {
	return ErrConfirmationRequired.Error()
}

func (e *ConfirmationRequiredError) Unwrap() error {
	return ErrConfirmationRequired
}

// Service 实现公共状态、权限、幂等、确认和取消规则。
// Service implements shared state, access, idempotency, confirmation, and cancellation rules.
type Service struct {
	repo             *Repository
	providers        *ProviderRegistry
	now              func() time.Time
	pollInterval     time.Duration
	auditRepo        *audit.Repository
	executionDetails sync.Map
}

// SetAuditRepository 设置公共执行服务使用的审计仓库。
// SetAuditRepository sets the audit repository used by the shared execution service.
func (s *Service) SetAuditRepository(repo *audit.Repository) {
	if s == nil {
		return
	}
	s.auditRepo = repo
}

// EnrichAuditDetails updates audit logs associated with an execution with extra details (e.g. engine URL).
func (s *Service) EnrichAuditDetails(ctx context.Context, executionID string, extraDetails map[string]any) error {
	if s == nil || executionID == "" || len(extraDetails) == 0 {
		return nil
	}
	val, _ := s.executionDetails.LoadOrStore(executionID, make(map[string]any))
	existing, _ := val.(map[string]any)
	merged := make(map[string]any, len(existing)+len(extraDetails))
	for k, v := range existing {
		merged[k] = v
	}
	for k, v := range extraDetails {
		merged[k] = v
	}
	s.executionDetails.Store(executionID, merged)

	if s.auditRepo != nil {
		return s.auditRepo.UpdateAuditLogDetailsByExecutionID(ctx, executionID, extraDetails)
	}
	return nil
}

// NewService 创建公共执行服务。
// NewService creates a shared execution service.
func NewService(repo *Repository, providers *ProviderRegistry) *Service {
	if providers == nil {
		providers = NewProviderRegistry()
	}
	return &Service{
		repo:         repo,
		providers:    providers,
		now:          time.Now,
		pollInterval: 200 * time.Millisecond,
	}
}

// Create 创建执行记录，风险操作应在调用前先通过 Authorize 校验。
// Create creates an execution record; risky operations should call Authorize first.
func (s *Service) Create(ctx context.Context, input CreateInput) (*Execution, bool, error) {
	input.OperationID = strings.TrimSpace(input.OperationID)
	input.Module = strings.TrimSpace(input.Module)
	if input.OperationID == "" || input.Module == "" {
		return nil, false, ErrInvalidExecution
	}
	if input.ActorType == "" {
		input.ActorType = ActorTypeUser
	}
	if input.Status == "" {
		input.Status = StatusPending
	}
	if input.RiskLevel == "" {
		input.RiskLevel = RiskLevelR0
	}
	var keyHash *string
	if strings.TrimSpace(input.IdempotencyKey) != "" {
		hash := HashString(input.IdempotencyKey)
		keyHash = &hash
	}
	item := &Execution{
		ExecutionID:        uuid.NewString(),
		OperationID:        input.OperationID,
		OwnerUserID:        input.OwnerUserID,
		ActorType:          input.ActorType,
		Module:             input.Module,
		ModuleRef:          strings.TrimSpace(input.ModuleRef),
		RequestID:          strings.TrimSpace(input.RequestID),
		ClientType:         strings.TrimSpace(input.ClientType),
		IdempotencyKeyHash: keyHash,
		RequestHash:        strings.TrimSpace(input.RequestHash),
		RiskLevel:          input.RiskLevel,
		Status:             input.Status,
		Cancellable:        input.Cancellable,
		CancellableReason:  strings.TrimSpace(input.CancellableReason),
	}
	result, created, err := s.repo.CreateOrGet(ctx, item)
	if err != nil || !created {
		return result, created, err
	}
	s.recordAudit(ctx, result, input.OwnerUserID, "execution.create", input.ClientType, string(result.Status), "")
	return result, true, nil
}

// Get 返回调用者有权查看的执行记录。
// Get returns an execution visible to the current actor.
func (s *Service) Get(ctx context.Context, actor Actor, executionID string) (*Execution, error) {
	item, err := s.repo.GetByExecutionID(ctx, strings.TrimSpace(executionID))
	if err != nil {
		return nil, err
	}
	if err := authorizeActor(actor, item); err != nil {
		return nil, err
	}
	return item, nil
}

// FindIdempotent 查找同一用户和操作下已存在的幂等执行记录。
// FindIdempotent finds an existing idempotent execution for the same owner and operation.
func (s *Service) FindIdempotent(ctx context.Context, actor Actor, operationID, idempotencyKey, requestHash string) (*Execution, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, false, nil
	}
	item, err := s.repo.findByIdempotency(ctx, actor.UserID, strings.TrimSpace(operationID), HashString(idempotencyKey))
	if err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if err := authorizeActor(actor, item); err != nil {
		return nil, false, err
	}
	if item.RequestHash != strings.TrimSpace(requestHash) {
		return nil, false, ErrIdempotencyConflict
	}
	return item, true, nil
}

// BeginSynchronous 校验确认与幂等约定，并创建同步写操作的运行记录。
// BeginSynchronous validates confirmation and idempotency contracts and creates a running record for a synchronous write.
func (s *Service) BeginSynchronous(ctx context.Context, actor Actor, input SynchronousInput) (*Execution, bool, error) {
	if existing, found, err := s.FindIdempotent(ctx, actor, input.OperationID, input.IdempotencyKey, input.RequestHash); err != nil || found {
		if err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if err := s.Authorize(ctx, actor, AuthorizationInput{
		OperationID:    input.OperationID,
		RiskLevel:      input.RiskLevel,
		Impact:         input.Impact,
		IdempotencyKey: input.IdempotencyKey,
		RequestHash:    input.RequestHash,
		Confirmed:      input.Confirmed,
		ConfirmationID: input.ConfirmationID,
	}); err != nil {
		return nil, false, err
	}
	item, created, err := s.Create(ctx, CreateInput{
		OperationID:       input.OperationID,
		OwnerUserID:       actor.UserID,
		ActorType:         ActorTypeUser,
		Module:            input.Module,
		ModuleRef:         input.ModuleRef,
		RequestID:         input.RequestID,
		IdempotencyKey:    input.IdempotencyKey,
		RequestHash:       input.RequestHash,
		RiskLevel:         input.RiskLevel,
		Status:            StatusPending,
		Cancellable:       false,
		CancellableReason: "synchronous_operation",
		ClientType:        input.ClientType,
	})
	if err != nil || !created {
		return item, !created, err
	}
	if err := s.Transition(ctx, item.ExecutionID, StatusPending, StatusRunning, nil); err != nil {
		return nil, false, err
	}
	item.Status = StatusRunning
	return item, false, nil
}

// FinishSynchronous 将同步写操作记录为成功或失败，只保存安全结果引用。
// FinishSynchronous marks a synchronous write as succeeded or failed and stores only a safe result reference.
func (s *Service) FinishSynchronous(ctx context.Context, item *Execution, resultRef string, runErr error) error {
	if item == nil {
		return ErrInvalidExecution
	}
	target := StatusSucceeded
	updates := map[string]any{
		"result_ref":         strings.TrimSpace(resultRef),
		"cancellable":        false,
		"cancellable_reason": "completed",
	}
	if runErr != nil {
		target = StatusFailed
		updates["error_message"] = runErr.Error()
	}
	return s.Transition(ctx, item.ExecutionID, item.Status, target, updates)
}

// BindModuleRef 将公共执行记录绑定到业务模块任务编号。
// BindModuleRef binds a shared execution to a business-module task identifier.
func (s *Service) BindModuleRef(ctx context.Context, executionID, moduleRef string) error {
	executionID = strings.TrimSpace(executionID)
	moduleRef = strings.TrimSpace(moduleRef)
	if executionID == "" || moduleRef == "" {
		return ErrInvalidExecution
	}
	return s.repo.BindModuleRef(ctx, executionID, moduleRef)
}

// Wait 等待执行进入终态；超时只结束本次等待，不修改任务状态。
// Wait waits for a terminal state; a timeout ends only this wait and does not mutate the execution.
func (s *Service) Wait(ctx context.Context, actor Actor, executionID string, timeout time.Duration) (*Execution, bool, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		item, err := s.Get(ctx, actor, executionID)
		if err != nil {
			return nil, false, err
		}
		if IsTerminal(item.Status) {
			return item, false, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-deadline.C:
			latest, err := s.Get(ctx, actor, executionID)
			return latest, true, err
		case <-ticker.C:
		}
	}
}

// Transition 按当前状态条件执行公共状态迁移。
// Transition performs a shared state transition guarded by the current status.
func (s *Service) Transition(ctx context.Context, executionID string, from, to Status, updates map[string]any) error {
	if !CanTransition(from, to) {
		return ErrInvalidTransition
	}
	if updates == nil {
		updates = make(map[string]any)
	}
	updates["status"] = to
	if to == StatusRunning {
		if _, ok := updates["started_at"]; !ok {
			updates["started_at"] = s.now()
		}
	}
	if IsTerminal(to) {
		if _, ok := updates["finished_at"]; !ok {
			updates["finished_at"] = s.now()
		}
	}
	if err := s.repo.UpdateStatus(ctx, executionID, []Status{from}, updates); err != nil {
		return err
	}
	if to == StatusRunning {
		if item, err := s.repo.GetByExecutionID(ctx, executionID); err == nil {
			s.recordAudit(ctx, item, item.OwnerUserID, "execution.start", auditClientType(item), string(to), "")
		}
	}
	if IsTerminal(to) {
		if item, err := s.repo.GetByExecutionID(ctx, executionID); err == nil {
			s.recordAudit(ctx, item, item.OwnerUserID, "execution.result", auditClientType(item), string(to), "")
		}
		s.executionDetails.Delete(executionID)
	}
	return nil
}

// Cancel 请求业务模块真实停止执行，重复请求返回同一取消进度。
// Cancel asks the business module to stop the execution and returns the same cancellation progress for repeated requests.
func (s *Service) Cancel(ctx context.Context, actor Actor, executionID string) (*Execution, error) {
	item, err := s.Get(ctx, actor, executionID)
	if err != nil {
		return nil, err
	}
	if IsTerminal(item.Status) || item.Status == StatusCancelRequested || item.Status == StatusCancelling {
		return item, nil
	}
	if !item.Cancellable {
		return nil, fmt.Errorf("%w: %s", ErrNotCancellable, item.CancellableReason)
	}
	provider, ok := s.providers.Get(item.Module)
	if !ok {
		return nil, ErrProviderNotRegistered
	}
	if err := s.Transition(ctx, item.ExecutionID, item.Status, StatusCancelRequested, nil); err != nil {
		if errors.Is(err, ErrConcurrentUpdate) {
			return s.Get(ctx, actor, item.ExecutionID)
		}
		return nil, err
	}
	item.Status = StatusCancelRequested

	result, err := provider.RequestCancel(ctx, item, actor)
	if err != nil {
		return nil, err
	}
	if result.Status == "" {
		result.Status = StatusCancelRequested
	}
	if !CanTransition(StatusCancelRequested, result.Status) {
		return nil, ErrInvalidTransition
	}
	updates := map[string]any{
		"cancellable":        result.Cancellable,
		"cancellable_reason": strings.TrimSpace(result.CancellableReason),
	}
	if err := s.Transition(ctx, item.ExecutionID, StatusCancelRequested, result.Status, updates); err != nil && !errors.Is(err, ErrConcurrentUpdate) {
		return nil, err
	}
	finalItem, err := s.Get(ctx, actor, item.ExecutionID)
	if err == nil {
		s.recordAudit(ctx, finalItem, actor.UserID, "execution.cancel", "", string(finalItem.Status), finalItem.CancellableReason)
	}
	return finalItem, err
}

// Authorize 校验 R0 至 R3 的幂等键、显式确认、一次性确认和管理员要求。
// Authorize validates idempotency, explicit confirmation, one-time confirmation, and administrator requirements for R0 through R3.
func (s *Service) Authorize(ctx context.Context, actor Actor, input AuthorizationInput) error {
	if input.RiskLevel == "" {
		input.RiskLevel = RiskLevelR0
	}
	if input.RiskLevel == RiskLevelR0 {
		return nil
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return ErrIdempotencyKeyMissing
	}
	if input.RiskLevel == RiskLevelR1 {
		if !input.Confirmed {
			return ErrExplicitConfirmNeeded
		}
		return nil
	}
	if input.RiskLevel == RiskLevelR3 && !actor.IsAdmin {
		return ErrAdminRequired
	}

	keyHash := HashString(input.IdempotencyKey)
	requestHash := strings.TrimSpace(input.RequestHash)
	if strings.TrimSpace(input.ConfirmationID) != "" {
		return s.repo.ConsumeConfirmation(ctx, HashString(input.ConfirmationID), actor.UserID, strings.TrimSpace(input.OperationID), keyHash, requestHash, s.now())
	}

	rawID := uuid.NewString()
	expiresAt := s.now().Add(defaultConfirmationTTL)
	confirmation := &Confirmation{
		ConfirmationHash:   HashString(rawID),
		OwnerUserID:        actor.UserID,
		OperationID:        strings.TrimSpace(input.OperationID),
		IdempotencyKeyHash: keyHash,
		RequestHash:        requestHash,
		RiskLevel:          input.RiskLevel,
		Impact:             strings.TrimSpace(input.Impact),
		ExpiresAt:          expiresAt,
	}
	if err := s.repo.CreateConfirmation(ctx, confirmation); err != nil {
		return err
	}
	return &ConfirmationRequiredError{
		ConfirmationID: rawID,
		RiskLevel:      input.RiskLevel,
		Impact:         confirmation.Impact,
		ExpiresAt:      expiresAt,
	}
}

// HashRequest 对规范 JSON 编码后的请求计算摘要，不应传入秘密原文以外的持久化用途。
// HashRequest hashes the canonical JSON encoding of a request and must not be used to persist the original secret-bearing body.
func HashRequest(value any) (string, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return HashString(string(content)), nil
}

// HashString 返回字符串的 SHA-256 十六进制摘要。
// HashString returns the hexadecimal SHA-256 digest of a string.
func HashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// auditClientType 优先使用创建时记下的客户端，没有时才记为 system。
// auditClientType prefers the client stored at create time and falls back to system.
func auditClientType(item *Execution) string {
	if item == nil {
		return "system"
	}
	if clientType := strings.TrimSpace(item.ClientType); clientType != "" {
		return clientType
	}
	return "system"
}

func (s *Service) recordAudit(ctx context.Context, item *Execution, actorUserID uint64, action, clientType, resultStatus, reason string) {
	if s == nil || s.auditRepo == nil || item == nil {
		return
	}
	var userID *uint
	if actorUserID > 0 {
		value := uint(actorUserID)
		userID = &value
	}
	trigger := "manual"
	if item.ActorType == ActorTypeSystem {
		trigger = "auto"
	}
	username := ""
	if userID != nil {
		username = s.auditRepo.LookupUsername(ctx, *userID)
	}
	details := audit.AuditDetails{
		"operation_id":       item.OperationID,
		"cancellable":        item.Cancellable,
		"cancellable_reason": strings.TrimSpace(reason),
	}
	if val, ok := s.executionDetails.Load(item.ExecutionID); ok {
		if extra, ok := val.(map[string]any); ok {
			for k, v := range extra {
				details[k] = v
			}
		}
	}
	if err := s.auditRepo.CreateAuditLog(ctx, &audit.AuditLog{
		UserID:       userID,
		Username:     username,
		Action:       action,
		ResourceType: item.Module,
		ResourceID:   item.ModuleRef,
		ResourceName: item.OperationID,
		RequestID:    item.RequestID,
		ExecutionID:  item.ExecutionID,
		ClientType:   strings.TrimSpace(clientType),
		RiskLevel:    string(item.RiskLevel),
		ResultStatus: strings.TrimSpace(resultStatus),
		Trigger:      trigger,
		Details:      details,
	}); err != nil {
		logger.WarnF(ctx, "[Execution] 保存审计记录失败: execution_id=%s action=%s err=%v", item.ExecutionID, action, err)
	}
}

func authorizeActor(actor Actor, item *Execution) error {
	if item == nil {
		return ErrExecutionNotFound
	}
	if actor.IsAdmin {
		return nil
	}
	if actor.UserID == 0 || item.OwnerUserID == 0 || actor.UserID != item.OwnerUserID {
		return ErrPermissionDenied
	}
	return nil
}
