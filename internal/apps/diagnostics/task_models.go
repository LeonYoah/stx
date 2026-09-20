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
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// DiagnosticTaskStatus represents the common status for diagnostics tasks,
// steps, and node executions.
// DiagnosticTaskStatus 表示诊断任务、步骤和节点执行共用状态。
type DiagnosticTaskStatus string

const (
	DiagnosticTaskStatusPending         DiagnosticTaskStatus = "pending"
	DiagnosticTaskStatusReady           DiagnosticTaskStatus = "ready"
	DiagnosticTaskStatusRunning         DiagnosticTaskStatus = "running"
	DiagnosticTaskStatusCancelRequested DiagnosticTaskStatus = "cancel_requested"
	DiagnosticTaskStatusCancelling      DiagnosticTaskStatus = "cancelling"
	DiagnosticTaskStatusSucceeded       DiagnosticTaskStatus = "succeeded"
	DiagnosticTaskStatusFailed          DiagnosticTaskStatus = "failed"
	DiagnosticTaskStatusSkipped         DiagnosticTaskStatus = "skipped"
	DiagnosticTaskStatusCancelled       DiagnosticTaskStatus = "cancelled"
)

// DiagnosticTaskSourceType represents where a diagnostic bundle task was created from.
// DiagnosticTaskSourceType 表示诊断任务的触发来源。
type DiagnosticTaskSourceType string

const (
	DiagnosticTaskSourceManual            DiagnosticTaskSourceType = "manual"
	DiagnosticTaskSourceErrorGroup        DiagnosticTaskSourceType = "error_group"
	DiagnosticTaskSourceInspectionFinding DiagnosticTaskSourceType = "inspection_finding"
	DiagnosticTaskSourceAlert             DiagnosticTaskSourceType = "alert"
)

// DiagnosticTaskNodeScope represents how task node targets are selected.
// DiagnosticTaskNodeScope 表示诊断任务节点选择范围。
type DiagnosticTaskNodeScope string

const (
	// DiagnosticTaskNodeScopeAll selects all managed nodes in the cluster.
	// DiagnosticTaskNodeScopeAll 表示选中集群内所有受管节点。
	DiagnosticTaskNodeScopeAll DiagnosticTaskNodeScope = "all"
	// DiagnosticTaskNodeScopeRelated selects only nodes related to the source context (e.g. error group / finding).
	// DiagnosticTaskNodeScopeRelated 表示仅选中与来源上下文相关的节点（如错误组 / 巡检发现）。
	DiagnosticTaskNodeScopeRelated DiagnosticTaskNodeScope = "related"
	// DiagnosticTaskNodeScopeCustom selects an explicit custom node list.
	// DiagnosticTaskNodeScopeCustom 表示使用显式自定义节点列表。
	DiagnosticTaskNodeScopeCustom DiagnosticTaskNodeScope = "custom"
)

// DiagnosticStepCode identifies one fixed step in the diagnostic bundle workflow.
// DiagnosticStepCode 表示诊断包编排中的固定步骤编码。
type DiagnosticStepCode string

const (
	DiagnosticStepCodeCollectErrorContext   DiagnosticStepCode = "COLLECT_ERROR_CONTEXT"
	DiagnosticStepCodeCollectProcessEvents  DiagnosticStepCode = "COLLECT_PROCESS_EVENTS"
	DiagnosticStepCodeCollectAlertSnapshot  DiagnosticStepCode = "COLLECT_ALERT_SNAPSHOT"
	DiagnosticStepCodeCollectConfigSnapshot DiagnosticStepCode = "COLLECT_CONFIG_SNAPSHOT"
	DiagnosticStepCodeCollectLogSample      DiagnosticStepCode = "COLLECT_LOG_SAMPLE"
	DiagnosticStepCodeCollectThreadDump     DiagnosticStepCode = "COLLECT_THREAD_DUMP"
	DiagnosticStepCodeCollectJVMDump        DiagnosticStepCode = "COLLECT_JVM_DUMP"
	DiagnosticStepCodeAssembleManifest      DiagnosticStepCode = "ASSEMBLE_MANIFEST"
	DiagnosticStepCodeRenderHTMLSummary     DiagnosticStepCode = "RENDER_HTML_SUMMARY"
	DiagnosticStepCodeComplete              DiagnosticStepCode = "COMPLETE"
)

// DiagnosticLogLevel represents the log level of task execution logs.
// DiagnosticLogLevel 表示诊断任务执行日志级别。
type DiagnosticLogLevel string

const (
	DiagnosticLogLevelInfo  DiagnosticLogLevel = "INFO"
	DiagnosticLogLevelWarn  DiagnosticLogLevel = "WARN"
	DiagnosticLogLevelError DiagnosticLogLevel = "ERROR"
)

// DiagnosticLogEventType represents the event type of task execution logs.
// DiagnosticLogEventType 表示诊断任务执行日志事件类型。
type DiagnosticLogEventType string

const (
	DiagnosticLogEventTypeStarted  DiagnosticLogEventType = "started"
	DiagnosticLogEventTypeProgress DiagnosticLogEventType = "progress"
	DiagnosticLogEventTypeSuccess  DiagnosticLogEventType = "success"
	DiagnosticLogEventTypeFailed   DiagnosticLogEventType = "failed"
	DiagnosticLogEventTypeNote     DiagnosticLogEventType = "note"
)

// DiagnosticTaskSourceRef stores typed references back to the originating context.
// DiagnosticTaskSourceRef 存储回溯到来源上下文的结构化引用。
type DiagnosticTaskSourceRef struct {
	ErrorGroupID        uint   `json:"error_group_id,omitempty"`
	InspectionReportID  uint   `json:"inspection_report_id,omitempty"`
	InspectionFindingID uint   `json:"inspection_finding_id,omitempty"`
	AlertID             string `json:"alert_id,omitempty"`
}

// Value implements driver.Valuer for JSON storage.
// Value 实现 driver.Valuer，用于 JSON 存储。
func (r DiagnosticTaskSourceRef) Value() (driver.Value, error) {
	return json.Marshal(r)
}

// Scan implements sql.Scanner for JSON retrieval.
// Scan 实现 sql.Scanner，用于 JSON 读取。
func (r *DiagnosticTaskSourceRef) Scan(value interface{}) error {
	bytes, err := normalizeJSONScanBytes(value)
	if err != nil {
		return err
	}
	if bytes == nil {
		*r = DiagnosticTaskSourceRef{}
		return nil
	}
	return json.Unmarshal(bytes, r)
}

// DiagnosticTaskNodeTarget stores one selected node snapshot inside a task.
// DiagnosticTaskNodeTarget 存储任务中选择的单个节点快照。
type DiagnosticTaskNodeTarget struct {
	ClusterNodeID uint   `json:"cluster_node_id"`
	NodeID        uint   `json:"node_id"`
	HostID        uint   `json:"host_id"`
	HostName      string `json:"host_name"`
	HostIP        string `json:"host_ip"`
	Role          string `json:"role"`
	AgentID       string `json:"agent_id"`
	InstallDir    string `json:"install_dir"`
}

// DiagnosticTaskNodeTargets stores selected node snapshots for one task.
// DiagnosticTaskNodeTargets 存储任务中选中的节点快照列表。
type DiagnosticTaskNodeTargets []DiagnosticTaskNodeTarget

// Value implements driver.Valuer for JSON storage.
// Value 实现 driver.Valuer，用于 JSON 存储。
func (n DiagnosticTaskNodeTargets) Value() (driver.Value, error) {
	if n == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(n)
}

// Scan implements sql.Scanner for JSON retrieval.
// Scan 实现 sql.Scanner，用于 JSON 读取。
func (n *DiagnosticTaskNodeTargets) Scan(value interface{}) error {
	bytes, err := normalizeJSONScanBytes(value)
	if err != nil {
		return err
	}
	if bytes == nil {
		*n = DiagnosticTaskNodeTargets{}
		return nil
	}
	return json.Unmarshal(bytes, n)
}

// DiagnosticLogMetadata stores structured metadata for one task log.
// DiagnosticLogMetadata 存储单条任务日志的结构化元数据。
type DiagnosticLogMetadata map[string]interface{}

// Value implements driver.Valuer for JSON storage.
// Value 实现 driver.Valuer，用于 JSON 存储。
func (m DiagnosticLogMetadata) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan implements sql.Scanner for JSON retrieval.
// Scan 实现 sql.Scanner，用于 JSON 读取。
func (m *DiagnosticLogMetadata) Scan(value interface{}) error {
	bytes, err := normalizeJSONScanBytes(value)
	if err != nil {
		return err
	}
	if bytes == nil {
		*m = nil
		return nil
	}
	return json.Unmarshal(bytes, m)
}

// DiagnosticTaskOptions stores bundle collection options.
// DiagnosticTaskOptions 存储诊断包采集选项。
type DiagnosticTaskOptions struct {
	IncludeThreadDump bool `json:"include_thread_dump"`
	IncludeJVMDump    bool `json:"include_jvm_dump"`
	JVMDumpMinFreeMB  int  `json:"jvm_dump_min_free_mb,omitempty"`
	// SelectedResources 显式限制组合任务需要采集的公开资源；空值保持原有默认行为。
	// SelectedResources explicitly limits public resources collected by a bundle task; empty keeps the legacy defaults.
	SelectedResources []DiagnosticResourceCode `json:"selected_resources,omitempty"`
	// ResourceOnly 表示任务只运行一个资源，不生成 Manifest 和 HTML 报告。
	// ResourceOnly marks a single-resource task that skips Manifest and HTML report generation.
	ResourceOnly bool `json:"resource_only,omitempty"`
}

// Normalize fills default option values.
// Normalize 填充采集选项默认值。
func (o DiagnosticTaskOptions) Normalize() DiagnosticTaskOptions {
	if o.JVMDumpMinFreeMB <= 0 {
		o.JVMDumpMinFreeMB = 2048
	}
	o.SelectedResources = normalizeDiagnosticResourceCodes(o.SelectedResources)
	if containsDiagnosticResource(o.SelectedResources, DiagnosticResourceThreadDump) {
		o.IncludeThreadDump = true
	}
	if containsDiagnosticResource(o.SelectedResources, DiagnosticResourceJVMDump) {
		o.IncludeJVMDump = true
	}
	return o
}

// Value implements driver.Valuer for JSON storage.
// Value 实现 driver.Valuer，用于 JSON 存储。
func (o DiagnosticTaskOptions) Value() (driver.Value, error) {
	normalized := o.Normalize()
	return json.Marshal(normalized)
}

// Scan implements sql.Scanner for JSON retrieval.
// Scan 实现 sql.Scanner，用于 JSON 读取。
func (o *DiagnosticTaskOptions) Scan(value interface{}) error {
	bytes, err := normalizeJSONScanBytes(value)
	if err != nil {
		return err
	}
	if bytes == nil {
		*o = DiagnosticTaskOptions{}.Normalize()
		return nil
	}
	if err := json.Unmarshal(bytes, o); err != nil {
		return err
	}
	*o = o.Normalize()
	return nil
}

// DiagnosticPlanStep describes one ordered step in the diagnostic bundle task.
// DiagnosticPlanStep 描述诊断包任务中的有序步骤。
type DiagnosticPlanStep struct {
	Sequence    int                `json:"sequence"`
	Code        DiagnosticStepCode `json:"code"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	NodeScoped  bool               `json:"node_scoped"`
	Required    bool               `json:"required"`
}

// DefaultDiagnosticTaskSteps returns the MVP step order for bundle collection.
// DefaultDiagnosticTaskSteps 返回诊断包采集 MVP 的固定步骤顺序。
func DefaultDiagnosticTaskSteps() []DiagnosticPlanStep {
	return []DiagnosticPlanStep{
		{Sequence: 1, Code: DiagnosticStepCodeCollectErrorContext, Title: bilingualText("汇总错误上下文", "Collect Error Context"), Description: bilingualText("加载错误组、巡检结果和来源上下文。", "Load error groups, inspection results, and source context."), NodeScoped: false, Required: true},
		{Sequence: 2, Code: DiagnosticStepCodeCollectProcessEvents, Title: bilingualText("收集进程事件", "Collect Process Events"), Description: bilingualText("采集近期进程事件和自动拉起记录。", "Collect recent process events and auto-restart history."), NodeScoped: false, Required: true},
		{Sequence: 3, Code: DiagnosticStepCodeCollectAlertSnapshot, Title: bilingualText("收集告警快照", "Collect Alert Snapshot"), Description: bilingualText("采集相关告警状态与通知上下文。", "Collect related alert states and notification context."), NodeScoped: false, Required: true},
		{Sequence: 4, Code: DiagnosticStepCodeCollectConfigSnapshot, Title: bilingualText("收集配置快照", "Collect Config Snapshot"), Description: bilingualText("导出 Seatunnel 与相关运行配置快照。", "Export Seatunnel runtime config snapshots and related settings."), NodeScoped: true, Required: true},
		{Sequence: 5, Code: DiagnosticStepCodeCollectLogSample, Title: bilingualText("收集日志样本", "Collect Log Sample"), Description: bilingualText("采集错误附近日志样本和近期运行日志片段。", "Collect nearby log samples and recent runtime log fragments."), NodeScoped: true, Required: false},
		{Sequence: 6, Code: DiagnosticStepCodeCollectThreadDump, Title: bilingualText("收集线程栈", "Collect Thread Dump"), Description: bilingualText("对选中节点执行线程栈采集。", "Collect thread dumps on selected nodes."), NodeScoped: true, Required: false},
		{Sequence: 7, Code: DiagnosticStepCodeCollectJVMDump, Title: bilingualText("收集 JVM Dump", "Collect JVM Dump"), Description: bilingualText("对选中节点执行 JVM Dump 采集。", "Collect JVM dumps on selected nodes."), NodeScoped: true, Required: false},
		{Sequence: 8, Code: DiagnosticStepCodeAssembleManifest, Title: bilingualText("生成 Manifest", "Assemble Manifest"), Description: bilingualText("生成机器可读的诊断证据清单。", "Generate a machine-readable diagnostic evidence manifest."), NodeScoped: false, Required: true},
		{Sequence: 9, Code: DiagnosticStepCodeRenderHTMLSummary, Title: bilingualText("生成诊断报告", "Render Diagnostic Report"), Description: bilingualText("渲染 index.html 诊断报告，便于离线查看与分享。", "Render the offline diagnostic report for viewing and sharing."), NodeScoped: false, Required: true},
		{Sequence: 10, Code: DiagnosticStepCodeComplete, Title: bilingualText("完成", "Complete"), Description: bilingualText("标记诊断任务完成并输出入口索引。", "Mark the diagnostic task as completed and publish the entry index."), NodeScoped: false, Required: true},
	}
}

// DiagnosticTask persists one diagnostic bundle task.
// DiagnosticTask 存储一条诊断包任务。
type DiagnosticTask struct {
	ID            uint                     `json:"id" gorm:"primaryKey;autoIncrement"`
	ExecutionID   string                   `json:"execution_id,omitempty" gorm:"size:36;index"`
	ClusterID     uint                     `json:"cluster_id" gorm:"index;not null"`
	TriggerSource DiagnosticTaskSourceType `json:"trigger_source" gorm:"size:40;index;not null"`
	SourceRef     DiagnosticTaskSourceRef  `json:"source_ref" gorm:"type:json;not null"`
	Options       DiagnosticTaskOptions    `json:"options" gorm:"type:json;not null"`
	// LookbackMinutes controls the diagnostics collection window in minutes.
	// LookbackMinutes 控制诊断采集时间窗口（分钟），0 表示使用默认或来源巡检窗口。
	LookbackMinutes int                       `json:"lookback_minutes" gorm:"default:0"`
	Status          DiagnosticTaskStatus      `json:"status" gorm:"size:32;index;not null"`
	CurrentStep     DiagnosticStepCode        `json:"current_step" gorm:"size:64;index"`
	FailureStep     DiagnosticStepCode        `json:"failure_step" gorm:"size:64"`
	FailureReason   string                    `json:"failure_reason" gorm:"type:text"`
	SelectedNodes   DiagnosticTaskNodeTargets `json:"selected_nodes" gorm:"type:json;not null"`
	Summary         string                    `json:"summary" gorm:"type:text"`
	BundleDir       string                    `json:"bundle_dir" gorm:"size:500"`
	ManifestPath    string                    `json:"manifest_path" gorm:"size:500"`
	IndexPath       string                    `json:"index_path" gorm:"size:500"`
	StartedAt       *time.Time                `json:"started_at"`
	CompletedAt     *time.Time                `json:"completed_at"`
	CreatedBy       uint                      `json:"created_by"`
	CreatedByName   string                    `json:"created_by_name" gorm:"size:120;index"`
	CreatedAt       time.Time                 `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt       time.Time                 `json:"updated_at" gorm:"autoUpdateTime"`
	Steps           []DiagnosticTaskStep      `json:"steps" gorm:"foreignKey:TaskID"`
	NodeExecutions  []DiagnosticNodeExecution `json:"node_executions" gorm:"foreignKey:TaskID"`
}

// TableName specifies the diagnostic task table name.
// TableName 指定诊断任务表名。
func (DiagnosticTask) TableName() string {
	return "diagnostics_tasks"
}

// DiagnosticTaskStep persists one task step.
// DiagnosticTaskStep 存储一条任务步骤。
type DiagnosticTaskStep struct {
	ID          uint                 `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID      uint                 `json:"task_id" gorm:"index;not null;uniqueIndex:idx_diagnostics_task_step_code"`
	Code        DiagnosticStepCode   `json:"code" gorm:"size:64;not null;uniqueIndex:idx_diagnostics_task_step_code"`
	Sequence    int                  `json:"sequence" gorm:"not null;index"`
	Title       string               `json:"title" gorm:"size:120"`
	Description string               `json:"description" gorm:"type:text"`
	Status      DiagnosticTaskStatus `json:"status" gorm:"size:32;index;not null"`
	Message     string               `json:"message" gorm:"type:text"`
	Error       string               `json:"error" gorm:"type:text"`
	StartedAt   *time.Time           `json:"started_at"`
	CompletedAt *time.Time           `json:"completed_at"`
	CreatedAt   time.Time            `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time            `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the diagnostic task step table name.
// TableName 指定诊断任务步骤表名。
func (DiagnosticTaskStep) TableName() string {
	return "diagnostics_task_steps"
}

// DiagnosticNodeExecution persists one node-level execution state.
// DiagnosticNodeExecution 存储单个节点级执行状态。
type DiagnosticNodeExecution struct {
	ID            uint                 `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID        uint                 `json:"task_id" gorm:"index;not null;uniqueIndex:idx_diagnostics_task_node_scope"`
	TaskStepID    *uint                `json:"task_step_id" gorm:"index"`
	ClusterNodeID uint                 `json:"cluster_node_id" gorm:"index"`
	NodeID        uint                 `json:"node_id" gorm:"index"`
	HostID        uint                 `json:"host_id" gorm:"index;not null;uniqueIndex:idx_diagnostics_task_node_scope"`
	HostName      string               `json:"host_name" gorm:"size:100"`
	HostIP        string               `json:"host_ip" gorm:"size:64"`
	Role          string               `json:"role" gorm:"size:32;not null;uniqueIndex:idx_diagnostics_task_node_scope"`
	AgentID       string               `json:"agent_id" gorm:"size:120;index"`
	InstallDir    string               `json:"install_dir" gorm:"size:255"`
	Status        DiagnosticTaskStatus `json:"status" gorm:"size:32;index;not null"`
	CurrentStep   DiagnosticStepCode   `json:"current_step" gorm:"size:64;index"`
	Message       string               `json:"message" gorm:"type:text"`
	Error         string               `json:"error" gorm:"type:text"`
	StartedAt     *time.Time           `json:"started_at"`
	CompletedAt   *time.Time           `json:"completed_at"`
	CreatedAt     time.Time            `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time            `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName specifies the diagnostic node execution table name.
// TableName 指定诊断节点执行表名。
func (DiagnosticNodeExecution) TableName() string {
	return "diagnostics_task_nodes"
}

// DiagnosticStepLog persists one task execution log.
// DiagnosticStepLog 存储一条任务执行日志。
type DiagnosticStepLog struct {
	ID              uint                   `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID          uint                   `json:"task_id" gorm:"index;not null"`
	TaskStepID      *uint                  `json:"task_step_id" gorm:"index"`
	NodeExecutionID *uint                  `json:"node_execution_id" gorm:"index"`
	StepCode        DiagnosticStepCode     `json:"step_code" gorm:"size:64;index"`
	Level           DiagnosticLogLevel     `json:"level" gorm:"size:10;index;not null"`
	EventType       DiagnosticLogEventType `json:"event_type" gorm:"size:20;index;not null"`
	Message         string                 `json:"message" gorm:"type:text"`
	CommandSummary  string                 `json:"command_summary" gorm:"type:text"`
	Metadata        DiagnosticLogMetadata  `json:"metadata" gorm:"type:json"`
	CreatedAt       time.Time              `json:"created_at" gorm:"autoCreateTime;index"`
}

// TableName specifies the diagnostic step log table name.
// TableName 指定诊断步骤日志表名。
func (DiagnosticStepLog) TableName() string {
	return "diagnostics_task_logs"
}

// DiagnosticTaskListFilter defines task list filters.
// DiagnosticTaskListFilter 定义任务列表过滤条件。
type DiagnosticTaskListFilter struct {
	ClusterID     uint                     `json:"cluster_id"`
	TriggerSource DiagnosticTaskSourceType `json:"trigger_source"`
	Status        DiagnosticTaskStatus     `json:"status"`
	Page          int                      `json:"page"`
	PageSize      int                      `json:"page_size"`
	OwnerUserID   uint                     `json:"-"`
	IncludeAll    bool                     `json:"-"`
}

// DiagnosticTaskSummary defines task list summary fields.
// DiagnosticTaskSummary 定义任务列表摘要字段。
type DiagnosticTaskSummary struct {
	ID            uint                     `json:"id"`
	ExecutionID   string                   `json:"execution_id,omitempty"`
	ClusterID     uint                     `json:"cluster_id"`
	TriggerSource DiagnosticTaskSourceType `json:"trigger_source"`
	Status        DiagnosticTaskStatus     `json:"status"`
	CurrentStep   DiagnosticStepCode       `json:"current_step"`
	FailureStep   DiagnosticStepCode       `json:"failure_step"`
	FailureReason string                   `json:"failure_reason"`
	Summary       string                   `json:"summary"`
	StartedAt     *time.Time               `json:"started_at"`
	CompletedAt   *time.Time               `json:"completed_at"`
	CreatedBy     uint                     `json:"created_by"`
	CreatedByName string                   `json:"created_by_name"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

// DiagnosticTaskLogFilter defines log query filters.
// DiagnosticTaskLogFilter 定义任务日志查询过滤条件。
type DiagnosticTaskLogFilter struct {
	TaskID          uint
	StepCode        DiagnosticStepCode
	NodeExecutionID *uint
	Level           DiagnosticLogLevel
	Page            int
	PageSize        int
	OwnerUserID     uint
	IncludeAll      bool
}

// CreateDiagnosticTaskRequest describes one task creation request.
// CreateDiagnosticTaskRequest 描述一条诊断任务创建请求。
type CreateDiagnosticTaskRequest struct {
	ClusterID       uint                     `json:"cluster_id"`
	TriggerSource   DiagnosticTaskSourceType `json:"trigger_source"`
	SourceRef       DiagnosticTaskSourceRef  `json:"source_ref"`
	NodeScope       DiagnosticTaskNodeScope  `json:"node_scope,omitempty"`
	SelectedNodeIDs []uint                   `json:"selected_node_ids"`
	Options         DiagnosticTaskOptions    `json:"options"`
	// LookbackMinutes overrides the default diagnostics collection window in minutes.
	// LookbackMinutes 覆盖默认诊断采集时间窗口（分钟），0 表示使用默认或来源巡检窗口。
	LookbackMinutes int    `json:"lookback_minutes,omitempty"`
	Summary         string `json:"summary"`
	AutoStart       bool   `json:"auto_start"`
}

// DiagnosticTaskListData is the paginated diagnostics task payload.
// DiagnosticTaskListData 是分页诊断任务列表载荷。
type DiagnosticTaskListData struct {
	Items    []*DiagnosticTaskSummary `json:"items"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

// DiagnosticTaskLogListData is the paginated diagnostics task log payload.
// DiagnosticTaskLogListData 是分页诊断任务日志载荷。
type DiagnosticTaskLogListData struct {
	Items    []*DiagnosticStepLog `json:"items"`
	Total    int64                `json:"total"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
}

func normalizeJSONScanBytes(value interface{}) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []byte:
		return typed, nil
	case string:
		return []byte(typed), nil
	default:
		return nil, errors.New("diagnostics: failed to scan JSON value")
	}
}
