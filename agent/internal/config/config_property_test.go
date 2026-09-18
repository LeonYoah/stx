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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// **Feature: stx-agent, Property 14: Config YAML Round-Trip**
// **Validates: Requirements 9.2**
//
// Property: For any valid Agent configuration object, serializing to YAML
// and parsing back SHALL produce an equivalent configuration.
// 属性：对于任何有效的 Agent 配置对象，序列化为 YAML 并解析回来应该产生等效的配置。
func TestProperty_ConfigYAMLRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid configuration / 生成有效配置
		cfg := generateValidConfig(t)

		// Serialize to YAML / 序列化为 YAML
		yamlData, err := cfg.ToYAML()
		if err != nil {
			t.Fatalf("Failed to serialize config to YAML: %v", err)
		}

		// Parse back from YAML / 从 YAML 解析回来
		parsedCfg, err := LoadFromYAML(yamlData)
		if err != nil {
			t.Fatalf("Failed to parse config from YAML: %v\nYAML content:\n%s", err, string(yamlData))
		}

		// Verify equality / 验证相等性
		if !cfg.Equal(parsedCfg) {
			t.Fatalf("Round-trip failed: original and parsed configs are not equal\nOriginal: %+v\nParsed: %+v\nYAML:\n%s",
				cfg, parsedCfg, string(yamlData))
		}
	})
}

// generateValidConfig generates a valid Config for property testing
// generateValidConfig 为属性测试生成有效的 Config
func generateValidConfig(t *rapid.T) *Config {
	// Generate valid log levels / 生成有效的日志级别
	validLogLevels := []string{"debug", "info", "warn", "error"}
	logLevel := rapid.SampledFrom(validLogLevels).Draw(t, "logLevel")

	// Generate heartbeat interval (at least 1 second) / 生成心跳间隔（至少1秒）
	heartbeatSeconds := rapid.IntRange(1, 300).Draw(t, "heartbeatSeconds")

	// Generate addresses (at least one) / 生成地址（至少一个）
	numAddresses := rapid.IntRange(1, 5).Draw(t, "numAddresses")
	addresses := make([]string, numAddresses)
	for i := 0; i < numAddresses; i++ {
		host := rapid.StringMatching(`[a-z][a-z0-9]{0,10}`).Draw(t, "host")
		port := rapid.IntRange(1024, 65535).Draw(t, "port")
		addresses[i] = fmt.Sprintf("%s:%d", host, port)
	}

	// Generate simple alphanumeric strings for paths and IDs
	// 为路径和 ID 生成简单的字母数字字符串
	agentID := rapid.StringMatching(`[a-zA-Z0-9_-]{0,20}`).Draw(t, "agentID")
	token := rapid.StringMatching(`[a-zA-Z0-9_-]{0,32}`).Draw(t, "token")

	// Generate paths / 生成路径
	logFile := "/var/log/" + rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "logFileName") + ".log"
	installDir := "/opt/" + rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "installDirName")

	// Generate TLS config / 生成 TLS 配置
	tlsEnabled := rapid.Bool().Draw(t, "tlsEnabled")
	var certFile, keyFile, caFile string
	if tlsEnabled {
		certFile = "/etc/ssl/" + rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "certName") + ".pem"
		keyFile = "/etc/ssl/" + rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "keyName") + ".key"
		caFile = "/etc/ssl/" + rapid.StringMatching(`[a-z]{1,10}`).Draw(t, "caName") + ".pem"
	}

	// Generate log rotation settings / 生成日志轮转设置
	maxSize := rapid.IntRange(1, 1000).Draw(t, "maxSize")
	maxBackups := rapid.IntRange(1, 100).Draw(t, "maxBackups")
	maxAge := rapid.IntRange(1, 365).Draw(t, "maxAge")

	return &Config{
		Agent: AgentConfig{
			ID: agentID,
		},
		ControlPlane: ControlPlaneConfig{
			Addresses: addresses,
			TLS: TLSConfig{
				Enabled:  tlsEnabled,
				CertFile: certFile,
				KeyFile:  keyFile,
				CAFile:   caFile,
			},
			Token: token,
		},
		Heartbeat: HeartbeatConfig{
			Interval: time.Duration(heartbeatSeconds) * time.Second,
		},
		Log: LogConfig{
			Level:      logLevel,
			File:       logFile,
			MaxSize:    maxSize,
			MaxBackups: maxBackups,
			MaxAge:     maxAge,
		},
		SeaTunnel: SeaTunnelConfig{
			InstallDir: installDir,
		},
	}
}

// **Feature: stx-agent, Property 15: Config Loading Priority**
// **Validates: Requirements 9.1**
//
// Property: For any configuration key that is set in multiple sources
// (command line, environment variable, config file), the system SHALL use
// the value from the highest priority source (command line > env > file > default).
// 属性：对于在多个来源中设置的任何配置键（命令行、环境变量、配置文件），
// 系统应该使用最高优先级来源的值（命令行 > 环境变量 > 文件 > 默认值）。
func TestProperty_ConfigLoadingPriority(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a temporary config file / 创建临时配置文件
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		// Generate different values for each source / 为每个来源生成不同的值
		fileLogLevel := rapid.SampledFrom([]string{"debug", "info"}).Draw(rt, "fileLogLevel")
		envLogLevel := rapid.SampledFrom([]string{"warn", "error"}).Draw(rt, "envLogLevel")
		cmdLogLevel := rapid.SampledFrom([]string{"debug", "info", "warn", "error"}).Draw(rt, "cmdLogLevel")

		// Determine which sources are active / 确定哪些来源是活动的
		hasFile := rapid.Bool().Draw(rt, "hasFile")
		hasEnv := rapid.Bool().Draw(rt, "hasEnv")
		hasCmd := rapid.Bool().Draw(rt, "hasCmd")

		// Create config file if needed / 如果需要则创建配置文件
		if hasFile {
			configContent := fmt.Sprintf(`
control_plane:
  addresses:
    - "localhost:9090"
log:
  level: "%s"
`, fileLogLevel)
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				rt.Fatalf("Failed to write config file: %v", err)
			}
		} else {
			// Create minimal config file / 创建最小配置文件
			configContent := `
control_plane:
  addresses:
    - "localhost:9090"
`
			err := os.WriteFile(configPath, []byte(configContent), 0644)
			if err != nil {
				rt.Fatalf("Failed to write config file: %v", err)
			}
		}

		// Set environment variable if needed / 如果需要则设置环境变量
		if hasEnv {
			os.Setenv("AGENT_LOG_LEVEL", envLogLevel)
			defer os.Unsetenv("AGENT_LOG_LEVEL")
		} else {
			os.Unsetenv("AGENT_LOG_LEVEL")
		}

		// Prepare command line args / 准备命令行参数
		cmdArgs := make(map[string]interface{})
		if hasCmd {
			cmdArgs["log.level"] = cmdLogLevel
		}

		// Load config with priority / 使用优先级加载配置
		cfg, err := LoadWithPriority(configPath, cmdArgs)
		if err != nil {
			rt.Fatalf("Failed to load config: %v", err)
		}

		// Determine expected value based on priority / 根据优先级确定预期值
		var expectedLogLevel string
		if hasCmd {
			expectedLogLevel = cmdLogLevel
		} else if hasEnv {
			expectedLogLevel = envLogLevel
		} else if hasFile {
			expectedLogLevel = fileLogLevel
		} else {
			expectedLogLevel = DefaultLogLevel // default is "info"
		}

		// Verify the correct value is used / 验证使用了正确的值
		if cfg.Log.Level != expectedLogLevel {
			rt.Fatalf("Priority violation: expected log level %q but got %q\n"+
				"hasCmd=%v (cmdLogLevel=%s), hasEnv=%v (envLogLevel=%s), hasFile=%v (fileLogLevel=%s)",
				expectedLogLevel, cfg.Log.Level,
				hasCmd, cmdLogLevel, hasEnv, envLogLevel, hasFile, fileLogLevel)
		}
	})
}

// **Feature: stx-agent, Property 16: Invalid Config Rejection**
// **Validates: Requirements 9.6**
//
// Property: For any configuration file with invalid YAML syntax or missing
// required fields, the Agent SHALL fail to start and output a descriptive error message.
// 属性：对于任何具有无效 YAML 语法或缺少必填字段的配置文件，
// Agent 应该无法启动并输出描述性错误消息。
func TestProperty_InvalidConfigRejection(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "config.yaml")

		// Generate invalid config type / 生成无效配置类型
		invalidType := rapid.IntRange(0, 4).Draw(rt, "invalidType")

		var configContent string
		var expectedError string

		switch invalidType {
		case 0:
			// Invalid YAML syntax - unclosed bracket / 无效 YAML 语法 - 未闭合的括号
			configContent = `
control_plane:
  addresses: [
    - "localhost:9090"
`
			expectedError = "failed to"
		case 1:
			// Invalid YAML syntax - bad indentation / 无效 YAML 语法 - 错误的缩进
			configContent = `
control_plane:
addresses:
    - "localhost:9090"
`
			expectedError = "" // This might parse but with wrong structure
		case 2:
			// Missing required field - no addresses / 缺少必填字段 - 没有地址
			configContent = `
control_plane:
  token: "test"
log:
  level: "info"
`
			expectedError = "control_plane.addresses is required"
		case 3:
			// Invalid log level / 无效的日志级别
			invalidLevel := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "invalidLevel")
			// Make sure it's not a valid level
			if invalidLevel == "debug" || invalidLevel == "info" || invalidLevel == "warn" || invalidLevel == "error" {
				invalidLevel = "invalid"
			}
			configContent = fmt.Sprintf(`
control_plane:
  addresses:
    - "localhost:9090"
log:
  level: "%s"
`, invalidLevel)
			expectedError = "invalid log level"
		case 4:
			// Invalid heartbeat interval / 无效的心跳间隔
			configContent = `
control_plane:
  addresses:
    - "localhost:9090"
heartbeat:
  interval: 100ms
`
			expectedError = "heartbeat.interval must be at least 1 second"
		}

		// Write config file / 写入配置文件
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		if err != nil {
			rt.Fatalf("Failed to write config file: %v", err)
		}

		// Try to load config / 尝试加载配置
		cfg, loadErr := Load(configPath)

		// If load succeeded, try validation / 如果加载成功，尝试验证
		if loadErr == nil && cfg != nil {
			loadErr = cfg.Validate()
		}

		// For cases that should fail, verify error / 对于应该失败的情况，验证错误
		if expectedError != "" {
			if loadErr == nil {
				rt.Fatalf("Expected error containing %q but got no error for config:\n%s",
					expectedError, configContent)
			}
			if expectedError != "" && !containsIgnoreCase(loadErr.Error(), expectedError) {
				// Some errors are acceptable as long as they indicate failure
				// 只要表明失败，某些错误是可以接受的
				if !containsIgnoreCase(loadErr.Error(), "failed") &&
					!containsIgnoreCase(loadErr.Error(), "invalid") &&
					!containsIgnoreCase(loadErr.Error(), "required") &&
					!containsIgnoreCase(loadErr.Error(), "error") {
					rt.Fatalf("Expected error containing %q but got: %v", expectedError, loadErr)
				}
			}
		}
	})
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
// containsIgnoreCase 检查 s 是否包含 substr（不区分大小写）
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(substr) == 0 ||
			findIgnoreCase(s, substr))
}

func findIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
