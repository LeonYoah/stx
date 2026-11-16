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
import type {HomeSummary} from '@/lib/services/dashboard';
import {trendChangePercent} from './dashboard-format';

interface KpiItem {
  key: string;
  href: string;
  label: string;
  value: number;
  suffix?: string;
  context: string;
  change?: string;
}

/**
 * 首页顶部 6 张指标卡（Arc MetricCard），整卡可点击跳到对应页签或告警中心。
 * 涨跌标签只用于「越大越好」的写入速率；失败数用中性文字，避免 Arc 把「+1」染成绿色。
 * The six home KPI cards (Arc MetricCard); each card links to its tab or the alert center. Signed change chips are used
 * only for the write rate (higher is better); failures use neutral text so Arc never paints "+1" green.
 */
export function DashboardKpiRow({
  summary,
  loading,
  buildHref,
}: {
  summary: HomeSummary | null;
  loading: boolean;
  buildHref: (patch: Record<string, string>) => string;
}) {
  const t = useTranslations('dashboard.home');

  if (loading && !summary) {
    return (
      <div
        className='grid grid-cols-1 gap-3 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6'
        data-testid='home-kpi-loading'
      >
        {Array.from({length: 6}).map((_, index) => (
          <Skeleton key={index} className='h-[158px] rounded-xl' />
        ))}
      </div>
    );
  }

  const jobs = summary?.jobs;
  const clusters = summary?.clusters;
  const writeChange = jobs ? trendChangePercent(jobs.write_trend) : null;
  const cdc = jobs?.max_cdc_delay_seconds;
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
      className='grid grid-cols-1 gap-3 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6'
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
