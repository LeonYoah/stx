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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	gosync "sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// reconcileActiveJobsAndWait 跑一轮对账并等后台的集群对账结束，供测试断言结果。
// reconcileActiveJobsAndWait runs one reconcile pass and waits for the background cluster reconciles so tests can
// assert on the outcome.
func (s *Service) reconcileActiveJobsAndWait(ctx context.Context) error {
	err := s.reconcileActiveJobs(ctx)
	s.reconcile.wait()
	return err
}

// engineTimeoutErr 模拟 http.Client 超时返回的错误。
// engineTimeoutErr mimics the error http.Client returns on a timeout.
func engineTimeoutErr(path string) error {
	return &url.Error{Op: "Get", URL: "http://engine" + path, Err: context.DeadlineExceeded}
}

// clusterEngineClient 按引擎地址返回不同结果，并记录每个地址收到的调用。
// clusterEngineClient answers per engine address and records the calls each address received.
type clusterEngineClient struct {
	stubEngineClient
	mu          gosync.Mutex
	calls       map[string][]string
	block       map[string]chan struct{}
	running     map[string][]*EngineJobListItem
	runningErr  map[string]error
	finishedErr map[string]error
	infoErr     map[string]error
}

func newClusterEngineClient() *clusterEngineClient {
	return &clusterEngineClient{
		calls:       map[string][]string{},
		block:       map[string]chan struct{}{},
		running:     map[string][]*EngineJobListItem{},
		runningErr:  map[string]error{},
		finishedErr: map[string]error{},
		infoErr:     map[string]error{},
	}
}

func (c *clusterEngineClient) record(endpoint *EngineEndpoint, call string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[endpoint.BaseURL] = append(c.calls[endpoint.BaseURL], call)
	return endpoint.BaseURL
}

func (c *clusterEngineClient) callsFor(baseURL, prefix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, call := range c.calls[baseURL] {
		if strings.HasPrefix(call, prefix) {
			count++
		}
	}
	return count
}

func (c *clusterEngineClient) ListRunningJobs(ctx context.Context, endpoint *EngineEndpoint) ([]*EngineJobListItem, error) {
	base := c.record(endpoint, "running")
	c.mu.Lock()
	block, items, err := c.block[base], c.running[base], c.runningErr[base]
	c.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return items, err
}

func (c *clusterEngineClient) ListFinishedJobs(ctx context.Context, endpoint *EngineEndpoint, state string) ([]*EngineJobListItem, error) {
	base := c.record(endpoint, "finished:"+state)
	c.mu.Lock()
	defer c.mu.Unlock()
	return nil, c.finishedErr[base]
}

func (c *clusterEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	base := c.record(endpoint, "info:"+jobID)
	c.mu.Lock()
	err := c.infoErr[base]
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &EngineJobInfo{JobID: jobID, JobStatus: "RUNNING", CreateTime: "2026-10-27 10:00:00"}, nil
}

// clusterResolver 把集群 ID 映射到固定的引擎地址；没有映射时返回错误。
// clusterResolver maps cluster IDs to fixed engine addresses and errors when a cluster is unknown.
type clusterResolver struct {
	byCluster map[uint]*EngineEndpoint
}

func (r *clusterResolver) ResolveEngineEndpoint(_ context.Context, clusterID uint, _ JSONMap) (*EngineEndpoint, error) {
	if endpoint, ok := r.byCluster[clusterID]; ok {
		return endpoint, nil
	}
	return nil, fmt.Errorf("no online node for cluster %d", clusterID)
}

func newSchedulerTestService(t *testing.T) (*Service, *clusterEngineClient) {
	t.Helper()
	service := newTestSyncService(t)
	engine := newClusterEngineClient()
	service.engineClient = engine
	service.runtimeResolver = &clusterResolver{byCluster: map[uint]*EngineEndpoint{
		1: {BaseURL: "http://cluster-a"},
		2: {BaseURL: "http://cluster-b"},
	}}
	return service, engine
}

func createClusterJob(t *testing.T, service *Service, clusterID uint, engineJobID string) *JobInstance {
	t.Helper()
	job := &JobInstance{
		TaskID:        clusterID,
		TaskVersion:   1,
		RunType:       RunTypeRun,
		Status:        JobStatusRunning,
		PlatformJobID: "platform-" + engineJobID,
		EngineJobID:   engineJobID,
		SubmitSpec:    JSONMap{"cluster_id": clusterID},
		CreatedBy:     1,
	}
	if err := service.repo.CreateJobInstance(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func loadJob(t *testing.T, service *Service, id uint) *JobInstance {
	t.Helper()
	job, err := service.repo.GetJobInstanceByID(context.Background(), id)
	if err != nil {
		t.Fatalf("load job %d: %v", id, err)
	}
	return job
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestReconcileSlowClusterDoesNotBlockOtherClusters(t *testing.T) {
	service, engine := newSchedulerTestService(t)
	ctx := context.Background()
	release := make(chan struct{})
	engine.block["http://cluster-a"] = release
	engine.running["http://cluster-a"] = []*EngineJobListItem{{JobID: "a1", JobStatus: "RUNNING"}}
	engine.running["http://cluster-b"] = []*EngineJobListItem{{JobID: "b1", JobStatus: "RUNNING"}}
	slow := createClusterJob(t, service, 1, "a1")
	fast := createClusterJob(t, service, 2, "b1")

	if err := service.reconcileActiveJobs(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	waitUntil(t, "cluster B to sync while A is stuck", func() bool {
		return loadJob(t, service, fast.ID).LastSyncedAt != nil
	})

	// 下一轮：A 上一轮还没结束，跳过；B 照常再对账一次。
	// Next pass: A is still running its previous pass and is skipped; B reconciles again as usual.
	if err := service.reconcileActiveJobs(ctx); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	waitUntil(t, "cluster B second pass", func() bool {
		return engine.callsFor("http://cluster-b", "running") == 2
	})
	if got := engine.callsFor("http://cluster-a", "running"); got != 1 {
		t.Fatalf("in-flight cluster A must be skipped, got %d running calls", got)
	}

	close(release)
	service.reconcile.wait()
	if loadJob(t, service, slow.ID).LastSyncedAt == nil {
		t.Fatalf("cluster A should sync once its engine answers")
	}
}

func TestReconcileClusterStopsAtFirstTransportError(t *testing.T) {
	t.Run("job-info timeout skips the remaining jobs", func(t *testing.T) {
		service, engine := newSchedulerTestService(t)
		engine.running["http://cluster-a"] = []*EngineJobListItem{
			{JobID: "a1", JobStatus: "RUNNING"}, {JobID: "a2", JobStatus: "RUNNING"}, {JobID: "a3", JobStatus: "RUNNING"},
		}
		engine.infoErr["http://cluster-a"] = engineTimeoutErr("/job-info/a1")
		jobs := []*JobInstance{
			createClusterJob(t, service, 1, "a1"), createClusterJob(t, service, 1, "a2"), createClusterJob(t, service, 1, "a3"),
		}

		err := service.reconcileClusterJobs(context.Background(), 1, jobs)
		if !isEngineTransportError(err) {
			t.Fatalf("expected the transport error back, got %v", err)
		}
		if got := engine.callsFor("http://cluster-a", "info:"); got != 1 {
			t.Fatalf("job-info must stop after the first timeout, got %d calls", got)
		}
		if first := loadJob(t, service, jobs[0].ID); first.SyncFailCount != 0 || first.LastSyncedAt == nil {
			t.Fatalf("the timed-out running job keeps the list status: fail=%d synced=%v", first.SyncFailCount, first.LastSyncedAt)
		}
		for _, job := range jobs[1:] {
			if got := loadJob(t, service, job.ID); got.SyncFailCount != 1 || got.SyncState != JobSyncStateDegraded {
				t.Fatalf("job %s not reached must record one sync failure: fail=%d state=%s", job.EngineJobID, got.SyncFailCount, got.SyncState)
			}
		}
	})

	t.Run("finished list timeout skips the remaining lists and job-info", func(t *testing.T) {
		service, engine := newSchedulerTestService(t)
		engine.finishedErr["http://cluster-a"] = engineTimeoutErr("/finished-jobs")
		job := createClusterJob(t, service, 1, "a1")

		err := service.reconcileClusterJobs(context.Background(), 1, []*JobInstance{job})
		if !isEngineTransportError(err) {
			t.Fatalf("expected the transport error back, got %v", err)
		}
		if got := engine.callsFor("http://cluster-a", "finished:"); got != 1 {
			t.Fatalf("finished lists must stop after the first timeout, got %d calls", got)
		}
		if got := engine.callsFor("http://cluster-a", "info:"); got != 0 {
			t.Fatalf("no job-info after a list timeout, got %d calls", got)
		}
		if got := loadJob(t, service, job.ID); got.SyncFailCount != 1 {
			t.Fatalf("job must record one sync failure, got %d", got.SyncFailCount)
		}
	})

	t.Run("non-2xx finished list keeps going", func(t *testing.T) {
		service, engine := newSchedulerTestService(t)
		engine.finishedErr["http://cluster-a"] = errors.New("sync: list engine jobs failed: status=404 body=")
		engine.running["http://cluster-a"] = []*EngineJobListItem{{JobID: "a1", JobStatus: "RUNNING"}}
		job := createClusterJob(t, service, 1, "a1")

		if err := service.reconcileClusterJobs(context.Background(), 1, []*JobInstance{job}); err != nil {
			t.Fatalf("non-2xx lists must not stop the pass: %v", err)
		}
		if got := engine.callsFor("http://cluster-a", "finished:"); got != 6 {
			t.Fatalf("all six finished lists should be tried, got %d", got)
		}
		if got := engine.callsFor("http://cluster-a", "info:"); got != 1 {
			t.Fatalf("job-info should still run, got %d", got)
		}
	})

	t.Run("no endpoint aborts", func(t *testing.T) {
		service, _ := newSchedulerTestService(t)
		job := createClusterJob(t, service, 9, "x1")
		if err := service.reconcileClusterJobs(context.Background(), 9, []*JobInstance{job}); !errors.Is(err, errReconcileNoEndpoint) {
			t.Fatalf("expected errReconcileNoEndpoint, got %v", err)
		}
	})
}

func TestReconcileBackoffDelaySequence(t *testing.T) {
	want := []time.Duration{10, 20, 40, 80, 120, 120}
	for i, seconds := range want {
		if got := reconcileBackoffDelay(i+1, 10*time.Second, 120*time.Second); got != seconds*time.Second {
			t.Fatalf("abort #%d: got %s want %ds", i+1, got, seconds)
		}
	}

	var scheduler clusterReconcileScheduler
	now := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	cause := engineTimeoutErr("/running-jobs")
	first := scheduler.finish(1, cause, 10*time.Second, 120*time.Second)
	if !first.Aborted || !first.EnteredBackoff || first.Delay != 10*time.Second || !first.FirstFailureAt.Equal(now) {
		t.Fatalf("first abort: %+v", first)
	}
	now = now.Add(15 * time.Second)
	second := scheduler.finish(1, cause, 10*time.Second, 120*time.Second)
	if second.EnteredBackoff || second.Delay != 20*time.Second || !second.FirstFailureAt.Equal(now.Add(-15*time.Second)) {
		t.Fatalf("second abort keeps the first failure time and doubles: %+v", second)
	}
	if !scheduler.inBackoff(1) {
		t.Fatalf("cluster should be backing off")
	}
	recovered := scheduler.finish(1, nil, 10*time.Second, 120*time.Second)
	if !recovered.Recovered || recovered.ConsecutiveAborts != 2 || scheduler.inBackoff(1) {
		t.Fatalf("success must reset backoff: %+v", recovered)
	}
	if again := scheduler.finish(1, cause, 10*time.Second, 120*time.Second); again.Delay != 10*time.Second || !again.EnteredBackoff {
		t.Fatalf("after recovery the sequence restarts at 10s: %+v", again)
	}

	// 直连键可能对应多个地址，不退避。
	// The direct-endpoint key may span several addresses and never backs off.
	scheduler.finish(directEndpointReconcileKey, cause, 10*time.Second, 120*time.Second)
	if scheduler.inBackoff(directEndpointReconcileKey) {
		t.Fatalf("direct-endpoint key must not back off")
	}
}

func TestReconcileSkipsClusterInBackoffAndRefreshUsesStoredRow(t *testing.T) {
	service, engine := newSchedulerTestService(t)
	ctx := context.Background()
	now := time.Now()
	service.reconcile.now = func() time.Time { return now }
	engine.runningErr["http://cluster-a"] = engineTimeoutErr("/running-jobs")
	engine.running["http://cluster-b"] = []*EngineJobListItem{{JobID: "b1", JobStatus: "RUNNING"}}
	slow := createClusterJob(t, service, 1, "a1")
	slow.SubmitSpec["engine_base_url"] = "http://cluster-a"
	if err := service.repo.UpdateJobSyncMetadata(ctx, slow.ID, map[string]any{"submit_spec": slow.SubmitSpec}); err != nil {
		t.Fatalf("seed submit_spec: %v", err)
	}
	createClusterJob(t, service, 2, "b1")

	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !service.reconcile.inBackoff(1) || service.reconcile.inBackoff(2) {
		t.Fatalf("only cluster A should back off")
	}

	// 退避期内：A 不再被请求，B 照常。
	// Within the backoff window A is not called again while B keeps going.
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile in backoff: %v", err)
	}
	if got := engine.callsFor("http://cluster-a", "running"); got != 1 {
		t.Fatalf("cluster A must be skipped during backoff, got %d running calls", got)
	}
	if got := engine.callsFor("http://cluster-b", "running"); got != 2 {
		t.Fatalf("cluster B keeps reconciling, got %d running calls", got)
	}

	// 刷新路径在退避期内不请求引擎，返回库里的数据。
	// The refresh path does not call the engine during backoff and returns the stored row.
	refreshed, err := service.RefreshJobByID(ctx, slow.ID)
	if err != nil || refreshed == nil || refreshed.ID != slow.ID {
		t.Fatalf("refresh during backoff: job=%v err=%v", refreshed, err)
	}
	if got := engine.callsFor("http://cluster-a", "info:"); got != 0 {
		t.Fatalf("refresh during backoff must not call the engine, got %d", got)
	}

	// 退避结束后重新请求；再次失败时推迟时间翻倍。
	// Once backoff ends the cluster is called again; another failure doubles the delay.
	now = now.Add(11 * time.Second)
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile after backoff: %v", err)
	}
	if got := engine.callsFor("http://cluster-a", "running"); got != 2 {
		t.Fatalf("cluster A should be retried after backoff, got %d running calls", got)
	}
	service.reconcile.mu.Lock()
	state := *service.reconcile.states[1]
	service.reconcile.mu.Unlock()
	if state.consecutiveAborts != 2 || state.nextAttemptAt.Sub(now) != 20*time.Second {
		t.Fatalf("second abort should back off 20s: %+v", state)
	}
}

func TestIsEngineTransportError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "client timeout", err: engineTimeoutErr("/running-jobs"), want: true},
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "refused", err: fmt.Errorf("dial: %w", syscall.ECONNREFUSED), want: true},
		{name: "reset", err: fmt.Errorf("read: %w", syscall.ECONNRESET), want: true},
		{name: "http status", err: errors.New("sync: get job info failed: status=500 body=x"), want: false},
		{name: "decode", err: errors.New("invalid character '<' looking for beginning of value"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEngineTransportError(tc.err); got != tc.want {
				t.Fatalf("isEngineTransportError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestEngineRequestTimeoutFromConfig(t *testing.T) {
	previous := config.Config.Sync.EngineRequestTimeoutSeconds
	t.Cleanup(func() { config.Config.Sync.EngineRequestTimeoutSeconds = previous })

	config.Config.Sync.EngineRequestTimeoutSeconds = 40
	if got := NewSeaTunnelEngineClient().httpClient.Timeout; got != 40*time.Second {
		t.Fatalf("configured timeout not applied: %s", got)
	}
	config.Config.Sync.EngineRequestTimeoutSeconds = 0
	if got := NewSeaTunnelEngineClient().httpClient.Timeout; got != 15*time.Second {
		t.Fatalf("unset timeout should default to 15s: %s", got)
	}
}

func TestListEngineJobsFallbackOnlyAfterDialFailure(t *testing.T) {
	var legacyHits atomic.Int32
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		legacyHits.Add(1)
		_, _ = w.Write([]byte("[]"))
	}))
	defer legacy.Close()

	t.Run("timeout does not retry the v1 path", func(t *testing.T) {
		legacyHits.Store(0)
		hanging := httptest.NewServer(http.HandlerFunc(hangingHandler))
		defer hanging.Close()
		_, err := newShortTimeoutEngineClient().ListRunningJobs(context.Background(), &EngineEndpoint{BaseURL: hanging.URL, LegacyURL: legacy.URL})
		if !isEngineTransportError(err) {
			t.Fatalf("expected a timeout, got %v", err)
		}
		if legacyHits.Load() != 0 {
			t.Fatalf("a timed-out v2 list must not fall back to v1")
		}
	})

	t.Run("refused connection still falls back", func(t *testing.T) {
		legacyHits.Store(0)
		items, err := newShortTimeoutEngineClient().ListRunningJobs(context.Background(), &EngineEndpoint{BaseURL: closedURL(t), LegacyURL: legacy.URL})
		if err != nil || items == nil {
			t.Fatalf("refused v2 should fall back to v1: items=%v err=%v", items, err)
		}
		if legacyHits.Load() != 1 {
			t.Fatalf("expected one v1 call, got %d", legacyHits.Load())
		}
	})
}
