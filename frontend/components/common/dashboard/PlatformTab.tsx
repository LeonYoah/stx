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
import {useFormatter, useTranslations} from 'next-intl';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {Badge} from '@/components/ui/badge';
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
import {toLineChartData} from '@/components/common/sync/job-detail/metrics-trend-data';
import services from '@/lib/services';
import type {
  PlatformAgentSnapshot,
  PlatformSnapshotData,
} from '@/lib/services/monitoring';
import {formatDateTimeMinuteTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {
  formatBytes,
  formatCompactNumber,
  formatDurationSeconds,
} from './dashboard-format';
import {DashboardPanel} from './DashboardPanel';
import {TrendEmptyState} from './TrendEmptyState';
import {usePolledResource} from './use-polled-resource';

const PLATFORM_POLL_MS = 30_000;

/** 提示里用完整时间，坐标轴只留 HH:mm。/ Full time in tooltips, HH:mm on axes. */
const formatLabel = (ts: number) => formatDateTimeMinuteTz(ts);
const formatAxis = (ts: number) => formatDateTimeMinuteTz(ts).slice(-5);

/** 与集群页签一致的在线/异常徽标色。/ Same health tones as the cluster tab badges. */
const HEALTH_TONE = {
  healthy:
    'border-emerald-500/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  degraded:
    'border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400',
  unhealthy:
    'border-red-500/30 bg-red-500/10 text-red-600 dark:text-red-400',
};

// 各趋势序列的固定语义色：RSS 沿维度色板写色、堆取第四分类色、QPS 主色、错误率危险色。
// Fixed semantic colors per trend series: RSS follows the palette write color,
// heap the fourth categorical color, QPS the primary, error ratio the danger color.
const SERIES_COLORS = {
  rss: 'var(--series-2)',
  heap: 'var(--series-4)',
  rps: 'var(--series-1)',
  http5xx: 'var(--danger)',
} as const;

/**
 * 控制台「平台」页签：STX Server 进程概览 + 内存/请求趋势 + Agent 进程表。
 * 数据来自 GET /api/v1/monitoring/platform；可观测性不可用时降级为统一空态。
 * The dashboard platform tab: STX server process overview, memory/request
 * trends and the agent process table, backed by GET /api/v1/monitoring/platform.
 * It degrades to the shared empty state while observability is unavailable.
 */
export function PlatformTab() {
  const t = useTranslations('dashboard.home.platformTab');
  const format = useFormatter();
  const durationUnits = {d: t('units.d'), h: t('units.h'), m: t('units.m')};

  const resource = usePolledResource<PlatformSnapshotData>(
    'platform-snapshot',
    async () => {
      const result = await services.monitoring.getPlatformSnapshotSafe();
      if (!result.success || !result.data) {
        throw new Error(result.error || t('loadError'));
      }
      return result.data;
    },
    PLATFORM_POLL_MS,
  );
  const snapshot = resource.data;
  const server = snapshot?.server;
  const agents = snapshot?.agents ?? [];
  const available = Boolean(snapshot?.available);

  const charts = useMemo(() => {
    const build = (
      entries: {key: string; name: string; color: string; points?: [number, number][]}[],
    ) =>
      toLineChartData(
        entries.map((item) => ({
          key: item.key,
          name: item.name,
          points: item.points ?? [],
          color: item.color,
        })),
        formatLabel,
        formatAxis,
      );
    return {
      memory: build([
        {
          key: 'rss',
          name: t('seriesRss'),
          color: SERIES_COLORS.rss,
          points: snapshot?.trends?.rss_bytes,
        },
        {
          key: 'heap',
          name: t('seriesHeap'),
          color: SERIES_COLORS.heap,
          points: snapshot?.trends?.heap_inuse_bytes,
        },
      ]),
      rps: build([
        {
          key: 'rps',
          name: t('panels.httpQps'),
          color: SERIES_COLORS.rps,
          points: snapshot?.trends?.http_rps,
        },
      ]),
      http5xx: build([
        {
          key: 'http5xx',
          name: t('panels.http5xx'),
          color: SERIES_COLORS.http5xx,
          points: snapshot?.trends?.http_5xx_ratio,
        },
      ]),
    };
  }, [snapshot, t]);

  return (
    <section className='flex flex-col gap-3' data-testid='home-platform-tab'>
      {resource.error && !snapshot ? (
        <p className='text-xs text-amber-600 dark:text-amber-400' role='alert'>
          {t('loadError')}
        </p>
      ) : null}

      {resource.loading && !snapshot ? (
        <div className='grid gap-3'>
          <div className='h-[300px] animate-pulse rounded-xl bg-muted/40' />
          <div className='h-[220px] animate-pulse rounded-xl bg-muted/40' />
        </div>
      ) : !available ? (
        // 自监控不可用时只出空态与探活提醒：进程内指标看不到 server 自身宕机。
        // While self-monitoring is unavailable only the empty state and the probe
        // note render: in-process metrics cannot see the server itself crash.
        <>
          <DashboardPanel>
            <TrendEmptyState
              reason={snapshot?.reason}
              description={t('noPromDesc')}
              unavailableDescription={t('noPromUnavailableDesc')}
            />
          </DashboardPanel>
          <p className='text-center text-[11px] text-muted-foreground'>
            {t('externalProbeNote')}
          </p>
        </>
      ) : (
        <>
          {/* Server 身份行 + 指标面板组（结构与主机卡一致）/ Identity header + panel grid, same shape as a host card */}
          <DashboardPanel>
            <div className='mb-3 flex flex-wrap items-center gap-x-3 gap-y-1'>
              <h3 className='text-sm font-medium'>{t('serverTitle')}</h3>
              {server?.version ? (
                <span className='rounded-md border border-border/60 bg-muted/30 px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground'>
                  v{server.version}
                </span>
              ) : null}
              {server?.uptime_seconds !== undefined ? (
                <span className='text-xs text-muted-foreground'>
                  {t('uptime', {
                    d: formatDurationSeconds(
                      server.uptime_seconds,
                      durationUnits,
                    ),
                  })}
                </span>
              ) : null}
              {server?.agents_connected !== undefined ? (
                <span className='text-xs text-muted-foreground'>
                  {t('agentsOnline', {
                    count: format.number(server.agents_connected),
                  })}
                </span>
              ) : null}
              {server?.goroutines !== undefined ? (
                <span className='text-xs text-muted-foreground'>
                  {t('goroutines', {
                    count: format.number(server.goroutines),
                  })}
                </span>
              ) : null}
            </div>

            <div className='grid gap-3 md:grid-cols-2 xl:grid-cols-3'>
              <div className='rounded-lg border border-border/50 p-3 xl:col-span-2'>
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{t('panels.memory')}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t('panels.memoryUnit')}
                  </span>
                </div>
                <LineChart
                  label={t('panels.memory')}
                  data={charts.memory.data}
                  series={charts.memory.series}
                  height={150}
                  emptyLabel={t('emptyPanel')}
                  clearLabel={t('legendClear')}
                  resetLabel={t('legendAll')}
                  categoryLabel={t('time')}
                  formatValue={formatBytes}
                />
              </div>
              <div className='rounded-lg border border-border/50 p-3'>
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{t('panels.httpQps')}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t('panels.qpsUnit')}
                  </span>
                </div>
                <LineChart
                  label={t('panels.httpQps')}
                  unit={t('panels.qpsUnit')}
                  data={charts.rps.data}
                  series={charts.rps.series}
                  height={150}
                  emptyLabel={t('emptyPanel')}
                  clearLabel={t('legendClear')}
                  resetLabel={t('legendAll')}
                  categoryLabel={t('time')}
                  formatValue={(value) =>
                    format.number(value, {maximumFractionDigits: 2})
                  }
                />
              </div>
              <div className='rounded-lg border border-border/50 p-3'>
                <div className='mb-2 flex items-baseline gap-2'>
                  <h4 className='text-xs font-medium'>{t('panels.http5xx')}</h4>
                  <span className='text-[11px] text-muted-foreground'>
                    {t('panels.ratioUnit')}
                  </span>
                </div>
                <LineChart
                  label={t('panels.http5xx')}
                  unit='%'
                  data={charts.http5xx.data}
                  series={charts.http5xx.series}
                  height={150}
                  emptyLabel={t('emptyPanel')}
                  clearLabel={t('legendClear')}
                  resetLabel={t('legendAll')}
                  categoryLabel={t('time')}
                  // 比例存的是 0-1，展示换算成百分数。/ The ratio is stored 0-1; display converts to percent.
                  formatValue={(value) =>
                    `${format.number(value * 100, {maximumFractionDigits: 2})}%`
                  }
                />
              </div>
              <PanelList
                title={t('panels.control')}
                rows={[
                  {
                    key: 'p95',
                    label: t('rows.httpP95'),
                    value: `${format.number((server?.http_p95_seconds ?? 0) * 1000, {maximumFractionDigits: 0})}ms`,
                  },
                  {
                    key: 'db',
                    label: t('rows.dbPool'),
                    title: t('rows.dbPoolHint'),
                    value: `${formatCompactNumber(server?.db_in_use)}/${formatCompactNumber(server?.db_open)}`,
                  },
                  {
                    key: 'wait',
                    label: t('rows.dbWait'),
                    value: `${formatCompactNumber(server?.db_wait_count_rate)}/s`,
                  },
                ]}
              />
              <PanelList
                title={t('panels.storage')}
                rows={[
                  {
                    key: 'head',
                    label: t('rows.headSeries'),
                    value: formatCompactNumber(server?.tsdb?.head_series),
                  },
                  {
                    key: 'blocks',
                    label: t('rows.blocks'),
                    value:
                      server?.tsdb?.blocks_bytes !== undefined
                        ? formatBytes(server.tsdb.blocks_bytes)
                        : '-',
                  },
                  {
                    key: 'compactions',
                    label: t('rows.compactions'),
                    value: formatCompactNumber(
                      server?.tsdb?.compactions_failed,
                    ),
                    ok: (server?.tsdb?.compactions_failed ?? 0) === 0,
                  },
                ]}
                okLabel={t('signals.ok')}
                badLabel={t('signals.bad')}
              />
            </div>
          </DashboardPanel>

          {/* Agent 进程表：与节点表同款 StudioTable / Agent table, same StudioTable as the node table */}
          <DashboardPanel
            title={t('agents.title')}
            hint={t('agents.count', {count: agents.length})}
            flush
            className='overflow-hidden'
            testId='home-agent-table'
          >
            <StudioTable label={t('agents.title')}>
              <StudioTableScroll>
                <StudioTableContent minWidth={880}>
                  <StudioTableHeader>
                    <StudioTableColumn>{t('agents.host')}</StudioTableColumn>
                    <StudioTableColumn>{t('agents.status')}</StudioTableColumn>
                    <StudioTableColumn>{t('agents.version')}</StudioTableColumn>
                    <StudioTableColumn>{t('agents.uptime')}</StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('agents.rss')}
                    </StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('agents.cpu')}
                    </StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('agents.goroutines')}
                    </StudioTableColumn>
                    <StudioTableColumn align='end' style={{minWidth: 0}}>
                      {t('agents.cmdFailures')}
                    </StudioTableColumn>
                    <StudioTableColumn>
                      {t('agents.lastReport')}
                    </StudioTableColumn>
                  </StudioTableHeader>
                  <StudioTableBody dimmed={resource.refreshing}>
                    {agents.length === 0 ? (
                      <StudioTableEmptyRow colSpan={9}>
                        {t('agents.empty')}
                      </StudioTableEmptyRow>
                    ) : (
                      agents.map((agent) => (
                        <AgentRow
                          key={`${agent.host_id ?? 'unknown'}-${agent.host_name ?? ''}`}
                          agent={agent}
                          serverVersion={server?.version}
                          durationUnits={durationUnits}
                        />
                      ))
                    )}
                  </StudioTableBody>
                </StudioTableContent>
              </StudioTableScroll>
            </StudioTable>
          </DashboardPanel>
        </>
      )}
      {resource.refreshing ? (
        <span className='text-xs text-muted-foreground'>{t('refreshing')}</span>
      ) : null}
    </section>
  );
}

/** 单值指标列表格（对应主机页签的控制面/信号小格）。/ Stat-list cell, the hosts tab's signals-cell counterpart. */
function PanelList({
  title,
  rows,
  okLabel,
  badLabel,
}: {
  title: string;
  rows: {key: string; label: string; title?: string; value: string; ok?: boolean}[];
  okLabel?: string;
  badLabel?: string;
}) {
  return (
    <div className='rounded-lg border border-border/50 p-3'>
      <div className='mb-1 flex items-baseline gap-2'>
        <h4 className='text-xs font-medium'>{title}</h4>
      </div>
      <ul className='flex flex-col divide-y divide-border/60'>
        {rows.map((row) => (
          <li key={row.key} className='flex items-center gap-2 py-2 text-xs'>
            <span
              className='min-w-0 flex-1 truncate text-muted-foreground'
              title={row.title ?? row.label}
            >
              {row.label}
            </span>
            <span className='shrink-0 text-sm font-medium tabular-nums'>
              {row.value}
            </span>
            {row.ok !== undefined ? (
              <Badge
                variant='outline'
                className={cn(
                  'shrink-0 rounded-sm px-1 py-0 text-[10px]',
                  row.ok ? HEALTH_TONE.healthy : HEALTH_TONE.degraded,
                )}
              >
                {row.ok ? okLabel : badLabel}
              </Badge>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  );
}

function AgentRow({
  agent,
  serverVersion,
  durationUnits,
}: {
  agent: PlatformAgentSnapshot;
  serverVersion?: string;
  durationUnits: {d: string; h: string; m: string};
}) {
  const t = useTranslations('dashboard.home.platformTab');
  const format = useFormatter();
  // 版本不一致由前端比对快照自带 server 版本得出（Agent 未上报版本时不判）。
  // Version mismatch is derived against the snapshot's server version (skipped
  // while the agent reports none).
  const mismatch =
    Boolean(agent.version) &&
    Boolean(serverVersion) &&
    agent.version !== serverVersion;
  return (
    <StudioTableRow>
      <StudioTableCell className='font-medium'>
        {agent.host_name || `host-${agent.host_id ?? '?'}`}
      </StudioTableCell>
      <StudioTableCell>
        <Badge
          variant='outline'
          className={cn(
            'rounded-sm px-1.5 py-0 text-[11px]',
            agent.online ? HEALTH_TONE.healthy : HEALTH_TONE.degraded,
          )}
        >
          {agent.online ? t('agents.online') : t('agents.offline')}
        </Badge>
      </StudioTableCell>
      <StudioTableCell className='font-mono'>
        <span className='inline-flex items-center gap-1.5'>
          {agent.version || '-'}
          {mismatch ? (
            <Badge
              variant='outline'
              className={cn(
                'rounded-sm px-1 py-0 text-[10px]',
                HEALTH_TONE.degraded,
              )}
            >
              {t('agents.versionMismatch')}
            </Badge>
          ) : null}
        </span>
      </StudioTableCell>
      <StudioTableCell className='whitespace-nowrap text-muted-foreground'>
        {agent.uptime_seconds !== undefined && agent.uptime_seconds !== null
          ? formatDurationSeconds(agent.uptime_seconds, durationUnits)
          : '-'}
      </StudioTableCell>
      <StudioTableCell align='end' className='tabular-nums'>
        {agent.rss_bytes !== undefined && agent.rss_bytes !== null
          ? formatBytes(agent.rss_bytes)
          : '-'}
      </StudioTableCell>
      <StudioTableCell align='end' className='tabular-nums'>
        {agent.cpu_ratio !== undefined && agent.cpu_ratio !== null
          ? `${format.number(agent.cpu_ratio * 100, {maximumFractionDigits: 1})}%`
          : '-'}
      </StudioTableCell>
      <StudioTableCell align='end' className='tabular-nums'>
        {agent.goroutines !== undefined && agent.goroutines !== null
          ? format.number(agent.goroutines)
          : '-'}
      </StudioTableCell>
      <StudioTableCell
        align='end'
        className={cn(
          'tabular-nums',
          (agent.command_failures_1h ?? 0) > 0 &&
            'text-destructive font-medium',
        )}
      >
        {agent.command_failures_1h !== undefined &&
        agent.command_failures_1h !== null
          ? format.number(agent.command_failures_1h)
          : '-'}
      </StudioTableCell>
      <StudioTableCell className='whitespace-nowrap text-muted-foreground'>
        {agent.last_report_at
          ? formatDateTimeMinuteTz(agent.last_report_at)
          : '-'}
      </StudioTableCell>
    </StudioTableRow>
  );
}
