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
//   - MySQL：DATETIME 不带时区，旧 DSN 为 loc=Local，按 legacyTimezone（为空则进程 time.Local）换算：固定偏移时区直接 CONVERT_TZ；
//     有夏令时的时区按每行日期的偏移换算（优先 MySQL 时区表，未加载时在 Go 中换算）。
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
	flagged, err := timeUTCMigrationFlagged(database)
	if err != nil || flagged {
		return err
	}

	dbType = strings.ToLower(strings.TrimSpace(dbType))
	detail := dbType
	var mysqlPlan mysqlUTCPlan
	if dbType == "mysql" {
		loc := time.Local
		if strings.TrimSpace(legacyTimezone) != "" {
			l, err := time.LoadLocation(strings.TrimSpace(legacyTimezone))
			if err != nil {
				return fmt.Errorf("invalid database.legacy_timezone %q: %w", legacyTimezone, err)
			}
			loc = l
		}
		mysqlPlan = planMySQLUTC(database, loc)
		detail = fmt.Sprintf("mysql from %s (%s)", loc.String(), mysqlPlan.describe())
	}

	err = database.Transaction(func(tx *gorm.DB) error {
		// 先占位写入标记：并发启动时，后到的实例在主键冲突处失败并整体回滚，不会重复改写数据。
		// Claim the flag first: a concurrent instance fails on the primary key and rolls back before touching data.
		if err := tx.Create(&SchemaFlag{Name: TimeUTCMigrationName, Detail: detail, AppliedAt: timex.Now()}).Error; err != nil {
			return err
		}
		if dbType != "sqlite" && dbType != "mysql" {
			return nil
		}
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
				var err error
				switch dbType {
				case "sqlite":
					stmt := fmt.Sprintf("UPDATE `%s` SET `%s` = strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ', `%s`) "+
						"WHERE `%s` IS NOT NULL AND typeof(`%s`) = 'text' AND `%s` NOT LIKE '%%Z' AND strftime('%%s', `%s`) IS NOT NULL",
						table, column, column, column, column, column, column)
					err = tx.Exec(stmt).Error
				case "mysql":
					err = mysqlPlan.migrate(tx, table, column)
				}
				if err != nil {
					return fmt.Errorf("migrate %s.%s: %w", table, column, err)
				}
			}
		}
		return nil
	})
	if err != nil {
		// 另一个实例已经完成迁移（标记主键冲突）时视为成功。/ Treat a flag written by a concurrent instance as success.
		if flagged, checkErr := timeUTCMigrationFlagged(database); checkErr == nil && flagged {
			log.Printf("[Database] time columns UTC migration already applied by another instance\n")
			return nil
		}
		return err
	}
	log.Printf("[Database] time columns migrated to UTC (%s)\n", detail)
	return nil
}

func timeUTCMigrationFlagged(database *gorm.DB) (bool, error) {
	var count int64
	if err := database.Model(&SchemaFlag{}).Where("name = ?", TimeUTCMigrationName).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// mysqlUTCPlan 描述 MySQL DATETIME 列从旧时区换算到 UTC 的方式。/ mysqlUTCPlan describes how MySQL DATETIME columns are converted.
//   - fixed：时区历史上偏移恒定，按固定偏移 CONVERT_TZ（最快）。
//   - named：偏移会变化（夏令时等）且 MySQL 已加载时区表，用 CONVERT_TZ(col, '<IANA>', '+00:00') 按行换算。
//   - goRows：偏移会变化但 MySQL 未加载时区表，在 Go 中用 time.ParseInLocation 按值换算。
type mysqlUTCPlan struct {
	loc    *time.Location
	mode   string
	offset string
}

const (
	mysqlUTCModeFixed  = "fixed"
	mysqlUTCModeNamed  = "named"
	mysqlUTCModeGoRows = "go-rows"
)

func planMySQLUTC(database *gorm.DB, loc *time.Location) mysqlUTCPlan {
	if offset, ok := fixedOffset(loc); ok {
		return mysqlUTCPlan{loc: loc, mode: mysqlUTCModeFixed, offset: formatOffset(offset)}
	}
	if mysqlNamedZoneProbe(database, loc.String()) {
		return mysqlUTCPlan{loc: loc, mode: mysqlUTCModeNamed}
	}
	return mysqlUTCPlan{loc: loc, mode: mysqlUTCModeGoRows}
}

func (p mysqlUTCPlan) describe() string {
	if p.mode == mysqlUTCModeFixed {
		return p.offset
	}
	return p.mode
}

// fixedOffset 判断时区在 2000-2037 年间（本产品数据可能出现的范围）偏移是否恒定。
// fixedOffset reports whether the zone keeps one UTC offset over 2000-2037 (the range product data can fall in).
func fixedOffset(loc *time.Location) (int, bool) {
	_, first := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).In(loc).Zone()
	for year := 2000; year <= 2037; year++ {
		for month := time.January; month <= time.December; month++ {
			if _, offset := time.Date(year, month, 15, 0, 0, 0, 0, time.UTC).In(loc).Zone(); offset != first {
				return 0, false
			}
		}
	}
	return first, true
}

// mysqlNamedZoneProbe 可在测试中替换以强制走 Go 换算路径。/ mysqlNamedZoneProbe is swappable in tests to force the Go path.
var mysqlNamedZoneProbe = mysqlNamedZoneSupported

// mysqlNamedZoneSupported 探测 MySQL 是否加载了时区表（未加载时 CONVERT_TZ 对命名时区返回 NULL）。
// mysqlNamedZoneSupported probes whether MySQL time zone tables are loaded (CONVERT_TZ returns NULL otherwise).
func mysqlNamedZoneSupported(database *gorm.DB, zone string) bool {
	if zone == "" || zone == "Local" {
		return false
	}
	var converted *string
	if err := database.Raw("SELECT CAST(CONVERT_TZ('2026-01-15 12:00:00', ?, '+00:00') AS CHAR)", zone).Scan(&converted).Error; err != nil {
		return false
	}
	return converted != nil && *converted != ""
}

func (p mysqlUTCPlan) migrate(tx *gorm.DB, table, column string) error {
	switch p.mode {
	case mysqlUTCModeFixed:
		if p.offset == "+00:00" {
			return nil
		}
		stmt := fmt.Sprintf("UPDATE `%s` SET `%s` = CONVERT_TZ(`%s`, ?, '+00:00') WHERE `%s` IS NOT NULL AND `%s` > '0001-01-02'",
			table, column, column, column, column)
		return tx.Exec(stmt, p.offset).Error
	case mysqlUTCModeNamed:
		stmt := fmt.Sprintf("UPDATE `%s` SET `%s` = CONVERT_TZ(`%s`, ?, '+00:00') WHERE `%s` IS NOT NULL AND `%s` > '0001-01-02'",
			table, column, column, column, column)
		return tx.Exec(stmt, p.loc.String()).Error
	default:
		return migrateMySQLColumnInGo(tx, table, column, p.loc)
	}
}

const mysqlLegacyDatetimeLayout = "2006-01-02 15:04:05.999999"

// migrateMySQLColumnInGo 读出列的不同取值，在 Go 中按各自日期的偏移换算，再经临时映射表一次性 UPDATE JOIN，
// 避免逐值更新时新值与尚未处理的旧值相撞。夏令时回拨的重复时刻取较早的一次（time.ParseInLocation 语义）。
// migrateMySQLColumnInGo converts distinct values in Go with the offset in effect on each date and applies them
// with one UPDATE JOIN through a temporary mapping table, so converted values never collide with pending ones.
// Ambiguous wall times during a DST fall-back resolve like time.ParseInLocation.
func migrateMySQLColumnInGo(tx *gorm.DB, table, column string, loc *time.Location) error {
	var values []string
	query := fmt.Sprintf("SELECT DISTINCT CAST(`%s` AS CHAR) FROM `%s` WHERE `%s` IS NOT NULL AND `%s` > '0001-01-02'",
		column, table, column, column)
	if err := tx.Raw(query).Scan(&values).Error; err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	if err := tx.Exec("DROP TEMPORARY TABLE IF EXISTS stx_utc_migration_map").Error; err != nil {
		return err
	}
	if err := tx.Exec("CREATE TEMPORARY TABLE stx_utc_migration_map (old_value DATETIME(6) NOT NULL PRIMARY KEY, new_value DATETIME(6) NOT NULL)").Error; err != nil {
		return err
	}
	defer tx.Exec("DROP TEMPORARY TABLE IF EXISTS stx_utc_migration_map")

	const batchSize = 500
	for start := 0; start < len(values); start += batchSize {
		end := start + batchSize
		if end > len(values) {
			end = len(values)
		}
		placeholders := make([]string, 0, end-start)
		args := make([]interface{}, 0, 2*(end-start))
		for _, raw := range values[start:end] {
			local, err := time.ParseInLocation(mysqlLegacyDatetimeLayout, raw, loc)
			if err != nil {
				return fmt.Errorf("parse %q: %w", raw, err)
			}
			placeholders = append(placeholders, "(?, ?)")
			args = append(args, raw, local.UTC().Format("2006-01-02 15:04:05.000000"))
		}
		stmt := "INSERT INTO stx_utc_migration_map (old_value, new_value) VALUES " + strings.Join(placeholders, ", ")
		if err := tx.Exec(stmt, args...).Error; err != nil {
			return err
		}
	}
	stmt := fmt.Sprintf("UPDATE `%s` AS t JOIN stx_utc_migration_map AS m ON t.`%s` = m.old_value SET t.`%s` = m.new_value",
		table, column, column)
	return tx.Exec(stmt).Error
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
