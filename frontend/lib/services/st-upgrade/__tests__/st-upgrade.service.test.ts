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

import {afterEach, describe, expect, it, vi} from 'vitest';
import apiClient from '../../core/api-client';
import {StUpgradeService} from '../st-upgrade.service';
import type {UpgradeTask} from '../types';

describe('StUpgradeService.executePlan', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('收到一次性确认编号后复用幂等键重试一次', async () => {
    const confirmationError = Object.assign(
      new Error('confirmation required'),
      {
        data: {
          confirmation_required: true,
          confirmation_id: 'confirmation-1',
        },
      },
    );
    const task = {id: 91, plan_id: 12, status: 'pending'} as UpgradeTask;
    const post = vi
      .spyOn(apiClient, 'post')
      .mockRejectedValueOnce(confirmationError)
      .mockResolvedValueOnce({data: {error_msg: '', data: task}});

    await expect(StUpgradeService.executePlan({plan_id: 12})).resolves.toBe(
      task,
    );

    expect(post).toHaveBeenCalledTimes(2);
    const firstHeaders = post.mock.calls[0][2]?.headers as Record<
      string,
      string
    >;
    const secondHeaders = post.mock.calls[1][2]?.headers as Record<
      string,
      string
    >;
    expect(firstHeaders['Idempotency-Key']).toMatch(/^web-/);
    expect(firstHeaders['X-STX-Confirm']).toBe('true');
    expect(firstHeaders['X-STX-Confirmation-ID']).toBeUndefined();
    expect(secondHeaders['Idempotency-Key']).toBe(
      firstHeaders['Idempotency-Key'],
    );
    expect(secondHeaders['X-STX-Confirmation-ID']).toBe('confirmation-1');
  });

  it('带确认编号的请求失败后不会继续自动重试', async () => {
    const error = Object.assign(new Error('confirmation expired'), {
      data: {
        confirmation_required: true,
        confirmation_id: 'confirmation-2',
      },
    });
    const post = vi.spyOn(apiClient, 'post').mockRejectedValue(error);

    await expect(
      StUpgradeService.executePlan(
        {plan_id: 12},
        'web-upgrade-fixed-key',
        'confirmation-1',
      ),
    ).rejects.toBe(error);
    expect(post).toHaveBeenCalledTimes(1);
  });
});
