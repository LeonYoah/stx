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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  AlertTriangle,
  Cpu,
  Database,
  HardDrive,
  RefreshCw,
  Server,
} from 'lucide-react';
import gsap from 'gsap';
import {toast} from 'sonner';

import services from '@/lib/services';
import type {PlatformSnapshotData} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {
  formatBytes,
  formatCompactNumber,
  formatDurationSeconds,
} from '@/components/common/dashboard/dashboard-format';
import {TableLoadingBar, TableSkeletonRows} from '@/components/common/layout';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Card} from '@/components/ui/card';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {animateSheetSections, animateTableRows} from '@/lib/animations/gsap-motion';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import {toLineChartData} from '@/components/common/sync/job-detail/metrics-trend-data';
import {formatDateTimeMinuteTz} from '@/lib/timezone';

// 平台快照自动刷新间隔：与后端采集周期（15s）错开一档，避免高频请求。
// Snapshot auto-refresh interval: one tier above the 15s backend scrape cadence.
const PLATFORM_REFRESH_MS = 30_000;

type RefreshMode = 'initial' | 'silent';

// 单条趋势的展示配置；value 由调用方传入 formatter。
// Display config for one trend; the caller supplies the value formatter.
interface TrendSpec {
  key: string;
  title: string;
  points: [number, number][] | undefined;
  format: (value: number) => string;
}

/**
 * 平台自监控面板：Server 进程指标、控制面（HTTP/gRPC/DB/TSDB）统计、
 * 轻量趋势与 Agent 进程表格。数据来自 GET /api/v1/monitoring/platform。
 * Platform self-monitoring panel: server process metrics, control-plane
 * (HTTP/gRPC/DB/TSDB) stats, lightweight trends and the Agent table.
 * Data comes from GET /api/v1/monitoring/platform.
 */
export function MonitoringPlatformPanel() {
  const t = useTranslations('monitoringCenter');
  const panelRef = useRef<HTMLDivElement>(null);

  const [snapshot, setSnapshot] = useState<PlatformSnapshotData | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);

  const durationUnits = useMemo(
    () => ({d: t('platform.units.d'), h: t('platform.units.h'), m: t('platform.units.m')}),
    [t],
  );

  const fetchSnapshot = useCallback(
    async (mode: RefreshMode) => {
      if (mode === 'initial') {
        setLoading(true);
      } else {
        setRefreshing(true);
      }
      const result = await services.monitoring.getPlatformSnapshotSafe();
      if (result.success && result.data) {
        setSnapshot(result.data);
      } else {
        toast.error(result.error || t('platform.loadError'));
      }
      setLoading(false);
      setRefreshing(false);
    },
    [t],
  );

  useEffect(() => {
    void fetchSnapshot('initial');
    const timer = window.setInterval(
      () => void fetchSnapshot('silent'),
      PLATFORM_REFRESH_MS,
    );
    return () => window.clearInterval(timer);
  }, [fetchSnapshot]);

  // 加载完成后的入场动效；gsap.context 以面板为作用域，辅助函数内部已遵循
  // prefers-reduced-motion。卸载时 revert 清理补间。
  // Entrance animation once loaded; gsap.context scopes the selectors to the
  // panel and the helpers already honor prefers-reduced-motion. Revert on unmount.
  useEffect(() => {
    if (loading || !panelRef.current) {
      return;
    }
    const ctx = gsap.context(() => {
      animateSheetSections('.platform-section-animate');
      animateTableRows('.platform-agent-row-animate');
    }, panelRef.current);
    return () => ctx.revert();
  }, [loading, snapshot]);

  const server = snapshot?.server;
  const agents = snapshot?.agents ?? [];

  const trends = useMemo<TrendSpec[]>(
    () => [
      {
        key: 'rss',
        title: t('platform.trendRss'),
        points: snapshot?.trends?.rss_bytes,
        format: formatBytes,
      },
      {
        key: 'heap',
        title: t('platform.trendHeap'),
        points: snapshot?.trends?.heap_inuse_bytes,
        format: formatBytes,
      },
      {
        key: 'rps',
        title: t('platform.trendHttpRps'),
        points: snapshot?.trends?.http_rps,
        format: (v) => `${formatCompactNumber(v)}/s`,
      },
      {
        key: '5xx',
        title: t('platform.trendHttp5xx'),
        points: snapshot?.trends?.http_5xx_ratio,
        format: (v) => `${(v * 100).toFixed(2)}%`,
      },
    ],
    [snapshot, t],
  );

  // 首次加载零塌陷：用定高骨架占位，而不是空容器里塞大 Spinner。
  // Zero-collapse first paint: fixed-height skeleton blocks instead of a big
  // spinner inside an empty container.
  if (loading) {
    return (
      <div ref={panelRef} className='space-y-3.5'>
        <Card className='h-36 animate-pulse bg-muted/30' />
        <div className='grid gap-3.5 sm:grid-cols-2 xl:grid-cols-4'>
          {Array.from({length: 4}).map((_, i) => (
            <Card key={i} className='h-40 animate-pulse bg-muted/30' />
          ))}
        </div>
        <Card className='p-3'>
          <Table>
            <TableBody>
              <TableSkeletonRows columns={10} rows={3} />
            </TableBody>
          </Table>
        </Card>
      </div>
    );
  }

  // 内嵌观测不可用/未就绪时的降级卡：说明原因，并提醒自监控的边界。
  // Degraded card when embedded observability is off or not ready: explain the
  // reason and remind users of the self-monitoring boundary.
  if (!snapshot || !snapshot.available) {
    const reason = snapshot?.reason || '';
    return (
      <div ref={panelRef}>
        <Card className='p-8 flex flex-col items-center gap-3 text-center'>
          <AlertTriangle className='size-8 text-muted-foreground/70' />
          <p className='text-sm font-medium'>
            {reason === 'observability_disabled'
              ? t('platform.unavailableDisabled')
              : t('platform.unavailableNotReady')}
          </p>
          <p className='text-xs text-muted-foreground max-w-md'>
            {t('platform.externalProbeNote')}
          </p>
          <Button
            variant='outline'
            size='sm'
            className='gap-1.5 active:scale-[0.98]'
            disabled={refreshing}
            onClick={() => void fetchSnapshot('silent')}
          >
            <RefreshCw className={cn('size-3.5', refreshing && 'animate-spin')} />
            {t('refresh')}
          </Button>
        </Card>
      </div>
    );
  }

  const p95Ms = (v?: number) =>
    v === undefined ? '—' : `${(v * 1000).toFixed(0)}ms`;

  return (
    <div ref={panelRef} className='space-y-3.5'>
      {/* 刷新指示：静默刷新只给顶置进度条，不重排内容 / Hairline progress for silent refreshes */}
      <TableLoadingBar loading={refreshing} />

      {/* Server 进程与控制面概览 / Server process & control-plane overview */}
      <Card className='platform-section-animate p-4 space-y-3'>
        <div className='flex items-center justify-between gap-2 flex-wrap'>
          <div className='flex items-center gap-2 text-sm font-medium'>
            <Server className='size-4 text-muted-foreground' />
            {t('platform.serverTitle')}
            {server?.version ? (
              <Badge variant='secondary' className='font-mono text-[11px]'>
                v{server.version}
              </Badge>
            ) : null}
          </div>
          <Button
            variant='ghost'
            size='sm'
            className='gap-1.5 h-7 text-xs active:scale-[0.98]'
            disabled={refreshing}
            onClick={() => void fetchSnapshot('silent')}
          >
            <RefreshCw
              className={cn('size-3.5', refreshing && 'animate-spin')}
            />
            {t('refresh')}
          </Button>
        </div>

        {/* 原子指标网格：每项不可拆分的键值单元 / Atomic stat grid */}
        <div className='grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-3 xl:grid-cols-6'>
          <StatItem label={t('platform.uptime')} value={server?.uptime_seconds !== undefined ? formatDurationSeconds(server.uptime_seconds, durationUnits) : undefined} />
          <StatItem label={t('platform.rss')} value={server?.rss_bytes !== undefined ? formatBytes(server.rss_bytes) : undefined} />
          <StatItem label={t('platform.heap')} value={server?.heap_inuse_bytes !== undefined ? formatBytes(server.heap_inuse_bytes) : undefined} />
          <StatItem label={t('platform.goroutines')} value={server?.goroutines} />
          <StatItem label={t('platform.agentsConnected')} value={server?.agents_connected} />
          <StatItem
            label={t('platform.dbPool')}
            value={
              server
                ? `${formatCompactNumber(server.db_in_use)}/${formatCompactNumber(server.db_open)}`
                : undefined
            }
            hint={t('platform.dbPoolHint')}
          />
        </div>

        {/* 控制面请求与存储统计 / Control-plane request & storage stats */}
        <div className='grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-3 xl:grid-cols-6 border-t border-border/60 pt-2.5'>
          <StatItem icon={<Cpu className='size-3' />} label={t('platform.httpRps')} value={server ? `${formatCompactNumber(server.http_rps)}/s` : undefined} />
          <StatItem
            label={t('platform.http5xxRatio')}
            value={server ? `${((server.http_5xx_ratio ?? 0) * 100).toFixed(2)}%` : undefined}
            tone={(server?.http_5xx_ratio ?? 0) > 0 ? 'danger' : 'normal'}
          />
          <StatItem label={t('platform.httpP95')} value={server ? p95Ms(server.http_p95_seconds) : undefined} />
          <StatItem label={t('platform.dbWaitRate')} value={server ? `${formatCompactNumber(server.db_wait_count_rate)}/s` : undefined} />
          <StatItem icon={<Database className='size-3' />} label={t('platform.tsdbHeadSeries')} value={server?.tsdb?.head_series} />
          <StatItem
            icon={<HardDrive className='size-3' />}
            label={t('platform.tsdbBlockBytes')}
            value={server?.tsdb?.blocks_bytes !== undefined ? formatBytes(server.tsdb.blocks_bytes) : undefined}
          />
          <StatItem
            label={t('platform.tsdbCompactionFailures')}
            value={server?.tsdb?.compactions_failed}
            tone={(server?.tsdb?.compactions_failed ?? 0) > 0 ? 'danger' : 'normal'}
          />
        </div>
      </Card>

      {/* 轻量趋势（近 1h） / Lightweight trends (last hour) */}
      <div className='grid gap-3.5 sm:grid-cols-2 xl:grid-cols-4'>
        {trends.map((spec) => (
          <PlatformTrendChart key={spec.key} spec={spec} />
        ))}
      </div>

      {/* Agent 进程列表 / Agent process list */}
      <Card className='platform-section-animate p-0 overflow-hidden'>
        <div className='px-4 pt-3 pb-2 text-sm font-medium'>
          {t('platform.agentsTitle')}
        </div>
        <div className='overflow-x-auto'>
          <Table>
            <TableHeader>
              <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentHostname')}</TableHead>
                <TableHead className='w-[80px] py-1.5 px-3 text-xs'>{t('platform.agentStatus')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentVersion')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentUptime')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentRss')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentCpu')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentGoroutines')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs'>{t('platform.agentCmdFailures')}</TableHead>
                <TableHead className='py-1.5 px-3 text-xs whitespace-nowrap'>{t('platform.agentLastReport')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {agents.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} className='py-8 text-center text-xs text-muted-foreground'>
                    {t('platform.noAgents')}
                  </TableCell>
                </TableRow>
              ) : (
                agents.map((agent) => {
                  // 版本不一致由前端比对快照自带 server 版本得出（Agent 未上报版本时不判）。
                  // Version mismatch is derived by comparing the snapshot's server
                  // version (skipped while the agent reports none).
                  const mismatch =
                    Boolean(agent.version) &&
                    Boolean(server?.version) &&
                    agent.version !== server?.version;
                  return (
                    <TableRow
                      key={`${agent.host_id ?? 'unknown'}-${agent.host_name ?? ''}`}
                      className='platform-agent-row-animate h-9'
                    >
                      <TableCell className='py-1.5 px-3'>
                        <span className='text-xs font-medium truncate max-w-[220px] inline-block align-middle'>
                          {agent.host_name || `host-${agent.host_id ?? '?'}`}
                        </span>
                      </TableCell>
                      <TableCell className='py-1.5 px-3'>
                        <Badge
                          variant={agent.online ? 'default' : 'secondary'}
                          className={cn(
                            'text-[11px]',
                            agent.online && 'bg-emerald-600/90 hover:bg-emerald-600/90 text-white dark:bg-emerald-500/80',
                          )}
                        >
                          {agent.online
                            ? t('platform.online')
                            : t('platform.offline')}
                        </Badge>
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-xs font-mono whitespace-nowrap'>
                        <span className='inline-flex items-center gap-1.5'>
                          {agent.version || '—'}
                          {mismatch ? (
                            <Badge variant='destructive' className='text-[10px] px-1 py-0'>
                              {t('platform.versionMismatch')}
                            </Badge>
                          ) : null}
                        </span>
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-xs whitespace-nowrap'>
                        {agent.uptime_seconds !== undefined && agent.uptime_seconds !== null
                          ? formatDurationSeconds(agent.uptime_seconds, durationUnits)
                          : '—'}
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-xs whitespace-nowrap'>
                        {agent.rss_bytes !== undefined && agent.rss_bytes !== null
                          ? formatBytes(agent.rss_bytes)
                          : '—'}
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-xs whitespace-nowrap'>
                        {agent.cpu_ratio !== undefined && agent.cpu_ratio !== null
                          ? `${(agent.cpu_ratio * 100).toFixed(1)}%`
                          : '—'}
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-xs'>
                        {agent.goroutines ?? '—'}
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-xs'>
                        {agent.command_failures_1h !== undefined &&
                        agent.command_failures_1h !== null ? (
                          <span className={cn(agent.command_failures_1h > 0 && 'text-destructive font-medium')}>
                            {formatCompactNumber(agent.command_failures_1h)}
                          </span>
                        ) : (
                          '—'
                        )}
                      </TableCell>
                      <TableCell className='py-1.5 px-3 text-[11px] text-muted-foreground whitespace-nowrap'>
                        {agent.last_report_at
                          ? formatDateTimeMinuteTz(agent.last_report_at)
                          : '—'}
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </div>
      </Card>
    </div>
  );
}

// 原子指标：标签 + 不可拆分值；undefined 兜底为 em dash。
// Atomic stat: label + indivisible value; undefined falls back to an em dash.
function StatItem({
  label,
  value,
  hint,
  icon,
  tone = 'normal',
}: {
  label: string;
  value?: number | string;
  hint?: string;
  icon?: React.ReactNode;
  tone?: 'normal' | 'danger';
}) {
  const display =
    value === undefined || value === null
      ? '—'
      : typeof value === 'number'
        ? formatCompactNumber(value)
        : value;
  return (
    <div className='min-w-0' title={hint}>
      <div className='flex items-center gap-1 text-[11px] text-muted-foreground whitespace-nowrap'>
        {icon}
        {label}
      </div>
      <div
        className={cn(
          'text-sm font-semibold tabular-nums whitespace-nowrap',
          tone === 'danger' && 'text-destructive',
        )}
      >
        {display}
      </div>
    </div>
  );
}

// 单卡轻量趋势图：复用 Arc LineChart；空序列由空态文案占位而非塌陷。
// Single lightweight trend card: reuses the Arc LineChart; an empty series
// shows a placeholder instead of collapsing.
function PlatformTrendChart({spec}: {spec: TrendSpec}) {
  const t = useTranslations('monitoringCenter');
  const points = spec.points ?? [];
  const chart = useMemo(
    () =>
      toLineChartData(
        [{key: spec.key, name: spec.title, points}],
        formatDateTimeMinuteTz,
        (ts) => formatDateTimeMinuteTz(ts).slice(-5),
      ),
    [spec.key, spec.title, spec.points],
  );
  const latest = points.length > 0 ? points[points.length - 1][1] : undefined;

  return (
    <Card className='platform-section-animate p-3 space-y-1'>
      <div className='flex items-baseline justify-between gap-2'>
        <span className='text-xs text-muted-foreground'>{spec.title}</span>
        <span className='text-sm font-semibold tabular-nums whitespace-nowrap'>
          {latest !== undefined ? spec.format(latest) : '—'}
        </span>
      </div>
      {points.length > 0 ? (
        <LineChart
          label={spec.title}
          data={chart.data}
          series={chart.series}
          height={96}
          legend={false}
          emptyLabel={t('platform.trendEmpty')}
          categoryLabel={t('platform.trendTime')}
          formatValue={(value) => spec.format(value)}
        />
      ) : (
        <div className='h-24 flex items-center justify-center text-[11px] text-muted-foreground'>
          {t('platform.trendEmpty')}
        </div>
      )}
    </Card>
  );
}
