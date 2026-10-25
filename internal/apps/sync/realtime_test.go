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
	gosync "sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRealtimeEngine 在 stubEngineClient 基础上实现 EngineRealtimeClient；masterURL 之外的节点一律返回非 master。
// fakeRealtimeEngine implements EngineRealtimeClient on top of stubEngineClient; nodes other than masterURL answer
// "not a master".
type fakeRealtimeEngine struct {
	stubEngineClient
	masterURL   string
	vertices    *EngineRealtimeSeries
	edges       *EngineRealtimeSeries
	probeCalls  atomic.Int32
	seriesCalls atomic.Int32
	seriesDelay time.Duration
}

func (f *fakeRealtimeEngine) GetRealtimeJobs(_ context.Context, endpoint *EngineEndpoint) (*EngineRealtimeJobs, error) {
	f.probeCalls.Add(1)
	if endpoint.BaseURL != f.masterURL {
		return nil, ErrEngineNotMaster
	}
	return &EngineRealtimeJobs{Jobs: []EngineRealtimeJobMeta{}}, nil
}

func (f *fakeRealtimeEngine) GetRealtimeVertices(_ context.Context, endpoint *EngineEndpoint, _ string, _ int64) (*EngineRealtimeSeries, error) {
	f.seriesCalls.Add(1)
	if f.seriesDelay > 0 {
		time.Sleep(f.seriesDelay)
	}
	if endpoint.BaseURL != f.masterURL {
		return nil, ErrEngineNotMaster
	}
	return f.vertices, nil
}

func (f *fakeRealtimeEngine) GetRealtimeEdges(_ context.Context, endpoint *EngineEndpoint, _ string, _ int64) (*EngineRealtimeSeries, error) {
	if endpoint.BaseURL != f.masterURL {
		return nil, ErrEngineNotMaster
	}
	return f.edges, nil
}

// fakeEndpointLister 返回固定的候选节点列表。
// fakeEndpointLister returns a fixed candidate list.
type fakeEndpointLister struct {
	endpoints []*EngineEndpoint
}

func (f *fakeEndpointLister) ResolveEngineEndpoint(context.Context, uint, JSONMap) (*EngineEndpoint, error) {
	return f.endpoints[0], nil
}

func (f *fakeEndpointLister) ListEngineEndpoints(context.Context, uint, JSONMap) ([]*EngineEndpoint, error) {
	return f.endpoints, nil
}

func realtimePoints(values ...map[string]any) []map[string]any {
	for i, value := range values {
		value["ts"] = float64(1000 * (i + 1))
	}
	return values
}

func repeatPoint(n int, point map[string]any) []map[string]any {
	points := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		clone := map[string]any{}
		for k, v := range point {
			clone[k] = v
		}
		points = append(points, clone)
	}
	return realtimePoints(points...)
}

func newRealtimeJob() *JobInstance {
	return &JobInstance{
		ID:          9,
		Status:      JobStatusRunning,
		EngineJobID: "1001",
		SubmitSpec:  JSONMap{"cluster_id": float64(3), "observability": map[string]any{"injected": true, "reason": "injected"}},
		ResultPreview: JSONMap{"job_dag": map[string]any{
			"vertexInfoMap": map[string]any{
				"1": map[string]any{"vertexId": float64(1), "type": "source", "connectorType": "Source[0]-FakeSource"},
				"2": map[string]any{"vertexId": float64(2), "type": "sink", "vertexName": "pipeline-1 [Sink[0]-Console]"},
			},
			"pipelineEdges": map[string]any{"1": []any{map[string]any{"inputVertexId": "1", "targetVertexId": "2"}}},
		}},
	}
}

func newRealtimeService(engine *fakeRealtimeEngine, version string) *Service {
	service := &Service{}
	service.engineClient = engine
	service.runtimeResolver = &fakeEndpointLister{endpoints: []*EngineEndpoint{{BaseURL: "http://worker"}, {BaseURL: "http://master"}}}
	service.SetClusterVersionProvider(fixedClusterVersionProvider{version: version})
	return service
}

func TestGetJobRealtime_findsMasterAndComputesHints(t *testing.T) {
	enabled := true
	engine := &fakeRealtimeEngine{
		masterURL: "http://master",
		vertices: &EngineRealtimeSeries{Known: true, BucketMs: 5000, FromMs: 1, ToMs: 2, Vertices: []EngineRealtimeVertex{
			{VertexID: 1, Points: repeatPoint(3, map[string]any{"sourceReadRatio": 0.3})},
			{VertexID: 2, Points: repeatPoint(3, map[string]any{"sinkBusyRatio": 0.95})},
		}},
		edges: &EngineRealtimeSeries{Known: true, Enabled: &enabled, BucketMs: 5000, Edges: []EngineRealtimeEdge{
			{QueueID: -5, TargetVertexID: 2, Points: repeatPoint(3, map[string]any{"bpRatio": 0.7, "queueFillRatio": 0.95})},
		}},
	}
	service := newRealtimeService(engine, "3.0.0")
	result, err := service.getJobRealtime(context.Background(), newRealtimeJob(), realtimeDefaultWindowMs)
	if err != nil {
		t.Fatalf("getJobRealtime: %v", err)
	}
	if !result.Available || result.Reason != "" {
		t.Fatalf("expected available result, got %+v", result)
	}
	if result.ClusterID != 3 || result.WebUIPath != "/api/v1/clusters/3/webui/#/jobs/1001" {
		t.Fatalf("unexpected cluster/webui path: %d %q", result.ClusterID, result.WebUIPath)
	}
	if result.Vertices[1].Type != "sink" || result.Vertices[1].Name != "pipeline-1 [Sink[0]-Console]" {
		t.Fatalf("vertex meta not enriched: %+v", result.Vertices[1])
	}
	if len(result.Hints) != 1 || result.Hints[0].Code != RealtimeHintSinkSlow || result.Hints[0].Severity != "critical" {
		t.Fatalf("expected one critical sink_slow hint, got %+v", result.Hints)
	}

	// 第二次请求在 2 秒内命中结果缓存，不再请求引擎。
	// A second request within 2s hits the result cache and does not call the engine.
	probes := engine.probeCalls.Load()
	if _, err := service.getJobRealtime(context.Background(), newRealtimeJob(), realtimeDefaultWindowMs); err != nil {
		t.Fatalf("second getJobRealtime: %v", err)
	}
	if engine.probeCalls.Load() != probes || engine.seriesCalls.Load() != 1 {
		t.Fatalf("expected cached result, probes %d→%d series %d", probes, engine.probeCalls.Load(), engine.seriesCalls.Load())
	}
}

func TestGetJobRealtime_reasons(t *testing.T) {
	disabled := false
	cases := []struct {
		name    string
		version string
		mutate  func(job *JobInstance, engine *fakeRealtimeEngine)
		want    string
	}{
		{name: "old version", version: "2.3.12", want: RealtimeReasonVersionUnsupported},
		{name: "local mode", version: "3.0.0", mutate: func(job *JobInstance, _ *fakeRealtimeEngine) { job.SubmitSpec["execution_mode"] = "local" }, want: RealtimeReasonNotClusterMode},
		{name: "no engine job", version: "3.0.0", mutate: func(job *JobInstance, _ *fakeRealtimeEngine) { job.EngineJobID = "" }, want: RealtimeReasonNoEngineJob},
		{name: "finished", version: "3.0.0", mutate: func(job *JobInstance, _ *fakeRealtimeEngine) { job.Status = JobStatusSuccess }, want: RealtimeReasonNotRunning},
		{name: "master unreachable", version: "3.0.0", mutate: func(_ *JobInstance, engine *fakeRealtimeEngine) { engine.masterURL = "http://nowhere" }, want: RealtimeReasonMasterUnreachable},
		{name: "unknown job is collecting", version: "3.0.0", mutate: func(_ *JobInstance, engine *fakeRealtimeEngine) {
			engine.vertices = &EngineRealtimeSeries{}
			engine.edges = &EngineRealtimeSeries{}
		}, want: RealtimeReasonCollecting},
		{name: "not enabled", version: "3.0.0", mutate: func(_ *JobInstance, engine *fakeRealtimeEngine) {
			engine.vertices = &EngineRealtimeSeries{Known: true, Enabled: &disabled}
			engine.edges = &EngineRealtimeSeries{Known: true, Enabled: &disabled}
		}, want: RealtimeReasonNotEnabled},
		{name: "enabled without points is collecting", version: "3.0.0", mutate: func(_ *JobInstance, engine *fakeRealtimeEngine) {
			engine.vertices = &EngineRealtimeSeries{Known: true, Vertices: []EngineRealtimeVertex{{VertexID: 1}}}
			engine.edges = &EngineRealtimeSeries{Known: true}
		}, want: RealtimeReasonCollecting},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := &fakeRealtimeEngine{
				masterURL: "http://master",
				vertices:  &EngineRealtimeSeries{Known: true, Vertices: []EngineRealtimeVertex{{VertexID: 1, Points: repeatPoint(1, map[string]any{})}}},
				edges:     &EngineRealtimeSeries{Known: true},
			}
			job := newRealtimeJob()
			if tc.mutate != nil {
				tc.mutate(job, engine)
			}
			result, err := newRealtimeService(engine, tc.version).getJobRealtime(context.Background(), job, realtimeDefaultWindowMs)
			if err != nil {
				t.Fatalf("getJobRealtime: %v", err)
			}
			if result.Available || result.Reason != tc.want {
				t.Fatalf("got available=%v reason=%q, want %q", result.Available, result.Reason, tc.want)
			}
			if result.Vertices == nil || result.Edges == nil || result.Hints == nil {
				t.Fatalf("array fields must never be nil: %+v", result)
			}
		})
	}
}

func TestGetJobRealtime_coalescesConcurrentRequests(t *testing.T) {
	engine := &fakeRealtimeEngine{
		masterURL:   "http://master",
		seriesDelay: 50 * time.Millisecond,
		vertices:    &EngineRealtimeSeries{Known: true, Vertices: []EngineRealtimeVertex{{VertexID: 1, Points: repeatPoint(1, map[string]any{})}}},
		edges:       &EngineRealtimeSeries{Known: true},
	}
	service := newRealtimeService(engine, "3.0.0")
	var wg gosync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := service.getJobRealtime(context.Background(), newRealtimeJob(), realtimeDefaultWindowMs); err != nil {
				t.Errorf("getJobRealtime: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := engine.seriesCalls.Load(); got != 1 {
		t.Fatalf("expected 1 engine series call for 8 concurrent viewers, got %d", got)
	}
}

func TestGetJobRealtime_staleMasterCacheIsRetried(t *testing.T) {
	engine := &fakeRealtimeEngine{
		masterURL: "http://master",
		vertices:  &EngineRealtimeSeries{Known: true, Vertices: []EngineRealtimeVertex{{VertexID: 1, Points: repeatPoint(1, map[string]any{})}}},
		edges:     &EngineRealtimeSeries{Known: true},
	}
	service := newRealtimeService(engine, "3.0.0")
	// 预置一个过期的 master（实际已切主到 worker 之外的节点）。
	// Seed a stale master that is no longer the active master.
	service.realtime.masters = map[string]realtimeMasterEntry{"cluster:3": {endpoint: &EngineEndpoint{BaseURL: "http://worker"}, expiresAt: time.Now().Add(time.Minute)}}
	result, err := service.getJobRealtime(context.Background(), newRealtimeJob(), realtimeDefaultWindowMs)
	if err != nil {
		t.Fatalf("getJobRealtime: %v", err)
	}
	if !result.Available {
		t.Fatalf("expected retry against the real master to succeed, got %+v", result)
	}
	if service.realtime.masters["cluster:3"].endpoint.BaseURL != "http://master" {
		t.Fatalf("master cache was not refreshed: %+v", service.realtime.masters)
	}
}

func TestComputeRealtimeHints(t *testing.T) {
	downstream := map[int64][]int64{1: {2}, 2: {3}}
	cases := []struct {
		name     string
		vertices []RealtimeVertex
		edges    []RealtimeEdge
		want     []string
	}{
		{
			name: "transform busy with backpressured input",
			vertices: []RealtimeVertex{
				{VertexID: 2, Type: "transform", Points: repeatPoint(3, map[string]any{"transformBusyRatio": 0.9})},
			},
			edges: []RealtimeEdge{{QueueID: -4, TargetVertexID: 2, Points: repeatPoint(3, map[string]any{"bpRatio": 0.3, "queueFillRatio": 0.9})}},
			want:  []string{RealtimeHintTransformBusy},
		},
		{
			name: "only two buckets do not trigger",
			vertices: []RealtimeVertex{
				{VertexID: 3, Type: "sink", Points: repeatPoint(2, map[string]any{"sinkBusyRatio": 0.99})},
			},
			edges: []RealtimeEdge{{QueueID: -7, TargetVertexID: 3, Points: repeatPoint(2, map[string]any{"bpRatio": 0.9, "queueFillRatio": 0.99})}},
			want:  []string{},
		},
		{
			name: "backpressure with idle target points elsewhere",
			vertices: []RealtimeVertex{
				{VertexID: 3, Type: "sink", Points: repeatPoint(3, map[string]any{"sinkBusyRatio": 0.2})},
			},
			edges: []RealtimeEdge{{QueueID: -7, TargetVertexID: 3, Points: repeatPoint(3, map[string]any{"bpRatio": 0.6, "queueFillRatio": 0.9})}},
			want:  []string{RealtimeHintDownstreamBackpressure},
		},
		{
			name: "source idle with empty queues",
			vertices: []RealtimeVertex{
				{VertexID: 1, Type: "source", Points: repeatPoint(3, map[string]any{"sourceIdleRatio": 0.95})},
			},
			edges: []RealtimeEdge{{QueueID: -7, TargetVertexID: 3, Points: repeatPoint(3, map[string]any{"bpRatio": 0.0, "queueFillRatio": 0.0})}},
			want:  []string{RealtimeHintSourceIdle},
		},
		{
			name: "source idle but queue still full is not idle",
			vertices: []RealtimeVertex{
				{VertexID: 1, Type: "source", Points: repeatPoint(3, map[string]any{"sourceIdleRatio": 0.95})},
			},
			edges: []RealtimeEdge{{QueueID: -7, TargetVertexID: 3, Points: repeatPoint(3, map[string]any{"bpRatio": 0.01, "queueFillRatio": 0.5})}},
			want:  []string{},
		},
		{
			name: "source busy without downstream backpressure",
			vertices: []RealtimeVertex{
				{VertexID: 1, Type: "source", Points: repeatPoint(3, map[string]any{"sourceReadRatio": 0.9})},
			},
			edges: []RealtimeEdge{{QueueID: -7, TargetVertexID: 3, Points: repeatPoint(3, map[string]any{"bpRatio": 0.01, "queueFillRatio": 0.3})}},
			want:  []string{RealtimeHintSourceBusy},
		},
		{
			name: "source busy but downstream is backpressured",
			vertices: []RealtimeVertex{
				{VertexID: 1, Type: "source", Points: repeatPoint(3, map[string]any{"sourceReadRatio": 0.9})},
			},
			edges: []RealtimeEdge{{QueueID: -7, TargetVertexID: 3, Points: repeatPoint(3, map[string]any{"bpRatio": 0.3, "queueFillRatio": 0.5})}},
			want:  []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hints := computeRealtimeHints(tc.vertices, tc.edges, downstream)
			got := make([]string, 0, len(hints))
			for _, hint := range hints {
				got = append(got, hint.Code)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestJobWatchWindow(t *testing.T) {
	service := &Service{}
	now := time.Unix(1000, 0)
	service.realtime.now = func() time.Time { return now }
	service.markJobWatched(5)
	if !service.jobWatchedRecently(5) {
		t.Fatalf("expected job to be watched")
	}
	now = now.Add(jobWatchWindow + time.Second)
	if service.jobWatchedRecently(5) {
		t.Fatalf("expected watch mark to expire")
	}
	if service.jobWatchedRecently(6) {
		t.Fatalf("unwatched job reported as watched")
	}
}

func TestNormalizeRealtimeWindow(t *testing.T) {
	if normalizeRealtimeWindow(0) != realtimeDefaultWindowMs || normalizeRealtimeWindow(-5) != realtimeDefaultWindowMs {
		t.Fatalf("non-positive window must use default")
	}
	if normalizeRealtimeWindow(realtimeMaxWindowMs+1) != realtimeMaxWindowMs {
		t.Fatalf("window must be clamped to max")
	}
	if normalizeRealtimeWindow(60000) != 60000 {
		t.Fatalf("valid window must be kept")
	}
}
