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
import {useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {DonutChart} from '@/components/arc/donut-chart/donut-chart';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Select} from '@/components/arc/select/select';
import {BarChart} from '@/components/charts/bar-chart';
import {
  StatusBar,
  StatusBarPlot,
  StatusBarTooltip,
} from '@/components/charts/status-bar';
import {SoftChip} from '@/components/common/layout';
import {Progress} from '@/components/ui/progress';
import {cn} from '@/lib/utils';
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

/** 节点选择器「全部节点」标识。/ Sentinel value for showing all node instances together. */
const ALL_INSTANCES = '__all__';

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
 * 恒零信号序列降采样为 ≤maxBuckets 桶：桶内取峰值（尖刺不能被平均抹掉）；
 * StatusBar 对重复 datum id 直接抛错，id 携带桶序号保证唯一。
 * Downsample a usually-zero signal series into ≤maxBuckets buckets pooled by
 * peak (a spike must never average away); StatusBar throws on duplicate datum
 * ids, so ids carry the bucket index to stay unique.
 */
export function bucketSignalPoints(
  points: [number, number][],
  maxBuckets = 24,
): {id: string; startTs: number; endTs: number; peak: number}[] {
  if (points.length === 0) {
    return [];
  }
  if (points.length <= maxBuckets) {
    return points.map(([ts, value], index) => ({
      id: `b-${index}-${ts}`,
      startTs: ts,
      endTs: ts,
      peak: value,
    }));
  }
  const buckets: {id: string; startTs: number; endTs: number; peak: number}[] =
    [];
  for (let index = 0; index < maxBuckets; index += 1) {
    const start = Math.floor((index * points.length) / maxBuckets);
    const end = Math.floor(((index + 1) * points.length) / maxBuckets);
    const slice = points.slice(start, end);
    const startTs = slice[0]?.[0];
    const endTs = slice[slice.length - 1]?.[0];
    if (startTs === undefined || endTs === undefined) {
      continue;
    }
    buckets.push({
      id: `b-${index}-${startTs}`,
      startTs,
      endTs,
      peak: slice.reduce((max, point) => Math.max(max, point[1]), 0),
    });
  }
  return buckets;
}

/**
 * 集群页签尾部的「节点指标细节」区：按组懒加载，只请求当前分组。
 * 一个指标一种图：趋势用折线（维度语义色，同维度跨节点同色、节点间可分辨）；
 * 恒零类「有没有」指标（死锁 / 线程池拒绝）换成单行信号条；分区分布用 Donut；
 * 本地持有 / 备份条目用横向分组柱；fd 用量用 Progress。
 * The "node metric details" section at the bottom of the cluster tab, lazily loading one
 * group at a time. One chart shape per metric: trends stay lines (semantic dimension
 * colors — one dim keeps one color across nodes while staying distinguishable);
 * usually-zero "is it happening" metrics (deadlocks, pool rejections) become flat
 * signal rows; partition distribution goes to a donut; owned/backup entries to a
 * grouped horizontal bar; fd usage to progress bars.
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
  const [selectedInstance, setSelectedInstance] = useState<string>('');

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

  // 提取当前分组全部节点实例并排序，多节点默认聚焦首节点防止图例笛卡尔积爆炸。
  // Collect sorted instances across panels; multi-node defaults to the first node to prevent legend explosion.
  const instances = useMemo(() => {
    const set = new Set<string>();
    for (const panel of data?.panels ?? []) {
      // members 序列的 instance 是集群名而非节点端点，不能混进节点选择器。
      // members carries cluster names as instance, not node endpoints — keep it
      // out of the node selector.
      if (panel.key === 'members') {
        continue;
      }
      for (const entry of panel.series) {
        if (entry.instance) {
          set.add(entry.instance);
        }
      }
    }
    return Array.from(set).sort((a, b) => a.localeCompare(b));
  }, [data]);

  const effectiveInstance =
    instances.length <= 1
      ? (instances[0] ?? ALL_INSTANCES)
      : selectedInstance === ALL_INSTANCES
        ? ALL_INSTANCES
        : selectedInstance && instances.includes(selectedInstance)
          ? selectedInstance
          : instances[0];

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
      const baseSeries =
        // 全 0 的 Hazelcast 队列类型不画；类加载只画已加载类数（速率是同一条线的斜率）。
        // All-zero Hazelcast queue types are skipped; class loading draws the loaded
        // count only — the rate is the same line's slope.
        item.key === 'hz_queue'
          ? item.series.filter((entry) => !isAllZero(entry.points))
          : item.key === 'classes'
            ? item.series.filter((entry) => entry.label === 'loaded')
            : item.series;
      // 聚焦单节点时仅保留所选实例的序列；选"全部节点"时全量对比。
      // members 的 instance 是集群名，按节点过滤会把它筛空，跳过过滤。
      // Filter to the focused instance when a single node is chosen; keep all when
      // comparing all nodes. members carries cluster names as instance — node
      // filtering would empty it, so it skips the filter.
      const series =
        effectiveInstance !== ALL_INSTANCES && item.key !== 'members'
          ? baseSeries.filter((entry) => entry.instance === effectiveInstance)
          : baseSeries;
      // 同维度标签只有一类（members / total 等）时按实例着色；多维度面板按
      // 维度语义色（同一维度跨节点同色）。/ Panels whose series all share one label
      // (members / total / …) color by instance; multi-dim panels take semantic
      // colors so one dimension keeps one color across nodes.
      const singleLabel = new Set(series.map((entry) => entry.label)).size <= 1;
      const singleInstanceView =
        new Set(series.map((entry) => entry.instance)).size <= 1;
      const dimColors = colorForLabels(series.map((entry) => entry.label));
      const instColors = colorForLabels(
        series.map((entry) => entry.instance),
      );
      const dashed = new Set(DASHED_DIMS[item.key] ?? []);
      const named = series.map((entry) => {
        // 单节点视角下多维面板去掉冗余 IP:port 前缀；跨节点对比时保留节点前缀以便分辨。
        // Strip redundant IP:port prefix in single-node multi-dim view; keep it when comparing nodes.
        const seriesName =
          singleInstanceView && !singleLabel
            ? entry.label
            : `${entry.instance} · ${entry.label}`;
        return {
          key: seriesName,
          name: seriesName,
          points: entry.points,
          color: singleLabel
            ? (nodeColors.get(entry.instance) ?? instColors.get(entry.instance))
            : dimColors.get(entry.label),
          dashed: dashed.has(entry.label) || undefined,
        };
      });
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
  }, [data, effectiveInstance, nodeColors, t]);

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

  // 死锁线程 / 线程池拒绝：正常恒 0，折线是平的，换单行信号条看「有没有」。
  // Deadlocks / pool rejections sit at zero — flat lines say nothing, flat signal
  // rows answer "is it happening".
  const metricRows = useMemo(() => {
    const build = (key: string, preferred: string[]) => {
      const item = panelsByKey.get(key);
      if (!item) {
        return [] as {
          instance: string;
          value: number;
          peak: number;
          points: [number, number][];
        }[];
      }
      const byInstance = new Map<
        string,
        {value: number; peak: number; points: [number, number][]}
      >();
      for (const entry of item.series) {
        if (!preferred.includes(entry.label)) {
          continue;
        }
        const value = latestValue(entry.points);
        const peak = entry.points.reduce(
          (max, point) => Math.max(max, point[1]),
          0,
        );
        const slot = byInstance.get(entry.instance);
        // 同节点多条候选维度（deadlocked / monitor）取窗口峰值最高的一条；峰值
        // 并列比最新值，再并列比有点者。/ With several candidate dims on one node
        // keep the series with the highest window peak; ties compare latest
        // value, then non-empty points.
        if (
          !slot ||
          peak > slot.peak ||
          (peak === slot.peak &&
            (value > slot.value || slot.points.length === 0))
        ) {
          byInstance.set(entry.instance, {value, peak, points: entry.points});
        }
      }
      return Array.from(byInstance, ([instance, slot]) => ({
        instance,
        value: slot.value,
        peak: slot.peak,
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

  // 避免把 key 放进展开对象：React 19 会对 `{...props}` 中包含 key 报错。
  // Keep `key` out of spread props: React 19 warns whenever a spread object carries `key`.
  const cellProps = (key: string, wide = false, testKey?: string) => ({
    className: `rounded-[var(--radius-panel)] border border-border/50 p-3${wide ? ' md:col-span-2' : ''}`,
    'data-testid': `detail-panel-${testKey ?? key}`,
  });

  return (
    <DashboardPanel
      title={t('title')}
      actions={
        // 动态兄弟元素拼进 children 数组时必须显式给 key，否则 React 19 报
        // 「unique key」告警（数组里的元素不走静态子级校验）。
        // Dynamic sibling expressions land in a children array and must carry
        // explicit keys — React 19's "unique key" warning otherwise fires
        // (array children skip static-children validation).
        <div className='flex flex-wrap items-center gap-2'>
          {instances.length >= 2 ? (
            <Select
              key='node-select'
              aria-label={t('nodeFilter')}
              data-testid='detail-node-select'
              className='w-[196px]'
              value={effectiveInstance}
              onValueChange={setSelectedInstance}
              options={[
                ...instances.map((inst) => ({value: inst, label: inst})),
                {
                  value: ALL_INSTANCES,
                  label: t('allNodes', {count: instances.length}),
                },
              ]}
            />
          ) : null}
          <SegmentedControl
            key='groups'
            label={t('title')}
            value={group}
            onValueChange={(value) => setGroup(value as DetailGroup)}
            options={DETAIL_GROUPS.map((key) => ({
              value: key,
              label: t(`groups.${key}`),
            }))}
          />
          {resource.refreshing ? (
            <span
              key='refreshing'
              className='ml-1 text-xs text-muted-foreground'
            >
              {t('refreshing')}
            </span>
          ) : null}
        </div>
      }
      testId='home-cluster-detail'
    >

      {data?.partial_errors?.length ? (
        <p
          key='partial-errors'
          className='mb-3 text-xs text-amber-600 dark:text-amber-400'
          role='status'
        >
          {t('partial', {keys: data.partial_errors.join(', ')})}
        </p>
      ) : null}

      {data && data.events.length > 0 ? (
        <ul
          key='detail-events'
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
        <div
          key='cells-loading'
          className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
        >
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
        <p
          key='cells-empty'
          className='py-10 text-center text-xs text-muted-foreground'
        >
          {t('emptyGroup')}
        </p>
      ) : (
        <div
          key='cells-grid'
          className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'
        >
          {cellOrder.map((key) => {
            const item = panelsByKey.get(key);
            if (!item) {
              return null;
            }
            const title = panelTitle(item);
            const props = cellProps(key);

            if (key === 'deadlocked' || key === 'reject') {
              const rows =
                key === 'deadlocked' ? metricRows.deadlocked : metricRows.reject;
              const suffix = unitSuffix(item.unit);
              // 恒零「有没有」指标：每节点一行（最新值 + 状态徽标 + 峰值信号条），
              // 单层排布不嵌卡片。/ Usually-zero "is it happening" metrics: one flat
              // row per node (latest value + status chip + peak signal bar), single
              // layer with no nested card.
              return (
                <div key={key} {...props}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{title}</h4>
                    <span className='text-[11px] text-muted-foreground'>
                      {t.has(`units.${item.unit}`) ? t(`units.${item.unit}`) : ''}
                    </span>
                  </div>
                  {rows.length === 0 ? (
                    <p
                      key='body'
                      className='py-6 text-center text-xs text-muted-foreground'
                    >
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <ul
                      key='body'
                      className='divide-y-[0.5px] divide-border/70'
                    >
                      {rows.map((row) => {
                        const buckets = bucketSignalPoints(row.points);
                        const hasAnomaly = row.value > 0 || row.peak > 0;
                        return (
                          <li
                            key={row.instance}
                            className='py-2 first:pt-0.5 last:pb-0'
                          >
                            <div className='flex items-center justify-between gap-2'>
                              <span
                                className='min-w-0 truncate font-mono text-xs text-muted-foreground'
                                title={row.instance}
                              >
                                {row.instance}
                              </span>
                              <span className='flex shrink-0 items-center gap-2'>
                                {row.peak > 0 && row.value === 0 ? (
                                  <span className='text-[11px] tabular-nums text-[var(--danger)]'>
                                    {t('signalPeak', {
                                      value: formatCompactNumber(row.peak),
                                    })}
                                  </span>
                                ) : null}
                                <span
                                  className={cn(
                                    'font-mono text-sm font-semibold tabular-nums',
                                    row.value > 0
                                      ? 'text-[var(--danger)]'
                                      : 'text-foreground',
                                  )}
                                >
                                  {formatCompactNumber(row.value)}
                                </span>
                                <SoftChip
                                  tone={hasAnomaly ? 'rose' : 'emerald'}
                                  className='px-1.5 py-0 text-[10px] font-medium'
                                >
                                  {hasAnomaly
                                    ? t('signalAlert')
                                    : t('signalClear')}
                                </SoftChip>
                              </span>
                            </div>
                            {buckets.length > 0 ? (
                              <StatusBar
                                className='mt-1 space-y-0'
                                label={`${row.instance} ${title}`}
                                statuses={[
                                  {
                                    id: 'ok',
                                    label: t('signalClear'),
                                    color:
                                      'color-mix(in oklab, var(--success) 42%, transparent)',
                                  },
                                  {
                                    id: 'alert',
                                    label: t('signalAlert'),
                                    color: 'var(--danger)',
                                  },
                                ]}
                                data={buckets.map((bucket) => ({
                                  id: bucket.id,
                                  label:
                                    bucket.startTs === bucket.endTs
                                      ? formatDateTimeMinuteTz(bucket.startTs)
                                      : `${formatDateTimeMinuteTz(bucket.startTs)}–${formatDateTimeMinuteTz(bucket.endTs).slice(-5)}`,
                                  status: bucket.peak > 0 ? 'alert' : 'ok',
                                  description: `${title}: ${formatCompactNumber(bucket.peak)}${suffix}`,
                                }))}
                              >
                                <StatusBarPlot
                                  barClassName='h-3.5 rounded-[3px]'
                                  showLabels={false}
                                />
                                <StatusBarTooltip />
                              </StatusBar>
                            ) : null}
                          </li>
                        );
                      })}
                    </ul>
                  )}
                </div>
              );
            }

            if (key === 'partitions') {
              // 分区分布：各节点占比 Donut。/ Partition spread: per-node share donut.
              return (
                <div key={key} {...props}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{title}</h4>
                  </div>
                  <DonutChart
                    key='body'
                    data={partitionDonut}
                    label={title}
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
                <div key={key} {...props}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>
                      {t('panels.store_entries')}
                    </h4>
                  </div>
                  {storeBars.categories.length === 0 ? (
                    <p
                      key='body'
                      className='py-6 text-center text-xs text-muted-foreground'
                    >
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <BarChart
                      key='body'
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
                      label={title}
                      formatValue={(value) => formatCompactNumber(value)}
                    />
                  )}
                </div>
              );
            }

            if (key === 'fds') {
              // fd：每节点一条 Progress（已打开 / 上限）。/ fds: open / max per node.
              return (
                <div key={key} {...props}>
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{title}</h4>
                  </div>
                  {fdRows.length === 0 ? (
                    <p
                      key='body'
                      className='py-6 text-center text-xs text-muted-foreground'
                    >
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <ul key='body' className='flex flex-col gap-2'>
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
                            aria-label={`${row.instance} ${title}`}
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
              <div key={key} {...props}>
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{title}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t.has(`units.${item.unit}`) ? t(`units.${item.unit}`) : ''}
                  </span>
                </div>
                <LineChart
                  label={title}
                  // 字节面板单位走 formatBytes（IEC），其余让 unit 拼后缀。
                  // Byte panels carry their unit in formatBytes (IEC); others take the suffix.
                  unit={item.unit === 'bytes' ? '' : suffix}
                  data={chart?.data ?? []}
                  series={chart?.series ?? []}
                  height={170}
                  legendPosition='bottom'
                  maxLegendItems={4}
                  moreLabel={(count) => t('legendMore', {count})}
                  emptySelectionLabel={t('legendEmptySelection')}
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
