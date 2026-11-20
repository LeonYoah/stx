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

import {fireEvent, render, screen, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {
  HomeClusterDashboard,
  HomeClusterDetail,
} from '@/lib/services/dashboard';
import {ClusterTab} from '../ClusterTab';
import type {DashboardQuery} from '../use-dashboard-query';

const home = vi.hoisted(() => ({
  getClusters: vi.fn(),
  getClustersDetail: vi.fn(),
}));
vi.mock('@/lib/services/dashboard', async () => {
  const actual = await vi.importActual<
    typeof import('@/lib/services/dashboard')
  >('@/lib/services/dashboard');
  return {
    ...actual,
    HomeService: {
      ...actual.HomeService,
      getClusters: home.getClusters,
      getClustersDetail: home.getClustersDetail,
    },
  };
});
vi.mock('@/components/arc/line-chart/line-chart', () => ({
  LineChart: ({label, series}: {label: string; series: {key: string}[]}) => (
    <div data-testid='line-chart'>
      {label}:{series.map((item) => item.key).join(',')}
    </div>
  ),
}));
vi.mock('@/components/charts/composition-chart', () => ({
  CompositionChart: ({series}: {series: {id: string}[]}) => (
    <div data-testid='composition'>
      {series.map((item) => item.id).join(',')}
    </div>
  ),
}));
vi.mock('@/components/charts/status-bar', () => ({
  StatusBar: ({label, data}: {label: string; data: {status: string}[]}) => (
    <div data-testid='status-bar'>
      {label}:{data.map((item) => item.status).join(',')}
    </div>
  ),
  StatusBarPlot: () => null,
  StatusBarTooltip: () => null,
}));

const dashboard = (source: 'prometheus' | 'none'): HomeClusterDashboard => ({
  source,
  reason: source === 'none' ? 'observability_disabled' : undefined,
  range: '1h',
  start_ms: 0,
  end_ms: 3_600_000,
  step_seconds: 15,
  cell_seconds: 75,
  clusters: [
    {
      id: 1,
      name: 'prod',
      version: '3.0.0',
      health_status: 'healthy',
      nodes_online: 2,
      nodes_total: 2,
      running_jobs: 3,
      active_alerts: 0,
      critical_alerts: 0,
      max_heap_percent: 71,
    },
    {
      id: 2,
      name: 'single',
      version: '3.0.0',
      health_status: 'unhealthy',
      nodes_online: 1,
      nodes_total: 1,
      running_jobs: 0,
      active_alerts: 2,
      critical_alerts: 1,
    },
  ],
  nodes: [
    {
      cluster_id: 1,
      cluster_name: 'prod',
      node_id: 11,
      instance: '10.0.0.1:5801',
      role: 'master',
      status: 'running',
      agent_online: true,
      master: true,
      scrape_up: true,
      heap_percent: 71,
      cpu_cores: 0.5,
      threads: 120,
      fd_percent: 1,
      uptime_seconds: 90_000,
    },
    {
      cluster_id: 1,
      cluster_name: 'prod',
      node_id: 12,
      instance: '10.0.0.2:5801',
      role: 'worker',
      status: 'running',
      agent_online: true,
      scrape_up: false,
    },
  ],
  signals:
    source === 'prometheus'
      ? {
          clusters_safe: 1,
          clusters_total: 1,
          deadlocked_threads: 0,
          fd_percent_max: 1,
          pool_reject_rate: 0,
          hz_queue_util_max: 0,
          endpoints_down: 1,
        }
      : null,
  series:
    source === 'prometheus'
      ? [
          {
            key: 'availability',
            label: '10.0.0.1:5801',
            points: [
              [0, 1],
              [75_000, 0],
            ],
          },
          {
            key: 'heap',
            label: '10.0.0.1:5801',
            points: [
              [0, 70],
              [15_000, 71],
            ],
          },
          {
            key: 'job_state',
            label: 'running',
            points: [
              [0, 3],
              [15_000, 3],
            ],
          },
          {
            key: 'job_state',
            label: 'pending',
            points: [
              [0, 0],
              [15_000, 0],
            ],
          },
        ]
      : [],
});

const detail = (group: string): HomeClusterDetail => ({
  source: 'prometheus',
  range: '1h',
  start_ms: 0,
  end_ms: 3_600_000,
  step_seconds: 15,
  group,
  panels:
    group === 'jvm'
      ? [
          {
            key: 'heap_pools',
            unit: 'bytes',
            series: [
              {
                instance: '10.0.0.1:5801',
                label: 'G1 Old Gen',
                points: [
                  [0, 30],
                  [15_000, 35],
                ],
              },
            ],
          },
        ]
      : [],
  events: [
    {ts_ms: 60_000, kind: 'node_restart', label: '10.0.0.1:5801'},
    {ts_ms: 120_000, kind: 'members_change', label: '2 → 3'},
  ],
  partial_errors: [],
});

const query: DashboardQuery = {
  tab: 'cluster',
  clusterId: undefined,
  status: '',
  mode: '',
  q: '',
  sort: '',
  order: '',
  page: 1,
  range: '1h',
  listRange: 'all',
  start: undefined,
  end: undefined,
};

const renderTab = (update = vi.fn()) => {
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='UTC'>
      <ClusterTab query={query} update={update} />
    </NextIntlClientProvider>,
  );
  return update;
};

describe('ClusterTab', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders cards, availability, trends with the heap reference line, nodes and signals', async () => {
    home.getClusters.mockResolvedValue(dashboard('prometheus'));
    const update = renderTab();
    expect(await screen.findByTestId('home-availability')).toBeTruthy();
    expect(home.getClusters).toHaveBeenCalledWith({
      cluster_id: undefined,
      range: '1h',
    });
    expect(screen.getByTestId('status-bar').textContent).toBe(
      '10.0.0.1:5801:up,down',
    );
    const heap = screen
      .getAllByTestId('line-chart')
      .find((node) => node.textContent?.startsWith('JVM'));
    expect(heap?.textContent).toBe(
      'JVM 堆内存使用率:10.0.0.1:5801,__reference',
    );
    // 全为 0 的状态不画进构成图。/ All-zero states stay out of the composition.
    expect(screen.getByTestId('composition').textContent).toBe('running');
    const table = screen.getByTestId('home-node-table');
    expect(within(table).getByText('抓取失败')).toBeTruthy();
    expect(within(table).getByText('1 天 1 小时')).toBeTruthy();
    expect(
      within(screen.getByTestId('home-signals')).getAllByText('异常'),
    ).toHaveLength(1);

    // 范围选择条：首格「全部集群」选中，集群格带健康徽标；点击切换范围。
    // Scope strip: the "all clusters" cell is selected, cluster cells carry health
    // badges; clicking a cell switches the scope.
    const scope = screen.getByTestId('home-cluster-scope');
    const cells = within(scope).getAllByRole('radio');
    expect(cells).toHaveLength(3);
    expect(cells[0]?.getAttribute('aria-checked')).toBe('true');
    expect(cells[1]?.getAttribute('aria-checked')).toBe('false');
    expect(within(cells[2]!).getByText('异常')).toBeTruthy();
    fireEvent.click(
      within(scope).getByText('single · 3.0.0'),
    );
    expect(update).toHaveBeenCalledWith({clusterId: '2', page: 1});
  });

  it('keeps cards and the node table without Prometheus and shows the trend empty state', async () => {
    home.getClusters.mockResolvedValue(dashboard('none'));
    renderTab();
    expect(await screen.findByTestId('home-trend-empty')).toBeTruthy();
    expect(screen.queryByTestId('home-availability')).toBeNull();
    expect(screen.queryByTestId('home-signals')).toBeNull();
    expect(
      within(screen.getByTestId('home-node-table')).getByText('10.0.0.1:5801'),
    ).toBeTruthy();
    expect(
      within(screen.getByTestId('home-cluster-scope')).getAllByRole('radio'),
    ).toHaveLength(3);
  });

  it('lazily loads detail groups and renders panels plus the events strip', async () => {
    home.getClusters.mockResolvedValue(dashboard('prometheus'));
    home.getClustersDetail.mockImplementation(({group}) =>
      Promise.resolve(detail(group)),
    );
    renderTab();
    // 默认请求 jvm 组；事件条展示重启与成员数变化。/ jvm loads by default; the events strip shows
    // the restart and membership change.
    const section = await screen.findByTestId('home-cluster-detail');
    expect(home.getClustersDetail).toHaveBeenCalledWith({
      cluster_id: undefined,
      group: 'jvm',
      range: '1h',
    });
    expect(await within(section).findByText('堆内存池')).toBeTruthy();
    const events = within(section).getByTestId('detail-events');
    expect(events.textContent).toContain('节点重启');
    expect(events.textContent).toContain('成员数变化');
    expect(events.textContent).toContain('2 → 3');

    // 切换到未上报的组 → 组级空态。/ Switching to an unreported group shows the group empty state.
    fireEvent.click(
      within(section).getByRole('button', {name: '状态存储'}),
    );
    expect(
      await within(section).findByText(
        '该集群未上报此组指标（引擎版本或配置不支持）',
      ),
    ).toBeTruthy();
    expect(home.getClustersDetail).toHaveBeenLastCalledWith(
      expect.objectContaining({group: 'store'}),
    );
  });
});
