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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
)

type AgentCommandSender interface {
	SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (success bool, output string, err error)
}

type ClusterLogProvider interface {
	GetNodeLogs(ctx context.Context, clusterID uint, nodeID uint, req *clusterapp.GetNodeLogsRequest) (string, error)
}

type ExecutionTargetResolver interface {
	ResolveExecutionTarget(ctx context.Context, clusterID uint, definition JSONMap) (*ExecutionTarget, error)
	ResolveExecutionTargets(ctx context.Context, clusterID uint, definition JSONMap) ([]*ExecutionTarget, error)
}

type ExecutionTarget struct {
	ClusterID     uint
	NodeID        uint
	HostID        uint
	AgentID       string
	InstallDir    string
	Role          string
	HostIP        string
	APIPort       int
	HazelcastPort int
}

type DefaultExecutionTargetResolver struct {
	clusterRepo *clusterapp.Repository
	hostRepo    *hostapp.Repository
}

func NewDefaultExecutionTargetResolver(clusterRepo *clusterapp.Repository, hostRepo *hostapp.Repository) *DefaultExecutionTargetResolver {
	return &DefaultExecutionTargetResolver{clusterRepo: clusterRepo, hostRepo: hostRepo}
}

func (r *DefaultExecutionTargetResolver) ResolveExecutionTarget(ctx context.Context, clusterID uint, definition JSONMap) (*ExecutionTarget, error) {
	targets, err := r.ResolveExecutionTargets(ctx, clusterID, definition)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, ErrExecutionTargetUnavailable
	}
	return targets[0], nil
}

func (r *DefaultExecutionTargetResolver) ResolveExecutionTargets(ctx context.Context, clusterID uint, definition JSONMap) ([]*ExecutionTarget, error) {
	if r == nil || r.clusterRepo == nil || r.hostRepo == nil {
		return nil, ErrExecutionTargetUnavailable
	}
	if clusterID == 0 {
		return nil, ErrLocalClusterRequired
	}
	if nodeID, ok := intValue(definition, "local_node_id", "node_id"); ok && nodeID > 0 {
		node, err := r.clusterRepo.GetNodeByID(ctx, uint(nodeID))
		if err == nil {
			if node.ClusterID != clusterID {
				return nil, fmt.Errorf(
					"sync: local node %d belongs to cluster %d, expected cluster %d: %w",
					node.ID,
					node.ClusterID,
					clusterID,
					ErrExecutionTargetClusterMismatch,
				)
			}
			target, buildErr := r.buildTarget(ctx, node)
			if buildErr != nil {
				return nil, buildErr
			}
			return []*ExecutionTarget{target}, nil
		}
	}
	nodes, err := r.clusterRepo.GetNodesByClusterID(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrExecutionTargetUnavailable
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return targetNodePriority(nodes[i]) < targetNodePriority(nodes[j])
	})
	targets := make([]*ExecutionTarget, 0, len(nodes))
	for _, node := range nodes {
		target, buildErr := r.buildTarget(ctx, node)
		if buildErr == nil {
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil, ErrExecutionTargetUnavailable
	}
	return targets, nil
}

func targetNodePriority(node *clusterapp.ClusterNode) int {
	if node == nil {
		return 99
	}
	switch node.Role {
	case clusterapp.NodeRoleMasterWorker:
		return 0
	case clusterapp.NodeRoleMaster:
		return 1
	case clusterapp.NodeRoleWorker:
		return 2
	default:
		return 3
	}
}

func (r *DefaultExecutionTargetResolver) buildTarget(ctx context.Context, node *clusterapp.ClusterNode) (*ExecutionTarget, error) {
	if node == nil {
		return nil, ErrExecutionTargetUnavailable
	}
	hostObj, err := r.hostRepo.GetByID(ctx, node.HostID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(hostObj.AgentID) == "" {
		return nil, ErrExecutionTargetUnavailable
	}
	installDir := strings.TrimSpace(node.InstallDir)
	if installDir == "" {
		installDir = "/opt/seatunnel"
	}
	return &ExecutionTarget{
		ClusterID:     node.ClusterID,
		NodeID:        node.ID,
		HostID:        node.HostID,
		AgentID:       strings.TrimSpace(hostObj.AgentID),
		InstallDir:    installDir,
		Role:          string(node.Role),
		HostIP:        strings.TrimSpace(hostObj.IPAddress),
		APIPort:       node.APIPort,
		HazelcastPort: node.HazelcastPort,
	}, nil
}

type LocalRunResponse struct {
	PID        int    `json:"pid"`
	ConfigFile string `json:"config_file"`
	LogFile    string `json:"log_file,omitempty"`
	StatusFile string `json:"status_file,omitempty"`
}

type LocalStatusResponse struct {
	Running    bool   `json:"running"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Message    string `json:"message"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type precheckJSONEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type JobLogsResult struct {
	Mode        string `json:"mode"`
	Source      string `json:"source"`
	Logs        string `json:"logs"`
	EmptyReason string `json:"empty_reason,omitempty"`
	NextOffset  string `json:"next_offset,omitempty"`
	FileSize    int64  `json:"file_size,omitempty"`
	UpdatedAt   string `json:"updated_at"`
}

type clusterJobLogPayload struct {
	Logs       string `json:"logs"`
	Path       string `json:"path,omitempty"`
	NextOffset string `json:"next_offset,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
}

type PreviewCollectRequest struct {
	PlatformJobID string                   `json:"platform_job_id"`
	EngineJobID   string                   `json:"engine_job_id"`
	Dataset       string                   `json:"dataset"`
	Catalog       map[string]interface{}   `json:"catalog"`
	Columns       []interface{}            `json:"columns"`
	Rows          []map[string]interface{} `json:"rows"`
	RowLimit      int                      `json:"row_limit"`
	Page          int                      `json:"page"`
	PageSize      int                      `json:"page_size"`
	Total         int                      `json:"total"`
	Replace       bool                     `json:"replace"`
}

func (s *Service) SetAgentCommandSender(sender AgentCommandSender) { s.agentSender = sender }
func (s *Service) SetExecutionTargetResolver(resolver ExecutionTargetResolver) {
	s.executionTargetResolver = resolver
}
func (s *Service) SetClusterLogProvider(provider ClusterLogProvider) { s.clusterLogProvider = provider }
func (s *Service) SetClusterVersionProvider(provider ClusterVersionProvider) {
	s.clusterVersionProvider = provider
}

func taskExecutionMode(task *Task) string {
	if task == nil {
		return "cluster"
	}
	mode := strings.ToLower(strings.TrimSpace(stringValue(task.Definition, "execution_mode")))
	if mode == "local" {
		return "local"
	}
	return "cluster"
}

func submitSpecExecutionMode(spec JSONMap) string {
	mode := strings.ToLower(strings.TrimSpace(stringValue(spec, "execution_mode")))
	if mode == "local" {
		return "local"
	}
	return "cluster"
}

func (s *Service) submitLocalTaskInstance(ctx context.Context, task *Task, createdBy uint, runType RunType, platformJobID string, body []byte, format, jobName string) (*JobInstance, error) {
	if s.agentSender == nil || s.executionTargetResolver == nil {
		return nil, ErrLocalExecutionUnavailable
	}
	target, err := s.executionTargetResolver.ResolveExecutionTarget(ctx, task.ClusterID, task.Definition)
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"sub_command":     "sync_local_run",
		"install_dir":     target.InstallDir,
		"cluster_id":      strconv.FormatUint(uint64(target.ClusterID), 10),
		"node_id":         strconv.FormatUint(uint64(target.NodeID), 10),
		"host_id":         strconv.FormatUint(uint64(target.HostID), 10),
		"platform_job_id": platformJobID,
		"job_name":        jobName,
		"content":         string(body),
		"content_format":  normalizeSubmitFormat(format),
	}
	success, output, err := s.agentSender.SendCommand(ctx, target.AgentID, "sync_local_run", params)
	if err != nil {
		return nil, err
	}
	if !success {
		if isLocalSyncCommandUnsupported(output) {
			return nil, fmt.Errorf("sync: local execution requires an upgraded agent that supports sync_local_run")
		}
		return nil, fmt.Errorf("sync: local run failed: %s", strings.TrimSpace(output))
	}
	decodedOutput := unwrapPrecheckPayload(output)
	var localRun LocalRunResponse
	if err := json.Unmarshal([]byte(decodedOutput), &localRun); err != nil {
		return nil, err
	}
	now := time.Now()
	instance := &JobInstance{
		TaskID:        task.ID,
		TaskVersion:   task.CurrentVersion,
		RunType:       runType,
		PlatformJobID: platformJobID,
		EngineJobID:   platformJobID,
		Status:        JobStatusRunning,
		SubmitSpec: JSONMap{
			"execution_mode":    "local",
			"cluster_id":        task.ClusterID,
			"target_node_id":    target.NodeID,
			"target_host_id":    target.HostID,
			"target_agent_id":   target.AgentID,
			"install_dir":       target.InstallDir,
			"format":            normalizeSubmitFormat(format),
			"submitted_content": string(body),
			"submitted_format":  normalizeSubmitFormat(format),
			"job_name":          jobName,
			"trigger_source":    triggerSourceForRunType(runType),
			"platform_job_id":   platformJobID,
			"config_file":       localRun.ConfigFile,
			"pid":               localRun.PID,
		},
		ResultPreview: JSONMap{
			"job_status":      "RUNNING",
			"submission_mode": "local",
		},
		StartedAt: &now,
		CreatedBy: createdBy,
	}
	if err := s.repo.CreateJobInstance(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) refreshLocalJob(ctx context.Context, instance *JobInstance) (*JobInstance, error) {
	if instance == nil || s.agentSender == nil {
		return instance, nil
	}
	agentID := strings.TrimSpace(stringValue(instance.SubmitSpec, "target_agent_id"))
	if agentID == "" {
		return instance, nil
	}
	params := map[string]string{
		"sub_command":     "sync_local_status",
		"pid":             strconv.Itoa(intValueOrZero(instance.SubmitSpec, "pid")),
		"platform_job_id": strings.TrimSpace(instance.PlatformJobID),
		"install_dir":     strings.TrimSpace(stringValue(instance.SubmitSpec, "install_dir")),
	}
	success, output, err := s.agentSender.SendCommand(ctx, agentID, "sync_local_status", params)
	if err != nil || !success {
		return instance, nil
	}
	decodedOutput := unwrapPrecheckPayload(output)
	var status LocalStatusResponse
	if err := json.Unmarshal([]byte(decodedOutput), &status); err != nil {
		return instance, nil
	}
	switch strings.ToLower(strings.TrimSpace(status.Status)) {
	case "success":
		instance.Status = JobStatusSuccess
	case "failed":
		instance.Status = JobStatusFailed
	case "canceled", "cancelled":
		instance.Status = JobStatusCanceled
	case "running":
		instance.Status = JobStatusRunning
	default:
		if status.Running {
			instance.Status = JobStatusRunning
		}
	}
	if instance.ResultPreview == nil {
		instance.ResultPreview = JSONMap{}
	}
	instance.ResultPreview["job_status"] = strings.ToUpper(strings.TrimSpace(status.Status))
	instance.ResultPreview["exit_code"] = status.ExitCode
	if strings.TrimSpace(status.Message) != "" {
		instance.ResultPreview["status_message"] = strings.TrimSpace(status.Message)
	}
	if (instance.Status == JobStatusSuccess || instance.Status == JobStatusFailed || instance.Status == JobStatusCanceled) && instance.FinishedAt == nil {
		now := time.Now()
		instance.FinishedAt = &now
		if strings.TrimSpace(status.Message) != "" && instance.Status == JobStatusFailed {
			instance.ErrorMessage = strings.TrimSpace(status.Message)
		}
	}
	if err := s.repo.UpdateJobInstance(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) stopLocalJob(ctx context.Context, instance *JobInstance) error {
	if instance == nil || s.agentSender == nil {
		return ErrLocalExecutionUnavailable
	}
	agentID := strings.TrimSpace(stringValue(instance.SubmitSpec, "target_agent_id"))
	if agentID == "" {
		return ErrLocalExecutionUnavailable
	}
	params := map[string]string{
		"sub_command":     "sync_local_stop",
		"pid":             strconv.Itoa(intValueOrZero(instance.SubmitSpec, "pid")),
		"platform_job_id": strings.TrimSpace(instance.PlatformJobID),
		"install_dir":     strings.TrimSpace(stringValue(instance.SubmitSpec, "install_dir")),
	}
	success, output, err := s.agentSender.SendCommand(ctx, agentID, "sync_local_stop", params)
	if err != nil {
		return err
	}
	if !success {
		if isLocalSyncCommandUnsupported(output) {
			return ErrLocalExecutionUnavailable
		}
		return fmt.Errorf("sync: local stop failed: %s", strings.TrimSpace(output))
	}
	return nil
}

func (s *Service) GetJobLogs(ctx context.Context, id uint, offset string, limitBytes int, keyword string, level string) (*JobLogsResult, error) {
	instance, err := s.repo.GetJobInstanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if limitBytes < 0 {
		limitBytes = 0
	}
	emptyResult := &JobLogsResult{
		Logs:        "",
		NextOffset:  "",
		FileSize:    0,
		EmptyReason: "logs_not_ready",
		UpdatedAt:   time.Now().Format(time.RFC3339),
	}
	if submitSpecExecutionMode(instance.SubmitSpec) == "local" {
		result, err := s.getLocalJobLogs(ctx, instance, offset, limitBytes, keyword, level)
		if errors.Is(err, ErrJobLogsUnavailable) {
			emptyResult.Mode = "local"
			emptyResult.Source = "agent-file"
			return emptyResult, nil
		}
		return result, err
	}
	result, err := s.getClusterJobLogs(ctx, instance, offset, limitBytes, keyword, level)
	if errors.Is(err, ErrJobLogsUnavailable) {
		emptyResult.Mode = "cluster"
		emptyResult.Source = "agent-file"
		return emptyResult, nil
	}
	return result, err
}

func (s *Service) getLocalJobLogs(ctx context.Context, instance *JobInstance, offset string, limitBytes int, keyword string, level string) (*JobLogsResult, error) {
	if s.agentSender == nil {
		return nil, ErrLocalExecutionUnavailable
	}
	agentID := strings.TrimSpace(stringValue(instance.SubmitSpec, "target_agent_id"))
	platformJobID := strings.TrimSpace(instance.PlatformJobID)
	if agentID == "" || platformJobID == "" {
		return nil, ErrJobLogsUnavailable
	}
	success, output, err := s.agentSender.SendCommand(ctx, agentID, "sync_local_logs", map[string]string{
		"sub_command":     "sync_local_logs",
		"platform_job_id": platformJobID,
		"keyword":         strings.TrimSpace(keyword),
		"level":           strings.TrimSpace(level),
		"install_dir":     strings.TrimSpace(stringValue(instance.SubmitSpec, "install_dir")),
		"offset":          strings.TrimSpace(offset),
		"limit_bytes":     strconv.Itoa(limitBytes),
	})
	if err != nil {
		return nil, err
	}
	if !success {
		if isLocalSyncCommandUnsupported(output) {
			return nil, ErrLocalExecutionUnavailable
		}
		return nil, fmt.Errorf("sync: get local logs failed: %s", strings.TrimSpace(output))
	}
	decodedOutput := unwrapPrecheckPayload(output)
	var payload struct {
		Logs       string `json:"logs"`
		NextOffset string `json:"next_offset"`
		FileSize   int64  `json:"file_size"`
	}
	if err := json.Unmarshal([]byte(decodedOutput), &payload); err != nil {
		return nil, err
	}
	return &JobLogsResult{Mode: "local", Source: "agent-file", Logs: payload.Logs, NextOffset: payload.NextOffset, FileSize: payload.FileSize, UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (s *Service) getClusterJobLogs(ctx context.Context, instance *JobInstance, offset string, limitBytes int, keyword string, level string) (*JobLogsResult, error) {
	if s.agentSender == nil || strings.TrimSpace(instance.EngineJobID) == "" {
		return nil, ErrJobLogsUnavailable
	}
	targets := s.resolveClusterLogTargets(ctx, instance)
	if len(targets) == 0 {
		return nil, ErrJobLogsUnavailable
	}
	logs, nextOffset, fileSize, found, err := s.readClusterLogsFromTargets(ctx, instance, targets, offset, limitBytes, keyword, level)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrJobLogsUnavailable
	}
	return &JobLogsResult{Mode: "cluster", Source: "agent-file", Logs: logs, NextOffset: nextOffset, FileSize: fileSize, UpdatedAt: time.Now().Format(time.RFC3339)}, nil
}

func (s *Service) resolveClusterLogTargets(ctx context.Context, instance *JobInstance) []*ExecutionTarget {
	targets := make([]*ExecutionTarget, 0, 4)
	seen := make(map[string]struct{})
	appendTarget := func(target *ExecutionTarget) {
		if target == nil {
			return
		}
		agentID := strings.TrimSpace(target.AgentID)
		installDir := strings.TrimSpace(target.InstallDir)
		if agentID == "" || installDir == "" {
			return
		}
		hostKey := strings.TrimSpace(target.HostIP)
		if target.HostID > 0 {
			hostKey = fmt.Sprintf("host:%d", target.HostID)
		}
		if hostKey == "" {
			hostKey = "agent:" + agentID
		}
		key := hostKey + "|" + installDir
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	appendTarget(&ExecutionTarget{
		AgentID:    strings.TrimSpace(stringValue(instance.SubmitSpec, "target_agent_id")),
		InstallDir: strings.TrimSpace(stringValue(instance.SubmitSpec, "install_dir")),
	})
	if s.executionTargetResolver == nil {
		return targets
	}
	clusterID := uintValue(instance.SubmitSpec, "cluster_id")
	resolved, err := s.executionTargetResolver.ResolveExecutionTargets(ctx, clusterID, nil)
	if err != nil {
		return targets
	}
	for _, target := range resolved {
		appendTarget(target)
	}
	return targets
}

func (s *Service) readClusterLogsFromTargets(ctx context.Context, instance *JobInstance, targets []*ExecutionTarget, offset string, limitBytes int, keyword string, level string) (string, string, int64, bool, error) {
	chunks := make([]string, 0, len(targets))
	found := false
	cursor := decodeClusterLogOffset(offset)
	nextCursor := make(map[string]int64)
	var totalFileSize int64
	for _, target := range targets {
		targetKey := buildClusterLogTargetKey(target)
		success, output, err := s.agentSender.SendCommand(ctx, target.AgentID, "sync_job_logs", map[string]string{
			"sub_command":     "sync_job_logs",
			"platform_job_id": strings.TrimSpace(instance.PlatformJobID),
			"engine_job_id":   strings.TrimSpace(instance.EngineJobID),
			"keyword":         strings.TrimSpace(keyword),
			"level":           strings.TrimSpace(level),
			"install_dir":     target.InstallDir,
			"offset":          strconv.FormatInt(cursor[targetKey], 10),
			"limit_bytes":     strconv.Itoa(limitBytes),
		})
		if err != nil || !success {
			continue
		}
		decodedOutput := unwrapPrecheckPayload(output)
		var payload clusterJobLogPayload
		if jsonErr := json.Unmarshal([]byte(decodedOutput), &payload); jsonErr != nil {
			continue
		}
		found = true
		if payload.NextOffset != "" {
			nextCursor[targetKey] = parseInt64OrZero(payload.NextOffset)
		}
		totalFileSize += payload.FileSize
		chunks = append(chunks, payload.Logs)
	}
	if !found {
		return "", "", 0, false, ErrJobLogsUnavailable
	}
	return mergeLogChunks(chunks), encodeClusterLogOffset(nextCursor), totalFileSize, true, nil
}

func buildClusterLogTargetKey(target *ExecutionTarget) string {
	hostKey := strings.TrimSpace(target.HostIP)
	if target.HostID > 0 {
		hostKey = fmt.Sprintf("host:%d", target.HostID)
	}
	if hostKey == "" {
		hostKey = "agent:" + strings.TrimSpace(target.AgentID)
	}
	return hostKey + "|" + strings.TrimSpace(target.InstallDir)
}

func decodeClusterLogOffset(raw string) map[string]int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]int64{}
	}
	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return map[string]int64{}
	}
	result := make(map[string]int64)
	_ = json.Unmarshal(decoded, &result)
	return result
}

func encodeClusterLogOffset(cursor map[string]int64) string {
	if len(cursor) == 0 {
		return ""
	}
	body, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(body)
}

func parseInt64OrZero(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func mergeLogChunks(chunks []string) string {
	merged := make([]string, 0, 256)
	for _, chunk := range chunks {
		for _, line := range strings.Split(strings.TrimSpace(chunk), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			merged = append(merged, trimmed)
		}
	}
	if len(merged) == 0 {
		return ""
	}
	return strings.Join(merged, "\n")
}

func isLocalSyncCommandUnsupported(output string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(output)), "unknown precheck sub-command")
}

func unwrapPrecheckPayload(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return trimmed
	}
	var envelope precheckJSONEnvelope
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil {
		message := strings.TrimSpace(envelope.Message)
		if message != "" {
			return message
		}
	}
	return trimmed
}

func (s *Service) CollectPreview(ctx context.Context, req *PreviewCollectRequest) error {
	if req == nil {
		return ErrPreviewPayloadInvalid
	}
	instance, err := s.repo.GetPreviewJobInstanceByPlatformOrEngineJobID(ctx, strings.TrimSpace(req.PlatformJobID), strings.TrimSpace(req.EngineJobID))
	if err != nil {
		return err
	}
	session, err := s.repo.GetPreviewSessionByJobInstanceID(ctx, instance.ID)
	if err != nil {
		if !errors.Is(err, ErrJobInstanceNotFound) {
			return err
		}
		now := time.Now()
		session = &PreviewSession{
			JobInstanceID:  instance.ID,
			TaskID:         instance.TaskID,
			PlatformJobID:  strings.TrimSpace(instance.PlatformJobID),
			EngineJobID:    strings.TrimSpace(instance.EngineJobID),
			RowLimit:       normalizePreviewRowLimit(req.RowLimit),
			TimeoutMinutes: defaultPreviewTimeoutMinutes,
			Status:         "collecting",
			StartedAt:      &now,
		}
		if err := s.repo.CreatePreviewSession(ctx, session); err != nil {
			return err
		}
	}
	if instance.ResultPreview == nil {
		instance.ResultPreview = JSONMap{}
	}
	datasetName := strings.TrimSpace(req.Dataset)
	if datasetName == "" {
		datasetName = "preview_dataset"
	}
	columns := interfaceSliceToStrings(req.Columns)
	rowLimit := normalizePreviewRowLimit(req.RowLimit)
	if session.RowLimit > 0 {
		rowLimit = normalizePreviewRowLimit(session.RowLimit)
	}
	acceptedRows := req.Rows
	if session.TotalRows >= rowLimit {
		acceptedRows = []map[string]interface{}{}
		session.Truncated = true
	} else if remaining := rowLimit - session.TotalRows; remaining < len(req.Rows) {
		acceptedRows = append([]map[string]interface{}{}, req.Rows[:remaining]...)
		session.Truncated = true
	}
	table, tableErr := s.repo.GetPreviewTableByPath(ctx, session.ID, datasetName)
	if tableErr != nil && !errors.Is(tableErr, ErrTaskNotFound) {
		return tableErr
	}
	if table == nil {
		table = &PreviewTable{
			SessionID:   session.ID,
			TablePath:   datasetName,
			DisplayName: datasetName,
			Columns:     append(JSONStringSlice{}, columns...),
		}
		if err := s.repo.CreatePreviewTable(ctx, table); err != nil {
			return err
		}
		session.TableCount += 1
	} else {
		if len(columns) > 0 {
			table.Columns = append(JSONStringSlice{}, columns...)
		}
		if req.Replace {
			session.TotalRows -= table.RowCount
			if session.TotalRows < 0 {
				session.TotalRows = 0
			}
			table.RowCount = 0
			if err := s.repo.DeletePreviewRowsByTableID(ctx, table.ID); err != nil {
				return err
			}
		}
	}
	if len(acceptedRows) > 0 {
		batch := make([]*PreviewRow, 0, len(acceptedRows))
		for index, row := range acceptedRows {
			batch = append(batch, &PreviewRow{
				SessionID: session.ID,
				TableID:   table.ID,
				RowIndex:  table.RowCount + index,
				RowData:   cloneJSONMap(row),
			})
		}
		if err := s.repo.CreatePreviewRows(ctx, batch); err != nil {
			return err
		}
		table.RowCount += len(acceptedRows)
		session.TotalRows += len(acceptedRows)
	}
	if err := s.repo.UpdatePreviewTable(ctx, table); err != nil {
		return err
	}
	if session.TotalRows >= rowLimit {
		session.Truncated = true
	}
	session.RowLimit = rowLimit
	if session.Truncated {
		session.Status = "truncated"
		finishedAt := time.Now()
		session.FinishedAt = &finishedAt
	} else {
		session.Status = "collecting"
	}
	if err := s.repo.UpdatePreviewSession(ctx, session); err != nil {
		return err
	}
	datasets := toDatasetSlice(instance.ResultPreview["datasets"])
	dataset := JSONMap{
		"name":       datasetName,
		"catalog":    cloneAnyMap(req.Catalog),
		"columns":    columns,
		"rows":       acceptedRows,
		"page":       normalizePositive(req.Page, 1),
		"page_size":  normalizePositive(req.PageSize, len(acceptedRows)),
		"total":      normalizePositive(req.Total, len(acceptedRows)),
		"updated_at": time.Now().Format(time.RFC3339),
	}
	replaced := false
	for idx, item := range datasets {
		if strings.EqualFold(strings.TrimSpace(stringValue(item, "name")), datasetName) {
			if !req.Replace {
				existingRows := mapRowsValue(item["rows"])
				dataset["rows"] = append(existingRows, acceptedRows...)
				dataset["rows"] = trimPreviewRows(dataset["rows"], rowLimit)
				dataset["page"] = 1
				dataset["page_size"] = len(mapRowsValue(dataset["rows"]))
				dataset["total"] = len(mapRowsValue(dataset["rows"]))
			}
			datasets[idx] = dataset
			replaced = true
			break
		}
	}
	if !replaced {
		dataset["rows"] = trimPreviewRows(dataset["rows"], rowLimit)
		dataset["page_size"] = len(mapRowsValue(dataset["rows"]))
		dataset["total"] = len(mapRowsValue(dataset["rows"]))
		datasets = append(datasets, dataset)
	}
	instance.ResultPreview["datasets"] = datasets
	instance.ResultPreview["columns"] = columns
	aggregatedRows := make([]map[string]interface{}, 0)
	for _, item := range datasets {
		aggregatedRows = append(aggregatedRows, mapRowsValue(item["rows"])...)
	}
	instance.ResultPreview["rows"] = aggregatedRows
	instance.ResultPreview["preview_row_limit"] = rowLimit
	instance.ResultPreview["preview_total_rows"] = session.TotalRows
	instance.ResultPreview["preview_table_count"] = session.TableCount
	instance.ResultPreview["preview_truncated"] = session.Truncated
	if err := s.repo.UpdateJobInstance(ctx, instance); err != nil {
		return err
	}
	if session.Truncated && (instance.Status == JobStatusPending || instance.Status == JobStatusRunning) {
		_, _ = s.CancelJob(ctx, instance.ID, false)
	}
	return nil
}

func trimPreviewRows(value interface{}, rowLimit int) []map[string]interface{} {
	rows := mapRowsValue(value)
	limit := normalizePreviewRowLimit(rowLimit)
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func normalizePreviewRowLimit(value int) int {
	if value <= 0 {
		return defaultPreviewRowLimit
	}
	if value > maxPreviewRowLimit {
		return maxPreviewRowLimit
	}
	return value
}

func normalizePreviewTimeoutMinutes(value int) int {
	if value <= 0 {
		return defaultPreviewTimeoutMinutes
	}
	if value > 24*60 {
		return 24 * 60
	}
	return value
}

func toDatasetSlice(value interface{}) []JSONMap {
	raw, ok := value.([]interface{})
	if !ok {
		if typed, ok := value.([]JSONMap); ok {
			return typed
		}
		return []JSONMap{}
	}
	result := make([]JSONMap, 0, len(raw))
	for _, item := range raw {
		if mapped, ok := item.(map[string]interface{}); ok {
			result = append(result, JSONMap(mapped))
		}
	}
	return result
}

func cloneAnyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func interfaceSliceToStrings(items []interface{}) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				result = append(result, strings.TrimSpace(value))
			}
		case map[string]interface{}:
			if name := strings.TrimSpace(stringValue(JSONMap(value), "name", "field", "column")); name != "" {
				result = append(result, name)
			}
		}
	}
	return result
}

func mapRowsValue(value interface{}) []map[string]interface{} {
	raw, ok := value.([]interface{})
	if !ok {
		if typed, ok := value.([]map[string]interface{}); ok {
			return typed
		}
		return []map[string]interface{}{}
	}
	result := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if mapped, ok := item.(map[string]interface{}); ok {
			result = append(result, mapped)
		}
	}
	return result
}

func intValueOrZero(src JSONMap, keys ...string) int {
	value, ok := intValue(src, keys...)
	if !ok {
		return 0
	}
	return value
}

func uintValue(src JSONMap, keys ...string) uint {
	value, ok := intValue(src, keys...)
	if !ok || value <= 0 {
		return 0
	}
	return uint(value)
}

func normalizePositive(value int, fallback int) int {
	if value > 0 {
		return value
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func mergePreviewDeriveMetadata(existing JSONMap, previewResult *ConfigToolPreviewResponse) JSONMap {
	if existing == nil {
		existing = JSONMap{}
	}
	if previewResult == nil {
		return existing
	}
	if strings.TrimSpace(previewResult.Content) != "" {
		existing["preview_content"] = previewResult.Content
	}
	if previewResult.ContentFormat != "" {
		existing["content_format"] = previewResult.ContentFormat
	}
	if len(previewResult.Warnings) > 0 {
		existing["warnings"] = previewResult.Warnings
	}
	if len(previewResult.Graph.Nodes) > 0 {
		existing["graph"] = map[string]interface{}{"nodes": previewResult.Graph.Nodes, "edges": previewResult.Graph.Edges}
	}
	return existing
}
