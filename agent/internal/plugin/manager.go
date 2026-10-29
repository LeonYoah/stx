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

// Package plugin provides plugin management functionality for the Agent.
// plugin 包提供 Agent 的插件管理功能。
//
// The PluginManager is responsible for:
// PluginManager 负责：
// - Receiving plugin files from Control Plane / 从 Control Plane 接收插件文件
// - Installing plugins to SeaTunnel directories / 将插件安装到 SeaTunnel 目录
// - Uninstalling plugins / 卸载插件
// - Listing installed plugins / 列出已安装的插件
package plugin

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Common errors for plugin management
// 插件管理的常见错误
var (
	// ErrPluginNotFound indicates the plugin was not found
	// ErrPluginNotFound 表示未找到插件
	ErrPluginNotFound = errors.New("plugin not found / 插件未找到")

	// ErrPluginAlreadyExists indicates the plugin already exists
	// ErrPluginAlreadyExists 表示插件已存在
	ErrPluginAlreadyExists = errors.New("plugin already exists / 插件已存在")

	// ErrInvalidChecksum indicates checksum verification failed
	// ErrInvalidChecksum 表示校验和验证失败
	ErrInvalidChecksum = errors.New("checksum verification failed / 校验和验证失败")

	// ErrTransferIncomplete indicates file transfer is incomplete
	// ErrTransferIncomplete 表示文件传输不完整
	ErrTransferIncomplete = errors.New("file transfer incomplete / 文件传输不完整")

	// ErrInstallPathNotSet indicates SeaTunnel install path is not set
	// ErrInstallPathNotSet 表示 SeaTunnel 安装路径未设置
	ErrInstallPathNotSet = errors.New("SeaTunnel install path not set / SeaTunnel 安装路径未设置")

	// ErrDependencyInUse indicates the dependency is used by other plugins
	// ErrDependencyInUse 表示依赖被其他插件使用
	ErrDependencyInUse = errors.New("dependency is used by other plugins / 依赖被其他插件使用")

	// ErrInvalidPluginIdentifier indicates plugin metadata contains unsafe path characters
	// ErrInvalidPluginIdentifier 表示插件元数据包含不安全的路径字符
	ErrInvalidPluginIdentifier = errors.New("invalid plugin identifier / 无效的插件标识")
)

// InstalledPluginInfo represents information about an installed plugin.
// InstalledPluginInfo 表示已安装插件的信息。
type InstalledPluginInfo struct {
	Name          string    `json:"name"`           // 插件名称 / Plugin name
	Version       string    `json:"version"`        // 版本号 / Version
	ConnectorPath string    `json:"connector_path"` // 连接器路径 / Connector path
	Size          int64     `json:"size"`           // 文件大小 / File size
	InstalledAt   time.Time `json:"installed_at"`   // 安装时间 / Installed at
}

// TransferState represents the state of an ongoing file transfer.
// TransferState 表示正在进行的文件传输状态。
type TransferState struct {
	PluginName    string    // 插件名称 / Plugin name
	Version       string    // 版本号 / Version
	FileType      string    // 文件类型: connector, dependency / File type
	TargetDir     string    // 目标目录（相对 SeaTunnel_HOME）/ Target dir
	FileName      string    // 文件名 / File name
	TempPath      string    // 临时文件路径 / Temp file path
	ReceivedBytes int64     // 已接收字节 / Received bytes
	TotalSize     int64     // 总大小 / Total size
	Checksum      string    // 预期校验和 / Expected checksum
	StartTime     time.Time // 开始时间 / Start time
}

// Manager manages plugin installation and uninstallation for the Agent.
// Manager 管理 Agent 的插件安装和卸载。
type Manager struct {
	// seatunnelPath is the SeaTunnel installation directory
	// seatunnelPath 是 SeaTunnel 安装目录
	seatunnelPath string

	// tempDir is the temporary directory for receiving files
	// tempDir 是接收文件的临时目录
	tempDir string

	// transfers tracks ongoing file transfers
	// transfers 跟踪正在进行的文件传输
	transfers map[string]*TransferState
	mu        sync.RWMutex
}

// NewManager creates a new plugin Manager instance.
// NewManager 创建一个新的插件 Manager 实例。
func NewManager(seatunnelPath string) *Manager {
	tempDir := filepath.Join(os.TempDir(), "seatunnel-plugins")
	os.MkdirAll(tempDir, 0755)

	return &Manager{
		seatunnelPath: seatunnelPath,
		tempDir:       tempDir,
		transfers:     make(map[string]*TransferState),
	}
}

// SetSeaTunnelPath sets the SeaTunnel installation path.
// SetSeaTunnelPath 设置 SeaTunnel 安装路径。
func (m *Manager) SetSeaTunnelPath(path string) {
	m.seatunnelPath = path
}

// GetSeaTunnelPath returns the SeaTunnel installation path.
// GetSeaTunnelPath 返回 SeaTunnel 安装路径。
func (m *Manager) GetSeaTunnelPath() string {
	return m.seatunnelPath
}

// GetConnectorsDir returns the connectors directory path.
// GetConnectorsDir 返回连接器目录路径。
func (m *Manager) GetConnectorsDir() string {
	return filepath.Join(m.seatunnelPath, "connectors")
}

// GetLibDir returns the lib directory path.
// GetLibDir 返回 lib 目录路径。
func (m *Manager) GetLibDir() string {
	return filepath.Join(m.seatunnelPath, "lib")
}

// ReceivePluginChunk receives a chunk of plugin file data.
// ReceivePluginChunk 接收插件文件数据块。
// This method is called multiple times to receive a complete file.
// 此方法被多次调用以接收完整文件。
func (m *Manager) ReceivePluginChunk(pluginName, version, fileType, targetDir, fileName string, chunk []byte, offset, totalSize int64, isLast bool, checksum string) (int64, error) {
	key := fmt.Sprintf("%s:%s:%s:%s", pluginName, version, targetDir, fileName)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create transfer state / 获取或创建传输状态
	state, exists := m.transfers[key]
	if !exists {
		// Create temp file / 创建临时文件
		tempPath := filepath.Join(m.tempDir, fmt.Sprintf("%s_%s_%s.tmp", pluginName, version, fileName))
		state = &TransferState{
			PluginName:    pluginName,
			Version:       version,
			FileType:      fileType,
			TargetDir:     targetDir,
			FileName:      fileName,
			TempPath:      tempPath,
			ReceivedBytes: 0,
			TotalSize:     totalSize,
			StartTime:     time.Now(),
		}
		m.transfers[key] = state
	}

	// Open temp file for writing / 打开临时文件进行写入
	file, err := os.OpenFile(state.TempPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return state.ReceivedBytes, fmt.Errorf("failed to open temp file: %w", err)
	}
	defer file.Close()

	// Seek to offset / 定位到偏移量
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return state.ReceivedBytes, fmt.Errorf("failed to seek: %w", err)
	}

	// Write chunk / 写入数据块
	n, err := file.Write(chunk)
	if err != nil {
		return state.ReceivedBytes, fmt.Errorf("failed to write chunk: %w", err)
	}

	state.ReceivedBytes += int64(n)

	// If this is the last chunk, store checksum / 如果是最后一块，存储校验和
	if isLast && checksum != "" {
		state.Checksum = checksum
	}

	return state.ReceivedBytes, nil
}

// FinalizeTransfer completes a file transfer and moves it to the target location.
// FinalizeTransfer 完成文件传输并将其移动到目标位置。
func (m *Manager) FinalizeTransfer(pluginName, version, targetDir, fileName string) (string, error) {
	key := fmt.Sprintf("%s:%s:%s:%s", pluginName, version, targetDir, fileName)

	m.mu.Lock()
	state, exists := m.transfers[key]
	if !exists {
		m.mu.Unlock()
		return "", ErrTransferIncomplete
	}
	delete(m.transfers, key)
	m.mu.Unlock()

	// Verify file size / 验证文件大小
	info, err := os.Stat(state.TempPath)
	if err != nil {
		return "", fmt.Errorf("failed to stat temp file: %w", err)
	}

	if info.Size() != state.TotalSize {
		os.Remove(state.TempPath)
		return "", fmt.Errorf("%w: expected %d bytes, got %d", ErrTransferIncomplete, state.TotalSize, info.Size())
	}

	// Verify checksum if provided / 如果提供了校验和则验证
	if state.Checksum != "" {
		actualChecksum, err := calculateSHA1(state.TempPath)
		if err != nil {
			os.Remove(state.TempPath)
			return "", fmt.Errorf("failed to calculate checksum: %w", err)
		}

		if !strings.EqualFold(actualChecksum, state.Checksum) {
			os.Remove(state.TempPath)
			return "", fmt.Errorf("%w: expected %s, got %s", ErrInvalidChecksum, state.Checksum, actualChecksum)
		}
	}

	// Determine target directory based on file type / 根据文件类型确定目标目录
	var targetRoot string
	switch state.FileType {
	case "connector":
		targetRoot = m.GetConnectorsDir()
	default:
		resolved, err := m.resolveTargetDir(state.TargetDir)
		if err != nil {
			_ = os.Remove(state.TempPath)
			return "", err
		}
		targetRoot = resolved
	}

	// Create target directory if not exists / 如果目标目录不存在则创建
	if err := os.MkdirAll(targetRoot, 0755); err != nil {
		os.Remove(state.TempPath)
		return "", fmt.Errorf("failed to create target directory: %w", err)
	}

	// Move file to target location / 将文件移动到目标位置
	targetPath := filepath.Join(targetRoot, state.FileName)
	if err := os.Rename(state.TempPath, targetPath); err != nil {
		// If rename fails (cross-device), try copy / 如果重命名失败（跨设备），尝试复制
		if err := copyFile(state.TempPath, targetPath); err != nil {
			os.Remove(state.TempPath)
			return "", fmt.Errorf("failed to move file: %w", err)
		}
		os.Remove(state.TempPath)
	}

	return targetPath, nil
}

// InstallPlugin installs a plugin by moving received files to SeaTunnel directories.
// InstallPlugin 通过将接收的文件移动到 SeaTunnel 目录来安装插件。
// The connector jar goes to connectors/ and dependencies go to lib/.
// 连接器 jar 放到 connectors/，依赖放到 lib/。
func (m *Manager) InstallPlugin(pluginName, version, installPath string, dependencies []string) (string, []string, error) {
	if installPath != "" {
		m.seatunnelPath = installPath
	}

	if m.seatunnelPath == "" {
		return "", nil, ErrInstallPathNotSet
	}

	// Ensure directories exist / 确保目录存在
	connectorsDir := m.GetConnectorsDir()
	libDir := m.GetLibDir()

	if err := os.MkdirAll(connectorsDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create connectors directory: %w", err)
	}

	if err := os.MkdirAll(libDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create lib directory: %w", err)
	}

	// The connector file should already be in place from FinalizeTransfer
	// 连接器文件应该已经通过 FinalizeTransfer 放置到位
	// Try to find the connector file with different naming patterns
	// 尝试使用不同的命名模式查找连接器文件
	connectorPath := m.findConnectorFile(connectorsDir, pluginName, version)
	if connectorPath == "" {
		return "", nil, fmt.Errorf("connector file not found for plugin %s version %s in %s", pluginName, version, connectorsDir)
	}

	// Collect installed dependency paths / 收集已安装的依赖路径
	var libPaths []string
	for _, dep := range dependencies {
		depPath := filepath.Join(libDir, dep)
		if _, err := os.Stat(depPath); err == nil {
			libPaths = append(libPaths, depPath)
		}
	}

	return connectorPath, libPaths, nil
}

func (m *Manager) resolveTargetDir(targetDir string) (string, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		targetDir = "lib"
	}
	cleaned := filepath.Clean(targetDir)
	if cleaned == "." || cleaned == string(filepath.Separator) || strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid target_dir: %s", targetDir)
	}
	return filepath.Join(m.seatunnelPath, cleaned), nil
}

// UninstallPlugin removes a plugin from SeaTunnel directories.
// UninstallPlugin 从 SeaTunnel 目录中删除插件。
func (m *Manager) UninstallPlugin(pluginName, version, installPath string, removeDependencies bool) error {
	if installPath != "" {
		m.seatunnelPath = installPath
	}

	if m.seatunnelPath == "" {
		return ErrInstallPathNotSet
	}

	if err := validatePluginIdentifier("plugin name", pluginName); err != nil {
		return err
	}

	if err := validatePluginIdentifier("version", version); err != nil {
		return err
	}

	// Remove connector jar / 删除连接器 jar
	connectorFileName := fmt.Sprintf("connector-%s-%s.jar", pluginName, version)
	connectorPath := filepath.Join(m.GetConnectorsDir(), connectorFileName)

	if err := os.Remove(connectorPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove connector: %w", err)
	}

	// When removeDependencies is true, delete the plugin-private dependency directory
	// (e.g. plugins/connector-cdc-postgres holding pgjdbc). Shared lib/ jars and companion
	// connectors under connectors/ are left alone — seed profiles put per-plugin drivers
	// under plugins/<artifactID>/.
	// removeDependencies=true 时删除插件私有依赖目录（如 plugins/connector-cdc-postgres 下的 pgjdbc）；
	// 共享 lib/ 与 connectors/ 下的伴生连接器保留不动。
	if removeDependencies {
		if err := m.removePluginDependencyDir(pluginName); err != nil {
			return err
		}
	}

	return nil
}

// removePluginDependencyDir deletes plugins/<artifactID>/ for this plugin when present.
// removePluginDependencyDir 在存在时删除该插件的 plugins/<artifactID>/ 目录。
func (m *Manager) removePluginDependencyDir(pluginName string) error {
	artifactID := getArtifactID(pluginName)
	if err := validatePluginIdentifier("artifact id", artifactID); err != nil {
		return err
	}
	pluginsRoot := filepath.Clean(filepath.Join(m.seatunnelPath, "plugins"))
	depDir := filepath.Clean(filepath.Join(pluginsRoot, artifactID))
	if depDir == pluginsRoot || !strings.HasPrefix(depDir, pluginsRoot+string(filepath.Separator)) {
		return fmt.Errorf("refusing to remove dependency dir outside plugins/: %s", depDir)
	}
	if _, err := os.Stat(depDir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(depDir); err != nil {
		return fmt.Errorf("failed to remove plugin dependencies at %s: %w", depDir, err)
	}
	return nil
}

func validatePluginIdentifier(field, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s is empty", ErrInvalidPluginIdentifier, field)
	}

	if strings.ContainsAny(value, `/\\`) {
		return fmt.Errorf("%w: %s contains path separator", ErrInvalidPluginIdentifier, field)
	}

	if value == "." || value == ".." {
		return fmt.Errorf("%w: %s contains invalid path segment", ErrInvalidPluginIdentifier, field)
	}

	return nil
}

// pluginArtifactMappings contains all special plugin name to artifact ID mappings.
// pluginArtifactMappings 包含所有特殊的插件名称到 artifact ID 的映射。
// This mapping must match the Control Plane's pluginArtifactMappings.
// 此映射必须与 Control Plane 的 pluginArtifactMappings 匹配。
var pluginArtifactMappings = map[string]string{
	// CDC connectors / CDC 连接器
	"mysql-cdc":     "connector-cdc-mysql",
	"postgres-cdc":  "connector-cdc-postgres",
	"sqlserver-cdc": "connector-cdc-sqlserver",
	"oracle-cdc":    "connector-cdc-oracle",
	"mongodb-cdc":   "connector-cdc-mongodb",
	"tidb-cdc":      "connector-cdc-tidb",
	"db2-cdc":       "connector-cdc-db2",
	"opengauss-cdc": "connector-cdc-opengauss",

	// File connectors / 文件连接器
	"localfile": "connector-file-local",
	"hdfsfile":  "connector-file-hadoop",
	"s3file":    "connector-file-s3",
	"ossfile":   "connector-file-oss",
	"ftpfile":   "connector-file-ftp",
	"sftpfile":  "connector-file-sftp",
	"cosfile":   "connector-file-cos",
	"obsfile":   "connector-file-obs",

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
}

// getArtifactID returns the correct Maven artifact ID for a plugin name.
// getArtifactID 返回插件名称对应的正确 Maven artifact ID。
// This must match the Control Plane's getArtifactID function.
// 这必须与 Control Plane 的 getArtifactID 函数匹配。
func getArtifactID(name string) string {
	// Check special mappings first / 首先检查特殊映射
	if artifactID, ok := pluginArtifactMappings[name]; ok {
		return artifactID
	}

	// Default: connector-${name} / 默认：connector-${name}
	return fmt.Sprintf("connector-%s", name)
}

// findConnectorFile finds the connector file in the connectors directory.
// findConnectorFile 在连接器目录中查找连接器文件。
// The searchName can be either:
// - artifact_id (e.g., connector-cdc-mysql, connector-file-cos) - preferred
// - plugin_name (e.g., mysql-cdc, cosfile) - fallback with mapping
// searchName 可以是：
// - artifact_id（如 connector-cdc-mysql, connector-file-cos）- 首选
// - plugin_name（如 mysql-cdc, cosfile）- 使用映射作为备用
func (m *Manager) findConnectorFile(connectorsDir, searchName, version string) string {
	// Build list of patterns to try / 构建要尝试的模式列表
	patterns := []string{
		// Try searchName directly (works if it's already artifact_id)
		// 直接尝试 searchName（如果它已经是 artifact_id 则有效）
		fmt.Sprintf("%s-%s.jar", searchName, version),
	}

	// If searchName doesn't start with "connector-", it's likely a plugin_name
	// 如果 searchName 不以 "connector-" 开头，它可能是 plugin_name
	if !strings.HasPrefix(searchName, "connector-") {
		// Get artifact ID from mapping / 从映射获取 artifact ID
		artifactID := getArtifactID(searchName)
		patterns = append(patterns, fmt.Sprintf("%s-%s.jar", artifactID, version))
		// Also try simple connector-{name} pattern / 也尝试简单的 connector-{name} 模式
		patterns = append(patterns, fmt.Sprintf("connector-%s-%s.jar", searchName, version))
	}

	// Try each pattern / 尝试每个模式
	for _, pattern := range patterns {
		path := filepath.Join(connectorsDir, pattern)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// If not found by pattern, scan directory for any matching file
	// 如果按模式未找到，扫描目录查找任何匹配的文件
	entries, err := os.ReadDir(connectorsDir)
	if err != nil {
		return ""
	}

	versionSuffix := fmt.Sprintf("-%s.jar", version)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Check if file contains search name and version
		// 检查文件是否包含搜索名称和版本
		if strings.Contains(name, searchName) && strings.HasSuffix(name, versionSuffix) {
			return filepath.Join(connectorsDir, name)
		}
	}

	return ""
}

// ListInstalledPlugins scans the connectors directory and returns installed plugins.
// ListInstalledPlugins 扫描连接器目录并返回已安装的插件。
func (m *Manager) ListInstalledPlugins(installPath string) ([]InstalledPluginInfo, error) {
	if installPath != "" {
		m.seatunnelPath = installPath
	}

	if m.seatunnelPath == "" {
		return nil, ErrInstallPathNotSet
	}

	connectorsDir := m.GetConnectorsDir()
	var plugins []InstalledPluginInfo

	entries, err := os.ReadDir(connectorsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return plugins, nil
		}
		return nil, fmt.Errorf("failed to read connectors directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jar") {
			continue
		}

		// Parse plugin name and version from filename
		// 从文件名解析插件名称和版本
		// Format: connector-{name}-{version}.jar
		name, version := parseConnectorFileName(entry.Name())
		if name == "" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		plugins = append(plugins, InstalledPluginInfo{
			Name:          name,
			Version:       version,
			ConnectorPath: filepath.Join(connectorsDir, entry.Name()),
			Size:          info.Size(),
			InstalledAt:   info.ModTime(),
		})
	}

	return plugins, nil
}

// IsPluginInstalled checks if a plugin is installed.
// IsPluginInstalled 检查插件是否已安装。
func (m *Manager) IsPluginInstalled(pluginName, version, installPath string) bool {
	if installPath != "" {
		m.seatunnelPath = installPath
	}

	if m.seatunnelPath == "" {
		return false
	}

	connectorFileName := fmt.Sprintf("connector-%s-%s.jar", pluginName, version)
	connectorPath := filepath.Join(m.GetConnectorsDir(), connectorFileName)

	_, err := os.Stat(connectorPath)
	return err == nil
}

// CancelTransfer cancels an ongoing file transfer.
// CancelTransfer 取消正在进行的文件传输。
func (m *Manager) CancelTransfer(pluginName, version, fileName string) error {
	key := fmt.Sprintf("%s:%s:%s", pluginName, version, fileName)

	m.mu.Lock()
	state, exists := m.transfers[key]
	if exists {
		delete(m.transfers, key)
	}
	m.mu.Unlock()

	if exists && state.TempPath != "" {
		os.Remove(state.TempPath)
	}

	return nil
}

// CleanupTempFiles removes all temporary files.
// CleanupTempFiles 删除所有临时文件。
func (m *Manager) CleanupTempFiles() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, state := range m.transfers {
		if state.TempPath != "" {
			os.Remove(state.TempPath)
		}
		delete(m.transfers, key)
	}

	return nil
}

// parseConnectorFileName parses plugin name and version from connector filename.
// parseConnectorFileName 从连接器文件名解析插件名称和版本。
// Format: connector-{name}-{version}.jar
func parseConnectorFileName(filename string) (name, version string) {
	// Remove .jar suffix / 移除 .jar 后缀
	filename = strings.TrimSuffix(filename, ".jar")

	// Remove connector- prefix / 移除 connector- 前缀
	if !strings.HasPrefix(filename, "connector-") {
		return "", ""
	}
	filename = strings.TrimPrefix(filename, "connector-")

	// Find the last dash followed by a version number / 找到最后一个后跟版本号的破折号
	// Version format: X.Y.Z or X.Y.Z-suffix
	lastDash := strings.LastIndex(filename, "-")
	if lastDash == -1 {
		return filename, ""
	}

	// Check if the part after the last dash looks like a version / 检查最后一个破折号后的部分是否像版本号
	possibleVersion := filename[lastDash+1:]
	if len(possibleVersion) > 0 && (possibleVersion[0] >= '0' && possibleVersion[0] <= '9') {
		return filename[:lastDash], possibleVersion
	}

	return filename, ""
}

// calculateSHA1 calculates the SHA1 checksum of a file.
// calculateSHA1 计算文件的 SHA1 校验和。
func calculateSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha1.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// copyFile copies a file from src to dst.
// copyFile 将文件从 src 复制到 dst。
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}
