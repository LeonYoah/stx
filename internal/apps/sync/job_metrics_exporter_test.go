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
