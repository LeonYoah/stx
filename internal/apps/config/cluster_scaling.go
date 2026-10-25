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
	"sort"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/db"
)

// ErrNodeConfigAccessUnavailable 未配置 Agent 客户端或节点信息提供者，无法读写节点文件。
// ErrNodeConfigAccessUnavailable means no Agent client or node info provider is configured, so node files
// cannot be read or written.
var ErrNodeConfigAccessUnavailable = errors.New("node config access is not configured / 未配置节点配置读写能力")

// ConfigChangeStatus 是扩缩容配置操作中单个配置的处理结果。
// ConfigChangeStatus is the outcome of one config in a scaling config operation.
type ConfigChangeStatus string

const (
	// ConfigChangeCreated 新建了配置记录。
	// ConfigChangeCreated means a config record was created.
	ConfigChangeCreated ConfigChangeStatus = "created"
	// ConfigChangeUpdated 内容已更新（节点配置已推送并提交）。
	// ConfigChangeUpdated means the content was updated (node configs pushed and committed).
	ConfigChangeUpdated ConfigChangeStatus = "updated"
	// ConfigChangeUnchanged 内容已一致，未产生新版本。
	// ConfigChangeUnchanged means the content already matched and no version was created.
	ConfigChangeUnchanged ConfigChangeStatus = "unchanged"
	// ConfigChangePendingPush 推送失败，新内容已暂存为待推送（Agent 重连后自动重推）。
	// ConfigChangePendingPush means the push failed and the content is parked for retry on reconnect.
	ConfigChangePendingPush ConfigChangeStatus = "pending_push"
	// ConfigChangeFailed 该配置处理失败（解析、校验或推送失败且未建立记录）。
	// ConfigChangeFailed means this config failed (parse, validation, or push failed without a record).
	ConfigChangeFailed ConfigChangeStatus = "failed"
)

// ConfigChangeItem 是单个配置的处理结果。
// ConfigChangeItem is the result of one config.
type ConfigChangeItem struct {
	ConfigID   uint               `json:"config_id,omitempty"`
	ConfigType ConfigType         `json:"config_type"`
	HostID     *uint              `json:"host_id,omitempty"`
	IsTemplate bool               `json:"is_template"`
	Status     ConfigChangeStatus `json:"status"`
	Version    int                `json:"version,omitempty"`
	Message    string             `json:"message,omitempty"`
}

// ConfigChangeResult 汇总一次扩缩容配置操作的逐配置结果；Items 永不为 nil。
// ConfigChangeResult aggregates per-config results of a scaling config operation; Items is never nil.
type ConfigChangeResult struct {
	Items          []*ConfigChangeItem `json:"items"`
	CreatedCount   int                 `json:"created_count"`
	UpdatedCount   int                 `json:"updated_count"`
	UnchangedCount int                 `json:"unchanged_count"`
	PendingCount   int                 `json:"pending_count"`
	FailedCount    int                 `json:"failed_count"`
}

// Partial 表示存在待推送或失败的配置，调用方应把步骤标记为 partial 并允许重试。
// Partial reports pending or failed configs; callers should mark the step partial and allow retry.
func (r *ConfigChangeResult) Partial() bool {
	return r != nil && (r.PendingCount > 0 || r.FailedCount > 0)
}

func newConfigChangeResult() *ConfigChangeResult {
	return &ConfigChangeResult{Items: make([]*ConfigChangeItem, 0)}
}

func (r *ConfigChangeResult) add(item *ConfigChangeItem) {
	r.Items = append(r.Items, item)
	switch item.Status {
	case ConfigChangeCreated:
		r.CreatedCount++
	case ConfigChangeUpdated:
		r.UpdatedCount++
	case ConfigChangeUnchanged:
		r.UnchangedCount++
	case ConfigChangePendingPush:
		r.PendingCount++
	case ConfigChangeFailed:
		r.FailedCount++
	}
}

// MemberListPatchResult 是 PatchMemberList 的结果，包含写入的完整成员列表。
// MemberListPatchResult is the result of PatchMemberList, including the full member list written.
type MemberListPatchResult struct {
	Members []string `json:"members"`
	ConfigChangeResult
}

// templateImportMode 控制从节点导入配置时如何对待集群模板。
// templateImportMode controls how cluster templates are treated when importing configs from a node.
type templateImportMode int

const (
	// templateOverwrite 用节点内容覆盖模板（InitClusterConfigs 的显式“从节点初始化”语义）。
	// templateOverwrite overwrites templates with node content (explicit "init from node" semantics).
	templateOverwrite templateImportMode = iota
	// templateCreateIfMissing 模板不存在才由节点内容创建，已存在则不改动（安装器路径）。
	// templateCreateIfMissing creates a template from node content only when missing (installer path).
	templateCreateIfMissing
	// templateUntouched 完全不触碰模板（纳管已运行节点）。
	// templateUntouched never touches templates (registering an already running node).
	templateUntouched
)

// EnsureClusterConfigsForHost 安装完成后登记节点配置：模板不存在时才由节点文件创建模板；模板已存在时只创建/刷新
// 该主机的配置记录，绝不覆盖模板（修复后续节点安装反向覆盖集群模板的问题）。
// EnsureClusterConfigsForHost registers node configs after installation: a template is created from the node
// file only when missing; when it exists only this host's config records are created/refreshed and the
// template is never overwritten (fixes later installs overwriting the cluster template).
func (s *Service) EnsureClusterConfigsForHost(ctx context.Context, clusterID uint, hostID uint, installDir string, userID uint) error {
	_, err := s.importConfigsFromNode(ctx, clusterID, hostID, installDir, userID, templateCreateIfMissing)
	return err
}

// RegisterHostConfigsFromNode 纳管已运行节点：拉取节点现有配置建立该主机的配置记录，不写节点磁盘、不改集群模板；
// 与模板的差异通过 match_template=false 呈现。
// RegisterHostConfigsFromNode registers an already running node: it pulls the node's current configs into
// this host's config records without writing to the node or touching templates; differences from the
// template show up as match_template=false.
func (s *Service) RegisterHostConfigsFromNode(ctx context.Context, clusterID uint, hostID uint, installDir string, userID uint) (*ConfigChangeResult, error) {
	return s.importConfigsFromNode(ctx, clusterID, hostID, installDir, userID, templateUntouched)
}

// importConfigsFromNode 从节点拉取全部支持的配置并建立/刷新该主机的配置记录，按 mode 处理模板。
// 拉取失败或为空的类型视为节点上不存在而跳过；内容非法时返回错误（与 InitClusterConfigs 原行为一致）。
// importConfigsFromNode pulls every supported config from the node and creates/refreshes this host's
// records, handling templates per mode. Types that fail to pull or are empty are treated as absent and
// skipped; invalid content returns an error (same as the original InitClusterConfigs behavior).
func (s *Service) importConfigsFromNode(ctx context.Context, clusterID uint, hostID uint, installDir string, userID uint, mode templateImportMode) (*ConfigChangeResult, error) {
	result := newConfigChangeResult()
	if s.agentClient == nil {
		return result, ErrNodeConfigAccessUnavailable
	}
	for _, configType := range SupportedConfigTypes {
		content, err := s.agentClient.PullConfig(ctx, hostID, installDir, configType)
		if err != nil || content == "" {
			// 某些配置文件可能不存在，跳过。
			// Some config files may not exist on the node; skip them.
			continue
		}
		if err := validateConfigContent(configType, content); err != nil {
			return result, err
		}

		if mode != templateUntouched {
			item, err := s.importTemplateFromNode(ctx, clusterID, configType, content, userID, mode == templateOverwrite)
			if err != nil {
				return result, err
			}
			if item != nil {
				result.add(item)
			}
		}

		item, err := s.importNodeConfigFromNode(ctx, clusterID, hostID, configType, content, userID)
		if err != nil {
			return result, err
		}
		result.add(item)
	}
	return result, nil
}

// importTemplateFromNode 按需创建或覆盖模板；overwrite=false 且模板已存在时返回 nil（不产生结果项）。
// importTemplateFromNode creates or overwrites a template; with overwrite=false and an existing template it
// returns nil (no result item).
func (s *Service) importTemplateFromNode(ctx context.Context, clusterID uint, configType ConfigType, content string, userID uint, overwrite bool) (*ConfigChangeItem, error) {
	existing, err := s.repo.GetTemplate(ctx, clusterID, configType)
	if err != nil && !errors.Is(err, ErrConfigNotFound) {
		return nil, err
	}
	if err == nil && existing != nil {
		if !overwrite {
			return nil, nil
		}
		item := &ConfigChangeItem{ConfigID: existing.ID, ConfigType: configType, IsTemplate: true, Status: ConfigChangeUnchanged, Version: existing.Version}
		if existing.Content.String() == content {
			return item, nil
		}
		existing.Content = db.ScriptText(content)
		existing.Version = existing.Version + 1
		existing.UpdatedBy = userID
		existing.UpdatedAt = time.Now()
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, err
		}
		if err := s.repo.CreateVersion(ctx, &ConfigVersion{
			ConfigID:  existing.ID,
			Version:   existing.Version,
			Content:   db.ScriptText(content),
			Comment:   "Updated from node sync",
			CreatedBy: userID,
		}); err != nil {
			return nil, err
		}
		item.Status = ConfigChangeUpdated
		item.Version = existing.Version
		return item, nil
	}

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
		return nil, err
	}
	if err := s.repo.CreateVersion(ctx, &ConfigVersion{
		ConfigID:  template.ID,
		Version:   1,
		Content:   db.ScriptText(content),
		Comment:   "Initial config from installation",
		CreatedBy: userID,
	}); err != nil {
		return nil, err
	}
	return &ConfigChangeItem{ConfigID: template.ID, ConfigType: configType, IsTemplate: true, Status: ConfigChangeCreated, Version: 1}, nil
}

// importNodeConfigFromNode 以节点真实内容创建或刷新该主机的配置记录（不推送，节点文件即真相）。
// importNodeConfigFromNode creates or refreshes this host's record with the node's real content (no push;
// the node file is the truth).
func (s *Service) importNodeConfigFromNode(ctx context.Context, clusterID uint, hostID uint, configType ConfigType, content string, userID uint) (*ConfigChangeItem, error) {
	hostRef := hostID
	existing, err := s.repo.GetNodeConfig(ctx, clusterID, hostID, configType)
	if err != nil && !errors.Is(err, ErrConfigNotFound) {
		return nil, err
	}
	if err == nil && existing != nil {
		item := &ConfigChangeItem{ConfigID: existing.ID, ConfigType: configType, HostID: &hostRef, Status: ConfigChangeUnchanged, Version: existing.Version}
		if existing.Content.String() == content {
			return item, nil
		}
		existing.Content = db.ScriptText(content)
		existing.Version = existing.Version + 1
		existing.UpdatedBy = userID
		existing.UpdatedAt = time.Now()
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, err
		}
		if err := s.repo.CreateVersion(ctx, &ConfigVersion{
			ConfigID:  existing.ID,
			Version:   existing.Version,
			Content:   db.ScriptText(content),
			Comment:   "Updated from node sync",
			CreatedBy: userID,
		}); err != nil {
			return nil, err
		}
		item.Status = ConfigChangeUpdated
		item.Version = existing.Version
		return item, nil
	}

	record, err := s.createNodeConfigRecord(ctx, clusterID, hostID, configType, content, "Initial config from installation", userID)
	if err != nil {
		return nil, err
	}
	return &ConfigChangeItem{ConfigID: record.ID, ConfigType: configType, HostID: &hostRef, Status: ConfigChangeCreated, Version: record.Version}, nil
}

// createNodeConfigRecord 创建版本 1 的节点配置记录及其版本行。
// createNodeConfigRecord creates a version-1 node config record and its version row.
func (s *Service) createNodeConfigRecord(ctx context.Context, clusterID uint, hostID uint, configType ConfigType, content, comment string, userID uint) (*Config, error) {
	hostRef := hostID
	record := &Config{
		ClusterID:  clusterID,
		HostID:     &hostRef,
		ConfigType: configType,
		FilePath:   GetConfigFilePath(configType),
		Content:    db.ScriptText(content),
		Version:    1,
		UpdatedBy:  userID,
	}
	err := s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.Create(ctx, record); err != nil {
			return err
		}
		return tx.CreateVersion(ctx, &ConfigVersion{
			ConfigID:  record.ID,
			Version:   1,
			Content:   db.ScriptText(content),
			Comment:   truncateComment(comment),
			CreatedBy: userID,
		})
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

// ApplyTemplatesToHost 安装模式：以集群模板为该主机生成配置并推送写盘（节点启动前调用）。members 非空时，
// 部署模式对应的 hazelcast*.yaml（与 PatchMemberList 相同的类型集合）的 member-list 先替换为该完整列表。主机尚无记录时先导入节点现有文件作为版本 1（保留真实历史），
// 再按“先推送后提交”应用模板；节点上也没有该文件时直接推送，成功后建立记录。调用前节点必须已登记（用于解析安装目录）。
// ApplyTemplatesToHost (install mode) renders this host's configs from the cluster templates and pushes them
// to disk before the node starts. When members is non-empty, the member-list of hazelcast*.yaml is replaced
// with that full list first. A host without a record first imports the node's current file as version 1
// (keeping the real history) and then applies the template push-then-commit; when the node has no such file
// the template is pushed directly and the record is created on success. The node must already be registered
// so its install directory can be resolved. Only the member-list types of the deployment mode (the same set as
// PatchMemberList) get the member list replaced.
func (s *Service) ApplyTemplatesToHost(ctx context.Context, clusterID uint, hostID uint, deploymentMode string, members []string, opRef string, userID uint) (*ConfigChangeResult, error) {
	result := newConfigChangeResult()
	if s.agentClient == nil || s.nodeInfoProvider == nil {
		return result, ErrNodeConfigAccessUnavailable
	}
	installDir, err := s.nodeInfoProvider.GetNodeInstallDir(ctx, clusterID, hostID)
	if err != nil {
		return result, fmt.Errorf("resolve node install dir / 解析节点安装目录: %w", err)
	}
	if strings.TrimSpace(installDir) == "" {
		return result, fmt.Errorf("node install dir is empty for cluster %d host %d / 集群 %d 主机 %d 的安装目录为空", clusterID, hostID, clusterID, hostID)
	}

	templates, err := s.repo.List(ctx, &ConfigFilter{ClusterID: clusterID, OnlyTemplate: true})
	if err != nil {
		return result, err
	}
	sortConfigsForScaling(templates)

	memberTypes := make(map[ConfigType]struct{})
	for _, configType := range MemberListConfigTypes(deploymentMode) {
		memberTypes[configType] = struct{}{}
	}
	comment := scalingComment("Applied cluster template / 应用集群模板", opRef)
	for _, template := range templates {
		hostRef := hostID
		item := &ConfigChangeItem{ConfigType: template.ConfigType, HostID: &hostRef}

		target := template.Content.String()
		if _, isMemberType := memberTypes[template.ConfigType]; isMemberType && len(normalizeMembers(members)) > 0 {
			patched, _, patchErr := PatchMemberListContent(target, members)
			if patchErr != nil {
				item.Status = ConfigChangeFailed
				item.Message = patchErr.Error()
				result.add(item)
				continue
			}
			target = patched
		}
		if validateErr := validateConfigContent(template.ConfigType, target); validateErr != nil {
			item.Status = ConfigChangeFailed
			item.Message = validateErr.Error()
			result.add(item)
			continue
		}

		record, err := s.repo.GetNodeConfig(ctx, clusterID, hostID, template.ConfigType)
		if err != nil && !errors.Is(err, ErrConfigNotFound) {
			return result, err
		}
		if errors.Is(err, ErrConfigNotFound) {
			record = nil
			pulled, pullErr := s.agentClient.PullConfig(ctx, hostID, installDir, template.ConfigType)
			if pullErr == nil && strings.TrimSpace(pulled) != "" && validateConfigContent(template.ConfigType, pulled) == nil {
				created, createErr := s.createNodeConfigRecord(ctx, clusterID, hostID, template.ConfigType, pulled, scalingComment("Imported from node before applying template / 应用模板前导入节点现有配置", opRef), userID)
				if createErr != nil {
					return result, createErr
				}
				record = created
			} else {
				// 节点上没有该文件：直接推送模板内容，成功后才建立记录，避免记录与节点不一致。
				// The node has no such file: push the template content directly and create the record only on
				// success so the record never disagrees with the node.
				if pushErr := s.agentClient.PushConfig(ctx, hostID, installDir, template.ConfigType, target); pushErr != nil {
					item.Status = ConfigChangeFailed
					item.Message = "推送配置到节点失败 / failed to push config to node: " + pushErr.Error()
					result.add(item)
					continue
				}
				created, createErr := s.createNodeConfigRecord(ctx, clusterID, hostID, template.ConfigType, target, comment, userID)
				if createErr != nil {
					return result, createErr
				}
				s.syncDerivedRuntimeMetadata(ctx, clusterID, created.HostID, created.ConfigType, target)
				item.ConfigID = created.ID
				item.Status = ConfigChangeCreated
				item.Version = created.Version
				result.add(item)
				continue
			}
		}

		if err := s.applyScalingContent(ctx, record, target, comment, userID, item); err != nil {
			return result, err
		}
		result.add(item)
	}
	return result, nil
}

// PatchMemberList 按部署模式对集群模板与每个主机配置分别只修补 hazelcast.network.join.tcp-ip.member-list，
// 内容变化才生成新版本（comment 带 opRef），主机配置按“先推送后提交”写入节点磁盘，不重启任何节点。
// deploymentMode 为空时修补全部 hazelcast*.yaml。推送失败的配置标记待推送，结果 Partial() 为 true，可重试。
// PatchMemberList patches only hazelcast.network.join.tcp-ip.member-list of the cluster templates and of
// every host config for the deployment mode; a new version (comment carries opRef) is created only when the
// content changes, host configs are written to disk push-then-commit, and no node is restarted. An empty
// deploymentMode patches every hazelcast*.yaml. Failed pushes are parked as pending, Partial() is true and the
// call can be retried.
func (s *Service) PatchMemberList(ctx context.Context, clusterID uint, deploymentMode string, members []string, opRef string, userID uint) (*MemberListPatchResult, error) {
	cleaned := normalizeMembers(members)
	result := &MemberListPatchResult{Members: cleaned, ConfigChangeResult: *newConfigChangeResult()}
	if len(cleaned) == 0 {
		return result, ErrEmptyMemberList
	}

	targetTypes := make(map[ConfigType]struct{})
	for _, configType := range MemberListConfigTypes(deploymentMode) {
		targetTypes[configType] = struct{}{}
	}
	configs, err := s.repo.ListByCluster(ctx, clusterID)
	if err != nil {
		return result, err
	}
	targets := make([]*Config, 0, len(configs))
	for _, config := range configs {
		if _, ok := targetTypes[config.ConfigType]; ok {
			targets = append(targets, config)
		}
	}
	sortConfigsForScaling(targets)

	comment := scalingComment("Patched member-list / 修补成员列表", opRef)
	for _, config := range targets {
		item := &ConfigChangeItem{ConfigID: config.ID, ConfigType: config.ConfigType, HostID: config.HostID, IsTemplate: config.IsTemplate(), Version: config.Version}

		base := config.Content.String()
		if config.HasPendingPush() {
			// 保留用户尚未下发的变更，只在其上修补成员列表。
			// Keep the user's not-yet-pushed change and patch the member list on top of it.
			base = config.PendingContent.String()
		}
		patched, _, patchErr := PatchMemberListContent(base, cleaned)
		if patchErr != nil {
			item.Status = ConfigChangeFailed
			item.Message = patchErr.Error()
			result.add(item)
			continue
		}

		if config.IsTemplate() {
			if patched == config.Content.String() {
				item.Status = ConfigChangeUnchanged
			} else {
				if err := s.commitConfigVersion(ctx, config, patched, comment, userID); err != nil {
					return result, err
				}
				item.Status = ConfigChangeUpdated
				item.Version = config.Version
			}
			result.add(item)
			continue
		}

		if err := s.applyScalingContent(ctx, config, patched, comment, userID, item); err != nil {
			return result, err
		}
		result.add(item)
	}
	return result, nil
}

// applyScalingContent 把主机配置同步到目标内容（先推送后提交），并把结果写入 item。
// applyScalingContent syncs a host config to the target content (push-then-commit) and fills item.
func (s *Service) applyScalingContent(ctx context.Context, config *Config, target, comment string, userID uint, item *ConfigChangeItem) error {
	item.ConfigID = config.ID
	switch {
	case config.Content.String() == target && !config.HasPendingPush():
		item.Status = ConfigChangeUnchanged
	case config.Content.String() == target:
		// 节点内容已是目标值，旧的待推送内容已过期，清除即可。
		// The node already has the target; the stale pending change is obsolete, just clear it.
		clearPendingPush(config)
		if err := s.repo.Update(ctx, config); err != nil {
			return err
		}
		item.Status = ConfigChangeUpdated
	default:
		outcome, err := s.applyNodeConfigContent(ctx, config, target, comment, userID)
		if err != nil {
			return err
		}
		if outcome.Pending {
			item.Status = ConfigChangePendingPush
			item.Message = outcome.PushError
		} else {
			item.Status = ConfigChangeUpdated
		}
	}
	item.Version = config.Version
	return nil
}

// sortConfigsForScaling 稳定排序：模板在前，其次按主机 ID、配置类型，保证结果顺序可预期且与数据库方言无关。
// sortConfigsForScaling sorts templates first, then by host ID and config type, so the order is predictable
// and independent of the database dialect.
func sortConfigsForScaling(configs []*Config) {
	sort.SliceStable(configs, func(i, j int) bool {
		left, right := configs[i], configs[j]
		if left.IsTemplate() != right.IsTemplate() {
			return left.IsTemplate()
		}
		if !left.IsTemplate() && *left.HostID != *right.HostID {
			return *left.HostID < *right.HostID
		}
		return left.ConfigType < right.ConfigType
	})
}

// scalingComment 生成带伸缩操作引用的版本说明，长度受版本表 comment 列限制。
// scalingComment builds a version comment carrying the scaling operation reference, bounded by the column size.
func scalingComment(base, opRef string) string {
	if ref := strings.TrimSpace(opRef); ref != "" {
		base = base + " [" + ref + "]"
	}
	return truncateComment(base)
}

// truncateComment 把版本说明截断到 255 个字符以内。
// truncateComment truncates a version comment to at most 255 characters.
func truncateComment(comment string) string {
	const maxLen = 255
	runes := []rune(comment)
	if len(runes) <= maxLen {
		return comment
	}
	return string(runes[:maxLen])
}

// DeleteHostConfigs 删除某主机在集群中的全部配置记录及其版本（缩容后该主机已无本集群节点时调用），不触碰集群模板与节点磁盘文件。
// 返回删除的配置记录数；主机没有记录时返回 0。
// DeleteHostConfigs deletes every config record of a host in a cluster together with its versions (called after
// scale-in once the host has no node of the cluster left); templates and files on the node are untouched. It
// returns how many config records were deleted, 0 when the host had none.
func (s *Service) DeleteHostConfigs(ctx context.Context, clusterID uint, hostID uint) (int, error) {
	if clusterID == 0 || hostID == 0 {
		return 0, fmt.Errorf("cluster id and host id are required / 集群 ID 与主机 ID 必填")
	}
	hostRef := hostID
	return s.deleteHostRecords(ctx, &ConfigFilter{ClusterID: clusterID, HostID: &hostRef})
}

// DeleteConfigsByHost 删除某主机在所有集群中的配置记录及其版本（主机被删除时调用：只有不再属于任何集群的主机才允许删除，
// 其配置记录均为孤儿，否则 sync-all 会永远把它们标记为待推送）。模板不受影响。
// DeleteConfigsByHost deletes a host's config records and versions across all clusters (called when the host is
// deleted: only hosts that no longer belong to any cluster can be deleted, so these records are orphans that sync-all
// would otherwise keep marking as pending push). Templates are untouched.
func (s *Service) DeleteConfigsByHost(ctx context.Context, hostID uint) (int, error) {
	if hostID == 0 {
		return 0, fmt.Errorf("host id is required / 主机 ID 必填")
	}
	hostRef := hostID
	return s.deleteHostRecords(ctx, &ConfigFilter{HostID: &hostRef})
}

// deleteHostRecords 在一个事务中删除过滤到的主机配置记录及其版本，跳过模板；返回删除的记录数。
// deleteHostRecords deletes the filtered host config records and their versions in one transaction, skipping
// templates; returns how many records were deleted.
func (s *Service) deleteHostRecords(ctx context.Context, filter *ConfigFilter) (int, error) {
	configs, err := s.repo.List(ctx, filter)
	if err != nil {
		return 0, err
	}
	deleted := 0
	err = s.repo.Transaction(ctx, func(tx *Repository) error {
		for _, record := range configs {
			// 只删除主机记录；模板 HostID 为空，不会被主机过滤条件选中，这里再做一次防御。
			// Only host records are deleted; templates have a nil HostID and never match the host filter, checked again defensively.
			if record == nil || record.IsTemplate() {
				continue
			}
			if err := tx.DeleteVersionsByConfig(ctx, record.ID); err != nil {
				return err
			}
			if err := tx.Delete(ctx, record.ID); err != nil {
				return err
			}
			deleted++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}
