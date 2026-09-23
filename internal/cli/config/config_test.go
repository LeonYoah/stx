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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreSaveUsesRestrictedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "stx", "config.yaml")
	store := NewStore(path)
	if err := store.Upsert("production", Namespace{Server: "https://stx.example.com", Token: "secret"}, true); err != nil {
		t.Fatalf("保存命名空间失败 / saving namespace failed: %v", err)
	}

	directoryInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("读取配置目录失败 / stat config directory failed: %v", err)
	}
	if permission := directoryInfo.Mode().Perm(); permission != 0o700 {
		t.Fatalf("配置目录权限错误 / unexpected config directory permission: %o", permission)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取配置文件失败 / stat config file failed: %v", err)
	}
	if permission := fileInfo.Mode().Perm(); permission != 0o600 {
		t.Fatalf("配置文件权限错误 / unexpected config file permission: %o", permission)
	}
}

func TestStoreListUseAndDelete(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	if err := store.Upsert("production", Namespace{Server: "https://prod.example.com"}, true); err != nil {
		t.Fatalf("保存 production 失败 / saving production failed: %v", err)
	}
	if err := store.Upsert("development", Namespace{Server: "https://dev.example.com"}, false); err != nil {
		t.Fatalf("保存 development 失败 / saving development failed: %v", err)
	}

	items, err := store.List()
	if err != nil {
		t.Fatalf("列出命名空间失败 / listing namespaces failed: %v", err)
	}
	if len(items) != 2 || items[0].Name != "development" || items[1].Name != "production" {
		t.Fatalf("命名空间排序错误 / namespaces are not sorted: %#v", items)
	}
	if err := store.Use("development"); err != nil {
		t.Fatalf("切换命名空间失败 / selecting namespace failed: %v", err)
	}
	if err := store.Delete("development"); err != nil {
		t.Fatalf("删除命名空间失败 / deleting namespace failed: %v", err)
	}
	file, err := store.Load()
	if err != nil {
		t.Fatalf("重新加载配置失败 / reloading config failed: %v", err)
	}
	if file.CurrentNamespace != "production" {
		t.Fatalf("删除当前命名空间后应选择剩余项 / deleting current namespace must select a remaining one: %s", file.CurrentNamespace)
	}
}

func TestStoreResolvePriority(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	store.getenv = func(key string) string {
		values := map[string]string{
			"STX_NAMESPACE": "production",
			"STX_SERVER":    "https://env.example.com",
			"STX_TOKEN":     "env-token",
			"STX_OUTPUT":    "yaml",
			"STX_TIMEOUT":   "45s",
		}
		return values[key]
	}
	if err := store.Upsert("production", Namespace{
		Server:  "https://file.example.com",
		Token:   "file-token",
		Output:  "table",
		Timeout: "20s",
	}, true); err != nil {
		t.Fatalf("保存命名空间失败 / saving namespace failed: %v", err)
	}

	resolved, err := store.Resolve(Overrides{Server: "https://flag.example.com", Output: "json"})
	if err != nil {
		t.Fatalf("解析配置失败 / resolving config failed: %v", err)
	}
	if resolved.Name != "production" || resolved.Server != "https://flag.example.com" {
		t.Fatalf("命令参数优先级错误 / command override priority is wrong: %#v", resolved)
	}
	if resolved.Token != "env-token" || resolved.Output != "json" || resolved.Timeout != 45*time.Second {
		t.Fatalf("环境变量或默认优先级错误 / environment or default priority is wrong: %#v", resolved)
	}
}

func TestStoreGetMissingNamespace(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	_, _, err := store.Get("missing")
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("应返回命名空间不存在错误 / expected namespace-not-found error: %v", err)
	}
}
