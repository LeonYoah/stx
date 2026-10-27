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
	"net/http"
	"strings"
	gosync "sync"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// recordingAlertReporter 记录收到的告警信号。
// recordingAlertReporter records the alert signals it receives.
type recordingAlertReporter struct {
	mu      gosync.Mutex
	signals []syncalert.Signal
}

func (r *recordingAlertReporter) ReportSyncAlert(signal syncalert.Signal) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, signal)
}

func (r *recordingAlertReporter) find(kind syncalert.SignalKind, rule string, scope syncalert.ResolveScope) []syncalert.Signal {
	r.mu.Lock()
	defer r.mu.Unlock()
	var matched []syncalert.Signal
	for _, signal := range r.signals {
		if signal.Kind == kind && signal.RuleKey == rule && signal.ResolveScope == scope {
			matched = append(matched, signal)
		}
	}
	return matched
}

func (r *recordingAlertReporter) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.signals)
}

func attachReporter(service *Service) *recordingAlertReporter {
	reporter := &recordingAlertReporter{}
	service.SetAlertReporter(reporter)
	return reporter
}

type failingRuntimeResolver struct{}

func (failingRuntimeResolver) ResolveEngineEndpoint(context.Context, uint, JSONMap) (*EngineEndpoint, error) {
	return nil, errors.New("no online node for cluster")
}

func TestAlertSignalSubmitFailedOnlyForEngineFailures(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	reporter := attachReporter(service)
	ctx := context.Background()

	engine.submitErr = errors.New("sync: submit job failed: status=500 body=plugin not found password=s3cret")
	if _, err := service.SubmitTaskWithOptions(ctx, task.ID, 1, &SubmitOptions{}); err == nil {
		t.Fatal("expected the definite failure")
	}
	fired := reporter.find(syncalert.KindFire, syncalert.RuleSubmitFailed, "")
	if len(fired) != 1 {
		t.Fatalf("expected one A1 signal, got %d", len(fired))
	}
	signal := fired[0]
	if signal.TaskID != task.ID || signal.TaskName != "demo" || signal.TaskCreatorID != 1 || signal.TriggerSource != "manual" {
		t.Fatalf("A1 signal lacks task snapshot: %+v", signal)
	}
	if !strings.Contains(signal.Message, "plugin not found") || strings.Contains(signal.Message, "s3cret") {
		t.Fatalf("A1 message must keep the cause and redact secrets: %q", signal.Message)
	}

	// 找不到端点同样是 A1。
	// A missing endpoint is A1 as well.
	service.runtimeResolver = failingRuntimeResolver{}
	if _, err := service.SubmitTaskWithOptions(ctx, task.ID, 1, &SubmitOptions{}); err == nil {
		t.Fatal("expected the endpoint failure")
	}
	if got := len(reporter.find(syncalert.KindFire, syncalert.RuleSubmitFailed, "")); got != 2 {
		t.Fatalf("endpoint failure should report A1, got %d", got)
	}

	// 结果未知不是 A1；之后的 409 拦截也不是。
	// An unknown outcome is not A1, and neither is the later 409 guard.
	service.runtimeResolver = staticRuntimeResolver{endpoint: &EngineEndpoint{BaseURL: "http://engine:8080"}}
	mustSubmitUnknown(t, service, engine, task.ID, nil)
	if _, err := service.SubmitTaskWithOptions(ctx, task.ID, 1, &SubmitOptions{}); !errors.Is(err, ErrSubmitOutcomePending) {
		t.Fatalf("expected the 409 guard, got %v", err)
	}
	if got := len(reporter.find(syncalert.KindFire, syncalert.RuleSubmitFailed, "")); got != 2 {
		t.Fatalf("unknown outcomes and 409 must not report A1, got %d", got)
	}
}

func TestAlertSignalJobFailedAndBatchRecovery(t *testing.T) {
	service, _, task := newOutcomeTestService(t, nil)
	reporter := attachReporter(service)
	ctx := context.Background()

	failing := createRunningJob(t, service, task.ID, JSONMap{"mode": "batch", "cluster_id": 3, "trigger_source": "schedule"})
	if _, err := service.applyEngineJobInfoUpdate(ctx, failing, &EngineJobInfo{JobID: "501", JobStatus: "FAILED", ErrorMsg: "sink write failed"}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	fired := reporter.find(syncalert.KindFire, syncalert.RuleJobFailed, "")
	if len(fired) != 1 || fired[0].JobID != failing.ID || fired[0].ClusterID != 3 || fired[0].TriggerSource != "schedule" || fired[0].Message != "sink write failed" {
		t.Fatalf("expected one B1 signal for the job, got %+v", fired)
	}

	// 已经是失败的作业再同步一次，不重复上报。
	// Syncing an already failed job again does not report twice.
	stored := reloadJob(t, service, failing.ID)
	if _, err := service.applyEngineJobInfoUpdate(ctx, stored, &EngineJobInfo{JobID: "501", JobStatus: "FAILED"}); err != nil {
		t.Fatalf("apply again: %v", err)
	}
	if got := len(reporter.find(syncalert.KindFire, syncalert.RuleJobFailed, "")); got != 1 {
		t.Fatalf("B1 must only fire on the transition, got %d", got)
	}

	// 批作业运行成功才算恢复。
	// A batch job recovers only on success.
	succeeding := createRunningJob(t, service, task.ID, JSONMap{"mode": "batch"})
	if _, err := service.applyEngineJobInfoUpdate(ctx, succeeding, &EngineJobInfo{JobID: "501", JobStatus: "FINISHED"}); err != nil {
		t.Fatalf("apply success: %v", err)
	}
	if got := reporter.find(syncalert.KindResolve, "", syncalert.ResolveTaskRunHealthy); len(got) != 1 || got[0].TaskID != task.ID {
		t.Fatalf("batch success should resolve the task, got %+v", got)
	}

	// 数据预览不产生信号。
	// Data previews never produce signals.
	before := reporter.count()
	preview := &JobInstance{TaskID: task.ID, PlatformJobID: "p1", EngineJobID: "p1", RunType: RunTypePreview, Status: JobStatusRunning, SubmitSpec: JSONMap{"engine_base_url": "http://engine:8080"}}
	if err := service.repo.CreateJobInstance(ctx, preview); err != nil {
		t.Fatalf("create preview: %v", err)
	}
	if _, err := service.applyEngineJobInfoUpdate(ctx, preview, &EngineJobInfo{JobID: "p1", JobStatus: "FAILED"}); err != nil {
		t.Fatalf("apply preview: %v", err)
	}
	if reporter.count() != before {
		t.Fatalf("preview runs must not report alerts")
	}
}

func TestAlertSignalStreamingRecoversOnRunning(t *testing.T) {
	service, _, task := newOutcomeTestService(t, nil)
	reporter := attachReporter(service)
	ctx := context.Background()
	job := &JobInstance{TaskID: task.ID, PlatformJobID: "s1", EngineJobID: "s1", RunType: RunTypeRun, Status: JobStatusPending, SubmitSpec: JSONMap{"mode": "streaming", "engine_base_url": "http://engine:8080"}}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if _, err := service.applyEngineJobInfoUpdate(ctx, job, &EngineJobInfo{JobID: "s1", JobStatus: "RUNNING"}); err != nil {
		t.Fatalf("apply running: %v", err)
	}
	if got := reporter.find(syncalert.KindResolve, "", syncalert.ResolveTaskRunHealthy); len(got) != 1 {
		t.Fatalf("a streaming job turning running resolves the task, got %d", len(got))
	}

	// 实时作业提交成功（直接记为运行中）同样解除；批作业提交成功不解除。
	// A successful streaming submit (recorded as running) also resolves; a batch submit does not.
	streamingTask := *task
	streamingTask.Mode = TaskModeStreaming
	service.reportSubmitSucceeded(ctx, &streamingTask, &JobInstance{ID: 9, TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusRunning, SubmitSpec: JSONMap{"mode": "streaming"}})
	service.reportSubmitSucceeded(ctx, task, &JobInstance{ID: 10, TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusRunning, SubmitSpec: JSONMap{"mode": "batch"}})
	if got := reporter.find(syncalert.KindResolve, "", syncalert.ResolveTaskRunHealthy); len(got) != 2 || got[1].JobID != 9 {
		t.Fatalf("only the streaming submit resolves, got %+v", got)
	}
}

func TestAlertSignalExpiredSubmitReportsTimeoutOnly(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	reporter := attachReporter(service)
	job := mustSubmitUnknown(t, service, engine, task.ID, nil)
	setSubmitDeadline(t, service, job, time.Now().Add(-time.Second))
	staleForReconcile(job)

	_ = service.reconcileClusterJobs(context.Background(), 0, []*JobInstance{job})

	if got := reporter.find(syncalert.KindFire, syncalert.RuleSubmitTimeout, ""); len(got) != 1 || got[0].JobID != job.ID || !strings.Contains(got[0].Message, "submit timed out") {
		t.Fatalf("expected one A2 signal, got %+v", got)
	}
	if got := reporter.find(syncalert.KindFire, syncalert.RuleJobFailed, ""); len(got) != 0 {
		t.Fatalf("an expired submit must not also report B1, got %d", len(got))
	}
}

func TestAlertSignalStopOverdueAndResolve(t *testing.T) {
	service, _, task := newOutcomeTestService(t, nil)
	reporter := attachReporter(service)
	ctx := context.Background()
	job := createRunningJob(t, service, task.ID, nil)
	if _, err := service.CancelJob(ctx, job.ID, false); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	stored := reloadJob(t, service, job.ID)
	stored.ResultPreview[cancelDeadlineKey] = time.Now().Add(-time.Second).UTC().Format(time.RFC3339)
	if err := service.repo.UpdateJobSyncMetadata(ctx, job.ID, map[string]any{"result_preview": stored.ResultPreview}); err != nil {
		t.Fatalf("move deadline: %v", err)
	}
	service.markCancelOverdue(ctx, stored, time.Now())
	service.markCancelOverdue(ctx, reloadJob(t, service, job.ID), time.Now())
	if got := reporter.find(syncalert.KindFire, syncalert.RuleStopTimeout, ""); len(got) != 1 || got[0].JobID != job.ID {
		t.Fatalf("expected exactly one C1 signal, got %+v", got)
	}

	if _, err := service.applyEngineJobInfoUpdate(ctx, reloadJob(t, service, job.ID), &EngineJobInfo{JobID: "501", JobStatus: "CANCELED"}); err != nil {
		t.Fatalf("apply canceled: %v", err)
	}
	if got := reporter.find(syncalert.KindResolve, "", syncalert.ResolveJobTerminal); len(got) != 1 || got[0].JobID != job.ID {
		t.Fatalf("stopping the overdue job resolves its C1, got %+v", got)
	}
}

func TestAlertSignalClusterStatusFromReconcile(t *testing.T) {
	service, engine := newSchedulerTestService(t)
	reporter := attachReporter(service)
	ctx := context.Background()
	engine.runningErr["http://cluster-a"] = engineTimeoutErr("/running-jobs")
	createClusterJob(t, service, 1, "a1")
	createClusterJob(t, service, 2, "b1")

	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	observed := reporter.find(syncalert.KindObserve, syncalert.RuleStatusUnavailable, "")
	if len(observed) != 1 || observed[0].ClusterID != 1 || observed[0].Since.IsZero() || !strings.Contains(observed[0].Message, "deadline") {
		t.Fatalf("cluster A should report a D1 observation, got %+v", observed)
	}
	// B 启动后第一次成功也发 Resolve，用来结束重启前遗留的阶段；之后的正常轮次不再发。
	// B's first success after start also resolves leftovers from before a restart; later normal passes do not.
	resolved := reporter.find(syncalert.KindResolve, syncalert.RuleStatusUnavailable, syncalert.ResolveClusterRule)
	if len(resolved) != 1 || resolved[0].ClusterID != 2 {
		t.Fatalf("cluster B first success should resolve, got %+v", resolved)
	}
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if got := reporter.find(syncalert.KindResolve, syncalert.RuleStatusUnavailable, syncalert.ResolveClusterRule); len(got) != 1 {
		t.Fatalf("normal passes must not resolve again, got %d", len(got))
	}

	// A 恢复后发 Resolve。
	// A resolves once it recovers.
	delete(engine.runningErr, "http://cluster-a")
	service.reconcile.now = func() time.Time { return time.Now().Add(time.Minute) }
	if err := service.reconcileActiveJobsAndWait(ctx); err != nil {
		t.Fatalf("recovery reconcile: %v", err)
	}
	resolved = reporter.find(syncalert.KindResolve, syncalert.RuleStatusUnavailable, syncalert.ResolveClusterRule)
	if len(resolved) != 2 || resolved[1].ClusterID != 1 {
		t.Fatalf("cluster A recovery should resolve, got %+v", resolved)
	}
}

func TestAlertSignalEventChannelDegraded(t *testing.T) {
	service, _ := newEventTestService(t, "3.0.0")
	reporter := attachReporter(service)
	ctx := context.Background()
	if err := service.repo.SaveEngineEventChannel(ctx, healthyChannel()); err != nil {
		t.Fatalf("save channel: %v", err)
	}
	missedAt := time.Now()
	if err := service.updateEventChannelStatsWithAlert(ctx, 5, nil, map[string]any{"expect_missed_at": missedAt}); err != nil {
		t.Fatalf("mark missed: %v", err)
	}
	if got := reporter.find(syncalert.KindFire, syncalert.RuleEventChannelDegraded, ""); len(got) != 1 || got[0].ClusterID != 5 {
		t.Fatalf("healthy → degraded should fire D2, got %+v", got)
	}
	// 再错过一次仍是降级，不重复上报。
	// Missing again stays degraded and does not report twice.
	if err := service.updateEventChannelStatsWithAlert(ctx, 5, nil, map[string]any{"expect_missed_at": missedAt.Add(time.Second)}); err != nil {
		t.Fatalf("mark missed again: %v", err)
	}
	if got := reporter.find(syncalert.KindFire, syncalert.RuleEventChannelDegraded, ""); len(got) != 1 {
		t.Fatalf("staying degraded must not fire again, got %d", len(got))
	}
	if err := service.updateEventChannelStatsWithAlert(ctx, 5, nil, map[string]any{"expect_satisfied_at": missedAt.Add(2 * time.Second)}); err != nil {
		t.Fatalf("mark satisfied: %v", err)
	}
	if got := reporter.find(syncalert.KindResolve, syncalert.RuleEventChannelDegraded, syncalert.ResolveClusterRule); len(got) != 1 {
		t.Fatalf("degraded → healthy should resolve D2, got %d", len(got))
	}
}

// memoryTaskAlertStore 是内存版任务告警存取，用来测试 sync 侧的权限和默认值。
// memoryTaskAlertStore is an in-memory task alert store for testing sync-side permissions and defaults.
type memoryTaskAlertStore struct {
	configs map[uint]*syncalert.TaskConfig
	refs    map[uint]syncalert.TaskRef
}

func (m *memoryTaskAlertStore) GetTaskAlertConfig(_ context.Context, taskID uint) (*syncalert.TaskConfig, error) {
	if cfg, ok := m.configs[taskID]; ok {
		return cfg, nil
	}
	return &syncalert.TaskConfig{Configured: false}, nil
}

func (m *memoryTaskAlertStore) SaveTaskAlertConfig(_ context.Context, task syncalert.TaskRef, cfg *syncalert.TaskConfig) (*syncalert.TaskConfig, error) {
	normalized := cfg.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	normalized.Configured = true
	m.configs[task.ID] = normalized
	m.refs[task.ID] = task
	return normalized, nil
}

func (m *memoryTaskAlertStore) DeleteTaskAlertConfig(_ context.Context, taskID uint) error {
	delete(m.configs, taskID)
	return nil
}

func TestTaskAlertServicePermissionsAndDefaults(t *testing.T) {
	service, _, task := newOutcomeTestService(t, nil)
	ctx := context.Background()
	owner := executionapp.Actor{UserID: 1}
	stranger := executionapp.Actor{UserID: 99}

	if _, err := service.GetTaskAlertForActor(ctx, owner, task.ID); !errors.Is(err, ErrTaskAlertUnavailable) {
		t.Fatalf("without a store the API is unavailable, got %v", err)
	}
	store := &memoryTaskAlertStore{configs: map[uint]*syncalert.TaskConfig{}, refs: map[uint]syncalert.TaskRef{}}
	service.SetTaskAlertPolicyStore(store)

	if _, err := service.UpdateTaskAlertForActor(ctx, stranger, task.ID, &TaskAlertRequest{Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{1}}); !errors.Is(err, ErrTaskPermissionDenied) {
		t.Fatalf("non-editors cannot configure alerts, got %v", err)
	}
	cfg, err := service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{1}})
	if err != nil {
		t.Fatalf("owner update: %v", err)
	}
	if !cfg.Enabled || !cfg.SendRecovery || cfg.CooldownMinutes != syncalert.DefaultCooldownMinutes {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if ref := store.refs[task.ID]; ref.Name != "demo" || ref.CreatorID != 1 {
		t.Fatalf("task snapshot not passed to the store: %+v", ref)
	}
	disabled := false
	zero := 0
	cfg, err = service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Enabled: &disabled, CooldownMinutes: &zero, Rules: []string{syncalert.RuleJobFailed}, ChannelIDs: []uint{1}})
	if err != nil || cfg.Enabled || cfg.CooldownMinutes != 0 {
		t.Fatalf("explicit false / 0 must be kept: %+v err=%v", cfg, err)
	}
	if _, err := service.UpdateTaskAlertForActor(ctx, owner, task.ID, &TaskAlertRequest{Rules: []string{"nope"}, ChannelIDs: []uint{1}}); !errors.Is(err, syncalert.ErrInvalidConfig) {
		t.Fatalf("unknown rule should be invalid, got %v", err)
	}
	if err := service.DeleteTaskAlertForActor(ctx, stranger, task.ID); !errors.Is(err, ErrTaskPermissionDenied) {
		t.Fatalf("non-editors cannot delete, got %v", err)
	}
	if err := service.DeleteTaskAlertForActor(ctx, owner, task.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	handler := NewHandler(service)
	cases := map[error]int{
		syncalert.InvalidConfig("x"): http.StatusBadRequest,
		syncalert.ErrConflict:        http.StatusConflict,
		ErrTaskAlertUnavailable:      http.StatusServiceUnavailable,
		ErrTaskPermissionDenied:      http.StatusForbidden,
		ErrTaskNotFound:              http.StatusNotFound,
	}
	for err, want := range cases {
		if got := handler.getStatusCodeForError(err); got != want {
			t.Fatalf("status for %v = %d, want %d", err, got, want)
		}
	}
}
