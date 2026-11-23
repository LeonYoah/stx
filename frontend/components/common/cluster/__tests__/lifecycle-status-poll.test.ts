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
  ClusterStatus,
  NodeStatus,
  OperationType,
} from '@/lib/services/cluster/types';
import {
  baselinePidsFromNodes,
  isLifecyclePollSatisfied,
  isRestartLifecycleOperation,
  lifecycleTargetForOperation,
} from '../lifecycle-status-poll';

describe('lifecycle-status-poll', () => {
  it('maps restart/start to running and stop to stopped', () => {
    expect(lifecycleTargetForOperation(OperationType.RESTART)).toBe('running');
    expect(lifecycleTargetForOperation(OperationType.START)).toBe('running');
    expect(lifecycleTargetForOperation(OperationType.STOP)).toBe('stopped');
    expect(lifecycleTargetForOperation('restart')).toBe('running');
  });

  it('requires running nodes with pid > 0 and running cluster status', () => {
    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.STOPPED,
          nodes: [{id: 1, status: NodeStatus.STOPPED, process_pid: 0}],
        },
        'running',
      ),
    ).toBe(false);

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [{id: 1, status: NodeStatus.RUNNING, process_pid: 0}],
        },
        'running',
      ),
    ).toBe(false);

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.STOPPED,
          nodes: [{id: 1, status: NodeStatus.RUNNING, process_pid: 3143723}],
        },
        'running',
      ),
    ).toBe(false);

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [{id: 1, status: NodeStatus.RUNNING, process_pid: 3143723}],
        },
        'running',
      ),
    ).toBe(true);
  });

  it('requires stopped nodes with cleared pid for stop target', () => {
    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [{id: 1, status: NodeStatus.RUNNING, process_pid: 1}],
        },
        'stopped',
      ),
    ).toBe(false);

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.STOPPED,
          nodes: [{id: 1, status: NodeStatus.STOPPED, process_pid: 0}],
        },
        'stopped',
      ),
    ).toBe(true);
  });

  it('scopes node-level stop without requiring the whole cluster stopped', () => {
    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [
            {id: 1, status: NodeStatus.STOPPED, process_pid: 0},
            {id: 2, status: NodeStatus.RUNNING, process_pid: 99},
          ],
        },
        'stopped',
        [1],
      ),
    ).toBe(true);
  });

  it('rejects empty node lists', () => {
    expect(
      isLifecyclePollSatisfied(
        {clusterStatus: ClusterStatus.RUNNING, nodes: []},
        'running',
      ),
    ).toBe(false);
  });

  it('restart baselines reject stale RUNNING+old PID (AC5 root cause)', () => {
    expect(isRestartLifecycleOperation(OperationType.RESTART)).toBe(true);
    expect(isRestartLifecycleOperation('restart')).toBe(true);
    expect(isRestartLifecycleOperation(OperationType.START)).toBe(false);

    const baselines = baselinePidsFromNodes(
      [
        {id: 4, process_pid: 1876},
        {id: 5, process_pid: 99},
      ],
      [4],
    );
    expect(baselines).toEqual({4: 1876});

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [
            {id: 4, status: NodeStatus.RUNNING, process_pid: 1876},
            {id: 5, status: NodeStatus.RUNNING, process_pid: 99},
          ],
        },
        'running',
        [4],
        baselines,
      ),
    ).toBe(false);

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [
            {id: 4, status: NodeStatus.RUNNING, process_pid: 2810},
            {id: 5, status: NodeStatus.RUNNING, process_pid: 99},
          ],
        },
        'running',
        [4],
        baselines,
      ),
    ).toBe(true);
  });

  it('header full-scope restart requires every scoped node to change PID', () => {
    const baselines = baselinePidsFromNodes([
      {id: 1, process_pid: 2810},
      {id: 2, process_pid: 100},
    ]);
    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [
            {id: 1, status: NodeStatus.RUNNING, process_pid: 2810},
            {id: 2, status: NodeStatus.RUNNING, process_pid: 3196},
          ],
        },
        'running',
        undefined,
        baselines,
      ),
    ).toBe(false);

    expect(
      isLifecyclePollSatisfied(
        {
          clusterStatus: ClusterStatus.RUNNING,
          nodes: [
            {id: 1, status: NodeStatus.RUNNING, process_pid: 3196},
            {id: 2, status: NodeStatus.RUNNING, process_pid: 3200},
          ],
        },
        'running',
        undefined,
        baselines,
      ),
    ).toBe(true);
  });
});
