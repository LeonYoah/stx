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

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadConfig tests configuration loading
// TestLoadConfig 测试配置加载
func TestLoadConfig(t *testing.T) {
	// Create a temporary config file / 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
agent:
  id: "test-agent-001"

control_plane:
  addresses:
    - "localhost:9090"
  tls:
    enabled: false
  token: "test-token"

heartbeat:
  interval: 15s

log:
  level: debug
  file: /tmp/agent.log
  max_size: 50
  max_backups: 5
  max_age: 14

seatunnel:
  install_dir: /opt/seatunnel
  config_dir: /opt/seatunnel/config
  log_dir: /opt/seatunnel/logs
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config / 加载配置
	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify values / 验证值
	assert.Equal(t, "test-agent-001", cfg.Agent.ID)
	assert.Equal(t, []string{"localhost:9090"}, cfg.ControlPlane.Addresses)
	assert.False(t, cfg.ControlPlane.TLS.Enabled)
	assert.Equal(t, "test-token", cfg.ControlPlane.Token)
	assert.Equal(t, 15*time.Second, cfg.Heartbeat.Interval)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "/tmp/agent.log", cfg.Log.File)
	assert.Equal(t, 50, cfg.Log.MaxSize)
	assert.Equal(t, 5, cfg.Log.MaxBackups)
	assert.Equal(t, 14, cfg.Log.MaxAge)
	assert.Equal(t, "/opt/seatunnel", cfg.SeaTunnel.InstallDir)
}

// TestLoadConfigDefaults tests default configuration values
// TestLoadConfigDefaults 测试默认配置值
func TestLoadConfigDefaults(t *testing.T) {
	// Create a minimal config file / 创建最小配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
control_plane:
  addresses:
    - "localhost:9090"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Load config / 加载配置
	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify default values / 验证默认值
	assert.Equal(t, "", cfg.Agent.ID)
	assert.Equal(t, DefaultHeartbeatInterval, cfg.Heartbeat.Interval)
	assert.Equal(t, DefaultLogLevel, cfg.Log.Level)
	assert.Equal(t, DefaultLogFile, cfg.Log.File)
	assert.Equal(t, DefaultLogMaxSize, cfg.Log.MaxSize)
	assert.Equal(t, DefaultLogMaxBackups, cfg.Log.MaxBackups)
	assert.Equal(t, DefaultLogMaxAge, cfg.Log.MaxAge)
	assert.Equal(t, DefaultSeaTunnelInstallDir, cfg.SeaTunnel.InstallDir)
}

// TestValidateConfig tests configuration validation
// TestValidateConfig 测试配置验证
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{"localhost:9090"},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 10 * time.Second,
				},
				Log: LogConfig{
					Level: "info",
				},
			},
			wantErr: false,
		},
		{
			name: "missing control plane addresses",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 10 * time.Second,
				},
				Log: LogConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "control_plane.addresses is required",
		},
		{
			name: "TLS enabled without ca file",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{"localhost:9090"},
					TLS: TLSConfig{
						Enabled: true,
					},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 10 * time.Second,
				},
				Log: LogConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "control_plane.tls.ca_file is required when TLS is enabled",
		},
		{
			name: "TLS one-way with only ca file",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{"localhost:9090"},
					TLS: TLSConfig{
						Enabled: true,
						CAFile:  "/etc/stx-agent/certs/ca.crt",
					},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 10 * time.Second,
				},
				Log: LogConfig{
					Level: "info",
				},
			},
			wantErr: false,
		},
		{
			name: "TLS mTLS missing key file",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{"localhost:9090"},
					TLS: TLSConfig{
						Enabled:  true,
						CAFile:   "/etc/stx-agent/certs/ca.crt",
						CertFile: "/path/to/cert.pem",
					},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 10 * time.Second,
				},
				Log: LogConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "cert_file and key_file must both be set",
		},
		{
			name: "invalid log level",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{"localhost:9090"},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 10 * time.Second,
				},
				Log: LogConfig{
					Level: "invalid",
				},
			},
			wantErr: true,
			errMsg:  "invalid log level",
		},
		{
			name: "heartbeat interval too short",
			config: &Config{
				ControlPlane: ControlPlaneConfig{
					Addresses: []string{"localhost:9090"},
				},
				Heartbeat: HeartbeatConfig{
					Interval: 500 * time.Millisecond,
				},
				Log: LogConfig{
					Level: "info",
				},
			},
			wantErr: true,
			errMsg:  "heartbeat.interval must be at least 1 second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestConfigString tests the String method
// TestConfigString 测试 String 方法
func TestConfigString(t *testing.T) {
	cfg := &Config{
		Agent: AgentConfig{
			ID: "test-agent",
		},
		ControlPlane: ControlPlaneConfig{
			Addresses: []string{"localhost:9090"},
		},
		Heartbeat: HeartbeatConfig{
			Interval: 10 * time.Second,
		},
		Log: LogConfig{
			Level: "info",
		},
	}

	str := cfg.String()
	assert.Contains(t, str, "test-agent")
	assert.Contains(t, str, "localhost:9090")
	assert.Contains(t, str, "10s")
	assert.Contains(t, str, "info")
}

// TestLoadConfigFromEnv tests loading config from environment variables
// TestLoadConfigFromEnv 测试从环境变量加载配置
func TestLoadConfigFromEnv(t *testing.T) {
	// Create a minimal config file / 创建最小配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
control_plane:
  addresses:
    - "localhost:9090"
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	// Set environment variable / 设置环境变量
	os.Setenv("AGENT_LOG_LEVEL", "debug")
	defer os.Unsetenv("AGENT_LOG_LEVEL")

	// Load config / 加载配置
	cfg, err := Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Environment variable should override default
	// 环境变量应该覆盖默认值
	assert.Equal(t, "debug", cfg.Log.Level)
}
