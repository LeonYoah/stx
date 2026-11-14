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

import {render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {SyncService, type SyncJobInstance, type SyncJobTimeseries} from '@/lib/services/sync';
import {MetricsTrendView} from '../MetricsTrendView';

if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver;
}

vi.mock('@/lib/i18n', () => ({
  useLocale: () => ({locale: 'zh'}),
}));

vi.mock('@/lib/services/sync', async () => {
  const actual = await vi.importActual<typeof import('@/lib/services/sync')>('@/lib/services/sync');
  return {
    ...actual,
    SyncService: {
      ...actual.SyncService,
      getJobTimeseries: vi.fn(),
    },
  };
});

vi.mock('@/components/arc/brush-chart/brush-chart', () => ({
  BrushChart: ({label}: {label: string}) => <div data-testid='arc-brush-chart'>{label}</div>,
}));

vi.mock('@/components/arc/line-chart/line-chart', () => ({
  LineChart: ({label}: {label: string}) => <div data-testid='arc-line-chart'>{label}</div>,
}));

vi.mock('@/components/arc/bar-chart/bar-chart', () => ({
  BarChart: ({label}: {label: string}) => <div data-testid='arc-bar-chart'>{label}</div>,
}));

vi.mock('@/components/arc/donut-chart/donut-chart', () => ({
  DonutChart: ({label}: {label: string}) => <div data-testid='arc-donut-chart'>{label}</div>,
}));

const copy = zh.workbenchStudio.jobDetail.metricsView;

function job(partial: Partial<SyncJobInstance> = {}): SyncJobInstance {
  return {
    id: 26,
    task_id: 11,
    task_version: 3,
    run_type: 'run',
    platform_job_id: '179312051430100000',
    engine_job_id: '179312051430100000',
    status: 'finished',
    submit_spec: {cluster_id: 1},
    result_preview: {metrics: {SourceReceivedQPS: 12, SinkWriteQPS: 10}},
    error_message: '',
    created_by: 1,
    created_at: '',
    ...partial,
  } as SyncJobInstance;
}

function renderTrend(instance: SyncJobInstance | null) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <MetricsTrendView job={instance} active />
    </NextIntlClientProvider>,
  );
}

describe('MetricsTrendView', () => {
  beforeEach(() => {
    vi.mocked(SyncService.getJobTimeseries).mockReset();
  });

  it('shows the observability empty state when Prometheus is off', async () => {
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue({
      source: 'none',
      reason: 'observability_disabled',
      range: '1h',
      step_seconds: 0,
      engine_observability_enabled: false,
      series: [],
      annotations: [],
    } as SyncJobTimeseries);
    renderTrend(job());
    expect(await screen.findByText(copy.emptyTitle)).toBeInTheDocument();
    expect(screen.queryByTestId('arc-line-chart')).not.toBeInTheDocument();
  });

  it('renders Arc brush, line, bar, and donut charts from timeseries', async () => {
    const t0 = Date.UTC(2026, 10, 14, 2, 0);
    const t1 = Date.UTC(2026, 10, 14, 2, 1);
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue({
      source: 'prometheus',
      range: '1h',
      step_seconds: 15,
      engine_observability_enabled: false,
      series: [
        {key: 'throughput_read', label: 'read', points: [[t0, 10], [t1, 12]]},
        {key: 'throughput_write', label: 'write', points: [[t0, 8], [t1, 9]]},
        {key: 'rows_read', label: 'read', points: [[t1, 100]]},
        {key: 'rows_write', label: 'write', points: [[t1, 80]]},
        {key: 'rows_committed', label: 'committed', points: [[t1, 50]]},
        {key: 'cdc_delay', label: 'cdc', points: [[t0, 1.2], [t1, 0.8]]},
        {key: 'checkpoint_duration', label: 'ckpt', points: [[t0, 0.4], [t1, 0.5]]},
      ],
      annotations: [{ts_ms: t1, kind: 'restart'}],
    } as SyncJobTimeseries);

    renderTrend(job());

    await waitFor(() => {
      expect(screen.getByTestId('arc-brush-chart')).toHaveTextContent(copy.charts.cdc);
    });
    expect(screen.getAllByRole('region').map((node) => node.getAttribute('aria-label'))).toEqual([
      copy.charts.throughput,
      copy.charts.rows,
      copy.charts.cdc,
      copy.charts.checkpoint,
      copy.pipeline.label,
      copy.charts.busy,
      copy.charts.backpressure,
    ]);
    expect(screen.getAllByTestId('arc-line-chart').map((node) => node.textContent)).toEqual(
      expect.arrayContaining([copy.charts.throughput, copy.charts.rows]),
    );
    expect(screen.getByTestId('arc-bar-chart')).toHaveTextContent(copy.charts.checkpoint);
    expect(screen.getByTestId('arc-donut-chart')).toHaveTextContent(copy.pipeline.label);
    expect(screen.getAllByText(copy.engineObsEmptyTitle)).toHaveLength(2);
  });
});
