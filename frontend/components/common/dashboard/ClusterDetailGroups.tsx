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

import {useMemo, useState} from 'react';
import {useFormatter, useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {toLineChartData} from '@/components/common/sync/job-detail/metrics-trend-data';
import {formatBytes, formatCompactNumber} from './dashboard-format';
import {
  HomeService,
  type HomeClusterDetail,
  type HomeDetailPanel,
} from '@/lib/services/dashboard';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import type {DashboardQuery} from './use-dashboard-query';
import {usePolledResource} from './use-polled-resource';

const DETAIL_POLL_MS = 30_000;

const DETAIL_GROUPS = ['jvm', 'gc', 'sched', 'store', 'proc'] as const;
type DetailGroup = (typeof DETAIL_GROUPS)[number];

/** 提示里用完整时间，坐标轴只留 HH:mm。/ Full time in tooltips, HH:mm on axes. */
const formatLabel = (ts: number) => formatDateTimeMinuteTz(ts);
const formatAxis = (ts: number) => formatDateTimeMinuteTz(ts).slice(-5);

/** 面板单位的数值后缀。/ Value suffix per panel unit. */
const unitSuffix = (unit: string) =>
  unit === 'bytes'
    ? 'B'
    : unit === 'percent'
      ? '%'
      : unit === 'per_second'
        ? '/s'
        : unit === 'ms'
          ? 'ms'
          : '';

/**
 * 集群页签尾部的「节点指标细节」区：按组懒加载，只请求当前分组。
 * 序列着色跟节点色板一致（instance 定色），维度名进图例。
 * The "node metric details" section at the bottom of the cluster tab: lazily loads one group at a
 * time. Series colors follow the node palette (instance picks the color); the dimension name is
 * the legend label.
 */
export function ClusterDetailGroups({
  query,
  nodeColors,
}: {
  query: DashboardQuery;
  nodeColors: Map<string, string>;
}) {
  const t = useTranslations('dashboard.home.clusterTab.detail');
  const format = useFormatter();
  const [group, setGroup] = useState<DetailGroup>('jvm');
  const panel = 'rounded-xl border border-border/70 bg-card/40 p-4';

  const resource = usePolledResource<HomeClusterDetail>(
    JSON.stringify([
      'cluster-detail',
      group,
      query.clusterId ?? 'all',
      query.range,
      query.start,
      query.end,
    ]),
    () =>
      HomeService.getClustersDetail({
        cluster_id: query.clusterId,
        group,
        ...(query.start && query.end
          ? {start: query.start, end: query.end}
          : {range: query.range}),
      }),
    // 自定义时间窗固定不变，不自动刷新。/ A custom window is fixed, so it does not auto-refresh.
    query.start ? 0 : DETAIL_POLL_MS,
  );
  const data = resource.data;

  const charts = useMemo(() => {
    const map = new Map<string, ReturnType<typeof toLineChartData>>();
    for (const item of data?.panels ?? []) {
      // 序列名 = 实例 · 维度；颜色只跟实例走，同一节点在所有面板里同色。
      // Series name = instance · dimension; color follows the instance only so one node keeps one
      // color across every panel.
      const named = item.series.map((entry) => ({
        key: `${entry.instance} · ${entry.label}`,
        name: `${entry.instance} · ${entry.label}`,
        points: entry.points,
        color: nodeColors.get(entry.instance),
      }));
      const chart = toLineChartData(named, formatLabel, formatAxis);
      if (item.reference !== undefined && chart.data.length > 0) {
        chart.series.push({
          key: '__reference',
          label: t('referenceNodes'),
          dashed: true,
          color: 'var(--warning)',
        });
        chart.data = chart.data.map((datum) => ({
          ...datum,
          values: {...datum.values, __reference: item.reference},
        }));
      }
      map.set(item.key, chart);
    }
    return map;
  }, [data, nodeColors, t]);

  const panelTitle = (item: HomeDetailPanel) =>
    t.has(`panels.${item.key}`) ? t(`panels.${item.key}`) : item.key;

  return (
    <section className={panel} data-testid='home-cluster-detail'>
      <div className='mb-3 flex flex-wrap items-center gap-3'>
        <h3 className='text-sm font-medium'>{t('title')}</h3>
        <SegmentedControl
          label={t('title')}
          value={group}
          onValueChange={(value) => setGroup(value as DetailGroup)}
          options={DETAIL_GROUPS.map((key) => ({
            value: key,
            label: t(`groups.${key}`),
          }))}
        />
        {resource.refreshing ? (
          <span className='text-xs text-muted-foreground'>{t('refreshing')}</span>
        ) : null}
      </div>

      {data?.partial_errors?.length ? (
        <p className='mb-3 text-xs text-amber-600 dark:text-amber-400' role='status'>
          {t('partial', {keys: data.partial_errors.join(', ')})}
        </p>
      ) : null}

      {data && data.events.length > 0 ? (
        <ul
          className='mb-3 flex flex-wrap gap-1.5'
          data-testid='detail-events'
        >
          {data.events.map((event, index) => (
            <li
              key={`${event.ts_ms}-${index}`}
              className='rounded-md border border-border/60 bg-muted/30 px-2 py-0.5 font-mono text-[11px] text-muted-foreground'
            >
              {formatDateTimeMinuteTz(event.ts_ms)} ·{' '}
              {t.has(`events.${event.kind}`)
                ? t(`events.${event.kind}`)
                : event.kind}{' '}
              · {event.label}
            </li>
          ))}
        </ul>
      ) : null}

      {resource.loading && !data ? (
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {Array.from({length: 3}).map((_, index) => (
            <div
              key={index}
              className='h-[220px] animate-pulse rounded-xl bg-muted/40'
            />
          ))}
        </div>
      ) : data && data.panels.every((item) => item.series.length === 0) ? (
        // 整组无序列：引擎未上报这类指标（老版本 / 未开启）。/ Whole group empty: the engine does not
        // report these metrics (old version / disabled).
        <p className='py-10 text-center text-xs text-muted-foreground'>
          {t('emptyGroup')}
        </p>
      ) : (
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {(data?.panels ?? []).map((item) => {
            const suffix = unitSuffix(item.unit);
            const chart = charts.get(item.key);
            return (
              <div
                key={item.key}
                className='rounded-lg border border-border/50 p-3'
                data-testid={`detail-panel-${item.key}`}
              >
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{panelTitle(item)}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t.has(`units.${item.unit}`) ? t(`units.${item.unit}`) : ''}
                  </span>
                </div>
                <LineChart
                  label={panelTitle(item)}
                  // 字节面板单位走 formatBytes（IEC），其余让 unit 拼后缀。
                  // Byte panels carry their unit in formatBytes (IEC); others take the suffix.
                  unit={item.unit === 'bytes' ? '' : suffix}
                  data={chart?.data ?? []}
                  series={chart?.series ?? []}
                  height={170}
                  emptyLabel={t('emptyPanel')}
                  clearLabel={t('legendClear')}
                  resetLabel={t('legendAll')}
                  categoryLabel={t('time')}
                  formatValue={(value) =>
                    item.unit === 'bytes'
                      ? formatBytes(value)
                      : formatCompactNumber(value)
                  }
                />
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
