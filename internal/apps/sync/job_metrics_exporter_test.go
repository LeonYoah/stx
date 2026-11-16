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

	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// instantJobVector 在内嵌 tsdb 上执行即时查询并返回 vector。
// instantJobVector runs an instant query on the embedded tsdb and returns the vector.
func instantJobVector(t *testing.T, ctx context.Context, expr string, at time.Time) promql.Vector {
	t.Helper()
	engine, queryable := embedts.QueryEngine()
	require.NotNil(t, engine)
	require.NotNil(t, queryable)
	qry, err := engine.NewInstantQuery(ctx, queryable, nil, expr, at.UTC())
	require.NoError(t, err)
	defer qry.Close()
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	vec, ok := res.Value.(promql.Vector)
	require.True(t, ok, "expected vector result, got %T", res.Value)
	return vec
}

func TestJobMetricsGatherExporter_WritesRegistryToTSDB(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true

	ctx := context.Background()
	require.NoError(t, embedts.Start(ctx, embedts.Options{
		Dir:               t.TempDir(),
		RetentionDuration: 24 * time.Hour,
		QueryTimeout:      5 * time.Second,
		MaxSamples:        100000,
	}))
	t.Cleanup(func() { _ = embedts.Close() })

	exp := newJobPrometheusExporter()
	job := &JobInstance{ID: 42, TaskID: 7, Status: JobStatusRunning, SubmitSpec: JSONMap{"cluster_id": 3}}
	exp.observeJobMetrics(job, map[string]interface{}{
		"SourceReceivedCount": float64(500),
		"SinkWriteCount":      float64(480),
	})

	exporter := &jobMetricsGatherExporter{gather: exp.reg.Gather, tracker: embedts.NewSeriesTracker()}
	exporter.exportOnce(ctx)

	now := time.Now()
	vec := instantJobVector(t, ctx, `stx_job_source_received_rows_total{task_id="7",job_id="42"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 500.0, vec[0].F)
	require.Equal(t, stxSyncJobsJobLabel, vec[0].Metric.Get("job"))
	require.Equal(t, "3", vec[0].Metric.Get("cluster_id"))

	vec = instantJobVector(t, ctx, `up{job="stx_sync_jobs"}`, now)
	require.Len(t, vec, 1)
	require.Equal(t, 1.0, vec[0].F)

	// 作业进入终态后序列被摘出 Registry，下一轮导出补 stale marker。
	// 查询点向后偏移避开写入水位线可能推到 wall clock +1ms 的 stale 边界。
	// Once the job reaches a terminal state its series leave the Registry and the next
	// export writes stale markers; the query point is offset past the watermark boundary.
	job.Status = JobStatusSuccess
	exp.observeJobMetrics(job, nil)
	exporter.exportOnce(ctx)

	now = time.Now().Add(5 * time.Millisecond)
	vec = instantJobVector(t, ctx, `stx_job_source_received_rows_total{task_id="7",job_id="42"}`, now)
	require.Empty(t, vec)
	// up 常数序列始终存在。
	// The constant up series is always present.
	vec = instantJobVector(t, ctx, `up{job="stx_sync_jobs"}`, now)
	require.Len(t, vec, 1)
}

func TestStartJobMetricsExporter_GatesOnConfigAndReadiness(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })

	ctx := context.Background()
	// observability 关闭：不起协程、不报错。
	// Observability off: no goroutine, no error.
	config.Config.Observability.Enabled = false
	StartJobMetricsExporter(ctx)

	// tsdb 未就绪：同样直接返回。
	// tsdb not ready: also returns without launching anything.
	config.Config.Observability.Enabled = true
	_ = embedts.Close()
	StartJobMetricsExporter(ctx)
}

// startJobMetricsTSDB 为测试启动内嵌 tsdb 并打开 observability。
// startJobMetricsTSDB starts an embedded tsdb with observability on for a test.
func startJobMetricsTSDB(t *testing.T) context.Context {
	t.Helper()
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true
	ctx := context.Background()
	require.NoError(t, embedts.Start(ctx, embedts.Options{
		Dir:               t.TempDir(),
		RetentionDuration: 24 * time.Hour,
		QueryTimeout:      5 * time.Second,
		MaxSamples:        100000,
	}))
	t.Cleanup(func() { _ = embedts.Close() })
	return ctx
}

// registryHasJob 报告 Registry 里是否还有该作业的 stx_job_* 序列。
// registryHasJob reports whether the Registry still holds stx_job_* series for the job.
func registryHasJob(t *testing.T, exp *jobPrometheusExporter, jobID string) bool {
	t.Helper()
	mfs, err := exp.reg.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "job_id" && lp.GetValue() == jobID {
					return true
				}
			}
		}
	}
	return false
}

func TestJobMetricsExporter_TerminalFinalValueWrittenBeforeDelete(t *testing.T) {
	ctx := startJobMetricsTSDB(t)
	exp := newJobPrometheusExporter()
	exp.enableDeferredDeletes()
	exporter := &jobMetricsGatherExporter{gather: exp.reg.Gather, source: exp, tracker: embedts.NewSeriesTracker()}

	job := &JobInstance{ID: 51, TaskID: 9, Status: JobStatusRunning, SubmitSpec: JSONMap{"cluster_id": 1}}
	exp.observeJobMetrics(job, map[string]interface{}{"SinkWriteCount": float64(500)})
	exporter.exportOnce(ctx)

	// 终态这次对账带来终值 900：先进 Registry，序列登记待删而不是立刻删除。
	// The terminal reconcile carries the final value 900: it lands in the Registry and the series
	// is queued for deletion instead of being removed right away.
	job.Status = JobStatusSuccess
	exp.observeJobMetrics(job, map[string]interface{}{"SinkWriteCount": float64(900)})
	require.True(t, registryHasJob(t, exp, "51"))

	exporter.exportOnce(ctx)
	vec := instantJobVector(t, ctx, `stx_job_sink_write_rows_total{job_id="51"}`, time.Now().Add(time.Millisecond))
	require.Len(t, vec, 1)
	require.Equal(t, 900.0, vec[0].F)
	// 写入之后才真正删除；下一轮导出给它写 stale marker。
	// Removal happens only after the write; the next export marks it stale.
	require.False(t, registryHasJob(t, exp, "51"))

	exporter.exportOnce(ctx)
	vec = instantJobVector(t, ctx, `stx_job_sink_write_rows_total{job_id="51"}`, time.Now().Add(5*time.Millisecond))
	require.Empty(t, vec)
}

func TestJobMetricsExporter_DeletesImmediatelyWithoutExportLoop(t *testing.T) {
	originalEnabled := config.Config.Observability.Enabled
	t.Cleanup(func() { config.Config.Observability.Enabled = originalEnabled })
	config.Config.Observability.Enabled = true

	exp := newJobPrometheusExporter()
	job := &JobInstance{ID: 52, TaskID: 9, Status: JobStatusRunning}
	exp.observeJobMetrics(job, map[string]interface{}{"SinkWriteCount": float64(10)})
	require.True(t, registryHasJob(t, exp, "52"))

	// 没有直写协程时维持立即删除，避免待删登记无人消费而累积。
	// Without the direct-write loop deletion stays immediate so the queue cannot pile up.
	job.Status = JobStatusFailed
	exp.observeJobMetrics(job, map[string]interface{}{"SinkWriteCount": float64(20)})
	require.False(t, registryHasJob(t, exp, "52"))
	require.Nil(t, exp.takePendingDeletes())
}

func TestJobMetricsExporter_ZeroStartForShortJob(t *testing.T) {
	ctx := startJobMetricsTSDB(t)
	exp := newJobPrometheusExporter()
	exp.enableDeferredDeletes()
	exporter := &jobMetricsGatherExporter{gather: exp.reg.Gather, source: exp, tracker: embedts.NewSeriesTracker()}

	started := time.Now().Add(-5 * time.Second)
	// 短批作业只在终态被看到一次：有零值起点才能算出 increase。
	// A short batch job seen only once at its terminal state: increase works only thanks to the zero start.
	short := &JobInstance{ID: 61, TaskID: 9, Status: JobStatusSuccess, StartedAt: &started}
	exp.observeJobMetrics(short, map[string]interface{}{"SinkWriteCount": float64(1000)})

	// STX 重启后接手的老作业：不补零值，避免把重启前的累计量算进当前窗口。
	// An old job taken over after an STX restart: no zero start so pre-restart totals are not counted now.
	oldStart := time.Now().Add(-2 * time.Hour)
	old := &JobInstance{ID: 62, TaskID: 9, Status: JobStatusRunning, StartedAt: &oldStart}
	exp.observeJobMetrics(old, map[string]interface{}{"SinkWriteCount": float64(5000)})

	exporter.exportOnce(ctx)
	at := time.Now().Add(time.Millisecond)

	vec := instantJobVector(t, ctx, `count_over_time(stx_job_sink_write_rows_total{job_id="61"}[1m])`, at)
	require.Len(t, vec, 1)
	require.Equal(t, 2.0, vec[0].F, "zero-start sample plus the final value")
	vec = instantJobVector(t, ctx, `increase(stx_job_sink_write_rows_total{job_id="61"}[1m])`, at)
	require.Len(t, vec, 1)
	require.Greater(t, vec[0].F, 0.0)

	vec = instantJobVector(t, ctx, `count_over_time(stx_job_sink_write_rows_total{job_id="62"}[1m])`, at)
	require.Len(t, vec, 1)
	require.Equal(t, 1.0, vec[0].F)

	// 零值起点只补一次：第二轮导出时序列已出现过。
	// The zero start is written once only: the series is already known in the second round.
	exp.observeJobMetrics(old, map[string]interface{}{"SinkWriteCount": float64(5100)})
	exporter.exportOnce(ctx)
	vec = instantJobVector(t, ctx, `count_over_time(stx_job_sink_write_rows_total{job_id="62"}[1m])`, time.Now().Add(time.Millisecond))
	require.Len(t, vec, 1)
	require.Equal(t, 2.0, vec[0].F)
}
