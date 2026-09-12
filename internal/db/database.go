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
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

// 全局数据库实例
var globalDB *gorm.DB

// DatabaseType 数据库类型常量
const (
	DatabaseTypeSQLite   = "sqlite"
	DatabaseTypeMySQL    = "mysql"
	DatabaseTypePostgres = "postgres"
)

// InitDatabase 根据配置初始化数据库连接
// 支持 SQLite、MySQL、PostgreSQL 三种数据库类型
// 默认使用 SQLite
// 此函数是幂等的，重复调用会跳过已初始化的数据库
func InitDatabase() error {
	// 如果已经初始化，直接返回
	if globalDB != nil {
		log.Println("[Database] 数据库已初始化，跳过重复初始化")
		return nil
	}

	dbConfig := config.Config.Database

	if !dbConfig.Enabled {
		log.Println("[Database] 数据库已禁用，跳过初始化")
		return nil
	}

	var err error
	var dialector gorm.Dialector

	// 根据配置的数据库类型选择驱动
	dbType := dbConfig.Type
	if dbType == "" {
		dbType = DatabaseTypeSQLite // 默认使用 SQLite
	}

	switch dbType {
	case DatabaseTypeSQLite:
		dialector, err = initSQLiteDialector(dbConfig.SQLitePath)
	case DatabaseTypeMySQL:
		dialector, err = initMySQLDialector(dbConfig)
	case DatabaseTypePostgres:
		dialector, err = initPostgresDialector(dbConfig)
	default:
		return fmt.Errorf("[Database] 不支持的数据库类型: %s，支持的类型: sqlite, mysql, postgres", dbType)
	}

	if err != nil {
		return fmt.Errorf("[Database] 初始化 %s 驱动失败: %w", dbType, err)
	}

	// 配置 GORM 日志级别
	gormLogger := getGormLogger(dbConfig.LogLevel)

	// 创建 GORM 实例
	globalDB, err = gorm.Open(dialector, &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   gormLogger,
	})
	if err != nil {
		return fmt.Errorf("[Database] 连接 %s 数据库失败: %w", dbType, err)
	}

	// 注入 OpenTelemetry 追踪
	if err := globalDB.Use(tracing.NewPlugin(tracing.WithoutMetrics())); err != nil {
		log.Printf("[Database] 初始化追踪插件失败: %v\n", err)
	}

	// 配置连接池 / Configure connection pool
	if dbType == DatabaseTypeSQLite {
		if err := configureSQLiteRuntime(); err != nil {
			return fmt.Errorf("[Database] 配置 SQLite 运行时失败: %w", err)
		}
	} else {
		if err := configureConnectionPool(dbConfig); err != nil {
			return fmt.Errorf("[Database] 配置连接池失败: %w", err)
		}
	}

	log.Printf("[Database] 成功连接到 %s 数据库\n", dbType)
	return nil
}

// initSQLiteDialector 初始化 SQLite 驱动
func initSQLiteDialector(sqlitePath string) (gorm.Dialector, error) {
	if sqlitePath == "" {
		sqlitePath = "./data/stx.db"
	}

	// 确保目录存在
	dir := filepath.Dir(sqlitePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建 SQLite 目录失败: %w", err)
	}

	log.Printf("[Database] 使用 SQLite 数据库: %s\n", sqlitePath)
	// 为 SQLite 启用 busy_timeout 和 WAL，缓解后台任务与前台保存并发时的锁竞争。
	// Enable busy_timeout and WAL for SQLite to reduce lock contention between
	// background preview maintenance and foreground saves.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)",
		sqlitePath,
	)
	return sqlite.Open(dsn), nil
}

// initMySQLDialector 初始化 MySQL 驱动
func initMySQLDialector(dbConfig config.DatabaseConfig) (gorm.Dialector, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbConfig.Username,
		dbConfig.Password,
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Database,
	)
	log.Printf("[Database] 连接 MySQL 数据库: %s:%d/%s\n", dbConfig.Host, dbConfig.Port, dbConfig.Database)
	return mysql.Open(dsn), nil
}

// initPostgresDialector 初始化 PostgreSQL 驱动
func initPostgresDialector(dbConfig config.DatabaseConfig) (gorm.Dialector, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbConfig.Host,
		dbConfig.Port,
		dbConfig.Username,
		dbConfig.Password,
		dbConfig.Database,
	)
	log.Printf("[Database] 连接 PostgreSQL 数据库: %s:%d/%s\n", dbConfig.Host, dbConfig.Port, dbConfig.Database)
	return postgres.Open(dsn), nil
}

// configureConnectionPool 配置数据库连接池
func configureConnectionPool(dbConfig config.DatabaseConfig) error {
	sqlDB, err := globalDB.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	// 设置连接池参数
	if dbConfig.MaxIdleConn > 0 {
		sqlDB.SetMaxIdleConns(dbConfig.MaxIdleConn)
	}
	if dbConfig.MaxOpenConn > 0 {
		sqlDB.SetMaxOpenConns(dbConfig.MaxOpenConn)
	}
	if dbConfig.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(time.Duration(dbConfig.ConnMaxLifetime) * time.Second)
	}

	return nil
}

// configureSQLiteRuntime configures SQLite-specific runtime behavior.
// configureSQLiteRuntime 配置 SQLite 运行时行为。
func configureSQLiteRuntime() error {
	sqlDB, err := globalDB.DB()
	if err != nil {
		return fmt.Errorf("获取底层 SQLite 连接失败: %w", err)
	}
	// SQLite 是单写者模型；限制为单连接能显著减少 database is locked。
	// SQLite uses a single-writer model; forcing a single connection reduces
	// "database is locked" during concurrent preview maintenance and saves.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	return nil
}

// getGormLogger 根据配置获取 GORM 日志记录器
func getGormLogger(level string) logger.Interface {
	var logLevel logger.LogLevel
	switch level {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		// Default to warn to avoid excessive SQL logging / 默认使用 warn 级别避免过多 SQL 日志
		logLevel = logger.Warn
	}

	return logger.Default.LogMode(logLevel)
}

// GetDB 获取带上下文的数据库实例
func GetDB(ctx context.Context) *gorm.DB {
	if globalDB == nil {
		return nil
	}
	return globalDB.WithContext(ctx)
}

// GetGlobalDB 获取全局数据库实例（不带上下文）
func GetGlobalDB() *gorm.DB {
	return globalDB
}

// CloseDatabase 关闭数据库连接
func CloseDatabase() error {
	if globalDB == nil {
		return nil
	}

	sqlDB, err := globalDB.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	return sqlDB.Close()
}

// IsDatabaseInitialized 检查数据库是否已初始化
func IsDatabaseInitialized() bool {
	return globalDB != nil
}

// GetDatabaseType 获取当前数据库类型
func GetDatabaseType() string {
	return config.Config.Database.Type
}
