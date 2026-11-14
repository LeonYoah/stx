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

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
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

vi.mock('@/components/arc/lib/use-today', () => ({
  useToday: () => new Date(2026, 10, 14),
}));

// 模拟选择器：暴露 withTime / 当前值，点击时交回一段精确到分钟的区间。
// Mock picker: exposes withTime and the current value, and hands back a minute-precise range on click.
const PICKED = {start: new Date(Date.UTC(2026, 10, 14, 4, 15)), end: new Date(Date.UTC(2026, 10, 14, 4, 30, 59, 999))};
vi.mock('@/components/arc/date-range-picker/date-range-picker', () => ({
  DateRangePicker: ({
    label,
    value,
    withTime,
    onChange,
  }: {
    label: string;
    value: {start: Date; end: Date} | null;
    withTime?: boolean;
    onChange?: (range: {start: Date; end: Date}) => void;
  }) => (
    <button
      type='button'
      aria-label={label}
      data-with-time={withTime ? 'true' : 'false'}
      data-value={value ? `${value.start.getTime()}-${value.end.getTime()}` : ''}
      onClick={() => onChange?.(PICKED)}
    >
      picker
    </button>
  ),
  defaultDateRangePresets: [],
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
      start_ms: 0,
      end_ms: 0,
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
      start_ms: 0,
      end_ms: 0,
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

  const START = '2026-11-14T04:13:03Z';
  const FINISH = '2026-11-14T04:38:28Z';
  const emptyPrometheus = (range = 'custom', start = 0, end = 0) => ({
    source: 'prometheus',
    range,
    start_ms: start,
    end_ms: end,
    step_seconds: 10,
    engine_observability_enabled: false,
    series: [],
    annotations: [],
  }) as SyncJobTimeseries;

  it('defaults a finished job to its lifetime and echoes the queried window in the picker', async () => {
    const start = Date.parse(START);
    const end = Date.parse(FINISH);
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue(emptyPrometheus('custom', start, end));
    renderTrend(job({status: 'failed', started_at: START, finished_at: FINISH}));
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalled());
    expect(vi.mocked(SyncService.getJobTimeseries).mock.calls[0][1]).toMatchObject({start, end});
    expect(screen.getByRole('button', {name: copy.rangeLifetime})).toHaveAttribute('aria-pressed', 'true');
    const picker = screen.getByRole('button', {name: copy.customRange});
    expect(picker).toHaveAttribute('data-with-time', 'true');
    await waitFor(() => expect(picker).toHaveAttribute('data-value', `${start}-${end}`));
  });

  it('keeps the last hour for a running job and omits end for its lifetime', async () => {
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue(emptyPrometheus('1h'));
    renderTrend(job({status: 'running', started_at: START}));
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalled());
    expect(vi.mocked(SyncService.getJobTimeseries).mock.calls[0][1]).toMatchObject({range: '1h'});
    fireEvent.click(screen.getByRole('button', {name: copy.rangeLifetime}));
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalledTimes(2));
    const lifetimeQuery = vi.mocked(SyncService.getJobTimeseries).mock.calls[1][1];
    expect(lifetimeQuery).toMatchObject({start: Date.parse(START)});
    expect(lifetimeQuery).not.toHaveProperty('end');
  });

  it('renders per-table chart sections only when table series exist', async () => {
    const t0 = Date.UTC(2026, 10, 14, 2, 0);
    const t1 = Date.UTC(2026, 10, 14, 2, 1);
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue({
      ...emptyPrometheus(),
      series: [
        {key: 'table_throughput_read', label: 'orders', points: [[t0, 10], [t1, 12]]},
        {key: 'table_throughput_read', label: 'users', points: [[t0, 5], [t1, 6]]},
        {key: 'table_rows_write', label: 'orders', points: [[t0, 100], [t1, 220]]},
      ],
    } as SyncJobTimeseries);
    renderTrend(job());
    await waitFor(() => {
      const labels = screen.getAllByRole('region').map((node) => node.getAttribute('aria-label'));
      expect(labels).toContain(copy.charts.tableThroughput);
      expect(labels).toContain(copy.charts.tableRows);
    });
  });

  it('falls back to updated_at when finished_at is before the start', async () => {
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue(emptyPrometheus());
    renderTrend(job({
      status: 'failed',
      started_at: '2026-11-14T12:13:03Z',
      finished_at: '2026-11-14T04:38:28Z',
      updated_at: '2026-11-14T12:39:44Z',
    } as Partial<SyncJobInstance>));
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalled());
    expect(vi.mocked(SyncService.getJobTimeseries).mock.calls[0][1]).toMatchObject({
      start: Date.parse('2026-11-14T12:13:03Z'),
      end: Date.parse('2026-11-14T12:39:44Z'),
    });
  });

  it('queries a custom window picked on the calendar', async () => {
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue(emptyPrometheus());
    renderTrend(job({status: 'failed', started_at: START, finished_at: FINISH}));
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalledTimes(1));
    fireEvent.click(screen.getByRole('button', {name: copy.customRange}));
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalledTimes(2));
    expect(vi.mocked(SyncService.getJobTimeseries).mock.calls[1][1]).toMatchObject({
      start: PICKED.start.getTime(),
      end: PICKED.end.getTime(),
    });
    expect(screen.getByRole('button', {name: copy.rangeLifetime})).toHaveAttribute('aria-pressed', 'false');
  });
});
