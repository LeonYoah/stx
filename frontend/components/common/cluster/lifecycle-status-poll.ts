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
 * 启停/重启后详情页主状态有界轮询：在 AutoRestarter 拉起窗口内静默刷新，
 * 达标即停、超时不伪造成功；纯函数可单测，定时器由 hook 管理。
 * Bounded detail-page status polling after start/stop/restart: silent refresh during
 * the AutoRestarter window, stop on success, never fake success on timeout.
 */

import {
  ClusterStatus,
  NodeStatus,
  OperationType,
  type NodeInfo,
} from '@/lib/services/cluster/types';

/** 轮询间隔（毫秒）。 / Poll interval in ms. */
export const LIFECYCLE_POLL_INTERVAL_MS = 2500;

/** 最长等待（毫秒）；覆盖实测 ~17–19s 拉起并留余量。 / Max wait; covers ~17–19s AutoRestarter with headroom. */
export const LIFECYCLE_POLL_TIMEOUT_MS = 90_000;

export type LifecyclePollTarget = 'running' | 'stopped';

export interface LifecyclePollSnapshot {
  clusterStatus?: ClusterStatus | string;
  nodes: Array<Pick<NodeInfo, 'id' | 'status' | 'process_pid'>>;
}

export type LifecycleBaselinePids = Readonly<Record<number, number>>;

export function lifecycleTargetForOperation(
  operation: OperationType | 'start' | 'stop' | 'restart',
): LifecyclePollTarget {
  // OperationType.STOP === 'stop'；统一用字符串比较即可。
  // OperationType.STOP === 'stop'; a single string compare is enough.
  return String(operation) === 'stop' ? 'stopped' : 'running';
}

/**
 * 重启必须换 PID：从操作前快照收集基准 PID，供达标判定拒绝「旧 RUNNING+旧 PID」。
 * Restart requires a PID change: capture baseline PIDs before the op so satisfaction rejects stale RUNNING+old PID.
 */
export function baselinePidsFromNodes(
  nodes: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  scopeNodeIds?: number[],
): LifecycleBaselinePids {
  const scoped =
    scopeNodeIds && scopeNodeIds.length > 0
      ? nodes.filter((node) => scopeNodeIds.includes(node.id))
      : nodes;
  const baselines: Record<number, number> = {};
  for (const node of scoped) {
    baselines[node.id] = Number(node.process_pid || 0);
  }
  return baselines;
}

export function isRestartLifecycleOperation(
  operation: OperationType | 'start' | 'stop' | 'restart',
): boolean {
  return String(operation) === 'restart';
}

/**
 * 判断快照是否已达启停/重启后的期望主状态。
 * scopeNodeIds 非空时只检查这些节点（单节点/批量）；全量操作才对齐 cluster.status。
 * baselinePids 非空时（重启）还要求 PID 相对基准发生变化，避免旧进程被当成成功。
 * Whether the snapshot matches the expected post-lifecycle main status.
 * When scopeNodeIds is set, only those nodes are checked; cluster.status is required for full-scope ops.
 * When baselinePids is set (restart), each node must also change PID vs baseline so a stale process is not success.
 */
export function isLifecyclePollSatisfied(
  snapshot: LifecyclePollSnapshot,
  target: LifecyclePollTarget,
  scopeNodeIds?: number[],
  baselinePids?: LifecycleBaselinePids,
): boolean {
  const scoped =
    scopeNodeIds && scopeNodeIds.length > 0
      ? snapshot.nodes.filter((node) => scopeNodeIds.includes(node.id))
      : snapshot.nodes;
  if (!scoped.length) {
    return false;
  }

  const fullScope =
    !scopeNodeIds ||
    scopeNodeIds.length === 0 ||
    scopeNodeIds.length >= snapshot.nodes.length;

  if (target === 'running') {
    const nodesOk = scoped.every((node) => {
      const pid = Number(node.process_pid || 0);
      if (node.status !== NodeStatus.RUNNING || pid <= 0) {
        return false;
      }
      if (!baselinePids || !(node.id in baselinePids)) {
        return true;
      }
      // 重启：PID 必须相对操作前变化（含 0→正数）；旧 PID 残留一律未达标。
      // Restart: PID must differ from the pre-op baseline (incl. 0→positive); stale PID is never success.
      return pid !== Number(baselinePids[node.id] || 0);
    });
    if (!nodesOk) {
      return false;
    }
    if (!fullScope || snapshot.clusterStatus === undefined) {
      return true;
    }
    return snapshot.clusterStatus === ClusterStatus.RUNNING;
  }

  const nodesOk = scoped.every(
    (node) =>
      node.status === NodeStatus.STOPPED && Number(node.process_pid || 0) <= 0,
  );
  if (!nodesOk) {
    return false;
  }
  if (!fullScope || snapshot.clusterStatus === undefined) {
    return true;
  }
  return snapshot.clusterStatus === ClusterStatus.STOPPED;
}
