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
  formatCompactNumber,
  formatDurationSeconds,
  trendChangePercent,
} from '../dashboard-format';
import {
  jobDisplayName,
  jobListRates,
  jobTimeBounds,
  jobWorkbenchHref,
} from '../job-list-utils';

describe('dashboard-format', () => {
  it('formats compact numbers and placeholders', () => {
    expect(formatCompactNumber(12345)).toBe('12.3K');
    expect(formatCompactNumber(undefined)).toBe('-');
    expect(formatCompactNumber(Number.NaN)).toBe('-');
  });

  it('keeps the two largest duration units', () => {
    const units = {d: 'd', h: 'h', m: 'm'};
    expect(formatDurationSeconds(2 * 86400 + 4 * 3600 + 59, units)).toBe(
      '2d 4h',
    );
    expect(formatDurationSeconds(3 * 3600 + 12 * 60, units)).toBe('3h 12m');
    expect(formatDurationSeconds(30, units)).toBe('<1m');
  });

  it('computes the write-rate change against the first sample', () => {
    const hour = 60 * 60 * 1000;
    expect(
      trendChangePercent([
        [0, 100],
        [hour, 106],
      ]),
    ).toBe(6);
    expect(
      trendChangePercent([
        [0, 100],
        [hour, 50],
      ]),
    ).toBe(-50);
    expect(
      trendChangePercent([
        [0, 0],
        [hour, 50],
      ]),
    ).toBeNull();
    expect(trendChangePercent([[0, 10]])).toBeNull();
    // 作业刚启动 1 分钟：不能说成「较 1 小时前」。/ A job one minute old must not read as "vs. 1 hour ago".
    expect(
      trendChangePercent([
        [0, 75],
        [60_000, 204],
      ]),
    ).toBeNull();
  });
});

describe('job-list-utils', () => {
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
});
