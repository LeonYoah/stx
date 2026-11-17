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

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/prometheus/prometheus/model/labels"
)

// BenchmarkTSDBJobTimeseries 测量作业趋势接口在内嵌 tsdb 上并发跑 throughput/rows/cdc_delay 的成本。
// BenchmarkTSDBJobTimeseries measures the job timeseries API running throughput/rows/cdc_delay concurrently on the embedded tsdb.
func BenchmarkTSDBJobTimeseries(b *testing.B) {
	original := config.Config.Observability.Enabled
	config.Config.Observability.Enabled = true
	_ = embedts.Close()
	ctx := context.Background()
	if err := embedts.Start(ctx, embedts.Options{
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
	now := time.Now().UTC()
	app := embedts.Appender(ctx)
	if app == nil {
		b.Fatal("appender is nil")
	}
	read := labels.FromStrings(labels.MetricName, "stx_job_source_received_rows_total", "job_id", "9")
	write := labels.FromStrings(labels.MetricName, "stx_job_sink_write_rows_total", "job_id", "9")
	cdc := labels.FromStrings(labels.MetricName, "stx_job_cdc_delay_seconds", "job_id", "9")
	for i := 0; i < 240; i++ {
		ts := now.Add(-time.Duration(240-i) * 15 * time.Second).UnixMilli()
		if _, err := app.Append(0, read, ts, float64((i+1)*100)); err != nil {
			b.Fatal(err)
		}
		if _, err := app.Append(0, write, ts, float64((i+1)*80)); err != nil {
			b.Fatal(err)
		}
		if _, err := app.Append(0, cdc, ts, 1.5); err != nil {
			b.Fatal(err)
		}
	}
	if err := app.Commit(); err != nil {
		b.Fatal(err)
	}

	svc := &Service{}
	job := &JobInstance{ID: 9, TaskID: 1, Status: JobStatusRunning}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := svc.getJobTimeseries(ctx, job, "1h", nil, "throughput,rows,cdc_delay", 15)
		if err != nil {
			b.Fatal(err)
		}
		if out.Source != "prometheus" {
			b.Fatalf("source=%s reason=%s", out.Source, out.Reason)
		}
	}
}
