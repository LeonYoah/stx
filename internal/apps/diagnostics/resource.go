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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// DiagnosticResourceCode identifies one user-visible evidence resource.
// DiagnosticResourceCode 标识一个面向用户的诊断证据资源。
type DiagnosticResourceCode string

const (
	DiagnosticResourceErrorContext  DiagnosticResourceCode = "error_context"
	DiagnosticResourceProcessEvents DiagnosticResourceCode = "process_events"
	DiagnosticResourceAlertSnapshot DiagnosticResourceCode = "alert_snapshot"
	DiagnosticResourceConfig        DiagnosticResourceCode = "config_snapshot"
	DiagnosticResourceLogSample     DiagnosticResourceCode = "log_sample"
	DiagnosticResourceThreadDump    DiagnosticResourceCode = "thread_dump"
	DiagnosticResourceJVMDump       DiagnosticResourceCode = "jvm_dump"
)

// DiagnosticResourceDefinition describes one registered public resource.
// DiagnosticResourceDefinition 描述一个已登记的公开诊断资源。
type DiagnosticResourceDefinition struct {
	Code              DiagnosticResourceCode `json:"code"`
	Title             string                 `json:"title"`
	Description       string                 `json:"description"`
	StepCode          DiagnosticStepCode     `json:"step_code"`
	Risk              string                 `json:"risk"`
	Impact            string                 `json:"impact"`
	TimeoutSeconds    int                    `json:"timeout_seconds"`
	ExecutionLocation string                 `json:"execution_location"`
	ResultType        string                 `json:"result_type"`
	BundleAllowed     bool                   `json:"bundle_allowed"`
	AdminOnly         bool                   `json:"admin_only"`
	DefaultSelected   bool                   `json:"default_selected"`
}

// DiagnosticArtifact describes one downloadable file produced by a task.
// DiagnosticArtifact 描述诊断任务产生的一个可下载文件。
type DiagnosticArtifact struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"media_type"`
}

// RunDiagnosticResourceRequest starts one public resource without producing a full bundle report.
// RunDiagnosticResourceRequest 启动单个公开诊断资源，不生成完整诊断包报告。
type RunDiagnosticResourceRequest struct {
	ClusterID        uint                    `json:"cluster_id"`
	NodeScope        DiagnosticTaskNodeScope `json:"node_scope,omitempty"`
	SelectedNodeIDs  []uint                  `json:"selected_node_ids"`
	LookbackMinutes  int                     `json:"lookback_minutes,omitempty"`
	Summary          string                  `json:"summary,omitempty"`
	JVMDumpMinFreeMB int                     `json:"jvm_dump_min_free_mb,omitempty"`
}

var diagnosticResourceDefinitions = []DiagnosticResourceDefinition{
	{Code: DiagnosticResourceErrorContext, Title: bilingualText("错误上下文", "Error Context"), Description: bilingualText("读取任务来源、错误组和巡检上下文。", "Read task source, error group, and inspection context."), StepCode: DiagnosticStepCodeCollectErrorContext, Risk: "R0", Impact: bilingualText("只读取 STX 已保存的数据。", "Reads data already stored by STX."), TimeoutSeconds: 30, ExecutionLocation: "control_plane", ResultType: "json", BundleAllowed: true, DefaultSelected: true},
	{Code: DiagnosticResourceProcessEvents, Title: bilingualText("进程事件", "Process Events"), Description: bilingualText("读取近期进程退出和自动拉起记录。", "Read recent process exits and automatic restart records."), StepCode: DiagnosticStepCodeCollectProcessEvents, Risk: "R0", Impact: bilingualText("只读取 STX 已保存的数据。", "Reads data already stored by STX."), TimeoutSeconds: 30, ExecutionLocation: "control_plane", ResultType: "json", BundleAllowed: true, DefaultSelected: true},
	{Code: DiagnosticResourceAlertSnapshot, Title: bilingualText("告警快照", "Alert Snapshot"), Description: bilingualText("读取相关告警和监控快照。", "Read related alerts and monitoring snapshots."), StepCode: DiagnosticStepCodeCollectAlertSnapshot, Risk: "R0", Impact: bilingualText("会读取监控接口，不修改集群。", "Reads monitoring endpoints without changing the cluster."), TimeoutSeconds: 60, ExecutionLocation: "control_plane", ResultType: "json", BundleAllowed: true, DefaultSelected: true},
	{Code: DiagnosticResourceConfig, Title: bilingualText("配置快照", "Configuration Snapshot"), Description: bilingualText("从选中节点读取 SeaTunnel 配置和目录清单。", "Read SeaTunnel configuration and directory inventories from selected nodes."), StepCode: DiagnosticStepCodeCollectConfigSnapshot, Risk: "R0", Impact: bilingualText("会通过 Agent 读取配置文件和目录清单。", "Uses the Agent to read configuration files and directory inventories."), TimeoutSeconds: 120, ExecutionLocation: "agent", ResultType: "files", BundleAllowed: true, DefaultSelected: true},
	{Code: DiagnosticResourceLogSample, Title: bilingualText("日志样本", "Log Sample"), Description: bilingualText("从选中节点读取指定时间窗口内的日志片段。", "Read log excerpts from selected nodes within the requested time window."), StepCode: DiagnosticStepCodeCollectLogSample, Risk: "R0", Impact: bilingualText("读取较大日志时会增加磁盘读取和网络传输。", "Reading large logs adds disk reads and network transfer."), TimeoutSeconds: 120, ExecutionLocation: "agent", ResultType: "files", BundleAllowed: true, DefaultSelected: true},
	{Code: DiagnosticResourceThreadDump, Title: bilingualText("线程快照", "Thread Dump"), Description: bilingualText("对选中节点的 SeaTunnel JVM 采集线程快照。", "Collect thread dumps from SeaTunnel JVMs on selected nodes."), StepCode: DiagnosticStepCodeCollectThreadDump, Risk: "R1", Impact: bilingualText("执行 jcmd 或 jstack，可能短暂增加目标 JVM 和主机负载。", "Runs jcmd or jstack and may briefly increase target JVM and host load."), TimeoutSeconds: 120, ExecutionLocation: "agent", ResultType: "files", BundleAllowed: true, DefaultSelected: false},
	{Code: DiagnosticResourceJVMDump, Title: "JVM Dump", Description: bilingualText("对选中节点的 SeaTunnel JVM 生成 Heap Dump。", "Create heap dumps from SeaTunnel JVMs on selected nodes."), StepCode: DiagnosticStepCodeCollectJVMDump, Risk: "R3", Impact: bilingualText("可能触发 Full GC、暂停目标 JVM，并产生较大的磁盘和网络开销。", "May trigger Full GC, pause the target JVM, and cause significant disk and network load."), TimeoutSeconds: 600, ExecutionLocation: "agent", ResultType: "files", BundleAllowed: true, AdminOnly: true},
}

// DefaultDiagnosticTaskOptions 返回默认诊断资源选择，高影响资源需由用户主动选择。
// DefaultDiagnosticTaskOptions returns the default diagnostic resources; higher-impact resources require explicit selection.
func DefaultDiagnosticTaskOptions() DiagnosticTaskOptions {
	selected := make([]DiagnosticResourceCode, 0, len(diagnosticResourceDefinitions))
	for _, resource := range diagnosticResourceDefinitions {
		if resource.BundleAllowed && resource.DefaultSelected {
			selected = append(selected, resource.Code)
		}
	}
	return DiagnosticTaskOptions{
		IncludeThreadDump: false,
		IncludeJVMDump:    false,
		JVMDumpMinFreeMB:  2048,
		SelectedResources: selected,
	}.Normalize()
}

// ListDiagnosticResources returns stable copies of all public resource definitions.
// ListDiagnosticResources 返回全部公开诊断资源定义的稳定副本。
func ListDiagnosticResources() []DiagnosticResourceDefinition {
	items := make([]DiagnosticResourceDefinition, len(diagnosticResourceDefinitions))
	copy(items, diagnosticResourceDefinitions)
	return items
}

// GetDiagnosticResource returns one public resource definition by code.
// GetDiagnosticResource 按编码返回一个公开诊断资源定义。
func GetDiagnosticResource(code DiagnosticResourceCode) (*DiagnosticResourceDefinition, error) {
	code = DiagnosticResourceCode(strings.TrimSpace(string(code)))
	for _, item := range diagnosticResourceDefinitions {
		if item.Code == code {
			copyItem := item
			return &copyItem, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrDiagnosticResourceNotFound, code)
}

// CreateDiagnosticResourceTask creates and starts a task containing exactly one public resource.
// CreateDiagnosticResourceTask 创建并启动一个只包含单项公开资源的诊断任务。
func (s *Service) CreateDiagnosticResourceTask(ctx context.Context, code DiagnosticResourceCode, req *RunDiagnosticResourceRequest, createdBy uint, createdByName string, executionRequest DiagnosticExecutionRequest) (*DiagnosticTask, error) {
	resource, err := GetDiagnosticResource(code)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidDiagnosticTaskRequest)
	}
	options := DiagnosticTaskOptions{
		SelectedResources: []DiagnosticResourceCode{resource.Code},
		ResourceOnly:      true,
		JVMDumpMinFreeMB:  req.JVMDumpMinFreeMB,
	}.Normalize()
	taskRequest := &CreateDiagnosticTaskRequest{
		ClusterID:       req.ClusterID,
		TriggerSource:   DiagnosticTaskSourceManual,
		NodeScope:       req.NodeScope,
		SelectedNodeIDs: req.SelectedNodeIDs,
		Options:         options,
		LookbackMinutes: req.LookbackMinutes,
		Summary:         req.Summary,
		AutoStart:       true,
	}
	executionRequest.OperationID = "diagnostics.resource.run"
	return s.CreateDiagnosticTaskWithExecution(ctx, taskRequest, createdBy, createdByName, executionRequest)
}

// ListDiagnosticTaskArtifacts returns safe relative files produced by one task.
// ListDiagnosticTaskArtifacts 返回一个诊断任务产生的安全相对文件列表。
func (s *Service) ListDiagnosticTaskArtifacts(ctx context.Context, actor executionapp.Actor, taskID uint) ([]DiagnosticArtifact, error) {
	task, err := s.GetDiagnosticTaskForActor(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	root := strings.TrimSpace(task.BundleDir)
	if root == "" {
		return []DiagnosticArtifact{}, nil
	}
	items := make([]DiagnosticArtifact, 0)
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		relativePath, relErr := filepath.Rel(root, path)
		if relErr != nil || relativePath == "." || strings.HasPrefix(relativePath, "..") {
			return relErr
		}
		checksum := ""
		if info.Size() <= 64*1024*1024 {
			var checksumErr error
			checksum, checksumErr = fileSHA256(path)
			if checksumErr != nil {
				return checksumErr
			}
		}
		items = append(items, DiagnosticArtifact{
			Path:      filepath.ToSlash(relativePath),
			Name:      info.Name(),
			Size:      info.Size(),
			SHA256:    checksum,
			MediaType: diagnosticArtifactMediaType(path),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, nil
}

func normalizeDiagnosticResourceCodes(values []DiagnosticResourceCode) []DiagnosticResourceCode {
	if len(values) == 0 {
		return nil
	}
	result := make([]DiagnosticResourceCode, 0, len(values))
	seen := make(map[DiagnosticResourceCode]struct{}, len(values))
	for _, value := range values {
		code := DiagnosticResourceCode(strings.TrimSpace(string(value)))
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func containsDiagnosticResource(values []DiagnosticResourceCode, target DiagnosticResourceCode) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func diagnosticResourceForStep(code DiagnosticStepCode) (DiagnosticResourceCode, bool) {
	for _, item := range diagnosticResourceDefinitions {
		if item.StepCode == code {
			return item.Code, true
		}
	}
	return "", false
}

func validateDiagnosticResourceSelection(options DiagnosticTaskOptions) error {
	for _, code := range options.SelectedResources {
		if _, err := GetDiagnosticResource(code); err != nil {
			return fmt.Errorf("%w: selected resource %q is unknown", ErrInvalidDiagnosticTaskRequest, code)
		}
	}
	if options.ResourceOnly && len(options.SelectedResources) != 1 {
		return fmt.Errorf("%w: resource-only task requires exactly one selected resource", ErrInvalidDiagnosticTaskRequest)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func diagnosticArtifactMediaType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".html":
		return "text/html"
	case ".log", ".txt", ".yaml", ".yml", ".conf":
		return "text/plain"
	case ".hprof":
		return "application/octet-stream"
	default:
		return "application/octet-stream"
	}
}
