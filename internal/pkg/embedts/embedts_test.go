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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/require"

	"github.com/LeonYoah/stx/internal/config"
)

// testOptions 返回指向 t.TempDir() 的启动参数，测试间目录互不干扰。
// testOptions returns startup Options rooted at t.TempDir() so tests never share a data dir.
func testOptions(t *testing.T) Options {
	t.Helper()
	return Options{
		Dir:               t.TempDir(),
		RetentionDuration: 24 * time.Hour,
		QueryTimeout:      5 * time.Second,
		MaxSamples:        1000,
	}
}

// appendSamples 向当前实例写入 metric 的若干样本并提交。
// appendSamples writes a batch of samples for metric into the live instance and commits them.
func appendSamples(t *testing.T, ctx context.Context, metric string, extra map[string]string, baseMs int64, stepMs int64, values []float64) {
	t.Helper()
	app := Appender(ctx)
	require.NotNil(t, app)
	pairs := make([]string, 0, 2+len(extra)*2)
	pairs = append(pairs, labels.MetricName, metric)
	for k, v := range extra {
		pairs = append(pairs, k, v)
	}
	lset := labels.FromStrings(pairs...)
	for i, v := range values {
		_, err := app.Append(0, lset, baseMs+int64(i)*stepMs, v)
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
}

func TestStart_AppendQueryCloseReopen(t *testing.T) {
	ctx := context.Background()
	opts := testOptions(t)

	require.NoError(t, Start(ctx, opts))
	// t.Cleanup 后进先出：本清理先于 testOptions 里 TempDir 的目录删除执行，db 先关后删目录。
	// t.Cleanup is LIFO: this cleanup runs before the TempDir removal registered inside
	// testOptions, so the db closes before its directory disappears.
	t.Cleanup(func() { _ = Close() })
	require.True(t, Ready())
	require.True(t, Available())

	engine, queryable := QueryEngine()
	require.NotNil(t, engine)
	require.NotNil(t, queryable)

	now := time.Now().UTC()
	baseMs := now.Add(-2 * time.Minute).UnixMilli()
	appendSamples(t, ctx, "stx_embedts_metric", map[string]string{"instance": "a"}, baseMs, 15*1000, []float64{1, 2, 3, 4, 5, 6})

	// 即时查询：应取到 lookback 内最后一个点。
	// Instant query: should resolve the last point inside the lookback window.
	qry, err := engine.NewInstantQuery(ctx, queryable, nil, "stx_embedts_metric", now)
	require.NoError(t, err)
	res := qry.Exec(ctx)
	require.NoError(t, res.Err)
	vec, ok := res.Value.(promql.Vector)
	require.True(t, ok, "expected vector result, got %T", res.Value)
	require.Len(t, vec, 1)
	require.Equal(t, 6.0, vec[0].F)
	require.Equal(t, "a", vec[0].Metric.Get("instance"))
	qry.Close()

	// 区间查询：窗口收紧到已写入的样本区间（PromQL 每步按 lookback 取最后样本，
	// 窗口外推会让尾部样本延续，与 HTTP query_range 语义一致）。
	// Range query: bound the window to the appended span (PromQL evaluates every step with
	// lookback, so the last sample would carry forward past its timestamp—same as HTTP query_range).
	rq, err := engine.NewRangeQuery(ctx, queryable, nil, "stx_embedts_metric",
		time.UnixMilli(baseMs), time.UnixMilli(baseMs+75*1000), 15*time.Second)
	require.NoError(t, err)
	rres := rq.Exec(ctx)
	require.NoError(t, rres.Err)
	matrix, ok := rres.Value.(promql.Matrix)
	require.True(t, ok, "expected matrix result, got %T", rres.Value)
	require.Len(t, matrix, 1)
	require.Len(t, matrix[0].Floats, 6)
	require.Equal(t, 1.0, matrix[0].Floats[0].F)
	require.Equal(t, 6.0, matrix[0].Floats[5].F)
	rq.Close()

	// 关闭后就绪探测回落、入口置空；再次 Close 幂等。
	// After close the probes drop and entry points go nil; a second Close is a no-op.
	require.NoError(t, Close())
	require.False(t, Ready())
	e, q := QueryEngine()
	require.Nil(t, e)
	require.Nil(t, q)
	require.Nil(t, Appender(ctx))
	require.NoError(t, Close())

	// 重开同一目录：已落盘的序列仍可查询，验证持久化。
	// Reopen the same dir: persisted series must still be queryable.
	require.NoError(t, Start(ctx, opts))
	engine2, queryable2 := QueryEngine()
	require.NotNil(t, engine2)
	qry2, err := engine2.NewInstantQuery(ctx, queryable2, nil, "stx_embedts_metric", now)
	require.NoError(t, err)
	res2 := qry2.Exec(ctx)
	require.NoError(t, res2.Err)
	vec2, ok := res2.Value.(promql.Vector)
	require.True(t, ok)
	require.Len(t, vec2, 1)
	require.Equal(t, 6.0, vec2[0].F)
	qry2.Close()
}

func TestStart_AlreadyStartedReturnsError(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	// 第二次 Start 应报错且不影响已就绪实例。
	// A second Start must error without disturbing the live instance.
	err := Start(ctx, testOptions(t))
	require.Error(t, err)
	require.True(t, Ready())
	engine, queryable := QueryEngine()
	require.NotNil(t, engine)
	require.NotNil(t, queryable)
}

func TestNotReadyEntryPoints(t *testing.T) {
	// 兜底关闭可能残留的实例；本用例只断言未就绪语义，不校验 Close 返回值。
	// Best-effort close of any leftover instance; this case only asserts not-ready semantics.
	_ = Close()
	ctx := context.Background()
	require.False(t, Ready())
	require.False(t, Available())
	require.Nil(t, Appender(ctx))
	engine, queryable := QueryEngine()
	require.Nil(t, engine)
	require.Nil(t, queryable)
}

func TestOptionsFromObservability(t *testing.T) {
	ctx := context.Background()
	t.Run("happy path", func(t *testing.T) {
		opts, err := OptionsFromObservability(ctx, config.ObservabilityConfig{
			Storage: config.ObservabilityStorageConfig{
				Dir:        "data/tsdb-x",
				Retention:  "7d",
				MaxSize:    1024,
				MaxSizeSet: true,
			},
			QueryTimeout: "3s",
			MaxSamples:   12345,
		})
		require.NoError(t, err)
		require.Equal(t, "data/tsdb-x", opts.Dir)
		require.Equal(t, 7*24*time.Hour, opts.RetentionDuration)
		require.Equal(t, int64(1024), opts.MaxBytes)
		require.Equal(t, MaxBytesSourceConfig, opts.MaxBytesSource)
		require.Equal(t, 3*time.Second, opts.QueryTimeout)
		require.Equal(t, 12345, opts.MaxSamples)
	})

	t.Run("explicit zero retention and zero max_size mean unlimited", func(t *testing.T) {
		opts, err := OptionsFromObservability(ctx, config.ObservabilityConfig{
			Storage: config.ObservabilityStorageConfig{
				Dir:        "data/tsdb-x",
				Retention:  "0",
				MaxSize:    0,
				MaxSizeSet: true,
			},
		})
		require.NoError(t, err)
		require.Zero(t, opts.RetentionDuration)
		require.Zero(t, opts.MaxBytes)
		require.Equal(t, MaxBytesSourceConfig, opts.MaxBytesSource)
	})

	t.Run("empty fields fall back to defaults", func(t *testing.T) {
		opts, err := OptionsFromObservability(ctx, config.ObservabilityConfig{
			Storage: config.ObservabilityStorageConfig{Dir: t.TempDir()},
		})
		require.NoError(t, err)
		require.NotEmpty(t, opts.Dir)
		// 空 retention 不再回填 31 天：0 表示不按时间删除。
		// An empty retention is no longer backfilled with 31d: 0 disables time-based deletion.
		require.Zero(t, opts.RetentionDuration)
		require.Equal(t, DefaultQueryTimeout, opts.QueryTimeout)
		require.Equal(t, DefaultMaxSamples, opts.MaxSamples)
		// 未配置 max_size：自动容量在上限与下限之间，来源为 auto/fallback 之一。
		// Unset max_size: the auto cap stays within its bounds, sourced from auto/fallback.
		require.GreaterOrEqual(t, opts.MaxBytes, minAutoMaxBytes)
		require.LessOrEqual(t, opts.MaxBytes, DefaultMaxBytesCap)
		require.Contains(t, []string{MaxBytesSourceAuto20, MaxBytesSourceAutoCap, MaxBytesSourceFallback}, opts.MaxBytesSource)
	})

	t.Run("bad retention rejected", func(t *testing.T) {
		_, err := OptionsFromObservability(ctx, config.ObservabilityConfig{
			Storage: config.ObservabilityStorageConfig{Retention: "abc"},
		})
		require.Error(t, err)
	})

	t.Run("bad query timeout rejected", func(t *testing.T) {
		_, err := OptionsFromObservability(ctx, config.ObservabilityConfig{QueryTimeout: "soon"})
		require.Error(t, err)
	})
}

func TestAutoMaxBytes(t *testing.T) {
	ctx := context.Background()
	stub := func(total int64) func(string) (int64, error) {
		return func(string) (int64, error) { return total, nil }
	}
	gib := int64(1) << 30
	cases := []struct {
		name       string
		total      int64
		wantBytes  int64
		wantSource string
	}{
		// 40GB 磁盘：20% = 8GiB。/ 40GB disk: 20% is 8GiB.
		{"40GB disk", 40 * gib, 8 * gib, MaxBytesSourceAuto20},
		// 1TB 磁盘：20% 超过 30GiB，取天花板。/ 1TB disk: 20% exceeds 30GiB, the cap wins.
		{"1TB disk", 1024 * gib, DefaultMaxBytesCap, MaxBytesSourceAutoCap},
		// 2GB 磁盘：20% 不足 1GiB，取下限。/ 2GB disk: 20% below 1GiB, the floor wins.
		{"2GB disk", 2 * gib, minAutoMaxBytes, MaxBytesSourceAuto20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, source := autoMaxBytes(ctx, t.TempDir(), stub(c.total))
			require.Equal(t, c.wantBytes, got)
			require.Equal(t, c.wantSource, source)
		})
	}
	t.Run("statfs failure falls back to 30GiB", func(t *testing.T) {
		got, source := autoMaxBytes(ctx, t.TempDir(), func(string) (int64, error) {
			return 0, errors.New("no statfs")
		})
		require.Equal(t, DefaultMaxBytesCap, got)
		require.Equal(t, MaxBytesSourceFallback, source)
	})
}

func TestResolveMaxBlockDuration(t *testing.T) {
	cases := []struct {
		name      string
		retention time.Duration
		override  time.Duration
		want      time.Duration
	}{
		// 无时间保留期：默认 7 天。/ No time retention: the 7d default.
		{"no retention", 0, 0, DefaultMaxBlockDuration},
		// retention=30d：收紧到 3d。/ retention=30d: tightened to 3d.
		{"30d retention", 30 * 24 * time.Hour, 0, 3 * 24 * time.Hour},
		// retention=1h：retention/10=6m 低于下限，取 2h。/ retention=1h: 6m below floor, clamped to 2h.
		{"1h retention", time.Hour, 0, minMaxBlockDuration},
		// 显式覆盖优先。/ An explicit override wins.
		{"override", 0, time.Hour, time.Hour},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveMaxBlockDuration(Options{RetentionDuration: c.retention, MaxBlockDuration: c.override})
			require.Equal(t, c.want, got)
		})
	}
}

func TestStart_ShortBlocksCompactWithinMax(t *testing.T) {
	// 短块合并：MinBlock=1m、MaxBlock=1h 时压实出的块都不超过 1h。
	// Short-block compaction: with MinBlock=1m and MaxBlock=1h no produced block exceeds 1h.
	ctx := context.Background()
	opts := testOptions(t)
	opts.MinBlockDuration = time.Minute
	opts.MaxBlockDuration = time.Hour
	require.NoError(t, Start(ctx, opts))
	t.Cleanup(func() { _ = Close() })

	now := time.Now().UTC()
	baseMs := now.Add(-10 * time.Minute).UnixMilli()
	appendSamples(t, ctx, "stx_embedts_compact", nil, baseMs, 15*1000, []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12})

	// Compact() 同步完成 head 截断与 level compaction。/ Compact() synchronously runs head
	// truncation and level compactions.
	require.NoError(t, current.db.Compact(ctx))
	blocks := current.db.Blocks()
	require.NotEmpty(t, blocks)
	for _, b := range blocks {
		require.LessOrEqual(t, b.Meta().MaxTime-b.Meta().MinTime, time.Hour.Milliseconds())
	}
}

func TestOldestSampleTime(t *testing.T) {
	// 未就绪返回 false。/ Not ready reports false.
	_ = Close()
	if _, ok := OldestSampleTime(); ok {
		t.Fatal("OldestSampleTime should be false before Start")
	}

	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	// 空库同样返回 false。/ An empty store also reports false.
	if _, ok := OldestSampleTime(); ok {
		t.Fatal("OldestSampleTime should be false with no samples")
	}

	// 写入两段样本后应取到最早时间戳。/ After appending, the earliest timestamp wins.
	oldMs := time.Now().Add(-time.Hour).UnixMilli()
	newMs := time.Now().Add(-time.Minute).UnixMilli()
	appendSamples(t, ctx, "stx_embedts_metric", nil, newMs, 1000, []float64{1})
	appendSamples(t, ctx, "stx_embedts_metric_old", nil, oldMs, 1000, []float64{1})
	got, ok := OldestSampleTime()
	require.True(t, ok)
	require.Equal(t, oldMs, got.UnixMilli())
}

func TestStart_ConfiguresConcurrentQueryCap(t *testing.T) {
	require.GreaterOrEqual(t, maxConcurrentQueries(), 4)

	dir := t.TempDir()
	require.NoError(t, Start(context.Background(), Options{Dir: dir}))
	t.Cleanup(func() { _ = Close() })
	// 并发上限由 ActiveQueryTracker 实现，跟踪文件落在 tsdb 目录。
	// The cap is enforced by ActiveQueryTracker, whose tracking file lives in the tsdb dir.
	_, err := os.Stat(filepath.Join(dir, "queries.active"))
	require.NoError(t, err)
}
