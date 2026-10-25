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

import {beforeEach, describe, expect, it, vi} from 'vitest';

const post = vi.fn();
vi.mock('../../core/api-client', () => ({
  default: {post: (...args: unknown[]) => post(...args)},
}));

import {SelfUpgradeService} from '../self-upgrade.service';
import {
  SelfUpgradeApiError,
  buildExecutePayload,
  countUpgradeNodes,
  restartingProcesses,
} from '..';

beforeEach(() => post.mockReset());

describe('SelfUpgradeService.execute', () => {
  it('posts the explicit mode to /self-upgrade/execute', async () => {
    post.mockResolvedValue({
      status: 202,
      data: {data: {id: 't1', execution_mode: 'dry_run'}},
    });
    const task = await SelfUpgradeService.execute(
      buildExecutePayload('dry_run'),
    );
    expect(post).toHaveBeenCalledTimes(1);
    const [url, body] = post.mock.calls[0];
    expect(String(url)).toMatch(/\/self-upgrade\/execute$/);
    expect(body).toEqual({mode: 'dry_run'});
    expect(task.execution_mode).toBe('dry_run');
  });

  it('surfaces a 400 for an invalid mode as an API error', async () => {
    post.mockResolvedValue({
      status: 400,
      data: {error_msg: 'invalid mode', error_code: 'invalid_request'},
    });
    await expect(
      SelfUpgradeService.execute({mode: 'apply'}),
    ).rejects.toBeInstanceOf(SelfUpgradeApiError);
  });
});

describe('confirm dialog helpers', () => {
  it('counts nodes including offline ones', () => {
    expect(
      countUpgradeNodes(
        [
          {
            host_id: 1,
            host_name: 'a',
            version: '1.0.0',
            online: true,
            up_to_date: false,
          },
          {
            host_id: 2,
            host_name: 'b',
            version: '1.0.0',
            online: false,
            up_to_date: false,
          },
          {
            host_id: 3,
            host_name: 'c',
            version: 'v1.0.1',
            online: false,
            up_to_date: true,
          },
        ],
        'v1.0.1',
      ),
    ).toEqual({
      total: 3,
      online: 1,
      offline: 2,
      toUpgrade: 2,
      offlineToUpgrade: 1,
    });
  });

  it('lists restarting processes from the plan', () => {
    expect(restartingProcesses(null)).toEqual([
      'controlPlane',
      'agents',
      'proxies',
    ]);
    expect(
      restartingProcesses({
        control_plane_strategy: 'systemd',
        frontend_detected: true,
        restarts_agents: true,
        restarts_proxies: true,
        seatunnel_affected: false,
      }),
    ).toEqual(['controlPlane', 'frontend', 'agents', 'proxies']);
  });
});
