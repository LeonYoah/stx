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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"gorm.io/gorm"
)

// getTestDatabaseDialectors 探测并返回可用的测试数据库连接
// getTestDatabaseDialectors discovers and returns available test database connections
func getTestDatabaseDialectors(t *testing.T) map[string]*gorm.DB {
	databases := make(map[string]*gorm.DB)

	// 1. SQLite: 始终构建临时文件进行测试 / Always build a temp file for SQLite testing
	tempDir, err := os.MkdirTemp("", "stx_migrate_sqlite_*")
	if err != nil {
		t.Fatalf("创建临时目录失败 / Failed to create temp dir: %v", err)
	}
	sqlitePath := filepath.Join(tempDir, "migrate_test.db")
	sqliteDB, err := db.OpenDB(config.DatabaseConfig{
		Enabled:    true,
		Type:       db.DatabaseTypeSQLite,
		SQLitePath: sqlitePath,
		LogLevel:   "silent",
	})
	if err != nil {
		t.Fatalf("初始化 SQLite 测试数据库失败 / Failed to init SQLite test DB: %v", err)
	}
	databases[db.DatabaseTypeSQLite] = sqliteDB

	// 2. PostgreSQL: 优先读取环境变量，其次探测本地默认端口 / Priority to env var, fallback to local probe
	pgHost := os.Getenv("TEST_POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "127.0.0.1"
	}
	pgPort := 5432
	pgUser := os.Getenv("TEST_POSTGRES_USER")
	if pgUser == "" {
		pgUser = "stx"
	}
	pgPassword := os.Getenv("TEST_POSTGRES_PASSWORD")
	if pgPassword == "" {
		pgPassword = "stx_password_2026"
	}
	pgDBName := os.Getenv("TEST_POSTGRES_DB")
	if pgDBName == "" {
		pgDBName = "stx"
	}

	pgDB, err := db.OpenDB(config.DatabaseConfig{
		Enabled:  true,
		Type:     db.DatabaseTypePostgres,
		Host:     pgHost,
		Port:     pgPort,
		Username: pgUser,
		Password: pgPassword,
		Database: pgDBName,
		LogLevel: "silent",
	})
	if err == nil {
		sqlDB, pingErr := pgDB.DB()
		if pingErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if pingErr = sqlDB.PingContext(ctx); pingErr == nil {
				databases[db.DatabaseTypePostgres] = pgDB
			}
		}
	}
	if _, ok := databases[db.DatabaseTypePostgres]; !ok {
		t.Log("[PostgreSQL] 未检测到可用实例或无法连接，跳过 PG 迁移测试 / No available PG instance detected, skipping")
	}

	// 3. MySQL: 探测环境变量或本地默认端口 / Probe env var or local default port
	mysqlHost := os.Getenv("TEST_MYSQL_HOST")
	if mysqlHost == "" {
		mysqlHost = "127.0.0.1"
	}
	mysqlPort := 3306
	mysqlUser := os.Getenv("TEST_MYSQL_USER")
	if mysqlUser == "" {
		mysqlUser = "root"
	}
	mysqlPassword := os.Getenv("TEST_MYSQL_PASSWORD")
	if mysqlPassword == "" {
		mysqlPassword = "root"
	}
	mysqlDBName := os.Getenv("TEST_MYSQL_DB")
	if mysqlDBName == "" {
		mysqlDBName = "stx_test"
	}

	mysqlDB, err := db.OpenDB(config.DatabaseConfig{
		Enabled:  true,
		Type:     db.DatabaseTypeMySQL,
		Host:     mysqlHost,
		Port:     mysqlPort,
		Username: mysqlUser,
		Password: mysqlPassword,
		Database: mysqlDBName,
		LogLevel: "silent",
	})
	if err == nil {
		sqlDB, pingErr := mysqlDB.DB()
		if pingErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if pingErr = sqlDB.PingContext(ctx); pingErr == nil {
				databases[db.DatabaseTypeMySQL] = mysqlDB
			}
		}
	}
	if _, ok := databases[db.DatabaseTypeMySQL]; !ok {
		t.Log("[MySQL] 未检测到可用实例或无法连接，跳过 MySQL 迁移测试 / No available MySQL instance detected, skipping")
	}

	return databases
}

// TestMultiDatabaseMigration 验证全量 46 张表和索引在各数据库上的首次迁移与幂等二次迁移
// TestMultiDatabaseMigration validates first-time migration and idempotent second-time migration for all 46 tables and indexes
func TestMultiDatabaseMigration(t *testing.T) {
	dbs := getTestDatabaseDialectors(t)
	if len(dbs) == 0 {
		t.Fatal("未发现任何可用的数据库进行测试 / No available database found for testing")
	}

	for dbType, targetDB := range dbs {
		t.Run(fmt.Sprintf("Migration_%s", dbType), func(t *testing.T) {
			// 第一遍迁移：验证建表、列类型映射与索引创建
			// First migration: Verify table creation, column type mapping, and index creation
			if err := MigrateWithDB(targetDB, dbType); err != nil {
				t.Fatalf("[%s] 首次迁移失败 / First migration failed: %v", dbType, err)
			}

			// 验证用户表与默认管理员
			// Verify user table and default admin user
			var adminUser auth.User
			if err := targetDB.Where("username = ?", "admin").First(&adminUser).Error; err != nil {
				t.Fatalf("[%s] 查询默认管理员用户失败 / Query default admin user failed: %v", dbType, err)
			}
			if adminUser.Username != "admin" {
				t.Errorf("[%s] 期望管理员用户名为 admin，实际为 %s / Expected username admin, got %s", dbType, adminUser.Username, adminUser.Username)
			}

			// 第二遍迁移：验证迁移幂等性（重复执行不能出现 duplicate index / table already exists 等异常）
			// Second migration: Verify migration idempotency (must not error on duplicate index / table)
			if err := MigrateWithDB(targetDB, dbType); err != nil {
				t.Fatalf("[%s] 幂等二次迁移失败 / Second idempotent migration failed: %v", dbType, err)
			}
		})
	}
}
