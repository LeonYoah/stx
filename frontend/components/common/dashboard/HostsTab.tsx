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
import {toLineChartData} from '@/components/common/sync/job-detail/metrics-trend-data';
import {formatBytes, formatCompactNumber} from './dashboard-format';
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

/**
 * 主机页签：每台 agent 主机一张卡，卡里是该主机的 stx_host_* 面板组；
 * 无 agent 的主机（docker/k8s）收成一条紧凑提示，不出空图。
 * The hosts tab: one card per agent host carrying its stx_host_* panel set;
 * agent-less hosts (docker/k8s) collapse into a compact note instead of empty charts.
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

/** 单台主机卡：头部一行身份 + 状态，身体是面板网格。/ One host card: identity header + panel grid. */
function HostCard({
  host,
  panelTitle,
  metricsOff,
}: {
  host: HomeHostEntry;
  panelTitle: (item: HomeDetailPanel) => string;
  metricsOff?: boolean;
}) {
  const t = useTranslations('dashboard.home.hostsTab');

  const charts = useMemo(() => {
    const map = new Map<string, ReturnType<typeof toLineChartData>>();
    for (const item of host.panels) {
      const named = item.series.map((entry) => ({
        key: entry.label,
        name: entry.label,
        points: entry.points,
      }));
      map.set(item.key, toLineChartData(named, formatLabel, formatAxis));
    }
    return map;
  }, [host.panels]);

  const memory = host.total_memory_bytes
    ? formatBytes(host.total_memory_bytes)
    : '—';

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
      ) : metricsOff ? (
        // 可观测关闭：保留身份行，指标位一句说明。/ Observability off: keep the identity
        // row, one line where the metrics would be.
        <p
          className='py-6 text-center text-xs text-muted-foreground'
          data-testid={`host-metrics-off-${host.id}`}
        >
          {t('metricsOff')}
        </p>
      ) : host.panels.every((item) => item.series.length === 0) ? (
        // 支持指标但尚无样本：agent 刚装上 / 上报断了。/ Supported but no samples yet: fresh
        // agent or a broken report path.
        <p className='py-6 text-center text-xs text-muted-foreground'>
          {t('noSamples')}
        </p>
      ) : (
        <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
          {host.panels.map((item) => {
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
