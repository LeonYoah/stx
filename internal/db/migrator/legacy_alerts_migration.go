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

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/pkg/timex"
	"gorm.io/gorm"
)

// LegacyRemoteAlertsMigrationName 是遗留告警收尾迁移的标记名。
// LegacyRemoteAlertsMigrationName is the flag name of the legacy remote-alert cleanup migration.
const LegacyRemoteAlertsMigrationName = "legacy_remote_alerts_resolved_v1"

// MigrateLegacyRemoteAlerts 把外部 Alertmanager 时代遗留的未恢复远程告警一次性标记为已恢复。
//
// 旧三件套告警来自 Alertmanager webhook，generator_url 非空；内嵌评估器产生的告警
// generator_url 为空。外部告警没有恢复事件通道，升级后这些记录会以非 resolved 状态永久
// 计入活跃告警，所以迁移把 generator_url 非空且状态未恢复的存量记录标记为 resolved，
// 并写入 ends_at / resolved_at 收尾时间。不发送恢复通知、不产生投递记录。
//
// 并发安全：先在事务里占位写 stx_schema_flags 标记；真有两个实例并发启动时，后到实例
// 会因主键冲突整体回滚并把错误抛给调用方——当前 Migrate() 对迁移错误是 log.Fatalf，
// 即后到实例直接退出（STX 单控制面部署，不会发生；语义宁可退出也不重复执行）。
// 幂等：标记存在即跳过；重复 UPDATE 也只影响仍为未恢复的行。
//
// MigrateLegacyRemoteAlerts marks leftover unresolved remote alerts from the external
// Alertmanager era as resolved, once.
//
// Legacy trio alerts arrive through the Alertmanager webhook and carry a non-empty
// generator_url; the embedded evaluator leaves generator_url empty. External alerts have no
// recovery channel, so after upgrade these rows would count as active forever; the migration
// marks stored rows with non-empty generator_url and a non-resolved status as resolved and
// stamps ends_at / resolved_at. No recovery notifications are sent and no delivery rows are
// created.
//
// Concurrency-safe: the transaction claims the stx_schema_flags flag first; if two instances
// ever raced, the later one fails on the primary key, rolls back and returns the error — the
// caller (Migrate) treats migration errors with log.Fatalf, i.e. the losing instance exits
// (STX is a single control plane, so this never happens in practice; exiting beats running
// the update twice). Idempotent: an existing flag short-circuits the run.
func MigrateLegacyRemoteAlerts(database *gorm.DB) error {
	if database == nil {
		return fmt.Errorf("database is nil")
	}
	if err := database.AutoMigrate(&SchemaFlag{}); err != nil {
		return err
	}
	if !database.Migrator().HasTable(&monitoringapp.RemoteAlertRecord{}) {
		return nil
	}
	var flagged int64
	if err := database.Model(&SchemaFlag{}).Where("name = ?", LegacyRemoteAlertsMigrationName).Count(&flagged).Error; err != nil {
		return err
	}
	if flagged > 0 {
		return nil
	}

	var affected int64
	err := database.Transaction(func(tx *gorm.DB) error {
		// 先占位写标记：并发启动时后到实例主键冲突并整体回滚。
		// Claim the flag first: a concurrent instance hits the primary key and rolls back.
		if err := tx.Create(&SchemaFlag{
			Name:      LegacyRemoteAlertsMigrationName,
			Detail:    "resolve unresolved remote alerts with non-empty generator_url",
			AppliedAt: timex.Now(),
		}).Error; err != nil {
			return err
		}
		// 时间列统一 UTC（timex.Now），否则 SQLite 会按本地偏移写 +08:00 文本，
		// 与其他行的 Z 后缀混排导致文本排序错位。
		// Time columns are stored in UTC (timex.Now); otherwise SQLite would write a
		// local-offset +08:00 text that mis-sorts against the Z-suffixed rows.
		now := timex.Now()
		res := tx.Model(&monitoringapp.RemoteAlertRecord{}).
			Where("generator_url <> ''").
			Where("LOWER(status) <> ?", "resolved").
			Updates(map[string]interface{}{
				"status":      "resolved",
				"ends_at":     now.Unix(),
				"resolved_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		affected = res.RowsAffected
		return nil
	})
	if err != nil {
		return err
	}
	if affected > 0 {
		log.Printf("[Database] 已收尾 %d 条遗留外部告警（标记 resolved，不发送通知）/ resolved %d legacy external alerts (no notifications sent)\n", affected, affected)
	}
	return nil
}
