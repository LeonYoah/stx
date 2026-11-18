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
  step_seconds: 15, oldest_sample_ms: 0,
  start_ms: 0,
  end_ms: 3_600_000,
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
      panels: [
        {
          key: 'cpu',
          unit: 'per_second',
          series: [
            {instance: 't1', label: 'user', points: [[0, 0.2]]},
            {instance: 't1', label: 'idle', points: [[0, 3.6]]},
          ],
        },
        {
          key: 'net',
          unit: 'per_second',
          series: [
            {instance: 't1', label: 'rx eth0', points: [[0, 1024]]},
            {instance: 't1', label: 'tx eth0', points: [[0, 512]]},
          ],
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
  q: '',
  sort: '',
  order: '',
  page: 1,
  range: '1h',
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
  });

  it('shows the shared unavailable state when observability is off', async () => {
    home.getHosts.mockResolvedValue(payload('none'));
    renderTab();
    expect(await screen.findByTestId('home-trend-empty')).toBeTruthy();
    expect(screen.queryByText('t1')).toBeNull();
  });

  it('shows a plain empty note when no hosts are in scope', async () => {
    home.getHosts.mockResolvedValue({...payload('prometheus'), hosts: []});
    renderTab();
    expect(
      await screen.findByText('当前范围没有主机'),
    ).toBeTruthy();
  });
});
