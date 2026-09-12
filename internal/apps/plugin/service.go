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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

// Common service errors / 常见服务错误
var (
	ErrInvalidVersion      = errors.New("invalid version / 无效的版本号")
	ErrInvalidMirror       = errors.New("invalid mirror source / 无效的镜像源")
	ErrPluginNotAvailable  = errors.New("plugin not available / 插件不可用")
	ErrClusterNotFound     = errors.New("cluster not found / 集群未找到")
	ErrPluginAlreadyExists = errors.New("plugin already installed / 插件已安装")
	ErrVersionMismatch     = errors.New("plugin version does not match cluster version / 插件版本与集群版本不匹配")
	ErrClusterVersionEmpty = errors.New("cluster version is not set / 集群版本未设置")
)

// SeaTunnel Maven repository and documentation URLs
// SeaTunnel Maven 仓库和文档 URL
const (
	// ConnectorRepoGroupPath is the Maven group path used for SeaTunnel connectors.
	// ConnectorRepoGroupPath 是 SeaTunnel 连接器所在的 Maven group 路径。
	ConnectorRepoGroupPath = "org/apache/seatunnel"
	// SeaTunnel documentation URL for connector docs / SeaTunnel 文档 URL，用于连接器文档
	SeaTunnelDocsBaseURL = "https://seatunnel.apache.org/zh-CN/docs"
	// HTTP request timeout for fetching plugin list / 获取插件列表的 HTTP 请求超时
	PluginFetchTimeout = 60 * time.Second
)

// skipModuleList contains modules to skip when fetching connectors from Maven.
// skipModuleList 包含从 Maven 获取连接器时需要跳过的模块。
var skipModuleList = []string{
	"connector-common",   // Common utilities / 通用工具
	"connector-cdc-base", // CDC base module / CDC 基础模块
	"connector-cdc",      // CDC parent module / CDC 父模块
	"connector-file",     // File parent module / File 父模块
	"connector-http",     // HTTP parent module / HTTP 父模块
}

func connectorRepoBaseURL(mirror MirrorSource) string {
	baseURL := strings.TrimSuffix(MirrorURLs[mirror], "/")
	if baseURL == "" {
		baseURL = strings.TrimSuffix(MirrorURLs[MirrorSourceApache], "/")
	}
	return baseURL + "/" + ConnectorRepoGroupPath
}

// isSkippedModule checks if the artifact ID should be skipped.
// isSkippedModule 检查 artifact ID 是否应该被跳过。
func isSkippedModule(artifactID string) bool {
	for _, skip := range skipModuleList {
		if artifactID == skip {
			return true
		}
	}
	return false
}

// ClusterGetter is an interface for getting cluster information.
// ClusterGetter 是获取集群信息的接口。
type ClusterGetter interface {
	GetClusterVersion(ctx context.Context, clusterID uint) (string, error)
}

// ClusterNodeInfo represents node information needed for plugin installation.
// ClusterNodeInfo 表示插件安装所需的节点信息。
type ClusterNodeInfo struct {
	NodeID     uint   // Node ID / 节点 ID
	HostID     uint   // Host ID / 主机 ID
	InstallDir string // SeaTunnel installation directory / SeaTunnel 安装目录
}

// ClusterNodeGetter is an interface for getting cluster nodes.
// ClusterNodeGetter 是获取集群节点的接口。
type ClusterNodeGetter interface {
	GetClusterNodes(ctx context.Context, clusterID uint) ([]ClusterNodeInfo, error)
}

// HostInfoGetter is an interface for getting host information.
// HostInfoGetter 是获取主机信息的接口。
type HostInfoGetter interface {
	GetHostAgentID(ctx context.Context, hostID uint) (string, error)
}

// Service provides plugin management functionality.
// Service 提供插件管理功能。
type Service struct {
	repo               *Repository
	clusterGetter      ClusterGetter
	downloader         *Downloader
	pluginFetcher      func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error)
	officialDocFetcher func(ctx context.Context, version, docSlug string) (string, error)
	mavenVersionLookup func(ctx context.Context, groupID, artifactID string) (string, error)

	// agentCommandSender is used to send commands to agents for plugin installation
	// agentCommandSender 用于向 Agent 发送命令进行插件安装
	agentCommandSender AgentCommandSender

	// clusterNodeGetter is used to get cluster nodes for plugin installation
	// clusterNodeGetter 用于获取集群节点进行插件安装
	clusterNodeGetter ClusterNodeGetter

	// hostInfoGetter is used to get host information (including AgentID)
	// hostInfoGetter 用于获取主机信息（包括 AgentID）
	hostInfoGetter HostInfoGetter

	// Plugin cache / 插件缓存
	cachedPlugins            map[string][]Plugin // key: version
	pluginsCacheTime         map[string]time.Time
	catalogFetchFailureUntil map[string]time.Time
	pluginsMu                sync.RWMutex

	// Installation progress tracking / 安装进度跟踪
	installProgress   map[string]*PluginInstallStatus // key: clusterID:pluginName
	installProgressMu sync.RWMutex

	// bundled seed load markers / 内置基线加载标记
	seedLoadedVersions map[string]bool
	seedLoadedMu       sync.RWMutex
}

// NewService creates a new Service instance.
// NewService 创建一个新的 Service 实例。
func NewService(repo *Repository) *Service {
	service := &Service{
		repo:                     repo,
		downloader:               NewDownloader(config.GetPluginsDir()),
		cachedPlugins:            make(map[string][]Plugin),
		pluginsCacheTime:         make(map[string]time.Time),
		catalogFetchFailureUntil: make(map[string]time.Time),
		installProgress:          make(map[string]*PluginInstallStatus),
		seedLoadedVersions:       make(map[string]bool),
	}
	service.pluginFetcher = service.fetchPluginsFromDocs
	service.officialDocFetcher = service.fetchOfficialDocMarkdown
	service.mavenVersionLookup = service.resolveLatestMavenVersion
	return service
}

// NewServiceWithDownloader creates a new Service instance with a custom downloader.
// NewServiceWithDownloader 创建一个带有自定义下载器的新 Service 实例。
func NewServiceWithDownloader(repo *Repository, pluginsDir string) *Service {
	service := &Service{
		repo:                     repo,
		downloader:               NewDownloader(pluginsDir),
		cachedPlugins:            make(map[string][]Plugin),
		pluginsCacheTime:         make(map[string]time.Time),
		catalogFetchFailureUntil: make(map[string]time.Time),
		installProgress:          make(map[string]*PluginInstallStatus),
		seedLoadedVersions:       make(map[string]bool),
	}
	service.pluginFetcher = service.fetchPluginsFromDocs
	service.officialDocFetcher = service.fetchOfficialDocMarkdown
	service.mavenVersionLookup = service.resolveLatestMavenVersion
	return service
}

// SetClusterGetter sets the cluster getter for version validation.
// SetClusterGetter 设置集群获取器用于版本校验。
func (s *Service) SetClusterGetter(getter ClusterGetter) {
	s.clusterGetter = getter
}

// SetPluginFetcher sets the plugin list fetcher, mainly used in tests.
// SetPluginFetcher 设置插件列表获取函数，主要用于测试。
func (s *Service) SetPluginFetcher(fetcher func(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error)) {
	if fetcher == nil {
		s.pluginFetcher = s.fetchPluginsFromDocs
		return
	}
	s.pluginFetcher = fetcher
}

// ==================== Available Plugins 可用插件 ====================

// ListAvailablePlugins returns available plugins from DB snapshot or Maven repository.
// ListAvailablePlugins 从数据库快照或 Maven 仓库获取可用插件列表。
func (s *Service) ListAvailablePlugins(ctx context.Context, version string, mirror MirrorSource) (*AvailablePluginsResponse, error) {
	if version == "" {
		version = seatunnel.DefaultVersion()
	}
	if mirror == "" {
		mirror = MirrorSourceApache
	}
	if _, ok := MirrorURLs[mirror]; !ok {
		return nil, ErrInvalidMirror
	}

	plugins, sourceMirror, refreshedAt, source, cacheHit := s.getPlugins(ctx, version, mirror)
	s.ensureBundledSeedLoaded(ctx, version)
	plugins = s.enrichPluginsWithDependencyState(ctx, version, plugins)

	return &AvailablePluginsResponse{
		Plugins:             plugins,
		Total:               len(plugins),
		Version:             version,
		Mirror:              string(mirror),
		Source:              source,
		CacheHit:            cacheHit,
		CatalogSourceMirror: string(sourceMirror),
		CatalogRefreshedAt:  refreshedAt,
	}, nil
}

// getPlugins returns the plugin list, preferring persisted DB snapshots and falling back to Maven.
// getPlugins 返回插件列表，优先使用数据库快照，不存在时回退到 Maven。
func (s *Service) getPlugins(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, *time.Time, PluginListSource, bool) {
	if persisted, sourceMirror, refreshedAt := s.loadPluginsFromCatalog(ctx, version); len(persisted) > 0 {
		return persisted, sourceMirror, refreshedAt, PluginListSourceDatabase, false
	}

	seedPlugins := s.pluginsFromBundledSeed(version)
	if len(seedPlugins) > 0 && s.shouldSkipRemoteCatalogFetch(version, mirror) {
		return seedPlugins, mirror, nil, PluginListSourceSeed, true
	}

	fetcher := s.pluginFetcher
	if fetcher == nil {
		fetcher = s.fetchPluginsFromDocs
	}
	plugins, usedMirror, err := fetcher(ctx, version, mirror)
	if err != nil {
		logger.WarnF(ctx, "[Plugin] Failed to fetch plugins from Maven: %v", err)
		if len(seedPlugins) > 0 {
			s.markRemoteCatalogFetchFailed(version, mirror)
			logger.WarnF(ctx, "[Plugin] Falling back to bundled connector catalog for version %s", version)
			return seedPlugins, mirror, nil, PluginListSourceSeed, false
		}
		return []Plugin{}, mirror, nil, PluginListSourceRemote, false
	}
	plugins = s.filterHiddenPluginsForVersion(version, plugins)
	refreshedAt := time.Now()
	if err := s.persistPluginCatalog(ctx, version, plugins, PluginCatalogSourceRemote, usedMirror, refreshedAt); err != nil {
		logger.WarnF(ctx, "[Plugin] 持久化插件目录失败: %v", err)
	}
	s.clearRemoteCatalogFetchFailure(version, mirror)
	return plugins, usedMirror, &refreshedAt, PluginListSourceRemote, false
}

func (s *Service) pluginsFromBundledSeed(version string) []Plugin {
	seed, err := loadOfficialDependencySeed(version)
	if err != nil || seed == nil || len(seed.Catalog) == 0 {
		return nil
	}

	plugins := make([]Plugin, 0, len(seed.Catalog))
	for _, item := range seed.Catalog {
		name := strings.TrimSpace(item.Name)
		artifactID := strings.TrimSpace(item.ArtifactID)
		if name == "" || artifactID == "" {
			continue
		}
		docURL := fmt.Sprintf("%s/%s/connector-v2", SeaTunnelDocsBaseURL, version)
		if strings.TrimSpace(item.DocURL) != "" {
			docURL = strings.TrimSpace(item.DocURL)
			if seed.TemplateVersion != "" && version != "" {
				docURL = strings.Replace(docURL, "/docs/"+seed.TemplateVersion+"/", "/docs/"+version+"/", 1)
			}
		}

		plugins = append(plugins, Plugin{
			Name:        name,
			DisplayName: firstNonEmpty(item.DisplayName, strings.Title(strings.ReplaceAll(name, "-", " "))),
			Category:    PluginCategoryConnector,
			Version:     version,
			Description: fmt.Sprintf("SeaTunnel %s connector / SeaTunnel %s 连接器", firstNonEmpty(item.DisplayName, name), firstNonEmpty(item.DisplayName, name)),
			GroupID:     firstNonEmpty(item.GroupID, "org.apache.seatunnel"),
			ArtifactID:  artifactID,
			DocURL:      docURL,
		})
	}

	plugins = s.filterHiddenPluginsForVersion(version, plugins)
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name < plugins[j].Name
	})
	return plugins
}

func catalogFetchFailureKey(version string, mirror MirrorSource) string {
	return strings.TrimSpace(version) + ":" + string(mirror)
}

func (s *Service) shouldSkipRemoteCatalogFetch(version string, mirror MirrorSource) bool {
	key := catalogFetchFailureKey(version, mirror)
	s.pluginsMu.RLock()
	until := s.catalogFetchFailureUntil[key]
	s.pluginsMu.RUnlock()
	return !until.IsZero() && time.Now().Before(until)
}

func (s *Service) markRemoteCatalogFetchFailed(version string, mirror MirrorSource) {
	key := catalogFetchFailureKey(version, mirror)
	s.pluginsMu.Lock()
	s.catalogFetchFailureUntil[key] = time.Now().Add(10 * time.Minute)
	s.pluginsMu.Unlock()
}

func (s *Service) clearRemoteCatalogFetchFailure(version string, mirror MirrorSource) {
	key := catalogFetchFailureKey(version, mirror)
	s.pluginsMu.Lock()
	delete(s.catalogFetchFailureUntil, key)
	s.pluginsMu.Unlock()
}

// fetchPluginsFromDocs fetches plugin list from Maven repository.
// fetchPluginsFromDocs 从 Maven 仓库获取插件列表。
// Strategy: Fetch connector list from Maven repo and filter by version
// 策略：从 Maven 仓库获取连接器列表并按版本过滤
func (s *Service) fetchPluginsFromDocs(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
	// Fetch connectors from Maven repo / 从 Maven 仓库获取连接器
	connectors, usedMirror, err := s.fetchConnectorsFromMaven(ctx, version, mirror)
	if err != nil {
		return nil, mirror, fmt.Errorf("failed to fetch connectors from Maven: %w / 从 Maven 获取连接器失败: %w", err, err)
	}

	if len(connectors) == 0 {
		return nil, usedMirror, fmt.Errorf("no connectors found for version %s / 未找到版本 %s 的连接器", version, version)
	}

	return connectors, usedMirror, nil
}

// fetchConnectorsFromMaven fetches connector list from Maven repository.
// fetchConnectorsFromMaven 从 Maven 仓库获取连接器列表。
// Uses concurrent version checking for better performance.
// 使用并发版本检查以提高性能。
func (s *Service) fetchConnectorsFromMaven(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, MirrorSource, error) {
	var lastErr error
	for _, candidate := range connectorMirrorFallbackOrder(mirror) {
		logger.InfoF(ctx, "[Plugin] Fetching connectors from Maven for version %s via mirror %s", version, candidate)
		connectors, err := s.fetchConnectorsFromMirror(ctx, version, candidate)
		if err == nil {
			return connectors, candidate, nil
		}
		lastErr = err
		logger.WarnF(ctx, "[Plugin] Mirror %s connector discovery failed: %v", candidate, err)
	}
	return nil, mirror, lastErr
}

func connectorMirrorFallbackOrder(preferred MirrorSource) []MirrorSource {
	ordered := []MirrorSource{}
	add := func(item MirrorSource) {
		if _, ok := MirrorURLs[item]; !ok {
			return
		}
		for _, existing := range ordered {
			if existing == item {
				return
			}
		}
		ordered = append(ordered, item)
	}
	add(preferred)
	add(MirrorSourceAliyun)
	add(MirrorSourceHuaweiCloud)
	add(MirrorSourceApache)
	return ordered
}

func (s *Service) fetchConnectorsFromMirror(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, error) {
	// Fetch the main directory listing / 获取主目录列表
	client := &http.Client{Timeout: PluginFetchTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, connectorRepoBaseURL(mirror)+"/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	applyMavenRequestHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorF(ctx, "[Plugin] Failed to fetch Maven repo: %v", err)
		return nil, fmt.Errorf("failed to fetch Maven repo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.ErrorF(ctx, "[Plugin] Maven repo returned status %d", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse HTML to extract connector names / 解析 HTML 提取连接器名称
	pattern := `<a[^>]*href="(connector-[^/"]+)/"[^>]*>`
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(string(body), -1)

	// Filter candidates / 过滤候选连接器
	var candidates []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		artifactID := match[1]
		if strings.Contains(artifactID, "-e2e") || isSkippedModule(artifactID) || seen[artifactID] {
			continue
		}
		seen[artifactID] = true
		candidates = append(candidates, artifactID)
	}

	logger.InfoF(ctx, "[Plugin] Checking %d connector candidates concurrently", len(candidates))

	// Concurrent version checking / 并发版本检查
	const maxConcurrency = 10
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var plugins []Plugin

	for _, artifactID := range candidates {
		wg.Add(1)
		go func(aid string) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			hasVersion, err := s.checkConnectorVersion(ctx, aid, version, mirror)
			if err != nil || !hasVersion {
				return
			}

			plugin := s.createPluginFromArtifactID(aid, version)
			mu.Lock()
			plugins = append(plugins, plugin)
			mu.Unlock()
		}(artifactID)
	}

	wg.Wait()
	logger.InfoF(ctx, "[Plugin] Found %d connectors with version %s", len(plugins), version)
	return plugins, nil
}

// checkConnectorVersion checks if a connector has the specified version in Maven.
// checkConnectorVersion 检查连接器在 Maven 中是否有指定版本。
func (s *Service) checkConnectorVersion(ctx context.Context, artifactID, version string, mirror MirrorSource) (bool, error) {
	url := fmt.Sprintf("%s/%s/", connectorRepoBaseURL(mirror), artifactID)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	applyMavenRequestHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if mirror != MirrorSourceApache {
			return s.checkConnectorVersion(ctx, artifactID, version, MirrorSourceApache)
		}
		return false, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	// Check if version directory exists / 检查版本目录是否存在
	// Pattern: <a href="2.3.12/">2.3.12/</a>
	pattern := fmt.Sprintf(`<a[^>]*href="%s/"`, regexp.QuoteMeta(version))
	matched, _ := regexp.MatchString(pattern, string(body))
	return matched, nil
}

// createPluginFromArtifactID creates a Plugin from Maven artifact ID.
// createPluginFromArtifactID 从 Maven artifact ID 创建 Plugin。
// Plugin name = artifact ID without "connector-" prefix (e.g., connector-jdbc -> jdbc)
// Display name = Title case with dashes replaced by spaces
// 插件名称 = artifact ID 去掉 "connector-" 前缀
// 显示名称 = 首字母大写，横杠替换为空格
func (s *Service) createPluginFromArtifactID(artifactID, version string) Plugin {
	// Extract plugin name from artifact ID / 从 artifact ID 提取插件名称
	// connector-jdbc -> jdbc, connector-cdc-mysql -> cdc-mysql
	name := strings.TrimPrefix(artifactID, "connector-")

	// Generate display name: title case with dashes replaced by spaces
	// 生成显示名称：首字母大写，横杠替换为空格
	displayName := strings.Title(strings.ReplaceAll(name, "-", " "))

	return Plugin{
		Name:        name,
		DisplayName: displayName,
		Category:    PluginCategoryConnector,
		Version:     version,
		Description: fmt.Sprintf("SeaTunnel %s connector / SeaTunnel %s 连接器", displayName, displayName),
		GroupID:     "org.apache.seatunnel",
		ArtifactID:  artifactID,
		DocURL:      fmt.Sprintf("%s/%s/connector-v2", SeaTunnelDocsBaseURL, version),
	}
}

// getArtifactID returns the Maven artifact ID for a plugin name.
// getArtifactID 返回插件名称对应的 Maven artifact ID。
// Since we fetch from Maven directly, artifact ID = "connector-" + name
// 由于我们直接从 Maven 获取，artifact ID = "connector-" + name
func getArtifactID(name string) string {
	return fmt.Sprintf("connector-%s", name)
}

// RefreshPlugins forces a refresh of the plugin list from Maven repository.
// RefreshPlugins 强制从 Maven 仓库刷新插件列表。
func (s *Service) RefreshPlugins(ctx context.Context, version string, mirror MirrorSource) ([]Plugin, error) {
	if version == "" {
		version = seatunnel.DefaultVersion()
	}
	if mirror == "" {
		mirror = MirrorSourceApache
	}
	if _, ok := MirrorURLs[mirror]; !ok {
		return nil, ErrInvalidMirror
	}

	plugins, usedMirror, err := s.fetchPluginsFromDocs(ctx, version, mirror)
	if err != nil {
		return nil, err
	}

	refreshedAt := time.Now()
	if err := s.persistPluginCatalog(ctx, version, plugins, PluginCatalogSourceRemote, usedMirror, refreshedAt); err != nil {
		logger.WarnF(ctx, "[Plugin] 刷新插件目录后持久化失败: %v", err)
	}

	// Update cache / 更新缓存
	s.pluginsMu.Lock()
	s.cachedPlugins[version] = plugins
	s.pluginsCacheTime[version] = time.Now()
	s.pluginsMu.Unlock()

	return plugins, nil
}

// GetPluginInfo returns detailed information about a specific plugin.
// GetPluginInfo 返回特定插件的详细信息。
func (s *Service) GetPluginInfo(ctx context.Context, name string, version string) (*Plugin, error) {
	return s.getPluginInfoWithMirror(ctx, name, version, MirrorSourceApache)
}

func (s *Service) getPluginInfoWithMirror(ctx context.Context, name string, version string, mirror MirrorSource) (*Plugin, error) {
	if version == "" {
		version = seatunnel.DefaultVersion()
	}

	// Normalize name to lowercase for comparison / 将名称转换为小写进行比较
	normalizedName := strings.ToLower(name)

	// Resolve from DB snapshot or remote fetch / 从数据库快照或远端抓取中解析
	fetchedPlugins, _, _, _, _ := s.getPlugins(ctx, version, mirror)
	for _, p := range fetchedPlugins {
		if strings.ToLower(p.Name) == normalizedName {
			enriched := s.enrichPluginsWithDependencyState(ctx, version, []Plugin{p})
			if len(enriched) > 0 {
				return &enriched[0], nil
			}
			return &p, nil
		}
	}

	return nil, ErrPluginNotAvailable
}

// ==================== Installed Plugins 已安装插件 ====================

// ListInstalledPlugins returns installed plugins for a cluster.
// ListInstalledPlugins 返回集群上已安装的插件列表。
func (s *Service) ListInstalledPlugins(ctx context.Context, clusterID uint) ([]InstalledPlugin, error) {
	plugins, err := s.repo.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	plugins = s.reconcileInstalledPluginVersions(ctx, clusterID, plugins)
	plugins = s.dedupeInstalledPlugins(ctx, clusterID, plugins)

	localPlugins, err := s.downloader.ListLocalPlugins()
	if err != nil {
		logger.WarnF(ctx, "[Plugin] 加载本地插件元数据失败: %v", err)
		return plugins, nil
	}

	type localPluginMetadataView struct {
		selectedProfileKeys []string
		attachedConnectors  []string
		dependencies        []PluginDependency
	}

	localByKey := make(map[string]localPluginMetadataView, len(localPlugins))
	for _, local := range localPlugins {
		key := fmt.Sprintf("%s:%s", local.Name, local.Version)
		localByKey[key] = localPluginMetadataView{
			selectedProfileKeys: append([]string(nil), local.SelectedProfileKeys...),
			attachedConnectors:  append([]string(nil), local.AttachedConnectors...),
			dependencies:        append([]PluginDependency(nil), local.Dependencies...),
		}
	}

	for index := range plugins {
		key := fmt.Sprintf("%s:%s", plugins[index].PluginName, plugins[index].Version)
		if metadata, ok := localByKey[key]; ok {
			plugins[index].SelectedProfileKeys = metadata.selectedProfileKeys
			plugins[index].AttachedConnectors = metadata.attachedConnectors
			plugins[index].Dependencies = metadata.dependencies
		}
	}

	return plugins, nil
}

func (s *Service) dedupeInstalledPlugins(ctx context.Context, clusterID uint, plugins []InstalledPlugin) []InstalledPlugin {
	if len(plugins) <= 1 {
		return plugins
	}

	result := make([]InstalledPlugin, 0, len(plugins))
	seen := make(map[string]InstalledPlugin, len(plugins))
	for _, plugin := range plugins {
		key := strings.TrimSpace(plugin.PluginName)
		if key == "" {
			result = append(result, plugin)
			continue
		}

		existing, exists := seen[key]
		if !exists {
			seen[key] = plugin
			continue
		}

		keep := existing
		drop := plugin
		if plugin.UpdatedAt.After(existing.UpdatedAt) || plugin.InstalledAt.After(existing.InstalledAt) {
			keep = plugin
			drop = existing
			seen[key] = plugin
		}

		if drop.ID > 0 {
			if err := s.repo.Delete(ctx, drop.ID); err != nil {
				logger.WarnF(ctx, "[Plugin] 删除集群 %d 重复插件记录失败: plugin=%s, id=%d, err=%v", clusterID, drop.PluginName, drop.ID, err)
			} else {
				logger.InfoF(ctx, "[Plugin] 已清理集群 %d 的重复插件记录: plugin=%s, removed_id=%d, kept_id=%d", clusterID, drop.PluginName, drop.ID, keep.ID)
			}
		}
	}

	for _, plugin := range plugins {
		key := strings.TrimSpace(plugin.PluginName)
		if key == "" {
			continue
		}
		if kept, ok := seen[key]; ok && kept.ID == plugin.ID {
			result = append(result, kept)
			delete(seen, key)
		}
	}

	return result
}

func (s *Service) reconcileInstalledPluginVersions(ctx context.Context, clusterID uint, plugins []InstalledPlugin) []InstalledPlugin {
	if len(plugins) == 0 || s.clusterGetter == nil {
		return plugins
	}

	clusterVersion, err := s.clusterGetter.GetClusterVersion(ctx, clusterID)
	if err != nil || strings.TrimSpace(clusterVersion) == "" {
		return plugins
	}
	clusterVersion = strings.TrimSpace(clusterVersion)
	now := time.Now()

	for index := range plugins {
		if strings.TrimSpace(plugins[index].Version) == clusterVersion {
			continue
		}

		plugins[index].Version = clusterVersion
		if strings.TrimSpace(plugins[index].ArtifactID) != "" {
			plugins[index].InstallPath = fmt.Sprintf("connectors/%s-%s.jar", plugins[index].ArtifactID, clusterVersion)
		}
		plugins[index].UpdatedAt = now

		if updateErr := s.repo.Update(ctx, &plugins[index]); updateErr != nil {
			logger.WarnF(
				ctx,
				"[Plugin] 对齐集群 %d 已安装插件 %s 版本到 %s 失败: %v",
				clusterID,
				plugins[index].PluginName,
				clusterVersion,
				updateErr,
			)
		}
	}

	return plugins
}

// GetInstalledPlugin returns an installed plugin by cluster and name.
// GetInstalledPlugin 通过集群和名称获取已安装插件。
func (s *Service) GetInstalledPlugin(ctx context.Context, clusterID uint, pluginName string) (*InstalledPlugin, error) {
	return s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
}

// ==================== Plugin Installation 插件安装 ====================

// InstallPlugin installs a plugin on a cluster via Agent.
// InstallPlugin 通过 Agent 在集群上安装插件。
// Requirements: Validates that plugin version matches cluster version.
// 需求：校验插件版本与集群版本是否匹配。
func (s *Service) InstallPlugin(ctx context.Context, clusterID uint, req *InstallPluginRequest) (*InstalledPlugin, error) {
	// Delegate to InstallPluginToCluster which handles the full installation flow
	// 委托给 InstallPluginToCluster 处理完整的安装流程
	return s.InstallPluginToCluster(ctx, clusterID, req)
}

// UninstallPlugin uninstalls a plugin from a cluster.
// UninstallPlugin 从集群上卸载插件。
// Sends uninstall_plugin command to each cluster node's agent to remove plugin files from install dir, then deletes the DB record.
// 向集群各节点 Agent 发送 uninstall_plugin 命令以从安装目录删除插件文件，再删除数据库记录。
func (s *Service) UninstallPlugin(ctx context.Context, clusterID uint, pluginName string) error {
	plugin, err := s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
	if err != nil {
		return err
	}

	// Send uninstall command to each node's agent so plugin files are removed from install dir
	// 向各节点 Agent 发送卸载命令，使安装目录中的插件文件被删除
	if s.agentCommandSender != nil && s.clusterNodeGetter != nil && s.hostInfoGetter != nil {
		nodes, err := s.clusterNodeGetter.GetClusterNodes(ctx, clusterID)
		if err != nil {
			logger.WarnF(ctx, "[Plugin] Uninstall: failed to get cluster nodes: %v / 卸载：获取集群节点失败: %v", err, err)
		} else {
			for _, node := range nodes {
				agentID, err := s.hostInfoGetter.GetHostAgentID(ctx, node.HostID)
				if err != nil || agentID == "" {
					logger.WarnF(ctx, "[Plugin] Uninstall: skip node %d (host %d), no agent / 卸载：跳过节点 %d（主机 %d），无 Agent", node.NodeID, node.HostID, node.NodeID, node.HostID)
					continue
				}
				params := map[string]string{
					"plugin_name":         pluginName,
					"version":             plugin.Version,
					"install_path":        node.InstallDir,
					"remove_dependencies": "true",
				}
				success, message, sendErr := s.agentCommandSender.SendCommand(ctx, agentID, "uninstall_plugin", params)
				if sendErr != nil {
					logger.WarnF(ctx, "[Plugin] Uninstall: send to agent %s failed: %v / 卸载：向 Agent %s 发送失败: %v", agentID, sendErr, agentID, sendErr)
					continue
				}
				if !success {
					logger.WarnF(ctx, "[Plugin] Uninstall: agent %s returned failure: %s / 卸载：Agent %s 返回失败: %s", agentID, message, agentID, message)
				}
			}
		}
	}

	// Delete installed plugin record / 删除已安装插件记录
	return s.repo.DeleteByClusterAndName(ctx, clusterID, pluginName)
}

// EnablePlugin enables an installed plugin.
// EnablePlugin 启用已安装的插件。
func (s *Service) EnablePlugin(ctx context.Context, clusterID uint, pluginName string) (*InstalledPlugin, error) {
	plugin, err := s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
	if err != nil {
		return nil, err
	}

	plugin.Status = PluginStatusEnabled
	plugin.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, plugin); err != nil {
		return nil, err
	}

	return plugin, nil
}

// DisablePlugin disables an installed plugin.
// DisablePlugin 禁用已安装的插件。
func (s *Service) DisablePlugin(ctx context.Context, clusterID uint, pluginName string) (*InstalledPlugin, error) {
	plugin, err := s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
	if err != nil {
		return nil, err
	}

	plugin.Status = PluginStatusDisabled
	plugin.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, plugin); err != nil {
		return nil, err
	}

	return plugin, nil
}

// ==================== Plugin Download Methods 插件下载方法 ====================

// DownloadPlugin downloads a plugin to the Control Plane local storage.
// DownloadPlugin 下载插件到 Control Plane 本地存储。
func (s *Service) DownloadPlugin(ctx context.Context, name, version string, mirror MirrorSource, profileKeys []string) (*DownloadProgress, error) {
	if version == "" {
		version = seatunnel.DefaultVersion() // Default version / 默认版本
	}

	if mirror == "" {
		mirror = MirrorSourceApache
	}

	// Get plugin info / 获取插件信息
	plugin, err := s.getPluginInfoWithMirror(ctx, name, version, mirror)
	if err != nil {
		return nil, err
	}

	// Ensure artifact_id is set / 确保 artifact_id 已设置
	if plugin.ArtifactID == "" {
		plugin.ArtifactID = getArtifactID(name)
		fmt.Printf("[DownloadPlugin] Warning: plugin.ArtifactID was empty for %s, set to: %s\n", name, plugin.ArtifactID)
	}
	fmt.Printf("[DownloadPlugin] Plugin: name=%s, artifactID=%s, version=%s\n", plugin.Name, plugin.ArtifactID, plugin.Version)

	selectedProfiles := normalizeProfileKeys(profileKeys)

	// Load configured dependencies from database / 从数据库加载配置的依赖
	deps, err := s.GetPluginDependenciesForVersionAndProfiles(ctx, name, version, selectedProfiles)
	if err == nil && len(deps) > 0 {
		plugin.Dependencies = deps
		fmt.Printf("[DownloadPlugin] Loaded %d dependencies for %s\n", len(deps), name)
	}

	connectorReady := s.downloader.IsConnectorDownloaded(name, version)
	dependenciesReady := s.arePluginDependenciesDownloaded(plugin)

	// Check if already downloaded / 检查是否已下载
	if connectorReady && dependenciesReady {
		return &DownloadProgress{
			PluginName:          name,
			Version:             version,
			Status:              "completed",
			Progress:            100,
			CurrentStep:         "Already downloaded / 已下载",
			SelectedProfileKeys: selectedProfiles,
			ConnectorCount:      1,
			ConnectorCompleted:  1,
			DependencyCount:     len(plugin.Dependencies),
			DependencyCompleted: len(plugin.Dependencies),
		}, nil
	}

	// Start download in background / 在后台开始下载
	go func() {
		downloadCtx := context.Background()
		if err := s.downloader.DownloadPlugin(downloadCtx, plugin, mirror, selectedProfiles, connectorReady, dependenciesReady, nil); err != nil {
			fmt.Printf("[Plugin Download Error] plugin=%s, version=%s, error=%v\n", name, version, err)
			return
		}
	}()

	// Return initial progress / 返回初始进度
	return &DownloadProgress{
		PluginName:          name,
		Version:             version,
		Status:              "downloading",
		Progress:            0,
		CurrentStep:         "Starting download / 开始下载",
		StartTime:           time.Now(),
		SelectedProfileKeys: selectedProfiles,
		ConnectorCount:      1,
		ConnectorCompleted:  0,
		DependencyCount:     len(plugin.Dependencies),
		DependencyCompleted: 0,
	}, nil
}

// GetDownloadStatus returns the current download status for a plugin.
// GetDownloadStatus 返回插件的当前下载状态。
func (s *Service) GetDownloadStatus(name, version string, profileKeys []string) *DownloadProgress {
	// Check if download is in progress / 检查是否正在下载
	progress := s.downloader.GetDownloadProgress(name, version)
	if progress != nil {
		return progress
	}

	selectedProfiles := normalizeProfileKeys(profileKeys)
	plugin := &Plugin{Name: name, Version: version}
	if info, err := s.GetPluginInfo(context.Background(), name, version); err == nil && info != nil {
		plugin = info
		if deps, depErr := s.GetPluginDependenciesForVersionAndProfiles(context.Background(), name, version, selectedProfiles); depErr == nil {
			plugin.Dependencies = deps
		}
	}

	// Check if already downloaded / 检查是否已下载
	if s.downloader.IsConnectorDownloaded(name, version) && s.arePluginDependenciesDownloaded(plugin) {
		return &DownloadProgress{
			PluginName:          name,
			Version:             version,
			Status:              "completed",
			Progress:            100,
			CurrentStep:         "Downloaded / 已下载",
			SelectedProfileKeys: selectedProfiles,
			ConnectorCount:      1,
			ConnectorCompleted:  1,
			DependencyCount:     len(plugin.Dependencies),
			DependencyCompleted: len(plugin.Dependencies),
		}
	}

	// Not downloaded / 未下载
	return &DownloadProgress{
		PluginName:          name,
		Version:             version,
		Status:              "not_started",
		Progress:            0,
		CurrentStep:         "Not downloaded / 未下载",
		SelectedProfileKeys: selectedProfiles,
		ConnectorCount:      1,
		ConnectorCompleted:  0,
		DependencyCount:     len(plugin.Dependencies),
		DependencyCompleted: 0,
	}
}

// ListLocalPlugins returns a list of locally downloaded plugins.
// ListLocalPlugins 返回本地已下载的插件列表。
func (s *Service) ListLocalPlugins() ([]LocalPlugin, error) {
	plugins, err := s.downloader.ListLocalPlugins()
	if err != nil {
		return nil, err
	}
	for i := range plugins {
		if len(plugins[i].Dependencies) > 0 {
			continue
		}
		deps, depErr := s.GetPluginDependenciesForVersionAndProfiles(
			context.Background(),
			plugins[i].Name,
			plugins[i].Version,
			plugins[i].SelectedProfileKeys,
		)
		if depErr != nil || len(deps) == 0 {
			continue
		}
		plugins[i].Dependencies = deps
		attached := make([]string, 0)
		seenAttached := make(map[string]struct{})
		for _, dep := range deps {
			if strings.TrimSpace(dep.TargetDir) == "connectors" {
				if _, exists := seenAttached[dep.ArtifactID]; exists {
					continue
				}
				seenAttached[dep.ArtifactID] = struct{}{}
				attached = append(attached, dep.ArtifactID)
			}
		}
		plugins[i].AttachedConnectors = attached
	}
	return plugins, nil
}

// DownloadAllPluginsProgress represents the progress of downloading all plugins.
// DownloadAllPluginsProgress 表示下载所有插件的进度。
type DownloadAllPluginsProgress struct {
	Total      int    `json:"total"`      // 总插件数 / Total plugins
	Downloaded int    `json:"downloaded"` // 已下载数 / Downloaded count
	Failed     int    `json:"failed"`     // 失败数 / Failed count
	Skipped    int    `json:"skipped"`    // 跳过数（已存在）/ Skipped count (already exists)
	Status     string `json:"status"`     // 状态 / Status
	Message    string `json:"message"`    // 消息 / Message
}

// DownloadAllPlugins downloads all available plugins for a version.
// DownloadAllPlugins 下载指定版本的所有可用插件。
func (s *Service) DownloadAllPlugins(ctx context.Context, version string, mirror MirrorSource, selectedProfilesByPlugin map[string][]string) (*DownloadAllPluginsProgress, error) {
	if version == "" {
		version = seatunnel.DefaultVersion()
	}
	if mirror == "" {
		mirror = MirrorSourceApache
	}

	// Get all available plugins / 获取所有可用插件
	plugins, _, _, _, _ := s.getPlugins(ctx, version, mirror)
	if len(plugins) == 0 {
		return nil, fmt.Errorf("no plugins found for version %s / 未找到版本 %s 的插件", version, version)
	}

	progress := &DownloadAllPluginsProgress{
		Total:   len(plugins),
		Status:  "downloading",
		Message: fmt.Sprintf("Starting download of %d plugins / 开始下载 %d 个插件", len(plugins), len(plugins)),
	}

	// Start downloading all plugins in background / 在后台开始下载所有插件
	go func() {
		downloadCtx := context.Background()
		for i := range plugins {
			plugin := &plugins[i]
			selectedProfiles := normalizeProfileKeys(selectedProfilesByPlugin[plugin.Name])

			// Check if already downloaded / 检查是否已下载
			if s.downloader.IsConnectorDownloaded(plugin.Name, version) {
				progress.Skipped++
				continue
			}

			if requiresExplicitProfileSelection(plugin) && len(selectedProfiles) == 0 {
				progress.Skipped++
				fmt.Printf("[DownloadAllPlugins] Skipped %s because profile selection is required\n", plugin.Name)
				continue
			}

			// Load configured dependencies from database / 从数据库加载配置的依赖
			deps, err := s.GetPluginDependenciesForVersionAndProfiles(downloadCtx, plugin.Name, version, selectedProfiles)
			if err == nil && len(deps) > 0 {
				plugin.Dependencies = deps
			}

			// Download plugin / 下载插件
			err = s.downloader.DownloadPlugin(downloadCtx, plugin, mirror, selectedProfiles, false, false, nil)
			if err != nil {
				progress.Failed++
				fmt.Printf("[DownloadAllPlugins] Failed to download %s: %v\n", plugin.Name, err)
			} else {
				progress.Downloaded++
				fmt.Printf("[DownloadAllPlugins] Downloaded %s successfully\n", plugin.Name)
			}
		}
		progress.Status = "completed"
		progress.Message = fmt.Sprintf("Download completed: %d downloaded, %d skipped, %d failed / 下载完成: %d 已下载, %d 已跳过, %d 失败",
			progress.Downloaded, progress.Skipped, progress.Failed,
			progress.Downloaded, progress.Skipped, progress.Failed)
	}()

	return progress, nil
}

func requiresExplicitProfileSelection(plugin *Plugin) bool {
	if plugin == nil {
		return false
	}
	return plugin.ArtifactID == "connector-jdbc" || plugin.Name == "jdbc"
}

// DeleteLocalPlugin deletes a locally downloaded plugin.
// DeleteLocalPlugin 删除本地已下载的插件。
func (s *Service) DeleteLocalPlugin(name, version string) error {
	return s.downloader.DeleteLocalPlugin(name, version)
}

// IsPluginDownloaded checks if a plugin is downloaded locally.
// IsPluginDownloaded 检查插件是否已在本地下载。
func (s *Service) IsPluginDownloaded(name, version string) bool {
	return s.downloader.IsConnectorDownloaded(name, version)
}

// ListActiveDownloads returns all active download tasks.
// ListActiveDownloads 返回所有活动的下载任务。
func (s *Service) ListActiveDownloads() []*DownloadProgress {
	return s.downloader.ListActiveDownloads()
}

// ==================== Plugin Installation Progress Methods 插件安装进度方法 ====================

// GetInstallProgress returns the installation progress for a plugin on a cluster.
// GetInstallProgress 返回集群上插件的安装进度。
func (s *Service) GetInstallProgress(clusterID uint, pluginName string) *PluginInstallStatus {
	key := fmt.Sprintf("%d:%s", clusterID, pluginName)

	s.installProgressMu.RLock()
	defer s.installProgressMu.RUnlock()

	if progress, exists := s.installProgress[key]; exists {
		return progress
	}

	return nil
}

// setInstallProgress sets the installation progress for a plugin on a cluster.
// setInstallProgress 设置集群上插件的安装进度。
func (s *Service) setInstallProgress(clusterID uint, pluginName string, status *PluginInstallStatus) {
	key := fmt.Sprintf("%d:%s", clusterID, pluginName)

	s.installProgressMu.Lock()
	defer s.installProgressMu.Unlock()

	s.installProgress[key] = status
}

// clearInstallProgress clears the installation progress for a plugin on a cluster.
// clearInstallProgress 清除集群上插件的安装进度。
func (s *Service) clearInstallProgress(clusterID uint, pluginName string) {
	key := fmt.Sprintf("%d:%s", clusterID, pluginName)

	s.installProgressMu.Lock()
	defer s.installProgressMu.Unlock()

	delete(s.installProgress, key)
}

// ==================== Cluster Plugin Installation Methods 集群插件安装方法 ====================

// AgentCommandSender is an interface for sending commands to agents.
// AgentCommandSender 是向 Agent 发送命令的接口。
type AgentCommandSender interface {
	SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error)
}

// SetAgentCommandSender sets the agent command sender for plugin installation.
// SetAgentCommandSender 设置用于插件安装的 Agent 命令发送器。
func (s *Service) SetAgentCommandSender(sender AgentCommandSender) {
	s.agentCommandSender = sender
}

// SetClusterNodeGetter sets the cluster node getter for plugin installation.
// SetClusterNodeGetter 设置用于插件安装的集群节点获取器。
func (s *Service) SetClusterNodeGetter(getter ClusterNodeGetter) {
	s.clusterNodeGetter = getter
}

// SetHostInfoGetter sets the host info getter for plugin installation.
// SetHostInfoGetter 设置用于插件安装的主机信息获取器。
func (s *Service) SetHostInfoGetter(getter HostInfoGetter) {
	s.hostInfoGetter = getter
}

// InstallPluginToCluster installs a plugin to all nodes in a cluster.
// InstallPluginToCluster 将插件安装到集群中的所有节点。
// This method:
// 1. Checks if plugin is downloaded locally (downloads if not)
// 2. Gets all cluster nodes
// 3. Transfers plugin files to each node's Agent
// 4. Sends install command to each Agent
// 5. Updates database record
func (s *Service) InstallPluginToCluster(ctx context.Context, clusterID uint, req *InstallPluginRequest) (*InstalledPlugin, error) {
	// Validate plugin version matches cluster version / 校验插件版本与集群版本是否匹配
	if s.clusterGetter != nil {
		clusterVersion, err := s.clusterGetter.GetClusterVersion(ctx, clusterID)
		if err != nil {
			return nil, err
		}
		if clusterVersion == "" {
			return nil, ErrClusterVersionEmpty
		}
		if req.Version != clusterVersion {
			return nil, fmt.Errorf("%w: plugin version %s, cluster version %s", ErrVersionMismatch, req.Version, clusterVersion)
		}
	}

	// Check if plugin already installed / 检查插件是否已安装
	exists, err := s.repo.ExistsByClusterAndName(ctx, clusterID, req.PluginName)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrPluginAlreadyExists
	}

	// Get plugin info / 获取插件信息
	pluginInfo, err := s.GetPluginInfo(ctx, req.PluginName, req.Version)
	if err != nil {
		return nil, err
	}

	// Initialize progress / 初始化进度
	progress := &PluginInstallStatus{
		PluginName: req.PluginName,
		Status:     "downloading",
		Progress:   0,
		Message:    "Checking local plugin files / 检查本地插件文件",
	}
	s.setInstallProgress(clusterID, req.PluginName, progress)

	// Check if plugin is downloaded locally / 检查插件是否已在本地下载
	effectiveDeps, err := s.GetPluginDependenciesForVersionAndProfiles(ctx, req.PluginName, req.Version, req.ProfileKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve plugin dependencies: %w", err)
	}
	pluginInfo.Dependencies = effectiveDeps

	connectorReady := s.downloader.IsConnectorDownloaded(req.PluginName, req.Version)
	dependenciesReady := s.arePluginDependenciesDownloaded(pluginInfo)
	if !connectorReady || !dependenciesReady {
		progress.Message = "Downloading plugin / 下载插件"
		s.setInstallProgress(clusterID, req.PluginName, progress)

		// Download plugin / 下载插件
		mirror := req.Mirror
		if mirror == "" {
			mirror = MirrorSourceApache
		}

		progressCallback := func(p *DownloadProgress) {
			progress.Progress = p.Progress / 2 // First half is download / 前半部分是下载
			progress.Message = p.CurrentStep
			s.setInstallProgress(clusterID, req.PluginName, progress)
		}
		if !connectorReady {
			if err := s.downloader.DownloadConnector(ctx, pluginInfo, mirror, progressCallback); err != nil {
				progress.Status = "failed"
				progress.Error = err.Error()
				s.setInstallProgress(clusterID, req.PluginName, progress)
				return nil, fmt.Errorf("failed to download plugin connector: %w", err)
			}
		}
		if !dependenciesReady {
			if err := s.downloader.DownloadDependencies(ctx, pluginInfo, mirror, progressCallback); err != nil {
				progress.Status = "failed"
				progress.Error = err.Error()
				s.setInstallProgress(clusterID, req.PluginName, progress)
				return nil, fmt.Errorf("failed to download plugin dependencies: %w", err)
			}
		}
	}

	// Update progress / 更新进度
	progress.Progress = 50
	progress.Status = "installing"
	progress.Message = "Plugin downloaded, preparing installation / 插件已下载，准备安装"
	s.setInstallProgress(clusterID, req.PluginName, progress)

	// Get cluster nodes / 获取集群节点
	// Log dependency status for debugging / 记录依赖状态用于调试
	fmt.Printf("[Plugin Install] Dependencies: clusterNodeGetter=%v, agentCommandSender=%v, hostInfoGetter=%v\n",
		s.clusterNodeGetter != nil, s.agentCommandSender != nil, s.hostInfoGetter != nil)
	fmt.Printf("[Plugin Install] Installing plugin %s v%s to cluster %d\n", req.PluginName, req.Version, clusterID)

	// Get artifact ID from plugin info, use mapping as fallback
	// 从插件信息获取 artifact ID，使用映射作为备用
	artifactID := pluginInfo.ArtifactID
	if artifactID == "" {
		artifactID = getArtifactID(req.PluginName)
	}
	fmt.Printf("[Plugin Install] Plugin %s -> ArtifactID: %s\n", req.PluginName, artifactID)

	if s.clusterNodeGetter != nil && s.agentCommandSender != nil && s.hostInfoGetter != nil {
		nodes, err := s.clusterNodeGetter.GetClusterNodes(ctx, clusterID)
		if err != nil {
			progress.Status = "failed"
			progress.Error = fmt.Sprintf("Failed to get cluster nodes: %v / 获取集群节点失败: %v", err, err)
			s.setInstallProgress(clusterID, req.PluginName, progress)
			return nil, fmt.Errorf("failed to get cluster nodes: %w", err)
		}

		fmt.Printf("[Plugin Install] Found %d nodes in cluster %d\n", len(nodes), clusterID)

		if len(nodes) == 0 {
			progress.Status = "failed"
			progress.Error = "No nodes found in cluster / 集群中没有节点"
			s.setInstallProgress(clusterID, req.PluginName, progress)
			return nil, fmt.Errorf("no nodes found in cluster")
		}

		// Transfer and install plugin to each node / 将插件传输并安装到每个节点
		totalNodes := len(nodes)
		for i, node := range nodes {
			// Update progress / 更新进度
			nodeProgress := 50 + (i * 50 / totalNodes)
			progress.Progress = nodeProgress
			progress.Message = fmt.Sprintf("Installing to node %d/%d / 正在安装到节点 %d/%d", i+1, totalNodes, i+1, totalNodes)
			s.setInstallProgress(clusterID, req.PluginName, progress)

			// Get agent ID for this host / 获取此主机的 Agent ID
			agentID, err := s.hostInfoGetter.GetHostAgentID(ctx, node.HostID)
			if err != nil {
				progress.Status = "failed"
				progress.Error = fmt.Sprintf("Failed to get agent ID for host %d: %v / 获取主机 %d 的 Agent ID 失败: %v", node.HostID, err, node.HostID, err)
				s.setInstallProgress(clusterID, req.PluginName, progress)
				return nil, fmt.Errorf("failed to get agent ID for host %d: %w", node.HostID, err)
			}

			fmt.Printf("[Plugin Install] Node %d: HostID=%d, AgentID=%s, InstallDir=%s\n", node.NodeID, node.HostID, agentID, node.InstallDir)

			if agentID == "" {
				progress.Status = "failed"
				progress.Error = fmt.Sprintf("Agent not installed on host %d / 主机 %d 未安装 Agent", node.HostID, node.HostID)
				s.setInstallProgress(clusterID, req.PluginName, progress)
				return nil, fmt.Errorf("agent not installed on host %d", node.HostID)
			}

			// Transfer plugin file to agent using artifact ID / 使用 artifact ID 传输插件文件到 Agent
			fmt.Printf("[Plugin Install] Transferring plugin %s (artifact: %s) to agent %s...\n", req.PluginName, artifactID, agentID)
			if err := s.transferPluginToAgent(ctx, agentID, artifactID, req.PluginName, req.Version, node.InstallDir, effectiveDeps); err != nil {
				progress.Status = "failed"
				progress.Error = fmt.Sprintf("Failed to transfer plugin to node %d: %v / 传输插件到节点 %d 失败: %v", node.NodeID, err, node.NodeID, err)
				s.setInstallProgress(clusterID, req.PluginName, progress)
				return nil, fmt.Errorf("failed to transfer plugin to node %d: %w", node.NodeID, err)
			}
		}
	}

	// Create database record / 创建数据库记录
	installed := &InstalledPlugin{
		ClusterID:   clusterID,
		PluginName:  req.PluginName,
		ArtifactID:  artifactID,
		Category:    pluginInfo.Category,
		Version:     req.Version,
		Status:      PluginStatusInstalled,
		InstallPath: fmt.Sprintf("connectors/%s-%s.jar", artifactID, req.Version),
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.repo.Create(ctx, installed); err != nil {
		progress.Status = "failed"
		progress.Error = err.Error()
		s.setInstallProgress(clusterID, req.PluginName, progress)
		return nil, err
	}
	if err := s.recordAttachedConnectorPlugins(ctx, clusterID, req.Version, artifactID, effectiveDeps); err != nil {
		progress.Status = "failed"
		progress.Error = err.Error()
		s.setInstallProgress(clusterID, req.PluginName, progress)
		return nil, err
	}

	// Mark as completed / 标记为完成
	progress.Status = "completed"
	progress.Progress = 100
	progress.Message = "Plugin installed successfully / 插件安装成功"
	s.setInstallProgress(clusterID, req.PluginName, progress)

	// Clear progress after a delay / 延迟后清除进度
	go func() {
		time.Sleep(30 * time.Second)
		s.clearInstallProgress(clusterID, req.PluginName)
	}()

	return installed, nil
}

// recordAttachedConnectorPlugins records companion connector dependencies as installed plugins.
// recordAttachedConnectorPlugins 将伴生 connector 依赖按普通已安装插件写入数据库。
func (s *Service) recordAttachedConnectorPlugins(ctx context.Context, clusterID uint, version, primaryArtifactID string, deps []PluginDependency) error {
	now := time.Now()
	for _, dep := range deps {
		if strings.TrimSpace(dep.TargetDir) != "connectors" {
			continue
		}
		artifactID := strings.TrimSpace(dep.ArtifactID)
		if artifactID == "" || artifactID == strings.TrimSpace(primaryArtifactID) {
			continue
		}
		pluginName := strings.TrimPrefix(artifactID, "connector-")
		if strings.TrimSpace(pluginName) == "" {
			continue
		}
		exists, err := s.repo.ExistsByClusterAndName(ctx, clusterID, pluginName)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		depVersion := strings.TrimSpace(dep.Version)
		if depVersion == "" {
			depVersion = version
		}
		if err := s.repo.Create(ctx, &InstalledPlugin{
			ClusterID:   clusterID,
			PluginName:  pluginName,
			ArtifactID:  artifactID,
			Category:    PluginCategoryConnector,
			Version:     depVersion,
			Status:      PluginStatusInstalled,
			InstallPath: fmt.Sprintf("connectors/%s-%s.jar", artifactID, depVersion),
			InstalledAt: now,
			UpdatedAt:   now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UninstallPluginFromCluster uninstalls a plugin from all nodes in a cluster.
// UninstallPluginFromCluster 从集群中的所有节点卸载插件。
func (s *Service) UninstallPluginFromCluster(ctx context.Context, clusterID uint, pluginName string) error {
	// Check if plugin exists / 检查插件是否存在
	plugin, err := s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
	if err != nil {
		return err
	}

	// Initialize progress / 初始化进度
	progress := &PluginInstallStatus{
		PluginName: pluginName,
		Status:     "uninstalling",
		Progress:   0,
		Message:    "Uninstalling plugin / 正在卸载插件",
	}
	s.setInstallProgress(clusterID, pluginName, progress)

	// TODO: Get cluster nodes and send uninstall commands to each Agent
	// TODO: 获取集群节点并向每个 Agent 发送卸载命令

	// Delete database record / 删除数据库记录
	if err := s.repo.Delete(ctx, plugin.ID); err != nil {
		progress.Status = "failed"
		progress.Error = err.Error()
		s.setInstallProgress(clusterID, pluginName, progress)
		return err
	}

	// Mark as completed / 标记为完成
	progress.Status = "completed"
	progress.Progress = 100
	progress.Message = "Plugin uninstalled successfully / 插件卸载成功"
	s.setInstallProgress(clusterID, pluginName, progress)

	// Clear progress after a delay / 延迟后清除进度
	go func() {
		time.Sleep(30 * time.Second)
		s.clearInstallProgress(clusterID, pluginName)
	}()

	return nil
}

// ==================== Plugin Transfer Methods 插件传输方法 ====================

// transferPluginToAgent transfers a plugin file and its dependencies to an Agent and installs it.
// transferPluginToAgent 将插件文件及其依赖传输到 Agent 并安装。
// This method:
// 1. Reads the plugin connector file from local storage
// 2. Sends connector file chunks to Agent via TRANSFER_PLUGIN command (file_type: connector)
// 3. Reads and transfers dependency files (file_type: dependency)
// 4. Sends INSTALL_PLUGIN command to finalize installation
// Parameters:
// - artifactID: Maven artifact ID (e.g., connector-cdc-mysql, connector-file-cos)
// - pluginName: Plugin display name (e.g., mysql-cdc, cosfile)
func (s *Service) transferPluginToAgent(ctx context.Context, agentID, artifactID, pluginName, version, installDir string, deps []PluginDependency) error {
	if s.agentCommandSender == nil {
		return fmt.Errorf("agent command sender not configured / Agent 命令发送器未配置")
	}

	// 1. Transfer connector file / 传输连接器文件
	// Use artifact ID directly for file name / 直接使用 artifact ID 作为文件名
	connectorFileName := fmt.Sprintf("%s-%s.jar", artifactID, version)

	// Read plugin file using artifact ID / 使用 artifact ID 读取插件文件
	fileData, err := s.downloader.ReadPluginFileByArtifactID(artifactID, version)
	if err != nil {
		return fmt.Errorf("failed to read plugin file: %w / 读取插件文件失败: %w", err, err)
	}

	// Transfer connector file in chunks / 分块传输连接器文件
	if err := s.transferFileToAgent(ctx, agentID, pluginName, version, "connector", "connectors", connectorFileName, fileData, installDir); err != nil {
		return fmt.Errorf("failed to transfer connector: %w / 传输连接器失败: %w", err, err)
	}

	// 2. Transfer dependencies / 传输依赖
	if len(deps) > 0 {
		fmt.Printf("[Plugin Transfer] Transferring %d dependencies for plugin %s\n", len(deps), pluginName)

		for _, dep := range deps {
			// Check if dependency is downloaded / 检查依赖是否已下载
			if !s.downloader.IsDependencyDownloaded(dep.ArtifactID, dep.Version, version, dep.TargetDir) {
				fmt.Printf("[Plugin Transfer] Warning: dependency %s-%s not downloaded, skipping\n", dep.ArtifactID, dep.Version)
				continue
			}

			// Read dependency file / 读取依赖文件
			depPath := s.downloader.GetDependencyPath(dep.ArtifactID, dep.Version, version, dep.TargetDir)
			depData, err := s.readFile(depPath)
			if err != nil {
				fmt.Printf("[Plugin Transfer] Warning: failed to read dependency %s: %v, skipping\n", dep.ArtifactID, err)
				continue
			}

			// Transfer dependency file / 传输依赖文件
			depFileName := fmt.Sprintf("%s-%s.jar", dep.ArtifactID, dep.Version)
			if err := s.transferFileToAgent(ctx, agentID, pluginName, version, "dependency", dep.TargetDir, depFileName, depData, installDir); err != nil {
				fmt.Printf("[Plugin Transfer] Warning: failed to transfer dependency %s: %v, skipping\n", dep.ArtifactID, err)
				continue
			}

			fmt.Printf("[Plugin Transfer] Dependency transferred: %s\n", depFileName)
		}
	}

	// 3. Send install command / 发送安装命令
	// Pass artifact_id so Agent can find the file directly
	// 传递 artifact_id 以便 Agent 可以直接找到文件
	installParams := map[string]string{
		"plugin_name":  pluginName,
		"artifact_id":  artifactID,
		"version":      version,
		"install_path": installDir,
	}

	success, message, err := s.agentCommandSender.SendCommand(ctx, agentID, "install_plugin", installParams)
	if err != nil {
		return fmt.Errorf("failed to send install command: %w / 发送安装命令失败: %w", err, err)
	}
	if !success {
		return fmt.Errorf("plugin installation failed: %s / 插件安装失败: %s", message, message)
	}

	return nil
}

// transferFileToAgent transfers a single file to an Agent in chunks.
// transferFileToAgent 分块传输单个文件到 Agent。
func (s *Service) transferFileToAgent(ctx context.Context, agentID, pluginName, version, fileType, targetDir, fileName string, fileData []byte, installDir string) error {
	// Transfer file in chunks / 分块传输文件
	// Chunk size: 1MB / 块大小: 1MB
	const chunkSize = 1024 * 1024
	totalSize := int64(len(fileData))
	var offset int64 = 0

	for offset < totalSize {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}

		chunk := fileData[offset:end]
		isLast := end >= totalSize

		// Encode chunk as base64 / 将块编码为 base64
		chunkBase64 := encodeBase64(chunk)

		// Send transfer command / 发送传输命令
		params := map[string]string{
			"plugin_name":  pluginName,
			"version":      version,
			"file_type":    fileType,
			"target_dir":   targetDir,
			"file_name":    fileName,
			"chunk":        chunkBase64,
			"offset":       fmt.Sprintf("%d", offset),
			"total_size":   fmt.Sprintf("%d", totalSize),
			"is_last":      fmt.Sprintf("%t", isLast),
			"install_path": installDir,
		}

		success, message, err := s.agentCommandSender.SendCommand(ctx, agentID, "transfer_plugin", params)
		if err != nil {
			return fmt.Errorf("failed to transfer chunk at offset %d: %w / 传输偏移 %d 处的块失败: %w", offset, err, offset, err)
		}
		if !success {
			return fmt.Errorf("transfer chunk failed: %s / 传输块失败: %s", message, message)
		}

		offset = end
	}

	return nil
}

func (s *Service) arePluginDependenciesDownloaded(plugin *Plugin) bool {
	if plugin == nil {
		return false
	}
	for _, dep := range plugin.Dependencies {
		if !s.downloader.IsDependencyDownloaded(dep.ArtifactID, dep.Version, plugin.Version, dep.TargetDir) {
			return false
		}
	}
	return true
}

// readFile reads a file from the filesystem.
// readFile 从文件系统读取文件。
func (s *Service) readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// encodeBase64 encodes data to base64 string.
// encodeBase64 将数据编码为 base64 字符串。
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// GetPluginDependencies returns effective dependencies for a plugin using the default SeaTunnel version.
// GetPluginDependencies 返回插件的生效依赖（使用默认 SeaTunnel 版本）。
func (s *Service) GetPluginDependencies(ctx context.Context, pluginName string) ([]PluginDependency, error) {
	return s.GetPluginDependenciesForVersion(ctx, pluginName, seatunnel.DefaultVersion())
}

// GetPluginDependenciesForVersion returns effective dependencies for a plugin using the given version.
// GetPluginDependenciesForVersion 返回指定版本下插件的生效依赖。
func (s *Service) GetPluginDependenciesForVersion(ctx context.Context, pluginName, version string) ([]PluginDependency, error) {
	return s.GetPluginDependenciesForVersionAndProfiles(ctx, pluginName, version, nil)
}

// GetPluginDependenciesForVersionAndProfiles returns effective dependencies for a plugin using the given version and selected profiles.
// GetPluginDependenciesForVersionAndProfiles 返回指定版本与画像下插件的生效依赖。
func (s *Service) GetPluginDependenciesForVersionAndProfiles(ctx context.Context, pluginName, version string, profileKeys []string) ([]PluginDependency, error) {
	if strings.TrimSpace(version) == "" {
		version = seatunnel.DefaultVersion()
	}
	return s.GetEffectiveDependencies(ctx, pluginName, version, normalizeProfileKeys(profileKeys))
}

// ==================== PluginTransferer Interface Implementation 插件传输器接口实现 ====================

// TransferPluginToAgent transfers a plugin to an agent during SeaTunnel installation.
// TransferPluginToAgent 在 SeaTunnel 安装过程中将插件传输到 Agent。
// This implements the installer.PluginTransferer interface.
// 这实现了 installer.PluginTransferer 接口。
func (s *Service) TransferPluginToAgent(ctx context.Context, agentID, pluginName, version, installDir string, profileKeys []string) error {
	// Get artifact ID for the plugin / 获取插件的 artifact ID
	artifactID := s.GetPluginArtifactID(pluginName)
	deps, err := s.GetPluginDependenciesForVersionAndProfiles(ctx, pluginName, version, profileKeys)
	if err != nil {
		deps = nil
	}

	// Use the existing transferPluginToAgent method / 使用现有的 transferPluginToAgent 方法
	return s.transferPluginToAgent(ctx, agentID, artifactID, pluginName, version, installDir, deps)
}

// GetPluginArtifactID returns the Maven artifact ID for a plugin name.
// GetPluginArtifactID 返回插件名称对应的 Maven artifact ID。
// This implements the installer.PluginTransferer interface.
// 这实现了 installer.PluginTransferer 接口。
func (s *Service) GetPluginArtifactID(pluginName string) string {
	return getArtifactID(pluginName)
}

// DownloadPluginSync downloads a plugin synchronously (blocking).
// DownloadPluginSync 同步下载插件（阻塞）。
// This implements the installer.PluginTransferer interface.
// 这实现了 installer.PluginTransferer 接口。
func (s *Service) DownloadPluginSync(ctx context.Context, pluginName, version, mirror string, profileKeys []string) error {
	// Get plugin info / 获取插件信息
	plugin, err := s.GetPluginInfo(ctx, pluginName, version)
	if err != nil {
		// Create a minimal plugin struct if not found / 如果未找到则创建最小插件结构
		plugin = &Plugin{
			Name:       pluginName,
			ArtifactID: getArtifactID(pluginName),
			Version:    version,
			GroupID:    "org.apache.seatunnel",
		}
	}

	selectedProfiles := normalizeProfileKeys(profileKeys)

	// Load effective dependencies using selected profiles / 使用选中的画像加载生效依赖
	deps, err := s.GetPluginDependenciesForVersionAndProfiles(ctx, pluginName, version, selectedProfiles)
	if err == nil {
		plugin.Dependencies = deps
		fmt.Printf("[DownloadPluginSync] Loaded %d dependencies for %s (profiles=%v)\n", len(deps), pluginName, selectedProfiles)
	}

	downloadMirror := MirrorSourceApache
	switch MirrorSource(strings.TrimSpace(mirror)) {
	case MirrorSourceAliyun:
		downloadMirror = MirrorSourceAliyun
	case MirrorSourceHuaweiCloud:
		downloadMirror = MirrorSourceHuaweiCloud
	case MirrorSourceApache:
		downloadMirror = MirrorSourceApache
	}

	connectorReady := s.downloader.IsConnectorDownloaded(pluginName, version)
	dependenciesReady := s.arePluginDependenciesDownloaded(plugin)

	// DownloadPlugin downloads both connector and dependencies / DownloadPlugin 同时下载连接器和依赖
	return s.downloader.DownloadPlugin(ctx, plugin, downloadMirror, selectedProfiles, connectorReady, dependenciesReady, nil)
}

// GetPluginPreparationFingerprint returns a stable fingerprint for the plugin's effective dependency set.
// GetPluginPreparationFingerprint 返回插件当前生效依赖集合的稳定指纹。
// This implements the installer.PluginTransferer interface.
// 这实现了 installer.PluginTransferer 接口。
func (s *Service) GetPluginPreparationFingerprint(ctx context.Context, pluginName, version string, profileKeys []string) (string, error) {
	deps, err := s.GetPluginDependenciesForVersionAndProfiles(ctx, pluginName, version, profileKeys)
	if err != nil {
		return "", err
	}
	return buildPluginPreparationFingerprint(deps)
}

func buildPluginPreparationFingerprint(deps []PluginDependency) (string, error) {
	type dependencyFingerprintEntry struct {
		GroupID          string                 `json:"group_id"`
		ArtifactID       string                 `json:"artifact_id"`
		Version          string                 `json:"version"`
		TargetDir        string                 `json:"target_dir"`
		SourceType       PluginDependencySource `json:"source_type,omitempty"`
		OriginalFileName string                 `json:"original_file_name,omitempty"`
		StoredPath       string                 `json:"stored_path,omitempty"`
	}

	entries := make([]dependencyFingerprintEntry, 0, len(deps))
	for _, dep := range deps {
		entries = append(entries, dependencyFingerprintEntry{
			GroupID:          dep.GroupID,
			ArtifactID:       dep.ArtifactID,
			Version:          dep.Version,
			TargetDir:        dep.TargetDir,
			SourceType:       dep.SourceType,
			OriginalFileName: dep.OriginalFileName,
			StoredPath:       dep.StoredPath,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		left := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", entries[i].GroupID, entries[i].ArtifactID, entries[i].Version, entries[i].TargetDir, entries[i].SourceType, entries[i].OriginalFileName, entries[i].StoredPath)
		right := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s", entries[j].GroupID, entries[j].ArtifactID, entries[j].Version, entries[j].TargetDir, entries[j].SourceType, entries[j].OriginalFileName, entries[j].StoredPath)
		return left < right
	})

	payload, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// RecordInstalledPlugin records a plugin as installed for a cluster.
// RecordInstalledPlugin 记录插件已安装到集群。
// Sets category (e.g. connector) and artifact_id so one-click install shows correct classification.
// 设置分类（如 connector）和 artifact_id，以便一键安装后显示正确分类。
// This implements the installer.PluginTransferer interface.
// 这实现了 installer.PluginTransferer 接口。
func (s *Service) RecordInstalledPlugin(ctx context.Context, clusterID uint, pluginName, version string) error {
	// Resolve category and artifact_id (for display and DB not-null) / 解析分类与 artifact_id（用于展示及 DB 非空）
	category := PluginCategoryConnector
	artifactID := getArtifactID(pluginName)
	if info, err := s.GetPluginInfo(ctx, pluginName, version); err == nil && info != nil && info.Category != "" {
		category = info.Category
		if info.ArtifactID != "" {
			artifactID = info.ArtifactID
		}
	}

	// Check if already recorded / 检查是否已记录
	existing, err := s.repo.GetByClusterAndName(ctx, clusterID, pluginName)
	if err == nil && existing != nil {
		// Already exists, update version/category/artifact_id if different / 已存在则更新版本/分类/artifact_id
		updated := false
		if existing.Version != version {
			existing.Version = version
			updated = true
		}
		if existing.Category != category {
			existing.Category = category
			updated = true
		}
		if existing.ArtifactID != artifactID {
			existing.ArtifactID = artifactID
			updated = true
		}
		if updated {
			return s.repo.Update(ctx, existing)
		}
		return nil
	}

	// Create new record / 创建新记录
	installed := &InstalledPlugin{
		ClusterID:   clusterID,
		PluginName:  pluginName,
		ArtifactID:  artifactID,
		Category:    category,
		Version:     version,
		Status:      PluginStatusInstalled,
		InstalledAt: time.Now(),
	}

	return s.repo.Create(ctx, installed)
}
