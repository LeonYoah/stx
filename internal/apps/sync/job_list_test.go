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
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// seedJobListFixture 建两个任务（一个私有）和一组不同状态 / 模式 / 集群的作业。
// seedJobListFixture creates two tasks (one private) and jobs across statuses, modes and clusters.
func seedJobListFixture(t *testing.T, service *Service) (publicTask, privateTask *Task) {
	t.Helper()
	ctx := context.Background()
	db := service.repo.db
	publicTask = &Task{NodeType: TaskNodeTypeFile, Name: "orders-cdc", ClusterID: 1, Mode: TaskModeStreaming, CreatedBy: 1}
	privateTask = &Task{NodeType: TaskNodeTypeFile, Name: "secret-batch", ClusterID: 2, Mode: TaskModeBatch, CreatedBy: 1,
		Definition: JSONMap{"is_public": false}}
	for _, task := range []*Task{publicTask, privateTask} {
		if err := db.WithContext(ctx).Create(task).Error; err != nil {
			t.Fatalf("create task: %v", err)
		}
	}
	base := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	at := func(minutes int) *time.Time { v := base.Add(time.Duration(minutes) * time.Minute); return &v }
	jobs := []*JobInstance{
		{TaskID: publicTask.ID, RunType: RunTypeRun, Status: JobStatusSuccess, CreatedBy: 1, StartedAt: at(1),
			SubmitSpec: JSONMap{"cluster_id": 1, "mode": "streaming"}},
		{TaskID: publicTask.ID, RunType: RunTypeRun, Status: JobStatusRunning, CreatedBy: 1, StartedAt: at(2),
			SubmitSpec:    JSONMap{"cluster_id": 1, "mode": "streaming"},
			ResultPreview: JSONMap{"metrics": map[string]interface{}{"SinkWriteQPS": "120", "SourceReceivedQPS": "130", "SinkWriteCount": "900"}}},
		{TaskID: publicTask.ID, RunType: RunTypeRun, Status: JobStatusRunning, CreatedBy: 2, StartedAt: at(3), EngineJobID: "888",
			SubmitSpec:    JSONMap{"cluster_id": 1, "mode": "streaming"},
			ResultPreview: JSONMap{"metrics": map[string]interface{}{"SinkWriteQPS": "500"}}},
		{TaskID: publicTask.ID, RunType: RunTypeRun, Status: JobStatusFailed, CreatedBy: 1, StartedAt: at(0),
			SubmitSpec: JSONMap{"cluster_id": 1, "mode": "streaming"}},
		{TaskID: privateTask.ID, RunType: RunTypeRun, Status: JobStatusFailing, CreatedBy: 1, StartedAt: at(4),
			SubmitSpec: JSONMap{"cluster_id": 2, "mode": "batch"}},
		{TaskID: publicTask.ID, RunType: RunTypePreview, Status: JobStatusSuccess, CreatedBy: 1, StartedAt: at(5),
			SubmitSpec: JSONMap{"mode": "streaming", "preview": true}},
	}
	for _, job := range jobs {
		if err := db.WithContext(ctx).Create(job).Error; err != nil {
			t.Fatalf("create job: %v", err)
		}
	}
	return publicTask, privateTask
}

func TestJobInstanceBeforeCreateFillsDimensions(t *testing.T) {
	service := newTestSyncService(t)
	// cluster_id 用 uint：提交路径直接放的是 task.ClusterID（uint），不是从库里读出的 float64。
	// cluster_id is a uint: the submit path stores task.ClusterID (uint), not a float64 read back from the database.
	job := &JobInstance{TaskID: 1, RunType: RunTypeRun, SubmitSpec: JSONMap{
		"cluster_id": uint(3), "mode": TaskModeStreaming, "submitted_content": "env {\n  job.mode = \"BATCH\"\n}"}}
	if err := service.repo.db.Create(job).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	if job.ClusterID != 3 || job.JobMode != "batch" {
		t.Fatalf("dimensions = (%d, %q), want (3, batch): submitted content wins over metadata", job.ClusterID, job.JobMode)
	}
}

func TestBackfillJobDimensionsUsesSnapshotThenTask(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	db := service.repo.db
	task := &Task{NodeType: TaskNodeTypeFile, Name: "t", ClusterID: 7, Mode: TaskModeBatch}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := &JobInstance{TaskID: task.ID, RunType: RunTypeRun, SubmitSpec: JSONMap{"cluster_id": 5, "mode": "streaming"}}
	bare := &JobInstance{TaskID: task.ID, RunType: RunTypeRun, SubmitSpec: JSONMap{}}
	orphan := &JobInstance{TaskID: 9999, RunType: RunTypeRun, SubmitSpec: JSONMap{}}
	for _, job := range []*JobInstance{snapshot, bare, orphan} {
		if err := db.Create(job).Error; err != nil {
			t.Fatal(err)
		}
	}
	// 模拟升级前的历史行：清空维度列。 / Simulate pre-upgrade rows: clear the dimension columns.
	if err := db.Model(&JobInstance{}).Where("1 = 1").UpdateColumns(map[string]interface{}{"cluster_id": 0, "job_mode": ""}).Error; err != nil {
		t.Fatal(err)
	}
	updated, err := service.repo.BackfillJobDimensions(ctx, 2)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated = %d, want 2 (orphan stays unresolved)", updated)
	}
	check := func(id uint, cluster uint, mode string) {
		t.Helper()
		var got JobInstance
		if err := db.First(&got, id).Error; err != nil {
			t.Fatal(err)
		}
		if got.ClusterID != cluster || got.JobMode != mode {
			t.Fatalf("job %d = (%d, %q), want (%d, %q)", id, got.ClusterID, got.JobMode, cluster, mode)
		}
	}
	check(snapshot.ID, 5, "streaming")
	check(bare.ID, 7, "batch")
	check(orphan.ID, 0, "")
	again, err := service.repo.BackfillJobDimensions(ctx, 2)
	if err != nil || again != 0 {
		t.Fatalf("second backfill = (%d, %v), want idempotent (0, nil)", again, err)
	}
}

func TestListJobsWithOptionsFiltersAndAttentionOrder(t *testing.T) {
	service := newTestSyncService(t)
	seedJobListFixture(t, service)
	ctx := context.Background()
	admin := executionapp.Actor{UserID: 1, IsAdmin: true}

	data, err := service.ListJobsWithOptions(ctx, admin, &JobFilter{Sort: jobListSortAttention,
		ExcludeRunTypes: []RunType{RunTypePreview}}, JobListOptions{WithCounts: true, WithLive: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var order []JobStatus
	for _, job := range data.Items {
		order = append(order, job.Status)
	}
	want := []JobStatus{JobStatusFailing, JobStatusFailed, JobStatusRunning, JobStatusRunning, JobStatusSuccess}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v (failed first, newest start first within a group)", order, want)
		}
	}
	if data.StatusCounts[JobStatusGroupFailed] != 2 || data.StatusCounts[JobStatusGroupRunning] != 2 || data.StatusCounts[JobStatusGroupFinished] != 1 {
		t.Fatalf("status counts = %v", data.StatusCounts)
	}
	var running *JobInstance
	for _, job := range data.Items {
		if job.Status == JobStatusRunning && job.CreatedBy == 1 {
			running = job
		}
	}
	if running == nil || running.TaskName != "orders-cdc" || running.Live == nil || running.Live.WriteQPS == nil || *running.Live.WriteQPS != 120 {
		t.Fatalf("live decoration missing: %+v", running)
	}

	cases := []struct {
		name   string
		filter JobFilter
		want   int
	}{
		{"mode batch", JobFilter{Mode: "batch"}, 1},
		{"cluster 1 without previews", JobFilter{ClusterID: 1, ExcludeRunTypes: []RunType{RunTypePreview}}, 4},
		{"keyword task name, case-insensitive", JobFilter{Keyword: "ORDERS"}, 5},
		{"keyword engine job id", JobFilter{Keyword: "888"}, 1},
		{"failed group", JobFilter{Statuses: []JobStatus{JobStatusFailed, JobStatusFailing}}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			filter := tc.filter
			got, err := service.ListJobsWithOptions(ctx, admin, &filter, JobListOptions{})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if int(got.Total) != tc.want {
				t.Fatalf("total = %d, want %d", got.Total, tc.want)
			}
		})
	}
}

func TestListJobsWithOptionsInMemoryRateSort(t *testing.T) {
	service := newTestSyncService(t)
	seedJobListFixture(t, service)
	ctx := context.Background()
	admin := executionapp.Actor{UserID: 1, IsAdmin: true}
	running, _ := ResolveJobStatusGroups(JobStatusGroupRunning)

	data, err := service.ListJobsWithOptions(ctx, admin, &JobFilter{Statuses: running, Sort: jobListSortRate, Page: 1, Size: 1}, JobListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if data.Total != 2 || len(data.Items) != 1 || data.Items[0].EngineJobID != "888" {
		t.Fatalf("rate sort page 1 = total %d items %+v, want the 500 rows/s job first", data.Total, data.Items)
	}
	_, err = service.ListJobsWithOptions(ctx, admin, &JobFilter{Sort: jobListSortRate}, JobListOptions{})
	if !errors.Is(err, ErrInvalidJobListQuery) {
		t.Fatalf("rate sort without running filter err = %v, want ErrInvalidJobListQuery", err)
	}
	if _, err := ResolveJobStatusGroups("running,bogus"); !errors.Is(err, ErrInvalidJobListQuery) {
		t.Fatalf("unknown group err = %v", err)
	}
}

func TestListJobsVisibilityMatchesGetJob(t *testing.T) {
	service := newTestSyncService(t)
	_, privateTask := seedJobListFixture(t, service)
	ctx := context.Background()
	other := executionapp.Actor{UserID: 2}

	data, err := service.ListJobsWithOptions(ctx, other, &JobFilter{}, JobListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, job := range data.Items {
		if job.TaskID == privateTask.ID {
			t.Fatalf("user 2 sees a job of a private task: %+v", job)
		}
	}
	if data.Total != 5 {
		t.Fatalf("total = %d, want 5 (all jobs under the public task, including other users')", data.Total)
	}
	legacy, total, err := service.ListJobsForActor(ctx, other, &JobFilter{})
	if err != nil || total != data.Total || len(legacy) != len(data.Items) {
		t.Fatalf("ListJobsForActor = (%d, %v), want the same scope as ListJobsWithOptions", total, err)
	}
}

// withEmbeddedObservability 打开 observability.enabled 并起一份指向 t.TempDir() 的内嵌 tsdb。
// withEmbeddedObservability flips observability.enabled on and starts an embedded tsdb rooted at t.TempDir().
func withEmbeddedObservability(t *testing.T) {
	t.Helper()
	enabled := config.Config.Observability.Enabled
	t.Cleanup(func() {
		config.Config.Observability.Enabled = enabled
		_ = embedts.Close()
	})
	config.Config.Observability.Enabled = true
	_ = embedts.Close()
	if err := embedts.Start(context.Background(), embedts.Options{Dir: t.TempDir()}); err != nil {
		t.Fatalf("start embedded tsdb: %v", err)
	}
}

func TestGetJobSparklinesDropsInvisibleIDs(t *testing.T) {
	service := newTestSyncService(t)
	seedJobListFixture(t, service)
	ctx := context.Background()
	withEmbeddedObservability(t)

	// 内嵌 tsdb 里同时写入可见作业 2 与不可见作业 5 的计数器；查询表达式只圈可见 ID，
	// job 5 的序列留在库里也不会混进结果。
	// Seed counters for both the visible job 2 and the invisible job 5; the query expression
	// only matches visible IDs, so job 5's series must never leak into the result.
	app := embedts.Appender(ctx)
	if app == nil {
		t.Fatal("embedded tsdb appender is nil")
	}
	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 6; i++ {
		tsMs := now.Add(time.Duration(i-5) * 15 * time.Second).UnixMilli()
		for jobID, v := range map[string]float64{"2": float64(i) * 10, "5": float64(i) * 100} {
			if _, err := app.Append(0, labels.FromStrings(labels.MetricName, "stx_job_sink_write_rows_total", "job_id", jobID), tsMs, v); err != nil {
				t.Fatalf("append: %v", err)
			}
		}
	}
	if err := app.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// 用户 2 看不到私有任务下的作业 5，也看不到不存在的 999。/ User 2 cannot view job 5 (private task) nor job 999.
	data, err := service.GetJobSparklinesForActor(ctx, executionapp.Actor{UserID: 2}, []uint{2, 5, 999}, "1h", "")
	if err != nil {
		t.Fatalf("sparklines: %v", err)
	}
	if data.Source != "prometheus" {
		t.Fatalf("source = %s, want prometheus", data.Source)
	}
	points := data.Items["2"]
	if len(points) == 0 || points[0][0] < 1e12 {
		t.Fatalf("items = %+v, want job 2 series with ms timestamps", data.Items)
	}
	if _, ok := data.Items["5"]; ok {
		t.Fatalf("items leaked invisible job 5: %+v", data.Items)
	}
	if _, ok := data.Items["999"]; ok {
		t.Fatalf("items leaked missing job 999: %+v", data.Items)
	}
	if _, err := ParseJobIDList("1,abc"); !errors.Is(err, ErrInvalidJobListQuery) {
		t.Fatalf("bad id err = %v", err)
	}
}

func TestGetJobSparklinesWithoutPrometheus(t *testing.T) {
	service := newTestSyncService(t)
	enabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = enabled })
	config.Config.Observability.Enabled = false
	data, err := service.GetJobSparklinesForActor(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, []uint{1}, "", "read")
	if err != nil {
		t.Fatalf("sparklines: %v", err)
	}
	if data.Source != "none" || data.Reason != "observability_disabled" || len(data.Items) != 0 || data.Metric != "read" {
		t.Fatalf("result = %+v", data)
	}
}

func TestGetHomeJobSummaryCountsAndAttention(t *testing.T) {
	service := newTestSyncService(t)
	_, privateTask := seedJobListFixture(t, service)
	ctx := context.Background()
	enabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = enabled })
	config.Config.Observability.Enabled = false
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	finished := now.Add(-time.Hour)
	if err := service.repo.db.Model(&JobInstance{}).Where("status IN ?", []JobStatus{JobStatusFailed, JobStatusFailing}).
		Update("finished_at", finished).Error; err != nil {
		t.Fatal(err)
	}

	summary, err := service.GetHomeJobSummary(ctx, executionapp.Actor{UserID: 2}, 0, now)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Running != 2 || summary.RunningStreaming != 2 || summary.WriteQPS != 620 || summary.ReadQPS != 130 {
		t.Fatalf("running = %+v", summary)
	}
	if summary.Failed24h != 2 || summary.FailedPrev24h != 0 {
		t.Fatalf("failed = %d / %d, want 2 / 0 (global, regardless of viewer)", summary.Failed24h, summary.FailedPrev24h)
	}
	if summary.TrendSource != "none" || len(summary.WriteTrend) != 0 {
		t.Fatalf("trend = %s %v", summary.TrendSource, summary.WriteTrend)
	}
	var privateItem *HomeAttentionItem
	for i := range summary.Attention {
		if summary.Attention[i].TaskID == privateTask.ID {
			privateItem = &summary.Attention[i]
		}
	}
	if len(summary.Attention) != 2 || privateItem == nil || privateItem.Viewable || privateItem.Kind != "job_failed" {
		t.Fatalf("attention = %+v, want 2 failed items with the private one not viewable", summary.Attention)
	}

	scoped, err := service.GetHomeJobSummary(ctx, executionapp.Actor{UserID: 1, IsAdmin: true}, 2, now)
	if err != nil {
		t.Fatalf("summary cluster 2: %v", err)
	}
	if scoped.Running != 0 || scoped.Failed24h != 1 {
		t.Fatalf("cluster 2 summary = running %d failed %d, want 0 / 1", scoped.Running, scoped.Failed24h)
	}
}

func TestRunningJobCountsByClusterExcludesPreviews(t *testing.T) {
	service := newTestSyncService(t)
	seedJobListFixture(t, service)
	if err := service.repo.db.Create(&JobInstance{TaskID: 1, RunType: RunTypePreview, Status: JobStatusRunning,
		SubmitSpec: JSONMap{"cluster_id": uint(1)}}).Error; err != nil {
		t.Fatal(err)
	}
	counts, err := service.RunningJobCountsByCluster(context.Background())
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts[1] != 2 || counts[2] != 0 {
		t.Fatalf("counts = %v, want cluster 1 = 2 (preview excluded), cluster 2 = 0", counts)
	}
}
