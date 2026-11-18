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

import {describe, expect, it} from 'vitest';
import {
  formatRatePair,
  formatTotalPair,
  isRunningStatus,
  jobDisplayName,
  jobDurationSeconds,
  jobListRates,
  jobTimeBounds,
  jobWorkbenchHref,
  resolveJobInitiatorName,
} from '../job-table-format';

// 迁移自 dashboard/job-list-utils 的用例；新基建（时长 / 速率对 / 发起人）在同文件补齐。
// Migrated from dashboard/job-list-utils; the new helpers (duration / rate pair / initiator) are covered here too.
describe('job-table-format', () => {
  it('prefers the submitted job name, then the task name', () => {
    expect(
      jobDisplayName({
        id: 3,
        submit_spec: {job_name: ' orders '},
        task_name: 't',
      }),
    ).toBe('orders');
    expect(jobDisplayName({id: 3, submit_spec: {}, task_name: 'task-a'})).toBe(
      'task-a',
    );
    expect(jobDisplayName({id: 3, submit_spec: {}})).toBe('#3');
  });

  it('builds the workbench deep link', () => {
    expect(jobWorkbenchHref({task_id: 7, id: 42})).toBe(
      '/workbench?task=7&job=42',
    );
    expect(jobWorkbenchHref({task_id: 7, job_id: 9})).toBe(
      '/workbench?task=7&job=9',
    );
    expect(jobWorkbenchHref({})).toBe('/workbench');
  });

  it('treats running and savepoint-in-flight as active for metrics, nothing else', () => {
    expect(isRunningStatus('running')).toBe(true);
    expect(isRunningStatus('doing_savepoint')).toBe(true);
    expect(isRunningStatus('pending')).toBe(false);
    expect(isRunningStatus('failed')).toBe(false);
  });

  it('falls back to updated_at when the finish time precedes the start (engine timezone skew)', () => {
    const skewed = jobTimeBounds({
      status: 'failed',
      started_at: '2026-11-14T12:13:03Z',
      finished_at: '2026-11-14T04:38:28Z',
      updated_at: '2026-11-14T12:39:44Z',
      created_at: '2026-11-14T12:13:00Z',
    });
    expect(skewed.endMs).toBe(Date.parse('2026-11-14T12:39:44Z'));
    const running = jobTimeBounds({
      status: 'running',
      started_at: '2026-11-14T12:00:00Z',
      created_at: '',
      updated_at: '',
    });
    expect(running.endMs).toBeNull();
  });

  it('uses live rates while running and run averages once finished', () => {
    expect(jobListRates(true, {read_qps: 3, write_qps: 2}, 0)).toEqual({
      read: 3,
      write: 2,
      average: false,
    });
    expect(
      jobListRates(false, {read_total: 600, write_total: 300}, 60),
    ).toEqual({read: 10, write: 5, average: true});
    expect(jobListRates(false, {read_total: 600}, 0)).toEqual({average: true});
  });

  it('measures durations against the end time for finished jobs and the refresh instant while running', () => {
    const finished = {
      status: 'success' as const,
      started_at: '2026-11-14T10:00:00Z',
      finished_at: '2026-11-14T10:10:00Z',
      updated_at: '2026-11-14T10:10:05Z',
      created_at: '2026-11-14T10:00:00Z',
    };
    expect(jobDurationSeconds(finished, null)).toBe(600);
    // 运行中：基准时刻是列表刷新时刻。/ Running jobs measure against the list refresh instant.
    const running = {
      status: 'running' as const,
      started_at: '2026-11-14T10:00:00Z',
      created_at: '2026-11-14T10:00:00Z',
      updated_at: '2026-11-14T10:05:00Z',
    };
    expect(
      jobDurationSeconds(running, Date.parse('2026-11-14T10:05:00Z')),
    ).toBe(300);
    // 没有基准时刻或没有开始时间时不估算。/ No estimate without a start or a base instant.
    expect(jobDurationSeconds(running, null)).toBe(0);
    expect(
      jobDurationSeconds(
        {
          status: 'pending' as const,
          created_at: '',
          started_at: '',
          finished_at: '',
          updated_at: '',
        },
        Date.now(),
      ),
    ).toBe(0);
  });

  it('formats read/write rate and total pairs with placeholders', () => {
    expect(formatRatePair(1234, 500)).toBe('1.2K / 500/s');
    expect(formatRatePair(undefined, 5)).toBe('- / 5/s');
    expect(formatRatePair(undefined, undefined)).toBe('-');
    expect(formatTotalPair(6000, 3000)).toBe('6K / 3K');
    expect(formatTotalPair(undefined, undefined)).toBe('-');
  });

  it('resolves the initiator name through self, the user list, then a uid fallback', () => {
    const job = {created_by: 7};
    const user = (extra: Partial<{username: string; nickname: string}>) => ({
      id: 7,
      username: '',
      nickname: '',
      email: '',
      is_admin: false,
      ...extra,
    });
    expect(
      resolveJobInitiatorName(job, 7, 'me', [user({username: 'someone'})]),
    ).toBe('me');
    expect(
      resolveJobInitiatorName(job, 1, 'me', [user({nickname: 'nick'})]),
    ).toBe('nick');
    expect(resolveJobInitiatorName(job, 1, 'me', [])).toBe('User #7');
    expect(resolveJobInitiatorName({created_by: 1}, 9, 'me', [])).toBe(
      'admin',
    );
    expect(resolveJobInitiatorName({created_by: 0}, 1)).toBe('-');
  });
});
