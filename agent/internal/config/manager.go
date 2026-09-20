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

// Package config provides SeaTunnel configuration file management for Agent.
// config 包提供 Agent 端的 SeaTunnel 配置文件管理功能。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ConfigType 配置文件类型
type ConfigType string

const (
	// 通用配置（Hybrid 模式）
	ConfigTypeSeatunnel       ConfigType = "seatunnel.yaml"
	ConfigTypeHazelcast       ConfigType = "hazelcast.yaml"
	ConfigTypeHazelcastClient ConfigType = "hazelcast-client.yaml"
	ConfigTypeJVMOptions      ConfigType = "jvm_options"
	ConfigTypeLog4j2          ConfigType = "log4j2.properties"

	// 环境变量配置
	// Environment variable configuration
	ConfigTypeSeatunnelEnv ConfigType = "seatunnel-env.sh"

	// 分离模式配置（Separated 模式）
	ConfigTypeHazelcastMaster  ConfigType = "hazelcast-master.yaml"
	ConfigTypeHazelcastWorker  ConfigType = "hazelcast-worker.yaml"
	ConfigTypeJVMMasterOptions ConfigType = "jvm_master_options"
	ConfigTypeJVMWorkerOptions ConfigType = "jvm_worker_options"
)

// GetConfigFilePath 获取配置文件相对于 SEATUNNEL_HOME 的路径
func GetConfigFilePath(configType ConfigType) string {
	switch configType {
	case ConfigTypeSeatunnel:
		return "config/seatunnel.yaml"
	case ConfigTypeHazelcast:
		return "config/hazelcast.yaml"
	case ConfigTypeHazelcastClient:
		return "config/hazelcast-client.yaml"
	case ConfigTypeJVMOptions:
		return "config/jvm_options"
	case ConfigTypeLog4j2:
		return "config/log4j2.properties"
	case ConfigTypeSeatunnelEnv:
		return "config/seatunnel-env.sh"
	case ConfigTypeHazelcastMaster:
		return "config/hazelcast-master.yaml"
	case ConfigTypeHazelcastWorker:
		return "config/hazelcast-worker.yaml"
	case ConfigTypeJVMMasterOptions:
		return "config/jvm_master_options"
	case ConfigTypeJVMWorkerOptions:
		return "config/jvm_worker_options"
	default:
		return ""
	}
}

// Manager 配置文件管理器
type Manager struct {
	backupDir string // 备份目录
}

// NewManager 创建配置管理器实例
func NewManager() *Manager {
	return &Manager{
		backupDir: "config_backups",
	}
}

// PullConfigResult 拉取配置结果
type PullConfigResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	ConfigType string `json:"config_type"`
	Content    string `json:"content"`
	FilePath   string `json:"file_path"`
}

// PullConfig 拉取配置文件内容
func (m *Manager) PullConfig(installDir string, configType string) (*PullConfigResult, error) {
	ct := ConfigType(configType)
	relativePath := GetConfigFilePath(ct)
	if relativePath == "" {
		return &PullConfigResult{
			Success:    false,
			Message:    fmt.Sprintf("unsupported config type: %s", configType),
			ConfigType: configType,
		}, nil
	}

	fullPath := filepath.Join(installDir, relativePath)

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return &PullConfigResult{
			Success:    false,
			Message:    fmt.Sprintf("config file not found: %s", fullPath),
			ConfigType: configType,
			FilePath:   fullPath,
		}, nil
	}

	// 读取文件内容
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return &PullConfigResult{
			Success:    false,
			Message:    fmt.Sprintf("failed to read config file: %v", err),
			ConfigType: configType,
			FilePath:   fullPath,
		}, nil
	}

	return &PullConfigResult{
		Success:    true,
		Message:    "config file read successfully",
		ConfigType: configType,
		Content:    string(content),
		FilePath:   fullPath,
	}, nil
}

// UpdateConfigResult 更新配置结果
type UpdateConfigResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	BackupPath string `json:"backup_path,omitempty"`
}

// UpdateConfig 更新配置文件内容
func (m *Manager) UpdateConfig(installDir string, configType string, content string, backup bool) (*UpdateConfigResult, error) {
	ct := ConfigType(configType)
	relativePath := GetConfigFilePath(ct)
	if relativePath == "" {
		return &UpdateConfigResult{
			Success: false,
			Message: fmt.Sprintf("unsupported config type: %s", configType),
		}, nil
	}

	fullPath := filepath.Join(installDir, relativePath)

	// 如果需要备份，先备份原文件
	var backupPath string
	if backup {
		if _, err := os.Stat(fullPath); err == nil {
			backupPath, err = m.backupConfig(installDir, fullPath, configType)
			if err != nil {
				return &UpdateConfigResult{
					Success: false,
					Message: fmt.Sprintf("failed to backup config file: %v", err),
				}, nil
			}
		}
	}

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &UpdateConfigResult{
			Success:    false,
			Message:    fmt.Sprintf("failed to create config directory: %v", err),
			BackupPath: backupPath,
		}, nil
	}

	// 写入新内容
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return &UpdateConfigResult{
			Success:    false,
			Message:    fmt.Sprintf("failed to write config file: %v", err),
			BackupPath: backupPath,
		}, nil
	}

	return &UpdateConfigResult{
		Success:    true,
		Message:    "config file updated successfully",
		BackupPath: backupPath,
	}, nil
}

// backupConfig 备份配置文件
func (m *Manager) backupConfig(installDir, filePath, configType string) (string, error) {
	// 创建备份目录
	backupDir := filepath.Join(installDir, m.backupDir)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// 生成备份文件名
	timestamp := time.Now().Format("20060102_150405")
	backupFileName := fmt.Sprintf("%s.%s.bak", configType, timestamp)
	backupPath := filepath.Join(backupDir, backupFileName)

	// 读取原文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read original file: %w", err)
	}

	// 写入备份文件
	if err := os.WriteFile(backupPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupPath, nil
}

// PullAllConfigs 拉取所有配置文件
func (m *Manager) PullAllConfigs(installDir string) (map[string]*PullConfigResult, error) {
	configTypes := []ConfigType{
		// 通用配置
		ConfigTypeSeatunnel,
		ConfigTypeHazelcast,
		ConfigTypeHazelcastClient,
		ConfigTypeJVMOptions,
		ConfigTypeLog4j2,
		ConfigTypeSeatunnelEnv,
		// 分离模式配置
		ConfigTypeHazelcastMaster,
		ConfigTypeHazelcastWorker,
		ConfigTypeJVMMasterOptions,
		ConfigTypeJVMWorkerOptions,
	}

	results := make(map[string]*PullConfigResult)
	for _, ct := range configTypes {
		result, err := m.PullConfig(installDir, string(ct))
		if err != nil {
			return nil, err
		}
		results[string(ct)] = result
	}

	return results, nil
}

// ToJSON 将结果转换为 JSON 字符串
func (r *PullConfigResult) ToJSON() string {
	data, _ := json.Marshal(r)
	return string(data)
}

// ToJSON 将结果转换为 JSON 字符串
func (r *UpdateConfigResult) ToJSON() string {
	data, _ := json.Marshal(r)
	return string(data)
}
