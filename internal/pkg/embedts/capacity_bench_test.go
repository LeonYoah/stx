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
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"
)

// capacityProfile 是一档序列规模的测量结果，写入 .trellis/spec/backend/embedded-tsdb-capacity.md。
// capacityProfile is one series-scale measurement, recorded in
// .trellis/spec/backend/embedded-tsdb-capacity.md.
type capacityProfile struct {
	Series         int
	AppendMs       float64
	CompactMs      float64
	Query1hMs      float64
	Query7dMs      float64
	HeapInuse      uint64
	RSS            int64
	HeadSeries     uint64
	HeadBytes      int64
	BlocksBytes    int64
	CompactionRan  bool
	Query1hMaxHeap uint64
	Query7dMaxHeap uint64
}

func TestCapacity_TenThousandSeries(t *testing.T) {
	report := runCapacityProfile(t, 10_000)
	logCapacityProfile(t, report)
	require.GreaterOrEqual(t, report.HeadSeries, uint64(10_000))
	require.Less(t, report.Query1hMs, 8000.0)
	require.Less(t, report.Query7dMs, 8000.0)
}

func TestCapacity_LargerProfiles(t *testing.T) {
	mode := strings.TrimSpace(os.Getenv("STX_TSDB_BENCH"))
	if mode == "" {
		t.Skip("set STX_TSDB_BENCH=1 (50k) or STX_TSDB_BENCH=full (50k+200k)")
	}
	sizes := []int{50_000}
	if mode == "full" {
		sizes = append(sizes, 200_000)
	}
	for _, n := range sizes {
		t.Run(fmt.Sprintf("%d", n), func(t *testing.T) {
			report := runCapacityProfile(t, n)
			logCapacityProfile(t, report)
			require.GreaterOrEqual(t, report.HeadSeries, uint64(n))
		})
	}
}

func runCapacityProfile(t *testing.T, series int) capacityProfile {
	t.Helper()
	ctx := context.Background()
	opts := testOptions(t)
	opts.MaxSamples = DefaultMaxSamples
	opts.QueryTimeout = 30 * time.Second
	require.NoError(t, Start(ctx, opts))
	t.Cleanup(func() { _ = Close() })

	runtime.GC()
	now := time.Now().UTC()
	// 时间戳贴着「现在」递增，避免出 appendable 窗口（过旧会 out of bounds）。
	// Timestamps step forward from now so they stay inside the appendable window
	// (too-old samples are rejected as out of bounds).
	stepMs := int64(15 * time.Second / time.Millisecond)
	points := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	startMs := now.UnixMilli()

	appendStart := time.Now()
	const batch = 500
	for start := 0; start < series; start += batch {
		end := start + batch
		if end > series {
			end = series
		}
		app := Appender(ctx)
		require.NotNil(t, app)
		for i := start; i < end; i++ {
			lset := labels.FromStrings(
				labels.MetricName, "stx_capacity_value",
				"job", "bench",
				"series", strconv.Itoa(i),
			)
			for p, v := range points {
				_, err := app.Append(0, lset, startMs+int64(p)*stepMs, v)
				require.NoError(t, err)
			}
		}
		require.NoError(t, app.Commit())
	}
	appendMs := time.Since(appendStart).Seconds() * 1000

	require.NoError(t, SelfScrapeOnce(ctx))
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	queryEnd := now.Add(2 * time.Minute)
	query1h, heap1h := rangeQueryMs(t, ctx, `sum(stx_capacity_value{job="bench"})`, queryEnd.Add(-time.Hour), queryEnd, 15*time.Second)
	query7d, heap7d := rangeQueryMs(t, ctx, `sum(stx_capacity_value{job="bench"})`, queryEnd.Add(-7*24*time.Hour), queryEnd, 30*time.Minute)

	mint := startMs
	maxt := startMs + int64(len(points)-1)*stepMs
	compactStart := time.Now()
	err := CompactHead(mint, maxt-1)
	compactMs := time.Since(compactStart).Seconds() * 1000
	compactionRan := err == nil
	if err != nil {
		t.Logf("CompactHead: %v", err)
	}

	status, ok := GetStorageStatus()
	require.True(t, ok)

	return capacityProfile{
		Series:         series,
		AppendMs:       appendMs,
		CompactMs:      compactMs,
		Query1hMs:      query1h,
		Query7dMs:      query7d,
		HeapInuse:      ms.HeapInuse,
		RSS:            processRSSBytes(),
		HeadSeries:     status.HeadSeries,
		HeadBytes:      status.HeadBytes,
		BlocksBytes:    status.BlocksBytes,
		CompactionRan:  compactionRan,
		Query1hMaxHeap: heap1h,
		Query7dMaxHeap: heap7d,
	}
}

func rangeQueryMs(t *testing.T, ctx context.Context, expr string, start, end time.Time, step time.Duration) (float64, uint64) {
	t.Helper()
	engine, queryable := QueryEngine()
	require.NotNil(t, engine)
	started := time.Now()
	qry, err := engine.NewRangeQuery(ctx, queryable, nil, expr, start, end, step)
	require.NoError(t, err)
	defer qry.Close()
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	_, ok := res.Value.(promql.Matrix)
	require.True(t, ok, "expected matrix, got %T", res.Value)
	elapsed := time.Since(started).Seconds() * 1000
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return elapsed, ms.HeapInuse
}

func logCapacityProfile(t *testing.T, p capacityProfile) {
	t.Helper()
	t.Logf("capacity series=%d append_ms=%.1f compact_ms=%.1f compact_ok=%v query_1h_ms=%.1f query_7d_ms=%.1f heap=%d rss=%d head_series=%d head_bytes=%d blocks_bytes=%d query_1h_heap=%d query_7d_heap=%d",
		p.Series, p.AppendMs, p.CompactMs, p.CompactionRan, p.Query1hMs, p.Query7dMs, p.HeapInuse, p.RSS, p.HeadSeries, p.HeadBytes, p.BlocksBytes, p.Query1hMaxHeap, p.Query7dMaxHeap)
}

func processRSSBytes() int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return kb * 1024
	}
	return 0
}
