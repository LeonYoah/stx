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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfigMissingFileReturnsDefaultsAndError(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "missing.yaml")
	t.Setenv("CONFIG_PATH", configPath)
	t.Setenv("STX_DATABASE_TYPE", "")
	t.Setenv("STX_DATABASE_SQLITE_PATH", "")

	loaded, err := loadConfig()
	if err == nil {
		t.Fatalf("缺少服务端配置时应保留错误 / missing server config must preserve an error")
	}
	if loaded == nil {
		t.Fatalf("缺少服务端配置时仍应返回 CLI 默认配置 / missing server config must return CLI defaults")
	}
	if loaded.Database.Type != "sqlite" || loaded.Database.SQLitePath == "" {
		t.Fatalf("默认数据库配置不完整 / default database config is incomplete: %#v", loaded.Database)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("加载配置不应创建服务端配置文件 / loading must not create the server config file: %v", statErr)
	}
}

func TestLoadConfigPreservesExplicitObservabilityDisabled(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte("observability:\n  enabled: false\n")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatalf("写入测试配置失败 / writing test config failed: %v", err)
	}
	t.Setenv("CONFIG_PATH", configPath)

	loaded, err := loadConfig()
	if err != nil {
		t.Fatalf("加载测试配置失败 / loading test config failed: %v", err)
	}
	if loaded.Observability.Enabled {
		t.Fatalf("显式关闭可观测性时不应被默认值覆盖 / explicit observability disablement must be preserved")
	}
}

// TestLoadConfig_IgnoresLegacyObservabilitySections 锁住破坏性升级的兼容面：旧配置文件残留
// prometheus / alertmanager / grafana / remote_ingest 段时仍能加载，且不影响内置监控配置。
// TestLoadConfig_IgnoresLegacyObservabilitySections pins the breaking-upgrade compatibility: old
// config files that still carry prometheus / alertmanager / grafana / remote_ingest sections keep
// loading and do not disturb the built-in monitoring settings.
func TestLoadConfig_IgnoresLegacyObservabilitySections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `app:
  external_url: "http://127.0.0.1:17800"
observability:
  enabled: true
  prometheus:
    url: "http://127.0.0.1:9090"
    http_sd_path: "/api/v1/monitoring/prometheus/discovery"
  alertmanager:
    url: "http://127.0.0.1:9093"
    webhook_path: "/api/v1/monitoring/alertmanager/webhook"
  grafana:
    url: "http://127.0.0.1:3000"
  remote_ingest:
    path: "/api/v1/monitoring/alertmanager/webhook"
    token: "secret-token-value"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("CONFIG_PATH", path)

	// 捕获 stderr：残留段只应产出一条列出段名的 WARN，token 等配置值不落日志。
	// Capture stderr: leftovers should yield one WARN naming the sections only; values like the
	// token must stay out of logs.
	stderr := captureStderr(t, func() {
		loaded, err := loadConfig()
		if err != nil {
			t.Fatalf("旧配置段不应阻止加载 / legacy sections must not block loading: %v", err)
		}
		if !loaded.Observability.Enabled || loaded.Observability.Storage.Retention == "" {
			t.Fatalf("内置监控配置应取默认值 / built-in monitoring should take defaults: %+v", loaded.Observability)
		}
	})
	for _, section := range []string{"observability.prometheus", "observability.alertmanager", "observability.grafana", "observability.remote_ingest"} {
		if !strings.Contains(stderr, section) {
			t.Fatalf("WARN 应列出残留段 %s / WARN should list leftover section %s: %q", section, section, stderr)
		}
	}
	if strings.Contains(stderr, "secret-token-value") {
		t.Fatalf("WARN 不应泄露配置值 / WARN must not leak config values: %q", stderr)
	}
}

// TestWarnLegacyObservabilitySections_None 验证没有残留段时不产生 WARN 噪音。
// TestWarnLegacyObservabilitySections_None verifies no WARN noise when no legacy section remains.
func TestWarnLegacyObservabilitySections_None(t *testing.T) {
	loader := viper.New()
	loader.SetConfigType("yaml")
	if err := loader.ReadConfig(strings.NewReader("observability:\n  enabled: true\n")); err != nil {
		t.Fatalf("read config: %v", err)
	}
	stderr := captureStderr(t, func() {
		warnLegacyObservabilitySections(loader)
	})
	if strings.Contains(stderr, "WARN") {
		t.Fatalf("无残留段不应输出 WARN / no WARN expected without leftover sections: %q", stderr)
	}
}

// captureStderr 执行 fn 并返回其向 stderr 写入的内容。
// captureStderr runs fn and returns everything it wrote to stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stderr: %v", err)
	}
	return string(out)
}
