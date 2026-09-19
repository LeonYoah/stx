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

package installer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/google/uuid"
)

// Common errors / 常见错误
var (
	ErrPackageNotFound        = errors.New("package not found / 安装包未找到")
	ErrPackageAlreadyExists   = errors.New("package already exists / 安装包已存在")
	ErrInvalidPackageVersion  = errors.New("invalid package version / 安装包版本不合法")
	ErrInvalidPackageFile     = errors.New("invalid package file / 安装包文件不合法")
	ErrInvalidPackagePath     = errors.New("invalid package path / 安装包路径不合法")
	ErrPackageTooLarge        = errors.New("package too large / 安装包过大")
	ErrInvalidUploadID        = errors.New("invalid upload id / 上传会话 ID 不合法")
	ErrInvalidChunkIndex      = errors.New("invalid chunk index / 分片索引不合法")
	ErrChunkOutOfOrder        = errors.New("chunk out of order / 分片顺序错误")
	ErrInstallationNotFound   = errors.New("installation not found / 安装任务未找到")
	ErrInstallationInProgress = errors.New("installation already in progress / 安装任务正在进行中")
	ErrHostNotConnected       = errors.New("host agent not connected / 主机 Agent 未连接")
	ErrAgentNotFound          = errors.New("agent not found / Agent 未找到")
)

var packageVersionRegexp = regexp.MustCompile(`^[0-9A-Za-z._+-]{1,64}$`)
var uploadIDRegexp = regexp.MustCompile(`^[0-9A-Za-z_-]{8,128}$`)

// AgentManager is the interface for communicating with agents
// AgentManager 是与 Agent 通信的接口
type AgentManager interface {
	// GetAgentByHostID returns the agent connection for a host
	// GetAgentByHostID 返回主机的 Agent 连接
	GetAgentByHostID(hostID uint) (agentID string, connected bool)

	// SendInstallCommand sends an installation command to an agent
	// SendInstallCommand 向 Agent 发送安装命令
	SendInstallCommand(ctx context.Context, agentID string, params map[string]string) (commandID string, err error)

	// GetCommandStatus returns the status of a command
	// GetCommandStatus 返回命令的状态
	GetCommandStatus(commandID string) (status string, progress int, message string, err error)

	// SendCommand sends a command to an agent and returns the result
	// SendCommand 向 Agent 发送命令并返回结果
	SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (success bool, output string, err error)

	// SendTransferPackageCommand sends a package transfer chunk to an agent
	// SendTransferPackageCommand 向 Agent 发送安装包传输块
	SendTransferPackageCommand(ctx context.Context, agentID string, version string, fileName string, chunk []byte, offset int64, totalSize int64, isLast bool, checksum string) (success bool, receivedBytes int64, localPath string, err error)
}

// PluginTransferer is the interface for transferring plugins to agents
// PluginTransferer 是向 Agent 传输插件的接口
type PluginTransferer interface {
	// TransferPluginToAgent transfers a plugin to an agent
	// TransferPluginToAgent 将插件传输到 Agent
	TransferPluginToAgent(ctx context.Context, agentID, pluginName, version, installDir string, profileKeys []string) error

	// GetPluginArtifactID returns the Maven artifact ID for a plugin name
	// GetPluginArtifactID 返回插件名称对应的 Maven artifact ID
	GetPluginArtifactID(pluginName string) string

	// IsPluginDownloaded checks if a plugin is downloaded locally
	// IsPluginDownloaded 检查插件是否已下载到本地
	IsPluginDownloaded(name, version string) bool

	// DownloadPluginSync downloads a plugin synchronously (blocking)
	// DownloadPluginSync 同步下载插件（阻塞）
	DownloadPluginSync(ctx context.Context, pluginName, version, mirror string, profileKeys []string) error

	// GetPluginPreparationFingerprint returns a stable fingerprint for the plugin's effective dependency set.
	// GetPluginPreparationFingerprint 返回插件当前生效依赖集合的稳定指纹。
	GetPluginPreparationFingerprint(ctx context.Context, pluginName, version string, profileKeys []string) (string, error)

	// RecordInstalledPlugin records a plugin as installed for a cluster
	// RecordInstalledPlugin 记录插件已安装到集群
	RecordInstalledPlugin(ctx context.Context, clusterID uint, pluginName, version string) error
}

// HostProvider is the interface for getting host information
// HostProvider 是获取主机信息的接口
type HostProvider interface {
	// GetHostByID returns host information by ID
	// GetHostByID 根据 ID 返回主机信息
	GetHostByID(ctx context.Context, hostID uint) (*HostInfo, error)
}

// NodeStatusUpdater is the interface for updating cluster node status
// NodeStatusUpdater 是更新集群节点状态的接口
type NodeStatusUpdater interface {
	// UpdateNodeStatusByClusterAndHost updates the node status by cluster ID and host ID
	// UpdateNodeStatusByClusterAndHost 根据集群 ID 和主机 ID 更新节点状态
	UpdateNodeStatusByClusterAndHost(ctx context.Context, clusterID uint, hostID uint, status string) error
}

// NodeStarter is the interface for starting cluster nodes
// NodeStarter 是启动集群节点的接口
type NodeStarter interface {
	// StartNodeByClusterAndHost starts a node by cluster ID and host ID
	// StartNodeByClusterAndHost 根据集群 ID 和主机 ID 启动节点
	StartNodeByClusterAndHost(ctx context.Context, clusterID uint, hostID uint) (bool, string, error)
	// StartNodeByClusterAndHostAndRole starts a node by cluster ID, host ID and role (for separated mode: same host can have master + worker)
	// StartNodeByClusterAndHostAndRole 根据集群 ID、主机 ID 和角色启动节点（分离模式下同一主机可有 master+worker）
	StartNodeByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (bool, string, error)
}

// NodeJVMResolver resolves cluster/node scoped JVM config for installation.
// NodeJVMResolver 解析安装时需要的集群/节点级 JVM 配置。
type NodeJVMResolver interface {
	// ResolveNodeJVMByClusterAndHostAndRole resolves effective JVM config for one logical node.
	// ResolveNodeJVMByClusterAndHostAndRole 解析一个逻辑节点的生效 JVM 配置。
	ResolveNodeJVMByClusterAndHostAndRole(ctx context.Context, clusterID uint, hostID uint, role string) (*JVMConfig, error)
}

// ConfigInitializer is the interface for initializing cluster configs after installation
// ConfigInitializer 是安装完成后初始化集群配置的接口
type ConfigInitializer interface {
	// InitClusterConfigs initializes cluster configs by pulling from a host
	// InitClusterConfigs 通过从主机拉取来初始化集群配置
	InitClusterConfigs(ctx context.Context, clusterID uint, hostID uint, installDir string, userID uint) error
}

// HostInfo contains host information for precheck
// HostInfo 包含预检查所需的主机信息
type HostInfo struct {
	ID          uint       `json:"id"`
	Name        string     `json:"name,omitempty"`
	AgentID     string     `json:"agent_id"`
	AgentStatus string     `json:"agent_status"`
	LastSeen    *time.Time `json:"last_seen"`
}

// IsOnline checks if the host agent is online within the timeout
// IsOnline 检查主机 Agent 是否在超时时间内在线
func (h *HostInfo) IsOnline(timeout time.Duration) bool {
	if h.LastSeen == nil {
		return false
	}
	return time.Since(*h.LastSeen) < timeout
}

// MirrorURLs maps mirror sources to their base URLs
// MirrorURLs 将镜像源映射到其基础 URL
var MirrorURLs = map[MirrorSource]string{
	MirrorAliyun:      "https://mirrors.aliyun.com/apache/seatunnel",
	MirrorApache:      "https://archive.apache.org/dist/seatunnel",
	MirrorHuaweiCloud: "https://mirrors.huaweicloud.com/apache/seatunnel",
}

// ApacheArchiveURL is the URL to fetch version list from Apache Archive
// ApacheArchiveURL 是从 Apache Archive 获取版本列表的 URL
const ApacheArchiveURL = seatunnel.ArchiveVersionsURL

// VersionCacheDuration is how long to cache the version list
// VersionCacheDuration 是版本列表的缓存时间
const VersionCacheDuration = 1 * time.Hour

// Service provides installation management functionality.
// Service 提供安装管理功能。
type Service struct {
	// packageDir is the directory for storing local packages
	// packageDir 是存储本地安装包的目录
	packageDir string

	// tempDir is the directory for temporary files (downloads in progress)
	// tempDir 是临时文件目录（下载中的文件）
	tempDir string

	// installations tracks ongoing installations by host ID
	// installations 按主机 ID 跟踪正在进行的安装
	installations map[string]*InstallationStatus
	installMu     sync.RWMutex

	// downloads tracks ongoing download tasks by version
	// downloads 按版本跟踪正在进行的下载任务
	downloads   map[string]*DownloadTask
	downloadsMu sync.RWMutex

	// cachedVersions stores the cached version list from Apache Archive
	// cachedVersions 存储从 Apache Archive 获取的缓存版本列表
	cachedVersions []string
	// versionsCacheTime is when the version cache was last updated
	// versionsCacheTime 是版本缓存最后更新的时间
	versionsCacheTime time.Time
	// versionsMu protects version cache access
	// versionsMu 保护版本缓存访问
	versionsMu sync.RWMutex

	// agentManager is used to communicate with agents
	// agentManager 用于与 Agent 通信
	agentManager AgentManager

	// hostProvider is used to get host information
	// hostProvider 用于获取主机信息
	hostProvider HostProvider

	// pluginTransferer is used to transfer plugins to agents
	// pluginTransferer 用于向 Agent 传输插件
	pluginTransferer PluginTransferer

	// nodeStatusUpdater is used to update cluster node status
	// nodeStatusUpdater 用于更新集群节点状态
	nodeStatusUpdater NodeStatusUpdater

	// nodeStarter is used to start cluster nodes
	// nodeStarter 用于启动集群节点
	nodeStarter NodeStarter

	// nodeJVMResolver is used to resolve node-level JVM overrides during installation
	// nodeJVMResolver 用于在安装时解析节点级 JVM 覆盖
	nodeJVMResolver NodeJVMResolver

	// configInitializer is used to initialize cluster configs after installation
	// configInitializer 用于安装完成后初始化集群配置
	configInitializer ConfigInitializer

	// heartbeatTimeout is the timeout for agent heartbeat
	// heartbeatTimeout 是 Agent 心跳超时时间
	heartbeatTimeout time.Duration

	// chunkUploadMu protects chunk upload state files
	// chunkUploadMu 保护分片上传状态文件
	chunkUploadMu sync.Mutex

	// preparedAssetMu protects prepared package/plugin caches for Agent reuse.
	// preparedAssetMu 保护用于复用 Agent 已准备安装包/插件的缓存。
	preparedAssetMu sync.Mutex
	// preparedPackages stores remote package paths already transferred to an Agent.
	// preparedPackages 保存已传输到 Agent 的安装包远程路径。
	preparedPackages map[string]preparedPackageCacheEntry
	// preparedPlugins stores plugin bundles already transferred and installed on an Agent.
	// preparedPlugins 保存已传输并安装到 Agent 的插件包标记。
	preparedPlugins map[string]time.Time

	// executionService stores shared execution state for package operations.
	// executionService 保存安装包操作的公共执行状态。
	executionService *executionapp.Service
}

type preparedPackageCacheEntry struct {
	RemotePath string
	UpdatedAt  time.Time
}

const preparedAssetCacheTTL = 30 * time.Minute

// PackageChunkUploadRequest is the request for uploading a package chunk.
// PackageChunkUploadRequest 表示安装包分片上传请求。
type PackageChunkUploadRequest struct {
	Version     string
	UploadID    string
	ChunkIndex  int
	TotalChunks int
	TotalSize   int64
	FileName    string
}

// PackageChunkUploadResult is the result of package chunk upload.
// PackageChunkUploadResult 表示安装包分片上传结果。
type PackageChunkUploadResult struct {
	UploadID       string       `json:"upload_id"`
	Completed      bool         `json:"completed"`
	ReceivedChunks int          `json:"received_chunks"`
	TotalChunks    int          `json:"total_chunks"`
	Package        *PackageInfo `json:"package,omitempty"`
}

type packageChunkUploadState struct {
	Version       string    `json:"version"`
	FileName      string    `json:"file_name"`
	TotalChunks   int       `json:"total_chunks"`
	TotalSize     int64     `json:"total_size"`
	NextChunk     int       `json:"next_chunk"`
	ReceivedBytes int64     `json:"received_bytes"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// NewService creates a new Service instance.
// NewService 创建一个新的 Service 实例。
// If packageDir is empty, it uses the configured packages directory.
// 如果 packageDir 为空，则使用配置的安装包目录。
func NewService(packageDir string, agentManager AgentManager) *Service {
	// Use configured directory if not specified / 如果未指定则使用配置的目录
	if packageDir == "" {
		packageDir = config.GetPackagesDir()
	}

	// Create package directory if not exists / 如果不存在则创建安装包目录
	if err := os.MkdirAll(packageDir, 0755); err != nil {
		// Log error but continue / 记录错误但继续
	}

	// Also create temp directory / 同时创建临时目录
	if err := os.MkdirAll(config.GetTempDir(), 0755); err != nil {
		// Log error but continue / 记录错误但继续
	}

	return &Service{
		packageDir:       packageDir,
		tempDir:          config.GetTempDir(),
		installations:    make(map[string]*InstallationStatus),
		downloads:        make(map[string]*DownloadTask),
		agentManager:     agentManager,
		heartbeatTimeout: 2 * time.Minute, // Default 2 minutes / 默认 2 分钟
		preparedPackages: make(map[string]preparedPackageCacheEntry),
		preparedPlugins:  make(map[string]time.Time),
	}
}

// NewServiceWithDefaults creates a new Service instance with default configuration.
// NewServiceWithDefaults 使用默认配置创建新的 Service 实例。
func NewServiceWithDefaults() *Service {
	return NewService("", nil)
}

// SetHostProvider sets the host provider for precheck operations.
// SetHostProvider 设置用于预检查操作的主机提供者。
func (s *Service) SetHostProvider(provider HostProvider) {
	s.hostProvider = provider
}

// SetAgentManager sets the agent manager for sending commands to agents.
// SetAgentManager 设置用于向 Agent 发送命令的 Agent 管理器。
func (s *Service) SetAgentManager(manager AgentManager) {
	s.agentManager = manager
}

// SetPluginTransferer sets the plugin transferer for transferring plugins to agents.
// SetPluginTransferer 设置用于向 Agent 传输插件的插件传输器。
func (s *Service) SetPluginTransferer(transferer PluginTransferer) {
	s.pluginTransferer = transferer
}

// SetNodeStatusUpdater sets the node status updater for updating cluster node status.
// SetNodeStatusUpdater 设置用于更新集群节点状态的节点状态更新器。
func (s *Service) SetNodeStatusUpdater(updater NodeStatusUpdater) {
	s.nodeStatusUpdater = updater
}

// SetNodeStarter sets the node starter for starting cluster nodes.
// SetNodeStarter 设置用于启动集群节点的节点启动器。
func (s *Service) SetNodeStarter(starter NodeStarter) {
	s.nodeStarter = starter
}

// SetNodeJVMResolver sets the resolver for node-level JVM config.
// SetNodeJVMResolver 设置节点级 JVM 配置解析器。
func (s *Service) SetNodeJVMResolver(resolver NodeJVMResolver) {
	s.nodeJVMResolver = resolver
}

// SetConfigInitializer sets the config initializer for initializing cluster configs.
// SetConfigInitializer 设置用于初始化集群配置的配置初始化器。
func (s *Service) SetConfigInitializer(initializer ConfigInitializer) {
	s.configInitializer = initializer
}

// SetExecutionService sets the shared execution service used by package operations.
// SetExecutionService 设置安装包操作使用的公共执行服务。
func (s *Service) SetExecutionService(service *executionapp.Service) {
	if s == nil {
		return
	}
	s.executionService = service
}

// RequestCancel stops a package download through the real downloader signal.
// RequestCancel 通过真实下载停止信号取消安装包下载。
func (s *Service) RequestCancel(ctx context.Context, item *executionapp.Execution, actor executionapp.Actor) (executionapp.CancelResult, error) {
	if item == nil || strings.TrimSpace(item.ModuleRef) == "" {
		return executionapp.CancelResult{}, ErrDownloadNotFound
	}
	task, err := s.GetDownloadStatus(ctx, item.ModuleRef)
	if err != nil {
		return executionapp.CancelResult{}, err
	}
	if task.OwnerUserID != 0 && task.OwnerUserID != actor.UserID && !actor.IsAdmin {
		return executionapp.CancelResult{}, executionapp.ErrPermissionDenied
	}
	updated, err := s.CancelDownload(ctx, task.Version)
	if err != nil {
		return executionapp.CancelResult{}, err
	}
	switch updated.Status {
	case DownloadStatusCancelled:
		return executionapp.CancelResult{Status: executionapp.StatusCancelled, Cancellable: false, CancellableReason: "package download stopped"}, nil
	case DownloadStatusCancelling:
		return executionapp.CancelResult{Status: executionapp.StatusCancelling, Cancellable: true, CancellableReason: "package download is still stopping"}, nil
	case DownloadStatusCompleted:
		return executionapp.CancelResult{Status: executionapp.StatusSucceeded, Cancellable: false, CancellableReason: "package download already completed"}, nil
	default:
		return executionapp.CancelResult{Status: executionapp.StatusFailed, Cancellable: false, CancellableReason: "package download is no longer cancellable"}, nil
	}
}

// ==================== Version Management 版本管理 ====================

// getVersions returns the version list, using cache if valid, otherwise fetching from Apache Archive.
// getVersions 返回版本列表，如果缓存有效则使用缓存，否则从 Apache Archive 获取。
func (s *Service) getVersions(ctx context.Context) []string {
	s.versionsMu.RLock()
	// Check if cache is valid / 检查缓存是否有效
	if len(s.cachedVersions) > 0 && time.Since(s.versionsCacheTime) < VersionCacheDuration {
		versions := s.cachedVersions
		s.versionsMu.RUnlock()
		return versions
	}
	s.versionsMu.RUnlock()

	// Try to fetch from Apache Archive / 尝试从 Apache Archive 获取
	versions, err := s.fetchVersionsFromApache(ctx)
	if err != nil {
		// Use fallback versions on error / 出错时使用备用版本
		return seatunnel.FallbackVersions()
	}

	// Update cache / 更新缓存
	s.versionsMu.Lock()
	s.cachedVersions = versions
	s.versionsCacheTime = time.Now()
	s.versionsMu.Unlock()

	return versions
}

// fetchVersionsFromApache fetches the version list from Apache Archive.
// fetchVersionsFromApache 从 Apache Archive 获取版本列表。
func (s *Service) fetchVersionsFromApache(ctx context.Context) ([]string, error) {
	// Create HTTP request with timeout / 创建带超时的 HTTP 请求
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ApacheArchiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body / 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse HTML to extract version directories / 解析 HTML 提取版本目录
	// Apache Archive HTML format: <a href="2.3.12/">2.3.12/</a>
	// Apache Archive HTML 格式: <a href="2.3.12/">2.3.12/</a>
	versionRegex := regexp.MustCompile(`<a href="(\d+\.\d+\.\d+(?:-[a-zA-Z0-9]+)?)/?">\d+\.\d+\.\d+(?:-[a-zA-Z0-9]+)?/?</a>`)
	matches := versionRegex.FindAllStringSubmatch(string(body), -1)

	if len(matches) == 0 {
		return nil, fmt.Errorf("no versions found in response")
	}

	// Extract versions and sort in descending order / 提取版本并按降序排序
	versions := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) >= 2 {
			version := strings.TrimSuffix(match[1], "/")
			versions = append(versions, version)
		}
	}

	// Sort versions in descending order (newest first) / 按降序排序（最新版本在前）
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})

	return versions, nil
}

// RefreshVersions forces a refresh of the version list from Apache Archive.
// RefreshVersions 强制从 Apache Archive 刷新版本列表。
func (s *Service) RefreshVersions(ctx context.Context) ([]string, error) {
	versions, err := s.fetchVersionsFromApache(ctx)
	if err != nil {
		return seatunnel.FallbackVersions(), err
	}

	// Update cache / 更新缓存
	s.versionsMu.Lock()
	s.cachedVersions = versions
	s.versionsCacheTime = time.Now()
	s.versionsMu.Unlock()

	return versions, nil
}

// compareVersions compares two version strings.
// compareVersions 比较两个版本字符串。
// Returns: >0 if v1 > v2, <0 if v1 < v2, 0 if equal
// 返回: >0 如果 v1 > v2, <0 如果 v1 < v2, 0 如果相等
func compareVersions(v1, v2 string) int {
	return seatunnel.CompareVersions(v1, v2)
}

// ==================== Package Management 安装包管理 ====================

// ListAvailableVersions returns available SeaTunnel versions.
// ListAvailableVersions 返回可用的 SeaTunnel 版本。
func (s *Service) ListAvailableVersions(ctx context.Context) (*AvailableVersions, error) {
	// Get versions (from cache, online, or fallback)
	// 获取版本（从缓存、在线或备用列表）
	versions := s.getVersions(ctx)

	recommended := seatunnel.RecommendedVersion()
	if len(versions) > 0 && versions[0] != "" {
		// 默认按抓取到的最新版本作为推荐版本（versions 已按降序排序）
		// By default, use the newest fetched version as the recommended version.
		recommended = versions[0]
	}

	result := &AvailableVersions{
		Versions:            versions,
		RecommendedVersion:  recommended,
		LocalPackages:       make([]PackageInfo, 0),
		VersionCapabilities: make(map[string]seatunnel.VersionCapabilities),
	}

	if recommended != "" {
		result.VersionCapabilities[recommended] = seatunnel.CapabilitiesForVersion(recommended)
	}
	for _, version := range versions {
		if version == "" {
			continue
		}
		result.VersionCapabilities[version] = seatunnel.CapabilitiesForVersion(version)
	}

	// Scan local packages / 扫描本地安装包
	entries, err := os.ReadDir(s.packageDir)
	if err != nil {
		// Directory might not exist, return empty list / 目录可能不存在，返回空列表
		return result, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Check if it's a SeaTunnel package / 检查是否是 SeaTunnel 安装包
		name := entry.Name()
		if !isSeaTunnelPackage(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		version := extractVersionFromFileName(name)
		uploadedAt := info.ModTime()

		result.LocalPackages = append(result.LocalPackages, PackageInfo{
			Version:      version,
			FileName:     name,
			FileSize:     info.Size(),
			IsLocal:      true,
			LocalPath:    filepath.Join(s.packageDir, name),
			UploadedAt:   &uploadedAt,
			DownloadURLs: getDownloadURLs(version),
		})
		if version != "" {
			result.VersionCapabilities[version] = seatunnel.CapabilitiesForVersion(version)
		}
	}

	return result, nil
}

// GetPackageInfo returns information about a specific package version.
// GetPackageInfo 返回特定版本安装包的信息。
func (s *Service) GetPackageInfo(ctx context.Context, version string) (*PackageInfo, error) {
	// Check if local package exists / 检查本地安装包是否存在
	fileName := packageFileName(version)
	localPath := filepath.Join(s.packageDir, fileName)

	info := &PackageInfo{
		Version:      version,
		FileName:     fileName,
		DownloadURLs: getDownloadURLs(version),
	}

	if fileInfo, err := os.Stat(localPath); err == nil {
		info.IsLocal = true
		info.LocalPath = localPath
		info.FileSize = fileInfo.Size()
		uploadedAt := fileInfo.ModTime()
		info.UploadedAt = &uploadedAt

		// Calculate checksum / 计算校验和
		checksum, err := calculateChecksum(localPath)
		if err == nil {
			info.Checksum = checksum
		}
	}

	return info, nil
}

// UploadPackage handles package file upload.
// UploadPackage 处理安装包文件上传。
func (s *Service) UploadPackage(ctx context.Context, version string, file *multipart.FileHeader) (*PackageInfo, error) {
	if file == nil {
		return nil, ErrInvalidPackageFile
	}

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	return s.savePackageFromReader(ctx, version, file.Filename, file.Size, src)
}

// UploadPackageChunk handles package chunk upload.
// UploadPackageChunk 处理安装包分片上传。
func (s *Service) UploadPackageChunk(ctx context.Context, req *PackageChunkUploadRequest, file *multipart.FileHeader) (*PackageChunkUploadResult, error) {
	if req == nil || file == nil {
		return nil, ErrInvalidPackageFile
	}

	req.Version = strings.TrimSpace(req.Version)
	req.UploadID = strings.TrimSpace(req.UploadID)
	req.FileName = strings.TrimSpace(req.FileName)
	if req.FileName == "" {
		req.FileName = file.Filename
	}

	if !uploadIDRegexp.MatchString(req.UploadID) {
		return nil, ErrInvalidUploadID
	}
	if req.ChunkIndex < 0 || req.TotalChunks <= 0 || req.ChunkIndex >= req.TotalChunks {
		return nil, ErrInvalidChunkIndex
	}
	if err := validatePackageUploadInput(req.Version, req.FileName, req.TotalSize); err != nil {
		return nil, err
	}

	s.chunkUploadMu.Lock()
	defer s.chunkUploadMu.Unlock()

	uploadDir, err := s.getChunkUploadDir(req.UploadID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create chunk upload dir: %w", err)
	}

	statePath := filepath.Join(uploadDir, "state.json")
	chunkFilePath := filepath.Join(uploadDir, "chunk.tmp")

	state, stateExists, err := loadChunkUploadState(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chunk upload state: %w", err)
	}

	if !stateExists {
		if req.ChunkIndex != 0 {
			return nil, ErrChunkOutOfOrder
		}
		state = &packageChunkUploadState{
			Version:       req.Version,
			FileName:      req.FileName,
			TotalChunks:   req.TotalChunks,
			TotalSize:     req.TotalSize,
			NextChunk:     0,
			ReceivedBytes: 0,
			UpdatedAt:     time.Now(),
		}
	} else {
		if state.Version != req.Version || state.FileName != req.FileName ||
			state.TotalChunks != req.TotalChunks || state.TotalSize != req.TotalSize {
			return nil, fmt.Errorf("%w: metadata mismatch", ErrInvalidChunkIndex)
		}
		if req.ChunkIndex < state.NextChunk {
			return &PackageChunkUploadResult{
				UploadID:       req.UploadID,
				Completed:      false,
				ReceivedChunks: state.NextChunk,
				TotalChunks:    state.TotalChunks,
			}, nil
		}
		if req.ChunkIndex > state.NextChunk {
			return nil, ErrChunkOutOfOrder
		}
	}

	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded chunk: %w", err)
	}
	defer src.Close()

	dst, err := os.OpenFile(chunkFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open chunk temp file: %w", err)
	}
	written, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("failed to append chunk: %w", copyErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("failed to close chunk temp file: %w", closeErr)
	}

	state.NextChunk++
	state.ReceivedBytes += written
	state.UpdatedAt = time.Now()

	result := &PackageChunkUploadResult{
		UploadID:       req.UploadID,
		Completed:      false,
		ReceivedChunks: state.NextChunk,
		TotalChunks:    state.TotalChunks,
	}

	if state.NextChunk < state.TotalChunks {
		if err := saveChunkUploadState(statePath, state); err != nil {
			return nil, fmt.Errorf("failed to persist chunk upload state: %w", err)
		}
		return result, nil
	}

	// Last chunk: finalize package
	// 最后一个分片：合并并落盘
	if state.ReceivedBytes != state.TotalSize {
		_ = os.RemoveAll(uploadDir)
		return nil, fmt.Errorf("%w: size mismatch", ErrInvalidPackageFile)
	}

	merged, err := os.Open(chunkFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open merged chunk file: %w", err)
	}
	defer merged.Close()

	info, err := s.savePackageFromReader(ctx, state.Version, state.FileName, state.TotalSize, merged)
	if err != nil {
		return nil, err
	}

	result.Completed = true
	result.Package = info
	_ = os.RemoveAll(uploadDir)
	return result, nil
}

func validatePackageUploadInput(version, fileName string, fileSize int64) error {
	version = strings.TrimSpace(version)
	if !packageVersionRegexp.MatchString(version) {
		return ErrInvalidPackageVersion
	}
	if fileSize <= 0 {
		return ErrInvalidPackageFile
	}
	maxPackageSize := config.GetMaxPackageSize()
	if maxPackageSize > 0 && fileSize > maxPackageSize {
		return ErrPackageTooLarge
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(fileName)), ".tar.gz") {
		return ErrInvalidPackageFile
	}
	return nil
}

func (s *Service) savePackageFromReader(ctx context.Context, version, fileName string, fileSize int64, src io.Reader) (*PackageInfo, error) {
	version = strings.TrimSpace(version)
	fileName = strings.TrimSpace(fileName)
	if err := validatePackageUploadInput(version, fileName, fileSize); err != nil {
		return nil, err
	}

	finalFileName := packageFileName(version)
	destPath, err := normalizePathInDir(s.packageDir, filepath.Join(s.packageDir, finalFileName))
	if err != nil {
		return nil, ErrInvalidPackagePath
	}
	if _, err := os.Stat(destPath); err == nil {
		return nil, ErrPackageAlreadyExists
	}

	tempFile, err := os.CreateTemp(s.tempDir, "package-upload-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp package file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	written, err := io.Copy(tempFile, src)
	if err != nil {
		return nil, fmt.Errorf("failed to write package file: %w", err)
	}
	if written != fileSize {
		return nil, fmt.Errorf("%w: expected=%d actual=%d", ErrInvalidPackageFile, fileSize, written)
	}
	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close package file: %w", err)
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		return nil, fmt.Errorf("failed to move package file: %w", err)
	}

	fileInfo, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get package file info: %w", err)
	}

	checksum, _ := calculateChecksum(destPath)
	uploadedAt := fileInfo.ModTime()
	logger.InfoF(ctx, "[Installer] package saved: version=%s size=%d path=%s", version, fileInfo.Size(), destPath)
	return &PackageInfo{
		Version:      version,
		FileName:     finalFileName,
		FileSize:     fileInfo.Size(),
		Checksum:     checksum,
		IsLocal:      true,
		LocalPath:    destPath,
		UploadedAt:   &uploadedAt,
		DownloadURLs: getDownloadURLs(version),
	}, nil
}

func (s *Service) getChunkUploadDir(uploadID string) (string, error) {
	baseDir := filepath.Join(s.tempDir, "package-uploads")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", err
	}
	return normalizePathInDir(baseDir, filepath.Join(baseDir, uploadID))
}

func loadChunkUploadState(path string) (*packageChunkUploadState, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var state packageChunkUploadState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, false, err
	}
	return &state, true, nil
}

func saveChunkUploadState(path string, state *packageChunkUploadState) error {
	if state == nil {
		return ErrInvalidPackageFile
	}
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// DeletePackage deletes a local package.
// DeletePackage 删除本地安装包。
func (s *Service) DeletePackage(ctx context.Context, version string) error {
	fileName := packageFileName(version)
	localPath := filepath.Join(s.packageDir, fileName)

	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		return ErrPackageNotFound
	}

	return os.Remove(localPath)
}

// ==================== Package Download 安装包下载 ====================

// ErrDownloadInProgress indicates a download is already in progress for this version
// ErrDownloadInProgress 表示该版本的下载已在进行中
var ErrDownloadInProgress = errors.New("download already in progress / 下载已在进行中")

// ErrDownloadNotFound indicates the download task was not found
// ErrDownloadNotFound 表示下载任务未找到
var ErrDownloadNotFound = errors.New("download task not found / 下载任务未找到")

// StartDownload starts downloading a package from mirror to local storage.
// StartDownload 开始从镜像源下载安装包到本地存储。
func (s *Service) StartDownload(ctx context.Context, req *DownloadRequest) (*DownloadTask, error) {
	return s.startDownload(ctx, req, "", 0)
}

// startDownload creates a download task with optional shared execution ownership before the worker starts.
// startDownload 在工作协程启动前创建带有可选公共执行归属的下载任务。
func (s *Service) startDownload(ctx context.Context, req *DownloadRequest, executionID string, ownerUserID uint64) (*DownloadTask, error) {
	version := strings.TrimSpace(req.Version)
	if !packageVersionRegexp.MatchString(version) {
		return nil, ErrInvalidPackageVersion
	}
	req.Version = version

	s.downloadsMu.Lock()

	// Check if download is already in progress / 检查是否已有下载正在进行
	if existing, ok := s.downloads[req.Version]; ok {
		if existing.Status == DownloadStatusDownloading || existing.Status == DownloadStatusPending {
			result := cloneDownloadTask(existing)
			s.downloadsMu.Unlock()
			return result, ErrDownloadInProgress
		}
	}

	// Use default mirror if not specified / 如果未指定则使用默认镜像源
	mirror := req.Mirror
	if mirror == "" {
		mirror = MirrorAliyun
	}

	// Get download URL / 获取下载 URL
	downloadURL := fmt.Sprintf("%s/%s/apache-seatunnel-%s-bin.tar.gz",
		MirrorURLs[mirror], req.Version, req.Version)

	// Create download task / 创建下载任务
	task := &DownloadTask{
		ID:          uuid.New().String(),
		Version:     req.Version,
		Mirror:      mirror,
		DownloadURL: downloadURL,
		Status:      DownloadStatusPending,
		Progress:    0,
		Message:     "准备下载 / Preparing download",
		StartTime:   time.Now(),
		ExecutionID: strings.TrimSpace(executionID),
		OwnerUserID: ownerUserID,
	}
	// Keep cancellation tied to the HTTP request so cancelling a task closes the network body.
	// 将取消信号绑定到 HTTP 请求，确保取消任务时同时关闭网络请求。
	downloadCtx, cancel := context.WithCancel(context.Background())
	task.cancel = cancel
	task.done = make(chan struct{})

	s.downloads[req.Version] = task
	result := cloneDownloadTask(task)
	s.downloadsMu.Unlock()

	// Start download in background / 在后台开始下载
	go s.runDownload(downloadCtx, task)

	return result, nil
}

// GetDownloadStatus returns the current download status for a version.
// GetDownloadStatus 返回某版本的当前下载状态。
func (s *Service) GetDownloadStatus(ctx context.Context, version string) (*DownloadTask, error) {
	s.downloadsMu.RLock()
	defer s.downloadsMu.RUnlock()

	task, ok := s.downloads[version]
	if !ok {
		return nil, ErrDownloadNotFound
	}

	return cloneDownloadTask(task), nil
}

// CancelDownload cancels an ongoing download.
// CancelDownload 取消正在进行的下载。
func (s *Service) CancelDownload(ctx context.Context, version string) (*DownloadTask, error) {
	version = strings.TrimSpace(version)
	if !packageVersionRegexp.MatchString(version) {
		return nil, ErrInvalidPackageVersion
	}

	s.downloadsMu.Lock()

	task, ok := s.downloads[version]
	if !ok {
		s.downloadsMu.Unlock()
		return nil, ErrDownloadNotFound
	}

	if task.Status != DownloadStatusDownloading && task.Status != DownloadStatusPending && task.Status != DownloadStatusCancelling {
		result := cloneDownloadTask(task)
		s.downloadsMu.Unlock()
		return result, nil // Already completed or failed / 已完成或失败
	}

	// Mark cancellation as in progress; only the downloader may publish cancelled after it stops.
	// 先标记为取消中，只有后台下载确认停止后才能对外报告 cancelled。
	task.Status = DownloadStatusCancelling
	task.Message = "正在停止下载 / Stopping download"
	if task.cancel != nil {
		task.cancel()
	}
	done := task.done
	s.downloadsMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// Return cancelling instead of claiming the download has stopped.
			// 超时后仍返回 cancelling，不能假报已经停止。
		}
	}
	s.downloadsMu.RLock()
	defer s.downloadsMu.RUnlock()
	return cloneDownloadTask(task), nil
}

// ListDownloads returns all download tasks.
// ListDownloads 返回所有下载任务。
func (s *Service) ListDownloads(ctx context.Context) []*DownloadTask {
	s.downloadsMu.RLock()
	defer s.downloadsMu.RUnlock()

	tasks := make([]*DownloadTask, 0, len(s.downloads))
	for _, task := range s.downloads {
		tasks = append(tasks, cloneDownloadTask(task))
	}
	return tasks
}

// cloneDownloadTask returns a response-safe copy without internal cancellation handles.
// cloneDownloadTask 返回不包含内部取消句柄的响应副本。
func cloneDownloadTask(task *DownloadTask) *DownloadTask {
	if task == nil {
		return nil
	}
	result := *task
	result.cancel = nil
	result.done = nil
	return &result
}

// runDownload executes the download process.
// runDownload 执行下载过程。
func (s *Service) runDownload(ctx context.Context, task *DownloadTask) {
	defer func() {
		close(task.done)
		s.finishDownloadExecution(task)
	}()
	logger.InfoF(ctx, "[Installer] 开始下载安装包 / Start downloading package: version=%s, mirror=%s", task.Version, task.Mirror)

	s.downloadsMu.Lock()
	if task.Status == DownloadStatusCancelling {
		now := time.Now()
		task.Status = DownloadStatusCancelled
		task.Message = "下载已取消 / Download cancelled"
		task.EndTime = &now
		s.downloadsMu.Unlock()
		return
	}
	task.Status = DownloadStatusDownloading
	task.Message = "正在下载 / Downloading"
	s.downloadsMu.Unlock()

	fileName := packageFileName(task.Version)
	tempPath, err := normalizePathInDir(s.tempDir, filepath.Join(s.tempDir, fileName+".tmp"))
	if err != nil {
		logger.ErrorF(ctx, "[Installer] 无效临时下载路径 / Invalid temp download path: version=%s, error=%v", task.Version, err)
		s.downloadsMu.Lock()
		now := time.Now()
		task.Status = DownloadStatusFailed
		task.Error = ErrInvalidPackagePath.Error()
		task.EndTime = &now
		s.downloadsMu.Unlock()
		return
	}
	finalPath, err := normalizePathInDir(s.packageDir, filepath.Join(s.packageDir, fileName))
	if err != nil {
		logger.ErrorF(ctx, "[Installer] 无效下载目标路径 / Invalid download destination path: version=%s, error=%v", task.Version, err)
		s.downloadsMu.Lock()
		now := time.Now()
		task.Status = DownloadStatusFailed
		task.Error = ErrInvalidPackagePath.Error()
		task.EndTime = &now
		s.downloadsMu.Unlock()
		return
	}

	// Create a cancellable HTTP request so cancellation closes the active network operation.
	// 创建可取消的 HTTP 请求，确保取消时能停止当前网络操作。
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, task.DownloadURL, nil)
	if err != nil {
		s.markDownloadFailed(task, fmt.Sprintf("创建请求失败 / Failed to create request: %v", err), tempPath)
		return
	}
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.markDownloadCancelled(task, tempPath)
			return
		}
		logger.ErrorF(ctx, "[Installer] 下载请求失败 / Download request failed: version=%s, error=%v", task.Version, err)
		s.markDownloadFailed(task, fmt.Sprintf("请求失败 / Request failed: %v", err), tempPath)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.ErrorF(ctx, "[Installer] 下载 HTTP 错误 / Download HTTP error: version=%s, status=%d", task.Version, resp.StatusCode)
		s.downloadsMu.Lock()
		now := time.Now()
		task.Status = DownloadStatusFailed
		task.Error = fmt.Sprintf("HTTP 错误 / HTTP error: %d", resp.StatusCode)
		task.EndTime = &now
		s.downloadsMu.Unlock()
		return
	}

	// Get total size / 获取总大小
	s.downloadsMu.Lock()
	task.TotalBytes = resp.ContentLength
	s.downloadsMu.Unlock()

	// Create temp file / 创建临时文件
	out, err := os.Create(tempPath)
	if err != nil {
		s.downloadsMu.Lock()
		now := time.Now()
		task.Status = DownloadStatusFailed
		task.Error = fmt.Sprintf("创建文件失败 / Failed to create file: %v", err)
		task.EndTime = &now
		s.downloadsMu.Unlock()
		return
	}
	defer out.Close()

	// Download with progress tracking / 带进度跟踪的下载
	buf := make([]byte, 32*1024) // 32KB buffer
	var downloaded int64
	lastUpdate := time.Now()
	var lastDownloaded int64

	for {
		select {
		case <-ctx.Done():
			s.markDownloadCancelled(task, tempPath)
			return
		default:
		}
		// Check if cancelled / 检查是否已取消
		s.downloadsMu.RLock()
		if task.Status == DownloadStatusCancelling || task.Status == DownloadStatusCancelled {
			s.downloadsMu.RUnlock()
			out.Close()
			os.Remove(tempPath)
			return
		}
		s.downloadsMu.RUnlock()

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				s.downloadsMu.Lock()
				now := time.Now()
				task.Status = DownloadStatusFailed
				task.Error = fmt.Sprintf("写入文件失败 / Failed to write file: %v", writeErr)
				task.EndTime = &now
				s.downloadsMu.Unlock()
				os.Remove(tempPath)
				return
			}
			downloaded += int64(n)

			// Update progress every 500ms / 每 500ms 更新一次进度
			if time.Since(lastUpdate) > 500*time.Millisecond {
				s.downloadsMu.Lock()
				task.DownloadedBytes = downloaded
				if task.TotalBytes > 0 {
					task.Progress = int(downloaded * 100 / task.TotalBytes)
				}
				// Calculate speed / 计算速度
				elapsed := time.Since(lastUpdate).Seconds()
				if elapsed > 0 {
					task.Speed = int64(float64(downloaded-lastDownloaded) / elapsed)
				}
				task.Message = fmt.Sprintf("正在下载 / Downloading: %d%%", task.Progress)
				s.downloadsMu.Unlock()

				lastUpdate = time.Now()
				lastDownloaded = downloaded
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				s.markDownloadCancelled(task, tempPath)
				return
			}
			s.markDownloadFailed(task, fmt.Sprintf("下载失败 / Download failed: %v", err), tempPath)
			return
		}
	}

	// Close file before moving / 移动前关闭文件
	out.Close()

	// Move temp file to final location / 将临时文件移动到最终位置
	s.downloadsMu.RLock()
	cancelling := task.Status == DownloadStatusCancelling || ctx.Err() != nil
	s.downloadsMu.RUnlock()
	if cancelling {
		s.markDownloadCancelled(task, tempPath)
		return
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		s.markDownloadFailed(task, fmt.Sprintf("移动文件失败 / Failed to move file: %v", err), tempPath)
		return
	}

	// Mark as completed / 标记为完成
	s.downloadsMu.Lock()
	if task.Status == DownloadStatusCancelling || ctx.Err() != nil {
		s.downloadsMu.Unlock()
		_ = os.Remove(finalPath)
		s.markDownloadCancelled(task, tempPath)
		return
	}
	now := time.Now()
	task.Status = DownloadStatusCompleted
	task.Progress = 100
	task.DownloadedBytes = downloaded
	task.Message = "下载完成 / Download completed"
	task.EndTime = &now
	s.downloadsMu.Unlock()

	logger.InfoF(ctx, "[Installer] 下载完成 / Download completed: version=%s, size=%d bytes", task.Version, downloaded)
}

// finishDownloadExecution mirrors the actual downloader result to the shared execution record.
// finishDownloadExecution 将真实下载结果同步到公共执行记录。
func (s *Service) finishDownloadExecution(task *DownloadTask) {
	if s.executionService == nil || task == nil || strings.TrimSpace(task.ExecutionID) == "" {
		return
	}
	s.downloadsMu.RLock()
	status := task.Status
	s.downloadsMu.RUnlock()
	var target executionapp.Status
	switch status {
	case DownloadStatusCompleted:
		target = executionapp.StatusSucceeded
	case DownloadStatusCancelled:
		target = executionapp.StatusCancelled
	case DownloadStatusFailed:
		target = executionapp.StatusFailed
	default:
		return
	}
	item, err := s.executionService.Get(context.Background(), executionapp.Actor{UserID: task.OwnerUserID}, task.ExecutionID)
	if err != nil || item == nil || executionapp.IsTerminal(item.Status) {
		return
	}
	_ = s.executionService.Transition(context.Background(), task.ExecutionID, item.Status, target, map[string]any{
		"progress":      task.Progress,
		"result_ref":    task.Version,
		"error_message": task.Error,
		"cancellable":   false,
	})
}

// markDownloadCancelled publishes cancelled only after the worker has stopped and removes partial output.
// markDownloadCancelled 仅在工作协程停止后发布 cancelled，并清理未完成文件。
func (s *Service) markDownloadCancelled(task *DownloadTask, tempPath string) {
	_ = os.Remove(tempPath)
	s.downloadsMu.Lock()
	defer s.downloadsMu.Unlock()
	now := time.Now()
	task.Status = DownloadStatusCancelled
	task.Message = "下载已取消 / Download cancelled"
	task.EndTime = &now
}

// markDownloadFailed records a failure and removes any partial output.
// markDownloadFailed 记录失败并清理未完成文件。
func (s *Service) markDownloadFailed(task *DownloadTask, message, tempPath string) {
	_ = os.Remove(tempPath)
	s.downloadsMu.Lock()
	defer s.downloadsMu.Unlock()
	if task.Status == DownloadStatusCancelled {
		return
	}
	now := time.Now()
	task.Status = DownloadStatusFailed
	task.Error = message
	task.EndTime = &now
}

// ==================== Precheck 预检查 ====================

// DefaultPrecheckPorts is the default list of ports to check for SeaTunnel installation
// DefaultPrecheckPorts 是 SeaTunnel 安装时默认检查的端口列表
var DefaultPrecheckPorts = []int{5801, 5802, 8080, 18080}

// RunPrecheck runs precheck on a host via Agent.
// RunPrecheck 通过 Agent 在主机上运行预检查。
// This is for INSTALLATION precheck - ports should be AVAILABLE (not in use).
// 这是安装预检查 - 端口应该可用（未被占用）。
// This is opposite to PrecheckNode which checks if SeaTunnel is running.
// 这与 PrecheckNode 相反，后者检查 SeaTunnel 是否正在运行。
func (s *Service) RunPrecheck(ctx context.Context, hostID uint, req *PrecheckRequest) (*PrecheckResult, error) {
	logger.InfoF(ctx, "[Installer] 开始预检查 / Start precheck: host=%d", hostID)

	// Initialize result
	// 初始化结果
	result := &PrecheckResult{
		Items:         make([]PrecheckItem, 0),
		OverallStatus: CheckStatusPassed,
	}

	// Check 1: Agent is available
	// 检查 1：Agent 可用
	agentItem := PrecheckItem{
		Name:    "agent",
		Details: make(map[string]interface{}),
	}

	// Get host information
	// 获取主机信息
	if s.hostProvider == nil {
		agentItem.Status = CheckStatusFailed
		agentItem.Message = "Host provider not configured / 主机提供者未配置"
		result.Items = append(result.Items, agentItem)
		result.OverallStatus = CheckStatusFailed
		result.Summary = "Precheck failed: host provider not configured / 预检查失败：主机提供者未配置"
		return result, nil
	}

	hostInfo, err := s.hostProvider.GetHostByID(ctx, hostID)
	if err != nil {
		agentItem.Status = CheckStatusFailed
		agentItem.Message = fmt.Sprintf("Failed to get host info: %v / 获取主机信息失败: %v", err, err)
		result.Items = append(result.Items, agentItem)
		result.OverallStatus = CheckStatusFailed
		result.Summary = "Precheck failed: cannot get host info / 预检查失败：无法获取主机信息"
		return result, nil
	}

	// Check agent status
	// 检查 Agent 状态
	if hostInfo.AgentStatus != "installed" {
		agentItem.Status = CheckStatusFailed
		agentItem.Message = "Agent is not installed / Agent 未安装"
		result.Items = append(result.Items, agentItem)
		result.OverallStatus = CheckStatusFailed
		result.Summary = "Precheck failed: Agent not installed / 预检查失败：Agent 未安装"
		return result, nil
	}

	if !hostInfo.IsOnline(s.heartbeatTimeout) {
		agentItem.Status = CheckStatusFailed
		agentItem.Message = "Agent is offline / Agent 离线"
		result.Items = append(result.Items, agentItem)
		result.OverallStatus = CheckStatusFailed
		result.Summary = "Precheck failed: Agent offline / 预检查失败：Agent 离线"
		return result, nil
	}

	agentItem.Status = CheckStatusPassed
	agentItem.Message = "Agent is installed and online / Agent 已安装且在线"
	result.Items = append(result.Items, agentItem)

	// Check if agentManager is available for sending commands
	// 检查 agentManager 是否可用于发送命令
	if s.agentManager == nil || hostInfo.AgentID == "" {
		// Cannot perform detailed checks, return with agent check only
		// 无法执行详细检查，仅返回 Agent 检查结果
		result.Summary = "Agent is available, but cannot perform detailed checks / Agent 可用，但无法执行详细检查"
		return result, nil
	}

	// Check 2: Ports are available (NOT in use) - opposite to PrecheckNode
	// 检查 2：端口可用（未被占用）- 与 PrecheckNode 相反
	ports := req.Ports
	if len(ports) == 0 {
		ports = DefaultPrecheckPorts
	}

	portsItem := PrecheckItem{
		Name:    "ports",
		Details: make(map[string]interface{}),
	}
	portsItem.Details["ports_to_check"] = ports

	unavailablePorts := make([]int, 0)
	availablePorts := make([]int, 0)

	for _, port := range ports {
		params := map[string]string{
			"port": fmt.Sprintf("%d", port),
		}
		success, _, err := s.agentManager.SendCommand(ctx, hostInfo.AgentID, "check_port", params)
		if err != nil {
			// Error checking port, treat as unavailable
			// 检查端口出错，视为不可用
			unavailablePorts = append(unavailablePorts, port)
		} else if success {
			// Port is listening = port is IN USE = FAILED for installation
			// 端口正在监听 = 端口被占用 = 安装失败
			unavailablePorts = append(unavailablePorts, port)
		} else {
			// Port is not listening = port is AVAILABLE = PASSED for installation
			// 端口未监听 = 端口可用 = 安装通过
			availablePorts = append(availablePorts, port)
		}
	}

	portsItem.Details["available_ports"] = availablePorts
	portsItem.Details["unavailable_ports"] = unavailablePorts

	if len(unavailablePorts) == 0 {
		portsItem.Status = CheckStatusPassed
		portsItem.Message = fmt.Sprintf("All ports are available: %v / 所有端口可用: %v", ports, ports)
	} else {
		portsItem.Status = CheckStatusFailed
		portsItem.Message = fmt.Sprintf("Ports in use: %v / 端口被占用: %v", unavailablePorts, unavailablePorts)
		result.OverallStatus = CheckStatusFailed
	}
	result.Items = append(result.Items, portsItem)

	// Check 3: Directory is writable
	// 检查 3：目录可写
	installDir := req.InstallDir
	if installDir == "" {
		installDir = "/opt/seatunnel"
	}

	dirItem := PrecheckItem{
		Name:    "disk",
		Details: make(map[string]interface{}),
	}
	dirItem.Details["install_dir"] = installDir

	params := map[string]string{
		"path": installDir,
	}
	success, _, err := s.agentManager.SendCommand(ctx, hostInfo.AgentID, "check_directory", params)
	if err != nil {
		dirItem.Status = CheckStatusFailed
		dirItem.Message = fmt.Sprintf("Failed to check directory: %v / 检查目录失败: %v", err, err)
		result.OverallStatus = CheckStatusFailed
	} else if success {
		dirItem.Status = CheckStatusPassed
		dirItem.Message = fmt.Sprintf("Directory %s is writable / 目录 %s 可写", installDir, installDir)
	} else {
		// Directory doesn't exist or not writable - this is OK for installation, we can create it
		// 目录不存在或不可写 - 对于安装来说这是可以的，我们可以创建它
		dirItem.Status = CheckStatusPassed
		dirItem.Message = fmt.Sprintf("Directory %s will be created / 目录 %s 将被创建", installDir, installDir)
	}
	result.Items = append(result.Items, dirItem)

	// Check 4: Java environment
	// 检查 4：Java 环境
	// Supported: Java 8, 11 (passed)
	// Other versions: warning (not blocking)
	// Not installed: failed
	// 支持：Java 8、11（通过）
	// 其他版本：警告（不阻塞）
	// 未安装：失败
	javaItem := PrecheckItem{
		Name:    "java",
		Details: make(map[string]interface{}),
	}
	javaItem.Details["supported_versions"] = []int{8, 11}

	// Check Java version via Agent command
	// 通过 Agent 命令检查 Java 版本
	javaParams := map[string]string{
		"sub_command": "check_java",
	}
	success, output, err := s.agentManager.SendCommand(ctx, hostInfo.AgentID, "check_java", javaParams)
	if err != nil {
		// Cannot check Java, treat as warning
		// 无法检查 Java，视为警告
		javaItem.Status = CheckStatusWarning
		javaItem.Message = fmt.Sprintf("Failed to check Java: %v / 检查 Java 失败: %v", err, err)
	} else if !success {
		// Java not installed
		// Java 未安装
		javaItem.Status = CheckStatusFailed
		javaItem.Message = "Java is not installed. Please install Java 8 or 11. / Java 未安装。请安装 Java 8 或 11。"
		if output != "" {
			javaItem.Details["output"] = output
		}
		result.OverallStatus = CheckStatusFailed
	} else {
		// Java is installed, check version from output
		// Java 已安装，从输出检查版本
		// Output format expected: "java_version=8" or "java_version=11" etc.
		// 预期输出格式："java_version=8" 或 "java_version=11" 等
		javaItem.Details["output"] = output

		// Parse Java version from output
		// 从输出解析 Java 版本
		javaVersion := parseJavaVersionFromOutput(output)
		javaItem.Details["detected_version"] = javaVersion

		if javaVersion == 8 || javaVersion == 11 {
			// Supported version
			// 支持的版本
			javaItem.Status = CheckStatusPassed
			javaItem.Message = fmt.Sprintf("Java %d is installed (supported) / Java %d 已安装（支持）", javaVersion, javaVersion)
		} else if javaVersion > 0 {
			// Other version - warning but not blocking
			// 其他版本 - 警告但不阻塞
			javaItem.Status = CheckStatusWarning
			javaItem.Message = fmt.Sprintf("Java %d is installed. Recommended: Java 8 or 11. / Java %d 已安装。推荐：Java 8 或 11。", javaVersion, javaVersion)
		} else {
			// Cannot determine version
			// 无法确定版本
			javaItem.Status = CheckStatusWarning
			javaItem.Message = "Java is installed but version cannot be determined / Java 已安装但无法确定版本"
		}
	}
	result.Items = append(result.Items, javaItem)

	// Set summary
	// 设置摘要
	passedCount := 0
	failedCount := 0
	warningCount := 0
	for _, item := range result.Items {
		switch item.Status {
		case CheckStatusPassed:
			passedCount++
		case CheckStatusFailed:
			failedCount++
		case CheckStatusWarning:
			warningCount++
		}
	}

	// 有失败优先 failed；无失败但有警告则整体为 warning，避免“全部就绪”掩盖 Java 版本告警。
	// Prefer failed when any item failed; otherwise elevate overall to warning so UI does not show "all ready".
	if failedCount > 0 {
		result.OverallStatus = CheckStatusFailed
	} else if warningCount > 0 {
		result.OverallStatus = CheckStatusWarning
	} else {
		result.OverallStatus = CheckStatusPassed
	}

	switch result.OverallStatus {
	case CheckStatusPassed:
		result.Summary = fmt.Sprintf("All checks passed (%d passed) / 所有检查通过（%d 通过）", passedCount, passedCount)
	case CheckStatusWarning:
		result.Summary = fmt.Sprintf(
			"Checks ready with warnings: %d passed, %d warnings / 预检查可继续但有警告：%d 通过，%d 警告",
			passedCount, warningCount, passedCount, warningCount,
		)
	default:
		result.Summary = fmt.Sprintf(
			"Precheck failed: %d passed, %d failed, %d warnings / 预检查失败：%d 通过，%d 失败，%d 警告",
			passedCount, failedCount, warningCount, passedCount, failedCount, warningCount,
		)
	}

	logger.InfoF(ctx, "[Installer] 预检查完成 / Precheck completed: host=%d, status=%s", hostID, result.OverallStatus)
	return result, nil
}

// ValidateRuntimeStorage validates checkpoint or IMAP storage reachability from selected hosts.
// ValidateRuntimeStorage 校验所选主机到 checkpoint 或 IMAP 存储的可达性。
func (s *Service) ValidateRuntimeStorage(ctx context.Context, req *RuntimeStorageValidationRequest) (*RuntimeStorageValidationResult, error) {
	if req == nil || len(req.HostIDs) == 0 {
		return nil, fmt.Errorf("host_ids is required / 必须提供 host_ids")
	}
	if s.hostProvider == nil || s.agentManager == nil {
		return nil, fmt.Errorf("runtime storage validation is not configured / 运行时存储校验服务未配置")
	}

	result := &RuntimeStorageValidationResult{
		Success: true,
		Kind:    req.Kind,
		Warning: "Validation checks local path readiness or remote endpoint reachability only. Credentials and write permissions still need runtime verification. / 当前校验仅检查本地路径可用性或远程端点可达性，凭证与写入权限仍需运行时验证。",
		Hosts:   make([]*RuntimeStorageValidationHostResult, 0, len(req.HostIDs)),
	}

	for _, hostID := range req.HostIDs {
		hostInfo, err := s.hostProvider.GetHostByID(ctx, hostID)
		hostName := fmt.Sprintf("host-%d", hostID)
		if hostInfo != nil && strings.TrimSpace(hostInfo.Name) != "" {
			hostName = strings.TrimSpace(hostInfo.Name)
		}
		if err != nil {
			result.Success = false
			result.Hosts = append(result.Hosts, &RuntimeStorageValidationHostResult{
				HostID:   hostID,
				HostName: hostName,
				Success:  false,
				Message:  fmt.Sprintf("failed to load host info: %v", err),
			})
			continue
		}
		if !hostInfo.IsOnline(2*time.Minute) || strings.TrimSpace(hostInfo.AgentID) == "" {
			result.Success = false
			result.Hosts = append(result.Hosts, &RuntimeStorageValidationHostResult{
				HostID:   hostID,
				HostName: hostName,
				Success:  false,
				Message:  "host agent is offline",
			})
			continue
		}

		var hostResult *RuntimeStorageValidationHostResult
		switch req.Kind {
		case RuntimeStorageValidationCheckpoint:
			hostResult = s.validateCheckpointStorageOnHost(ctx, hostInfo, req.Checkpoint)
		case RuntimeStorageValidationIMAP:
			hostResult = s.validateIMAPStorageOnHost(ctx, hostInfo, req.IMAP)
		default:
			return nil, fmt.Errorf("unsupported runtime storage validation kind: %s", req.Kind)
		}
		hostResult.HostID = hostID
		hostResult.HostName = hostName
		if !hostResult.Success {
			result.Success = false
		}
		result.Hosts = append(result.Hosts, hostResult)
	}

	return result, nil
}

func (s *Service) validateCheckpointStorageOnHost(ctx context.Context, host *HostInfo, cfg *CheckpointConfig) *RuntimeStorageValidationHostResult {
	if cfg == nil {
		return &RuntimeStorageValidationHostResult{Success: false, Message: "checkpoint config is required"}
	}
	switch cfg.StorageType {
	case CheckpointStorageLocalFile:
		return s.runPathReadyCheck(ctx, host.AgentID, strings.TrimSpace(cfg.Namespace))
	case CheckpointStorageHDFS:
		if cfg.HDFSHAEnabled {
			if strings.TrimSpace(cfg.HDFSNameServices) == "" {
				return &RuntimeStorageValidationHostResult{Success: false, Message: "hdfs_name_services is required for HDFS HA storage"}
			}
			return s.validateHDFSHAOnHost(ctx, host.AgentID, cfg.HDFSHANamenodes, cfg.HDFSNamenodeRPCAddress1, cfg.HDFSNamenodeRPCAddress2)
		}
		return s.runTCPCheck(ctx, host.AgentID, strings.TrimSpace(cfg.HDFSNameNodeHost), cfg.HDFSNameNodePort)
	case CheckpointStorageOSS, CheckpointStorageS3:
		return s.validateObjectStoreEndpoint(ctx, host.AgentID, cfg.StorageEndpoint, cfg.StorageBucket)
	default:
		return &RuntimeStorageValidationHostResult{Success: false, Message: fmt.Sprintf("unsupported checkpoint storage type: %s", cfg.StorageType)}
	}
}

func (s *Service) validateIMAPStorageOnHost(ctx context.Context, host *HostInfo, cfg *IMAPConfig) *RuntimeStorageValidationHostResult {
	if cfg == nil {
		return &RuntimeStorageValidationHostResult{Success: false, Message: "imap config is required"}
	}
	switch cfg.StorageType {
	case IMAPStorageDisabled:
		return &RuntimeStorageValidationHostResult{
			Success: true,
			Message: "IMAP uses in-memory mode; no external storage validation required",
			Details: map[string]string{"mode": "disabled"},
		}
	case IMAPStorageLocalFile:
		return s.runPathReadyCheck(ctx, host.AgentID, strings.TrimSpace(cfg.Namespace))
	case IMAPStorageHDFS:
		if cfg.HDFSHAEnabled {
			if strings.TrimSpace(cfg.HDFSNameServices) == "" {
				return &RuntimeStorageValidationHostResult{Success: false, Message: "hdfs_name_services is required for HDFS HA storage"}
			}
			return s.validateHDFSHAOnHost(ctx, host.AgentID, cfg.HDFSHANamenodes, cfg.HDFSNamenodeRPCAddress1, cfg.HDFSNamenodeRPCAddress2)
		}
		return s.runTCPCheck(ctx, host.AgentID, strings.TrimSpace(cfg.HDFSNameNodeHost), cfg.HDFSNameNodePort)
	case IMAPStorageOSS, IMAPStorageS3:
		return s.validateObjectStoreEndpoint(ctx, host.AgentID, cfg.StorageEndpoint, cfg.StorageBucket)
	default:
		return &RuntimeStorageValidationHostResult{Success: false, Message: fmt.Sprintf("unsupported imap storage type: %s", cfg.StorageType)}
	}
}

func (s *Service) validateHDFSHAOnHost(ctx context.Context, agentID string, rawNamenodes string, addr1, addr2 string) *RuntimeStorageValidationHostResult {
	endpoints, err := seatunnel.ResolveHDFSHARPCAddresses(rawNamenodes, addr1, addr2)
	if err != nil {
		return &RuntimeStorageValidationHostResult{Success: false, Message: err.Error()}
	}
	details := map[string]string{}
	allSuccess := true
	messages := make([]string, 0, len(endpoints))
	for idx, endpoint := range endpoints {
		host, port := splitHostPortWithDefault(endpoint.Address, 8020)
		result := s.runTCPCheck(ctx, agentID, host, port)
		key := fmt.Sprintf("namenode_%d", idx+1)
		details[key] = endpoint.Address
		details[fmt.Sprintf("%s_alias", key)] = endpoint.Name
		if !result.Success {
			allSuccess = false
		}
		messages = append(messages, result.Message)
		for k, v := range result.Details {
			details[fmt.Sprintf("%s_%s", key, k)] = v
		}
	}
	return &RuntimeStorageValidationHostResult{
		Success: allSuccess,
		Message: strings.Join(messages, "; "),
		Details: details,
	}
}

func (s *Service) validateObjectStoreEndpoint(ctx context.Context, agentID, endpoint, bucket string) *RuntimeStorageValidationHostResult {
	host, port := parseEndpointHostPort(endpoint, 443)
	result := s.runTCPCheck(ctx, agentID, host, port)
	if result.Details == nil {
		result.Details = map[string]string{}
	}
	if strings.TrimSpace(bucket) != "" {
		result.Details["bucket"] = strings.TrimSpace(bucket)
	}
	return result
}

func (s *Service) runPathReadyCheck(ctx context.Context, agentID, path string) *RuntimeStorageValidationHostResult {
	if strings.TrimSpace(path) == "" {
		return &RuntimeStorageValidationHostResult{Success: false, Message: "path is required"}
	}
	success, output, err := s.agentManager.SendCommand(ctx, agentID, "check_path_ready", map[string]string{"path": path})
	if err != nil {
		return &RuntimeStorageValidationHostResult{Success: false, Message: err.Error()}
	}
	return runtimeStorageHostResultFromCommandOutput(success, output)
}

func (s *Service) runTCPCheck(ctx context.Context, agentID, host string, port int) *RuntimeStorageValidationHostResult {
	if strings.TrimSpace(host) == "" || port <= 0 {
		return &RuntimeStorageValidationHostResult{Success: false, Message: "host and port are required"}
	}
	success, output, err := s.agentManager.SendCommand(ctx, agentID, "check_tcp", map[string]string{
		"host": host,
		"port": strconv.Itoa(port),
	})
	if err != nil {
		return &RuntimeStorageValidationHostResult{Success: false, Message: err.Error()}
	}
	return runtimeStorageHostResultFromCommandOutput(success, output)
}

func runtimeStorageHostResultFromCommandOutput(success bool, output string) *RuntimeStorageValidationHostResult {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return &RuntimeStorageValidationHostResult{Success: success, Message: ""}
	}
	var parsed struct {
		Success bool              `json:"success"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil && parsed.Message != "" {
		return &RuntimeStorageValidationHostResult{
			Success: parsed.Success,
			Message: parsed.Message,
			Details: parsed.Details,
		}
	}
	return &RuntimeStorageValidationHostResult{
		Success: success,
		Message: trimmed,
	}
}

func parseEndpointHostPort(raw string, defaultPort int) (string, int) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", 0
	}
	if strings.Contains(trimmed, "://") {
		if u, err := url.Parse(trimmed); err == nil {
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

// javaCheckResponse represents the JSON response from Agent's check_java command
// javaCheckResponse 表示 Agent check_java 命令的 JSON 响应
type javaCheckResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Details map[string]string `json:"details"`
}

// parseJavaVersionFromOutput parses Java major version from command output.
// parseJavaVersionFromOutput 从命令输出解析 Java 主版本号。
// Expected formats: JSON from Agent, "java_version=8", "8", "1.8.0_xxx", "11.0.x", etc.
// 预期格式：Agent 返回的 JSON、"java_version=8"、"8"、"1.8.0_xxx"、"11.0.x" 等
func parseJavaVersionFromOutput(output string) int {
	output = strings.TrimSpace(output)

	// Try to parse JSON response from Agent
	// 尝试解析 Agent 返回的 JSON 响应
	if strings.HasPrefix(output, "{") {
		var resp javaCheckResponse
		if err := json.Unmarshal([]byte(output), &resp); err == nil {
			// Try to get version from details.installed_version
			// 尝试从 details.installed_version 获取版本
			if versionStr, ok := resp.Details["installed_version"]; ok {
				var version int
				fmt.Sscanf(versionStr, "%d", &version)
				if version > 0 {
					return version
				}
			}
		}
	}

	// Try to parse "java_version=X" format
	// 尝试解析 "java_version=X" 格式
	if strings.Contains(output, "java_version=") {
		parts := strings.Split(output, "java_version=")
		if len(parts) >= 2 {
			versionStr := strings.TrimSpace(parts[1])
			// Take first number
			// 取第一个数字
			versionStr = strings.Split(versionStr, "\n")[0]
			versionStr = strings.Split(versionStr, " ")[0]
			var version int
			fmt.Sscanf(versionStr, "%d", &version)
			if version > 0 {
				return version
			}
		}
	}

	// Try to parse "1.8.0_xxx" format (Java 8)
	// 尝试解析 "1.8.0_xxx" 格式（Java 8）
	if strings.HasPrefix(output, "1.") {
		parts := strings.Split(output, ".")
		if len(parts) >= 2 {
			var version int
			fmt.Sscanf(parts[1], "%d", &version)
			if version > 0 {
				return version
			}
		}
	}

	// Try to parse "11.0.x" or just "11" format (Java 9+)
	// 尝试解析 "11.0.x" 或 "11" 格式（Java 9+）
	parts := strings.Split(output, ".")
	if len(parts) >= 1 {
		var version int
		fmt.Sscanf(parts[0], "%d", &version)
		if version > 0 {
			return version
		}
	}

	return 0
}

// ==================== Installation 安装 ====================

// StartInstallation starts a new installation.
// StartInstallation 开始新的安装。
func (s *Service) StartInstallation(ctx context.Context, req *InstallationRequest) (*InstallationStatus, error) {
	if req.InstallMode == "" {
		req.InstallMode = InstallModeOnline
	}

	s.resolveInstallationJVM(ctx, req)

	s.installMu.Lock()
	defer s.installMu.Unlock()

	// Check if installation is already in progress / 检查是否已有安装正在进行
	if existing, ok := s.installations[req.HostID]; ok {
		if existing.Status == StepStatusRunning {
			return nil, ErrInstallationInProgress
		}
	}

	// Create new installation status / 创建新的安装状态
	status := &InstallationStatus{
		ID:          uuid.New().String(),
		HostID:      req.HostID,
		ClusterID:   strings.TrimSpace(req.ClusterID),
		Status:      StepStatusRunning,
		CurrentStep: InstallStepDownload,
		Steps:       createInitialSteps(),
		Progress:    0,
		Message:     "Installation started / 安装已开始",
		StartTime:   time.Now(),
	}

	s.installations[req.HostID] = status

	// Start installation in background / 在后台开始安装
	go s.runInstallation(context.Background(), req, status)

	return status, nil
}

func (s *Service) resolveInstallationJVM(ctx context.Context, req *InstallationRequest) {
	if s == nil || req == nil || req.JVM != nil || s.nodeJVMResolver == nil {
		return
	}
	if strings.TrimSpace(req.ClusterID) == "" || strings.TrimSpace(req.HostID) == "" || strings.TrimSpace(string(req.NodeRole)) == "" {
		return
	}

	clusterID, err := strconv.ParseUint(strings.TrimSpace(req.ClusterID), 10, 64)
	if err != nil {
		return
	}
	hostID, err := strconv.ParseUint(strings.TrimSpace(req.HostID), 10, 64)
	if err != nil {
		return
	}

	resolved, err := s.nodeJVMResolver.ResolveNodeJVMByClusterAndHostAndRole(ctx, uint(clusterID), uint(hostID), string(req.NodeRole))
	if err != nil {
		logger.WarnF(ctx, "[Installer] failed to resolve node JVM config: cluster=%d, host=%d, role=%s, err=%v", clusterID, hostID, req.NodeRole, err)
		return
	}
	if resolved == nil {
		return
	}

	req.JVM = resolved
	logger.InfoF(ctx, "[Installer] resolved node JVM config for installation: cluster=%d, host=%d, role=%s", clusterID, hostID, req.NodeRole)
}

// GetInstallationStatus returns the current installation status.
// GetInstallationStatus 返回当前安装状态。
func (s *Service) GetInstallationStatus(ctx context.Context, hostID uint) (*InstallationStatus, error) {
	s.installMu.RLock()
	defer s.installMu.RUnlock()

	hostIDStr := fmt.Sprintf("%d", hostID)
	status, ok := s.installations[hostIDStr]
	if !ok {
		return nil, ErrInstallationNotFound
	}

	return status, nil
}

// RetryStep retries a failed installation step.
// RetryStep 重试失败的安装步骤。
func (s *Service) RetryStep(ctx context.Context, hostID uint, step string) (*InstallationStatus, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()

	hostIDStr := fmt.Sprintf("%d", hostID)
	status, ok := s.installations[hostIDStr]
	if !ok {
		return nil, ErrInstallationNotFound
	}

	// Find and reset the step / 找到并重置步骤
	for i := range status.Steps {
		if status.Steps[i].Name == step {
			status.Steps[i].Status = StepStatusPending
			status.Steps[i].Error = ""
			break
		}
	}

	status.Status = StepStatusRunning
	status.Error = ""

	// TODO: Resume installation from the failed step
	// TODO: 从失败的步骤恢复安装

	return status, nil
}

// CancelInstallation cancels an ongoing installation.
// CancelInstallation 取消正在进行的安装。
func (s *Service) CancelInstallation(ctx context.Context, hostID uint) (*InstallationStatus, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()

	hostIDStr := fmt.Sprintf("%d", hostID)
	status, ok := s.installations[hostIDStr]
	if !ok {
		return nil, ErrInstallationNotFound
	}

	// TODO: Send cancel command to Agent
	// TODO: 向 Agent 发送取消命令

	now := time.Now()
	status.Status = StepStatusFailed
	status.Message = "Installation cancelled / 安装已取消"
	status.EndTime = &now

	return status, nil
}

// runInstallation runs the installation process via Agent gRPC.
// runInstallation 通过 Agent gRPC 运行安装过程。
func (s *Service) runInstallation(ctx context.Context, req *InstallationRequest, status *InstallationStatus) {
	logger.InfoF(ctx, "[Installer] 开始安装 / Start installation: host=%s, version=%s, mode=%s", req.HostID, req.Version, req.InstallMode)

	// Check if agent manager is available
	// 检查 Agent 管理器是否可用
	if s.agentManager == nil {
		logger.ErrorF(ctx, "[Installer] Agent 管理器不可用 / Agent manager not available")
		s.installMu.Lock()
		now := time.Now()
		status.Status = StepStatusFailed
		status.Error = "Agent manager not available / Agent 管理器不可用"
		status.EndTime = &now
		s.installMu.Unlock()
		return
	}

	// Get agent connection for the host
	// 获取主机的 Agent 连接
	hostID, err := parseHostID(req.HostID)
	if err != nil {
		logger.ErrorF(ctx, "[Installer] 无效的主机 ID / Invalid host ID: %s", req.HostID)
		s.installMu.Lock()
		now := time.Now()
		status.Status = StepStatusFailed
		status.Error = fmt.Sprintf("Invalid host ID: %v / 无效的主机 ID: %v", err, err)
		status.EndTime = &now
		s.installMu.Unlock()
		return
	}

	agentID, connected := s.agentManager.GetAgentByHostID(hostID)
	if !connected || agentID == "" {
		// Agent not connected, return error
		// Agent 未连接，返回错误
		logger.ErrorF(ctx, "[Installer] Agent 未连接 / Agent not connected: host=%d", hostID)
		s.installMu.Lock()
		now := time.Now()
		status.Status = StepStatusFailed
		status.Error = "Host agent not connected / 主机 Agent 未连接"
		status.EndTime = &now
		s.installMu.Unlock()
		return
	}

	logger.DebugF(ctx, "[Installer] 连接到 Agent / Connected to Agent: host=%d, agent=%s", hostID, agentID)

	// For online/offline mode, resolve package on Control Plane and transfer to Agent
	// 对于在线/离线模式，先在 Control Plane 上确定安装包并传输到 Agent
	var localPackagePath string

	if req.InstallMode == InstallModeOnline {
		localPackagePath = filepath.Join(s.packageDir, packageFileName(req.Version))

		if _, err := os.Stat(localPackagePath); os.IsNotExist(err) {
			// Package not found locally, need to download first
			// 本地未找到安装包，需要先下载
			logger.InfoF(ctx, "[Installer] 本地未找到安装包，开始下载 / Package not found locally, starting download: version=%s", req.Version)

			s.installMu.Lock()
			status.Message = "Downloading package to Control Plane... / 正在下载安装包到控制平面..."
			s.installMu.Unlock()

			// Start download task
			// 启动下载任务
			mirror := req.Mirror
			if mirror == "" {
				mirror = MirrorAliyun
			}
			task, err := s.StartDownload(ctx, &DownloadRequest{
				Version: req.Version,
				Mirror:  mirror,
			})
			if err != nil && err != ErrDownloadInProgress {
				logger.ErrorF(ctx, "[Installer] 启动下载失败 / Failed to start download: %v", err)
				s.installMu.Lock()
				now := time.Now()
				status.Status = StepStatusFailed
				status.Error = fmt.Sprintf("Failed to download package: %v / 下载安装包失败: %v", err, err)
				status.EndTime = &now
				s.installMu.Unlock()
				return
			}

			// Wait for download to complete
			// 等待下载完成（GetDownloadStatus 按 version 查 map，勿用 task.ID）
			for {
				task, err = s.GetDownloadStatus(ctx, task.Version)
				if err != nil {
					logger.ErrorF(ctx, "[Installer] 获取下载状态失败 / Failed to get download status: %v", err)
					s.installMu.Lock()
					now := time.Now()
					status.Status = StepStatusFailed
					status.Error = fmt.Sprintf("Failed to get download status: %v / 获取下载状态失败: %v", err, err)
					status.EndTime = &now
					s.installMu.Unlock()
					return
				}

				if task.Status == DownloadStatusCompleted {
					logger.InfoF(ctx, "[Installer] 安装包下载完成 / Package download completed: version=%s", req.Version)
					break
				}

				if task.Status == DownloadStatusFailed {
					logger.ErrorF(ctx, "[Installer] 安装包下载失败 / Package download failed: %s", task.Error)
					s.installMu.Lock()
					now := time.Now()
					status.Status = StepStatusFailed
					status.Error = fmt.Sprintf("Package download failed: %s / 安装包下载失败: %s", task.Error, task.Error)
					status.EndTime = &now
					s.installMu.Unlock()
					return
				}

				s.installMu.Lock()
				status.Message = fmt.Sprintf("Downloading package... %d%% / 正在下载安装包... %d%%", task.Progress, task.Progress)
				s.installMu.Unlock()

				time.Sleep(1 * time.Second)
			}
		} else {
			logger.InfoF(ctx, "[Installer] 使用本地已有安装包 / Using existing local package: %s", localPackagePath)
		}
	}

	if req.InstallMode == InstallModeOffline {
		localPackagePath, err = s.resolveOfflinePackagePath(req)
		if err != nil {
			s.installMu.Lock()
			now := time.Now()
			status.Status = StepStatusFailed
			status.Error = err.Error()
			status.EndTime = &now
			s.installMu.Unlock()
			return
		}

		if _, err := os.Stat(localPackagePath); err != nil {
			logger.ErrorF(ctx, "[Installer] 离线安装包不存在 / Offline package not found: %s", localPackagePath)
			s.installMu.Lock()
			now := time.Now()
			status.Status = StepStatusFailed
			status.Error = fmt.Sprintf("Offline package not found: %s / 离线安装包不存在: %s", localPackagePath, localPackagePath)
			status.EndTime = &now
			s.installMu.Unlock()
			return
		}

		logger.InfoF(ctx, "[Installer] 离线模式使用本地安装包 / Offline mode using local package: %s", localPackagePath)
	}

	if localPackagePath != "" {
		if cachedRemotePath, ok := s.getPreparedPackageRemotePath(agentID, req.Version, localPackagePath); ok {
			logger.InfoF(
				ctx,
				"[Installer] 复用已传输安装包 / Reusing previously transferred package: agent=%s, version=%s, remote_path=%s",
				agentID,
				req.Version,
				cachedRemotePath,
			)
			s.installMu.Lock()
			status.Message = "Reusing package already transferred to Agent... / 复用已传输到 Agent 的安装包..."
			s.installMu.Unlock()
			req.PackagePath = cachedRemotePath
		} else {
			// Transfer package to Agent via gRPC
			// 通过 gRPC 传输安装包到 Agent
			s.installMu.Lock()
			status.Message = "Transferring package to Agent... / 正在传输安装包到 Agent..."
			s.installMu.Unlock()

			remotePath, err := s.transferPackageFileToAgent(ctx, agentID, req.Version, localPackagePath, status)
			if err != nil {
				if req.InstallMode == InstallModeOnline {
					// Transfer failed, fallback to mirror download
					// 传输失败，回退到镜像源下载
					logger.WarnF(ctx, "[Installer] 安装包传输失败，回退到镜像源下载 / Package transfer failed, fallback to mirror download: %v", err)
					// Continue with mirror download mode
					// 继续使用镜像源下载模式
				} else {
					s.installMu.Lock()
					now := time.Now()
					status.Status = StepStatusFailed
					status.Error = fmt.Sprintf("Failed to transfer offline package: %v / 传输离线安装包失败: %v", err, err)
					status.EndTime = &now
					s.installMu.Unlock()
					return
				}
			} else {
				// Transfer succeeded, update params to use package on Agent
				// 传输成功，更新参数使用 Agent 本地安装包
				logger.InfoF(ctx, "[Installer] 安装包传输成功 / Package transfer succeeded: remote_path=%s", remotePath)
				req.PackagePath = remotePath
				s.rememberPreparedPackage(agentID, req.Version, localPackagePath, remotePath)
			}
		}
	}

	// Transfer selected plugins to Agent before installation
	// 在安装之前将选中的插件传输到 Agent
	if req.Connector != nil && len(req.Connector.SelectedPlugins) > 0 && s.pluginTransferer != nil {
		s.installMu.Lock()
		status.Message = "Transferring plugins to Agent... / 正在传输插件到 Agent..."
		s.installMu.Unlock()

		// Use install dir from request, default to /opt/seatunnel-{version}
		// 使用请求中的安装目录，默认为 /opt/seatunnel-{version}
		installDir := req.InstallDir
		if installDir == "" {
			installDir = seatunnel.DefaultInstallDir(req.Version)
		}
		pluginMirror := string(req.Mirror)
		if req.Connector.PluginRepo != "" {
			pluginMirror = string(req.Connector.PluginRepo)
		}
		for i, pluginName := range req.Connector.SelectedPlugins {
			selectedProfileKeys := normalizeProfileKeys(req.Connector.SelectedPluginProfiles[pluginName])
			logger.InfoF(ctx, "[Installer] 传输插件 / Transferring plugin: %s (%d/%d)", pluginName, i+1, len(req.Connector.SelectedPlugins))

			preparedPluginFingerprint, fingerprintErr := s.resolvePreparedPluginFingerprint(ctx, pluginName, req.Version, selectedProfileKeys)
			if fingerprintErr != nil {
				logger.WarnF(
					ctx,
					"[Installer] 计算插件依赖指纹失败，将跳过缓存复用 / Failed to compute plugin dependency fingerprint, cache reuse disabled: plugin=%s, profiles=%v, error=%v",
					pluginName,
					selectedProfileKeys,
					fingerprintErr,
				)
			}

			if s.hasPreparedPlugin(agentID, pluginName, req.Version, installDir, selectedProfileKeys, preparedPluginFingerprint) {
				logger.InfoF(
					ctx,
					"[Installer] 复用已准备插件 / Reusing prepared plugin: %s, profiles=%v, agent=%s, fingerprint=%s",
					pluginName,
					selectedProfileKeys,
					agentID,
					preparedPluginFingerprint,
				)
				s.installMu.Lock()
				status.Message = fmt.Sprintf(
					"Reusing prepared plugin %s (%d/%d)... / 正在复用已准备插件 %s (%d/%d)...",
					pluginName, i+1, len(req.Connector.SelectedPlugins),
					pluginName, i+1, len(req.Connector.SelectedPlugins),
				)
				s.installMu.Unlock()
				continue
			}

			s.installMu.Lock()
			status.Message = fmt.Sprintf("Transferring plugin %s (%d/%d)... / 正在传输插件 %s (%d/%d)...",
				pluginName, i+1, len(req.Connector.SelectedPlugins),
				pluginName, i+1, len(req.Connector.SelectedPlugins))
			s.installMu.Unlock()

			// Always prepare the effective plugin package before transfer.
			// 在传输前始终准备插件的生效安装包。
			logger.InfoF(
				ctx,
				"[Installer] 准备插件包 / Preparing plugin package: %s, profiles=%v, mirror=%s",
				pluginName,
				selectedProfileKeys,
				pluginMirror,
			)
			if err := s.pluginTransferer.DownloadPluginSync(ctx, pluginName, req.Version, pluginMirror, selectedProfileKeys); err != nil {
				logger.WarnF(ctx, "[Installer] 准备插件失败，跳过 / Failed to prepare plugin, skipping: %s, error=%v", pluginName, err)
				continue
			}

			// Transfer plugin to Agent
			// 传输插件到 Agent
			if err := s.pluginTransferer.TransferPluginToAgent(ctx, agentID, pluginName, req.Version, installDir, selectedProfileKeys); err != nil {
				logger.WarnF(ctx, "[Installer] 传输插件失败，跳过 / Failed to transfer plugin, skipping: %s, error=%v", pluginName, err)
				continue
			}

			logger.InfoF(ctx, "[Installer] 插件传输成功 / Plugin transferred successfully: %s", pluginName)
			if preparedPluginFingerprint == "" {
				preparedPluginFingerprint, fingerprintErr = s.resolvePreparedPluginFingerprint(ctx, pluginName, req.Version, selectedProfileKeys)
				if fingerprintErr != nil {
					logger.WarnF(
						ctx,
						"[Installer] 传输后重新计算插件依赖指纹失败，将不记录缓存 / Failed to recompute plugin dependency fingerprint after transfer, skip cache record: plugin=%s, profiles=%v, error=%v",
						pluginName,
						selectedProfileKeys,
						fingerprintErr,
					)
				}
			}
			s.rememberPreparedPlugin(agentID, pluginName, req.Version, installDir, selectedProfileKeys, preparedPluginFingerprint)
		}
	}

	// Build installation parameters for Agent
	// 构建 Agent 的安装参数
	params := buildInstallParams(req)

	// Send install command to Agent
	// 向 Agent 发送安装命令
	commandID, err := s.agentManager.SendInstallCommand(ctx, agentID, params)
	if err != nil {
		logger.ErrorF(ctx, "[Installer] 发送安装命令失败 / Failed to send install command: host=%d, error=%v", hostID, err)
		s.installMu.Lock()
		now := time.Now()
		status.Status = StepStatusFailed
		status.Error = fmt.Sprintf("Failed to send install command: %v / 发送安装命令失败: %v", err, err)
		status.EndTime = &now
		s.installMu.Unlock()
		return
	}

	logger.InfoF(ctx, "[Installer] 安装命令已发送 / Install command sent: host=%d, command=%s", hostID, commandID)

	// Poll for command status updates
	// 轮询命令状态更新
	s.pollInstallationStatus(ctx, commandID, status, agentID, req)
}

// runInstallationSimulated runs a simulated installation (for testing or when Agent is not available).
// runInstallationSimulated 运行模拟安装（用于测试或 Agent 不可用时）。
// pollInstallationStatus polls the Agent for installation status updates.
// pollInstallationStatus 轮询 Agent 获取安装状态更新。
func (s *Service) pollInstallationStatus(ctx context.Context, commandID string, status *InstallationStatus, agentID string, req *InstallationRequest) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.installMu.Lock()
			now := time.Now()
			status.Status = StepStatusFailed
			status.Error = "Installation cancelled / 安装已取消"
			status.EndTime = &now
			s.installMu.Unlock()
			return

		case <-ticker.C:
			cmdStatus, progress, message, err := s.agentManager.GetCommandStatus(commandID)
			if err != nil {
				// Command not found or error, continue polling
				// 命令未找到或出错，继续轮询
				continue
			}

			s.installMu.Lock()
			status.Progress = progress
			status.Message = message
			if warningMessage := extractInstallationWarning(message); warningMessage != "" {
				appendInstallationWarning(status, warningMessage)
			}

			// Parse step from message format: [step] message
			// 从消息格式解析步骤: [step] message
			currentStep := parseStepFromMessage(message)
			if currentStep != "" {
				status.CurrentStep = InstallStep(currentStep)
				// Update step status based on current step
				// 根据当前步骤更新步骤状态
				updateStepStatus(status, currentStep, progress, message)
			}

			// Map command status to installation status
			// 将命令状态映射到安装状态
			switch cmdStatus {
			case "success":
				now := time.Now()
				status.Status = StepStatusSuccess
				status.Progress = 100
				if len(status.Warnings) > 0 {
					status.Message = "Installation completed with warnings, starting cluster... / 安装完成，但存在警告，正在启动集群..."
				} else {
					status.Message = "Installation completed, starting cluster... / 安装完成，正在启动集群..."
				}
				status.EndTime = &now
				// Mark all steps as complete
				// 将所有步骤标记为完成
				for j := range status.Steps {
					status.Steps[j].Status = StepStatusSuccess
					status.Steps[j].Progress = 100
					status.Steps[j].EndTime = &now
				}
				s.installMu.Unlock()
				logger.InfoF(ctx, "[Installer] 安装成功 / Installation succeeded: command=%s", commandID)

				// Start SeaTunnel cluster after installation
				// 安装完成后启动 SeaTunnel 集群
				s.startClusterAfterInstall(ctx, agentID, req, status)
				return

			case "failed":
				now := time.Now()
				status.Status = StepStatusFailed
				status.Error = message
				status.EndTime = &now
				s.installMu.Unlock()
				logger.ErrorF(ctx, "[Installer] 安装失败 / Installation failed: command=%s, error=%s", commandID, message)
				return

			case "cancelled":
				now := time.Now()
				status.Status = StepStatusFailed
				status.Error = "Installation cancelled / 安装已取消"
				status.EndTime = &now
				s.installMu.Unlock()
				logger.InfoF(ctx, "[Installer] 安装已取消 / Installation cancelled: command=%s", commandID)
				return

			case "running":
				// Status already updated above
				// 状态已在上面更新
			}
			s.installMu.Unlock()
		}
	}
}

// startClusterAfterInstall starts the SeaTunnel cluster after installation completes.
// startClusterAfterInstall 在安装完成后启动 SeaTunnel 集群。
func (s *Service) startClusterAfterInstall(ctx context.Context, agentID string, req *InstallationRequest, status *InstallationStatus) {
	// Build node info for logging / 构建节点信息用于日志
	nodeRole := string(req.NodeRole)
	if nodeRole == "" {
		nodeRole = "unknown"
	}

	logger.InfoF(ctx, "[Installer] 开始启动节点 / Starting node: cluster=%s, host=%s, role=%s, agent=%s",
		req.ClusterID, req.HostID, nodeRole, agentID)

	// Update status
	// 更新状态
	s.installMu.Lock()
	status.Message = fmt.Sprintf("Starting SeaTunnel node (%s)... / 正在启动 SeaTunnel 节点 (%s)...", nodeRole, nodeRole)
	s.installMu.Unlock()

	// Parse cluster ID and host ID
	// 解析集群 ID 和主机 ID
	clusterID, clusterErr := parseClusterID(req.ClusterID)
	hostID, hostErr := parseHostID(req.HostID)
	if clusterErr != nil || hostErr != nil {
		logger.ErrorF(ctx, "[Installer] 解析 ID 失败 / Failed to parse IDs: cluster=%s, host=%s, role=%s, clusterErr=%v, hostErr=%v",
			req.ClusterID, req.HostID, nodeRole, clusterErr, hostErr)
		s.installMu.Lock()
		status.Message = fmt.Sprintf("Installation completed but failed to start node (%s): invalid cluster or host ID / 安装完成但启动节点 (%s) 失败: 无效的集群或主机 ID", nodeRole, nodeRole)
		s.installMu.Unlock()
		return
	}

	// Use nodeStarter to start the node (reuses cluster service logic)
	// 使用 nodeStarter 启动节点（复用集群服务逻辑）
	if s.nodeStarter == nil {
		logger.ErrorF(ctx, "[Installer] nodeStarter 未配置 / nodeStarter not configured: cluster=%d, host=%d, role=%s",
			clusterID, hostID, nodeRole)
		s.installMu.Lock()
		status.Message = fmt.Sprintf("Installation completed but failed to start node (%s): nodeStarter not configured / 安装完成但启动节点 (%s) 失败: nodeStarter 未配置", nodeRole, nodeRole)
		s.installMu.Unlock()
		return
	}

	// Use role to start the specific node; same host may have both master and worker (separated mode)
	// 按角色启动对应节点；同一主机可能同时有 master 与 worker（分离模式）
	success, message, err := s.nodeStarter.StartNodeByClusterAndHostAndRole(ctx, clusterID, hostID, nodeRole)
	if err != nil {
		logger.ErrorF(ctx, "[Installer] 启动节点失败 / Failed to start node: cluster=%d, host=%d, role=%s, error=%v",
			clusterID, hostID, nodeRole, err)
		s.installMu.Lock()
		status.Message = fmt.Sprintf("Installation completed but failed to start node (%s): %v / 安装完成但启动节点 (%s) 失败: %v", nodeRole, err, nodeRole, err)
		s.installMu.Unlock()
		return
	}

	if !success {
		logger.WarnF(ctx, "[Installer] 启动节点返回失败 / Start node returned failure: cluster=%d, host=%d, role=%s, message=%s",
			clusterID, hostID, nodeRole, message)
		s.installMu.Lock()
		status.Message = fmt.Sprintf("Installation completed but node (%s) start failed: %s / 安装完成但节点 (%s) 启动失败: %s", nodeRole, message, nodeRole, message)
		s.installMu.Unlock()
		return
	}

	logger.InfoF(ctx, "[Installer] 节点启动成功 / Node started successfully: cluster=%d, host=%d, role=%s",
		clusterID, hostID, nodeRole)

	// Note: Plugin recording is handled at cluster level, not per-node
	// 注意：插件记录在集群级别处理，不是每个节点
	// The first node installation will record plugins for the cluster
	// 第一个节点安装时会为集群记录插件
	// Check if plugins already recorded for this cluster to avoid duplicates
	// 检查是否已为此集群记录插件，避免重复
	if s.pluginTransferer != nil && req.Connector != nil && len(req.Connector.SelectedPlugins) > 0 {
		for _, pluginName := range req.Connector.SelectedPlugins {
			// RecordInstalledPlugin should handle duplicates internally (upsert or skip)
			// RecordInstalledPlugin 应该在内部处理重复（更新或跳过）
			if err := s.pluginTransferer.RecordInstalledPlugin(ctx, clusterID, pluginName, req.Version); err != nil {
				// Only log warning, don't fail the installation
				// 只记录警告，不要让安装失败
				logger.DebugF(ctx, "[Installer] 记录插件时出现问题（可能已存在）/ Issue recording plugin (may already exist): cluster=%d, plugin=%s, error=%v",
					clusterID, pluginName, err)
			}
		}
	}

	// Initialize cluster configs after successful installation
	// 安装成功后初始化集群配置
	if s.configInitializer != nil {
		logger.InfoF(ctx, "[Installer] 初始化集群配置 / Initializing cluster configs: cluster=%d, host=%d",
			clusterID, hostID)
		if err := s.configInitializer.InitClusterConfigs(ctx, clusterID, hostID, req.InstallDir, 0); err != nil {
			// Only log warning, don't fail the installation
			// 只记录警告，不要让安装失败
			logger.WarnF(ctx, "[Installer] 初始化集群配置失败（不影响安装）/ Failed to initialize cluster configs (non-fatal): cluster=%d, host=%d, error=%v",
				clusterID, hostID, err)
		} else {
			logger.InfoF(ctx, "[Installer] 集群配置初始化成功 / Cluster configs initialized successfully: cluster=%d, host=%d",
				clusterID, hostID)
		}
	}

	// Final status update
	// 最终状态更新
	s.installMu.Lock()
	if len(status.Warnings) > 0 {
		status.Message = fmt.Sprintf(
			"Installation and node (%s) startup completed with warnings / 安装和节点 (%s) 启动完成，但存在警告",
			nodeRole,
			nodeRole,
		)
	} else {
		status.Message = fmt.Sprintf("Installation and node (%s) startup completed / 安装和节点 (%s) 启动完成", nodeRole, nodeRole)
	}
	s.installMu.Unlock()
}

// parseStepFromMessage extracts the step name from message format: [step] message
// parseStepFromMessage 从消息格式中提取步骤名称: [step] message
func parseStepFromMessage(message string) string {
	if len(message) < 3 || message[0] != '[' {
		return ""
	}
	endIdx := strings.Index(message, "]")
	if endIdx == -1 {
		return ""
	}
	return message[1:endIdx]
}

// updateStepStatus updates the step status based on current step and progress
// updateStepStatus 根据当前步骤和进度更新步骤状态
func updateStepStatus(status *InstallationStatus, currentStep string, progress int, message string) {
	// Find the index of current step
	// 找到当前步骤的索引
	currentIdx := -1
	for i, step := range status.Steps {
		if string(step.Step) == currentStep {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return
	}

	now := time.Now()

	// Mark previous steps as complete
	// 将之前的步骤标记为完成
	for j := 0; j < currentIdx; j++ {
		if status.Steps[j].Status != StepStatusSuccess {
			status.Steps[j].Status = StepStatusSuccess
			status.Steps[j].Progress = 100
			status.Steps[j].EndTime = &now
		}
	}

	// Mark current step as running
	// 将当前步骤标记为运行中
	if status.Steps[currentIdx].Status != StepStatusRunning {
		status.Steps[currentIdx].Status = StepStatusRunning
		status.Steps[currentIdx].StartTime = &now
	}
	// Extract message without step prefix
	// 提取不带步骤前缀的消息
	msgWithoutPrefix := message
	if idx := strings.Index(message, "] "); idx != -1 {
		msgWithoutPrefix = message[idx+2:]
	}
	status.Steps[currentIdx].Message = msgWithoutPrefix
	status.Steps[currentIdx].Progress = progress
}

func extractInstallationWarning(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, "] "); idx != -1 {
		trimmed = strings.TrimSpace(trimmed[idx+2:])
	}
	if strings.HasPrefix(trimmed, "Warning:") {
		return trimmed
	}
	return ""
}

func appendInstallationWarning(status *InstallationStatus, warning string) {
	if status == nil {
		return
	}
	trimmed := strings.TrimSpace(warning)
	if trimmed == "" {
		return
	}
	for _, existing := range status.Warnings {
		if existing == trimmed {
			return
		}
	}
	status.Warnings = append(status.Warnings, trimmed)
}

// buildInstallParams builds installation parameters for Agent command.
// buildInstallParams 构建 Agent 命令的安装参数。
func buildInstallParams(req *InstallationRequest) map[string]string {
	// Use install_dir from request, default to /opt/seatunnel-{version}
	// 使用请求中的 install_dir，默认为 /opt/seatunnel-{version}
	installDir := req.InstallDir
	if installDir == "" {
		installDir = seatunnel.DefaultInstallDir(req.Version)
	}

	params := map[string]string{
		"version":         req.Version,
		"install_dir":     installDir,
		"host_id":         req.HostID,
		"cluster_id":      req.ClusterID,
		"install_mode":    string(req.InstallMode),
		"deployment_mode": string(req.DeploymentMode),
		"node_role":       string(req.NodeRole),
	}

	if req.Mirror != "" {
		params["mirror"] = string(req.Mirror)
	}

	if req.PackagePath != "" {
		params["package_path"] = req.PackagePath
	}

	// Add cluster configuration / 添加集群配置
	if len(req.MasterAddresses) > 0 {
		params["master_addresses"] = strings.Join(req.MasterAddresses, ",")
	}
	if len(req.WorkerAddresses) > 0 {
		params["worker_addresses"] = strings.Join(req.WorkerAddresses, ",")
	}
	if req.ClusterPort > 0 {
		params["cluster_port"] = fmt.Sprintf("%d", req.ClusterPort)
	}
	if req.WorkerPort > 0 {
		params["worker_port"] = fmt.Sprintf("%d", req.WorkerPort)
	}
	if req.HTTPPort > 0 {
		params["http_port"] = fmt.Sprintf("%d", req.HTTPPort)
	}
	if req.JavaProxyPort > 0 {
		params["java_proxy_port"] = fmt.Sprintf("%d", req.JavaProxyPort)
	}
	if req.EnableHTTP != nil {
		params["enable_http"] = strconv.FormatBool(*req.EnableHTTP)
	}
	if req.DynamicSlot != nil {
		params["dynamic_slot"] = strconv.FormatBool(*req.DynamicSlot)
	}
	if req.SlotNum != nil && *req.SlotNum > 0 {
		params["slot_num"] = fmt.Sprintf("%d", *req.SlotNum)
	}
	if req.SlotAllocationStrategy != "" {
		params["slot_allocation_strategy"] = string(req.SlotAllocationStrategy)
	}
	if req.JobScheduleStrategy != "" {
		params["job_schedule_strategy"] = string(req.JobScheduleStrategy)
	}
	if req.HistoryJobExpireMinutes != nil && *req.HistoryJobExpireMinutes > 0 {
		params["history_job_expire_minutes"] = fmt.Sprintf("%d", *req.HistoryJobExpireMinutes)
	}
	if req.ScheduledDeletionEnable != nil {
		params["scheduled_deletion_enable"] = strconv.FormatBool(*req.ScheduledDeletionEnable)
	}
	if req.JobLogMode != "" {
		params["job_log_mode"] = string(req.JobLogMode)
	}

	// Add JVM config / 添加 JVM 配置
	if req.JVM != nil {
		logger.InfoF(context.Background(), "[Installer] JVM config received: hybrid=%d, master=%d, worker=%d",
			req.JVM.HybridHeapSize, req.JVM.MasterHeapSize, req.JVM.WorkerHeapSize)
		params["jvm_hybrid_heap"] = fmt.Sprintf("%d", req.JVM.HybridHeapSize)
		params["jvm_master_heap"] = fmt.Sprintf("%d", req.JVM.MasterHeapSize)
		params["jvm_worker_heap"] = fmt.Sprintf("%d", req.JVM.WorkerHeapSize)
	} else {
		logger.InfoF(context.Background(), "[Installer] JVM config is nil, using defaults")
	}

	// Add checkpoint config / 添加检查点配置
	if req.Checkpoint != nil {
		params["checkpoint_storage_type"] = string(req.Checkpoint.StorageType)
		params["checkpoint_namespace"] = req.Checkpoint.Namespace
		if req.Checkpoint.HDFSNameNodeHost != "" {
			params["checkpoint_hdfs_host"] = req.Checkpoint.HDFSNameNodeHost
			params["checkpoint_hdfs_port"] = fmt.Sprintf("%d", req.Checkpoint.HDFSNameNodePort)
		}
		if req.Checkpoint.StorageEndpoint != "" {
			params["checkpoint_storage_endpoint"] = req.Checkpoint.StorageEndpoint
			params["checkpoint_storage_bucket"] = req.Checkpoint.StorageBucket
			params["checkpoint_storage_access_key"] = req.Checkpoint.StorageAccessKey
			params["checkpoint_storage_secret_key"] = req.Checkpoint.StorageSecretKey
		}
		// HDFS Kerberos config / HDFS Kerberos 配置
		if req.Checkpoint.KerberosPrincipal != "" {
			params["checkpoint_kerberos_principal"] = req.Checkpoint.KerberosPrincipal
		}
		if req.Checkpoint.KerberosKeytabFilePath != "" {
			params["checkpoint_kerberos_keytab_path"] = req.Checkpoint.KerberosKeytabFilePath
		}
		// HDFS HA config / HDFS HA 配置
		if req.Checkpoint.HDFSHAEnabled {
			params["checkpoint_hdfs_ha_enabled"] = "true"
			if req.Checkpoint.HDFSNameServices != "" {
				params["checkpoint_hdfs_name_services"] = req.Checkpoint.HDFSNameServices
			}
			if req.Checkpoint.HDFSHANamenodes != "" {
				params["checkpoint_hdfs_ha_namenodes"] = req.Checkpoint.HDFSHANamenodes
			}
			if req.Checkpoint.HDFSNamenodeRPCAddress1 != "" {
				params["checkpoint_hdfs_namenode_rpc_address_1"] = req.Checkpoint.HDFSNamenodeRPCAddress1
			}
			if req.Checkpoint.HDFSNamenodeRPCAddress2 != "" {
				params["checkpoint_hdfs_namenode_rpc_address_2"] = req.Checkpoint.HDFSNamenodeRPCAddress2
			}
			if req.Checkpoint.HDFSFailoverProxyProvider != "" {
				params["checkpoint_hdfs_failover_proxy_provider"] = req.Checkpoint.HDFSFailoverProxyProvider
			}
		}
	}

	// Add IMAP config / 添加 IMAP 配置
	if req.IMAP != nil {
		params["imap_storage_type"] = string(req.IMAP.StorageType)
		params["imap_namespace"] = req.IMAP.Namespace
		if req.IMAP.HDFSNameNodeHost != "" {
			params["imap_hdfs_host"] = req.IMAP.HDFSNameNodeHost
			params["imap_hdfs_port"] = fmt.Sprintf("%d", req.IMAP.HDFSNameNodePort)
		}
		if req.IMAP.StorageEndpoint != "" {
			params["imap_storage_endpoint"] = req.IMAP.StorageEndpoint
			params["imap_storage_bucket"] = req.IMAP.StorageBucket
			params["imap_storage_access_key"] = req.IMAP.StorageAccessKey
			params["imap_storage_secret_key"] = req.IMAP.StorageSecretKey
		}
		if req.IMAP.KerberosPrincipal != "" {
			params["imap_kerberos_principal"] = req.IMAP.KerberosPrincipal
		}
		if req.IMAP.KerberosKeytabFilePath != "" {
			params["imap_kerberos_keytab_path"] = req.IMAP.KerberosKeytabFilePath
		}
		if req.IMAP.HDFSHAEnabled {
			params["imap_hdfs_ha_enabled"] = "true"
			if req.IMAP.HDFSNameServices != "" {
				params["imap_hdfs_name_services"] = req.IMAP.HDFSNameServices
			}
			if req.IMAP.HDFSHANamenodes != "" {
				params["imap_hdfs_ha_namenodes"] = req.IMAP.HDFSHANamenodes
			}
			if req.IMAP.HDFSNamenodeRPCAddress1 != "" {
				params["imap_hdfs_namenode_rpc_address_1"] = req.IMAP.HDFSNamenodeRPCAddress1
			}
			if req.IMAP.HDFSNamenodeRPCAddress2 != "" {
				params["imap_hdfs_namenode_rpc_address_2"] = req.IMAP.HDFSNamenodeRPCAddress2
			}
			if req.IMAP.HDFSFailoverProxyProvider != "" {
				params["imap_hdfs_failover_proxy_provider"] = req.IMAP.HDFSFailoverProxyProvider
			}
		}
	}

	// Add connector config / 添加连接器配置
	if req.Connector != nil && req.Connector.InstallConnectors {
		params["install_connectors"] = "true"
		if len(req.Connector.SelectedPlugins) > 0 {
			params["selected_plugins"] = strings.Join(req.Connector.SelectedPlugins, ",")
		}
	}

	return params
}

// parseHostID parses host ID from string to uint.
// parseHostID 将主机 ID 从字符串解析为 uint。
func parseHostID(hostIDStr string) (uint, error) {
	if hostIDStr == "" {
		return 0, fmt.Errorf("host ID is empty / 主机 ID 为空")
	}
	var hostID uint
	_, err := fmt.Sscanf(hostIDStr, "%d", &hostID)
	if err != nil {
		return 0, fmt.Errorf("invalid host ID format: %s / 无效的主机 ID 格式: %s", hostIDStr, hostIDStr)
	}
	return hostID, nil
}

// parseClusterID parses cluster ID from string to uint.
// parseClusterID 将集群 ID 从字符串解析为 uint。
func parseClusterID(clusterIDStr string) (uint, error) {
	if clusterIDStr == "" {
		return 0, fmt.Errorf("cluster ID is empty / 集群 ID 为空")
	}
	var clusterID uint
	_, err := fmt.Sscanf(clusterIDStr, "%d", &clusterID)
	if err != nil {
		return 0, fmt.Errorf("invalid cluster ID format: %s / 无效的集群 ID 格式: %s", clusterIDStr, clusterIDStr)
	}
	return clusterID, nil
}

// ==================== Helper Functions 辅助函数 ====================

// createInitialSteps creates the initial step list.
// createInitialSteps 创建初始步骤列表。
func createInitialSteps() []StepInfo {
	return []StepInfo{
		{Step: InstallStepDownload, Name: "download", Description: "Download package / 下载安装包", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepVerify, Name: "verify", Description: "Verify checksum / 验证校验和", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepExtract, Name: "extract", Description: "Extract package / 解压安装包", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepConfigureCluster, Name: "configure_cluster", Description: "Configure cluster / 配置集群", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepConfigureCheckpoint, Name: "configure_checkpoint", Description: "Configure checkpoint / 配置检查点", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepConfigureIMAP, Name: "configure_imap", Description: "Configure IMAP / 配置 IMAP", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepConfigureJVM, Name: "configure_jvm", Description: "Configure JVM / 配置 JVM", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepInstallPlugins, Name: "install_plugins", Description: "Install plugins / 安装插件", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepRegisterCluster, Name: "register_cluster", Description: "Register to cluster / 注册到集群", Status: StepStatusPending, Retryable: true},
		{Step: InstallStepComplete, Name: "complete", Description: "Complete / 完成", Status: StepStatusPending, Retryable: false},
	}
}

// getDownloadURLs returns download URLs for a version.
// getDownloadURLs 返回某版本的下载 URL。
func getDownloadURLs(version string) map[MirrorSource]string {
	urls := make(map[MirrorSource]string)
	for mirror, baseURL := range MirrorURLs {
		urls[mirror] = fmt.Sprintf("%s/%s/%s", baseURL, version, packageFileName(version))
	}
	return urls
}

func packageFileName(version string) string {
	return fmt.Sprintf("apache-seatunnel-%s-bin.tar.gz", version)
}

func preparedPackageCacheKey(agentID, version, localPath string) string {
	return fmt.Sprintf("%s|%s|%s", agentID, version, filepath.Base(localPath))
}

func preparedPluginCacheKey(agentID, pluginName, version, installDir string, profileKeys []string, dependencyFingerprint string) string {
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s",
		agentID,
		pluginName,
		version,
		filepath.Clean(installDir),
		strings.Join(normalizeProfileKeys(profileKeys), ","),
		strings.TrimSpace(dependencyFingerprint),
	)
}

func (s *Service) prunePreparedAssetCacheLocked(now time.Time) {
	for key, entry := range s.preparedPackages {
		if now.Sub(entry.UpdatedAt) > preparedAssetCacheTTL {
			delete(s.preparedPackages, key)
		}
	}
	for key, updatedAt := range s.preparedPlugins {
		if now.Sub(updatedAt) > preparedAssetCacheTTL {
			delete(s.preparedPlugins, key)
		}
	}
}

func (s *Service) getPreparedPackageRemotePath(agentID, version, localPath string) (string, bool) {
	s.preparedAssetMu.Lock()
	defer s.preparedAssetMu.Unlock()

	now := time.Now()
	s.prunePreparedAssetCacheLocked(now)

	entry, ok := s.preparedPackages[preparedPackageCacheKey(agentID, version, localPath)]
	if !ok {
		return "", false
	}
	return entry.RemotePath, true
}

func (s *Service) rememberPreparedPackage(agentID, version, localPath, remotePath string) {
	s.preparedAssetMu.Lock()
	defer s.preparedAssetMu.Unlock()

	now := time.Now()
	s.prunePreparedAssetCacheLocked(now)
	s.preparedPackages[preparedPackageCacheKey(agentID, version, localPath)] = preparedPackageCacheEntry{
		RemotePath: remotePath,
		UpdatedAt:  now,
	}
}

func (s *Service) hasPreparedPlugin(agentID, pluginName, version, installDir string, profileKeys []string, dependencyFingerprint string) bool {
	if strings.TrimSpace(dependencyFingerprint) == "" {
		return false
	}

	s.preparedAssetMu.Lock()
	defer s.preparedAssetMu.Unlock()

	now := time.Now()
	s.prunePreparedAssetCacheLocked(now)

	_, ok := s.preparedPlugins[preparedPluginCacheKey(agentID, pluginName, version, installDir, profileKeys, dependencyFingerprint)]
	return ok
}

func (s *Service) rememberPreparedPlugin(agentID, pluginName, version, installDir string, profileKeys []string, dependencyFingerprint string) {
	if strings.TrimSpace(dependencyFingerprint) == "" {
		return
	}

	s.preparedAssetMu.Lock()
	defer s.preparedAssetMu.Unlock()

	now := time.Now()
	s.prunePreparedAssetCacheLocked(now)
	s.preparedPlugins[preparedPluginCacheKey(agentID, pluginName, version, installDir, profileKeys, dependencyFingerprint)] = now
}

func (s *Service) resolvePreparedPluginFingerprint(ctx context.Context, pluginName, version string, profileKeys []string) (string, error) {
	if s.pluginTransferer == nil {
		return "", nil
	}
	return s.pluginTransferer.GetPluginPreparationFingerprint(ctx, pluginName, version, profileKeys)
}

func normalizeProfileKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func normalizePathInDir(baseDir, candidatePath string) (string, error) {
	base := filepath.Clean(baseDir)
	target := filepath.Clean(candidatePath)

	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", ErrInvalidPackagePath
	}
	return target, nil
}

func (s *Service) resolveOfflinePackagePath(req *InstallationRequest) (string, error) {
	expectedFileName := packageFileName(req.Version)
	defaultPath := filepath.Join(s.packageDir, expectedFileName)

	if strings.TrimSpace(req.PackagePath) == "" {
		return defaultPath, nil
	}

	normalized, err := normalizePathInDir(s.packageDir, req.PackagePath)
	if err != nil {
		return "", ErrInvalidPackagePath
	}
	if !isSeaTunnelPackage(filepath.Base(normalized)) {
		return "", ErrInvalidPackageFile
	}
	if filepath.Base(normalized) != expectedFileName {
		return "", fmt.Errorf("%w: version mismatch", ErrInvalidPackagePath)
	}

	return normalized, nil
}

// isSeaTunnelPackage checks if a file name is a SeaTunnel package.
// isSeaTunnelPackage 检查文件名是否是 SeaTunnel 安装包。
func isSeaTunnelPackage(name string) bool {
	return len(name) > 20 && name[:17] == "apache-seatunnel-" && name[len(name)-11:] == "-bin.tar.gz"
}

// extractVersionFromFileName extracts version from package file name.
// extractVersionFromFileName 从安装包文件名中提取版本。
func extractVersionFromFileName(name string) string {
	// Format: apache-seatunnel-{version}-bin.tar.gz
	if len(name) < 29 {
		return ""
	}
	return name[17 : len(name)-11]
}

// calculateChecksum calculates SHA256 checksum of a file.
// calculateChecksum 计算文件的 SHA256 校验和。
func calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ==================== Package Transfer 安装包传输 ====================

// PackageTransferChunkSize is the size of each chunk for package transfer (1MB)
// PackageTransferChunkSize 是安装包传输每个块的大小（1MB）
const PackageTransferChunkSize = 1024 * 1024

// TransferPackageToAgent transfers a package to an Agent via gRPC
// TransferPackageToAgent 通过 gRPC 将安装包传输到 Agent
func (s *Service) TransferPackageToAgent(ctx context.Context, agentID string, version string, status *InstallationStatus) (remotePath string, err error) {
	localPath := filepath.Join(s.packageDir, packageFileName(version))
	return s.transferPackageFileToAgent(ctx, agentID, version, localPath, status)
}

func (s *Service) transferPackageFileToAgent(ctx context.Context, agentID string, version string, localPath string, status *InstallationStatus) (remotePath string, err error) {
	logger.InfoF(ctx, "[Installer] 开始传输安装包到 Agent / Start transferring package to Agent: agent=%s, version=%s", agentID, version)

	// Get file name / 获取文件名
	fileName := filepath.Base(localPath)

	// Check if package exists / 检查安装包是否存在
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("package not found: %s / 安装包未找到: %s", localPath, localPath)
	}
	totalSize := fileInfo.Size()

	// Calculate checksum / 计算校验和
	checksum, err := calculateChecksum(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to calculate checksum: %w / 计算校验和失败: %w", err, err)
	}

	// Open file / 打开文件
	file, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open package: %w / 打开安装包失败: %w", err, err)
	}
	defer file.Close()

	// Transfer in chunks / 分块传输
	buf := make([]byte, PackageTransferChunkSize)
	var offset int64
	var lastReceivedBytes int64

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		// Read chunk / 读取数据块
		n, readErr := file.Read(buf)
		if n == 0 && readErr == io.EOF {
			break
		}
		if readErr != nil && readErr != io.EOF {
			return "", fmt.Errorf("failed to read package: %w / 读取安装包失败: %w", readErr, readErr)
		}

		chunk := buf[:n]
		isLast := readErr == io.EOF || offset+int64(n) >= totalSize

		// Send chunk to Agent / 发送数据块到 Agent
		chunkChecksum := ""
		if isLast {
			chunkChecksum = checksum
		}

		success, receivedBytes, path, err := s.agentManager.SendTransferPackageCommand(
			ctx, agentID, version, fileName, chunk, offset, totalSize, isLast, chunkChecksum,
		)
		if err != nil {
			return "", fmt.Errorf("failed to send chunk: %w / 发送数据块失败: %w", err, err)
		}
		if !success {
			return "", fmt.Errorf("chunk transfer failed at offset %d / 数据块传输失败，偏移量 %d", offset, offset)
		}

		offset += int64(n)
		lastReceivedBytes = receivedBytes

		// Update status / 更新状态
		if status != nil {
			s.installMu.Lock()
			progress := int(float64(offset) / float64(totalSize) * 100)
			status.Message = fmt.Sprintf("Transferring package... %d%% / 正在传输安装包... %d%%", progress, progress)
			s.installMu.Unlock()
		}

		// If last chunk, get the remote path / 如果是最后一块，获取远程路径
		if isLast {
			remotePath = path
			break
		}
	}

	logger.InfoF(ctx, "[Installer] 安装包传输完成 / Package transfer completed: agent=%s, version=%s, received=%d, remote_path=%s",
		agentID, version, lastReceivedBytes, remotePath)

	return remotePath, nil
}
