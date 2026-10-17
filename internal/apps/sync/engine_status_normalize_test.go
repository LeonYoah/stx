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

func TestNormalizeJobStatusEngineAligned(t *testing.T) {
	cases := map[string]JobStatus{
		"RUNNING":          JobStatusRunning,
		"DOING_SAVEPOINT":  JobStatusDoingSavepoint,
		"SAVEPOINT_DONE":   JobStatusSavepointDone,
		"FAILING":          JobStatusFailing,
		"FAILED":           JobStatusFailed,
		"FINISHED":         JobStatusSuccess,
		"CANCELED":         JobStatusCanceled,
		"CANCELLED":        JobStatusCanceled,
		"CREATED":          JobStatusPending,
		"CANCELING":        JobStatusCancelling,
	}
	for raw, want := range cases {
		if got := normalizeJobStatus(raw); got != want {
			t.Fatalf("%s: got %s want %s", raw, got, want)
		}
	}
	if !isFinalNormalizedJobStatus(JobStatusSavepointDone) {
		t.Fatal("savepoint_done must be terminal")
	}
	if isFinalNormalizedJobStatus(JobStatusFailing) || isFinalNormalizedJobStatus(JobStatusDoingSavepoint) {
		t.Fatal("failing/doing_savepoint must stay non-terminal")
	}
	if !isActiveJobStatus(JobStatusFailing) || !isActiveJobStatus(JobStatusDoingSavepoint) {
		t.Fatal("failing/doing_savepoint must stay active for reconciler")
	}
}
