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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// ListSchedulerConnections 返回脱敏后的调度器连接列表。
// ListSchedulerConnections returns scheduler connections with tokens masked.
func (s *Service) ListSchedulerConnections(ctx context.Context) ([]*SchedulerConnection, error) {
	items, err := s.repo.ListSchedulerConnections(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		item.MaskToken()
	}
	return items, nil
}

// CreateSchedulerConnection 保存一条新的调度器连接。
// CreateSchedulerConnection stores a new scheduler connection.
func (s *Service) CreateSchedulerConnection(ctx context.Context, req *SchedulerConnectionRequest, createdBy uint) (*SchedulerConnection, error) {
	item, err := normalizeSchedulerConnection(nil, req)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(item.Token) == "" {
		return nil, fmt.Errorf("%w: token is required", ErrSchedulerConnectionInvalid)
	}
	item.CreatedBy = createdBy
	if err := s.repo.CreateSchedulerConnection(ctx, item); err != nil {
		return nil, err
	}
	item.MaskToken()
	return item, nil
}

// UpdateSchedulerConnection 更新调度器连接；空 token 表示保留原值。
// UpdateSchedulerConnection updates a scheduler connection and keeps the previous token when the new one is empty.
func (s *Service) UpdateSchedulerConnection(ctx context.Context, id uint, req *SchedulerConnectionRequest) (*SchedulerConnection, error) {
	existing, err := s.repo.GetSchedulerConnectionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	item, err := normalizeSchedulerConnection(existing, req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateSchedulerConnection(ctx, item); err != nil {
		return nil, err
	}
	item.MaskToken()
	return item, nil
}

// ListDolphinProjects 列出连接上的 Dolphin 项目。
// ListDolphinProjects lists Dolphin projects available on a connection.
func (s *Service) ListDolphinProjects(ctx context.Context, connectionID uint) ([]DolphinProject, error) {
	client, _, err := s.dolphinClient(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	items, err := client.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []DolphinProject{}
	}
	return items, nil
}

// ListDolphinWorkflows 列出项目下的工作流定义。
// ListDolphinWorkflows lists workflow definitions in a project.
func (s *Service) ListDolphinWorkflows(ctx context.Context, connectionID uint, projectCode int64) ([]DolphinWorkflowSummary, error) {
	client, _, err := s.dolphinClient(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	items, err := client.ListWorkflows(ctx, projectCode)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []DolphinWorkflowSummary{}
	}
	return items, nil
}

// GetDolphinWorkflow 读取工作流定义，供复用时勾选上下游。
// GetDolphinWorkflow loads a workflow definition so callers can pick upstream/downstream nodes.
func (s *Service) GetDolphinWorkflow(ctx context.Context, connectionID uint, projectCode, workflowCode int64) (*DolphinWorkflowDetail, error) {
	client, _, err := s.dolphinClient(ctx, connectionID)
	if err != nil {
		return nil, err
	}
	graph, err := client.GetDefinition(ctx, projectCode, workflowCode)
	if err != nil {
		return nil, err
	}
	detail := &DolphinWorkflowDetail{
		Code:         graph.Code,
		Name:         graph.Name,
		Version:      graph.Version,
		ReleaseState: graph.ReleaseState,
		Tasks:        make([]DolphinWorkflowTask, 0, len(graph.Tasks)),
		Relations:    make([]DolphinWorkflowRelation, 0, len(graph.Relations)),
	}
	for _, task := range graph.Tasks {
		detail.Tasks = append(detail.Tasks, DolphinWorkflowTask{
			Code:      task.Code,
			Name:      task.Name,
			TaskType:  task.TaskType,
			RawScript: dolphinTaskRawScript(task),
		})
	}
	for _, rel := range graph.Relations {
		detail.Relations = append(detail.Relations, DolphinWorkflowRelation{
			PreTaskCode:  rel.PreTaskCode,
			PostTaskCode: rel.PostTaskCode,
		})
	}
	return detail, nil
}

// GetDolphinScript 生成当前任务的 SHELL 节点脚本。
// GetDolphinScript builds the SHELL node script for the current task.
func (s *Service) GetDolphinScript(ctx context.Context, actor executionapp.Actor, taskID uint, timeout string, variables map[string]string, workerGroup, environment string) (*DolphinScriptResult, error) {
	task, err := s.GetTaskForActor(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	return BuildDolphinShellScript(task, timeout, variables, workerGroup, environment), nil
}

// GetDolphinTaskStatus 返回侧边栏需要的绑定、脚本、最近作业和调度令牌摘要。
// GetDolphinTaskStatus returns the sidebar summary of binding, script, recent jobs, and scheduler token.
func (s *Service) GetDolphinTaskStatus(ctx context.Context, actor executionapp.Actor, taskID uint) (*DolphinTaskStatus, error) {
	task, err := s.GetTaskForActor(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	binding, err := s.repo.GetSchedulerBindingByTaskID(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	status := &DolphinTaskStatus{
		Binding:       binding,
		BuiltinLocked: binding != nil,
		RecentJobs:    []*JobInstance{},
		Script:        BuildDolphinShellScript(task, "", nil, "", ""),
	}
	if binding != nil {
		if conn, err := s.repo.GetSchedulerConnectionByID(ctx, binding.ConnectionID); err == nil {
			conn.MaskToken()
			status.Connection = conn
		}
		jobs, err := s.repo.ListRecentJobsByExternalRefPrefix(ctx, task.ID, dolphinExternalRefPrefix, 8)
		if err != nil {
			return nil, err
		}
		status.RecentJobs = jobs
	}
	if token, err := auth.LatestSchedulerTokenForTask(s.repo.db, task.ID); err == nil && token != nil {
		status.SchedulerToken = &SchedulerTokenView{
			Issued:     true,
			Prefix:     token.TokenPrefix,
			LastUsedAt: token.LastUsedAt,
		}
	}
	return status, nil
}

// PushDolphin 把任务推送到 Dolphin：新建工作流、复用已有工作流，或只更新已绑定节点。
// PushDolphin pushes a task into Dolphin as a new workflow, a reused workflow, or a node-only update.
func (s *Service) PushDolphin(ctx context.Context, actor executionapp.Actor, taskID uint, req *DolphinPushRequest) (*DolphinPushResult, error) {
	if req == nil {
		return nil, ErrDolphinPushInvalid
	}
	task, err := s.GetTaskForActor(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskReadOnly
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}

	binding, err := s.repo.GetSchedulerBindingByTaskID(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if binding != nil {
		mode = "update"
	}
	if mode == "" {
		if binding != nil {
			mode = "update"
		} else {
			return nil, fmt.Errorf("%w: mode must be new or reuse", ErrDolphinPushInvalid)
		}
	}
	if binding == nil && mode != "new" && mode != "reuse" {
		return nil, fmt.Errorf("%w: mode must be new or reuse", ErrDolphinPushInvalid)
	}

	script := BuildDolphinShellScript(task, req.Timeout, req.Variables, req.WorkerGroup, req.Environment)
	result := &DolphinPushResult{DryRun: req.DryRun, Mode: mode, Script: script, Cycle: false}

	// 已绑定的更新请求可以省略 connection_id / project_code，改从绑定回填。
	// Bound update requests may omit connection_id / project_code; fall back to the saved binding.
	if req.ConnectionID == 0 && binding != nil {
		req.ConnectionID = binding.ConnectionID
	}
	client, conn, err := s.dolphinClient(ctx, req.ConnectionID)
	if err != nil {
		return nil, err
	}
	projectCode := req.ProjectCode
	if projectCode == 0 {
		projectCode = conn.DefaultProjectCode
	}
	if projectCode == 0 && binding != nil {
		projectCode = binding.ProjectCode
	}
	if projectCode == 0 && mode != "update" {
		return nil, fmt.Errorf("%w: project_code is required", ErrDolphinPushInvalid)
	}

	switch mode {
	case "new":
		return s.pushNewWorkflow(ctx, actor, client, conn, task, binding, req, script, result, projectCode)
	case "reuse":
		return s.pushReuseWorkflow(ctx, actor, client, conn, task, binding, req, script, result, projectCode)
	case "update":
		if binding == nil {
			return nil, ErrSchedulerBindingNotFound
		}
		return s.pushUpdateNode(ctx, actor, client, conn, task, binding, req, script, result)
	default:
		return nil, fmt.Errorf("%w: unsupported mode %s", ErrDolphinPushInvalid, mode)
	}
}

// UnbindDolphin 只删除 STX 侧绑定并吊销调度令牌，不改 Dolphin 节点。
// UnbindDolphin removes the STX binding and revokes scheduler tokens without changing the Dolphin node.
func (s *Service) UnbindDolphin(ctx context.Context, actor executionapp.Actor, taskID uint) error {
	task, err := s.GetTaskForActor(ctx, actor, taskID)
	if err != nil {
		return err
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return ErrTaskReadOnly
	}
	if err := s.repo.DeleteSchedulerBindingByTaskID(ctx, task.ID); err != nil {
		return err
	}
	return auth.RevokeSchedulerTokensForTask(s.repo.db, task.ID, time.Now())
}

func (s *Service) pushNewWorkflow(ctx context.Context, actor executionapp.Actor, client *DolphinClient, conn *SchedulerConnection, task *Task, binding *TaskSchedulerBinding, req *DolphinPushRequest, script *DolphinScriptResult, result *DolphinPushResult, projectCode int64) (*DolphinPushResult, error) {
	name := strings.TrimSpace(req.WorkflowName)
	if name == "" {
		name = strings.TrimSpace(task.Name)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: workflow_name is required", ErrDolphinPushInvalid)
	}
	if req.DryRun {
		result.Message = "create a one-node SHELL workflow"
		return result, nil
	}
	codes, err := client.GenTaskCodes(ctx, projectCode, 1)
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 || codes[0] == 0 {
		return nil, fmt.Errorf("%w: gen-task-codes returned empty", ErrDolphinPushInvalid)
	}
	shell := buildShellTask(codes[0], task.Name, script.Script, script.WorkerGroup, dolphinNodeTimeoutMinutes(script.Timeout))
	graph := &dolphinGraph{
		Name:          name,
		Description:   strings.TrimSpace(task.Description),
		ExecutionType: "PARALLEL",
		TenantCode:    "default",
		GlobalParams:  "[]",
		Locations:     fmt.Sprintf(`{"%d":{"x":240,"y":160}}`, shell.Code),
		Tasks:         []dolphinTaskDef{shell},
		Relations:     []dolphinRelation{startRelation(shell)},
	}
	created, err := client.CreateDefinition(ctx, projectCode, graph)
	if err != nil {
		return nil, err
	}
	if created != nil && created.Code > 0 {
		graph.Code = created.Code
		if created.Version > 0 {
			graph.Version = created.Version
		}
	}
	if req.UseBuiltinSchedule {
		cronExpr, timezone := builtinScheduleForDolphin(task)
		if cronExpr != "" {
			if err := client.CreateSchedule(ctx, projectCode, graph.Code, dolphinCrontab(cronExpr), timezone); err != nil {
				result.Warning = err.Error()
			}
		}
	}
	if err := client.Release(ctx, projectCode, graph.Code, dolphinReleaseOnline); err != nil {
		result.Warning = joinWarning(result.Warning, err.Error())
	} else if err := client.OnlineSchedules(ctx, projectCode, graph.Code); err != nil {
		result.Warning = joinWarning(result.Warning, err.Error())
	}
	saved, err := s.saveBindingAfterPush(ctx, task.ID, conn, projectCode, graph.Code, shell.Code, name, uint(actor.UserID), client.style)
	if err != nil {
		return nil, err
	}
	if err := s.disableTaskBuiltinSchedule(ctx, task.ID); err != nil {
		return nil, err
	}
	result.Binding = saved
	result.WorkflowCode = graph.Code
	result.TaskCode = shell.Code
	result.Version = graph.Version
	result.ReleaseState = dolphinReleaseOnline
	return result, nil
}

func (s *Service) pushReuseWorkflow(ctx context.Context, actor executionapp.Actor, client *DolphinClient, conn *SchedulerConnection, task *Task, _ *TaskSchedulerBinding, req *DolphinPushRequest, script *DolphinScriptResult, result *DolphinPushResult, projectCode int64) (*DolphinPushResult, error) {
	if req.WorkflowCode == 0 {
		return nil, fmt.Errorf("%w: workflow_code is required", ErrDolphinPushInvalid)
	}
	graph, err := client.GetDefinition(ctx, projectCode, req.WorkflowCode)
	if err != nil {
		return nil, err
	}
	if req.ExpectedVersion > 0 && graph.Version != req.ExpectedVersion {
		return nil, ErrDolphinVersionConflict
	}
	upstreams := uniqueInt64(req.Upstreams)
	downstreams := uniqueInt64(req.Downstreams)
	if containsInt64(upstreams, 0) || containsInt64(downstreams, 0) {
		return nil, fmt.Errorf("%w: invalid upstream/downstream code", ErrDolphinPushInvalid)
	}
	placeholder := int64(-1)
	extra := make([][2]int64, 0, len(upstreams)+len(downstreams))
	if len(upstreams) == 0 {
		extra = append(extra, [2]int64{0, placeholder})
	}
	for _, code := range upstreams {
		if !graphHasTask(graph, code) {
			return nil, fmt.Errorf("%w: upstream %d is missing", ErrDolphinPushInvalid, code)
		}
		extra = append(extra, [2]int64{code, placeholder})
	}
	for _, code := range downstreams {
		if !graphHasTask(graph, code) {
			return nil, fmt.Errorf("%w: downstream %d is missing", ErrDolphinPushInvalid, code)
		}
		extra = append(extra, [2]int64{placeholder, code})
	}
	if dolphinHasCycle(graph.Relations, extra) {
		result.Cycle = true
		return nil, ErrDolphinCycle
	}
	wasOnline := strings.EqualFold(graph.ReleaseState, dolphinReleaseOnline)
	result.WouldOffline = wasOnline
	result.Version = graph.Version
	result.WorkflowCode = graph.Code
	result.ReleaseState = graph.ReleaseState
	if req.DryRun {
		result.Message = "add a SHELL node and selected edges"
		return result, nil
	}

	offlineWarning := ""
	if wasOnline {
		if err := client.Release(ctx, projectCode, graph.Code, dolphinReleaseOffline); err != nil {
			return nil, err
		}
		offlineWarning = "DolphinScheduler workflow is OFFLINE; a later write failure needs a manual re-online"
		refreshed, err := client.GetDefinition(ctx, projectCode, graph.Code)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", offlineWarning, err)
		}
		if req.ExpectedVersion > 0 && refreshed.Version != req.ExpectedVersion && refreshed.Version != graph.Version {
			return nil, ErrDolphinVersionConflict
		}
		graph = refreshed
	}

	codes, err := client.GenTaskCodes(ctx, projectCode, 1)
	if err != nil {
		return nil, wrapOfflineWarning(offlineWarning, err)
	}
	if len(codes) == 0 || codes[0] == 0 {
		return nil, wrapOfflineWarning(offlineWarning, fmt.Errorf("%w: gen-task-codes returned empty", ErrDolphinPushInvalid))
	}
	shell := buildShellTask(codes[0], task.Name, script.Script, script.WorkerGroup, dolphinNodeTimeoutMinutes(script.Timeout))
	graph.Tasks = append(graph.Tasks, shell)
	graph.Locations = mergeDolphinLocation(graph.Locations, shell.Code)
	if len(upstreams) == 0 {
		graph.Relations = append(graph.Relations, startRelation(shell))
	}
	for _, code := range upstreams {
		graph.Relations = append(graph.Relations, edgeRelation(code, taskVersionByCode(graph.Tasks, code), shell))
	}
	for _, code := range downstreams {
		graph.Relations = stripStartRelation(graph.Relations, code)
		downstream := findTask(graph, code)
		if downstream == nil {
			return nil, wrapOfflineWarning(offlineWarning, fmt.Errorf("%w: downstream %d is missing", ErrDolphinPushInvalid, code))
		}
		graph.Relations = append(graph.Relations, edgeRelation(shell.Code, shell.Version, *downstream))
	}
	if err := client.UpdateDefinition(ctx, projectCode, graph); err != nil {
		return nil, wrapOfflineWarning(offlineWarning, err)
	}
	if wasOnline {
		if err := client.Release(ctx, projectCode, graph.Code, dolphinReleaseOnline); err != nil {
			return nil, fmt.Errorf("DolphinScheduler workflow is OFFLINE after a failed re-online: %w", err)
		}
		if err := client.OnlineSchedules(ctx, projectCode, graph.Code); err != nil {
			result.Warning = err.Error()
		}
	}
	saved, err := s.saveBindingAfterPush(ctx, task.ID, conn, projectCode, graph.Code, shell.Code, graph.Name, uint(actor.UserID), client.style)
	if err != nil {
		return nil, err
	}
	if err := s.disableTaskBuiltinSchedule(ctx, task.ID); err != nil {
		return nil, err
	}
	result.Binding = saved
	result.TaskCode = shell.Code
	result.Version = graph.Version
	result.ReleaseState = graph.ReleaseState
	return result, nil
}

func (s *Service) pushUpdateNode(ctx context.Context, actor executionapp.Actor, client *DolphinClient, conn *SchedulerConnection, task *Task, binding *TaskSchedulerBinding, req *DolphinPushRequest, script *DolphinScriptResult, result *DolphinPushResult) (*DolphinPushResult, error) {
	graph, err := client.GetDefinition(ctx, binding.ProjectCode, binding.WorkflowCode)
	if err != nil {
		return nil, err
	}
	if req.ExpectedVersion > 0 && graph.Version != req.ExpectedVersion {
		return nil, ErrDolphinVersionConflict
	}
	index := -1
	for i, item := range graph.Tasks {
		if item.Code == binding.TaskCode {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, ErrDolphinNodeMissing
	}
	wasOnline := strings.EqualFold(graph.ReleaseState, dolphinReleaseOnline)
	result.WouldOffline = wasOnline
	result.Version = graph.Version
	result.WorkflowCode = graph.Code
	result.TaskCode = binding.TaskCode
	result.ReleaseState = graph.ReleaseState
	if req.DryRun {
		result.Message = "overwrite the bound SHELL node without changing edges"
		return result, nil
	}

	offlineWarning := ""
	if wasOnline {
		if err := client.Release(ctx, binding.ProjectCode, graph.Code, dolphinReleaseOffline); err != nil {
			return nil, err
		}
		offlineWarning = "DolphinScheduler workflow is OFFLINE; a later write failure needs a manual re-online"
		refreshed, err := client.GetDefinition(ctx, binding.ProjectCode, graph.Code)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", offlineWarning, err)
		}
		if req.ExpectedVersion > 0 && refreshed.Version != req.ExpectedVersion && refreshed.Version != graph.Version {
			return nil, ErrDolphinVersionConflict
		}
		graph = refreshed
		index = -1
		for i, item := range graph.Tasks {
			if item.Code == binding.TaskCode {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, wrapOfflineWarning(offlineWarning, ErrDolphinNodeMissing)
		}
	}

	current := graph.Tasks[index]
	if current.TaskParams == nil {
		current.TaskParams = map[string]interface{}{}
	}
	current.TaskParams["rawScript"] = script.Script
	current.WorkerGroup = defaultString(script.WorkerGroup, current.WorkerGroup)
	current.Timeout = dolphinNodeTimeoutMinutes(script.Timeout)
	current.TimeoutFlag = "OPEN"
	current.TimeoutNotifyStrategy = "FAILED"
	current.Name = defaultString(strings.TrimSpace(task.Name), current.Name)
	if current.Version > 0 {
		current.Version++
	} else {
		current.Version = 1
	}
	graph.Tasks[index] = current
	if err := client.UpdateDefinition(ctx, binding.ProjectCode, graph); err != nil {
		return nil, wrapOfflineWarning(offlineWarning, err)
	}
	if wasOnline {
		if err := client.Release(ctx, binding.ProjectCode, graph.Code, dolphinReleaseOnline); err != nil {
			return nil, fmt.Errorf("DolphinScheduler workflow is OFFLINE after a failed re-online: %w", err)
		}
		if err := client.OnlineSchedules(ctx, binding.ProjectCode, graph.Code); err != nil {
			result.Warning = err.Error()
		}
	}
	saved, err := s.saveBindingAfterPush(ctx, task.ID, conn, binding.ProjectCode, graph.Code, binding.TaskCode, graph.Name, uint(actor.UserID), client.style)
	if err != nil {
		return nil, err
	}
	if err := s.disableTaskBuiltinSchedule(ctx, task.ID); err != nil {
		return nil, err
	}
	result.Binding = saved
	result.Version = graph.Version
	return result, nil
}

// saveBindingAfterPush 写入绑定，并用探测到的 API 族生成控制台链接。
// saveBindingAfterPush stores the binding and builds the console URL from the probed API family.
func (s *Service) saveBindingAfterPush(ctx context.Context, taskID uint, conn *SchedulerConnection, projectCode, workflowCode, taskCode int64, workflowName string, pushedBy uint, apiStyle string) (*TaskSchedulerBinding, error) {
	now := time.Now()
	item := &TaskSchedulerBinding{
		TaskID:       taskID,
		Kind:         SchedulerKindDolphin,
		ConnectionID: conn.ID,
		ProjectCode:  projectCode,
		WorkflowCode: workflowCode,
		TaskCode:     taskCode,
		WorkflowName: workflowName,
		ConsoleURL:   dolphinConsoleURL(conn.BaseURL, projectCode, workflowCode, apiStyle),
		PushedAt:     &now,
		PushedBy:     pushedBy,
	}
	if err := s.repo.SaveSchedulerBinding(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) dolphinClient(ctx context.Context, connectionID uint) (*DolphinClient, *SchedulerConnection, error) {
	if connectionID == 0 {
		return nil, nil, fmt.Errorf("%w: connection_id is required", ErrSchedulerConnectionInvalid)
	}
	conn, err := s.repo.GetSchedulerConnectionByID(ctx, connectionID)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(conn.Token) == "" || strings.TrimSpace(conn.BaseURL) == "" {
		return nil, nil, ErrSchedulerConnectionInvalid
	}
	client := newDolphinClient(conn, s.dolphinHTTPClient)
	if s.dolphinHTTPClient != nil {
		client.httpClient = s.dolphinHTTPClient
	}
	return client, conn, nil
}

func normalizeSchedulerConnection(existing *SchedulerConnection, req *SchedulerConnectionRequest) (*SchedulerConnection, error) {
	if req == nil {
		return nil, ErrSchedulerConnectionInvalid
	}
	item := &SchedulerConnection{Kind: SchedulerKindDolphin, APIStyle: SchedulerAPIStyleAuto}
	if existing != nil {
		clone := *existing
		item = &clone
	}
	if name := strings.TrimSpace(req.Name); name != "" {
		item.Name = name
	}
	if item.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrSchedulerConnectionInvalid)
	}
	if kind := strings.TrimSpace(req.Kind); kind != "" {
		item.Kind = kind
	}
	if item.Kind == "" {
		item.Kind = SchedulerKindDolphin
	}
	if baseURL := strings.TrimSpace(req.BaseURL); baseURL != "" {
		item.BaseURL = strings.TrimRight(baseURL, "/")
	}
	if item.BaseURL == "" {
		return nil, fmt.Errorf("%w: base_url is required", ErrSchedulerConnectionInvalid)
	}
	if token := strings.TrimSpace(req.Token); token != "" {
		item.Token = token
	}
	if req.DefaultProjectCode != 0 || existing == nil {
		item.DefaultProjectCode = req.DefaultProjectCode
	}
	style := strings.TrimSpace(req.APIStyle)
	if style == "" {
		style = item.APIStyle
	}
	if style == "" {
		style = SchedulerAPIStyleAuto
	}
	switch style {
	case SchedulerAPIStyleAuto, SchedulerAPIStyleProcess, SchedulerAPIStyleWorkflow:
		item.APIStyle = style
	default:
		return nil, fmt.Errorf("%w: unsupported api_style", ErrSchedulerConnectionInvalid)
	}
	if req.DolphinVersion != "" || existing == nil {
		item.DolphinVersion = strings.TrimSpace(req.DolphinVersion)
	}
	if item.DolphinVersion != "" {
		family, err := dolphinAPIFamilyFromVersion(item.DolphinVersion)
		if err != nil {
			return nil, err
		}
		if item.APIStyle == SchedulerAPIStyleAuto {
			item.APIStyle = family
		}
	}
	return item, nil
}

func builtinScheduleForDolphin(task *Task) (cronExpr, timezone string) {
	cfg, err := parseTaskSchedule(task.Definition)
	if err != nil || cfg == nil || strings.TrimSpace(cfg.CronExpr) == "" {
		return "", defaultTaskScheduleTimezone
	}
	timezone = cfg.Timezone
	if timezone == "" {
		timezone = defaultTaskScheduleTimezone
	}
	return cfg.CronExpr, timezone
}

func dolphinCrontab(cronExpr string) string {
	fields := strings.Fields(strings.TrimSpace(cronExpr))
	switch len(fields) {
	case 5:
		return "0 " + strings.Join(fields, " ")
	default:
		return strings.TrimSpace(cronExpr)
	}
}

func graphHasTask(graph *dolphinGraph, code int64) bool {
	return findTask(graph, code) != nil
}

func dolphinTaskRawScript(task dolphinTaskDef) string {
	if task.TaskParams == nil {
		return ""
	}
	for _, key := range []string{"rawScript", "raw_script"} {
		if value, ok := task.TaskParams[key].(string); ok {
			return value
		}
	}
	return ""
}

func findTask(graph *dolphinGraph, code int64) *dolphinTaskDef {
	if graph == nil {
		return nil
	}
	for i := range graph.Tasks {
		if graph.Tasks[i].Code == code {
			return &graph.Tasks[i]
		}
	}
	return nil
}

func uniqueInt64(values []int64) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	seen := map[int64]struct{}{}
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func stripStartRelation(relations []dolphinRelation, postCode int64) []dolphinRelation {
	filtered := make([]dolphinRelation, 0, len(relations))
	for _, rel := range relations {
		if rel.PostTaskCode == postCode && rel.PreTaskCode == 0 {
			continue
		}
		filtered = append(filtered, rel)
	}
	return filtered
}

func mergeDolphinLocation(raw string, code int64) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return fmt.Sprintf(`{"%d":{"x":320,"y":200}}`, code)
	}
	if strings.HasSuffix(trimmed, "}") {
		insert := fmt.Sprintf(`"%d":{"x":360,"y":240}`, code)
		if strings.TrimSpace(trimmed) == "{}" {
			return "{" + insert + "}"
		}
		return strings.TrimSuffix(strings.TrimSpace(trimmed), "}") + "," + insert + "}"
	}
	return trimmed
}

func joinWarning(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func wrapOfflineWarning(warning string, err error) error {
	if err == nil {
		return nil
	}
	if strings.TrimSpace(warning) == "" {
		return err
	}
	return fmt.Errorf("%s: %w", warning, err)
}

// IssueSchedulerTokenForTask 为已绑定任务签发调度令牌，旧令牌会被吊销。
// IssueSchedulerTokenForTask issues a scheduler token for a bound task and revokes previous ones.
func (s *Service) IssueSchedulerTokenForTask(ctx context.Context, actor executionapp.Actor, taskID uint, user *auth.User) (*SchedulerTokenIssueResult, error) {
	task, err := s.GetTaskForActor(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskReadOnly
	}
	binding, err := s.repo.GetSchedulerBindingByTaskID(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, ErrSchedulerTaskNotBound
	}
	issued, err := auth.IssueSchedulerToken(s.repo.db, user, task.ID, time.Now())
	if err != nil {
		return nil, err
	}
	result := &SchedulerTokenIssueResult{Token: issued.Token, Prefix: issued.Token, Kind: auth.CLITokenKindScheduler}
	if len(issued.Token) >= 12 {
		result.Prefix = issued.Token[:12]
	}
	if !issued.ExpiresAt.IsZero() {
		expires := issued.ExpiresAt
		result.ExpiresAt = &expires
	}
	return result, nil
}

// RevokeSchedulerTokenForTask 吊销任务上的调度令牌。
// RevokeSchedulerTokenForTask revokes scheduler tokens bound to the task.
func (s *Service) RevokeSchedulerTokenForTask(ctx context.Context, actor executionapp.Actor, taskID uint) error {
	task, err := s.GetTaskForActor(ctx, actor, taskID)
	if err != nil {
		return err
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return ErrTaskReadOnly
	}
	return auth.RevokeSchedulerTokensForTask(s.repo.db, task.ID, time.Now())
}
