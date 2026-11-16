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
} from '@/lib/services/dashboard';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {formatCompactNumber} from './dashboard-format';
import {TrendEmptyState} from './TrendEmptyState';
import type {DashboardQuery} from './use-dashboard-query';
import {usePolledResource} from './use-polled-resource';

const JOBS_POLL_MS = 30_000;
const THROUGHPUT_KEYS = ['write', 'read', 'write_bytes', 'read_bytes'] as const;
type ThroughputKey = (typeof THROUGHPUT_KEYS)[number];

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
      {extra ? <div className='ml-auto'>{extra}</div> : null}
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
              cell: t('calendar.cell'),
              cellUndated: t('calendar.cellUndated'),
              range: t('calendar.range'),
              days: t('calendar.days'),
              less: t('calendar.less'),
              more: t('calendar.more'),
              level: t('calendar.level'),
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
          <TrendEmptyState />
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
