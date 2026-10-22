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

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/LeonYoah/stx/internal/logger"
)

// ErrPluginReplayUnavailable 未配置 Agent 命令发送器或主机信息获取器，无法向节点补齐插件。
// ErrPluginReplayUnavailable means no Agent command sender or host info getter is configured, so plugins
// cannot be replayed to a node.
var ErrPluginReplayUnavailable = errors.New("plugin replay is not configured / 未配置插件补齐能力")

// 插件补齐的逐插件结果状态。
// Per-plugin outcome of a plugin replay.
const (
	// PluginReplayInstalled 已向节点传输并安装。
	// PluginReplayInstalled means the plugin was transferred and installed on the node.
	PluginReplayInstalled = "installed"
	// PluginReplayAlreadyPresent 节点上已有该连接器，未重复下发。
	// PluginReplayAlreadyPresent means the node already has the connector, so it was not sent again.
	PluginReplayAlreadyPresent = "already_present"
	// PluginReplaySkipped 插件记录状态不需要补齐（如 available 等非安装态）。
	// PluginReplaySkipped means the record status does not require replay (e.g. a non-installed state such as available).
	PluginReplaySkipped = "skipped"
	// PluginReplayFailed 补齐失败，可单独重试。
	// PluginReplayFailed means the replay failed and can be retried on its own.
	PluginReplayFailed = "failed"
)

// PluginReplayResult 是单个插件的补齐结果。
// PluginReplayResult is the replay result of one plugin.
type PluginReplayResult struct {
	PluginName  string   `json:"plugin_name"`
	ArtifactID  string   `json:"artifact_id"`
	Version     string   `json:"version"`
	ProfileKeys []string `json:"profile_keys"`
	// ProfileUnknown 表示控制面本地缺少该插件的画像元数据，已按默认画像下发。
	// ProfileUnknown means the Control Plane has no local profile metadata for the plugin, so the default
	// profile was used.
	ProfileUnknown bool `json:"profile_unknown"`
	// ProfileInferred 表示元数据缺失时，按控制面缓存中已存在的依赖反推出画像。
	// ProfileInferred means metadata was missing and the profile was inferred from dependencies cached on the
	// Control Plane.
	ProfileInferred bool   `json:"profile_inferred"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
}

// PluginReplayReport 汇总一次向主机补齐集群插件的结果；Results 永不为 nil。
// PluginReplayReport aggregates one replay of cluster plugins to a host; Results is never nil.
type PluginReplayReport struct {
	ClusterID      uint                 `json:"cluster_id"`
	HostID         uint                 `json:"host_id"`
	InstallDir     string               `json:"install_dir"`
	Results        []PluginReplayResult `json:"results"`
	InstalledCount int                  `json:"installed_count"`
	PresentCount   int                  `json:"present_count"`
	SkippedCount   int                  `json:"skipped_count"`
	FailedCount    int                  `json:"failed_count"`
	// NodeListError 读取节点现有插件失败时的原因；此时按“全部缺失”补齐（重复下发会覆盖同名文件）。
	// NodeListError is why listing the node's plugins failed; every plugin is then treated as missing
	// (re-sending overwrites the same file).
	NodeListError string `json:"node_list_error,omitempty"`
}

// Partial 表示存在补齐失败的插件，调用方应把步骤标记为 partial 并允许重试。
// Partial reports failed plugins; callers should mark the step partial and allow retry.
func (r *PluginReplayReport) Partial() bool {
	return r != nil && r.FailedCount > 0
}

func (r *PluginReplayReport) add(result PluginReplayResult) {
	if result.ProfileKeys == nil {
		result.ProfileKeys = []string{}
	}
	r.Results = append(r.Results, result)
	switch result.Status {
	case PluginReplayInstalled:
		r.InstalledCount++
	case PluginReplayAlreadyPresent:
		r.PresentCount++
	case PluginReplaySkipped:
		r.SkippedCount++
	case PluginReplayFailed:
		r.FailedCount++
	}
}

// nodePluginListResult 对应 Agent LIST_PLUGINS 命令的输出。
// nodePluginListResult mirrors the output of the Agent LIST_PLUGINS command.
type nodePluginListResult struct {
	Success bool `json:"success"`
	Plugins []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"plugins"`
}

// ReplayClusterPluginsToHost 把集群插件记录（installed / enabled / disabled / partial，即除「已卸载 / 安装失败」外的全部，
// 这两种情况记录已被删除）补齐到新加入的主机：disabled 也要补，因为启用只改库，新节点缺 jar 会让日后重新启用失效；
// partial 本就应在所有节点上。先通过 LIST_PLUGINS 读取节点已有连接器，只补缺失项，不删除节点上多出的插件；画像取自
// 控制面本地插件元数据，缺失时按默认画像下发并标注 profile_unknown。单个插件失败不会中断其余插件，结果逐插件返回。
// ReplayClusterPluginsToHost replays the cluster's plugin records (installed / enabled / disabled / partial, i.e.
// everything except uninstalled or failed installs, whose records are deleted) to a newly joined host. Disabled
// plugins are replayed too because enabling only flips the database, so a node missing the jar would break a later
// re-enable; partial plugins belong on every node anyway. It lists
// the node's connectors via LIST_PLUGINS and only sends what is missing, never deleting extra plugins on the
// node. Profiles come from the Control Plane's local plugin metadata; without it the default profile is used
// and the result is marked profile_unknown. One failing plugin does not stop the others; results are
// reported per plugin.
func (s *Service) ReplayClusterPluginsToHost(ctx context.Context, clusterID uint, hostID uint, installDir string) (*PluginReplayReport, error) {
	report := &PluginReplayReport{ClusterID: clusterID, HostID: hostID, InstallDir: installDir, Results: make([]PluginReplayResult, 0)}
	if s.agentCommandSender == nil || s.hostInfoGetter == nil {
		return report, ErrPluginReplayUnavailable
	}
	if strings.TrimSpace(installDir) == "" {
		return report, fmt.Errorf("install dir is required / 安装目录不能为空")
	}
	agentID, err := s.hostInfoGetter.GetHostAgentID(ctx, hostID)
	if err != nil {
		return report, fmt.Errorf("resolve agent of host %d / 解析主机 %d 的 Agent: %w", hostID, hostID, err)
	}
	if strings.TrimSpace(agentID) == "" {
		return report, fmt.Errorf("agent not installed on host %d / 主机 %d 未安装 Agent", hostID, hostID)
	}

	plugins, err := s.ListInstalledPlugins(ctx, clusterID)
	if err != nil {
		return report, err
	}

	present, listErr := s.listNodeConnectors(ctx, agentID, installDir)
	if listErr != nil {
		report.NodeListError = listErr.Error()
		present = make(map[string]struct{})
		logger.WarnF(ctx, "[Plugin] 读取节点插件失败，按全部缺失补齐 / Failed to list node plugins, replaying all: cluster=%d, host=%d, err=%v", clusterID, hostID, listErr)
	}

	// 伴生连接器（由主插件依赖带出）排在主插件之后，主插件下发后它们通常已存在，避免重复传输。
	// Companion connectors (brought in by a primary plugin's dependencies) go after primaries; they usually
	// exist once the primary was sent, which avoids duplicate transfers.
	companions := make(map[string]struct{})
	for _, installed := range plugins {
		for _, artifactID := range installed.AttachedConnectors {
			if trimmed := strings.TrimSpace(artifactID); trimmed != "" {
				companions[trimmed] = struct{}{}
			}
		}
	}
	sort.SliceStable(plugins, func(i, j int) bool {
		_, leftCompanion := companions[replayArtifactID(plugins[i])]
		_, rightCompanion := companions[replayArtifactID(plugins[j])]
		return !leftCompanion && rightCompanion
	})

	for _, installed := range plugins {
		_, isCompanion := companions[replayArtifactID(installed)]
		result := s.replayOnePlugin(ctx, agentID, installDir, installed, isCompanion, present)
		report.add(result)
	}

	logger.InfoF(ctx, "[Plugin] 插件补齐完成 / Plugin replay finished: cluster=%d, host=%d, installed=%d, present=%d, skipped=%d, failed=%d",
		clusterID, hostID, report.InstalledCount, report.PresentCount, report.SkippedCount, report.FailedCount)
	return report, nil
}

// PluginReplayCandidate 是扩容计划中展示的一个待补齐插件（只读，不访问节点）。
// PluginReplayCandidate is one plugin shown in a scale-out plan as to-be-replayed (read-only, no node access).
type PluginReplayCandidate struct {
	PluginName  string       `json:"name"`
	ArtifactID  string       `json:"artifact_id"`
	Version     string       `json:"version"`
	Status      PluginStatus `json:"status"`
	ProfileKeys []string     `json:"profile_keys"`
	// ProfileUnknown 表示控制面本地缺少画像元数据，执行时会按默认画像下发。
	// ProfileUnknown means the Control Plane lacks local profile metadata; the default profile will be used.
	ProfileUnknown bool `json:"profile_unknown"`
	// ProfileInferred 表示画像由控制面缓存中的依赖反推得出。
	// ProfileInferred means the profile was inferred from dependencies cached on the Control Plane.
	ProfileInferred bool `json:"profile_inferred"`
}

// ListReplayCandidates 列出新节点入群时需要补齐的集群插件（与 ReplayClusterPluginsToHost 相同的状态范围与画像来源），
// 供扩容计划展示与计划指纹使用；结果按插件名稳定排序，永不为 nil。
// ListReplayCandidates lists the cluster plugins a joining node must receive (same status scope and profile source
// as ReplayClusterPluginsToHost) for plan display and the plan fingerprint; sorted by plugin name and never nil.
func (s *Service) ListReplayCandidates(ctx context.Context, clusterID uint) ([]PluginReplayCandidate, error) {
	candidates := make([]PluginReplayCandidate, 0)
	plugins, err := s.ListInstalledPlugins(ctx, clusterID)
	if err != nil {
		return candidates, err
	}
	companions := make(map[string]struct{})
	for _, installed := range plugins {
		for _, artifactID := range installed.AttachedConnectors {
			if trimmed := strings.TrimSpace(artifactID); trimmed != "" {
				companions[trimmed] = struct{}{}
			}
		}
	}
	for _, installed := range plugins {
		if !isReplayablePluginStatus(installed.Status) {
			continue
		}
		artifactID := replayArtifactID(installed)
		version := strings.TrimSpace(installed.Version)
		candidate := PluginReplayCandidate{
			PluginName:  installed.PluginName,
			ArtifactID:  artifactID,
			Version:     version,
			Status:      installed.Status,
			ProfileKeys: []string{},
		}
		metadata, metaErr := s.downloader.readPluginMetadata(installed.PluginName, version)
		if metaErr == nil && metadata != nil {
			candidate.ProfileKeys = normalizeProfileKeys(metadata.SelectedProfileKeys)
			if candidate.ProfileKeys == nil {
				candidate.ProfileKeys = []string{}
			}
		} else if _, isCompanion := companions[artifactID]; !isCompanion {
			if inferred := s.inferProfileKeysFromCache(ctx, installed.PluginName, version); len(inferred) > 0 {
				candidate.ProfileKeys = inferred
				candidate.ProfileInferred = true
			} else {
				candidate.ProfileUnknown = true
			}
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].PluginName != candidates[j].PluginName {
			return candidates[i].PluginName < candidates[j].PluginName
		}
		return candidates[i].Version < candidates[j].Version
	})
	return candidates, nil
}

// replayOnePlugin 补齐单个插件；成功后把它及其伴生连接器记为已存在。
// replayOnePlugin replays one plugin and, on success, marks it and its companion connectors as present.
func (s *Service) replayOnePlugin(ctx context.Context, agentID, installDir string, installed InstalledPlugin, isCompanion bool, present map[string]struct{}) PluginReplayResult {
	artifactID := replayArtifactID(installed)
	version := strings.TrimSpace(installed.Version)
	result := PluginReplayResult{
		PluginName: installed.PluginName,
		ArtifactID: artifactID,
		Version:    version,
	}

	if !isReplayablePluginStatus(installed.Status) {
		result.Status = PluginReplaySkipped
		result.Message = fmt.Sprintf("plugin status %s is not replayed / 插件状态 %s 不补齐", installed.Status, installed.Status)
		return result
	}
	_, connectorPresent := present[connectorKey(artifactID, version)]

	// 画像只来自控制面本地元数据；伴生连接器本身没有画像。
	// Profiles only come from local Control Plane metadata; companion connectors have no profile themselves.
	metadata, metaErr := s.downloader.readPluginMetadata(installed.PluginName, version)
	if metaErr == nil && metadata != nil {
		result.ProfileKeys = normalizeProfileKeys(metadata.SelectedProfileKeys)
	} else if !isCompanion {
		if inferred := s.inferProfileKeysFromCache(ctx, installed.PluginName, version); len(inferred) > 0 {
			result.ProfileKeys = inferred
			result.ProfileInferred = true
		} else {
			result.ProfileUnknown = true
		}
	}

	deps, depErr := s.GetPluginDependenciesForVersionAndProfiles(ctx, installed.PluginName, version, result.ProfileKeys)
	if depErr != nil {
		result.Status = PluginReplayFailed
		result.Message = fmt.Sprintf("resolve dependencies / 解析依赖失败: %v", depErr)
		return result
	}

	// 连接器已在节点上：只补传非连接器依赖（如驱动 jar）。节点无法列出依赖目录，重复传输同名文件是幂等覆盖；
	// 没有此类依赖时视为已存在。
	// The connector is already on the node: only resend non-connector dependencies (e.g. driver jars). The node cannot
	// list its dependency directories, and resending a file of the same name is an idempotent overwrite; without such
	// dependencies the plugin counts as already present.
	if connectorPresent {
		extraDeps := make([]PluginDependency, 0, len(deps))
		for _, dep := range deps {
			if strings.TrimSpace(dep.TargetDir) != "connectors" {
				extraDeps = append(extraDeps, dep)
			}
		}
		if len(extraDeps) == 0 {
			result.Status = PluginReplayAlreadyPresent
			return result
		}
		if !s.arePluginDependenciesDownloaded(&Plugin{Version: version, Dependencies: extraDeps}) {
			if err := s.DownloadPluginSync(ctx, installed.PluginName, version, "", result.ProfileKeys); err != nil {
				result.Status = PluginReplayFailed
				result.Message = fmt.Sprintf("prepare local plugin files / 准备本地插件文件失败: %v", err)
				return result
			}
		}
		if err := s.transferDependenciesToAgent(ctx, agentID, installed.PluginName, version, installDir, extraDeps); err != nil {
			result.Status = PluginReplayFailed
			result.Message = err.Error()
			return result
		}
		result.Status = PluginReplayInstalled
		result.Message = fmt.Sprintf("connector present; %d dependency file(s) synced / 连接器已存在，已补传 %d 个依赖", len(extraDeps), len(extraDeps))
		return result
	}

	// 本地缓存缺失连接器或依赖时先下载，避免传输阶段静默跳过依赖。
	// Download first when the local cache lacks the connector or dependencies so the transfer never silently
	// skips a dependency.
	if !s.downloader.IsConnectorDownloadedByArtifactID(artifactID, version) || !s.arePluginDependenciesDownloaded(&Plugin{Version: version, Dependencies: deps}) {
		if err := s.DownloadPluginSync(ctx, installed.PluginName, version, "", result.ProfileKeys); err != nil {
			result.Status = PluginReplayFailed
			result.Message = fmt.Sprintf("prepare local plugin files / 准备本地插件文件失败: %v", err)
			return result
		}
	}

	if err := s.transferPluginToAgent(ctx, agentID, artifactID, installed.PluginName, version, installDir, deps); err != nil {
		result.Status = PluginReplayFailed
		result.Message = err.Error()
		logger.WarnF(ctx, "[Plugin] 插件补齐失败 / Plugin replay failed: plugin=%s, version=%s, err=%v", installed.PluginName, version, err)
		return result
	}

	result.Status = PluginReplayInstalled
	present[connectorKey(artifactID, version)] = struct{}{}
	for _, dep := range deps {
		if strings.TrimSpace(dep.TargetDir) != "connectors" {
			continue
		}
		depVersion := strings.TrimSpace(dep.Version)
		if depVersion == "" {
			depVersion = version
		}
		present[connectorKey(dep.ArtifactID, depVersion)] = struct{}{}
	}
	return result
}

// transferDependenciesToAgent 只把依赖文件传到节点（连接器已存在时使用）；任一依赖缺失或传输失败即返回错误，
// 不像整包传输那样跳过，避免「看似补齐、实则缺驱动」。
// transferDependenciesToAgent transfers only dependency files to the node (used when the connector is present); a
// missing or failed dependency returns an error instead of being skipped like the full transfer does, so the replay
// never looks complete while a driver is missing.
func (s *Service) transferDependenciesToAgent(ctx context.Context, agentID, pluginName, version, installDir string, deps []PluginDependency) error {
	if s.agentCommandSender == nil {
		return fmt.Errorf("agent command sender not configured / Agent 命令发送器未配置")
	}
	for _, dep := range deps {
		if !s.downloader.IsDependencyDownloaded(dep.ArtifactID, dep.Version, version, dep.TargetDir) {
			return fmt.Errorf("dependency %s-%s is not cached / 依赖 %s-%s 未缓存", dep.ArtifactID, dep.Version, dep.ArtifactID, dep.Version)
		}
		depData, err := s.readFile(s.downloader.GetDependencyPath(dep.ArtifactID, dep.Version, version, dep.TargetDir))
		if err != nil {
			return fmt.Errorf("read dependency %s / 读取依赖 %s 失败: %w", dep.ArtifactID, dep.ArtifactID, err)
		}
		depFileName := fmt.Sprintf("%s-%s.jar", dep.ArtifactID, dep.Version)
		if err := s.transferFileToAgent(ctx, agentID, pluginName, version, "dependency", dep.TargetDir, depFileName, depData, installDir); err != nil {
			return fmt.Errorf("transfer dependency %s / 传输依赖 %s 失败: %w", dep.ArtifactID, dep.ArtifactID, err)
		}
	}
	return nil
}

// inferProfileKeysFromCache 在画像元数据缺失时，按控制面本地缓存反推当初选择的画像：某个非默认画像相对默认画像额外引入的
// 依赖若全部已在本地缓存中，就视为已选择。缓存内容来自集群当初的安装，因此重放的依赖与已有节点一致；子集画像被同时选中
// 只会重复传输已有文件，不会引入额外依赖。
// inferProfileKeysFromCache infers the originally selected profiles from the local Control Plane cache when profile
// metadata is missing: a non-default profile counts as selected when every dependency it adds over the default
// profile is already cached. The cache comes from the cluster's original install, so replayed dependencies match the
// existing nodes; selecting a subset profile as well only resends cached files and never adds new dependencies.
func (s *Service) inferProfileKeysFromCache(ctx context.Context, pluginName, version string) []string {
	if s.repo == nil || s.downloader == nil {
		return nil
	}
	profiles, err := s.repo.ListDependencyProfilesByPlugin(ctx, pluginName)
	if err != nil || len(profiles) == 0 {
		return nil
	}
	baseline, err := s.GetPluginDependenciesForVersionAndProfiles(ctx, pluginName, version, nil)
	if err != nil {
		return nil
	}
	baseKeys := make(map[string]struct{}, len(baseline))
	for _, dep := range baseline {
		baseKeys[dependencyIdentity(dep)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(profiles))
	inferred := make([]string, 0)
	for _, profile := range profiles {
		key := strings.TrimSpace(profile.ProfileKey)
		if key == "" || profile.IsDefault || profile.NoAdditionalDependencies {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deps, depErr := s.GetPluginDependenciesForVersionAndProfiles(ctx, pluginName, version, []string{key})
		if depErr != nil {
			continue
		}
		extra := make([]PluginDependency, 0, len(deps))
		for _, dep := range deps {
			if _, ok := baseKeys[dependencyIdentity(dep)]; !ok {
				extra = append(extra, dep)
			}
		}
		if len(extra) == 0 {
			continue
		}
		if s.arePluginDependenciesDownloaded(&Plugin{Version: version, Dependencies: extra}) {
			inferred = append(inferred, key)
		}
	}
	sort.Strings(inferred)
	return inferred
}

// dependencyIdentity 返回依赖的唯一标识（groupId:artifactId:version@targetDir）。
// dependencyIdentity returns a dependency's identity (groupId:artifactId:version@targetDir).
func dependencyIdentity(dep PluginDependency) string {
	return strings.TrimSpace(dep.GroupID) + ":" + strings.TrimSpace(dep.ArtifactID) + ":" + strings.TrimSpace(dep.Version) + "@" + strings.TrimSpace(dep.TargetDir)
}

// isReplayablePluginStatus 判断插件记录是否需要补齐到新节点：installed / enabled / disabled / partial。
// isReplayablePluginStatus reports whether a plugin record must be replayed to a new node: installed / enabled /
// disabled / partial.
func isReplayablePluginStatus(status PluginStatus) bool {
	switch status {
	case PluginStatusInstalled, PluginStatusEnabled, PluginStatusDisabled, PluginStatusPartial:
		return true
	default:
		return false
	}
}

// listNodeConnectors 通过 LIST_PLUGINS 读取节点 connectors 目录下已有的连接器。
// listNodeConnectors reads the connectors already in the node's connectors directory via LIST_PLUGINS.
func (s *Service) listNodeConnectors(ctx context.Context, agentID, installDir string) (map[string]struct{}, error) {
	success, output, err := s.agentCommandSender.SendCommand(ctx, agentID, "list_plugins", map[string]string{"install_path": installDir})
	if err != nil {
		return nil, err
	}
	var parsed nodePluginListResult
	if parseErr := json.Unmarshal([]byte(output), &parsed); parseErr != nil {
		if !success {
			return nil, fmt.Errorf("list plugins failed / 列出插件失败: %s", output)
		}
		return nil, fmt.Errorf("parse plugin list / 解析插件列表失败: %w", parseErr)
	}
	if !success || !parsed.Success {
		return nil, fmt.Errorf("list plugins failed / 列出插件失败: %s", output)
	}
	present := make(map[string]struct{}, len(parsed.Plugins))
	for _, item := range parsed.Plugins {
		present[connectorKey("connector-"+strings.TrimSpace(item.Name), item.Version)] = struct{}{}
	}
	return present, nil
}

// replayArtifactID 返回插件记录的 artifact ID，缺失时按插件名映射。
// replayArtifactID returns the record's artifact ID, falling back to the plugin name mapping.
func replayArtifactID(installed InstalledPlugin) string {
	if artifactID := strings.TrimSpace(installed.ArtifactID); artifactID != "" {
		return artifactID
	}
	return getArtifactID(installed.PluginName)
}

// connectorKey 生成与 Agent 连接器文件名（connector-<name>-<version>.jar）对应的比较键。
// connectorKey builds a comparison key matching the Agent connector file name (connector-<name>-<version>.jar).
func connectorKey(artifactID, version string) string {
	return strings.TrimSpace(artifactID) + "@" + strings.TrimSpace(version)
}
