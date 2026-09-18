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
	"time"

	"gorm.io/gorm"
)

// Repository 负责公共执行记录和一次性确认记录的持久化。
// Repository persists shared execution records and one-time confirmations.
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建公共执行仓储。
// NewRepository creates a shared execution repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreateOrGet 按幂等约定创建执行记录，或返回同一请求已创建的记录。
// CreateOrGet creates an execution under the idempotency contract or returns the record created by the same request.
func (r *Repository) CreateOrGet(ctx context.Context, item *Execution) (*Execution, bool, error) {
	if item == nil {
		return nil, false, ErrInvalidExecution
	}
	if item.IdempotencyKeyHash == nil {
		if err := r.db.WithContext(ctx).Create(item).Error; err != nil {
			return nil, false, err
		}
		return item, true, nil
	}

	existing, err := r.findByIdempotency(ctx, item.OwnerUserID, item.OperationID, *item.IdempotencyKeyHash)
	if err == nil {
		if existing.RequestHash != item.RequestHash {
			return nil, false, ErrIdempotencyConflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrExecutionNotFound) {
		return nil, false, err
	}

	if err := r.db.WithContext(ctx).Create(item).Error; err == nil {
		return item, true, nil
	}

	// 并发创建可能由唯一索引拒绝，此时重新读取并核对请求摘要。
	// A concurrent create may be rejected by the unique index, so reload and compare the request digest.
	existing, reloadErr := r.findByIdempotency(ctx, item.OwnerUserID, item.OperationID, *item.IdempotencyKeyHash)
	if reloadErr != nil {
		return nil, false, reloadErr
	}
	if existing.RequestHash != item.RequestHash {
		return nil, false, ErrIdempotencyConflict
	}
	return existing, false, nil
}

// GetByExecutionID 按对外执行编号读取记录。
// GetByExecutionID loads a record by its public execution ID.
func (r *Repository) GetByExecutionID(ctx context.Context, executionID string) (*Execution, error) {
	var item Execution
	if err := r.db.WithContext(ctx).Where("execution_id = ?", executionID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}
	return &item, nil
}

// BindModuleRef 将公共执行记录绑定到业务模块任务，且不覆盖已经存在的绑定。
// BindModuleRef binds a shared execution to its business task without replacing an existing binding.
func (r *Repository) BindModuleRef(ctx context.Context, executionID, moduleRef string) error {
	result := r.db.WithContext(ctx).Model(&Execution{}).
		Where("execution_id = ? AND (module_ref = '' OR module_ref IS NULL)", executionID).
		Update("module_ref", moduleRef)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		item, err := r.GetByExecutionID(ctx, executionID)
		if err != nil {
			return err
		}
		if item.ModuleRef != moduleRef {
			return ErrConcurrentUpdate
		}
	}
	return nil
}

// UpdateStatus 使用旧状态条件更新记录，防止迟到回调覆盖终态。
// UpdateStatus updates a record with an expected-state condition so late callbacks cannot overwrite terminal states.
func (r *Repository) UpdateStatus(ctx context.Context, executionID string, expected []Status, updates map[string]any) error {
	if len(expected) == 0 {
		return ErrInvalidTransition
	}
	result := r.db.WithContext(ctx).Model(&Execution{}).
		Where("execution_id = ? AND status IN ?", executionID, expected).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if _, err := r.GetByExecutionID(ctx, executionID); err != nil {
			return err
		}
		return ErrConcurrentUpdate
	}
	return nil
}

// CreateConfirmation 保存一次性确认记录。
// CreateConfirmation stores a one-time confirmation record.
func (r *Repository) CreateConfirmation(ctx context.Context, item *Confirmation) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// ConsumeConfirmation 原子消费仍有效且尚未使用的确认记录。
// ConsumeConfirmation atomically consumes a confirmation that is valid and unused.
func (r *Repository) ConsumeConfirmation(ctx context.Context, confirmationHash string, ownerUserID uint64, operationID, idempotencyKeyHash, requestHash string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&Confirmation{}).
		Where("confirmation_hash = ?", confirmationHash).
		Where("owner_user_id = ? AND operation_id = ?", ownerUserID, operationID).
		Where("idempotency_key_hash = ? AND request_hash = ?", idempotencyKeyHash, requestHash).
		Where("consumed_at IS NULL AND expires_at > ?", now).
		Update("consumed_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConfirmationInvalid
	}
	return nil
}

func (r *Repository) findByIdempotency(ctx context.Context, ownerUserID uint64, operationID, keyHash string) (*Execution, error) {
	var item Execution
	if err := r.db.WithContext(ctx).
		Where("owner_user_id = ? AND operation_id = ? AND idempotency_key_hash = ?", ownerUserID, operationID, keyHash).
		First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExecutionNotFound
		}
		return nil, err
	}
	return &item, nil
}
