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

import {renderHook, waitFor} from '@testing-library/react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import type {SyncJobInstance, SyncJobListData} from '@/lib/services/sync';
import {
  buildJobListParams,
  resolveJobListSort,
  useJobList,
} from '../use-job-list';
import {DEFAULT_JOB_LIST_QUERY, type JobListQuery} from '../types';

// usePolledResource 只在页面可见时加载；jsdom 里显式钉成 visible。
// usePolledResource only loads while the page is visible; pin it in jsdom.
Object.defineProperty(document, 'visibilityState', {
  value: 'visible',
  configurable: true,
});

const sync = vi.hoisted(() => ({
  listJobs: vi.fn(),
  getJob: vi.fn(),
}));
vi.mock('@/lib/services', () => ({default: {sync}}));

const job = (
  id: number,
  status: SyncJobInstance['status'],
  extra: Partial<SyncJobInstance> = {},
): SyncJobInstance => ({
  id,
  task_id: 10 + id,
  task_version: 1,
  run_type: 'run',
  platform_job_id: String(id),
  engine_job_id: `e${id}`,
  status,
  submit_spec: {job_name: `job-${id}`},
  result_preview: {},
  error_message: '',
  created_by: 1,
  created_at: '2026-11-14T10:00:00Z',
  updated_at: '2026-11-14T10:00:00Z',
  started_at: '2026-11-14T10:00:00Z',
  cluster_id: 1,
  mode: 'streaming',
  ...extra,
});

const listData = (items: SyncJobInstance[]): SyncJobListData => ({
  total: items.length,
  items,
  status_counts: {},
});

const query = (extra: Partial<JobListQuery> = {}): JobListQuery => ({
  ...DEFAULT_JOB_LIST_QUERY,
  ...extra,
});

describe('resolveJobListSort / buildJobListParams', () => {
  it('falls back to started_at for rate / cdc_delay sorts outside the running group', () => {
    expect(resolveJobListSort({sort: 'rate', statusGroup: ''})).toBe(
      'started_at',
    );
    expect(resolveJobListSort({sort: 'cdc_delay', statusGroup: 'failed'})).toBe(
      'started_at',
    );
    expect(resolveJobListSort({sort: 'rate', statusGroup: 'running'})).toBe(
      'rate',
    );
    expect(
      resolveJobListSort({sort: 'cdc_delay', statusGroup: 'running'}),
    ).toBe('cdc_delay');
  });

  it('sends no run-type params for either scope: preview and formal runs are merged', () => {
    const task = buildJobListParams({kind: 'task', taskId: 9}, query(), 20);
    expect(task.task_id).toBe(9);
    expect(task.exclude_run_type).toBeUndefined();
    expect(task.run_type).toBeUndefined();
    const global = buildJobListParams({kind: 'all'}, query(), 20);
    expect(global.exclude_run_type).toBeUndefined();
    expect(global.run_type).toBeUndefined();
    expect(global.task_id).toBeUndefined();
  });
});

describe('useJobList', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sync.listJobs.mockResolvedValue(listData([job(1, 'running')]));
    sync.getJob.mockResolvedValue(job(99, 'failed'));
  });

  it('merges overlay copies only when they are newer, preserving live / task_name', async () => {
    const base = job(1, 'running', {
      updated_at: '2026-11-14T10:00:00Z',
      task_name: 'orders',
      live: {write_qps: 7},
    });
    sync.listJobs.mockResolvedValue(listData([base]));
    const {result, rerender} = renderHook(
      ({overlay}: {overlay: SyncJobInstance[]}) =>
        useJobList({
          scope: {kind: 'all'},
          query: query(),
          pageSize: 50,
          pollMs: 0,
          overlay,
        }),
      {initialProps: {overlay: [] as SyncJobInstance[]}},
    );
    await waitFor(() => expect(result.current.items).toHaveLength(1));

    // 更新的 overlay 覆盖行数据，但 live / task_name 保留列表行的版本（overlay 不带 with_live）。
    // A newer overlay wins the row, but live / task_name stay from the list row (overlays lack with_live).
    rerender({
      overlay: [
        job(1, 'failed', {
          updated_at: '2026-11-14T10:05:00Z',
          task_name: undefined,
          live: undefined,
        }),
      ],
    });
    expect(result.current.items[0].status).toBe('failed');
    expect(result.current.items[0].live).toEqual({write_qps: 7});
    expect(result.current.items[0].task_name).toBe('orders');

    // 更旧的 overlay 不覆盖。/ An older overlay does not replace the row.
    rerender({
      overlay: [
        job(1, 'canceled', {updated_at: '2026-11-14T09:00:00Z'}),
      ],
    });
    expect(result.current.items[0].status).toBe('running');
  });

  it('prepends the pinned job on page 1 without filters and keeps total unchanged', async () => {
    const {result} = renderHook(() =>
      useJobList({
        scope: {kind: 'all'},
        query: query(),
        pageSize: 50,
        pollMs: 0,
        pinnedJobId: 99,
      }),
    );
    await waitFor(() =>
      expect(result.current.items.map((item) => item.id)).toEqual([99, 1]),
    );
    expect(sync.getJob).toHaveBeenCalledWith(99);
    expect(result.current.total).toBe(1);
  });

  it('skips the pinned fetch when the job is already on the page or filters are set', async () => {
    sync.listJobs.mockResolvedValue(listData([job(99, 'failed')]));
    const {result, unmount} = renderHook(() =>
      useJobList({
        scope: {kind: 'all'},
        query: query(),
        pageSize: 50,
        pollMs: 0,
        pinnedJobId: 99,
      }),
    );
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    expect(sync.getJob).not.toHaveBeenCalled();
    unmount();

    sync.listJobs.mockResolvedValue(listData([job(1, 'failed')]));
    renderHook(() =>
      useJobList({
        scope: {kind: 'all'},
        query: query({statusGroup: 'failed'}),
        pageSize: 50,
        pollMs: 0,
        pinnedJobId: 99,
      }),
    );
    await waitFor(() => expect(sync.listJobs).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(sync.getJob).not.toHaveBeenCalled();
  });

  it('reloads when refreshKey changes', async () => {
    const {result, rerender} = renderHook(
      ({refreshKey}: {refreshKey: number}) =>
        useJobList({
          scope: {kind: 'all'},
          query: query(),
          pageSize: 50,
          pollMs: 0,
          refreshKey,
        }),
      {initialProps: {refreshKey: 1}},
    );
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    expect(sync.listJobs).toHaveBeenCalledTimes(1);
    rerender({refreshKey: 2});
    await waitFor(() => expect(sync.listJobs).toHaveBeenCalledTimes(2));
    // 同样的 key 重复渲染不再触发。/ The same key does not retrigger.
    rerender({refreshKey: 2});
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(sync.listJobs).toHaveBeenCalledTimes(2);
  });
});
