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
  source: '',
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
    expect(events.textContent).toContain('时间窗变动');
    expect(events.textContent).toContain('节点重启 1');
    expect(events.textContent).toContain('成员数变化 1');
    expect(events.textContent).toContain('2 → 3');

    // 点开明细弹层，按时间倒序展示全部事件（最新在最前）。
    // Opening the details popover lists every event newest-first.
    fireEvent.click(within(events).getByTestId('detail-events-trigger'));
    const popover = await screen.findByTestId('detail-events-popover');
    expect(popover.textContent).toContain('共 2 条');
    const items = within(popover).getAllByRole('listitem');
    expect(items).toHaveLength(2);
    expect(items[0]?.textContent).toContain('2 → 3');
    expect(items[1]?.textContent).toContain('10.0.0.1:5801');

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

  it('defaults detail groups to the first node on multi-node clusters and switches between node & all views', async () => {
    const pools = [
      "CodeHeap 'non-nmethods'",
      "CodeHeap 'non-profiled nmethods'",
      "CodeHeap 'profiled nmethods'",
      'Compressed Class Space',
      'Metaspace',
    ];
    const multiDetail: HomeClusterDetail = {
      source: 'prometheus',
      range: '1h',
      start_ms: 0,
      end_ms: 3_600_000,
      step_seconds: 15,
      group: 'jvm',
      panels: [
        {
          key: 'nonheap',
          unit: 'bytes',
          series: [
            ...pools.map((pool) => ({
              instance: '192.168.0.105:15801',
              label: pool,
              points: [
                [0, 100],
                [15_000, 110],
              ] as [number, number][],
            })),
            ...pools.map((pool) => ({
              instance: '192.168.0.105:35801',
              label: pool,
              points: [
                [0, 200],
                [15_000, 220],
              ] as [number, number][],
            })),
          ],
        },
      ],
      events: [],
      partial_errors: [],
    };
    home.getClusters.mockResolvedValue(dashboard('prometheus'));
    home.getClustersDetail.mockResolvedValue(multiDetail);
    renderTab();

    const section = await screen.findByTestId('home-cluster-detail');
    const nonheapChart = await within(section).findByText(/^非堆内存:/);
    // 默认聚焦首个节点且去掉冗长 IP 前缀：仅 5 条序列。
    // Defaults to first node and drops redundant IP prefix: 5 series total.
    expect(nonheapChart.textContent).toBe(`非堆内存:${pools.join(',')}`);

    const selectBtn = within(section).getByTestId('detail-node-select');
    expect(selectBtn.textContent).toContain('192.168.0.105:15801');

    // 切到第二节点，序列仍保持精简 5 条。/ Switch to node 2: stays at 5 clean labels.
    fireEvent.click(selectBtn);
    fireEvent.click(
      await screen.findByRole('option', {name: '192.168.0.105:35801'}),
    );
    expect(selectBtn.textContent).toContain('192.168.0.105:35801');
    expect(nonheapChart.textContent).toBe(`非堆内存:${pools.join(',')}`);

    // 切到全部节点对比，恢复带节点前缀的 10 条序列。/ Switch to all nodes: shows 10 prefixed series.
    fireEvent.click(selectBtn);
    fireEvent.click(await screen.findByRole('option', {name: '全部节点 (2)'}));
    const keys = (nonheapChart.textContent ?? '').replace('非堆内存:', '').split(',');
    expect(keys).toHaveLength(10);
    expect(keys[0]).toBe("192.168.0.105:15801 · CodeHeap 'non-nmethods'");
    expect(keys[5]).toBe("192.168.0.105:35801 · CodeHeap 'non-nmethods'");
  });

  it('flattens deadlocked/reject into signal rows and keeps members visible under node filter', async () => {
    // 30 点序列带一个历史尖刺：最新值 0 但窗口峰值 >0。
    // A 30-point series with one historical spike: latest is 0 while the window peak >0.
    const spikePoints = (
      count: number,
      spikeIndex: number,
      spikeValue: number,
    ): [number, number][] =>
      Array.from(
        {length: count},
        (_, index): [number, number] => [
          index * 15_000,
          index === spikeIndex ? spikeValue : 0,
        ],
      );
    const detailFor = (group: string): HomeClusterDetail => ({
      source: 'prometheus',
      range: '1h',
      start_ms: 0,
      end_ms: 3_600_000,
      step_seconds: 15,
      group,
      panels:
        group === 'gc'
          ? [
              {
                key: 'deadlocked',
                unit: 'count',
                series: [
                  {
                    instance: '10.0.0.1:5801',
                    label: 'deadlocked',
                    points: spikePoints(30, 10, 5),
                  },
                ],
              },
            ]
          : group === 'sched'
            ? [
                {
                  key: 'reject',
                  unit: 'per_second',
                  series: [
                    {
                      instance: '10.0.0.1:5801',
                      label: 'reject',
                      points: spikePoints(30, 3, 2),
                    },
                    {
                      instance: '10.0.0.2:5801',
                      label: 'reject',
                      points: [
                        [0, 0],
                        [15_000, 0],
                      ],
                    },
                  ],
                },
                {
                  key: 'members',
                  unit: 'count',
                  series: [
                    {
                      instance: 'prod',
                      label: 'members',
                      points: [
                        [0, 2],
                        [15_000, 3],
                      ],
                    },
                  ],
                },
              ]
            : [],
      events: [],
      partial_errors: [],
    });
    home.getClusters.mockResolvedValue(dashboard('prometheus'));
    home.getClustersDetail.mockImplementation(({group}) =>
      Promise.resolve(detailFor(group)),
    );
    renderTab();
    const section = await screen.findByTestId('home-cluster-detail');

    // gc 组：死锁面板是单层行 + 峰值信号条，不再嵌 MetricCard（<article>）。
    // gc group: the deadlocked panel is flat rows + a peak signal bar — no nested
    // MetricCard (<article>).
    fireEvent.click(within(section).getByRole('button', {name: 'GC 与线程'}));
    const deadlocked = await within(section).findByTestId(
      'detail-panel-deadlocked',
    );
    expect(deadlocked.querySelector('article')).toBeNull();
    const deadBar = within(deadlocked).getByTestId('status-bar');
    const deadText = deadBar.textContent ?? '';
    const deadStatuses = deadText.slice(deadText.lastIndexOf(':') + 1).split(',');
    // 30 个原始点降采样到 ≤24 桶，尖刺所在桶保留为 alert。
    // 30 raw points downsample to ≤24 buckets and the spike bucket stays alert.
    expect(deadStatuses.length).toBeLessThanOrEqual(24);
    expect(deadStatuses).toContain('alert');
    // 最新值 0、窗口峰值 5 → 峰值提示 + 检出异常徽标。
    // Latest 0 with window peak 5 → peak hint + detected chip.
    expect(deadlocked.textContent).toContain('窗内峰值 5');
    expect(deadlocked.textContent).toContain('检出异常');

    // sched 组：拒绝面板同构；members 的 instance 是集群名，按节点过滤不能筛没。
    // sched group: reject follows the same shape; members carries cluster names
    // as instance, so node filtering must not empty it.
    fireEvent.click(within(section).getByRole('button', {name: '调度与集群'}));
    const reject = await within(section).findByTestId('detail-panel-reject');
    expect(reject.querySelector('article')).toBeNull();
    expect(within(reject).getAllByTestId('status-bar')).toHaveLength(2);
    const selectBtn = within(section).getByTestId('detail-node-select');
    fireEvent.click(selectBtn);
    // 集群名 'prod' 不进入节点选项。/ The cluster name 'prod' stays out of the node options.
    expect(screen.queryByRole('option', {name: 'prod'})).toBeNull();
    fireEvent.click(
      await screen.findByRole('option', {name: '10.0.0.2:5801'}),
    );
    const members = within(section).getByTestId('detail-panel-members');
    expect(
      within(members).getByTestId('line-chart').textContent,
    ).toContain('prod · members');
  });
});
