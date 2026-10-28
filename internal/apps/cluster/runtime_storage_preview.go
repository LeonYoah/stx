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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type RuntimeStoragePreviewResult struct {
	ClusterID   uint   `json:"cluster_id"`
	Kind        string `json:"kind"`
	StorageType string `json:"storage_type,omitempty"`
	Path        string `json:"path,omitempty"`
	FileName    string `json:"file_name,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Binary      bool   `json:"binary,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	TextPreview string `json:"text_preview,omitempty"`
	HexPreview  string `json:"hex_preview,omitempty"`
}

type RuntimeStorageCheckpointInspectResult struct {
	ClusterID           uint                                              `json:"cluster_id"`
	Path                string                                            `json:"path,omitempty"`
	FileName            string                                            `json:"file_name,omitempty"`
	StorageType         string                                            `json:"storage_type,omitempty"`
	SizeBytes           int64                                             `json:"size_bytes,omitempty"`
	Truncated           bool                                              `json:"truncated,omitempty"`
	Binary              bool                                              `json:"binary,omitempty"`
	Encoding            string                                            `json:"encoding,omitempty"`
	TextPreview         string                                            `json:"text_preview,omitempty"`
	HexPreview          string                                            `json:"hex_preview,omitempty"`
	PipelineState       map[string]interface{}                            `json:"pipeline_state,omitempty"`
	CompletedCheckpoint map[string]interface{}                            `json:"completed_checkpoint,omitempty"`
	ActionStates        []map[string]interface{}                          `json:"action_states,omitempty"`
	TaskStatistics      []map[string]interface{}                          `json:"task_statistics,omitempty"`
	SourceStateInspect  *RuntimeStorageCheckpointSourceStateInspectResult `json:"source_state_inspect,omitempty"`
}

type RuntimeStorageCheckpointInspectJobConfig struct {
	Content       string                 `json:"content,omitempty"`
	ContentFormat string                 `json:"content_format,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

type RuntimeStorageCheckpointSourceStateInspectResult struct {
	PipelineState       map[string]interface{}   `json:"pipeline_state,omitempty"`
	CompletedCheckpoint map[string]interface{}   `json:"completed_checkpoint,omitempty"`
	Sources             []map[string]interface{} `json:"sources,omitempty"`
	Sinks               []map[string]interface{} `json:"sinks,omitempty"`
	UnsupportedSources  []map[string]interface{} `json:"unsupported_sources,omitempty"`
	Warnings            []string                 `json:"warnings,omitempty"`
	ErrorMessage        string                   `json:"error_message,omitempty"`
}

type RuntimeStorageIMAPInspectResult struct {
	ClusterID   uint                     `json:"cluster_id"`
	Path        string                   `json:"path,omitempty"`
	FileName    string                   `json:"file_name,omitempty"`
	StorageType string                   `json:"storage_type,omitempty"`
	SizeBytes   int64                    `json:"size_bytes,omitempty"`
	Truncated   bool                     `json:"truncated,omitempty"`
	Binary      bool                     `json:"binary,omitempty"`
	Encoding    string                   `json:"encoding,omitempty"`
	TextPreview string                   `json:"text_preview,omitempty"`
	HexPreview  string                   `json:"hex_preview,omitempty"`
	EntryCount  int                      `json:"entry_count,omitempty"`
	Entries     []map[string]interface{} `json:"entries,omitempty"`
}

func (s *Service) PreviewRuntimeStorage(
	ctx context.Context,
	clusterID uint,
	kind installerapp.RuntimeStorageValidationKind,
	path string,
	maxBytes int,
) (*RuntimeStoragePreviewResult, error) {
	if s.agentSender == nil || s.hostProvider == nil {
		return nil, fmt.Errorf("agent sender or host provider is not configured")
	}
	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	node, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	cfg, err := s.resolveRuntimeStorageValidationConfig(ctx, clusterObj, node, kind)
	if err != nil {
		return nil, err
	}
	if runtimeStorageValidationDisabled(kind, cfg) {
		return nil, fmt.Errorf("imap runtime storage is disabled")
	}
	params := runtimeStorageProxyParams(node.InstallDir, clusterObj.Version, kind, cfg.Checkpoint, cfg.IMAP, clusterObj)
	params["path"] = strings.TrimSpace(path)
	if maxBytes <= 0 {
		maxBytes = 64 * 1024
	}
	params["max_bytes"] = strconv.Itoa(maxBytes)
	success, output, err := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "stx_java_proxy_preview", params)
	result := runtimeStorageHostResultFromCommandOutput(success, output)
	if err == nil && result.Success {
		return decodeRuntimeStoragePreviewResult(clusterID, string(kind), result), nil
	}
	resolved, resolveErr := s.loadRuntimeStorageResolvedConfigFromNode(ctx, node, string(kind))
	if resolveErr == nil && resolved != nil && strings.EqualFold(strings.TrimSpace(resolved.StorageType), "S3") {
		fallback, fallbackErr := previewS3RuntimeStorage(ctx, clusterID, string(kind), resolved, path, maxBytes)
		if fallbackErr == nil {
			return fallback, nil
		}
	}
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("%s", firstNonEmpty(result.Message, "runtime storage preview failed"))
}

func (s *Service) InspectCheckpointRuntimeStorage(
	ctx context.Context,
	clusterID uint,
	path string,
	jobConfig *RuntimeStorageCheckpointInspectJobConfig,
) (*RuntimeStorageCheckpointInspectResult, error) {
	if s.agentSender == nil || s.hostProvider == nil {
		return nil, fmt.Errorf("agent sender or host provider is not configured")
	}
	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	node, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	resolved, err := s.loadRuntimeStorageResolvedConfigFromNode(ctx, node, "checkpoint")
	if err != nil {
		return nil, err
	}
	var contentBase64 string
	if resolved != nil && strings.EqualFold(strings.TrimSpace(resolved.StorageType), "S3") {
		content, _, readErr := readS3RuntimeStorageBytes(ctx, resolved, path, 8<<20)
		if readErr != nil {
			return nil, readErr
		}
		contentBase64 = base64.StdEncoding.EncodeToString(content)
	} else {
		cfg, cfgErr := s.resolveRuntimeStorageValidationConfig(ctx, clusterObj, node, installerapp.RuntimeStorageValidationCheckpoint)
		if cfgErr != nil {
			return nil, cfgErr
		}
		params := runtimeStorageProxyParams(node.InstallDir, clusterObj.Version, installerapp.RuntimeStorageValidationCheckpoint, cfg.Checkpoint, cfg.IMAP, clusterObj)
		params["path"] = strings.TrimSpace(path)
		success, output, sendErr := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "stx_java_proxy_inspect_checkpoint", params)
		result := runtimeStorageHostResultFromCommandOutput(success, output)
		if sendErr == nil && result.Success {
			inspectResult := decodeRuntimeStorageCheckpointInspectResult(clusterID, result)
			effectiveJobConfig := jobConfig
			if effectiveJobConfig == nil || strings.TrimSpace(effectiveJobConfig.Content) == "" {
				effectiveJobConfig = s.tryResolveJobConfigForCheckpoint(ctx, inspectResult, path)
			}
			s.attachCheckpointSourceStateInspect(
				ctx,
				clusterObj,
				node,
				hostInfo,
				inspectResult,
				path,
				effectiveJobConfig,
				"",
			)
			return inspectResult, nil
		}
		if sendErr != nil {
			return nil, sendErr
		}
		return nil, fmt.Errorf("%s", firstNonEmpty(result.Message, "checkpoint deserialize failed"))
	}
	params := map[string]string{
		"install_dir":    node.InstallDir,
		"version":        clusterObj.Version,
		"path":           strings.TrimSpace(path),
		"content_base64": contentBase64,
	}
	success, output, sendErr := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "stx_java_proxy_inspect_checkpoint", params)
	result := runtimeStorageHostResultFromCommandOutput(success, output)
	if sendErr != nil {
		return nil, sendErr
	}
	if !result.Success {
		return nil, fmt.Errorf("%s", firstNonEmpty(result.Message, "checkpoint deserialize failed"))
	}
	inspectResult := decodeRuntimeStorageCheckpointInspectResult(clusterID, result)
	effectiveJobConfig := jobConfig
	if effectiveJobConfig == nil || strings.TrimSpace(effectiveJobConfig.Content) == "" {
		effectiveJobConfig = s.tryResolveJobConfigForCheckpoint(ctx, inspectResult, path)
	}
	s.attachCheckpointSourceStateInspect(
		ctx,
		clusterObj,
		node,
		hostInfo,
		inspectResult,
		path,
		effectiveJobConfig,
		contentBase64,
	)
	return inspectResult, nil
}

func (s *Service) InspectIMAPRuntimeStorage(
	ctx context.Context,
	clusterID uint,
	path string,
) (*RuntimeStorageIMAPInspectResult, error) {
	if s.agentSender == nil || s.hostProvider == nil {
		return nil, fmt.Errorf("agent sender or host provider is not configured")
	}
	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	node, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	resolved, err := s.loadRuntimeStorageResolvedConfigFromNode(ctx, node, "imap")
	if err != nil {
		return nil, err
	}
	if resolved != nil && strings.EqualFold(strings.TrimSpace(resolved.StorageType), string(installerapp.IMAPStorageDisabled)) {
		return nil, fmt.Errorf("imap runtime storage is disabled")
	}
	var params map[string]string
	if resolved != nil && strings.EqualFold(strings.TrimSpace(resolved.StorageType), "S3") {
		content, _, readErr := readS3RuntimeStorageBytes(ctx, resolved, path, 8<<20)
		if readErr != nil {
			return nil, readErr
		}
		params = map[string]string{
			"install_dir":    node.InstallDir,
			"version":        clusterObj.Version,
			"path":           strings.TrimSpace(path),
			"content_base64": base64.StdEncoding.EncodeToString(content),
		}
	} else {
		cfg, cfgErr := s.resolveRuntimeStorageValidationConfig(ctx, clusterObj, node, installerapp.RuntimeStorageValidationIMAP)
		if cfgErr != nil {
			return nil, cfgErr
		}
		params = runtimeStorageProxyParams(node.InstallDir, clusterObj.Version, installerapp.RuntimeStorageValidationIMAP, cfg.Checkpoint, cfg.IMAP, clusterObj)
		params["path"] = strings.TrimSpace(path)
	}
	success, output, sendErr := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "stx_java_proxy_inspect_imap_wal", params)
	result := runtimeStorageHostResultFromCommandOutput(success, output)
	if sendErr != nil {
		return nil, sendErr
	}
	if !result.Success {
		return nil, fmt.Errorf("%s", firstNonEmpty(result.Message, "imap wal inspect failed"))
	}
	return decodeRuntimeStorageIMAPInspectResult(clusterID, result), nil
}

// buildMinimalHoconJobConfig generates a minimal synthetic HOCON configuration for the given source connector.
// buildMinimalHoconJobConfig 为指定的源端连接器生成用于反序列化快照位点的最小化 HOCON 配置。
func buildMinimalHoconJobConfig(pluginName string) *RuntimeStorageCheckpointInspectJobConfig {
	cleaned := strings.TrimSpace(pluginName)
	if cleaned == "" {
		return nil
	}
	hocon := fmt.Sprintf("env {\n  job.mode = \"STREAMING\"\n}\nsource {\n  %s {\n    result_table_name = \"src\"\n  }\n}\nsink {\n  Console {}\n}\n", cleaned)
	return &RuntimeStorageCheckpointInspectJobConfig{
		Content:       hocon,
		ContentFormat: "hocon",
	}
}

// tryResolveJobConfigForCheckpoint attempts to auto-resolve or infer the job config for a checkpoint snapshot.
// tryResolveJobConfigForCheckpoint 尝试通过引擎作业 ID 或算子名称自动关联/推断 Checkpoint 快照对应的作业配置。
func (s *Service) tryResolveJobConfigForCheckpoint(
	ctx context.Context,
	inspectResult *RuntimeStorageCheckpointInspectResult,
	path string,
) *RuntimeStorageCheckpointInspectJobConfig {
	if inspectResult == nil {
		return nil
	}

	// 1. 尝试从 CompletedCheckpoint、PipelineState 或文件路径中提取引擎作业 ID (engineJobID)
	// 1. Try to extract engineJobID from CompletedCheckpoint, PipelineState, or file path
	var engineJobID string
	if inspectResult.CompletedCheckpoint != nil {
		if jid, ok := inspectResult.CompletedCheckpoint["jobId"]; ok && jid != nil {
			engineJobID = strings.TrimSpace(fmt.Sprintf("%v", jid))
		}
	}
	if engineJobID == "" && inspectResult.PipelineState != nil {
		if jid, ok := inspectResult.PipelineState["jobId"]; ok && jid != nil {
			engineJobID = strings.TrimSpace(fmt.Sprintf("%v", jid))
		}
	}
	if engineJobID == "" {
		re := regexp.MustCompile(`/checkpoint[^/]*/([0-9a-zA-Z_-]+)/`)
		if matches := re.FindStringSubmatch(path); len(matches) > 1 {
			engineJobID = strings.TrimSpace(matches[1])
		}
	}

	// 2. 若找到 engineJobID 且数据库可用，优先从同步任务实例表检索提交配置
	// 2. If engineJobID is found and DB is available, query sync_job_instances for original submission script
	if engineJobID != "" && s.repo != nil && s.repo.db != nil {
		if s.repo.db.Migrator().HasTable("sync_job_instances") {
			var row struct {
				SubmitSpec    string `gorm:"column:submit_spec"`
				ResultPreview string `gorm:"column:result_preview"`
				TaskID        uint   `gorm:"column:task_id"`
			}
			if err := s.repo.db.WithContext(ctx).Table("sync_job_instances").
				Select("submit_spec, result_preview, task_id").
				Where("engine_job_id = ?", engineJobID).
				Order("id DESC").
				Limit(1).
				Scan(&row).Error; err == nil {
				if strings.TrimSpace(row.SubmitSpec) != "" {
					var spec map[string]interface{}
					if json.Unmarshal([]byte(row.SubmitSpec), &spec) == nil {
						if content, ok := spec["submitted_content"].(string); ok && strings.TrimSpace(content) != "" {
							format := "hocon"
							if f, ok := spec["submitted_format"].(string); ok && f != "" {
								format = f
							} else if f, ok := spec["format"].(string); ok && f != "" {
								format = f
							}
							return &RuntimeStorageCheckpointInspectJobConfig{
								Content:       content,
								ContentFormat: format,
							}
						}
					}
				}
				if strings.TrimSpace(row.ResultPreview) != "" {
					var preview map[string]interface{}
					if json.Unmarshal([]byte(row.ResultPreview), &preview) == nil {
						if content, ok := preview["preview_content"].(string); ok && strings.TrimSpace(content) != "" {
							format := "hocon"
							if f, ok := preview["content_format"].(string); ok && f != "" {
								format = f
							}
							return &RuntimeStorageCheckpointInspectJobConfig{
								Content:       content,
								ContentFormat: format,
							}
						}
					}
				}
				if row.TaskID > 0 && s.repo.db.Migrator().HasTable("sync_tasks") {
					var task struct {
						Content       string `gorm:"column:content"`
						ContentFormat string `gorm:"column:content_format"`
					}
					if err := s.repo.db.WithContext(ctx).Table("sync_tasks").
						Select("content, content_format").
						Where("id = ?", row.TaskID).
						Scan(&task).Error; err == nil && strings.TrimSpace(task.Content) != "" {
						format := task.ContentFormat
						if format == "" {
							format = "hocon"
						}
						return &RuntimeStorageCheckpointInspectJobConfig{
							Content:       task.Content,
							ContentFormat: format,
						}
					}
				}
			}
		}
	}

	// 3. 若数据库中未匹配到作业配置，从快照 ActionStates 算子标识中推断源端插件类型
	// 3. If DB has no match, infer source connector plugin name directly from checkpoint ActionStates
	if len(inspectResult.ActionStates) > 0 {
		reAction := regexp.MustCompile(`\[Source\[\d+\]-([A-Za-z0-9_-]+)\]`)
		for _, action := range inspectResult.ActionStates {
			name, _ := action["name"].(string)
			if matches := reAction.FindStringSubmatch(name); len(matches) > 1 {
				pluginName := strings.TrimSpace(matches[1])
				if pluginName != "" {
					return buildMinimalHoconJobConfig(pluginName)
				}
			}
		}
	}

	return nil
}

func (s *Service) attachCheckpointSourceStateInspect(
	ctx context.Context,
	clusterObj *Cluster,
	node *NodeInfo,
	hostInfo *HostInfo,
	inspectResult *RuntimeStorageCheckpointInspectResult,
	path string,
	jobConfig *RuntimeStorageCheckpointInspectJobConfig,
	contentBase64 string,
) {
	if inspectResult == nil || clusterObj == nil || node == nil || hostInfo == nil {
		return
	}
	if jobConfig == nil || strings.TrimSpace(jobConfig.Content) == "" {
		return
	}
	result, err := s.inspectCheckpointSourceStateRuntimeStorage(
		ctx,
		clusterObj,
		node,
		hostInfo,
		path,
		jobConfig,
		contentBase64,
	)
	if err != nil {
		inspectResult.SourceStateInspect = &RuntimeStorageCheckpointSourceStateInspectResult{
			ErrorMessage: err.Error(),
		}
		return
	}
	inspectResult.SourceStateInspect = result
}

func (s *Service) inspectCheckpointSourceStateRuntimeStorage(
	ctx context.Context,
	clusterObj *Cluster,
	node *NodeInfo,
	hostInfo *HostInfo,
	path string,
	jobConfig *RuntimeStorageCheckpointInspectJobConfig,
	contentBase64 string,
) (*RuntimeStorageCheckpointSourceStateInspectResult, error) {
	var params map[string]string
	if strings.TrimSpace(contentBase64) != "" {
		params = map[string]string{
			"install_dir":    node.InstallDir,
			"version":        clusterObj.Version,
			"path":           strings.TrimSpace(path),
			"content_base64": strings.TrimSpace(contentBase64),
		}
		attachClusterJavaProxyPort(params, clusterObj)
	} else {
		cfg, err := s.resolveRuntimeStorageValidationConfig(
			ctx, clusterObj, node, installerapp.RuntimeStorageValidationCheckpoint)
		if err != nil {
			return nil, err
		}
		params = runtimeStorageProxyParams(
			node.InstallDir,
			clusterObj.Version,
			installerapp.RuntimeStorageValidationCheckpoint,
			cfg.Checkpoint,
			cfg.IMAP,
			clusterObj,
		)
		params["path"] = strings.TrimSpace(path)
	}
	if jobConfig != nil && strings.TrimSpace(jobConfig.Content) != "" {
		re := regexp.MustCompile(`([:=]\s*)(\*{2,})([ \t\r\n,]|\z)`)
		jobConfig.Content = re.ReplaceAllString(jobConfig.Content, `${1}"${2}"${3}`)
	}
	jobConfigJSON, err := json.Marshal(jobConfig)
	if err != nil {
		return nil, err
	}
	params["job_config_json"] = string(jobConfigJSON)
	success, output, sendErr := s.agentSender.SendCommand(
		ctx,
		hostInfo.AgentID,
		"stx_java_proxy_inspect_checkpoint_source_state",
		params,
	)
	if sendErr != nil {
		return nil, sendErr
	}
	result := runtimeStorageHostResultFromCommandOutput(success, output)
	if !result.Success {
		return nil, fmt.Errorf("%s", firstNonEmpty(result.Message, "checkpoint source state inspect failed"))
	}
	return decodeRuntimeStorageCheckpointSourceStateInspectResult(result), nil
}

func decodeRuntimeStoragePreviewResult(clusterID uint, kind string, result *installerapp.RuntimeStorageValidationHostResult) *RuntimeStoragePreviewResult {
	preview := &RuntimeStoragePreviewResult{ClusterID: clusterID, Kind: kind}
	if result == nil {
		return preview
	}
	preview.Path = strings.TrimSpace(result.Details["path"])
	preview.FileName = strings.TrimSpace(result.Details["file_name"])
	preview.StorageType = strings.TrimSpace(result.Details["storage_type"])
	preview.SizeBytes, _ = strconv.ParseInt(strings.TrimSpace(result.Details["size_bytes"]), 10, 64)
	preview.Truncated = strings.EqualFold(strings.TrimSpace(result.Details["truncated"]), "true")
	preview.Binary = strings.EqualFold(strings.TrimSpace(result.Details["binary"]), "true")
	preview.Encoding = strings.TrimSpace(result.Details["encoding"])
	preview.TextPreview = result.Details["text_preview"]
	preview.HexPreview = result.Details["hex_preview"]
	return preview
}

func decodeRuntimeStorageCheckpointInspectResult(clusterID uint, result *installerapp.RuntimeStorageValidationHostResult) *RuntimeStorageCheckpointInspectResult {
	inspect := &RuntimeStorageCheckpointInspectResult{ClusterID: clusterID}
	if result == nil {
		return inspect
	}
	inspect.Path = strings.TrimSpace(result.Details["path"])
	inspect.FileName = strings.TrimSpace(result.Details["file_name"])
	inspect.StorageType = strings.TrimSpace(result.Details["storage_type"])
	inspect.SizeBytes, _ = strconv.ParseInt(strings.TrimSpace(result.Details["size_bytes"]), 10, 64)
	inspect.Truncated = strings.EqualFold(strings.TrimSpace(result.Details["truncated"]), "true")
	inspect.Binary = strings.EqualFold(strings.TrimSpace(result.Details["binary"]), "true")
	inspect.Encoding = strings.TrimSpace(result.Details["encoding"])
	inspect.TextPreview = result.Details["text_preview"]
	inspect.HexPreview = result.Details["hex_preview"]
	if raw := strings.TrimSpace(result.Details["pipeline_state_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.PipelineState)
	}
	if raw := strings.TrimSpace(result.Details["completed_checkpoint_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.CompletedCheckpoint)
	}
	if raw := strings.TrimSpace(result.Details["action_states_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.ActionStates)
	}
	if raw := strings.TrimSpace(result.Details["task_statistics_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.TaskStatistics)
	}
	return inspect
}

func decodeRuntimeStorageCheckpointSourceStateInspectResult(
	result *installerapp.RuntimeStorageValidationHostResult,
) *RuntimeStorageCheckpointSourceStateInspectResult {
	inspect := &RuntimeStorageCheckpointSourceStateInspectResult{}
	if result == nil {
		return inspect
	}
	if raw := strings.TrimSpace(result.Details["pipeline_state_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.PipelineState)
	}
	if raw := strings.TrimSpace(result.Details["completed_checkpoint_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.CompletedCheckpoint)
	}
	if raw := strings.TrimSpace(result.Details["sources_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.Sources)
	}
	if raw := strings.TrimSpace(result.Details["sinks_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.Sinks)
	}
	if raw := strings.TrimSpace(result.Details["unsupported_sources_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.UnsupportedSources)
	}
	if raw := strings.TrimSpace(result.Details["warnings_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.Warnings)
	}
	return inspect
}

func decodeRuntimeStorageIMAPInspectResult(clusterID uint, result *installerapp.RuntimeStorageValidationHostResult) *RuntimeStorageIMAPInspectResult {
	inspect := &RuntimeStorageIMAPInspectResult{ClusterID: clusterID}
	if result == nil {
		return inspect
	}
	inspect.Path = strings.TrimSpace(result.Details["path"])
	inspect.FileName = strings.TrimSpace(result.Details["file_name"])
	inspect.StorageType = strings.TrimSpace(result.Details["storage_type"])
	inspect.SizeBytes, _ = strconv.ParseInt(strings.TrimSpace(result.Details["size_bytes"]), 10, 64)
	inspect.Truncated = strings.EqualFold(strings.TrimSpace(result.Details["truncated"]), "true")
	inspect.Binary = strings.EqualFold(strings.TrimSpace(result.Details["binary"]), "true")
	inspect.Encoding = strings.TrimSpace(result.Details["encoding"])
	inspect.TextPreview = result.Details["text_preview"]
	inspect.HexPreview = result.Details["hex_preview"]
	inspect.EntryCount, _ = strconv.Atoi(strings.TrimSpace(result.Details["entry_count"]))
	if raw := strings.TrimSpace(result.Details["entries_json"]); raw != "" {
		_ = json.Unmarshal([]byte(raw), &inspect.Entries)
	}
	return inspect
}

func previewS3RuntimeStorage(
	ctx context.Context,
	clusterID uint,
	kind string,
	cfg *runtimeStorageResolvedConfig,
	path string,
	maxBytes int,
) (*RuntimeStoragePreviewResult, error) {
	content, objectInfo, err := readS3RuntimeStorageBytes(ctx, cfg, path, int64(maxBytes))
	if err != nil {
		return nil, err
	}
	binary := isLikelyBinary(content)
	preview := &RuntimeStoragePreviewResult{
		ClusterID:   clusterID,
		Kind:        kind,
		StorageType: cfg.StorageType,
		Path:        objectInfo.Key,
		FileName:    pathBase(objectInfo.Key),
		SizeBytes:   objectInfo.Size,
		Truncated:   objectInfo.Size > int64(len(content)),
		Binary:      binary,
		Encoding:    "utf-8",
	}
	if binary {
		preview.Encoding = "binary"
		preview.HexPreview = hexPreview(content, 256)
	} else {
		preview.TextPreview = string(content)
	}
	return preview, nil
}

func readS3RuntimeStorageBytes(
	ctx context.Context,
	cfg *runtimeStorageResolvedConfig,
	path string,
	maxBytes int64,
) ([]byte, minio.ObjectInfo, error) {
	var empty minio.ObjectInfo
	if cfg == nil {
		return nil, empty, fmt.Errorf("storage config is nil")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return nil, empty, fmt.Errorf("s3 endpoint or bucket is empty")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, empty, fmt.Errorf("s3 credentials are unavailable")
	}
	parsed, err := neturl.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil {
		return nil, empty, err
	}
	host := parsed.Host
	if host == "" {
		host = strings.TrimSpace(cfg.Endpoint)
	}
	bucket := sanitizeObjectStoreBucket(cfg.Bucket)
	key := sanitizeObjectStorePrefix(path)
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       strings.EqualFold(parsed.Scheme, "https"),
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return nil, empty, err
	}
	stat, err := client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, empty, err
	}
	reader, err := client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, empty, err
	}
	defer reader.Close()
	if maxBytes <= 0 {
		maxBytes = stat.Size
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxBytes))
	if err != nil {
		return nil, empty, err
	}
	return content, stat, nil
}

func isLikelyBinary(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	control := 0
	for _, b := range content {
		if b == 0 {
			return true
		}
		if (b < 0x09 || (b > 0x0d && b < 0x20)) && b != 0x1b {
			control++
		}
	}
	return control > len(content)/8
}

func hexPreview(content []byte, limit int) string {
	if limit <= 0 || len(content) == 0 {
		return ""
	}
	if len(content) > limit {
		content = content[:limit]
	}
	parts := make([]string, 0, len(content))
	for _, b := range content {
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	result := strings.Join(parts, " ")
	if len(parts) == limit {
		result += " …"
	}
	return result
}

func pathBase(path string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return ""
	}
	index := strings.LastIndex(trimmed, "/")
	if index < 0 {
		return trimmed
	}
	return trimmed[index+1:]
}
