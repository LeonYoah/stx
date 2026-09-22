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
			s.attachCheckpointSourceStateInspect(
				ctx,
				clusterObj,
				node,
				hostInfo,
				inspectResult,
				path,
				jobConfig,
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
	s.attachCheckpointSourceStateInspect(
		ctx,
		clusterObj,
		node,
		hostInfo,
		inspectResult,
		path,
		jobConfig,
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
