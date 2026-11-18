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
	"testing"
	"time"

	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestMigrateLegacyRemoteAlerts 覆盖三类记录：外部未恢复告警被收尾、外部已恢复保持原样、
// 内嵌评估器告警（generator_url 为空）不受影响；且迁移幂等、不产生通知投递记录。
// TestMigrateLegacyRemoteAlerts covers the three record kinds: unresolved external alerts get
// resolved, already-resolved external alerts stay untouched, and embedded-evaluator alerts
// (empty generator_url) are unaffected; the migration is idempotent and writes no
// notification delivery rows.
func TestMigrateLegacyRemoteAlerts(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(
		&monitoringapp.RemoteAlertRecord{},
		&monitoringapp.NotificationDelivery{},
	))

	oldResolvedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	legacyFiring := &monitoringapp.RemoteAlertRecord{
		Fingerprint: "legacy-firing", StartsAt: 100, Status: "firing",
		GeneratorURL: "http://prometheus:9090/graph", AlertName: "LegacyFiring",
	}
	legacyResolved := &monitoringapp.RemoteAlertRecord{
		Fingerprint: "legacy-resolved", StartsAt: 200, Status: "resolved",
		GeneratorURL: "http://prometheus:9090/graph", AlertName: "LegacyResolved",
		EndsAt: 300, ResolvedAt: &oldResolvedAt,
	}
	embeddedFiring := &monitoringapp.RemoteAlertRecord{
		Fingerprint: "embedded-firing", StartsAt: 400, Status: "firing",
		GeneratorURL: "", AlertName: "EmbeddedFiring",
	}
	require.NoError(t, database.Create(legacyFiring).Error)
	require.NoError(t, database.Create(legacyResolved).Error)
	require.NoError(t, database.Create(embeddedFiring).Error)

	require.NoError(t, MigrateLegacyRemoteAlerts(database))

	fetch := func(id uint) monitoringapp.RemoteAlertRecord {
		var rec monitoringapp.RemoteAlertRecord
		require.NoError(t, database.First(&rec, id).Error)
		return rec
	}

	// 外部未恢复 → resolved，收尾字段有值。
	// External unresolved -> resolved, with close-out fields stamped.
	after := fetch(legacyFiring.ID)
	require.Equal(t, "resolved", after.Status)
	require.NotNil(t, after.ResolvedAt)
	require.Positive(t, after.EndsAt)

	// 外部已恢复 → 完全不动。/ External already-resolved -> fully untouched.
	after = fetch(legacyResolved.ID)
	require.Equal(t, "resolved", after.Status)
	require.Equal(t, int64(300), after.EndsAt)
	require.Equal(t, oldResolvedAt.Unix(), after.ResolvedAt.Unix())

	// 内嵌评估器告警 → 完全不动。/ Embedded-evaluator alert -> fully untouched.
	after = fetch(embeddedFiring.ID)
	require.Equal(t, "firing", after.Status)
	require.Nil(t, after.ResolvedAt)
	require.Equal(t, int64(0), after.EndsAt)

	// 不产生恢复通知投递记录。/ No recovery-notification delivery rows are created.
	var deliveries int64
	require.NoError(t, database.Model(&monitoringapp.NotificationDelivery{}).Count(&deliveries).Error)
	require.Zero(t, deliveries)

	// 幂等：标记已写，第二次调用空跑，行不变。
	// Idempotent: the flag is written, a second run is a no-op.
	var flag SchemaFlag
	require.NoError(t, database.Where("name = ?", LegacyRemoteAlertsMigrationName).First(&flag).Error)
	require.NoError(t, MigrateLegacyRemoteAlerts(database))
	after = fetch(embeddedFiring.ID)
	require.Equal(t, "firing", after.Status)
}
