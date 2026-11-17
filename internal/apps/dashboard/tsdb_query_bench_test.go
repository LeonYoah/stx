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

package dashboard

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
	"github.com/prometheus/prometheus/model/labels"
)

type benchJobs struct{}

func (benchJobs) DescribeJobsForDashboard(_ context.Context, _ executionapp.Actor, ids []uint) (map[uint]syncapp.JobBrief, error) {
	out := make(map[uint]syncapp.JobBrief, len(ids))
	for _, id := range ids {
		out[id] = syncapp.JobBrief{ID: id, TaskID: id, ClusterID: 1, Name: "job-" + strconv.FormatUint(uint64(id), 10), Viewable: true}
	}
	return out, nil
}

func (benchJobs) JobRunCalendar(context.Context, uint, int, time.Time) (*syncapp.JobCalendar, error) {
	return &syncapp.JobCalendar{Weeks: 1, EndDate: "2026-01-01", Runs: [][]int{{0}}, Failed: [][]int{{0}}}, nil
}

func (benchJobs) JobAnnotationsBetween(context.Context, executionapp.Actor, uint, time.Time, time.Time) ([]syncapp.JobAnnotation, error) {
	return []syncapp.JobAnnotation{}, nil
}

// startBenchTSDB 拉起内嵌 tsdb 并写入与首页大盘同形状的 1h 样本。
// startBenchTSDB starts the embedded tsdb and seeds 1h of samples in the home-dashboard shape.
func startBenchTSDB(b *testing.B, end time.Time) {
	b.Helper()
	original := config.Config.Observability.Enabled
	config.Config.Observability.Enabled = true
	_ = embedts.Close()
	if err := embedts.Start(context.Background(), embedts.Options{
		Dir:               b.TempDir(),
		RetentionDuration: 48 * time.Hour,
		QueryTimeout:      8 * time.Second,
		MaxSamples:        50_000_000,
	}); err != nil {
		b.Fatalf("start tsdb: %v", err)
	}
	b.Cleanup(func() {
		config.Config.Observability.Enabled = original
		_ = embedts.Close()
	})
	seedBenchMetrics(b, end)
}

// seedBenchMetrics 写入 3 集群 × 2 节点的引擎指标和 8 个作业的吞吐/延迟点。
// seedBenchMetrics writes engine metrics for 3 clusters × 2 nodes and throughput/delay points for 8 jobs.
func seedBenchMetrics(b *testing.B, end time.Time) {
	b.Helper()
	ctx := context.Background()
	app := embedts.Appender(ctx)
	if app == nil {
		b.Fatal("appender is nil")
	}
	start := end.Add(-time.Hour)
	step := 15 * time.Second
	appendPoint := func(name string, extra []string, ts int64, value float64) {
		b.Helper()
		pairs := append([]string{labels.MetricName, name}, extra...)
		if _, err := app.Append(0, labels.FromStrings(pairs...), ts, value); err != nil {
			b.Fatalf("append %s: %v", name, err)
		}
	}
	for clusterID := 1; clusterID <= 3; clusterID++ {
		cid := strconv.Itoa(clusterID)
		for n := 1; n <= 2; n++ {
			instance := fmt.Sprintf("10.0.%d.%d:5801", clusterID, n)
			base := []string{"job", "seatunnel_engine_http", "instance", instance, "cluster_id", cid}
			i := 0
			for ts := start; !ts.After(end); ts = ts.Add(step) {
				ms := ts.UnixMilli()
				v := float64(i)
				appendPoint("up", base, ms, 1)
				appendPoint("node_state", base, ms, 1)
				appendPoint("jvm_memory_bytes_used", append(append([]string{}, base...), "area", "heap"), ms, 1e9+v*1e6)
				appendPoint("jvm_memory_bytes_max", append(append([]string{}, base...), "area", "heap"), ms, 4e9)
				appendPoint("process_cpu_seconds_total", base, ms, v)
				appendPoint("jvm_gc_collection_seconds_sum", base, ms, v/10)
				appendPoint("job_count", append(append([]string{}, base...), "type", "running"), ms, 3)
				appendPoint("jvm_threads_current", base, ms, 80)
				appendPoint("process_open_fds", base, ms, 200)
				appendPoint("process_max_fds", base, ms, 65535)
				appendPoint("process_start_time_seconds", base, ms, float64(end.Add(-24*time.Hour).Unix()))
				appendPoint("cluster_info", append(append([]string{}, base...), "master", instance), ms, 1)
				appendPoint("hazelcast_partition_isClusterSafe", base, ms, 1)
				appendPoint("jvm_threads_deadlocked", base, ms, 0)
				appendPoint("job_thread_pool_rejection_total", base, ms, 0)
				appendPoint("hazelcast_executor_queueSize", base, ms, 1)
				appendPoint("hazelcast_executor_queueRemainingCapacity", base, ms, 99)
				i++
			}
		}
	}
	for jobID := 1; jobID <= 8; jobID++ {
		jid := strconv.Itoa(jobID)
		job := []string{"job_id", jid, "cluster_id", "1"}
		i := 0
		for ts := start; !ts.After(end); ts = ts.Add(step) {
			ms := ts.UnixMilli()
			v := float64(i * 100)
			appendPoint("stx_job_source_received_rows_total", job, ms, v)
			appendPoint("stx_job_sink_write_rows_total", job, ms, v*0.9)
			appendPoint("stx_job_source_received_bytes_total", job, ms, v*64)
			appendPoint("stx_job_sink_write_bytes_total", job, ms, v*60)
			appendPoint("stx_job_cdc_delay_seconds", job, ms, float64(jobID))
			appendPoint("stx_job_checkpoint_duration_seconds", job, ms, 0.2)
			appendPoint("stx_job_checkpoint_failed_total", job, ms, 0)
			appendPoint("stx_job_vertex_busy_ratio", append(append([]string{}, job...), "vertex", "Source[0]-FakeSource"), ms, 0.4)
			appendPoint("stx_job_edge_backpressure_ratio", append(append([]string{}, job...), "edge", "e0"), ms, 0.1)
			i++
		}
	}
	if err := app.Commit(); err != nil {
		b.Fatalf("commit: %v", err)
	}
}

// BenchmarkTSDBClusterDashboardCold 测量绕过 15s 缓存时集群首页大盘的冷 PromQL 成本。
// BenchmarkTSDBClusterDashboardCold measures the cluster home dashboard with the 15s PromQL cache bypassed.
func BenchmarkTSDBClusterDashboardCold(b *testing.B) {
	end := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	startBenchTSDB(b, end)
	svc := NewClusterDashboardService(fakeHealth{}, fakeClusterNodes{}, fakeRunning{})
	svc.now = func() time.Time { return end }
	svc.rangeQuery = promquery.Range
	svc.instantQuery = promquery.Instant
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := svc.GetClusterDashboard(ctx, 0, "1h", nil)
		if err != nil {
			b.Fatal(err)
		}
		if out.Source != promquery.SourcePrometheus {
			b.Fatalf("source=%s reason=%s", out.Source, out.Reason)
		}
	}
}

// BenchmarkTSDBClusterDashboardCached 先预热再测命中进程内 PromQL 缓存的大盘延迟。
// BenchmarkTSDBClusterDashboardCached warms the in-process PromQL cache, then measures a hit.
func BenchmarkTSDBClusterDashboardCached(b *testing.B) {
	end := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	startBenchTSDB(b, end)
	svc := NewClusterDashboardService(fakeHealth{}, fakeClusterNodes{}, fakeRunning{})
	svc.now = func() time.Time { return end }
	ctx := context.Background()
	if _, err := svc.GetClusterDashboard(ctx, 0, "1h", nil); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.GetClusterDashboard(ctx, 0, "1h", nil); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTSDBJobDashboardCold 测量作业首页大盘冷查询（同样绕过缓存包装）。
// BenchmarkTSDBJobDashboardCold measures the job home dashboard cold path (cache wrapper bypassed).
func BenchmarkTSDBJobDashboardCold(b *testing.B) {
	end := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	startBenchTSDB(b, end)
	svc := NewJobDashboardService(benchJobs{})
	svc.now = func() time.Time { return end }
	svc.rangeQuery = promquery.Range
	svc.instantQuery = promquery.Instant
	ctx := context.Background()
	actor := executionapp.Actor{UserID: 1, IsAdmin: true}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := svc.GetJobDashboard(ctx, actor, 0, "1h", nil, 1)
		if err != nil {
			b.Fatal(err)
		}
		if out.Source != promquery.SourcePrometheus {
			b.Fatalf("source=%s reason=%s", out.Source, out.Reason)
		}
	}
}

// BenchmarkTSDBRangeHeapExpr 单独跑 heap 占比这条最重的区间 PromQL，用来对照大盘里的单查询成本。
// BenchmarkTSDBRangeHeapExpr runs the heap-ratio range PromQL alone so it can be compared with the dashboard fan-out.
func BenchmarkTSDBRangeHeapExpr(b *testing.B) {
	end := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	startBenchTSDB(b, end)
	start := end.Add(-time.Hour)
	expr := `100 * max by (cluster_id, instance) (jvm_memory_bytes_used{job="seatunnel_engine_http",area="heap"}) / max by (cluster_id, instance) (clamp_min(jvm_memory_bytes_max{job="seatunnel_engine_http",area="heap"}, 1))`
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		series, err := promquery.Range(ctx, "embedded", expr, start, end, 15)
		if err != nil {
			b.Fatal(err)
		}
		if len(series) == 0 {
			b.Fatal("expected heap series")
		}
	}
}
