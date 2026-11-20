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
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/diagnostics"
	"gorm.io/gorm"
)

// backfillDiagnosticsReportAutoPolicyIDs 回填存量巡检报告的 auto_policy_id：
// 历史自动触发报告只在 auto_trigger_reason 里写 "policy:<id>|<细节>" 前缀，
// 这里按 id 游标分批解析前缀写回；解析失败的行跳过，重跑幂等。
// backfillDiagnosticsReportAutoPolicyIDs backfills auto_policy_id on stored
// inspection reports: legacy auto-triggered rows only carry a
// "policy:<id>|<detail>" prefix in auto_trigger_reason, so this parses that
// prefix in id-cursor batches. Unparseable rows are skipped and reruns are idempotent.
func backfillDiagnosticsReportAutoPolicyIDs(database *gorm.DB) error {
	if database == nil || !database.Migrator().HasTable(&diagnostics.ClusterInspectionReport{}) {
		return nil
	}

	const batchSize = 500
	var lastID uint
	for {
		var rows []diagnostics.ClusterInspectionReport
		if err := database.
			Select("id", "auto_trigger_reason").
			Where("id > ? AND auto_policy_id = 0 AND auto_trigger_reason LIKE 'policy:%'", lastID).
			Order("id").
			Limit(batchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			lastID = row.ID
			policyID, ok := parseAutoTriggerPolicyID(row.AutoTriggerReason)
			if !ok {
				continue
			}
			if err := database.Model(&diagnostics.ClusterInspectionReport{}).
				Where("id = ?", row.ID).
				Update("auto_policy_id", policyID).Error; err != nil {
				return err
			}
		}
	}
}

// parseAutoTriggerPolicyID 提取 "policy:<id>|..." 前缀里的策略 ID。
// parseAutoTriggerPolicyID extracts the policy id from a "policy:<id>|..." prefix.
func parseAutoTriggerPolicyID(reason string) (uint, bool) {
	trimmed := strings.TrimSpace(reason)
	if !strings.HasPrefix(trimmed, "policy:") {
		return 0, false
	}
	raw := strings.TrimPrefix(trimmed, "policy:")
	if idx := strings.IndexByte(raw, '|'); idx >= 0 {
		raw = raw[:idx]
	}
	id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}
