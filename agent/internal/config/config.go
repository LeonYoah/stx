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

// Package config provides configuration management for the Agent service.
// config 包提供 Agent 服务的配置管理功能。
//
// Configuration loading priority (highest to lowest):
// 配置加载优先级（从高到低）：
// 1. Command line arguments / 命令行参数
// 2. Environment variables / 环境变量
// 3. Configuration file / 配置文件
// 4. Default values / 默认值
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Default configuration values
// 默认配置值
const (
	DefaultConfigPath          = "/etc/stx-agent/config.yaml"
	DefaultHeartbeatInterval   = 10 * time.Second
	DefaultLogLevel            = "info"
	DefaultLogFile             = "/var/log/stx-agent/agent.log"
	DefaultLogMaxSize          = 100 // MB
	DefaultLogMaxBackups       = 3
	DefaultLogMaxAge           = 7 // days
	DefaultSeaTunnelInstallDir = "/opt/seatunnel"
)

// Config represents the Agent configuration
// Config 表示 Agent 配置
type Config struct {
	// Agent configuration / Agent 配置
	Agent AgentConfig `mapstructure:"agent"`

	// Control Plane connection configuration / Control Plane 连接配置
	ControlPlane ControlPlaneConfig `mapstructure:"control_plane"`

	// Heartbeat configuration / 心跳配置
	Heartbeat HeartbeatConfig `mapstructure:"heartbeat"`

	// Log configuration / 日志配置
	Log LogConfig `mapstructure:"log"`

	// SeaTunnel configuration / SeaTunnel 配置
	SeaTunnel SeaTunnelConfig `mapstructure:"seatunnel"`
}

// AgentConfig contains Agent-specific configuration
// AgentConfig 包含 Agent 特定配置
type AgentConfig struct {
	// ID is the unique identifier for this Agent (auto-generated if empty)
	// ID 是此 Agent 的唯一标识符（如果为空则自动生成）
	ID string `mapstructure:"id"`

	// HostID is the pre-bound Host ID from Control Plane (0 means auto-match)
	// HostID 是来自 Control Plane 的预绑定主机 ID（0 表示自动匹配）
	HostID uint64 `mapstructure:"host_id"`

	// IP is an optional manually configured IP address to override auto-detection
	// IP 是可选的手动配置 IP 地址，用于覆盖自动探测
	IP string `mapstructure:"ip"`
}

// ControlPlaneConfig contains Control Plane connection settings
// ControlPlaneConfig 包含 Control Plane 连接设置
type ControlPlaneConfig struct {
	// Addresses is a list of Control Plane gRPC addresses for high availability
	// Addresses 是用于高可用的 Control Plane gRPC 地址列表
	Addresses []string `mapstructure:"addresses"`

	// TLS configuration / TLS 配置
	TLS TLSConfig `mapstructure:"tls"`

	// Token for authentication / 用于认证的 Token
	Token string `mapstructure:"token"`
}

// TLSConfig contains TLS settings
// TLSConfig 包含 TLS 设置
type TLSConfig struct {
	// Enabled indicates whether TLS is enabled
	// Enabled 表示是否启用 TLS
	Enabled bool `mapstructure:"enabled"`

	// CertFile is the optional client certificate for mTLS (not required for one-way TLS).
	// CertFile 是可选的 mTLS 客户端证书（单向 TLS 不需要）。
	CertFile string `mapstructure:"cert_file"`

	// KeyFile is the optional client private key for mTLS (not required for one-way TLS).
	// KeyFile 是可选的 mTLS 客户端私钥（单向 TLS 不需要）。
	KeyFile string `mapstructure:"key_file"`

	// CAFile is the CA certificate used to verify the Control Plane server (required for one-way TLS).
	// CAFile 是用于校验 Control Plane 服务端的 CA 证书（单向 TLS 必需）。
	CAFile string `mapstructure:"ca_file"`
}

// HeartbeatConfig contains heartbeat settings
// HeartbeatConfig 包含心跳设置
type HeartbeatConfig struct {
	// Interval is the heartbeat interval
	// Interval 是心跳间隔
	Interval time.Duration `mapstructure:"interval"`
}

// LogConfig contains logging settings
// LogConfig 包含日志设置
type LogConfig struct {
	// Level is the log level (debug, info, warn, error)
	// Level 是日志级别（debug, info, warn, error）
	Level string `mapstructure:"level"`

	// File is the log file path
	// File 是日志文件路径
	File string `mapstructure:"file"`

	// MaxSize is the maximum size of log file in MB before rotation
	// MaxSize 是日志文件轮转前的最大大小（MB）
	MaxSize int `mapstructure:"max_size"`

	// MaxBackups is the maximum number of old log files to retain
	// MaxBackups 是保留的旧日志文件的最大数量
	MaxBackups int `mapstructure:"max_backups"`

	// MaxAge is the maximum number of days to retain old log files
	// MaxAge 是保留旧日志文件的最大天数
	MaxAge int `mapstructure:"max_age"`
}

// SeaTunnelConfig contains SeaTunnel-related settings
// SeaTunnelConfig 包含 SeaTunnel 相关设置
// Note: SeaTunnel manages its own config and log directories internally
// 注意：SeaTunnel 内部自己管理配置目录和日志目录
type SeaTunnelConfig struct {
	// InstallDir is the default SeaTunnel installation directory
	// InstallDir 是默认的 SeaTunnel 安装目录
	// This is only used as a fallback; actual install_dir is specified
	// by Control Plane commands when installing SeaTunnel
	// 这只是作为后备使用；实际的 install_dir 在安装 SeaTunnel 时由 Control Plane 命令指定
	InstallDir string `mapstructure:"install_dir"`
}

// Load loads configuration from file and environment variables
// Load 从文件和环境变量加载配置
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set default values / 设置默认值
	setDefaults(v)

	// Set config file path / 设置配置文件路径
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Check environment variable / 检查环境变量
		envPath := os.Getenv("AGENT_CONFIG_PATH")
		if envPath != "" {
			v.SetConfigFile(envPath)
		} else {
			v.SetConfigFile(DefaultConfigPath)
		}
	}

	// Enable environment variable override / 启用环境变量覆盖
	v.SetEnvPrefix("AGENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file / 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		// Config file not found is not an error if we have defaults
		// 如果有默认值，配置文件未找到不是错误
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			// Check if file exists / 检查文件是否存在
			if _, statErr := os.Stat(v.ConfigFileUsed()); statErr == nil {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
			// File doesn't exist, use defaults / 文件不存在，使用默认值
		}
	}

	// Unmarshal config / 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// setDefaults sets default configuration values
// setDefaults 设置默认配置值
func setDefaults(v *viper.Viper) {
	// Agent defaults / Agent 默认值
	v.SetDefault("agent.id", "")

	// Control Plane defaults / Control Plane 默认值
	v.SetDefault("control_plane.addresses", []string{})
	v.SetDefault("control_plane.tls.enabled", false)
	v.SetDefault("control_plane.token", "")

	// Heartbeat defaults / 心跳默认值
	v.SetDefault("heartbeat.interval", DefaultHeartbeatInterval)

	// Log defaults / 日志默认值
	v.SetDefault("log.level", DefaultLogLevel)
	v.SetDefault("log.file", DefaultLogFile)
	v.SetDefault("log.max_size", DefaultLogMaxSize)
	v.SetDefault("log.max_backups", DefaultLogMaxBackups)
	v.SetDefault("log.max_age", DefaultLogMaxAge)

	// SeaTunnel defaults / SeaTunnel 默认值
	// Note: config_dir and log_dir are automatically derived from install_dir
	// 注意：config_dir 和 log_dir 自动基于 install_dir 计算
	v.SetDefault("seatunnel.install_dir", DefaultSeaTunnelInstallDir)
}

// Validate validates the configuration
// Validate 验证配置
func (c *Config) Validate() error {
	// Validate Control Plane addresses / 验证 Control Plane 地址
	if len(c.ControlPlane.Addresses) == 0 {
		return errors.New("control_plane.addresses is required")
	}

	// Validate TLS: one-way TLS only needs ca_file; client cert/key are optional for mTLS.
	// 校验 TLS：单向 TLS 仅需 ca_file；客户端 cert/key 可选（留给 mTLS）。
	if c.ControlPlane.TLS.Enabled {
		if c.ControlPlane.TLS.CAFile == "" {
			return errors.New("control_plane.tls.ca_file is required when TLS is enabled")
		}
		certSet := c.ControlPlane.TLS.CertFile != ""
		keySet := c.ControlPlane.TLS.KeyFile != ""
		if certSet != keySet {
			return errors.New("control_plane.tls.cert_file and key_file must both be set for mTLS (or both empty for one-way TLS)")
		}
	}

	// Validate log level / 验证日志级别
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.Log.Level)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Log.Level)
	}

	// Validate heartbeat interval / 验证心跳间隔
	if c.Heartbeat.Interval < time.Second {
		return errors.New("heartbeat.interval must be at least 1 second")
	}

	return nil
}

// String returns a string representation of the config (for debugging)
// String 返回配置的字符串表示（用于调试）
func (c *Config) String() string {
	return fmt.Sprintf(
		"Config{Agent.ID: %s, ControlPlane.Addresses: %v, Heartbeat.Interval: %v, Log.Level: %s}",
		c.Agent.ID,
		c.ControlPlane.Addresses,
		c.Heartbeat.Interval,
		c.Log.Level,
	)
}

// ToYAML serializes the configuration to YAML format
// ToYAML 将配置序列化为 YAML 格式
func (c *Config) ToYAML() ([]byte, error) {
	// Build YAML structure manually to ensure proper formatting
	// 手动构建 YAML 结构以确保正确的格式
	yamlContent := fmt.Sprintf(`agent:
  id: "%s"

control_plane:
  addresses:
%s  tls:
    enabled: %t
    cert_file: "%s"
    key_file: "%s"
    ca_file: "%s"
  token: "%s"

heartbeat:
  interval: %s

log:
  level: "%s"
  file: "%s"
  max_size: %d
  max_backups: %d
  max_age: %d

seatunnel:
  # Note: config_dir and log_dir are automatically derived from install_dir
  # 注意：config_dir 和 log_dir 自动基于 install_dir 计算
  install_dir: "%s"
`,
		c.Agent.ID,
		formatAddresses(c.ControlPlane.Addresses),
		c.ControlPlane.TLS.Enabled,
		c.ControlPlane.TLS.CertFile,
		c.ControlPlane.TLS.KeyFile,
		c.ControlPlane.TLS.CAFile,
		c.ControlPlane.Token,
		c.Heartbeat.Interval.String(),
		c.Log.Level,
		c.Log.File,
		c.Log.MaxSize,
		c.Log.MaxBackups,
		c.Log.MaxAge,
		c.SeaTunnel.InstallDir,
	)
	return []byte(yamlContent), nil
}

// formatAddresses formats addresses slice for YAML output
// formatAddresses 格式化地址切片用于 YAML 输出
func formatAddresses(addresses []string) string {
	if len(addresses) == 0 {
		return "    []\n"
	}
	result := ""
	for _, addr := range addresses {
		result += fmt.Sprintf("    - \"%s\"\n", addr)
	}
	return result
}

// LoadFromYAML loads configuration from YAML bytes
// LoadFromYAML 从 YAML 字节加载配置
func LoadFromYAML(yamlData []byte) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	// Set defaults first / 首先设置默认值
	setDefaults(v)

	// Read from bytes / 从字节读取
	if err := v.ReadConfig(strings.NewReader(string(yamlData))); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Unmarshal config / 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// Equal compares two configs for equality
// Equal 比较两个配置是否相等
func (c *Config) Equal(other *Config) bool {
	if c == nil || other == nil {
		return c == other
	}

	// Compare Agent / 比较 Agent
	if c.Agent.ID != other.Agent.ID {
		return false
	}

	// Compare ControlPlane / 比较 ControlPlane
	if len(c.ControlPlane.Addresses) != len(other.ControlPlane.Addresses) {
		return false
	}
	for i, addr := range c.ControlPlane.Addresses {
		if addr != other.ControlPlane.Addresses[i] {
			return false
		}
	}
	if c.ControlPlane.TLS.Enabled != other.ControlPlane.TLS.Enabled ||
		c.ControlPlane.TLS.CertFile != other.ControlPlane.TLS.CertFile ||
		c.ControlPlane.TLS.KeyFile != other.ControlPlane.TLS.KeyFile ||
		c.ControlPlane.TLS.CAFile != other.ControlPlane.TLS.CAFile {
		return false
	}
	if c.ControlPlane.Token != other.ControlPlane.Token {
		return false
	}

	// Compare Heartbeat / 比较 Heartbeat
	if c.Heartbeat.Interval != other.Heartbeat.Interval {
		return false
	}

	// Compare Log / 比较 Log
	if c.Log.Level != other.Log.Level ||
		c.Log.File != other.Log.File ||
		c.Log.MaxSize != other.Log.MaxSize ||
		c.Log.MaxBackups != other.Log.MaxBackups ||
		c.Log.MaxAge != other.Log.MaxAge {
		return false
	}

	// Compare SeaTunnel / 比较 SeaTunnel
	// Note: Only compare InstallDir since ConfigDir and LogDir are derived from it
	// 注意：只比较 InstallDir，因为 ConfigDir 和 LogDir 是从它派生的
	if c.SeaTunnel.InstallDir != other.SeaTunnel.InstallDir {
		return false
	}

	return true
}

// LoadWithPriority loads configuration with explicit priority handling
// LoadWithPriority 使用显式优先级处理加载配置
// Priority: cmdArgs > envVars > configFile > defaults
// 优先级：命令行参数 > 环境变量 > 配置文件 > 默认值
func LoadWithPriority(configPath string, cmdArgs map[string]interface{}) (*Config, error) {
	v := viper.New()

	// Set default values / 设置默认值
	setDefaults(v)

	// Set config file path / 设置配置文件路径
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Check environment variable / 检查环境变量
		envPath := os.Getenv("AGENT_CONFIG_PATH")
		if envPath != "" {
			v.SetConfigFile(envPath)
		} else {
			v.SetConfigFile(DefaultConfigPath)
		}
	}

	// Enable environment variable override / 启用环境变量覆盖
	v.SetEnvPrefix("AGENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file / 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			if _, statErr := os.Stat(v.ConfigFileUsed()); statErr == nil {
				return nil, fmt.Errorf("failed to read config file: %w", err)
			}
		}
	}

	// Apply command line arguments (highest priority)
	// 应用命令行参数（最高优先级）
	for key, value := range cmdArgs {
		v.Set(key, value)
	}

	// Unmarshal config / 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}
