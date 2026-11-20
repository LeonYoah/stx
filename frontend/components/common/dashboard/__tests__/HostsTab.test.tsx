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

import {render, screen, within} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {HomeHostDashboard} from '@/lib/services/dashboard';
import {HostsTab} from '../HostsTab';
import type {DashboardQuery} from '../use-dashboard-query';

const home = vi.hoisted(() => ({
  getHosts: vi.fn(),
}));
vi.mock('@/lib/services/dashboard', async () => {
  const actual = await vi.importActual<
    typeof import('@/lib/services/dashboard')
  >('@/lib/services/dashboard');
  return {
    ...actual,
    HomeService: {
      ...actual.HomeService,
      getHosts: home.getHosts,
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
vi.mock('next/link', () => ({
  default: ({children}: {children: React.ReactNode}) => <>{children}</>,
}));

const payload = (source: 'prometheus' | 'none'): HomeHostDashboard => ({
  source,
  reason: source === 'none' ? 'observability_disabled' : undefined,
  range: '1h',
  step_seconds: 15,
  start_ms: 0,
  end_ms: 1_700_000_000_000,
  hosts: [
    {
      id: 1,
      name: 't1',
      ip: '10.66.0.2',
      host_type: 'bare_metal',
      agent_status: 'installed',
      cpu_cores: 4,
      total_memory_bytes: 8 * 1024 ** 3,
      last_heartbeat_ms: 1_700_000_000_000,
      metrics_supported: true,
      boot_time_ms: 1_700_000_000_000 - 3 * 86_400_000,
      panels: [
        {
          key: 'cpu',
          unit: 'percent',
          series: [
            {instance: 't1', label: 'user', points: [[0, 20]]},
            {instance: 't1', label: 'system', points: [[0, 10]]},
          ],
        },
        {
          key: 'net',
          unit: 'bytes_per_second',
          series: [
            {instance: 't1', label: 'rx eth0', points: [[0, 1024]]},
            {instance: 't1', label: 'tx eth0', points: [[0, 512]]},
          ],
        },
        {
          key: 'fs_used',
          unit: 'percent',
          series: [{instance: 't1', label: '/', points: [[0, 92]]}],
        },
        {
          key: 'fs_inodes',
          unit: 'percent',
          series: [{instance: 't1', label: '/', points: [[0, 45]]}],
        },
        {
          key: 'tcp',
          unit: 'count',
          series: [{instance: 't1', label: 'established', points: [[0, 12]]}],
        },
        {
          key: 'fds',
          unit: 'count',
          series: [{instance: 't1', label: 'allocated', points: [[0, 640]]}],
        },
      ],
    },
    {
      id: 2,
      name: 'dock',
      ip: '10.66.0.9',
      host_type: 'docker',
      agent_status: 'not_installed',
      cpu_cores: 0,
      total_memory_bytes: 0,
      metrics_supported: false,
      panels: [],
    },
  ],
  partial_errors: [],
});

const query: DashboardQuery = {
  tab: 'hosts',
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

const renderTab = () =>
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='UTC'>
      <HostsTab query={query} />
    </NextIntlClientProvider>,
  );

describe('HostsTab', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders one card per host with panels for supported and a note for unsupported', async () => {
    home.getHosts.mockResolvedValue(payload('prometheus'));
    renderTab();
    expect(await screen.findByText('t1')).toBeTruthy();
    expect(home.getHosts).toHaveBeenCalledWith({
      cluster_id: undefined,
      range: '1h',
    });
    // 支持的主机出面板；不支持的主机收成一句提示。/ Supported hosts render panels; the
    // unsupported one collapses to a note.
    expect(await screen.findByTestId('host-panel-1-cpu')).toBeTruthy();
    expect(screen.getByTestId('host-unsupported-2').textContent).toContain(
      '未绑定 Agent',
    );
    const net = screen.getByTestId('host-panel-1-net');
    expect(within(net).getByTestId('line-chart').textContent).toBe(
      '网络吞吐:rx eth0,tx eth0',
    );
    // fs_used/fs_inodes 合成一格 Progress 列表；tcp/fds 合成一格指标卡。/ fs_used +
    // fs_inodes fold into one progress cell; tcp + fds into one metric-card cell.
    const fs = screen.getByTestId('host-panel-1-fs');
    expect(fs.textContent).toContain('92%');
    expect(fs.textContent).toContain('45%');
    expect(screen.queryByTestId('host-panel-1-fs_inodes')).toBeNull();
    const conn = screen.getByTestId('host-panel-1-conn');
    expect(conn.textContent).toContain('12');
    expect(screen.queryByTestId('host-panel-1-fds')).toBeNull();
    // 开机时间换算成「已运行 N 天」进卡头。/ Boot time becomes an "up N days" note in the header.
    expect(screen.getByText(/已运行/)).toBeTruthy();
  });

  it('keeps the inventory under a banner when observability is off', async () => {
    home.getHosts.mockResolvedValue(payload('none'));
    renderTab();
    expect(await screen.findByTestId('home-trend-empty')).toBeTruthy();
    // 清单不因可观测关闭而消失：主机名照出；卡片不再重复原因文案（顶部横幅已说明），
    // 指标区为空、无面板。/ The inventory survives an observability-off state: host names
    // still render; cards don't repeat the reason (the banner covers it) — no note, no panels.
    expect(await screen.findByText('t1')).toBeTruthy();
    expect(screen.queryByTestId('host-metrics-off-1')).toBeNull();
    expect(screen.queryByTestId('host-panel-1-cpu')).toBeNull();
  });

  it('shows a plain empty note when no hosts are in scope', async () => {
    home.getHosts.mockResolvedValue({...payload('prometheus'), hosts: []});
    renderTab();
    expect(
      await screen.findByText('当前范围没有主机'),
    ).toBeTruthy();
  });
});
