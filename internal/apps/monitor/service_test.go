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

package monitor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupMonitorServiceTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "monitor_service_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	database, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("open database: %v", err)
	}
	if err := database.AutoMigrate(&MonitorConfig{}); err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("migrate monitor config table: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.RemoveAll(tempDir)
	}
	return database, cleanup
}

func TestGetOrCreateConfig_normalizesLegacyAutoMonitorFalse(t *testing.T) {
	database, cleanup := setupMonitorServiceTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(repo)

	config := DefaultMonitorConfig(7)
	config.AutoMonitor = false
	if err := repo.CreateConfig(ctx, config); err != nil {
		t.Fatalf("create config: %v", err)
	}

	got, err := service.GetOrCreateConfig(ctx, 7)
	if err != nil {
		t.Fatalf("GetOrCreateConfig returned error: %v", err)
	}
	if !got.AutoMonitor {
		t.Fatalf("expected auto_monitor to be normalized to true")
	}

	stored, err := repo.GetConfigByClusterID(ctx, 7)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if !stored.AutoMonitor {
		t.Fatalf("expected stored auto_monitor to be persisted as true")
	}
}

func TestUpdateConfig_keepsAutoMonitorEnabled(t *testing.T) {
	database, cleanup := setupMonitorServiceTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(repo)

	got, err := service.UpdateConfig(ctx, 8, &UpdateMonitorConfigRequest{
		AutoMonitor: boolPtr(false),
		AutoRestart: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("UpdateConfig returned error: %v", err)
	}
	if !got.AutoMonitor {
		t.Fatalf("expected auto_monitor to remain enabled")
	}
	if got.AutoRestart {
		t.Fatalf("expected auto_restart update to still be applied")
	}
}

func boolPtr(value bool) *bool {
	return &value
}

// TestUpdateConfig_rejectsKubernetesCluster 验证 K8s 集群的 Agent 自愈配置被明确拒绝：
// 进程自愈由 kubelet 原生托管，保存只能空转误导。
// TestUpdateConfig_rejectsKubernetesCluster verifies agent self-healing config is rejected
// for Kubernetes clusters because self-healing is natively managed by the kubelet.
func TestUpdateConfig_rejectsKubernetesCluster(t *testing.T) {
	database, cleanup := setupMonitorServiceTestDB(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	repo := NewRepository(database)
	service := NewService(repo)
	service.SetKubernetesChecker(func(_ context.Context, clusterID uint) bool {
		return clusterID == 42
	})

	if _, err := service.UpdateConfig(ctx, 42, &UpdateMonitorConfigRequest{
		AutoRestart: boolPtr(true),
	}); err != ErrKubernetesMonitorConfigUnsupported {
		t.Fatalf("expected ErrKubernetesMonitorConfigUnsupported, got %v", err)
	}

	// 非 K8s 集群不受影响 / bare-metal clusters are unaffected
	if _, err := service.UpdateConfig(ctx, 8, &UpdateMonitorConfigRequest{
		AutoRestart: boolPtr(true),
	}); err != nil {
		t.Fatalf("bare-metal update must not be rejected: %v", err)
	}
}
