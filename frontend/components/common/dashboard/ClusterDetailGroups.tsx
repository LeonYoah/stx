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

import {useMemo, useState, type CSSProperties} from 'react';
import {useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {MetricCard} from '@/components/arc/metric-card/metric-card';
import {DonutChart} from '@/components/arc/donut-chart/donut-chart';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {BarChart} from '@/components/charts/bar-chart';
import {Sparkline} from '@/components/charts/sparkline';
import {Progress} from '@/components/ui/progress';
import {
  latestValue,
  toLineChartData,
} from '@/components/common/sync/job-detail/metrics-trend-data';
import {
  colorForLabels,
  formatBytes,
  formatCompactNumber,
  NEUTRAL_LINE,
} from './dashboard-format';
import {
  HomeService,
  type HomeClusterDetail,
  type HomeDetailPanel,
} from '@/lib/services/dashboard';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {DashboardPanel} from './DashboardPanel';
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

/** 虚线展示的维度：已提交堆 / 峰值线程是参考量不是流量线。
 * Dims drawn dashed: committed heap and peak threads are bounds, not flow lines. */
const DASHED_DIMS: Record<string, string[]> = {
  heap: ['committed'],
  threads: ['peak'],
};

/** 阈值参考线（虚线常量）：GC 时间占比 10%。/ Dashed threshold constants: GC pause share 10%. */
const REF_LINES: Record<string, number> = {
  gc_pause: 10,
};

/** 全零序列判定：8 种 Hazelcast 执行器大多恒 0，整条都是 0 的不画。
 * All-zero detection: most of the 8 Hazelcast executor types sit at zero — series that
 * never leave zero are not drawn. */
const isAllZero = (points: [number, number][]) =>
  points.every((point) => Math.abs(point[1]) < 1e-9);

/** fd 使用率分档色：≥80% warning。/ fd usage tier: ≥80% warns. */
const fdTone = (percent: number) =>
  percent >= 80 ? 'bg-[var(--warning)]' : undefined;

/**
 * 集群页签尾部的「节点指标细节」区：按组懒加载，只请求当前分组。
 * 一个指标一种图：趋势用折线（维度语义色，同维度跨节点同色、节点间可分辨）；
 * 恒零类「有没有」指标（死锁 / 线程池拒绝）换成指标卡；分区分布用 Donut；
 * 本地持有 / 备份条目用横向分组柱；fd 用量用 Progress。
 * The "node metric details" section at the bottom of the cluster tab, lazily loading one
 * group at a time. One chart shape per metric: trends stay lines (semantic dimension
 * colors — one dim keeps one color across nodes while staying distinguishable);
 * usually-zero "is it happening" metrics (deadlocks, pool rejections) become metric
 * cards; partition distribution goes to a donut; owned/backup entries to a grouped
 * horizontal bar; fd usage to progress bars.
 */
export function ClusterDetailGroups({
  query,
  nodeColors,
}: {
  query: DashboardQuery;
  nodeColors: Map<string, string>;
}) {
  const t = useTranslations('dashboard.home.clusterTab.detail');
  const [group, setGroup] = useState<DetailGroup>('jvm');

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

  const panelsByKey = useMemo(() => {
    const map = new Map<string, HomeDetailPanel>();
    for (const item of data?.panels ?? []) {
      map.set(item.key, item);
    }
    return map;
  }, [data]);

  const charts = useMemo(() => {
    const map = new Map<string, ReturnType<typeof toLineChartData>>();
    for (const item of data?.panels ?? []) {
      const series =
        // 全 0 的 Hazelcast 队列类型不画；类加载只画已加载类数（速率是同一条线的斜率）。
        // All-zero Hazelcast queue types are skipped; class loading draws the loaded
        // count only — the rate is the same line's slope.
        item.key === 'hz_queue'
          ? item.series.filter((entry) => !isAllZero(entry.points))
          : item.key === 'classes'
            ? item.series.filter((entry) => entry.label === 'loaded')
            : item.series;
      // 同维度标签只有一类（members / total 等）时按实例着色；多维度面板按
      // 维度语义色（同一维度跨节点同色）。/ Panels whose series all share one label
      // (members / total / …) color by instance; multi-dim panels take semantic
      // colors so one dimension keeps one color across nodes.
      const singleLabel = new Set(series.map((entry) => entry.label)).size <= 1;
      const dimColors = colorForLabels(series.map((entry) => entry.label));
      const instColors = colorForLabels(
        series.map((entry) => entry.instance),
      );
      const dashed = new Set(DASHED_DIMS[item.key] ?? []);
      const named = series.map((entry) => ({
        key: `${entry.instance} · ${entry.label}`,
        name: `${entry.instance} · ${entry.label}`,
        points: entry.points,
        color: singleLabel
          ? (nodeColors.get(entry.instance) ?? instColors.get(entry.instance))
          : dimColors.get(entry.label),
        dashed: dashed.has(entry.label) || undefined,
      }));
      const chart = toLineChartData(named, formatLabel, formatAxis);
      const refValue = item.reference ?? REF_LINES[item.key];
      const refLabel =
        item.reference !== undefined
          ? t('referenceNodes')
          : `${refValue}${item.unit === 'percent' ? '%' : ''}`;
      if (refValue !== undefined && chart.data.length > 0) {
        chart.series.push({
          key: '__reference',
          label: refLabel,
          dashed: true,
          color: 'var(--warning)',
        });
        chart.data = chart.data.map((datum) => ({
          ...datum,
          values: {...datum.values, __reference: refValue},
        }));
      }
      map.set(item.key, chart);
    }
    return map;
  }, [data, nodeColors, t]);

  const panelTitle = (item: HomeDetailPanel) =>
    t.has(`panels.${item.key}`) ? t(`panels.${item.key}`) : item.key;

  // 状态持有 / 备份条目合并为一张横向分组柱：每行一个 store（多节点时带节点前缀），
  // 持有 / 备份两根柱一眼看出备份缺失。/ Local owned + backup entries merge into one
  // grouped horizontal bar: one row per store (node-prefixed when several nodes), the
  // owned/backup pair makes missing backups obvious.
  const storeBars = useMemo(() => {
    const owned = panelsByKey.get('store_owned')?.series ?? [];
    const backup = panelsByKey.get('store_backup')?.series ?? [];
    const singleNode = new Set(owned.map((entry) => entry.instance)).size <= 1;
    const categories: string[] = [];
    const ownedValues: number[] = [];
    const backupValues: number[] = [];
    const backupMap = new Map(
      backup.map((entry) => [`${entry.instance}|${entry.label}`, entry]),
    );
    for (const entry of owned) {
      categories.push(
        singleNode ? entry.label : `${entry.instance} · ${entry.label}`,
      );
      ownedValues.push(latestValue(entry.points));
      backupValues.push(
        latestValue(
          backupMap.get(`${entry.instance}|${entry.label}`)?.points ?? [],
        ),
      );
    }
    return {categories, ownedValues, backupValues};
  }, [panelsByKey]);

  // 死锁线程 / 线程池拒绝：正常恒 0，折线是平的，换指标卡看「有没有」。
  // Deadlocks / pool rejections sit at zero — flat lines say nothing, metric cards
  // answer "is it happening".
  const metricCards = useMemo(() => {
    const build = (key: string, preferred: string[]) => {
      const item = panelsByKey.get(key);
      if (!item) {
        return [] as {instance: string; value: number; points: [number, number][]}[];
      }
      const byInstance = new Map<string, {value: number; points: [number, number][]}>();
      for (const entry of item.series) {
        if (!preferred.includes(entry.label)) {
          continue;
        }
        const slot = byInstance.get(entry.instance) ?? {value: 0, points: []};
        const value = latestValue(entry.points);
        if (value > slot.value || slot.points.length === 0) {
          slot.value = value;
          slot.points = entry.points;
        }
        byInstance.set(entry.instance, slot);
      }
      return Array.from(byInstance, ([instance, slot]) => ({
        instance,
        value: slot.value,
        points: slot.points,
      })).sort((a, b) => a.instance.localeCompare(b.instance));
    };
    return {
      deadlocked: build('deadlocked', ['deadlocked', 'monitor']),
      reject: build('reject', ['reject']),
    };
  }, [panelsByKey]);

  // Hazelcast 分区：各节点活跃分区数 → Donut 分布，看是否均衡。
  // Hazelcast partitions: active count per node → donut, showing whether the spread is even.
  const partitionDonut = useMemo(() => {
    const item = panelsByKey.get('partitions');
    if (!item) {
      return [];
    }
    const byInstance = new Map<string, number>();
    for (const entry of item.series) {
      if (entry.label === 'total') {
        continue;
      }
      byInstance.set(
        entry.instance,
        (byInstance.get(entry.instance) ?? 0) + latestValue(entry.points),
      );
    }
    const colors = colorForLabels(Array.from(byInstance.keys()));
    return Array.from(byInstance, ([instance, value]) => ({
      key: instance,
      label: instance,
      value,
      color: colors.get(instance),
    })).sort((a, b) => a.label.localeCompare(b.label));
  }, [panelsByKey]);

  // fd：每个节点一条 Progress（已打开 / 上限）。/ fds: one progress bar per node
  // (open / max).
  const fdRows = useMemo(() => {
    const item = panelsByKey.get('fds');
    if (!item) {
      return [];
    }
    const open = new Map<string, number>();
    const max = new Map<string, number>();
    for (const entry of item.series) {
      if (entry.label === 'open') {
        open.set(entry.instance, latestValue(entry.points));
      } else if (entry.label === 'max') {
        max.set(entry.instance, latestValue(entry.points));
      }
    }
    return Array.from(open, ([instance, openFd]) => {
      const maxFd = max.get(instance) ?? 0;
      return {
        instance,
        open: openFd,
        max: maxFd,
        percent: maxFd > 0 ? (openFd / maxFd) * 100 : 0,
      };
    }).sort((a, b) => b.percent - a.percent);
  }, [panelsByKey]);

  // 渲染顺序跟面板定义走；合并面板在首个成员的槽位出：
  // store_backup / class_load_rate 不再单独渲染。
  // Render order follows the panel list; merged panels take their first member's slot:
  // store_backup / class_load_rate no longer render on their own.
  const hasOwned = panelsByKey.has('store_owned');
  const hasClasses = panelsByKey.has('classes');
  const cellOrder = (data?.panels ?? [])
    .filter(
      // 合并面板只在伙伴存在时让位：store_backup 有 owned 时并入柱图槽，
      // class_load_rate 有 classes 时并入折线槽；伙伴缺失时各自照常渲染。
      // Merged panels yield only when their partner exists: store_backup folds into
      // the bar slot when owned is there, class_load_rate into the classes line —
      // without a partner each renders on its own.
      (item) =>
        !(item.key === 'store_backup' && hasOwned) &&
        !(item.key === 'class_load_rate' && hasClasses),
    )
    .map((item) => item.key);

  const cellShell = (key: string, title: string, unit: string, wide = false, testKey?: string) => ({
    key,
    className: `rounded-lg border border-border/50 p-3${wide ? ' md:col-span-2' : ''}`,
    'data-testid': `detail-panel-${testKey ?? key}`,
    title,
    unit,
  });

  return (
    <DashboardPanel
      title={t('title')}
      actions={
        <>
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
            <span className='ml-2 text-xs text-muted-foreground'>
              {t('refreshing')}
            </span>
          ) : null}
        </>
      }
      testId='home-cluster-detail'
    >

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
          {cellOrder.map((key) => {
            const item = panelsByKey.get(key);
            if (!item) {
              return null;
            }
            const shell = cellShell(key, panelTitle(item), item.unit);

            if (key === 'deadlocked' || key === 'reject') {
              const cards =
                key === 'deadlocked' ? metricCards.deadlocked : metricCards.reject;
              // 恒零「有没有」指标：每节点一张指标卡（>0 标红），拒绝带迷你趋势。
              // Usually-zero "is it happening" metrics: one card per node (red when
              // >0); rejections carry a mini trend.
              return (
                <div {...shell} key={key}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{shell.title}</h4>
                    <span className='text-[11px] text-muted-foreground'>
                      {t.has(`units.${item.unit}`) ? t(`units.${item.unit}`) : ''}
                    </span>
                  </div>
                  {cards.length === 0 ? (
                    <p className='py-6 text-center text-xs text-muted-foreground'>
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <div className='flex flex-col gap-3'>
                      {cards.map((card) => (
                        <div
                          key={card.instance}
                          // 大于 0 时用 danger 色渲染数值（--foreground 局部覆盖）。
                          // Values >0 render in danger via a scoped --foreground.
                          style={
                            card.value > 0
                              ? ({'--foreground': 'var(--danger)'} as CSSProperties)
                              : undefined
                          }
                        >
                          <MetricCard
                            label={card.instance}
                            value={card.value}
                            context={shell.title}
                          />
                          {key === 'reject' ? (
                            <Sparkline
                              data={card.points.map((point) => point[1])}
                              label={`${card.instance} ${shell.title}`}
                              variant='area'
                              height={28}
                              className='mt-1 text-[var(--series-1)]'
                            />
                          ) : null}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              );
            }

            if (key === 'partitions') {
              // 分区分布：各节点占比 Donut。/ Partition spread: per-node share donut.
              return (
                <div {...shell} key={key}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{shell.title}</h4>
                  </div>
                  <DonutChart
                    data={partitionDonut}
                    label={shell.title}
                    size={170}
                    legend
                    otherLabel={t('other')}
                    emptyLabel={t('emptyPanel')}
                  />
                </div>
              );
            }

            if (key === 'store_owned') {
              // 本地持有 + 备份合并的横向分组柱。/ Owned + backup merged grouped bars.
              return (
                <div {...shell} key={key}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>
                      {t('panels.store_entries')}
                    </h4>
                  </div>
                  {storeBars.categories.length === 0 ? (
                    <p className='py-6 text-center text-xs text-muted-foreground'>
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <BarChart
                      categories={storeBars.categories}
                      series={[
                        {
                          id: 'owned',
                          name: t('legendOwned'),
                          values: storeBars.ownedValues,
                          color: 'var(--series-1)',
                        },
                        {
                          id: 'backup',
                          name: t('legendBackup'),
                          values: storeBars.backupValues,
                          color: 'var(--series-4)',
                        },
                      ]}
                      orientation='horizontal'
                      layout='grouped'
                      label={shell.title}
                      formatValue={(value) => formatCompactNumber(value)}
                    />
                  )}
                </div>
              );
            }

            if (key === 'fds') {
              // fd：每节点一条 Progress（已打开 / 上限）。/ fds: open / max per node.
              return (
                <div {...shell} key={key}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{shell.title}</h4>
                  </div>
                  {fdRows.length === 0 ? (
                    <p className='py-6 text-center text-xs text-muted-foreground'>
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <ul className='flex flex-col gap-2'>
                      {fdRows.map((row) => (
                        <li
                          key={row.instance}
                          className='grid grid-cols-[minmax(0,8rem)_1fr_auto] items-center gap-2'
                        >
                          <span
                            className='truncate text-[11px] text-muted-foreground'
                            title={row.instance}
                          >
                            {row.instance}
                          </span>
                          <Progress
                            value={row.percent}
                            className='h-1.5'
                            indicatorClassName={fdTone(row.percent)}
                            aria-label={`${row.instance} ${shell.title}`}
                          />
                          <span className='font-mono text-[11px]'>
                            {formatCompactNumber(row.open)} /{' '}
                            {formatCompactNumber(row.max)}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              );
            }

            const suffix = unitSuffix(item.unit);
            const chart = charts.get(item.key);
            return (
              <div {...shell} key={key}>
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{shell.title}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t.has(`units.${item.unit}`) ? t(`units.${item.unit}`) : ''}
                  </span>
                </div>
                <LineChart
                  label={shell.title}
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
    </DashboardPanel>
  );
}
