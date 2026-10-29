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

// Package discovery provides simplified SeaTunnel process discovery for the Agent.
// discovery 包提供 Agent 的简化 SeaTunnel 进程发现功能。
//
// Note: Config parsing is no longer needed for cluster discovery.
// 注意：集群发现不再需要配置解析。
// User creates cluster manually and specifies version/mode in frontend.
// 用户手动创建集群并在前端指定版本/模式。
// This file only provides utility functions for version detection.
// 此文件仅提供版本检测的实用函数。
package discovery

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// VersionDetector detects SeaTunnel version from installation directory
// VersionDetector 从安装目录检测 SeaTunnel 版本
type VersionDetector struct{}

// NewVersionDetector creates a new VersionDetector instance
// NewVersionDetector 创建一个新的 VersionDetector 实例
func NewVersionDetector() *VersionDetector {
	return &VersionDetector{}
}

// DetectVersion tries to detect SeaTunnel version from the installation directory
// DetectVersion 尝试从安装目录检测 SeaTunnel 版本
// Looks for version in connector jar files or lib directory
// 在 connector jar 文件或 lib 目录中查找版本
func (d *VersionDetector) DetectVersion(installDir string) string {
	// Try connectors directory first (newer versions)
	// 首先尝试 connectors 目录（较新版本）
	if version := d.detectFromConnectors(installDir); version != "" {
		return version
	}

	// Fallback to lib directory
	// 回退到 lib 目录
	if version := d.detectFromLib(installDir); version != "" {
		return version
	}

	return "unknown"
}

// detectFromConnectors detects version from connectors directory
// detectFromConnectors 从 connectors 目录检测版本
func (d *VersionDetector) detectFromConnectors(installDir string) string {
	connectorsDir := filepath.Join(installDir, "connectors")
	entries, err := os.ReadDir(connectorsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		name := entry.Name()
		// Look for connector-fake-*.jar (e.g., connector-fake-2.3.12.jar)
		// 查找 connector-fake-*.jar（例如 connector-fake-2.3.12.jar）
		if strings.HasPrefix(name, "connector-fake-") && strings.HasSuffix(name, ".jar") {
			version := strings.TrimPrefix(name, "connector-fake-")
			version = strings.TrimSuffix(version, ".jar")
			return version
		}
	}

	return ""
}

// detectFromLib detects version from lib directory
// detectFromLib 从 lib 目录检测版本
func (d *VersionDetector) detectFromLib(installDir string) string {
	libDir := filepath.Join(installDir, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		name := entry.Name()
		// Look for seatunnel-engine-core-*.jar
		// 查找 seatunnel-engine-core-*.jar
		if strings.HasPrefix(name, "seatunnel-engine-core-") && strings.HasSuffix(name, ".jar") {
			version := strings.TrimPrefix(name, "seatunnel-engine-core-")
			version = strings.TrimSuffix(version, ".jar")
			return version
		}
	}

	return ""
}

// =============================================================================
// ConfigReader reads port configuration from SeaTunnel config files
// ConfigReader 从 SeaTunnel 配置文件读取端口配置
// =============================================================================

// ConfigReader reads configuration from SeaTunnel YAML files
// ConfigReader 从 SeaTunnel YAML 文件读取配置
type ConfigReader struct{}

// NewConfigReader creates a new ConfigReader instance
// NewConfigReader 创建一个新的 ConfigReader 实例
func NewConfigReader() *ConfigReader {
	return &ConfigReader{}
}

// ReadHazelcastPort reads hazelcast port from config file based on role
// ReadHazelcastPort 根据角色从配置文件读取 hazelcast 端口
// Master -> hazelcast-master.yaml
// Worker -> hazelcast-worker.yaml
// Hybrid/Unknown -> hazelcast.yaml
func (r *ConfigReader) ReadHazelcastPort(installDir, role string) int {
	configDir := filepath.Join(installDir, "config")

	// Determine which config file to read based on role
	// 根据角色确定读取哪个配置文件
	var configFile string
	switch role {
	case "master":
		configFile = filepath.Join(configDir, "hazelcast-master.yaml")
	case "worker":
		configFile = filepath.Join(configDir, "hazelcast-worker.yaml")
	default:
		configFile = filepath.Join(configDir, "hazelcast.yaml")
	}

	// Try role-specific file first, fallback to hazelcast.yaml
	// 首先尝试角色特定文件，回退到 hazelcast.yaml
	port := r.parseHazelcastPort(configFile)
	if port == 0 {
		// Fallback to generic hazelcast.yaml / 回退到通用 hazelcast.yaml
		port = r.parseHazelcastPort(filepath.Join(configDir, "hazelcast.yaml"))
	}

	// Return default port if not found / 如果未找到返回默认端口
	if port == 0 {
		if role == "worker" {
			return 5802 // Default worker port / 默认 worker 端口
		}
		return 5801 // Default master port / 默认 master 端口
	}

	return port
}

// ReadHazelcastClusterName 从配置文件中读取 Hazelcast 集群名称。
// ReadHazelcastClusterName reads hazelcast cluster-name from configuration file.
func (r *ConfigReader) ReadHazelcastClusterName(installDir, role string) string {
	configDir := filepath.Join(installDir, "config")

	var configFile string
	switch role {
	case "master":
		configFile = filepath.Join(configDir, "hazelcast-master.yaml")
	case "worker":
		configFile = filepath.Join(configDir, "hazelcast-worker.yaml")
	default:
		configFile = filepath.Join(configDir, "hazelcast.yaml")
	}

	name := r.parseHazelcastClusterName(configFile)
	if name == "" && configFile != filepath.Join(configDir, "hazelcast.yaml") {
		name = r.parseHazelcastClusterName(filepath.Join(configDir, "hazelcast.yaml"))
	}

	return name
}

// parseHazelcastClusterName 从 YAML 文件解析 Hazelcast 集群名称。
// parseHazelcastClusterName parses hazelcast cluster-name from YAML file.
func (r *ConfigReader) parseHazelcastClusterName(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return ""
	}
	if len(root.Content) == 0 {
		return ""
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return ""
	}

	for i := 0; i+1 < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]
		if keyNode.Value == "hazelcast" && valNode.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(valNode.Content); j += 2 {
				k := valNode.Content[j]
				v := valNode.Content[j+1]
				if k.Value == "cluster-name" {
					return strings.TrimSpace(v.Value)
				}
			}
		}
	}

	return ""
}

// parseHazelcastPort parses hazelcast port from YAML file
// parseHazelcastPort 从 YAML 文件解析 hazelcast 端口
// Looks for: network.port.port or port.port
// 查找：network.port.port 或 port.port
func (r *ConfigReader) parseHazelcastPort(filePath string) int {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}

	// Simple YAML parsing for port value
	// 简单的 YAML 解析获取端口值
	// Looking for pattern like:
	//   port:
	//     port: 5801
	// or
	//   port: 5801
	lines := strings.Split(string(content), "\n")
	inPortSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering port section / 检查是否进入 port 部分
		if strings.HasPrefix(trimmed, "port:") && !strings.Contains(trimmed, "port:") {
			// "port:" alone means we're entering a section
			// 单独的 "port:" 表示进入一个部分
			if trimmed == "port:" {
				inPortSection = true
				continue
			}
			// "port: 5801" inline format / 内联格式
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				portStr := strings.TrimSpace(parts[1])
				if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
					return port
				}
			}
		}

		// If in port section, look for nested port value
		// 如果在 port 部分，查找嵌套的 port 值
		if inPortSection && strings.HasPrefix(trimmed, "port:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				portStr := strings.TrimSpace(parts[1])
				if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
					return port
				}
			}
		}

		// Exit port section if we hit another top-level key
		// 如果遇到另一个顶级键，退出 port 部分
		if inPortSection && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			inPortSection = false
		}
	}

	return 0
}

// ReadAPIPort reads REST API port from seatunnel.yaml
// ReadAPIPort 从 seatunnel.yaml 读取 REST API 端口
// Looks for: http.port
// 查找：http.port
func (r *ConfigReader) ReadAPIPort(installDir string) int {
	configFile := filepath.Join(installDir, "config", "seatunnel.yaml")
	content, err := os.ReadFile(configFile)
	if err != nil {
		return 0
	}

	// Simple YAML parsing for http.port
	// 简单的 YAML 解析获取 http.port
	lines := strings.Split(string(content), "\n")
	inHttpSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're entering http section / 检查是否进入 http 部分
		if strings.HasPrefix(trimmed, "http:") {
			inHttpSection = true
			continue
		}

		// If in http section, look for port value
		// 如果在 http 部分，查找 port 值
		if inHttpSection && strings.HasPrefix(trimmed, "port:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				portStr := strings.TrimSpace(parts[1])
				if port, err := strconv.Atoi(portStr); err == nil && port > 0 {
					return port
				}
			}
		}

		// Exit http section if we hit another top-level key
		// 如果遇到另一个顶级键，退出 http 部分
		if inHttpSection && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" && !strings.HasPrefix(trimmed, "http:") {
			inHttpSection = false
		}
	}

	return 0
}

// =============================================================================
// Legacy types for backward compatibility
// 为了向后兼容的遗留类型
// =============================================================================

// ConfigParser is kept for backward compatibility
// ConfigParser 保留用于向后兼容
// Deprecated: Config parsing is no longer needed
// 已弃用：不再需要配置解析
type ConfigParser struct {
	detector *VersionDetector
}

// NewConfigParser creates a new ConfigParser (deprecated)
// NewConfigParser 创建一个新的 ConfigParser（已弃用）
func NewConfigParser() *ConfigParser {
	return &ConfigParser{
		detector: NewVersionDetector(),
	}
}

// ClusterConfig is kept for backward compatibility
// ClusterConfig 保留用于向后兼容
type ClusterConfig struct {
	ClusterName   string            `json:"cluster_name"`
	HazelcastPort int               `json:"hazelcast_port"`
	Members       []string          `json:"members"`
	Version       string            `json:"version"`
	Extra         map[string]string `json:"extra"`
}

// ParseClusterConfig is kept for backward compatibility
// ParseClusterConfig 保留用于向后兼容
// Returns minimal config with detected version only
// 仅返回包含检测到的版本的最小配置
func (p *ConfigParser) ParseClusterConfig(installDir string, _ string) (*ClusterConfig, error) {
	return &ClusterConfig{
		ClusterName:   "seatunnel",
		HazelcastPort: 5801,
		Version:       p.detector.DetectVersion(installDir),
		Extra:         make(map[string]string),
	}, nil
}
