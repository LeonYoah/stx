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

import {useTranslations} from 'next-intl';
import type {HomeSummary, HomeVolume} from '@/lib/services/dashboard';
import {formatDateTz} from '@/lib/timezone';
import {
  formatCompactNumber,
  scaledCompact,
  trendChangePercent,
} from './dashboard-format';
import {KpiStrip, KpiStripSkeleton, type KpiStripItem} from './KpiStrip';

/**
 * 指标格数值缩放：value 可能任意大（一年窗口的行数可达百亿级），统一经 scaledCompact 压到
 * ≤999.9 + 量级后缀（K/M/B），避免长数字溢出相邻格；计量单位单独进 unit，小号显示。
 * KPI value scaling: values can be arbitrarily large (a year of rows reaches tens of
 * billions), so every number goes through scaledCompact to stay ≤999.9 plus a magnitude
 * suffix (K/M/B), keeping long digits from overflowing into the neighboring cell; the
 * measure unit goes to `unit` separately and renders smaller.
 */
function scaledKpi(value: number) {
  const scaled = scaledCompact(value);
  return {value: scaled.value, decimals: scaled.decimals, scale: scaled.unit};
}

/** 最早样本日的标注（走系统显示时区的 M/D 或本地化短日期）。/ Oldest-sample day marker in the display timezone. */
function volumeSinceLabel(oldestMs: number): string {
  return formatDateTz(oldestMs);
}

/**
 * 首页顶部指标栏：把汇总 / 数据量接口数据映射成 KpiStripItem[]，渲染交给 KpiStrip。
 * 涨跌只用于「越大越好」的写入速率；失败数等保持中性文字。异常染色：
 * 失败 > 0 / CDC 超阈值 / 严重告警 > 0 → danger，集群有降级 → warning。
 * The home KPI strip: maps summary / volume payloads to KpiStripItem[] and lets
 * KpiStrip render. Signed changes are used only for the write rate (higher is
 * better); failures stay neutral. Tints: failures > 0 / CDC over threshold /
 * critical alerts > 0 → danger, degraded clusters → warning.
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
    return <KpiStripSkeleton count={7} />;
  }

  const jobs = summary?.jobs;
  const clusters = summary?.clusters;
  const writeChange = jobs ? trendChangePercent(jobs.write_trend) : null;
  const cdc = jobs?.max_cdc_delay_seconds;
  const cdcOver =
    cdc !== undefined && jobs !== null && jobs !== undefined
      ? cdc >= jobs.cdc_threshold_seconds
      : false;
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
  const degraded = (clusters?.degraded ?? 0) + (clusters?.unhealthy ?? 0);
  const items: KpiStripItem[] = [
    {
      key: 'running',
      href: buildHref({tab: 'list', status: 'running'}),
      label: t('kpi.running'),
      ...scaledKpi(jobs?.running ?? 0),
      context: t('kpi.runningContext', {
        streaming: jobs?.running_streaming ?? 0,
        batch: jobs?.running_batch ?? 0,
      }),
    },
    {
      key: 'write',
      href: buildHref({tab: 'list', status: 'running', sort: 'rate'}),
      label: t('kpi.writeRate'),
      ...scaledKpi(Math.round(jobs?.write_qps ?? 0)),
      unit: t('unit.rowsPerSec').trim(),
      context:
        writeChange === null
          ? t('kpi.writeContextNow')
          : t('kpi.writeContextTrend'),
      change:
        writeChange === null
          ? undefined
          : {percent: writeChange, good: writeChange >= 0},
    },
    {
      key: 'cdc',
      href: buildHref({tab: 'list', status: 'running', sort: 'cdc_delay'}),
      label: t('kpi.cdc'),
      ...scaledKpi(Math.round(cdc ?? 0)),
      unit: t('unit.seconds').trim(),
      context: cdcOver
        ? t('kpi.cdcOverContext', {
            name: jobs?.max_cdc_job_name || t('kpi.cdcNone'),
          })
        : jobs?.max_cdc_job_name || t('kpi.cdcNone'),
      tone: cdcOver ? 'danger' : undefined,
    },
    {
      key: 'failed',
      href: buildHref({tab: 'list', status: 'failed'}),
      label: t('kpi.failed'),
      ...scaledKpi(jobs?.failed_24h ?? 0),
      context: t('kpi.failedContextWithPrev', {
        count: jobs?.failed_prev_24h ?? 0,
      }),
      tone: (jobs?.failed_24h ?? 0) > 0 ? 'danger' : undefined,
    },
    {
      key: 'volume',
      href: buildHref({tab: 'jobs'}),
      label: t('kpi.volume'),
      ...scaledKpi(Math.round(volumeTotals?.window.write_rows ?? 0)),
      unit: t('unit.rows').trim(),
      context: volumeContext,
    },
    {
      key: 'clusters',
      href: '/clusters',
      label: t('kpi.clusters'),
      value: clusters?.healthy ?? 0,
      unit: `/ ${clusters?.total ?? 0}`,
      context:
        degraded > 0
          ? t('kpi.clustersDegradedContext', {
              count: degraded,
              online: clusters?.nodes_online ?? 0,
              total: clusters?.nodes_total ?? 0,
            })
          : t('kpi.clustersContext', {
              online: clusters?.nodes_online ?? 0,
              total: clusters?.nodes_total ?? 0,
            }),
      tone: degraded > 0 ? 'warning' : undefined,
    },
    {
      key: 'alerts',
      href: '/monitoring',
      label: t('kpi.alerts'),
      ...scaledKpi(summary?.alerts.active ?? 0),
      context: t('kpi.alertsContext', {count: summary?.alerts.critical ?? 0}),
      tone: (summary?.alerts.critical ?? 0) > 0 ? 'danger' : undefined,
    },
  ];

  return <KpiStrip items={items} label={t('kpi.label')} />;
}
