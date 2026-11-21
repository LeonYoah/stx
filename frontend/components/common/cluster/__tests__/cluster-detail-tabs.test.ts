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
  resolveClusterDetailRoute,
} from '../detail/cluster-detail-tabs';

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
