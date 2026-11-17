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

// knownJobs 解析契约（父 design D5）：jobs > 当前页 items > retained 快照；翻页后选中 / 详情作业仍可解析。
// retainedActiveJobIds：只有「不在 jobs 且仍活动」的 retained 作业才补进 3s 轮询，刷新只回写 retained。
// The knownJobs resolution contract (parent design D5): jobs > current page items > retained snapshots, so the
// selected / detail job keeps resolving after paging away. retainedActiveJobIds only schedules retained jobs
// that are missing from `jobs` yet still active; refreshes write back to retained only.

import {describe, expect, it} from 'vitest';
import type {SyncJobInstance} from '@/lib/services/sync';
import {mergeKnownJobs, retainedActiveJobIds} from '../known-jobs';

function job(partial: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    id: 1,
    task_id: 11,
    run_type: 'run',
    status: 'running',
    submit_spec: {},
    result_preview: {},
    error_message: '',
    created_by: 1,
    created_at: '2026-11-14T10:00:00Z',
    updated_at: '2026-11-14T10:00:00Z',
    ...partial,
  } as SyncJobInstance;
}

describe('mergeKnownJobs', () => {
  it('prefers jobs over page items over retained snapshots', () => {
    const retained = new Map<number, SyncJobInstance>([
      [1, job({id: 1, status: 'running', updated_at: '2026-01-01T00:00:00Z'})],
      [2, job({id: 2, status: 'running'})],
    ]);
    const pageItems = [
      job({id: 1, status: 'canceled', updated_at: '2026-01-02T00:00:00Z'}),
      job({id: 3, status: 'running'}),
    ];
    const jobs = [job({id: 1, status: 'failed'})];

    const merged = mergeKnownJobs(jobs, pageItems, retained);
    expect(merged.get(1)?.status).toBe('failed');
    expect(merged.get(2)?.status).toBe('running');
    expect(merged.get(3)?.status).toBe('running');
  });

  it('keeps a retained snapshot resolvable after it leaves the page', () => {
    const retained = new Map<number, SyncJobInstance>([
      [7, job({id: 7, status: 'running'})],
    ]);
    const merged = mergeKnownJobs([], [job({id: 9})], retained);
    // 翻页把 7 移出 items 且尚未进入 jobs（50 条窗口外）：仍要解析到 retained 快照。
    // Paging moved 7 off the page and it is outside the jobs window too: it must still resolve from retained.
    expect(merged.get(7)?.id).toBe(7);
  });
});

describe('retainedActiveJobIds', () => {
  const known = mergeKnownJobs(
    [],
    [],
    new Map<number, SyncJobInstance>([
      [1, job({id: 1, status: 'running'})],
      [2, job({id: 2, status: 'failed'})],
      [3, job({id: 3, status: 'running', run_type: 'preview'})],
    ]),
  );

  it('returns retained jobs that are active and absent from jobs', () => {
    expect(retainedActiveJobIds(known, [], [1, 2, 3, 9, null])).toEqual([1, 3]);
  });

  it('skips ids already covered by jobs', () => {
    const jobs = [job({id: 1, status: 'running'})];
    expect(retainedActiveJobIds(known, jobs, [1, 2])).toEqual([]);
  });
});
