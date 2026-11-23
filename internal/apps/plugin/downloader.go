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

// Package plugin provides SeaTunnel plugin marketplace management.
// plugin 包提供 SeaTunnel 插件市场管理功能。
package plugin

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Downloader errors / 下载器错误
var (
	ErrDownloadFailed     = errors.New("download failed / 下载失败")
	ErrChecksumMismatch   = errors.New("checksum mismatch / 校验和不匹配")
	ErrFileNotFound       = errors.New("file not found on server / 服务器上未找到文件")
	ErrDownloadCancelled  = errors.New("download cancelled / 下载已取消")
	ErrDownloadInProgress = errors.New("download already in progress / 下载正在进行中")

	// errTransportFailed 标记传输层失败（连接重置、超时等），换源重试有成功率。
	// errTransportFailed marks transport-layer failures (reset, timeout); switching mirrors may succeed.
	errTransportFailed = errors.New("transport failure")
)

// httpStatusError 携带 HTTP 状态码，兜底逻辑据此判断是否换源。
// httpStatusError carries the HTTP status so the fallback can decide whether to switch mirrors.
type httpStatusError struct {
	code int
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d", ErrDownloadFailed, e.code)
}

func (e httpStatusError) Unwrap() error { return ErrDownloadFailed }

// transportError 保留原始错误，同时暴露传输失败标记供兜底判断。
// transportError keeps the original error while exposing the transport-failure marker.
type transportError struct{ err error }

func (e transportError) Error() string { return fmt.Sprintf("failed to download: %s", e.err) }
func (e transportError) Unwrap() []error {
	return []error{errTransportFailed, e.err}
}

// shouldTryNextMirror 判断失败是否值得换下一个镜像源重试。
// shouldTryNextMirror reports whether the failure is worth retrying on the next mirror.
func shouldTryNextMirror(err error) bool {
	if errors.Is(err, ErrFileNotFound) {
		return true
	}
	var hs httpStatusError
	if errors.As(err, &hs) {
		// 限流与服务端错误属临时性，换源可能成功；其余 4xx 视为永久错误。
		// Rate limiting and server errors are transient; other 4xx are permanent.
		return hs.code == http.StatusTooManyRequests || hs.code >= http.StatusInternalServerError
	}
	return errors.Is(err, errTransportFailed)
}

// DownloadProgress represents the progress of a download task.
// DownloadProgress 表示下载任务的进度。
type DownloadProgress struct {
	PluginName          string     `json:"plugin_name"`                     // 插件名称 / Plugin name
	Version             string     `json:"version"`                         // 版本号 / Version
	Status              string     `json:"status"`                          // 状态: pending/downloading/completed/failed / Status
	Progress            int        `json:"progress"`                        // 进度 (0-100) / Progress
	CurrentStep         string     `json:"current_step"`                    // 当前步骤 / Current step
	CurrentArtifact     string     `json:"current_artifact,omitempty"`      // 当前下载条目 / Current downloading artifact
	CurrentArtifactKind string     `json:"current_artifact_kind,omitempty"` // connector / dependency
	DownloadedBytes     int64      `json:"downloaded_bytes"`                // 已下载字节 / Downloaded bytes
	TotalBytes          int64      `json:"total_bytes"`                     // 总字节 / Total bytes
	Speed               int64      `json:"speed"`                           // 下载速度 (bytes/s) / Download speed
	Message             string     `json:"message,omitempty"`               // 消息 / Message
	Error               string     `json:"error,omitempty"`                 // 错误信息 / Error message
	StartTime           time.Time  `json:"start_time"`                      // 开始时间 / Start time
	EndTime             *time.Time `json:"end_time,omitempty"`              // 结束时间 / End time
	ConnectorCount      int        `json:"connector_count,omitempty"`       // 连接器总数 / Total connectors
	ConnectorCompleted  int        `json:"connector_completed,omitempty"`   // 已完成连接器 / Completed connectors
	DependencyCount     int        `json:"dependency_count,omitempty"`      // 依赖总数 / Total dependencies
	DependencyCompleted int        `json:"dependency_completed,omitempty"`  // 已完成依赖 / Completed dependencies
	SelectedProfileKeys []string   `json:"selected_profile_keys,omitempty"` // 选中的画像 / Selected profiles
}

// ProgressCallback is a callback function for reporting download progress.
// ProgressCallback 是用于报告下载进度的回调函数。
type ProgressCallback func(progress *DownloadProgress)

// Downloader handles plugin file downloads from Maven repositories.
// Downloader 处理从 Maven 仓库下载插件文件。
type Downloader struct {
	// pluginsDir is the base directory for storing downloaded plugins
	// pluginsDir 是存储下载插件的基础目录
	pluginsDir string

	// httpClient is the HTTP client for downloading files
	// httpClient 是用于下载文件的 HTTP 客户端
	httpClient *http.Client

	// activeDownloads tracks currently active downloads
	// activeDownloads 跟踪当前活动的下载
	activeDownloads map[string]*DownloadProgress
	downloadsMu     sync.RWMutex

	// cancelFuncs stores cancel functions for active downloads
	// cancelFuncs 存储活动下载的取消函数
	cancelFuncs map[string]context.CancelFunc
	cancelMu    sync.Mutex
}

type localPluginMetadata struct {
	Name                string             `json:"name"`
	Version             string             `json:"version"`
	ArtifactID          string             `json:"artifact_id"`
	SelectedProfileKeys []string           `json:"selected_profile_keys,omitempty"`
	AttachedConnectors  []string           `json:"attached_connectors,omitempty"`
	Dependencies        []PluginDependency `json:"dependencies,omitempty"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

var connectorFilenamePattern = regexp.MustCompile(`^(connector-.+)-(\d+\.\d+\.\d+(?:[-A-Za-z0-9._]+)?)\.jar$`)

// NewDownloader creates a new Downloader instance.
// NewDownloader 创建一个新的 Downloader 实例。
func NewDownloader(pluginsDir string) *Downloader {
	return &Downloader{
		pluginsDir: pluginsDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute, // Long timeout for large files / 大文件的长超时
		},
		activeDownloads: make(map[string]*DownloadProgress),
		cancelFuncs:     make(map[string]context.CancelFunc),
	}
}

// GetPluginsDir returns the plugins directory path.
// GetPluginsDir 返回插件目录路径。
func (d *Downloader) GetPluginsDir() string {
	return d.pluginsDir
}

// GetConnectorPath returns the path for a connector jar file.
// GetConnectorPath 返回连接器 jar 文件的路径。
// Path format: plugins_dir/${version}/connectors/${artifactId}-${version}.jar
// The artifactID should be provided directly from plugin.ArtifactID (e.g., connector-cdc-mysql)
// artifactID 应该直接从 plugin.ArtifactID 提供（如 connector-cdc-mysql）
func (d *Downloader) GetConnectorPath(artifactID, version string) string {
	return filepath.Join(d.pluginsDir, version, "connectors", fmt.Sprintf("%s-%s.jar", artifactID, version))
}

// GetConnectorPathByName returns the path for a connector jar file by plugin name.
// GetConnectorPathByName 通过插件名称返回连接器 jar 文件的路径。
// This is a convenience method that looks up the artifact ID from the plugin name.
// 这是一个便捷方法，从插件名称查找 artifact ID。
// Deprecated: Use GetConnectorPath with artifact_id directly when possible.
// 已弃用：尽可能直接使用 GetConnectorPath 和 artifact_id。
func (d *Downloader) GetConnectorPathByName(name, version string) string {
	artifactID := getArtifactIDForPath(name)
	return d.GetConnectorPath(artifactID, version)
}

// pluginArtifactMappingsForPath contains all special plugin name to artifact ID mappings.
// pluginArtifactMappingsForPath 包含所有特殊的插件名称到 artifact ID 的映射。
// This mapping is based on SeaTunnel's Maven repository structure.
// 此映射基于 SeaTunnel 的 Maven 仓库结构。
// NOTE: This must be kept in sync with service.go's pluginArtifactMappings
// 注意：此映射必须与 service.go 的 pluginArtifactMappings 保持同步 TODO 看看后面有没有更方便的页面抓取
var pluginArtifactMappingsForPath = map[string]string{
	// CDC connectors / CDC 连接器
	"mysql-cdc":     "connector-cdc-mysql",
	"postgres-cdc":  "connector-cdc-postgres",
	"sqlserver-cdc": "connector-cdc-sqlserver",
	"oracle-cdc":    "connector-cdc-oracle",
	"mongodb-cdc":   "connector-cdc-mongodb",
	"tidb-cdc":      "connector-cdc-tidb",
	"db2-cdc":       "connector-cdc-db2",
	"opengauss-cdc": "connector-cdc-opengauss",
	// 3.1.0 起新增 / Added in 3.1.0
	"oceanbase-cdc": "connector-cdc-oceanbase",

	// File connectors / 文件连接器
	"localfile": "connector-file-local",
	"hdfsfile":  "connector-file-hadoop",
	"s3file":    "connector-file-s3",
	"ossfile":   "connector-file-oss",
	"ftpfile":   "connector-file-ftp",
	"sftpfile":  "connector-file-sftp",
	"cosfile":   "connector-file-cos",
	"obsfile":   "connector-file-obs",
	// 3.1.0 起新增 / Added in 3.1.0
	"smbfile": "connector-file-smb",

	// HTTP-based connectors / 基于 HTTP 的连接器
	"http":      "connector-http-base",
	"feishu":    "connector-http-feishu",
	"github":    "connector-http-github",
	"gitlab":    "connector-http-gitlab",
	"jira":      "connector-http-jira",
	"klaviyo":   "connector-http-klaviyo",
	"lemlist":   "connector-http-lemlist",
	"myhours":   "connector-http-myhours",
	"notion":    "connector-http-notion",
	"onesignal": "connector-http-onesignal",
	"persistiq": "connector-http-persistiq",
	"wechat":    "connector-http-wechat",
	// 3.1.0 起新增 / Added in 3.1.0
	"asana":    "connector-http-asana",
	"intercom": "connector-http-intercom",
	"paypal":   "connector-http-paypal",
	"splunk":   "connector-http-splunk",
	"typeform": "connector-http-typeform",

	// JDBC connector and JDBC-based databases / JDBC 连接器和基于 JDBC 的数据库
	// All these databases use connector-jdbc with their respective drivers
	// 所有这些数据库都使用 connector-jdbc 配合各自的驱动
	"jdbc":       "connector-jdbc",
	"mysql":      "connector-jdbc", // Driver: com.mysql.cj.jdbc.Driver
	"postgresql": "connector-jdbc", // Driver: org.postgresql.Driver
	"dm":         "connector-jdbc", // Driver: dm.jdbc.driver.DmDriver (达梦数据库)
	"phoenix":    "connector-jdbc", // Driver: org.apache.phoenix.queryserver.client.Driver
	"sqlserver":  "connector-jdbc", // Driver: com.microsoft.sqlserver.jdbc.SQLServerDriver
	"oracle":     "connector-jdbc", // Driver: oracle.jdbc.OracleDriver
	"sqlite":     "connector-jdbc", // Driver: org.sqlite.JDBC
	"gbase8a":    "connector-jdbc", // Driver: com.gbase.jdbc.Driver
	"db2":        "connector-jdbc", // Driver: com.ibm.db2.jcc.DB2Driver
	"tablestore": "connector-jdbc", // Driver: com.alicloud.openservices.tablestore.jdbc.OTSDriver
	"saphana":    "connector-jdbc", // Driver: com.sap.db.jdbc.Driver
	"teradata":   "connector-jdbc", // Driver: com.teradata.jdbc.TeraDriver
	"snowflake":  "connector-jdbc", // Driver: net.snowflake.client.jdbc.SnowflakeDriver
	"redshift":   "connector-jdbc", // Driver: com.amazon.redshift.jdbc42.Driver
	"vertica":    "connector-jdbc", // Driver: com.vertica.jdbc.Driver
	"kingbase":   "connector-jdbc", // Driver: com.kingbase8.Driver (人大金仓)
	"oceanbase":  "connector-jdbc", // Driver: com.oceanbase.jdbc.Driver
	"xugu":       "connector-jdbc", // Driver: com.xugu.cloudjdbc.Driver (虚谷数据库)
	"iris":       "connector-jdbc", // Driver: com.intersystems.jdbc.IRISDriver
	"opengauss":  "connector-jdbc", // Driver: org.opengauss.Driver
	"highgo":     "connector-jdbc", // Driver: com.highgo.jdbc.Driver (瀚高数据库)
	"presto":     "connector-jdbc", // Driver: com.facebook.presto.jdbc.PrestoDriver
	"trino":      "connector-jdbc", // Driver: io.trino.jdbc.TrinoDriver

	// 3.1.0 起新增的消息与分析类连接器：名字与 connector- 前缀推导结果不一致，需显式映射。
	// Messaging/analytics connectors added in 3.1.0 whose names do not match the
	// connector-<name> default derivation and need explicit mapping.
	"azureeventhubs":   "connector-azure-event-hubs",
	"googleanalytics4": "connector-google-analytics4",
}

// getArtifactIDForPath returns the correct Maven artifact ID for a plugin name.
// getArtifactIDForPath 返回插件名称对应的正确 Maven artifact ID。
// This is used as a fallback when artifact_id is not available.
// 当 artifact_id 不可用时作为备用。
func getArtifactIDForPath(name string) string {
	// Check special mappings first / 首先检查特殊映射
	if artifactID, ok := pluginArtifactMappingsForPath[name]; ok {
		return artifactID
	}

	// Default: connector-${name} / 默认：connector-${name}
	return fmt.Sprintf("connector-%s", name)
}

// pluginNameForArtifact 把 Maven artifact ID 反查成控制面插件名（connector-cdc-mysql → mysql-cdc）。
// pluginNameForArtifact reverses a Maven artifact ID to the Control Plane plugin name (connector-cdc-mysql → mysql-cdc).
func pluginNameForArtifact(artifactID string) string {
	artifactID = strings.TrimSpace(artifactID)
	if artifactID == "" {
		return ""
	}
	matches := make([]string, 0, 1)
	for name, mapped := range pluginArtifactMappingsForPath {
		if mapped == artifactID {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return matches[0]
	}
	canonical := strings.TrimPrefix(artifactID, "connector-")
	for _, name := range matches {
		if name == canonical {
			return name
		}
	}
	if len(matches) > 0 {
		sort.Strings(matches)
		return matches[0]
	}
	return canonical
}

// pluginNameFromConnectorList 把节点 list_plugins 返回的短名（文件名里 connector- 之后的一段）还原成控制面插件名。
// pluginNameFromConnectorList turns the short name from node list_plugins (the segment after connector- in the filename) into the Control Plane plugin name.
func pluginNameFromConnectorList(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if _, ok := pluginArtifactMappingsForPath[name]; ok {
		return name
	}
	return pluginNameForArtifact("connector-" + name)
}

// GetDependencyPath returns the path for a dependency file under the requested target dir.
// GetDependencyPath 返回指定 target_dir 下的依赖文件路径。
func (d *Downloader) GetDependencyPath(artifactID, version, pluginVersion, targetDir string) string {
	normalized, err := normalizePluginTargetDir(targetDir)
	if err != nil {
		normalized = "lib"
	}
	return filepath.Join(d.pluginsDir, pluginVersion, filepath.FromSlash(normalized), fmt.Sprintf("%s-%s.jar", artifactID, version))
}

// GetLibPath returns the legacy lib directory path for a dependency file.
// GetLibPath 返回依赖库文件在传统 lib 目录下的路径。
func (d *Downloader) GetLibPath(artifactID, version, pluginVersion string) string {
	return d.GetDependencyPath(artifactID, version, pluginVersion, "lib")
}

func (d *Downloader) getMetadataPath(pluginName, version string) string {
	return filepath.Join(d.pluginsDir, version, "metadata", fmt.Sprintf("%s.json", pluginName))
}

func (d *Downloader) writePluginMetadata(plugin *Plugin, selectedProfileKeys []string) error {
	if plugin == nil {
		return nil
	}
	metadataPath := d.getMetadataPath(plugin.Name, plugin.Version)
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0755); err != nil {
		return err
	}
	attachedConnectors := make([]string, 0)
	dependencies := make([]PluginDependency, 0)
	for _, dep := range plugin.Dependencies {
		if strings.TrimSpace(dep.TargetDir) == "connectors" {
			attachedConnectors = append(attachedConnectors, dep.ArtifactID)
			continue
		}
		dependencies = append(dependencies, dep)
	}
	payload := localPluginMetadata{
		Name:                plugin.Name,
		Version:             plugin.Version,
		ArtifactID:          plugin.ArtifactID,
		SelectedProfileKeys: append([]string(nil), selectedProfileKeys...),
		AttachedConnectors:  attachedConnectors,
		Dependencies:        dependencies,
		UpdatedAt:           time.Now(),
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metadataPath, content, 0644)
}

func (d *Downloader) readPluginMetadata(pluginName, version string) (*localPluginMetadata, error) {
	content, err := os.ReadFile(d.getMetadataPath(pluginName, version))
	if err != nil {
		return nil, err
	}
	var payload localPluginMetadata
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// IsConnectorDownloaded checks if a connector is already downloaded.
// IsConnectorDownloaded 检查连接器是否已下载。
// Note: Uses GetConnectorPathByName which maps plugin name to artifact ID.
// 注意：使用 GetConnectorPathByName 将插件名称映射到 artifact ID。
func (d *Downloader) IsConnectorDownloaded(name, version string) bool {
	path := d.GetConnectorPathByName(name, version)
	_, err := os.Stat(path)
	return err == nil
}

// IsConnectorDownloadedByArtifactID checks if a connector is already downloaded using artifact ID.
// IsConnectorDownloadedByArtifactID 使用 artifact ID 检查连接器是否已下载。
func (d *Downloader) IsConnectorDownloadedByArtifactID(artifactID, version string) bool {
	path := d.GetConnectorPath(artifactID, version)
	_, err := os.Stat(path)
	return err == nil
}

// ReadPluginFile reads the plugin file content from local storage.
// ReadPluginFile 从本地存储读取插件文件内容。
// Note: Uses GetConnectorPathByName which maps plugin name to artifact ID.
// 注意：使用 GetConnectorPathByName 将插件名称映射到 artifact ID。
func (d *Downloader) ReadPluginFile(name, version string) ([]byte, error) {
	path := d.GetConnectorPathByName(name, version)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin file %s: %w / 读取插件文件 %s 失败: %w", path, err, path, err)
	}
	return data, nil
}

// ReadPluginFileByArtifactID reads the plugin file content using artifact ID.
// ReadPluginFileByArtifactID 使用 artifact ID 读取插件文件内容。
func (d *Downloader) ReadPluginFileByArtifactID(artifactID, version string) ([]byte, error) {
	path := d.GetConnectorPath(artifactID, version)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin file %s: %w / 读取插件文件 %s 失败: %w", path, err, path, err)
	}
	return data, nil
}

// IsDependencyDownloaded checks if a dependency is already downloaded.
// IsDependencyDownloaded 检查依赖是否已下载。
func (d *Downloader) IsDependencyDownloaded(artifactID, depVersion, pluginVersion, targetDir string) bool {
	path := d.GetDependencyPath(artifactID, depVersion, pluginVersion, targetDir)
	_, err := os.Stat(path)
	return err == nil
}

// GetDownloadProgress returns the current download progress for a plugin.
// GetDownloadProgress 返回插件的当前下载进度。
func (d *Downloader) GetDownloadProgress(name, version string) *DownloadProgress {
	key := fmt.Sprintf("%s:%s", name, version)
	d.downloadsMu.RLock()
	defer d.downloadsMu.RUnlock()
	return d.activeDownloads[key]
}

// ListActiveDownloads returns all active download tasks.
// ListActiveDownloads 返回所有活动的下载任务。
func (d *Downloader) ListActiveDownloads() []*DownloadProgress {
	d.downloadsMu.RLock()
	defer d.downloadsMu.RUnlock()

	result := make([]*DownloadProgress, 0, len(d.activeDownloads))
	for _, p := range d.activeDownloads {
		result = append(result, p)
	}
	return result
}

// UpsertActiveDownload stores or updates an active download progress entry.
// UpsertActiveDownload 存储或更新活动下载进度。
func (d *Downloader) UpsertActiveDownload(progress *DownloadProgress) {
	if progress == nil {
		return
	}
	key := fmt.Sprintf("%s:%s", progress.PluginName, progress.Version)
	d.downloadsMu.Lock()
	d.activeDownloads[key] = progress
	d.downloadsMu.Unlock()
}

// ClearActiveDownload removes an active download progress entry.
// ClearActiveDownload 删除活动下载进度。
func (d *Downloader) ClearActiveDownload(name, version string) {
	key := fmt.Sprintf("%s:%s", name, version)
	d.downloadsMu.Lock()
	delete(d.activeDownloads, key)
	d.downloadsMu.Unlock()
}

// CancelDownload cancels an active download.
// CancelDownload 取消活动的下载。
func (d *Downloader) CancelDownload(name, version string) error {
	key := fmt.Sprintf("%s:%s", name, version)

	d.cancelMu.Lock()
	cancel, exists := d.cancelFuncs[key]
	d.cancelMu.Unlock()

	if !exists {
		return fmt.Errorf("no active download for %s:%s", name, version)
	}

	cancel()
	return nil
}

// DownloadConnector downloads a connector jar from Maven repository.
// DownloadConnector 从 Maven 仓库下载连接器 jar。
// Downloads to: plugins_dir/${version}/connectors/connector-${name}-${version}.jar
func (d *Downloader) DownloadConnector(ctx context.Context, plugin *Plugin, mirror MirrorSource, callback ProgressCallback) error {
	key := fmt.Sprintf("%s:%s", plugin.Name, plugin.Version)

	// Check if already downloading / 检查是否正在下载
	// 终态（failed/completed）条目不拦截重试，仅在进行中时报冲突。
	// Terminal entries (failed/completed) do not block retries; only in-flight ones conflict.
	d.downloadsMu.RLock()
	if p, exists := d.activeDownloads[key]; exists && p.Status == "downloading" {
		d.downloadsMu.RUnlock()
		return ErrDownloadInProgress
	}
	d.downloadsMu.RUnlock()

	// Create cancellable context / 创建可取消的上下文
	downloadCtx, cancel := context.WithCancel(ctx)

	d.cancelMu.Lock()
	d.cancelFuncs[key] = cancel
	d.cancelMu.Unlock()

	defer func() {
		d.cancelMu.Lock()
		delete(d.cancelFuncs, key)
		d.cancelMu.Unlock()
	}()

	// Ensure artifact_id is set / 确保 artifact_id 已设置
	artifactID := plugin.ArtifactID
	if artifactID == "" {
		// Fallback to mapping if artifact_id is not set / 如果 artifact_id 未设置则使用映射
		artifactID = getArtifactIDForPath(plugin.Name)
		fmt.Printf("[Download] Warning: plugin.ArtifactID is empty for %s, using fallback: %s\n", plugin.Name, artifactID)
	}

	// Initialize progress / 初始化进度
	progress := &DownloadProgress{
		PluginName:          plugin.Name,
		Version:             plugin.Version,
		Status:              "downloading",
		Progress:            0,
		CurrentStep:         "Downloading connector / 下载连接器",
		CurrentArtifact:     artifactID,
		CurrentArtifactKind: "connector",
		StartTime:           time.Now(),
		ConnectorCount:      1,
		ConnectorCompleted:  0,
		DependencyCount:     len(plugin.Dependencies),
		DependencyCompleted: 0,
	}

	d.downloadsMu.Lock()
	d.activeDownloads[key] = progress
	d.downloadsMu.Unlock()

	defer func() {
		d.downloadsMu.Lock()
		// 失败态保留，供状态接口返回 failed 与错误信息；成功态由磁盘文件判定。
		// Keep failed entries so the status API can report the error; success is derived from files on disk.
		if progress.Status != "failed" {
			delete(d.activeDownloads, key)
		}
		d.downloadsMu.Unlock()
	}()

	// Build Maven URL / 构建 Maven URL
	baseURL := MirrorURLs[mirror]
	if baseURL == "" {
		baseURL = MirrorURLs[MirrorSourceApache]
	}

	// Maven path: groupId/artifactId/version/artifactId-version.jar
	// Example: org/apache/seatunnel/connector-jdbc/2.3.12/connector-jdbc-2.3.12.jar
	groupPath := strings.ReplaceAll(plugin.GroupID, ".", "/")

	jarName := fmt.Sprintf("%s-%s.jar", artifactID, plugin.Version)
	urls := d.buildArtifactURLs(baseURL, groupPath, artifactID, plugin.Version, jarName, mirror)
	fmt.Printf("[Download] URLs: %v\n", urls)

	// Create target directory / 创建目标目录
	// Use artifact_id directly for file path / 直接使用 artifact_id 作为文件路径
	targetPath := d.GetConnectorPath(artifactID, plugin.Version)
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		progress.Status = "failed"
		progress.Error = err.Error()
		if callback != nil {
			callback(progress)
		}
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download the jar file / 下载 jar 文件
	_, sha1URL, err := d.downloadArtifactWithFallback(downloadCtx, urls, targetPath, progress, callback)
	if err != nil {
		progress.Status = "failed"
		progress.Error = err.Error()
		now := time.Now()
		progress.EndTime = &now
		if callback != nil {
			callback(progress)
		}
		return err
	}

	// Verify checksum / 验证校验和
	progress.CurrentStep = "Verifying checksum / 验证校验和"
	progress.CurrentArtifact = artifactID
	progress.CurrentArtifactKind = "connector"
	if callback != nil {
		callback(progress)
	}

	if err := d.verifyChecksum(downloadCtx, targetPath, sha1URL); err != nil {
		// Remove the downloaded file if checksum fails / 如果校验失败则删除下载的文件
		os.Remove(targetPath)
		progress.Status = "failed"
		progress.Error = err.Error()
		now := time.Now()
		progress.EndTime = &now
		if callback != nil {
			callback(progress)
		}
		return err
	}

	// Mark as completed / 标记为完成
	progress.Status = "completed"
	progress.Progress = 100
	progress.CurrentStep = "Completed / 完成"
	progress.CurrentArtifact = artifactID
	progress.CurrentArtifactKind = "connector"
	progress.ConnectorCompleted = 1
	now := time.Now()
	progress.EndTime = &now
	if callback != nil {
		callback(progress)
	}

	return nil
}

// DownloadDependencies downloads all dependencies for a plugin.
// DownloadDependencies 下载插件的所有依赖。
// Downloads to: plugins_dir/${version}/lib/
func (d *Downloader) DownloadDependencies(ctx context.Context, plugin *Plugin, mirror MirrorSource, callback ProgressCallback) error {
	if len(plugin.Dependencies) == 0 {
		return nil
	}

	baseURL := MirrorURLs[mirror]
	if baseURL == "" {
		baseURL = MirrorURLs[MirrorSourceApache]
	}

	for i, dep := range plugin.Dependencies {
		select {
		case <-ctx.Done():
			return ErrDownloadCancelled
		default:
		}

		targetDir, err := normalizePluginTargetDir(dep.TargetDir)
		if err != nil {
			return fmt.Errorf("invalid dependency target dir for %s: %w", dep.ArtifactID, err)
		}
		if err := os.MkdirAll(filepath.Join(d.pluginsDir, plugin.Version, filepath.FromSlash(targetDir)), 0755); err != nil {
			return fmt.Errorf("failed to create dependency directory: %w", err)
		}

		// Check if already downloaded / 检查是否已下载
		if d.IsDependencyDownloaded(dep.ArtifactID, dep.Version, plugin.Version, targetDir) {
			continue
		}

		targetPath := d.GetDependencyPath(dep.ArtifactID, dep.Version, plugin.Version, targetDir)

		// Create progress for this dependency / 为此依赖创建进度
		progress := &DownloadProgress{
			PluginName: plugin.Name,
			Version:    plugin.Version,
			Status:     "downloading",
			Progress:   (i * 100) / len(plugin.Dependencies),
			CurrentStep: fmt.Sprintf("Downloading dependency %d/%d: %s / 下载依赖 %d/%d: %s",
				i+1, len(plugin.Dependencies), dep.ArtifactID,
				i+1, len(plugin.Dependencies), dep.ArtifactID),
			StartTime:           time.Now(),
			ConnectorCount:      1,
			ConnectorCompleted:  1,
			DependencyCount:     len(plugin.Dependencies),
			DependencyCompleted: i,
			CurrentArtifact:     dep.ArtifactID,
			CurrentArtifactKind: "dependency",
		}

		if callback != nil {
			callback(progress)
		}

		switch dep.SourceType {
		case PluginDependencySourceUpload:
			if err := d.copyUploadedDependency(targetPath, dep.StoredPath); err != nil {
				progress.Message = fmt.Sprintf("Warning: failed to copy uploaded dependency %s: %v", dep.ArtifactID, err)
				if callback != nil {
					callback(progress)
				}
				continue
			}
		default:
			// Build Maven URL / 构建 Maven URL
			groupPath := strings.ReplaceAll(dep.GroupID, ".", "/")
			jarName := fmt.Sprintf("%s-%s.jar", dep.ArtifactID, dep.Version)
			urls := d.buildArtifactURLs(baseURL, groupPath, dep.ArtifactID, dep.Version, jarName, mirror)

			// Download the dependency / 下载依赖
			_, sha1URL, err := d.downloadArtifactWithFallback(ctx, urls, targetPath, progress, callback)
			if err != nil {
				// Log warning but continue with other dependencies / 记录警告但继续下载其他依赖
				progress.Message = fmt.Sprintf("Warning: failed to download %s: %v", dep.ArtifactID, err)
				if callback != nil {
					callback(progress)
				}
				continue
			}

			// Verify checksum (optional for dependencies) / 验证校验和（依赖可选）
			if err := d.verifyChecksum(ctx, targetPath, sha1URL); err != nil {
				// Log warning but don't fail / 记录警告但不失败
				progress.Message = fmt.Sprintf("Warning: checksum verification failed for %s", dep.ArtifactID)
				if callback != nil {
					callback(progress)
				}
			}
		}

		progress.DependencyCompleted = i + 1
		progress.Progress = ((i + 1) * 100) / len(plugin.Dependencies)
		progress.CurrentStep = fmt.Sprintf("Dependency %d/%d completed: %s / 依赖 %d/%d 已完成: %s",
			i+1, len(plugin.Dependencies), dep.ArtifactID,
			i+1, len(plugin.Dependencies), dep.ArtifactID)
		progress.CurrentArtifact = dep.ArtifactID
		progress.CurrentArtifactKind = "dependency"
		if callback != nil {
			callback(progress)
		}
	}

	return nil
}

func (d *Downloader) copyUploadedDependency(targetPath, storedPath string) error {
	storedPath = strings.TrimSpace(storedPath)
	if storedPath == "" {
		return fmt.Errorf("stored path is empty / 上传依赖的存储路径为空")
	}
	src, err := os.Open(storedPath)
	if err != nil {
		return err
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	return nil
}

func (d *Downloader) buildArtifactURLs(baseURL, groupPath, artifactID, version, jarName string, mirror MirrorSource) []string {
	primary := fmt.Sprintf("%s/%s/%s/%s/%s", baseURL, groupPath, artifactID, version, jarName)
	urls := []string{primary}
	// 其余镜像按固定顺序兜底：默认 Apache 源被限流/宕机时也有 aliyun、华为云可切。
	// Remaining mirrors are fallbacks in fixed order: a rate-limited or down Apache still has aliyun/huaweicloud to try.
	for _, m := range []MirrorSource{MirrorSourceApache, MirrorSourceAliyun, MirrorSourceHuaweiCloud} {
		if m == mirror {
			continue
		}
		base := MirrorURLs[m]
		if base == "" {
			continue
		}
		u := fmt.Sprintf("%s/%s/%s/%s/%s", base, groupPath, artifactID, version, jarName)
		if u != primary {
			urls = append(urls, u)
		}
	}
	return urls
}

func (d *Downloader) downloadArtifactWithFallback(ctx context.Context, urls []string, targetPath string, progress *DownloadProgress, callback ProgressCallback) (string, string, error) {
	var lastErr error
	for _, jarURL := range urls {
		if err := d.downloadFile(ctx, jarURL, targetPath, progress, callback); err != nil {
			lastErr = err
			if !shouldTryNextMirror(err) {
				return "", "", err
			}
			continue
		}
		return jarURL, jarURL + ".sha1", nil
	}
	if lastErr == nil {
		lastErr = ErrFileNotFound
	}
	return "", "", lastErr
}

// DownloadPlugin downloads a plugin and all its dependencies.
// DownloadPlugin 下载插件及其所有依赖。
func (d *Downloader) DownloadPlugin(ctx context.Context, plugin *Plugin, mirror MirrorSource, selectedProfileKeys []string, connectorReady, dependenciesReady bool, callback ProgressCallback) error {
	if plugin == nil {
		return fmt.Errorf("plugin is nil / 插件为空")
	}

	if !connectorReady {
		if err := d.DownloadConnector(ctx, plugin, mirror, callback); err != nil {
			return fmt.Errorf("failed to download connector: %w", err)
		}
	}

	if !dependenciesReady {
		progress := &DownloadProgress{
			PluginName:          plugin.Name,
			Version:             plugin.Version,
			Status:              "downloading",
			Progress:            50,
			CurrentStep:         "Preparing dependency download / 准备下载依赖",
			CurrentArtifactKind: "dependency",
			StartTime:           time.Now(),
			ConnectorCount:      1,
			ConnectorCompleted:  1,
			DependencyCount:     len(plugin.Dependencies),
			DependencyCompleted: 0,
			SelectedProfileKeys: append([]string(nil), selectedProfileKeys...),
		}
		d.UpsertActiveDownload(progress)
		defer func() {
			// 失败态保留，供状态接口返回 failed 与错误信息。
			// Keep failed entries so the status API can report the error.
			if progress.Status != "failed" {
				d.ClearActiveDownload(plugin.Name, plugin.Version)
			}
		}()
		dependencyCallback := func(depProgress *DownloadProgress) {
			if depProgress == nil {
				return
			}
			progress.Status = depProgress.Status
			progress.DownloadedBytes = depProgress.DownloadedBytes
			progress.TotalBytes = depProgress.TotalBytes
			progress.Speed = depProgress.Speed
			progress.Message = depProgress.Message
			progress.Error = depProgress.Error
			progress.DependencyCount = depProgress.DependencyCount
			progress.DependencyCompleted = depProgress.DependencyCompleted
			progress.CurrentStep = depProgress.CurrentStep
			progress.CurrentArtifact = depProgress.CurrentArtifact
			progress.CurrentArtifactKind = depProgress.CurrentArtifactKind
			progress.Progress = 50 + depProgress.Progress/2
			if depProgress.EndTime != nil {
				progress.EndTime = depProgress.EndTime
			}
			d.UpsertActiveDownload(progress)
			if callback != nil {
				callback(progress)
			}
		}
		if err := d.DownloadDependencies(ctx, plugin, mirror, dependencyCallback); err != nil {
			progress.Status = "failed"
			progress.Error = err.Error()
			now := time.Now()
			progress.EndTime = &now
			d.UpsertActiveDownload(progress)
			return fmt.Errorf("failed to download dependencies: %w", err)
		}
	}

	if err := d.writePluginMetadata(plugin, selectedProfileKeys); err != nil {
		return fmt.Errorf("failed to write plugin metadata: %w", err)
	}
	return nil
}

// downloadFile downloads a file from URL to the target path with progress reporting.
// downloadFile 从 URL 下载文件到目标路径，并报告进度。
func (d *Downloader) downloadFile(ctx context.Context, url, targetPath string, progress *DownloadProgress, callback ProgressCallback) error {
	// Create HTTP request / 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	applyMavenRequestHeaders(req)

	// Execute request / 执行请求
	resp, err := d.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return ErrDownloadCancelled
		}
		return transportError{err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrFileNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return httpStatusError{code: resp.StatusCode}
	}

	// Get content length / 获取内容长度
	progress.TotalBytes = resp.ContentLength

	// Create target file / 创建目标文件
	tmpPath := targetPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		file.Close()
		// Clean up temp file on error / 出错时清理临时文件
		if progress.Status != "completed" {
			os.Remove(tmpPath)
		}
	}()

	// Download with progress tracking / 带进度跟踪的下载
	buf := make([]byte, 32*1024) // 32KB buffer
	startTime := time.Now()
	lastReportTime := startTime

	for {
		select {
		case <-ctx.Done():
			return ErrDownloadCancelled
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write file: %w", writeErr)
			}

			progress.DownloadedBytes += int64(n)

			// Calculate progress and speed / 计算进度和速度
			if progress.TotalBytes > 0 {
				progress.Progress = int((progress.DownloadedBytes * 100) / progress.TotalBytes)
			}

			elapsed := time.Since(startTime).Seconds()
			if elapsed > 0 {
				progress.Speed = int64(float64(progress.DownloadedBytes) / elapsed)
			}

			// Report progress every 500ms / 每 500ms 报告一次进度
			if time.Since(lastReportTime) > 500*time.Millisecond && callback != nil {
				callback(progress)
				lastReportTime = time.Now()
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
	}

	// Close file before rename / 重命名前关闭文件
	file.Close()

	// Rename temp file to target / 将临时文件重命名为目标文件
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// verifyChecksum verifies the SHA1 checksum of a downloaded file.
// verifyChecksum 验证下载文件的 SHA1 校验和。
func (d *Downloader) verifyChecksum(ctx context.Context, filePath, sha1URL string) error {
	// Download SHA1 checksum / 下载 SHA1 校验和
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sha1URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create checksum request: %w", err)
	}
	applyMavenRequestHeaders(req)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		// If checksum file not available, skip verification / 如果校验和文件不可用，跳过验证
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If checksum file not available, skip verification / 如果校验和文件不可用，跳过验证
		return nil
	}

	// Read expected checksum / 读取预期的校验和
	checksumBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil // Skip verification on read error / 读取错误时跳过验证
	}

	expectedChecksum := strings.TrimSpace(string(checksumBytes))
	// Some checksum files contain additional info, extract just the hash / 某些校验和文件包含额外信息，只提取哈希值
	if idx := strings.Index(expectedChecksum, " "); idx > 0 {
		expectedChecksum = expectedChecksum[:idx]
	}

	// Calculate actual checksum / 计算实际校验和
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer file.Close()

	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedChecksum, actualChecksum)
	}

	return nil
}

// ListLocalPlugins returns a list of locally downloaded plugins.
// ListLocalPlugins 返回本地已下载的插件列表。
func (d *Downloader) ListLocalPlugins() ([]LocalPlugin, error) {
	var plugins []LocalPlugin
	metadataByVersion := make(map[string]map[string]*localPluginMetadata)
	attachedArtifactsByVersion := make(map[string]map[string]struct{})
	hiddenPluginsByVersion := make(map[string]map[string]struct{})

	// Walk through plugins directory / 遍历插件目录
	entries, err := os.ReadDir(d.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return plugins, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		version := entry.Name()
		hiddenPluginsByVersion[version] = loadHiddenPluginNamesForVersion(version)
		metadataDir := filepath.Join(d.pluginsDir, version, "metadata")
		if metadataEntries, metaErr := os.ReadDir(metadataDir); metaErr == nil {
			metadataByVersion[version] = make(map[string]*localPluginMetadata)
			attachedArtifactsByVersion[version] = make(map[string]struct{})
			for _, metaEntry := range metadataEntries {
				if metaEntry.IsDir() || !strings.HasSuffix(metaEntry.Name(), ".json") {
					continue
				}
				content, readErr := os.ReadFile(filepath.Join(metadataDir, metaEntry.Name()))
				if readErr != nil {
					continue
				}
				var payload localPluginMetadata
				if json.Unmarshal(content, &payload) != nil {
					continue
				}
				metadataByVersion[version][payload.Name] = &payload
				for _, artifactID := range payload.AttachedConnectors {
					if strings.TrimSpace(artifactID) != "" {
						attachedArtifactsByVersion[version][artifactID] = struct{}{}
					}
				}
			}
		}
		connectorsDir := filepath.Join(d.pluginsDir, version, "connectors")

		// List connector jars / 列出连接器 jar
		connectorEntries, err := os.ReadDir(connectorsDir)
		if err != nil {
			continue
		}

		for _, connEntry := range connectorEntries {
			if connEntry.IsDir() || !strings.HasSuffix(connEntry.Name(), ".jar") {
				continue
			}

			name, artifactID, embeddedVersion := parseConnectorFilename(connEntry.Name(), version)
			if embeddedVersion != "" && embeddedVersion != version {
				continue
			}

			info, _ := connEntry.Info()
			var size int64
			var modTime time.Time
			if info != nil {
				size = info.Size()
				modTime = info.ModTime()
			}

			// Determine category from plugin name / 从插件名称判断分类
			category := determinePluginCategory(name)

			plugins = append(plugins, LocalPlugin{
				Name:          name,
				ArtifactID:    artifactID,
				Version:       version,
				Category:      category,
				ConnectorPath: filepath.Join(connectorsDir, connEntry.Name()),
				Size:          size,
				DownloadedAt:  modTime,
			})
		}
	}

	filtered := make([]LocalPlugin, 0, len(plugins))
	for i := range plugins {
		var metadata *localPluginMetadata
		if items := metadataByVersion[plugins[i].Version]; items != nil {
			metadata = items[plugins[i].Name]
		}
		if metadata == nil {
			if attached := attachedArtifactsByVersion[plugins[i].Version]; attached != nil {
				if _, ok := attached[plugins[i].ArtifactID]; ok && shouldHideAutoAttachedLocalPlugin(plugins[i], hiddenPluginsByVersion[plugins[i].Version]) {
					continue
				}
			}
			filtered = append(filtered, plugins[i])
			continue
		}
		if strings.TrimSpace(metadata.ArtifactID) != "" {
			plugins[i].ArtifactID = metadata.ArtifactID
		}
		plugins[i].SelectedProfileKeys = append([]string(nil), metadata.SelectedProfileKeys...)
		plugins[i].AttachedConnectors = append([]string(nil), metadata.AttachedConnectors...)
		plugins[i].Dependencies = append([]PluginDependency(nil), metadata.Dependencies...)
		filtered = append(filtered, plugins[i])
	}

	return filtered, nil
}

func loadHiddenPluginNamesForVersion(version string) map[string]struct{} {
	hidden := make(map[string]struct{})
	seed, err := loadOfficialDependencySeed(version)
	if err != nil || seed == nil {
		return hidden
	}
	for _, name := range seed.HiddenPlugins {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		hidden[name] = struct{}{}
	}
	return hidden
}

func shouldHideAutoAttachedLocalPlugin(plugin LocalPlugin, hiddenPlugins map[string]struct{}) bool {
	if len(hiddenPlugins) == 0 {
		return false
	}
	_, ok := hiddenPlugins[strings.TrimSpace(plugin.Name)]
	return ok
}

// parsePluginNameFromFilename extracts the plugin name from a jar filename.
// parsePluginNameFromFilename 从 jar 文件名中提取插件名称。
// Returns plugin name, artifact ID and embedded version.
// 返回插件名称、artifact ID 和文件名内嵌版本。
func parseConnectorFilename(filename, fallbackVersion string) (string, string, string) {
	if matches := connectorFilenamePattern.FindStringSubmatch(filename); len(matches) == 3 {
		artifactID := matches[1]
		return strings.TrimPrefix(artifactID, "connector-"), artifactID, matches[2]
	}

	artifactID := strings.TrimSuffix(filename, fmt.Sprintf("-%s.jar", fallbackVersion))
	if strings.HasSuffix(artifactID, ".jar") {
		artifactID = strings.TrimSuffix(artifactID, ".jar")
	}
	name := artifactID
	if strings.HasPrefix(name, "connector-") {
		name = strings.TrimPrefix(name, "connector-")
	}
	return name, artifactID, ""
}

// determinePluginCategory determines the category of a plugin from its name.
// determinePluginCategory 从插件名称判断插件分类。
// Now all plugins are connectors.
// 现在所有插件都是连接器。
func determinePluginCategory(name string) PluginCategory {
	// All plugins fetched from Maven are connectors / 从 Maven 获取的所有插件都是连接器
	return PluginCategoryConnector
}

// DeleteLocalPlugin deletes a locally downloaded plugin.
// DeleteLocalPlugin 删除本地已下载的插件。
// Note: Uses GetConnectorPathByName which maps plugin name to artifact ID.
// 注意：使用 GetConnectorPathByName 将插件名称映射到 artifact ID。
func (d *Downloader) DeleteLocalPlugin(name, version string) error {
	connectorPath := d.GetConnectorPathByName(name, version)

	// Check if exists / 检查是否存在
	if _, err := os.Stat(connectorPath); os.IsNotExist(err) {
		return ErrFileNotFound
	}

	// Remove connector jar / 删除连接器 jar
	if err := os.Remove(connectorPath); err != nil {
		return fmt.Errorf("failed to delete connector: %w", err)
	}

	_ = os.Remove(d.getMetadataPath(name, version))

	return nil
}

// LocalPlugin represents a locally downloaded plugin.
// LocalPlugin 表示本地已下载的插件。
type LocalPlugin struct {
	Name                string             `json:"name"`                            // 插件名称 / Plugin name
	ArtifactID          string             `json:"artifact_id"`                     // Maven artifact ID / Maven artifact ID
	Version             string             `json:"version"`                         // 版本号 / Version
	Category            PluginCategory     `json:"category"`                        // 插件分类 / Plugin category
	ConnectorPath       string             `json:"connector_path"`                  // 连接器路径 / Connector path
	Size                int64              `json:"size"`                            // 文件大小 / File size
	DownloadedAt        time.Time          `json:"downloaded_at"`                   // 下载时间 / Downloaded at
	SelectedProfileKeys []string           `json:"selected_profile_keys,omitempty"` // 选中的画像 / Selected profiles
	AttachedConnectors  []string           `json:"attached_connectors,omitempty"`   // 自动附带的连接器 / Attached connectors
	Dependencies        []PluginDependency `json:"dependencies,omitempty"`          // 自动附带的依赖 / Auto attached dependencies
}
