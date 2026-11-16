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
import type {HomeJobDashboard} from '@/lib/services/dashboard';
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

const home = vi.hoisted(() => ({getJobs: vi.fn()}));
vi.mock('@/lib/services/dashboard', async () => {
  const actual = await vi.importActual<
    typeof import('@/lib/services/dashboard')
  >('@/lib/services/dashboard');
  return {
    ...actual,
    HomeService: {...actual.HomeService, getJobs: home.getJobs},
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
  BarChart: ({categories, label}: {categories: string[]; label: string}) => (
    <div data-testid='beui-bar'>
      {label}:{categories.join(',')}
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

const base: DashboardQuery = {
  tab: 'jobs',
  clusterId: undefined,
  status: '',
  mode: '',
  q: '',
  sort: '',
  order: '',
  page: 1,
  range: '1h',
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
  beforeEach(() => vi.clearAllMocks());

  it('renders every block with Prometheus and switches throughput metrics', async () => {
    home.getJobs.mockResolvedValue(payload('prometheus'));
    renderTab({clusterId: 2});
    expect((await screen.findByTestId('brush')).textContent).toBe(
      '写入 行/秒|2|作业失败',
    );
    expect(home.getJobs).toHaveBeenCalledWith({cluster_id: 2, range: '1h'});
    fireEvent.click(screen.getByText('读取 行/秒'));
    expect(screen.getByTestId('brush').textContent).toBe(
      '读取 行/秒|1|作业失败',
    );
    expect(screen.getByTestId('beui-bar').textContent).toBe(
      '写入速率 Top 8:orders',
    );
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
});
