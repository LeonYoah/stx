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

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
)

type discoveryEngineClient struct {
	*clusterEngineClient
	runningItems  []*EngineJobListItem
	finishedItems []*EngineJobListItem
}

func (c *discoveryEngineClient) ListRunningJobs(ctx context.Context, endpoint *EngineEndpoint) ([]*EngineJobListItem, error) {
	c.record(endpoint, "running")
	return c.runningItems, nil
}

func (c *discoveryEngineClient) ListFinishedJobs(ctx context.Context, endpoint *EngineEndpoint, state string) ([]*EngineJobListItem, error) {
	c.record(endpoint, "finished:"+state)
	return c.finishedItems, nil
}

type staticDiscoveryProvider []uint

func (p staticDiscoveryProvider) ListExternalDiscoveryClusterIDs(context.Context) ([]uint, error) {
	return p, nil
}

func newDiscoveryTestService(t *testing.T) (*Service, *discoveryEngineClient) {
	t.Helper()
	service, base := newSchedulerTestService(t)
	engine := &discoveryEngineClient{clusterEngineClient: base}
	service.engineClient = engine
	service.SetClusterDiscoveryProvider(staticDiscoveryProvider{1})
	return service, engine
}

func externalJobsOf(t *testing.T, service *Service) []*JobInstance {
	t.Helper()
	jobs, _, err := service.repo.ListJobInstances(context.Background(), &JobFilter{IncludeAll: true, Source: JobSourceExternal})
	if err != nil {
		t.Fatalf("list external jobs: %v", err)
	}
	return jobs
}

func TestExternalDiscoveryImportsAfterSecondSightingWithoutDetailCalls(t *testing.T) {
	service, engine := newDiscoveryTestService(t)
	ctx := context.Background()
	engine.runningItems = []*EngineJobListItem{{
		JobID: "9001", JobName: "cli-stream", JobStatus: "RUNNING", CreateTime: time.Now().Format(time.DateTime),
		JobDag:  map[string]interface{}{"envOptions": map[string]interface{}{"job.mode": "STREAMING"}},
		Metrics: map[string]interface{}{"SinkWriteCount": "10"},
	}}
	engine.finishedItems = []*EngineJobListItem{{
		JobID: "9002", JobName: "cli-batch", JobStatus: "FINISHED", CreateTime: time.Now().Format(time.DateTime),
		FinishedTime: time.Now().Format(time.DateTime), JobDag: map[string]interface{}{"envOptions": map[string]interface{}{}},
		Metrics: map[string]interface{}{"SinkWriteCount": "5"},
	}}

	if err := service.reconcileClusterPass(ctx, 1, nil, true); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if got := externalJobsOf(t, service); len(got) != 0 {
		t.Fatalf("first sighting must not import, got %d", len(got))
	}
	if err := service.reconcileClusterPass(ctx, 1, nil, true); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	jobs := externalJobsOf(t, service)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 external jobs, got %d", len(jobs))
	}
	byEngine := map[string]*JobInstance{}
	for _, job := range jobs {
		byEngine[job.EngineJobID] = job
		if job.TaskID != 0 || job.CreatedBy != 0 || job.RunType != RunTypeExternal || job.ClusterID != 1 {
			t.Fatalf("unexpected external job shape: %+v", job)
		}
	}
	if job := byEngine["9001"]; job == nil || job.Status != JobStatusRunning || job.JobMode != string(TaskModeStreaming) {
		t.Fatalf("running external job wrong: %+v", job)
	}
	if job := byEngine["9002"]; job == nil || job.Status != JobStatusSuccess || job.JobMode != string(TaskModeBatch) || job.FinishedAt == nil {
		t.Fatalf("finished external job wrong: %+v", job)
	}
	if n := engine.callsFor("http://cluster-a", "info:"); n != 0 {
		t.Fatalf("discovery must not call job-info, got %d", n)
	}
	if n := engine.callsFor("http://cluster-a", "finished:"); n != 2 {
		t.Fatalf("expected one finished list per pass, got %d", n)
	}

	if err := service.reconcileClusterPass(ctx, 1, nil, true); err != nil {
		t.Fatalf("third pass: %v", err)
	}
	if got := externalJobsOf(t, service); len(got) != 2 {
		t.Fatalf("re-import must be blocked, got %d", len(got))
	}
}

func TestExternalDiscoverySkipsKnownInflightAndRespectsCap(t *testing.T) {
	service, engine := newDiscoveryTestService(t)
	ctx := context.Background()
	createClusterJob(t, service, 1, "stx-1")
	release := service.trackInflightSubmit("inflight-1")
	defer release()
	old := time.Now().Add(-time.Hour).Format(time.DateTime)
	engine.runningItems = []*EngineJobListItem{
		{JobID: "stx-1", JobStatus: "RUNNING", CreateTime: old},
		{JobID: "inflight-1", JobStatus: "RUNNING", CreateTime: old},
	}
	for i := 0; i < 5; i++ {
		engine.finishedItems = append(engine.finishedItems, &EngineJobListItem{
			JobID: "old-" + string(rune('a'+i)), JobStatus: "FAILED", CreateTime: old, FinishedTime: old,
		})
	}
	service.external.mu.Lock()
	service.external.firstSeen = nil
	service.external.mu.Unlock()
	maxPerPass := 3
	prev := config.Config.Sync.ExternalJobDiscoveryMaxNewPerRound
	config.Config.Sync.ExternalJobDiscoveryMaxNewPerRound = maxPerPass
	defer func() { config.Config.Sync.ExternalJobDiscoveryMaxNewPerRound = prev }()

	if err := service.discoverExternalJobs(ctx, 1, &EngineEndpoint{BaseURL: "http://cluster-a"}, engine.runningItems, engine.finishedItems); err != nil {
		t.Fatalf("discover: %v", err)
	}
	jobs := externalJobsOf(t, service)
	if len(jobs) != maxPerPass {
		t.Fatalf("old jobs import immediately up to the cap, got %d", len(jobs))
	}
	for _, job := range jobs {
		if job.EngineJobID == "stx-1" || job.EngineJobID == "inflight-1" {
			t.Fatalf("known or in-flight job imported: %s", job.EngineJobID)
		}
	}
	if err := service.discoverExternalJobs(ctx, 1, &EngineEndpoint{BaseURL: "http://cluster-a"}, engine.runningItems, engine.finishedItems); err != nil {
		t.Fatalf("discover again: %v", err)
	}
	if got := externalJobsOf(t, service); len(got) != 5 {
		t.Fatalf("rest imported next pass, got %d", len(got))
	}
}

func TestExternalDiscoveryOffWhenGloballyDisabledOrNoProvider(t *testing.T) {
	service, _ := newDiscoveryTestService(t)
	service.SetClusterDiscoveryProvider(nil)
	if got := service.externalDiscoveryClusters(context.Background()); len(got) != 0 {
		t.Fatalf("no provider must disable discovery, got %v", got)
	}
}

func TestExternalJobModeDefaults(t *testing.T) {
	if got := externalJobMode(nil); got != "" {
		t.Fatalf("no dag: %q", got)
	}
	if got := externalJobMode(map[string]interface{}{"vertexInfoMap": []interface{}{}}); got != string(TaskModeBatch) {
		t.Fatalf("dag without job.mode: %q", got)
	}
	if got := externalJobMode(map[string]interface{}{"envOptions": map[string]interface{}{"job.mode": "STREAMING"}}); got != string(TaskModeStreaming) {
		t.Fatalf("streaming: %q", got)
	}
}

func TestEvaluateJobPermissionsMatrix(t *testing.T) {
	admin := executionapp.Actor{UserID: 1, IsAdmin: true}
	owner := executionapp.Actor{UserID: 2}
	viewer := executionapp.Actor{UserID: 3}
	stranger := executionapp.Actor{UserID: 4}
	collaborator := executionapp.Actor{UserID: 5}
	task := &Task{ID: 7, CreatedBy: 2, Definition: JSONMap{"is_public": false, "collaborators": []interface{}{float64(3), float64(5)}}}
	external := &JobInstance{Source: JobSourceExternal, RunType: RunTypeExternal}
	internal := &JobInstance{Source: JobSourceSTX, TaskID: 7, RunType: RunTypeRun, PlatformJobID: "p1", CreatedBy: 2}
	deleted := &JobInstance{Source: JobSourceSTX, TaskID: 7, RunType: RunTypeRun, PlatformJobID: "p1", CreatedBy: 2, TaskDeletedAt: &time.Time{}}

	cases := []struct {
		name  string
		job   *JobInstance
		actor executionapp.Actor
		want  JobPermissions
	}{
		{"external admin", external, admin, JobPermissions{CanView: true, CanStop: true}},
		{"external user", external, stranger, JobPermissions{CanView: true}},
		{"internal admin", internal, admin, JobPermissions{CanView: true, CanStop: true, CanRecover: true}},
		{"internal owner", internal, owner, JobPermissions{CanView: true, CanStop: true, CanRecover: true}},
		{"internal collaborator", internal, collaborator, JobPermissions{CanView: true, CanRecover: true}},
		{"internal stranger", internal, stranger, JobPermissions{}},
		{"deleted task owner", deleted, owner, JobPermissions{CanView: true, CanStop: true}},
		{"deleted task viewer", deleted, viewer, JobPermissions{}},
	}
	for _, tc := range cases {
		if got := EvaluateJobPermissions(tc.job, task, tc.actor); got != tc.want {
			t.Errorf("%s: got %+v want %+v", tc.name, got, tc.want)
		}
	}
}

func TestDeleteTaskKeepsJobRowsAndHidesThemFromTaskScope(t *testing.T) {
	service, _ := newDiscoveryTestService(t)
	ctx := context.Background()
	task := &Task{Name: "t1", NodeType: TaskNodeTypeFile, CreatedBy: 2}
	if err := service.repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	job := &JobInstance{TaskID: task.ID, TaskVersion: 1, RunType: RunTypeRun, Status: JobStatusSuccess, EngineJobID: "e1", ClusterID: 1, CreatedBy: 2}
	if err := service.repo.CreateJobInstance(ctx, job); err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := service.DeleteTaskForActor(ctx, executionapp.Actor{UserID: 2}, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	stored, err := service.repo.GetJobInstanceByID(ctx, job.ID)
	if err != nil || stored.TaskDeletedAt == nil {
		t.Fatalf("job row must be kept and stamped: %+v err=%v", stored, err)
	}
	if _, total, _ := service.repo.ListJobInstances(ctx, &JobFilter{IncludeAll: true, TaskID: task.ID}); total != 0 {
		t.Fatalf("task scope must skip rows of a deleted task, got %d", total)
	}
	existing, _ := service.repo.ExistingEngineJobIDs(ctx, 1, []string{"e1"})
	if _, ok := existing["e1"]; !ok {
		t.Fatalf("kept row must block re-import")
	}
}

func TestListJobsSourceFilterAndExternalVisibility(t *testing.T) {
	service, _ := newDiscoveryTestService(t)
	ctx := context.Background()
	if err := service.repo.CreateJobInstance(ctx, &JobInstance{Source: JobSourceExternal, RunType: RunTypeExternal, EngineJobID: "x1", ClusterID: 1, Status: JobStatusRunning}); err != nil {
		t.Fatalf("create external: %v", err)
	}
	createClusterJob(t, service, 1, "s1")
	stranger := executionapp.Actor{UserID: 99}
	data, err := service.ListJobsWithOptions(ctx, stranger, &JobFilter{}, JobListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if data.Total != 1 || data.Items[0].Source != JobSourceExternal || data.Items[0].CanStop || data.Items[0].CanRecover {
		t.Fatalf("stranger must see only the read-only external job: %+v", data.Items)
	}
	admin := executionapp.Actor{UserID: 1, IsAdmin: true}
	data, err = service.ListJobsWithOptions(ctx, admin, &JobFilter{Source: JobSourceSTX}, JobListOptions{})
	if err != nil || data.Total != 1 || data.Items[0].Source != JobSourceSTX {
		t.Fatalf("source filter: %+v err=%v", data, err)
	}
	if _, err := service.ListJobsWithOptions(ctx, admin, &JobFilter{Source: "bogus"}, JobListOptions{}); err == nil {
		t.Fatalf("unknown source must be rejected")
	}
	if _, err := service.CancelJobForActor(ctx, stranger, data.Items[0].ID, false); err == nil {
		t.Fatalf("stranger must not see internal job")
	}
}
