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

// Package tzit 是跨数据库驱动的时间区间集成测试：默认跑 SQLite；设置 STX_TEST_MYSQL_PORT / STX_TEST_PG_PORT 时额外跑 MySQL / PostgreSQL。
// Package tzit holds cross-driver time-range integration tests: SQLite always, MySQL/PostgreSQL when their ports are set.
package tzit

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/apps/audit"
	configapp "github.com/LeonYoah/stx/internal/apps/config"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/db/migrator"
)

func openTargets(t *testing.T) map[string]*gorm.DB {
	t.Helper()
	targets := map[string]*gorm.DB{}
	sqliteDB, err := db.OpenDB(config.DatabaseConfig{Type: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "tz.db")})
	if err != nil {
		t.Fatal(err)
	}
	targets["sqlite"] = sqliteDB
	if port := os.Getenv("STX_TEST_MYSQL_PORT"); port != "" {
		p, _ := strconv.Atoi(port)
		mysqlDB, err := db.OpenDB(config.DatabaseConfig{
			Type: "mysql", Host: envOr("STX_TEST_MYSQL_HOST", "127.0.0.1"), Port: p,
			Username: envOr("STX_TEST_MYSQL_USER", "root"), Password: os.Getenv("STX_TEST_MYSQL_PASSWORD"),
			Database: envOr("STX_TEST_MYSQL_DATABASE", "stx_tz_it"),
		})
		if err != nil {
			t.Fatal(err)
		}
		targets["mysql"] = mysqlDB
	}
	if port := os.Getenv("STX_TEST_PG_PORT"); port != "" {
		p, _ := strconv.Atoi(port)
		pgDB, err := db.OpenDB(config.DatabaseConfig{
			Type: "postgres", Host: envOr("STX_TEST_PG_HOST", "127.0.0.1"), Port: p,
			Username: envOr("STX_TEST_PG_USER", "postgres"), Password: os.Getenv("STX_TEST_PG_PASSWORD"),
			Database: envOr("STX_TEST_PG_DATABASE", "stx_tz_it"),
		})
		if err != nil {
			t.Fatal(err)
		}
		targets["postgres"] = pgDB
	}
	return targets
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestScheduledWindowAndConfigRangeAcrossDrivers(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	for name, database := range openTargets(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if err := database.Migrator().DropTable(&syncapp.JobInstance{}, &configapp.Config{}); err != nil {
				t.Fatal(err)
			}
			if err := database.AutoMigrate(&syncapp.JobInstance{}, &configapp.Config{}); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			job := &syncapp.JobInstance{TaskID: 7, TaskVersion: 1, RunType: syncapp.RunTypeSchedule, CreatedAt: now.In(shanghai)}
			if err := database.Create(job).Error; err != nil {
				t.Fatal(err)
			}
			repo := syncapp.NewRepository(database)
			windowStart := now.Truncate(time.Minute)
			for label, bounds := range map[string][2]time.Time{
				"utc":      {windowStart.UTC(), windowStart.Add(time.Minute).UTC()},
				"shanghai": {windowStart.In(shanghai), windowStart.Add(time.Minute).In(shanghai)},
			} {
				exists, err := repo.HasScheduledJobInstanceInWindow(ctx, 7, bounds[0], bounds[1])
				if err != nil || !exists {
					t.Fatalf("%s window: expected existing scheduled job, got %v %v", label, exists, err)
				}
			}
			if exists, _ := repo.HasScheduledJobInstanceInWindow(ctx, 7, windowStart.Add(time.Hour), windowStart.Add(time.Hour+time.Minute)); exists {
				t.Fatal("future window must not match")
			}

			cfg := &configapp.Config{ClusterID: 3, ConfigType: "seatunnel"}
			if err := database.Create(cfg).Error; err != nil {
				t.Fatal(err)
			}
			items, err := configapp.NewRepository(database).ListUpdatedByClusterBetween(ctx, 3, now.Add(-time.Minute).In(shanghai), now.Add(time.Minute).In(shanghai))
			if err != nil || len(items) != 1 {
				t.Fatalf("config range: expected 1 item, got %d %v", len(items), err)
			}
		})
	}
}

// TestAuditRangeRetentionAndOrderingAcrossDrivers 覆盖审计区间查询、保留期删除与混合 Location 写入后的排序。
// TestAuditRangeRetentionAndOrderingAcrossDrivers covers audit range filters, retention deletes and ordering.
func TestAuditRangeRetentionAndOrderingAcrossDrivers(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	newYork, _ := time.LoadLocation("America/New_York")
	for name, database := range openTargets(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			_ = database.Migrator().DropTable(&audit.AuditLog{})
			if err := database.AutoMigrate(&audit.AuditLog{}); err != nil {
				t.Fatal(err)
			}
			base := time.Date(2026, 10, 14, 7, 0, 0, 0, time.UTC)
			// 交错的 Location：按绝对时刻顺序应为 id 1..4 / interleaved locations, instant order is ids 1..4
			for i, at := range []time.Time{
				base.Add(-48 * time.Hour).In(shanghai),
				base.Add(-1 * time.Hour).In(newYork),
				base.In(shanghai),
				base.Add(30 * time.Minute),
			} {
				row := &audit.AuditLog{Action: "create", ResourceType: "probe", ResourceID: strconv.Itoa(i + 1), CreatedAt: at}
				if err := database.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			var ordered []audit.AuditLog
			if err := database.Order("created_at ASC").Find(&ordered).Error; err != nil {
				t.Fatal(err)
			}
			for i, row := range ordered {
				if row.ResourceID != strconv.Itoa(i+1) {
					t.Fatalf("ordering broken at %d: %+v", i, row.ResourceID)
				}
			}
			repo := audit.NewRepository(database)
			start := base.Add(-90 * time.Minute).In(shanghai)
			end := base.Add(10 * time.Minute).In(newYork)
			items, total, err := repo.ListAuditLogs(ctx, &audit.AuditLogFilter{StartTime: &start, EndTime: &end, Page: 1, PageSize: 10})
			if err != nil || total != 2 || len(items) != 2 {
				t.Fatalf("range: total=%d len=%d err=%v", total, len(items), err)
			}
			deleted, err := repo.DeleteAuditLogsBefore(ctx, base.Add(-24*time.Hour).In(shanghai))
			if err != nil || deleted != 1 {
				t.Fatalf("retention delete: deleted=%d err=%v", deleted, err)
			}
		})
	}
}

type mysqlLegacyRow struct {
	ID         uint
	FinishedAt *time.Time
	CreatedAt  time.Time
}

// TestMySQLLegacyMigration 验证 MySQL CONVERT_TZ 迁移：毫秒精度保留、NULL 不变、仅执行一次。
// TestMySQLLegacyMigration verifies the MySQL CONVERT_TZ migration keeps millisecond precision, NULLs and runs once.
func TestMySQLLegacyMigration(t *testing.T) {
	database, ok := openTargets(t)["mysql"]
	if !ok {
		t.Skip("STX_TEST_MYSQL_PORT not set")
	}
	_ = database.Migrator().DropTable(&mysqlLegacyRow{}, &migrator.SchemaFlag{})
	if err := database.AutoMigrate(&mysqlLegacyRow{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("INSERT INTO mysql_legacy_rows (id, created_at, finished_at) VALUES (1, '2026-10-14 15:04:44.576', NULL), (2, '2026-01-01 00:30:00.001', '2026-01-01 08:00:00')").Error; err != nil {
		t.Fatal(err)
	}
	models := []interface{}{&mysqlLegacyRow{}}
	for i := 0; i < 2; i++ {
		if err := migrator.MigrateTimeColumnsToUTC(database, "mysql", models, "Asia/Shanghai"); err != nil {
			t.Fatal(err)
		}
	}
	var rows []mysqlLegacyRow
	if err := database.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if !rows[0].CreatedAt.Equal(time.Date(2026, 10, 14, 7, 4, 44, 576_000_000, time.UTC)) || rows[0].FinishedAt != nil {
		t.Fatalf("row1: %+v", rows[0])
	}
	if !rows[1].CreatedAt.Equal(time.Date(2025, 12, 31, 16, 30, 0, 1_000_000, time.UTC)) ||
		rows[1].FinishedAt == nil || !rows[1].FinishedAt.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("row2: %+v", rows[1])
	}
}
