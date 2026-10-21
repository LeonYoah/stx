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

package config

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/db"
	"gopkg.in/yaml.v3"
)

var (
	ErrTemplateNotFound      = errors.New("configuration template not found")
	ErrCannotPromoteTemplate = errors.New("cannot promote template config")
	ErrCannotSyncTemplate    = errors.New("cannot sync template config from itself")
)

// HostProvider 主机信息提供者接口
type HostProvider interface {
	GetHostByID(ctx context.Context, id uint) (*HostInfo, error)
}

// NodeInfoProvider 节点信息提供者接口
type NodeInfoProvider interface {
	GetNodeInstallDir(ctx context.Context, clusterID uint, hostID uint) (string, error)
}

// HostInfo 主机信息
type HostInfo struct {
	ID        uint
	Name      string
	IPAddress string
}

// AgentClient Agent 客户端接口
type AgentClient interface {
	PullConfig(ctx context.Context, hostID uint, installDir string, configType ConfigType) (string, error)
	PushConfig(ctx context.Context, hostID uint, installDir string, configType ConfigType, content string) error
}

// PortMetadataUpdater updates cluster node API port metadata after config changes.
// PortMetadataUpdater 在配置变更后更新集群节点 API 端口元数据。
type PortMetadataUpdater interface {
	UpdateSeatunnelAPIPortByHost(ctx context.Context, clusterID uint, hostID uint, port int) error
	UpdateHazelcastPortByHost(ctx context.Context, clusterID uint, hostID uint, configType ConfigType, port int) error
	UpdateClusterJobLogMode(ctx context.Context, clusterID uint, mode string) error
}

// Service 配置管理服务
type Service struct {
	repo             *Repository
	hostProvider     HostProvider
	nodeInfoProvider NodeInfoProvider
	agentClient      AgentClient
	portUpdater      PortMetadataUpdater
}

// NewService 创建配置服务实例
func NewService(repo *Repository, hostProvider HostProvider, nodeInfoProvider NodeInfoProvider, agentClient AgentClient) *Service {
	return &Service{
		repo:             repo,
		hostProvider:     hostProvider,
		nodeInfoProvider: nodeInfoProvider,
		agentClient:      agentClient,
	}
}

// SetPortMetadataUpdater sets the cluster port metadata updater.
// SetPortMetadataUpdater 设置集群端口元数据更新器。
func (s *Service) SetPortMetadataUpdater(updater PortMetadataUpdater) {
	s.portUpdater = updater
}

// Get 获取配置详情
func (s *Service) Get(ctx context.Context, id uint) (*ConfigInfo, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toConfigInfo(ctx, config)
}

// NormalizeContent normalizes one config content when it is parseable and valid.
// NormalizeContent 对可解析且合法的配置内容做规范化处理。
func (s *Service) NormalizeContent(_ context.Context, req *NormalizeConfigRequest) (string, error) {
	return normalizeConfigContent(req.ConfigType, req.Content)
}

// GetByCluster 获取集群所有配置
func (s *Service) GetByCluster(ctx context.Context, clusterID uint) ([]*ConfigInfo, error) {
	configs, err := s.repo.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	// 获取模板配置用于比较
	templates := make(map[ConfigType]*Config)
	for _, c := range configs {
		if c.IsTemplate() {
			templates[c.ConfigType] = c
		}
	}

	result := make([]*ConfigInfo, 0, len(configs))
	for _, c := range configs {
		info, err := s.toConfigInfo(ctx, c)
		if err != nil {
			return nil, err
		}
		// 检查是否与模板一致
		if !c.IsTemplate() {
			if tpl, ok := templates[c.ConfigType]; ok {
				info.MatchTemplate = c.Content == tpl.Content
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// Create 创建配置
func (s *Service) Create(ctx context.Context, req *CreateConfigRequest, userID uint) (*ConfigInfo, error) {
	config := &Config{
		ClusterID:  req.ClusterID,
		HostID:     req.HostID,
		ConfigType: req.ConfigType,
		FilePath:   GetConfigFilePath(req.ConfigType),
		Content:    db.ScriptText(req.Content),
		Version:    1,
		UpdatedBy:  userID,
	}

	err := s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.Create(ctx, config); err != nil {
			return err
		}
		// 创建初始版本
		version := &ConfigVersion{
			ConfigID:  config.ID,
			Version:   1,
			Content:   db.ScriptText(req.Content),
			Comment:   req.Comment,
			CreatedBy: userID,
		}
		return tx.CreateVersion(ctx, version)
	})
	if err != nil {
		return nil, err
	}

	return s.toConfigInfo(ctx, config)
}

// Update 更新配置。节点配置先推送到节点，推送成功才提交新版本；推送失败时新内容标记为待推送，
// 版本与当前内容保持节点上的真实值，Agent 重连后自动重推。
// Update updates a config. Node configs are pushed first and committed as a new version only on success;
// on push failure the new content is marked pending, version/content keep the node's real value, and the
// push is retried when the Agent reconnects.
func (s *Service) Update(ctx context.Context, id uint, req *UpdateConfigRequest, userID uint) (*ConfigInfo, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := validateConfigContent(config.ConfigType, req.Content); err != nil {
		return nil, err
	}

	// 内容没变化则不更新；若存在待推送内容，说明用户放弃了未下发的变更，清除待推送标记。
	// Unchanged content: nothing to commit; a pending change is discarded since the user reverted it.
	if config.Content.String() == req.Content {
		if config.HasPendingPush() {
			clearPendingPush(config)
			if err := s.repo.Update(ctx, config); err != nil {
				return nil, err
			}
		}
		return s.toConfigInfo(ctx, config)
	}

	outcome, err := s.applyNodeConfigContent(ctx, config, req.Content, req.Comment, userID)
	if err != nil {
		return nil, err
	}

	info, err := s.toConfigInfo(ctx, config)
	if err != nil {
		return nil, err
	}
	info.PushError = outcome.PushError
	if config.ConfigType == ConfigTypeLog4j2 && outcome.Applied {
		s.syncDerivedRuntimeMetadata(ctx, config.ClusterID, config.HostID, config.ConfigType, config.Content.String())
	}

	return info, nil
}

// GetVersions 获取版本历史
func (s *Service) GetVersions(ctx context.Context, configID uint) ([]*ConfigVersionInfo, error) {
	versions, err := s.repo.ListVersions(ctx, configID)
	if err != nil {
		return nil, err
	}

	result := make([]*ConfigVersionInfo, len(versions))
	for i, v := range versions {
		result[i] = &ConfigVersionInfo{
			ID:        v.ID,
			ConfigID:  v.ConfigID,
			Version:   v.Version,
			Content:   v.Content.String(),
			Comment:   v.Comment,
			CreatedBy: v.CreatedBy,
			CreatedAt: v.CreatedAt,
		}
	}
	return result, nil
}

// Rollback 回滚到指定版本（节点配置先推送后提交）
// Rollback rolls back to a version (node configs: push first, then commit).
func (s *Service) Rollback(ctx context.Context, id uint, req *RollbackConfigRequest, userID uint) (*ConfigInfo, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 获取目标版本
	targetVersion, err := s.repo.GetVersion(ctx, id, req.Version)
	if err != nil {
		return nil, err
	}

	if err := validateConfigContent(config.ConfigType, targetVersion.Content.String()); err != nil {
		return nil, err
	}

	comment := req.Comment
	if comment == "" {
		comment = fmt.Sprintf("Rollback to version %d", req.Version)
	}

	outcome, err := s.applyNodeConfigContent(ctx, config, targetVersion.Content.String(), comment, userID)
	if err != nil {
		return nil, err
	}

	info, err := s.toConfigInfo(ctx, config)
	if err != nil {
		return nil, err
	}
	info.PushError = outcome.PushError
	return info, nil
}

// Promote 推广配置到集群（节点配置 → 集群模板 → 所有节点）。
// 模板在事务内更新；各节点按“先推送后提交”同步，推送失败的节点标记待推送。
// Promote promotes a node config to the cluster (node config → template → all nodes). The template is
// updated in a transaction; nodes are synced push-then-commit, and nodes whose push fails are marked pending.
func (s *Service) Promote(ctx context.Context, id uint, req *PromoteConfigRequest, userID uint) (*SyncAllResult, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 不能推广模板配置
	if config.IsTemplate() {
		return nil, ErrCannotPromoteTemplate
	}

	if err := validateConfigContent(config.ConfigType, config.Content.String()); err != nil {
		return nil, err
	}

	err = s.repo.Transaction(ctx, func(tx *Repository) error {
		// 1. 更新或创建集群模板
		template, err := tx.GetTemplate(ctx, config.ClusterID, config.ConfigType)
		if errors.Is(err, ErrConfigNotFound) {
			// 创建模板
			template = &Config{
				ClusterID:  config.ClusterID,
				HostID:     nil,
				ConfigType: config.ConfigType,
				FilePath:   config.FilePath,
				Content:    config.Content,
				Version:    1,
				UpdatedBy:  userID,
			}
			if err := tx.Create(ctx, template); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			// 更新模板
			template.Content = config.Content
			template.Version = template.Version + 1
			template.UpdatedBy = userID
			template.UpdatedAt = time.Now()
			if err := tx.Update(ctx, template); err != nil {
				return err
			}
		}

		// 创建模板版本
		templateVersion := &ConfigVersion{
			ConfigID:  template.ID,
			Version:   template.Version,
			Content:   config.Content,
			Comment:   req.Comment,
			CreatedBy: userID,
		}
		return tx.CreateVersion(ctx, templateVersion)
	})
	if err != nil {
		return nil, err
	}

	// 2. 同步到所有节点配置（先推送后提交）
	nodeConfigs, err := s.repo.ListNodeConfigs(ctx, config.ClusterID, config.ConfigType)
	if err != nil {
		return nil, err
	}
	return s.syncNodesToContent(ctx, nodeConfigs, config.Content.String(), "Synced from cluster template", userID)
}

// SyncFromTemplate 从集群模板同步到节点（先推送后提交）
// SyncFromTemplate syncs a node config from the cluster template (push first, then commit).
func (s *Service) SyncFromTemplate(ctx context.Context, id uint, req *SyncConfigRequest, userID uint) (*ConfigInfo, error) {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// 不能同步模板配置
	if config.IsTemplate() {
		return nil, ErrCannotSyncTemplate
	}

	// 获取模板
	template, err := s.repo.GetTemplate(ctx, config.ClusterID, config.ConfigType)
	if err != nil {
		return nil, ErrTemplateNotFound
	}

	if err := validateConfigContent(config.ConfigType, template.Content.String()); err != nil {
		return nil, err
	}

	comment := req.Comment
	if comment == "" {
		comment = "Synced from cluster template"
	}

	result, err := s.syncNodesToContent(ctx, []*Config{config}, template.Content.String(), comment, userID)
	if err != nil {
		return nil, err
	}
	info, err := s.toConfigInfo(ctx, config)
	if err != nil {
		return nil, err
	}
	if result.PendingCount > 0 && len(result.PushErrors) > 0 {
		info.PushError = result.PushErrors[0].Message
	}
	return info, nil
}

// InitClusterConfigs 初始化集群配置（安装完成后调用）
func (s *Service) InitClusterConfigs(ctx context.Context, clusterID uint, hostID uint, installDir string, userID uint) error {
	for _, configType := range SupportedConfigTypes {
		// 从节点拉取配置
		content, err := s.agentClient.PullConfig(ctx, hostID, installDir, configType)
		if err != nil {
			continue // 某些配置文件可能不存在，跳过
		}

		if content == "" {
			continue // 空内容跳过
		}

		if err := validateConfigContent(configType, content); err != nil {
			return err
		}

		// 检查模板是否已存在
		existingTemplate, err := s.repo.GetTemplate(ctx, clusterID, configType)
		if err == nil && existingTemplate != nil {
			// 模板已存在，更新内容
			if existingTemplate.Content.String() != content {
				existingTemplate.Content = db.ScriptText(content)
				existingTemplate.Version = existingTemplate.Version + 1
				existingTemplate.UpdatedBy = userID
				existingTemplate.UpdatedAt = time.Now()
				if err := s.repo.Update(ctx, existingTemplate); err != nil {
					return err
				}
				// 创建新版本
				templateVersion := &ConfigVersion{
					ConfigID:  existingTemplate.ID,
					Version:   existingTemplate.Version,
					Content:   db.ScriptText(content),
					Comment:   "Updated from node sync",
					CreatedBy: userID,
				}
				if err := s.repo.CreateVersion(ctx, templateVersion); err != nil {
					return err
				}
			}
		} else {
			// 创建集群模板
			template := &Config{
				ClusterID:  clusterID,
				HostID:     nil,
				ConfigType: configType,
				FilePath:   GetConfigFilePath(configType),
				Content:    db.ScriptText(content),
				Version:    1,
				UpdatedBy:  userID,
			}
			if err := s.repo.Create(ctx, template); err != nil {
				return err
			}

			// 创建模板版本
			templateVersion := &ConfigVersion{
				ConfigID:  template.ID,
				Version:   1,
				Content:   db.ScriptText(content),
				Comment:   "Initial config from installation",
				CreatedBy: userID,
			}
			if err := s.repo.CreateVersion(ctx, templateVersion); err != nil {
				return err
			}
		}

		// 检查节点配置是否已存在
		existingNodeConfig, err := s.repo.GetNodeConfig(ctx, clusterID, hostID, configType)
		if err == nil && existingNodeConfig != nil {
			// 节点配置已存在，更新内容
			if existingNodeConfig.Content.String() != content {
				existingNodeConfig.Content = db.ScriptText(content)
				existingNodeConfig.Version = existingNodeConfig.Version + 1
				existingNodeConfig.UpdatedBy = userID
				existingNodeConfig.UpdatedAt = time.Now()
				if err := s.repo.Update(ctx, existingNodeConfig); err != nil {
					return err
				}
				// 创建新版本
				nodeVersion := &ConfigVersion{
					ConfigID:  existingNodeConfig.ID,
					Version:   existingNodeConfig.Version,
					Content:   db.ScriptText(content),
					Comment:   "Updated from node sync",
					CreatedBy: userID,
				}
				if err := s.repo.CreateVersion(ctx, nodeVersion); err != nil {
					return err
				}
			}
		} else {
			// 创建节点配置
			nodeConfig := &Config{
				ClusterID:  clusterID,
				HostID:     &hostID,
				ConfigType: configType,
				FilePath:   GetConfigFilePath(configType),
				Content:    db.ScriptText(content),
				Version:    1,
				UpdatedBy:  userID,
			}
			if err := s.repo.Create(ctx, nodeConfig); err != nil {
				return err
			}

			// 创建节点版本
			nodeVersion := &ConfigVersion{
				ConfigID:  nodeConfig.ID,
				Version:   1,
				Content:   db.ScriptText(content),
				Comment:   "Initial config from installation",
				CreatedBy: userID,
			}
			if err := s.repo.CreateVersion(ctx, nodeVersion); err != nil {
				return err
			}
		}
	}
	return nil
}

// SyncTemplateToAllNodes 将集群模板同步到所有节点配置：逐节点先推送后提交，推送失败的节点标记待推送且版本不变。
// SyncTemplateToAllNodes syncs the cluster template to every node config: push-then-commit per node; nodes
// whose push fails are marked pending and keep their version.
func (s *Service) SyncTemplateToAllNodes(ctx context.Context, clusterID uint, configType ConfigType, userID uint) (*SyncAllResult, error) {
	// 获取模板
	template, err := s.repo.GetTemplate(ctx, clusterID, configType)
	if err != nil {
		return nil, ErrTemplateNotFound
	}

	if err := validateConfigContent(configType, template.Content.String()); err != nil {
		return nil, err
	}

	// 获取所有节点配置
	nodeConfigs, err := s.repo.ListNodeConfigs(ctx, clusterID, configType)
	if err != nil {
		return nil, err
	}

	return s.syncNodesToContent(ctx, nodeConfigs, template.Content.String(), "Synced from cluster template (batch)", userID)
}

// PushConfigToNode 推送配置到节点；若存在待推送内容则推送待推送内容，成功后提交为新版本。
// PushConfigToNode pushes the config to its node; when a pending change exists it pushes that change and
// commits it as a new version on success.
func (s *Service) PushConfigToNode(ctx context.Context, id uint, installDir string) error {
	config, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if config.HostID == nil {
		return errors.New("cannot push template config directly")
	}

	content := config.Content.String()
	if config.HasPendingPush() {
		content = config.PendingContent.String()
	}
	if err := validateConfigContent(config.ConfigType, content); err != nil {
		return err
	}

	if err := s.agentClient.PushConfig(ctx, *config.HostID, installDir, config.ConfigType, content); err != nil {
		if config.HasPendingPush() {
			config.SyncError = truncateSyncError("推送配置到节点失败 / failed to push config to node: " + err.Error())
			_ = s.repo.Update(ctx, config)
		}
		return err
	}
	if config.HasPendingPush() {
		comment := config.PendingComment
		if comment == "" {
			comment = "Re-pushed pending change / 重推待推送变更"
		}
		if content == config.Content.String() {
			clearPendingPush(config)
			if err := s.repo.Update(ctx, config); err != nil {
				return err
			}
		} else if err := s.commitConfigVersion(ctx, config, content, comment, config.PendingBy); err != nil {
			return err
		}
		s.syncDerivedRuntimeMetadata(ctx, config.ClusterID, config.HostID, config.ConfigType, content)
	}
	return nil
}

// toConfigInfo 转换为 ConfigInfo
func (s *Service) toConfigInfo(ctx context.Context, config *Config) (*ConfigInfo, error) {
	info := &ConfigInfo{
		ID:         config.ID,
		ClusterID:  config.ClusterID,
		HostID:     config.HostID,
		ConfigType: config.ConfigType,
		FilePath:   config.FilePath,
		Content:    config.Content.String(),
		Version:    config.Version,
		IsTemplate: config.IsTemplate(),
		UpdatedAt:  config.UpdatedAt,
		UpdatedBy:  config.UpdatedBy,
	}
	if !config.IsTemplate() {
		info.SyncStatus = ConfigSyncStatusSynced
		if config.HasPendingPush() {
			info.SyncStatus = ConfigSyncStatusPendingPush
			info.SyncError = config.SyncError
			info.PendingContent = config.PendingContent.String()
			info.PendingSince = config.PendingSince
		}
	}

	// 获取主机信息
	if config.HostID != nil && s.hostProvider != nil {
		host, err := s.hostProvider.GetHostByID(ctx, *config.HostID)
		if err == nil {
			info.HostName = host.Name
			info.HostIP = host.IPAddress
		}
	}

	return info, nil
}

func (s *Service) syncDerivedRuntimeMetadata(ctx context.Context, clusterID uint, hostID *uint, configType ConfigType, content string) {
	if s.portUpdater == nil {
		return
	}
	switch configType {
	case ConfigTypeSeatunnel:
		if hostID == nil {
			return
		}
		port, ok, err := extractSeatunnelHTTPPort(content)
		if err != nil || !ok || port <= 0 {
			return
		}
		_ = s.portUpdater.UpdateSeatunnelAPIPortByHost(ctx, clusterID, *hostID, port)
	case ConfigTypeHazelcast, ConfigTypeHazelcastMaster, ConfigTypeHazelcastWorker:
		if hostID == nil {
			return
		}
		port, ok, err := extractHazelcastNetworkPort(content)
		if err != nil || !ok || port <= 0 {
			return
		}
		_ = s.portUpdater.UpdateHazelcastPortByHost(ctx, clusterID, *hostID, configType, port)
	case ConfigTypeLog4j2:
		mode, ok := extractJobLogMode(content)
		if !ok {
			return
		}
		_ = s.portUpdater.UpdateClusterJobLogMode(ctx, clusterID, mode)
	}
}

func extractSeatunnelHTTPPort(content string) (int, bool, error) {
	if strings.TrimSpace(content) == "" {
		return 0, false, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return 0, false, fmt.Errorf("invalid seatunnel yaml: %w", err)
	}
	if len(root.Content) == 0 {
		return 0, false, nil
	}
	seatunnel := findYAMLMapChild(root.Content[0], "seatunnel")
	if seatunnel == nil {
		return 0, false, nil
	}
	engine := findYAMLMapChild(seatunnel, "engine")
	if engine == nil {
		return 0, false, nil
	}
	httpNode := findYAMLMapChild(engine, "http")
	if httpNode == nil {
		return 0, false, nil
	}
	portNode := findYAMLMapChild(httpNode, "port")
	if portNode == nil {
		return 0, false, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(portNode.Value))
	if err != nil {
		return 0, false, err
	}
	return port, true, nil
}

func findYAMLMapChild(parent *yaml.Node, key string) *yaml.Node {
	if parent == nil || parent.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == key {
			return parent.Content[i+1]
		}
	}
	return nil
}

func extractHazelcastNetworkPort(content string) (int, bool, error) {
	if strings.TrimSpace(content) == "" {
		return 0, false, nil
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(content), &root); err != nil {
		return 0, false, fmt.Errorf("invalid hazelcast yaml: %w", err)
	}
	if len(root.Content) == 0 {
		return 0, false, nil
	}
	hazelcast := findYAMLMapChild(root.Content[0], "hazelcast")
	if hazelcast == nil {
		return 0, false, nil
	}
	network := findYAMLMapChild(hazelcast, "network")
	if network == nil {
		return 0, false, nil
	}
	portNode := findYAMLMapChild(network, "port")
	if portNode == nil {
		return 0, false, nil
	}
	if portNode.Kind == yaml.MappingNode {
		portValue := findYAMLMapChild(portNode, "port")
		if portValue == nil {
			return 0, false, nil
		}
		portNode = portValue
	}
	port, err := strconv.Atoi(strings.TrimSpace(portNode.Value))
	if err != nil {
		return 0, false, err
	}
	return port, true, nil
}

func extractJobLogMode(content string) (string, bool) {
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "rootLogger.appenderRef.file.ref") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return "", false
		}
		value := strings.TrimSpace(parts[1])
		if strings.EqualFold(value, "routingAppender") {
			return "per_job", true
		}
		return "mixed", true
	}
	return "", false
}
