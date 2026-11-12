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

import {
  ONBOARDING_STEP_IDS,
  type OnboardingPath,
  type OnboardingPersistedState,
  type OnboardingStepId,
  type StepCheckResult,
} from './types';

/** Host snapshot for Agent coverage / 主机 Agent 覆盖快照 */
export type HostAgentSnapshot = {
  id: number;
  name: string;
  isOnline: boolean;
  agentReady: boolean;
};

/** Cluster node snapshot / 集群节点快照 */
export type ClusterNodeSnapshot = {
  hostId: number;
  hostName: string;
  isOnline: boolean;
  /** SeaTunnel process running / 进程运行中 */
  processRunning: boolean;
};

/** Local package readiness / 本地安装包就绪 */
export type LocalPackageSnapshot = {
  version: string;
  isLocal: boolean;
};

/** Local plugin cache entry / 控制面本地插件缓存 */
export type LocalPluginSnapshot = {
  name: string;
  version: string;
  selectedProfileKeys?: string[];
};

/** Job instance for first-script completion / 首脚本作业实例 */
export type JobSnapshot = {
  id: number;
  clusterId?: number | null;
  status: string;
};

/** Known cluster id+name for target resolution / 已知集群用于目标解析 */
export type ClusterRef = {
  id: number;
  name?: string;
};

export type CompletionInput = {
  path: OnboardingPath;
  state: OnboardingPersistedState;
  localPackages: LocalPackageSnapshot[];
  localPlugins: LocalPluginSnapshot[];
  hosts: HostAgentSnapshot[];
  /** All known clusters (for ambiguity checks) / 全部已知集群（歧义判定） */
  clusters: ClusterRef[];
  /** Nodes of resolved target cluster; empty when unbound / 已解析目标的节点 */
  targetNodes: ClusterNodeSnapshot[];
  /**
   * Resolved target for this evaluation (may differ from persisted when unique).
   * 本次评估解析出的目标（唯一集群时可与持久化不同）。
   */
  resolvedTargetClusterId: number | null;
  /** Multiple clusters and no user-bound target / 多集群且用户未绑定目标 */
  targetAmbiguous?: boolean;
  clusterHealthyEnough: boolean;
  jobs: JobSnapshot[];
  /** Hosts list API failed — must not vacuous-complete / 主机列表失败，禁止空集误完成 */
  hostsFetchFailed?: boolean;
  /** Cluster status API failed after bind / 绑定后集群状态失败 */
  clusterStatusFailed?: boolean;
  /** Packages API failed / 安装包 API 失败 */
  packagesFetchFailed?: boolean;
  /** Plugins API failed / 插件 API 失败 */
  pluginsFetchFailed?: boolean;
};

/**
 * Normalize version strings for package↔plugin matching (trim, strip leading v).
 * 规范化版本字符串以便安装包与插件匹配（去空白、去前导 v）。
 */
export function normalizeInventoryVersion(version: string | null | undefined): string {
  return String(version || '')
    .trim()
    .replace(/^v/i, '');
}

/**
 * Find a same-version local package + plugin combo (never mix versions).
 * 查找同版本本地安装包+插件组合（禁止跨版本拼凑）。
 */
export function findReadyInventoryCombo(
  localPackages: LocalPackageSnapshot[],
  localPlugins: LocalPluginSnapshot[],
):
  | {ok: true; version: string; pluginNames: string[]}
  | {
      ok: false;
      reason: 'package_missing' | 'plugin_missing';
    } {
  const localVersions = [
    ...new Set(
      localPackages
        .filter((p) => p.isLocal && normalizeInventoryVersion(p.version))
        .map((p) => normalizeInventoryVersion(p.version)),
    ),
  ];
  if (localVersions.length === 0) {
    return {ok: false, reason: 'package_missing'};
  }
  for (const version of localVersions) {
    const plugins = localPlugins.filter(
      (p) => normalizeInventoryVersion(p.version) === version,
    );
    if (plugins.length > 0) {
      return {
        ok: true,
        version,
        pluginNames: [...new Set(plugins.map((p) => p.name))],
      };
    }
  }
  // Have local package(s) but no same-version plugin — do not mix other versions.
  // 有本地包但无同版本插件——禁止用其他版本插件凑数。
  return {ok: false, reason: 'plugin_missing'};
}

/**
 * Resolve which cluster the checklist should evaluate.
 * Never picks an arbitrary first among many; unique cluster may be used for evaluation.
 * 解析清单应评估的集群；多集群时绝不随意挑第一个；唯一集群可用于评估。
 */
export function resolveOnboardingTargetCluster(
  state: OnboardingPersistedState,
  clusters: ClusterRef[],
): {
  clusterId: number | null;
  ambiguous: boolean;
  /** Persist when user-bound id is stale / 用户绑定 id 已失效时需清理 */
  clearStaleBinding: boolean;
} {
  const ids = new Set(clusters.map((c) => c.id));
  if (state.targetClusterId != null) {
    if (ids.has(state.targetClusterId)) {
      return {
        clusterId: state.targetClusterId,
        ambiguous: false,
        clearStaleBinding: false,
      };
    }
    // Stale binding — fall through / 失效绑定，继续解析
    if (clusters.length === 1) {
      return {
        clusterId: clusters[0]!.id,
        ambiguous: false,
        clearStaleBinding: true,
      };
    }
    if (clusters.length === 0) {
      return {clusterId: null, ambiguous: false, clearStaleBinding: true};
    }
    return {clusterId: null, ambiguous: true, clearStaleBinding: true};
  }
  if (clusters.length === 0) {
    return {clusterId: null, ambiguous: false, clearStaleBinding: false};
  }
  if (clusters.length === 1) {
    // Sole cluster: evaluate against it without claiming a multi-pick.
    // 唯一集群：对其评估，不构成「多选中随意挑一个」。
    return {
      clusterId: clusters[0]!.id,
      ambiguous: false,
      clearStaleBinding: false,
    };
  }
  return {clusterId: null, ambiguous: true, clearStaleBinding: false};
}

/**
 * Step order for a path (denominator for progress). No Proxy step (AC8).
 * 路径步骤顺序（进度分母）。无 Proxy 步（AC8）。
 */
export function stepsForPath(path: OnboardingPath): OnboardingStepId[] {
  if (path === 'new') {
    return [
      ONBOARDING_STEP_IDS.resources,
      ONBOARDING_STEP_IDS.hostsAgents,
      ONBOARDING_STEP_IDS.cluster,
      ONBOARDING_STEP_IDS.firstScript,
    ];
  }
  return [
    ONBOARDING_STEP_IDS.hostsAgents,
    ONBOARDING_STEP_IDS.cluster,
    ONBOARDING_STEP_IDS.firstScript,
  ];
}

/**
 * AC6 resources: auto-detect same-version local package + plugin; no plan fields.
 * JDBC without driver profiles still counts as local plugin ready.
 * AC6 资源：自动识别同版本本地包+插件；无计划字段。JDBC 无驱动配置仍算本地插件就绪。
 */
export function evaluateResources(
  localPackages: LocalPackageSnapshot[],
  localPlugins: LocalPluginSnapshot[],
  options?: {packagesFetchFailed?: boolean; pluginsFetchFailed?: boolean},
): StepCheckResult {
  const id = ONBOARDING_STEP_IDS.resources;
  if (options?.packagesFetchFailed) {
    return {id, status: 'error', errorMessage: 'packages_fetch_failed'};
  }
  if (options?.pluginsFetchFailed) {
    return {id, status: 'error', errorMessage: 'plugins_fetch_failed'};
  }
  const combo = findReadyInventoryCombo(localPackages, localPlugins);
  if (!combo.ok) {
    return {
      id,
      status: 'incomplete',
      errorMessage: combo.reason,
    };
  }
  return {
    id,
    status: 'complete',
    meta: {
      matchedVersion: combo.version,
      pluginCount: combo.pluginNames.length,
    },
  };
}

/**
 * Host Agent coverage: all known hosts before bind; all target node hosts after bind.
 * Empty set never completes. One online + one offline ⇒ incomplete.
 * 主机 Agent：绑定前检查全部已知主机；绑定后检查目标全部节点主机。空集永不完成。
 */
export function evaluateHostsAgents(
  hosts: HostAgentSnapshot[],
  targetNodes: ClusterNodeSnapshot[],
  resolvedTargetClusterId: number | null,
  options?: {hostsFetchFailed?: boolean; clusterStatusFailed?: boolean},
): StepCheckResult {
  const id = ONBOARDING_STEP_IDS.hostsAgents;
  if (options?.hostsFetchFailed) {
    return {id, status: 'error', errorMessage: 'hosts_fetch_failed'};
  }
  if (resolvedTargetClusterId != null && options?.clusterStatusFailed) {
    return {id, status: 'error', errorMessage: 'cluster_status_failed'};
  }

  let scopeIds: number[];
  if (resolvedTargetClusterId != null) {
    // Bound/resolved target: every current node host (never fall back to unrelated hosts).
    // 已绑定/解析目标：覆盖当前全部节点主机（绝不回退无关主机）。
    scopeIds = [...new Set(targetNodes.map((n) => n.hostId))];
    if (scopeIds.length === 0) {
      return {id, status: 'incomplete', errorMessage: 'bound_nodes_empty'};
    }
  } else {
    // Pre-cluster: progress on servers the user has actually added.
    // 建集群前：按用户已添加的服务器展示进度。
    scopeIds = hosts.map((h) => h.id);
    if (scopeIds.length === 0) {
      return {id, status: 'incomplete', errorMessage: 'hosts_missing'};
    }
  }

  const byId = new Map(hosts.map((h) => [h.id, h]));
  const missing: string[] = [];
  for (const hostId of scopeIds) {
    const host = byId.get(hostId);
    if (!host) {
      missing.push(`#${hostId}`);
      continue;
    }
    if (!host.agentReady) {
      missing.push(host.name || `#${hostId}`);
    }
  }
  if (missing.length > 0) {
    return {
      id,
      status: 'incomplete',
      missing,
      errorMessage: 'hosts_not_connected',
    };
  }
  return {id, status: 'complete'};
}

/**
 * Cluster nodes process-running enough for "集群运行正常".
 * 集群节点进程就绪，对应「集群运行正常」。
 */
export function evaluateCluster(
  resolvedTargetClusterId: number | null,
  targetNodes: ClusterNodeSnapshot[],
  clusterHealthyEnough: boolean,
  options?: {
    clusterStatusFailed?: boolean;
    targetAmbiguous?: boolean;
  },
): StepCheckResult {
  const id = ONBOARDING_STEP_IDS.cluster;
  if (options?.targetAmbiguous) {
    return {id, status: 'incomplete', errorMessage: 'cluster_ambiguous'};
  }
  if (resolvedTargetClusterId == null) {
    return {id, status: 'incomplete', errorMessage: 'target_unbound'};
  }
  if (options?.clusterStatusFailed) {
    return {id, status: 'error', errorMessage: 'cluster_status_failed'};
  }
  if (targetNodes.length === 0) {
    return {id, status: 'incomplete', errorMessage: 'nodes_empty'};
  }
  const allRunning = targetNodes.every((n) => n.processRunning && n.isOnline);
  const notRunning = targetNodes.filter((n) => !n.processRunning || !n.isOnline);
  if (!allRunning || notRunning.length > 0 || !clusterHealthyEnough) {
    return {
      id,
      status: 'incomplete',
      missing: notRunning.map((n) => n.hostName || `#${n.hostId}`),
      errorMessage: 'nodes_not_ready',
    };
  }
  return {id, status: 'complete'};
}

/**
 * First script: job on resolved target reaches running or success.
 * 首脚本：解析目标上的作业达到 running/success。
 */
export function evaluateFirstScript(
  resolvedTargetClusterId: number | null,
  jobs: JobSnapshot[],
  options?: {targetAmbiguous?: boolean},
): StepCheckResult {
  const id = ONBOARDING_STEP_IDS.firstScript;
  if (options?.targetAmbiguous) {
    return {id, status: 'incomplete', errorMessage: 'cluster_ambiguous'};
  }
  if (resolvedTargetClusterId == null) {
    return {id, status: 'incomplete', errorMessage: 'target_unbound'};
  }
  const ok = jobs.some((job) => {
    if (job.clusterId !== resolvedTargetClusterId) {
      return false;
    }
    const status = (job.status || '').toLowerCase();
    return status === 'running' || status === 'success';
  });
  if (!ok) {
    return {id, status: 'incomplete', errorMessage: 'job_not_running_or_success'};
  }
  return {id, status: 'complete'};
}

/**
 * Aggregate all step checks for the selected path.
 * 按路径汇总全部步骤检查。
 */
export function evaluateAllSteps(input: CompletionInput): StepCheckResult[] {
  const ids = stepsForPath(input.path);
  return ids.map((stepId) => {
    switch (stepId) {
      case ONBOARDING_STEP_IDS.resources:
        return evaluateResources(input.localPackages, input.localPlugins, {
          packagesFetchFailed: input.packagesFetchFailed,
          pluginsFetchFailed: input.pluginsFetchFailed,
        });
      case ONBOARDING_STEP_IDS.hostsAgents:
        return evaluateHostsAgents(
          input.hosts,
          input.targetNodes,
          input.resolvedTargetClusterId,
          {
            hostsFetchFailed: input.hostsFetchFailed,
            clusterStatusFailed: input.clusterStatusFailed,
          },
        );
      case ONBOARDING_STEP_IDS.cluster:
        return evaluateCluster(
          input.resolvedTargetClusterId,
          input.targetNodes,
          input.clusterHealthyEnough,
          {
            clusterStatusFailed: input.clusterStatusFailed,
            targetAmbiguous: input.targetAmbiguous,
          },
        );
      case ONBOARDING_STEP_IDS.firstScript:
        return evaluateFirstScript(
          input.resolvedTargetClusterId,
          input.jobs,
          {targetAmbiguous: input.targetAmbiguous},
        );
      default:
        return {id: stepId, status: 'incomplete'};
    }
  });
}

/**
 * Completed step ids from evaluation results.
 * 从检查结果提取已完成步骤 id。
 */
export function completedIdsFromResults(results: StepCheckResult[]): string[] {
  return results.filter((r) => r.status === 'complete').map((r) => r.id);
}

/**
 * Whether every path step is complete → candidate for finished latch.
 * 路径步骤是否全部完成（可闩锁）。
 */
export function isPathFullyComplete(results: StepCheckResult[]): boolean {
  return (
    results.length > 0 &&
    results.every((r) => r.status === 'complete') &&
    !results.some((r) => r.status === 'error')
  );
}
