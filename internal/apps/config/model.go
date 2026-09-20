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

// Package config provides cluster configuration file management.
// config 包提供集群配置文件管理功能。
package config

import (
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

// HybridConfigTypes Hybrid 模式支持的配置文件类型
var HybridConfigTypes = []ConfigType{
	ConfigTypeSeatunnel,
	ConfigTypeHazelcast,
	ConfigTypeHazelcastClient,
	ConfigTypeJVMOptions,
	ConfigTypeLog4j2,
	ConfigTypeSeatunnelEnv,
}

// SeparatedConfigTypes Separated 模式支持的配置文件类型
var SeparatedConfigTypes = []ConfigType{
	ConfigTypeSeatunnel,
	ConfigTypeHazelcastMaster,
	ConfigTypeHazelcastWorker,
	ConfigTypeHazelcastClient,
	ConfigTypeJVMMasterOptions,
	ConfigTypeJVMWorkerOptions,
	ConfigTypeLog4j2,
	ConfigTypeSeatunnelEnv,
}

// SupportedConfigTypes 支持的配置文件类型列表（所有类型）
var SupportedConfigTypes = []ConfigType{
	ConfigTypeSeatunnel,
	ConfigTypeHazelcast,
	ConfigTypeHazelcastClient,
	ConfigTypeJVMOptions,
	ConfigTypeLog4j2,
	ConfigTypeSeatunnelEnv,
	ConfigTypeHazelcastMaster,
	ConfigTypeHazelcastWorker,
	ConfigTypeJVMMasterOptions,
	ConfigTypeJVMWorkerOptions,
}

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

// GetConfigTypesForMode 根据部署模式获取支持的配置类型
func GetConfigTypesForMode(deploymentMode string) []ConfigType {
	if deploymentMode == "separated" {
		return SeparatedConfigTypes
	}
	return HybridConfigTypes
}

// Config 配置文件表
// HostID 为 NULL 表示集群模板配置，否则为节点级配置
type Config struct {
	ID         uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	ClusterID  uint       `json:"cluster_id" gorm:"index;not null"`
	HostID     *uint      `json:"host_id" gorm:"index"`                // NULL = 集群模板
	ConfigType ConfigType `json:"config_type" gorm:"size:50;not null"` // 配置类型
	FilePath   string     `json:"file_path" gorm:"size:255"`           // 节点上的实际路径
	Content    string     `json:"content" gorm:"type:text"`            // 配置内容
	Version    int        `json:"version" gorm:"default:1"`            // 当前版本号
	UpdatedAt  time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	UpdatedBy  uint       `json:"updated_by"`
	CreatedAt  time.Time  `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定表名
func (Config) TableName() string {
	return "configs"
}

// IsTemplate 判断是否为集群模板配置
func (c *Config) IsTemplate() bool {
	return c.HostID == nil
}

// ConfigVersion 配置版本表
type ConfigVersion struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	ConfigID  uint      `json:"config_id" gorm:"index;not null"`
	Version   int       `json:"version" gorm:"not null"`
	Content   string    `json:"content" gorm:"type:text"`
	Comment   string    `json:"comment" gorm:"size:255"` // 修改说明
	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// TableName 指定表名
func (ConfigVersion) TableName() string {
	return "config_versions"
}

// ConfigInfo 配置信息（用于 API 响应）
type ConfigInfo struct {
	ID            uint       `json:"id"`
	ClusterID     uint       `json:"cluster_id"`
	HostID        *uint      `json:"host_id"`
	HostName      string     `json:"host_name,omitempty"`
	HostIP        string     `json:"host_ip,omitempty"`
	ConfigType    ConfigType `json:"config_type"`
	FilePath      string     `json:"file_path"`
	Content       string     `json:"content"`
	Version       int        `json:"version"`
	IsTemplate    bool       `json:"is_template"`
	MatchTemplate bool       `json:"match_template"` // 是否与模板一致
	UpdatedAt     time.Time  `json:"updated_at"`
	UpdatedBy     uint       `json:"updated_by"`
	PushError     string     `json:"push_error,omitempty"` // 推送到节点的错误信息
}

// ConfigVersionInfo 版本信息（用于 API 响应）
type ConfigVersionInfo struct {
	ID        uint      `json:"id"`
	ConfigID  uint      `json:"config_id"`
	Version   int       `json:"version"`
	Content   string    `json:"content"`
	Comment   string    `json:"comment"`
	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateConfigRequest 创建配置请求
type CreateConfigRequest struct {
	ClusterID  uint       `json:"cluster_id" binding:"required"`
	HostID     *uint      `json:"host_id"`
	ConfigType ConfigType `json:"config_type" binding:"required"`
	Content    string     `json:"content"`
	Comment    string     `json:"comment"`
}

// UpdateConfigRequest 更新配置请求
type UpdateConfigRequest struct {
	Content string `json:"content" binding:"required"`
	Comment string `json:"comment"`
}

// PromoteConfigRequest 推广配置到集群请求
type PromoteConfigRequest struct {
	Comment string `json:"comment"`
}

// SyncConfigRequest 从集群模板同步请求
type SyncConfigRequest struct {
	Comment string `json:"comment"`
}

// RollbackConfigRequest 回滚配置请求
type RollbackConfigRequest struct {
	Version int    `json:"version" binding:"required"`
	Comment string `json:"comment"`
}

// NormalizeConfigRequest 配置规范化请求
type NormalizeConfigRequest struct {
	ConfigType ConfigType `json:"config_type" binding:"required"`
	Content    string     `json:"content" binding:"required"`
}

// ConfigFilter 配置过滤条件
type ConfigFilter struct {
	ClusterID    uint       `json:"cluster_id"`
	HostID       *uint      `json:"host_id"`
	ConfigType   ConfigType `json:"config_type"`
	OnlyTemplate bool       `json:"only_template"`
}

// SyncAllResult 批量同步结果
type SyncAllResult struct {
	SyncedCount int          `json:"synced_count"` // 同步成功的数量
	PushErrors  []*PushError `json:"push_errors"`  // 推送失败的节点列表
}

// PushError 推送错误信息
type PushError struct {
	HostID  uint   `json:"host_id"`
	HostIP  string `json:"host_ip,omitempty"`
	Message string `json:"message"`
}
