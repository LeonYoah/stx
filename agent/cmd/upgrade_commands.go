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

package main

import (
	"context"
	"fmt"
	"strings"

	pb "github.com/LeonYoah/stx/agent"
	agentconfig "github.com/LeonYoah/stx/agent/internal/config"
	"github.com/LeonYoah/stx/agent/internal/executor"
)

func (a *Agent) handleManagedUpgradeCommand(ctx context.Context, cmd *pb.CommandRequest, reporter executor.ProgressReporter) (*pb.CommandResponse, error) {
	subCommand := strings.TrimSpace(cmd.Parameters["sub_command"])
	if subCommand == "" {
		err := fmt.Errorf("旧的破坏式升级路径已禁用，请使用受管升级 sub_command 原语")
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}

	switch subCommand {
	case "backup_install_dir":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		backupDir := getParamString(cmd.Parameters, "backup_dir", "")
		backupPath, err := a.installerManager.BackupInstallDir(ctx, installDir, backupDir)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("backup_path=%s", backupPath)), nil
	case "extract_package_to_dir":
		packagePath := getParamString(cmd.Parameters, "package_path", "")
		targetDir := getParamString(cmd.Parameters, "target_dir", "")
		expectedChecksum := getParamString(cmd.Parameters, "expected_checksum", "")
		if err := a.installerManager.ExtractPackageToDir(ctx, packagePath, targetDir, expectedChecksum, &installerProgressAdapter{reporter: reporter, commandID: cmd.CommandId}); err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("target_dir=%s", targetDir)), nil
	case "sync_connectors_manifest":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		keepFiles := splitCSV(cmd.Parameters["keep_files"])
		if err := a.installerManager.SyncConnectorsManifest(installDir, keepFiles); err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("connectors_dir=%s/connectors", installDir)), nil
	case "sync_lib_manifest":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		keepFiles := splitCSV(cmd.Parameters["keep_files"])
		if err := a.installerManager.SyncLibManifest(installDir, keepFiles); err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("lib_dir=%s/lib", installDir)), nil
	case "sync_plugins_manifest":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		keepFiles := splitCSV(cmd.Parameters["keep_files"])
		if err := a.installerManager.SyncPluginsManifest(installDir, keepFiles); err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("plugins_dir=%s/plugins", installDir)), nil
	case "apply_merged_config":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		configType := getParamString(cmd.Parameters, "config_type", "")
		content := getParamString(cmd.Parameters, "content", "")
		backup := getParamString(cmd.Parameters, "backup", "true") == "true"
		manager := agentconfig.NewManager()
		result, err := manager.UpdateConfig(installDir, configType, content, backup)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		if !result.Success {
			return executor.CreateErrorResponse(cmd.CommandId, result.Message), fmt.Errorf("%s", result.Message)
		}
		return executor.CreateSuccessResponse(cmd.CommandId, result.ToJSON()), nil
	case "switch_install_dir":
		targetDir := getParamString(cmd.Parameters, "target_dir", "")
		currentLink := getParamString(cmd.Parameters, "current_link", "")
		switchedPath, err := a.installerManager.SwitchInstallDir(targetDir, currentLink)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("switched_path=%s", switchedPath)), nil
	case "restore_snapshot":
		backupDir := getParamString(cmd.Parameters, "backup_dir", "")
		restoreDir := getParamString(cmd.Parameters, "restore_dir", "")
		if err := a.installerManager.RestoreInstallDir(backupDir, restoreDir); err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("restore_dir=%s", restoreDir)), nil
	case "verify_cluster_health":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		if err := a.installerManager.VerifyInstallLayout(installDir); err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		processName := getParamString(cmd.Parameters, "process_name", "")
		if processName != "" {
			status, err := a.processManager.GetStatus(ctx, processName)
			if err != nil {
				return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
			}
			if status.PID == 0 {
				err := fmt.Errorf("进程 %s 未运行", processName)
				return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
			}
		}
		return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("install_dir=%s", installDir)), nil
	case "run_smoke_test_template":
		installDir := getParamString(cmd.Parameters, "install_dir", "")
		output, err := a.installerManager.RunSmokeTestTemplate(ctx, installDir)
		if err != nil {
			return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
		}
		if strings.TrimSpace(output) == "" {
			return executor.CreateSuccessResponse(cmd.CommandId, fmt.Sprintf("install_dir=%s", installDir)), nil
		}
		return executor.CreateSuccessResponse(cmd.CommandId, output), nil
	default:
		err := fmt.Errorf("不支持的受管升级子命令: %s", subCommand)
		return executor.CreateErrorResponse(cmd.CommandId, err.Error()), err
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}
