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
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {HomeJobDashboard, HomeVolume} from '@/lib/services/dashboard';
import {JobsTab} from '../JobsTab';
import type {DashboardQuery} from '../use-dashboard-query';

if (typeof window.matchMedia !== 'function') {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

const home = vi.hoisted(() => ({getJobs: vi.fn(), getVolume: vi.fn()}));
vi.mock('@/lib/services/dashboard', async () => {
  const actual = await vi.importActual<
    typeof import('@/lib/services/dashboard')
  >('@/lib/services/dashboard');
  return {
    ...actual,
    HomeService: {
      ...actual.HomeService,
      getJobs: home.getJobs,
      getVolume: home.getVolume,
    },
  };
});
vi.mock('@/components/arc/brush-chart/brush-chart', () => ({
  BrushChart: ({
    label,
    data,
    annotations,
  }: {
    label: string;
    data: unknown[];
    annotations: {label: string}[];
  }) => (
    <div data-testid='brush'>
      {label}|{data.length}|{annotations.map((item) => item.label).join(',')}
    </div>
  ),
}));
vi.mock('@/components/arc/bar-chart/bar-chart', () => ({
  BarChart: ({data}: {data: {label: string}[]}) => (
    <div data-testid='arc-bar'>{data.map((item) => item.label).join(',')}</div>
  ),
}));
vi.mock('@/components/charts/bar-chart', () => ({
  BarChart: ({
    categories,
    series,
    label,
  }: {
    categories: string[];
    series?: {name: string}[];
    label: string;
  }) => (
    <div data-testid='beui-bar'>
      {label}:{categories.join(',')}|{(series ?? []).map((s) => s.name).join(',')}
    </div>
  ),
}));
vi.mock('@/components/charts/sankey-chart', () => ({
  SankeyChart: ({
    nodes,
    links,
  }: {
    nodes: {label: string}[];
    links: unknown[];
  }) => (
    <div data-testid='sankey'>
      {nodes.map((node) => node.label).join(',')}|{links.length}
    </div>
  ),
}));
vi.mock('@/components/charts/heat-calendar', () => ({
  HeatCalendar: ({
    values,
    endDate,
    unit,
  }: {
    values: number[][];
    endDate: Date;
    unit: string;
  }) => (
    <div data-testid='calendar'>
      {unit}|{values.flat().reduce((a, b) => a + b, 0)}|
      {endDate.toISOString().slice(0, 10)}
    </div>
  ),
}));

const payload = (source: 'prometheus' | 'none'): HomeJobDashboard => ({
  source,
  range: '1h',
  start_ms: 0,
  end_ms: 3_600_000,
  step_seconds: 15,
  window_seconds: 3600,
  throughput: [
    {
      key: 'write',
      points: [
        [0, 5],
        [15_000, 6],
      ],
    },
    {key: 'read', points: [[0, 7]]},
    {key: 'write_bytes', points: []},
    {key: 'read_bytes', points: []},
  ],
  annotations: [
    {ts_ms: 1, kind: 'job_failed', title: 'orders', viewable: true},
  ],
  top_write: [{job_id: 1, name: 'orders', value: 2, viewable: true}],
  top_cdc: [],
  flow: [{source: 'MySQL-CDC', sink: '?', value: 10}],
  checkpoints: [
    {
      job_id: 1,
      name: 'orders',
      value: 2.5,
      max_seconds: 2.5,
      failures: 2,
      viewable: true,
    },
  ],
  hotspots: [
    {
      job_id: 1,
      name: 'orders',
      value: 0.82,
      kind: 'backpressure',
      vertex: 'Sink[0]',
      viewable: true,
    },
  ],
  calendar: {
    weeks: 2,
    end_date: '2026-11-15',
    runs: [
      [0, 0, 0, 0, 0, 0, 0],
      [0, 1, 0, 0, 0, 0, 2],
    ],
    failed: [
      [0, 0, 0, 0, 0, 0, 0],
      [0, 0, 0, 0, 0, 0, 1],
    ],
  },
});

const volumePayload = (): HomeVolume => ({
  source: 'prometheus',
  range: '24h',
  start_ms: 1_000,
  end_ms: 90_000_000,
  granularity: 'day',
  split: 'none',
  oldest_sample_ms: 0,
  totals: null,
  buckets: [
    {start_ms: 1_000, end_ms: 86_400_000, label: '2026-11-15', partial: true},
    {
      start_ms: 86_400_000,
      end_ms: 90_000_000,
      label: '2026-11-16',
      partial: true,
    },
  ],
  series: [
    {metric: 'write_rows', group_id: '0', totals: [100, 50]},
    {metric: 'read_rows', group_id: '0', totals: [80, 40]},
  ],
  groups: {},
});

const base: DashboardQuery = {
  tab: 'jobs',
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
const renderTab = (query: Partial<DashboardQuery> = {}) =>
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='UTC'>
      <JobsTab query={{...base, ...query}} />
    </NextIntlClientProvider>,
  );

describe('JobsTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    home.getVolume.mockResolvedValue(volumePayload());
  });

  it('renders every block with Prometheus and switches throughput metrics', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    renderTab({clusterId: 2});
    expect((await screen.findByTestId('brush')).textContent).toBe(
      '写入 行/秒|2|作业失败',
    );
    expect(home.getJobs).toHaveBeenCalledWith({cluster_id: 2, range: '1h'});
    // 吞吐分段控件已换成页签格：同名的 role=group 不再存在，只有 role=tablist。
    // The segmented control became tab cells: no role=group with that label, only a tablist.
    const tablist = screen.getByRole('tablist', {name: '吞吐指标'});
    expect(
      screen.queryByRole('group', {name: '吞吐指标'}),
    ).toBeNull();
    // 格子主数值是窗口均值，说明行是峰值：write [[0,5],[15000,6]] → 5.5 /s，峰值 6 /s。
    // Cell headline is the window average, caption the peak: write → 5.5 /s, peak 6 /s.
    const writeTab = within(tablist).getByTestId('home-throughput-tab-write');
    expect(writeTab.textContent).toContain('5.5');
    expect(writeTab.textContent).toContain('峰值 6 /s');
    // 空序列的格子显示「—」。/ An empty series shows "—".
    expect(
      within(tablist).getByTestId('home-throughput-tab-write_bytes')
        .textContent,
    ).toContain('—');
    fireEvent.click(
      within(tablist).getByTestId('home-throughput-tab-read'),
    );
    expect(screen.getByTestId('brush').textContent).toBe(
      '读取 行/秒|1|作业失败',
    );
    // roving tabindex：选中格上按 →，选择跟随焦点移动到下一格。
    // Roving tabindex: ArrowRight on the selected tab moves selection and focus together.
    const readTab = within(tablist).getByTestId(
      'home-throughput-tab-read',
    );
    readTab.focus();
    fireEvent.keyDown(readTab, {key: 'ArrowRight'});
    const bytesTab = within(tablist).getByTestId(
      'home-throughput-tab-write_bytes',
    );
    expect(bytesTab.getAttribute('aria-selected')).toBe('true');
    expect(bytesTab).toHaveFocus();
    expect(screen.getByTestId('brush').textContent).toBe(
      '写入 字节/秒|0|作业失败',
    );
    expect(
      within(screen.getByTestId('home-top-write')).getByTestId('beui-bar')
        .textContent,
    ).toBe('写入速率 Top 8:orders|平均写入速率');
    expect(
      within(screen.getByTestId('home-top-cdc')).getByText(
        '该时间窗内没有数据',
      ),
    ).toBeTruthy();
    expect(screen.getByTestId('sankey').textContent).toBe('MySQL-CDC,未识别|1');
    expect(screen.getByTestId('arc-bar').textContent).toBe('orders（失败 2）');
    expect(
      within(screen.getByTestId('home-hotspots')).getByText('82%'),
    ).toBeTruthy();
    expect(screen.getByTestId('calendar').textContent).toBe(
      '次运行|3|2026-11-15',
    );
    fireEvent.click(screen.getByText('失败'));
    expect(screen.getByTestId('calendar').textContent).toBe(
      '次失败|1|2026-11-15',
    );
  });

  it('shows sub-tick nonzero rates as "<step" instead of 0', async () => {
    // 真实 7d 窗口量级：行类 avg 0.00208 / peak 0.00417，字节类 avg 0.01875 / peak 0.0375。
    // Real 7d-window magnitudes: rows avg 0.00208 / peak 0.00417, bytes avg 0.01875 / peak 0.0375.
    const tiny = payload('prometheus');
    tiny.throughput = [
      {
        key: 'write',
        points: [
          [0, 0.00208],
          [15_000, 0.00417],
        ],
      },
      {key: 'read', points: [[0, 0]]},
      {
        key: 'write_bytes',
        points: [
          [0, 0.01875],
          [15_000, 0.0375],
        ],
      },
      {key: 'read_bytes', points: [[0, 0]]},
    ];
    home.getJobs.mockResolvedValue(tiny);
    renderTab();
    await screen.findByTestId('home-throughput');
    const tablist = screen.getByRole('tablist', {name: '吞吐指标'});
    const writeTab = within(tablist).getByTestId('home-throughput-tab-write');
    expect(writeTab.textContent).toContain('<0.1');
    expect(writeTab.textContent).toContain('峰值 <0.1 /s');
    const bytesTab = within(tablist).getByTestId(
      'home-throughput-tab-write_bytes',
    );
    expect(bytesTab.textContent).toContain('<1');
    expect(bytesTab.textContent).toContain('峰值 <1 B/s');
    // 全 0 序列仍显示 0。/ An all-zero series still shows 0.
    const readTab = within(tablist).getByTestId('home-throughput-tab-read');
    expect(readTab.textContent).toContain('峰值 0 /s');
    expect(readTab.textContent).not.toContain('<');
  });

  it('collapses empty secondary panels into one-line strips', async () => {
    const empty = payload('prometheus');
    empty.top_write = [];
    empty.top_cdc = [];
    empty.checkpoints = [];
    empty.hotspots = [];
    empty.flow = [];
    home.getJobs.mockResolvedValue(empty);
    // 数据量：不拆分且四个指标都没有行 → 面板缩矮条（分段控件随之隐藏）。
    // Volume: unsplit with zero rows across all four metrics → strip (segmented
    // controls go with it).
    home.getVolume.mockResolvedValue({...volumePayload(), series: []});
    renderTab();
    await screen.findByTestId('home-throughput');
    // 等 volume 数据回来后 home-volume 才缩条。/ home-volume only strips after volume data arrives.
    await waitFor(() =>
      expect(screen.getByTestId('home-volume').className).toContain('h-11'),
    );
    for (const testId of [
      'home-top-write',
      'home-top-cdc',
      'home-flow',
      'home-checkpoints',
      'home-hotspots',
      'home-volume',
    ]) {
      const strip = screen.getByTestId(testId);
      expect(strip.className).toContain('h-11');
      expect(strip.textContent).toContain('该时间窗内没有数据');
      expect(strip.querySelector('h3')?.textContent).toBeTruthy();
    }
    // 矮条不渲染图表与分段控件。/ Strips render neither charts nor segmented controls.
    expect(screen.queryByTestId('beui-bar')).toBeNull();
    expect(screen.queryByText('按集群')).toBeNull();
    expect(screen.queryByRole('table')).toBeNull();
  });

  it('shows only the calendar without Prometheus and requests a custom window as given', async () => {
    home.getJobs.mockResolvedValue(payload('none'));
    renderTab({start: 1_000, end: 2_000});
    expect(await screen.findByTestId('home-trend-empty')).toBeTruthy();
    expect(screen.getByTestId('calendar')).toBeTruthy();
    expect(screen.queryByTestId('home-throughput')).toBeNull();
    expect(home.getJobs).toHaveBeenCalledWith({
      cluster_id: undefined,
      start: 1_000,
      end: 2_000,
    });
  });

  it('renders the volume chart and refetches on split changes', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    renderTab();
    const panel = await screen.findByTestId('home-volume');
    expect(within(panel).getByTestId('beui-bar').textContent).toBe(
      '数据量:11-15*,11-16*|全部',
    );
    // 粒度由后端按窗口自动选：前端固定请求 auto。/ Granularity follows the window: the
    // frontend always asks for auto.
    expect(home.getVolume).toHaveBeenCalledWith({
      cluster_id: undefined,
      granularity: 'auto',
      split: 'none',
      range: '1h',
    });
    // 部分桶标注脚注。/ Partial buckets get a footnote.
    expect(within(panel).getByText(/首末桶被时间窗裁掉/)).toBeTruthy();

    fireEvent.click(within(panel).getByText('按集群'));
    expect(home.getVolume).toHaveBeenLastCalledWith(
      expect.objectContaining({split: 'cluster'}),
    );
    // 指标切换不重新拉取：四个方向一次取回。/ The metric toggle does not refetch.
    const calls = home.getVolume.mock.calls.length;
    fireEvent.click(within(panel).getByText('读取 行'));
    expect(home.getVolume.mock.calls.length).toBe(calls);
  });

  it('renders week buckets as Mon–Sun ranges and gaps no_data buckets', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    home.getVolume.mockResolvedValue({
      ...volumePayload(),
      granularity: 'week',
      buckets: [
        {
          start_ms: 1_000,
          end_ms: 86_400_000,
          label: '2026-11-09',
          partial: false,
          no_data: true,
        },
        {
          start_ms: 86_400_000,
          end_ms: 90_000_000,
          label: '2026-11-16',
          partial: true,
        },
      ],
    });
    renderTab();
    const panel = await screen.findByTestId('home-volume');
    // 周标签渲染成「周一–周日」，no_data 桶带 · 标记并留空柱，脚注解释两种标记。
    // Week labels render as "Mon–Sun", no_data buckets carry a · marker and a gap bar,
    // and footnotes explain both markers.
    expect(within(panel).getByTestId('beui-bar').textContent).toBe(
      '数据量:11/09–11/15·,11/16–11/22*|全部',
    );
    expect(within(panel).getByText(/首末桶被时间窗裁掉/)).toBeTruthy();
    expect(within(panel).getByText(/早于最早样本的桶没有数据/)).toBeTruthy();
  });

  it('names split groups from the groups map', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    home.getVolume.mockResolvedValue({
      ...volumePayload(),
      split: 'cluster',
      series: [
        {metric: 'write_rows', group_id: '3', totals: [10, 5]},
        {metric: 'write_rows', group_id: '9', totals: [2, 1]},
      ],
      groups: {'3': 'prod'},
    });
    renderTab();
    const panel = await screen.findByTestId('home-volume');
    fireEvent.click(within(panel).getByText('按集群'));
    await within(panel).findByText('按集群');
    // 分组名来自 groups 映射，未知名回退 #id。/ Group names come from the groups map, unknown ids fall back to #id.
    expect(
      within(panel).getByTestId('beui-bar').textContent,
    ).toContain('prod,#9');
  });

  it('keeps the date on sub-day labels spanning days and drops it within one day', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    // 跨天的小时桶：两个 "00:00" 若只截时刻会撞成同一类目。
    // Hour buckets across midnight: two "00:00"s would collapse into one category if only
    // the clock were kept.
    home.getVolume.mockResolvedValue({
      ...volumePayload(),
      granularity: 'hour',
      buckets: [
        {start_ms: 1, end_ms: 2, label: '2026-11-15 22:00', partial: false},
        {start_ms: 2, end_ms: 3, label: '2026-11-15 23:00', partial: false},
        {start_ms: 3, end_ms: 4, label: '2026-11-16 00:00', partial: false},
        {start_ms: 4, end_ms: 5, label: '2026-11-16 01:00', partial: false},
      ],
      series: [{metric: 'write_rows', group_id: '0', totals: [1, 2, 3, 4]}],
    });
    renderTab();
    const panel = await screen.findByTestId('home-volume');
    expect(within(panel).getByTestId('beui-bar').textContent).toBe(
      '数据量:11-15 22:00,11-15 23:00,11-16 00:00,11-16 01:00|全部',
    );
  });

  it('shows bare clock labels when all sub-day buckets share one day', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    home.getVolume.mockResolvedValue({
      ...volumePayload(),
      granularity: 'hour',
      buckets: [
        {start_ms: 1, end_ms: 2, label: '2026-11-15 12:00', partial: true},
        {start_ms: 2, end_ms: 3, label: '2026-11-15 13:00', partial: false},
      ],
      series: [{metric: 'write_rows', group_id: '0', totals: [7, 9]}],
    });
    renderTab();
    const panel = await screen.findByTestId('home-volume');
    expect(within(panel).getByTestId('beui-bar').textContent).toBe(
      '数据量:12:00*,13:00|全部',
    );
  });
});
