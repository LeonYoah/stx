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

vi.mock('@/components/arc/animated-counter/animated-counter', () => ({
  AnimatedCounter: (props: {
    value: number;
    decimals?: number;
    suffix?: string;
  }) => (
    <span data-testid='counter'>
      {props.value}
      {props.suffix ?? ''}
    </span>
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

const cellOf = (testId: string) =>
  screen.getByTestId(testId).closest('li') as HTMLElement;

describe('DashboardKpiRow', () => {
  it('renders the seven KPI cells as links with their hrefs unchanged', () => {
    renderRow(summary, false, volume);
    expect(screen.getByTestId('home-kpi-row').tagName).toBe('NAV');
    for (const key of [
      'running',
      'write',
      'cdc',
      'failed',
      'volume',
      'clusters',
      'alerts',
    ]) {
      const link = screen.getByTestId(`home-kpi-${key}`);
      expect(link.tagName).toBe('A');
      expect(link).toHaveTextContent(/\S/);
    }
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

  it('scales values, renders the magnitude next to digits and the unit as small text', () => {
    renderRow(summary, false, volume);
    // 大数字经 scaledCompact 缩放：27,200 → 27.2K；1,523,000 → 1.5M。量级与数字同字号，
    // 计量单位是独立的小号节点，不混进计数器。
    // Large numbers go through scaledCompact: 27,200 → 27.2K, 1,523,000 → 1.5M.
    // The magnitude shares the digits' size; the measure unit is a separate small node.
    const write = screen.getByTestId('home-kpi-write');
    expect(write).toHaveTextContent('27.2K');
    expect(write).toHaveTextContent('行/秒');
    const counters = write.querySelectorAll("[data-testid='counter']");
    expect(counters).toHaveLength(1);
    expect(counters[0].textContent).toBe('27.2K');
    expect(counters[0].textContent).not.toContain('行/秒');
    expect(screen.getByTestId('home-kpi-volume')).toHaveTextContent('1.5M');
    expect(screen.getByTestId('home-kpi-volume')).toHaveTextContent(
      '今日 5M · 本月 60M',
    );
  });

  it('tints danger cells: failures > 0, CDC over threshold, critical alerts', () => {
    renderRow(summary, false, volume);
    expect(cellOf('home-kpi-failed').dataset.tone).toBe('danger');
    // CDC 超阈值：数值染红，说明以「超阈值 ·」开头。/ CDC over threshold: danger tint, caption opens with "超阈值 ·".
    expect(cellOf('home-kpi-cdc').dataset.tone).toBe('danger');
    expect(screen.getByTestId('home-kpi-cdc').textContent).toContain(
      '超阈值 · pg-cdc-ledger',
    );
    expect(cellOf('home-kpi-alerts').dataset.tone).toBe('danger');
    // 集群有降级 → 警告色，说明以「N 个降级 ·」开头。/ Degraded clusters → warning tint, caption opens with "N 个降级 ·".
    expect(cellOf('home-kpi-clusters').dataset.tone).toBe('warning');
    expect(screen.getByTestId('home-kpi-clusters').textContent).toContain(
      '1 个降级 · 节点 4/4 在线',
    );
    // 其余格不染色。/ The rest carry no tone.
    expect(cellOf('home-kpi-running')).not.toHaveAttribute('data-tone');
    expect(cellOf('home-kpi-write')).not.toHaveAttribute('data-tone');
    expect(cellOf('home-kpi-volume')).not.toHaveAttribute('data-tone');
  });

  it('clears the tone when failures, CDC and alerts are all fine', () => {
    renderRow(
      {
        ...summary,
        jobs: {...summary.jobs!, failed_24h: 0, max_cdc_delay_seconds: 10},
        clusters: {...summary.clusters, degraded: 0},
        alerts: {active: 1, critical: 0},
      },
      false,
      volume,
    );
    expect(cellOf('home-kpi-failed')).not.toHaveAttribute('data-tone');
    expect(cellOf('home-kpi-cdc')).not.toHaveAttribute('data-tone');
    expect(cellOf('home-kpi-clusters')).not.toHaveAttribute('data-tone');
    expect(cellOf('home-kpi-alerts')).not.toHaveAttribute('data-tone');
  });

  it('shows a signed up/down change only on the write rate', () => {
    renderRow(summary, false, volume);
    const write = screen.getByTestId('home-kpi-write');
    // 写入速率上涨：↗ +6%（涨绿）。/ Rising write rate: up arrow +6%.
    expect(write.textContent).toContain('+6%');
    expect(write.textContent).toContain('较 1 小时前');
    // 失败数与昨日数仍是中性文字，不带正负号。/ Failure and yesterday counts stay neutral, no sign.
    expect(screen.getByTestId('home-kpi-failed').textContent).toContain(
      '近 24 小时 · 昨日 1',
    );
    expect(screen.getByTestId('home-kpi-failed').textContent).not.toContain(
      '+1',
    );
  });

  it('marks a falling write rate and falls back to "current total" without a trend', () => {
    const falling: HomeSummary = {
      ...summary,
      jobs: {
        ...summary.jobs!,
        write_trend: [
          [0, 100],
          [3_600_000, 50],
        ],
      },
    };
    const {unmount} = renderRow(falling, false, volume);
    expect(screen.getByTestId('home-kpi-write').textContent).toContain('−50%');
    unmount();
    const noTrend: HomeSummary = {
      ...summary,
      jobs: {...summary.jobs!, write_trend: []},
    };
    renderRow(noTrend, false, volume);
    expect(screen.getByTestId('home-kpi-write').textContent).toContain(
      '当前合计（10 秒对账）',
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

  it('keeps a same-grid skeleton while the first load is pending', () => {
    renderRow(null, true);
    // 骨架与真实指标栏同一底板与网格：ul 下仍是 7 个格子。
    // The skeleton shares the strip's board and grid: seven cells under the ul.
    const board = screen.getByTestId('home-kpi-loading');
    expect(board.querySelector('ul')?.children).toHaveLength(7);
  });
});
