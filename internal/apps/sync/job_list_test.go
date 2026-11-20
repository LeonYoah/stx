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
	"math"
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

func TestListJobsFinishedAtAndDurationSort(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	db := service.repo.db
	admin := executionapp.Actor{UserID: 1, IsAdmin: true}
	base := time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC)
	at := func(minutes int) *time.Time { v := base.Add(time.Duration(minutes) * time.Minute); return &v }
	runningStart := time.Now().Add(-30 * time.Minute) // 运行中约 30 分钟，介于 10/60/120 分钟之间。/ ~30m so it lands mid-order.
	jobs := []*JobInstance{
		{RunType: RunTypeRun, Status: JobStatusSuccess, CreatedBy: 1, StartedAt: at(0), FinishedAt: at(120)}, // 120 分钟
		{RunType: RunTypeRun, Status: JobStatusSuccess, CreatedBy: 1, StartedAt: at(10), FinishedAt: at(20)}, // 10 分钟
		{RunType: RunTypeRun, Status: JobStatusFailed, CreatedBy: 1, StartedAt: at(30), FinishedAt: at(90)},  // 60 分钟
		{RunType: RunTypeRun, Status: JobStatusRunning, CreatedBy: 1, StartedAt: &runningStart},              // 无 finished_at
	}
	for _, job := range jobs {
		if err := db.WithContext(ctx).Create(job).Error; err != nil {
			t.Fatalf("create job: %v", err)
		}
	}
	ids := func(data *JobListData) []uint {
		out := make([]uint, 0, len(data.Items))
		for _, job := range data.Items {
			out = append(out, job.ID)
		}
		return out
	}
	list := func(sort, order string) []uint {
		data, err := service.ListJobsWithOptions(ctx, admin, &JobFilter{Sort: sort, Order: order}, JobListOptions{})
		if err != nil {
			t.Fatalf("list sort=%s order=%s: %v", sort, order, err)
		}
		return ids(data)
	}
	// 结束时间：NULL finished_at 的 running 行不论方向都垫底。
	// finished_at: the running row with NULL finished_at always sorts last.
	if got, want := list(jobListSortFinishedAt, "desc"), []uint{jobs[0].ID, jobs[2].ID, jobs[1].ID, jobs[3].ID}; !equalUints(got, want) {
		t.Fatalf("finished_at desc = %v, want %v", got, want)
	}
	if got, want := list(jobListSortFinishedAt, "asc"), []uint{jobs[1].ID, jobs[2].ID, jobs[0].ID, jobs[3].ID}; !equalUints(got, want) {
		t.Fatalf("finished_at asc = %v, want %v", got, want)
	}
	// 耗时：已完成按 finished-started，运行中按 now-started（≈30 分钟，夹在 10 与 60 之间）。
	// duration: finished jobs order by finished-started; the running job counts to now (~30m).
	if got, want := list(jobListSortDuration, "desc"), []uint{jobs[0].ID, jobs[2].ID, jobs[3].ID, jobs[1].ID}; !equalUints(got, want) {
		t.Fatalf("duration desc = %v, want %v", got, want)
	}
	if got, want := list(jobListSortDuration, "asc"), []uint{jobs[1].ID, jobs[3].ID, jobs[2].ID, jobs[0].ID}; !equalUints(got, want) {
		t.Fatalf("duration asc = %v, want %v", got, want)
	}
	if _, err := service.ListJobsWithOptions(ctx, admin, &JobFilter{Sort: "bogus"}, JobListOptions{}); !errors.Is(err, ErrInvalidJobListQuery) {
		t.Fatalf("unsupported sort err = %v, want ErrInvalidJobListQuery", err)
	}
}

func equalUints(a, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	// t.TempDir 必须先建：它的删除清理在调用时就注册，而 t.Cleanup 是后进先出——
	// 若 Close 的清理先注册，结束时会先删 chunks_head 再关库，Head.Close 的
	// mmapHeadChunks 就会写进已删目录直接 panic（样本恰好跨 2h chunk 边界时必现）。
	// t.TempDir must come first: its removal cleanup registers at call time and
	// cleanups run LIFO — registering Close first would delete chunks_head before
	// the db closes, and Head.Close's mmapHeadChunks then writes into a missing dir
	// (deterministic panic whenever samples straddle a 2h chunk boundary).
	dir := t.TempDir()
	t.Cleanup(func() {
		config.Config.Observability.Enabled = enabled
		_ = embedts.Close()
	})
	config.Config.Observability.Enabled = true
	_ = embedts.Close()
	if err := embedts.Start(context.Background(), embedts.Options{Dir: dir}); err != nil {
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

func intPtr(v int) *int { return &v }

// TestBuildJobLiveMetricsParity 覆盖与前端 extractJobMetricSummary 对齐的回退口径。
// 每组 fixture 的 metrics 结构与 SeaTunnel 引擎 REST 快照一致（含 3.0 表级 Table* 映射），
// 前端对同一份输入的解析结果应与本测试期望一致。
// TestBuildJobLiveMetricsParity covers the fallback semantics aligned with the frontend
// extractJobMetricSummary. Each fixture mirrors a SeaTunnel engine REST metrics snapshot
// (including 3.0 table-level Table* maps); the frontend must derive identical values.
func TestBuildJobLiveMetricsParity(t *testing.T) {
	f64 := func(v float64) *float64 { return &v }
	cases := []struct {
		name    string
		status  JobStatus
		metrics map[string]interface{}
		want    *JobLiveMetrics
	}{
		{
			name:   "仅全局指标 running",
			status: JobStatusRunning,
			metrics: map[string]interface{}{
				"SourceReceivedCount": float64(100),
				"SinkWriteCount":      float64(90),
				"SourceReceivedQPS":   float64(10),
				"SinkWriteQPS":        float64(9),
			},
			want: &JobLiveMetrics{ReadTotal: f64(100), WriteTotal: f64(90), ReadQPS: f64(10), WriteQPS: f64(9)},
		},
		{
			name:   "仅 SinkCommitted 写侧回退",
			status: JobStatusRunning,
			metrics: map[string]interface{}{
				"SourceReceivedCount": float64(50),
				"SinkCommittedCount":  float64(40),
				"SourceReceivedQPS":   float64(5),
				"SinkCommittedQPS":    float64(4),
			},
			want: &JobLiveMetrics{ReadTotal: f64(50), WriteTotal: f64(40), ReadQPS: f64(5), WriteQPS: f64(4)},
		},
		{
			name:   "SinkWrite 优先于 SinkCommitted",
			status: JobStatusRunning,
			metrics: map[string]interface{}{
				"SinkWriteCount":     float64(10),
				"SinkCommittedCount": float64(99),
				"SinkWriteQPS":       float64(1),
				"SinkCommittedQPS":   float64(9),
			},
			want: &JobLiveMetrics{WriteTotal: f64(10), WriteQPS: f64(1)},
		},
		{
			name:   "仅表级指标求和且逻辑表去重",
			status: JobStatusRunning,
			metrics: map[string]interface{}{
				"TableSourceReceivedCount": map[string]interface{}{
					"Source[0].db.orders":    float64(300),
					"Source[0].db.customers": float64(200),
				},
				"TableSinkWriteCount": map[string]interface{}{
					"Sink[0].db.orders":    float64(280),
					"Sink[0].db.customers": float64(190),
				},
				"TableSourceReceivedQPS": map[string]interface{}{
					"Source[0].db.orders":    float64(30),
					"Source[0].db.customers": float64(20),
				},
				"TableSinkWriteQPS": map[string]interface{}{
					"Sink[0].db.orders":    float64(28),
					"Sink[0].db.customers": float64(19),
				},
			},
			want: &JobLiveMetrics{ReadTotal: f64(500), WriteTotal: f64(470), ReadQPS: f64(50), WriteQPS: f64(47), TableCount: intPtr(2)},
		},
		{
			name:   "表级写侧先 TableSinkWriteCount 再 TableSinkCommittedCount",
			status: JobStatusRunning,
			metrics: map[string]interface{}{
				"TableSinkWriteCount": map[string]interface{}{
					"Sink[0].db.t": float64(7),
				},
				"TableSinkCommittedCount": map[string]interface{}{
					"Sink[0].db.t": float64(77),
				},
			},
			want: &JobLiveMetrics{WriteTotal: f64(7), TableCount: intPtr(1)},
		},
		{
			name:   "Source 与 Sink 同表去重 archive_ 前缀忽略",
			status: JobStatusRunning,
			metrics: map[string]interface{}{
				"TableSourceReceivedCount": map[string]interface{}{
					"Source[0].db.archive_orders": float64(1),
				},
				"TableSinkCommittedCount": map[string]interface{}{
					"Sink[0].db.Orders": float64(1),
				},
			},
			want: &JobLiveMetrics{ReadTotal: f64(1), WriteTotal: f64(1), TableCount: intPtr(1)},
		},
		{
			name:   "非运行中不返回 qps 但保留累计",
			status: JobStatusSuccess,
			metrics: map[string]interface{}{
				"SourceReceivedCount": float64(10),
				"SinkWriteCount":      float64(8),
				"SourceReceivedQPS":   float64(3),
				"SinkWriteQPS":        float64(2),
			},
			want: &JobLiveMetrics{ReadTotal: f64(10), WriteTotal: f64(8)},
		},
		{
			name:    "空指标返回 nil",
			status:  JobStatusRunning,
			metrics: map[string]interface{}{},
			want:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			job := &JobInstance{
				Status: tc.status,
				ResultPreview: map[string]interface{}{
					"metrics": tc.metrics,
				},
			}
			got := buildJobLiveMetrics(job)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("live = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("live = nil, want non-nil")
			}
			check := func(name string, got, want *float64) {
				t.Helper()
				if want == nil {
					if got != nil {
						t.Fatalf("%s = %v, want nil", name, *got)
					}
					return
				}
				if got == nil || *got != *want {
					t.Fatalf("%s = %v, want %v", name, got, *want)
				}
			}
			check("read_total", got.ReadTotal, tc.want.ReadTotal)
			check("write_total", got.WriteTotal, tc.want.WriteTotal)
			check("read_qps", got.ReadQPS, tc.want.ReadQPS)
			check("write_qps", got.WriteQPS, tc.want.WriteQPS)
			if tc.want.TableCount == nil {
				if got.TableCount != nil {
					t.Fatalf("table_count = %d, want nil", *got.TableCount)
				}
			} else if got.TableCount == nil || *got.TableCount != *tc.want.TableCount {
				t.Fatalf("table_count = %v, want %d", got.TableCount, *tc.want.TableCount)
			}
		})
	}
}

// TestListJobsActiveWindowFilter 覆盖「窗口内活跃」重叠语义：窗口内运行过的作业命中，
// 运行中（finished_at 为空）与排队（started_at 为空）作业在任何窗口都算活跃。
// TestListJobsActiveWindowFilter covers the active-in-window overlap: jobs that ran during the window match,
// while running (finished_at NULL) and pending (started_at NULL) jobs count as active in every window.
func TestListJobsActiveWindowFilter(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	db := service.repo.db
	task := &Task{NodeType: TaskNodeTypeFile, Name: "windowed", ClusterID: 1, Mode: TaskModeStreaming, CreatedBy: 1}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	at := func(h int) *time.Time { v := base.Add(time.Duration(h) * time.Hour); return &v }
	jobs := map[string]*JobInstance{
		// 窗口内运行结束：09:00–11:00，与 [10:00,14:00] 重叠 → 命中。/ Ended inside the overlap → in.
		"span": {TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusSuccess, CreatedBy: 1,
			StartedAt: at(-1), FinishedAt: at(1)},
		// 窗口开始前就结束：05:00–06:00 → 不命中。/ Finished before the window → out.
		"early": {TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusSuccess, CreatedBy: 1,
			StartedAt: at(-5), FinishedAt: at(-4)},
		// 窗口结束后才启动：15:00 → 不命中。/ Starts after the window → out.
		"late": {TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusPending, CreatedBy: 1,
			StartedAt: at(5)},
		// 一直运行：started 很久以前、未结束 → 任何窗口命中。/ Long-running: started long ago, still running → in.
		"running": {TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusRunning, CreatedBy: 1,
			StartedAt: at(-48)},
		// 排队中：尚未启动 → 视为活跃。/ Pending: not started yet → counts as active.
		"pending": {TaskID: task.ID, RunType: RunTypeRun, Status: JobStatusPending, CreatedBy: 1},
	}
	for name, job := range jobs {
		if err := db.Create(job).Error; err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	idsOf := func(items []*JobInstance) map[uint]bool {
		out := make(map[uint]bool, len(items))
		for _, item := range items {
			out[item.ID] = true
		}
		return out
	}
	window := func(hFrom, hTo int) (*time.Time, *time.Time) {
		var from, to *time.Time
		if hFrom != math.MinInt {
			from = at(hFrom)
		}
		if hTo != math.MinInt {
			to = at(hTo)
		}
		return from, to
	}
	cases := []struct {
		name       string
		hFrom, hTo int
		want       []string
	}{
		{"window 10:00-14:00", 0, 4, []string{"span", "running", "pending"}},
		// 远过去窗口：连 running 都还没启动，只有排队作业算活跃。/ Far-past window: only pending is active.
		{"window before all runs", -100, -60, []string{"pending"}},
		// 覆盖 running 启动瞬间的窗口。/ Window covering the moment running started.
		{"window around running start", -49, -47, []string{"running", "pending"}},
		// 远未来窗口：未结束的作业（late 排队未结束、running、pending）都命中。
		// Far-future window: unfinished jobs (late/running/pending have no finished_at) all match.
		{"window after all runs", 50, 60, []string{"late", "running", "pending"}},
		{"only from bound", 0, math.MinInt, []string{"span", "late", "running", "pending"}},
		{"only to bound", math.MinInt, 0, []string{"span", "early", "running", "pending"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := window(tc.hFrom, tc.hTo)
			items, total, err := service.repo.ListJobInstances(ctx, &JobFilter{
				IncludeAll: true, TaskID: task.ID, ActiveFrom: from, ActiveTo: to, Size: 50,
			})
			if err != nil {
				t.Fatal(err)
			}
			if int(total) != len(tc.want) {
				t.Fatalf("total = %d, want %d (%v)", total, len(tc.want), idsOf(items))
			}
			got := idsOf(items)
			for _, name := range tc.want {
				if !got[jobs[name].ID] {
					t.Fatalf("missing %s in %v", name, got)
				}
			}
		})
	}
}
