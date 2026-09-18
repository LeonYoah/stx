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
	"errors"

	"gorm.io/gorm"
)

// Repository 提供升级领域的持久化访问。
// Repository provides persistence access for the upgrade domain.
type Repository struct {
	db *gorm.DB
}

// NewRepository 创建升级仓库实例。
// NewRepository creates a new upgrade repository instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// CreatePlan 创建升级计划快照。
// CreatePlan creates an upgrade plan snapshot.
func (r *Repository) CreatePlan(ctx context.Context, plan *UpgradePlanRecord) error {
	return r.db.WithContext(ctx).Create(plan).Error
}

// GetPlanByID 根据 ID 获取升级计划。
// GetPlanByID gets an upgrade plan by ID.
func (r *Repository) GetPlanByID(ctx context.Context, id uint) (*UpgradePlanRecord, error) {
	var plan UpgradePlanRecord
	if err := r.db.WithContext(ctx).First(&plan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUpgradePlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

// GetPlanByIDForOwner 按用户归属读取升级计划，管理员可查看全部。
// GetPlanByIDForOwner loads an upgrade plan under owner scope, while administrators may view all plans.
func (r *Repository) GetPlanByIDForOwner(ctx context.Context, id, ownerUserID uint, includeAll bool) (*UpgradePlanRecord, error) {
	var plan UpgradePlanRecord
	query := r.db.WithContext(ctx).Where("id = ?", id)
	if !includeAll {
		query = query.Where("created_by = ?", ownerUserID)
	}
	if err := query.First(&plan).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUpgradePlanNotFound
		}
		return nil, err
	}
	return &plan, nil
}

// UpdatePlan 更新升级计划。
// UpdatePlan updates an upgrade plan.
func (r *Repository) UpdatePlan(ctx context.Context, plan *UpgradePlanRecord) error {
	result := r.db.WithContext(ctx).Save(plan)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUpgradePlanNotFound
	}
	return nil
}

// CreateTask 创建升级任务。
// CreateTask creates an upgrade task.
func (r *Repository) CreateTask(ctx context.Context, task *UpgradeTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetTaskByID 获取升级任务及其关联数据。
// GetTaskByID fetches an upgrade task with its related data.
func (r *Repository) GetTaskByID(ctx context.Context, id uint) (*UpgradeTask, error) {
	var task UpgradeTask
	query := r.db.WithContext(ctx).
		Preload("Plan").
		Preload("Steps", func(db *gorm.DB) *gorm.DB {
			return db.Order("sequence ASC")
		}).
		Preload("NodeExecutions", func(db *gorm.DB) *gorm.DB {
			return db.Order("host_id ASC, role ASC")
		})
	if err := query.First(&task, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUpgradeTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// GetTaskByIDForOwner 按用户归属读取升级任务，管理员可查看全部。
// GetTaskByIDForOwner loads an upgrade task under owner scope, while administrators may view all tasks.
func (r *Repository) GetTaskByIDForOwner(ctx context.Context, id, ownerUserID uint, includeAll bool) (*UpgradeTask, error) {
	var task UpgradeTask
	query := r.db.WithContext(ctx).Where("id = ?", id)
	if !includeAll {
		query = query.Where("created_by = ?", ownerUserID)
	}
	if err := query.
		Preload("Plan").
		Preload("Steps", func(db *gorm.DB) *gorm.DB { return db.Order("sequence ASC") }).
		Preload("NodeExecutions", func(db *gorm.DB) *gorm.DB { return db.Order("host_id ASC, role ASC") }).
		First(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUpgradeTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// UpdateTask 更新升级任务。
// UpdateTask updates an upgrade task.
func (r *Repository) UpdateTask(ctx context.Context, task *UpgradeTask) error {
	result := r.db.WithContext(ctx).Save(task)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUpgradeTaskNotFound
	}
	return nil
}

// UpdateTaskFields 按旧状态条件更新升级任务，避免取消与执行起步互相覆盖。
// UpdateTaskFields updates an upgrade task under an expected-status guard to prevent cancellation and startup races.
func (r *Repository) UpdateTaskFields(ctx context.Context, taskID uint, expected []ExecutionStatus, updates map[string]any) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&UpgradeTask{}).
		Where("id = ? AND status IN ?", taskID, expected).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// ListTasks 按过滤条件分页查询升级任务摘要。
// ListTasks queries upgrade task summaries with filters and pagination.
func (r *Repository) ListTasks(ctx context.Context, filter *TaskListFilter) ([]*UpgradeTaskSummary, int64, error) {
	query := r.db.WithContext(ctx).Model(&UpgradeTask{})
	if filter != nil {
		if !filter.IncludeAll && filter.OwnerUserID > 0 {
			query = query.Where("created_by = ?", filter.OwnerUserID)
		}
		if filter.ClusterID > 0 {
			query = query.Where("cluster_id = ?", filter.ClusterID)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := 1
	pageSize := 10
	if filter != nil {
		if filter.Page > 0 {
			page = filter.Page
		}
		if filter.PageSize > 0 {
			pageSize = filter.PageSize
		}
	}

	items := make([]*UpgradeTaskSummary, 0)
	err := query.
		Select([]string{
			"id",
			"execution_id",
			"cluster_id",
			"plan_id",
			"source_version",
			"target_version",
			"status",
			"current_step",
			"failure_step",
			"failure_reason",
			"rollback_status",
			"rollback_reason",
			"started_at",
			"completed_at",
			"created_by",
			"created_at",
			"updated_at",
		}).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// CreateTaskSteps 批量创建升级步骤。
// CreateTaskSteps creates upgrade steps in batch.
func (r *Repository) CreateTaskSteps(ctx context.Context, steps []*UpgradeTaskStep) error {
	if len(steps) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&steps).Error
}

// UpdateTaskStep 更新升级步骤。
// UpdateTaskStep updates an upgrade step.
func (r *Repository) UpdateTaskStep(ctx context.Context, step *UpgradeTaskStep) error {
	result := r.db.WithContext(ctx).Save(step)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUpgradeTaskStepNotFound
	}
	return nil
}

// ListTaskSteps 获取任务步骤列表。
// ListTaskSteps lists steps of a task.
func (r *Repository) ListTaskSteps(ctx context.Context, taskID uint) ([]*UpgradeTaskStep, error) {
	var steps []*UpgradeTaskStep
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("sequence ASC").Find(&steps).Error
	return steps, err
}

// ListTaskStepsForOwner 在父任务归属检查后返回升级步骤。
// ListTaskStepsForOwner returns upgrade steps after checking parent-task ownership.
func (r *Repository) ListTaskStepsForOwner(ctx context.Context, taskID, ownerUserID uint, includeAll bool) ([]*UpgradeTaskStep, error) {
	if _, err := r.GetTaskByIDForOwner(ctx, taskID, ownerUserID, includeAll); err != nil {
		return nil, err
	}
	return r.ListTaskSteps(ctx, taskID)
}

// CreateNodeExecutions 批量创建节点执行记录。
// CreateNodeExecutions creates node execution records in batch.
func (r *Repository) CreateNodeExecutions(ctx context.Context, nodes []*UpgradeNodeExecution) error {
	if len(nodes) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&nodes).Error
}

// UpdateNodeExecution 更新节点执行记录。
// UpdateNodeExecution updates a node execution record.
func (r *Repository) UpdateNodeExecution(ctx context.Context, node *UpgradeNodeExecution) error {
	result := r.db.WithContext(ctx).Save(node)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUpgradeNodeExecutionNotFound
	}
	return nil
}

// ListNodeExecutions 获取任务的节点执行列表。
// ListNodeExecutions lists node executions of a task.
func (r *Repository) ListNodeExecutions(ctx context.Context, taskID uint) ([]*UpgradeNodeExecution, error) {
	var nodes []*UpgradeNodeExecution
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).Order("host_id ASC, role ASC").Find(&nodes).Error
	return nodes, err
}

// CreateStepLog 创建单条步骤日志。
// CreateStepLog creates a single step log.
func (r *Repository) CreateStepLog(ctx context.Context, log *UpgradeStepLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ListStepLogs 按过滤条件分页查询步骤日志。
// ListStepLogs queries step logs with filters and pagination.
func (r *Repository) ListStepLogs(ctx context.Context, filter *StepLogFilter) ([]*UpgradeStepLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&UpgradeStepLog{})
	if filter != nil {
		if !filter.IncludeAll && filter.OwnerUserID > 0 {
			query = query.Where("task_id IN (?)", r.db.WithContext(ctx).Model(&UpgradeTask{}).Select("id").Where("created_by = ?", filter.OwnerUserID))
		}
		if filter.TaskID > 0 {
			query = query.Where("task_id = ?", filter.TaskID)
		}
		if filter.StepCode != "" {
			query = query.Where("step_code = ?", filter.StepCode)
		}
		if filter.NodeExecutionID != nil {
			query = query.Where("node_execution_id = ?", *filter.NodeExecutionID)
		}
		if filter.Level != "" {
			query = query.Where("level = ?", filter.Level)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := 1
	pageSize := 50
	if filter != nil {
		if filter.Page > 0 {
			page = filter.Page
		}
		if filter.PageSize > 0 {
			pageSize = filter.PageSize
		}
	}

	var logs []*UpgradeStepLog
	err := query.Order("created_at ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

// Transaction 在事务中执行升级持久化操作。
// Transaction executes upgrade persistence operations inside a transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}
