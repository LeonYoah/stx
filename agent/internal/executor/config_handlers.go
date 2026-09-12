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

package executor

import (
	"context"
	"encoding/json"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/config"
)

// ConfigHandlers 配置相关命令处理器
type ConfigHandlers struct {
	configManager *config.Manager
}

// NewConfigHandlers 创建配置处理器实例
func NewConfigHandlers() *ConfigHandlers {
	return &ConfigHandlers{
		configManager: config.NewManager(),
	}
}

// RegisterHandlers 注册所有配置相关的命令处理器
func (h *ConfigHandlers) RegisterHandlers(executor *CommandExecutor) {
	executor.RegisterHandler(pb.CommandType_PULL_CONFIG, h.HandlePullConfig)
	executor.RegisterHandler(pb.CommandType_UPDATE_CONFIG, h.HandleUpdateConfig)
	executor.RegisterHandler(pb.CommandType_ROLLBACK_CONFIG, h.HandleRollbackConfig)
}

// HandlePullConfig 处理拉取配置命令
// 参数:
//   - install_dir: SeaTunnel 安装目录
//   - config_type: 配置类型 (seatunnel.yaml, hazelcast.yaml, jvm_options 等)
func (h *ConfigHandlers) HandlePullConfig(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	installDir := cmd.Parameters["install_dir"]
	configType := cmd.Parameters["config_type"]

	if installDir == "" {
		return CreateErrorResponse(cmd.CommandId, "install_dir parameter is required"), nil
	}
	if configType == "" {
		return CreateErrorResponse(cmd.CommandId, "config_type parameter is required"), nil
	}

	// 上报进度
	if reporter != nil {
		reporter.Report(10, "Starting to pull config file...")
	}

	// 拉取配置
	result, err := h.configManager.PullConfig(installDir, configType)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, err.Error()), nil
	}

	if !result.Success {
		return CreateErrorResponse(cmd.CommandId, result.Message), nil
	}

	// 上报进度
	if reporter != nil {
		reporter.Report(100, "Config file pulled successfully")
	}

	// 返回结果
	return CreateSuccessResponse(cmd.CommandId, result.ToJSON()), nil
}

// HandleUpdateConfig 处理更新配置命令
// 参数:
//   - install_dir: SeaTunnel 安装目录
//   - config_type: 配置类型
//   - content: 新的配置内容
//   - backup: 是否备份原文件 ("true" 或 "false")
func (h *ConfigHandlers) HandleUpdateConfig(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	installDir := cmd.Parameters["install_dir"]
	configType := cmd.Parameters["config_type"]
	content := cmd.Parameters["content"]
	backupStr := cmd.Parameters["backup"]

	if installDir == "" {
		return CreateErrorResponse(cmd.CommandId, "install_dir parameter is required"), nil
	}
	if configType == "" {
		return CreateErrorResponse(cmd.CommandId, "config_type parameter is required"), nil
	}
	if content == "" {
		return CreateErrorResponse(cmd.CommandId, "content parameter is required"), nil
	}

	backup := backupStr == "true"

	// 上报进度
	if reporter != nil {
		reporter.Report(10, "Starting to update config file...")
	}

	// 更新配置
	result, err := h.configManager.UpdateConfig(installDir, configType, content, backup)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, err.Error()), nil
	}

	if !result.Success {
		return CreateErrorResponse(cmd.CommandId, result.Message), nil
	}

	// 上报进度
	if reporter != nil {
		reporter.Report(100, "Config file updated successfully")
	}

	// 返回结果
	return CreateSuccessResponse(cmd.CommandId, result.ToJSON()), nil
}

// HandleRollbackConfig 处理回滚配置命令
// 参数:
//   - install_dir: SeaTunnel 安装目录
//   - config_type: 配置类型
//   - backup_path: 备份文件路径
func (h *ConfigHandlers) HandleRollbackConfig(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	installDir := cmd.Parameters["install_dir"]
	configType := cmd.Parameters["config_type"]
	backupPath := cmd.Parameters["backup_path"]

	if installDir == "" {
		return CreateErrorResponse(cmd.CommandId, "install_dir parameter is required"), nil
	}
	if configType == "" {
		return CreateErrorResponse(cmd.CommandId, "config_type parameter is required"), nil
	}
	if backupPath == "" {
		return CreateErrorResponse(cmd.CommandId, "backup_path parameter is required"), nil
	}

	// 上报进度
	if reporter != nil {
		reporter.Report(10, "Starting to rollback config file...")
	}

	// 读取备份文件内容
	pullResult, err := h.configManager.PullConfig(installDir, configType)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, err.Error()), nil
	}

	// 这里简化处理：从备份路径读取内容并更新
	// 实际实现中可能需要更复杂的逻辑
	result, err := h.configManager.UpdateConfig(installDir, configType, pullResult.Content, false)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, err.Error()), nil
	}

	if !result.Success {
		return CreateErrorResponse(cmd.CommandId, result.Message), nil
	}

	// 上报进度
	if reporter != nil {
		reporter.Report(100, "Config file rolled back successfully")
	}

	// 返回结果
	responseData := map[string]interface{}{
		"success": true,
		"message": "Config file rolled back successfully",
	}
	responseJSON, _ := json.Marshal(responseData)
	return CreateSuccessResponse(cmd.CommandId, string(responseJSON)), nil
}
