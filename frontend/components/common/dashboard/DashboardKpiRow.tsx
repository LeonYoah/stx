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

import Link from 'next/link';
import {useTranslations} from 'next-intl';
import {MetricCard} from '@/components/arc/metric-card/metric-card';
import {Skeleton} from '@/components/ui/skeleton';
import type {HomeSummary, HomeVolume} from '@/lib/services/dashboard';
import {formatDateTz} from '@/lib/timezone';
import {formatCompactNumber, trendChangePercent} from './dashboard-format';

interface KpiItem {
  key: string;
  href: string;
  label: string;
  value: number;
  suffix?: string;
  context: string;
  change?: string;
}

/** 最早样本日的标注（走系统显示时区的 M/D 或本地化短日期）。/ Oldest-sample day marker in the display timezone. */
function volumeSinceLabel(oldestMs: number): string {
  return formatDateTz(oldestMs);
}

/**
 * 首页顶部 7 张指标卡（Arc MetricCard），整卡可点击跳到对应页签或告警中心；数据量卡展示当前时间窗写入行数。
 * 涨跌标签只用于「越大越好」的写入速率；失败数用中性文字，避免 Arc 把「+1」染成绿色。
 * The seven home KPI cards (Arc MetricCard); each card links to its tab or the alert center, and the volume card shows
 * rows written in the current window. Signed change chips are used only for the write rate (higher is better);
 * failures use neutral text so Arc never paints "+1" green.
 */
export function DashboardKpiRow({
  summary,
  volume,
  loading,
  buildHref,
}: {
  summary: HomeSummary | null;
  volume?: HomeVolume | null;
  loading: boolean;
  buildHref: (patch: Record<string, string>) => string;
}) {
  const t = useTranslations('dashboard.home');

  if (loading && !summary) {
    return (
      <div
        className='grid grid-cols-1 gap-3 min-[480px]:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'
        data-testid='home-kpi-loading'
      >
        {Array.from({length: 7}).map((_, index) => (
          <Skeleton key={index} className='h-[158px] rounded-xl' />
        ))}
      </div>
    );
  }

  const jobs = summary?.jobs;
  const clusters = summary?.clusters;
  const writeChange = jobs ? trendChangePercent(jobs.write_trend) : null;
  const cdc = jobs?.max_cdc_delay_seconds;
  // 本月起点早于最早样本时，月累计实际从最早样本算起，文案要注明起始日。
  // When the month start predates the oldest sample, the month total really starts at that
  // sample, so the caption names the effective start date.
  const volumeTotals = volume?.totals ?? null;
  const monthClipped =
    volumeTotals !== null &&
    (volume?.oldest_sample_ms ?? 0) > volumeTotals.month_start_ms;
  const volumeContext = volumeTotals
    ? monthClipped
      ? t('kpi.volumeContextSince', {
          today: formatCompactNumber(volumeTotals.today.write_rows),
          date: volumeSinceLabel(volume!.oldest_sample_ms),
          month: formatCompactNumber(volumeTotals.month.write_rows),
        })
      : t('kpi.volumeContext', {
          today: formatCompactNumber(volumeTotals.today.write_rows),
          month: formatCompactNumber(volumeTotals.month.write_rows),
        })
    : t('kpi.volumeEmpty');
  const items: KpiItem[] = [
    {
      key: 'running',
      href: buildHref({tab: 'list', status: 'running'}),
      label: t('kpi.running'),
      value: jobs?.running ?? 0,
      context: t('kpi.runningContext', {
        streaming: jobs?.running_streaming ?? 0,
        batch: jobs?.running_batch ?? 0,
      }),
    },
    {
      key: 'write',
      href: buildHref({tab: 'list', status: 'running', sort: 'rate'}),
      label: t('kpi.writeRate'),
      value: Math.round(jobs?.write_qps ?? 0),
      suffix: t('unit.rowsPerSec'),
      context:
        writeChange === null
          ? t('kpi.writeContextNow')
          : t('kpi.writeContextTrend'),
      change:
        writeChange === null
          ? undefined
          : `${writeChange >= 0 ? '+' : '−'}${Math.abs(writeChange)}%`,
    },
    {
      key: 'cdc',
      href: buildHref({tab: 'list', status: 'running', sort: 'cdc_delay'}),
      label: t('kpi.cdc'),
      value: Math.round(cdc ?? 0),
      suffix: t('unit.seconds'),
      context: jobs?.max_cdc_job_name || t('kpi.cdcNone'),
      change:
        cdc !== undefined && jobs && cdc >= jobs.cdc_threshold_seconds
          ? t('kpi.cdcOver')
          : undefined,
    },
    {
      key: 'failed',
      href: buildHref({tab: 'list', status: 'failed'}),
      label: t('kpi.failed'),
      value: jobs?.failed_24h ?? 0,
      context: t('kpi.failedContext'),
      change: t('kpi.failedYesterday', {count: jobs?.failed_prev_24h ?? 0}),
    },
    {
      key: 'volume',
      href: buildHref({tab: 'jobs'}),
      label: t('kpi.volume'),
      value: Math.round(volumeTotals?.window.write_rows ?? 0),
      suffix: t('unit.rows'),
      context: volumeContext,
    },
    {
      key: 'clusters',
      href: '/clusters',
      label: t('kpi.clusters'),
      value: clusters?.healthy ?? 0,
      suffix: `/${clusters?.total ?? 0}`,
      context: t('kpi.clustersContext', {
        online: clusters?.nodes_online ?? 0,
        total: clusters?.nodes_total ?? 0,
      }),
      change:
        clusters && clusters.degraded + clusters.unhealthy > 0
          ? t('kpi.clustersDegraded', {
              count: clusters.degraded + clusters.unhealthy,
            })
          : undefined,
    },
    {
      key: 'alerts',
      href: '/monitoring',
      label: t('kpi.alerts'),
      value: summary?.alerts.active ?? 0,
      context: t('kpi.alertsContext', {count: summary?.alerts.critical ?? 0}),
    },
  ];

  return (
    <div
      className='grid grid-cols-1 gap-3 min-[480px]:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7'
      data-testid='home-kpi-row'
    >
      {items.map((item) => (
        <Link
          key={item.key}
          href={item.href}
          className='block h-full min-w-0 rounded-xl transition-transform active:scale-[0.99] [&>article]:h-full'
          data-testid={`home-kpi-${item.key}`}
        >
          <MetricCard
            label={item.label}
            value={item.value}
            suffix={item.suffix}
            context={item.context}
            change={item.change}
          />
        </Link>
      ))}
    </div>
  );
}
