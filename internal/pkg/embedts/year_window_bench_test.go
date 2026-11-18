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
	"os"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"
)

// yearBenchSeriesScrapeInterval 是基准数据的采样间隔，与生产 scrape_interval 一致。
// yearBenchSeriesScrapeInterval matches the production scrape_interval for the seeded data.
const yearBenchSeriesScrapeInterval = 15 * time.Second

// TestCapacity_YearWindow 是一年长窗口的容量基准：合成指定数量的作业计数器序列
// （15s 间隔 × 约一年），测量代表性区间查询耗时。仅 STX_TSDB_YEAR_BENCH=1 时运行。
// 门槛：200 条序列的一年窗口必须在 32s（query_timeout ×4）内返回；不达标时不要放开一年窗口，
// 降采样/预聚合需单独立项（见 embedded-tsdb-capacity.md）。
// TestCapacity_YearWindow is the year-long capacity benchmark: it seeds the given number of
// job counter series (15s interval × ~1y) and measures representative range queries. It only
// runs with STX_TSDB_YEAR_BENCH=1. Gate: the 200-series year window must answer within 32s
// (query_timeout ×4); below that do not ship the year window and file a separate
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
			// 基准要用生产级配额而不是测试桩的小值：MaxSamples=1000 万、query_timeout=8s。
			// The bench needs production-grade quotas rather than the test stub's small values:
			// MaxSamples=10M and query_timeout=8s.
			opts.MaxSamples = DefaultMaxSamples
			opts.QueryTimeout = 8 * time.Second
			// 一年数据必须留得住：不限时间保留，否则旧块边写边被删，测不出真实查询成本。
			// The year of data must actually persist: unlimited time retention, otherwise old
			// blocks are deleted while seeding and the query cost is not representative.
			opts.RetentionDuration = 0
			require.NoError(t, Start(ctx, opts))
			t.Cleanup(func() { _ = Close() })

			seedYearJobCounters(t, ctx, seriesCount, start, end)
			t.Logf("seeded %d counter series over %s", seriesCount, end.Sub(start))

			// 不做额外压实：全量压实一年数据本身要几十分钟；后台压实照常运行，
			// 查询落在生产同样的「边压实边查询」状态。
			// No extra flush: compacting a full year takes tens of minutes; background
			// compaction keeps running, so the query measures the same query-while-compacting
			// state as production.

			engine, queryable := QueryEngineLong()
			require.NotNil(t, engine)
			// jobs 面板代表性查询：跨全部作业写入速率的求和，一年窗口、2d 步长。
			// Representative jobs-panel query: summed write rate over all jobs, year window, 2d step.
			expr := `sum(rate(stx_job_sink_write_rows_total[2d]))`
			t0 := time.Now()
			qry, err := engine.NewRangeQuery(ctx, queryable, nil, expr, start, end, 48*time.Hour)
			require.NoError(t, err)
			res := qry.Exec(ctx)
			qry.Close()
			require.NoError(t, res.Err)
			matrix, ok := res.Value.(promql.Matrix)
			require.True(t, ok)
			require.NotEmpty(t, matrix)
			took := time.Since(t0)
			t.Logf("year-window query %s took %s (%d result points)", expr, took, len(matrix[0].Floats))
			if seriesCount <= 200 {
				require.Less(t, took, 32*time.Second,
					"200 序列一年窗口必须在 32s 内返回 / the 200-series year window must return within 32s")
			}
		})
	}
}

// seedYearJobCounters 以 15s 间隔写入 seriesCount 条递增计数器序列（job_id=i），覆盖 start..end。
// 分批提交以降低单次 appender 开销；旧样本会随 head compaction 落块，内存不随数据量膨胀。
// seedYearJobCounters writes seriesCount monotonically increasing counter series (job_id=i)
// at the scrape interval across start..end. Batched commits keep appender overhead low; old
// samples compact into blocks so memory does not grow with data volume.
func seedYearJobCounters(t *testing.T, ctx context.Context, seriesCount int, start, end time.Time) {
	t.Helper()
	series := make([]labels.Labels, seriesCount)
	for i := 0; i < seriesCount; i++ {
		series[i] = labels.FromStrings(labels.MetricName, "stx_job_sink_write_rows_total", "job_id", fmt.Sprint(i))
	}
	stepMs := int64(yearBenchSeriesScrapeInterval / time.Millisecond)
	// 每批覆盖约 21 小时（5000 点 × 15s）。/ Each batch covers ~21h (5000 points × 15s).
	const batchPoints = 5000
	totalPoints := int(end.Sub(start) / yearBenchSeriesScrapeInterval)
	for base := 0; base < totalPoints; base += batchPoints {
		app := Appender(ctx)
		require.NotNil(t, app)
		points := batchPoints
		if base+points > totalPoints {
			points = totalPoints - base
		}
		for i, lset := range series {
			baseMs := start.Add(time.Duration(base) * yearBenchSeriesScrapeInterval).UnixMilli()
			for p := 0; p < points; p++ {
				// 计数器取累计值：job i 在 base+p 点写入 i*1e9 + base+p。
				// Cumulative counter: job i reports i*1e9 + base + p at point base+p.
				_, err := app.Append(0, lset, baseMs+int64(p)*stepMs, float64(i)*1e9+float64(base+p))
				require.NoError(t, err)
			}
		}
		require.NoError(t, app.Commit())
	}
}
