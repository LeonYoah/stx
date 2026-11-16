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

import {useEffect, useMemo, useState} from 'react';
import Link from 'next/link';
import {useFormatter, useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  TREND_COLORS,
  jobLifetimeWindow,
  toLineChartData,
} from '@/components/common/sync/job-detail/metrics-trend-data';
import {
  getDisplayJobLifecycleStatus,
  getJobStatusBadgeClass,
  getJobStatusLabel,
} from '@/components/common/sync/sync-studio-utils';
import services from '@/lib/services';
import type {SyncJobInstance, SyncJobTimeseries} from '@/lib/services/sync';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {formatCompactNumber} from './dashboard-format';
import {jobDisplayName, jobWorkbenchHref} from './job-list-utils';

/**
 * 作业行预览抽屉：当前值、吞吐趋势（运行中近 1 小时，已结束取作业全程）与失败原因；「打开作业详情」进入工作台深链。
 * Job row preview drawer: live values, throughput trend (last hour while running, lifetime once finished) and the
 * failure reason; "Open job details" goes to the workbench deep link.
 */
export function JobPreviewDrawer({
  job,
  clusterName,
  onOpenChange,
}: {
  job: SyncJobInstance | null;
  clusterName?: string;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations('dashboard.home.preview');
  const tList = useTranslations('dashboard.home.list');
  const format = useFormatter();
  const [trend, setTrend] = useState<SyncJobTimeseries | null>(null);
  const [trendLoading, setTrendLoading] = useState(false);
  const running =
    job?.status === 'running' || job?.status === 'doing_savepoint';

  useEffect(() => {
    setTrend(null);
    if (!job) {
      return;
    }
    const controller = new AbortController();
    const window = running ? null : jobLifetimeWindow(job, false, Date.now());
    setTrendLoading(true);
    services.sync
      .getJobTimeseries(job.id, {
        ...(window ? {start: window.start, end: window.end} : {range: '1h'}),
        metrics: 'throughput',
        signal: controller.signal,
      })
      .then((data) => setTrend(data))
      .catch(() => setTrend(null))
      .finally(() => {
        if (!controller.signal.aborted) {
          setTrendLoading(false);
        }
      });
    return () => controller.abort();
  }, [job, running]);

  const chart = useMemo(() => {
    const points = (key: string) =>
      trend?.series.find((item) => item.key === key)?.points ?? [];
    return toLineChartData(
      [
        {
          key: 'read',
          name: t('read'),
          points: points('throughput_read'),
          color: TREND_COLORS.read,
        },
        {
          key: 'write',
          name: t('write'),
          points: points('throughput_write'),
          color: TREND_COLORS.write,
        },
      ],
      (ts) => formatDateTimeMinuteTz(ts),
      // 轴标签只留 HH:mm。/ Axis labels keep only HH:mm.
      (ts) => formatDateTimeMinuteTz(ts).slice(-5),
    );
  }, [trend, t]);

  const displayStatus = job ? getDisplayJobLifecycleStatus(job) : '';
  const live = job?.live;

  return (
    <Sheet open={job !== null} onOpenChange={onOpenChange}>
      <SheetContent
        className='w-full gap-0 overflow-y-auto sm:max-w-xl'
        data-testid='home-job-preview'
      >
        {job ? (
          <>
            <SheetHeader className='border-b'>
              <SheetTitle className='truncate pr-6'>
                {jobDisplayName(job)}
              </SheetTitle>
              <SheetDescription className='font-mono text-xs'>
                {tList('taskRef', {id: job.task_id})} · #{job.id}
                {job.engine_job_id ? ` · ${job.engine_job_id}` : ''}
              </SheetDescription>
              <div className='flex flex-wrap items-center gap-1.5 pt-1'>
                <Badge
                  variant='outline'
                  className={cn(
                    'rounded-sm px-2 py-0.5 text-[11px]',
                    getJobStatusBadgeClass(displayStatus),
                  )}
                >
                  {getJobStatusLabel(displayStatus)}
                </Badge>
                {job.mode ? (
                  <Badge
                    variant='outline'
                    className='rounded-sm px-2 py-0.5 text-[11px]'
                  >
                    {tList(`mode.${job.mode}`)}
                  </Badge>
                ) : null}
                {clusterName ? (
                  <Badge
                    variant='outline'
                    className='rounded-sm px-2 py-0.5 text-[11px]'
                  >
                    {clusterName}
                  </Badge>
                ) : null}
              </div>
            </SheetHeader>

            <div className='flex flex-col gap-4 p-4'>
              <dl className='grid grid-cols-3 gap-2'>
                {[
                  {
                    label: t('writeRate'),
                    value:
                      live?.write_qps === undefined
                        ? '-'
                        : `${formatCompactNumber(live.write_qps)}/s`,
                  },
                  {
                    label: t('total'),
                    value: formatCompactNumber(live?.write_total),
                  },
                  {
                    label: t('cdc'),
                    value:
                      live?.cdc_delay_seconds === undefined
                        ? '-'
                        : `${format.number(live.cdc_delay_seconds, {maximumFractionDigits: 1})}s`,
                  },
                ].map((item) => (
                  <div
                    key={item.label}
                    className='rounded-lg border border-border/60 bg-muted/20 px-3 py-2'
                  >
                    <dt className='text-xs text-muted-foreground'>
                      {item.label}
                    </dt>
                    <dd className='mt-0.5 truncate text-base font-medium tabular-nums'>
                      {item.value}
                    </dd>
                  </div>
                ))}
              </dl>

              <section className='flex flex-col gap-2'>
                <h3 className='text-sm font-medium'>
                  {running ? t('trend') : t('trendLifetime')}
                </h3>
                {trend && trend.source !== 'prometheus' ? (
                  <p className='rounded-lg border border-dashed border-border/70 px-3 py-6 text-center text-xs text-muted-foreground'>
                    {trend.reason === 'observability_disabled'
                      ? t('noTrend')
                      : t('trendUnavailable')}
                  </p>
                ) : (
                  <LineChart
                    label={running ? t('trend') : t('trendLifetime')}
                    unit='records/s'
                    data={chart.data}
                    series={chart.series}
                    height={180}
                    loading={trendLoading}
                    emptyLabel={t('empty')}
                    clearLabel={t('legendClear')}
                    resetLabel={t('legendAll')}
                    categoryLabel={t('categoryTime')}
                    formatValue={(value) => `${formatCompactNumber(value)}/s`}
                  />
                )}
              </section>

              {job.error_message ? (
                <section className='flex flex-col gap-1.5'>
                  <h3 className='text-sm font-medium'>{t('error')}</h3>
                  <pre className='max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-destructive/30 bg-destructive/5 p-3 font-mono text-[11px] leading-relaxed text-muted-foreground'>
                    {job.error_message}
                  </pre>
                </section>
              ) : null}

              <div className='flex flex-wrap gap-2'>
                <Button asChild size='sm'>
                  <Link href={jobWorkbenchHref(job)}>{t('openDetail')}</Link>
                </Button>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => onOpenChange(false)}
                >
                  {t('close')}
                </Button>
              </div>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
