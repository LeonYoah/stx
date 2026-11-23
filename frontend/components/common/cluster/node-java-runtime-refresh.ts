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

import type {NodeJavaRuntimeView} from '@/components/common/java/NodeJavaHomeCell';
import type {NodeInfo} from '@/lib/services/cluster/types';

export type JavaRuntimeRefreshMode = 'full' | 'incremental' | 'skip';

export interface LoadClusterDataOptions {
  /** 不显示整页 skeleton / loading。 / Do not show full-page skeleton/loading. */
  silent?: boolean;
  /** 失败时是否 toast；默认 true。仅轮询/事件联动应关。 / Toast on error; default true. Only poll/event linkage should disable. */
  notifyError?: boolean;
  /** JDK 运行时刷新策略。 / JDK runtime refresh strategy. */
  javaRuntimeMode?: JavaRuntimeRefreshMode;
}

export interface NormalizedLoadClusterDataOptions {
  silent: boolean;
  notifyError: boolean;
  javaRuntimeMode: JavaRuntimeRefreshMode;
}

/**
 * 归一化 loadClusterData 选项：silent 只控骨架；notifyError 独立，默认开启。
 * Normalize loadClusterData options: silent only controls skeleton; notifyError defaults on.
 */
export function normalizeLoadClusterDataOptions(
  options?: LoadClusterDataOptions,
): NormalizedLoadClusterDataOptions {
  return {
    silent: options?.silent ?? false,
    notifyError: options?.notifyError ?? true,
    javaRuntimeMode: options?.javaRuntimeMode ?? 'full',
  };
}

/**
 * 增量模式下：保留已有 ok 且 PID 未变的节点，其余需要重新拉取。
 * Incremental mode: keep existing ok runtimes when PID is unchanged; refetch the rest.
 */
export function selectNodesNeedingJavaRuntime(
  nodeList: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  previousNodes: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  previousRuntimes: Record<number, NodeJavaRuntimeView | undefined>,
  mode: Exclude<JavaRuntimeRefreshMode, 'skip'>,
): number[] {
  if (mode === 'full') {
    return nodeList.map((node) => node.id);
  }
  const prevById = new Map(previousNodes.map((node) => [node.id, node]));
  const need: number[] = [];
  for (const node of nodeList) {
    const existing = previousRuntimes[node.id];
    const prevNode = prevById.get(node.id);
    if (
      existing?.status === 'ok' &&
      prevNode &&
      Number(prevNode.process_pid) === Number(node.process_pid)
    ) {
      continue;
    }
    need.push(node.id);
  }
  return need;
}

/**
 * 合并 runtime 视图：保留未改动的 ok，其余标 loading；去掉已删除节点。
 * Merge runtime views: keep unchanged ok entries, mark others loading, drop removed nodes.
 */
export function mergeJavaRuntimeViews(
  nodeList: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  previousNodes: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  previousRuntimes: Record<number, NodeJavaRuntimeView | undefined>,
  mode: Exclude<JavaRuntimeRefreshMode, 'skip'>,
): Record<number, NodeJavaRuntimeView> {
  const need = new Set(
    selectNodesNeedingJavaRuntime(
      nodeList,
      previousNodes,
      previousRuntimes,
      mode,
    ),
  );
  const next: Record<number, NodeJavaRuntimeView> = {};
  for (const node of nodeList) {
    if (need.has(node.id)) {
      next[node.id] = {status: 'loading'};
      continue;
    }
    const existing = previousRuntimes[node.id];
    if (existing) {
      next[node.id] = existing;
    }
  }
  return next;
}
