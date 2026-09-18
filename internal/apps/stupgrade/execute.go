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
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
)

// ClusterOperator 定义升级执行期所需的集群生命周期能力。
// ClusterOperator defines the cluster lifecycle capabilities required during upgrade execution.
type ClusterOperator interface {
	Start(ctx context.Context, clusterID uint) (*clusterapp.OperationResult, error)
	Stop(ctx context.Context, clusterID uint) (*clusterapp.OperationResult, error)
	Update(ctx context.Context, id uint, req *clusterapp.UpdateClusterRequest) (*clusterapp.Cluster, error)
	UpdateNode(ctx context.Context, clusterID uint, nodeID uint, req *clusterapp.UpdateNodeRequest) (*clusterapp.ClusterNode, error)
}

// PackageTransferer 定义安装包分发能力。
// PackageTransferer defines the package distribution capability.
type PackageTransferer interface {
	TransferPackageToAgent(ctx context.Context, agentID string, version string, status *installerapp.InstallationStatus) (remotePath string, err error)
}

// AgentCommandSender 定义升级执行所需的 Agent 命令能力。
// AgentCommandSender defines the Agent command capabilities required by upgrade execution.
type AgentCommandSender interface {
	GetAgentByHostID(hostID uint) (agentID string, connected bool)
	SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (success bool, output string, err error)
}

// ExecutePlan 基于已落盘计划同步执行批次升级，并在失败时自动回滚。
// ExecutePlan executes a batch upgrade synchronously from a persisted plan and automatically rolls back on failure.
func (s *Service) ExecutePlan(ctx context.Context, planID uint, createdBy uint) (*UpgradeTask, error) {
	task, err := s.prepareExecutionTask(ctx, planID, createdBy)
	if err != nil {
		return nil, err
	}
	return s.executeTask(ctx, task.ID)
}

// StartPlanExecution 基于已落盘计划异步启动批次升级。
// StartPlanExecution starts a batch upgrade asynchronously from a persisted plan.
func (s *Service) StartPlanExecution(ctx context.Context, planID uint, createdBy uint) (*UpgradeTask, error) {
	task, err := s.prepareExecutionTask(ctx, planID, createdBy)
	if err != nil {
		return nil, err
	}

	go func(taskID uint) {
		_, _ = s.executeTask(context.Background(), taskID)
	}(task.ID)

	return s.GetTaskDetail(ctx, task.ID)
}

func (s *Service) prepareExecutionTask(ctx context.Context, planID uint, createdBy uint) (*UpgradeTask, error) {
	if err := s.ensureExecutionDependencies(); err != nil {
		return nil, err
	}

	plan, err := s.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan.Status == PlanStatusBlocked || plan.BlockingIssueCount > 0 {
		return nil, ErrUpgradePlanNotReady
	}

	return s.CreateTaskFromPlan(ctx, planID, createdBy)
}

func (s *Service) executeTask(ctx context.Context, taskID uint) (*UpgradeTask, error) {
	task, err := s.GetTaskDetail(ctx, taskID)
	if err != nil {
		return nil, err
	}
	ctx = audit.WithCommandMetadata(ctx, audit.CommandMetadata{
		ExecutionID: task.ExecutionID,
		OwnerUserID: task.CreatedBy,
	})
	plan := normalizePlanSnapshot(task.Plan.Snapshot)
	startedAt := time.Now()
	updated, err := s.repo.UpdateTaskFields(ctx, task.ID, []ExecutionStatus{ExecutionStatusPending, ExecutionStatusReady}, map[string]any{
		"status":       ExecutionStatusRunning,
		"current_step": plan.Steps[0].Code,
		"started_at":   startedAt,
		"updated_at":   startedAt,
	})
	if err != nil {
		return nil, err
	}
	if !updated {
		latest, loadErr := s.GetTaskDetail(ctx, task.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		if latest.Status == ExecutionStatusCancelled {
			_ = s.syncExecutionFromTask(ctx, latest)
			return latest, nil
		}
		return latest, executionapp.ErrConcurrentUpdate
	}
	task.Status = ExecutionStatusRunning
	task.CurrentStep = plan.Steps[0].Code
	task.StartedAt = &startedAt
	task.UpdatedAt = startedAt
	s.publishTaskEvent(newTaskUpdatedEvent(task))
	if err := s.syncExecutionFromTask(ctx, task); err != nil {
		return nil, err
	}

	nodesByKey, nodesByClusterNodeID := buildNodeExecutionIndexes(task.NodeExecutions)
	backupPaths := make(map[string]string)
	if err := s.executePlanSteps(ctx, task, plan, backupPaths, nodesByKey, nodesByClusterNodeID); err != nil {
		rollbackEligible := canRollbackTask(task.Steps)
		task.Status = ExecutionStatusFailed
		task.FailureReason = normalizeUserVisibleText(err.Error())
		if rollbackEligible {
			task.RollbackStatus = ExecutionStatusRollbackRunning
			if updateErr := s.UpdateTask(ctx, task); updateErr != nil {
				return nil, updateErr
			}
			rollbackErr := s.rollbackTask(ctx, task, plan, backupPaths, nodesByKey, nodesByClusterNodeID)
			if rollbackErr != nil {
				task.RollbackStatus = ExecutionStatusRollbackFailed
				task.RollbackReason = normalizeUserVisibleText(rollbackErr.Error())
				_ = s.finalizeNodeExecutionsRollbackFailed(ctx, task, rollbackErr.Error())
				_ = s.recordFailureClosureStep(ctx, task, fmt.Sprintf("upgrade failed at %s: %s; rollback failed: %s / 升级在 %s 失败：%s；回滚失败：%s", task.FailureStep, task.FailureReason, rollbackErr.Error(), task.FailureStep, task.FailureReason, rollbackErr.Error()))
			} else {
				task.RollbackStatus = ExecutionStatusRollbackSucceeded
				_ = s.recordFailureClosureStep(ctx, task, fmt.Sprintf("upgrade failed at %s: %s; rollback succeeded / 升级在 %s 失败：%s；回滚成功", task.FailureStep, task.FailureReason, task.FailureStep, task.FailureReason))
			}
		} else {
			task.RollbackStatus = ExecutionStatusSkipped
			_ = s.finalizeNodeExecutionsFailed(ctx, task, normalizeUserVisibleText(err.Error()))
			_ = s.recordFailureClosureStep(ctx, task, fmt.Sprintf("upgrade failed before rollback point: %s / 升级在回滚点之前失败：%s", err.Error(), err.Error()))
		}
		completedAt := time.Now()
		task.CompletedAt = &completedAt
		_ = s.UpdateTask(ctx, task)
		_ = s.syncExecutionFromTask(ctx, task)
		return s.GetTaskDetail(ctx, task.ID)
	}

	if err := s.finalizeNodeExecutionsSuccess(ctx, task); err != nil {
		return nil, err
	}
	completedAt := time.Now()
	task.Status = ExecutionStatusSucceeded
	task.RollbackStatus = ExecutionStatusPending
	task.CurrentStep = StepCodeComplete
	task.CompletedAt = &completedAt
	if err := s.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	if err := s.syncExecutionFromTask(ctx, task); err != nil {
		return nil, err
	}
	return s.GetTaskDetail(ctx, task.ID)
}

func (s *Service) executePlanSteps(ctx context.Context, task *UpgradeTask, plan UpgradePlanSnapshot, backupPaths map[string]string, nodesByKey map[string]*UpgradeNodeExecution, nodesByClusterNodeID map[uint]*UpgradeNodeExecution) error {
	connectorKeepFiles := collectConnectorFileNames(plan.ConnectorManifest.Connectors)
	libraryKeepFiles := collectLibraryFileNames(plan.ConnectorManifest.Libraries)
	pluginKeepFiles := collectPluginDependencyPaths(plan.ConnectorManifest.PluginDeps)

	for i := range task.Steps {
		step := &task.Steps[i]
		if IsRollbackStep(step.Code) {
			continue
		}
		if err := s.startStep(ctx, task, step); err != nil {
			return err
		}

		var (
			stepErr        error
			successMessage string
		)
		switch step.Code {
		case StepCodePrecheckPackage:
			stepErr = s.markPlanCheckStep(plan.PackageManifest.Checksum != "", "package checksum is missing / 安装包缺少 checksum")
			successMessage = fmt.Sprintf("package %s passed checksum gate / 安装包 %s 通过 checksum 门禁", plan.TargetVersion, plan.TargetVersion)
		case StepCodePrecheckConnector:
			stepErr = s.markPlanCheckStep(plan.ConnectorManifest.Version != "", "connector manifest is missing / connector 清单缺失")
			successMessage = fmt.Sprintf(
				"plugin manifest ready with %d connectors, %d libs and %d isolated dependencies / 插件清单已就绪，包含 %d 个 connector、%d 个 lib、%d 个隔离依赖",
				len(plan.ConnectorManifest.Connectors),
				len(plan.ConnectorManifest.Libraries),
				len(plan.ConnectorManifest.PluginDeps),
				len(plan.ConnectorManifest.Connectors),
				len(plan.ConnectorManifest.Libraries),
				len(plan.ConnectorManifest.PluginDeps),
			)
		case StepCodeBackup:
			stepErr = s.executeBackupStep(ctx, task, step, plan.NodeTargets, backupPaths, nodesByKey)
			successMessage = fmt.Sprintf("backed up %d node targets / 已完成 %d 个节点目标的备份", len(plan.NodeTargets), len(plan.NodeTargets))
		case StepCodeDistributePackage:
			stepErr = s.executeDistributePackageStep(ctx, task, step, plan, nodesByKey)
			successMessage = fmt.Sprintf("distributed package %s to %d node targets / 已将安装包 %s 分发到 %d 个节点目标", plan.PackageManifest.FileName, len(plan.NodeTargets), plan.PackageManifest.FileName, len(plan.NodeTargets))
		case StepCodeSyncLib:
			stepErr = s.executeSyncLibStep(ctx, task, step, plan.NodeTargets, libraryKeepFiles, nodesByKey)
			successMessage = fmt.Sprintf("synchronized %d lib artifacts on %d node targets / 已在 %d 个节点目标上同步 %d 个 lib 工件", len(libraryKeepFiles), len(plan.NodeTargets), len(plan.NodeTargets), len(libraryKeepFiles))
		case StepCodeSyncConnectors:
			stepErr = s.executeSyncConnectorsStep(ctx, task, step, plan, connectorKeepFiles, nodesByKey)
			successMessage = fmt.Sprintf("synchronized %d connectors on %d node targets / 已在 %d 个节点目标上同步 %d 个 connector", len(plan.ConnectorManifest.Connectors), len(plan.NodeTargets), len(plan.NodeTargets), len(plan.ConnectorManifest.Connectors))
		case StepCodeSyncPlugins:
			stepErr = s.executeSyncPluginsStep(ctx, task, step, plan.NodeTargets, pluginKeepFiles, nodesByKey)
			successMessage = fmt.Sprintf("synchronized %d isolated dependencies on %d node targets / 已在 %d 个节点目标上同步 %d 个隔离依赖", len(pluginKeepFiles), len(plan.NodeTargets), len(plan.NodeTargets), len(pluginKeepFiles))
		case StepCodeMergeConfig:
			stepErr = s.executeMergeConfigStep(ctx, task, step, plan, nodesByKey)
			successMessage = fmt.Sprintf("applied %d merged config files / 已应用 %d 个合并后的配置文件", len(plan.ConfigMergePlan.Files), len(plan.ConfigMergePlan.Files))
		case StepCodeStopCluster:
			stepErr = s.executeClusterLifecycleStep(ctx, task, step, clusterapp.OperationStop, nodesByClusterNodeID, ExecutionStatusRunning)
			successMessage = "cluster stopped and upgrade window opened / 集群已停止，切换窗口已打开"
		case StepCodeSwitchVersion:
			stepErr = s.executeSwitchVersionStep(ctx, task, step, plan, nodesByKey)
			successMessage = fmt.Sprintf("switched cluster metadata to target version %s / 已将集群元数据切换到目标版本 %s", plan.TargetVersion, plan.TargetVersion)
		case StepCodeStartCluster:
			stepErr = s.executeClusterLifecycleStep(ctx, task, step, clusterapp.OperationStart, nodesByClusterNodeID, ExecutionStatusRunning)
			successMessage = "cluster started on target version / 目标版本集群已启动"
		case StepCodeHealthCheck:
			stepErr = s.executeHealthCheckStep(ctx, task, step, plan.NodeTargets, nodesByKey)
			successMessage = fmt.Sprintf("health checks passed on %d node targets / %d 个节点目标健康检查通过", len(plan.NodeTargets), len(plan.NodeTargets))
		case StepCodeSmokeTest:
			successMessage, stepErr = s.executeSmokeTestStep(ctx, task, step, plan.NodeTargets, nodesByKey)
		case StepCodeComplete:
			successMessage = "upgrade workflow completed / 升级工作流执行完成"
		default:
			stepErr = fmt.Errorf("unsupported upgrade step: %s", step.Code)
		}
		if stepErr != nil {
			if err := s.failStep(ctx, task, step, stepErr); err != nil {
				return err
			}
			return stepErr
		}
		if err := s.finishStep(ctx, task, step, successMessage); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) rollbackTask(ctx context.Context, task *UpgradeTask, plan UpgradePlanSnapshot, backupPaths map[string]string, nodesByKey map[string]*UpgradeNodeExecution, nodesByClusterNodeID map[uint]*UpgradeNodeExecution) error {
	rollbackSteps := make(map[StepCode]*UpgradeTaskStep)
	for i := range task.Steps {
		step := &task.Steps[i]
		if IsRollbackStep(step.Code) {
			rollbackSteps[step.Code] = step
		}
	}

	prepareStep := rollbackSteps[StepCodeRollbackPrepare]
	restoreStep := rollbackSteps[StepCodeRollbackRestore]
	restartStep := rollbackSteps[StepCodeRollbackRestart]
	verifyStep := rollbackSteps[StepCodeRollbackVerify]
	if prepareStep == nil || restoreStep == nil || restartStep == nil || verifyStep == nil {
		return fmt.Errorf("回滚步骤未完整初始化")
	}

	if err := s.startStep(ctx, task, prepareStep); err != nil {
		return err
	}
	if err := s.appendTaskLog(ctx, task.ID, uintPtr(prepareStep.ID), nil, prepareStep.Code, LogLevelWarn, LogEventTypeRollback, fmt.Sprintf("rollback triggered because %s failed: %s / 因 %s 失败触发回滚：%s", task.FailureStep, task.FailureReason, task.FailureStep, task.FailureReason), ""); err != nil {
		return err
	}
	if err := s.executeClusterLifecycleStep(ctx, task, prepareStep, clusterapp.OperationStop, nodesByClusterNodeID, ExecutionStatusRollbackRunning); err != nil {
		if failErr := s.failStep(ctx, task, prepareStep, err); failErr != nil {
			return failErr
		}
		return err
	}
	if err := s.finishStep(ctx, task, prepareStep, "rollback preparation completed / 回滚准备完成"); err != nil {
		return err
	}

	if err := s.startStep(ctx, task, restoreStep); err != nil {
		return err
	}
	for _, target := range plan.NodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		backupDir := backupPaths[nodeExecutionKey(target.HostID, target.Role)]
		commandSummary := fmt.Sprintf("upgrade restore source_install_dir=%s backup_dir=%s", target.SourceInstallDir, backupDir)
		restoreMessage := fmt.Sprintf("restoring snapshot from %s / 正在从 %s 恢复快照", backupDir, backupDir)
		if strings.TrimSpace(backupDir) == "" {
			restoreMessage = fmt.Sprintf("source dir %s was preserved, restoring metadata only / 源目录 %s 未被修改，本次仅恢复元数据", target.SourceInstallDir, target.SourceInstallDir)
		}
		if err := s.beginNodeStep(ctx, restoreStep, node, ExecutionStatusRollbackRunning, restoreMessage, commandSummary); err != nil {
			return err
		}
		if strings.TrimSpace(backupDir) == "" {
			if err := s.finishNodeStep(ctx, restoreStep, node, ExecutionStatusRollbackRunning, fmt.Sprintf("source dir %s already intact / 源目录 %s 保持完好", target.SourceInstallDir, target.SourceInstallDir), commandSummary); err != nil {
				return err
			}
			continue
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "restore_snapshot",
			"backup_dir":  backupDir,
			"restore_dir": target.SourceInstallDir,
		}); err != nil {
			_ = s.failNodeStep(ctx, restoreStep, node, ExecutionStatusRollbackFailed, err, commandSummary)
			if failErr := s.failStep(ctx, task, restoreStep, err); failErr != nil {
				return failErr
			}
			return err
		}
		if err := s.finishNodeStep(ctx, restoreStep, node, ExecutionStatusRollbackRunning, fmt.Sprintf("snapshot restored to %s / 已恢复到 %s", target.SourceInstallDir, target.SourceInstallDir), commandSummary); err != nil {
			return err
		}
	}
	if err := s.restoreClusterMetadata(ctx, task.ClusterID, plan.NodeTargets, plan.SourceVersion); err != nil {
		if failErr := s.failStep(ctx, task, restoreStep, err); failErr != nil {
			return failErr
		}
		return err
	}
	if err := s.finishStep(ctx, task, restoreStep, "rollback restore completed / 回滚恢复完成"); err != nil {
		return err
	}

	if err := s.startStep(ctx, task, restartStep); err != nil {
		return err
	}
	if err := s.executeClusterLifecycleStep(ctx, task, restartStep, clusterapp.OperationStart, nodesByClusterNodeID, ExecutionStatusRollbackRunning); err != nil {
		if failErr := s.failStep(ctx, task, restartStep, err); failErr != nil {
			return failErr
		}
		return err
	}
	if err := s.finishStep(ctx, task, restartStep, "rollback restart completed / 回滚重启完成"); err != nil {
		return err
	}

	if err := s.startStep(ctx, task, verifyStep); err != nil {
		return err
	}
	for _, target := range plan.NodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		commandSummary := fmt.Sprintf("upgrade verify_cluster_health install_dir=%s", target.SourceInstallDir)
		if err := s.beginNodeStep(ctx, verifyStep, node, ExecutionStatusRollbackRunning, fmt.Sprintf("verifying restored install dir %s / 正在校验恢复后的目录 %s", target.SourceInstallDir, target.SourceInstallDir), commandSummary); err != nil {
			return err
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "verify_cluster_health",
			"install_dir": target.SourceInstallDir,
		}); err != nil {
			_ = s.failNodeStep(ctx, verifyStep, node, ExecutionStatusRollbackFailed, err, commandSummary)
			if failErr := s.failStep(ctx, task, verifyStep, err); failErr != nil {
				return failErr
			}
			return err
		}
		if err := s.finishNodeStep(ctx, verifyStep, node, ExecutionStatusRollbackSucceeded, fmt.Sprintf("rollback health check passed for %s / %s 回滚健康检查通过", target.SourceInstallDir, target.SourceInstallDir), commandSummary); err != nil {
			return err
		}
	}
	if err := s.finishStep(ctx, task, verifyStep, "rollback verification completed / 回滚校验完成"); err != nil {
		return err
	}

	return nil
}

func (s *Service) recordFailureClosureStep(ctx context.Context, task *UpgradeTask, message string) error {
	var failedStep *UpgradeTaskStep
	for i := range task.Steps {
		if task.Steps[i].Code == StepCodeFailed {
			failedStep = &task.Steps[i]
			break
		}
	}
	if failedStep == nil {
		return s.appendTaskLog(ctx, task.ID, nil, nil, StepCodeFailed, LogLevelError, LogEventTypeFailed, message, "")
	}
	if failedStep.Status == ExecutionStatusSucceeded {
		return nil
	}
	if err := s.startStep(ctx, task, failedStep); err != nil {
		return err
	}
	return s.finishStep(ctx, task, failedStep, message)
}

func (s *Service) ensureExecutionDependencies() error {
	if s.clusterOperator == nil || s.packageTransferer == nil || s.agentCommandSender == nil {
		return fmt.Errorf("st upgrade execution dependencies are not fully configured")
	}
	return nil
}

func (s *Service) markPlanCheckStep(ready bool, message string) error {
	if !ready {
		return fmt.Errorf("%s", message)
	}
	return nil
}

func (s *Service) executeBackupStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, nodeTargets []NodeTarget, backupPaths map[string]string, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range nodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		nodeKey := nodeExecutionKey(target.HostID, target.Role)
		if sameInstallDirForUpgrade(target.SourceInstallDir, target.TargetInstallDir) {
			backupDir := fmt.Sprintf("%s.stupgrade-%d-backup", strings.TrimSpace(target.SourceInstallDir), task.ID)
			commandSummary := fmt.Sprintf("upgrade backup_install_dir install_dir=%s backup_dir=%s", target.SourceInstallDir, backupDir)
			if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("source and target install dir are the same, backing up %s / 源目录与目标目录相同，正在备份 %s", target.SourceInstallDir, target.SourceInstallDir), commandSummary); err != nil {
				return err
			}
			if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
				"sub_command": "backup_install_dir",
				"install_dir": target.SourceInstallDir,
				"backup_dir":  backupDir,
			}); err != nil {
				_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
				return err
			}
			backupPaths[nodeKey] = backupDir
			if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("backup created at %s / 备份已创建到 %s", backupDir, backupDir), commandSummary); err != nil {
				return err
			}
			continue
		}

		commandSummary := fmt.Sprintf("upgrade backup skipped source_install_dir=%s target_install_dir=%s", target.SourceInstallDir, target.TargetInstallDir)
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("double-directory upgrade keeps %s intact, skipping extra snapshot / 双目录升级会保留 %s 原目录，本次跳过额外快照", target.SourceInstallDir, target.SourceInstallDir), commandSummary); err != nil {
			return err
		}
		backupPaths[nodeKey] = ""
		message := fmt.Sprintf("no physical backup required because source dir %s and target dir %s are different / 源目录 %s 与目标目录 %s 不重合，无需额外物理备份", target.SourceInstallDir, target.TargetInstallDir, target.SourceInstallDir, target.TargetInstallDir)
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, message, commandSummary); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeDistributePackageStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, plan UpgradePlanSnapshot, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range plan.NodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		agentID, connected := s.agentCommandSender.GetAgentByHostID(target.HostID)
		if !connected || agentID == "" {
			err := fmt.Errorf("主机 %d 的 Agent 未连接", target.HostID)
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, "")
			return err
		}
		transferSummary := fmt.Sprintf("transfer package version=%s agent_id=%s", plan.TargetVersion, agentID)
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("transferring package %s / 正在传输安装包 %s", plan.PackageManifest.FileName, plan.PackageManifest.FileName), transferSummary); err != nil {
			return err
		}
		remotePath, err := s.packageTransferer.TransferPackageToAgent(ctx, agentID, plan.TargetVersion, nil)
		if err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, transferSummary)
			return err
		}
		extractSummary := fmt.Sprintf("upgrade extract_package_to_dir package_path=%s target_dir=%s checksum=%s", remotePath, target.TargetInstallDir, plan.PackageManifest.Checksum)
		if err := s.appendNodeLog(ctx, step, node, LogLevelInfo, LogEventTypeProgress, fmt.Sprintf("package transferred to %s / 安装包已传输到 %s", remotePath, remotePath), extractSummary); err != nil {
			return err
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command":       "extract_package_to_dir",
			"package_path":      remotePath,
			"target_dir":        target.TargetInstallDir,
			"expected_checksum": plan.PackageManifest.Checksum,
		}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, extractSummary)
			return err
		}
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("package extracted to %s / 安装包已解压到 %s", target.TargetInstallDir, target.TargetInstallDir), extractSummary); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeSyncLibStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, nodeTargets []NodeTarget, keepFiles []string, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range nodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		commandSummary := fmt.Sprintf("upgrade sync_lib_manifest install_dir=%s keep_files=%s", target.TargetInstallDir, strings.Join(keepFiles, ","))
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("synchronizing lib manifest in %s / 正在同步 %s 的 lib 清单", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "sync_lib_manifest",
			"install_dir": target.TargetInstallDir,
			"keep_files":  strings.Join(keepFiles, ","),
		}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
			return err
		}
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("lib manifest synchronized for %s / %s 的 lib 清单已同步", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeSyncConnectorsStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, plan UpgradePlanSnapshot, keepFiles []string, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range plan.NodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		agentID, connected := s.agentCommandSender.GetAgentByHostID(target.HostID)
		if !connected || agentID == "" {
			err := fmt.Errorf("主机 %d 的 Agent 未连接", target.HostID)
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, "")
			return err
		}
		manifestSummary := fmt.Sprintf("upgrade sync_connectors_manifest install_dir=%s keep_files=%s", target.TargetInstallDir, strings.Join(keepFiles, ","))
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("synchronizing connectors in %s / 正在同步 %s 的 connector", target.TargetInstallDir, target.TargetInstallDir), manifestSummary); err != nil {
			return err
		}
		for _, connector := range plan.ConnectorManifest.Connectors {
			transferSummary := fmt.Sprintf("transfer plugin plugin=%s version=%s install_dir=%s", connector.PluginName, connector.Version, target.TargetInstallDir)
			if err := s.appendNodeLog(ctx, step, node, LogLevelInfo, LogEventTypeProgress, fmt.Sprintf("transferring connector %s / 正在传输 connector %s", connector.PluginName, connector.PluginName), transferSummary); err != nil {
				return err
			}
			if err := s.pluginProvider.TransferPluginToAgent(ctx, agentID, connector.PluginName, connector.Version, target.TargetInstallDir, nil); err != nil {
				_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, transferSummary)
				return err
			}
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "sync_connectors_manifest",
			"install_dir": target.TargetInstallDir,
			"keep_files":  strings.Join(keepFiles, ","),
		}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, manifestSummary)
			return err
		}
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("connector manifest synchronized for %s / %s 的 connector 清单已同步", target.TargetInstallDir, target.TargetInstallDir), manifestSummary); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeSyncPluginsStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, nodeTargets []NodeTarget, keepFiles []string, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range nodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		commandSummary := fmt.Sprintf("upgrade sync_plugins_manifest install_dir=%s keep_files=%s", target.TargetInstallDir, strings.Join(keepFiles, ","))
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("synchronizing isolated dependencies in %s / 正在同步 %s 的隔离依赖", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "sync_plugins_manifest",
			"install_dir": target.TargetInstallDir,
			"keep_files":  strings.Join(keepFiles, ","),
		}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
			return err
		}
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("isolated dependency manifest synchronized for %s / %s 的隔离依赖清单已同步", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeMergeConfigStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, plan UpgradePlanSnapshot, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range plan.NodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		for _, file := range plan.ConfigMergePlan.Files {
			commandSummary := fmt.Sprintf("upgrade apply_merged_config install_dir=%s config_type=%s", target.TargetInstallDir, file.ConfigType)
			if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("applying merged config %s / 正在应用合并配置 %s", file.ConfigType, file.ConfigType), commandSummary); err != nil {
				return err
			}
			if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
				"sub_command": "apply_merged_config",
				"install_dir": target.TargetInstallDir,
				"config_type": file.ConfigType,
				"content":     file.MergedContent,
				"backup":      "true",
			}); err != nil {
				_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
				return err
			}
			if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("merged config %s applied / 合并配置 %s 已应用", file.ConfigType, file.ConfigType), commandSummary); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) executeSwitchVersionStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, plan UpgradePlanSnapshot, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range plan.NodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		commandSummary := fmt.Sprintf("upgrade switch_install_dir target_dir=%s", target.TargetInstallDir)
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("switching install dir to %s / 正在切换安装目录到 %s", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "switch_install_dir",
			"target_dir":  target.TargetInstallDir,
		}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
			return err
		}
		installDir := target.TargetInstallDir
		if _, err := s.clusterOperator.UpdateNode(ctx, task.ClusterID, target.ClusterNodeID, &clusterapp.UpdateNodeRequest{InstallDir: &installDir}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
			return err
		}
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("install dir switched to %s / 安装目录已切换到 %s", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
	}
	clusterInstallDir := ""
	if len(plan.NodeTargets) > 0 {
		clusterInstallDir = plan.NodeTargets[0].TargetInstallDir
	}
	if _, err := s.clusterOperator.Update(ctx, task.ClusterID, &clusterapp.UpdateClusterRequest{Version: &plan.TargetVersion, InstallDir: &clusterInstallDir}); err != nil {
		return err
	}

	if s.pluginProvider == nil {
		return nil
	}

	installedPlugins, err := s.pluginProvider.ListInstalledPlugins(ctx, task.ClusterID)
	if err != nil {
		return err
	}
	for _, plugin := range installedPlugins {
		if strings.TrimSpace(plugin.PluginName) == "" {
			continue
		}
		if err := s.pluginProvider.RecordInstalledPlugin(ctx, task.ClusterID, plugin.PluginName, plan.TargetVersion); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) executeHealthCheckStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, nodeTargets []NodeTarget, nodesByKey map[string]*UpgradeNodeExecution) error {
	for _, target := range nodeTargets {
		node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
		commandSummary := fmt.Sprintf("upgrade verify_cluster_health install_dir=%s", target.TargetInstallDir)
		if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("verifying health of %s / 正在校验 %s 的健康状态", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
		if _, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
			"sub_command": "verify_cluster_health",
			"install_dir": target.TargetInstallDir,
		}); err != nil {
			_ = s.failNodeStep(ctx, step, node, ExecutionStatusFailed, err, commandSummary)
			return err
		}
		if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, fmt.Sprintf("health verified for %s / %s 健康状态校验通过", target.TargetInstallDir, target.TargetInstallDir), commandSummary); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeSmokeTestStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, nodeTargets []NodeTarget, nodesByKey map[string]*UpgradeNodeExecution) (string, error) {
	target, ok := selectSmokeTestTarget(nodeTargets)
	if !ok {
		message := "未找到可执行升级后可用性验证的节点，已跳过该校验（不阻塞升级）"
		if err := s.appendTaskLog(ctx, task.ID, uintPtr(step.ID), nil, step.Code, LogLevelWarn, LogEventTypeNote, message, ""); err != nil {
			return "", err
		}
		return message, nil
	}

	node := nodesByKey[nodeExecutionKey(target.HostID, target.Role)]
	commandSummary := fmt.Sprintf("upgrade run_smoke_test_template install_dir=%s", target.TargetInstallDir)
	startMessage := fmt.Sprintf("running post-upgrade smoke test in %s / 正在对 %s 执行升级后可用性验证", target.TargetInstallDir, target.TargetInstallDir)
	if err := s.beginNodeStep(ctx, step, node, ExecutionStatusRunning, startMessage, commandSummary); err != nil {
		return "", err
	}

	output, err := s.runManagedCommand(ctx, target.HostID, map[string]string{
		"sub_command": "run_smoke_test_template",
		"install_dir": target.TargetInstallDir,
	})
	if err != nil {
		nodeMessage := fmt.Sprintf("smoke test reported warning on %s / %s 的可用性验证出现告警", target.HostName, target.HostName)
		if finishErr := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, nodeMessage, commandSummary); finishErr != nil {
			return "", finishErr
		}
		warningDetail := fmt.Sprintf("post-upgrade smoke test warned on %s: %s / %s 的升级后可用性验证告警：%s", target.HostName, err.Error(), target.HostName, err.Error())
		if logErr := s.appendNodeLog(ctx, step, node, LogLevelWarn, LogEventTypeNote, warningDetail, commandSummary); logErr != nil {
			return "", logErr
		}
		if logErr := s.appendTaskLog(ctx, task.ID, uintPtr(step.ID), nil, step.Code, LogLevelWarn, LogEventTypeNote, warningDetail, commandSummary); logErr != nil {
			return "", logErr
		}
		return "已完成升级后可用性验证（有告警，不阻塞升级）", nil
	}

	successMessage := fmt.Sprintf("smoke test passed on %s / %s 的可用性验证通过", target.HostName, target.HostName)
	if err := s.finishNodeStep(ctx, step, node, ExecutionStatusRunning, successMessage, commandSummary); err != nil {
		return "", err
	}
	if strings.TrimSpace(output) != "" {
		outputMessage := fmt.Sprintf("post-upgrade smoke test output: %s / 升级后可用性验证输出：%s", output, output)
		if err := s.appendNodeLog(ctx, step, node, LogLevelInfo, LogEventTypeNote, outputMessage, commandSummary); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("已完成升级后可用性验证，执行节点：%s", target.HostName), nil
}

func (s *Service) executeClusterLifecycleStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, operation clusterapp.OperationType, nodesByClusterNodeID map[uint]*UpgradeNodeExecution, nodeSuccessStatus ExecutionStatus) error {
	commandSummary := fmt.Sprintf("cluster.%s cluster_id=%d", operation, task.ClusterID)
	for _, node := range task.NodeExecutions {
		trackedNode := nodesByClusterNodeID[node.ClusterNodeID]
		if trackedNode == nil {
			continue
		}
		message := fmt.Sprintf("cluster %s started on host %s / 主机 %s 开始执行集群%s", operation, trackedNode.HostName, trackedNode.HostName, operation)
		if err := s.beginNodeStep(ctx, step, trackedNode, nodeSuccessStatus, message, commandSummary); err != nil {
			return err
		}
	}

	var (
		result *clusterapp.OperationResult
		err    error
	)
	switch operation {
	case clusterapp.OperationStop:
		result, err = s.clusterOperator.Stop(ctx, task.ClusterID)
	case clusterapp.OperationStart:
		result, err = s.clusterOperator.Start(ctx, task.ClusterID)
	default:
		return fmt.Errorf("unsupported cluster operation: %s", operation)
	}
	if err != nil {
		for _, node := range nodesByClusterNodeID {
			_ = s.failNodeStep(ctx, step, node, deriveFailureStatus(nodeSuccessStatus), err, commandSummary)
		}
		return err
	}
	if result == nil {
		return fmt.Errorf("cluster %s returned empty result", operation)
	}

	var firstErr error
	for _, nodeResult := range result.NodeResults {
		node := nodesByClusterNodeID[nodeResult.NodeID]
		if node == nil {
			continue
		}
		if nodeResult.Success {
			if err := s.finishNodeStep(ctx, step, node, nodeSuccessStatus, nodeResult.Message, commandSummary); err != nil {
				return err
			}
			continue
		}
		err := fmt.Errorf("%s", nodeResult.Message)
		if firstErr == nil {
			firstErr = err
		}
		_ = s.failNodeStep(ctx, step, node, deriveFailureStatus(nodeSuccessStatus), err, commandSummary)
	}
	if !result.Success {
		if firstErr != nil {
			return firstErr
		}
		return fmt.Errorf("%s", result.Message)
	}
	return nil
}

func (s *Service) finalizeNodeExecutionsSuccess(ctx context.Context, task *UpgradeTask) error {
	for i := range task.NodeExecutions {
		node := &task.NodeExecutions[i]
		node.Status = ExecutionStatusSucceeded
		node.CurrentStep = StepCodeComplete
		node.Message = normalizeUserVisibleText(fmt.Sprintf("upgrade to %s completed / 已完成升级到 %s", task.TargetVersion, task.TargetVersion))
		node.Error = ""
		now := time.Now()
		node.CompletedAt = &now
		if err := s.UpdateNodeExecution(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) finalizeNodeExecutionsFailed(ctx context.Context, task *UpgradeTask, reason string) error {
	for i := range task.NodeExecutions {
		node := &task.NodeExecutions[i]
		if node.Status == ExecutionStatusRollbackSucceeded || node.Status == ExecutionStatusRollbackFailed {
			continue
		}
		node.Status = ExecutionStatusFailed
		node.CurrentStep = task.FailureStep
		node.Message = normalizeUserVisibleText(fmt.Sprintf("upgrade failed at %s / 升级在 %s 失败", task.FailureStep, task.FailureStep))
		node.Error = normalizeUserVisibleText(reason)
		now := time.Now()
		node.CompletedAt = &now
		if err := s.UpdateNodeExecution(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) finalizeNodeExecutionsRollbackFailed(ctx context.Context, task *UpgradeTask, reason string) error {
	for i := range task.NodeExecutions {
		node := &task.NodeExecutions[i]
		if node.Status == ExecutionStatusRollbackSucceeded {
			continue
		}
		node.Status = ExecutionStatusRollbackFailed
		node.CurrentStep = task.CurrentStep
		node.Message = normalizeUserVisibleText("rollback failed / 回滚失败")
		node.Error = normalizeUserVisibleText(reason)
		now := time.Now()
		node.CompletedAt = &now
		if err := s.UpdateNodeExecution(ctx, node); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) restoreClusterMetadata(ctx context.Context, clusterID uint, nodeTargets []NodeTarget, sourceVersion string) error {
	for _, target := range nodeTargets {
		installDir := target.SourceInstallDir
		if _, err := s.clusterOperator.UpdateNode(ctx, clusterID, target.ClusterNodeID, &clusterapp.UpdateNodeRequest{InstallDir: &installDir}); err != nil {
			return err
		}
	}
	clusterInstallDir := ""
	if len(nodeTargets) > 0 {
		clusterInstallDir = nodeTargets[0].SourceInstallDir
	}
	_, err := s.clusterOperator.Update(ctx, clusterID, &clusterapp.UpdateClusterRequest{Version: &sourceVersion, InstallDir: &clusterInstallDir})
	return err
}

func (s *Service) runManagedCommand(ctx context.Context, hostID uint, params map[string]string) (string, error) {
	agentID, connected := s.agentCommandSender.GetAgentByHostID(hostID)
	if !connected || agentID == "" {
		return "", fmt.Errorf("主机 %d 的 Agent 未连接", hostID)
	}
	success, output, err := s.agentCommandSender.SendCommand(ctx, agentID, "upgrade", params)
	if err != nil {
		return "", err
	}
	if !success {
		return "", fmt.Errorf("%s", output)
	}
	return output, nil
}

func sameInstallDirForUpgrade(sourceDir, targetDir string) bool {
	source := strings.TrimSpace(sourceDir)
	target := strings.TrimSpace(targetDir)
	if source == "" || target == "" {
		return false
	}
	return filepath.Clean(source) == filepath.Clean(target)
}

func (s *Service) startStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep) error {
	now := time.Now()
	step.Status = ExecutionStatusRunning
	step.StartedAt = &now
	step.CompletedAt = nil
	step.Error = ""
	step.Message = normalizeUserVisibleText(fmt.Sprintf("%s is running / %s 执行中", step.Code, step.Code))
	if err := s.UpdateTaskStep(ctx, step); err != nil {
		return err
	}
	task.CurrentStep = step.Code
	switch {
	case step.Code == StepCodeFailed:
		task.Status = ExecutionStatusFailed
	case IsRollbackStep(step.Code):
		task.RollbackStatus = ExecutionStatusRollbackRunning
	default:
		task.Status = ExecutionStatusRunning
	}
	if task.StartedAt == nil {
		task.StartedAt = &now
	}
	if err := s.UpdateTask(ctx, task); err != nil {
		return err
	}
	return s.appendTaskLog(ctx, task.ID, uintPtr(step.ID), nil, step.Code, LogLevelInfo, LogEventTypeStarted, normalizeUserVisibleText(fmt.Sprintf("%s started / %s 开始执行", step.Code, step.Code)), "")
}

func (s *Service) finishStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, message string) error {
	now := time.Now()
	step.Status = ExecutionStatusSucceeded
	step.CompletedAt = &now
	step.Message = normalizeUserVisibleText(message)
	if err := s.UpdateTaskStep(ctx, step); err != nil {
		return err
	}
	return s.appendTaskLog(ctx, task.ID, uintPtr(step.ID), nil, step.Code, LogLevelInfo, LogEventTypeSuccess, normalizeUserVisibleText(message), "")
}

func (s *Service) failStep(ctx context.Context, task *UpgradeTask, step *UpgradeTaskStep, err error) error {
	now := time.Now()
	step.Status = ExecutionStatusFailed
	step.CompletedAt = &now
	step.Error = normalizeUserVisibleText(err.Error())
	step.Message = normalizeUserVisibleText(fmt.Sprintf("%s failed / %s 执行失败", step.Code, step.Code))
	task.CurrentStep = step.Code
	task.FailureStep = step.Code
	if IsRollbackStep(step.Code) {
		task.RollbackStatus = ExecutionStatusRollbackFailed
		task.RollbackReason = normalizeUserVisibleText(err.Error())
	} else {
		task.Status = ExecutionStatusFailed
		task.FailureReason = normalizeUserVisibleText(err.Error())
	}
	if updateErr := s.UpdateTaskStep(ctx, step); updateErr != nil {
		return updateErr
	}
	if updateErr := s.UpdateTask(ctx, task); updateErr != nil {
		return updateErr
	}
	return s.appendTaskLog(ctx, task.ID, uintPtr(step.ID), nil, step.Code, LogLevelError, LogEventTypeFailed, normalizeUserVisibleText(err.Error()), "")
}

func (s *Service) beginNodeStep(ctx context.Context, step *UpgradeTaskStep, node *UpgradeNodeExecution, status ExecutionStatus, message, commandSummary string) error {
	if node == nil {
		return nil
	}
	now := time.Now()
	node.TaskStepID = uintPtr(step.ID)
	node.CurrentStep = step.Code
	node.Status = status
	node.Message = normalizeUserVisibleText(message)
	node.Error = ""
	if node.StartedAt == nil {
		node.StartedAt = &now
	}
	node.CompletedAt = nil
	if err := s.UpdateNodeExecution(ctx, node); err != nil {
		return err
	}
	return s.appendNodeLog(ctx, step, node, LogLevelInfo, LogEventTypeStarted, normalizeUserVisibleText(message), commandSummary)
}

func (s *Service) finishNodeStep(ctx context.Context, step *UpgradeTaskStep, node *UpgradeNodeExecution, status ExecutionStatus, message, commandSummary string) error {
	if node == nil {
		return nil
	}
	node.TaskStepID = uintPtr(step.ID)
	node.CurrentStep = step.Code
	node.Status = status
	node.Message = normalizeUserVisibleText(message)
	node.Error = ""
	if isTerminalNodeStatus(status) {
		now := time.Now()
		node.CompletedAt = &now
	} else {
		node.CompletedAt = nil
	}
	if err := s.UpdateNodeExecution(ctx, node); err != nil {
		return err
	}
	return s.appendNodeLog(ctx, step, node, LogLevelInfo, LogEventTypeSuccess, normalizeUserVisibleText(message), commandSummary)
}

func (s *Service) failNodeStep(ctx context.Context, step *UpgradeTaskStep, node *UpgradeNodeExecution, status ExecutionStatus, err error, commandSummary string) error {
	if node == nil {
		return nil
	}
	now := time.Now()
	node.TaskStepID = uintPtr(step.ID)
	node.CurrentStep = step.Code
	node.Status = status
	node.Message = normalizeUserVisibleText(fmt.Sprintf("%s failed on %s / %s 在 %s 上失败", step.Code, node.HostName, step.Code, node.HostName))
	node.Error = normalizeUserVisibleText(err.Error())
	node.CompletedAt = &now
	if updateErr := s.UpdateNodeExecution(ctx, node); updateErr != nil {
		return updateErr
	}
	return s.appendNodeLog(ctx, step, node, LogLevelError, LogEventTypeFailed, normalizeUserVisibleText(err.Error()), commandSummary)
}

func (s *Service) appendTaskLog(ctx context.Context, taskID uint, stepID *uint, nodeID *uint, stepCode StepCode, level LogLevel, eventType LogEventType, message, commandSummary string) error {
	return s.appendStructuredLog(ctx, taskID, stepID, nodeID, stepCode, level, eventType, message, commandSummary, nil)
}

func (s *Service) appendNodeLog(ctx context.Context, step *UpgradeTaskStep, node *UpgradeNodeExecution, level LogLevel, eventType LogEventType, message, commandSummary string) error {
	metadata := LogMetadata{
		"host_id":   node.HostID,
		"host_name": node.HostName,
		"host_ip":   node.HostIP,
		"role":      node.Role,
	}
	return s.appendStructuredLog(ctx, node.TaskID, uintPtr(step.ID), uintPtr(node.ID), step.Code, level, eventType, message, commandSummary, metadata)
}

func (s *Service) appendStructuredLog(ctx context.Context, taskID uint, stepID *uint, nodeID *uint, stepCode StepCode, level LogLevel, eventType LogEventType, message, commandSummary string, metadata LogMetadata) error {
	return s.AppendStepLog(ctx, &UpgradeStepLog{
		TaskID:          taskID,
		TaskStepID:      stepID,
		NodeExecutionID: nodeID,
		StepCode:        stepCode,
		Level:           level,
		EventType:       eventType,
		Message:         normalizeUserVisibleText(message),
		CommandSummary:  commandSummary,
		Metadata:        metadata,
	})
}

func buildNodeExecutionIndexes(nodes []UpgradeNodeExecution) (map[string]*UpgradeNodeExecution, map[uint]*UpgradeNodeExecution) {
	byKey := make(map[string]*UpgradeNodeExecution, len(nodes))
	byClusterNodeID := make(map[uint]*UpgradeNodeExecution, len(nodes))
	for i := range nodes {
		node := &nodes[i]
		byKey[nodeExecutionKey(node.HostID, node.Role)] = node
		byClusterNodeID[node.ClusterNodeID] = node
	}
	return byKey, byClusterNodeID
}

func selectSmokeTestTarget(nodeTargets []NodeTarget) (NodeTarget, bool) {
	if len(nodeTargets) == 0 {
		return NodeTarget{}, false
	}

	bestIndex := -1
	bestRank := 100
	for i, target := range nodeTargets {
		rank := smokeTestRoleRank(target.Role)
		if rank < bestRank {
			bestRank = rank
			bestIndex = i
		}
	}
	if bestIndex < 0 {
		return NodeTarget{}, false
	}
	return nodeTargets[bestIndex], true
}

func smokeTestRoleRank(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "master", "master_worker", "master-worker", "hybrid":
		return 0
	case "worker":
		return 1
	default:
		return 2
	}
}

func isTerminalNodeStatus(status ExecutionStatus) bool {
	switch status {
	case ExecutionStatusSucceeded, ExecutionStatusFailed, ExecutionStatusRollbackSucceeded, ExecutionStatusRollbackFailed, ExecutionStatusCancelled:
		return true
	default:
		return false
	}
}

func deriveFailureStatus(successStatus ExecutionStatus) ExecutionStatus {
	switch successStatus {
	case ExecutionStatusRollbackRunning, ExecutionStatusRollbackSucceeded:
		return ExecutionStatusRollbackFailed
	default:
		return ExecutionStatusFailed
	}
}

func collectConnectorFileNames(connectors []ConnectorArtifact) []string {
	files := make([]string, 0, len(connectors))
	for _, connector := range connectors {
		if connector.FileName != "" {
			files = append(files, connector.FileName)
		}
	}
	return dedupeSortedStrings(files)
}

func collectLibraryFileNames(libraries []LibraryArtifact) []string {
	files := make([]string, 0, len(libraries))
	for _, library := range libraries {
		if library.FileName != "" {
			files = append(files, library.FileName)
		}
	}
	return dedupeSortedStrings(files)
}

func collectPluginDependencyPaths(dependencies []PluginDependencyArtifact) []string {
	files := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		if strings.TrimSpace(dependency.RelativePath) != "" {
			files = append(files, dependency.RelativePath)
		}
	}
	return dedupeSortedStrings(files)
}

func nodeExecutionKey(hostID uint, role string) string {
	return fmt.Sprintf("%d:%s", hostID, role)
}

func parseOutputValue(output, key, fallback string) string {
	parts := strings.SplitN(output, "=", 2)
	if len(parts) == 2 && parts[0] == key {
		return parts[1]
	}
	return fallback
}

func canRollbackTask(steps []UpgradeTaskStep) bool {
	for _, step := range steps {
		if step.Code == StepCodeBackup && step.Status == ExecutionStatusSucceeded {
			return true
		}
	}
	return false
}
