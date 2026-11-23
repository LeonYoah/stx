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

package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestClusterConfigScan_AcceptsBytesStringNilAndRejectsInvalid(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		var cfg ClusterConfig
		if err := cfg.Scan(nil); err != nil {
			t.Fatalf("nil scan: %v", err)
		}
		if cfg != nil {
			t.Fatalf("expected nil config, got %#v", cfg)
		}
	})

	t.Run("bytes", func(t *testing.T) {
		var cfg ClusterConfig
		if err := cfg.Scan([]byte(`{"k":"v"}`)); err != nil {
			t.Fatalf("bytes scan: %v", err)
		}
		if cfg["k"] != "v" {
			t.Fatalf("got %#v", cfg)
		}
	})

	t.Run("string", func(t *testing.T) {
		var cfg ClusterConfig
		if err := cfg.Scan(`{"k":"v"}`); err != nil {
			t.Fatalf("string scan: %v", err)
		}
		if cfg["k"] != "v" {
			t.Fatalf("got %#v", cfg)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		var cfg ClusterConfig
		if err := cfg.Scan(`{not-json`); err == nil {
			t.Fatal("expected invalid JSON error")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		var cfg ClusterConfig
		if err := cfg.Scan(42); err == nil {
			t.Fatal("expected unsupported type error")
		}
	})
}

func TestListWithWarnings_TextAndInvalidConfigDoNotFailPage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	ctx := context.Background()

	healthy := &Cluster{
		Name:           "healthy-cluster",
		ClusterCode:    "healthy01",
		DeploymentMode: DeploymentModeHybrid,
		Version:        "2.3.12",
		Status:         ClusterStatusRunning,
		InstallDir:     "/opt/seatunnel",
		Config:         ClusterConfig{"port": float64(8080)},
		CreatedAt:      time.Now().Add(-2 * time.Hour),
	}
	if err := db.Create(healthy).Error; err != nil {
		t.Fatalf("create healthy: %v", err)
	}

	textCfg := &Cluster{
		Name:           "text-config-cluster",
		ClusterCode:    "textcfg01",
		DeploymentMode: DeploymentModeHybrid,
		Version:        "2.3.12",
		Status:         ClusterStatusRunning,
		InstallDir:     "/opt/seatunnel",
		CreatedAt:      time.Now().Add(-time.Hour),
	}
	if err := db.Omit("Config").Create(textCfg).Error; err != nil {
		t.Fatalf("create text row shell: %v", err)
	}
	// 用 Python json.dumps 风格：TEXT 亲和性写入合法 JSON 字符串。
	// Mimic Python json.dumps into a TEXT-affinity column with valid JSON.
	if err := db.Exec("UPDATE clusters SET config = CAST(? AS TEXT) WHERE id = ?", `{"from":"text"}`, textCfg.ID).Error; err != nil {
		t.Fatalf("write TEXT config: %v", err)
	}

	badCfg := &Cluster{
		Name:           "bad-config-cluster",
		ClusterCode:    "badcfg01",
		DeploymentMode: DeploymentModeHybrid,
		Version:        "2.3.12",
		Status:         ClusterStatusRunning,
		InstallDir:     "/opt/seatunnel",
		CreatedAt:      time.Now(),
	}
	if err := db.Omit("Config").Create(badCfg).Error; err != nil {
		t.Fatalf("create bad row shell: %v", err)
	}
	if err := db.Exec("UPDATE clusters SET config = CAST(? AS TEXT) WHERE id = ?", `{not-json`, badCfg.ID).Error; err != nil {
		t.Fatalf("write invalid TEXT config: %v", err)
	}

	clusters, total, warnings, err := repo.ListWithWarnings(ctx, &ClusterFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListWithWarnings: %v", err)
	}
	if total != 3 {
		t.Fatalf("total=%d want 3", total)
	}
	if len(clusters) != 3 {
		t.Fatalf("len=%d want 3 (degraded rows kept)", len(clusters))
	}

	byName := map[string]*Cluster{}
	for _, c := range clusters {
		byName[c.Name] = c
	}
	if byName["healthy-cluster"].Config["port"] != float64(8080) {
		t.Fatalf("healthy config: %#v", byName["healthy-cluster"].Config)
	}
	if byName["text-config-cluster"].Config["from"] != "text" {
		t.Fatalf("text config: %#v", byName["text-config-cluster"].Config)
	}
	if byName["bad-config-cluster"].ConfigScanError == "" {
		t.Fatal("expected ConfigScanError on invalid JSON row")
	}
	if byName["bad-config-cluster"].Config != nil {
		t.Fatalf("bad config should be nil, got %#v", byName["bad-config-cluster"].Config)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for invalid JSON row")
	}

	// 直接 Scan 路径也要接受 string（GetByID / 非 Omit 查询）。
	// Direct Scan path must accept string (GetByID / non-Omit queries).
	loaded, err := repo.GetByID(ctx, textCfg.ID, false)
	if err != nil {
		t.Fatalf("GetByID text config: %v", err)
	}
	raw, _ := json.Marshal(loaded.Config)
	if string(raw) != `{"from":"text"}` && loaded.Config["from"] != "text" {
		t.Fatalf("GetByID config: %#v", loaded.Config)
	}
}
