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
import type {SyncJobInstance} from '@/lib/services/sync';
import {
  getCancelTimeoutMinutes,
  getSubmitOutcomeDeadline,
  getSubmitTimeoutMinutes,
  getTaskTimeoutMinutes,
  isCancelOutcomeUnknown,
  isCancelOverdue,
  isSubmitOutcomeExpired,
  isSubmitOutcomeUnknown,
  parseSubmitOutcomePendingError,
  parseTaskTimeoutInput,
} from '../sync-studio-utils';

function job(overrides: Partial<SyncJobInstance>): SyncJobInstance {
  return {
    id: 1,
    task_id: 1,
    task_version: 1,
    run_type: 'run',
    platform_job_id: '9',
    engine_job_id: '9',
    status: 'running',
    submit_spec: {},
    result_preview: {},
    error_message: '',
    created_by: 1,
    created_at: '',
    updated_at: '',
    ...overrides,
  } as SyncJobInstance;
}

describe('submit outcome helpers', () => {
  it('flags only pending jobs whose submit outcome is unknown', () => {
    const pending = job({status: 'pending', submit_spec: {submit_outcome: 'unknown', submit_deadline: '2026-10-27T08:05:00Z', submit_timeout_seconds: 120}});
    expect(isSubmitOutcomeUnknown(pending)).toBe(true);
    expect(getSubmitOutcomeDeadline(pending)).toBe('2026-10-27T08:05:00Z');
    expect(getSubmitTimeoutMinutes(pending)).toBe(2);
    expect(isSubmitOutcomeUnknown(job({status: 'running', submit_spec: {submit_outcome: 'unknown'}}))).toBe(false);
    expect(isSubmitOutcomeUnknown(job({status: 'pending'}))).toBe(false);
    expect(isSubmitOutcomeUnknown(null)).toBe(false);
  });

  it('recognises expired submits and defaults the timeout to five minutes', () => {
    const expired = job({status: 'failed', submit_spec: {submit_outcome: 'expired'}});
    expect(isSubmitOutcomeExpired(expired)).toBe(true);
    expect(getSubmitTimeoutMinutes(expired)).toBe(5);
    expect(isSubmitOutcomeExpired(job({submit_spec: {submit_outcome: 'confirmed'}}))).toBe(false);
  });

  it('parses the 409 pending-submit message', () => {
    expect(
      parseSubmitOutcomePendingError(
        'sync: previous submit outcome is still being confirmed: job #42 is still being confirmed until 2026-10-27T08:05:00Z; do not resubmit',
      ),
    ).toEqual({jobId: '42', deadline: '2026-10-27T08:05:00Z'});
    expect(parseSubmitOutcomePendingError('sync: task has not been published')).toBeNull();
  });
});

describe('cancel outcome helpers', () => {
  it('reads the unknown stop outcome and the backend overdue flag', () => {
    const cancelling = job({status: 'cancelling', result_preview: {cancel_outcome: 'unknown', cancel_timeout_seconds: 600}, cancel_overdue: true});
    expect(isCancelOutcomeUnknown(cancelling)).toBe(true);
    expect(isCancelOverdue(cancelling)).toBe(true);
    expect(getCancelTimeoutMinutes(cancelling)).toBe(10);
    expect(isCancelOverdue(job({status: 'cancelling'}))).toBe(false);
  });
});

describe('task timeout inputs', () => {
  it('reads overrides from the definition', () => {
    expect(getTaskTimeoutMinutes({submit_timeout_minutes: 15}, 'submit_timeout_minutes')).toBe('15');
    expect(getTaskTimeoutMinutes({}, 'stop_timeout_minutes')).toBe('');
    expect(getTaskTimeoutMinutes(undefined, 'stop_timeout_minutes')).toBe('');
  });

  it('accepts empty or whole minutes between 1 and 120', () => {
    expect(parseTaskTimeoutInput('')).toBeNull();
    expect(parseTaskTimeoutInput('  ')).toBeNull();
    expect(parseTaskTimeoutInput('1')).toBe(1);
    expect(parseTaskTimeoutInput('120')).toBe(120);
    expect(parseTaskTimeoutInput('0')).toBeUndefined();
    expect(parseTaskTimeoutInput('121')).toBeUndefined();
    expect(parseTaskTimeoutInput('1.5')).toBeUndefined();
    expect(parseTaskTimeoutInput('abc')).toBeUndefined();
  });
});
