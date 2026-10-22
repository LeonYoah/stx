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

package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/db"
	"gorm.io/gorm"
)

// ScaleKind 是伸缩操作类型：扩容或缩容。
// ScaleKind is the scaling operation type: scale-out or scale-in.
type ScaleKind string

const (
	// ScaleKindOut 扩容（添加节点）。
	// ScaleKindOut is a scale-out (add a node).
	ScaleKindOut ScaleKind = "scale_out"
	// ScaleKindIn 缩容（移除节点）。
	// ScaleKindIn is a scale-in (remove a node).
	ScaleKindIn ScaleKind = "scale_in"
)

// ScaleMode 是扩容模式：在主机上安装 SeaTunnel 后入群，或纳管主机上已运行的 SeaTunnel。
// ScaleMode is the scale-out mode: install SeaTunnel on the host and join, or register an already running one.
type ScaleMode string

const (
	// ScaleModeInstall 安装并加入集群。
	// ScaleModeInstall installs SeaTunnel and joins the cluster.
	ScaleModeInstall ScaleMode = "install"
	// ScaleModeRegister 纳管已运行的节点（以节点为准，只登记不覆盖）。
	// ScaleModeRegister registers an already running node (the node is the truth; register only, never overwrite).
	ScaleModeRegister ScaleMode = "register"
)

// ScaleOperationStatus 是伸缩操作整体状态。
// ScaleOperationStatus is the overall status of a scaling operation.
type ScaleOperationStatus string

const (
	ScaleOperationRunning   ScaleOperationStatus = "running"
	ScaleOperationSucceeded ScaleOperationStatus = "succeeded"
	ScaleOperationFailed    ScaleOperationStatus = "failed"
)

// ScaleStepName 是伸缩操作中的步骤编码（稳定常量，前端据此本地化展示）。
// ScaleStepName is a step code of a scaling operation (stable constant localized by the frontend).
type ScaleStepName string

const (
	ScaleStepValidatePlan     ScaleStepName = "validate_plan"
	ScaleStepFixLoopback      ScaleStepName = "fix_loopback"
	ScaleStepInstallFiles     ScaleStepName = "install_files"
	ScaleStepRegisterNode     ScaleStepName = "register_node"
	ScaleStepApplyConfigs     ScaleStepName = "apply_configs"
	ScaleStepReplayPlugins    ScaleStepName = "replay_plugins"
	ScaleStepStartNode        ScaleStepName = "start_node"
	ScaleStepPatchMembers     ScaleStepName = "patch_members"
	ScaleStepVerifyMembership ScaleStepName = "verify_membership"
	ScaleStepStopNode         ScaleStepName = "stop_node"
	ScaleStepCleanupRecords   ScaleStepName = "cleanup_records"
	ScaleStepRefreshMonitor   ScaleStepName = "refresh_monitor"
	ScaleStepFollowJavaProxy  ScaleStepName = "follow_java_proxy"
	ScaleStepVerify           ScaleStepName = "verify"
)

// ScaleStepStatus 是单个步骤的状态；partial 表示完成但有可重试的局部失败（不阻断后续步骤）。
// ScaleStepStatus is the status of one step; partial means done with retryable partial failures (does not block
// later steps).
type ScaleStepStatus string

const (
	ScaleStepPending   ScaleStepStatus = "pending"
	ScaleStepRunning   ScaleStepStatus = "running"
	ScaleStepSucceeded ScaleStepStatus = "succeeded"
	ScaleStepPartial   ScaleStepStatus = "partial"
	ScaleStepFailed    ScaleStepStatus = "failed"
	ScaleStepSkipped   ScaleStepStatus = "skipped"
)

// isDone 判断步骤是否已完成（重试时跳过）。
// isDone reports whether a step is finished (skipped on retry).
func (s ScaleStepStatus) isDone() bool {
	return s == ScaleStepSucceeded || s == ScaleStepPartial || s == ScaleStepSkipped
}

// scaleErrorInterruptedByRestart 是控制面重启打断进行中操作时写入的错误编码。
// scaleErrorInterruptedByRestart is the error code written when a Control Plane restart interrupts an operation.
const scaleErrorInterruptedByRestart = "interrupted_by_restart"

// ScaleStepState 是持久化的单步状态，Result 保存该步骤的结构化结果（不含敏感内容）。
// ScaleStepState is the persisted state of one step; Result keeps the step's structured result (no secrets).
type ScaleStepState struct {
	Name       ScaleStepName   `json:"name"`
	Status     ScaleStepStatus `json:"status"`
	Attempts   int             `json:"attempts"`
	Message    string          `json:"message,omitempty"`
	Error      string          `json:"error,omitempty"`
	StartedAt  *time.Time      `json:"started_at,omitempty"`
	FinishedAt *time.Time      `json:"finished_at,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}

// ClusterScaleOperation 伸缩操作（扩容/缩容）及其分步状态。
// ClusterScaleOperation is a scale-out/scale-in operation with per-step status.
type ClusterScaleOperation struct {
	ID          uint                 `json:"id" gorm:"primaryKey;autoIncrement"`
	ExecutionID string               `json:"execution_id" gorm:"size:64;index"`
	ClusterID   uint                 `json:"cluster_id" gorm:"index;not null"`
	HostID      uint                 `json:"host_id" gorm:"index"`
	NodeIDs     string               `json:"-" gorm:"type:text"` // JSON []uint
	Kind        ScaleKind            `json:"kind" gorm:"size:20;not null"`
	Mode        ScaleMode            `json:"mode" gorm:"size:20"`
	Status      ScaleOperationStatus `json:"status" gorm:"size:20;index"`
	CurrentStep ScaleStepName        `json:"current_step" gorm:"size:40"`
	// LoopbackRestartAck / JobsRiskAck 记录提交时用户的显式确认，执行与重试时据此复核计划护栏。
	// LoopbackRestartAck / JobsRiskAck record the user's explicit acknowledgements at submission, re-checked by
	// the plan guardrails during execution and retry.
	LoopbackRestartAck bool `json:"loopback_restart_ack" gorm:"not null"`
	JobsRiskAck        bool `json:"jobs_risk_ack" gorm:"not null"`
	// Lang 是提交时的界面语言（zh/en），执行器据此输出单语言步骤说明。
	// Lang is the UI language at submission (zh/en); the executor writes single-language step messages with it.
	Lang      string        `json:"lang" gorm:"size:8"`
	PlanJSON  db.ScriptText `json:"-"` // 提交时的 Plan 快照（不含敏感内容） / plan snapshot at submission (no secrets)
	StepsJSON db.ScriptText `json:"-"` // []ScaleStepState
	Error     string        `json:"error" gorm:"type:text"`
	CreatedBy uint          `json:"created_by" gorm:"index"`
	// CreatedByName 是发起人登录名快照，用于并发冲突提示（用户改名不回溯）。
	// CreatedByName is a snapshot of the initiator's login name for conflict messages (not updated on rename).
	CreatedByName string     `json:"created_by_name" gorm:"size:100"`
	CreatedAt     time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	FinishedAt    *time.Time `json:"finished_at"`
}

// TableName 指定伸缩操作表名。
// TableName returns the scaling operation table name.
func (ClusterScaleOperation) TableName() string {
	return "cluster_scale_operations"
}

// steps 解析持久化的步骤状态；解析失败或为空时返回空切片。
// steps decodes the persisted step states; an empty slice is returned when empty or invalid.
func (op *ClusterScaleOperation) steps() []ScaleStepState {
	states := make([]ScaleStepState, 0)
	if op == nil || strings.TrimSpace(op.StepsJSON.String()) == "" {
		return states
	}
	if err := json.Unmarshal([]byte(op.StepsJSON.String()), &states); err != nil || states == nil {
		return make([]ScaleStepState, 0)
	}
	return states
}

// setSteps 序列化步骤状态到 StepsJSON。
// setSteps encodes the step states into StepsJSON.
func (op *ClusterScaleOperation) setSteps(states []ScaleStepState) {
	if states == nil {
		states = make([]ScaleStepState, 0)
	}
	content, err := json.Marshal(states)
	if err != nil {
		return
	}
	op.StepsJSON = db.ScriptText(content)
}

// plan 解析提交时的计划快照。
// plan decodes the plan snapshot captured at submission.
func (op *ClusterScaleOperation) plan() (*ScalePlan, error) {
	if op == nil || strings.TrimSpace(op.PlanJSON.String()) == "" {
		return nil, ErrScaleOperationCorrupted
	}
	var plan ScalePlan
	if err := json.Unmarshal([]byte(op.PlanJSON.String()), &plan); err != nil {
		return nil, ErrScaleOperationCorrupted
	}
	plan.normalize()
	return &plan, nil
}

// nodeIDs 解析操作关联的节点 ID 列表。
// nodeIDs decodes the node IDs linked to the operation.
func (op *ClusterScaleOperation) nodeIDs() []uint {
	ids := make([]uint, 0)
	if op == nil || strings.TrimSpace(op.NodeIDs) == "" {
		return ids
	}
	if err := json.Unmarshal([]byte(op.NodeIDs), &ids); err != nil || ids == nil {
		return make([]uint, 0)
	}
	return ids
}

// setNodeIDs 写入操作关联的节点 ID 列表。
// setNodeIDs stores the node IDs linked to the operation.
func (op *ClusterScaleOperation) setNodeIDs(ids []uint) {
	if ids == nil {
		ids = make([]uint, 0)
	}
	content, err := json.Marshal(ids)
	if err != nil {
		return
	}
	op.NodeIDs = string(content)
}

// ScaleOperationInfo 是伸缩操作的 API 视图：计划与步骤已解析，数组字段永不为 null。
// ScaleOperationInfo is the API view of a scaling operation with the plan and steps decoded; array fields are
// never null.
type ScaleOperationInfo struct {
	ID                 uint                 `json:"id"`
	ExecutionID        string               `json:"execution_id"`
	ClusterID          uint                 `json:"cluster_id"`
	HostID             uint                 `json:"host_id"`
	NodeIDs            []uint               `json:"node_ids"`
	Kind               ScaleKind            `json:"kind"`
	Mode               ScaleMode            `json:"mode,omitempty"`
	Status             ScaleOperationStatus `json:"status"`
	CurrentStep        ScaleStepName        `json:"current_step"`
	LoopbackRestartAck bool                 `json:"loopback_restart_ack"`
	JobsRiskAck        bool                 `json:"jobs_risk_ack"`
	Error              string               `json:"error,omitempty"`
	// Warnings 汇总 partial 步骤的说明，操作仍按成功结束。
	// Warnings summarizes partial steps; the operation still finishes as succeeded.
	Warnings  []string         `json:"warnings"`
	Retryable bool             `json:"retryable"`
	Plan      *ScalePlan       `json:"plan"`
	Steps     []ScaleStepState `json:"steps"`
	CreatedBy uint             `json:"created_by"`
	// CreatedByName 发起人登录名快照 / initiator login name snapshot
	CreatedByName string     `json:"created_by_name"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// toInfo 转换为 API 视图。
// toInfo converts the operation to its API view.
func (op *ClusterScaleOperation) toInfo() *ScaleOperationInfo {
	if op == nil {
		return nil
	}
	steps := op.steps()
	warnings := make([]string, 0)
	for _, step := range steps {
		if step.Status == ScaleStepPartial {
			warnings = append(warnings, firstNonEmpty(step.Message, step.Error, string(step.Name)))
		}
	}
	info := &ScaleOperationInfo{
		ID:                 op.ID,
		ExecutionID:        op.ExecutionID,
		ClusterID:          op.ClusterID,
		HostID:             op.HostID,
		NodeIDs:            op.nodeIDs(),
		Kind:               op.Kind,
		Mode:               op.Mode,
		Status:             op.Status,
		CurrentStep:        op.CurrentStep,
		LoopbackRestartAck: op.LoopbackRestartAck,
		JobsRiskAck:        op.JobsRiskAck,
		Error:              op.Error,
		Warnings:           warnings,
		Retryable:          op.Status == ScaleOperationFailed,
		Steps:              steps,
		CreatedBy:          op.CreatedBy,
		CreatedByName:      op.CreatedByName,
		CreatedAt:          op.CreatedAt,
		UpdatedAt:          op.UpdatedAt,
		FinishedAt:         op.FinishedAt,
	}
	if plan, err := op.plan(); err == nil {
		info.Plan = plan
	}
	return info
}

// ScaleOperationFilter 是伸缩操作列表的过滤条件；OwnerUserID 与 IncludeAll 在仓库查询时限定归属。
// ScaleOperationFilter filters scaling operation lists; OwnerUserID and IncludeAll scope ownership in the query.
type ScaleOperationFilter struct {
	ClusterID   uint
	Status      ScaleOperationStatus
	OwnerUserID uint
	IncludeAll  bool
	Limit       int
}

// ScaleRepository 是伸缩操作的数据访问层。
// ScaleRepository is the data access layer of scaling operations.
type ScaleRepository struct {
	db *gorm.DB
}

// NewScaleRepository 创建伸缩操作仓库。
// NewScaleRepository creates a scaling operation repository.
func NewScaleRepository(database *gorm.DB) *ScaleRepository {
	return &ScaleRepository{db: database}
}

// Create 新建伸缩操作。
// Create inserts a scaling operation.
func (r *ScaleRepository) Create(ctx context.Context, op *ClusterScaleOperation) error {
	return r.db.WithContext(ctx).Create(op).Error
}

// Save 保存伸缩操作的全部字段。
// Save persists every field of a scaling operation.
func (r *ScaleRepository) Save(ctx context.Context, op *ClusterScaleOperation) error {
	return r.db.WithContext(ctx).Save(op).Error
}

// GetByID 按 ID 读取伸缩操作，不存在时返回 ErrScaleOperationNotFound。
// GetByID loads a scaling operation by ID and returns ErrScaleOperationNotFound when missing.
func (r *ScaleRepository) GetByID(ctx context.Context, id uint) (*ClusterScaleOperation, error) {
	var op ClusterScaleOperation
	if err := r.db.WithContext(ctx).First(&op, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScaleOperationNotFound
		}
		return nil, err
	}
	return &op, nil
}

// GetForOwner 在集群与归属条件下读取伸缩操作；非管理员只能读取自己创建的操作。
// GetForOwner loads a scaling operation scoped by cluster and ownership; non-admins only see their own.
func (r *ScaleRepository) GetForOwner(ctx context.Context, clusterID, id, ownerUserID uint, includeAll bool) (*ClusterScaleOperation, error) {
	query := r.db.WithContext(ctx).Where("id = ? AND cluster_id = ?", id, clusterID)
	if !includeAll {
		query = query.Where("created_by = ?", ownerUserID)
	}
	var op ClusterScaleOperation
	if err := query.First(&op).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScaleOperationNotFound
		}
		return nil, err
	}
	return &op, nil
}

// List 按过滤条件列出伸缩操作（最新在前），归属条件在查询中加入。
// List lists scaling operations (newest first) with ownership applied in the query.
func (r *ScaleRepository) List(ctx context.Context, filter ScaleOperationFilter) ([]*ClusterScaleOperation, error) {
	query := r.db.WithContext(ctx).Model(&ClusterScaleOperation{})
	if filter.ClusterID > 0 {
		query = query.Where("cluster_id = ?", filter.ClusterID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if !filter.IncludeAll {
		query = query.Where("created_by = ?", filter.OwnerUserID)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	ops := make([]*ClusterScaleOperation, 0)
	if err := query.Order("id DESC").Limit(limit).Find(&ops).Error; err != nil {
		return nil, err
	}
	return ops, nil
}

// FindRunningByCluster 返回集群上进行中的伸缩操作（不区分归属，用于互斥），没有时返回 nil。
// FindRunningByCluster returns the running scaling operation of a cluster regardless of owner (for mutual
// exclusion), or nil when none.
func (r *ScaleRepository) FindRunningByCluster(ctx context.Context, clusterID uint) (*ClusterScaleOperation, error) {
	var op ClusterScaleOperation
	err := r.db.WithContext(ctx).
		Where("cluster_id = ? AND status = ?", clusterID, ScaleOperationRunning).
		Order("id DESC").
		First(&op).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &op, nil
}

// ListRunning 返回全部进行中的伸缩操作（控制面启动时用于标记被打断的操作）。
// ListRunning returns every running scaling operation (used at Control Plane startup to mark interruptions).
func (r *ScaleRepository) ListRunning(ctx context.Context) ([]*ClusterScaleOperation, error) {
	ops := make([]*ClusterScaleOperation, 0)
	if err := r.db.WithContext(ctx).Where("status = ?", ScaleOperationRunning).Order("id").Find(&ops).Error; err != nil {
		return nil, err
	}
	return ops, nil
}
