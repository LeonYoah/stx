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

package sync

import (
	"strings"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

const (
	// RestoreModeCheckpoint 从最新 completed checkpoint 恢复（3.0+，新 jobId）。
	// RestoreModeCheckpoint restores from latest completed checkpoint (3.0+, new jobId).
	RestoreModeCheckpoint = "CHECKPOINT"
	// RestoreModeSavepoint 从 savepoint 恢复（3.0+，复用 jobId）。
	// RestoreModeSavepoint restores from savepoint (3.0+, reuse jobId).
	RestoreModeSavepoint = "SAVEPOINT"
)

// JobRestorePlan 描述一次 recover 提交应使用的引擎参数。
// JobRestorePlan describes engine submit parameters for one recover action.
type JobRestorePlan struct {
	PlatformJobID      string
	RestoreMode        string
	RestoreSourceJobID string
	LegacySavepoint    bool
}

// sourceJobLifecycleStatus 与 Studio getDisplayJobLifecycleStatus 对齐。
// sourceJobLifecycleStatus aligns with Studio getDisplayJobLifecycleStatus.
func sourceJobLifecycleStatus(source *JobInstance) string {
	if source == nil {
		return ""
	}
	if raw := strings.TrimSpace(stringValue(source.ResultPreview, "job_status")); raw != "" {
		return strings.ToUpper(raw)
	}
	// 无预览时把实例状态大写：savepoint_done → SAVEPOINT_DONE，2.x 仍忽略该值走旧路径。
	// Without a preview, uppercase the instance status so savepoint_done matches SAVEPOINT_DONE; 2.x still ignores it.
	return strings.ToUpper(strings.TrimSpace(string(source.Status)))
}

// engineSupportsRestoreMode 是否启用 3.0 restoreMode 参数（版本门控）。
// engineSupportsRestoreMode gates 3.0 restoreMode query parameters.
func engineSupportsRestoreMode(engineVersion string) bool {
	version := strings.TrimSpace(engineVersion)
	if version == "" {
		return false
	}
	return seatunnel.CompareVersions(version, "3.0.0") >= 0
}

// resolveRestorePlan 按引擎版本与源作业终态选择恢复模式。
// resolveRestorePlan picks restore mode from engine version and source terminal status.
func resolveRestorePlan(engineVersion, lifecycleStatus, sourcePlatformJobID string) JobRestorePlan {
	sourceID := strings.TrimSpace(sourcePlatformJobID)
	lifecycle := strings.ToUpper(strings.TrimSpace(lifecycleStatus))
	if !engineSupportsRestoreMode(engineVersion) {
		return JobRestorePlan{
			PlatformJobID:   sourceID,
			LegacySavepoint: true,
		}
	}
	if lifecycle == "SAVEPOINT_DONE" {
		return JobRestorePlan{
			PlatformJobID:      sourceID,
			RestoreMode:        RestoreModeSavepoint,
			RestoreSourceJobID: sourceID,
		}
	}
	return JobRestorePlan{
		RestoreMode:        RestoreModeCheckpoint,
		RestoreSourceJobID: sourceID,
	}
}
