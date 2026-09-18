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

package stupgrade

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

const packageArchNoarch = "noarch"

// ClusterProvider 定义升级预检查所需的集群读取能力。
// ClusterProvider defines the cluster read capabilities required by upgrade precheck.
type ClusterProvider interface {
	Get(ctx context.Context, id uint) (*clusterapp.Cluster, error)
	GetClusterNodesWithAgentInfo(ctx context.Context, clusterID uint) ([]*clusterapp.NodeInfo, error)
}

// HostProvider 定义升级预检查所需的主机读取能力。
// HostProvider defines the host read capabilities required by upgrade precheck.
type HostProvider interface {
	Get(ctx context.Context, id uint) (*hostapp.Host, error)
}

// PackageProvider 定义升级预检查所需的安装包读取能力。
// PackageProvider defines the package read capabilities required by upgrade precheck.
type PackageProvider interface {
	GetPackageInfo(ctx context.Context, version string) (*installerapp.PackageInfo, error)
}

// PluginProvider 定义升级预检查所需的插件读取能力。
// PluginProvider defines the plugin read capabilities required by upgrade precheck.
type PluginProvider interface {
	ListInstalledPlugins(ctx context.Context, clusterID uint) ([]pluginapp.InstalledPlugin, error)
	ListLocalPlugins() ([]pluginapp.LocalPlugin, error)
	GetPluginDependenciesForVersion(ctx context.Context, pluginName, version string) ([]pluginapp.PluginDependency, error)
	GetPluginArtifactID(pluginName string) string
	TransferPluginToAgent(ctx context.Context, agentID, pluginName, version, installDir string, profileKeys []string) error
	RecordInstalledPlugin(ctx context.Context, clusterID uint, pluginName, version string) error
}

// ConfigProvider 定义升级预检查所需的配置读取能力。
// ConfigProvider defines the config read capabilities required by upgrade precheck.
type ConfigProvider interface {
	GetByCluster(ctx context.Context, clusterID uint) ([]*appconfig.ConfigInfo, error)
}

// RunPrecheck 运行集群升级预检查。
// RunPrecheck runs the cluster upgrade precheck.
func (s *Service) RunPrecheck(ctx context.Context, req *PrecheckRequest) (*PrecheckResult, error) {
	if req == nil {
		return nil, fmt.Errorf("st upgrade precheck request is required")
	}
	if err := s.ensurePlanningDependencies(); err != nil {
		return nil, err
	}

	targetVersion := seatunnel.ResolveVersion(req.TargetVersion)
	result := &PrecheckResult{
		ClusterID:     req.ClusterID,
		TargetVersion: targetVersion,
		Issues:        make([]BlockingIssue, 0),
		NodeTargets:   make([]NodeTarget, 0),
		GeneratedAt:   time.Now(),
	}

	clusterInfo, err := s.clusterProvider.Get(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(clusterInfo.Version) != "" && clusterInfo.Version == targetVersion {
		result.Issues = append(result.Issues, blockingIssue(
			CheckCategoryPackage,
			"same_version",
			fmt.Sprintf("target version %s is the same as current cluster version / 目标版本 %s 与当前集群版本相同", targetVersion, targetVersion),
			nil,
		))
	}

	packageInfo, err := s.packageProvider.GetPackageInfo(ctx, targetVersion)
	if err != nil {
		return nil, err
	}
	packageManifest := buildPackageManifest(packageInfo, req)
	result.PackageManifest = &packageManifest
	result.Issues = append(result.Issues, validatePackageManifest(packageInfo, packageManifest, req.PackageChecksum)...)

	nodes, err := s.clusterProvider.GetClusterNodesWithAgentInfo(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	selectedNodes, nodeSelectionIssues := filterNodeScope(nodes, req.NodeIDs)
	result.Issues = append(result.Issues, nodeSelectionIssues...)

	nodeTargets, nodeIssues, err := s.buildNodeTargets(ctx, selectedNodes, clusterInfo.Version, targetVersion, packageManifest.Arch, packageManifest.SizeBytes, req.TargetInstallDir)
	if err != nil {
		return nil, err
	}
	result.NodeTargets = nodeTargets
	result.Issues = append(result.Issues, nodeIssues...)

	connectorManifest, connectorIssues, err := s.buildConnectorManifest(ctx, req.ClusterID, targetVersion, req.ConnectorNames)
	if err != nil {
		return nil, err
	}
	result.ConnectorManifest = &connectorManifest
	result.Issues = append(result.Issues, connectorIssues...)

	configMergePlan, configIssues, err := s.buildConfigMergePlan(ctx, req.ClusterID, targetVersion, packageManifest.LocalPath)
	if err != nil {
		return nil, err
	}
	result.ConfigMergePlan = &configMergePlan
	result.Issues = append(result.Issues, configIssues...)
	result.Ready = !hasBlockingIssues(result.Issues)
	return result, nil
}

// CreatePlanFromRequest 基于预检查结果创建升级计划。
// CreatePlanFromRequest creates an upgrade plan based on the precheck result.
func (s *Service) CreatePlanFromRequest(ctx context.Context, req *CreatePlanRequest, createdBy uint) (*CreatePlanResult, error) {
	if req == nil {
		return nil, fmt.Errorf("st upgrade plan request is required")
	}

	precheck, err := s.RunPrecheck(ctx, &req.PrecheckRequest)
	if err != nil {
		return nil, err
	}

	mergePlan := precheck.ConfigMergePlan
	if req.ConfigMergePlan != nil {
		mergePlan = req.ConfigMergePlan
	}
	if mergePlan == nil {
		precheck.Issues = append(precheck.Issues, blockingIssue(
			CheckCategoryConfig,
			"config_merge_plan_missing",
			"config merge plan is required before plan creation / 生成升级计划前必须提供配置合并计划",
			nil,
		))
	} else if !mergePlan.Ready || mergePlan.HasConflicts {
		precheck.Issues = append(precheck.Issues, blockingIssue(
			CheckCategoryConfig,
			"config_merge_plan_unresolved",
			"config merge plan still contains unresolved conflicts / 配置合并计划仍包含未解决冲突",
			map[string]string{"conflict_count": fmt.Sprintf("%d", mergePlan.ConflictCount)},
		))
	}
	precheck.ConfigMergePlan = mergePlan
	precheck.Ready = !hasBlockingIssues(precheck.Issues)
	if !precheck.Ready {
		return &CreatePlanResult{Precheck: precheck}, nil
	}

	clusterInfo, err := s.clusterProvider.Get(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	if precheck.PackageManifest == nil || precheck.ConnectorManifest == nil || precheck.ConfigMergePlan == nil {
		return nil, ErrUpgradePlanSnapshotEmpty
	}

	snapshot := UpgradePlanSnapshot{
		ClusterID:         req.ClusterID,
		DeploymentMode:    string(clusterInfo.DeploymentMode),
		SourceVersion:     clusterInfo.Version,
		TargetVersion:     precheck.TargetVersion,
		PackageManifest:   *precheck.PackageManifest,
		ConnectorManifest: *precheck.ConnectorManifest,
		ConfigMergePlan:   *precheck.ConfigMergePlan,
		NodeTargets:       append([]NodeTarget(nil), precheck.NodeTargets...),
		Steps:             DefaultExecutionSteps(),
		GeneratedAt:       time.Now(),
	}
	plan, err := s.CreatePlan(ctx, snapshot, createdBy, PlanStatusReady, 0)
	if err != nil {
		return nil, err
	}
	return &CreatePlanResult{Precheck: precheck, Plan: plan}, nil
}

func (s *Service) ensurePlanningDependencies() error {
	if s.clusterProvider == nil || s.hostProvider == nil || s.packageProvider == nil || s.pluginProvider == nil || s.configProvider == nil {
		return fmt.Errorf("st upgrade planning dependencies are not fully configured")
	}
	return nil
}

func buildPackageManifest(info *installerapp.PackageInfo, req *PrecheckRequest) PackageManifest {
	targetVersion := seatunnel.ResolveVersion(req.TargetVersion)
	manifest := PackageManifest{
		Version:   targetVersion,
		FileName:  fmt.Sprintf("apache-seatunnel-%s-bin.tar.gz", targetVersion),
		Source:    AssetSourceMirrorDownload,
		Arch:      packageArchNoarch,
		SizeBytes: 0,
	}
	if info != nil {
		if info.FileName != "" {
			manifest.FileName = info.FileName
		}
		manifest.SizeBytes = info.FileSize
		manifest.Checksum = info.Checksum
		manifest.LocalPath = info.LocalPath
		if info.IsLocal {
			manifest.Source = AssetSourceLocalPackage
		}
	}
	if arch := strings.TrimSpace(req.PackageArch); arch != "" {
		manifest.Arch = arch
	}
	return manifest
}

func validatePackageManifest(info *installerapp.PackageInfo, manifest PackageManifest, expectedChecksum string) []BlockingIssue {
	issues := make([]BlockingIssue, 0)
	if info == nil || !info.IsLocal {
		issues = append(issues, blockingIssue(
			CheckCategoryPackage,
			"package_missing",
			fmt.Sprintf("target package %s is not available locally / 目标安装包 %s 尚未在本地就绪", manifest.Version, manifest.Version),
			map[string]string{"version": manifest.Version},
		))
		return issues
	}
	if strings.TrimSpace(manifest.Checksum) == "" {
		issues = append(issues, blockingIssue(
			CheckCategoryPackage,
			"checksum_missing",
			fmt.Sprintf("package checksum is missing for %s / 安装包 %s 缺少 checksum", manifest.Version, manifest.Version),
			map[string]string{"version": manifest.Version},
		))
	}
	if expected := strings.TrimSpace(expectedChecksum); expected != "" && !strings.EqualFold(expected, manifest.Checksum) {
		issues = append(issues, blockingIssue(
			CheckCategoryPackage,
			"checksum_mismatch",
			fmt.Sprintf("package checksum mismatch for %s / 安装包 %s checksum 不匹配", manifest.Version, manifest.Version),
			map[string]string{"expected_checksum": expected, "actual_checksum": manifest.Checksum},
		))
	}
	return issues
}

func (s *Service) buildNodeTargets(ctx context.Context, nodes []*clusterapp.NodeInfo, sourceVersion, targetVersion, packageArch string, packageSize int64, targetInstallDir string) ([]NodeTarget, []BlockingIssue, error) {
	targets := make([]NodeTarget, 0, len(nodes))
	issues := make([]BlockingIssue, 0)
	resolvedTargetInstallDir := strings.TrimSpace(targetInstallDir)
	for _, node := range nodes {
		hostInfo, err := s.hostProvider.Get(ctx, node.HostID)
		if err != nil {
			return nil, nil, err
		}
		target := NodeTarget{
			ClusterNodeID:    node.ID,
			HostID:           node.HostID,
			HostName:         node.HostName,
			HostIP:           node.HostIP,
			Role:             string(node.Role),
			Arch:             hostInfo.Arch,
			SourceVersion:    sourceVersion,
			TargetVersion:    targetVersion,
			SourceInstallDir: node.InstallDir,
			TargetInstallDir: resolveTargetInstallDir(node.InstallDir, sourceVersion, targetVersion, resolvedTargetInstallDir),
		}
		targets = append(targets, target)

		if !node.IsOnline {
			issues = append(issues, blockingIssue(
				CheckCategoryNode,
				"node_offline",
				fmt.Sprintf("node %s is offline / 节点 %s 当前离线", node.HostName, node.HostName),
				map[string]string{"host_id": fmt.Sprintf("%d", node.HostID)},
			))
		}
		if node.Status == clusterapp.NodeStatusError {
			issues = append(issues, blockingIssue(
				CheckCategoryNode,
				"node_error",
				fmt.Sprintf("node %s is in error state / 节点 %s 当前处于错误状态", node.HostName, node.HostName),
				map[string]string{"host_id": fmt.Sprintf("%d", node.HostID)},
			))
		}
		if isArchBlocking(packageArch, hostInfo.Arch) {
			issues = append(issues, blockingIssue(
				CheckCategoryPackage,
				"arch_mismatch",
				fmt.Sprintf("package arch %s does not match node %s arch %s / 安装包架构 %s 与节点 %s 架构 %s 不匹配", packageArch, node.HostName, hostInfo.Arch, packageArch, node.HostName, hostInfo.Arch),
				map[string]string{"package_arch": packageArch, "host_arch": hostInfo.Arch, "host_id": fmt.Sprintf("%d", node.HostID)},
			))
		}
		if isDiskInsufficient(hostInfo, packageSize) {
			issues = append(issues, blockingIssue(
				CheckCategoryNode,
				"disk_insufficient",
				fmt.Sprintf("node %s does not have enough disk space for package distribution / 节点 %s 可用磁盘不足，无法分发安装包", node.HostName, node.HostName),
				map[string]string{
					"host_id":         fmt.Sprintf("%d", node.HostID),
					"required_bytes":  fmt.Sprintf("%d", packageSize),
					"available_bytes": fmt.Sprintf("%d", estimateAvailableDisk(hostInfo)),
				},
			))
		}
	}
	return targets, issues, nil
}

func (s *Service) buildConnectorManifest(ctx context.Context, clusterID uint, targetVersion string, connectorNames []string) (ConnectorManifest, []BlockingIssue, error) {
	manifest := ConnectorManifest{
		Version:         targetVersion,
		ReplacementMode: "package_overlay",
		Connectors:      make([]ConnectorArtifact, 0),
		Libraries:       make([]LibraryArtifact, 0),
		PluginDeps:      make([]PluginDependencyArtifact, 0),
	}
	issues := make([]BlockingIssue, 0)

	installedPlugins, err := s.pluginProvider.ListInstalledPlugins(ctx, clusterID)
	if err != nil {
		return manifest, nil, err
	}
	requiredConnectors := resolveRequiredConnectors(installedPlugins, connectorNames)
	localPlugins, err := s.pluginProvider.ListLocalPlugins()
	if err != nil {
		return manifest, nil, err
	}
	localPluginMap := make(map[string]pluginapp.LocalPlugin, len(localPlugins))
	for _, localPlugin := range localPlugins {
		localPluginMap[localPlugin.Name+"@"+localPlugin.Version] = localPlugin
	}

	for _, connectorName := range requiredConnectors {
		key := connectorName + "@" + targetVersion
		localPlugin, ok := localPluginMap[key]
		if !ok {
			issues = append(issues, blockingIssue(
				CheckCategoryConnector,
				"connector_missing",
				fmt.Sprintf("connector %s for version %s is not downloaded locally / 版本 %s 的连接器 %s 尚未下载到本地", connectorName, targetVersion, targetVersion, connectorName),
				map[string]string{"connector": connectorName, "version": targetVersion},
			))
			continue
		}

		manifest.Connectors = append(manifest.Connectors, ConnectorArtifact{
			PluginName: connectorName,
			ArtifactID: s.pluginProvider.GetPluginArtifactID(connectorName),
			Version:    targetVersion,
			Category:   string(localPlugin.Category),
			FileName:   fileNameFromPath(localPlugin.ConnectorPath),
			LocalPath:  localPlugin.ConnectorPath,
			Source:     AssetSourceLocalPlugin,
			Required:   true,
		})

		dependencies, err := s.pluginProvider.GetPluginDependenciesForVersion(ctx, connectorName, targetVersion)
		if err != nil {
			return manifest, nil, err
		}
		for _, dependency := range dependencies {
			version := dependency.Version
			if strings.TrimSpace(version) == "" {
				version = targetVersion
			}
			fileName := fmt.Sprintf("%s-%s.jar", dependency.ArtifactID, version)
			targetDir := strings.TrimSpace(dependency.TargetDir)
			if strings.HasPrefix(targetDir, "plugins/") {
				relativePath := strings.TrimPrefix(targetDir, "plugins/")
				relativePath = strings.TrimPrefix(relativePath, "/")
				if relativePath != "" {
					relativePath = fmt.Sprintf("%s/%s", relativePath, fileName)
				} else {
					relativePath = fileName
				}
				manifest.PluginDeps = append(manifest.PluginDeps, PluginDependencyArtifact{
					GroupID:      dependency.GroupID,
					ArtifactID:   dependency.ArtifactID,
					Version:      version,
					FileName:     fileName,
					Source:       AssetSourceLocalPlugin,
					TargetDir:    targetDir,
					RelativePath: relativePath,
				})
				continue
			}
			manifest.Libraries = append(manifest.Libraries, LibraryArtifact{
				GroupID:    dependency.GroupID,
				ArtifactID: dependency.ArtifactID,
				Version:    version,
				FileName:   fileName,
				Source:     AssetSourceLocalPlugin,
				Scope:      dependency.TargetDir,
			})
		}
	}

	sort.Slice(manifest.Connectors, func(i, j int) bool {
		return manifest.Connectors[i].PluginName < manifest.Connectors[j].PluginName
	})
	sort.Slice(manifest.Libraries, func(i, j int) bool {
		if manifest.Libraries[i].ArtifactID == manifest.Libraries[j].ArtifactID {
			return manifest.Libraries[i].Version < manifest.Libraries[j].Version
		}
		return manifest.Libraries[i].ArtifactID < manifest.Libraries[j].ArtifactID
	})
	sort.Slice(manifest.PluginDeps, func(i, j int) bool {
		if manifest.PluginDeps[i].RelativePath == manifest.PluginDeps[j].RelativePath {
			return manifest.PluginDeps[i].ArtifactID < manifest.PluginDeps[j].ArtifactID
		}
		return manifest.PluginDeps[i].RelativePath < manifest.PluginDeps[j].RelativePath
	})
	return manifest, issues, nil
}

func resolveRequiredConnectors(installedPlugins []pluginapp.InstalledPlugin, connectorNames []string) []string {
	if len(connectorNames) > 0 {
		return dedupeSortedStrings(connectorNames)
	}
	required := make([]string, 0, len(installedPlugins))
	for _, installedPlugin := range installedPlugins {
		required = append(required, installedPlugin.PluginName)
	}
	return dedupeSortedStrings(required)
}

func (s *Service) buildConfigMergePlan(ctx context.Context, clusterID uint, targetVersion, packagePath string) (ConfigMergePlan, []BlockingIssue, error) {
	plan := ConfigMergePlan{Files: make([]ConfigMergeFile, 0), GeneratedAt: time.Now()}
	issues := make([]BlockingIssue, 0)
	configs, err := s.configProvider.GetByCluster(ctx, clusterID)
	if err != nil {
		return plan, nil, err
	}
	if len(configs) == 0 {
		issues = append(issues, blockingIssue(
			CheckCategoryConfig,
			"config_missing",
			"cluster configs are not initialized / 集群配置尚未初始化，无法生成升级配置合并计划",
			map[string]string{"cluster_id": fmt.Sprintf("%d", clusterID)},
		))
		return plan, issues, nil
	}

	inputs, inputIssues := buildConfigMergeInputs(configs)
	issues = append(issues, inputIssues...)

	targetPaths := make([]string, 0, len(inputs))
	for _, input := range inputs {
		if input.TargetPath == "" {
			continue
		}
		targetPaths = append(targetPaths, input.TargetPath)
	}
	targetContents, targetIssues := readTargetConfigContentsFromPackage(packagePath, targetVersion, targetPaths)
	issues = append(issues, targetIssues...)

	for _, input := range inputs {
		plan.Files = append(plan.Files, buildConfigMergeFile(input, targetContents[input.TargetPath]))
	}
	sort.Slice(plan.Files, func(i, j int) bool {
		if plan.Files[i].ConfigType == plan.Files[j].ConfigType {
			return plan.Files[i].TargetPath < plan.Files[j].TargetPath
		}
		return plan.Files[i].ConfigType < plan.Files[j].ConfigType
	})
	plan.ConflictCount = 0
	for _, file := range plan.Files {
		plan.ConflictCount += file.ConflictCount
	}
	plan.HasConflicts = plan.ConflictCount > 0
	plan.Ready = !plan.HasConflicts
	return plan, issues, nil
}

func filterNodeScope(nodes []*clusterapp.NodeInfo, requestedNodeIDs []uint) ([]*clusterapp.NodeInfo, []BlockingIssue) {
	if len(requestedNodeIDs) == 0 {
		return nodes, nil
	}
	requested := make(map[uint]struct{}, len(requestedNodeIDs))
	for _, nodeID := range requestedNodeIDs {
		requested[nodeID] = struct{}{}
	}
	selected := make([]*clusterapp.NodeInfo, 0, len(requestedNodeIDs))
	issues := make([]BlockingIssue, 0)
	for _, node := range nodes {
		if _, ok := requested[node.ID]; ok {
			selected = append(selected, node)
			delete(requested, node.ID)
		}
	}
	for nodeID := range requested {
		issues = append(issues, blockingIssue(
			CheckCategoryNode,
			"node_not_found",
			fmt.Sprintf("node %d is not part of the cluster / 节点 %d 不属于当前集群", nodeID, nodeID),
			map[string]string{"node_id": fmt.Sprintf("%d", nodeID)},
		))
	}
	return selected, issues
}

func buildTargetInstallDir(currentInstallDir, sourceVersion, targetVersion string) string {
	if currentInstallDir != "" && sourceVersion != "" && strings.Contains(currentInstallDir, sourceVersion) {
		return strings.Replace(currentInstallDir, sourceVersion, targetVersion, 1)
	}
	return seatunnel.DefaultInstallDir(targetVersion)
}

func resolveTargetInstallDir(currentInstallDir, sourceVersion, targetVersion, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return buildTargetInstallDir(currentInstallDir, sourceVersion, targetVersion)
}

func isArchBlocking(packageArch, hostArch string) bool {
	trimmedPackageArch := strings.TrimSpace(strings.ToLower(packageArch))
	trimmedHostArch := strings.TrimSpace(strings.ToLower(hostArch))
	if trimmedPackageArch == "" || trimmedPackageArch == packageArchNoarch || trimmedHostArch == "" {
		return false
	}
	return trimmedPackageArch != trimmedHostArch
}

func isDiskInsufficient(hostInfo *hostapp.Host, requiredBytes int64) bool {
	if hostInfo == nil || requiredBytes <= 0 || hostInfo.TotalDisk <= 0 {
		return false
	}
	return estimateAvailableDisk(hostInfo) < requiredBytes
}

func estimateAvailableDisk(hostInfo *hostapp.Host) int64 {
	if hostInfo == nil || hostInfo.TotalDisk <= 0 {
		return 0
	}
	usageRatio := hostInfo.DiskUsage / 100
	if usageRatio < 0 {
		usageRatio = 0
	}
	if usageRatio > 1 {
		usageRatio = 1
	}
	available := float64(hostInfo.TotalDisk) * (1 - usageRatio)
	if available < 0 {
		return 0
	}
	return int64(available)
}

func hasBlockingIssues(issues []BlockingIssue) bool {
	for _, issue := range issues {
		if issue.Blocking {
			return true
		}
	}
	return false
}

func blockingIssue(category CheckCategory, code, message string, metadata map[string]string) BlockingIssue {
	return BlockingIssue{
		Category: category,
		Code:     code,
		Message:  normalizeUserVisibleText(message),
		Blocking: true,
		Metadata: metadata,
	}
}

func dedupeSortedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	uniq := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := uniq[trimmed]; ok {
			continue
		}
		uniq[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func fileNameFromPath(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
