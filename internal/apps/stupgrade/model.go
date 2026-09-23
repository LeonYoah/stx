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
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// LogMetadata 表示步骤日志的结构化上下文。
// LogMetadata represents the structured context of a step log.
type LogMetadata map[string]interface{}

// Value 实现 driver.Valuer，用于 JSON 存储。
// Value implements driver.Valuer for JSON storage.
func (m LogMetadata) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

// Scan 实现 sql.Scanner，用于 JSON 读取。
// Scan implements sql.Scanner for JSON retrieval.
func (m *LogMetadata) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("stupgrade: failed to scan LogMetadata - expected []byte")
	}
	return json.Unmarshal(bytes, m)
}

// Value 实现 driver.Valuer，用于计划快照存储。
// Value implements driver.Valuer for plan snapshot storage.
func (p UpgradePlanSnapshot) Value() (driver.Value, error) {
	return json.Marshal(p)
}

// Scan 实现 sql.Scanner，用于计划快照读取。
// Scan implements sql.Scanner for plan snapshot retrieval.
func (p *UpgradePlanSnapshot) Scan(value interface{}) error {
	if value == nil {
		*p = UpgradePlanSnapshot{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("stupgrade: failed to scan UpgradePlanSnapshot - expected []byte")
	}
	return json.Unmarshal(bytes, p)
}

// UpgradePlanRecord 持久化升级计划快照。
// UpgradePlanRecord persists the upgrade plan snapshot.
type UpgradePlanRecord struct {
	ID                 uint                `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID          uint                `json:"cluster_id" gorm:"index;not null"`
	SourceVersion      string              `json:"source_version" gorm:"size:50;not null"`
	TargetVersion      string              `json:"target_version" gorm:"size:50;not null;index"`
	Status             PlanStatus          `json:"status" gorm:"size:20;not null;index"`
	BlockingIssueCount int                 `json:"blocking_issue_count" gorm:"default:0"`
	Snapshot           UpgradePlanSnapshot `json:"snapshot" gorm:"type:json;not null"`
	CreatedBy          uint                `json:"created_by"`
	CreatedAt          time.Time           `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          time.Time           `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定升级计划表名。
// TableName specifies the upgrade plan table name.
func (UpgradePlanRecord) TableName() string {
	return "st_upgrade_plans"
}

// UpgradeTask 持久化升级执行任务。
// UpgradeTask persists an upgrade execution task.
type UpgradeTask struct {
	ID             uint                   `json:"id" gorm:"primaryKey;autoIncrement"`
	ExecutionID    string                 `json:"execution_id,omitempty" gorm:"size:36;index"`
	ClusterID      uint                   `json:"cluster_id" gorm:"index;not null"`
	PlanID         uint                   `json:"plan_id" gorm:"index;not null"`
	SourceVersion  string                 `json:"source_version" gorm:"size:50;not null"`
	TargetVersion  string                 `json:"target_version" gorm:"size:50;not null;index"`
	Status         ExecutionStatus        `json:"status" gorm:"size:32;not null;index"`
	CurrentStep    StepCode               `json:"current_step" gorm:"size:64;index"`
	FailureStep    StepCode               `json:"failure_step" gorm:"size:64"`
	FailureReason  string                 `json:"failure_reason" gorm:"type:text"`
	RollbackStatus ExecutionStatus        `json:"rollback_status" gorm:"size:32;index"`
	RollbackReason string                 `json:"rollback_reason" gorm:"type:text"`
	StartedAt      *time.Time             `json:"started_at"`
	CompletedAt    *time.Time             `json:"completed_at"`
	CreatedBy      uint                   `json:"created_by"`
	CreatedAt      time.Time              `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time              `json:"updated_at" gorm:"autoUpdateTime"`
	Plan           UpgradePlanRecord      `json:"plan" gorm:"foreignKey:PlanID"`
	Steps          []UpgradeTaskStep      `json:"steps" gorm:"foreignKey:TaskID"`
	NodeExecutions []UpgradeNodeExecution `json:"node_executions" gorm:"foreignKey:TaskID"`
}

// TableName 指定升级任务表名。
// TableName specifies the upgrade task table name.
func (UpgradeTask) TableName() string {
	return "st_upgrade_tasks"
}

// UpgradeTaskStep 持久化任务步骤。
// UpgradeTaskStep persists a task step.
type UpgradeTaskStep struct {
	ID          uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID      uint            `json:"task_id" gorm:"index;not null;uniqueIndex:idx_st_upgrade_task_step_code"`
	Code        StepCode        `json:"code" gorm:"size:64;not null;uniqueIndex:idx_st_upgrade_task_step_code"`
	Sequence    int             `json:"sequence" gorm:"not null;index"`
	Status      ExecutionStatus `json:"status" gorm:"size:32;not null;index"`
	Message     string          `json:"message" gorm:"type:text"`
	Error       string          `json:"error" gorm:"type:text"`
	RetryCount  int             `json:"retry_count" gorm:"default:0"`
	StartedAt   *time.Time      `json:"started_at"`
	CompletedAt *time.Time      `json:"completed_at"`
	CreatedAt   time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定升级任务步骤表名。
// TableName specifies the upgrade task step table name.
func (UpgradeTaskStep) TableName() string {
	return "st_upgrade_task_steps"
}

// UpgradeNodeExecution 持久化节点级执行状态。
// UpgradeNodeExecution persists node-level execution state.
type UpgradeNodeExecution struct {
	ID               uint            `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID           uint            `json:"task_id" gorm:"index;not null;uniqueIndex:idx_st_upgrade_task_node_scope"`
	TaskStepID       *uint           `json:"task_step_id" gorm:"index"`
	ClusterNodeID    uint            `json:"cluster_node_id" gorm:"index"`
	HostID           uint            `json:"host_id" gorm:"index;not null;uniqueIndex:idx_st_upgrade_task_node_scope"`
	HostName         string          `json:"host_name" gorm:"size:100"`
	HostIP           string          `json:"host_ip" gorm:"size:64"`
	Role             string          `json:"role" gorm:"size:32;not null;uniqueIndex:idx_st_upgrade_task_node_scope"`
	Status           ExecutionStatus `json:"status" gorm:"size:32;not null;index"`
	CurrentStep      StepCode        `json:"current_step" gorm:"size:64;index"`
	SourceVersion    string          `json:"source_version" gorm:"size:50;not null"`
	TargetVersion    string          `json:"target_version" gorm:"size:50;not null"`
	SourceInstallDir string          `json:"source_install_dir" gorm:"size:255"`
	TargetInstallDir string          `json:"target_install_dir" gorm:"size:255"`
	Message          string          `json:"message" gorm:"type:text"`
	Error            string          `json:"error" gorm:"type:text"`
	StartedAt        *time.Time      `json:"started_at"`
	CompletedAt      *time.Time      `json:"completed_at"`
	CreatedAt        time.Time       `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        time.Time       `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定节点执行表名。
// TableName specifies the node execution table name.
func (UpgradeNodeExecution) TableName() string {
	return "st_upgrade_task_nodes"
}

// UpgradeStepLog 持久化步骤与节点日志。
// UpgradeStepLog persists step and node logs.
type UpgradeStepLog struct {
	ID              uint         `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID          uint         `json:"task_id" gorm:"index;not null"`
	TaskStepID      *uint        `json:"task_step_id" gorm:"index"`
	NodeExecutionID *uint        `json:"node_execution_id" gorm:"index"`
	StepCode        StepCode     `json:"step_code" gorm:"size:64;index"`
	Level           LogLevel     `json:"level" gorm:"size:10;not null;index"`
	EventType       LogEventType `json:"event_type" gorm:"size:20;not null;index"`
	Message         string       `json:"message" gorm:"type:text"`
	CommandSummary  string       `json:"command_summary" gorm:"type:text"`
	ExitCode        *int         `json:"exit_code"`
	Metadata        LogMetadata  `json:"metadata" gorm:"type:json"`
	CreatedAt       time.Time    `json:"created_at" gorm:"autoCreateTime;index"`
}

// TableName 指定升级日志表名。
// TableName specifies the upgrade step log table name.
func (UpgradeStepLog) TableName() string {
	return "st_upgrade_step_logs"
}
