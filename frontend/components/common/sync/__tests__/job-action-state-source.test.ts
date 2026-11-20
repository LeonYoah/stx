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

import {getJobActionState} from '../sync-studio-utils';
import type {SyncJobInstance} from '@/lib/services/sync/types';

const baseJob = {
  id: 1,
  task_id: 0,
  task_version: 0,
  run_type: 'external',
  source: 'external',
  platform_job_id: '',
  engine_job_id: '9001',
  status: 'running',
  submit_spec: {},
  result_preview: {},
  error_message: '',
  created_by: 0,
  created_at: '2026-11-20T00:00:00Z',
  updated_at: '2026-11-20T00:00:00Z',
} as SyncJobInstance;

const ctx = {
  isAdmin: false,
  isOwner: false,
  canRun: true,
  disableRecover: false,
};

describe('getJobActionState with job source', () => {
  it('never offers recover for external jobs and follows backend can_stop', () => {
    const user = getJobActionState(
      {...baseJob, can_stop: false, can_recover: false},
      ctx,
    );
    expect(user.showRecover).toBe(false);
    expect(user.canRecover).toBe(false);
    expect(user.canStop).toBe(false);

    const admin = getJobActionState(
      {...baseJob, can_stop: true},
      {...ctx, isAdmin: true},
    );
    expect(admin.canStop).toBe(true);
    expect(admin.showRecover).toBe(false);
  });

  it('backend can_stop overrides local owner inference', () => {
    const job = {
      ...baseJob,
      source: 'stx',
      run_type: 'run',
      task_id: 3,
      created_by: 7,
      can_stop: false,
    } as SyncJobInstance;
    expect(
      getJobActionState(job, {...ctx, isOwner: true, currentUserId: 7}).canStop,
    ).toBe(false);
  });
});
