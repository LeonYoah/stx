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

package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"gorm.io/gorm"
)

// CompatTestModel 跨数据库兼容性测试模型
// CompatTestModel model for cross-database compatibility testing
type CompatTestModel struct {
	ID        uint                   `gorm:"primaryKey"`
	Name      string                 `gorm:"type:varchar(100);index"`
	Status    string                 `gorm:"type:varchar(32)"`
	Content   ScriptText             `gorm:""`
	Metadata  map[string]interface{} `gorm:"type:json;serializer:json"`
	CreatedAt time.Time
}

// TableName 自定义表名
// TableName customizes the database table name
func (CompatTestModel) TableName() string {
	return "stx_compat_test_records"
}

// getTestDatabases 探测并返回可用的跨数据库测试实例
// getTestDatabases discovers and returns available cross-database test instances
func getTestDatabases(t *testing.T) map[string]*gorm.DB {
	databases := make(map[string]*gorm.DB)

	// 1. SQLite
	tempDir, err := os.MkdirTemp("", "stx_compat_sqlite_*")
	if err != nil {
		t.Fatalf("创建临时目录失败 / Failed to create temp dir: %v", err)
	}
	sqlitePath := filepath.Join(tempDir, "compat_test.db")
	sqliteDB, err := OpenDB(config.DatabaseConfig{
		Enabled:    true,
		Type:       DatabaseTypeSQLite,
		SQLitePath: sqlitePath,
		LogLevel:   "silent",
	})
	if err != nil {
		t.Fatalf("初始化 SQLite 测试库失败 / Failed to init SQLite test DB: %v", err)
	}
	databases[DatabaseTypeSQLite] = sqliteDB

	// 2. PostgreSQL
	pgHost := os.Getenv("TEST_POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "127.0.0.1"
	}
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

	pgDB, err := OpenDB(config.DatabaseConfig{
		Enabled:  true,
		Type:     DatabaseTypePostgres,
		Host:     pgHost,
		Port:     5432,
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
				databases[DatabaseTypePostgres] = pgDB
			}
		}
	}

	// 3. MySQL
	mysqlHost := os.Getenv("TEST_MYSQL_HOST")
	if mysqlHost == "" {
		mysqlHost = "127.0.0.1"
	}
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

	mysqlDB, err := OpenDB(config.DatabaseConfig{
		Enabled:  true,
		Type:     DatabaseTypeMySQL,
		Host:     mysqlHost,
		Port:     3306,
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
				databases[DatabaseTypeMySQL] = mysqlDB
			}
		}
	}

	return databases
}

// TestDatabaseCompatibilitySuite 跨数据库核心行为一致性测试套件
// TestDatabaseCompatibilitySuite validates behavioral parity across SQLite, PostgreSQL, and MySQL
func TestDatabaseCompatibilitySuite(t *testing.T) {
	dbs := getTestDatabases(t)

	for dbType, targetDB := range dbs {
		t.Run(fmt.Sprintf("Suite_%s", dbType), func(t *testing.T) {
			// 初始化测试表 / AutoMigrate test table
			if err := targetDB.AutoMigrate(&CompatTestModel{}); err != nil {
				t.Fatalf("[%s] 迁移测试表失败 / Failed to migrate test table: %v", dbType, err)
			}
			defer targetDB.Migrator().DropTable(&CompatTestModel{})

			// 1. 大小写不敏感模糊查询一致性测试 / Case-insensitive LIKE query parity
			t.Run("CaseInsensitiveSearch", func(t *testing.T) {
				uniqueName := fmt.Sprintf("STX-Node-Alpha-%d", time.Now().UnixNano())
				record := &CompatTestModel{
					Name:   uniqueName,
					Status: "online",
				}
				if err := targetDB.Create(record).Error; err != nil {
					t.Fatalf("[%s] 创建测试记录失败: %v", dbType, err)
				}

				// 分别测试大写、小写、混合子串匹配 / Test uppercase, lowercase, and mixed substring matching
				patterns := []string{
					"%" + strings.ToLower(uniqueName) + "%",
					"%" + strings.ToUpper(uniqueName) + "%",
					"%alpha%",
					"%ALPHA%",
					"%stx-node%",
				}

				for _, pattern := range patterns {
					var found CompatTestModel
					err := targetDB.Where("LOWER(name) LIKE LOWER(?)", pattern).First(&found).Error
					if err != nil {
						t.Errorf("[%s] 模式 '%s' 未能匹配到记录 '%s': %v", dbType, pattern, uniqueName, err)
					}
					if found.ID != record.ID {
						t.Errorf("[%s] 模式 '%s' 匹配到的 ID 不一致: 期望 %d, 实际 %d", dbType, pattern, record.ID, found.ID)
					}
				}

				// 负向测试：不匹配的模式应返回 RecordNotFound / Negative test: non-matching pattern
				var notFound CompatTestModel
				err := targetDB.Where("LOWER(name) LIKE LOWER(?)", "%nonexistent_xyz%").First(&notFound).Error
				if err == nil {
					t.Errorf("[%s] 预期不匹配任何记录，却查出了 ID %d", dbType, notFound.ID)
				}
			})

			// 2. 大文本字段存取一致性测试 (>64KB，验证不会在任何数据库被意外截断)
			// 2. Large text storage parity (>64KB, verify no unexpected truncation in any DB)
			t.Run("LargeTextStorage", func(t *testing.T) {
				largeText := strings.Repeat("ABCDEFGHIJ", 7000) // 70,000 字节 / 70,000 bytes
				record := &CompatTestModel{
					Name:    "large-text-test",
					Content: ScriptText(largeText),
				}
				if err := targetDB.Create(record).Error; err != nil {
					t.Fatalf("[%s] 插入大文本记录失败: %v", dbType, err)
				}

				var retrieved CompatTestModel
				if err := targetDB.First(&retrieved, record.ID).Error; err != nil {
					t.Fatalf("[%s] 读取大文本记录失败: %v", dbType, err)
				}

				if len(retrieved.Content) != len(largeText) {
					t.Errorf("[%s] 大文本发生截断: 期望长度 %d, 实际读取长度 %d", dbType, len(largeText), len(retrieved.Content))
				}
			})

			// 3. JSON 字段结构化序列化与读取一致性测试
			// 3. JSON field serialization and retrieval parity
			t.Run("JSONFieldParity", func(t *testing.T) {
				record := &CompatTestModel{
					Name: "json-test",
					Metadata: map[string]interface{}{
						"cluster_name": "stx-prod",
						"nodes_count":  float64(3), // JSON 数字反序列化通常为 float64
						"is_active":    true,
					},
				}
				if err := targetDB.Create(record).Error; err != nil {
					t.Fatalf("[%s] 插入 JSON 记录失败: %v", dbType, err)
				}

				var retrieved CompatTestModel
				if err := targetDB.First(&retrieved, record.ID).Error; err != nil {
					t.Fatalf("[%s] 读取 JSON 记录失败: %v", dbType, err)
				}

				if retrieved.Metadata["cluster_name"] != "stx-prod" {
					t.Errorf("[%s] JSON 字段反序列化不一致: 期望 'stx-prod', 实际 %v", dbType, retrieved.Metadata["cluster_name"])
				}
				if retrieved.Metadata["is_active"] != true {
					t.Errorf("[%s] JSON 布尔字段反序列化不一致: 期望 true, 实际 %v", dbType, retrieved.Metadata["is_active"])
				}
			})

			// 4. 事务回滚一致性测试 / Transaction rollback parity
			t.Run("TransactionRollback", func(t *testing.T) {
				rollbackName := fmt.Sprintf("Rollback-%d", time.Now().UnixNano())
				tx := targetDB.Begin()
				txRecord := &CompatTestModel{
					Name: rollbackName,
				}
				if err := tx.Create(txRecord).Error; err != nil {
					tx.Rollback()
					t.Fatalf("[%s] 事务内创建记录失败: %v", dbType, err)
				}
				// 显式回滚 / Explicit rollback
				tx.Rollback()

				// 查询主连接，必须不存在该记录 / Query main connection, must not exist
				var check CompatTestModel
				err := targetDB.Where("name = ?", rollbackName).First(&check).Error
				if err == nil {
					t.Errorf("[%s] 事务回滚失败，记录仍然存在: ID=%d", dbType, check.ID)
				}
			})

			// 5. 分页与排序一致性测试 / Pagination and ordering parity
			t.Run("PaginationAndOrder", func(t *testing.T) {
				prefix := fmt.Sprintf("Page-%d", time.Now().UnixNano())
				for i := 1; i <= 5; i++ {
					item := &CompatTestModel{
						Name: fmt.Sprintf("%s-%02d", prefix, i),
					}
					if err := targetDB.Create(item).Error; err != nil {
						t.Fatalf("[%s] 插入分页数据失败: %v", dbType, err)
					}
				}

				// 测试 Offset(1).Limit(2).Order("name ASC")
				var pageResult []CompatTestModel
				err := targetDB.Where("name LIKE ?", prefix+"%").
					Order("name ASC").
					Offset(1).
					Limit(2).
					Find(&pageResult).Error
				if err != nil {
					t.Fatalf("[%s] 分页查询失败: %v", dbType, err)
				}

				if len(pageResult) != 2 {
					t.Fatalf("[%s] 分页条数不正确: 期望 2, 实际 %d", dbType, len(pageResult))
				}
				expectedFirst := fmt.Sprintf("%s-02", prefix)
				expectedSecond := fmt.Sprintf("%s-03", prefix)
				if pageResult[0].Name != expectedFirst || pageResult[1].Name != expectedSecond {
					t.Errorf("[%s] 分页排序结果不符合预期: 期望 [%s, %s], 实际 [%s, %s]",
						dbType, expectedFirst, expectedSecond, pageResult[0].Name, pageResult[1].Name)
				}
			})
		})
	}
}
