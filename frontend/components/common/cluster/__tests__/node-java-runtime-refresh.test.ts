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

import {describe, expect, it, vi} from 'vitest';
import type {NodeJavaRuntimeView} from '@/components/common/java/NodeJavaHomeCell';
import {
  mergeJavaRuntimeViews,
  normalizeLoadClusterDataOptions,
  refreshNodeJavaRuntimes,
  selectNodesNeedingJavaRuntime,
  shouldClearClusterLoading,
  shouldCommitClusterLoadResult,
} from '../node-java-runtime-refresh';

describe('normalizeLoadClusterDataOptions', () => {
  it('keeps notifyError true by default even when silent', () => {
    expect(normalizeLoadClusterDataOptions({silent: true})).toEqual({
      silent: true,
      notifyError: true,
      javaRuntimeMode: 'full',
    });
  });

  it('allows poll/event linkage to disable notifyError explicitly', () => {
    expect(
      normalizeLoadClusterDataOptions({
        silent: true,
        notifyError: false,
        javaRuntimeMode: 'skip',
      }),
    ).toEqual({
      silent: true,
      notifyError: false,
      javaRuntimeMode: 'skip',
    });
  });
});

describe('cluster load seq / loading reset', () => {
  it('only commits results for the current seq', () => {
    expect(shouldCommitClusterLoadResult(1, 1)).toBe(true);
    expect(shouldCommitClusterLoadResult(1, 2)).toBe(false);
  });

  it('always clears loading for non-silent loads even when seq was superseded', () => {
    expect(shouldClearClusterLoading(false)).toBe(true);
    expect(shouldClearClusterLoading(true)).toBe(false);
  });
});

describe('selectNodesNeedingJavaRuntime / mergeJavaRuntimeViews', () => {
  const ok: NodeJavaRuntimeView = {
    status: 'ok',
    version: 11,
    version_string: '11.0.11',
    java_home: '/jdk',
  };

  it('full mode marks every node for refetch', () => {
    expect(
      selectNodesNeedingJavaRuntime([{id: 1, process_pid: 10}], {1: 10}, {1: ok}, 'full'),
    ).toEqual([1]);
  });

  it('incremental keeps ok rows when runtimePid matches current pid', () => {
    expect(
      selectNodesNeedingJavaRuntime(
        [
          {id: 1, process_pid: 10},
          {id: 2, process_pid: 20},
        ],
        {1: 10},
        {1: ok},
        'incremental',
      ),
    ).toEqual([2]);
  });

  it('incremental detects pid change via runtimePid baseline, not live nodes list', () => {
    // 模拟 nodesRef 已被轮询写成新 PID，但 runtimePid 仍是旧值。
    // Simulate nodesRef already updated to the new PID while runtimePid stays old.
    expect(
      selectNodesNeedingJavaRuntime(
        [{id: 1, process_pid: 200}],
        {1: 100},
        {1: ok},
        'incremental',
      ),
    ).toEqual([1]);
  });

  it('incremental keeps old ok while pid-changed fetch is in flight', () => {
    const merged = mergeJavaRuntimeViews(
      [{id: 1, process_pid: 200}],
      {1: 100},
      {1: ok},
      'incremental',
    );
    expect(merged[1]).toEqual(ok);
  });

  it('comparing current nodes to themselves with matching runtimePid needs no fetch', () => {
    expect(
      selectNodesNeedingJavaRuntime(
        [{id: 1, process_pid: 200}],
        {1: 200},
        {1: ok},
        'incremental',
      ),
    ).toEqual([]);
  });
});

describe('refreshNodeJavaRuntimes', () => {
  it('refetches once when pid changes 100 → 200 (satisfied / event incremental path)', async () => {
    const fetchRuntime = vi.fn(async () => ({
      version: 11,
      version_string: '11.0.11',
      java_home: '/jdk-new',
    }));

    const first = await refreshNodeJavaRuntimes({
      nodeList: [{id: 2, process_pid: 100}],
      mode: 'full',
      previousRuntimes: {},
      runtimePidByNodeId: {},
      fetchRuntime,
    });
    expect(fetchRuntime).toHaveBeenCalledTimes(1);
    expect(first.runtimePidByNodeId[2]).toBe(100);
    expect(first.views[2]).toMatchObject({
      status: 'ok',
      java_home: '/jdk-new',
    });

    fetchRuntime.mockClear();
    const second = await refreshNodeJavaRuntimes({
      nodeList: [{id: 2, process_pid: 200}],
      mode: 'incremental',
      previousRuntimes: first.views,
      runtimePidByNodeId: first.runtimePidByNodeId,
      fetchRuntime,
    });
    expect(fetchRuntime).toHaveBeenCalledTimes(1);
    expect(second.fetchedNodeIds).toEqual([2]);
    expect(second.runtimePidByNodeId[2]).toBe(200);

    fetchRuntime.mockClear();
    const third = await refreshNodeJavaRuntimes({
      nodeList: [{id: 2, process_pid: 200}],
      mode: 'incremental',
      previousRuntimes: second.views,
      runtimePidByNodeId: second.runtimePidByNodeId,
      fetchRuntime,
    });
    expect(fetchRuntime).not.toHaveBeenCalled();
    expect(third.fetchedNodeIds).toEqual([]);
  });
});
