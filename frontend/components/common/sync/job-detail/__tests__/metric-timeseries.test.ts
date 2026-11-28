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

import {act, renderHook, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {SyncService, type SyncJobInstance, type SyncJobTimeseries} from '@/lib/services/sync';
import {clearJobTimeseriesCache, useWriteRateSparkline} from '../metric-timeseries';

vi.mock('@/lib/services/sync', async () => {
  const actual = await vi.importActual<typeof import('@/lib/services/sync')>('@/lib/services/sync');
  return {
    ...actual,
    SyncService: {
      ...actual.SyncService,
      getJobTimeseries: vi.fn(),
    },
  };
});

const runningJob = (id: number): SyncJobInstance =>
  ({id, status: 'running', updated_at: '2026-01-01T00:00:00Z'}) as SyncJobInstance;

const series = (base: number): SyncJobTimeseries =>
  ({
    series: [
      {
        key: 'throughput_write',
        points: Array.from({length: 12}, (_, index) => [index * 1000, base + index]),
      },
    ],
  }) as unknown as SyncJobTimeseries;

describe('useWriteRateSparkline', () => {
  beforeEach(() => {
    clearJobTimeseriesCache();
    vi.mocked(SyncService.getJobTimeseries).mockReset();
  });

  // 切换作业后新请求未返回前不得显示上一个作业的曲线。
  // After a job switch the previous job's series must not show while the new request is pending.
  it('drops the previous job points while the next job is loading', async () => {
    let resolveSecond: (value: SyncJobTimeseries) => void = () => undefined;
    vi.mocked(SyncService.getJobTimeseries)
      .mockResolvedValueOnce(series(100))
      .mockReturnValueOnce(
        new Promise<SyncJobTimeseries>((resolve) => {
          resolveSecond = resolve;
        }),
      );
    const {result, rerender} = renderHook(({job}) => useWriteRateSparkline(job), {
      initialProps: {job: runningJob(1)},
    });
    await waitFor(() => expect(result.current?.[0]?.[1]).toBe(100));

    rerender({job: runningJob(2)});
    expect(result.current).toBeNull();

    await act(async () => {
      resolveSecond(series(200));
    });
    await waitFor(() => expect(result.current?.[0]?.[1]).toBe(200));
  });

  it('switches to bytes_throughput when unit changes to bytes and accepts 2+ points', async () => {
    const rowSeries = {
      series: [{key: 'throughput_write', points: [[1000, 12], [2000, 18]]}],
    } as unknown as SyncJobTimeseries;
    const byteSeries = {
      series: [{key: 'bytes_throughput_write', points: [[1000, 2048], [2000, 4096]]}],
    } as unknown as SyncJobTimeseries;
    let resolveBytes: (value: SyncJobTimeseries) => void = () => undefined;
    vi.mocked(SyncService.getJobTimeseries)
      .mockResolvedValueOnce(rowSeries)
      .mockReturnValueOnce(
        new Promise<SyncJobTimeseries>((resolve) => {
          resolveBytes = resolve;
        }),
      );
    const {result, rerender} = renderHook(
      ({job, unit}: {job: SyncJobInstance; unit: 'rows' | 'bytes'}) =>
        useWriteRateSparkline(job, unit),
      {initialProps: {job: runningJob(7), unit: 'rows'}},
    );
    await waitFor(() => expect(result.current?.[1]?.[1]).toBe(18));

    rerender({job: runningJob(7), unit: 'bytes'});
    expect(result.current).toBeNull();
    expect(SyncService.getJobTimeseries).toHaveBeenLastCalledWith(7, {
      range: '1h',
      metrics: 'bytes_throughput',
    });

    await act(async () => {
      resolveBytes(byteSeries);
    });
    await waitFor(() => expect(result.current?.[1]?.[1]).toBe(4096));
  });
});
