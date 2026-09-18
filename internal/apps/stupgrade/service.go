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
	"context"
	"time"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

// Service 封装升级计划和任务的持久化编排基础能力。
// Service wraps the persistence-oriented orchestration primitives for upgrade plans and tasks.
type Service struct {
	repo               *Repository
	events             *taskEventHub
	clusterProvider    ClusterProvider
	hostProvider       HostProvider
	packageProvider    PackageProvider
	pluginProvider     PluginProvider
	configProvider     ConfigProvider
	clusterOperator    ClusterOperator
	packageTransferer  PackageTransferer
	agentCommandSender AgentCommandSender
}

// NewService 创建升级服务实例。
// NewService creates a new upgrade service instance.
func NewService(repo *Repository) *Service {
	return &Service{
		repo:   repo,
		events: newTaskEventHub(),
	}
}

// SetClusterProvider 设置集群查询依赖。
// SetClusterProvider sets the cluster query dependency.
func (s *Service) SetClusterProvider(provider ClusterProvider) {
	s.clusterProvider = provider
}

// SetHostProvider 设置主机查询依赖。
// SetHostProvider sets the host query dependency.
func (s *Service) SetHostProvider(provider HostProvider) {
	s.hostProvider = provider
}

// SetPackageProvider 设置安装包查询依赖。
// SetPackageProvider sets the package query dependency.
func (s *Service) SetPackageProvider(provider PackageProvider) {
	s.packageProvider = provider
}

// SetPluginProvider 设置插件查询依赖。
// SetPluginProvider sets the plugin query dependency.
func (s *Service) SetPluginProvider(provider PluginProvider) {
	s.pluginProvider = provider
}

// SetConfigProvider 设置配置查询依赖。
// SetConfigProvider sets the config query dependency.
func (s *Service) SetConfigProvider(provider ConfigProvider) {
	s.configProvider = provider
}

// SetClusterOperator 设置集群启停与元数据更新依赖。
// SetClusterOperator sets the cluster lifecycle and metadata dependency.
func (s *Service) SetClusterOperator(operator ClusterOperator) {
	s.clusterOperator = operator
}

// SetPackageTransferer 设置安装包分发依赖。
// SetPackageTransferer sets the package transfer dependency.
func (s *Service) SetPackageTransferer(transferer PackageTransferer) {
	s.packageTransferer = transferer
}

// SetAgentCommandSender 设置 Agent 命令发送依赖。
// SetAgentCommandSender sets the Agent command sender dependency.
func (s *Service) SetAgentCommandSender(sender AgentCommandSender) {
	s.agentCommandSender = sender
}

// CreatePlan 持久化升级计划快照。
// CreatePlan persists an upgrade plan snapshot.
func (s *Service) CreatePlan(ctx context.Context, snapshot UpgradePlanSnapshot, createdBy uint, status PlanStatus, blockingIssueCount int) (*UpgradePlanRecord, error) {
	normalized := normalizePlanSnapshot(snapshot)
	plan := &UpgradePlanRecord{
		ClusterID:          normalized.ClusterID,
		SourceVersion:      normalized.SourceVersion,
		TargetVersion:      normalized.TargetVersion,
		Status:             status,
		BlockingIssueCount: blockingIssueCount,
		Snapshot:           normalized,
		CreatedBy:          createdBy,
	}
	if err := s.repo.CreatePlan(ctx, plan); err != nil {
		return nil, err
	}
	return plan, nil
}

// GetPlan 获取升级计划。
// GetPlan returns an upgrade plan.
func (s *Service) GetPlan(ctx context.Context, planID uint) (*UpgradePlanRecord, error) {
	return s.repo.GetPlanByID(ctx, planID)
}

// UpdatePlan 更新升级计划状态。
// UpdatePlan updates the status of an upgrade plan.
func (s *Service) UpdatePlan(ctx context.Context, plan *UpgradePlanRecord) error {
	return s.repo.UpdatePlan(ctx, plan)
}

// CreateTaskFromPlan 基于计划创建持久化升级任务、步骤和节点执行记录。
// CreateTaskFromPlan creates a persistent upgrade task, steps, and node executions from a plan.
func (s *Service) CreateTaskFromPlan(ctx context.Context, planID uint, createdBy uint) (*UpgradeTask, error) {
	plan, err := s.repo.GetPlanByID(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Snapshot.TargetVersion == "" {
		return nil, ErrUpgradePlanSnapshotEmpty
	}

	normalized := normalizePlanSnapshot(plan.Snapshot)
	now := time.Now()
	task := &UpgradeTask{
		ClusterID:      plan.ClusterID,
		PlanID:         plan.ID,
		SourceVersion:  normalized.SourceVersion,
		TargetVersion:  normalized.TargetVersion,
		Status:         ExecutionStatusPending,
		CurrentStep:    normalized.Steps[0].Code,
		RollbackStatus: ExecutionStatusPending,
		CreatedBy:      createdBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	steps := make([]*UpgradeTaskStep, 0, len(normalized.Steps))
	for _, step := range normalized.Steps {
		steps = append(steps, &UpgradeTaskStep{
			Code:      step.Code,
			Sequence:  step.Sequence,
			Status:    ExecutionStatusPending,
			Message:   step.Description,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	nodes := make([]*UpgradeNodeExecution, 0, len(normalized.NodeTargets))
	for _, target := range normalized.NodeTargets {
		nodes = append(nodes, &UpgradeNodeExecution{
			ClusterNodeID:    target.ClusterNodeID,
			HostID:           target.HostID,
			HostName:         target.HostName,
			HostIP:           target.HostIP,
			Role:             target.Role,
			Status:           ExecutionStatusPending,
			CurrentStep:      normalized.Steps[0].Code,
			SourceVersion:    target.SourceVersion,
			TargetVersion:    target.TargetVersion,
			SourceInstallDir: target.SourceInstallDir,
			TargetInstallDir: target.TargetInstallDir,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}

	if err := s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.CreateTask(ctx, task); err != nil {
			return err
		}
		for _, step := range steps {
			step.TaskID = task.ID
		}
		if err := tx.CreateTaskSteps(ctx, steps); err != nil {
			return err
		}
		for _, node := range nodes {
			node.TaskID = task.ID
		}
		if err := tx.CreateNodeExecutions(ctx, nodes); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return s.repo.GetTaskByID(ctx, task.ID)
}

// GetTaskDetail 获取升级任务详情。
// GetTaskDetail gets the detailed view of an upgrade task.
func (s *Service) GetTaskDetail(ctx context.Context, taskID uint) (*UpgradeTask, error) {
	return s.repo.GetTaskByID(ctx, taskID)
}

// ListTasks 获取升级任务摘要列表。
// ListTasks returns the summary list of upgrade tasks.
func (s *Service) ListTasks(ctx context.Context, filter *TaskListFilter) ([]*UpgradeTaskSummary, int64, error) {
	return s.repo.ListTasks(ctx, filter)
}

// ListTaskSteps 获取升级任务步骤列表。
// ListTaskSteps lists steps of an upgrade task.
func (s *Service) ListTaskSteps(ctx context.Context, taskID uint) ([]*UpgradeTaskStep, error) {
	return s.repo.ListTaskSteps(ctx, taskID)
}

// ListNodeExecutions 获取升级任务的节点执行状态。
// ListNodeExecutions lists node execution states of an upgrade task.
func (s *Service) ListNodeExecutions(ctx context.Context, taskID uint) ([]*UpgradeNodeExecution, error) {
	return s.repo.ListNodeExecutions(ctx, taskID)
}

// AppendStepLog 追加升级步骤日志。
// AppendStepLog appends an upgrade step log.
func (s *Service) AppendStepLog(ctx context.Context, log *UpgradeStepLog) error {
	if err := s.repo.CreateStepLog(ctx, log); err != nil {
		return err
	}
	s.publishTaskEvent(newLogAppendedEvent(log))
	return nil
}

// ListStepLogs 分页查询升级步骤日志。
// ListStepLogs lists upgrade step logs with pagination.
func (s *Service) ListStepLogs(ctx context.Context, filter *StepLogFilter) ([]*UpgradeStepLog, int64, error) {
	return s.repo.ListStepLogs(ctx, filter)
}

// UpdateTask 更新升级任务状态。
// UpdateTask updates the status of an upgrade task.
func (s *Service) UpdateTask(ctx context.Context, task *UpgradeTask) error {
	task.UpdatedAt = time.Now()
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	s.publishTaskEvent(newTaskUpdatedEvent(task))
	return nil
}

// UpdateTaskStep 更新升级步骤状态。
// UpdateTaskStep updates the status of an upgrade step.
func (s *Service) UpdateTaskStep(ctx context.Context, step *UpgradeTaskStep) error {
	step.UpdatedAt = time.Now()
	if err := s.repo.UpdateTaskStep(ctx, step); err != nil {
		return err
	}
	s.publishTaskEvent(newStepUpdatedEvent(step))
	return nil
}

// UpdateNodeExecution 更新节点执行状态。
// UpdateNodeExecution updates the status of a node execution.
func (s *Service) UpdateNodeExecution(ctx context.Context, node *UpgradeNodeExecution) error {
	node.UpdatedAt = time.Now()
	if err := s.repo.UpdateNodeExecution(ctx, node); err != nil {
		return err
	}
	s.publishTaskEvent(newNodeUpdatedEvent(node))
	return nil
}

func normalizePlanSnapshot(snapshot UpgradePlanSnapshot) UpgradePlanSnapshot {
	normalized := snapshot
	if normalized.TargetVersion == "" {
		normalized.TargetVersion = seatunnel.DefaultVersion()
	}
	if normalized.GeneratedAt.IsZero() {
		normalized.GeneratedAt = time.Now()
	}
	if normalized.ConfigMergePlan.GeneratedAt.IsZero() {
		normalized.ConfigMergePlan.GeneratedAt = normalized.GeneratedAt
	}
	if len(normalized.Steps) == 0 {
		normalized.Steps = DefaultExecutionSteps()
	}
	if !containsRollbackSteps(normalized.Steps) {
		normalized.Steps = append(normalized.Steps, DefaultRollbackSteps(len(normalized.Steps)+1)...)
	}
	for i := range normalized.NodeTargets {
		if normalized.NodeTargets[i].TargetVersion == "" {
			normalized.NodeTargets[i].TargetVersion = normalized.TargetVersion
		}
		if normalized.NodeTargets[i].TargetInstallDir == "" {
			normalized.NodeTargets[i].TargetInstallDir = seatunnel.DefaultInstallDir(normalized.TargetVersion)
		}
		if normalized.NodeTargets[i].SourceVersion == "" {
			normalized.NodeTargets[i].SourceVersion = normalized.SourceVersion
		}
	}
	return normalized
}

func containsRollbackSteps(steps []PlanStep) bool {
	for _, step := range steps {
		if IsRollbackStep(step.Code) {
			return true
		}
	}
	return false
}
