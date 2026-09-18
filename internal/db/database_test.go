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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// 获取项目根目录
func getProjectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	// internal/db/database_test.go -> 项目根目录
	return filepath.Join(filepath.Dir(filename), "..", "..")
}

// TestMain 设置测试环境
func TestMain(m *testing.M) {
	// 设置配置文件路径为项目根目录下的 config.yaml
	projectRoot := getProjectRoot()
	configPath := filepath.Join(projectRoot, "config.yaml")

	// 如果 config.yaml 不存在，从 config.example.yaml 复制
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		examplePath := filepath.Join(projectRoot, "config.example.yaml")
		if data, err := os.ReadFile(examplePath); err == nil {
			os.WriteFile(configPath, data, 0644)
		}
	}

	os.Setenv("CONFIG_PATH", configPath)

	// 运行测试
	os.Exit(m.Run())
}

// **Feature: stx-platform-login, Property 5: Session store consistency** (部分)
// **Validates: Requirements 4.3**
//
// 此属性测试验证数据库初始化的一致性行为：
// 对于任何有效的数据库配置，初始化操作应该产生一致的结果，
// 无论使用哪种数据库类型（SQLite、MySQL、PostgreSQL）。
// 由于 MySQL 和 PostgreSQL 需要外部服务，此测试主要验证 SQLite 的一致性行为。

// TestProperty_DatabaseInitConsistency 测试数据库初始化一致性
// 对于任何有效的 SQLite 路径，数据库初始化应该：
// 1. 成功创建数据库连接
// 2. 返回可用的数据库实例
// 3. 支持基本的数据库操作
func TestProperty_DatabaseInitConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42) // 固定种子以确保可重复性

	properties := gopter.NewProperties(parameters)

	// 属性：对于任何有效的 SQLite 文件名，初始化应该成功
	properties.Property("SQLite 初始化一致性", prop.ForAll(
		func(filename string) bool {
			// 创建临时目录
			tempDir, err := os.MkdirTemp("", "db_test_*")
			if err != nil {
				t.Logf("创建临时目录失败: %v", err)
				return false
			}
			defer os.RemoveAll(tempDir)

			// 构建完整路径
			dbPath := filepath.Join(tempDir, filename+".db")

			// 配置数据库
			config.Config.Database = config.DatabaseConfig{
				Enabled:    true,
				Type:       DatabaseTypeSQLite,
				SQLitePath: dbPath,
				LogLevel:   "silent",
			}

			// 重置全局数据库实例
			globalDB = nil

			// 初始化数据库
			err = InitDatabase()
			if err != nil {
				t.Logf("初始化数据库失败: %v", err)
				return false
			}
			defer CloseDatabase()

			// 验证数据库已初始化
			if !IsDatabaseInitialized() {
				t.Log("数据库未正确初始化")
				return false
			}

			// 验证可以获取数据库实例
			db := GetGlobalDB()
			if db == nil {
				t.Log("无法获取全局数据库实例")
				return false
			}

			// 验证带上下文的数据库实例
			ctxDB := GetDB(context.Background())
			if ctxDB == nil {
				t.Log("无法获取带上下文的数据库实例")
				return false
			}

			// 验证数据库类型
			if GetDatabaseType() != DatabaseTypeSQLite {
				t.Log("数据库类型不正确")
				return false
			}

			return true
		},
		// 生成有效的文件名（字母数字组合，长度1-20）
		gen.RegexMatch("[a-zA-Z][a-zA-Z0-9]{0,19}"),
	))

	properties.TestingRun(t)
}

// TestProperty_DatabaseTypeSelection 测试数据库类型选择一致性
// 对于任何支持的数据库类型，系统应该正确识别并选择对应的驱动
func TestProperty_DatabaseTypeSelection(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// 属性：对于任何支持的数据库类型，GetDatabaseType 应该返回正确的类型
	properties.Property("数据库类型选择一致性", prop.ForAll(
		func(dbType string) bool {
			// 创建临时目录用于 SQLite
			tempDir, err := os.MkdirTemp("", "db_type_test_*")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tempDir)

			// 配置数据库
			config.Config.Database = config.DatabaseConfig{
				Enabled:    true,
				Type:       dbType,
				SQLitePath: filepath.Join(tempDir, "test.db"),
				LogLevel:   "silent",
			}

			// 验证 GetDatabaseType 返回配置的类型
			return GetDatabaseType() == dbType
		},
		// 生成支持的数据库类型
		gen.OneConstOf(DatabaseTypeSQLite, DatabaseTypeMySQL, DatabaseTypePostgres),
	))

	properties.TestingRun(t)
}

// TestProperty_DefaultDatabaseType 测试默认数据库类型
// 当未指定数据库类型时，系统应该默认使用 SQLite
func TestProperty_DefaultDatabaseType(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// 属性：当类型为空时，初始化应该默认使用 SQLite
	properties.Property("默认数据库类型为 SQLite", prop.ForAll(
		func(_ int) bool {
			// 创建临时目录
			tempDir, err := os.MkdirTemp("", "db_default_test_*")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tempDir)

			// 配置数据库，类型为空
			config.Config.Database = config.DatabaseConfig{
				Enabled:    true,
				Type:       "", // 空类型
				SQLitePath: filepath.Join(tempDir, "test.db"),
				LogLevel:   "silent",
			}

			// 重置全局数据库实例
			globalDB = nil

			// 初始化数据库
			err = InitDatabase()
			if err != nil {
				return false
			}
			defer CloseDatabase()

			// 验证数据库已初始化（说明默认使用了 SQLite）
			return IsDatabaseInitialized()
		},
		// 生成任意整数作为测试迭代标识
		gen.Int(),
	))

	properties.TestingRun(t)
}

// TestProperty_UnsupportedDatabaseType 测试不支持的数据库类型
// 对于任何不支持的数据库类型，初始化应该返回错误
func TestProperty_UnsupportedDatabaseType(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// 属性：对于不支持的数据库类型，初始化应该失败
	properties.Property("不支持的数据库类型应返回错误", prop.ForAll(
		func(dbType string) bool {
			// 配置数据库
			config.Config.Database = config.DatabaseConfig{
				Enabled:  true,
				Type:     dbType,
				LogLevel: "silent",
			}

			// 重置全局数据库实例
			globalDB = nil

			// 初始化数据库
			err := InitDatabase()

			// 不支持的类型应该返回错误
			return err != nil
		},
		// 生成不支持的数据库类型（排除支持的类型和空字符串）
		gen.Identifier().SuchThat(func(s string) bool {
			return s != DatabaseTypeSQLite &&
				s != DatabaseTypeMySQL &&
				s != DatabaseTypePostgres &&
				s != "" && // 空字符串会默认使用 SQLite
				len(s) > 0
		}),
	))

	properties.TestingRun(t)
}

// TestProperty_DatabaseDisabled 测试数据库禁用状态
// 当数据库被禁用时，初始化应该跳过且不返回错误
func TestProperty_DatabaseDisabled(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// 属性：当数据库禁用时，初始化应该成功但不创建连接
	properties.Property("禁用数据库时跳过初始化", prop.ForAll(
		func(_ int) bool {
			// 配置数据库为禁用状态
			config.Config.Database = config.DatabaseConfig{
				Enabled: false,
			}

			// 重置全局数据库实例
			globalDB = nil

			// 初始化数据库
			err := InitDatabase()

			// 应该成功但数据库未初始化
			return err == nil && !IsDatabaseInitialized()
		},
		gen.Int(),
	))

	properties.TestingRun(t)
}
