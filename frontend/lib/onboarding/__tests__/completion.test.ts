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
  evaluateAllSteps,
  evaluateCluster,
  evaluateFirstScript,
  evaluateHostsAgents,
  evaluateResources,
  findReadyInventoryCombo,
  isHostAgentReady,
  normalizeInventoryVersion,
  resolveOnboardingTargetCluster,
  shouldBindOnboardingTargetCluster,
  shouldEvaluateOnboardingProgress,
  stepsForPath,
} from '../completion';
import {DEFAULT_ONBOARDING_STATE, ONBOARDING_STEP_IDS} from '../types';

describe('stepsForPath (#109 simplified)', () => {
  it('new path has 4 steps without proxy; existing has 3', () => {
    expect(stepsForPath('new')).toEqual([
      ONBOARDING_STEP_IDS.resources,
      ONBOARDING_STEP_IDS.hostsAgents,
      ONBOARDING_STEP_IDS.cluster,
      ONBOARDING_STEP_IDS.firstScript,
    ]);
    expect(stepsForPath('existing')).toEqual([
      ONBOARDING_STEP_IDS.hostsAgents,
      ONBOARDING_STEP_IDS.cluster,
      ONBOARDING_STEP_IDS.firstScript,
    ]);
    expect(stepsForPath('new')).not.toContain('proxy' as never);
  });
});

describe('findReadyInventoryCombo AC6', () => {
  it('completes when local 3.0.0 package and jdbc plugin match (t3 case)', () => {
    const combo = findReadyInventoryCombo(
      [{version: '3.0.0', isLocal: true}],
      [{name: 'jdbc', version: '3.0.0'}],
    );
    expect(combo.ok).toBe(true);
    if (combo.ok) {
      expect(combo.version).toBe('3.0.0');
      expect(combo.pluginNames).toContain('jdbc');
    }
  });

  it('allows jdbc without profile keys', () => {
    const result = evaluateResources(
      [{version: '3.0.0', isLocal: true}],
      [{name: 'jdbc', version: '3.0.0', selectedProfileKeys: []}],
    );
    expect(result.status).toBe('complete');
  });

  it('reports package_missing when no local package', () => {
    expect(
      findReadyInventoryCombo([], [{name: 'jdbc', version: '3.0.0'}]).ok,
    ).toBe(false);
    expect(
      evaluateResources([], [{name: 'jdbc', version: '3.0.0'}]).errorMessage,
    ).toBe('package_missing');
  });

  it('reports plugin_missing when package exists but only other-version plugins', () => {
    const combo = findReadyInventoryCombo(
      [{version: '3.0.0', isLocal: true}],
      [{name: 'jdbc', version: '2.3.12'}],
    );
    expect(combo).toEqual({ok: false, reason: 'plugin_missing'});
  });

  it('does not mix package 3.0.0 with plugin 2.3.12', () => {
    const result = evaluateResources(
      [
        {version: '3.0.0', isLocal: true},
        {version: '2.3.12', isLocal: false},
      ],
      [{name: 'jdbc', version: '2.3.12'}],
    );
    expect(result.status).toBe('incomplete');
    expect(result.errorMessage).toBe('plugin_missing');
  });

  it('normalizes leading v for matching', () => {
    expect(normalizeInventoryVersion('v3.0.0')).toBe('3.0.0');
    const combo = findReadyInventoryCombo(
      [{version: 'v3.0.0', isLocal: true}],
      [{name: 'jdbc', version: '3.0.0'}],
    );
    expect(combo.ok).toBe(true);
  });

  it('ignores remote-only catalog packages', () => {
    const result = evaluateResources(
      [{version: '3.0.0', isLocal: false}],
      [{name: 'jdbc', version: '3.0.0'}],
    );
    expect(result.errorMessage).toBe('package_missing');
  });
});

describe('isHostAgentReady AC7', () => {
  it('requires installed AND online together', () => {
    expect(
      isHostAgentReady({is_online: true, agent_status: 'installed'}),
    ).toBe(true);
    expect(
      isHostAgentReady({is_online: false, agent_status: 'installed'}),
    ).toBe(false);
    expect(
      isHostAgentReady({is_online: true, agent_status: 'not_installed'}),
    ).toBe(false);
    expect(
      isHostAgentReady({status: 'connected', agent_status: 'installed'}),
    ).toBe(true);
  });
});

describe('evaluateHostsAgents AC7', () => {
  it('incomplete with no hosts before cluster', () => {
    expect(
      evaluateHostsAgents([], [], null).errorMessage,
    ).toBe('hosts_missing');
  });

  it('completes when every managed host is online and ready', () => {
    const hosts = [
      {
        id: 1,
        name: 'alpha',
        isOnline: true,
        agentReady: isHostAgentReady({
          is_online: true,
          agent_status: 'installed',
        }),
      },
      {
        id: 2,
        name: 'beta',
        isOnline: true,
        agentReady: isHostAgentReady({
          is_online: true,
          agent_status: 'installed',
        }),
      },
    ];
    const result = evaluateHostsAgents(hosts, [], null);
    expect(result.status).toBe('complete');
    expect(result.missing).toBeUndefined();
  });

  it('stays incomplete and names the offline host when one of many is down', () => {
    const hosts = [
      {
        id: 1,
        name: 'alpha',
        isOnline: true,
        agentReady: isHostAgentReady({
          is_online: true,
          agent_status: 'installed',
        }),
      },
      {
        id: 2,
        name: 'beta',
        isOnline: false,
        // Bug repro: installed but offline must NOT be ready.
        // 缺陷复现：已安装但离线不得算就绪。
        agentReady: isHostAgentReady({
          is_online: false,
          agent_status: 'installed',
        }),
      },
    ];
    const result = evaluateHostsAgents(hosts, [], null);
    expect(result.status).toBe('incomplete');
    expect(result.errorMessage).toBe('hosts_not_connected');
    expect(result.missing).toEqual(['beta']);
  });

  it('lists every host name when all managed hosts are offline', () => {
    const hosts = [
      {
        id: 1,
        name: 'alpha',
        isOnline: false,
        agentReady: isHostAgentReady({
          is_online: false,
          agent_status: 'installed',
        }),
      },
      {
        id: 2,
        name: 'beta',
        isOnline: false,
        agentReady: isHostAgentReady({
          is_online: false,
          agent_status: 'installed',
        }),
      },
    ];
    const result = evaluateHostsAgents(hosts, [], null);
    expect(result.status).toBe('incomplete');
    expect(result.errorMessage).toBe('hosts_not_connected');
    expect(result.missing).toEqual(['alpha', 'beta']);
  });

  it('after bind checks all node hosts, not unrelated hosts', () => {
    const result = evaluateHostsAgents(
      [
        {id: 1, name: 'a', isOnline: true, agentReady: true},
        {id: 2, name: 'b', isOnline: true, agentReady: true},
        {id: 9, name: 'unrelated', isOnline: false, agentReady: false},
      ],
      [
        {hostId: 1, hostName: 'a', isOnline: true, processRunning: true},
        {hostId: 2, hostName: 'b', isOnline: true, processRunning: true},
      ],
      7,
    );
    expect(result.status).toBe('complete');
  });

  it('two-node cluster with one agent missing stays incomplete', () => {
    const result = evaluateHostsAgents(
      [
        {id: 1, name: 'a', isOnline: true, agentReady: true},
        {id: 2, name: 'b', isOnline: false, agentReady: false},
      ],
      [
        {hostId: 1, hostName: 'a', isOnline: true, processRunning: true},
        {hostId: 2, hostName: 'b', isOnline: false, processRunning: false},
      ],
      7,
    );
    expect(result.status).toBe('incomplete');
    expect(result.missing).toContain('b');
  });
});

describe('shouldBindOnboardingTargetCluster AC1 loop guard', () => {
  it('binds a new positive id once and no-ops when already bound', () => {
    expect(shouldBindOnboardingTargetCluster(null, 7)).toBe(true);
    expect(shouldBindOnboardingTargetCluster(7, 7)).toBe(false);
    expect(shouldBindOnboardingTargetCluster(7, 9)).toBe(true);
    expect(shouldBindOnboardingTargetCluster(null, 0)).toBe(false);
    expect(shouldBindOnboardingTargetCluster(null, Number.NaN)).toBe(false);
  });
});

describe('finished latch AC4 vs AC7 offline', () => {
  it('does not re-evaluate progress after finished even if Agents go offline', () => {
    const finishedState = {
      ...DEFAULT_ONBOARDING_STATE,
      path: 'existing' as const,
      finished: true,
      displayMode: 'hidden' as const,
      completedStepIds: [
        ONBOARDING_STEP_IDS.hostsAgents,
        ONBOARDING_STEP_IDS.cluster,
        ONBOARDING_STEP_IDS.firstScript,
      ],
    };
    // AC7 would mark hosts incomplete if evaluated — but AC4 latch blocks evaluation.
    // 若仍评估，AC7 会判主机未完成；AC4 闩锁禁止评估，避免引导回退/重现。
    const offlineEval = evaluateHostsAgents(
      [
        {
          id: 1,
          name: 'alpha',
          isOnline: false,
          agentReady: isHostAgentReady({
            is_online: false,
            agent_status: 'installed',
          }),
        },
      ],
      [],
      null,
    );
    expect(offlineEval.status).toBe('incomplete');
    expect(shouldEvaluateOnboardingProgress(finishedState)).toBe(false);
    expect(
      shouldEvaluateOnboardingProgress({
        ...finishedState,
        finished: false,
      }),
    ).toBe(true);
  });
});

describe('resolveOnboardingTargetCluster AC1', () => {
  it('uses user-bound target when still present', () => {
    const r = resolveOnboardingTargetCluster(
      {...DEFAULT_ONBOARDING_STATE, targetClusterId: 5},
      [
        {id: 5, name: 'a'},
        {id: 9, name: 'b'},
      ],
    );
    expect(r).toEqual({
      clusterId: 5,
      ambiguous: false,
      clearStaleBinding: false,
    });
  });

  it('marks ambiguous when multiple clusters and no binding', () => {
    const r = resolveOnboardingTargetCluster(DEFAULT_ONBOARDING_STATE, [
      {id: 1},
      {id: 2},
    ]);
    expect(r.ambiguous).toBe(true);
    expect(r.clusterId).toBeNull();
  });

  it('uses the sole cluster for evaluation without multi-pick', () => {
    const r = resolveOnboardingTargetCluster(DEFAULT_ONBOARDING_STATE, [
      {id: 42, name: 'only'},
    ]);
    expect(r.clusterId).toBe(42);
    expect(r.ambiguous).toBe(false);
  });
});

describe('evaluateCluster / firstScript', () => {
  it('does not complete cluster on empty nodes', () => {
    expect(
      evaluateCluster(1, [], true).errorMessage,
    ).toBe('nodes_empty');
  });

  it('rejects null and other-cluster jobs', () => {
    expect(
      evaluateFirstScript(3, [{id: 1, clusterId: null, status: 'success'}])
        .status,
    ).toBe('incomplete');
    expect(
      evaluateFirstScript(3, [{id: 1, clusterId: 99, status: 'running'}])
        .status,
    ).toBe('incomplete');
    expect(
      evaluateFirstScript(3, [{id: 1, clusterId: 3, status: 'running'}])
        .status,
    ).toBe('complete');
  });

  it('reports ambiguity instead of picking a cluster', () => {
    expect(
      evaluateCluster(null, [], false, {targetAmbiguous: true}).errorMessage,
    ).toBe('cluster_ambiguous');
    expect(
      evaluateFirstScript(null, [], {targetAmbiguous: true}).errorMessage,
    ).toBe('cluster_ambiguous');
  });
});

describe('evaluateAllSteps path denominator', () => {
  it('existing path never includes resources or proxy', () => {
    const results = evaluateAllSteps({
      path: 'existing',
      state: DEFAULT_ONBOARDING_STATE,
      localPackages: [{version: '3.0.0', isLocal: true}],
      localPlugins: [{name: 'jdbc', version: '3.0.0'}],
      hosts: [{id: 1, name: 'h1', isOnline: true, agentReady: true}],
      clusters: [{id: 1}],
      targetNodes: [
        {hostId: 1, hostName: 'h1', isOnline: true, processRunning: true},
      ],
      resolvedTargetClusterId: 1,
      clusterHealthyEnough: true,
      jobs: [{id: 1, clusterId: 1, status: 'running'}],
    });
    expect(results.map((r) => r.id)).toEqual([
      ONBOARDING_STEP_IDS.hostsAgents,
      ONBOARDING_STEP_IDS.cluster,
      ONBOARDING_STEP_IDS.firstScript,
    ]);
    expect(results.every((r) => r.status === 'complete')).toBe(true);
  });

  it('new path completes resources from inventory without plan fields', () => {
    const results = evaluateAllSteps({
      path: 'new',
      state: DEFAULT_ONBOARDING_STATE,
      localPackages: [{version: '3.0.0', isLocal: true}],
      localPlugins: [{name: 'jdbc', version: '3.0.0'}],
      hosts: [{id: 1, name: 'h1', isOnline: true, agentReady: true}],
      clusters: [{id: 1}],
      targetNodes: [
        {hostId: 1, hostName: 'h1', isOnline: true, processRunning: true},
      ],
      resolvedTargetClusterId: 1,
      clusterHealthyEnough: true,
      jobs: [{id: 1, clusterId: 1, status: 'success'}],
    });
    expect(results).toHaveLength(4);
    expect(results.every((r) => r.status === 'complete')).toBe(true);
  });
});
