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

package migrator

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type legacyMySQLRow struct {
	ID        uint
	CreatedAt time.Time
	EndedAt   *time.Time
}

func (legacyMySQLRow) TableName() string { return "legacy_mysql_rows" }

// openMigratorTestMySQL 连接 STX_TEST_MYSQL_PORT 指定的 MySQL，并重建独立测试库。
// openMigratorTestMySQL connects to the MySQL at STX_TEST_MYSQL_PORT and recreates a dedicated database.
func openMigratorTestMySQL(t *testing.T, name string) *gorm.DB {
	t.Helper()
	port := os.Getenv("STX_TEST_MYSQL_PORT")
	if port == "" {
		t.Skip("STX_TEST_MYSQL_PORT not set")
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatalf("invalid STX_TEST_MYSQL_PORT %q", port)
	}
	host := os.Getenv("STX_TEST_MYSQL_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	user := os.Getenv("STX_TEST_MYSQL_USER")
	if user == "" {
		user = "root"
	}
	server := fmt.Sprintf("%s:%s@tcp(%s:%s)/", user, os.Getenv("STX_TEST_MYSQL_PASSWORD"), host, port)
	admin, err := gorm.Open(mysql.Open(server+"?parseTime=true&loc=UTC"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	dbName := "stx_utc_mig_" + name
	for _, stmt := range []string{"DROP DATABASE IF EXISTS `" + dbName + "`", "CREATE DATABASE `" + dbName + "`"} {
		if err := admin.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		admin.Exec("DROP DATABASE IF EXISTS `" + dbName + "`")
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	database, err := gorm.Open(mysql.Open(server+dbName+"?parseTime=true&loc=UTC"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := database.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := database.AutoMigrate(&legacyMySQLRow{}); err != nil {
		t.Fatal(err)
	}
	return database
}

func seedLegacyMySQLRows(t *testing.T, database *gorm.DB) {
	t.Helper()
	for _, stmt := range []string{
		// 冬令时 EST(-05:00) 与夏令时 EDT(-04:00) / winter EST and summer EDT wall times
		"INSERT INTO legacy_mysql_rows (id, created_at, ended_at) VALUES (1, '2026-01-15 12:00:00', '2026-07-15 12:00:00.250')",
		"INSERT INTO legacy_mysql_rows (id, created_at, ended_at) VALUES (2, '2026-07-15 12:00:00', NULL)",
		// 值互相“撞车”：行 3 的旧值等于行 1 换算后的新值 / row 3's old value equals row 1's converted value
		"INSERT INTO legacy_mysql_rows (id, created_at, ended_at) VALUES (3, '2026-01-15 17:00:00', NULL)",
	} {
		if err := database.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func assertNewYorkRowsUTC(t *testing.T, database *gorm.DB) {
	t.Helper()
	want := map[uint][2]*time.Time{
		1: {ptrTime(time.Date(2026, 1, 15, 17, 0, 0, 0, time.UTC)), ptrTime(time.Date(2026, 7, 15, 16, 0, 0, 250_000_000, time.UTC))},
		2: {ptrTime(time.Date(2026, 7, 15, 16, 0, 0, 0, time.UTC)), nil},
		3: {ptrTime(time.Date(2026, 1, 15, 22, 0, 0, 0, time.UTC)), nil},
	}
	var rows []legacyMySQLRow
	if err := database.Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d", len(rows))
	}
	for _, row := range rows {
		w := want[row.ID]
		if !row.CreatedAt.Equal(*w[0]) {
			t.Fatalf("row %d created_at = %v, want %v", row.ID, row.CreatedAt, *w[0])
		}
		if (row.EndedAt == nil) != (w[1] == nil) || (row.EndedAt != nil && !row.EndedAt.Equal(*w[1])) {
			t.Fatalf("row %d ended_at = %v, want %v", row.ID, row.EndedAt, w[1])
		}
	}
}

func ptrTime(v time.Time) *time.Time { return &v }

// TestMigrateTimeColumnsToUTC_MySQLDST 覆盖夏令时时区按行换算：MySQL 时区表路径（若已加载）与 Go 回退路径。
// TestMigrateTimeColumnsToUTC_MySQLDST covers per-row DST conversion via MySQL tz tables (when loaded) and the Go fallback.
func TestMigrateTimeColumnsToUTC_MySQLDST(t *testing.T) {
	models := []interface{}{&legacyMySQLRow{}}
	t.Run("go-rows", func(t *testing.T) {
		database := openMigratorTestMySQL(t, "go_rows")
		original := mysqlNamedZoneProbe
		mysqlNamedZoneProbe = func(*gorm.DB, string) bool { return false }
		t.Cleanup(func() { mysqlNamedZoneProbe = original })
		seedLegacyMySQLRows(t, database)
		if err := MigrateTimeColumnsToUTC(database, "mysql", models, "America/New_York"); err != nil {
			t.Fatal(err)
		}
		assertNewYorkRowsUTC(t, database)
		assertFlagDetailContains(t, database, mysqlUTCModeGoRows)
	})
	t.Run("named", func(t *testing.T) {
		database := openMigratorTestMySQL(t, "named")
		if !mysqlNamedZoneSupported(database, "America/New_York") {
			t.Skip("MySQL time zone tables are not loaded")
		}
		seedLegacyMySQLRows(t, database)
		if err := MigrateTimeColumnsToUTC(database, "mysql", models, "America/New_York"); err != nil {
			t.Fatal(err)
		}
		assertNewYorkRowsUTC(t, database)
		assertFlagDetailContains(t, database, mysqlUTCModeNamed)
	})
	t.Run("fixed", func(t *testing.T) {
		database := openMigratorTestMySQL(t, "fixed")
		if err := database.Exec("INSERT INTO legacy_mysql_rows (id, created_at) VALUES (1, '2026-01-15 12:00:00'), (2, '2026-07-15 12:00:00')").Error; err != nil {
			t.Fatal(err)
		}
		if err := MigrateTimeColumnsToUTC(database, "mysql", models, "Asia/Shanghai"); err != nil {
			t.Fatal(err)
		}
		var rows []legacyMySQLRow
		database.Order("id").Find(&rows)
		if len(rows) != 2 || !rows[0].CreatedAt.Equal(time.Date(2026, 1, 15, 4, 0, 0, 0, time.UTC)) ||
			!rows[1].CreatedAt.Equal(time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)) {
			t.Fatalf("fixed offset rows = %+v", rows)
		}
		assertFlagDetailContains(t, database, "+08:00")
	})
}

// TestMigrateTimeColumnsToUTC_MySQLConcurrent 并发启动时只迁移一次，所有实例都成功返回。
// TestMigrateTimeColumnsToUTC_MySQLConcurrent migrates once under concurrent starts and every instance succeeds.
func TestMigrateTimeColumnsToUTC_MySQLConcurrent(t *testing.T) {
	database := openMigratorTestMySQL(t, "concurrent")
	original := mysqlNamedZoneProbe
	mysqlNamedZoneProbe = func(*gorm.DB, string) bool { return false }
	t.Cleanup(func() { mysqlNamedZoneProbe = original })
	seedLegacyMySQLRows(t, database)
	if err := database.AutoMigrate(&SchemaFlag{}); err != nil {
		t.Fatal(err)
	}
	const workers = 4
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- MigrateTimeColumnsToUTC(database, "mysql", []interface{}{&legacyMySQLRow{}}, "America/New_York")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent migration must succeed: %v", err)
		}
	}
	assertNewYorkRowsUTC(t, database)
}

func assertFlagDetailContains(t *testing.T, database *gorm.DB, want string) {
	t.Helper()
	var flag SchemaFlag
	if err := database.First(&flag, "name = ?", TimeUTCMigrationName).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(flag.Detail, want) {
		t.Fatalf("flag detail %q does not mention %q", flag.Detail, want)
	}
}
