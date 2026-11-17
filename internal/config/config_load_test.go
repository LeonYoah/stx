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

	// 加载只记录检测结果、不向 stderr 打 WARN（WARN 由服务启动时统一记一次，CLI 命令不带噪音）。
	// Loading only records the detection result without writing a WARN to stderr (the WARN is
	// logged once at server startup, so CLI commands stay quiet).
	stderr := captureStderr(t, func() {
		loaded, err := loadConfig()
		if err != nil {
			t.Fatalf("旧配置段不应阻止加载 / legacy sections must not block loading: %v", err)
		}
		if !loaded.Observability.Enabled || loaded.Observability.Storage.Retention == "" {
			t.Fatalf("内置监控配置应取默认值 / built-in monitoring should take defaults: %+v", loaded.Observability)
		}
	})
	if strings.Contains(stderr, "WARN") {
		t.Fatalf("加载配置不应向 stderr 打 WARN / loading must not write WARN to stderr: %q", stderr)
	}
	detected := LegacyObservabilitySections()
	for _, section := range []string{"observability.prometheus", "observability.alertmanager", "observability.grafana", "observability.remote_ingest"} {
		found := false
		for _, got := range detected {
			if got == section {
				found = true
			}
			if strings.Contains(got, "secret-token-value") {
				t.Fatalf("检测结果不应泄露配置值 / detected sections must not leak config values: %v", detected)
			}
		}
		if !found {
			t.Fatalf("应检测到残留段 %s / should detect leftover section %s: %v", section, section, detected)
		}
	}
}

// TestDetectLegacyObservabilitySections_None 验证没有残留段时检测结果为空。
// TestDetectLegacyObservabilitySections_None verifies detection is empty when no legacy section remains.
func TestDetectLegacyObservabilitySections_None(t *testing.T) {
	loader := viper.New()
	loader.SetConfigType("yaml")
	if err := loader.ReadConfig(strings.NewReader("observability:\n  enabled: true\n")); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := detectLegacyObservabilitySections(loader); len(got) != 0 {
		t.Fatalf("无残留段不应有检测结果 / no detection expected without leftover sections: %v", got)
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
