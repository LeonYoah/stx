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
import {HostService} from '../host.service';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('HostService delete R2 confirmation', () => {
  it('deleteHost 首次取得确认编号后复用幂等键重试一次', async () => {
    const confirmationError = Object.assign(new Error('confirmation required'), {
      data: {confirmation_required: true, confirmation_id: 'confirm-host-1'},
    });
    const del = vi
      .spyOn(apiClient, 'delete')
      .mockRejectedValueOnce(confirmationError)
      .mockResolvedValueOnce({data: {error_msg: '', data: null}});

    await expect(HostService.deleteHost(12)).resolves.toBeUndefined();

    expect(del).toHaveBeenCalledTimes(2);
    expect(del.mock.calls[0][0]).toBe('/hosts/12');
    const first = del.mock.calls[0][1]?.headers as Record<string, string>;
    const second = del.mock.calls[1][1]?.headers as Record<string, string>;
    expect(first['Idempotency-Key']).toMatch(/^web-host-delete-/);
    expect(second['Idempotency-Key']).toBe(first['Idempotency-Key']);
    expect(second['X-STX-Confirmation-ID']).toBe('confirm-host-1');
  });

  it('deleteHostSafe 透传关联集群冲突', async () => {
    const error = Object.assign(
      new Error('host is associated with clusters and cannot be deleted'),
      {},
    );
    vi.spyOn(apiClient, 'delete').mockRejectedValue(error);

    await expect(HostService.deleteHostSafe(8)).resolves.toEqual({
      success: false,
      error: 'host is associated with clusters and cannot be deleted',
    });
  });
});
