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
	"errors"
	"strings"
	"testing"
	"time"
)

func TestFormatJobMetricsLogBlockIncludesBilingualLabels(t *testing.T) {
	block := FormatJobMetricsLogBlock(map[string]interface{}{
		"SourceReceivedCount": float64(20),
		"SinkWriteCount":      float64(20),
		"SourceReceivedQPS":   1.5,
	}, time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC), false)
	if !strings.Contains(block, "read_count:") {
		t.Fatalf("missing bilingual read label: %s", block)
	}
	if !strings.Contains(block, "20") {
		t.Fatalf("missing read count: %s", block)
	}
	if !strings.Contains(block, "\n") {
		t.Fatal("metrics block must preserve real newlines")
	}
}

func TestMaybeRecordMetricsLogRespectsIntervalAndForce(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	metrics := map[string]interface{}{"SourceReceivedCount": float64(1)}
	preview := maybeRecordMetricsLog(nil, metrics, now, false)
	first := metricsLogFromPreview(preview)
	if first == "" {
		t.Fatal("expected first metrics log")
	}
	preview = maybeRecordMetricsLog(preview, metrics, now.Add(10*time.Second), false)
	if metricsLogFromPreview(preview) != first {
		t.Fatal("should not append within 30s interval")
	}
	preview = maybeRecordMetricsLog(preview, metrics, now.Add(31*time.Second), false)
	second := metricsLogFromPreview(preview)
	if !strings.Contains(second, first) || strings.Count(second, "STX metrics snapshot") < 2 {
		t.Fatalf("expected second snapshot after interval, got %s", second)
	}
	preview = maybeRecordMetricsLog(preview, metrics, now.Add(32*time.Second), true)
	if strings.Count(metricsLogFromPreview(preview), "STX metrics final") != 1 {
		t.Fatalf("force should write final block: %s", metricsLogFromPreview(preview))
	}
}

func TestEngineJobInfoLooksOrphaned(t *testing.T) {
	if !engineJobInfoLooksOrphaned(&EngineJobInfo{JobID: "1"}) {
		t.Fatal("empty shell should look orphaned")
	}
	if engineJobInfoLooksOrphaned(&EngineJobInfo{JobID: "1", JobStatus: "RUNNING"}) {
		t.Fatal("running should not look orphaned")
	}
}

func TestNormalizeLogNewlinesUnescapesLiteral(t *testing.T) {
	got := normalizeLogNewlines(`line1\nline2`)
	if got != "line1\nline2" {
		t.Fatalf("got %q", got)
	}
	got = normalizeLogNewlines("line1\nline2")
	if got != "line1\nline2" {
		t.Fatalf("real newlines must stay: %q", got)
	}
}

func TestFormatLogPayloadKeepsJSONStringNewlines(t *testing.T) {
	got := formatLogPayload([]byte(`"alpha\nbeta"`))
	if got != "alpha\nbeta" {
		t.Fatalf("expected real newlines, got %q", got)
	}
}

func TestJobReconcilerMarksOrphanedRunningJobFailed(t *testing.T) {
	service := newTestSyncService(t)
	service.engineClient = &orphanEngineClient{}
	ctx := context.Background()
	job := &JobInstance{
		TaskID:      1,
		TaskVersion: 1,
		RunType:     RunTypeRun,
		Status:      JobStatusRunning,
		EngineJobID: "missing-job",
		SubmitSpec:  JSONMap{"engine_base_url": "http://127.0.0.1:8080", "cluster_id": 1},
		CreatedAt:   time.Now().Add(-2 * time.Minute),
		UpdatedAt:   time.Now().Add(-2 * time.Minute),
		CreatedBy:   1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	stored, err := service.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if stored.Status != JobStatusFailed {
		t.Fatalf("expected failed orphan, got %s", stored.Status)
	}
	if !strings.Contains(stored.ErrorMessage, "engine no longer has this job") {
		t.Fatalf("unexpected error message: %q", stored.ErrorMessage)
	}
}

type orphanEngineClient struct{}

func (o *orphanEngineClient) Submit(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error) {
	return nil, nil
}
func (o *orphanEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	return &EngineJobInfo{JobID: jobID}, nil
}
func (o *orphanEngineClient) GetJobCheckpointOverview(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineCheckpointOverview, error) {
	return nil, nil
}
func (o *orphanEngineClient) GetJobCheckpointHistory(ctx context.Context, endpoint *EngineEndpoint, jobID string, pipelineID *int, limit int, status string) ([]*EngineCheckpointRecord, error) {
	return nil, nil
}
func (o *orphanEngineClient) StopJob(ctx context.Context, endpoint *EngineEndpoint, jobID string, stopWithSavepoint bool) error {
	return nil
}
func (o *orphanEngineClient) GetJobLogs(ctx context.Context, endpoint *EngineEndpoint, jobID string) (string, error) {
	return "", nil
}
func (o *orphanEngineClient) ListRunningJobs(ctx context.Context, endpoint *EngineEndpoint) ([]*EngineJobListItem, error) {
	return []*EngineJobListItem{}, nil
}
func (o *orphanEngineClient) ListFinishedJobs(ctx context.Context, endpoint *EngineEndpoint, state string) ([]*EngineJobListItem, error) {
	return []*EngineJobListItem{}, nil
}


func TestJobReconcilerUsesSubmitSpecEndpointWithoutClusterID(t *testing.T) {
	service := newTestSyncService(t)
	service.engineClient = &orphanEngineClient{}
	ctx := context.Background()
	job := &JobInstance{
		TaskID:      1,
		TaskVersion: 1,
		RunType:     RunTypeRun,
		Status:      JobStatusRunning,
		EngineJobID: "no-cluster-1",
		SubmitSpec:  JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		CreatedAt:   time.Now().Add(-2 * time.Minute),
		UpdatedAt:   time.Now().Add(-2 * time.Minute),
		CreatedBy:   1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	stored, err := service.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if stored.Status != JobStatusFailed {
		t.Fatalf("expected failed via submit_spec endpoint, got %s", stored.Status)
	}
}

func TestHasFinalMetricsLogAndLatestBlockMarker(t *testing.T) {
	preview := maybeRecordMetricsLog(nil, map[string]interface{}{"SourceReceivedCount": float64(1)}, time.Now().UTC(), false)
	if hasFinalMetricsLog(preview) {
		t.Fatal("snapshot must not count as final")
	}
	preview = maybeRecordMetricsLog(preview, map[string]interface{}{"SourceReceivedCount": float64(9)}, time.Now().UTC().Add(time.Minute), true)
	if !hasFinalMetricsLog(preview) {
		t.Fatalf("expected final block: %s", metricsLogFromPreview(preview))
	}
	latest := latestMetricsBlockFromPreview(preview)
	if !strings.Contains(latest, "STX metrics final") {
		t.Fatalf("latest block should prefer final: %s", latest)
	}
}

func TestRefreshJobInstanceBackfillsFinalMetricsWhenOnlySnapshotExists(t *testing.T) {
	service := newTestSyncService(t)
	service.engineClient = &stubEngineClient{info: &EngineJobInfo{
		JobID:     "engine-final-metrics",
		JobStatus: "FINISHED",
		Metrics: map[string]interface{}{
			"SourceReceivedCount": float64(50),
			"SinkWriteCount":      float64(50),
		},
	}}
	ctx := context.Background()
	now := time.Now().UTC()
	preview := maybeRecordMetricsLog(nil, map[string]interface{}{"SourceReceivedCount": float64(10)}, now, false)
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusSuccess,
		PlatformJobID: "platform-final-metrics",
		EngineJobID:   "engine-final-metrics",
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080"},
		ResultPreview: preview,
		FinishedAt:    &now,
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if hasFinalMetricsLog(job.ResultPreview) {
		t.Fatal("precondition: only snapshot")
	}
	refreshed, err := service.RefreshJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !hasFinalMetricsLog(refreshed.ResultPreview) {
		t.Fatalf("expected final metrics after refresh: %s", metricsLogFromPreview(refreshed.ResultPreview))
	}
	metrics := mapValue(refreshed.ResultPreview, "metrics")
	if metrics == nil || metrics["SourceReceivedCount"] != float64(50) {
		t.Fatalf("structured metrics not updated: %#v", metrics)
	}
}

func TestReconcileFinishedWaitsForJobInfoBeforeSettling(t *testing.T) {
	service := newTestSyncService(t)
	client := &flakyFinishedEngineClient{failInfoTimes: 2, info: &EngineJobInfo{
		JobID:     "engine-flaky-finish",
		JobStatus: "FINISHED",
		Metrics:   map[string]interface{}{"SourceReceivedCount": float64(7), "SinkWriteCount": float64(7)},
	}}
	service.engineClient = client
	ctx := context.Background()
	job := &JobInstance{
		TaskID:        1,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "platform-flaky-finish",
		EngineJobID:   "engine-flaky-finish",
		SubmitSpec:    JSONMap{"engine_base_url": "http://127.0.0.1:8080", "cluster_id": 1},
		ResultPreview: maybeRecordMetricsLog(nil, map[string]interface{}{"SourceReceivedCount": float64(1)}, time.Now().UTC(), false),
		CreatedBy:     1,
		UpdatedAt:     time.Now().UTC(),
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	stored, err := service.repo.GetJobInstanceByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.Status != JobStatusRunning {
		t.Fatalf("first GetJobInfo failure must not settle terminal, got %s", stored.Status)
	}
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile 3: %v", err)
	}
	stored, err = service.repo.GetJobInstanceByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get final: %v", err)
	}
	if stored.Status != JobStatusSuccess {
		t.Fatalf("expected success after job-info succeeds, got %s", stored.Status)
	}
	if !hasFinalMetricsLog(stored.ResultPreview) {
		t.Fatalf("expected final metrics log: %s", metricsLogFromPreview(stored.ResultPreview))
	}
}

type flakyFinishedEngineClient struct {
	failInfoTimes int
	infoCalls     int
	info          *EngineJobInfo
}

func (f *flakyFinishedEngineClient) Submit(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error) {
	return nil, nil
}
func (f *flakyFinishedEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	f.infoCalls++
	if f.infoCalls <= f.failInfoTimes {
		// 用非 2xx 失败：超时会让集群进入退避，下一轮被跳过，那是退避测试的范围。
		// Fail with a non-2xx error: a timeout would put the cluster into backoff and skip the next pass, which the
		// backoff tests cover.
		return nil, errors.New("sync: get job info failed: status=500 body=busy")
	}
	return f.info, nil
}
func (f *flakyFinishedEngineClient) GetJobCheckpointOverview(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineCheckpointOverview, error) {
	return nil, nil
}
func (f *flakyFinishedEngineClient) GetJobCheckpointHistory(ctx context.Context, endpoint *EngineEndpoint, jobID string, pipelineID *int, limit int, status string) ([]*EngineCheckpointRecord, error) {
	return nil, nil
}
func (f *flakyFinishedEngineClient) StopJob(ctx context.Context, endpoint *EngineEndpoint, jobID string, stopWithSavepoint bool) error {
	return nil
}
func (f *flakyFinishedEngineClient) GetJobLogs(ctx context.Context, endpoint *EngineEndpoint, jobID string) (string, error) {
	return "", nil
}
func (f *flakyFinishedEngineClient) ListRunningJobs(ctx context.Context, endpoint *EngineEndpoint) ([]*EngineJobListItem, error) {
	return nil, nil
}
func (f *flakyFinishedEngineClient) ListFinishedJobs(ctx context.Context, endpoint *EngineEndpoint, state string) ([]*EngineJobListItem, error) {
	if f.info == nil {
		return nil, nil
	}
	return []*EngineJobListItem{{JobID: f.info.JobID, JobStatus: f.info.JobStatus}}, nil
}
