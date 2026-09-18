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

package audit

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// Repository provides data access operations for CommandLog and AuditLog entities.
// Repository 提供 CommandLog 和 AuditLog 实体的数据访问操作。
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository instance.
// NewRepository 创建一个新的 Repository 实例。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ============================================================================
// CommandLog Operations - 命令日志操作
// ============================================================================

// CreateCommandLog creates a new command log record in the database.
// CreateCommandLog 在数据库中创建新的命令日志记录。
// Returns ErrCommandIDDuplicate if a command log with the same command ID already exists.
// 如果具有相同命令 ID 的命令日志已存在，则返回 ErrCommandIDDuplicate。
// Requirements: 10.1
func (r *Repository) CreateCommandLog(ctx context.Context, log *CommandLog) error {
	// Validate required fields
	// 验证必填字段
	if log.CommandID == "" {
		return ErrCommandIDEmpty
	}
	if log.AgentID == "" {
		return ErrAgentIDEmpty
	}
	if log.CommandType == "" {
		return ErrCommandTypeEmpty
	}

	// Check for duplicate command ID
	// 检查命令 ID 是否重复
	var count int64
	if err := r.db.WithContext(ctx).Model(&CommandLog{}).Where("command_id = ?", log.CommandID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrCommandIDDuplicate
	}
	log.Parameters = RedactParameters(log.Parameters)
	log.Output = RedactText(log.Output)
	log.Error = RedactText(log.Error)

	return r.db.WithContext(ctx).Create(log).Error
}

// GetCommandLogByID retrieves a command log by its ID.
// GetCommandLogByID 通过 ID 获取命令日志。
// Returns ErrCommandLogNotFound if the command log does not exist.
// 如果命令日志不存在，则返回 ErrCommandLogNotFound。
func (r *Repository) GetCommandLogByID(ctx context.Context, id uint) (*CommandLog, error) {
	var log CommandLog
	if err := r.db.WithContext(ctx).First(&log, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommandLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// GetCommandLogByIDForOwner 按用户归属读取命令日志，管理员可查看全部。
// GetCommandLogByIDForOwner loads a command log under owner scope, while administrators may view all entries.
func (r *Repository) GetCommandLogByIDForOwner(ctx context.Context, id, ownerUserID uint, includeAll bool) (*CommandLog, error) {
	var log CommandLog
	query := r.db.WithContext(ctx).Where("id = ?", id)
	if !includeAll {
		query = query.Where("created_by = ?", ownerUserID)
	}
	if err := query.First(&log).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommandLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// GetCommandLogByCommandID retrieves a command log by its command ID.
// GetCommandLogByCommandID 通过命令 ID 获取命令日志。
// Returns ErrCommandLogNotFound if the command log does not exist.
// 如果命令日志不存在，则返回 ErrCommandLogNotFound。
func (r *Repository) GetCommandLogByCommandID(ctx context.Context, commandID string) (*CommandLog, error) {
	var log CommandLog
	if err := r.db.WithContext(ctx).Where("command_id = ?", commandID).First(&log).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCommandLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// ListCommandLogs retrieves command logs based on filter criteria with pagination.
// ListCommandLogs 根据过滤条件和分页获取命令日志列表。
// Returns the list of command logs and total count.
// 返回命令日志列表和总数。
// Requirements: 10.1, 10.4
func (r *Repository) ListCommandLogs(ctx context.Context, filter *CommandLogFilter) ([]*CommandLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&CommandLog{})

	// Apply filters - 应用过滤条件
	if filter != nil {
		if filter.CommandID != "" {
			query = query.Where("command_id = ?", filter.CommandID)
		}
		if filter.AgentID != "" {
			query = query.Where("agent_id = ?", filter.AgentID)
		}
		if filter.HostID != nil {
			query = query.Where("host_id = ?", *filter.HostID)
		}
		if filter.CommandType != "" {
			query = query.Where("command_type = ?", filter.CommandType)
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.StartTime != nil {
			query = query.Where("created_at >= ?", *filter.StartTime)
		}
		if filter.EndTime != nil {
			query = query.Where("created_at <= ?", *filter.EndTime)
		}
		if filter.CreatedBy != nil {
			query = query.Where("created_by = ?", *filter.CreatedBy)
		}
	}

	// Get total count - 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination - 应用分页
	if filter != nil && filter.PageSize > 0 {
		offset := 0
		if filter.Page > 0 {
			offset = (filter.Page - 1) * filter.PageSize
		}
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	// Execute query - 执行查询
	var logs []*CommandLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// UpdateCommandLog updates an existing command log record.
// UpdateCommandLog 更新现有的命令日志记录。
// Returns ErrCommandLogNotFound if the command log does not exist.
// 如果命令日志不存在，则返回 ErrCommandLogNotFound。
func (r *Repository) UpdateCommandLog(ctx context.Context, log *CommandLog) error {
	// Check if command log exists
	// 检查命令日志是否存在
	var existing CommandLog
	if err := r.db.WithContext(ctx).First(&existing, log.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommandLogNotFound
		}
		return err
	}

	log.Parameters = RedactParameters(log.Parameters)
	log.Output = RedactText(log.Output)
	log.Error = RedactText(log.Error)
	return r.db.WithContext(ctx).Save(log).Error
}

// UpdateCommandLogStatus updates the status and related fields of a command log.
// UpdateCommandLogStatus 更新命令日志的状态和相关字段。
// Returns ErrCommandLogNotFound if the command log does not exist.
// 如果命令日志不存在，则返回 ErrCommandLogNotFound。
func (r *Repository) UpdateCommandLogStatus(ctx context.Context, id uint, updates map[string]interface{}) error {
	if parameters, ok := updates["parameters"].(CommandParameters); ok {
		updates["parameters"] = RedactParameters(parameters)
	}
	for _, key := range []string{"output", "error"} {
		if value, ok := updates[key].(string); ok {
			updates[key] = RedactText(value)
		}
	}
	result := r.db.WithContext(ctx).Model(&CommandLog{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCommandLogNotFound
	}
	return nil
}

// DeleteCommandLog removes a command log record from the database.
// DeleteCommandLog 从数据库中删除命令日志记录。
// Returns ErrCommandLogNotFound if the command log does not exist.
// 如果命令日志不存在，则返回 ErrCommandLogNotFound。
func (r *Repository) DeleteCommandLog(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&CommandLog{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCommandLogNotFound
	}
	return nil
}

// ============================================================================
// AuditLog Operations - 审计日志操作
// ============================================================================

// CreateAuditLog creates a new audit log record in the database.
// CreateAuditLog 在数据库中创建新的审计日志记录。
// Requirements: 10.3
func (r *Repository) CreateAuditLog(ctx context.Context, log *AuditLog) error {
	// Validate required fields
	// 验证必填字段
	if log.Action == "" {
		return ErrActionEmpty
	}
	if log.ResourceType == "" {
		return ErrResourceTypeEmpty
	}
	log.Details = RedactDetails(log.Details)

	return r.db.WithContext(ctx).Create(log).Error
}

// GetAuditLogByID retrieves an audit log by its ID.
// GetAuditLogByID 通过 ID 获取审计日志。
// Returns ErrAuditLogNotFound if the audit log does not exist.
// 如果审计日志不存在，则返回 ErrAuditLogNotFound。
func (r *Repository) GetAuditLogByID(ctx context.Context, id uint) (*AuditLog, error) {
	var log AuditLog
	if err := r.db.WithContext(ctx).First(&log, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAuditLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// GetAuditLogByIDForOwner 按用户归属读取审计记录，管理员可查看全部。
// GetAuditLogByIDForOwner loads an audit entry under owner scope, while administrators may view all entries.
func (r *Repository) GetAuditLogByIDForOwner(ctx context.Context, id, ownerUserID uint, includeAll bool) (*AuditLog, error) {
	var log AuditLog
	query := r.db.WithContext(ctx).Where("id = ?", id)
	if !includeAll {
		query = query.Where("user_id = ?", ownerUserID)
	}
	if err := query.First(&log).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAuditLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// ListAuditLogs retrieves audit logs based on filter criteria with pagination.
// ListAuditLogs 根据过滤条件和分页获取审计日志列表。
// Returns the list of audit logs and total count.
// 返回审计日志列表和总数。
// Requirements: 10.4 - Supports filtering by time range, action type, user, and host.
// 需求: 10.4 - 支持按时间范围、操作类型、用户和主机过滤。
func (r *Repository) ListAuditLogs(ctx context.Context, filter *AuditLogFilter) ([]*AuditLog, int64, error) {
	query := r.db.WithContext(ctx).Model(&AuditLog{})

	// Apply filters - 应用过滤条件
	if filter != nil {
		if !filter.IncludeAll && filter.UserID != nil {
			query = query.Where("user_id = ?", *filter.UserID)
		}
		// Filter by user ID - 按用户 ID 过滤
		if filter.IncludeAll && filter.UserID != nil {
			query = query.Where("user_id = ?", *filter.UserID)
		}
		// Filter by username - 按用户名过滤（使用 LOWER 忽略大小写，兼容多数据库）
		// Filter by username - case-insensitive using LOWER for multi-database compatibility
		if filter.Username != "" {
			query = query.Where("LOWER(username) LIKE LOWER(?)", "%"+filter.Username+"%")
		}
		// Filter by action type - 按操作类型过滤
		if filter.Action != "" {
			query = query.Where("action = ?", filter.Action)
		}
		// Filter by resource type - 按资源类型过滤
		if filter.ResourceType != "" {
			query = query.Where("resource_type = ?", filter.ResourceType)
		}
		// Filter by resource ID - 按资源 ID 过滤
		if filter.ResourceID != "" {
			query = query.Where("resource_id = ?", filter.ResourceID)
		}
		if filter.RequestID != "" {
			query = query.Where("request_id = ?", filter.RequestID)
		}
		if filter.ExecutionID != "" {
			query = query.Where("execution_id = ?", filter.ExecutionID)
		}
		if filter.CommandID != "" {
			query = query.Where("command_id = ?", filter.CommandID)
		}
		if filter.ClientType != "" {
			query = query.Where("client_type = ?", filter.ClientType)
		}
		if filter.ResultStatus != "" {
			query = query.Where("result_status = ?", filter.ResultStatus)
		}
		// Filter by trigger column - 按 trigger 字段过滤
		if filter.Trigger == "auto" || filter.Trigger == "manual" {
			query = query.Where("trigger = ?", filter.Trigger)
		}
		// Filter by time range - 按时间范围过滤
		if filter.StartTime != nil {
			query = query.Where("created_at >= ?", *filter.StartTime)
		}
		if filter.EndTime != nil {
			query = query.Where("created_at <= ?", *filter.EndTime)
		}
	}

	// Get total count - 获取总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination - 应用分页
	if filter != nil && filter.PageSize > 0 {
		offset := 0
		if filter.Page > 0 {
			offset = (filter.Page - 1) * filter.PageSize
		}
		query = query.Offset(offset).Limit(filter.PageSize)
	}

	// Execute query - 执行查询
	var logs []*AuditLog
	if err := query.Order("created_at DESC").Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// DeleteAuditLog removes an audit log record from the database.
// DeleteAuditLog 从数据库中删除审计日志记录。
// Returns ErrAuditLogNotFound if the audit log does not exist.
// 如果审计日志不存在，则返回 ErrAuditLogNotFound。
func (r *Repository) DeleteAuditLog(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&AuditLog{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAuditLogNotFound
	}
	return nil
}

// DeleteAuditLogsBefore deletes audit logs created before the specified time.
// DeleteAuditLogsBefore 删除指定时间之前创建的审计日志。
// This is useful for implementing log retention policies.
// 这对于实现日志保留策略很有用。
// Requirements: 10.5
func (r *Repository) DeleteAuditLogsBefore(ctx context.Context, before interface{}) (int64, error) {
	result := r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&AuditLog{})
	return result.RowsAffected, result.Error
}

// DeleteCommandLogsBefore deletes command logs created before the specified time.
// DeleteCommandLogsBefore 删除指定时间之前创建的命令日志。
// This is useful for implementing log retention policies.
// 这对于实现日志保留策略很有用。
func (r *Repository) DeleteCommandLogsBefore(ctx context.Context, before interface{}) (int64, error) {
	result := r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&CommandLog{})
	return result.RowsAffected, result.Error
}

// ExistsByCommandID checks if a command log with the given command ID exists.
// ExistsByCommandID 检查具有给定命令 ID 的命令日志是否存在。
func (r *Repository) ExistsByCommandID(ctx context.Context, commandID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&CommandLog{}).Where("command_id = ?", commandID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
