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
import {MetricsDialogContent} from '../StudioDialogs';
import type {MetricUnitMode} from '../sync-studio-utils';
import {clearJobTimeseriesCache} from '../job-detail/metric-timeseries';

vi.mock('@/components/charts/sankey-chart', () => ({
  SankeyChart: ({label}: {label: string}) => (
    <section aria-label={label} data-testid='snapshot-sankey-chart'>
      {label}
    </section>
  ),
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

const copy = zh.workbenchStudio;

function job(partial: Partial<SyncJobInstance> = {}): SyncJobInstance {
  return {
    id: 26,
    task_id: 11,
    task_version: 3,
    run_type: 'run',
    platform_job_id: '179312051430100000',
    engine_job_id: '179312051430100000',
    status: 'finished',
    submit_spec: {},
    result_preview: {metrics: {}},
    error_message: '',
    created_by: 1,
    created_at: '',
    ...partial,
  } as SyncJobInstance;
}

function renderSnapshot(
  instance: SyncJobInstance | null,
  opts?: {unit?: MetricUnitMode; onOpenTrend?: () => void; onOpenCheckpoint?: () => void},
) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh} timeZone='Asia/Shanghai'>
      <MetricsDialogContent
        job={instance}
        unit={opts?.unit}
        onOpenTrend={opts?.onOpenTrend}
        onOpenCheckpoint={opts?.onOpenCheckpoint}
      />
    </NextIntlClientProvider>,
  );
}

const healthyMetrics = {
  SourceReceivedCount: 5000,
  SinkWriteCount: 5000,
  SinkCommittedCount: 5000,
  SourceReceivedBytesPerSeconds: 1_280_000,
  SinkWriteBytesPerSeconds: 1_280_000,
  SinkWriteQPS: 0.65,
};

const tableMetrics = (count: number) => {
  const source: Record<string, number> = {};
  const sink: Record<string, number> = {};
  const committed: Record<string, number> = {};
  for (let i = 0; i < count; i += 1) {
    source[`Source[${i}].db.t${i}`] = 100;
    sink[`Sink[${i}].db.t${i}`] = 100;
    committed[`Sink[${i}].db.t${i}`] = 100;
  }
  return {
    TableSourceReceivedCount: source,
    TableSinkWriteCount: sink,
    TableSinkCommittedCount: committed,
  };
};

describe('MetricsDialogContent', () => {
  beforeEach(() => {
    vi.mocked(SyncService.getJobTimeseries).mockReset();
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
    clearJobTimeseriesCache();
  });

  it('renders KPIs and a neutral conclusion without the old status bar', () => {
    renderSnapshot(job({result_preview: {metrics: healthyMetrics}}));
    expect(screen.getByText(copy.metricFlowKpiRead)).toBeInTheDocument();
    expect(screen.getByText(copy.metricFlowKpiWrite)).toBeInTheDocument();
    expect(screen.getByText(copy.pipelineWriteRate)).toBeInTheDocument();
    // 状态条已删除：不再出现 Engine 芯片、状态徽章与「吞吐达成」。
    // The status strip is gone: no Engine chip, no status badge, no sync-ratio label.
    expect(screen.queryByText(/^Engine:/)).not.toBeInTheDocument();
    expect(screen.queryByText('吞吐达成:')).not.toBeInTheDocument();
    expect(screen.getByText(`${copy.metricFlowBalanced} · ${copy.metricFlowAllCommitted}`)).toBeInTheDocument();
    expect(screen.getByRole('img')).toBeInTheDocument();
    // 终态作业不请求 timeseries。 / Terminal jobs never fetch timeseries.
    expect(SyncService.getJobTimeseries).not.toHaveBeenCalled();
  });

  it('shows a warning conclusion when write lags behind read', () => {
    renderSnapshot(
      job({
        status: 'running',
        result_preview: {
          metrics: {
            SourceReceivedCount: 5000,
            SinkWriteCount: 3100,
            SinkCommittedCount: 2800,
          },
        },
      }),
    );
    expect(screen.getByText(/已写入 62%/)).toBeInTheDocument();
  });

  it('expands per-table details by default and collapses on click', () => {
    renderSnapshot(
      job({
        result_preview: {
          metrics: {...healthyMetrics, ...tableMetrics(3)},
        },
      }),
    );
    const fold = screen.getByRole('button', {name: /逐表明细/});
    expect(fold).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByText(copy.metricFlowLink)).toBeInTheDocument();
    fireEvent.click(fold);
    expect(screen.queryByText(copy.metricFlowLink)).not.toBeInTheDocument();
    // 链路数 ≤ 5 时搜索框不渲染。 / Search stays hidden at 5 links or fewer.
    expect(screen.queryByPlaceholderText(copy.pipelineSearchTable)).not.toBeInTheDocument();
  });

  it('shows the search box only beyond five links', () => {
    renderSnapshot(
      job({
        result_preview: {
          metrics: {...healthyMetrics, ...tableMetrics(6)},
        },
      }),
    );
    const search = screen.getByPlaceholderText(copy.pipelineSearchTable);
    fireEvent.change(search, {target: {value: 'db.t5'}});
    expect(screen.getByText('Sink #6')).toBeInTheDocument();
    expect(screen.queryByText('Sink #1')).not.toBeInTheDocument();
  });

  it('requests timeseries only for running jobs', async () => {
    vi.mocked(SyncService.getJobTimeseries).mockResolvedValue({
      source: 'prometheus',
      range: '1h',
      start_ms: 0,
      end_ms: 0,
      step_seconds: 15,
      engine_observability_enabled: false,
      series: [
        {
          key: 'throughput_write',
          label: 'write',
          unit: 'records/s',
          points: Array.from({length: 12}, (_, i) => [i * 1000, i]),
        },
      ],
      annotations: [],
    } as SyncJobTimeseries);
    renderSnapshot(
      job({status: 'running', result_preview: {metrics: healthyMetrics}}),
    );
    await waitFor(() => expect(SyncService.getJobTimeseries).toHaveBeenCalled());
    expect(vi.mocked(SyncService.getJobTimeseries).mock.calls[0][0]).toBe(26);
  });

  it('folds extended metrics and zero metrics into the more-metrics row', () => {
    renderSnapshot(
      job({
        result_preview: {
          metrics: {
            ...healthyMetrics,
            CheckpointDurationAvg: 320,
            RestartsCount: 0,
          },
        },
      }),
    );
    const more = screen.getByRole('button', {name: /更多指标/});
    expect(more).toHaveAttribute('aria-expanded', 'false');
    fireEvent.click(more);
    expect(screen.getByText('CheckpointDurationAvg')).toBeInTheDocument();
    expect(screen.getByText(/RestartsCount/)).toBeInTheDocument();
  });

  it('keeps the empty state for jobs without metrics', () => {
    renderSnapshot(job());
    expect(screen.getByText(copy.noMetricsOutput)).toBeInTheDocument();
  });

  it('switches KPI, delivery bar, and per-table columns when unit is bytes', async () => {
    const onOpenTrend = vi.fn();
    renderSnapshot(
      job({
        status: 'running',
        result_preview: {
          metrics: {
            SourceReceivedCount: 70,
            SinkWriteCount: 70,
            SinkCommittedCount: 62,
            SourceReceivedBytes: 65536,
            SinkWriteBytes: 65536,
            SinkCommittedBytes: 49152,
            SinkWriteBytesPerSeconds: 2048,
            TableSourceReceivedCount: {'Source[0].db.a': 50, 'Source[1].db.b': 20},
            TableSinkWriteCount: {'Sink[0].db.a': 50, 'Sink[1].db.b': 20},
            TableSourceReceivedBytes: {'Source[0].db.a': 16384, 'Source[1].db.b': 49152},
            TableSinkWriteBytes: {'Sink[0].db.a': 16384, 'Sink[1].db.b': 49152},
            TableSinkCommittedBytes: {'Sink[0].db.a': 16384, 'Sink[1].db.b': 32768},
          },
        },
      }),
      {unit: 'bytes', onOpenTrend},
    );
    expect(screen.getByText(copy.metricFlowKpiReadBytes)).toBeInTheDocument();
    expect(screen.getByText(copy.metricFlowKpiWriteBytes)).toBeInTheDocument();
    expect(screen.getByText(copy.metricFlowKpiWriteThroughput)).toBeInTheDocument();
    expect(screen.getByText(copy.metricFlowReadBytes)).toBeInTheDocument();
    await waitFor(() =>
      expect(SyncService.getJobTimeseries).toHaveBeenCalledWith(26, {
        range: '1h',
        metrics: 'bytes_throughput',
      }),
    );

    fireEvent.click(screen.getByRole('button', {name: /查看趋势/}));
    expect(onOpenTrend).toHaveBeenCalledTimes(1);

    // 多链路时可在逐表明细中就地切换至流向图。 / Multi-link jobs can switch per-table details to Sankey flow in place.
    fireEvent.click(screen.getByRole('button', {name: copy.metricFlowDetailFlow}));
    expect(screen.getByTestId('snapshot-sankey-chart')).toBeInTheDocument();
  });

  it('opens the delivery stages guide popover on click and jumps to checkpoint', () => {
    const onOpenCheckpoint = vi.fn();
    renderSnapshot(
      job({
        result_preview: {
          metrics: {
            SourceReceivedCount: 70,
            SinkWriteCount: 70,
            SinkCommittedCount: 62,
          },
        },
      }),
      {onOpenCheckpoint},
    );
    fireEvent.click(screen.getByRole('button', {name: copy.metricFlowGuideTrigger}));
    expect(screen.getByText(copy.metricFlowGuideTitle)).toBeInTheDocument();
    expect(screen.getByText(copy.metricFlowBarDesc_pendingCommit)).toBeInTheDocument();
    expect(screen.getByText(copy.metricFlowBarDesc_backlog)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', {name: /查看检查点/}));
    expect(onOpenCheckpoint).toHaveBeenCalledTimes(1);
  });
});
