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

import type {AgentNodeUpgrade} from './types';

// AgentNodeSummary 汇总各节点 Agent 升级结果。/ AgentNodeSummary aggregates per-node Agent results.
export interface AgentNodeSummary {
  total: number;
  succeeded: number;
  failed: number;
  rolledBack: number;
  offline: number;
  inProgress: number;
  // 全部节点都到达目标版本 / every node reached the target version
  allDone: boolean;
}

export function summarizeAgentNodes(
  nodes?: AgentNodeUpgrade[] | null,
): AgentNodeSummary {
  const list = nodes ?? [];
  const count = (s: string) => list.filter((n) => n.status === s).length;
  const succeeded = count('succeeded');
  return {
    total: list.length,
    succeeded,
    failed: count('failed'),
    rolledBack: count('rolled_back'),
    offline: count('offline'),
    inProgress: count('pending') + count('upgrading'),
    allDone: list.length > 0 && succeeded === list.length,
  };
}

export type AgentNodeTone =
  | 'success'
  | 'warning'
  | 'danger'
  | 'progress'
  | 'muted';

// agentNodeTone 把节点状态映射为展示色调。/ agentNodeTone maps a node status to a display tone.
export function agentNodeTone(status: string): AgentNodeTone {
  switch (status) {
    case 'succeeded':
    case 'updated':
      return 'success';
    case 'rolled_back':
    case 'offline':
      return 'warning';
    case 'failed':
      return 'danger';
    case 'pending':
    case 'upgrading':
      return 'progress';
    default:
      return 'muted';
  }
}
