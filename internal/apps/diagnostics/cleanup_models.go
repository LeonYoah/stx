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
	"time"
)

const (
	// maxInspectionRetentionDays 防止误输入天文数字让记录事实上永不清理。
	// maxInspectionRetentionDays prevents absurd inputs that would effectively disable cleanup.
	maxInspectionRetentionDays = 3650
	// maxInspectionReportKeep 同样是输入兜底上限。
	// maxInspectionReportKeep is the input-sanity cap for report count retention.
	maxInspectionReportKeep = 100000
	// inspectionDeleteBatchLimit 限制单次删除规模，避免长事务与超大响应体。
	// inspectionDeleteBatchLimit caps one delete call so transactions stay short.
	inspectionDeleteBatchLimit = 500
)

// InspectionCleanupSettings 是单行全局巡检清理设置（diagnostics_cleanup_settings）：
// 保留天数与最多份数，0 表示对应维度不清理，对所有触发来源一视同仁。
// InspectionCleanupSettings is the single-row global inspection cleanup settings
// (diagnostics_cleanup_settings): retention days and max report count, 0 disables each
// dimension, applied to every trigger source alike.
type InspectionCleanupSettings struct {
	ID            uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	RetentionDays int       `json:"retention_days"`
	MaxReports    int       `json:"max_reports"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the inspection cleanup settings table name.
// TableName 指定巡检清理设置表名。
func (InspectionCleanupSettings) TableName() string {
	return "diagnostics_cleanup_settings"
}

// InspectionCleanupSettingsInfo 是清理设置的 API 视图模型。
// InspectionCleanupSettingsInfo is the API view model for cleanup settings.
type InspectionCleanupSettingsInfo struct {
	RetentionDays int       `json:"retention_days"`
	MaxReports    int       `json:"max_reports"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
}

// ToInfo 将存储行转为 API 视图。
// ToInfo converts the stored row into the API view.
func (s *InspectionCleanupSettings) ToInfo() *InspectionCleanupSettingsInfo {
	if s == nil {
		return &InspectionCleanupSettingsInfo{}
	}
	return &InspectionCleanupSettingsInfo{
		RetentionDays: s.RetentionDays,
		MaxReports:    s.MaxReports,
		UpdatedAt:     s.UpdatedAt,
	}
}

// UpdateInspectionCleanupSettingsRequest 描述一次清理设置更新。
// UpdateInspectionCleanupSettingsRequest describes one cleanup settings update.
type UpdateInspectionCleanupSettingsRequest struct {
	RetentionDays *int `json:"retention_days"`
	MaxReports    *int `json:"max_reports"`
}

// InspectionDeleteResultCode 标识一条巡检记录删除尝试的结果。
// InspectionDeleteResultCode identifies the outcome of one inspection record delete attempt.
type InspectionDeleteResultCode string

const (
	// InspectionDeleteResultDeleted 记录与绑定物已删除。
	// InspectionDeleteResultDeleted means the record and its bound objects were deleted.
	InspectionDeleteResultDeleted InspectionDeleteResultCode = "deleted"
	// InspectionDeleteResultNotFound 记录不存在。
	// InspectionDeleteResultNotFound means the record does not exist.
	InspectionDeleteResultNotFound InspectionDeleteResultCode = "not_found"
	// InspectionDeleteResultReportRunning 记录尚未终态，不允许删除。
	// InspectionDeleteResultReportRunning means the record is not terminal yet.
	InspectionDeleteResultReportRunning InspectionDeleteResultCode = "skipped_running"
	// InspectionDeleteResultTaskRunning 绑定的诊断任务尚未终态，本次跳过。
	// InspectionDeleteResultTaskRunning means a bound diagnostic task is not terminal yet.
	InspectionDeleteResultTaskRunning InspectionDeleteResultCode = "skipped_task_running"
)

// InspectionDeleteResult 是逐条删除结果，批量删除按 id 原样回传。
// InspectionDeleteResult is the per-record delete outcome, returned as-is per id.
type InspectionDeleteResult struct {
	ID     uint                       `json:"id"`
	Result InspectionDeleteResultCode `json:"result"`
	Reason string                     `json:"reason,omitempty"`
	TaskID uint                       `json:"task_id,omitempty"`
}

// BatchDeleteInspectionReportsRequest 描述一次巡检记录批量删除请求。
// BatchDeleteInspectionReportsRequest describes one inspection batch delete request.
type BatchDeleteInspectionReportsRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

// BatchDeleteInspectionReportsData 是批量删除响应载荷。
// BatchDeleteInspectionReportsData is the batch delete response payload.
type BatchDeleteInspectionReportsData struct {
	Results []*InspectionDeleteResult `json:"results"`
	Deleted int                       `json:"deleted"`
	Skipped int                       `json:"skipped"`
}
