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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSelfScrapeOnce_NotReady(t *testing.T) {
	_ = Close()
	err := SelfScrapeOnce(context.Background())
	require.ErrorIs(t, err, errSelfNotReady)
	_, ok := GetStorageStatus()
	require.False(t, ok)
}

func TestSelfScrapeOnce_WritesAC1Metrics(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	require.NoError(t, SelfScrapeOnce(ctx))
	// 第一轮 Gather 时 head 还是空的，head_series 样本为 0；先查一次再抓，第二轮才带上真实序列数和 engine 耗时。
	// The first gather still sees an empty head so head_series is 0; query once then scrape again
	// so the second round carries the real series count and engine timings.
	now := time.Now().Add(5 * time.Millisecond)
	_ = instantVector(t, ctx, `up{job="stx_embedded"}`, now)
	require.NoError(t, SelfScrapeOnce(ctx))
	now = time.Now().Add(5 * time.Millisecond)

	// AC1：tsdb / engine / Go 指标能按 job=stx_embedded 查到。
	// AC1: tsdb / engine / Go metrics resolve with job=stx_embedded.
	head := instantVector(t, ctx, `prometheus_tsdb_head_series{job="stx_embedded"}`, now)
	require.NotEmpty(t, head)
	require.Greater(t, head[0].F, 0.0)
	require.Equal(t, InstanceEmbedded, head[0].Metric.Get("instance"))

	appended := instantVector(t, ctx, `prometheus_tsdb_head_samples_appended_total{job="stx_embedded"}`, now)
	require.NotEmpty(t, appended)

	failed := instantVector(t, ctx, `prometheus_tsdb_compactions_failed_total{job="stx_embedded"}`, now)
	require.NotEmpty(t, failed)
	require.Equal(t, 0.0, failed[0].F)

	heap := instantVector(t, ctx, `go_memstats_heap_inuse_bytes{job="stx_embedded"}`, now)
	require.NotEmpty(t, heap)
	require.Greater(t, heap[0].F, 0.0)

	up := instantVector(t, ctx, `up{job="stx_embedded"}`, now)
	require.Len(t, up, 1)
	require.Equal(t, 1.0, up[0].F)

	engineCount := instantVector(t, ctx, `prometheus_engine_query_duration_seconds_count{job="stx_embedded"}`, now)
	require.NotEmpty(t, engineCount)
	require.Greater(t, engineCount[0].F, 0.0)
}

func TestStorageStatus_AfterAppend(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, Start(ctx, testOptions(t)))
	t.Cleanup(func() { _ = Close() })

	status, ok := GetStorageStatus()
	require.True(t, ok)
	require.Equal(t, 0.0, status.CompactionsFailed)

	oldMs := time.Now().Add(-time.Hour).UnixMilli()
	appendSamples(t, ctx, "stx_embedts_metric", map[string]string{"job": "bench"}, oldMs, 1000, []float64{1, 2})
	require.NoError(t, SelfScrapeOnce(ctx))

	status, ok = GetStorageStatus()
	require.True(t, ok)
	require.Greater(t, status.HeadSeries, uint64(0))
	require.True(t, status.HasOldest)
	require.Equal(t, oldMs, status.OldestSampleMs)
	require.GreaterOrEqual(t, status.HeadBytes, int64(0))
}
