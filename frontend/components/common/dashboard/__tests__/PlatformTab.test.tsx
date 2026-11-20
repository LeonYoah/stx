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

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {PlatformSnapshotData} from '@/lib/services/monitoring';
import {PlatformTab} from '../PlatformTab';

const mock = vi.hoisted(() => ({
  getPlatformSnapshotSafe: vi.fn(),
}));
vi.mock('@/lib/services', () => ({
  default: {
    monitoring: {
      getPlatformSnapshotSafe: mock.getPlatformSnapshotSafe,
    },
  },
}));
vi.mock('@/components/arc/line-chart/line-chart', () => ({
  LineChart: ({label, series}: {label: string; series: {key: string}[]}) => (
    <div data-testid='line-chart'>
      {label}:{series.map((item) => item.key).join(',')}
    </div>
  ),
}));

function snapshot(overrides: Partial<PlatformSnapshotData> = {}): PlatformSnapshotData {
  return {
    available: true,
    server: {
      version: '1.2.3',
      uptime_seconds: 7200,
      goroutines: 42,
      rss_bytes: 256 * 1024 * 1024,
      heap_inuse_bytes: 96 * 1024 * 1024,
      http_rps: 12.5,
      http_5xx_ratio: 0.001,
      http_p95_seconds: 0.05,
      db_open: 4,
      db_in_use: 2,
      db_wait_count_rate: 0.01,
      agents_connected: 1,
      tsdb: {
        head_series: 1500,
        blocks_bytes: 2 * 1024 * 1024,
        compactions_failed: 0,
      },
    },
    agents: [
      {
        host_id: 7,
        host_name: 'prod-01',
        online: true,
        version: '1.2.3',
        uptime_seconds: 3600,
        rss_bytes: 40 * 1024 * 1024,
        cpu_ratio: 0.12,
        goroutines: 18,
        command_failures_1h: 0,
        last_report_at: '2025-01-01T12:00:00+08:00',
      },
      {
        host_id: 8,
        host_name: 'legacy-02',
        online: false,
        version: '1.0.0',
        uptime_seconds: null,
        rss_bytes: null,
        cpu_ratio: null,
        goroutines: null,
        command_failures_1h: null,
        last_report_at: null,
      },
    ],
    trends: {
      rss_bytes: [
        [0, 100],
        [60_000, 120],
      ],
      heap_inuse_bytes: [[0, 50]],
      http_rps: [],
      http_5xx_ratio: [],
    },
    ...overrides,
  };
}

const renderTab = () =>
  render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='UTC'>
      <PlatformTab />
    </NextIntlClientProvider>,
  );

describe('PlatformTab', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders server overview panels and the agent table', async () => {
    mock.getPlatformSnapshotSafe.mockResolvedValue({
      success: true,
      data: snapshot(),
    });
    renderTab();

    expect(await screen.findByText('STX Server')).toBeTruthy();
    expect(screen.getByText('v1.2.3')).toBeTruthy();
    // 三张图：内存双序列 + QPS + 5xx。/ Three charts: memory (2 series), QPS, 5xx.
    const charts = await screen.findAllByTestId('line-chart');
    expect(charts).toHaveLength(3);
    expect(charts[0].textContent).toBe('内存:rss,heap');
    // 控制面与内嵌存储的小格。/ Control-plane and storage stat cells.
    expect(screen.getByText('HTTP P95')).toBeTruthy();
    expect(screen.getByText('DB 连接池')).toBeTruthy();
    expect(screen.getByText('压缩失败')).toBeTruthy();
    // Agent 表：在线有数据，旧版全 -；版本不一致给徽标。
    // Agent rows: the reporting agent shows metrics, the legacy one shows -;
    // a version mismatch gets a badge.
    expect(screen.getByText('prod-01')).toBeTruthy();
    expect(screen.getByText('legacy-02')).toBeTruthy();
    expect(screen.getByText('版本不一致')).toBeTruthy();
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
  });

  it('degrades to the shared empty state plus the external probe note', async () => {
    mock.getPlatformSnapshotSafe.mockResolvedValue({
      success: true,
      data: {
        available: false,
        reason: 'observability_disabled',
        agents: [],
      } as unknown as PlatformSnapshotData,
    });
    renderTab();
    expect(await screen.findByTestId('home-trend-empty')).toBeTruthy();
    expect(screen.getByText(/api\/v1\/health/)).toBeTruthy();
    expect(screen.queryByText('STX Server')).toBeNull();
  });

  it('shows an empty agent table message when no agents are registered', async () => {
    mock.getPlatformSnapshotSafe.mockResolvedValue({
      success: true,
      data: snapshot({agents: []}),
    });
    renderTab();
    expect(await screen.findByText('暂无已注册 Agent')).toBeTruthy();
  });

  it('keeps the error line when the snapshot request fails', async () => {
    mock.getPlatformSnapshotSafe.mockResolvedValue({
      success: false,
      error: 'boom',
    });
    renderTab();
    expect(
      await screen.findByText('加载平台监控数据失败'),
    ).toBeTruthy();
  });
});
