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

// Package audit provides command logging and audit trail functionality for the STX Agent system.
// 审计包提供 STX Agent 系统的命令日志和审计追踪功能。
package audit

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// CommandStatus represents the execution status of a command.
// CommandStatus 表示命令的执行状态。
type CommandStatus string

const (
	// CommandStatusPending indicates the command is waiting to be executed.
	// CommandStatusPending 表示命令正在等待执行。
	CommandStatusPending CommandStatus = "pending"
	// CommandStatusRunning indicates the command is currently executing.
	// CommandStatusRunning 表示命令正在执行中。
	CommandStatusRunning CommandStatus = "running"
	// CommandStatusSuccess indicates the command completed successfully.
	// CommandStatusSuccess 表示命令执行成功。
	CommandStatusSuccess CommandStatus = "success"
	// CommandStatusFailed indicates the command execution failed.
	// CommandStatusFailed 表示命令执行失败。
	CommandStatusFailed CommandStatus = "failed"
	// CommandStatusCancelled indicates the command was cancelled.
	// CommandStatusCancelled 表示命令已被取消。
	CommandStatusCancelled CommandStatus = "cancelled"
)

// CommandParameters represents the JSON parameters for a command.
// CommandParameters 表示命令的 JSON 参数。
type CommandParameters map[string]interface{}

// Value implements the driver.Valuer interface for database storage.
// Value 实现 driver.Valuer 接口用于数据库存储。
func (p CommandParameters) Value() (driver.Value, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(p)
}

// Scan implements the sql.Scanner interface for database retrieval.
// Scan 实现 sql.Scanner 接口用于数据库读取。
func (p *CommandParameters) Scan(value interface{}) error {
	if value == nil {
		*p = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("audit: failed to scan CommandParameters - expected []byte")
	}
	return json.Unmarshal(bytes, p)
}

// AuditDetails represents the JSON details for an audit log entry.
// AuditDetails 表示审计日志条目的 JSON 详情。
type AuditDetails map[string]interface{}

// Value implements the driver.Valuer interface for database storage.
// Value 实现 driver.Valuer 接口用于数据库存储。
func (d AuditDetails) Value() (driver.Value, error) {
	if d == nil {
		return nil, nil
	}
	return json.Marshal(d)
}

// Scan implements the sql.Scanner interface for database retrieval.
// Scan 实现 sql.Scanner 接口用于数据库读取。
func (d *AuditDetails) Scan(value interface{}) error {
	if value == nil {
		*d = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("audit: failed to scan AuditDetails - expected []byte")
	}
	return json.Unmarshal(bytes, d)
}

// CommandLog represents a record of command execution by an Agent.
// CommandLog 表示 Agent 执行命令的记录。
// Requirements: 10.1, 10.3
type CommandLog struct {
	ID          uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	CommandID   string `json:"command_id" gorm:"size:50;uniqueIndex;not null"`
	RequestID   string `json:"request_id,omitempty" gorm:"size:64;index"`
	ExecutionID string `json:"execution_id,omitempty" gorm:"size:36;index"`
	AgentID     string `json:"agent_id" gorm:"size:100;not null;index"`
	HostID      *uint  `json:"host_id" gorm:"index"`
	CommandType string `json:"command_type" gorm:"size:30;not null"`
	ClientType  string `json:"client_type,omitempty" gorm:"size:20;index"`
	// DisplayCommand 保存经过脱敏、适合审计展示的实际命令或 Agent 操作说明。
	// DisplayCommand stores a redacted command or Agent action description suitable for audit display.
	DisplayCommand string            `json:"display_command,omitempty" gorm:"type:text"`
	Parameters     CommandParameters `json:"parameters" gorm:"type:json"`
	Status         CommandStatus     `json:"status" gorm:"size:20;not null;index"`
	Progress       int               `json:"progress" gorm:"default:0"`
	// Output 存储命令执行的原始内容输出，兼容 SQLite/MySQL/PostgreSQL。
	// Output stores the raw command execution output, compatible with SQLite/MySQL/PostgreSQL.
	Output     string     `json:"output" gorm:"type:text"`
	Error      string     `json:"error" gorm:"type:text"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	CreatedAt  time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	CreatedBy  *uint      `json:"created_by"`
}

// TableName specifies the table name for the CommandLog model.
// TableName 指定 CommandLog 模型的表名。
func (CommandLog) TableName() string {
	return "command_logs"
}

// AuditLog represents an audit trail entry for system operations.
// AuditLog 表示系统操作的审计追踪条目。
// Requirements: 10.3, 10.4
type AuditLog struct {
	ID           uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       *uint  `json:"user_id" gorm:"index"`
	Username     string `json:"username" gorm:"size:100"`
	Action       string `json:"action" gorm:"size:50;not null;index"`
	ResourceType string `json:"resource_type" gorm:"size:50;not null;index:idx_resource"`
	ResourceID   string `json:"resource_id" gorm:"size:100;index:idx_resource"`
	ResourceName string `json:"resource_name" gorm:"size:200"`
	RequestID    string `json:"request_id,omitempty" gorm:"size:64;index"`
	ExecutionID  string `json:"execution_id,omitempty" gorm:"size:36;index"`
	CommandID    string `json:"command_id,omitempty" gorm:"size:50;index"`
	ClientType   string `json:"client_type,omitempty" gorm:"size:20;index"`
	RiskLevel    string `json:"risk_level,omitempty" gorm:"size:4;index"`
	ResultStatus string `json:"result_status,omitempty" gorm:"size:32;index"`
	// Trigger: "auto" (Agent) or "manual" (user), empty for legacy records.
	// Trigger：自动（Agent）或手动（用户），空表示旧数据。
	Trigger   string       `json:"trigger" gorm:"size:20;index"`
	Details   AuditDetails `json:"details" gorm:"type:json"`
	IPAddress string       `json:"ip_address" gorm:"size:45"`
	UserAgent string       `json:"user_agent" gorm:"size:500"`
	CreatedAt time.Time    `json:"created_at" gorm:"autoCreateTime;index"`
}

// TableName specifies the table name for the AuditLog model.
// TableName 指定 AuditLog 模型的表名。
func (AuditLog) TableName() string {
	return "audit_logs"
}

// CommandLogFilter represents filter criteria for querying command logs.
// CommandLogFilter 表示查询命令日志的过滤条件。
type CommandLogFilter struct {
	CommandID   string        `json:"command_id"`
	AgentID     string        `json:"agent_id"`
	HostID      *uint         `json:"host_id"`
	CommandType string        `json:"command_type"`
	Status      CommandStatus `json:"status"`
	StartTime   *time.Time    `json:"start_time"`
	EndTime     *time.Time    `json:"end_time"`
	CreatedBy   *uint         `json:"created_by"`
	RequestID   string        `json:"request_id"`
	Page        int           `json:"page"`
	PageSize    int           `json:"page_size"`
}

// AuditLogFilter represents filter criteria for querying audit logs.
// AuditLogFilter 表示查询审计日志的过滤条件。
// Requirements: 10.4
type AuditLogFilter struct {
	UserID   *uint  `json:"user_id"`
	Username string `json:"username"`
	Action   string `json:"action"`
	// ActionGroup 是界面上的操作类别，展开后过滤多个 action。
	// ActionGroup is a UI action category that filters several stored actions.
	ActionGroup  string `json:"-"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	RequestID    string `json:"request_id"`
	ExecutionID  string `json:"execution_id"`
	CommandID    string `json:"command_id"`
	ClientType   string `json:"client_type"`
	ResultStatus string `json:"result_status"`
	IncludeAll   bool   `json:"-"`
	// Trigger filters by trigger column: "auto" (agent) or "manual" (user).
	// Trigger 按 trigger 字段过滤：auto（Agent 自动）或 manual（手动）。
	Trigger   string     `json:"trigger"`
	StartTime *time.Time `json:"start_time"`
	EndTime   *time.Time `json:"end_time"`
	Page      int        `json:"page"`
	PageSize  int        `json:"page_size"`
}

// CommandLogInfo represents command log information for API responses.
// CommandLogInfo 表示 API 响应的命令日志信息。
type CommandLogInfo struct {
	ID             uint              `json:"id"`
	CommandID      string            `json:"command_id"`
	RequestID      string            `json:"request_id,omitempty"`
	ExecutionID    string            `json:"execution_id,omitempty"`
	AgentID        string            `json:"agent_id"`
	HostID         *uint             `json:"host_id"`
	CommandType    string            `json:"command_type"`
	ClientType     string            `json:"client_type,omitempty"`
	DisplayCommand string            `json:"display_command,omitempty"`
	Parameters     CommandParameters `json:"parameters"`
	Status         CommandStatus     `json:"status"`
	Progress       int               `json:"progress"`
	Output         string            `json:"output"`
	Error          string            `json:"error"`
	StartedAt      *time.Time        `json:"started_at"`
	FinishedAt     *time.Time        `json:"finished_at"`
	CreatedAt      time.Time         `json:"created_at"`
	CreatedBy      *uint             `json:"created_by"`
}

// ToCommandLogInfo converts a CommandLog to CommandLogInfo.
// ToCommandLogInfo 将 CommandLog 转换为 CommandLogInfo。
func (c *CommandLog) ToCommandLogInfo() *CommandLogInfo {
	return &CommandLogInfo{
		ID:             c.ID,
		CommandID:      c.CommandID,
		RequestID:      c.RequestID,
		ExecutionID:    c.ExecutionID,
		AgentID:        c.AgentID,
		HostID:         c.HostID,
		CommandType:    c.CommandType,
		ClientType:     c.ClientType,
		DisplayCommand: c.DisplayCommand,
		Parameters:     c.Parameters,
		Status:         c.Status,
		Progress:       c.Progress,
		Output:         c.Output,
		Error:          c.Error,
		StartedAt:      c.StartedAt,
		FinishedAt:     c.FinishedAt,
		CreatedAt:      c.CreatedAt,
		CreatedBy:      c.CreatedBy,
	}
}

// AuditLogInfo represents audit log information for API responses.
// AuditLogInfo 表示 API 响应的审计日志信息。
type AuditLogInfo struct {
	ID           uint         `json:"id"`
	UserID       *uint        `json:"user_id"`
	Username     string       `json:"username"`
	Action       string       `json:"action"`
	ResourceType string       `json:"resource_type"`
	ResourceID   string       `json:"resource_id"`
	ResourceName string       `json:"resource_name"`
	RequestID    string       `json:"request_id,omitempty"`
	ExecutionID  string       `json:"execution_id,omitempty"`
	CommandID    string       `json:"command_id,omitempty"`
	ClientType   string       `json:"client_type,omitempty"`
	RiskLevel    string       `json:"risk_level,omitempty"`
	ResultStatus string       `json:"result_status,omitempty"`
	Trigger      string       `json:"trigger"` // "auto" | "manual" | ""
	Details      AuditDetails `json:"details"`
	IPAddress    string       `json:"ip_address"`
	UserAgent    string       `json:"user_agent"`
	CreatedAt    time.Time    `json:"created_at"`
	// CommandCount 是同一次请求下发到 Agent 的命令数，只在列表接口填充。
	// CommandCount is the number of Agent commands from the same request. Only list responses fill it.
	CommandCount int `json:"command_count,omitempty"`
}

// ToAuditLogInfo converts an AuditLog to AuditLogInfo.
// ToAuditLogInfo 将 AuditLog 转换为 AuditLogInfo。
func (a *AuditLog) ToAuditLogInfo() *AuditLogInfo {
	return &AuditLogInfo{
		ID:           a.ID,
		UserID:       a.UserID,
		Username:     a.Username,
		Action:       a.Action,
		ResourceType: a.ResourceType,
		ResourceID:   a.ResourceID,
		ResourceName: a.ResourceName,
		RequestID:    a.RequestID,
		ExecutionID:  a.ExecutionID,
		CommandID:    a.CommandID,
		ClientType:   a.ClientType,
		RiskLevel:    a.RiskLevel,
		ResultStatus: a.ResultStatus,
		Trigger:      a.Trigger,
		Details:      a.Details,
		IPAddress:    a.IPAddress,
		UserAgent:    a.UserAgent,
		CreatedAt:    a.CreatedAt,
	}
}

// CreateCommandLogRequest represents a request to create a new command log.
// CreateCommandLogRequest 表示创建新命令日志的请求。
type CreateCommandLogRequest struct {
	CommandID   string            `json:"command_id" binding:"required,max=50"`
	AgentID     string            `json:"agent_id" binding:"required,max=100"`
	HostID      *uint             `json:"host_id"`
	CommandType string            `json:"command_type" binding:"required,max=30"`
	Parameters  CommandParameters `json:"parameters"`
	CreatedBy   *uint             `json:"created_by"`
}

// UpdateCommandLogRequest represents a request to update a command log.
// UpdateCommandLogRequest 表示更新命令日志的请求。
type UpdateCommandLogRequest struct {
	Status     *CommandStatus `json:"status"`
	Progress   *int           `json:"progress"`
	Output     *string        `json:"output"`
	Error      *string        `json:"error"`
	StartedAt  *time.Time     `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at"`
}

// CreateAuditLogRequest represents a request to create a new audit log.
// CreateAuditLogRequest 表示创建新审计日志的请求。
type CreateAuditLogRequest struct {
	UserID       *uint        `json:"user_id"`
	Username     string       `json:"username" binding:"max=100"`
	Action       string       `json:"action" binding:"required,max=50"`
	ResourceType string       `json:"resource_type" binding:"required,max=50"`
	ResourceID   string       `json:"resource_id" binding:"max=100"`
	ResourceName string       `json:"resource_name" binding:"max=200"`
	Details      AuditDetails `json:"details"`
	IPAddress    string       `json:"ip_address" binding:"max=45"`
	UserAgent    string       `json:"user_agent" binding:"max=500"`
}
