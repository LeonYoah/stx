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

package embedts

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/require"
)

// yearBenchSeriesScrapeInterval 是基准数据的采样间隔，与生产 scrape_interval 一致。
// yearBenchSeriesScrapeInterval matches the production scrape_interval for the seeded data.
const yearBenchSeriesScrapeInterval = 15 * time.Second

// yearBenchBlockSize 与生产 max_block=1w 对齐：一年数据直接落成约 52 个周块，
// 模拟「历史早已压实」的生产状态，而不是 head/2h 碎块的边写边查状态。
// yearBenchBlockSize matches the production max_block=1w layout: a year lands as ~52
// weekly blocks, modeling the compacted production state rather than a head/2h-fragment
// query-while-compacting state.
const yearBenchBlockSize = 7 * 24 * time.Hour

// TestCapacity_YearWindow 是一年长窗口的容量基准：用 tsdb.BlockWriter 把一年计数器数据
// （15s 间隔）直接写成生产形态的周块，再测三类会扫满一年的代表性查询。仅
// STX_TSDB_YEAR_BENCH=1 时运行。门槛：200 序列下每条查询必须在 32s（query_timeout ×4）
// 内返回；不达标时不要放开一年窗口，降采样/预聚合需单独立项（见 embedded-tsdb-capacity.md）。
// TestCapacity_YearWindow is the year-long capacity benchmark: it writes a year of counter
// data (15s interval) into production-shaped weekly blocks via tsdb.BlockWriter, then
// measures three representative queries that scan the whole year. It only runs with
// STX_TSDB_YEAR_BENCH=1. Gate: every query must answer within 32s (query_timeout ×4) at
// 200 series; below that do not ship the year window and file a separate
// downsample/pre-aggregation task (see embedded-tsdb-capacity.md).
func TestCapacity_YearWindow(t *testing.T) {
	if os.Getenv("STX_TSDB_YEAR_BENCH") != "1" {
		t.Skip("一年窗口基准需 STX_TSDB_YEAR_BENCH=1 / year-window bench requires STX_TSDB_YEAR_BENCH=1")
	}
	ctx := context.Background()
	end := time.Now().UTC()
	start := end.Add(-365 * 24 * time.Hour)
	for _, seriesCount := range []int{200, 1000} {
		t.Run(fmt.Sprintf("series=%d", seriesCount), func(t *testing.T) {
			opts := testOptions(t)
			opts.MaxSamples = DefaultMaxSamples
			opts.QueryTimeout = 8 * time.Second
			// 一年数据必须留得住：不限时间保留，否则旧块边写边被删，测不出真实查询成本。
			// The year of data must actually persist: unlimited time retention, otherwise old
			// blocks are deleted while seeding and the query cost is not representative.
			opts.RetentionDuration = 0

			// 先写块再开库：生产状态是「历史早已压实成周块」，用 head appender 边写边查
			// 会把压实竞争算进查询耗时，测出来的是写路径而不是查询路径。
			// Write blocks before opening the DB: production state means history is already
			// compacted into weekly blocks; seeding through the head appender would fold
			// compaction contention into the measurement and time the write path instead.
			seedYearJobBlocks(t, opts.Dir, seriesCount, start, end)
			require.NoError(t, Start(ctx, opts))
			t.Cleanup(func() { _ = Close() })

			engine, queryable := QueryEngineLong()
			require.NotNil(t, engine)
			// 三条覆盖一年窗口的代表查询：作业趋势范围查询、TopWrite 的窗口级即时查询、
			// 读写量的逐日增量范围查询。
			// Three representative queries spanning the year: the jobs-panel rate range
			// query, TopWrite's window-level instant query, and the volume daily-increment
			// range query.
			for _, qc := range []struct {
				name string
				expr string
				step time.Duration // 0 表示即时查询 / 0 for instant query
			}{
				{"jobs_rate_range", `sum(rate(stx_job_sink_write_rows_total[2d]))`, 48 * time.Hour},
				{"jobs_increase_instant", `sum by (job_id) (increase(stx_job_sink_write_rows_total[1y]))`, 0},
				{"volume_daily_range", `increase(stx_job_sink_write_rows_total[1d])`, 24 * time.Hour},
			} {
				t0 := time.Now()
				if qc.step > 0 {
					qry, err := engine.NewRangeQuery(ctx, queryable, nil, qc.expr, start, end, qc.step)
					require.NoError(t, err)
					res := qry.Exec(ctx)
					qry.Close()
					require.NoError(t, res.Err)
					matrix, ok := res.Value.(promql.Matrix)
					require.True(t, ok)
					require.NotEmpty(t, matrix)
					t.Logf("%s took %s (%d series)", qc.name, time.Since(t0), len(matrix))
				} else {
					qry, err := engine.NewInstantQuery(ctx, queryable, nil, qc.expr, end)
					require.NoError(t, err)
					res := qry.Exec(ctx)
					qry.Close()
					require.NoError(t, res.Err)
					vector, ok := res.Value.(promql.Vector)
					require.True(t, ok)
					require.NotEmpty(t, vector)
					t.Logf("%s took %s (%d samples)", qc.name, time.Since(t0), len(vector))
				}
				took := time.Since(t0)
				if seriesCount <= 200 {
					require.Less(t, took, 32*time.Second,
						"%s：200 序列一年窗口必须在 32s 内返回 / %s: the 200-series year window must return within 32s", qc.name, qc.name)
				}
			}
		})
	}
}

// seedYearJobBlocks 用 tsdb.BlockWriter 把 seriesCount 条 15s 间隔的递增计数器按 7 天一个
// 块直接写进 dir，产出与生产压实后相同的落盘形态（无 WAL/head）。
// seedYearJobBlocks writes seriesCount 15s-interval counter series into dir as one 7-day
// block per segment via tsdb.BlockWriter, producing the same on-disk layout as compacted
// production data (no WAL/head).
func seedYearJobBlocks(t *testing.T, dir string, seriesCount int, start, end time.Time) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	series := make([]labels.Labels, seriesCount)
	for i := range series {
		series[i] = labels.FromStrings(labels.MetricName, "stx_job_sink_write_rows_total", "job_id", fmt.Sprint(i))
	}
	stepMs := int64(yearBenchSeriesScrapeInterval / time.Millisecond)
	blockMs := int64(yearBenchBlockSize / time.Millisecond)
	startMs, endMs := start.UnixMilli(), end.UnixMilli()
	blocks := 0
	for wStart := startMs; wStart < endMs; wStart += blockMs {
		wEnd := wStart + blockMs
		if wEnd > endMs {
			wEnd = endMs
		}
		w, err := tsdb.NewBlockWriter(logger, dir, blockMs)
		require.NoError(t, err)
		app := w.Appender(context.Background())
		for i, lset := range series {
			base := float64(i) * 1e9
			for ts := wStart; ts < wEnd; ts += stepMs {
				// 计数器累计值与样本序号一致，保证单调递增。/ Counter value tracks the
				// sample index so the series stays monotonic.
				_, err := app.Append(0, lset, ts, base+float64((ts-startMs)/stepMs))
				require.NoError(t, err)
			}
		}
		require.NoError(t, app.Commit())
		_, err = w.Flush(context.Background())
		require.NoError(t, err)
		require.NoError(t, w.Close())
		blocks++
	}
	t.Logf("seeded %d counter series over %s into %d weekly blocks", seriesCount, end.Sub(start), blocks)
}
