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
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

const (
	// promCacheTTL 是大盘 PromQL 结果的缓存时长；预设时间窗的终点按同样粒度取整，15 秒内同参数请求命中同一键。
	// promCacheTTL is how long dashboard PromQL results are cached; preset window ends are truncated
	// to the same granularity so identical requests within 15 seconds share one key.
	promCacheTTL = 15 * time.Second
	// promCacheErrTTL 是查询失败结果的缓存时长，短于成功结果，避免故障时放大重试又不长期卡住。
	// promCacheErrTTL caches failed queries briefly—shorter than successes—so outages are not
	// amplified by retries yet do not stick for long.
	promCacheErrTTL = 5 * time.Second
	// promCacheMaxEntries 是缓存键上限；超限时整体清空（键空间小，不值得 LRU）。
	// promCacheMaxEntries caps cached keys; the whole cache is dropped when exceeded (the key space
	// is small, so an LRU is not worth it).
	promCacheMaxEntries = 256
)

type rangeQueryFunc func(ctx context.Context, base, q string, start, end time.Time, step int) ([]promquery.Series, error)
type instantQueryFunc func(ctx context.Context, base, q string, at time.Time) ([]promquery.Sample, error)

// promCache 缓存首页大盘的 PromQL 结果并合并同键并发：内嵌后查询消耗 STX 自身 CPU，
// 多人同时打开首页时同参数的查询只算一次。只缓存时序结果；作业名、viewable 等逐用户字段在缓存外装饰。
// promCache caches home dashboard PromQL results and coalesces concurrent identical queries: with the
// embedded engine queries burn STX's own CPU, so identical queries from many open home pages run
// once. Only time-series results are cached; per-user fields such as job names and viewable are
// decorated outside the cache.
type promCache struct {
	now   func() time.Time
	group singleflight.Group

	mu      sync.Mutex
	entries map[string]promCacheEntry
}

type promCacheEntry struct {
	value   any
	err     error
	expires time.Time
}

// dashboardPromCache 是集群 / 作业大盘共用的进程级缓存。
// dashboardPromCache is the process-wide cache shared by the cluster and job dashboards.
var dashboardPromCache = newPromCache(time.Now)

func newPromCache(now func() time.Time) *promCache {
	return &promCache{now: now, entries: make(map[string]promCacheEntry)}
}

// get 返回未过期的缓存项。
// get returns an unexpired cache entry.
func (c *promCache) get(key string) (any, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !c.now().Before(entry.expires) {
		return nil, nil, false
	}
	return entry.value, entry.err, true
}

// put 写入缓存；调用方取消导致的错误不缓存，避免把一个请求的取消传染给别人。
// put stores a result; errors caused by caller cancellation are not cached so one request's
// cancellation does not leak to others.
func (c *promCache) put(key string, value any, err error) {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	ttl := promCacheTTL
	if err != nil {
		ttl = promCacheErrTTL
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= promCacheMaxEntries {
		c.entries = make(map[string]promCacheEntry)
	}
	c.entries[key] = promCacheEntry{value: value, err: err, expires: c.now().Add(ttl)}
}

// do 先查缓存，未命中时经 singleflight 只执行一次 fn。
// do checks the cache first and runs fn once through singleflight on a miss.
func (c *promCache) do(key string, fn func() (any, error)) (any, error) {
	if value, err, ok := c.get(key); ok {
		return value, err
	}
	value, err, _ := c.group.Do(key, func() (any, error) {
		if value, err, ok := c.get(key); ok {
			return value, err
		}
		value, err := fn()
		c.put(key, value, err)
		return value, err
	})
	return value, err
}

// wrapRange 给区间查询加缓存。返回切片的顶层是副本：调用方会就地排序，不能改到共享结果。
// wrapRange adds caching to range queries. The returned top-level slice is a copy because callers
// sort in place and must not reorder the shared result.
func (c *promCache) wrapRange(next rangeQueryFunc) rangeQueryFunc {
	return func(ctx context.Context, base, q string, start, end time.Time, step int) ([]promquery.Series, error) {
		key := fmt.Sprintf("range|%s|%d|%d|%d", q, start.UnixMilli(), end.UnixMilli(), step)
		value, err := c.do(key, func() (any, error) { return next(ctx, base, q, start, end, step) })
		series, _ := value.([]promquery.Series)
		return append([]promquery.Series(nil), series...), err
	}
}

// wrapInstant 给即时查询加缓存，同样返回顶层副本。
// wrapInstant adds caching to instant queries and likewise returns a top-level copy.
func (c *promCache) wrapInstant(next instantQueryFunc) instantQueryFunc {
	return func(ctx context.Context, base, q string, at time.Time) ([]promquery.Sample, error) {
		key := fmt.Sprintf("instant|%s|%d", q, at.UnixMilli())
		value, err := c.do(key, func() (any, error) { return next(ctx, base, q, at) })
		samples, _ := value.([]promquery.Sample)
		return append([]promquery.Sample(nil), samples...), err
	}
}

// alignPresetNow 把预设时间窗的「现在」按缓存粒度向下取整，让 15 秒内的同参数请求得到相同的窗口与缓存键；
// 自定义窗口原样使用。
// alignPresetNow truncates "now" for preset windows to the cache granularity so identical requests
// within 15 seconds resolve to the same window and cache keys; custom windows are used as is.
func alignPresetNow(now time.Time, custom *promquery.Window) time.Time {
	if custom != nil {
		return now
	}
	return now.Truncate(promCacheTTL)
}
