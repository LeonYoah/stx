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

import {useMemo} from 'react';
import {useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {MetricCard} from '@/components/arc/metric-card/metric-card';
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
  formatDurationSeconds,
  NEUTRAL_LINE,
} from './dashboard-format';
import {
  HomeService,
  type HomeDetailPanel,
  type HomeHostEntry,
  type HomeHostDashboard,
} from '@/lib/services/dashboard';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import type {DashboardQuery} from './use-dashboard-query';
import {usePolledResource} from './use-polled-resource';
import {TrendEmptyState} from './TrendEmptyState';

const HOSTS_POLL_MS = 30_000;

/** 提示里用完整时间，坐标轴只留 HH:mm。/ Full time in tooltips, HH:mm on axes. */
const formatLabel = (ts: number) => formatDateTimeMinuteTz(ts);
const formatAxis = (ts: number) => formatDateTimeMinuteTz(ts).slice(-5);

/** 面板单位的数值后缀；字节类（含速率）走 formatBytes 自带量纲，不再拼后缀。
 * Value suffix per panel unit; byte-flavored units (incl. rates) format through
 * formatBytes, which carries the suffix itself. */
const unitSuffix = (unit: string) =>
  unit === 'percent'
    ? '%'
    : unit === 'per_second'
      ? '/s'
      : unit === 'seconds'
        ? 's'
        : '';

/** 是否走 formatBytes 的字节类单位（bytes 与 bytes_per_second 都是 IEC 量纲）。
 * Whether the unit formats through formatBytes (bytes and bytes_per_second are IEC). */
const isByteUnit = (unit: string) =>
  unit === 'bytes' || unit === 'bytes_per_second';

/** 字节面板用 IEC 量纲（速率面板追加 /s）、其余用紧凑数（unit 由 LineChart 自拼）。
 * Byte panels take IEC units (rate panels append /s); the rest use compact numbers
 * (LineChart appends the unit itself). */
const formatPanelValue = (unit: string) => (value: number) =>
  unit === 'bytes'
    ? formatBytes(value)
    : unit === 'bytes_per_second'
      ? `${formatBytes(value)}/s`
      : formatCompactNumber(value);

/** 文件系统使用率的分档色：≥95% danger、≥85% warning、其余主色。
 * Usage-tier color for filesystem bars: ≥95% danger, ≥85% warning, primary otherwise. */
const usageTone = (percent: number) =>
  percent >= 95
    ? 'bg-[var(--danger)]'
    : percent >= 85
      ? 'bg-[var(--warning)]'
      : undefined;

/**
 * 主机页签：每台 agent 主机一张卡，卡里是该主机的 stx_host_* 面板组；
 * 无 agent 的主机（docker/k8s）收成一条紧凑提示，不出空图。
 * 一个指标一种图：折线看趋势；文件系统改 Progress 列表回答「哪个快满了」；
 * TCP / fd 是单值指标，收成 MetricCard + Sparkline 小卡。
 * The hosts tab: one card per agent host carrying its stx_host_* panel set;
 * agent-less hosts (docker/k8s) collapse into a compact note instead of empty charts.
 * One chart shape per metric: lines answer "how did it trend"; the filesystem view is a
 * sorted progress list answering "what fills up first"; TCP/fd are single values and get
 * MetricCard + Sparkline mini cards.
 */
export function HostsTab({query}: {query: DashboardQuery}) {
  const t = useTranslations('dashboard.home.hostsTab');
  const panel = 'rounded-xl border border-border/70 bg-card/40 p-4';

  const resource = usePolledResource<HomeHostDashboard>(
    JSON.stringify([
      'home-hosts',
      query.clusterId ?? 'all',
      query.range,
      query.start,
      query.end,
    ]),
    () =>
      HomeService.getHosts({
        cluster_id: query.clusterId,
        ...(query.start && query.end
          ? {start: query.start, end: query.end}
          : {range: query.range}),
      }),
    query.start ? 0 : HOSTS_POLL_MS,
  );
  const data = resource.data;

  const panelTitle = (item: HomeDetailPanel) =>
    t.has(`panels.${item.key}`) ? t(`panels.${item.key}`) : item.key;

  // 可观测未接入：清单照列（主机名 / IP / 规格 / 心跳仍在），上面叠一条原因横幅，
  // 卡片内不画图。/ Observability off: the inventory still renders (name/IP/spec/heartbeat
  // survive) under a reason banner; cards simply draw no charts.
  const metricsOff = data?.source === 'none';

  return (
    <section className='flex flex-col gap-3' data-testid='home-hosts'>
      {metricsOff ? (
        <div className={panel}>
          <TrendEmptyState reason={data.reason} />
        </div>
      ) : null}
      {data?.partial_errors?.length ? (
        <p className='text-xs text-amber-600 dark:text-amber-400' role='status'>
          {t('partial', {keys: data.partial_errors.join(', ')})}
        </p>
      ) : null}

      {resource.loading && !data ? (
        <div className='grid gap-3'>
          {Array.from({length: 2}).map((_, index) => (
            <div
              key={index}
              className='h-[300px] animate-pulse rounded-xl bg-muted/40'
            />
          ))}
        </div>
      ) : !data || data.hosts.length === 0 ? (
        // 无主机（或集群过滤后为空）：一句话空态，不伪装成「指标缺失」。
        // No hosts (or the cluster filter emptied the list): a plain note, not a fake
        // "metrics missing" state.
        <section className={panel}>
          <p className='py-10 text-center text-xs text-muted-foreground'>
            {t('emptyDescription')}
          </p>
        </section>
      ) : (
        data.hosts.map((item) => (
          <HostCard
            key={item.id}
            host={item}
            endMs={data.end_ms}
            panelTitle={panelTitle}
            metricsOff={metricsOff}
          />
        ))
      )}
      {resource.refreshing ? (
        <span className='text-xs text-muted-foreground'>{t('refreshing')}</span>
      ) : null}
    </section>
  );
}

/** 文件系统行：挂载点 + 容量 / inode 两条使用率。/ One filesystem row: mount + capacity/inode usage. */
interface FsRow {
  mount: string;
  capacity: number;
  inodes: number;
}

/** 单台主机卡：头部一行身份 + 状态，身体是面板网格。/ One host card: identity header + panel grid. */
function HostCard({
  host,
  endMs,
  panelTitle,
  metricsOff,
}: {
  host: HomeHostEntry;
  endMs: number;
  panelTitle: (item: HomeDetailPanel) => string;
  metricsOff?: boolean;
}) {
  const t = useTranslations('dashboard.home.hostsTab');
  const durationUnits = {d: t('units.d'), h: t('units.h'), m: t('units.m')};

  const panelsByKey = useMemo(() => {
    const map = new Map<string, HomeDetailPanel>();
    for (const item of host.panels) {
      map.set(item.key, item);
    }
    return map;
  }, [host.panels]);

  // 折线面板：维度语义色（rx/tx、读/写、CPU 模式各归其色），负载加核数参考虚线、
  // 磁盘使用率加 100% 阈值线。/ Line panels: semantic colors per dimension (rx/tx,
  // read/write, CPU modes); the load panel adds a dashed core-count reference and
  // disk_util a dashed 100% ceiling.
  const charts = useMemo(() => {
    const map = new Map<string, ReturnType<typeof toLineChartData>>();
    for (const item of host.panels) {
      if (item.key === 'fs_used' || item.key === 'fs_inodes') {
        continue;
      }
      const colors = colorForLabels(item.series.map((entry) => entry.label));
      const named = item.series.map((entry) => ({
        key: entry.label,
        name: entry.label,
        points: entry.points,
        color: colors.get(entry.label),
      }));
      const chart = toLineChartData(named, formatLabel, formatAxis);
      const reference =
        item.key === 'load' && host.cpu_cores > 0
          ? {label: t('refCores'), value: host.cpu_cores}
          : item.key === 'disk_util'
            ? {label: '100%', value: 100}
            : undefined;
      if (reference && chart.data.length > 0) {
        chart.series.push({
          key: '__reference',
          label: reference.label,
          dashed: true,
          color:
            item.key === 'disk_util' ? 'var(--warning)' : NEUTRAL_LINE,
        });
        chart.data = chart.data.map((datum) => ({
          ...datum,
          values: {...datum.values, __reference: reference.value},
        }));
      }
      map.set(item.key, chart);
    }
    return map;
  }, [host.panels, host.cpu_cores, t]);

  // 文件系统两个面板合并成一张按容量排序的 Progress 列表：回答「哪个挂载点先满」。
  // The two filesystem panels merge into one usage-sorted progress list answering
  // "which mount fills up first".
  const fsRows = useMemo<FsRow[]>(() => {
    const used = panelsByKey.get('fs_used')?.series ?? [];
    const inodes = new Map(
      (panelsByKey.get('fs_inodes')?.series ?? []).map((entry) => [
        entry.label,
        latestValue(entry.points),
      ]),
    );
    return used
      .map((entry) => ({
        mount: entry.label,
        capacity: latestValue(entry.points),
        inodes: inodes.get(entry.label) ?? 0,
      }))
      .sort((a, b) => b.capacity - a.capacity);
  }, [panelsByKey]);

  // TCP 连接与 fd 是单值指标：最新值 + 迷你趋势，不占整图。
  // TCP connections and fds are single values: latest number + a mini trend, not a
  // full chart.
  const connCards = useMemo(
    () =>
      (['tcp', 'fds'] as const)
        .map((key) => {
          const item = panelsByKey.get(key);
          const points = item?.series[0]?.points ?? [];
          return item
            ? {key, title: panelTitle(item), value: latestValue(points), points}
            : null;
        })
        .filter((item): item is NonNullable<typeof item> => item !== null),
    [panelsByKey, panelTitle],
  );

  const memory = host.total_memory_bytes
    ? formatBytes(host.total_memory_bytes)
    : '—';
  // 「已运行 N 天」来自开机时间（stx_host_boot_time_seconds → boot_time_ms）。
  // "Up N days" comes from boot time (stx_host_boot_time_seconds → boot_time_ms).
  const uptime =
    host.boot_time_ms && endMs > host.boot_time_ms
      ? formatDurationSeconds((endMs - host.boot_time_ms) / 1000, durationUnits)
      : null;

  // 渲染顺序跟面板定义一致：fs_used/fs_inodes 在 fs_used 处合并，tcp/fds 在 tcp 处合并。
  // Render order follows the panel list: fs_used/fs_inodes fold into the fs cell at
  // fs_used's slot, tcp/fds into the conn cell at tcp's slot.
  const hasTcp = panelsByKey.has('tcp');
  const cellOrder = host.panels
    .filter(
      // fs_inodes 永远并入 fs 槽；fds 有 tcp 在时并入 conn 槽，缺 tcp 时自己顶 conn 槽。
      // fs_inodes always folds into the fs slot; fds folds into the conn slot when
      // tcp exists, otherwise it takes the conn slot itself.
      (item) =>
        item.key !== 'fs_inodes' && !(item.key === 'fds' && hasTcp),
    )
    .map((item) => item.key);

  return (
    <div className='rounded-xl border border-border/70 bg-card/40 p-4'>
      <div className='mb-3 flex flex-wrap items-center gap-x-3 gap-y-1'>
        <h3 className='text-sm font-medium'>{host.name}</h3>
        <span className='font-mono text-xs text-muted-foreground'>{host.ip}</span>
        <span
          className='rounded-md border border-border/60 bg-muted/30 px-1.5 py-0.5 text-[11px] text-muted-foreground'
          data-testid={`host-agent-${host.id}`}
        >
          {t.has(`agentStatus.${host.agent_status}`)
            ? t(`agentStatus.${host.agent_status}`)
            : host.agent_status || t('agentStatus.none')}
        </span>
        <span className='text-xs text-muted-foreground'>
          {t('spec', {cores: host.cpu_cores || 0, memory})}
        </span>
        {uptime ? (
          <span className='text-xs text-muted-foreground'>
            {t('uptime', {d: uptime})}
          </span>
        ) : null}
        {host.last_heartbeat_ms ? (
          <span className='text-xs text-muted-foreground'>
            {t('heartbeat', {at: formatDateTimeMinuteTz(host.last_heartbeat_ms)})}
          </span>
        ) : null}
      </div>

      {!host.metrics_supported ? (
        // 无 agent 主机：一句话说明，不出一排空图。/ Agent-less host: one note, no empty charts.
        <p
          className='py-6 text-center text-xs text-muted-foreground'
          data-testid={`host-unsupported-${host.id}`}
        >
          {t('unsupported')}
        </p>
      ) : metricsOff ? null : host.panels.every((item) => item.series.length === 0) ? (
        // 支持指标但尚无样本：agent 刚装上 / 上报断了。/ Supported but no samples yet: fresh
        // agent or a broken report path.
        <p className='py-6 text-center text-xs text-muted-foreground'>
          {t('noSamples')}
        </p>
      ) : (
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {cellOrder.map((key) => {
            if (key === 'fs_used') {
              // 文件系统：按容量使用率排序的 Progress 列表（容量 / inode 两条）。
              // Filesystem: a usage-sorted progress list (capacity / inodes per mount).
              return (
                <div
                  key='fs'
                  className='rounded-lg border border-border/50 p-3 xl:col-span-2'
                  data-testid={`host-panel-${host.id}-fs`}
                >
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{t('panels.fs')}</h4>
                    <span className='text-[11px] text-muted-foreground'>
                      {t('fsHint')}
                    </span>
                  </div>
                  {fsRows.length === 0 ? (
                    <p className='py-6 text-center text-xs text-muted-foreground'>
                      {t('emptyPanel')}
                    </p>
                  ) : (
                    <ul className='flex flex-col gap-2'>
                      {fsRows.map((row) => (
                        <li
                          key={row.mount}
                          className='grid grid-cols-[minmax(0,7rem)_1fr_2.6rem_1fr_2.6rem] items-center gap-2'
                        >
                          <span
                            className='truncate font-mono text-[11px] text-muted-foreground'
                            title={row.mount}
                          >
                            {row.mount}
                          </span>
                          <Progress
                            value={row.capacity}
                            className='h-1.5'
                            indicatorClassName={usageTone(row.capacity)}
                            aria-label={`${row.mount} ${t('fsCapacity')}`}
                          />
                          <span className='text-right font-mono text-[11px]'>
                            {Math.round(row.capacity)}%
                          </span>
                          <Progress
                            value={row.inodes}
                            className='h-1.5'
                            indicatorClassName={usageTone(row.inodes)}
                            aria-label={`${row.mount} ${t('fsInodes')}`}
                          />
                          <span className='text-right font-mono text-[11px]'>
                            {Math.round(row.inodes)}%
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              );
            }
            if (key === 'tcp' || key === 'fds') {
              // TCP + fd：两张 MetricCard + Sparkline 小卡合成一格。
              // TCP + fds: two MetricCard + Sparkline mini cards in one cell.
              return (
                <div
                  key='conn'
                  className='rounded-lg border border-border/50 p-3'
                  data-testid={`host-panel-${host.id}-conn`}
                >
                  <div className='mb-2 flex items-baseline gap-2'>
                    <h4 className='text-xs font-medium'>{t('panels.conn')}</h4>
                  </div>
                  <div className='flex flex-col gap-3'>
                    {connCards.map((card) => (
                      <div key={card.key}>
                        <MetricCard
                          label={card.title}
                          value={card.value}
                          context={t('units.count')}
                        />
                        <Sparkline
                          data={card.points.map((point) => point[1])}
                          label={card.title}
                          variant='area'
                          height={30}
                          className='mt-1 text-[var(--series-1)]'
                        />
                      </div>
                    ))}
                  </div>
                </div>
              );
            }
            const item = panelsByKey.get(key);
            if (!item) {
              return null;
            }
            const suffix = unitSuffix(item.unit);
            const chart = charts.get(item.key);
            return (
              <div
                key={item.key}
                className='rounded-lg border border-border/50 p-3'
                data-testid={`host-panel-${host.id}-${item.key}`}
              >
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{panelTitle(item)}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t.has(`units.${item.unit}`) ? t(`units.${item.unit}`) : ''}
                  </span>
                </div>
                <LineChart
                  label={panelTitle(item)}
                  // 字节类面板的单位走 formatBytes（IEC），不再让 unit 追加裸 "B"。
                  // Byte-flavored panels carry their unit in formatBytes (IEC) — no bare
                  // "B" suffix.
                  unit={isByteUnit(item.unit) ? '' : suffix}
                  data={chart?.data ?? []}
                  series={chart?.series ?? []}
                  height={150}
                  emptyLabel={t('emptyPanel')}
                  clearLabel={t('legendClear')}
                  resetLabel={t('legendAll')}
                  categoryLabel={t('time')}
                  formatValue={formatPanelValue(item.unit)}
                />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
