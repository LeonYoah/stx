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

/**
 * 行为回归：旧实现（无 baseline）会在「旧 RUNNING+旧 PID」上立刻达标停轮询，
 * 与实机 AC1/AC2/AC5「停留 90s+ 仍旧 PID」一致；新实现必须继续轮询直到 PID 变化。
 * Behavior regression: without baselines the old poll stops on stale RUNNING+old PID
 * (matches real AC1/AC2/AC5 stay-stale); with baselines it must keep polling until PID changes.
 */
import {act, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {
  ClusterStatus,
  NodeStatus,
  OperationType,
} from '@/lib/services/cluster/types';
import {
  baselinePidsFromNodes,
  isRestartLifecycleOperation,
  lifecycleTargetForOperation,
} from '../lifecycle-status-poll';
import {useLifecycleStatusPoll} from '../useLifecycleStatusPoll';

describe('restart baseline poll behavior (node + header)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('node restart entry keeps polling while PID stays at the pre-op value', async () => {
    const preOpNodes = [{id: 4, status: NodeStatus.RUNNING, process_pid: 2334}];
    const baselines = baselinePidsFromNodes(preOpNodes, [4]);
    expect(isRestartLifecycleOperation(OperationType.RESTART)).toBe(true);

    const loadSnapshot = vi.fn().mockResolvedValue({
      clusterStatus: ClusterStatus.RUNNING,
      nodes: [{id: 4, status: NodeStatus.RUNNING, process_pid: 2334}],
    });

    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 20_000,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll(
        lifecycleTargetForOperation(OperationType.RESTART),
        {scopeNodeIds: [4], baselinePids: baselines},
      );
    });

    for (let i = 0; i < 4; i += 1) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(2500);
      });
    }
    expect(loadSnapshot.mock.calls.length).toBeGreaterThanOrEqual(4);
  });

  it('header cluster restart entry requires all nodes to leave baseline PIDs', async () => {
    const preOpNodes = [
      {id: 1, status: NodeStatus.RUNNING, process_pid: 2810},
      {id: 2, status: NodeStatus.RUNNING, process_pid: 100},
    ];
    const baselines = baselinePidsFromNodes(preOpNodes);

    const loadSnapshot = vi
      .fn()
      .mockResolvedValueOnce({
        clusterStatus: ClusterStatus.RUNNING,
        nodes: [
          {id: 1, status: NodeStatus.RUNNING, process_pid: 2810},
          {id: 2, status: NodeStatus.RUNNING, process_pid: 100},
        ],
      })
      .mockResolvedValueOnce({
        clusterStatus: ClusterStatus.RUNNING,
        nodes: [
          {id: 1, status: NodeStatus.RUNNING, process_pid: 3196},
          {id: 2, status: NodeStatus.RUNNING, process_pid: 100},
        ],
      })
      .mockResolvedValueOnce({
        clusterStatus: ClusterStatus.RUNNING,
        nodes: [
          {id: 1, status: NodeStatus.RUNNING, process_pid: 3196},
          {id: 2, status: NodeStatus.RUNNING, process_pid: 3201},
        ],
      });

    const onSatisfied = vi.fn();
    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
        onSatisfied,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll(
        lifecycleTargetForOperation('restart'),
        {baselinePids: baselines},
      );
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(onSatisfied).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(onSatisfied).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(onSatisfied).toHaveBeenCalledTimes(1);
    expect(loadSnapshot).toHaveBeenCalledTimes(3);
  });
});
