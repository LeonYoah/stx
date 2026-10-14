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
	"log"
	"reflect"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// TimeUTCMigrationName 是 UTC 迁移的标记名。/ TimeUTCMigrationName is the flag name of the UTC migration.
const TimeUTCMigrationName = "time_columns_utc_v1"

// SchemaFlag 记录一次性数据迁移是否已执行。/ SchemaFlag records whether a one-time data migration has run.
type SchemaFlag struct {
	Name      string    `gorm:"primaryKey;size:128"`
	Detail    string    `gorm:"size:512"`
	AppliedAt time.Time `gorm:"not null"`
}

// TableName implements gorm tabler.
func (SchemaFlag) TableName() string { return "stx_schema_flags" }

// MigrateTimeColumnsToUTC 将历史时间列一次性迁移为 UTC，执行后写入标记，重复调用为空操作。
//   - SQLite：值以文本保存并带偏移（如 2026-10-14 13:31:03.96+08:00），用 strftime 重写为 ...Z；
//     WHERE 条件只匹配非 Z 结尾且可解析的值，因此本身也是幂等的。
//   - MySQL：DATETIME 不带时区，旧 DSN 为 loc=Local，按 legacyTimezone（为空则进程 time.Local）的当前偏移做 CONVERT_TZ。
//   - PostgreSQL：GORM 默认 timestamptz，存储的是绝对时刻，无需改写，仅写入标记。
//
// 建议在升级前备份数据库（SQLite 复制 data/stx.db*，MySQL 使用 mysqldump）。
//
// MigrateTimeColumnsToUTC migrates legacy time columns to UTC once and records a flag; later calls are no-ops.
// Back up the database before upgrading.
func MigrateTimeColumnsToUTC(database *gorm.DB, dbType string, models []interface{}, legacyTimezone string) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	if err := database.AutoMigrate(&SchemaFlag{}); err != nil {
		return err
	}
	var count int64
	if err := database.Model(&SchemaFlag{}).Where("name = ?", TimeUTCMigrationName).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	dbType = strings.ToLower(strings.TrimSpace(dbType))
	detail := dbType
	var mysqlOffset string
	if dbType == "mysql" {
		loc := time.Local
		if strings.TrimSpace(legacyTimezone) != "" {
			l, err := time.LoadLocation(strings.TrimSpace(legacyTimezone))
			if err != nil {
				return fmt.Errorf("invalid database.legacy_timezone %q: %w", legacyTimezone, err)
			}
			loc = l
		}
		_, offset := time.Now().In(loc).Zone()
		mysqlOffset = formatOffset(offset)
		detail = fmt.Sprintf("mysql from %s (%s)", loc.String(), mysqlOffset)
	}

	err := database.Transaction(func(tx *gorm.DB) error {
		if dbType == "sqlite" || dbType == "mysql" {
			targets, err := timeColumns(tx, models)
			if err != nil {
				return err
			}
			for table, columns := range targets {
				if !tx.Migrator().HasTable(table) {
					continue
				}
				for _, column := range columns {
					if !tx.Migrator().HasColumn(table, column) {
						continue
					}
					var stmt string
					args := []interface{}{}
					switch dbType {
					case "sqlite":
						stmt = fmt.Sprintf("UPDATE `%s` SET `%s` = strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ', `%s`) "+
							"WHERE `%s` IS NOT NULL AND typeof(`%s`) = 'text' AND `%s` NOT LIKE '%%Z' AND strftime('%%s', `%s`) IS NOT NULL",
							table, column, column, column, column, column, column)
					case "mysql":
						if mysqlOffset == "+00:00" {
							continue
						}
						stmt = fmt.Sprintf("UPDATE `%s` SET `%s` = CONVERT_TZ(`%s`, ?, '+00:00') WHERE `%s` IS NOT NULL AND `%s` > '0001-01-02'",
							table, column, column, column, column)
						args = append(args, mysqlOffset)
					}
					if err := tx.Exec(stmt, args...).Error; err != nil {
						return fmt.Errorf("migrate %s.%s: %w", table, column, err)
					}
				}
			}
		}
		return tx.Create(&SchemaFlag{Name: TimeUTCMigrationName, Detail: detail, AppliedAt: timex.Now()}).Error
	})
	if err != nil {
		return err
	}
	log.Printf("[Database] time columns migrated to UTC (%s)\n", detail)
	return nil
}

var (
	timeReflectType    = reflect.TypeOf(time.Time{})
	timePtrReflectType = reflect.TypeOf(&time.Time{})
)

// timeColumns 解析模型，返回 表 -> time.Time 列 的映射。/ timeColumns maps table -> time.Time columns.
func timeColumns(database *gorm.DB, models []interface{}) (map[string][]string, error) {
	result := map[string][]string{}
	for _, model := range models {
		stmt := &gorm.Statement{DB: database}
		if err := stmt.Parse(model); err != nil {
			return nil, err
		}
		for _, field := range stmt.Schema.Fields {
			if field.DBName == "" {
				continue
			}
			if field.FieldType == timeReflectType || field.FieldType == timePtrReflectType || field.DataType == schema.Time {
				result[stmt.Schema.Table] = append(result[stmt.Schema.Table], field.DBName)
			}
		}
	}
	return result, nil
}

func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	return fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, (seconds%3600)/60)
}
