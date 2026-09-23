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
	"time"

	"gorm.io/gorm"
)

// Repository provides persistence operations for sync tasks and instances.
type Repository struct {
	db *gorm.DB
}

// DeleteTaskVersionsByTaskIDs deletes snapshots for the provided task ids.
func (r *Repository) DeleteTaskVersionsByTaskIDs(ctx context.Context, taskIDs []uint) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("task_id IN ?", taskIDs).Delete(&TaskVersion{}).Error
}

// DeleteJobInstancesByTaskIDs deletes run history for the provided task ids.
func (r *Repository) DeleteJobInstancesByTaskIDs(ctx context.Context, taskIDs []uint) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("task_id IN ?", taskIDs).Delete(&JobInstance{}).Error
}

// DeletePreviewRowsByTaskIDs deletes preview rows for the provided task ids.
// DeletePreviewRowsByTaskIDs 删除指定任务的预览数据行。
func (r *Repository) DeletePreviewRowsByTaskIDs(ctx context.Context, taskIDs []uint) error {
	if len(taskIDs) == 0 {
		return nil
	}
	subQuery := r.db.WithContext(ctx).Model(&PreviewSession{}).Select("id").Where("task_id IN ?", taskIDs)
	return r.db.WithContext(ctx).Where("session_id IN (?)", subQuery).Delete(&PreviewRow{}).Error
}

// DeletePreviewTablesByTaskIDs deletes preview tables for the provided task ids.
// DeletePreviewTablesByTaskIDs 删除指定任务的预览表分组。
func (r *Repository) DeletePreviewTablesByTaskIDs(ctx context.Context, taskIDs []uint) error {
	if len(taskIDs) == 0 {
		return nil
	}
	subQuery := r.db.WithContext(ctx).Model(&PreviewSession{}).Select("id").Where("task_id IN ?", taskIDs)
	return r.db.WithContext(ctx).Where("session_id IN (?)", subQuery).Delete(&PreviewTable{}).Error
}

// DeletePreviewSessionsByTaskIDs deletes preview sessions for the provided task ids.
// DeletePreviewSessionsByTaskIDs 删除指定任务的预览会话。
func (r *Repository) DeletePreviewSessionsByTaskIDs(ctx context.Context, taskIDs []uint) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("task_id IN ?", taskIDs).Delete(&PreviewSession{}).Error
}

// DeleteTasksByIDs deletes the provided workspace nodes.
func (r *Repository) DeleteTasksByIDs(ctx context.Context, taskIDs []uint) error {
	if len(taskIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", taskIDs).Delete(&Task{}).Error
}

// NewRepository creates a new sync repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Transaction executes fn inside one database transaction.
func (r *Repository) Transaction(ctx context.Context, fn func(tx *Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{db: tx})
	})
}

// CreateTask creates a sync task.
func (r *Repository) CreateTask(ctx context.Context, task *Task) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetTaskByID returns one task by id.
func (r *Repository) GetTaskByID(ctx context.Context, id uint) (*Task, error) {
	var task Task
	if err := r.db.WithContext(ctx).First(&task, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

// ListTasks lists tasks with filter and pagination.
func (r *Repository) ListTasks(ctx context.Context, filter *TaskFilter) ([]*Task, int64, error) {
	query := r.db.WithContext(ctx).Model(&Task{})
	if filter != nil {
		if filter.Name != "" {
			// 按任务名称模糊过滤（使用 LOWER 忽略大小写，兼容多数据库）
			// Filter by task name (case-insensitive using LOWER for multi-database compatibility)
			query = query.Where("LOWER(name) LIKE LOWER(?)", "%"+filter.Name+"%")
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filter != nil && filter.Size > 0 {
		offset := 0
		if filter.Page > 1 {
			offset = (filter.Page - 1) * filter.Size
		}
		query = query.Offset(offset).Limit(filter.Size)
	}

	var tasks []*Task
	if err := query.Order("node_type ASC").Order("sort_order ASC").Order("updated_at DESC").Find(&tasks).Error; err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// ListAllTasks returns all nodes for tree building.
func (r *Repository) ListAllTasks(ctx context.Context) ([]*Task, error) {
	var tasks []*Task
	if err := r.db.WithContext(ctx).
		Order("sort_order ASC").
		Order("CASE WHEN parent_id IS NULL THEN 0 ELSE 1 END ASC").
		Order("node_type ASC").
		Order("name ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// UpdateTask updates a sync task.
func (r *Repository) UpdateTask(ctx context.Context, task *Task) error {
	result := r.db.WithContext(ctx).Save(task)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// ExistsSiblingTaskName reports whether the same folder already contains a node with the provided name.
// ExistsSiblingTaskName 返回同级目录下是否已存在同名节点。
func (r *Repository) ExistsSiblingTaskName(ctx context.Context, parentID *uint, name string, excludeID *uint) (bool, error) {
	query := r.db.WithContext(ctx).Model(&Task{}).Where("name = ?", strings.TrimSpace(name))
	if parentID == nil || *parentID == 0 {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if excludeID != nil && *excludeID > 0 {
		query = query.Where("id <> ?", *excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateTaskVersion stores one immutable task snapshot.
func (r *Repository) CreateTaskVersion(ctx context.Context, version *TaskVersion) error {
	return r.db.WithContext(ctx).Create(version).Error
}

// ListTaskVersionsByTaskID lists immutable snapshots for one task.
func (r *Repository) ListTaskVersionsByTaskID(ctx context.Context, taskID uint) ([]*TaskVersion, error) {
	items, _, err := r.ListTaskVersionsByTaskIDPaginated(ctx, taskID, 0, 0)
	return items, err
}

// ListTaskVersionsByTaskIDPaginated lists immutable snapshots for one task with pagination.
func (r *Repository) ListTaskVersionsByTaskIDPaginated(ctx context.Context, taskID uint, page, size int) ([]*TaskVersion, int64, error) {
	query := r.db.WithContext(ctx).Model(&TaskVersion{}).Where("task_id = ?", taskID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var versions []*TaskVersion
	if size > 0 {
		offset := 0
		if page > 1 {
			offset = (page - 1) * size
		}
		query = query.Offset(offset).Limit(size)
	}
	if err := query.Order("version DESC, created_at DESC").Find(&versions).Error; err != nil {
		return nil, 0, err
	}
	return versions, total, nil
}

// GetTaskVersionByID returns one immutable task snapshot.
func (r *Repository) GetTaskVersionByID(ctx context.Context, taskID uint, versionID uint) (*TaskVersion, error) {
	var version TaskVersion
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		First(&version, versionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskVersionNotFound
		}
		return nil, err
	}
	return &version, nil
}

// GetTaskVersionByVersion returns one immutable task snapshot by version number.
func (r *Repository) GetTaskVersionByVersion(ctx context.Context, taskID uint, version int) (*TaskVersion, error) {
	var item TaskVersion
	if err := r.db.WithContext(ctx).Where("task_id = ? AND version = ?", taskID, version).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskVersionNotFound
		}
		return nil, err
	}
	return &item, nil
}

// HasScheduledJobInstanceInWindow reports whether a scheduled job already exists in the target trigger window.
func (r *Repository) HasScheduledJobInstanceInWindow(ctx context.Context, taskID uint, startedAt time.Time, finishedAt time.Time) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&JobInstance{}).
		Where("task_id = ?", taskID).
		Where("run_type = ?", RunTypeSchedule).
		Where("created_at >= ? AND created_at < ?", startedAt, finishedAt).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetLatestScheduledJobInstancesByTaskIDs returns the latest scheduled job instance keyed by task id.
func (r *Repository) GetLatestScheduledJobInstancesByTaskIDs(ctx context.Context, taskIDs []uint) (map[uint]*JobInstance, error) {
	result := make(map[uint]*JobInstance)
	if len(taskIDs) == 0 {
		return result, nil
	}
	var instances []*JobInstance
	if err := r.db.WithContext(ctx).
		Where("task_id IN ?", taskIDs).
		Where("run_type = ?", RunTypeSchedule).
		Order("task_id ASC").
		Order("created_at DESC").
		Find(&instances).Error; err != nil {
		return nil, err
	}
	for _, item := range instances {
		if item == nil {
			continue
		}
		if _, ok := result[item.TaskID]; ok {
			continue
		}
		result[item.TaskID] = item
	}
	return result, nil
}

// DeleteTaskVersion removes one immutable task snapshot.
func (r *Repository) DeleteTaskVersion(ctx context.Context, taskID uint, versionID uint) error {
	result := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Delete(&TaskVersion{}, versionID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskVersionNotFound
	}
	return nil
}

// CreateJobInstance creates a new job instance record.
func (r *Repository) CreateJobInstance(ctx context.Context, instance *JobInstance) error {
	return r.db.WithContext(ctx).Create(instance).Error
}

// GetJobInstanceByID returns one job instance by id.
func (r *Repository) GetJobInstanceByID(ctx context.Context, id uint) (*JobInstance, error) {
	var instance JobInstance
	if err := r.db.WithContext(ctx).First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobInstanceNotFound
		}
		return nil, err
	}
	return &instance, nil
}

// GetJobInstanceByIDForOwner 按用户归属读取作业实例，管理员可跳过归属限制。
// GetJobInstanceByIDForOwner loads a job instance under owner scope, while administrators may bypass the owner filter.
func (r *Repository) GetJobInstanceByIDForOwner(ctx context.Context, id, ownerUserID uint, includeAll bool) (*JobInstance, error) {
	query := r.db.WithContext(ctx).Where("id = ?", id)
	if !includeAll {
		query = query.Where("created_by = ?", ownerUserID)
	}
	var instance JobInstance
	if err := query.First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobInstanceNotFound
		}
		return nil, err
	}
	return &instance, nil
}

// ListJobInstances lists job instances with filter and pagination.
func (r *Repository) ListJobInstances(ctx context.Context, filter *JobFilter) ([]*JobInstance, int64, error) {
	query := r.db.WithContext(ctx).Model(&JobInstance{})
	if filter != nil {
		if !filter.IncludeAll {
			query = query.Where("created_by = ?", filter.OwnerUserID)
		}
		if filter.TaskID > 0 {
			query = query.Where("task_id = ?", filter.TaskID)
		}
		if filter.RunType != "" {
			query = query.Where("run_type = ?", filter.RunType)
		}
		if strings.TrimSpace(filter.PlatformJobID) != "" {
			query = query.Where("platform_job_id = ?", strings.TrimSpace(filter.PlatformJobID))
		}
		if strings.TrimSpace(filter.EngineJobID) != "" {
			query = query.Where("engine_job_id = ?", strings.TrimSpace(filter.EngineJobID))
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if filter != nil && filter.Size > 0 {
		offset := 0
		if filter.Page > 1 {
			offset = (filter.Page - 1) * filter.Size
		}
		query = query.Offset(offset).Limit(filter.Size)
	}

	var instances []*JobInstance
	if err := query.Order("created_at DESC").Find(&instances).Error; err != nil {
		return nil, 0, err
	}
	return instances, total, nil
}

// UpdateJobStatus 使用旧状态条件更新作业，防止完成回调覆盖取消状态。
// UpdateJobStatus updates a job under an expected-status condition so completion callbacks cannot overwrite cancellation state.
func (r *Repository) UpdateJobStatus(ctx context.Context, id uint, expected []JobStatus, updates map[string]any) error {
	if len(expected) == 0 {
		return ErrJobStatusChanged
	}
	result := r.db.WithContext(ctx).Model(&JobInstance{}).
		Where("id = ? AND status IN ?", id, expected).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if _, err := r.GetJobInstanceByID(ctx, id); err != nil {
			return err
		}
		return ErrJobStatusChanged
	}
	return nil
}

// GetPreviewJobInstanceByPlatformOrEngineJobID retrieves one preview job by platform or engine job id.
func (r *Repository) GetPreviewJobInstanceByPlatformOrEngineJobID(ctx context.Context, platformJobID string, engineJobID string) (*JobInstance, error) {
	query := r.db.WithContext(ctx).Where("run_type = ?", RunTypePreview)
	if strings.TrimSpace(platformJobID) != "" {
		query = query.Where("platform_job_id = ?", strings.TrimSpace(platformJobID))
	} else if strings.TrimSpace(engineJobID) != "" {
		query = query.Where("engine_job_id = ?", strings.TrimSpace(engineJobID))
	} else {
		return nil, ErrJobInstanceNotFound
	}
	var instance JobInstance
	if err := query.Order("created_at DESC").First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrJobInstanceNotFound
		}
		return nil, err
	}
	return &instance, nil
}

// UpdateJobInstance updates one job instance.
func (r *Repository) UpdateJobInstance(ctx context.Context, instance *JobInstance) error {
	result := r.db.WithContext(ctx).Save(instance)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrJobInstanceNotFound
	}
	return nil
}

// CreatePreviewSession creates one preview session.
// CreatePreviewSession 创建一条预览会话记录。
func (r *Repository) CreatePreviewSession(ctx context.Context, session *PreviewSession) error {
	return r.db.WithContext(ctx).Create(session).Error
}

// GetPreviewSessionByJobInstanceID returns the preview session for one job instance.
// GetPreviewSessionByJobInstanceID 返回指定作业实例的预览会话。
func (r *Repository) GetPreviewSessionByJobInstanceID(ctx context.Context, jobInstanceID uint) (*PreviewSession, error) {
	var session PreviewSession
	if err := r.db.WithContext(ctx).Where("job_instance_id = ?", jobInstanceID).First(&session).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPreviewSessionNotFound
		}
		return nil, err
	}
	return &session, nil
}

// UpdatePreviewSession updates one preview session.
// UpdatePreviewSession 更新一条预览会话。
func (r *Repository) UpdatePreviewSession(ctx context.Context, session *PreviewSession) error {
	result := r.db.WithContext(ctx).Save(session)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrJobInstanceNotFound
	}
	return nil
}

// GetPreviewTableByPath returns one preview table by session and table path.
// GetPreviewTableByPath 按会话和表路径返回一条预览表记录。
func (r *Repository) GetPreviewTableByPath(ctx context.Context, sessionID uint, tablePath string) (*PreviewTable, error) {
	var table PreviewTable
	if err := r.db.WithContext(ctx).Where("session_id = ? AND table_path = ?", sessionID, strings.TrimSpace(tablePath)).First(&table).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &table, nil
}

// CreatePreviewTable creates one preview table.
// CreatePreviewTable 创建一条预览表记录。
func (r *Repository) CreatePreviewTable(ctx context.Context, table *PreviewTable) error {
	return r.db.WithContext(ctx).Create(table).Error
}

// UpdatePreviewTable updates one preview table.
// UpdatePreviewTable 更新一条预览表记录。
func (r *Repository) UpdatePreviewTable(ctx context.Context, table *PreviewTable) error {
	result := r.db.WithContext(ctx).Save(table)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// CreatePreviewRows appends preview rows in batch.
// CreatePreviewRows 批量追加预览数据行。
func (r *Repository) CreatePreviewRows(ctx context.Context, rows []*PreviewRow) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

// DeletePreviewRowsByTableID deletes preview rows by table.
// DeletePreviewRowsByTableID 按表删除预览数据行。
func (r *Repository) DeletePreviewRowsByTableID(ctx context.Context, tableID uint) error {
	return r.db.WithContext(ctx).Where("table_id = ?", tableID).Delete(&PreviewRow{}).Error
}

// ListPreviewTablesBySessionID lists preview tables for one session.
// ListPreviewTablesBySessionID 返回一个会话下的预览表列表。
func (r *Repository) ListPreviewTablesBySessionID(ctx context.Context, sessionID uint) ([]*PreviewTable, error) {
	var items []*PreviewTable
	if err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("table_path ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListPreviewRowsByTableID lists preview rows for one table with pagination.
// ListPreviewRowsByTableID 按表分页返回预览数据行。
func (r *Repository) ListPreviewRowsByTableID(ctx context.Context, tableID uint, offset, limit int) ([]*PreviewRow, error) {
	query := r.db.WithContext(ctx).Where("table_id = ?", tableID).Order("row_index ASC")
	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var items []*PreviewRow
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListExpiredPreviewSessions returns finished preview sessions older than cutoff.
// ListExpiredPreviewSessions 返回超过截止时间的已完成预览会话。
func (r *Repository) ListExpiredPreviewSessions(ctx context.Context, cutoff time.Time) ([]*PreviewSession, error) {
	var items []*PreviewSession
	if err := r.db.WithContext(ctx).
		Where("finished_at IS NOT NULL AND finished_at < ?", cutoff).
		Order("finished_at ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListTimedOutPreviewSessions returns collecting preview sessions that exceeded timeout.
// ListTimedOutPreviewSessions 返回超过超时时间仍在收集中预览会话。
func (r *Repository) ListTimedOutPreviewSessions(ctx context.Context, now time.Time) ([]*PreviewSession, error) {
	var items []*PreviewSession
	if err := r.db.WithContext(ctx).
		Where("finished_at IS NULL AND status = ?", "collecting").
		Find(&items).Error; err != nil {
		return nil, err
	}
	result := make([]*PreviewSession, 0, len(items))
	for _, item := range items {
		if item == nil || item.StartedAt == nil {
			continue
		}
		timeoutMinutes := item.TimeoutMinutes
		if timeoutMinutes <= 0 {
			timeoutMinutes = 10
		}
		if item.StartedAt.Add(time.Duration(timeoutMinutes) * time.Minute).Before(now) {
			result = append(result, item)
		}
	}
	return result, nil
}

// DeletePreviewRowsBySessionIDs deletes preview rows for sessions.
// DeletePreviewRowsBySessionIDs 删除指定会话的预览数据行。
func (r *Repository) DeletePreviewRowsBySessionIDs(ctx context.Context, sessionIDs []uint) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("session_id IN ?", sessionIDs).Delete(&PreviewRow{}).Error
}

// DeletePreviewTablesBySessionIDs deletes preview tables for sessions.
// DeletePreviewTablesBySessionIDs 删除指定会话的预览表。
func (r *Repository) DeletePreviewTablesBySessionIDs(ctx context.Context, sessionIDs []uint) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("session_id IN ?", sessionIDs).Delete(&PreviewTable{}).Error
}

// DeletePreviewSessionsByIDs deletes preview sessions by ids.
// DeletePreviewSessionsByIDs 删除指定预览会话。
func (r *Repository) DeletePreviewSessionsByIDs(ctx context.Context, sessionIDs []uint) error {
	if len(sessionIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", sessionIDs).Delete(&PreviewSession{}).Error
}

// ListGlobalVariables returns all global variables ordered by key.
func (r *Repository) ListGlobalVariables(ctx context.Context) ([]*GlobalVariable, error) {
	items, _, err := r.ListGlobalVariablesPaginated(ctx, 0, 0)
	return items, err
}

// ListGlobalVariablesPaginated returns global variables ordered by key with pagination.
func (r *Repository) ListGlobalVariablesPaginated(ctx context.Context, page, size int) ([]*GlobalVariable, int64, error) {
	query := r.db.WithContext(ctx).Model(&GlobalVariable{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []*GlobalVariable
	if size > 0 {
		offset := 0
		if page > 1 {
			offset = (page - 1) * size
		}
		query = query.Offset(offset).Limit(size)
	}
	if err := query.Order("key ASC").Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// GetGlobalVariableByID returns one global variable by id.
func (r *Repository) GetGlobalVariableByID(ctx context.Context, id uint) (*GlobalVariable, error) {
	var item GlobalVariable
	if err := r.db.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGlobalVariableNotFound
		}
		return nil, err
	}
	return &item, nil
}

// GetGlobalVariableByKey returns one global variable by key.
func (r *Repository) GetGlobalVariableByKey(ctx context.Context, key string) (*GlobalVariable, error) {
	var item GlobalVariable
	if err := r.db.WithContext(ctx).Where("key = ?", strings.TrimSpace(key)).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGlobalVariableNotFound
		}
		return nil, err
	}
	return &item, nil
}

// CreateGlobalVariable creates one global variable.
func (r *Repository) CreateGlobalVariable(ctx context.Context, item *GlobalVariable) error {
	return r.db.WithContext(ctx).Create(item).Error
}

// UpdateGlobalVariable updates one global variable.
func (r *Repository) UpdateGlobalVariable(ctx context.Context, item *GlobalVariable) error {
	result := r.db.WithContext(ctx).Save(item)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrGlobalVariableNotFound
	}
	return nil
}

// DeleteGlobalVariable deletes one global variable.
func (r *Repository) DeleteGlobalVariable(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&GlobalVariable{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrGlobalVariableNotFound
	}
	return nil
}
