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

import {useCallback, useEffect, useMemo, useRef} from 'react';
import {useFormatter, useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {
  CompositionChart,
  type CompositionChartSeries,
} from '@/components/charts/composition-chart';
import {
  StatusBar,
  StatusBarPlot,
  StatusBarTooltip,
} from '@/components/charts/status-bar';
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
  colorByName,
  toLineChartData,
} from '@/components/common/sync/job-detail/metrics-trend-data';
import {Badge} from '@/components/ui/badge';
import {
  HomeService,
  type HomeClusterDashboard,
  type HomeDashboardSeries,
} from '@/lib/services/dashboard';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {formatDurationSeconds} from './dashboard-format';
import {ClusterDetailGroups} from './ClusterDetailGroups';
import {
  ClusterScopeStrip,
  ClusterScopeStripSkeleton,
  type ClusterScopeItem,
} from './ClusterScopeStrip';
import {DashboardPanel} from './DashboardPanel';
import {TrendEmptyState} from './TrendEmptyState';
import type {DashboardQuery} from './use-dashboard-query';
import {usePolledResource} from './use-polled-resource';

const CLUSTER_POLL_MS = 30_000;
const HEAP_REFERENCE = 85;
const MAX_COMPOSITION_PERIODS = 24;

/** 提示里用完整时间，坐标轴只留 HH:mm。/ Full time in tooltips, HH:mm on axes. */
const formatLabel = (ts: number) => formatDateTimeMinuteTz(ts);
const formatAxis = (ts: number) => formatDateTimeMinuteTz(ts).slice(-5);

/** 引擎作业状态的显示顺序与语义色（状态色只表示状态）。/ Engine job state order and semantic colors. */
const JOB_STATE_COLORS: Record<string, string> = {
  running: 'var(--info)',
  scheduled: 'var(--series-3)',
  pending: 'var(--warning)',
  created: 'var(--series-4)',
  finished: 'var(--success)',
  failing: 'color-mix(in oklab, var(--danger) 60%, var(--warning))',
  failed: 'var(--danger)',
  cancelling: 'color-mix(in oklab, var(--text-muted) 70%, var(--warning))',
  canceled: 'var(--text-muted)',
};

const HEALTH_TONE: Record<string, string> = {
  healthy:
    'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  degraded:
    'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
  unhealthy: 'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400',
};

/**
 * 首页集群页签：集群范围选择条（切换范围）→ 节点可用性 → 堆 / CPU / GC / 引擎作业状态趋势 → 节点表 + 异常信号。
 * 未接入 Prometheus 时选择条与节点表照常（Agent 心跳），趋势区显示统一空态。
 * The home cluster tab: cluster scope strip (scope switch) → node availability → heap / CPU / GC / engine job state
 * trends → node table + anomaly signals. Without Prometheus the strip and node table still load (agent heartbeats)
 * and the trend area shows the shared empty state.
 */
export function ClusterTab({
  query,
  update,
}: {
  query: DashboardQuery;
  update: (
    patch: Partial<Record<keyof DashboardQuery, string | number | undefined>>,
  ) => void;
}) {
  const t = useTranslations('dashboard.home.clusterTab');
  const format = useFormatter();
  const resource = usePolledResource<HomeClusterDashboard>(
    JSON.stringify([
      query.clusterId ?? 'all',
      query.range,
      query.start,
      query.end,
    ]),
    () =>
      HomeService.getClusters({
        cluster_id: query.clusterId,
        ...(query.start && query.end
          ? {start: query.start, end: query.end}
          : {range: query.range}),
      }),
    // 自定义时间窗固定不变，不自动刷新。/ A custom window is fixed, so it does not auto-refresh.
    query.start ? 0 : CLUSTER_POLL_MS,
  );
  const data = resource.data;
  const hasMetrics = data?.source === 'prometheus';
  const durationUnits = {d: t('units.d'), h: t('units.h'), m: t('units.m')};

  const byKey = useMemo(() => {
    const map = new Map<string, HomeDashboardSeries[]>();
    for (const item of data?.series ?? []) {
      map.set(item.key, [...(map.get(item.key) ?? []), item]);
    }
    return map;
  }, [data]);
  // 节点颜色跟实例名固定，同一节点在各图里同色。/ Node colors follow the instance name across charts.
  const nodeColors = useMemo(
    () =>
      colorByName(
        (data?.nodes ?? []).map((node) => node.instance).filter(Boolean),
      ),
    [data],
  );
  const clusterNames = useMemo(() => {
    const map = new Map<number, string>();
    for (const card of data?.clusters ?? []) {
      map.set(card.id, card.name);
    }
    return map;
  }, [data]);
  const disambiguate = useCallback(
    (items: HomeDashboardSeries[]) => {
      const counts = new Map<string, number>();
      for (const item of items) {
        counts.set(item.label, (counts.get(item.label) ?? 0) + 1);
      }
      return items.map((item, index) => {
        const clusterName =
          item.cluster_id != null ? clusterNames.get(item.cluster_id) : undefined;
        const dup = (counts.get(item.label) ?? 0) > 1;
        const key =
          item.cluster_id != null
            ? `${item.label}#${item.cluster_id}`
            : dup
              ? `${item.label}#${index}`
              : item.label;
        return {
          key,
          name: dup && clusterName ? `${item.label} · ${clusterName}` : item.label,
          points: item.points,
          color: nodeColors.get(item.label),
        };
      });
    },
    [clusterNames, nodeColors],
  );
  const lineFor = (key: string, reference?: {label: string; value: number}) => {
    const items = byKey.get(key) ?? [];
    const named = disambiguate(items);
    const chart = toLineChartData(named, formatLabel, formatAxis);
    if (reference && chart.data.length > 0) {
      chart.series.push({
        key: '__reference',
        label: reference.label,
        dashed: true,
        color: 'var(--warning)',
      });
      chart.data = chart.data.map((datum) => ({
        ...datum,
        values: {...datum.values, __reference: reference.value},
      }));
    }
    return chart;
  };

  const composition = useMemo(() => {
    const items = byKey.get('job_state') ?? [];
    const timestamps = Array.from(
      new Set(items.flatMap((item) => item.points.map((point) => point[0]))),
    ).sort((a, b) => a - b);
    const stride = Math.max(
      1,
      Math.ceil(timestamps.length / MAX_COMPOSITION_PERIODS),
    );
    const picked = timestamps.filter(
      (_, index) => index % stride === 0 || index === timestamps.length - 1,
    );
    const order = Object.keys(JOB_STATE_COLORS);
    const series: CompositionChartSeries[] = items
      .filter((item) => item.points.some((point) => point[1] > 0))
      .sort((a, b) => order.indexOf(a.label) - order.indexOf(b.label))
      .map((item) => {
        const values = new Map(
          item.points.map((point) => [point[0], point[1]]),
        );
        return {
          id: item.label,
          name: t.has(`jobStates.${item.label}`)
            ? t(`jobStates.${item.label}`)
            : item.label,
          color: JOB_STATE_COLORS[item.label] ?? 'var(--series-2)',
          values: picked.map((ts) => values.get(ts) ?? null),
        };
      });
    return {series, periods: picked.map(formatAxis)};
  }, [byKey, t]);

  const cards = data?.clusters ?? [];
  const totals = cards.reduce(
    (acc, card) => ({
      online: acc.online + card.nodes_online,
      total: acc.total + card.nodes_total,
      running: acc.running + card.running_jobs,
      alerts: acc.alerts + card.active_alerts,
      heap: Math.max(acc.heap, card.max_heap_percent ?? 0),
    }),
    {online: 0, total: 0, running: 0, alerts: 0, heap: 0},
  );
  const cardDescription = (info: {
    online: number;
    total: number;
    running: number;
    heap?: number;
    alerts: number;
  }) => (
    <span className='flex flex-col gap-0.5'>
      <span>
        {t('cardDesc', {
          online: info.online,
          total: info.total,
          running: info.running,
        })}
      </span>
      <span>
        {info.heap !== undefined && info.heap > 0
          ? `${t('cardHeap', {value: format.number(info.heap, {maximumFractionDigits: 0})})} · `
          : ''}
        {t('cardAlerts', {count: info.alerts})}
      </span>
    </span>
  );
  const items = [
    {
      key: 'all',
      title: t('allClusters'),
      description: cardDescription({...totals, heap: totals.heap || undefined}),
    },
    ...cards.map((card) => ({
      key: String(card.id),
      title: card.version ? `${card.name} · ${card.version}` : card.name,
      description: cardDescription({
        online: card.nodes_online,
        total: card.nodes_total,
        running: card.running_jobs,
        heap: card.max_heap_percent,
        alerts: card.active_alerts,
      }),
      meta: (
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm px-1.5 py-0 text-[11px]',
            HEALTH_TONE[card.health_status],
          )}
        >
          {t.has(`health.${card.health_status}`)
            ? t(`health.${card.health_status}`)
            : card.health_status}
        </Badge>
      ),
    })),
  ];
  // 集群格数据与所选范围无关，切换范围触发的重拉会先把 data 清空；
  // 留上一份格子让选择条不卸载，键盘焦点才不会掉到 <body>（零塌陷）。
  // Cluster cells are scope-independent, but a scope-switch refetch clears
  // data first; keep the previous cells so the strip stays mounted and
  // keyboard focus is not dropped to <body> (zero-collapse).
  const lastItems = useRef<ClusterScopeItem[]>([]);
  useEffect(() => {
    if (data) {
      lastItems.current = items;
    }
  }, [data, items]);
  const stripItems =
    data || lastItems.current.length === 0 ? items : lastItems.current;

  const availability = byKey.get('availability') ?? [];
  const cellText = data
    ? formatDurationSeconds(data.cell_seconds, durationUnits)
    : '';
  const signals = data?.signals;
  const signalItems = signals
    ? [
        {
          key: 'clusterSafe',
          ok: signals.clusters_safe === signals.clusters_total,
          value: `${signals.clusters_safe}/${signals.clusters_total}`,
        },
        {
          key: 'deadlocked',
          ok: (signals.deadlocked_threads ?? 0) === 0,
          value: format.number(signals.deadlocked_threads ?? 0),
        },
        {
          key: 'fd',
          ok: (signals.fd_percent_max ?? 0) < 80,
          value: `${format.number(signals.fd_percent_max ?? 0, {maximumFractionDigits: 1})}%`,
        },
        {
          key: 'reject',
          ok: (signals.pool_reject_rate ?? 0) === 0,
          value: t('signals.perSec', {
            value: format.number(signals.pool_reject_rate ?? 0, {
              maximumFractionDigits: 2,
            }),
          }),
        },
        {
          key: 'hzQueue',
          ok: (signals.hz_queue_util_max ?? 0) < 80,
          value: `${format.number(signals.hz_queue_util_max ?? 0, {maximumFractionDigits: 1})}%`,
        },
        {
          key: 'endpoints',
          ok: signals.endpoints_down === 0,
          value: format.number(signals.endpoints_down),
        },
      ]
    : [];
  const allGood =
    signalItems.length > 0 && signalItems.every((item) => item.ok);

  return (
    <div className='flex flex-col gap-3' data-testid='home-cluster-tab'>
      {resource.loading && !data && lastItems.current.length === 0 ? (
        <ClusterScopeStripSkeleton count={3} />
      ) : (
        <ClusterScopeStrip
          label={t('cardsLabel')}
          value={query.clusterId ? String(query.clusterId) : 'all'}
          onValueChange={(value) =>
            update({clusterId: value === 'all' ? undefined : value, page: 1})
          }
          items={stripItems}
        />
      )}
      {data?.partial_errors?.length ? (
        <p className='text-xs text-amber-600 dark:text-amber-400' role='status'>
          {t('partial', {keys: data.partial_errors.join(', ')})}
        </p>
      ) : null}

      {data && !hasMetrics ? (
        <TrendEmptyState reason={data.reason} />
      ) : data ? (
        <>
          <DashboardPanel
            title={t('availability.title')}
            hint={t('availability.hint', {cell: cellText})}
            testId='home-availability'
          >
            {availability.length === 0 ? (
              <p className='py-4 text-center text-xs text-muted-foreground'>
                {t('availability.empty')}
              </p>
            ) : (
              <div className='flex flex-col gap-2'>
                {disambiguate(availability).map((item) => {
                  const ok = item.points.filter(
                    (point) => point[1] >= 1,
                  ).length;
                  const percent = item.points.length
                    ? (ok / item.points.length) * 100
                    : 0;
                  return (
                    <div
                      key={item.key}
                      className='grid grid-cols-[minmax(0,1fr)_3.5rem] items-center gap-x-3 gap-y-1 sm:grid-cols-[minmax(0,10rem)_minmax(0,1fr)_3.5rem]'
                    >
                      <span
                        className='truncate font-mono text-xs text-muted-foreground'
                        title={item.name}
                      >
                        {item.name}
                      </span>
                      <StatusBar
                        // 窄屏：节点名与可用率一行，状态条独占下一行。/ Narrow: name + ratio on one row, the bar below.
                        className='order-last col-span-2 sm:order-none sm:col-span-1'
                        label={item.name}
                        statuses={[
                          {
                            id: 'up',
                            label: t('availability.up'),
                            color: 'var(--success)',
                          },
                          {
                            id: 'down',
                            label: t('availability.down'),
                            color: 'var(--danger)',
                          },
                          {
                            id: 'unknown',
                            label: t('availability.unknown'),
                            color: 'var(--text-muted)',
                          },
                        ]}
                        data={item.points.map((point) => ({
                          id: String(point[0]),
                          label: formatDateTimeMinuteTz(point[0]),
                          status: point[1] >= 1 ? 'up' : 'down',
                        }))}
                      >
                        <StatusBarPlot />
                        <StatusBarTooltip />
                      </StatusBar>
                      <span
                        className={cn(
                          'text-right text-xs tabular-nums',
                          percent < 100
                            ? 'text-foreground'
                            : 'text-muted-foreground',
                        )}
                      >
                        {format.number(percent, {maximumFractionDigits: 1})}%
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
          </DashboardPanel>

          <div className='grid gap-3 lg:grid-cols-2'>
            {[
              {
                key: 'heap',
                title: t('charts.heap'),
                unit: t('charts.heapUnit'),
                suffix: '%',
                reference: {label: t('charts.heapLine'), value: HEAP_REFERENCE},
              },
              {
                key: 'cpu',
                title: t('charts.cpu'),
                unit: t('charts.cpuUnit'),
                suffix: '',
              },
              {
                key: 'gc',
                title: t('charts.gc'),
                unit: t('charts.gcUnit'),
                suffix: '%',
              },
            ].map((chart) => {
              const line = lineFor(chart.key, chart.reference);
              return (
                <DashboardPanel
                  key={chart.key}
                  title={chart.title}
                  hint={chart.unit}
                >
                  <LineChart
                    label={chart.title}
                    unit={chart.unit}
                    data={line.data}
                    series={line.series}
                    height={200}
                    emptyLabel={t('charts.empty')}
                    clearLabel={t('charts.legendClear')}
                    resetLabel={t('charts.legendAll')}
                    categoryLabel={t('charts.time')}
                    formatValue={(value) =>
                      `${format.number(value, {maximumFractionDigits: 2})}${chart.suffix}`
                    }
                  />
                </DashboardPanel>
              );
            })}
            <DashboardPanel
              title={t('charts.jobState')}
              hint={t('charts.jobStateUnit')}
            >
              {composition.series.length === 0 ? (
                <p className='py-16 text-center text-xs text-muted-foreground'>
                  {t('charts.empty')}
                </p>
              ) : (
                <CompositionChart
                  label={t('charts.jobState')}
                  series={composition.series}
                  periods={composition.periods}
                  formatValue={(value) => format.number(value)}
                />
              )}
            </DashboardPanel>
          </div>

          {/* 节点指标细节：按组懒加载，引擎未上报的组显示空态。/ Node metric details: lazily loaded
          per group, groups the engine does not report show an empty state. */}
          <ClusterDetailGroups query={query} nodeColors={nodeColors} />
        </>
      ) : null}

      {data ? (
        <div className='grid gap-3 xl:grid-cols-[minmax(0,1fr)_20rem]'>
          <DashboardPanel
            title={t('nodes.title')}
            hint={t('nodes.count', {count: data.nodes.length})}
            flush
            className='overflow-hidden'
            testId='home-node-table'
          >
            <StudioTable label={t('nodes.title')}>
              <StudioTableScroll>
                <StudioTableContent minWidth={760}>
                  <StudioTableHeader>
                    <StudioTableColumn>{t('nodes.node')}</StudioTableColumn>
                    <StudioTableColumn>{t('nodes.cluster')}</StudioTableColumn>
                    <StudioTableColumn>{t('nodes.role')}</StudioTableColumn>
                    <StudioTableColumn>{t('nodes.status')}</StudioTableColumn>
                    <StudioTableColumn>{t('nodes.heap')}</StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('nodes.cpu')}
                    </StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('nodes.threads')}
                    </StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('nodes.fd')}
                    </StudioTableColumn>
                    <StudioTableColumn>{t('nodes.uptime')}</StudioTableColumn>
                  </StudioTableHeader>
                  <StudioTableBody dimmed={resource.refreshing}>
                    {data.nodes.length === 0 ? (
                      <StudioTableEmptyRow colSpan={9}>
                        {t('nodes.empty')}
                      </StudioTableEmptyRow>
                    ) : (
                      data.nodes.map((node) => {
                        const problem = !node.agent_online
                          ? t('nodes.agentOffline')
                          : node.scrape_up === false
                            ? t('nodes.scrapeDown')
                            : node.node_active === false
                              ? t('nodes.nodeDown')
                              : null;
                        return (
                          <StudioTableRow key={node.node_id}>
                            <StudioTableCell className='font-mono'>
                              <span className='inline-flex items-center gap-1.5'>
                                <span
                                  className='size-2 shrink-0 rounded-full'
                                  style={{
                                    background:
                                      nodeColors.get(node.instance) ??
                                      'var(--text-muted)',
                                  }}
                                  aria-hidden='true'
                                />
                                {node.instance || '-'}
                              </span>
                            </StudioTableCell>
                            <StudioTableCell className='text-muted-foreground'>
                              {node.cluster_name}
                            </StudioTableCell>
                            <StudioTableCell className='text-muted-foreground'>
                              {node.role}
                              {node.master ? ` · ${t('nodes.master')}` : ''}
                            </StudioTableCell>
                            <StudioTableCell>
                              <Badge
                                variant='outline'
                                className={cn(
                                  'rounded-sm px-1.5 py-0 text-[11px]',
                                  problem
                                    ? HEALTH_TONE.degraded
                                    : HEALTH_TONE.healthy,
                                )}
                              >
                                {problem ?? node.status}
                              </Badge>
                            </StudioTableCell>
                            <StudioTableCell>
                              {node.heap_percent === undefined ? (
                                '-'
                              ) : (
                                <span className='flex items-center gap-2'>
                                  <span className='h-1.5 w-16 overflow-hidden rounded-full bg-muted'>
                                    <span
                                      className='block h-full rounded-full'
                                      style={{
                                        width: `${Math.min(100, node.heap_percent)}%`,
                                        background:
                                          node.heap_percent >= HEAP_REFERENCE
                                            ? 'var(--danger)'
                                            : 'var(--series-1)',
                                      }}
                                    />
                                  </span>
                                  <span className='tabular-nums'>
                                    {format.number(node.heap_percent, {
                                      maximumFractionDigits: 0,
                                    })}
                                    %
                                  </span>
                                </span>
                              )}
                            </StudioTableCell>
                            <StudioTableCell
                              align='end'
                              className='tabular-nums'
                            >
                              {node.cpu_cores === undefined
                                ? '-'
                                : t('nodes.cores', {
                                    value: format.number(node.cpu_cores, {
                                      maximumFractionDigits: 2,
                                    }),
                                  })}
                            </StudioTableCell>
                            <StudioTableCell
                              align='end'
                              className='tabular-nums'
                            >
                              {node.threads === undefined
                                ? '-'
                                : format.number(node.threads)}
                            </StudioTableCell>
                            <StudioTableCell
                              align='end'
                              className='tabular-nums'
                            >
                              {node.fd_percent === undefined
                                ? '-'
                                : `${format.number(node.fd_percent, {maximumFractionDigits: 1})}%`}
                            </StudioTableCell>
                            <StudioTableCell className='whitespace-nowrap text-muted-foreground'>
                              {node.uptime_seconds === undefined
                                ? '-'
                                : formatDurationSeconds(
                                    node.uptime_seconds,
                                    durationUnits,
                                  )}
                            </StudioTableCell>
                          </StudioTableRow>
                        );
                      })
                    )}
                  </StudioTableBody>
                </StudioTableContent>
              </StudioTableScroll>
            </StudioTable>
          </DashboardPanel>

          {signals ? (
            <DashboardPanel
              title={t('signals.title')}
              hint={allGood ? t('signals.allGood') : undefined}
              testId='home-signals'
            >
              <ul className='mt-1 flex flex-col divide-y divide-border/60'>
                {signalItems.map((item) => (
                  <li
                    key={item.key}
                    className='flex items-center gap-2 py-2 text-xs'
                  >
                    <span
                      className='min-w-0 flex-1 truncate text-muted-foreground'
                      title={t(`signals.${item.key}`)}
                    >
                      {t(`signals.${item.key}`)}
                    </span>
                    <span className='shrink-0 text-sm font-medium tabular-nums'>
                      {item.value}
                    </span>
                    <Badge
                      variant='outline'
                      className={cn(
                        'shrink-0 rounded-sm px-1 py-0 text-[10px]',
                        item.ok ? HEALTH_TONE.healthy : HEALTH_TONE.degraded,
                      )}
                    >
                      {item.ok ? t('signals.ok') : t('signals.bad')}
                    </Badge>
                  </li>
                ))}
              </ul>
            </DashboardPanel>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
