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

// Package executor provides command execution functionality for the Agent.
// executor 包提供 Agent 的命令执行功能。
package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/plugin"
)

// PluginResult represents the result of a plugin operation.
// PluginResult 表示插件操作的结果。
type PluginResult struct {
	Success       bool     `json:"success"`
	Message       string   `json:"message"`
	ConnectorPath string   `json:"connector_path,omitempty"`
	LibPaths      []string `json:"lib_paths,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// PluginListResult represents the result of listing installed plugins.
// PluginListResult 表示列出已安装插件的结果。
type PluginListResult struct {
	Success bool                         `json:"success"`
	Message string                       `json:"message"`
	Plugins []plugin.InstalledPluginInfo `json:"plugins"`
}

// pluginManager is the global plugin manager instance.
// pluginManager 是全局插件管理器实例。
var pluginManager *plugin.Manager

// InitPluginManager initializes the plugin manager with the SeaTunnel home directory.
// InitPluginManager 使用 SeaTunnel 主目录初始化插件管理器。
func InitPluginManager(seatunnelHome string) {
	pluginManager = plugin.NewManager(seatunnelHome)
}

// GetPluginManager returns the plugin manager instance.
// GetPluginManager 返回插件管理器实例。
func GetPluginManager() *plugin.Manager {
	return pluginManager
}

// RegisterPluginHandlers registers all plugin-related command handlers.
// RegisterPluginHandlers 注册所有插件相关的命令处理器。
func RegisterPluginHandlers(executor *CommandExecutor) {
	executor.RegisterHandler(pb.CommandType_TRANSFER_PLUGIN, HandleTransferPluginCommand)
	executor.RegisterHandler(pb.CommandType_INSTALL_PLUGIN, HandleInstallPluginCommand)
	executor.RegisterHandler(pb.CommandType_UNINSTALL_PLUGIN, HandleUninstallPluginCommand)
	executor.RegisterHandler(pb.CommandType_LIST_PLUGINS, HandleListPluginsCommand)
}

// HandleTransferPluginCommand handles the TRANSFER_PLUGIN command type.
// HandleTransferPluginCommand 处理 TRANSFER_PLUGIN 命令类型。
// This command receives plugin file chunks from Control Plane.
// 此命令从 Control Plane 接收插件文件块。
func HandleTransferPluginCommand(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	if pluginManager == nil {
		return CreateErrorResponse(cmd.CommandId, "plugin manager not initialized / 插件管理器未初始化"), nil
	}

	// Extract parameters / 提取参数
	pluginName := cmd.Parameters["plugin_name"]
	version := cmd.Parameters["version"]
	fileType := cmd.Parameters["file_type"]
	targetDir := cmd.Parameters["target_dir"]
	fileName := cmd.Parameters["file_name"]
	chunkData := cmd.Parameters["chunk"]
	offsetStr := cmd.Parameters["offset"]
	totalSizeStr := cmd.Parameters["total_size"]
	isLastStr := cmd.Parameters["is_last"]
	checksum := cmd.Parameters["checksum"]
	installPath := cmd.Parameters["install_path"]

	// install_path is required for plugin installation
	// install_path 是插件安装的必需参数
	if installPath == "" {
		return CreateErrorResponse(cmd.CommandId, "install_path is required for plugin transfer / 插件传输需要 install_path 参数"), nil
	}

	// Set install path / 设置安装路径
	pluginManager.SetSeaTunnelPath(installPath)

	if pluginName == "" || version == "" || fileName == "" {
		return CreateErrorResponse(cmd.CommandId, "missing required parameters: plugin_name, version, file_name"), nil
	}

	// Parse numeric parameters / 解析数字参数
	offset, _ := strconv.ParseInt(offsetStr, 10, 64)
	totalSize, _ := strconv.ParseInt(totalSizeStr, 10, 64)
	isLast := isLastStr == "true"

	// Decode chunk data (base64 encoded) / 解码数据块（base64 编码）
	chunk, err := base64.StdEncoding.DecodeString(chunkData)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, fmt.Sprintf("failed to decode chunk data: %v", err)), nil
	}

	// Receive chunk using ReceivePluginChunk / 使用 ReceivePluginChunk 接收数据块
	receivedBytes, err := pluginManager.ReceivePluginChunk(pluginName, version, fileType, targetDir, fileName, chunk, offset, totalSize, isLast, checksum)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, fmt.Sprintf("failed to receive chunk: %v", err)), nil
	}

	// Calculate progress / 计算进度
	progress := int32(0)
	if totalSize > 0 {
		progress = int32((receivedBytes * 100) / totalSize)
	}

	if isLast {
		// Finalize transfer / 完成传输
		targetPath, err := pluginManager.FinalizeTransfer(pluginName, version, targetDir, fileName)
		if err != nil {
			return CreateErrorResponse(cmd.CommandId, fmt.Sprintf("failed to finalize transfer: %v", err)), nil
		}

		result := &PluginResult{
			Success:       true,
			Message:       fmt.Sprintf("File transfer completed: %s / 文件传输完成: %s", fileName, fileName),
			ConnectorPath: targetPath,
		}
		output, _ := json.Marshal(result)
		return CreateSuccessResponse(cmd.CommandId, string(output)), nil
	}

	// Report progress / 报告进度
	if reporter != nil {
		reporter.Report(progress, fmt.Sprintf("Receiving %s: %d/%d bytes", fileName, receivedBytes, totalSize))
	}

	return CreateProgressResponse(cmd.CommandId, progress, fmt.Sprintf("Received %d/%d bytes", receivedBytes, totalSize)), nil
}

// HandleInstallPluginCommand handles the INSTALL_PLUGIN command type.
// HandleInstallPluginCommand 处理 INSTALL_PLUGIN 命令类型。
// This command installs a plugin from temp directory to SeaTunnel directories.
// 此命令将插件从临时目录安装到 SeaTunnel 目录。
func HandleInstallPluginCommand(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	if pluginManager == nil {
		return CreateErrorResponse(cmd.CommandId, "plugin manager not initialized / 插件管理器未初始化"), nil
	}

	// Extract parameters / 提取参数
	pluginName := cmd.Parameters["plugin_name"]
	artifactID := cmd.Parameters["artifact_id"] // Maven artifact ID (e.g., connector-cdc-mysql)
	version := cmd.Parameters["version"]
	installPath := cmd.Parameters["install_path"]
	dependenciesStr := cmd.Parameters["dependencies"]

	if pluginName == "" || version == "" {
		return CreateErrorResponse(cmd.CommandId, "missing required parameters: plugin_name, version"), nil
	}

	// install_path is required for plugin installation
	// install_path 是插件安装的必需参数
	if installPath == "" {
		return CreateErrorResponse(cmd.CommandId, "install_path is required for plugin installation / 插件安装需要 install_path 参数"), nil
	}

	// Parse dependencies / 解析依赖
	var dependencies []string
	if dependenciesStr != "" {
		dependencies = strings.Split(dependenciesStr, ",")
	}

	if reporter != nil {
		reporter.Report(10, fmt.Sprintf("Installing plugin %s v%s / 正在安装插件 %s v%s", pluginName, version, pluginName, version))
	}

	// Install plugin using artifact_id if provided, otherwise use plugin_name
	// 如果提供了 artifact_id 则使用它，否则使用 plugin_name
	searchName := pluginName
	if artifactID != "" {
		searchName = artifactID
	}
	connectorPath, libPaths, err := pluginManager.InstallPlugin(searchName, version, installPath, dependencies)
	if err != nil {
		result := &PluginResult{
			Success: false,
			Message: "Plugin installation failed / 插件安装失败",
			Error:   err.Error(),
		}
		output, _ := json.Marshal(result)
		return CreateErrorResponse(cmd.CommandId, string(output)), nil
	}

	if reporter != nil {
		reporter.Report(100, fmt.Sprintf("Plugin %s installed successfully / 插件 %s 安装成功", pluginName, pluginName))
	}

	result := &PluginResult{
		Success:       true,
		Message:       fmt.Sprintf("Plugin %s v%s installed successfully / 插件 %s v%s 安装成功", pluginName, version, pluginName, version),
		ConnectorPath: connectorPath,
		LibPaths:      libPaths,
	}
	output, _ := json.Marshal(result)
	return CreateSuccessResponse(cmd.CommandId, string(output)), nil
}

// HandleUninstallPluginCommand handles the UNINSTALL_PLUGIN command type.
// HandleUninstallPluginCommand 处理 UNINSTALL_PLUGIN 命令类型。
// This command uninstalls a plugin from SeaTunnel directories.
// 此命令从 SeaTunnel 目录卸载插件。
func HandleUninstallPluginCommand(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	if pluginManager == nil {
		return CreateErrorResponse(cmd.CommandId, "plugin manager not initialized / 插件管理器未初始化"), nil
	}

	// Extract parameters / 提取参数
	pluginName := cmd.Parameters["plugin_name"]
	version := cmd.Parameters["version"]
	installPath := cmd.Parameters["install_path"]
	removeDepsStr := cmd.Parameters["remove_dependencies"]

	if pluginName == "" || version == "" {
		return CreateErrorResponse(cmd.CommandId, "missing required parameters: plugin_name, version"), nil
	}

	removeDeps := removeDepsStr == "true"

	if reporter != nil {
		reporter.Report(10, fmt.Sprintf("Uninstalling plugin %s v%s / 正在卸载插件 %s v%s", pluginName, version, pluginName, version))
	}

	// Uninstall plugin / 卸载插件
	if err := pluginManager.UninstallPlugin(pluginName, version, installPath, removeDeps); err != nil {
		result := &PluginResult{
			Success: false,
			Message: "Plugin uninstallation failed / 插件卸载失败",
			Error:   err.Error(),
		}
		output, _ := json.Marshal(result)
		return CreateErrorResponse(cmd.CommandId, string(output)), nil
	}

	if reporter != nil {
		reporter.Report(100, fmt.Sprintf("Plugin %s uninstalled successfully / 插件 %s 卸载成功", pluginName, pluginName))
	}

	result := &PluginResult{
		Success: true,
		Message: fmt.Sprintf("Plugin %s v%s uninstalled successfully / 插件 %s v%s 卸载成功", pluginName, version, pluginName, version),
	}
	output, _ := json.Marshal(result)
	return CreateSuccessResponse(cmd.CommandId, string(output)), nil
}

// HandleListPluginsCommand handles the LIST_PLUGINS command type.
// HandleListPluginsCommand 处理 LIST_PLUGINS 命令类型。
// This command lists all installed plugins.
// 此命令列出所有已安装的插件。
func HandleListPluginsCommand(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	if pluginManager == nil {
		return CreateErrorResponse(cmd.CommandId, "plugin manager not initialized / 插件管理器未初始化"), nil
	}

	// Extract parameters / 提取参数
	installPath := cmd.Parameters["install_path"]

	// List installed plugins / 列出已安装插件
	plugins, err := pluginManager.ListInstalledPlugins(installPath)
	if err != nil {
		result := &PluginListResult{
			Success: false,
			Message: fmt.Sprintf("Failed to list plugins: %v / 列出插件失败: %v", err, err),
			Plugins: []plugin.InstalledPluginInfo{},
		}
		output, _ := json.Marshal(result)
		return CreateErrorResponse(cmd.CommandId, string(output)), nil
	}

	result := &PluginListResult{
		Success: true,
		Message: fmt.Sprintf("Found %d installed plugins / 找到 %d 个已安装插件", len(plugins), len(plugins)),
		Plugins: plugins,
	}
	output, _ := json.Marshal(result)
	return CreateSuccessResponse(cmd.CommandId, string(output)), nil
}
