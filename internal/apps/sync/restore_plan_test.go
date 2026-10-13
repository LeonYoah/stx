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

import "testing"

func TestResolveRestorePlan(t *testing.T) {
	const sourceID = "177467000000000001"

	t.Run("legacy 2.x uses savepoint flag path", func(t *testing.T) {
		plan := resolveRestorePlan("2.3.12", "FAILED", sourceID)
		if !plan.LegacySavepoint || plan.RestoreMode != "" {
			t.Fatalf("expected legacy savepoint plan, got %+v", plan)
		}
		if plan.PlatformJobID != sourceID {
			t.Fatalf("expected reused platform job id, got %q", plan.PlatformJobID)
		}
	})

	t.Run("3.0 savepoint done uses SAVEPOINT mode", func(t *testing.T) {
		plan := resolveRestorePlan("3.0.0", "SAVEPOINT_DONE", sourceID)
		if plan.LegacySavepoint || plan.RestoreMode != RestoreModeSavepoint {
			t.Fatalf("expected SAVEPOINT mode, got %+v", plan)
		}
		if plan.PlatformJobID != sourceID || plan.RestoreSourceJobID != sourceID {
			t.Fatalf("expected same job id for savepoint recover, got %+v", plan)
		}
	})

	t.Run("3.0 failed uses CHECKPOINT mode with new job id assigned later", func(t *testing.T) {
		plan := resolveRestorePlan("3.0.0", "FAILED", sourceID)
		if plan.LegacySavepoint || plan.RestoreMode != RestoreModeCheckpoint {
			t.Fatalf("expected CHECKPOINT mode, got %+v", plan)
		}
		if plan.RestoreSourceJobID != sourceID {
			t.Fatalf("expected restore source %q, got %q", sourceID, plan.RestoreSourceJobID)
		}
		if plan.PlatformJobID != "" {
			t.Fatalf("checkpoint plan should defer platform job id to nextJobID(), got %q", plan.PlatformJobID)
		}
	})
}

func TestSourceJobLifecycleStatus(t *testing.T) {
	source := &JobInstance{
		Status: JobStatusFailed,
		ResultPreview: JSONMap{
			"job_status": "SAVEPOINT_DONE",
		},
	}
	if got := sourceJobLifecycleStatus(source); got != "SAVEPOINT_DONE" {
		t.Fatalf("expected result_preview job_status, got %q", got)
	}
}
