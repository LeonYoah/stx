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
	"time"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
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
// 并发安全：先在事务里占位写 stx_schema_flags 标记，后到的实例主键冲突回滚，不会重复执行。
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
// Concurrency-safe: the transaction claims the stx_schema_flags flag first, so a racing
// instance fails on the primary key and rolls back before touching data. Idempotent: an
// existing flag short-circuits the run.
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
			AppliedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		now := time.Now()
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
