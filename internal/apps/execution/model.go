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

// Package execution 提供跨业务模块的公共执行记录、安全确认和取消约定。
// Package execution provides shared execution records, safety confirmation, and cancellation contracts across modules.
package execution

import "time"

// Status 表示对外统一的执行状态。
// Status represents the public execution status shared by all modules.
type Status string

const (
	StatusPending         Status = "pending"
	StatusRunning         Status = "running"
	StatusCancelRequested Status = "cancel_requested"
	StatusCancelling      Status = "cancelling"
	StatusCancelled       Status = "cancelled"
	StatusSucceeded       Status = "succeeded"
	StatusFailed          Status = "failed"
	StatusTimedOut        Status = "timed_out"
)

// RiskLevel 表示操作对服务器或集群的影响等级。
// RiskLevel represents the impact level of an operation on a server or cluster.
type RiskLevel string

const (
	RiskLevelR0 RiskLevel = "R0"
	RiskLevelR1 RiskLevel = "R1"
	RiskLevelR2 RiskLevel = "R2"
	RiskLevelR3 RiskLevel = "R3"
)

// ActorType 表示执行由用户还是系统发起。
// ActorType identifies whether an execution was started by a user or by the system.
type ActorType string

const (
	ActorTypeUser   ActorType = "user"
	ActorTypeSystem ActorType = "system"
)

// Execution 保存统一执行协议所需的最小信息，不保存任务正文或敏感配置。
// Execution stores the minimum shared execution contract without request bodies or sensitive configuration.
type Execution struct {
	ID                 uint64     `json:"-" gorm:"primaryKey;autoIncrement"`
	ExecutionID        string     `json:"execution_id" gorm:"size:36;uniqueIndex;not null"`
	OperationID        string     `json:"operation_id" gorm:"size:160;not null;index;uniqueIndex:idx_execution_idempotency"`
	OwnerUserID        uint64     `json:"owner_user_id" gorm:"not null;index;uniqueIndex:idx_execution_idempotency"`
	ActorType          ActorType  `json:"actor_type" gorm:"size:16;not null"`
	Module             string     `json:"module" gorm:"size:64;not null;index"`
	ModuleRef          string     `json:"module_ref" gorm:"size:160;index"`
	RequestID          string     `json:"request_id" gorm:"size:64;index"`
	// ClientType 记录发起端（cli/web/api/system），后续 start/result 审计沿用，避免被改写成 system。
	// ClientType records the originating client so later start/result audits keep it instead of rewriting system.
	ClientType         string     `json:"client_type,omitempty" gorm:"size:20"`
	IdempotencyKeyHash *string    `json:"-" gorm:"size:64;uniqueIndex:idx_execution_idempotency"`
	RequestHash        string     `json:"-" gorm:"size:64"`
	RiskLevel          RiskLevel  `json:"risk_level" gorm:"size:4;not null"`
	Status             Status     `json:"status" gorm:"size:32;not null;index"`
	Cancellable        bool       `json:"cancellable" gorm:"not null;default:false"`
	CancellableReason  string     `json:"cancellable_reason,omitempty" gorm:"size:160"`
	Progress           int        `json:"progress" gorm:"not null;default:0"`
	ResultRef          string     `json:"result_ref,omitempty" gorm:"size:500"`
	ErrorCode          string     `json:"error_code,omitempty" gorm:"size:80"`
	ErrorMessage       string     `json:"error_message,omitempty" gorm:"type:text"`
	StartedAt          *time.Time `json:"started_at,omitempty"`
	FinishedAt         *time.Time `json:"finished_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at" gorm:"autoCreateTime;index"`
	UpdatedAt          time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回公共执行记录的固定表名。
// TableName returns the fixed table name for shared execution records.
func (Execution) TableName() string {
	return "executions"
}

// Confirmation 保存一次性确认编号的哈希和绑定条件，不保存确认编号原文。
// Confirmation stores a one-time confirmation hash and its binding conditions without the raw token.
type Confirmation struct {
	ID                 uint64     `json:"-" gorm:"primaryKey;autoIncrement"`
	ConfirmationHash   string     `json:"-" gorm:"size:64;uniqueIndex;not null"`
	OwnerUserID        uint64     `json:"owner_user_id" gorm:"not null;index"`
	OperationID        string     `json:"operation_id" gorm:"size:160;not null;index"`
	IdempotencyKeyHash string     `json:"-" gorm:"size:64;not null"`
	RequestHash        string     `json:"-" gorm:"size:64;not null"`
	RiskLevel          RiskLevel  `json:"risk_level" gorm:"size:4;not null"`
	Impact             string     `json:"impact" gorm:"type:text"`
	ExpiresAt          time.Time  `json:"expires_at" gorm:"not null;index"`
	ConsumedAt         *time.Time `json:"consumed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 返回一次性确认记录的固定表名。
// TableName returns the fixed table name for one-time confirmation records.
func (Confirmation) TableName() string {
	return "execution_confirmations"
}

// Actor 保存本次调用者的用户身份和管理员标记。
// Actor stores the current user's identity and administrator flag.
type Actor struct {
	UserID  uint64
	IsAdmin bool
}

// IsTerminal 判断状态是否已经结束。
// IsTerminal reports whether a status is terminal.
func IsTerminal(status Status) bool {
	switch status {
	case StatusCancelled, StatusSucceeded, StatusFailed, StatusTimedOut:
		return true
	default:
		return false
	}
}

// CanTransition 判断公共执行状态是否允许迁移到目标状态。
// CanTransition reports whether a public execution status may move to the target status.
func CanTransition(from, to Status) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusPending:
		return to == StatusRunning || to == StatusCancelRequested || to == StatusCancelled || to == StatusFailed
	case StatusRunning:
		return to == StatusCancelRequested || to == StatusSucceeded || to == StatusFailed || to == StatusTimedOut
	case StatusCancelRequested:
		return to == StatusRunning || to == StatusCancelling || to == StatusCancelled || to == StatusSucceeded || to == StatusFailed
	case StatusCancelling:
		return to == StatusCancelled || to == StatusFailed || to == StatusTimedOut
	default:
		return false
	}
}
