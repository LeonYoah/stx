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
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Service) CreateDiagnosticTask(ctx context.Context, req *CreateDiagnosticTaskRequest, createdBy uint, createdByName string) (*DiagnosticTask, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidDiagnosticTaskRequest)
	}

	triggerSource := req.TriggerSource
	if triggerSource == "" {
		triggerSource = DiagnosticTaskSourceManual
	}

	sourceRef := req.SourceRef
	nodeScope := req.NodeScope
	if nodeScope == "" {
		nodeScope = DiagnosticTaskNodeScopeRelated
	}
	requestedNodeIDs := normalizeDiagnosticSelectedNodeIDs(req.SelectedNodeIDs)
	clusterID := req.ClusterID

	switch triggerSource {
	case DiagnosticTaskSourceManual:
		if clusterID == 0 {
			return nil, fmt.Errorf("%w: cluster_id is required for manual tasks", ErrInvalidDiagnosticTaskRequest)
		}
	case DiagnosticTaskSourceErrorGroup:
		if sourceRef.ErrorGroupID == 0 {
			return nil, fmt.Errorf("%w: source_ref.error_group_id is required", ErrInvalidDiagnosticTaskRequest)
		}
		group, err := s.repo.GetErrorGroupByID(ctx, sourceRef.ErrorGroupID)
		if err != nil {
			return nil, err
		}
		clusterID, err = resolveDiagnosticTaskClusterID(clusterID, group.LastClusterID)
		if err != nil {
			return nil, err
		}
		if nodeScope != DiagnosticTaskNodeScopeAll && len(requestedNodeIDs) == 0 && group.LastNodeID > 0 {
			requestedNodeIDs = []uint{group.LastNodeID}
		}
	case DiagnosticTaskSourceInspectionFinding:
		if sourceRef.InspectionFindingID == 0 && sourceRef.InspectionReportID == 0 {
			return nil, fmt.Errorf("%w: source_ref.inspection_report_id or source_ref.inspection_finding_id is required", ErrInvalidDiagnosticTaskRequest)
		}
		if sourceRef.InspectionFindingID > 0 {
			finding, err := s.repo.GetInspectionFindingByID(ctx, sourceRef.InspectionFindingID)
			if err != nil {
				return nil, err
			}
			clusterID, err = resolveDiagnosticTaskClusterID(clusterID, finding.ClusterID)
			if err != nil {
				return nil, err
			}
			if sourceRef.InspectionReportID == 0 {
				sourceRef.InspectionReportID = finding.ReportID
			}
			if nodeScope != DiagnosticTaskNodeScopeAll && len(requestedNodeIDs) == 0 && finding.RelatedNodeID > 0 {
				requestedNodeIDs = []uint{finding.RelatedNodeID}
			}
		} else {
			report, err := s.repo.GetInspectionReportByID(ctx, sourceRef.InspectionReportID)
			if err != nil {
				return nil, err
			}
			clusterID, err = resolveDiagnosticTaskClusterID(clusterID, report.ClusterID)
			if err != nil {
				return nil, err
			}
		}
	case DiagnosticTaskSourceAlert:
		if strings.TrimSpace(sourceRef.AlertID) == "" {
			return nil, fmt.Errorf("%w: source_ref.alert_id is required", ErrInvalidDiagnosticTaskRequest)
		}
		if clusterID == 0 {
			return nil, fmt.Errorf("%w: cluster_id is required for alert tasks", ErrInvalidDiagnosticTaskRequest)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported trigger_source %q", ErrInvalidDiagnosticTaskRequest, triggerSource)
	}

	options := req.Options.Normalize()
	if err := validateDiagnosticResourceSelection(options); err != nil {
		return nil, err
	}

	nodeTargets, err := s.buildDiagnosticTaskNodeTargets(ctx, clusterID, requestedNodeIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	planSteps := DefaultDiagnosticTaskSteps()
	initialStepCode := resolveInitialDiagnosticTaskStep(planSteps, options)
	lookbackMinutes := req.LookbackMinutes
	if lookbackMinutes < minInspectionLookbackMinutes || lookbackMinutes > maxInspectionLookbackMinutes {
		// 0 或非法值时回退到默认巡检窗口，后续可根据来源巡检窗口进一步细化。
		lookbackMinutes = 0
	}
	task := &DiagnosticTask{
		ClusterID:       clusterID,
		TriggerSource:   triggerSource,
		SourceRef:       sourceRef,
		Options:         options,
		LookbackMinutes: lookbackMinutes,
		Status:          DiagnosticTaskStatusReady,
		CurrentStep:     initialStepCode,
		SelectedNodes:   nodeTargets,
		Summary:         buildDiagnosticTaskSummary(triggerSource, sourceRef, req.Summary),
		CreatedBy:       createdBy,
		CreatedByName:   strings.TrimSpace(createdByName),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	steps := make([]*DiagnosticTaskStep, 0, len(planSteps))
	for _, planStep := range planSteps {
		status := DiagnosticTaskStatusPending
		message := planStep.Description
		if reason, skipped := shouldSkipDiagnosticPlanStep(planStep.Code, options); skipped {
			status = DiagnosticTaskStatusSkipped
			message = reason
		}
		steps = append(steps, &DiagnosticTaskStep{
			Code:        planStep.Code,
			Sequence:    planStep.Sequence,
			Title:       planStep.Title,
			Description: planStep.Description,
			Status:      status,
			Message:     message,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}

	nodes := make([]*DiagnosticNodeExecution, 0, len(nodeTargets))
	for _, target := range nodeTargets {
		nodes = append(nodes, &DiagnosticNodeExecution{
			ClusterNodeID: target.ClusterNodeID,
			NodeID:        target.NodeID,
			HostID:        target.HostID,
			HostName:      target.HostName,
			HostIP:        target.HostIP,
			Role:          target.Role,
			AgentID:       target.AgentID,
			InstallDir:    target.InstallDir,
			Status:        DiagnosticTaskStatusPending,
			CurrentStep:   planSteps[0].Code,
			CreatedAt:     now,
			UpdatedAt:     now,
		})
	}

	if err := s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.CreateDiagnosticTask(ctx, task); err != nil {
			return err
		}
		for _, step := range steps {
			step.TaskID = task.ID
		}
		if err := tx.CreateDiagnosticTaskSteps(ctx, steps); err != nil {
			return err
		}
		for _, node := range nodes {
			node.TaskID = task.ID
		}
		if err := tx.CreateDiagnosticNodeExecutions(ctx, nodes); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	result, err := s.repo.GetDiagnosticTaskByID(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	s.publishDiagnosticTaskEvent(newDiagnosticTaskSnapshotEvent(result))
	if req.AutoStart {
		if err := s.StartDiagnosticTask(context.Background(), result.ID); err != nil {
			return result, err
		}
		result, _ = s.repo.GetDiagnosticTaskByID(ctx, result.ID)
	}
	return result, nil
}

// GetDiagnosticTaskDetail returns one diagnostics task with related steps and node executions.
// GetDiagnosticTaskDetail 返回单个诊断任务及其关联步骤和节点执行。
func (s *Service) GetDiagnosticTaskDetail(ctx context.Context, taskID uint) (*DiagnosticTask, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	return s.repo.GetDiagnosticTaskByID(ctx, taskID)
}

// ListDiagnosticTasks returns paginated diagnostics task summaries.
// ListDiagnosticTasks 返回分页诊断任务摘要列表。
func (s *Service) ListDiagnosticTasks(ctx context.Context, filter *DiagnosticTaskListFilter) ([]*DiagnosticTaskSummary, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}
	return s.repo.ListDiagnosticTasks(ctx, filter)
}

// ListDiagnosticTaskSteps returns all steps of one diagnostics task.
// ListDiagnosticTaskSteps 返回单个诊断任务的全部步骤。
func (s *Service) ListDiagnosticTaskSteps(ctx context.Context, taskID uint) ([]*DiagnosticTaskStep, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	return s.repo.ListDiagnosticTaskSteps(ctx, taskID)
}

// ListDiagnosticNodeExecutions returns node executions of one diagnostics task.
// ListDiagnosticNodeExecutions 返回单个诊断任务的节点执行列表。
func (s *Service) ListDiagnosticNodeExecutions(ctx context.Context, taskID uint) ([]*DiagnosticNodeExecution, error) {
	if s == nil || s.repo == nil {
		return nil, ErrDiagnosticsRepositoryUnavailable
	}
	return s.repo.ListDiagnosticNodeExecutions(ctx, taskID)
}

// AppendDiagnosticStepLog appends one diagnostics task log and publishes a streaming event.
// AppendDiagnosticStepLog 追加一条诊断任务日志并发布事件流更新。
func (s *Service) AppendDiagnosticStepLog(ctx context.Context, log *DiagnosticStepLog) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	if err := s.repo.CreateDiagnosticStepLog(ctx, log); err != nil {
		return err
	}
	s.publishDiagnosticTaskEvent(newDiagnosticLogAppendedEvent(log))
	return nil
}

// ListDiagnosticStepLogs lists diagnostics task logs with filters and pagination.
// ListDiagnosticStepLogs 按过滤条件分页查询诊断任务日志。
func (s *Service) ListDiagnosticStepLogs(ctx context.Context, filter *DiagnosticTaskLogFilter) ([]*DiagnosticStepLog, int64, error) {
	if s == nil || s.repo == nil {
		return nil, 0, ErrDiagnosticsRepositoryUnavailable
	}
	return s.repo.ListDiagnosticStepLogs(ctx, filter)
}

// UpdateDiagnosticTask updates one diagnostics task and publishes a streaming event.
// UpdateDiagnosticTask 更新诊断任务并发布事件流更新。
func (s *Service) UpdateDiagnosticTask(ctx context.Context, task *DiagnosticTask) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	task.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateDiagnosticTask(ctx, task); err != nil {
		return err
	}
	s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	return nil
}

// UpdateDiagnosticTaskStep updates one diagnostics task step and publishes a streaming event.
// UpdateDiagnosticTaskStep 更新诊断任务步骤并发布事件流更新。
func (s *Service) UpdateDiagnosticTaskStep(ctx context.Context, step *DiagnosticTaskStep) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	step.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateDiagnosticTaskStep(ctx, step); err != nil {
		return err
	}
	s.publishDiagnosticTaskEvent(newDiagnosticStepUpdatedEvent(step))
	return nil
}

// UpdateDiagnosticNodeExecution updates one diagnostics node execution and publishes a streaming event.
// UpdateDiagnosticNodeExecution 更新诊断节点执行并发布事件流更新。
func (s *Service) UpdateDiagnosticNodeExecution(ctx context.Context, node *DiagnosticNodeExecution) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	node.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateDiagnosticNodeExecution(ctx, node); err != nil {
		return err
	}
	s.publishDiagnosticTaskEvent(newDiagnosticNodeUpdatedEvent(node))
	return nil
}

func (s *Service) buildDiagnosticTaskNodeTargets(ctx context.Context, clusterID uint, selectedNodeIDs []uint) (DiagnosticTaskNodeTargets, error) {
	if s.clusterService == nil {
		return nil, fmt.Errorf("%w: cluster service is unavailable", ErrInvalidDiagnosticTaskRequest)
	}
	clusterInfo, err := s.clusterService.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if len(clusterInfo.Nodes) == 0 {
		return nil, fmt.Errorf("%w: cluster has no managed nodes", ErrInvalidDiagnosticTaskRequest)
	}

	selected := make(map[uint]struct{}, len(selectedNodeIDs))
	for _, nodeID := range selectedNodeIDs {
		selected[nodeID] = struct{}{}
	}

	targets := make(DiagnosticTaskNodeTargets, 0, len(clusterInfo.Nodes))
	for _, node := range clusterInfo.Nodes {
		if len(selected) > 0 {
			if _, ok := selected[node.ID]; !ok {
				continue
			}
		}

		target := DiagnosticTaskNodeTarget{
			ClusterNodeID: node.ID,
			NodeID:        node.ID,
			HostID:        node.HostID,
			Role:          string(node.Role),
			InstallDir:    strings.TrimSpace(node.InstallDir),
		}
		if target.InstallDir == "" {
			target.InstallDir = strings.TrimSpace(clusterInfo.InstallDir)
		}
		if s.hostService != nil {
			hostInfo, err := s.hostService.GetHostByID(ctx, node.HostID)
			if err == nil && hostInfo != nil {
				target.HostName = hostInfo.Name
				target.HostIP = hostInfo.IPAddress
				target.AgentID = hostInfo.AgentID
			}
		}
		targets = append(targets, target)
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: selected nodes are not part of the cluster", ErrInvalidDiagnosticTaskRequest)
	}
	return targets, nil
}

func resolveDiagnosticTaskClusterID(requestClusterID, sourceClusterID uint) (uint, error) {
	if sourceClusterID > 0 {
		if requestClusterID > 0 && requestClusterID != sourceClusterID {
			return 0, fmt.Errorf("%w: cluster_id does not match source context", ErrInvalidDiagnosticTaskRequest)
		}
		return sourceClusterID, nil
	}
	if requestClusterID == 0 {
		return 0, fmt.Errorf("%w: cluster_id is required", ErrInvalidDiagnosticTaskRequest)
	}
	return requestClusterID, nil
}

func normalizeDiagnosticSelectedNodeIDs(nodeIDs []uint) []uint {
	if len(nodeIDs) == 0 {
		return nil
	}
	result := make([]uint, 0, len(nodeIDs))
	seen := make(map[uint]struct{}, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if nodeID == 0 {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		result = append(result, nodeID)
	}
	return result
}

func buildDiagnosticTaskSummary(triggerSource DiagnosticTaskSourceType, sourceRef DiagnosticTaskSourceRef, summary string) string {
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		return trimmed
	}
	switch triggerSource {
	case DiagnosticTaskSourceErrorGroup:
		return bilingualText(
			fmt.Sprintf("错误组 #%d 触发的诊断包", sourceRef.ErrorGroupID),
			fmt.Sprintf("Diagnostic bundle created from error group #%d", sourceRef.ErrorGroupID),
		)
	case DiagnosticTaskSourceInspectionFinding:
		if sourceRef.InspectionFindingID == 0 {
			return bilingualText(
				fmt.Sprintf("巡检报告 #%d 触发的诊断包", sourceRef.InspectionReportID),
				fmt.Sprintf("Diagnostic bundle created from inspection report #%d", sourceRef.InspectionReportID),
			)
		}
		return bilingualText(
			fmt.Sprintf("巡检发现 #%d 触发的诊断包", sourceRef.InspectionFindingID),
			fmt.Sprintf("Diagnostic bundle created from inspection finding #%d", sourceRef.InspectionFindingID),
		)
	case DiagnosticTaskSourceAlert:
		return bilingualText(
			fmt.Sprintf("告警 %s 触发的诊断包", strings.TrimSpace(sourceRef.AlertID)),
			fmt.Sprintf("Diagnostic bundle created from alert %s", strings.TrimSpace(sourceRef.AlertID)),
		)
	default:
		return bilingualText("手动创建的诊断包任务", "Manual diagnostic bundle task")
	}
}

func shouldSkipDiagnosticPlanStep(code DiagnosticStepCode, options DiagnosticTaskOptions) (string, bool) {
	if resourceCode, public := diagnosticResourceForStep(code); public && len(options.SelectedResources) > 0 && !containsDiagnosticResource(options.SelectedResources, resourceCode) {
		return bilingualText("任务未选择该诊断资源。", "The diagnostics resource was not selected for this task."), true
	}
	if options.ResourceOnly && (code == DiagnosticStepCodeAssembleManifest || code == DiagnosticStepCodeRenderHTMLSummary) {
		return bilingualText("单项资源任务不生成诊断包清单或 HTML 报告。", "A single-resource task does not generate a bundle manifest or HTML report."), true
	}
	switch code {
	case DiagnosticStepCodeCollectThreadDump:
		if !options.IncludeThreadDump {
			return bilingualText("任务配置未开启线程栈采集。", "Thread dump is disabled by task options."), true
		}
	case DiagnosticStepCodeCollectJVMDump:
		if !options.IncludeJVMDump {
			return bilingualText("任务配置未开启 JVM Dump 采集。", "JVM dump is disabled by task options."), true
		}
	}
	return "", false
}

func resolveInitialDiagnosticTaskStep(planSteps []DiagnosticPlanStep, options DiagnosticTaskOptions) DiagnosticStepCode {
	for _, planStep := range planSteps {
		if _, skipped := shouldSkipDiagnosticPlanStep(planStep.Code, options); skipped {
			continue
		}
		return planStep.Code
	}
	if len(planSteps) == 0 {
		return ""
	}
	return planSteps[0].Code
}
