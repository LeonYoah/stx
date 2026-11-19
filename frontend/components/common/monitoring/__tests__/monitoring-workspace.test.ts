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

// 深链映射とアラート一覧のクエリ構築。design.md §9.1 の旧URLマッピングを担保する。
// Deep-link mapping and alert-list query construction, covering design.md §9.1 legacy URLs.

import {beforeEach, describe, expect, it} from 'vitest';
import {resolveMonitoringTab} from '../MonitoringWorkspace';
import {
  applyAlertFilterChange,
  buildAlertInstanceParams,
  type AlertFilters,
} from '../alerts/AlertsPanel';

describe('resolveMonitoringTab', () => {
  it('maps the three canonical tabs', () => {
    expect(resolveMonitoringTab('alerts', null)).toBe('alerts');
    expect(resolveMonitoringTab('rules', null)).toBe('rules');
    expect(resolveMonitoringTab('notify', null)).toBe('notify');
  });

  it('maps legacy tabs: policies → rules, history/integrations/notifications → notify', () => {
    expect(resolveMonitoringTab('policies', null)).toBe('rules');
    expect(resolveMonitoringTab('history', null)).toBe('notify');
    expect(resolveMonitoringTab('integrations', null)).toBe('notify');
    expect(resolveMonitoringTab('notifications', null)).toBe('notify');
  });

  it('maps legacy section=* under policies', () => {
    expect(resolveMonitoringTab('policies', 'rules')).toBe('rules');
    expect(resolveMonitoringTab('policies', 'history')).toBe('notify');
    expect(resolveMonitoringTab('policies', 'deliveries')).toBe('notify');
    expect(resolveMonitoringTab('policies', 'channels')).toBe('notify');
  });

  it('maps bare section deep links and defaults to alerts', () => {
    expect(resolveMonitoringTab(null, 'history')).toBe('notify');
    expect(resolveMonitoringTab(null, 'channels')).toBe('notify');
    expect(resolveMonitoringTab(null, null)).toBe('alerts');
    expect(resolveMonitoringTab('bogus', null)).toBe('alerts');
  });
});

const base: AlertFilters = {
  scope: 'all',
  status: '',
  severity: '',
  clusterId: '',
  query: '',
  page: 1,
  startTime: '',
  endTime: '',
};

describe('buildAlertInstanceParams', () => {
  it('sends scope only for cluster/job', () => {
    expect(buildAlertInstanceParams({...base, scope: 'cluster'}).scope).toBe('cluster');
    expect(buildAlertInstanceParams({...base, scope: 'job'}).scope).toBe('job');
    expect(buildAlertInstanceParams(base).scope).toBeUndefined();
  });

  it('maps acknowledged/silenced to handling_status + status=firing', () => {
    const p = buildAlertInstanceParams({...base, status: 'acknowledged'});
    expect(p.handling_status).toBe('acknowledged');
    expect(p.status).toBe('firing');
    const s = buildAlertInstanceParams({...base, status: 'silenced'});
    expect(s.handling_status).toBe('silenced');
  });

  it('passes resolved/closed/firing as display status and severity/cluster_id/page', () => {
    const p = buildAlertInstanceParams({
      ...base,
      status: 'resolved',
      severity: 'critical',
      clusterId: '11',
      page: 3,
    });
    expect(p.status).toBe('resolved');
    expect(p.severity).toBe('critical');
    expect(p.cluster_id).toBe('11');
    expect(p.page).toBe(3);
    expect(p.page_size).toBeGreaterThan(0);
  });

  it('converts wall-time start/end inputs to RFC3339 params', () => {
    const p = buildAlertInstanceParams({
      ...base,
      startTime: '2026-01-02T10:00',
      endTime: '2026-01-02T12:30',
    });
    expect(p.start_time).toBeTruthy();
    expect(p.end_time).toBeTruthy();
    expect(new Date(p.start_time!).getTime()).toBeLessThan(new Date(p.end_time!).getTime());
    const bare = buildAlertInstanceParams(base);
    expect(bare.start_time).toBeUndefined();
    expect(bare.end_time).toBeUndefined();
  });
});

describe('applyAlertFilterChange', () => {
  it('resets page to 1 when a filter changes', () => {
    const next = applyAlertFilterChange({...base, page: 5}, {severity: 'critical'}, true);
    expect(next.page).toBe(1);
    expect(next.severity).toBe('critical');
  });

  it('keeps the page for pure pagination', () => {
    const next = applyAlertFilterChange({...base, page: 2}, {page: 4}, false);
    expect(next.page).toBe(4);
  });
});

import {findAlertMemories} from '../alerts/AlertsPanel';
import type {AlertInstance} from '@/lib/services/monitoring';
import type {TroubleshootingMemoryEntry} from '@/lib/services/troubleshooting';

const MEMORY_KEY = 'stx_troubleshooting_memories';

function seedMemories(entries: Partial<TroubleshootingMemoryEntry>[]) {
  const now = new Date('2026-11-19T00:00:00Z').toISOString();
  localStorage.setItem(
    MEMORY_KEY,
    JSON.stringify(
      entries.map((e, i) => ({
        id: `mem-${i}`,
        target_type: 'alert',
        fingerprint: '',
        title: '',
        solution: '',
        created_at: now,
        updated_at: now,
        ...e,
      })),
    ),
  );
}

function alertWith(patch: Partial<AlertInstance>): AlertInstance {
  return {
    alert_id: 'remote:fp:1',
    source_type: 'metric',
    cluster_id: '6',
    cluster_name: 'c1',
    severity: 'critical',
    alert_name: '节点离线',
    rule_key: 'node_offline',
    summary: '',
    description: '',
    status: 'firing',
    lifecycle_status: 'firing',
    handling_status: 'none',
    created_at: '2026-11-19T00:00:00Z',
    firing_at: '2026-11-19T00:00:00Z',
    last_seen_at: '2026-11-19T00:00:00Z',
    ...patch,
  } as AlertInstance;
}

describe('findAlertMemories', () => {
  beforeEach(() => localStorage.removeItem(MEMORY_KEY));

  it('matches memories saved under the catalog rule key', () => {
    seedMemories([{fingerprint: 'node_offline', title: 'node offline runbook'}]);
    expect(findAlertMemories(alertWith({}))).toHaveLength(1);
  });

  it('falls back to original_alert_name for memories saved under the legacy YAML name', () => {
    seedMemories([{fingerprint: 'seatunnelmetricsendpointdown', title: 'legacy runbook'}]);
    const alert = alertWith({
      source_ref: {original_alert_name: 'SeaTunnelMetricsEndpointDown'},
    });
    const matches = findAlertMemories(alert);
    expect(matches).toHaveLength(1);
    expect(matches[0].title).toBe('legacy runbook');
  });

  it('prefers the catalog key over the legacy fallback', () => {
    seedMemories([
      {fingerprint: 'node_offline', title: 'catalog runbook'},
      {fingerprint: 'seatunnelmetricsendpointdown', title: 'legacy runbook'},
    ]);
    const alert = alertWith({
      source_ref: {original_alert_name: 'SeaTunnelMetricsEndpointDown'},
    });
    expect(findAlertMemories(alert)[0].title).toBe('catalog runbook');
  });

  it('returns empty when neither fingerprint matches', () => {
    seedMemories([{fingerprint: 'unrelated', title: 'other'}]);
    expect(findAlertMemories(alertWith({}))).toHaveLength(0);
  });
});
