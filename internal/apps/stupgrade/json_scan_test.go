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

package stupgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUpgradeScanDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "stupgrade_scan_*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	dbPath := filepath.Join(tempDir, "test.db")
	database, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("open db: %v", err)
	}
	if err := database.AutoMigrate(&UpgradePlanRecord{}); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("migrate: %v", err)
	}
	cleanup := func() {
		sqlDB, _ := database.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		os.RemoveAll(tempDir)
	}
	return database, cleanup
}

func TestUpgradePlanSnapshotScan_AcceptsBytesStringNilAndRejectsInvalid(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		var snap UpgradePlanSnapshot
		if err := snap.Scan(nil); err != nil {
			t.Fatalf("nil: %v", err)
		}
	})

	payload := `{"cluster_id":1,"source_version":"2.3.11","target_version":"2.3.12"}`

	t.Run("bytes", func(t *testing.T) {
		var snap UpgradePlanSnapshot
		if err := snap.Scan([]byte(payload)); err != nil {
			t.Fatalf("bytes: %v", err)
		}
		if snap.ClusterID != 1 || snap.SourceVersion != "2.3.11" {
			t.Fatalf("got %#v", snap)
		}
	})

	t.Run("string", func(t *testing.T) {
		var snap UpgradePlanSnapshot
		if err := snap.Scan(payload); err != nil {
			t.Fatalf("string: %v", err)
		}
		if snap.TargetVersion != "2.3.12" {
			t.Fatalf("got %#v", snap)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		var snap UpgradePlanSnapshot
		if err := snap.Scan(`{bad`); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLogMetadataScan_AcceptsString(t *testing.T) {
	t.Parallel()
	var meta LogMetadata
	if err := meta.Scan(`{"host":"a"}`); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if meta["host"] != "a" {
		t.Fatalf("got %#v", meta)
	}
}

func TestGetPlanByID_ReadsTextSnapshot(t *testing.T) {
	database, cleanup := setupUpgradeScanDB(t)
	defer cleanup()
	repo := NewRepository(database)
	ctx := context.Background()

	rec := &UpgradePlanRecord{
		ClusterID:     9,
		SourceVersion: "2.3.11",
		TargetVersion: "2.3.12",
		Status:        PlanStatusReady,
		Snapshot:      UpgradePlanSnapshot{ClusterID: 9},
	}
	if err := database.Create(rec).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	snapshotJSON := `{"cluster_id":9,"source_version":"2.3.11","target_version":"2.3.12","steps":[]}`
	// 覆盖为 TEXT 亲和性，模拟测试夹具用字符串写入 JSON 列。
	// Overwrite as TEXT affinity to mimic fixtures that store JSON as a string.
	if err := database.Exec("UPDATE st_upgrade_plans SET snapshot = CAST(? AS TEXT) WHERE id = ?", snapshotJSON, rec.ID).Error; err != nil {
		t.Fatalf("write TEXT snapshot: %v", err)
	}

	loaded, err := repo.GetPlanByID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("GetPlanByID: %v", err)
	}
	if loaded.Snapshot.ClusterID != 9 || loaded.Snapshot.TargetVersion != "2.3.12" {
		t.Fatalf("snapshot %#v", loaded.Snapshot)
	}
}
