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
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import type {SyncJobLogsResult} from '@/lib/services/sync';
import {useJobLogs} from '../use-job-logs';

// 只 mock sync.getJobLogs，其余服务不进本测试。/ Only sync.getJobLogs is mocked; other services stay out.
const getJobLogs = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({default: {sync: {getJobLogs}}}));

const makeResult = (
  logs: string,
  nextOffset = '',
): SyncJobLogsResult => ({
  mode: 'per-job',
  source: 'file',
  logs,
  next_offset: nextOffset,
  updated_at: '2025-01-01T00:00:00Z',
});

/** 可控的异步结果（模拟慢请求）。/ Controllable async result (simulates a slow request). */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return {promise, resolve, reject};
}

const baseProps = {jobId: 1, enabled: true, keyword: '', level: 'all'} as const;

describe('useJobLogs', () => {
  beforeEach(() => {
    getJobLogs.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('增量拼接：带 offset 的后续块追加到已显示日志之后', async () => {
    getJobLogs
      .mockResolvedValueOnce(makeResult('first chunk', 'o1'))
      .mockResolvedValueOnce(makeResult('second chunk', 'o2'));
    const {result} = renderHook(() => useJobLogs(baseProps));

    await waitFor(() => expect(result.current.result?.logs).toBe('first chunk'));
    expect(result.current.offset).toBe('o1');

    await act(async () => {
      result.current.reload();
    });
    await waitFor(() =>
      expect(result.current.result?.logs).toBe('first chunk\nsecond chunk'),
    );
    expect(result.current.offset).toBe('o2');
    // 第二次请求携带上一次返回的游标。/ The second request carries the cursor from the first response.
    expect(getJobLogs).toHaveBeenNthCalledWith(
      2,
      1,
      expect.objectContaining({offset: 'o1'}),
    );
  });

  it('错误重置：保留已显示日志，offset 归零，下一次从头拉取', async () => {
    getJobLogs.mockResolvedValueOnce(makeResult('kept logs', 'o1'));
    const {result} = renderHook(() => useJobLogs(baseProps));
    await waitFor(() => expect(result.current.result?.logs).toBe('kept logs'));

    getJobLogs.mockRejectedValueOnce(new Error('boom'));
    await act(async () => {
      result.current.reload();
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    // 已显示的日志保留（避免闪空），但游标归零。/ On-screen logs survive (no flash) while the cursor resets.
    expect(result.current.result?.logs).toBe('kept logs');
    expect(result.current.offset).toBe('');

    getJobLogs.mockResolvedValueOnce(makeResult('fresh start', 'o9'));
    await act(async () => {
      result.current.reload();
    });
    await waitFor(() =>
      expect(result.current.result?.logs).toBe('fresh start'),
    );
    expect(getJobLogs).toHaveBeenLastCalledWith(
      1,
      expect.objectContaining({offset: undefined}),
    );
  });

  it('jobId 切换时 reset：清空旧结果，新作业从空游标开始', async () => {
    getJobLogs.mockImplementation((id: number) =>
      Promise.resolve(makeResult(`logs-of-${id}`, `offset-${id}`)),
    );
    const {result, rerender} = renderHook(
      ({jobId}) => useJobLogs({...baseProps, jobId}),
      {initialProps: {jobId: 1}},
    );
    await waitFor(() =>
      expect(result.current.result?.logs).toBe('logs-of-1'),
    );

    rerender({jobId: 2});
    // 复位立即生效：不再展示上一个作业的日志。/ Reset applies immediately: the previous job's logs are gone.
    expect(result.current.result).toBeNull();

    await waitFor(() =>
      expect(result.current.result?.logs).toBe('logs-of-2'),
    );
    expect(getJobLogs).toHaveBeenLastCalledWith(
      2,
      expect.objectContaining({offset: undefined}),
    );
    expect(result.current.offset).toBe('offset-2');
  });

  it('abort / 版本防串：慢响应在切换 jobId 后才返回时被丢弃', async () => {
    const slow = deferred<SyncJobLogsResult>();
    getJobLogs.mockImplementation((id: number) =>
      id === 1 ? slow.promise : Promise.resolve(makeResult('job-2 logs', 'o2')),
    );
    const {result, rerender} = renderHook(
      ({jobId}) => useJobLogs({...baseProps, jobId}),
      {initialProps: {jobId: 1}},
    );
    // 作业 1 的请求在途时切走。/ Switch away while job 1's request is still in flight.
    rerender({jobId: 2});
    await waitFor(() =>
      expect(result.current.result?.logs).toBe('job-2 logs'),
    );

    // 作业 1 的迟到响应不能回写。/ The late response for job 1 must not be written back.
    await act(async () => {
      slow.resolve(makeResult('stale-job-1-logs'));
    });
    expect(result.current.result?.logs).toBe('job-2 logs');
    expect(result.current.offset).toBe('o2');
  });

  it('enabled=false 时不发请求', async () => {
    renderHook(() => useJobLogs({...baseProps, enabled: false}));
    await act(async () => {
      await Promise.resolve();
    });
    expect(getJobLogs).not.toHaveBeenCalled();
  });

  it('enabled 期间每 3s 轮询增量拉取', async () => {
    vi.useFakeTimers();
    getJobLogs.mockResolvedValue(makeResult('tick', 'o1'));
    renderHook(() => useJobLogs(baseProps));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10);
    });
    expect(getJobLogs).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });
    expect(getJobLogs).toHaveBeenCalledTimes(2);
  });
});
