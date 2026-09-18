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
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// JSONMap represents JSON object payload stored in database.
// JSONMap 表示存储在数据库中的 JSON 对象载荷。
type JSONMap map[string]interface{}

// Value implements the driver.Valuer interface.
// Value 实现 driver.Valuer 接口。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements the sql.Scanner interface.
// Scan 实现 sql.Scanner 接口。
func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, m)
	case string:
		return json.Unmarshal([]byte(v), m)
	default:
		return errors.New("sync: failed to scan JSONMap - expected []byte or string")
	}
}

// JSONStringSlice represents one JSON-encoded string array.
// JSONStringSlice 表示一个 JSON 编码的字符串数组。
type JSONStringSlice []string

// Value implements the driver.Valuer interface.
// Value 实现 driver.Valuer 接口。
func (s JSONStringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface.
// Scan 实现 sql.Scanner 接口。
func (s *JSONStringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return errors.New("sync: failed to scan JSONStringSlice - expected []byte or string")
	}
}

// TaskStatus represents sync task lifecycle status.
// TaskStatus 表示同步任务生命周期状态。
type TaskStatus string

const (
	TaskStatusDraft     TaskStatus = "draft"
	TaskStatusPublished TaskStatus = "published"
	TaskStatusArchived  TaskStatus = "archived"
)

// TaskMode represents sync task runtime mode.
// TaskMode 表示同步任务运行模式。
type TaskMode string

const (
	TaskModeStreaming TaskMode = "streaming"
	TaskModeBatch     TaskMode = "batch"
)

// TaskNodeType represents sync workspace node type.
// TaskNodeType 表示同步工作台节点类型。
type TaskNodeType string

const (
	TaskNodeTypeFolder TaskNodeType = "folder"
	TaskNodeTypeFile   TaskNodeType = "file"
)

// ContentFormat represents the editor content format.
// ContentFormat 表示编辑器内容格式。
type ContentFormat string

const (
	ContentFormatHOCON ContentFormat = "hocon"
	ContentFormatJSON  ContentFormat = "json"
)

// RunType represents sync job instance run type.
// RunType 表示同步作业实例运行类型。
type RunType string

const (
	RunTypePreview  RunType = "preview"
	RunTypeRun      RunType = "run"
	RunTypeRecover  RunType = "recover"
	RunTypeSchedule RunType = "schedule"
)

// JobStatus represents sync job instance status.
// JobStatus 表示同步作业实例状态。
type JobStatus string

const (
	JobStatusPending         JobStatus = "pending"
	JobStatusRunning         JobStatus = "running"
	JobStatusCancelRequested JobStatus = "cancel_requested"
	JobStatusCancelling      JobStatus = "cancelling"
	JobStatusSuccess         JobStatus = "success"
	JobStatusFailed          JobStatus = "failed"
	JobStatusCanceled        JobStatus = "canceled"
)

// Task represents one sync studio workspace node.
// Task 表示一个数据同步工作台节点。
type Task struct {
	ID            uint          `json:"id" gorm:"primaryKey;autoIncrement"`
	ParentID      *uint         `json:"parent_id,omitempty" gorm:"index"`
	NodeType      TaskNodeType  `json:"node_type" gorm:"size:20;not null;default:file;index"`
	Name          string        `json:"name" gorm:"size:120;not null;index"`
	Description   string        `json:"description" gorm:"type:text"`
	ClusterID     uint          `json:"cluster_id" gorm:"index"`
	EngineVersion string        `json:"engine_version" gorm:"size:50"`
	Mode          TaskMode      `json:"mode" gorm:"size:20;default:streaming"`
	Status        TaskStatus    `json:"status" gorm:"size:20;default:draft;index"`
	ContentFormat ContentFormat `json:"content_format" gorm:"size:20;not null;default:hocon"`
	// Content 存储任务定义内容（HOCON 或 JSON），兼容多数据库。
	// Content stores task definition content (HOCON or JSON), compatible with multiple databases.
	Content                 string     `json:"content" gorm:"type:text"`
	JobName                 string     `json:"job_name" gorm:"size:255"`
	Definition              JSONMap    `json:"definition" gorm:"type:json"`
	SortOrder               int        `json:"sort_order" gorm:"default:0;index"`
	CurrentVersion          int        `json:"current_version" gorm:"default:0"`
	ScheduleEnabled         bool       `json:"schedule_enabled" gorm:"-"`
	ScheduleCronExpr        string     `json:"schedule_cron_expr,omitempty" gorm:"-"`
	ScheduleTimezone        string     `json:"schedule_timezone,omitempty" gorm:"-"`
	ScheduleLastTriggeredAt *time.Time `json:"schedule_last_triggered_at,omitempty" gorm:"-"`
	ScheduleNextTriggeredAt *time.Time `json:"schedule_next_triggered_at,omitempty" gorm:"-"`
	CreatedBy               uint       `json:"created_by"`
	CreatedAt               time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt               time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName returns the sync task table name.
// TableName 返回同步任务表名。
func (Task) TableName() string {
	return "sync_tasks"
}

// TaskVersion stores one immutable snapshot of a sync file task.
// TaskVersion 存储同步文件任务的一次不可变快照。
type TaskVersion struct {
	ID                    uint          `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID                uint          `json:"task_id" gorm:"index;not null"`
	Version               int           `json:"version" gorm:"not null"`
	NameSnapshot          string        `json:"name_snapshot" gorm:"size:120"`
	DescriptionSnapshot   string        `json:"description_snapshot" gorm:"type:text"`
	ClusterIDSnapshot     uint          `json:"cluster_id_snapshot"`
	EngineVersionSnapshot string        `json:"engine_version_snapshot" gorm:"size:50"`
	ModeSnapshot          TaskMode      `json:"mode_snapshot" gorm:"size:20"`
	ContentFormatSnapshot ContentFormat `json:"content_format_snapshot" gorm:"size:20"`
	// ContentSnapshot 存储版本快照内容（HOCON 或 JSON），兼容多数据库。
	// ContentSnapshot stores version snapshot content (HOCON or JSON), compatible with multiple databases.
	ContentSnapshot    string    `json:"content_snapshot" gorm:"type:text"`
	JobNameSnapshot    string    `json:"job_name_snapshot" gorm:"size:255"`
	DefinitionSnapshot JSONMap   `json:"definition_snapshot" gorm:"type:json"`
	Comment            string    `json:"comment" gorm:"size:255"`
	CreatedBy          uint      `json:"created_by"`
	CreatedAt          time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName returns the sync task version table name.
// TableName 返回同步任务版本表名。
func (TaskVersion) TableName() string {
	return "sync_task_versions"
}

// JobInstance stores one preview/run/recover execution instance.
// JobInstance 存储一次预览/运行/恢复执行实例。
type JobInstance struct {
	ID                      uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	ExecutionID             string     `json:"execution_id" gorm:"size:36;index"`
	TaskID                  uint       `json:"task_id" gorm:"index;not null"`
	TaskVersion             int        `json:"task_version" gorm:"not null"`
	RunType                 RunType    `json:"run_type" gorm:"size:20;not null;index"`
	PlatformJobID           string     `json:"platform_job_id" gorm:"size:32;index"`
	EngineJobID             string     `json:"engine_job_id" gorm:"size:255;index"`
	RecoveredFromInstanceID *uint      `json:"recovered_from_instance_id,omitempty" gorm:"index"`
	Status                  JobStatus  `json:"status" gorm:"size:20;default:pending;index"`
	SubmitSpec              JSONMap    `json:"submit_spec" gorm:"type:json"`
	ResultPreview           JSONMap    `json:"result_preview" gorm:"type:json"`
	ErrorMessage            string     `json:"error_message" gorm:"type:text"`
	StartedAt               *time.Time `json:"started_at"`
	FinishedAt              *time.Time `json:"finished_at"`
	CreatedBy               uint       `json:"created_by"`
	CreatedAt               time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt               time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName returns the sync job instance table name.
// TableName 返回同步作业实例表名。
func (JobInstance) TableName() string {
	return "sync_job_instances"
}

const (
	// GlobalVariableTypeString represents a plain text variable.
	// GlobalVariableTypeString 表示普通文本变量。
	GlobalVariableTypeString = "string"

	// GlobalVariableTypeSecret represents a password/secret variable that is never displayed after creation.
	// GlobalVariableTypeSecret 表示密码/机密变量，创建或更新后不回显明文。
	GlobalVariableTypeSecret = "secret"
)

// GlobalVariable represents one workspace-wide runtime variable.
// GlobalVariable 表示一个工作台级别的全局运行时变量。
type GlobalVariable struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Key         string    `json:"key" gorm:"size:120;not null;uniqueIndex"`
	Value       string    `json:"value" gorm:"type:text"`
	ValueType   string    `json:"value_type" gorm:"size:32;not null;default:'string'"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName returns the sync global variable table name.
// TableName 返回同步全局变量表名。
func (GlobalVariable) TableName() string {
	return "sync_global_variables"
}

// MaskSecret returns a safe copy with value masked to "******" if it is a secret variable.
// MaskSecret 若为敏感机密变量，返回变量值脱敏为 "******" 的安全副本。
func (v *GlobalVariable) MaskSecret() *GlobalVariable {
	if v == nil {
		return nil
	}
	clone := *v
	if clone.ValueType == GlobalVariableTypeSecret {
		clone.Value = "******"
	}
	return &clone
}

// PreviewSession stores one incremental preview session snapshot.
// PreviewSession 存储一次增量预览会话快照。
type PreviewSession struct {
	ID             uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	JobInstanceID  uint       `json:"job_instance_id" gorm:"uniqueIndex;not null"`
	TaskID         uint       `json:"task_id" gorm:"index;not null"`
	PlatformJobID  string     `json:"platform_job_id" gorm:"size:32;index"`
	EngineJobID    string     `json:"engine_job_id" gorm:"size:255;index"`
	RowLimit       int        `json:"row_limit" gorm:"not null;default:100"`
	TimeoutMinutes int        `json:"timeout_minutes" gorm:"not null;default:10"`
	Status         string     `json:"status" gorm:"size:32;not null;default:collecting;index"`
	TotalRows      int        `json:"total_rows" gorm:"not null;default:0"`
	TableCount     int        `json:"table_count" gorm:"not null;default:0"`
	Truncated      bool       `json:"truncated" gorm:"not null;default:false"`
	LastError      string     `json:"last_error" gorm:"type:text"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	CreatedAt      time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName returns the preview session table name.
// TableName 返回预览会话表名。
func (PreviewSession) TableName() string {
	return "sync_preview_sessions"
}

// PreviewTable stores one preview table group under a session.
// PreviewTable 存储预览会话中的一张表分组。
type PreviewTable struct {
	ID          uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	SessionID   uint            `json:"session_id" gorm:"index;not null"`
	TablePath   string          `json:"table_path" gorm:"size:512;index;not null"`
	DisplayName string          `json:"display_name" gorm:"size:512"`
	Columns     JSONStringSlice `json:"columns" gorm:"type:json"`
	RowCount    int             `json:"row_count" gorm:"not null;default:0"`
	CreatedAt   time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName returns the preview table table name.
// TableName 返回预览表分组表名。
func (PreviewTable) TableName() string {
	return "sync_preview_tables"
}

// PreviewRow stores one preview data row.
// PreviewRow 存储一条预览数据行。
type PreviewRow struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	SessionID uint      `json:"session_id" gorm:"index;not null"`
	TableID   uint      `json:"table_id" gorm:"index;not null"`
	RowIndex  int       `json:"row_index" gorm:"index;not null"`
	RowData   JSONMap   `json:"row_data" gorm:"type:json"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName returns the preview row table name.
// TableName 返回预览数据行表名。
func (PreviewRow) TableName() string {
	return "sync_preview_rows"
}
