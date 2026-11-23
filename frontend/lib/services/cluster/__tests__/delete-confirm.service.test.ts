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
import {ClusterService} from '../cluster.service';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('ClusterService delete R2 confirmation', () => {
  it('deleteCluster 首次取得确认编号后复用幂等键重试一次', async () => {
    const confirmationError = Object.assign(new Error('confirmation required'), {
      data: {confirmation_required: true, confirmation_id: 'confirm-del-1'},
    });
    const del = vi
      .spyOn(apiClient, 'delete')
      .mockRejectedValueOnce(confirmationError)
      .mockResolvedValueOnce({data: {error_msg: '', data: null}});

    await expect(ClusterService.deleteCluster(9, {forceDelete: true})).resolves.toBeUndefined();

    expect(del).toHaveBeenCalledTimes(2);
    expect(del.mock.calls[0][0]).toBe('/clusters/9');
    const first = del.mock.calls[0][1]?.headers as Record<string, string>;
    const second = del.mock.calls[1][1]?.headers as Record<string, string>;
    expect(first['Idempotency-Key']).toMatch(/^web-cluster-delete-/);
    expect(first['X-STX-Confirm']).toBe('true');
    expect(first['X-STX-Confirmation-ID']).toBeUndefined();
    expect(second['Idempotency-Key']).toBe(first['Idempotency-Key']);
    expect(second['X-STX-Confirmation-ID']).toBe('confirm-del-1');
    expect(del.mock.calls[0][1]?.params).toEqual({force_delete: '1'});
    expect(del.mock.calls[1][1]?.params).toEqual({force_delete: '1'});
  });

  it('removeNode 带确认编号仍失败时不再二次重试', async () => {
    const error = Object.assign(new Error('confirmation expired'), {
      data: {confirmation_required: true, confirmation_id: 'confirm-rm-2'},
    });
    const del = vi.spyOn(apiClient, 'delete').mockRejectedValue(error);

    await expect(ClusterService.removeNode(1, 3)).rejects.toBe(error);
    expect(del).toHaveBeenCalledTimes(2);
  });

  it('deleteClusterSafe 透传业务错误文案', async () => {
    const error = Object.assign(new Error('cluster has running tasks'), {
      data: {error_msg: 'cluster has running tasks'},
    });
    vi.spyOn(apiClient, 'delete').mockRejectedValue(error);

    await expect(ClusterService.deleteClusterSafe(4)).resolves.toEqual({
      success: false,
      error: 'cluster has running tasks',
    });
  });
});
