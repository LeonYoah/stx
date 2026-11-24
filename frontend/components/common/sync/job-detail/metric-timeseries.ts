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

'use client';

import {useEffect, useState} from 'react';
import {
  SyncService,
  type SyncJobInstance,
  type SyncJobTimeseries,
} from '@/lib/services/sync';
import {getRealtimeEntry} from '../sync-studio-utils';
import type {TrendPoint} from './metrics-trend-data';

/** 快照 sparkline 与趋势视图共用的短 TTL 缓存：切视图回来不重复打时序接口，又不至于久到掩盖 30s 自动刷新的新数据。 */
// Short-TTL cache shared by the snapshot sparkline and the trend view: switching views reuses the
// in-flight/last result, yet stays short enough not to hide new data between 30s auto refreshes.
const TIMESERIES_CACHE_TTL_MS = 20_000;
const TIMESERIES_CACHE_MAX = 24;

type TimeseriesQuery = {
  range?: string;
  start?: number;
  end?: number;
  step?: number;
  metrics?: string;
};

const timeseriesCache = new Map<
  string,
  {at: number; promise: Promise<SyncJobTimeseries>}
>();

const timeseriesKey = (jobId: number, query: TimeseriesQuery) =>
  `${jobId}|${query.range ?? ''}|${query.start ?? ''}|${query.end ?? ''}|${query.step ?? ''}|${query.metrics ?? ''}`;

/**
 * 带 TTL 的 getJobTimeseries：相同查询在 TTL 内复用同一个 Promise（并发去重 + 视图切换免重取）。
 * 缓存的 Promise 不带 AbortSignal，调用方自行在 resolve 后丢弃过期结果；失败响应不缓存。
 * TTL wrapper over getJobTimeseries: identical queries share one promise (dedupe concurrent calls
 * and view switches). Cached promises are not abortable, so callers discard stale results after
 * resolve; failed responses are evicted.
 */
export function fetchJobTimeseriesCached(
  jobId: number,
  query: TimeseriesQuery,
): Promise<SyncJobTimeseries> {
  const key = timeseriesKey(jobId, query);
  const hit = timeseriesCache.get(key);
  if (hit && Date.now() - hit.at < TIMESERIES_CACHE_TTL_MS) {
    return hit.promise;
  }
  const promise = SyncService.getJobTimeseries(jobId, query);
  void promise.then(
    () => undefined,
    () => timeseriesCache.delete(key),
  );
  timeseriesCache.set(key, {at: Date.now(), promise});
  if (timeseriesCache.size > TIMESERIES_CACHE_MAX) {
    const oldest = timeseriesCache.keys().next().value;
    if (oldest !== undefined) {
      timeseriesCache.delete(oldest);
    }
  }
  return promise;
}

/** 测试专用：清空时序缓存。 / Test-only: clear the timeseries cache. */
export function clearJobTimeseriesCache() {
  timeseriesCache.clear();
}

const SPARKLINE_RANGE = '1h';
const SPARKLINE_MIN_POINTS = 10;

/**
 * 快照页「写入速率」旁 sparkline 的取数：仅运行中作业请求 timeseries，点不足 10 个返回 null。
 * 作业轮询换对象时重新检查，TTL 内直接命中缓存不会重复请求。
 * Timeseries points for the write-rate sparkline next to the snapshot KPI: only running jobs issue
 * the request, fewer than 10 points yields null. Re-checks on each polled job object, while the
 * cache TTL prevents duplicate requests.
 */
export function useWriteRateSparkline(
  job: SyncJobInstance | null,
): TrendPoint[] | null {
  const jobId = job?.id ?? null;
  const updatedAt = job?.updated_at ?? '';
  const running = Boolean(job) && getRealtimeEntry(job).running;
  const [points, setPoints] = useState<TrendPoint[] | null>(null);
  useEffect(() => {
    if (!jobId || !running) {
      setPoints(null);
      return;
    }
    let disposed = false;
    void fetchJobTimeseriesCached(jobId, {
      range: SPARKLINE_RANGE,
      metrics: 'throughput',
    })
      .then((timeseries) => {
        if (disposed) {
          return;
        }
        const write = (timeseries.series ?? []).find(
          (item) => item.key === 'throughput_write',
        );
        setPoints(write?.points?.length ? write.points : null);
      })
      .catch(() => {
        if (!disposed) {
          setPoints(null);
        }
      });
    return () => {
      disposed = true;
    };
  }, [jobId, running, updatedAt]);
  return points && points.length >= SPARKLINE_MIN_POINTS ? points : null;
}
