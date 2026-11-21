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
 * 集群详情页 6 个一级 Tab 与子视角路由解析（含全部历史 ?tab= 深链兼容映射）。
 * Cluster detail 6-tab & sub-view route resolver with full legacy ?tab= compatibility.
 */

export type ClusterDetailTab =
  | 'overview'
  | 'nodes'
  | 'configs'
  | 'plugins'
  | 'runtime'
  | 'ops';

export type RuntimeSubView = 'checkpoint' | 'imap' | 'proxy' | 'webui';
export type OpsSubView = 'guard' | 'diagnostics' | 'upgrades';

export const CLUSTER_DETAIL_TABS: readonly ClusterDetailTab[] = [
  'overview',
  'nodes',
  'configs',
  'plugins',
  'runtime',
  'ops',
] as const;

export interface ResolvedClusterDetailRoute {
  tab: ClusterDetailTab;
  runtimeView: RuntimeSubView;
  opsView: OpsSubView;
  /** 是否命中了旧 URL 参数并需要无感归一化。Whether a legacy param matched and should normalize. */
  isLegacy: boolean;
}

function resolveRuntimeView(
  view: string | null,
  hasWebUI: boolean,
): RuntimeSubView {
  if (view === 'imap' || view === 'proxy') {
    return view;
  }
  if (view === 'webui' && hasWebUI) {
    return 'webui';
  }
  return 'checkpoint';
}

function resolveOpsView(view: string | null): OpsSubView {
  if (view === 'diagnostics' || view === 'upgrades') {
    return view;
  }
  return 'guard';
}

/**
 * 将 URL 查询参数解析为 6 个规范一级 Tab + 子视角；旧链接无损映射：
 * storage → runtime/checkpoint, proxy → runtime/proxy, webui → runtime/webui,
 * monitoring → ops/guard, diagnostics → ops/diagnostics, upgrades → ops/upgrades。
 * Resolve URL params to the 6 canonical tabs + sub-views with legacy deep-link mapping.
 */
export function resolveClusterDetailRoute(
  tabParam: string | null,
  viewParam: string | null,
  hasWebUI = false,
): ResolvedClusterDetailRoute {
  switch (tabParam) {
    case 'overview':
    case 'nodes':
    case 'configs':
    case 'plugins':
      return {
        tab: tabParam,
        runtimeView: resolveRuntimeView(viewParam, hasWebUI),
        opsView: resolveOpsView(viewParam),
        isLegacy: false,
      };
    case 'runtime':
      return {
        tab: 'runtime',
        runtimeView: resolveRuntimeView(viewParam, hasWebUI),
        opsView: 'guard',
        isLegacy: false,
      };
    case 'ops':
      return {
        tab: 'ops',
        runtimeView: 'checkpoint',
        opsView: resolveOpsView(viewParam),
        isLegacy: false,
      };
    // 历史深链兼容 / Legacy deep-link mapping
    case 'storage':
      return {
        tab: 'runtime',
        runtimeView: viewParam === 'imap' ? 'imap' : 'checkpoint',
        opsView: 'guard',
        isLegacy: true,
      };
    case 'proxy':
      return {
        tab: 'runtime',
        runtimeView: 'proxy',
        opsView: 'guard',
        isLegacy: true,
      };
    case 'webui':
      return {
        tab: 'runtime',
        runtimeView: hasWebUI ? 'webui' : 'checkpoint',
        opsView: 'guard',
        isLegacy: true,
      };
    case 'monitoring':
      return {
        tab: 'ops',
        runtimeView: 'checkpoint',
        opsView: 'guard',
        isLegacy: true,
      };
    case 'diagnostics':
      return {
        tab: 'ops',
        runtimeView: 'checkpoint',
        opsView: 'diagnostics',
        isLegacy: true,
      };
    case 'upgrades':
      return {
        tab: 'ops',
        runtimeView: 'checkpoint',
        opsView: 'upgrades',
        isLegacy: true,
      };
    default:
      return {
        tab: 'overview',
        runtimeView: 'checkpoint',
        opsView: 'guard',
        isLegacy: false,
      };
  }
}
