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
	"testing"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
)

// TestMigrateDeletesLegacyCustomPromQLPolicies 验证迁移删除存量 custom_promql 策略且幂等。
// TestMigrateDeletesLegacyCustomPromQLPolicies verifies migration deletes stored
// custom_promql policies while keeping other policy types, and is idempotent.
func TestMigrateDeletesLegacyCustomPromQLPolicies(t *testing.T) {
	dbs := getTestDatabaseDialectors(t)
	if len(dbs) == 0 {
		t.Fatal("未发现任何可用的数据库进行测试 / No available database found for testing")
	}

	for dbType, targetDB := range dbs {
		t.Run(fmt.Sprintf("CustomPromQLCleanup_%s", dbType), func(t *testing.T) {
			if err := MigrateWithDB(targetDB, dbType); err != nil {
				t.Fatalf("[%s] 首次迁移失败 / First migration failed: %v", dbType, err)
			}

			// 造数：一条 custom_promql + 一条 metrics_template + 关联 notification_delivery 行。
			// Seed: one custom_promql + one metrics_template policy plus a notification_delivery
			// row referencing the legacy policy_id.
			legacy := &monitoringapp.AlertPolicy{
				Name:       "legacy custom promql",
				PolicyType: "custom_promql",
				Severity:   monitoringapp.AlertSeverityWarning,
				Enabled:    true,
			}
			kept := &monitoringapp.AlertPolicy{
				Name:        "cpu template",
				PolicyType:  monitoringapp.AlertPolicyBuilderKindMetricsTemplate,
				TemplateKey: "seatunnel_cpu_usage",
				Severity:    monitoringapp.AlertSeverityWarning,
				Enabled:     true,
			}
			if err := targetDB.Create(legacy).Error; err != nil {
				t.Fatalf("[%s] 创建 legacy policy 失败: %v", dbType, err)
			}
			if err := targetDB.Create(kept).Error; err != nil {
				t.Fatalf("[%s] 创建 metrics_template policy 失败: %v", dbType, err)
			}
			delivery := &monitoringapp.NotificationDelivery{
				SourceKey: "legacy-fp-1",
				PolicyID:  legacy.ID,
				ChannelID: 1,
				EventType: "firing",
				Status:    "success",
			}
			if err := targetDB.Create(delivery).Error; err != nil {
				t.Fatalf("[%s] 创建 notification delivery 失败: %v", dbType, err)
			}

			// 第二遍迁移应删除 custom_promql 行、保留其余行（幂等）。
			// A second migration run deletes custom_promql rows, keeps the rest (idempotent).
			if err := MigrateWithDB(targetDB, dbType); err != nil {
				t.Fatalf("[%s] 幂等二次迁移失败 / Second migration failed: %v", dbType, err)
			}

			var remaining int64
			if err := targetDB.Model(&monitoringapp.AlertPolicy{}).
				Where("policy_type = ?", "custom_promql").
				Count(&remaining).Error; err != nil {
				t.Fatalf("[%s] 统计 custom_promql 行失败: %v", dbType, err)
			}
			if remaining != 0 {
				t.Fatalf("[%s] 预期 0 条 custom_promql 策略，实际 %d", dbType, remaining)
			}

			var keptCount int64
			if err := targetDB.Model(&monitoringapp.AlertPolicy{}).
				Where("id = ?", kept.ID).
				Count(&keptCount).Error; err != nil {
				t.Fatalf("[%s] 统计保留策略失败: %v", dbType, err)
			}
			if keptCount != 1 {
				t.Fatalf("[%s] 预期 metrics_template 策略保留，实际 %d 条", dbType, keptCount)
			}

			// notification_deliveries.policy_id 无 DB 级外键，投递历史保留为悬挂引用。
			// notification_deliveries.policy_id has no DB-level FK; delivery history is kept as
			// a dangling reference.
			var deliveryCount int64
			if err := targetDB.Model(&monitoringapp.NotificationDelivery{}).
				Where("id = ?", delivery.ID).
				Count(&deliveryCount).Error; err != nil {
				t.Fatalf("[%s] 统计投递记录失败: %v", dbType, err)
			}
			if deliveryCount != 1 {
				t.Fatalf("[%s] 预期投递历史保留，实际 %d 条", dbType, deliveryCount)
			}

			// 第三遍迁移保持幂等。/ Third run stays idempotent.
			if err := MigrateWithDB(targetDB, dbType); err != nil {
				t.Fatalf("[%s] 第三次迁移失败: %v", dbType, err)
			}
		})
	}
}
