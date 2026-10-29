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
	"sync/atomic"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// countingObsEngine counts Checkpoint vs realtime engine calls for collector gate tests.
// countingObsEngine 统计 Checkpoint 与 realtime 引擎调用次数，用于采集门控测试。
type countingObsEngine struct {
	stubEngineClient
	cpCalls      atomic.Int32
	realtimeJobs atomic.Int32
	overview     *EngineCheckpointOverview
}

func (c *countingObsEngine) GetJobCheckpointOverview(context.Context, *EngineEndpoint, string) (*EngineCheckpointOverview, error) {
	c.cpCalls.Add(1)
	if c.overview != nil {
		return c.overview, nil
	}
	return &EngineCheckpointOverview{
		JobID: "e1",
		Pipelines: []*EngineCheckpointPipeline{{
			PipelineID: 1,
			Counts:     map[string]int64{"FAILED": 0},
			LatestCompleted: &EngineCheckpoint{
				CheckpointID:       1,
				CompletedTimestamp: time.Now().UnixMilli(),
				DurationMillis:     1200,
			},
		}},
	}, nil
}

func (c *countingObsEngine) GetRealtimeJobs(context.Context, *EngineEndpoint) (*EngineRealtimeJobs, error) {
	c.realtimeJobs.Add(1)
	return &EngineRealtimeJobs{Jobs: []EngineRealtimeJobMeta{}}, nil
}

func (c *countingObsEngine) GetRealtimeVertices(context.Context, *EngineEndpoint, string, int64) (*EngineRealtimeSeries, error) {
	c.realtimeJobs.Add(1)
	enabled := true
	return &EngineRealtimeSeries{Known: true, Enabled: &enabled, Vertices: []EngineRealtimeVertex{}}, nil
}

func (c *countingObsEngine) GetRealtimeEdges(context.Context, *EngineEndpoint, string, int64) (*EngineRealtimeSeries, error) {
	c.realtimeJobs.Add(1)
	enabled := true
	return &EngineRealtimeSeries{Known: true, Enabled: &enabled, Edges: []EngineRealtimeEdge{}}, nil
}

func TestCollectJobObservabilityMetrics_splitsCheckpointAndRealtimeGates(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true

	service := newTestSyncService(t)
	engine := &countingObsEngine{}
	service.engineClient = engine
	service.runtimeResolver = staticRuntimeResolver{endpoint: &EngineEndpoint{BaseURL: "http://engine.test:8080"}}

	ctx := context.Background()
	now := time.Now()

	// Job without engine observability: Checkpoint should still be collected; realtime must not.
	// 未开引擎可观测性：仍应采 Checkpoint，不得采 realtime。
	jobNoEngineObs := &JobInstance{
		TaskID:      1,
		TaskVersion: 1,
		RunType:     RunTypeRun,
		Status:      JobStatusRunning,
		EngineJobID: "engine-no-obs",
		SubmitSpec: JSONMap{
			"cluster_id":     float64(3),
			"engine_version": "3.0.0",
			"observability": map[string]any{
				"injected": false,
				"reason":   "disabled_by_task",
			},
		},
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
		CreatedBy: 1,
	}
	if err := service.repo.CreateJobInstance(ctx, jobNoEngineObs); err != nil {
		t.Fatalf("create job no-obs: %v", err)
	}

	service.collectJobObservabilityMetrics(ctx)
	if got := engine.cpCalls.Load(); got != 1 {
		t.Fatalf("checkpoint calls=%d want 1 (STX obs on, engine obs off)", got)
	}
	if got := engine.realtimeJobs.Load(); got != 0 {
		t.Fatalf("realtime calls=%d want 0 when engine obs off", got)
	}

	// Job with engine observability: both Checkpoint and realtime should run.
	// 开启引擎可观测性：Checkpoint 与 realtime 都应采集。
	jobWithEngineObs := &JobInstance{
		TaskID:      1,
		TaskVersion: 1,
		RunType:     RunTypeRun,
		Status:      JobStatusRunning,
		EngineJobID: "engine-with-obs",
		SubmitSpec: JSONMap{
			"cluster_id":     float64(3),
			"engine_version": "3.0.0",
			"observability": map[string]any{
				"injected": true,
				"reason":   "injected",
			},
		},
		CreatedAt: now.Add(-time.Minute),
		UpdatedAt: now.Add(-time.Minute),
		CreatedBy: 1,
	}
	if err := service.repo.CreateJobInstance(ctx, jobWithEngineObs); err != nil {
		t.Fatalf("create job with-obs: %v", err)
	}

	engine.cpCalls.Store(0)
	engine.realtimeJobs.Store(0)
	service.collectJobObservabilityMetrics(ctx)
	if got := engine.cpCalls.Load(); got != 2 {
		t.Fatalf("checkpoint calls=%d want 2 (both running jobs)", got)
	}
	if got := engine.realtimeJobs.Load(); got < 1 {
		t.Fatalf("realtime calls=%d want >=1 when engine obs on", got)
	}
}

func TestCollectJobObservabilityMetrics_skipsWhenSTXObservabilityDisabled(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = false

	service := newTestSyncService(t)
	engine := &countingObsEngine{}
	service.engineClient = engine
	service.runtimeResolver = staticRuntimeResolver{endpoint: &EngineEndpoint{BaseURL: "http://engine.test:8080"}}

	ctx := context.Background()
	job := &JobInstance{
		TaskID:      1,
		TaskVersion: 1,
		RunType:     RunTypeRun,
		Status:      JobStatusRunning,
		EngineJobID: "engine-stx-off",
		SubmitSpec: JSONMap{
			"cluster_id":    float64(3),
			"observability": map[string]any{"injected": true, "reason": "injected"},
		},
		CreatedAt: time.Now().Add(-time.Minute),
		UpdatedAt: time.Now().Add(-time.Minute),
		CreatedBy: 1,
	}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	service.collectJobObservabilityMetrics(ctx)
	if got := engine.cpCalls.Load(); got != 0 {
		t.Fatalf("checkpoint calls=%d want 0 when STX obs off", got)
	}
	if got := engine.realtimeJobs.Load(); got != 0 {
		t.Fatalf("realtime calls=%d want 0 when STX obs off", got)
	}
}
