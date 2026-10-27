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
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// outcomeEngineClient 是可编排提交/停止结果与列表内容的假引擎。
// outcomeEngineClient is a fake engine whose submit/stop results and list contents are scripted per test.
type outcomeEngineClient struct {
	submitErr      error
	onSubmit       func()
	submitCtxErr   error
	submitCalls    int
	stopErr        error
	stopCalls      int
	running        []*EngineJobListItem
	finished       []*EngineJobListItem
	jobInfo        map[string]*EngineJobInfo
	jobInfoCalls   int
	lastSubmitBody []byte
}

func (f *outcomeEngineClient) Submit(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error) {
	f.submitCalls++
	if f.onSubmit != nil {
		f.onSubmit()
	}
	f.submitCtxErr = ctx.Err()
	f.lastSubmitBody = req.Body
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return &EngineSubmitResponse{JobID: req.JobID, APIMode: "v2", EndpointBaseURL: req.Endpoint.BaseURL}, nil
}
func (f *outcomeEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	f.jobInfoCalls++
	if info, ok := f.jobInfo[jobID]; ok {
		return info, nil
	}
	return &EngineJobInfo{JobID: jobID}, nil
}
func (f *outcomeEngineClient) GetJobCheckpointOverview(context.Context, *EngineEndpoint, string) (*EngineCheckpointOverview, error) {
	return nil, nil
}
func (f *outcomeEngineClient) GetJobCheckpointHistory(context.Context, *EngineEndpoint, string, *int, int, string) ([]*EngineCheckpointRecord, error) {
	return nil, nil
}
func (f *outcomeEngineClient) StopJob(context.Context, *EngineEndpoint, string, bool) error {
	f.stopCalls++
	return f.stopErr
}
func (f *outcomeEngineClient) GetJobLogs(context.Context, *EngineEndpoint, string) (string, error) {
	return "", nil
}
func (f *outcomeEngineClient) ListRunningJobs(context.Context, *EngineEndpoint) ([]*EngineJobListItem, error) {
	return f.running, nil
}
func (f *outcomeEngineClient) ListFinishedJobs(context.Context, *EngineEndpoint, string) ([]*EngineJobListItem, error) {
	return f.finished, nil
}

func unknownSubmitErr() error {
	return newEngineCallUnknownError(context.DeadlineExceeded, "v2", "http://engine:8080", "http://engine:8080/api/submit-job")
}

// newOutcomeTestService 构造带假引擎和固定端点的服务，并发布一个批任务。
// newOutcomeTestService builds a service with a fake engine and a fixed endpoint, and publishes one batch task.
func newOutcomeTestService(t *testing.T, definition JSONMap) (*Service, *outcomeEngineClient, *Task) {
	t.Helper()
	service := newTestSyncService(t)
	engine := &outcomeEngineClient{jobInfo: map[string]*EngineJobInfo{}}
	service.engineClient = engine
	service.runtimeResolver = staticRuntimeResolver{endpoint: &EngineEndpoint{BaseURL: "http://engine:8080", ContextPath: "/api"}}
	ctx := context.Background()
	folder, err := service.CreateTask(ctx, &CreateTaskRequest{
		NodeType: string(TaskNodeTypeFolder), Name: "workspace", ContentFormat: string(ContentFormatHOCON),
	}, 1)
	if err != nil {
		t.Fatalf("create folder: %v", err)
	}
	file, err := service.CreateTask(ctx, &CreateTaskRequest{
		ParentID: uintPtr(folder.ID), NodeType: string(TaskNodeTypeFile), Name: "demo",
		Mode: string(TaskModeBatch), ContentFormat: string(ContentFormatHOCON), Content: fakeBatchHOCON,
		Definition: definition,
	}, 1)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if _, _, err := service.PublishTask(ctx, file.ID, "v1", 1); err != nil {
		t.Fatalf("publish: %v", err)
	}
	return service, engine, file
}

func mustSubmitUnknown(t *testing.T, service *Service, engine *outcomeEngineClient, taskID uint, opts *SubmitOptions) *JobInstance {
	t.Helper()
	engine.submitErr = unknownSubmitErr()
	if opts == nil {
		opts = &SubmitOptions{}
	}
	job, err := service.SubmitTaskWithOptions(context.Background(), taskID, 1, opts)
	if err != nil {
		t.Fatalf("submit with unknown outcome must not fail: %v", err)
	}
	return job
}

func setSubmitDeadline(t *testing.T, service *Service, job *JobInstance, deadline time.Time) {
	t.Helper()
	job.SubmitSpec[submitDeadlineKey] = deadline.UTC().Format(time.RFC3339)
	if err := service.repo.UpdateJobSyncMetadata(context.Background(), job.ID, map[string]any{"submit_spec": job.SubmitSpec}); err != nil {
		t.Fatalf("update deadline: %v", err)
	}
}

func TestSubmitUnknownOutcomeRecordsPendingJob(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := mustSubmitUnknown(t, service, engine, task.ID, nil)

	stored, err := service.repo.GetJobInstanceByID(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if stored.Status != JobStatusPending {
		t.Fatalf("status = %s, want pending", stored.Status)
	}
	if stored.EngineJobID == "" || stored.EngineJobID != stored.PlatformJobID {
		t.Fatalf("engine_job_id %q must equal platform job id %q", stored.EngineJobID, stored.PlatformJobID)
	}
	spec := stored.SubmitSpec
	if stringValue(spec, submitOutcomeKey) != submitOutcomeUnknown {
		t.Fatalf("submit_outcome = %q", stringValue(spec, submitOutcomeKey))
	}
	if stringValue(spec, "engine_base_url") != "http://engine:8080" || stringValue(spec, "engine_context_path") != "/api" || stringValue(spec, "engine_api_mode") != "v2" {
		t.Fatalf("endpoint fields not recorded: %#v", spec)
	}
	since, _ := parseRFC3339(stringValue(spec, submitOutcomeSinceKey))
	deadline, _ := parseRFC3339(stringValue(spec, submitDeadlineKey))
	if got := deadline.Sub(since); got != defaultSubmitTimeout {
		t.Fatalf("deadline - since = %s, want %s", got, defaultSubmitTimeout)
	}
	if !strings.Contains(stringValue(spec, submitErrorKey), "outcome unknown") {
		t.Fatalf("submit_error = %q", stringValue(spec, submitErrorKey))
	}
	if endpointFromSubmitSpec(spec) == nil {
		t.Fatalf("reconcile must be able to rebuild the endpoint from submit_spec")
	}
}

func TestSubmitUnknownOutcomeUsesTaskTimeoutOverride(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, JSONMap{submitTimeoutSettingKey: float64(2)})
	job := mustSubmitUnknown(t, service, engine, task.ID, nil)
	since, _ := parseRFC3339(stringValue(job.SubmitSpec, submitOutcomeSinceKey))
	deadline, _ := parseRFC3339(stringValue(job.SubmitSpec, submitDeadlineKey))
	if got := deadline.Sub(since); got != 2*time.Minute {
		t.Fatalf("deadline - since = %s, want 2m", got)
	}
	if jsonMapInt(job.SubmitSpec, submitTimeoutSecondsKey) != 120 {
		t.Fatalf("submit_timeout_seconds = %d", jsonMapInt(job.SubmitSpec, submitTimeoutSecondsKey))
	}
}

func TestSubmitDefiniteFailureRecordsNothing(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	engine.submitErr = errors.New("sync: submit job failed: status=500 body=plugin not found")
	if _, err := service.SubmitTaskWithOptions(context.Background(), task.ID, 1, &SubmitOptions{}); err == nil {
		t.Fatal("expected the definite failure to be returned")
	}
	_, total, err := service.repo.ListJobInstances(context.Background(), &JobFilter{TaskID: task.ID, IncludeAll: true, Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if total != 0 {
		t.Fatalf("definite failures must not create a job row, got %d", total)
	}
}

func TestSubmitUnknownOutcomeSurvivesCallerCancel(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine.submitErr = unknownSubmitErr()
	engine.onSubmit = cancel // 调用方在引擎调用期间断开 / caller disconnects during the engine call

	job, err := service.SubmitTaskWithOptions(ctx, task.ID, 1, &SubmitOptions{})
	if err != nil {
		t.Fatalf("submit must still record the job: %v", err)
	}
	if engine.submitCtxErr != nil {
		t.Fatalf("engine call must run on a detached context, got %v", engine.submitCtxErr)
	}
	if _, err := service.repo.GetJobInstanceByID(context.Background(), job.ID); err != nil {
		t.Fatalf("job must be persisted after caller cancel: %v", err)
	}
}

func TestSubmitGuardBlocksWhileOutcomePending(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	pending := mustSubmitUnknown(t, service, engine, task.ID, nil)
	engine.submitErr = nil

	_, err := service.SubmitTaskWithOptions(context.Background(), task.ID, 1, &SubmitOptions{})
	if !errors.Is(err, ErrSubmitOutcomePending) {
		t.Fatalf("expected ErrSubmitOutcomePending, got %v", err)
	}
	if !strings.Contains(err.Error(), "job #") {
		t.Fatalf("error must name the pending job: %v", err)
	}
	if _, err := service.RecoverJob(context.Background(), pending.ID, 1, nil); !errors.Is(err, ErrSubmitOutcomePending) {
		t.Fatalf("recover must be blocked too, got %v", err)
	}
	if status := (&Handler{}).getStatusCodeForError(err); status != http.StatusConflict {
		t.Fatalf("status code = %d, want 409", status)
	}

	setSubmitDeadline(t, service, pending, time.Now().Add(-time.Second))
	if _, err := service.SubmitTaskWithOptions(context.Background(), task.ID, 1, &SubmitOptions{}); err != nil {
		t.Fatalf("submit must be allowed after the deadline, got %v", err)
	}
}

func TestSubmitSameExternalRefAttachesToPendingJob(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	first := mustSubmitUnknown(t, service, engine, task.ID, &SubmitOptions{ExternalRef: "ds:1:2"})
	engine.submitErr = nil

	again, err := service.SubmitTaskWithOptions(context.Background(), task.ID, 1, &SubmitOptions{ExternalRef: "ds:1:2"})
	if err != nil {
		t.Fatalf("same external_ref retry must attach, got %v", err)
	}
	if again.ID != first.ID || engine.submitCalls != 1 {
		t.Fatalf("expected attach to job %d without a new submit, got job %d and %d submits", first.ID, again.ID, engine.submitCalls)
	}
}

func TestSubmitUnknownOutcomeSkipsEventExpectation(t *testing.T) {
	service, _ := newEventTestService(t, "3.0.0")
	engine := &outcomeEngineClient{jobInfo: map[string]*EngineJobInfo{}}
	service.engineClient = engine
	service.runtimeResolver = staticRuntimeResolver{endpoint: &EngineEndpoint{BaseURL: "http://engine:8080"}}
	channel := healthyChannel()
	if err := service.repo.SaveEngineEventChannel(context.Background(), channel); err != nil {
		t.Fatalf("save channel: %v", err)
	}
	instance := &JobInstance{TaskID: 1, PlatformJobID: "77", SubmitSpec: JSONMap{}}
	if _, err := service.recordUnknownSubmitOutcome(context.Background(), &Task{ID: 1, ClusterID: channel.ClusterID}, instance, &EngineEndpoint{BaseURL: "http://engine:8080"}, newEngineCallUnknownError(context.DeadlineExceeded, "v2", "http://engine:8080", "")); err != nil {
		t.Fatalf("record unknown: %v", err)
	}
	if len(service.events.expectations) != 0 {
		t.Fatalf("unknown submits must not register an event expectation, got %d", len(service.events.expectations))
	}
}

// staleForReconcile 让对账把作业当作"列表里暂时看不到、需要拉详情"的旧记录。
// staleForReconcile makes reconcile treat the job as an older row that needs a detail fetch.
func staleForReconcile(job *JobInstance) {
	job.UpdatedAt = time.Now().Add(-time.Minute)
	job.CreatedAt = time.Now().Add(-time.Minute)
}

func reloadJob(t *testing.T, service *Service, id uint) *JobInstance {
	t.Helper()
	job, err := service.repo.GetJobInstanceByID(context.Background(), id)
	if err != nil {
		t.Fatalf("reload job: %v", err)
	}
	return job
}

func TestReconcileConfirmsUnknownSubmitWhenRunning(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := mustSubmitUnknown(t, service, engine, task.ID, nil)
	engine.running = []*EngineJobListItem{{JobID: job.EngineJobID, JobStatus: "RUNNING"}}
	engine.jobInfo[job.EngineJobID] = &EngineJobInfo{JobID: job.EngineJobID, JobStatus: "RUNNING"}

	service.reconcileClusterJobs(context.Background(), 0, []*JobInstance{job})

	stored := reloadJob(t, service, job.ID)
	if stored.Status != JobStatusRunning {
		t.Fatalf("status = %s, want running", stored.Status)
	}
	if stringValue(stored.SubmitSpec, submitOutcomeKey) != submitOutcomeConfirmed || stringValue(stored.SubmitSpec, submitOutcomeAtKey) == "" {
		t.Fatalf("submit outcome must be confirmed, got %#v", stored.SubmitSpec)
	}
}

func TestReconcileKeepsUnknownSubmitPendingBeforeDeadline(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := mustSubmitUnknown(t, service, engine, task.ID, nil)
	staleForReconcile(job)

	service.reconcileClusterJobs(context.Background(), 0, []*JobInstance{job})

	stored := reloadJob(t, service, job.ID)
	if stored.Status != JobStatusPending || !submitOutcomeIsUnknown(stored) {
		t.Fatalf("job must stay pending/unknown before the deadline, got %s %q", stored.Status, stringValue(stored.SubmitSpec, submitOutcomeKey))
	}
	if engine.jobInfoCalls == 0 {
		t.Fatalf("reconcile should still look the job up by id")
	}
}

func TestReconcileExpiresUnknownSubmitAfterDeadline(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := mustSubmitUnknown(t, service, engine, task.ID, nil)
	setSubmitDeadline(t, service, job, time.Now().Add(-time.Second))
	staleForReconcile(job)

	service.reconcileClusterJobs(context.Background(), 0, []*JobInstance{job})

	stored := reloadJob(t, service, job.ID)
	if stored.Status != JobStatusFailed {
		t.Fatalf("status = %s, want failed", stored.Status)
	}
	if stringValue(stored.SubmitSpec, submitOutcomeKey) != submitOutcomeExpired {
		t.Fatalf("submit_outcome = %q, want expired", stringValue(stored.SubmitSpec, submitOutcomeKey))
	}
	if !strings.Contains(stored.ErrorMessage, "submit timed out") || !strings.Contains(stored.ErrorMessage, "5m") {
		t.Fatalf("error_message = %q", stored.ErrorMessage)
	}
}

func TestReconcileIgnoresPreviousRunTerminalForUnknownRecover(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	// 先有一条已结束的旧运行作为恢复来源。
	// Start from a finished earlier run as the recovery source.
	engine.submitErr = nil
	source, err := service.SubmitTaskWithOptions(context.Background(), task.ID, 1, &SubmitOptions{})
	if err != nil {
		t.Fatalf("initial submit: %v", err)
	}
	if err := service.repo.UpdateJobSyncMetadata(context.Background(), source.ID, map[string]any{"status": JobStatusSavepointDone}); err != nil {
		t.Fatalf("finish source: %v", err)
	}
	engine.submitErr = unknownSubmitErr()
	recovered, err := service.RecoverJob(context.Background(), source.ID, 1, nil)
	if err != nil {
		t.Fatalf("recover with unknown outcome must not fail: %v", err)
	}
	if !submitOutcomeIsUnknown(recovered) || recovered.EngineJobID != source.PlatformJobID {
		t.Fatalf("recover must reuse the job id and stay unknown, got %q %q", recovered.EngineJobID, stringValue(recovered.SubmitSpec, submitOutcomeKey))
	}
	previous := &EngineJobInfo{JobID: recovered.EngineJobID, JobStatus: "SAVEPOINT_DONE"}
	engine.finished = []*EngineJobListItem{{JobID: recovered.EngineJobID, JobStatus: "SAVEPOINT_DONE"}}
	engine.jobInfo[recovered.EngineJobID] = previous

	service.reconcileClusterJobs(context.Background(), 0, []*JobInstance{recovered})
	if stored := reloadJob(t, service, recovered.ID); stored.Status != JobStatusPending || !submitOutcomeIsUnknown(stored) {
		t.Fatalf("previous run's terminal state must be ignored before the deadline, got %s", stored.Status)
	}

	stored := reloadJob(t, service, recovered.ID)
	setSubmitDeadline(t, service, stored, time.Now().Add(-time.Second))
	service.reconcileClusterJobs(context.Background(), 0, []*JobInstance{stored})
	if after := reloadJob(t, service, recovered.ID); after.Status != JobStatusSavepointDone {
		t.Fatalf("after the deadline the engine state is accepted, got %s", after.Status)
	}
}

func createRunningJob(t *testing.T, service *Service, taskID uint, submitSpec JSONMap) *JobInstance {
	t.Helper()
	spec := JSONMap{"engine_base_url": "http://engine:8080", "engine_api_mode": "v2"}
	for key, value := range submitSpec {
		spec[key] = value
	}
	job := &JobInstance{TaskID: taskID, PlatformJobID: "501", EngineJobID: "501", RunType: RunTypeRun, Status: JobStatusRunning, SubmitSpec: spec, CreatedBy: 1}
	if err := service.repo.CreateJobInstance(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return job
}

func TestCancelUnknownOutcomeStaysCancelling(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := createRunningJob(t, service, task.ID, JSONMap{submitStopTimeoutSecsKey: 180})
	engine.stopErr = newEngineCallUnknownError(context.DeadlineExceeded, "v2", "http://engine:8080", "")

	result, err := service.CancelJob(context.Background(), job.ID, false)
	if err != nil {
		t.Fatalf("unknown stop outcome must not fail the request: %v", err)
	}
	stored := reloadJob(t, service, result.ID)
	if stored.Status != JobStatusCancelling {
		t.Fatalf("status = %s, want cancelling", stored.Status)
	}
	if stringValue(stored.ResultPreview, cancelOutcomeKey) != cancelOutcomeUnknown {
		t.Fatalf("cancel_outcome = %q", stringValue(stored.ResultPreview, cancelOutcomeKey))
	}
	requested, _ := parseRFC3339(stringValue(stored.ResultPreview, cancelRequestedAtKey))
	if got := cancelDeadline(stored).Sub(requested); got != 3*time.Minute {
		t.Fatalf("cancel deadline uses the stop timeout pinned at submit, got %s", got)
	}

	// 引擎随后报告 CANCELED：停止结果确认。
	// The engine later reports CANCELED: the stop is confirmed.
	updated, err := service.applyEngineJobInfoUpdate(context.Background(), stored, &EngineJobInfo{JobID: "501", JobStatus: "CANCELED"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if updated.Status != JobStatusCanceled || stringValue(updated.ResultPreview, cancelOutcomeKey) != cancelOutcomeConfirmed {
		t.Fatalf("expected canceled + confirmed, got %s %q", updated.Status, stringValue(updated.ResultPreview, cancelOutcomeKey))
	}
}

func TestCancelDefiniteFailureRestoresStatus(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := createRunningJob(t, service, task.ID, nil)
	engine.stopErr = errors.New("sync: stop job failed: status=400 body=no such job")

	if _, err := service.CancelJob(context.Background(), job.ID, false); err == nil {
		t.Fatal("expected the definite stop failure to be returned")
	}
	stored := reloadJob(t, service, job.ID)
	if stored.Status != JobStatusRunning || stored.ErrorMessage != "cancel request failed" {
		t.Fatalf("definite stop failures restore the previous status, got %s %q", stored.Status, stored.ErrorMessage)
	}
}

func TestMarkCancelOverdueOnceAndExposeFlag(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	job := createRunningJob(t, service, task.ID, nil)
	if _, err := service.CancelJob(context.Background(), job.ID, false); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_ = engine
	stored := reloadJob(t, service, job.ID)
	if sanitizeJobForResponse(stored).CancelOverdue {
		t.Fatalf("a fresh cancel is not overdue yet")
	}
	stored.ResultPreview[cancelDeadlineKey] = time.Now().Add(-time.Second).UTC().Format(time.RFC3339)
	if err := service.repo.UpdateJobSyncMetadata(context.Background(), job.ID, map[string]any{"result_preview": stored.ResultPreview}); err != nil {
		t.Fatalf("move deadline: %v", err)
	}

	service.markCancelOverdue(context.Background(), stored, time.Now())
	first := reloadJob(t, service, job.ID)
	markedAt := stringValue(first.ResultPreview, cancelOverdueAtKey)
	if markedAt == "" || first.Status != JobStatusCancelling {
		t.Fatalf("overdue must be flagged without changing status, got %q %s", markedAt, first.Status)
	}
	service.markCancelOverdue(context.Background(), first, time.Now().Add(time.Minute))
	if again := reloadJob(t, service, job.ID); stringValue(again.ResultPreview, cancelOverdueAtKey) != markedAt {
		t.Fatalf("overdue must be flagged only once")
	}
	if !sanitizeJobForResponse(first).CancelOverdue {
		t.Fatalf("job responses must expose cancel_overdue")
	}
}

func TestTaskTimeoutParsingAndEffectiveValues(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		want    int
		ok      bool
		invalid bool
	}{
		{name: "unset", value: nil},
		{name: "empty string", value: ""},
		{name: "float minutes", value: float64(15), want: 15, ok: true},
		{name: "int minutes", value: 3, want: 3, ok: true},
		{name: "numeric string", value: "7", want: 7, ok: true},
		{name: "lower bound", value: float64(1), want: 1, ok: true},
		{name: "upper bound", value: float64(120), want: 120, ok: true},
		{name: "zero", value: float64(0), invalid: true},
		{name: "too large", value: float64(121), invalid: true},
		{name: "fraction", value: 1.5, invalid: true},
		{name: "garbage", value: "abc", invalid: true},
		{name: "wrong type", value: true, invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := JSONMap{submitTimeoutSettingKey: tc.value}
			got, ok, err := parseTaskTimeoutMinutes(definition, submitTimeoutSettingKey)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidTaskTimeout) || !errors.Is(validateTaskTimeouts(definition), ErrInvalidTaskTimeout) {
					t.Fatalf("expected ErrInvalidTaskTimeout, got %v", err)
				}
				if effectiveSubmitTimeout(&Task{Definition: definition}) != globalSubmitTimeout() {
					t.Fatalf("invalid overrides fall back to the global value")
				}
				return
			}
			if err != nil || ok != tc.ok || got != tc.want {
				t.Fatalf("parse = %d %v %v, want %d %v", got, ok, err, tc.want, tc.ok)
			}
		})
	}

	previousSubmit, previousStop := config.Config.Sync.SubmitTimeoutSeconds, config.Config.Sync.StopTimeoutSeconds
	t.Cleanup(func() {
		config.Config.Sync.SubmitTimeoutSeconds, config.Config.Sync.StopTimeoutSeconds = previousSubmit, previousStop
	})
	config.Config.Sync.SubmitTimeoutSeconds, config.Config.Sync.StopTimeoutSeconds = 0, 0
	if effectiveSubmitTimeout(nil) != defaultSubmitTimeout || effectiveStopTimeout(nil) != defaultStopTimeout {
		t.Fatalf("unset config falls back to 5 minutes")
	}
	config.Config.Sync.SubmitTimeoutSeconds, config.Config.Sync.StopTimeoutSeconds = 600, 90
	if effectiveSubmitTimeout(&Task{}) != 10*time.Minute || effectiveStopTimeout(&Task{}) != 90*time.Second {
		t.Fatalf("global config must apply when the task has no override")
	}
	override := &Task{Definition: JSONMap{submitTimeoutSettingKey: float64(2), stopTimeoutSettingKey: float64(4)}}
	if effectiveSubmitTimeout(override) != 2*time.Minute || effectiveStopTimeout(override) != 4*time.Minute {
		t.Fatalf("task overrides win over the global config")
	}
	if defaults := currentTaskTimeoutDefaults(); defaults.SubmitTimeoutSeconds != 600 || defaults.StopTimeoutSeconds != 90 {
		t.Fatalf("timeout defaults = %#v", defaults)
	}
	if status := (&Handler{}).getStatusCodeForError(validateTaskTimeouts(JSONMap{stopTimeoutSettingKey: float64(0)})); status != http.StatusBadRequest {
		t.Fatalf("invalid timeout status = %d, want 400", status)
	}
}
