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
  CLUSTER_DETAIL_TABS,
  getLegacyClusterRouteRewrite,
  resolveClusterDetailRoute,
  type ResolvedClusterDetailRoute,
} from '../detail/cluster-detail-tabs';

/**
 * 模拟 ClusterDetail 冷开时序：先以未知可用性解析，再按 rewrite 助手写 URL，最后再以真实可用性解析。
 * Simulates ClusterDetail cold-load: resolve with unknown availability, optionally rewrite, then re-resolve.
 */
function simulateColdLoadRoute(args: {
  initialTab: string | null;
  initialView: string | null;
  afterLoadHasWebUI: boolean;
}): {
  urlAfterPending: {tab: string | null; view: string | null};
  urlAfterReady: {tab: string | null; view: string | null};
  viewAfterReady: ResolvedClusterDetailRoute['runtimeView'];
  opsAfterReady: ResolvedClusterDetailRoute['opsView'];
} {
  let tab = args.initialTab;
  let view = args.initialView;

  const pendingResolved = resolveClusterDetailRoute(tab, view, false);
  const pendingRewrite = getLegacyClusterRouteRewrite(tab, pendingResolved, {
    clusterDataReady: false,
  });
  if (pendingRewrite) {
    tab = pendingRewrite.tab;
    view = pendingRewrite.view;
  }
  const urlAfterPending = {tab, view};

  const readyResolved = resolveClusterDetailRoute(
    tab,
    view,
    args.afterLoadHasWebUI,
  );
  const readyRewrite = getLegacyClusterRouteRewrite(tab, readyResolved, {
    clusterDataReady: true,
  });
  if (readyRewrite) {
    tab = readyRewrite.tab;
    view = readyRewrite.view;
  }

  const finalResolved = resolveClusterDetailRoute(
    tab,
    view,
    args.afterLoadHasWebUI,
  );
  return {
    urlAfterPending,
    urlAfterReady: {tab, view},
    viewAfterReady: finalResolved.runtimeView,
    opsAfterReady: finalResolved.opsView,
  };
}

describe('resolveClusterDetailRoute', () => {
  it('exposes strictly 6 canonical primary tabs', () => {
    expect(CLUSTER_DETAIL_TABS).toEqual([
      'overview',
      'nodes',
      'configs',
      'plugins',
      'runtime',
      'ops',
    ]);
  });

  it('maps legacy tab deep links to runtime/ops sub-views losslessly', () => {
    expect(resolveClusterDetailRoute('storage', null, false)).toEqual({
      tab: 'runtime',
      runtimeView: 'checkpoint',
      opsView: 'guard',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('storage', 'imap', false)).toEqual({
      tab: 'runtime',
      runtimeView: 'imap',
      opsView: 'guard',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('proxy', null, false)).toEqual({
      tab: 'runtime',
      runtimeView: 'proxy',
      opsView: 'guard',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('webui', null, true)).toEqual({
      tab: 'runtime',
      runtimeView: 'webui',
      opsView: 'guard',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('webui', null, false)).toEqual({
      tab: 'runtime',
      runtimeView: 'checkpoint',
      opsView: 'guard',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('monitoring', null, false)).toEqual({
      tab: 'ops',
      runtimeView: 'checkpoint',
      opsView: 'guard',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('diagnostics', null, false)).toEqual({
      tab: 'ops',
      runtimeView: 'checkpoint',
      opsView: 'diagnostics',
      isLegacy: true,
    });
    expect(resolveClusterDetailRoute('upgrades', null, false)).toEqual({
      tab: 'ops',
      runtimeView: 'checkpoint',
      opsView: 'upgrades',
      isLegacy: true,
    });
  });

  it('preserves canonical tabs and falls back unknown values to overview', () => {
    expect(resolveClusterDetailRoute('runtime', 'proxy', false)).toEqual({
      tab: 'runtime',
      runtimeView: 'proxy',
      opsView: 'guard',
      isLegacy: false,
    });
    expect(resolveClusterDetailRoute('ops', 'upgrades', false)).toEqual({
      tab: 'ops',
      runtimeView: 'checkpoint',
      opsView: 'upgrades',
      isLegacy: false,
    });
    expect(resolveClusterDetailRoute('unknown', null, false).tab).toBe(
      'overview',
    );
  });
});

describe('getLegacyClusterRouteRewrite / cold-load Web UI deep link', () => {
  it('defers rewriting legacy ?tab=webui until cluster+nodes are ready', () => {
    const pending = resolveClusterDetailRoute('webui', null, false);
    expect(
      getLegacyClusterRouteRewrite('webui', pending, {
        clusterDataReady: false,
      }),
    ).toBeNull();
  });

  it('keeps supported Web UI intent across cold load then normalizes to runtime/webui', () => {
    const result = simulateColdLoadRoute({
      initialTab: 'webui',
      initialView: null,
      afterLoadHasWebUI: true,
    });
    expect(result.urlAfterPending).toEqual({tab: 'webui', view: null});
    expect(result.urlAfterReady).toEqual({tab: 'runtime', view: 'webui'});
    expect(result.viewAfterReady).toBe('webui');
  });

  it('falls back to checkpoint only after load when Web UI is definitively unavailable', () => {
    const result = simulateColdLoadRoute({
      initialTab: 'webui',
      initialView: null,
      afterLoadHasWebUI: false,
    });
    expect(result.urlAfterPending).toEqual({tab: 'webui', view: null});
    expect(result.urlAfterReady).toEqual({
      tab: 'runtime',
      view: 'checkpoint',
    });
    expect(result.viewAfterReady).toBe('checkpoint');
  });

  it('does not rewrite canonical ?tab=runtime&view=webui and recovers after load', () => {
    const result = simulateColdLoadRoute({
      initialTab: 'runtime',
      initialView: 'webui',
      afterLoadHasWebUI: true,
    });
    expect(result.urlAfterPending).toEqual({
      tab: 'runtime',
      view: 'webui',
    });
    expect(result.urlAfterReady).toEqual({tab: 'runtime', view: 'webui'});
    expect(result.viewAfterReady).toBe('webui');
  });

  it('still normalizes other legacy tabs immediately without waiting for nodes', () => {
    const storagePending = resolveClusterDetailRoute('storage', null, false);
    expect(
      getLegacyClusterRouteRewrite('storage', storagePending, {
        clusterDataReady: false,
      }),
    ).toEqual({tab: 'runtime', view: 'checkpoint'});

    const proxyPending = resolveClusterDetailRoute('proxy', null, false);
    expect(
      getLegacyClusterRouteRewrite('proxy', proxyPending, {
        clusterDataReady: false,
      }),
    ).toEqual({tab: 'runtime', view: 'proxy'});

    const monitoringPending = resolveClusterDetailRoute(
      'monitoring',
      null,
      false,
    );
    expect(
      getLegacyClusterRouteRewrite('monitoring', monitoringPending, {
        clusterDataReady: false,
      }),
    ).toEqual({tab: 'ops', view: 'guard'});

    const diagnostics = simulateColdLoadRoute({
      initialTab: 'diagnostics',
      initialView: null,
      afterLoadHasWebUI: false,
    });
    expect(diagnostics.urlAfterPending).toEqual({
      tab: 'ops',
      view: 'diagnostics',
    });
    expect(diagnostics.opsAfterReady).toBe('diagnostics');
  });
});
