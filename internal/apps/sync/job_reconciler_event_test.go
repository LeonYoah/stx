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

// countingEngineClient 统计列表与 job-info 调用次数。
// countingEngineClient counts list and job-info calls.
type countingEngineClient struct {
	stubEngineClient
	listCalls atomic.Int32
	infoCalls atomic.Int32
}

func (c *countingEngineClient) ListRunningJobs(ctx context.Context, endpoint *EngineEndpoint) ([]*EngineJobListItem, error) {
	c.listCalls.Add(1)
	return c.stubEngineClient.ListRunningJobs(ctx, endpoint)
}

func (c *countingEngineClient) ListFinishedJobs(ctx context.Context, endpoint *EngineEndpoint, state string) ([]*EngineJobListItem, error) {
	c.listCalls.Add(1)
	return c.stubEngineClient.ListFinishedJobs(ctx, endpoint, state)
}

func (c *countingEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	c.infoCalls.Add(1)
	return &EngineJobInfo{JobID: jobID, JobStatus: "RUNNING"}, nil
}

func withEventModeInterval(t *testing.T, seconds int) {
	t.Helper()
	previous := config.Config.Sync.JobReconcilerEventModeIntervalSeconds
	config.Config.Sync.JobReconcilerEventModeIntervalSeconds = seconds
	t.Cleanup(func() { config.Config.Sync.JobReconcilerEventModeIntervalSeconds = previous })
}

func setupEventModeReconcile(t *testing.T, version string, channel *EngineEventChannel) (*Service, *countingEngineClient, []*JobInstance) {
	t.Helper()
	service, _ := newEventTestService(t, version)
	engine := &countingEngineClient{}
	service.engineClient = engine
	service.runtimeResolver = &fakeEndpointLister{endpoints: []*EngineEndpoint{{BaseURL: "http://engine"}}}
	if channel != nil {
		if err := service.repo.SaveEngineEventChannel(context.Background(), channel); err != nil {
			t.Fatalf("save channel: %v", err)
		}
	}
	now := time.Now()
	var jobs []*JobInstance
	for _, engineJobID := range []string{"501", "502"} {
		job := createEventJob(t, service, engineJobID, 5)
		if err := service.repo.UpdateJobSyncMetadata(context.Background(), job.ID, map[string]any{"last_synced_at": now}); err != nil {
			t.Fatalf("seed last_synced_at: %v", err)
		}
		jobs = append(jobs, job)
	}
	return service, engine, jobs
}

func healthyChannel() *EngineEventChannel {
	now := time.Now()
	return &EngineEventChannel{ClusterID: 5, Enabled: true, ReportNonTerminal: true, TokenNonce: "n", LastEventAt: &now}
}

func TestReconcileEventMode_skipsListsAndPullsOnlyWatchedJobs(t *testing.T) {
	withEventModeInterval(t, 0)
	service, engine, jobs := setupEventModeReconcile(t, "3.0.0", healthyChannel())
	ctx := context.Background()

	if err := service.reconcileActiveJobs(ctx); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if engine.listCalls.Load() == 0 {
		t.Fatalf("first tick must be a full reconcile")
	}
	lists, infos := engine.listCalls.Load(), engine.infoCalls.Load()

	service.markJobWatched(jobs[0].ID)
	if err := service.reconcileActiveJobs(ctx); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if engine.listCalls.Load() != lists {
		t.Fatalf("event mode must not call list endpoints within the interval")
	}
	if got := engine.infoCalls.Load() - infos; got != 1 {
		t.Fatalf("only the watched job should pull job-info, got %d calls", got)
	}

	// 超过完整对账间隔后恢复一次完整对账。
	// Once the full-reconcile interval passes, a full reconcile runs again.
	service.events.now = func() time.Time { return time.Now().Add(2 * defaultEventModeFullReconcileInterval) }
	if err := service.reconcileActiveJobs(ctx); err != nil {
		t.Fatalf("third reconcile: %v", err)
	}
	if engine.listCalls.Load() == lists {
		t.Fatalf("full reconcile should run after the interval")
	}
}

func TestReconcileEventMode_fallsBackToFullReconcile(t *testing.T) {
	missed := time.Now()
	degraded := healthyChannel()
	degraded.ExpectMissedAt = &missed
	cases := []struct {
		name     string
		version  string
		channel  *EngineEventChannel
		interval int
	}{
		{name: "no channel", version: "3.0.0", channel: nil},
		{name: "degraded channel", version: "3.0.0", channel: degraded},
		{name: "terminal-only cluster", version: "2.3.12", channel: healthyChannel()},
		{name: "adaptive disabled", version: "3.0.0", channel: healthyChannel(), interval: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withEventModeInterval(t, tc.interval)
			service, engine, _ := setupEventModeReconcile(t, tc.version, tc.channel)
			ctx := context.Background()
			if err := service.reconcileActiveJobs(ctx); err != nil {
				t.Fatalf("first reconcile: %v", err)
			}
			lists := engine.listCalls.Load()
			if err := service.reconcileActiveJobs(ctx); err != nil {
				t.Fatalf("second reconcile: %v", err)
			}
			if engine.listCalls.Load() <= lists {
				t.Fatalf("every tick must stay a full reconcile")
			}
		})
	}
}

func TestReconcileEventMode_staleUnwatchedJobIsRefreshed(t *testing.T) {
	withEventModeInterval(t, 0)
	service, engine, jobs := setupEventModeReconcile(t, "3.0.0", healthyChannel())
	ctx := context.Background()
	service.markFullReconcile(5)
	old := time.Now().Add(-eventModeUnwatchedRefreshInterval - time.Second)
	if err := service.repo.UpdateJobSyncMetadata(ctx, jobs[1].ID, map[string]any{"last_synced_at": old}); err != nil {
		t.Fatalf("age job: %v", err)
	}
	if err := service.reconcileActiveJobs(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if engine.listCalls.Load() != 0 || engine.infoCalls.Load() != 1 {
		t.Fatalf("expected only the stale job to be refreshed, lists=%d infos=%d", engine.listCalls.Load(), engine.infoCalls.Load())
	}
}
