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
	"fmt"
	"net"
	neturl "net/url"
	"strconv"
	"strings"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gopkg.in/yaml.v3"
)

type runtimeStorageResolvedConfig struct {
	Kind        string
	StorageType string
	Namespace   string
	Endpoint    string
	Bucket      string
	AccessKey   string
	SecretKey   string
}

func (s *Service) ValidateRuntimeStorage(
	ctx context.Context,
	clusterID uint,
	kind installerapp.RuntimeStorageValidationKind,
) (*installerapp.RuntimeStorageValidationResult, error) {
	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	nodes, err := s.GetNodes(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	uniqueNodes := uniqueNodesByHost(nodes)

	result := &installerapp.RuntimeStorageValidationResult{
		Kind:    kind,
		Warning: "",
		Hosts:   make([]*installerapp.RuntimeStorageValidationHostResult, 0, len(uniqueNodes)),
	}
	result.Success = true

	for _, node := range uniqueNodes {
		if node == nil {
			continue
		}
		hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
		hostName := node.HostName
		if hostInfo != nil && strings.TrimSpace(hostInfo.Name) != "" {
			hostName = hostInfo.Name
		}
		if err != nil || hostInfo == nil || !hostInfo.IsOnline(s.heartbeatTimeout) || strings.TrimSpace(hostInfo.AgentID) == "" {
			result.Success = false
			result.Hosts = append(result.Hosts, &installerapp.RuntimeStorageValidationHostResult{
				HostID:   node.HostID,
				HostName: hostName,
				Success:  false,
				Message:  "host agent is offline",
			})
			continue
		}

		cfg, cfgErr := s.resolveRuntimeStorageValidationConfig(ctx, clusterObj, node, kind)
		if cfgErr != nil {
			result.Success = false
			result.Hosts = append(result.Hosts, &installerapp.RuntimeStorageValidationHostResult{
				HostID:   node.HostID,
				HostName: hostName,
				Success:  false,
				Message:  cfgErr.Error(),
			})
			continue
		}

		var hostResult *installerapp.RuntimeStorageValidationHostResult
		switch kind {
		case installerapp.RuntimeStorageValidationCheckpoint:
			hostResult = s.runRuntimeStorageProbeOnHost(ctx, clusterObj, node, hostInfo, kind, cfg.Checkpoint, nil)
		case installerapp.RuntimeStorageValidationIMAP:
			hostResult = s.runRuntimeStorageProbeOnHost(ctx, clusterObj, node, hostInfo, kind, nil, cfg.IMAP)
		default:
			hostResult = &installerapp.RuntimeStorageValidationHostResult{
				HostID:   node.HostID,
				HostName: hostName,
				Success:  false,
				Message:  fmt.Sprintf("unsupported runtime storage kind: %s", kind),
			}
		}

		hostResult.HostID = node.HostID
		hostResult.HostName = hostName
		if !hostResult.Success {
			result.Success = false
		}
		result.Hosts = append(result.Hosts, hostResult)
	}

	return result, nil
}

func (s *Service) fillRemoteRuntimeStorageStats(ctx context.Context, clusterObj *Cluster, node *NodeInfo, spec *RuntimeStorageSpec) {
	if spec == nil || !spec.Enabled || !spec.External || spec.SizeAvailable {
		return
	}
	if node == nil || s.agentSender == nil || s.hostProvider == nil {
		return
	}
	hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
	if err != nil {
		spec.Warning = firstNonEmpty(spec.Warning, fmt.Sprintf("remote storage statistics unavailable: %v", err))
		return
	}
	if hostInfo == nil || !hostInfo.IsOnline(s.heartbeatTimeout) || strings.TrimSpace(hostInfo.AgentID) == "" {
		spec.Warning = firstNonEmpty(spec.Warning, "remote storage statistics unavailable: host agent is offline")
		return
	}
	kind := installerapp.RuntimeStorageValidationKind(spec.Kind)
	cfg, err := s.resolveRuntimeStorageValidationConfig(ctx, clusterObj, node, kind)
	if err != nil {
		spec.Warning = firstNonEmpty(spec.Warning, fmt.Sprintf("remote storage statistics unavailable: %v", err))
		return
	}
	params := runtimeStorageProxyParams(node.InstallDir, clusterObj.Version, kind, cfg.Checkpoint, cfg.IMAP)
	success, output, sendErr := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "stx_java_proxy_stat", params)
	if sendErr != nil {
		spec.Warning = firstNonEmpty(spec.Warning, fmt.Sprintf("remote storage statistics unavailable: %v", sendErr))
		return
	}
	result := runtimeStorageHostResultFromCommandOutput(success, output)
	if !result.Success {
		if fallbackApplied := s.tryFallbackRemoteRuntimeStorageStats(ctx, node, spec); fallbackApplied {
			return
		}
		spec.Warning = firstNonEmpty(spec.Warning, fmt.Sprintf("remote storage statistics unavailable: %s", result.Message))
		return
	}
	if total, ok := parseInt64Detail(result.Details, "total_size_bytes"); ok {
		spec.TotalSizeBytes = total
		spec.SizeAvailable = true
	}
	if !spec.SizeAvailable {
		if s.tryFallbackRemoteRuntimeStorageStats(ctx, node, spec) {
			return
		}
		spec.Warning = firstNonEmpty(spec.Warning, "remote storage statistics unavailable")
	}
}

func (s *Service) tryFallbackRemoteRuntimeStorageStats(ctx context.Context, node *NodeInfo, spec *RuntimeStorageSpec) bool {
	cfg, err := s.loadRuntimeStorageResolvedConfigFromNode(ctx, node, spec.Kind)
	if err != nil || cfg == nil {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.StorageType)) {
	case "S3":
		totalSize, statErr := statS3RuntimeStorage(ctx, cfg)
		if statErr != nil {
			return false
		}
		spec.TotalSizeBytes = totalSize
		spec.SizeAvailable = true
		spec.Warning = ""
		return true
	default:
		return false
	}
}

func (s *Service) loadRuntimeStorageResolvedConfigFromNode(ctx context.Context, node *NodeInfo, kind string) (*runtimeStorageResolvedConfig, error) {
	switch kind {
	case "checkpoint":
		content, err := s.pullConfigFromNode(ctx, node, "seatunnel.yaml")
		if err != nil {
			return nil, err
		}
		return parseCheckpointResolvedConfigFromYAML(content), nil
	case "imap":
		configType := imapHazelcastConfigType(node.Role)
		content, err := s.pullConfigFromNode(ctx, node, configType)
		if err != nil {
			return nil, err
		}
		return parseIMAPResolvedConfigFromYAML(content), nil
	default:
		return nil, fmt.Errorf("unsupported runtime storage kind: %s", kind)
	}
}

type runtimeStorageValidationConfig struct {
	Checkpoint *installerapp.CheckpointConfig
	IMAP       *installerapp.IMAPConfig
}

func (s *Service) resolveRuntimeStorageValidationConfig(
	ctx context.Context,
	clusterObj *Cluster,
	node *NodeInfo,
	kind installerapp.RuntimeStorageValidationKind,
) (*runtimeStorageValidationConfig, error) {
	if node != nil && node.IsOnline && s.agentSender != nil {
		if cfg, err := s.resolveRuntimeStorageValidationConfigFromNode(ctx, node, kind); err == nil && cfg != nil {
			return cfg, nil
		}
	}
	return resolveRuntimeStorageValidationConfigFromCluster(clusterObj.Config, kind)
}

func (s *Service) resolveRuntimeStorageValidationConfigFromNode(
	ctx context.Context,
	node *NodeInfo,
	kind installerapp.RuntimeStorageValidationKind,
) (*runtimeStorageValidationConfig, error) {
	switch kind {
	case installerapp.RuntimeStorageValidationCheckpoint:
		content, err := s.pullConfigFromNode(ctx, node, "seatunnel.yaml")
		if err != nil {
			return nil, err
		}
		cfg := parseCheckpointValidationConfigFromYAML(content)
		if cfg == nil {
			return nil, fmt.Errorf("checkpoint config is unavailable")
		}
		return &runtimeStorageValidationConfig{Checkpoint: cfg}, nil
	case installerapp.RuntimeStorageValidationIMAP:
		configType := imapHazelcastConfigType(node.Role)
		content, err := s.pullConfigFromNode(ctx, node, configType)
		if err != nil {
			return nil, err
		}
		cfg := parseIMAPValidationConfigFromYAML(content)
		if cfg == nil {
			return nil, fmt.Errorf("imap config is unavailable")
		}
		return &runtimeStorageValidationConfig{IMAP: cfg}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime storage kind: %s", kind)
	}
}

func resolveRuntimeStorageValidationConfigFromCluster(
	cfg ClusterConfig,
	kind installerapp.RuntimeStorageValidationKind,
) (*runtimeStorageValidationConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cluster config is unavailable")
	}
	switch kind {
	case installerapp.RuntimeStorageValidationCheckpoint:
		raw := asMap(cfg["checkpoint"])
		if len(raw) == 0 {
			return nil, fmt.Errorf("checkpoint config is unavailable")
		}
		return &runtimeStorageValidationConfig{Checkpoint: checkpointValidationConfigFromCluster(raw)}, nil
	case installerapp.RuntimeStorageValidationIMAP:
		raw := asMap(cfg["imap"])
		if len(raw) == 0 {
			return nil, fmt.Errorf("imap config is unavailable")
		}
		return &runtimeStorageValidationConfig{IMAP: imapValidationConfigFromCluster(raw)}, nil
	default:
		return nil, fmt.Errorf("unsupported runtime storage kind: %s", kind)
	}
}

func checkpointValidationConfigFromCluster(raw map[string]interface{}) *installerapp.CheckpointConfig {
	return &installerapp.CheckpointConfig{
		StorageType:               installerapp.CheckpointStorageType(strings.ToUpper(asString(raw["storage_type"]))),
		Namespace:                 asString(raw["namespace"]),
		HDFSNameNodeHost:          asString(raw["hdfs_namenode_host"]),
		HDFSNameNodePort:          asInt(raw["hdfs_namenode_port"]),
		KerberosPrincipal:         asString(raw["kerberos_principal"]),
		KerberosKeytabFilePath:    asString(raw["kerberos_keytab_file_path"]),
		HDFSHAEnabled:             asBool(raw["hdfs_ha_enabled"]),
		HDFSNameServices:          asString(raw["hdfs_name_services"]),
		HDFSHANamenodes:           asString(raw["hdfs_ha_namenodes"]),
		HDFSNamenodeRPCAddress1:   asString(raw["hdfs_namenode_rpc_address_1"]),
		HDFSNamenodeRPCAddress2:   asString(raw["hdfs_namenode_rpc_address_2"]),
		HDFSFailoverProxyProvider: asString(raw["hdfs_failover_proxy_provider"]),
		StorageEndpoint:           asString(raw["storage_endpoint"]),
		StorageAccessKey:          asString(raw["storage_access_key"]),
		StorageSecretKey:          asString(raw["storage_secret_key"]),
		StorageBucket:             asString(raw["storage_bucket"]),
	}
}

func imapValidationConfigFromCluster(raw map[string]interface{}) *installerapp.IMAPConfig {
	return &installerapp.IMAPConfig{
		StorageType:               installerapp.IMAPStorageType(strings.ToUpper(asString(raw["storage_type"]))),
		Namespace:                 asString(raw["namespace"]),
		HDFSNameNodeHost:          asString(raw["hdfs_namenode_host"]),
		HDFSNameNodePort:          asInt(raw["hdfs_namenode_port"]),
		KerberosPrincipal:         asString(raw["kerberos_principal"]),
		KerberosKeytabFilePath:    asString(raw["kerberos_keytab_file_path"]),
		HDFSHAEnabled:             asBool(raw["hdfs_ha_enabled"]),
		HDFSNameServices:          asString(raw["hdfs_name_services"]),
		HDFSHANamenodes:           asString(raw["hdfs_ha_namenodes"]),
		HDFSNamenodeRPCAddress1:   asString(raw["hdfs_namenode_rpc_address_1"]),
		HDFSNamenodeRPCAddress2:   asString(raw["hdfs_namenode_rpc_address_2"]),
		HDFSFailoverProxyProvider: asString(raw["hdfs_failover_proxy_provider"]),
		StorageEndpoint:           asString(raw["storage_endpoint"]),
		StorageAccessKey:          asString(raw["storage_access_key"]),
		StorageSecretKey:          asString(raw["storage_secret_key"]),
		StorageBucket:             asString(raw["storage_bucket"]),
	}
}

func parseCheckpointValidationConfigFromYAML(content string) *installerapp.CheckpointConfig {
	resolved := parseCheckpointResolvedConfigFromYAML(content)
	if resolved == nil {
		return nil
	}
	return &installerapp.CheckpointConfig{
		StorageType:      installerapp.CheckpointStorageType(strings.ToUpper(resolved.StorageType)),
		Namespace:        resolved.Namespace,
		StorageEndpoint:  resolved.Endpoint,
		StorageBucket:    resolved.Bucket,
		StorageAccessKey: resolved.AccessKey,
		StorageSecretKey: resolved.SecretKey,
	}
}

func parseIMAPValidationConfigFromYAML(content string) *installerapp.IMAPConfig {
	resolved := parseIMAPResolvedConfigFromYAML(content)
	if resolved == nil {
		return nil
	}
	return &installerapp.IMAPConfig{
		StorageType:      installerapp.IMAPStorageType(strings.ToUpper(resolved.StorageType)),
		Namespace:        resolved.Namespace,
		StorageEndpoint:  resolved.Endpoint,
		StorageBucket:    resolved.Bucket,
		StorageAccessKey: resolved.AccessKey,
		StorageSecretKey: resolved.SecretKey,
	}
}

func parseCheckpointResolvedConfigFromYAML(content string) *runtimeStorageResolvedConfig {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return nil
	}
	pluginConfig := nestedMap(root, "seatunnel", "engine", "checkpoint", "storage", "plugin-config")
	if len(pluginConfig) == 0 {
		return nil
	}
	return resolvedConfigFromPluginConfig("checkpoint", pluginConfig)
}

func parseIMAPResolvedConfigFromYAML(content string) *runtimeStorageResolvedConfig {
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return nil
	}
	engineMapStore := nestedMap(root, "map", "engine*", "map-store")
	if len(engineMapStore) == 0 {
		engineMapStore = nestedMap(root, "hazelcast", "map", "engine*", "map-store")
	}
	if len(engineMapStore) == 0 {
		return nil
	}
	properties := asMap(engineMapStore["properties"])
	if len(properties) == 0 {
		return nil
	}
	return resolvedConfigFromPluginConfig("imap", properties)
}

func resolvedConfigFromPluginConfig(kind string, pluginConfig map[string]interface{}) *runtimeStorageResolvedConfig {
	storageType := strings.ToUpper(asString(pluginConfig["storage.type"]))
	if storageType == "" {
		if fsDefault := asString(pluginConfig["fs.defaultFS"]); strings.HasPrefix(fsDefault, "file:///") {
			storageType = "LOCAL_FILE"
		}
	}
	cfg := &runtimeStorageResolvedConfig{
		Kind:        kind,
		StorageType: storageType,
		Namespace:   asString(pluginConfig["namespace"]),
	}
	switch storageType {
	case "S3":
		cfg.Endpoint = asString(pluginConfig["fs.s3a.endpoint"])
		cfg.Bucket = runtimeStorageFirstNonEmpty(asString(pluginConfig["s3.bucket"]), asString(pluginConfig["fs.defaultFS"]))
		cfg.AccessKey = asString(pluginConfig["fs.s3a.access.key"])
		cfg.SecretKey = asString(pluginConfig["fs.s3a.secret.key"])
	case "OSS":
		cfg.Endpoint = asString(pluginConfig["fs.oss.endpoint"])
		cfg.Bucket = asString(pluginConfig["oss.bucket"])
		cfg.AccessKey = asString(pluginConfig["fs.oss.accessKeyId"])
		cfg.SecretKey = asString(pluginConfig["fs.oss.accessKeySecret"])
	case "HDFS":
		if fsDefault := asString(pluginConfig["fs.defaultFS"]); strings.HasPrefix(fsDefault, "file:///") {
			cfg.StorageType = "LOCAL_FILE"
			cfg.Endpoint = fsDefault
		} else {
			cfg.Endpoint = asString(pluginConfig["fs.defaultFS"])
		}
	default:
		if fsDefault := asString(pluginConfig["fs.defaultFS"]); strings.HasPrefix(fsDefault, "file:///") {
			cfg.StorageType = "LOCAL_FILE"
			cfg.Endpoint = fsDefault
		}
	}
	return cfg
}

func (s *Service) runRuntimeStorageProbeOnHost(
	ctx context.Context,
	clusterObj *Cluster,
	node *NodeInfo,
	host *HostInfo,
	kind installerapp.RuntimeStorageValidationKind,
	checkpoint *installerapp.CheckpointConfig,
	imap *installerapp.IMAPConfig,
) *installerapp.RuntimeStorageValidationHostResult {
	if node == nil || host == nil || strings.TrimSpace(host.AgentID) == "" {
		return &installerapp.RuntimeStorageValidationHostResult{Success: false, Message: "host agent is offline"}
	}
	params := runtimeStorageProxyParams(node.InstallDir, clusterObj.Version, kind, checkpoint, imap)
	success, output, err := s.agentSender.SendCommand(ctx, host.AgentID, "stx_java_proxy_probe", params)
	if err != nil {
		return &installerapp.RuntimeStorageValidationHostResult{Success: false, Message: err.Error()}
	}
	return runtimeStorageHostResultFromCommandOutput(success, output)
}

func runtimeStorageHostResultFromCommandOutput(success bool, output string) *installerapp.RuntimeStorageValidationHostResult {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return &installerapp.RuntimeStorageValidationHostResult{Success: success, Message: ""}
	}
	var parsed struct {
		Success bool              `json:"success"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed.Message != "" {
		return &installerapp.RuntimeStorageValidationHostResult{
			Success: parsed.Success,
			Message: parsed.Message,
			Details: parsed.Details,
		}
	}
	return &installerapp.RuntimeStorageValidationHostResult{
		Success: success,
		Message: trimmed,
	}
}

func statS3RuntimeStorage(ctx context.Context, cfg *runtimeStorageResolvedConfig) (int64, error) {
	if cfg == nil {
		return 0, fmt.Errorf("storage config is nil")
	}
	if strings.TrimSpace(cfg.Endpoint) == "" || strings.TrimSpace(cfg.Bucket) == "" {
		return 0, fmt.Errorf("s3 endpoint or bucket is empty")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return 0, fmt.Errorf("s3 credentials are unavailable")
	}

	parsed, err := neturl.Parse(strings.TrimSpace(cfg.Endpoint))
	if err != nil {
		return 0, err
	}
	host := parsed.Host
	if host == "" {
		host = strings.TrimSpace(cfg.Endpoint)
	}
	bucket := sanitizeObjectStoreBucket(cfg.Bucket)
	prefix := sanitizeObjectStorePrefix(cfg.Namespace)
	client, err := minio.New(host, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       strings.EqualFold(parsed.Scheme, "https"),
		BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return 0, err
	}

	var totalSize int64
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
		UseV1:     true,
	}) {
		if object.Err != nil {
			return 0, object.Err
		}
		totalSize += object.Size
	}
	return totalSize, nil
}

func sanitizeObjectStoreBucket(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "s3a://")
	trimmed = strings.TrimPrefix(trimmed, "s3://")
	trimmed = strings.TrimPrefix(trimmed, "oss://")
	return strings.Trim(trimmed, "/")
}

func sanitizeObjectStorePrefix(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "/")
	return trimmed
}

func parseEndpointHostPort(raw string, defaultPort int) (string, int) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", 0
	}
	if strings.Contains(trimmed, "://") {
		if u, err := neturl.Parse(trimmed); err == nil {
			host := u.Hostname()
			port := defaultPort
			if parsed := u.Port(); parsed != "" {
				if p, convErr := strconv.Atoi(parsed); convErr == nil {
					port = p
				}
			} else if strings.EqualFold(u.Scheme, "http") {
				port = 80
			}
			return host, port
		}
	}
	return splitHostPortWithDefault(trimmed, defaultPort)
}

func runtimeStorageProxyParams(
	installDir string,
	version string,
	kind installerapp.RuntimeStorageValidationKind,
	checkpoint *installerapp.CheckpointConfig,
	imap *installerapp.IMAPConfig,
) map[string]string {
	params := map[string]string{
		"kind":        string(kind),
		"install_dir": installDir,
		"version":     version,
	}
	switch kind {
	case installerapp.RuntimeStorageValidationCheckpoint:
		if checkpoint != nil {
			fillRuntimeStorageProxyParams(params, string(checkpoint.StorageType), checkpoint.Namespace, checkpoint.HDFSNameNodeHost, checkpoint.HDFSNameNodePort, checkpoint.KerberosPrincipal, checkpoint.KerberosKeytabFilePath, checkpoint.HDFSHAEnabled, checkpoint.HDFSNameServices, checkpoint.HDFSHANamenodes, checkpoint.HDFSNamenodeRPCAddress1, checkpoint.HDFSNamenodeRPCAddress2, checkpoint.HDFSFailoverProxyProvider, checkpoint.StorageEndpoint, checkpoint.StorageAccessKey, checkpoint.StorageSecretKey, checkpoint.StorageBucket)
		}
	case installerapp.RuntimeStorageValidationIMAP:
		if imap != nil {
			fillRuntimeStorageProxyParams(params, string(imap.StorageType), imap.Namespace, imap.HDFSNameNodeHost, imap.HDFSNameNodePort, imap.KerberosPrincipal, imap.KerberosKeytabFilePath, imap.HDFSHAEnabled, imap.HDFSNameServices, imap.HDFSHANamenodes, imap.HDFSNamenodeRPCAddress1, imap.HDFSNamenodeRPCAddress2, imap.HDFSFailoverProxyProvider, imap.StorageEndpoint, imap.StorageAccessKey, imap.StorageSecretKey, imap.StorageBucket)
		}
	}
	return params
}

func fillRuntimeStorageProxyParams(
	params map[string]string,
	storageType string,
	namespace string,
	hdfsHost string,
	hdfsPort int,
	kerberosPrincipal string,
	kerberosKeytab string,
	hdfsHAEnabled bool,
	hdfsNameServices string,
	hdfsHANamenodes string,
	hdfsRPC1 string,
	hdfsRPC2 string,
	hdfsFailover string,
	storageEndpoint string,
	storageAccessKey string,
	storageSecretKey string,
	storageBucket string,
) {
	params["storage_type"] = storageType
	params["namespace"] = namespace
	params["hdfs_namenode_host"] = hdfsHost
	params["hdfs_namenode_port"] = strconv.Itoa(hdfsPort)
	params["kerberos_principal"] = kerberosPrincipal
	params["kerberos_keytab_file_path"] = kerberosKeytab
	params["hdfs_ha_enabled"] = strconv.FormatBool(hdfsHAEnabled)
	params["hdfs_name_services"] = hdfsNameServices
	params["hdfs_ha_namenodes"] = hdfsHANamenodes
	params["hdfs_namenode_rpc_address_1"] = hdfsRPC1
	params["hdfs_namenode_rpc_address_2"] = hdfsRPC2
	params["hdfs_failover_proxy_provider"] = hdfsFailover
	params["storage_endpoint"] = storageEndpoint
	params["storage_access_key"] = storageAccessKey
	params["storage_secret_key"] = storageSecretKey
	params["storage_bucket"] = storageBucket
}

func parseInt64Detail(details map[string]string, key string) (int64, bool) {
	if len(details) == 0 {
		return 0, false
	}
	raw := strings.TrimSpace(details[key])
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func splitHostPortWithDefault(raw string, defaultPort int) (string, int) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", 0
	}
	if strings.Count(trimmed, ":") == 1 && !strings.Contains(trimmed, "]") {
		host, portStr, err := net.SplitHostPort(trimmed)
		if err == nil {
			if port, convErr := strconv.Atoi(portStr); convErr == nil {
				return host, port
			}
		}
	}
	if strings.HasPrefix(trimmed, "[") {
		if host, portStr, err := net.SplitHostPort(trimmed); err == nil {
			if port, convErr := strconv.Atoi(portStr); convErr == nil {
				return strings.Trim(host, "[]"), port
			}
		}
	}
	return trimmed, defaultPort
}

func asInt(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func asBool(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}
