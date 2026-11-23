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
 * 序号只决定是否提交数据；loading 复位不看序号。
 * Seq only gates committing data; loading reset ignores seq.
 */
export function shouldCommitClusterLoadResult(
  seq: number,
  currentSeq: number,
): boolean {
  return seq === currentSeq;
}

/**
 * 非静默加载在 finally 中无条件清除 loading（即使序号已被更新请求抢走）。
 * Non-silent loads always clear loading in finally, even when seq was superseded.
 */
export function shouldClearClusterLoading(silent: boolean): boolean {
  return !silent;
}

/**
 * 增量模式用「上次成功拉 runtime 时记录的 PID」作基准，而不是当前 nodes 列表。
 * Incremental mode compares against the PID recorded when runtime was last fetched, not the live nodes list.
 */
export function selectNodesNeedingJavaRuntime(
  nodeList: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  runtimePidByNodeId: Record<number, number | undefined>,
  previousRuntimes: Record<number, NodeJavaRuntimeView | undefined>,
  mode: Exclude<JavaRuntimeRefreshMode, 'skip'>,
): number[] {
  if (mode === 'full') {
    return nodeList.map((node) => node.id);
  }
  const need: number[] = [];
  for (const node of nodeList) {
    const existing = previousRuntimes[node.id];
    const runtimePid = runtimePidByNodeId[node.id];
    if (
      existing?.status === 'ok' &&
      runtimePid !== undefined &&
      Number(runtimePid) === Number(node.process_pid)
    ) {
      continue;
    }
    need.push(node.id);
  }
  return need;
}

/**
 * 合并 runtime 视图：PID 变化时先保留旧 ok（不闪 spinner），新值返回后再替换。
 * Merge runtime views: on PID change keep the old ok until the new fetch resolves (no spinner flash).
 */
export function mergeJavaRuntimeViews(
  nodeList: Array<Pick<NodeInfo, 'id' | 'process_pid'>>,
  runtimePidByNodeId: Record<number, number | undefined>,
  previousRuntimes: Record<number, NodeJavaRuntimeView | undefined>,
  mode: Exclude<JavaRuntimeRefreshMode, 'skip'>,
): Record<number, NodeJavaRuntimeView> {
  const need = new Set(
    selectNodesNeedingJavaRuntime(
      nodeList,
      runtimePidByNodeId,
      previousRuntimes,
      mode,
    ),
  );
  const next: Record<number, NodeJavaRuntimeView> = {};
  for (const node of nodeList) {
    const existing = previousRuntimes[node.id];
    if (need.has(node.id)) {
      if (mode === 'incremental' && existing?.status === 'ok') {
        next[node.id] = existing;
      } else {
        next[node.id] = {status: 'loading'};
      }
      continue;
    }
    if (existing) {
      next[node.id] = existing;
    }
  }
  return next;
}

export interface JavaRuntimeFetchResult {
  version?: number;
  version_string?: string;
  java_home?: string;
}

export interface RefreshNodeJavaRuntimesResult {
  views: Record<number, NodeJavaRuntimeView>;
  runtimePidByNodeId: Record<number, number>;
  fetchedNodeIds: number[];
}

/**
 * 按 full/incremental 计划并拉取节点 JDK runtime；更新 runtimePid 基准。
 * Plan and fetch node JDK runtimes; update the runtimePid baseline.
 */
export async function refreshNodeJavaRuntimes(args: {
  nodeList: Array<Pick<NodeInfo, 'id' | 'process_pid'>>;
  mode: Exclude<JavaRuntimeRefreshMode, 'skip'>;
  previousRuntimes: Record<number, NodeJavaRuntimeView | undefined>;
  runtimePidByNodeId: Record<number, number | undefined>;
  fetchRuntime: (nodeId: number) => Promise<JavaRuntimeFetchResult>;
}): Promise<RefreshNodeJavaRuntimesResult> {
  const {
    nodeList,
    mode,
    previousRuntimes,
    runtimePidByNodeId,
    fetchRuntime,
  } = args;

  if (nodeList.length === 0) {
    return {views: {}, runtimePidByNodeId: {}, fetchedNodeIds: []};
  }

  const needIds = selectNodesNeedingJavaRuntime(
    nodeList,
    runtimePidByNodeId,
    previousRuntimes,
    mode,
  );
  const views = mergeJavaRuntimeViews(
    nodeList,
    runtimePidByNodeId,
    previousRuntimes,
    mode,
  );
  const nextPid: Record<number, number> = {};
  for (const node of nodeList) {
    if (!needIds.includes(node.id) && runtimePidByNodeId[node.id] !== undefined) {
      nextPid[node.id] = Number(runtimePidByNodeId[node.id]);
    }
  }

  if (needIds.length === 0) {
    return {views, runtimePidByNodeId: nextPid, fetchedNodeIds: []};
  }

  const needSet = new Set(needIds);
  const toFetch = nodeList.filter((node) => needSet.has(node.id));
  await Promise.all(
    toFetch.map(async (node) => {
      try {
        const rt = await fetchRuntime(node.id);
        views[node.id] = {
          status: 'ok',
          version: rt.version,
          version_string: rt.version_string,
          java_home: rt.java_home,
        };
        nextPid[node.id] = Number(node.process_pid);
      } catch {
        views[node.id] = {status: 'error'};
        // 失败不更新 pid 基准，下次仍会重试。 / Keep pid baseline unchanged on failure so the next pass retries.
      }
    }),
  );

  return {
    views,
    runtimePidByNodeId: nextPid,
    fetchedNodeIds: needIds,
  };
}
