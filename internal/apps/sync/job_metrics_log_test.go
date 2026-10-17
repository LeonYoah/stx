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
	if err := service.reconcileActiveJobs(ctx); err != nil {
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
	if err := service.reconcileActiveJobs(ctx); err != nil {
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
