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
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// fakeClock 是可手动推进的时钟。/ fakeClock is a manually advanced clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestPromCache_CoalescesConcurrentIdenticalQueries(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	cache := newPromCache(clock.Now)
	var calls atomic.Int32
	release := make(chan struct{})
	query := cache.wrapInstant(func(ctx context.Context, base, q string, at time.Time) ([]promquery.Sample, error) {
		calls.Add(1)
		<-release
		return []promquery.Sample{{Value: 1}, {Value: 3}, {Value: 2}}, nil
	})

	at := clock.Now()
	var wg sync.WaitGroup
	results := make([][]promquery.Sample, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := query(context.Background(), "embedded", "up", at)
			require.NoError(t, err)
			// 调用方就地排序不能影响其他人拿到的结果。
			// In-place sorting by one caller must not affect what others receive.
			sort.Slice(out, func(a, b int) bool { return out[a].Value > out[b].Value })
			results[i] = out
		}(i)
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	require.Equal(t, int32(1), calls.Load())
	for _, out := range results {
		require.Equal(t, []float64{3, 2, 1}, []float64{out[0].Value, out[1].Value, out[2].Value})
	}

	// TTL 内再次请求走缓存；过期后重新查询。
	// Within the TTL the cache answers; after expiry the query runs again.
	_, _ = query(context.Background(), "embedded", "up", at)
	require.Equal(t, int32(1), calls.Load())
	clock.Advance(promCacheTTL)
	_, _ = query(context.Background(), "embedded", "up", at)
	require.Equal(t, int32(2), calls.Load())
}

func TestPromCache_ErrorsUseShorterTTLAndCancellationIsNotCached(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	cache := newPromCache(clock.Now)
	var calls atomic.Int32
	// 查询按顺序调用，用普通变量控制返回的错误。/ Queries run sequentially, so a plain variable drives the error.
	fail := errors.New("boom")
	query := cache.wrapRange(func(ctx context.Context, base, q string, start, end time.Time, step int) ([]promquery.Series, error) {
		calls.Add(1)
		if fail != nil {
			return nil, fail
		}
		return []promquery.Series{{}}, nil
	})
	start, end := clock.Now().Add(-time.Hour), clock.Now()

	_, err := query(context.Background(), "embedded", "q", start, end, 60)
	require.Error(t, err)
	_, err = query(context.Background(), "embedded", "q", start, end, 60)
	require.Error(t, err)
	require.Equal(t, int32(1), calls.Load())

	clock.Advance(promCacheErrTTL)
	_, err = query(context.Background(), "embedded", "q", start, end, 60)
	require.Error(t, err)
	require.Equal(t, int32(2), calls.Load())

	// 调用方取消导致的错误不进缓存。
	// Errors caused by caller cancellation are not cached.
	fail = context.Canceled
	_, _ = query(context.Background(), "embedded", "q2", start, end, 60)
	fail = nil
	out, err := query(context.Background(), "embedded", "q2", start, end, 60)
	require.NoError(t, err)
	require.Len(t, out, 1)
}

func TestAlignPresetNow(t *testing.T) {
	now := time.Date(2026, 11, 16, 10, 0, 22, 0, time.UTC)
	require.Equal(t, time.Date(2026, 11, 16, 10, 0, 15, 0, time.UTC), alignPresetNow(now, nil))
	custom := &promquery.Window{}
	require.Equal(t, now, alignPresetNow(now, custom))
}
