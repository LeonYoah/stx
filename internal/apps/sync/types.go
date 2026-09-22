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

import "time"

// CreateTaskRequest represents the payload for creating a sync workspace node.
type CreateTaskRequest struct {
	ParentID      *uint   `json:"parent_id"`
	NodeType      string  `json:"node_type"`
	Name          string  `json:"name" binding:"required"`
	Description   string  `json:"description"`
	ClusterID     uint    `json:"cluster_id"`
	EngineVersion string  `json:"engine_version"`
	Mode          string  `json:"mode"`
	ContentFormat string  `json:"content_format"`
	Content       string  `json:"content"`
	JobName       string  `json:"job_name"`
	SortOrder     int     `json:"sort_order"`
	Definition    JSONMap `json:"definition"`
}

// UpdateTaskRequest represents the payload for updating a sync workspace node.
type UpdateTaskRequest struct {
	ParentID      *uint   `json:"parent_id"`
	NodeType      string  `json:"node_type"`
	Name          string  `json:"name" binding:"required"`
	Description   string  `json:"description"`
	ClusterID     uint    `json:"cluster_id"`
	EngineVersion string  `json:"engine_version"`
	Mode          string  `json:"mode"`
	ContentFormat string  `json:"content_format"`
	Content       string  `json:"content"`
	JobName       string  `json:"job_name"`
	SortOrder     int     `json:"sort_order"`
	Definition    JSONMap `json:"definition"`
}

// UpdateTaskPermissionsRequest 表示一次只修改共享权限字段的请求。
// UpdateTaskPermissionsRequest represents a partial sharing-permission update.
type UpdateTaskPermissionsRequest struct {
	IsPublic        *bool  `json:"is_public"`
	CollaboratorIDs []uint `json:"collaborator_ids"`
}

// PublishTaskRequest represents the payload for publishing a sync task.
type PublishTaskRequest struct {
	Comment string `json:"comment"`
}

// CreateGlobalVariableRequest represents one global variable payload.
// CreateGlobalVariableRequest 表示创建全局变量请求体。
type CreateGlobalVariableRequest struct {
	Key         string `json:"key" binding:"required"`
	Value       string `json:"value"`
	ValueType   string `json:"value_type"`
	Description string `json:"description"`
}

// UpdateGlobalVariableRequest represents one global variable update payload.
// UpdateGlobalVariableRequest 表示更新全局变量请求体。
type UpdateGlobalVariableRequest struct {
	Key         string `json:"key" binding:"required"`
	Value       string `json:"value"`
	ValueType   string `json:"value_type"`
	Description string `json:"description"`
}

// RecoverTaskRequest represents the payload for recovering from one previous job.
type RecoverTaskRequest struct {
	Comment string `json:"comment,omitempty"`
}

// CancelJobRequest represents stop-job behavior.
type CancelJobRequest struct {
	StopWithSavepoint bool `json:"stop_with_savepoint"`
}

// TaskDraftPayload represents one unsaved in-memory task draft used by actions.
type TaskDraftPayload struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	ClusterID     uint    `json:"cluster_id"`
	EngineVersion string  `json:"engine_version"`
	Mode          string  `json:"mode"`
	ContentFormat string  `json:"content_format"`
	Content       string  `json:"content"`
	JobName       string  `json:"job_name"`
	Definition    JSONMap `json:"definition"`
}

// TaskActionRequest represents an action request with an optional unsaved draft.
type TaskActionRequest struct {
	Draft *TaskDraftPayload `json:"draft,omitempty"`
}

// PreviewTaskRequest represents optional preview execution parameters.
type PreviewTaskRequest struct {
	RowLimit        int               `json:"row_limit"`
	TimeoutMinutes  int               `json:"timeout_minutes"`
	SourceNodeID    string            `json:"source_node_id,omitempty"`
	SourceIndex     *int              `json:"source_index,omitempty"`
	TransformNodeID string            `json:"transform_node_id,omitempty"`
	TransformIndex  *int              `json:"transform_index,omitempty"`
	Mode            string            `json:"mode,omitempty"`
	Draft           *TaskDraftPayload `json:"draft,omitempty"`
}

// RecoverJobRequest represents a recover request with an optional unsaved task draft.
type RecoverJobRequest struct {
	Draft *TaskDraftPayload `json:"draft,omitempty"`
}

// TaskFilter represents task list query filters.
type TaskFilter struct {
	Name   string
	Status TaskStatus
	Page   int
	Size   int
}

// JobFilter represents job instance list query filters.
type JobFilter struct {
	TaskID        uint
	RunType       RunType
	PlatformJobID string
	EngineJobID   string
	OwnerUserID   uint
	IncludeAll    bool
	Page          int
	Size          int
}

// ValidateResult represents validation result payload.
type ValidateResult struct {
	Valid         bool              `json:"valid"`
	Errors        []string          `json:"errors"`
	Warnings      []string          `json:"warnings"`
	Summary       string            `json:"summary"`
	Resolved      map[string]string `json:"resolved,omitempty"`
	DetectedVars  []string          `json:"detected_vars,omitempty"`
	DetectedFiles []string          `json:"detected_files,omitempty"`
	Checks        []ValidateCheck   `json:"checks,omitempty"`
}

// ValidateCheck represents one connector config/connection validation entry.
type ValidateCheck struct {
	NodeID        string `json:"node_id"`
	Kind          string `json:"kind"`
	ConnectorType string `json:"connector_type"`
	Target        string `json:"target,omitempty"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

// DAGResult represents DAG response payload.
type DAGResult struct {
	Nodes       []JSONMap `json:"nodes"`
	Edges       []JSONMap `json:"edges"`
	WebUIJob    JSONMap   `json:"webui_job,omitempty"`
	SimpleGraph bool      `json:"simple_graph,omitempty"`
	Warnings    []string  `json:"warnings,omitempty"`
}

// PreviewSnapshot represents one incremental preview snapshot payload.
// PreviewSnapshot 表示一次增量预览快照返回。
type PreviewSnapshot struct {
	SessionID      uint                `json:"session_id"`
	JobInstanceID  uint                `json:"job_instance_id"`
	PlatformJobID  string              `json:"platform_job_id"`
	EngineJobID    string              `json:"engine_job_id"`
	Status         string              `json:"status"`
	EmptyReason    string              `json:"empty_reason,omitempty"`
	RowLimit       int                 `json:"row_limit"`
	TimeoutMinutes int                 `json:"timeout_minutes"`
	TotalRows      int                 `json:"total_rows"`
	TableCount     int                 `json:"table_count"`
	Truncated      bool                `json:"truncated"`
	InjectedScript string              `json:"injected_script,omitempty"`
	ContentFormat  string              `json:"content_format,omitempty"`
	Tables         []*PreviewTableData `json:"tables"`
	SelectedTable  *PreviewTableData   `json:"selected_table,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

// PreviewTableData represents one table group inside a preview snapshot.
// PreviewTableData 表示预览快照中的单张表数据。
type PreviewTableData struct {
	ID        uint                     `json:"id"`
	TablePath string                   `json:"table_path"`
	Columns   []string                 `json:"columns"`
	RowCount  int                      `json:"row_count"`
	Rows      []map[string]interface{} `json:"rows,omitempty"`
}

// CheckpointSnapshot represents checkpoint overview and history for one job.
// CheckpointSnapshot 表示单个作业的 checkpoint 概览与历史。
type CheckpointSnapshot struct {
	JobInstanceID uint                      `json:"job_instance_id"`
	PlatformJobID string                    `json:"platform_job_id"`
	EngineJobID   string                    `json:"engine_job_id"`
	Status        string                    `json:"status"`
	EmptyReason   string                    `json:"empty_reason,omitempty"`
	Message       string                    `json:"message,omitempty"`
	Overview      *EngineCheckpointOverview `json:"overview,omitempty"`
	History       []*EngineCheckpointRecord `json:"history,omitempty"`
}

// TaskListData represents task list response data.
type TaskListData struct {
	Total int64   `json:"total"`
	Items []*Task `json:"items"`
}

// TaskVersionListData represents paginated task versions.
type TaskVersionListData struct {
	Total int64          `json:"total"`
	Items []*TaskVersion `json:"items"`
}

// JobListData represents job list response data.
type JobListData struct {
	Total int64          `json:"total"`
	Items []*JobInstance `json:"items"`
}

// GlobalVariableListData represents paginated global variables.
type GlobalVariableListData struct {
	Total int64             `json:"total"`
	Items []*GlobalVariable `json:"items"`
}

// TaskTreeNode represents one node in the sync workspace tree.
type TaskTreeNode struct {
	ID                      uint            `json:"id"`
	ParentID                *uint           `json:"parent_id,omitempty"`
	NodeType                TaskNodeType    `json:"node_type"`
	Name                    string          `json:"name"`
	Description             string          `json:"description"`
	ClusterID               uint            `json:"cluster_id"`
	EngineVersion           string          `json:"engine_version"`
	Mode                    TaskMode        `json:"mode"`
	Status                  TaskStatus      `json:"status"`
	ContentFormat           ContentFormat   `json:"content_format"`
	Content                 string          `json:"content"`
	JobName                 string          `json:"job_name"`
	Definition              JSONMap         `json:"definition"`
	SortOrder               int             `json:"sort_order"`
	CurrentVersion          int             `json:"current_version"`
	ScheduleEnabled         bool            `json:"schedule_enabled,omitempty"`
	ScheduleCronExpr        string          `json:"schedule_cron_expr,omitempty"`
	ScheduleTimezone        string          `json:"schedule_timezone,omitempty"`
	ScheduleLastTriggeredAt *time.Time      `json:"schedule_last_triggered_at,omitempty"`
	ScheduleNextTriggeredAt *time.Time      `json:"schedule_next_triggered_at,omitempty"`
	CreatedBy               uint            `json:"created_by"`
	CanEdit                 bool            `json:"can_edit"`
	CanRun                  bool            `json:"can_run"`
	IsOwner                 bool            `json:"is_owner"`
	IsCollaborator          bool            `json:"is_collaborator"`
	IsPublicTask            bool            `json:"is_public"`
	Children                []*TaskTreeNode `json:"children,omitempty"`
}

// TaskTreeData represents tree response payload.
type TaskTreeData struct {
	Items []*TaskTreeNode `json:"items"`
}
