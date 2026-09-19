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
	"fmt"
	"sort"
	"strconv"
	"strings"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/seatunnel"
	"gopkg.in/yaml.v3"
)

// runtimeConfigStore 读写集群配置模板并生成版本。
// runtimeConfigStore reads and writes cluster config templates, creating versions.
type runtimeConfigStore interface {
	GetByCluster(ctx context.Context, clusterID uint) ([]*appconfig.ConfigInfo, error)
	Update(ctx context.Context, id uint, req *appconfig.UpdateConfigRequest, userID uint) (*appconfig.ConfigInfo, error)
}

// ApplyRuntimeStorageRequest 是可视化存储表单提交的草稿。
// ApplyRuntimeStorageRequest is the draft submitted by the visual storage form.
type ApplyRuntimeStorageRequest struct {
	Enabled     bool   `json:"enabled"`
	StorageType string `json:"storage_type"`
	Namespace   string `json:"namespace"`
	Endpoint    string `json:"endpoint"`
	Bucket      string `json:"bucket"`
	AccessKey   string `json:"access_key"`
	SecretKey   string `json:"secret_key"`
	// HDFS 单 NameNode / HA / Kerberos / hdfs-site
	// HDFS single NameNode / HA / Kerberos / hdfs-site
	HDFSNameNodeHost          string `json:"hdfs_namenode_host"`
	HDFSNameNodePort          int    `json:"hdfs_namenode_port"`
	HDFSHAEnabled             bool   `json:"hdfs_ha_enabled"`
	HDFSNameServices          string `json:"hdfs_name_services"`
	HDFSHANamenodes           string `json:"hdfs_ha_namenodes"`
	HDFSNamenodeRPCAddress1   string `json:"hdfs_namenode_rpc_address_1"`
	HDFSNamenodeRPCAddress2   string `json:"hdfs_namenode_rpc_address_2"`
	HDFSFailoverProxyProvider string `json:"hdfs_failover_proxy_provider"`
	KerberosPrincipal         string `json:"kerberos_principal"`
	KerberosKeytabFilePath    string `json:"kerberos_keytab_file_path"`
	HdfsSitePath              string `json:"hdfs_site_path"`
	// DisableCache 对应 disable.cache；nil 不写入。
	// DisableCache maps to disable.cache; nil omits the key.
	DisableCache *bool `json:"disable_cache"`
	// S3 凭证提供方，对应 fs.s3a.aws.credentials.provider。
	// S3 credentials provider, mapped to fs.s3a.aws.credentials.provider.
	S3CredentialsProvider string `json:"s3_credentials_provider"`
}

// ApplyRuntimeStorageVersion 记录本次写回的配置版本。
// ApplyRuntimeStorageVersion records one config version written by this apply.
type ApplyRuntimeStorageVersion struct {
	ConfigType string `json:"config_type"`
	ConfigID   uint   `json:"config_id"`
	Version    int    `json:"version"`
}

// ApplyRuntimeStorageResult 是保存存储配置的结果。
// ApplyRuntimeStorageResult is the result of saving runtime storage settings.
type ApplyRuntimeStorageResult struct {
	Saved           bool                                         `json:"saved"`
	RestartRequired bool                                         `json:"restart_required"`
	Message         string                                       `json:"message"`
	Versions        []ApplyRuntimeStorageVersion                 `json:"versions,omitempty"`
	Validation      *installerapp.RuntimeStorageValidationResult `json:"validation,omitempty"`
}

// SetRuntimeConfigStore 注入配置服务，用于存储可视化与配置版本联动。
// SetRuntimeConfigStore injects the config service used to link storage edits with config versions.
func (s *Service) SetRuntimeConfigStore(store runtimeConfigStore) {
	s.runtimeConfigStore = store
}

// ApplyRuntimeStorage 先做真实读写测试，通过后再写回配置模板并生成新版本。
// ApplyRuntimeStorage runs a real read/write probe first, then writes config templates and creates versions.
func (s *Service) ApplyRuntimeStorage(
	ctx context.Context,
	clusterID uint,
	kind installerapp.RuntimeStorageValidationKind,
	req *ApplyRuntimeStorageRequest,
	userID uint,
) (*ApplyRuntimeStorageResult, error) {
	if s.runtimeConfigStore == nil {
		return nil, fmt.Errorf("runtime storage config store is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	kind = installerapp.RuntimeStorageValidationKind(strings.ToLower(strings.TrimSpace(string(kind))))
	if kind != installerapp.RuntimeStorageValidationCheckpoint && kind != installerapp.RuntimeStorageValidationIMAP {
		return nil, fmt.Errorf("unsupported runtime storage kind: %s", kind)
	}
	req.StorageType = strings.ToUpper(strings.TrimSpace(req.StorageType))
	req.Namespace = strings.TrimSpace(req.Namespace)
	if kind == installerapp.RuntimeStorageValidationIMAP && !req.Enabled {
		req.StorageType = string(installerapp.IMAPStorageDisabled)
	}
	if req.StorageType == "" {
		return nil, fmt.Errorf("storage_type is required")
	}
	if req.Enabled && req.StorageType != string(installerapp.IMAPStorageDisabled) && req.Namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	clusterObj, err := s.Get(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	configs, err := s.runtimeConfigStore.GetByCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	targets := runtimeStorageConfigTargets(configs, kind)
	if len(targets) == 0 {
		return nil, fmt.Errorf("config template not found")
	}

	result := &ApplyRuntimeStorageResult{Saved: false, RestartRequired: false}
	if req.Enabled && req.StorageType != string(installerapp.IMAPStorageDisabled) {
		validation, probeErr := s.probeRuntimeStorageDraft(ctx, clusterObj, kind, req, targets[0].Content)
		result.Validation = validation
		if probeErr != nil {
			return nil, probeErr
		}
		if validation != nil && !validation.Success {
			result.Message = "runtime storage read/write test failed"
			return result, nil
		}
	}

	versions := make([]ApplyRuntimeStorageVersion, 0, len(targets))
	for _, target := range targets {
		next, patchErr := patchRuntimeStorageYAML(target.Content, kind, req)
		if patchErr != nil {
			return nil, patchErr
		}
		updated, updateErr := s.runtimeConfigStore.Update(ctx, target.ID, &appconfig.UpdateConfigRequest{
			Content: next,
			Comment: fmt.Sprintf("update %s runtime storage", kind),
		}, userID)
		if updateErr != nil {
			return nil, updateErr
		}
		versions = append(versions, ApplyRuntimeStorageVersion{
			ConfigType: string(updated.ConfigType),
			ConfigID:   updated.ID,
			Version:    updated.Version,
		})
	}
	result.Saved = true
	result.RestartRequired = true
	result.Versions = versions
	result.Message = "runtime storage saved"
	return result, nil
}

func (s *Service) probeRuntimeStorageDraft(
	ctx context.Context,
	clusterObj *Cluster,
	kind installerapp.RuntimeStorageValidationKind,
	req *ApplyRuntimeStorageRequest,
	currentContent string,
) (*installerapp.RuntimeStorageValidationResult, error) {
	nodes, err := s.GetNodes(ctx, clusterObj.ID)
	if err != nil {
		return nil, err
	}
	checkpoint, imap := draftRuntimeStorageConfigs(kind, req, currentContent)
	result := &installerapp.RuntimeStorageValidationResult{
		Kind:  kind,
		Hosts: make([]*installerapp.RuntimeStorageValidationHostResult, 0),
	}
	result.Success = true
	for _, node := range uniqueNodesByHost(nodes) {
		if node == nil {
			continue
		}
		hostInfo, hostErr := s.hostProvider.GetHostByID(ctx, node.HostID)
		hostName := node.HostName
		if hostInfo != nil && strings.TrimSpace(hostInfo.Name) != "" {
			hostName = hostInfo.Name
		}
		if hostErr != nil || hostInfo == nil || !hostInfo.IsOnline(s.heartbeatTimeout) || strings.TrimSpace(hostInfo.AgentID) == "" {
			result.Success = false
			result.Hosts = append(result.Hosts, &installerapp.RuntimeStorageValidationHostResult{
				HostID: node.HostID, HostName: hostName, Success: false, Message: "host agent is offline",
			})
			continue
		}
		var hostResult *installerapp.RuntimeStorageValidationHostResult
		switch kind {
		case installerapp.RuntimeStorageValidationCheckpoint:
			hostResult = s.runRuntimeStorageProbeOnHost(ctx, clusterObj, node, hostInfo, kind, checkpoint, nil)
		default:
			hostResult = s.runRuntimeStorageProbeOnHost(ctx, clusterObj, node, hostInfo, kind, nil, imap)
		}
		hostResult.HostID = node.HostID
		hostResult.HostName = hostName
		if !hostResult.Success {
			result.Success = false
		}
		result.Hosts = append(result.Hosts, hostResult)
	}
	if len(result.Hosts) == 0 {
		result.Success = false
		result.Warning = "no host available for runtime storage probe"
	}
	return result, nil
}

func draftRuntimeStorageConfigs(kind installerapp.RuntimeStorageValidationKind, req *ApplyRuntimeStorageRequest, currentContent string) (*installerapp.CheckpointConfig, *installerapp.IMAPConfig) {
	accessKey := strings.TrimSpace(req.AccessKey)
	secretKey := strings.TrimSpace(req.SecretKey)
	if kind == installerapp.RuntimeStorageValidationCheckpoint {
		if current := parseCheckpointValidationConfigFromYAML(currentContent); current != nil {
			if accessKey == "" {
				accessKey = current.StorageAccessKey
			}
			if secretKey == "" {
				secretKey = current.StorageSecretKey
			}
		}
		return &installerapp.CheckpointConfig{
			StorageType:               installerapp.CheckpointStorageType(req.StorageType),
			Namespace:                 req.Namespace,
			HDFSNameNodeHost:          strings.TrimSpace(req.HDFSNameNodeHost),
			HDFSNameNodePort:          req.HDFSNameNodePort,
			HDFSHAEnabled:             req.HDFSHAEnabled,
			HDFSNameServices:          strings.TrimSpace(req.HDFSNameServices),
			HDFSHANamenodes:           strings.TrimSpace(req.HDFSHANamenodes),
			HDFSNamenodeRPCAddress1:   strings.TrimSpace(req.HDFSNamenodeRPCAddress1),
			HDFSNamenodeRPCAddress2:   strings.TrimSpace(req.HDFSNamenodeRPCAddress2),
			HDFSFailoverProxyProvider: strings.TrimSpace(req.HDFSFailoverProxyProvider),
			KerberosPrincipal:         strings.TrimSpace(req.KerberosPrincipal),
			KerberosKeytabFilePath:    strings.TrimSpace(req.KerberosKeytabFilePath),
			HdfsSitePath:              strings.TrimSpace(req.HdfsSitePath),
			DisableCache:              req.DisableCache,
			StorageEndpoint:           strings.TrimSpace(req.Endpoint),
			StorageBucket:             strings.TrimSpace(req.Bucket),
			StorageAccessKey:          accessKey,
			StorageSecretKey:          secretKey,
			S3CredentialsProvider:     strings.TrimSpace(req.S3CredentialsProvider),
		}, nil
	}
	if current := parseIMAPValidationConfigFromYAML(currentContent); current != nil {
		if accessKey == "" {
			accessKey = current.StorageAccessKey
		}
		if secretKey == "" {
			secretKey = current.StorageSecretKey
		}
	}
	return nil, &installerapp.IMAPConfig{
		StorageType:               installerapp.IMAPStorageType(req.StorageType),
		Namespace:                 req.Namespace,
		HDFSNameNodeHost:          strings.TrimSpace(req.HDFSNameNodeHost),
		HDFSNameNodePort:          req.HDFSNameNodePort,
		HDFSHAEnabled:             req.HDFSHAEnabled,
		HDFSNameServices:          strings.TrimSpace(req.HDFSNameServices),
		HDFSHANamenodes:           strings.TrimSpace(req.HDFSHANamenodes),
		HDFSNamenodeRPCAddress1:   strings.TrimSpace(req.HDFSNamenodeRPCAddress1),
		HDFSNamenodeRPCAddress2:   strings.TrimSpace(req.HDFSNamenodeRPCAddress2),
		HDFSFailoverProxyProvider: strings.TrimSpace(req.HDFSFailoverProxyProvider),
		KerberosPrincipal:         strings.TrimSpace(req.KerberosPrincipal),
		KerberosKeytabFilePath:    strings.TrimSpace(req.KerberosKeytabFilePath),
		HdfsSitePath:              strings.TrimSpace(req.HdfsSitePath),
		DisableCache:              req.DisableCache,
		StorageEndpoint:           strings.TrimSpace(req.Endpoint),
		StorageBucket:             strings.TrimSpace(req.Bucket),
		StorageAccessKey:          accessKey,
		StorageSecretKey:          secretKey,
		S3CredentialsProvider:     strings.TrimSpace(req.S3CredentialsProvider),
	}
}

func runtimeStorageConfigTargets(configs []*appconfig.ConfigInfo, kind installerapp.RuntimeStorageValidationKind) []*appconfig.ConfigInfo {
	wanted := map[appconfig.ConfigType]struct{}{}
	if kind == installerapp.RuntimeStorageValidationCheckpoint {
		wanted[appconfig.ConfigTypeSeatunnel] = struct{}{}
	} else {
		wanted[appconfig.ConfigTypeHazelcast] = struct{}{}
		wanted[appconfig.ConfigTypeHazelcastMaster] = struct{}{}
	}
	targets := make([]*appconfig.ConfigInfo, 0)
	for _, cfg := range configs {
		if cfg == nil || !cfg.IsTemplate {
			continue
		}
		if _, ok := wanted[cfg.ConfigType]; ok {
			targets = append(targets, cfg)
		}
	}
	return targets
}

func patchRuntimeStorageYAML(content string, kind installerapp.RuntimeStorageValidationKind, req *ApplyRuntimeStorageRequest) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", fmt.Errorf("config yaml root is not a mapping")
	}
	root := doc.Content[0]
	if kind == installerapp.RuntimeStorageValidationCheckpoint {
		plugin := findOrCreateMapping(root, "seatunnel", "engine", "checkpoint", "storage", "plugin-config")
		replaceMapping(plugin, runtimeStoragePluginValues(kind, req, content))
	} else {
		mapStore := findIMAPMapStore(root)
		if mapStore == nil {
			parent := findOrCreateMapping(root, "hazelcast", "map", "engine*")
			mapStore = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMappingChild(parent, "map-store", mapStore)
		}
		setMappingChild(mapStore, "enabled", boolNode(req.Enabled && req.StorageType != string(installerapp.IMAPStorageDisabled)))
		if req.Enabled && req.StorageType != string(installerapp.IMAPStorageDisabled) {
			properties := mappingChild(mapStore, "properties")
			if properties == nil || properties.Kind != yaml.MappingNode {
				properties = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				setMappingChild(mapStore, "properties", properties)
			}
			replaceMapping(properties, runtimeStoragePluginValues(kind, req, content))
		}
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func runtimeStoragePluginValues(kind installerapp.RuntimeStorageValidationKind, req *ApplyRuntimeStorageRequest, currentContent string) map[string]string {
	checkpoint, imap := draftRuntimeStorageConfigs(kind, req, currentContent)
	var cfg *installerapp.CheckpointConfig
	if kind == installerapp.RuntimeStorageValidationCheckpoint {
		cfg = checkpoint
	} else if imap != nil {
		cfg = &installerapp.CheckpointConfig{
			StorageType:               installerapp.CheckpointStorageType(imap.StorageType),
			Namespace:                 imap.Namespace,
			HDFSNameNodeHost:          imap.HDFSNameNodeHost,
			HDFSNameNodePort:          imap.HDFSNameNodePort,
			HDFSHAEnabled:             imap.HDFSHAEnabled,
			HDFSNameServices:          imap.HDFSNameServices,
			HDFSHANamenodes:           imap.HDFSHANamenodes,
			HDFSNamenodeRPCAddress1:   imap.HDFSNamenodeRPCAddress1,
			HDFSNamenodeRPCAddress2:   imap.HDFSNamenodeRPCAddress2,
			HDFSFailoverProxyProvider: imap.HDFSFailoverProxyProvider,
			KerberosPrincipal:         imap.KerberosPrincipal,
			KerberosKeytabFilePath:    imap.KerberosKeytabFilePath,
			HdfsSitePath:              imap.HdfsSitePath,
			DisableCache:              imap.DisableCache,
			StorageEndpoint:           imap.StorageEndpoint,
			StorageAccessKey:          imap.StorageAccessKey,
			StorageSecretKey:          imap.StorageSecretKey,
			StorageBucket:             imap.StorageBucket,
			S3CredentialsProvider:     imap.S3CredentialsProvider,
		}
	}
	if cfg == nil {
		return map[string]string{"namespace": strings.TrimSpace(req.Namespace)}
	}
	return checkpointPluginValues(cfg)
}

// checkpointPluginValues 按 SeaTunnel checkpoint plugin-config 生成差异化键。
// checkpointPluginValues writes SeaTunnel checkpoint plugin-config keys per storage type.
func checkpointPluginValues(cfg *installerapp.CheckpointConfig) map[string]string {
	namespace := strings.TrimSpace(cfg.Namespace)
	values := map[string]string{"namespace": namespace}
	switch string(cfg.StorageType) {
	case string(installerapp.CheckpointStorageS3):
		values["storage.type"] = "s3"
		if bucket := strings.TrimSpace(cfg.StorageBucket); bucket != "" {
			values["s3.bucket"] = bucket
		}
		if endpoint := strings.TrimSpace(cfg.StorageEndpoint); endpoint != "" {
			values["fs.s3a.endpoint"] = endpoint
		}
		if key := strings.TrimSpace(cfg.StorageAccessKey); key != "" {
			values["fs.s3a.access.key"] = key
		}
		if secret := strings.TrimSpace(cfg.StorageSecretKey); secret != "" {
			values["fs.s3a.secret.key"] = secret
		}
		provider := strings.TrimSpace(cfg.S3CredentialsProvider)
		if provider == "" {
			provider = "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider"
		}
		values["fs.s3a.aws.credentials.provider"] = provider
	case string(installerapp.CheckpointStorageOSS):
		values["storage.type"] = "oss"
		if bucket := strings.TrimSpace(cfg.StorageBucket); bucket != "" {
			values["oss.bucket"] = bucket
		}
		if endpoint := strings.TrimSpace(cfg.StorageEndpoint); endpoint != "" {
			values["fs.oss.endpoint"] = endpoint
		}
		if key := strings.TrimSpace(cfg.StorageAccessKey); key != "" {
			values["fs.oss.accessKeyId"] = key
		}
		if secret := strings.TrimSpace(cfg.StorageSecretKey); secret != "" {
			values["fs.oss.accessKeySecret"] = secret
		}
	case string(installerapp.CheckpointStorageHDFS):
		values["storage.type"] = "hdfs"
		if cfg.HDFSHAEnabled {
			nameServices := strings.TrimSpace(cfg.HDFSNameServices)
			values["fs.defaultFS"] = "hdfs://" + nameServices
			if nameServices != "" {
				values["seatunnel.hadoop.dfs.nameservices"] = nameServices
				if namenodes := strings.TrimSpace(cfg.HDFSHANamenodes); namenodes != "" {
					values["seatunnel.hadoop.dfs.ha.namenodes."+nameServices] = namenodes
				}
				if endpoints, err := seatunnel.ResolveHDFSHARPCAddresses(cfg.HDFSHANamenodes, cfg.HDFSNamenodeRPCAddress1, cfg.HDFSNamenodeRPCAddress2); err == nil {
					for _, endpoint := range endpoints {
						values[fmt.Sprintf("seatunnel.hadoop.dfs.namenode.rpc-address.%s.%s", nameServices, endpoint.Name)] = endpoint.Address
					}
				}
				provider := strings.TrimSpace(cfg.HDFSFailoverProxyProvider)
				if provider == "" {
					provider = "org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider"
				}
				values["seatunnel.hadoop.dfs.client.failover.proxy.provider."+nameServices] = provider
			}
		} else if host := strings.TrimSpace(cfg.HDFSNameNodeHost); host != "" && cfg.HDFSNameNodePort > 0 {
			values["fs.defaultFS"] = fmt.Sprintf("hdfs://%s:%d", host, cfg.HDFSNameNodePort)
		}
		if principal := strings.TrimSpace(cfg.KerberosPrincipal); principal != "" {
			values["kerberosPrincipal"] = principal
		}
		if keytab := strings.TrimSpace(cfg.KerberosKeytabFilePath); keytab != "" {
			values["kerberosKeytabFilePath"] = keytab
		}
		if site := strings.TrimSpace(cfg.HdfsSitePath); site != "" {
			values["hdfs_site_path"] = site
		}
		if cfg.DisableCache != nil {
			values["disable.cache"] = strconv.FormatBool(*cfg.DisableCache)
		}
	default:
		values["storage.type"] = "hdfs"
		values["fs.defaultFS"] = "file:///"
	}
	return values
}

func findIMAPMapStore(root *yaml.Node) *yaml.Node {
	if store := findMapping(root, "map", "engine*", "map-store"); store != nil {
		return store
	}
	return findMapping(root, "hazelcast", "map", "engine*", "map-store")
}

func findOrCreateMapping(root *yaml.Node, keys ...string) *yaml.Node {
	current := root
	for _, key := range keys {
		next := mappingChild(current, key)
		if next == nil || next.Kind != yaml.MappingNode {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			setMappingChild(current, key, next)
		}
		current = next
	}
	return current
}

func findMapping(root *yaml.Node, keys ...string) *yaml.Node {
	current := root
	for _, key := range keys {
		next := mappingChild(current, key)
		if next == nil || next.Kind != yaml.MappingNode {
			return nil
		}
		current = next
	}
	return current
}

func mappingChild(parent *yaml.Node, key string) *yaml.Node {
	if parent == nil || parent.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			return parent.Content[i+1]
		}
	}
	return nil
}

func setMappingChild(parent *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1] = value
			return
		}
	}
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

func replaceMapping(parent *yaml.Node, values map[string]string) {
	preferred := []string{
		"storage.type",
		"namespace",
		"fs.defaultFS",
		"disable.cache",
		"hdfs_site_path",
		"kerberosPrincipal",
		"kerberosKeytabFilePath",
		"s3.bucket",
		"fs.s3a.endpoint",
		"fs.s3a.access.key",
		"fs.s3a.secret.key",
		"fs.s3a.aws.credentials.provider",
		"oss.bucket",
		"fs.oss.endpoint",
		"fs.oss.accessKeyId",
		"fs.oss.accessKeySecret",
	}
	seen := map[string]struct{}{}
	keys := make([]string, 0, len(values))
	for _, key := range preferred {
		if _, ok := values[key]; ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	extra := make([]string, 0)
	for key := range values {
		if _, ok := seen[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	keys = append(keys, extra...)
	parent.Content = nil
	parent.Kind = yaml.MappingNode
	parent.Tag = "!!map"
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		setMappingChild(parent, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
}

func (s *Service) loadConfiguredRuntimeStorage(ctx context.Context, clusterID uint) (*RuntimeStorageSpec, *RuntimeStorageSpec) {
	if s == nil || s.runtimeConfigStore == nil {
		return nil, nil
	}
	configs, err := s.runtimeConfigStore.GetByCluster(ctx, clusterID)
	if err != nil {
		return nil, nil
	}
	var checkpoint *RuntimeStorageSpec
	var imap *RuntimeStorageSpec
	for _, cfg := range runtimeStorageConfigTargets(configs, installerapp.RuntimeStorageValidationCheckpoint) {
		checkpoint = parseCheckpointStorageFromYAML(cfg.Content)
		break
	}
	for _, cfg := range runtimeStorageConfigTargets(configs, installerapp.RuntimeStorageValidationIMAP) {
		imap = parseIMAPStorageFromYAML(cfg.Content)
		break
	}
	return checkpoint, imap
}

func boolNode(value bool) *yaml.Node {
	text := "false"
	if value {
		text = "true"
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: text}
}
