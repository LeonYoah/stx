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
	"context"
	"testing"
	"time"
)

// 引擎先报 FAILED、稍后才给 errorMsg：失败窗口内的作业要补上错误信息，即使已有终态指标块；窗口外的不再请求。
// The engine reports FAILED before errorMsg: jobs inside the failure window get their error backfilled even with a
// final metrics block; jobs outside the window are left alone.
func TestBackfillFailedJobErrorsFillsLateEngineError(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	engine := &stubEngineClient{info: &EngineJobInfo{JobID: "e1", JobStatus: "FAILED", ErrorMsg: "SchemaValidationException: SE-07"}}
	service.engineClient = engine

	now := time.Now().UTC()
	newFailedJob := func(engineJobID string, finishedAgo time.Duration) *JobInstance {
		finishedAt := now.Add(-finishedAgo)
		job := &JobInstance{
			TaskID:      1,
			TaskVersion: 1,
			RunType:     RunTypeRun,
			Status:      JobStatusFailed,
			EngineJobID: engineJobID,
			FinishedAt:  &finishedAt,
			SubmitSpec:  JSONMap{"engine_base_url": "http://engine:8080", "engine_api_mode": "v2"},
			// 已有终态指标块：旧逻辑会因此跳过刷新。/ A final metrics block: the old logic skipped refresh because of it.
			ResultPreview: JSONMap{"job_status": "FAILED", "stx_metrics_log": "STX metrics final snapshot"},
		}
		if err := service.repo.CreateJobInstance(ctx, job); err != nil {
			t.Fatalf("create job: %v", err)
		}
		return job
	}
	recent := newFailedJob("e1", time.Minute)
	stale := newFailedJob("e2", 10*time.Minute)

	service.backfillFailedJobErrors(ctx, now)
	deadline := time.Now().Add(5 * time.Second)
	for service.errorBackfill.running.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	got, err := service.repo.GetJobInstanceByID(ctx, recent.ID)
	if err != nil {
		t.Fatalf("get recent job: %v", err)
	}
	if got.ErrorMessage != "SchemaValidationException: SE-07" || got.Status != JobStatusFailed {
		t.Fatalf("expected late engine error to be backfilled, got status=%s error=%q", got.Status, got.ErrorMessage)
	}
	old, err := service.repo.GetJobInstanceByID(ctx, stale.ID)
	if err != nil {
		t.Fatalf("get stale job: %v", err)
	}
	if old.ErrorMessage != "" {
		t.Fatalf("jobs outside the window must not be probed, got %q", old.ErrorMessage)
	}

	// 同一作业 15 秒内不重复补拉。/ The same job is not probed again within 15 seconds.
	if service.errorBackfill.due(recent.ID, now.Add(time.Second)) {
		t.Fatalf("expected the probe interval to throttle repeated backfills")
	}
}
