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
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {HomeSummary, HomeVolume} from '@/lib/services/dashboard';
import {DashboardKpiRow} from '../DashboardKpiRow';

vi.mock('@/components/arc/metric-card/metric-card', () => ({
  MetricCard: (props: {
    label: string;
    value: number;
    suffix?: string;
    context: string;
    change?: string;
  }) => (
    <div data-testid='metric-card'>
      {props.label}|{props.value}
      {props.suffix ?? ''}|{props.context}|{props.change ?? ''}
    </div>
  ),
}));

const summary: HomeSummary = {
  generated_at: '2026-11-14T12:00:00Z',
  jobs: {
    running: 9,
    running_streaming: 7,
    running_batch: 2,
    failed_24h: 2,
    failed_prev_24h: 1,
    write_qps: 27200.4,
    read_qps: 28000,
    max_cdc_delay_seconds: 74.2,
    max_cdc_job_name: 'pg-cdc-ledger',
    cdc_threshold_seconds: 60,
    trend_source: 'prometheus',
    write_trend: [
      [0, 100],
      [3_600_000, 106],
    ],
    cdc_trend: [],
    attention: [],
  },
  clusters: {
    total: 2,
    healthy: 1,
    degraded: 1,
    unhealthy: 0,
    nodes_online: 4,
    nodes_total: 4,
  },
  alerts: {active: 3, critical: 3},
  attention: [],
};

const volume: HomeVolume = {
  source: 'prometheus',
  range: '1h',
  start_ms: 0,
  end_ms: 3_600_000,
  split: 'none',
  oldest_sample_ms: 0,
  totals: {
    window: {read_rows: 0, write_rows: 1_523_000, read_bytes: 0, write_bytes: 0},
    today: {read_rows: 0, write_rows: 5_000_000, read_bytes: 0, write_bytes: 0},
    month: {read_rows: 0, write_rows: 60_000_000, read_bytes: 0, write_bytes: 0},
    today_start_ms: 0,
    month_start_ms: 0,
  },
  buckets: [],
  series: [],
};

const renderRow = (
  data: HomeSummary | null,
  loading = false,
  vol: HomeVolume | null | undefined = undefined,
) =>
  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <DashboardKpiRow
        summary={data}
        volume={vol}
        loading={loading}
        buildHref={(patch) => `/dashboard?${new URLSearchParams(patch)}`}
      />
    </NextIntlClientProvider>,
  );

describe('DashboardKpiRow', () => {
  it('renders the seven KPIs with neutral failure wording and a signed write-rate change', () => {
    renderRow(summary, false, volume);
    const cards = screen
      .getAllByTestId('metric-card')
      .map((node) => node.textContent);
    expect(cards).toHaveLength(7);
    expect(cards[0]).toBe('运行中作业|9|流 7 · 批 2|');
    // 大数字经 scaledCompact 缩放：27,200 → 27.2K；1,523,000 → 1.5M，避免溢出相邻卡片。
    // Large numbers go through scaledCompact: 27,200 → 27.2K, 1,523,000 → 1.5M, so long digits
    // never overflow into the neighboring card.
    expect(cards[1]).toBe('全局写入速率|27.2K 行/秒|较 1 小时前|+6%');
    expect(cards[2]).toBe('最大 CDC 延迟|74 秒|pg-cdc-ledger|超阈值');
    // 失败数用中性文字，不带正负号，避免 Arc 染成绿色。/ Neutral failure wording, no sign, so Arc never paints it green.
    expect(cards[3]).toBe('失败作业|2|近 24 小时（含失败中）|昨日 1');
    expect(cards[4]).toBe('数据量|1.5M 行|今日 5M · 本月 60M|');
    expect(cards[5]).toBe('集群健康|1/2|节点 4/4 在线|1 个降级');
    expect(screen.getByTestId('home-kpi-failed').getAttribute('href')).toBe(
      '/dashboard?tab=list&status=failed',
    );
    expect(screen.getByTestId('home-kpi-volume').getAttribute('href')).toBe(
      '/dashboard?tab=jobs',
    );
    expect(screen.getByTestId('home-kpi-alerts').getAttribute('href')).toBe(
      '/monitoring',
    );
  });

  it('marks the month caption when the oldest sample starts after month start', () => {
    const clipped: HomeVolume = {
      ...volume,
      oldest_sample_ms: Date.UTC(2026, 10, 10),
      totals: {
        ...volume.totals!,
        month_start_ms: Date.UTC(2026, 10, 1),
      },
    };
    renderRow(summary, false, clipped);
    expect(screen.getByTestId('home-kpi-volume').textContent).toContain(
      '自 2026/11/10 起',
    );
  });

  it('falls back to an off caption without volume totals', () => {
    renderRow(summary, false, {
      ...volume,
      source: 'none',
      reason: 'observability_disabled',
      totals: null,
    });
    expect(screen.getByTestId('home-kpi-volume').textContent).toContain(
      '可观测未开启',
    );
  });

  it('keeps a fixed-height skeleton while the first load is pending', () => {
    renderRow(null, true);
    expect(screen.getByTestId('home-kpi-loading').children).toHaveLength(7);
  });
});
