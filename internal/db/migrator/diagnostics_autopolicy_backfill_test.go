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

	"github.com/LeonYoah/stx/internal/apps/diagnostics"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&diagnostics.ClusterInspectionReport{}))
	return database
}

func insertLegacyAutoReport(t *testing.T, database *gorm.DB, reason string) diagnostics.ClusterInspectionReport {
	t.Helper()

	report := diagnostics.ClusterInspectionReport{
		ClusterID:         1,
		Status:            diagnostics.InspectionReportStatusCompleted,
		TriggerSource:     diagnostics.InspectionTriggerSourceAuto,
		AutoTriggerReason: reason,
	}
	require.NoError(t, database.Create(&report).Error)
	return report
}

func TestBackfillDiagnosticsReportAutoPolicyIDs(t *testing.T) {
	database := newBackfillTestDB(t)

	matched := insertLegacyAutoReport(t, database, "policy:42|JAVA_OOM: heap exhausted")
	unparseable := insertLegacyAutoReport(t, database, "policy:abc|JAVA_OOM")
	manual := diagnostics.ClusterInspectionReport{
		ClusterID:     1,
		Status:        diagnostics.InspectionReportStatusCompleted,
		TriggerSource: diagnostics.InspectionTriggerSourceDiagnosticsWorkspace,
	}
	require.NoError(t, database.Create(&manual).Error)

	require.NoError(t, backfillDiagnosticsReportAutoPolicyIDs(database))

	var gotMatched diagnostics.ClusterInspectionReport
	require.NoError(t, database.First(&gotMatched, matched.ID).Error)
	assert.Equal(t, uint(42), gotMatched.AutoPolicyID)

	var gotUnparseable diagnostics.ClusterInspectionReport
	require.NoError(t, database.First(&gotUnparseable, unparseable.ID).Error)
	assert.Equal(t, uint(0), gotUnparseable.AutoPolicyID)

	var gotManual diagnostics.ClusterInspectionReport
	require.NoError(t, database.First(&gotManual, manual.ID).Error)
	assert.Equal(t, uint(0), gotManual.AutoPolicyID)

	// 幂等：重跑不改动已回填行。/ Idempotent: a rerun leaves the backfilled rows untouched.
	require.NoError(t, backfillDiagnosticsReportAutoPolicyIDs(database))
	var gotAgain diagnostics.ClusterInspectionReport
	require.NoError(t, database.First(&gotAgain, matched.ID).Error)
	assert.Equal(t, uint(42), gotAgain.AutoPolicyID)
}

func TestBackfillDiagnosticsReportAutoPolicyIDsBatches(t *testing.T) {
	database := newBackfillTestDB(t)

	// 超过一个批次（500）验证游标推进。/ Exceed one batch (500) to prove the id cursor advances.
	const total = 505
	for i := 0; i < total; i++ {
		insertLegacyAutoReport(t, database, "policy:7|SCHEDULED cron=* * * * *")
	}
	require.NoError(t, backfillDiagnosticsReportAutoPolicyIDs(database))

	var count int64
	require.NoError(t, database.Model(&diagnostics.ClusterInspectionReport{}).Where("auto_policy_id = 7").Count(&count).Error)
	assert.Equal(t, int64(total), count)
}

func TestParseAutoTriggerPolicyID(t *testing.T) {
	cases := []struct {
		reason   string
		wantID   uint
		wantOkay bool
	}{
		{"policy:42|JAVA_OOM: x", 42, true},
		{"policy:7|SCHEDULED cron=* * * * *", 7, true},
		{"policy:9", 9, true},
		{" policy:3|X ", 3, true},
		{"policy:abc|X", 0, false},
		{"policy:0|X", 0, false},
		{"policy:|X", 0, false},
		{"policyx:5|X", 0, false},
		{"manual reason", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		id, ok := parseAutoTriggerPolicyID(tc.reason)
		assert.Equal(t, tc.wantOkay, ok, "reason=%q", tc.reason)
		assert.Equal(t, tc.wantID, id, "reason=%q", tc.reason)
	}
}
