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
import {afterEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import en from '@/lib/i18n/locales/en.json';
import {MonitoringService} from '@/lib/services/monitoring';
import type {PlatformSnapshotData} from '@/lib/services/monitoring';
import {MonitoringPlatformPanel} from '../MonitoringPlatformPanel';

// jsdom 缺少 matchMedia（动效辅助函数会用到）。
// jsdom lacks matchMedia (used by the motion helpers).
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

// Arc LineChart 依赖 motion 与真实布局，测试中替身化。
// The Arc LineChart depends on motion and real layout; substitute it in tests.
vi.mock('@/components/arc/line-chart/line-chart', () => ({
  LineChart: ({label}: {label: string}) => (
    <div data-testid='platform-trend-chart'>{label}</div>
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
      http_requests_total: 12000,
      http_5xx_total: 3,
      http_p95_seconds: 0.05,
      grpc_requests_total: 900,
      grpc_error_total: 0,
      grpc_p95_seconds: 0.01,
      agents_connected: 1,
      db: {open_connections: 4, max_open_connections: 20, in_use: 2, idle: 2},
      tsdb: {
        head_series: 1500,
        stale_series: 10,
        head_bytes: 1024 * 1024,
        block_bytes: 2 * 1024 * 1024,
        oldest_sample_ms: 0,
        compaction_failures: 0,
        size_retention_deletes: 0,
        last_scrape_ms: 0,
      },
    },
    agents: [
      {
        host_id: 7,
        hostname: 'prod-01',
        address: '10.0.0.7:17801',
        online: true,
        version: '1.2.3',
        version_mismatch: false,
        uptime_seconds: 3600,
        rss_bytes: 40 * 1024 * 1024,
        cpu_seconds_total: 120,
        goroutines: 18,
        commands_total: 5,
        heartbeat_failures: 0,
        last_metrics_report_at: '2025-01-01 12:00:00',
      },
    ],
    trends: {
      rss_bytes: [
        [0, 100],
        [60_000, 120],
      ],
      http_rps: [
        [0, 1],
        [60_000, 2],
      ],
    },
    ...overrides,
  };
}

function renderPanel(messages: typeof zh = zh, locale = 'zh-CN') {
  return render(
    <NextIntlClientProvider locale={locale} messages={messages}>
      <MonitoringPlatformPanel />
    </NextIntlClientProvider>,
  );
}

describe('MonitoringPlatformPanel', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders server stats and the agent table for an available snapshot', async () => {
    vi.spyOn(MonitoringService, 'getPlatformSnapshotSafe').mockResolvedValue({
      success: true,
      data: snapshot(),
    });
    renderPanel();

    expect(await screen.findByText('STX Server')).toBeTruthy();
    expect(screen.getByText('v1.2.3')).toBeTruthy();
    expect(screen.getByText('256 MB')).toBeTruthy();
    expect(screen.getByText('prod-01')).toBeTruthy();
    expect(screen.getByText('10.0.0.7:17801')).toBeTruthy();
    // 两条趋势序列渲染两张图，其余两张显示空态。/ Two trend series render
    // two charts; the other two show the empty placeholder.
    expect(screen.getAllByTestId('platform-trend-chart')).toHaveLength(2);
    expect(screen.getAllByText('暂无趋势数据')).toHaveLength(2);
  });

  it('shows the degraded card when embedded observability is disabled', async () => {
    vi.spyOn(MonitoringService, 'getPlatformSnapshotSafe').mockResolvedValue({
      success: true,
      data: {available: false, reason: 'observability_disabled', agents: []},
    });
    renderPanel();

    expect(
      await screen.findByText('可观测性未启用，平台自监控不可用'),
    ).toBeTruthy();
    expect(screen.getByText(/\/api\/v1\/health/)).toBeTruthy();
  });

  it('shows the empty-agents state and keeps the table structure', async () => {
    vi.spyOn(MonitoringService, 'getPlatformSnapshotSafe').mockResolvedValue({
      success: true,
      data: snapshot({agents: []}),
    });
    renderPanel();

    expect(await screen.findByText('暂无已注册 Agent')).toBeTruthy();
    expect(screen.getByText('Agent 进程')).toBeTruthy();
  });

  it('renders English copy for the en locale', async () => {
    vi.spyOn(MonitoringService, 'getPlatformSnapshotSafe').mockResolvedValue({
      success: true,
      data: snapshot(),
    });
    renderPanel(en, 'en');

    expect(await screen.findByText('Agent processes')).toBeTruthy();
    expect(screen.getByText('Online')).toBeTruthy();
  });
});
