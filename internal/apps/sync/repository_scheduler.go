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
	"errors"
	"strings"

	"gorm.io/gorm"
)

var activeJobStatuses = []JobStatus{
	JobStatusPending,
	JobStatusRunning,
	JobStatusCancelRequested,
	JobStatusCancelling,
}

// GetActiveJobByExternalRef 返回同一外部引用上仍在运行或停止中的作业。
// GetActiveJobByExternalRef returns the in-flight job that currently owns the external reference.
func (r *Repository) GetActiveJobByExternalRef(ctx context.Context, ref string) (*JobInstance, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	var instance JobInstance
	err := r.db.WithContext(ctx).
		Where("external_ref = ? AND status IN ?", ref, activeJobStatuses).
		Order("id DESC").
		First(&instance).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &instance, nil
}

// ListRecentJobsByExternalRefPrefix 列出任务上带外部引用的最近作业。
// ListRecentJobsByExternalRefPrefix lists recent jobs for a task that carry an external reference.
func (r *Repository) ListRecentJobsByExternalRefPrefix(ctx context.Context, taskID uint, prefix string, limit int) ([]*JobInstance, error) {
	if limit <= 0 {
		limit = 10
	}
	query := r.db.WithContext(ctx).Where("task_id = ?", taskID)
	if strings.TrimSpace(prefix) != "" {
		query = query.Where("external_ref LIKE ?", strings.TrimSpace(prefix)+"%")
	} else {
		query = query.Where("external_ref <> ''")
	}
	var items []*JobInstance
	if err := query.Order("created_at DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	if items == nil {
		items = []*JobInstance{}
	}
	return items, nil
}

// CreateSchedulerConnection 保存一条调度器连接。
// CreateSchedulerConnection stores one scheduler connection.
func (r *Repository) CreateSchedulerConnection(ctx context.Context, item *SchedulerConnection) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// GetSchedulerConnectionByID 按编号读取调度器连接。
// GetSchedulerConnectionByID loads a scheduler connection by id.
func (r *Repository) GetSchedulerConnectionByID(ctx context.Context, id uint) (*SchedulerConnection, error) {
	var item SchedulerConnection
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSchedulerConnectionNotFound
		}
		return nil, err
	}
	return &item, nil
}

// ListSchedulerConnections 列出全部调度器连接。
// ListSchedulerConnections lists every scheduler connection.
func (r *Repository) ListSchedulerConnections(ctx context.Context) ([]*SchedulerConnection, error) {
	var items []*SchedulerConnection
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	if items == nil {
		items = []*SchedulerConnection{}
	}
	return items, nil
}

// UpdateSchedulerConnection 保存调度器连接的更新。
// UpdateSchedulerConnection persists scheduler connection updates.
func (r *Repository) UpdateSchedulerConnection(ctx context.Context, item *SchedulerConnection) error {
	result := r.db.WithContext(ctx).Save(item)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSchedulerConnectionNotFound
	}
	return nil
}

// DeleteSchedulerConnection 删除调度器连接。
// DeleteSchedulerConnection deletes a scheduler connection.
func (r *Repository) DeleteSchedulerConnection(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&SchedulerConnection{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSchedulerConnectionNotFound
	}
	return nil
}

// GetSchedulerBindingByTaskID 读取任务的 Dolphin 绑定。
// GetSchedulerBindingByTaskID loads the Dolphin binding for a task.
func (r *Repository) GetSchedulerBindingByTaskID(ctx context.Context, taskID uint) (*TaskSchedulerBinding, error) {
	var item TaskSchedulerBinding
	if err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// SaveSchedulerBinding 创建或更新任务绑定。
// SaveSchedulerBinding creates or updates a task binding.
func (r *Repository) SaveSchedulerBinding(ctx context.Context, item *TaskSchedulerBinding) error {
	if item == nil {
		return ErrSchedulerBindingNotFound
	}
	existing, err := r.GetSchedulerBindingByTaskID(ctx, item.TaskID)
	if err != nil {
		return err
	}
	if existing == nil {
		return r.db.WithContext(ctx).Create(item).Error
	}
	item.ID = existing.ID
	item.CreatedAt = existing.CreatedAt
	return r.db.WithContext(ctx).Save(item).Error
}

// DeleteSchedulerBindingByTaskID 删除任务绑定，不影响 Dolphin 侧节点。
// DeleteSchedulerBindingByTaskID removes the STX binding without touching the Dolphin node.
func (r *Repository) DeleteSchedulerBindingByTaskID(ctx context.Context, taskID uint) error {
	result := r.db.WithContext(ctx).Where("task_id = ?", taskID).Delete(&TaskSchedulerBinding{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSchedulerBindingNotFound
	}
	return nil
}
