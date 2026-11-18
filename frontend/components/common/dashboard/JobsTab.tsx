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

'use client';

import {type CSSProperties, useMemo, useState} from 'react';
import {useFormatter, useLocale, useTranslations} from 'next-intl';
import {BarChart as ArcBarChart} from '@/components/arc/bar-chart/bar-chart';
import {BrushChart} from '@/components/arc/brush-chart/brush-chart';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {BarChart} from '@/components/charts/bar-chart';
import {LocaleProvider} from '@/components/charts/beui-i18n';
import {HeatCalendar} from '@/components/charts/heat-calendar';
import {SankeyChart} from '@/components/charts/sankey-chart';
import {
  StudioTable,
  StudioTableBody,
  StudioTableCell,
  StudioTableColumn,
  StudioTableContent,
  StudioTableEmptyRow,
  StudioTableHeader,
  StudioTableRow,
  StudioTableScroll,
} from '@/components/common/sync/StudioDataTable';
import {
  brushMinSpan,
  toBrushChartData,
} from '@/components/common/sync/job-detail/metrics-trend-data';
import {
  HomeService,
  type HomeJobDashboard,
  type HomeJobRank,
  type HomeVolume,
} from '@/lib/services/dashboard';
import {formatDateTimeMinuteTz, formatDateTz} from '@/lib/timezone';
import {
  formatBytes,
  formatCompactNumber,
  SERIES_PALETTE,
} from './dashboard-format';
import {TrendEmptyState} from './TrendEmptyState';
import type {DashboardQuery} from './use-dashboard-query';
import {usePolledResource} from './use-polled-resource';

const JOBS_POLL_MS = 30_000;
const THROUGHPUT_KEYS = ['write', 'read', 'write_bytes', 'read_bytes'] as const;
type ThroughputKey = (typeof THROUGHPUT_KEYS)[number];

const VOLUME_METRICS = [
  'write_rows',
  'read_rows',
  'write_bytes',
  'read_bytes',
] as const;
type VolumeMetricKey = (typeof VOLUME_METRICS)[number];
const VOLUME_SPLITS = ['none', 'cluster', 'task'] as const;
type VolumeSplit = (typeof VOLUME_SPLITS)[number];

/** 拆分序列色板：直接用共享 SERIES_PALETTE，同组在各图里同色。/ Split-series colors: reuse the shared SERIES_PALETTE so a group keeps one color across charts. */
const VOLUME_COLORS = SERIES_PALETTE;

/** 周桶标签的「MM/DD」格式化。/ MM/DD formatter for week-bucket labels. */
const formatDay = (date: Date) =>
  `${String(date.getMonth() + 1).padStart(2, '0')}/${String(date.getDate()).padStart(2, '0')}`;

/** 读用 --series-1、写用 --series-2，与作业详情趋势图同色。/ Read --series-1, write --series-2, matching job trends. */
const THROUGHPUT_COLOR: Record<ThroughputKey, string> = {
  write: 'var(--series-2)',
  read: 'var(--series-1)',
  write_bytes: 'var(--series-2)',
  read_bytes: 'var(--series-1)',
};

/**
 * 首页作业页签：全局吞吐（事件标注）→ 写入 / CDC 延迟 Top 8 → 数据流向 + 运行日历 → Checkpoint + 算子热点。
 * 聚合为全局口径；未接入 Prometheus 时只有运行日历出图。
 * The home jobs tab: total throughput (with event markers) → top 8 writers / CDC delay → data flow + run calendar →
 * checkpoints + operator hotspots. Aggregates are global; without Prometheus only the run calendar renders.
 */
export function JobsTab({query}: {query: DashboardQuery}) {
  const t = useTranslations('dashboard.home.jobsTab');
  const format = useFormatter();
  const locale = useLocale();
  const [metric, setMetric] = useState<ThroughputKey>('write');
  const [calendarMode, setCalendarMode] = useState<'runs' | 'failed'>('runs');
  const [volumeMetric, setVolumeMetric] = useState<VolumeMetricKey>('write_rows');
  const [volumeSplit, setVolumeSplit] = useState<VolumeSplit>('none');
  const resource = usePolledResource<HomeJobDashboard>(
    JSON.stringify([
      'jobs',
      query.clusterId ?? 'all',
      query.range,
      query.start,
      query.end,
    ]),
    () =>
      HomeService.getJobs({
        cluster_id: query.clusterId,
        ...(query.start && query.end
          ? {start: query.start, end: query.end}
          : {range: query.range}),
      }),
    // 自定义时间窗固定不变，不自动刷新。/ A custom window is fixed, so it does not auto-refresh.
    query.start ? 0 : JOBS_POLL_MS,
  );
  const data = resource.data;
  const hasMetrics = data?.source === 'prometheus';
  const panel = 'rounded-xl border border-border/70 bg-card/40 p-4';
  const isBytes = metric.endsWith('bytes');
  const formatRate = (value: number) => `${formatCompactNumber(value)}/s`;

  // 数据量面板独立拉取：桶粒度由后端按窗口跨度自动选（auto），只有拆分会重写序列形状，
  // 全部指标一次取回再本地切换。
  // The volume panel fetches on its own: the backend auto-picks the bucket granularity from the
  // window span, only the split changes the series shape, and all four metrics arrive together
  // so the metric toggle stays local.
  const volume = usePolledResource<HomeVolume>(
    JSON.stringify([
      'volume',
      query.clusterId ?? 'all',
      query.range,
      query.start,
      query.end,
      volumeSplit,
    ]),
    () =>
      HomeService.getVolume({
        cluster_id: query.clusterId,
        granularity: 'auto',
        split: volumeSplit,
        ...(query.start && query.end
          ? {start: query.start, end: query.end}
          : {range: query.range}),
      }),
    query.start ? 0 : JOBS_POLL_MS,
  );
  const volumeData = volume.data;
  const volumeIsBytes = volumeMetric.endsWith('bytes');
  // 字节口径统一走 formatBytes（IEC），不再用 compact+B（compact 的 B 是 Billion 会撞名）。
  // Byte metrics go through formatBytes (IEC) — compact+"B" collided since compact's B
  // already means Billion.
  const formatVolume = (value: number) =>
    volumeIsBytes ? formatBytes(value) : formatCompactNumber(value);
  const volumeChart = useMemo(() => {
    const buckets = volumeData?.buckets ?? [];
    const rows = (volumeData?.series ?? []).filter(
      (item) => item.metric === volumeMetric,
    );
    const granularity = volumeData?.granularity;
    // 亚日粒度的标签是 "YYYY-MM-DD HH:mm"：窗口跨天时只截 HH:mm 会出重复类目
    // （前后两天同一时刻撞名，且 24h 窗的首尾残缺桶同名）。同日才截 HH:mm，跨天带 MM-DD。
    // Sub-day labels look like "YYYY-MM-DD HH:mm": slicing to HH:mm duplicates categories
    // across days (same clock time on two dates, plus the two clipped ends of a 24h
    // window). Only same-day windows may drop the date; otherwise keep MM-DD.
    const subDay =
      granularity === 'quarter_hour' || granularity === 'hour';
    const sameDay =
      subDay &&
      buckets.length > 0 &&
      buckets.every(
        (bucket) => bucket.label.slice(0, 10) === buckets[0].label.slice(0, 10),
      );
    return {
      categories: buckets.map((bucket) => {
        // 亚日粒度标签带时刻（"YYYY-MM-DD HH:mm"），同日取 "HH:mm"、跨日取 "MM-DD"；
        // 日取 "MM-DD"，周取「周一–周日」，月原样。no_data 桶（整桶早于最早样本）加 · 标记，柱体留空。
        // Sub-day labels carry the clock ("YYYY-MM-DD HH:mm"): same-day shows "HH:mm", multi-day
        // shows "MM-DD"; days show "MM-DD", weeks "Mon–Sun", months stay as they are. no_data
        // buckets (entirely before the oldest sample) get a · marker and an empty bar.
        let label: string;
        if (granularity === 'month') {
          label = bucket.label;
        } else if (granularity === 'week') {
          const monday = new Date(`${bucket.label}T00:00:00`);
          const sunday = new Date(monday.getTime() + 6 * 24 * 60 * 60 * 1000);
          label = `${formatDay(monday)}–${formatDay(sunday)}`;
        } else if (granularity === 'day') {
          label = bucket.label.slice(5);
        } else {
          label = sameDay ? bucket.label.slice(11) : bucket.label.slice(5);
        }
        const marked = bucket.no_data ? `${label}·` : label;
        return bucket.partial ? `${marked}*` : marked;
      }),
      series: rows.map((row, index) => ({
        id: row.group_id,
        name:
          volumeSplit === 'none'
            ? t('volume.all')
            : (volumeData?.groups?.[row.group_id] ?? `#${row.group_id}`),
        color: VOLUME_COLORS[index % VOLUME_COLORS.length],
        // no_data 桶画成空柱（NaN 留缺口），不显示成 0。/ no_data buckets render as gaps (NaN), not zeros.
        values: row.totals.map((value, i) => (buckets[i]?.no_data ? NaN : value)),
      })),
      hasPartial: buckets.some((bucket) => bucket.partial),
      hasNoData: buckets.some((bucket) => bucket.no_data),
    };
  }, [volumeData, volumeMetric, volumeSplit, t]);

  const throughput = useMemo(() => {
    const points =
      data?.throughput.find((item) => item.key === metric)?.points ?? [];
    return {
      data: toBrushChartData(points),
      minSpan: brushMinSpan(points, data?.step_seconds ?? 0),
    };
  }, [data, metric]);
  const annotations = useMemo(
    () =>
      (data?.annotations ?? []).map((item) => ({
        date: item.ts_ms,
        label: t(`annotation.${item.kind}`),
        description: item.title,
      })),
    [data, t],
  );

  const bars = (items: HomeJobRank[], name: string, color: string) => ({
    categories: items.map((item) => item.name),
    series: [
      {id: 'value', name, color, values: items.map((item) => item.value)},
    ],
  });

  const flow = useMemo(() => {
    const links = data?.flow ?? [];
    const label = (name: string) => (name === '?' ? t('flow.unknown') : name);
    const nodes = new Map<string, {id: string; label: string}>();
    for (const link of links) {
      nodes.set(`src:${link.source}`, {
        id: `src:${link.source}`,
        label: label(link.source),
      });
      nodes.set(`sink:${link.sink}`, {
        id: `sink:${link.sink}`,
        label: label(link.sink),
      });
    }
    return {
      nodes: Array.from(nodes.values()),
      links: links.map((link) => ({
        source: `src:${link.source}`,
        target: `sink:${link.sink}`,
        value: link.value,
      })),
    };
  }, [data, t]);

  const calendar = data?.calendar;
  const calendarEnd = useMemo(() => {
    if (!calendar?.end_date) {
      return undefined;
    }
    const [y, m, d] = calendar.end_date.split('-').map(Number);
    // beUI 日历按 UTC 日历日排布，用 UTC 构造同一天。/ beUI lays out UTC calendar days, so build the same day in UTC.
    return new Date(Date.UTC(y, m - 1, d));
  }, [calendar?.end_date]);

  const checkpointData = (data?.checkpoints ?? []).map((item) => ({
    key: String(item.job_id),
    label: item.failures
      ? t('checkpoint.failures', {name: item.name, count: item.failures})
      : item.name,
    axisLabel: item.name,
    value: item.max_seconds ?? item.value,
  }));

  const empty = (height: string) => (
    <p
      className={`${height} flex items-center justify-center text-xs text-muted-foreground`}
    >
      {t('empty')}
    </p>
  );
  const header = (title: string, hint: string, extra?: React.ReactNode) => (
    <div className='mb-2 flex flex-wrap items-center gap-2'>
      <h3 className='text-sm font-medium'>{title}</h3>
      <span className='text-xs text-muted-foreground'>{hint}</span>
      {/* 窄屏时控件换到下一行，并在容器内横向滚动，不撑破页面。/ On narrow screens controls wrap and scroll inside. */}
      {extra ? (
        <div className='w-full max-w-full overflow-x-auto sm:ml-auto sm:w-auto'>
          {extra}
        </div>
      ) : null}
    </div>
  );

  if (resource.loading && !data) {
    return (
      <div
        className='grid gap-3 lg:grid-cols-2'
        data-testid='home-jobs-loading'
      >
        <div className='h-[340px] animate-pulse rounded-xl bg-muted/40 lg:col-span-2' />
        <div className='h-[300px] animate-pulse rounded-xl bg-muted/40' />
        <div className='h-[300px] animate-pulse rounded-xl bg-muted/40' />
      </div>
    );
  }

  const calendarBlock = calendar ? (
    <section className={panel} data-testid='home-calendar'>
      {header(
        t('calendar.title'),
        t('calendar.hint', {weeks: calendar.weeks}),
        <SegmentedControl
          label={t('calendar.modeLabel')}
          value={calendarMode}
          onValueChange={(value) => setCalendarMode(value as 'runs' | 'failed')}
          options={[
            {value: 'runs', label: t('calendar.runs')},
            {value: 'failed', label: t('calendar.failed')},
          ]}
        />,
      )}
      <div className='overflow-x-auto'>
        {/* beUI 日历的月份与图例走它自己的 LocaleProvider。/ beUI reads month names and legend text from its LocaleProvider. */}
        <LocaleProvider locale={locale === 'zh' ? 'zh-CN' : 'en-US'}>
          <HeatCalendar
            weeks={calendar.weeks}
            values={calendarMode === 'runs' ? calendar.runs : calendar.failed}
            endDate={calendarEnd}
            color={
              calendarMode === 'runs' ? 'var(--series-1)' : 'var(--danger)'
            }
            unit={
              calendarMode === 'runs'
                ? t('calendar.unitRuns')
                : t('calendar.unitFailed')
            }
            labels={{
              // 带 {var} 的模板必须走 t.raw 原样交给 beUI 插值，t() 会因缺参抛 FORMATTING_ERROR。
              // Templates with {var} must go through t.raw so beUI interpolates them; t() throws FORMATTING_ERROR on missing params.
              cell: t.raw('calendar.cell'),
              cellUndated: t.raw('calendar.cellUndated'),
              range: t.raw('calendar.range'),
              days: t.raw('calendar.days'),
              less: t('calendar.less'),
              more: t('calendar.more'),
              level: t.raw('calendar.level'),
            }}
          />
        </LocaleProvider>
      </div>
    </section>
  ) : null;

  return (
    <div className='flex flex-col gap-3' data-testid='home-jobs-tab'>
      {data?.partial_errors?.length ? (
        <p className='text-xs text-amber-600 dark:text-amber-400' role='status'>
          {t('partial', {keys: data.partial_errors.join(', ')})}
        </p>
      ) : null}
      {!hasMetrics ? (
        <>
          <TrendEmptyState
            reason={data?.reason}
            description={t('noPromDesc')}
            unavailableDescription={t('unavailableDesc')}
          />
          {calendarBlock}
        </>
      ) : (
        <>
          <section className={panel} data-testid='home-throughput'>
            {header(
              t('throughput.title'),
              t('throughput.hint'),
              <SegmentedControl
                label={t('throughput.metricLabel')}
                value={metric}
                onValueChange={(value) => setMetric(value as ThroughputKey)}
                options={THROUGHPUT_KEYS.map((key) => ({
                  value: key,
                  label: t(`throughput.${key}`),
                }))}
              />,
            )}
            <div style={{color: THROUGHPUT_COLOR[metric]}}>
              <BrushChart
                data={throughput.data}
                label={t(`throughput.${metric}`)}
                unit={isBytes ? 'B/s' : '/s'}
                height={240}
                overviewHeight={48}
                minSpan={throughput.minSpan}
                annotations={annotations}
                formatDate={(date) => formatDateTimeMinuteTz(date.getTime())}
                formatTime={(date) =>
                  formatDateTimeMinuteTz(date.getTime()).slice(-5)
                }
                formatValue={formatRate}
                emptyLabel={t('empty')}
              />
            </div>
          </section>

          <section className={panel} data-testid='home-volume'>
            {header(
              t('volume.title'),
              volumeData?.oldest_sample_ms &&
                volumeData.oldest_sample_ms > volumeData.start_ms
                ? t('volume.since', {
                    date: formatDateTz(volumeData.oldest_sample_ms),
                  })
                : t('volume.hint'),
              <div className='flex flex-wrap items-center gap-2'>
                <SegmentedControl
                  label={t('volume.metricLabel')}
                  value={volumeMetric}
                  onValueChange={(value) =>
                    setVolumeMetric(value as VolumeMetricKey)
                  }
                  options={VOLUME_METRICS.map((key) => ({
                    value: key,
                    label: t(`volume.metrics.${key}`),
                  }))}
                />
                <SegmentedControl
                  label={t('volume.splitLabel')}
                  value={volumeSplit}
                  onValueChange={(value) => setVolumeSplit(value as VolumeSplit)}
                  options={VOLUME_SPLITS.map((key) => ({
                    value: key,
                    label: t(`volume.splits.${key}`),
                  }))}
                />
              </div>,
            )}
            {volume.loading && !volumeData ? (
              <div className='h-[240px] animate-pulse rounded-lg bg-muted/40' />
            ) : volumeChart.series.length === 0 ? (
              empty('h-[240px]')
            ) : (
              <>
                <BarChart
                  categories={volumeChart.categories}
                  series={volumeChart.series}
                  layout={volumeSplit === 'none' ? 'grouped' : 'stacked'}
                  label={t('volume.title')}
                  formatValue={formatVolume}
                />
                {volumeChart.hasPartial ? (
                  <p className='mt-1 text-xs text-muted-foreground'>
                    {t('volume.partialHint')}
                  </p>
                ) : null}
                {volumeChart.hasNoData ? (
                  <p className='mt-1 text-xs text-muted-foreground'>
                    {t('volume.noDataHint')}
                  </p>
                ) : null}
              </>
            )}
          </section>

          <div className='grid gap-3 lg:grid-cols-2'>
            <section className={panel} data-testid='home-top-write'>
              {header(t('topWrite.title'), t('topWrite.hint'))}
              {data.top_write.length === 0 ? (
                empty('h-[220px]')
              ) : (
                <BarChart
                  {...bars(
                    data.top_write,
                    t('topWrite.series'),
                    'var(--series-2)',
                  )}
                  orientation='horizontal'
                  label={t('topWrite.title')}
                  formatValue={formatRate}
                />
              )}
            </section>
            <section className={panel} data-testid='home-top-cdc'>
              {header(t('topCdc.title'), t('topCdc.hint'))}
              {data.top_cdc.length === 0 ? (
                empty('h-[220px]')
              ) : (
                <BarChart
                  {...bars(data.top_cdc, t('topCdc.series'), 'var(--warning)')}
                  orientation='horizontal'
                  label={t('topCdc.title')}
                  formatValue={(value) =>
                    `${format.number(value, {maximumFractionDigits: 1})}s`
                  }
                />
              )}
            </section>
            <section className={panel} data-testid='home-flow'>
              {header(t('flow.title'), t('flow.hint'))}
              {flow.links.length === 0 ? (
                empty('h-[240px]')
              ) : (
                <SankeyChart
                  nodes={flow.nodes}
                  links={flow.links}
                  label={t('flow.title')}
                  formatValue={(value) => formatCompactNumber(value)}
                />
              )}
            </section>
            {calendarBlock}
            <section
              className={panel}
              data-testid='home-checkpoints'
              // Arc BarChart 只读 --accent；在本区块内换成 Checkpoint 阶段色。/ Arc BarChart reads --accent only.
              style={{'--accent': 'var(--series-3)'} as CSSProperties}
            >
              {header(t('checkpoint.title'), t('checkpoint.hint'))}
              {checkpointData.length === 0 ? (
                empty('h-[220px]')
              ) : (
                <ArcBarChart
                  data={checkpointData}
                  label={t('checkpoint.title')}
                  period={t('checkpoint.period')}
                  unit={t('checkpoint.unit')}
                  height={220}
                  showAverage={false}
                  averageLabel={t('checkpoint.average')}
                  valueLabel={t('checkpoint.value')}
                  categoryLabel={t('checkpoint.category')}
                  formatValue={(value) =>
                    format.number(value, {maximumFractionDigits: 2})
                  }
                />
              )}
            </section>
            <section
              className='overflow-hidden rounded-xl border border-border/70 bg-card/40'
              data-testid='home-hotspots'
            >
              <div className='px-4 pt-4'>
                {header(t('hotspots.title'), t('hotspots.hint'))}
              </div>
              <StudioTable label={t('hotspots.title')}>
                <StudioTableScroll>
                  <StudioTableContent>
                    <StudioTableHeader>
                      <StudioTableColumn>{t('hotspots.job')}</StudioTableColumn>
                      <StudioTableColumn>
                        {t('hotspots.vertex')}
                      </StudioTableColumn>
                      <StudioTableColumn style={{minWidth: 0}}>
                        {t('hotspots.kind')}
                      </StudioTableColumn>
                      <StudioTableColumn align='end' style={{minWidth: 0}}>
                        {t('hotspots.value')}
                      </StudioTableColumn>
                    </StudioTableHeader>
                    <StudioTableBody>
                      {data.hotspots.length === 0 ? (
                        <StudioTableEmptyRow colSpan={4}>
                          {t('empty')}
                        </StudioTableEmptyRow>
                      ) : (
                        data.hotspots.map((item) => (
                          <StudioTableRow
                            key={`${item.kind}-${item.job_id}-${item.vertex}`}
                          >
                            <StudioTableCell
                              className='max-w-[14rem] truncate'
                              title={item.name}
                            >
                              {item.name}
                            </StudioTableCell>
                            <StudioTableCell className='font-mono text-muted-foreground'>
                              {item.vertex || '-'}
                            </StudioTableCell>
                            <StudioTableCell className='text-muted-foreground'>
                              {t(`hotspots.${item.kind ?? 'busy'}`)}
                            </StudioTableCell>
                            <StudioTableCell
                              align='end'
                              className='tabular-nums'
                            >
                              {format.number(item.value, {
                                style: 'percent',
                                maximumFractionDigits: 0,
                              })}
                            </StudioTableCell>
                          </StudioTableRow>
                        ))
                      )}
                    </StudioTableBody>
                  </StudioTableContent>
                </StudioTableScroll>
              </StudioTable>
            </section>
          </div>
        </>
      )}
    </div>
  );
}
