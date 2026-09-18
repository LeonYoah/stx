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
