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

import {act, renderHook} from '@testing-library/react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {
  ClusterStatus,
  NodeStatus,
} from '@/lib/services/cluster/types';
import {useLifecycleStatusPoll} from '../useLifecycleStatusPoll';

describe('useLifecycleStatusPoll', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('polls until nodes become running with a new pid, then stops', async () => {
    const loadSnapshot = vi
      .fn()
      .mockResolvedValueOnce({
        // 旧 PID 仍 running：有 baseline 时不得达标（实机 AC5）。
        // Still running with the old PID: must not satisfy when baselines are set (real AC5).
        clusterStatus: ClusterStatus.RUNNING,
        nodes: [{id: 2, status: NodeStatus.RUNNING, process_pid: 1876}],
      })
      .mockResolvedValueOnce({
        clusterStatus: ClusterStatus.RUNNING,
        nodes: [{id: 2, status: NodeStatus.RUNNING, process_pid: 3143723}],
      });

    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running', {
        scopeNodeIds: [2],
        baselinePids: {2: 1876},
      });
    });
    expect(loadSnapshot).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(2);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(2);
  });

  it('stops polling after timeout without faking success', async () => {
    const loadSnapshot = vi.fn().mockResolvedValue({
      clusterStatus: ClusterStatus.STOPPED,
      nodes: [{id: 2, status: NodeStatus.STOPPED, process_pid: 0}],
    });

    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 1000,
        timeoutMs: 2500,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running');
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(loadSnapshot.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(loadSnapshot.mock.calls.length).toBeLessThanOrEqual(3);
    const callsAfterTimeout = loadSnapshot.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(callsAfterTimeout);
  });

  it('clears timers on unmount so polls do not continue', async () => {
    const loadSnapshot = vi.fn().mockResolvedValue({
      clusterStatus: ClusterStatus.STOPPED,
      nodes: [{id: 2, status: NodeStatus.STOPPED, process_pid: 0}],
    });

    const {result, unmount} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running');
    });
    unmount();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(loadSnapshot).not.toHaveBeenCalled();
  });

  it('does not issue further requests after unmount even if an in-flight tick resolves', async () => {
    let resolveLoad:
      | ((value: {
          clusterStatus: ClusterStatus;
          nodes: Array<{id: number; status: NodeStatus; process_pid: number}>;
        }) => void)
      | undefined;
    const loadSnapshot = vi.fn(
      () =>
        new Promise<{
          clusterStatus: ClusterStatus;
          nodes: Array<{id: number; status: NodeStatus; process_pid: number}>;
        }>((resolve) => {
          resolveLoad = resolve;
        }),
    );

    const {result, unmount} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running');
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(1);

    unmount();
    await act(async () => {
      resolveLoad?.({
        clusterStatus: ClusterStatus.STOPPED,
        nodes: [{id: 2, status: NodeStatus.STOPPED, process_pid: 0}],
      });
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(1);
  });

  it('stopLifecyclePoll stops further requests (cluster switch)', async () => {
    const loadSnapshot = vi.fn().mockResolvedValue({
      clusterStatus: ClusterStatus.STOPPED,
      nodes: [{id: 2, status: NodeStatus.STOPPED, process_pid: 0}],
    });

    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running');
    });
    act(() => {
      result.current.stopLifecyclePoll();
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10_000);
    });
    expect(loadSnapshot).not.toHaveBeenCalled();
  });

  it('replacing a poll session cancels the previous one', async () => {
    const loadSnapshot = vi.fn().mockResolvedValue({
      clusterStatus: ClusterStatus.STOPPED,
      nodes: [{id: 2, status: NodeStatus.STOPPED, process_pid: 0}],
    });

    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running');
    });
    act(() => {
      result.current.startLifecyclePoll('running');
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(loadSnapshot).toHaveBeenCalledTimes(1);
  });

  it('invokes onSatisfied once when the running target is met', async () => {
    const onSatisfied = vi.fn();
    const loadSnapshot = vi.fn().mockResolvedValue({
      clusterStatus: ClusterStatus.RUNNING,
      nodes: [{id: 2, status: NodeStatus.RUNNING, process_pid: 3143723}],
    });

    const {result} = renderHook(() =>
      useLifecycleStatusPoll({
        loadSnapshot,
        intervalMs: 2500,
        timeoutMs: 90_000,
        onSatisfied,
      }),
    );

    act(() => {
      result.current.startLifecyclePoll('running');
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    expect(onSatisfied).toHaveBeenCalledTimes(1);
  });
});
