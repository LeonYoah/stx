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
import {agentNodeTone, summarizeAgentNodes} from '../agent-nodes';
import type {AgentNodeUpgrade} from '../types';

const node = (hostId: number, status: string): AgentNodeUpgrade => ({
  host_id: hostId,
  host_name: `worker-${hostId}`,
  target_version: '1.0.1',
  status,
});

describe('summarizeAgentNodes', () => {
  it('handles empty input', () => {
    expect(summarizeAgentNodes(undefined)).toEqual({
      total: 0,
      succeeded: 0,
      failed: 0,
      rolledBack: 0,
      offline: 0,
      inProgress: 0,
      allDone: false,
    });
  });

  it('counts each per-node status', () => {
    const summary = summarizeAgentNodes([
      node(1, 'succeeded'),
      node(2, 'failed'),
      node(3, 'rolled_back'),
      node(4, 'offline'),
      node(5, 'upgrading'),
      node(6, 'pending'),
    ]);
    expect(summary).toMatchObject({
      total: 6,
      succeeded: 1,
      failed: 1,
      rolledBack: 1,
      offline: 1,
      inProgress: 2,
      allDone: false,
    });
  });

  it('is done only when every node succeeded', () => {
    expect(
      summarizeAgentNodes([node(1, 'succeeded'), node(2, 'succeeded')]).allDone,
    ).toBe(true);
  });
});

describe('agentNodeTone', () => {
  it.each([
    ['succeeded', 'success'],
    ['updated', 'success'],
    ['rolled_back', 'warning'],
    ['offline', 'warning'],
    ['failed', 'danger'],
    ['upgrading', 'progress'],
    ['skipped', 'muted'],
  ])('%s -> %s', (status, tone) => {
    expect(agentNodeTone(status)).toBe(tone);
  });
});
