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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/apps/javaruntime"
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
	GetPluginDependenciesForVersionAndProfiles(ctx context.Context, pluginName, version string, profileKeys []string) ([]pluginapp.PluginDependency, error)
	ResolvePluginProfiles(ctx context.Context, pluginName, targetVersion string, candidateProfiles ...[]string) []string
	HasApplicableDependencyProfiles(ctx context.Context, pluginName, version string) bool
	IsDependencyDownloaded(artifactID, depVersion, pluginVersion, targetDir string) bool
	GetPluginArtifactID(pluginName string) string
	TransferPluginToAgent(ctx context.Context, agentID, pluginName, version, installDir string, profileKeys []string) error
	RecordInstalledPlugin(ctx context.Context, clusterID uint, pluginName, version string, profileKeys []string) error
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

	nodeTargets, nodeIssues, err := s.buildNodeTargets(ctx, selectedNodes, clusterInfo.Version, targetVersion, packageManifest.Arch, packageManifest.SizeBytes, req)
	if err != nil {
		return nil, err
	}
	result.NodeTargets = nodeTargets
	result.Issues = append(result.Issues, nodeIssues...)

	connectorManifest, connectorIssues, err := s.buildConnectorManifest(ctx, req.ClusterID, targetVersion, req.ConnectorNames, req.ConnectorProfiles)
	if err != nil {
		return nil, err
	}
	result.ConnectorManifest = &connectorManifest
	result.Issues = append(result.Issues, connectorIssues...)

	sourceInstallDirs := make([]string, 0, len(nodeTargets))
	for _, target := range nodeTargets {
		if strings.TrimSpace(target.SourceInstallDir) == "" {
			continue
		}
		sourceInstallDirs = append(sourceInstallDirs, target.SourceInstallDir)
	}
	configMergePlan, configIssues, err := s.buildConfigMergePlan(
		ctx,
		req.ClusterID,
		string(clusterInfo.DeploymentMode),
		targetVersion,
		packageManifest.LocalPath,
		sourceInstallDirs,
	)
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

func (s *Service) buildNodeTargets(ctx context.Context, nodes []*clusterapp.NodeInfo, sourceVersion, targetVersion, packageArch string, packageSize int64, req *PrecheckRequest) ([]NodeTarget, []BlockingIssue, error) {
	targets := make([]NodeTarget, 0, len(nodes))
	issues := make([]BlockingIssue, 0)
	targetInstallDir := ""
	if req != nil {
		targetInstallDir = req.TargetInstallDir
	}
	resolvedTargetInstallDir := strings.TrimSpace(targetInstallDir)
	for _, node := range nodes {
		hostInfo, err := s.hostProvider.Get(ctx, node.HostID)
		if err != nil {
			return nil, nil, err
		}
		targetJavaHome, javaHomeErr := resolveTargetJavaHome(node.ID, node.JavaHome, req)
		sourceJavaHome := node.JavaHome
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
			SourceJavaHome:   &sourceJavaHome,
			JavaHome:         targetJavaHome,
		}
		targets = append(targets, target)
		if javaHomeErr != nil {
			issues = append(issues, blockingIssue(
				CheckCategoryNode,
				"jdk_path_invalid",
				fmt.Sprintf("node %s target JDK path is invalid: %v / 节点 %s 的目标 JDK 路径不合法：%v", node.HostName, javaHomeErr, node.HostName, javaHomeErr),
				map[string]string{
					"host_id":   fmt.Sprintf("%d", node.HostID),
					"java_home": firstNonEmptyStr(targetJavaHome, node.JavaHome),
				},
			))
		}

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
		if issue := s.checkTargetInstallDirWritable(ctx, node, target); issue != nil {
			issues = append(issues, *issue)
		}
		if javaHomeErr == nil {
			if issue := s.checkJavaRuntimeForTarget(ctx, node, target); issue != nil {
				issues = append(issues, *issue)
			}
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

func (s *Service) buildConnectorManifest(ctx context.Context, clusterID uint, targetVersion string, connectorNames []string, requestedProfiles map[string][]string) (ConnectorManifest, []BlockingIssue, error) {
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
	installedByName := make(map[string]pluginapp.InstalledPlugin, len(installedPlugins))
	for _, p := range installedPlugins {
		installedByName[p.PluginName] = p
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

		// 解析目标版本生效画像：1) 显式请求画像 2) 当前集群已安装画像 3) 目标版本本地元数据画像
		// Resolve profiles to use on targetVersion:
		// 1) explicitly requested profiles (if any)
		// 2) installed profiles on cluster's current version
		// 3) local target plugin metadata profiles
		var candidateLists [][]string
		if len(requestedProfiles) > 0 && len(requestedProfiles[connectorName]) > 0 {
			candidateLists = append(candidateLists, requestedProfiles[connectorName])
		}
		if installed, exists := installedByName[connectorName]; exists && len(installed.SelectedProfileKeys) > 0 {
			candidateLists = append(candidateLists, installed.SelectedProfileKeys)
		}
		if len(localPlugin.SelectedProfileKeys) > 0 {
			candidateLists = append(candidateLists, localPlugin.SelectedProfileKeys)
		}

		profileKeys := s.pluginProvider.ResolvePluginProfiles(ctx, connectorName, targetVersion, candidateLists...)

		// Fail closed: plugins with dependency profiles must resolve at least one profile.
		// Otherwise empty deps look "Ready" while agents miss drivers (e.g. jdbc multi-profile, no IsDefault).
		// 失败关闭：存在依赖画像的插件必须解析出至少一个画像，否则空依赖会显示 Ready 但 Agent 缺少驱动。
		if len(profileKeys) == 0 && s.pluginProvider.HasApplicableDependencyProfiles(ctx, connectorName, targetVersion) {
			issues = append(issues, blockingIssue(
				CheckCategoryConnector,
				"profiles_missing",
				fmt.Sprintf("connector %s (%s) requires dependency profiles but none were resolved; set connector_profiles / 连接器 %s (%s) 需要依赖画像但未能解析，请设置 connector_profiles", connectorName, targetVersion, connectorName, targetVersion),
				map[string]string{"connector": connectorName, "version": targetVersion},
			))
			continue
		}

		manifest.Connectors = append(manifest.Connectors, ConnectorArtifact{
			PluginName:  connectorName,
			ArtifactID:  s.pluginProvider.GetPluginArtifactID(connectorName),
			Version:     targetVersion,
			ProfileKeys: profileKeys,
			Category:    string(localPlugin.Category),
			FileName:    fileNameFromPath(localPlugin.ConnectorPath),
			LocalPath:   localPlugin.ConnectorPath,
			Source:      AssetSourceLocalPlugin,
			Required:    true,
		})

		dependencies, err := s.pluginProvider.GetPluginDependenciesForVersionAndProfiles(ctx, connectorName, targetVersion, profileKeys)
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

			// Check if dependency is downloaded locally on control plane
			// 校验依赖是否已下载到控制面本地
			if !s.pluginProvider.IsDependencyDownloaded(dependency.ArtifactID, version, targetVersion, targetDir) {
				issues = append(issues, blockingIssue(
					CheckCategoryConnector,
					"dependency_missing",
					fmt.Sprintf("dependency %s-%s for connector %s (%s) is not downloaded locally / 连接器 %s (%s) 的依赖 %s-%s 尚未下载到本地", dependency.ArtifactID, version, connectorName, targetVersion, connectorName, targetVersion, dependency.ArtifactID, version),
					map[string]string{"connector": connectorName, "dependency": dependency.ArtifactID, "version": version, "target_dir": targetDir},
				))
			}

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
					LocalPath:    dependency.StoredPath,
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
				LocalPath:  dependency.StoredPath,
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

func (s *Service) buildConfigMergePlan(
	ctx context.Context,
	clusterID uint,
	deploymentMode string,
	targetVersion, packagePath string,
	sourceInstallDirs []string,
) (ConfigMergePlan, []BlockingIssue, error) {
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

	dbInputs, inputIssues := buildConfigMergeInputs(configs)
	issues = append(issues, inputIssues...)

	// 先探测目标包内容：包含 DB 已有路径 + 部署模式可能补齐的路径。
	// Probe package contents for DB paths plus mode-supported candidates.
	probePaths := make([]string, 0, len(dbInputs)+8)
	for _, input := range dbInputs {
		if input.TargetPath != "" {
			probePaths = append(probePaths, input.TargetPath)
		}
	}
	for _, configType := range appconfig.GetConfigTypesForMode(deploymentMode) {
		probePaths = append(probePaths, normalizeConfigArchivePath(appconfig.GetConfigFilePath(configType)))
	}
	targetContents, targetIssues := readTargetConfigContentsFromPackage(packagePath, targetVersion, probePaths)

	// 仅当源目录可读或目标包存在时，补齐 DB 未入库的模式配置（如 seatunnel-env.sh）。
	// Backfill mode configs missing from DB only when source dir or target package has them.
	inputs := supplementConfigMergeInputs(dbInputs, deploymentMode, sourceInstallDirs, targetContents)

	// package_config_missing 只针对「原本就在 DB / 已纳入 inputs」的配置；可选配置降级为告警。
	// Restrict package_config_missing to configs included in inputs; soften optional ones.
	requiredPaths := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if input.TargetPath != "" {
			requiredPaths[input.TargetPath] = struct{}{}
		}
	}
	filteredTargetIssues := make([]BlockingIssue, 0, len(targetIssues))
	for _, issue := range targetIssues {
		if issue.Code != "package_config_missing" {
			filteredTargetIssues = append(filteredTargetIssues, issue)
			continue
		}
		targetPath := ""
		if issue.Metadata != nil {
			targetPath = normalizeConfigArchivePath(issue.Metadata["target_path"])
		}
		if _, ok := requiredPaths[targetPath]; !ok {
			continue
		}
		filteredTargetIssues = append(filteredTargetIssues, issue)
	}
	issues = append(issues, softenOptionalPackageConfigIssues(filteredTargetIssues)...)

	for _, input := range inputs {
		targetContent, found := targetContents[input.TargetPath]
		// 必选配置若目标包缺失：已有 package_config_missing 阻断，不再生成空合并文件。
		// Required configs missing from package are blocked already; skip empty merge files.
		if !found && !isOptionalUpgradeConfigType(input.ConfigType) {
			continue
		}
		plan.Files = append(plan.Files, buildConfigMergeFile(input, targetContent))
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

// supplementConfigMergeInputs 为部署模式支持、但 DB 尚未入库的配置类型补齐合并输入。
// 仅在源安装目录可读或目标包存在该文件时补齐，避免凭空制造 package_config_missing。
// supplementConfigMergeInputs backfills mode-supported types missing from DB.
// Only add when the source install dir or target package actually has the file.
func supplementConfigMergeInputs(
	inputs []configMergeInput,
	deploymentMode string,
	sourceInstallDirs []string,
	targetContents map[string]string,
) []configMergeInput {
	existing := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		existing[input.ConfigType] = struct{}{}
	}

	for _, configType := range appconfig.GetConfigTypesForMode(deploymentMode) {
		typeName := string(configType)
		if _, ok := existing[typeName]; ok {
			continue
		}
		targetPath := normalizeConfigArchivePath(appconfig.GetConfigFilePath(configType))
		localContent := tryReadConfigFromInstallDirs(sourceInstallDirs, targetPath)
		_, inPackage := targetContents[targetPath]
		if localContent == "" && !inPackage {
			continue
		}
		inputs = append(inputs, configMergeInput{
			ConfigType:   typeName,
			TargetPath:   targetPath,
			BaseContent:  "",
			LocalContent: localContent,
		})
		existing[typeName] = struct{}{}
	}
	return inputs
}

func tryReadConfigFromInstallDirs(installDirs []string, relativePath string) string {
	rel := normalizeConfigArchivePath(relativePath)
	if rel == "" {
		return ""
	}
	for _, installDir := range installDirs {
		dir := strings.TrimSpace(installDir)
		if dir == "" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		return string(content)
	}
	return ""
}

func isOptionalUpgradeConfigType(configType string) bool {
	switch strings.TrimSpace(configType) {
	case string(appconfig.ConfigTypeSeatunnelEnv):
		return true
	default:
		return false
	}
}

// softenOptionalPackageConfigIssues 将可选配置的 package_config_missing 降为非阻断告警。
// softenOptionalPackageConfigIssues downgrades optional package_config_missing issues to non-blocking warnings.
func softenOptionalPackageConfigIssues(issues []BlockingIssue) []BlockingIssue {
	if len(issues) == 0 {
		return issues
	}
	out := make([]BlockingIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Code != "package_config_missing" {
			out = append(out, issue)
			continue
		}
		targetPath := ""
		if issue.Metadata != nil {
			targetPath = issue.Metadata["target_path"]
		}
		configType := filepath.Base(normalizeConfigArchivePath(targetPath))
		if !isOptionalUpgradeConfigType(configType) {
			out = append(out, issue)
			continue
		}
		out = append(out, warningIssue(
			issue.Category,
			issue.Code,
			fmt.Sprintf("%s (optional / 可选配置不阻断升级)", issue.Message),
			issue.Metadata,
		))
	}
	return out
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
	currentInstallDir = strings.TrimSpace(currentInstallDir)
	if currentInstallDir != "" && sourceVersion != "" && strings.Contains(currentInstallDir, sourceVersion) {
		return strings.Replace(currentInstallDir, sourceVersion, targetVersion, 1)
	}
	// 源目录不含版本号时，默认放在源目录的同级目录（<父目录>/<目录名>-<目标版本>），
	// 避免落到与源目录无关、可能不可写的 /opt/seatunnel-<version>。
	// When the source dir has no version in it, default to a sibling of the source dir
	// (<parent>/<base>-<target>) instead of an unrelated and possibly unwritable /opt/seatunnel-<version>.
	if currentInstallDir != "" && strings.TrimSpace(targetVersion) != "" {
		cleaned := filepath.Clean(currentInstallDir)
		base := filepath.Base(cleaned)
		parent := filepath.Dir(cleaned)
		if base != "" && base != "." && base != string(filepath.Separator) && filepath.IsAbs(cleaned) {
			return filepath.Join(parent, fmt.Sprintf("%s-%s", base, strings.TrimSpace(targetVersion)))
		}
	}
	return seatunnel.DefaultInstallDir(targetVersion)
}

func resolveTargetInstallDir(currentInstallDir, sourceVersion, targetVersion, override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}
	return buildTargetInstallDir(currentInstallDir, sourceVersion, targetVersion)
}

// resolveTargetJavaHome 解析本次升级要用的目标 JDK：节点级覆盖 > 请求级 java_home > 当前节点 java_home。
// 节点级显式给出键（含空串）表示自动探测或指定路径，不再回落到当前节点路径。
// resolveTargetJavaHome resolves the target JDK for this upgrade: per-node override > request java_home > current node java_home.
// A present per-node key (including empty) means auto detection or an explicit path and does not fall back to the current node path.
func resolveTargetJavaHome(nodeID uint, current string, req *PrecheckRequest) (string, error) {
	if req != nil && req.NodeJavaHomes != nil {
		if value, ok := req.NodeJavaHomes[nodeID]; ok {
			return javaruntime.Validate(value)
		}
	}
	if req != nil && strings.TrimSpace(req.JavaHome) != "" {
		return javaruntime.Validate(req.JavaHome)
	}
	return javaruntime.Validate(current)
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

func warningIssue(category CheckCategory, code, message string, metadata map[string]string) BlockingIssue {
	return BlockingIssue{
		Category: category,
		Code:     code,
		Message:  normalizeUserVisibleText(message),
		Blocking: false,
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

// checkTargetInstallDirWritable 通过节点 Agent 校验目标安装目录可写（已存在时可写、不存在时父目录可写）。
// Agent 不在线或未配置命令发送器时跳过（离线已由 node_offline 拦截）。
// checkTargetInstallDirWritable verifies via the node Agent that the target install dir is writable
// (writable when it exists, or its parent is writable when it does not). Skipped when the Agent is not
// connected or no command sender is configured (offline nodes are already blocked by node_offline).
func (s *Service) checkTargetInstallDirWritable(ctx context.Context, node *clusterapp.NodeInfo, target NodeTarget) *BlockingIssue {
	if s.agentCommandSender == nil || node == nil || !node.IsOnline {
		return nil
	}
	targetDir := strings.TrimSpace(target.TargetInstallDir)
	if targetDir == "" {
		return nil
	}
	agentID, connected := s.agentCommandSender.GetAgentByHostID(target.HostID)
	if !connected || agentID == "" {
		return nil
	}
	success, output, err := s.agentCommandSender.SendCommand(ctx, agentID, "check_path_ready", map[string]string{
		"sub_command": "check_path_ready",
		"path":        targetDir,
	})
	if err == nil && success {
		return nil
	}
	reason := extractAgentResultMessage(output)
	if err != nil {
		reason = err.Error()
	}
	issue := blockingIssue(
		CheckCategoryNode,
		"target_dir_not_writable",
		fmt.Sprintf("target install dir %s on node %s is not writable: %s / 节点 %s 上的目标安装目录 %s 不可写：%s", targetDir, target.HostName, reason, target.HostName, targetDir, reason),
		map[string]string{
			"host_id":            fmt.Sprintf("%d", target.HostID),
			"target_install_dir": targetDir,
		},
	)
	return &issue
}

// checkJavaRuntimeForTarget 通过节点 Agent 校验节点 JDK 满足目标版本的最低要求。
// Agent 不在线或未配置命令发送器时跳过（离线已由 node_offline 拦截）。
// SeaTunnel 3.1.0 起官方硬性要求 JDK 11+（class major 55，JDK 8 启动器必然失败），
// 必须提前阻断而非拖到引擎启动步骤；更早目标版本沿用 {8,11} 通过、其余警告的矩阵。
// checkJavaRuntimeForTarget verifies via the node Agent that the node JDK meets the
// target version's minimum requirement. Skipped when the Agent is offline or no command
// sender is configured (offline nodes are already blocked by node_offline). SeaTunnel
// 3.1.0 hard-requires JDK 11+ (class major 55, a JDK 8 launcher always fails), so block
// early instead of dying at engine startup; earlier targets keep the {8,11} matrix.
func (s *Service) checkJavaRuntimeForTarget(ctx context.Context, node *clusterapp.NodeInfo, target NodeTarget) *BlockingIssue {
	if s.agentCommandSender == nil || node == nil || !node.IsOnline {
		return nil
	}
	agentID, connected := s.agentCommandSender.GetAgentByHostID(target.HostID)
	if !connected || agentID == "" {
		return nil
	}
	success, output, err := s.agentCommandSender.SendCommand(ctx, agentID, "check_java", javaruntime.CheckParams(target.JavaHome))
	if err != nil {
		issue := blockingIssue(
			CheckCategoryNode,
			"java_check_failed",
			fmt.Sprintf("cannot check Java on node %s: %v / 无法检查节点 %s 的 Java 环境：%v", target.HostName, err, target.HostName, err),
			map[string]string{"host_id": fmt.Sprintf("%d", target.HostID)},
		)
		return &issue
	}
	rt := javaruntime.Interpret(target.JavaHome, success, output)
	metadata := map[string]string{
		"host_id":        fmt.Sprintf("%d", target.HostID),
		"target_version": target.TargetVersion,
		"required_java":  fmt.Sprintf("%d+", javaruntime.MinSupportedMajor(target.TargetVersion)),
		"detected_java":  fmt.Sprintf("%d", rt.Version),
		"java_home":      firstNonEmptyStr(rt.JavaHome, target.JavaHome),
	}
	if rt.Status == javaruntime.StatusFailed {
		issue := blockingIssue(
			CheckCategoryNode,
			"java_unusable",
			fmt.Sprintf("node %s has no usable JDK for SeaTunnel %s: %s / 节点 %s 的 JDK 不可用于 SeaTunnel %s：%s", target.HostName, target.TargetVersion, rt.FailureMessage(target.HostName), target.HostName, target.TargetVersion, rt.FailureMessage(target.HostName)),
			metadata,
		)
		return &issue
	}
	switch javaruntime.StatusForTarget(rt.Version, target.TargetVersion) {
	case javaruntime.StatusPassed:
		return nil
	case javaruntime.StatusFailed:
		if rt.Version > javaruntime.MaxSupportedMajor(target.TargetVersion) {
			// 高于支持上限（JDK 24+，JEP 486）与低于下限一样阻断，但文案与 reason 需要区分。
			// Exceeding the supported ceiling (JDK 24+, JEP 486) blocks just like being below the
			// floor, but needs distinct wording and reason code.
			issue := blockingIssue(
				CheckCategoryNode,
				"jdk_above_supported",
				fmt.Sprintf("node %s runs Java %d, above the maximum supported Java %d (JDK 24+ removed Subject.getSubject / JEP 486); choose JDK 8, 11, 17 or 21 on the upgrade prepare page / 节点 %s 的 Java %d 超过最高支持版本 %d（JDK 24+ 移除 Subject.getSubject / JEP 486）；请在升级准备页为目标节点选择 JDK 8 / 11 / 17 / 21", target.HostName, rt.Version, javaruntime.MaxSupportedMajor(target.TargetVersion), target.HostName, rt.Version, javaruntime.MaxSupportedMajor(target.TargetVersion)),
				metadata,
			)
			return &issue
		}
		issue := blockingIssue(
			CheckCategoryNode,
			"jdk_below_required",
			fmt.Sprintf("node %s runs Java %d but SeaTunnel %s requires Java %d+; choose a newer target JDK on the upgrade prepare page / 节点 %s 的 Java %d 低于 SeaTunnel %s 要求的 Java %d+；请在升级准备页为目标节点选择更高版本 JDK", target.HostName, rt.Version, target.TargetVersion, javaruntime.MinSupportedMajor(target.TargetVersion), target.HostName, rt.Version, target.TargetVersion, javaruntime.MinSupportedMajor(target.TargetVersion)),
			metadata,
		)
		return &issue
	default:
		recommended := seatunnel.JavaRequirementForVersion(target.TargetVersion)
		issue := warningIssue(
			CheckCategoryNode,
			"jdk_unusual_version",
			fmt.Sprintf("node %s runs Java %d; SeaTunnel %s recommends Java %s / 节点 %s 运行 Java %d；SeaTunnel %s 推荐 Java %s", target.HostName, rt.Version, target.TargetVersion, recommended.RecommendedText(" or "), target.HostName, rt.Version, target.TargetVersion, recommended.RecommendedText(" 或 ")),
			metadata,
		)
		return &issue
	}
}

// firstNonEmptyStr 返回第一个非空字符串。
// firstNonEmptyStr returns the first non-empty string.
func firstNonEmptyStr(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// extractAgentResultMessage 从 Agent 预检 JSON 输出中提取 message，非 JSON 时原样返回。
// extractAgentResultMessage extracts "message" from an Agent precheck JSON output, returning raw text otherwise.
func extractAgentResultMessage(output string) string {
	trimmed := strings.TrimSpace(output)
	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if strings.TrimSpace(payload.Message) != "" {
			return strings.TrimSpace(payload.Message)
		}
		if strings.TrimSpace(payload.Error) != "" {
			return strings.TrimSpace(payload.Error)
		}
	}
	return trimmed
}
