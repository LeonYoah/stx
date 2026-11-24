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

// useJobDetail 契约（作业列表行瘦身后）：详情用摘要行先渲染，getJob 全量按字段级合并补齐；
// 摘要行绝不整行替换全量（最新 ≠ 最完整），updated_at 变新才重取；活动作业按 activePollMs 轮询。
// The useJobDetail contract (after list-row slimming): the detail renders the summary row first and the
// getJob full record merges field-wise on top; a summary row never wholesale-replaces the full record
// (freshest ≠ most complete) and only a newer updated_at refetches; active jobs poll at activePollMs.

import {act, renderHook, waitFor} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import type {
  SyncJobInstance,
  SyncJobSummary,
} from '@/lib/services/sync';
import {useJobDetail} from '../use-job-detail';

// 只 mock sync.getJob，其余服务不进本测试。/ Only sync.getJob is mocked; other services stay out.
const getJob = vi.hoisted(() => vi.fn());
vi.mock('@/lib/services', () => ({default: {sync: {getJob}}}));

const T0 = '2026-01-01T00:00:00Z';
const T1 = '2026-01-01T00:00:01Z';
const T2 = '2026-01-01T00:00:02Z';

/** /sync/jobs 摘要行：result_preview 只留白名单键、submit_spec 无 submitted_content。/ A /sync/jobs summary row: allow-listed result_preview keys only, no submitted_content. */
function summary(partial: Partial<SyncJobSummary>): SyncJobSummary {
  return {
    id: 1,
    task_id: 11,
    run_type: 'run',
    status: 'running',
    submit_spec: {job_name: 'job-1'},
    result_preview: {job_status: 'RUNNING'},
    error_message: '',
    created_by: 1,
    created_at: T0,
    updated_at: T0,
    ...partial,
  } as SyncJobSummary;
}

/** getJob 完整记录：带 job_dag / metrics / submitted_content 重字段。/ A getJob full record with job_dag / metrics / submitted_content heavy fields. */
function full(partial: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    ...summary({}),
    submit_spec: {job_name: 'job-1', submitted_content: 'env { parallel = 1 }'},
    result_preview: {
      job_status: 'RUNNING',
      job_dag: {vertices: [{id: 'v1'}]},
      metrics: {SourceReceivedCount: 10},
    },
    ...partial,
  } as SyncJobInstance;
}

const baseProps = {enabled: true, activePollMs: 0} as const;

describe('useJobDetail', () => {
  beforeEach(() => {
    getJob.mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('全量未落地前先用摘要行渲染，落地后补齐重字段', async () => {
    let resolveGet!: (job: SyncJobInstance) => void;
    getJob.mockReturnValueOnce(
      new Promise<SyncJobInstance>((resolve) => {
        resolveGet = resolve;
      }),
    );
    const {result, rerender} = renderHook(
      ({job}) => useJobDetail({...baseProps, job}),
      {initialProps: {job: summary({status: 'running'})}},
    );

    // 全量未到：返回的是摘要视图，状态可见、重字段为空。/ Before the full record lands the summary
    // view is returned — status visible, heavy fields empty.
    expect(result.current?.status).toBe('running');
    expect(result.current?.result_preview?.job_dag).toBeUndefined();

    await act(async () => {
      resolveGet(full({}));
    });
    rerender({job: summary({status: 'running'})});
    await waitFor(() =>
      expect(result.current?.result_preview?.job_dag).toEqual({
        vertices: [{id: 'v1'}],
      }),
    );
    expect(result.current?.submit_spec?.submitted_content).toBe(
      'env { parallel = 1 }',
    );
  });

  it('摘要行更新（同 updated_at 的新对象）不整行替换：全量重字段保留、轻字段刷新', async () => {
    getJob.mockResolvedValueOnce(full({updated_at: T1}));
    const {result, rerender} = renderHook(
      ({job}) => useJobDetail({...baseProps, job}),
      {initialProps: {job: summary({updated_at: T0})}},
    );
    await waitFor(() =>
      expect(result.current?.result_preview?.job_dag).toBeTruthy(),
    );

    // 列表轮询给了一个新对象（同 updated_at）：轻字段取摘要值，重字段不能被清空。
    // The list poll hands a fresh object (same updated_at): light fields take the summary value while
    // heavy fields must not be wiped.
    rerender({
      job: summary({
        status: 'canceled',
        updated_at: T1,
        live: {read_total: 42},
      }),
    });
    await waitFor(() => expect(result.current?.status).toBe('canceled'));
    expect(result.current?.result_preview?.job_dag).toEqual({
      vertices: [{id: 'v1'}],
    });
    expect(result.current?.result_preview?.metrics).toEqual({
      SourceReceivedCount: 10,
    });
    expect(result.current?.submit_spec?.submitted_content).toBe(
      'env { parallel = 1 }',
    );
    expect(result.current?.live).toEqual({read_total: 42});
    // 同 updated_at 不触发重取。/ Same updated_at must not refetch.
    expect(getJob).toHaveBeenCalledTimes(1);
  });

  it('摘要行 updated_at 新于已持有全量时重新 getJob', async () => {
    getJob
      .mockResolvedValueOnce(full({updated_at: T1}))
      .mockResolvedValue(
        full({updated_at: T2, status: 'success', result_preview: {
          job_status: 'FINISHED',
          job_dag: {vertices: [{id: 'v2'}]},
          metrics: {SourceReceivedCount: 99},
        }}),
      );
    const {result, rerender} = renderHook(
      ({job}) => useJobDetail({...baseProps, job}),
      {initialProps: {job: summary({updated_at: T0})}},
    );
    await waitFor(() => expect(result.current?.updated_at).toBe(T1));

    // 终态切换：列表行 updated_at 变新，全量需要重取以免拓扑 / 指标停在旧值。
    // Terminal switch: the list row's updated_at advances, so the full record must be refetched to
    // keep topology / metrics from sticking at stale values.
    rerender({
      job: summary({
        status: 'success',
        updated_at: T2,
        result_preview: {job_status: 'FINISHED'},
      }),
    });
    // 除重取外，终态的 active 翻转还会做最后一次取数——只断言「确实重取了」与字段已更新。
    // Beyond the refetch, the terminal active flip issues one last fetch — only assert the refetch
    // happened and the fields updated.
    await waitFor(() =>
      expect(getJob.mock.calls.length).toBeGreaterThanOrEqual(2),
    );
    await waitFor(() =>
      expect(result.current?.result_preview?.metrics).toEqual({
        SourceReceivedCount: 99,
      }),
    );
  });

  it('宿主喂进来的完整记录直接采纳：仍视为全量，不重复拉取', async () => {
    getJob.mockResolvedValueOnce(full({}));
    const {result} = renderHook(
      ({job}) => useJobDetail({...baseProps, job}),
      {initialProps: {job: full({updated_at: T1})}},
    );
    // 打开时的一次 getJob 之外，全量行不再触发补充拉取。
    // Beyond the initial getJob on open, a full row triggers no extra fetch.
    await waitFor(() => expect(getJob).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(result.current?.submit_spec?.submitted_content).toBe(
        'env { parallel = 1 }',
      ),
    );
  });

  it('活动作业按 activePollMs 轮询 getJob，转终态后停止', async () => {
    vi.useFakeTimers();
    getJob
      .mockResolvedValueOnce(full({updated_at: T1}))
      .mockResolvedValueOnce(full({updated_at: T1}))
      .mockResolvedValueOnce(
        full({
          updated_at: T2,
          status: 'success',
          result_preview: {job_status: 'FINISHED'},
        }),
      )
      .mockResolvedValue(
        full({
          updated_at: T2,
          status: 'success',
          result_preview: {job_status: 'FINISHED'},
        }),
      );
    renderHook(
      ({job}) => useJobDetail({enabled: true, activePollMs: 5000, job}),
      {initialProps: {job: summary({})}},
    );

    // 打开即取一次。/ One fetch on open.
    await act(async () => {
      await Promise.resolve();
    });
    expect(getJob).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(5000);
      await Promise.resolve();
    });
    expect(getJob).toHaveBeenCalledTimes(2);

    // 第二次轮询得到终态：active 翻转会做最后一次取数，此后定时器停止、不再请求。
    // The second poll returns a terminal record: the active flip issues one last fetch, then the
    // timer stops and no further requests fire.
    await act(async () => {
      vi.advanceTimersByTime(5000);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(getJob).toHaveBeenCalledTimes(4);
    await act(async () => {
      vi.advanceTimersByTime(20000);
      await Promise.resolve();
    });
    expect(getJob).toHaveBeenCalledTimes(4);
  });

  it('enabled=false 时不发请求', async () => {
    renderHook(
      ({job}) => useJobDetail({enabled: false, activePollMs: 5000, job}),
      {initialProps: {job: summary({})}},
    );
    await Promise.resolve();
    expect(getJob).not.toHaveBeenCalled();
  });
});
