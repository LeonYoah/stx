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
import {useCallback, useEffect, useMemo, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import {Activity} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Switch} from '@/components/ui/switch';
import {cn} from '@/lib/utils';
import {SyncService, type SyncJobInstance, type SyncJobTimeseries} from '@/lib/services/sync';

const RANGES = ['15m', '1h', '6h', '24h', '7d'] as const;
const COLOR_READ = '#3987e5';
const COLOR_WRITE = '#d95926';
const COLOR_COMMITTED = '#199e70';

type RangeKey = (typeof RANGES)[number];

/**
 * 指标趋势视图：双开关空态 + Prometheus 时序图（吞吐 / 条数 / CDC / Checkpoint / 忙碌度 / 背压）。
 * Metrics trend view: two-switch empty states plus Prometheus charts (throughput / rows / CDC / checkpoint / busy / BP).
 */
export function MetricsTrendView({job, active}: {job: SyncJobInstance | null; active: boolean}) {
  const t = useTranslations('workbenchStudio.jobDetail.metricsView');
  const [range, setRange] = useState<RangeKey>('1h');
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [data, setData] = useState<SyncJobTimeseries | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetchedAt, setFetchedAt] = useState<number | null>(null);

  const running = useMemo(() => {
    const status = String(job?.status || '').toLowerCase();
    return status === 'running' || status === 'pending' || status.includes('cancel') || status.includes('savepoint');
  }, [job?.status]);

  const load = useCallback(async (signal?: AbortSignal) => {
    if (!job?.id) return;
    setLoading(true);
    setError(null);
    try {
      const result = await SyncService.getJobTimeseries(job.id, {range, signal});
      setData(result);
      setFetchedAt(Date.now());
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [job?.id, range]);

  useEffect(() => {
    if (!active || !job?.id) return;
    const ac = new AbortController();
    void load(ac.signal);
    return () => ac.abort();
  }, [active, job?.id, load]);

  useEffect(() => {
    if (!active || !autoRefresh || !running || !job?.id) return;
    const timer = window.setInterval(() => {
      void load();
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [active, autoRefresh, running, job?.id, load]);

  if (!job) {
    return <div className='flex h-40 items-center justify-center text-sm text-muted-foreground'>{t('noJob')}</div>;
  }

  if (loading && !data) {
    return <div className='flex h-40 items-center justify-center text-sm text-muted-foreground'>{t('loading')}</div>;
  }

  if (!loading && data && (data.source === 'none' || data.reason === 'observability_disabled' || data.reason === 'prometheus_unavailable')) {
    return (
      <div className='flex min-h-[420px] items-center justify-center rounded-lg border border-border'>
        <div className='max-w-md px-6 text-center'>
          <div className='mx-auto mb-3 flex h-11 w-11 items-center justify-center rounded-lg bg-muted text-muted-foreground'>
            <Activity className='h-5 w-5' />
          </div>
          <div className='text-[15px] font-semibold'>{t('emptyTitle')}</div>
          <p className='mt-2 text-[12.5px] leading-relaxed text-muted-foreground'>{t('emptyBody')}</p>
          <div className='mt-4 flex items-center justify-center gap-3'>
            <Button asChild size='sm'>
              <Link href='/monitoring/integrations'>{t('emptyEnable')}</Link>
            </Button>
            <Link href='/monitoring/integrations' className='text-[12.5px] underline underline-offset-4'>
              {t('emptyHow')}
            </Link>
          </div>
          <p className='mt-5 border-t border-border pt-3 text-xs text-muted-foreground'>{t('emptySnapshotHint')}</p>
        </div>
      </div>
    );
  }

  const seriesByKey = (key: string) => (data?.series ?? []).filter((s) => s.key === key || s.key.startsWith(key));
  const engineObs = Boolean(data?.engine_observability_enabled);

  return (
    <div className='flex flex-col gap-3 pb-2'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='text-xs text-muted-foreground'>{t('range')}</span>
        <div className='inline-flex h-7 items-center rounded-md bg-muted p-0.5 font-mono text-xs'>
          {RANGES.map((r) => (
            <button
              key={r}
              type='button'
              className={cn(
                'rounded-[5px] px-2.5',
                range === r ? 'border border-border bg-background shadow-sm' : 'text-muted-foreground',
              )}
              onClick={() => setRange(r)}
            >
              {r}
            </button>
          ))}
        </div>
        <div className='mx-1 h-4 w-px bg-border' />
        <div className='flex items-center gap-2 text-xs'>
          <Switch checked={autoRefresh} onCheckedChange={setAutoRefresh} />
          <span>{t('autoRefresh')}</span>
        </div>
        <span className='text-xs text-muted-foreground'>
          {fetchedAt ? t('lastRefresh', {time: new Date(fetchedAt).toLocaleTimeString()}) : null}
          {loading ? ` · ${t('loading')}` : null}
        </span>
        <div className='ml-auto flex items-center gap-2 text-xs'>
          <span className='text-muted-foreground'>{t('source')}</span>
          <span className='inline-flex items-center gap-1.5 rounded border border-border px-2 py-0.5'>
            <span className='h-1.5 w-1.5 rounded-full bg-emerald-500' />
            Prometheus
          </span>
          {data?.step_seconds ? (
            <span className='font-mono text-[11px] text-muted-foreground'>step {data.step_seconds}s</span>
          ) : null}
        </div>
      </div>

      {error ? <div className='rounded-md border border-destructive/40 bg-destructive/5 px-3 py-2 text-xs text-destructive'>{error}</div> : null}

      <HeroStrip job={job} data={data} t={t} />

      <div className='grid gap-3 md:grid-cols-2'>
        <TrendCard title={t('charts.throughput')} unit='records/s'>
          <MultiLineChart
            series={[
              {name: t('series.read'), color: COLOR_READ, points: seriesByKey('throughput_read')[0]?.points ?? []},
              {name: t('series.write'), color: COLOR_WRITE, points: seriesByKey('throughput_write')[0]?.points ?? []},
            ]}
            annotations={data?.annotations}
          />
        </TrendCard>
        <TrendCard title={t('charts.rows')} unit={t('unit.rows')}>
          <MultiLineChart
            series={[
              {name: t('series.read'), color: COLOR_READ, points: seriesByKey('rows_read')[0]?.points ?? []},
              {name: t('series.write'), color: COLOR_WRITE, points: seriesByKey('rows_write')[0]?.points ?? []},
              {name: t('series.committed'), color: COLOR_COMMITTED, points: seriesByKey('rows_committed')[0]?.points ?? []},
            ]}
            annotations={data?.annotations}
          />
        </TrendCard>
        <TrendCard title={t('charts.cdc')} unit={t('unit.seconds')}>
          <MultiLineChart
            series={[{name: t('charts.cdc'), color: COLOR_READ, points: seriesByKey('cdc_delay')[0]?.points ?? []}]}
            annotations={data?.annotations}
          />
        </TrendCard>
        <TrendCard title={t('charts.checkpoint')} unit={t('unit.seconds')}>
          <MultiLineChart
            series={[{name: t('charts.checkpoint'), color: COLOR_READ, points: seriesByKey('checkpoint_duration')[0]?.points ?? []}]}
            annotations={data?.annotations}
          />
        </TrendCard>
        <TrendCard title={t('charts.busy')} unit='%'>
          {engineObs ? (
            <MultiLineChart
              series={seriesByKey('busy').map((s, i) => ({
                name: s.label || s.key,
                color: [COLOR_READ, COLOR_COMMITTED, COLOR_WRITE][i % 3],
                points: s.points,
              }))}
              annotations={data?.annotations}
              yMax={1}
              percent
            />
          ) : (
            <EngineObsEmpty t={t} />
          )}
        </TrendCard>
        <TrendCard title={t('charts.backpressure')} unit='%'>
          {engineObs ? (
            <MultiLineChart
              series={seriesByKey('backpressure').map((s, i) => ({
                name: s.label || s.key,
                color: [COLOR_READ, COLOR_WRITE][i % 2],
                points: s.points,
              }))}
              annotations={data?.annotations}
              yMax={1}
              percent
            />
          ) : (
            <EngineObsEmpty t={t} />
          )}
        </TrendCard>
      </div>

      <div className='rounded-lg border border-dashed border-border px-3 py-6 text-center text-xs text-muted-foreground'>
        {t('perTablePhase3')}
      </div>
    </div>
  );
}

function EngineObsEmpty({t}: {t: ReturnType<typeof useTranslations>}) {
  return (
    <div className='flex h-[200px] flex-col items-center justify-center gap-1 rounded-md border border-dashed border-border bg-[repeating-linear-gradient(135deg,transparent_0_10px,rgba(127,127,127,0.04)_10px_20px)] text-muted-foreground'>
      <div className='text-[12.5px] text-foreground'>{t('engineObsEmptyTitle')}</div>
      <div className='text-[11.5px]'>{t('engineObsEmptyBody')}</div>
    </div>
  );
}

function TrendCard({title, unit, children}: {title: string; unit?: string; children: ReactNode}) {
  return (
    <div className='rounded-lg border border-border bg-card p-3'>
      <div className='mb-1 flex h-5 items-center gap-2'>
        <span className='text-[13px] font-semibold'>{title}</span>
        {unit ? <span className='text-[11.5px] text-muted-foreground'>{unit}</span> : null}
      </div>
      {children}
    </div>
  );
}

function HeroStrip({
  job,
  data,
  t,
}: {
  job: SyncJobInstance;
  data: SyncJobTimeseries | null;
  t: ReturnType<typeof useTranslations>;
}) {
  const metrics = (job.result_preview?.metrics || {}) as Record<string, unknown>;
  const readQps = Number(metrics.SourceReceivedQPS ?? 0);
  const writeQps = Number(metrics.SinkWriteQPS ?? 0);
  const lag = Math.max(0, readQps - writeQps);
  const cdc = job.cdc_delay?.latest_ms != null ? job.cdc_delay.latest_ms / 1000 : null;
  const items = [
    {k: t('hero.readQps'), v: fmtNum(readQps), u: 'rec/s'},
    {k: t('hero.writeQps'), v: fmtNum(writeQps), u: 'rec/s'},
    {k: t('hero.lag'), v: fmtNum(lag), u: 'rec/s'},
    {k: t('hero.cdc'), v: cdc == null ? '—' : cdc.toFixed(1), u: 's'},
    {k: t('hero.checkpoint'), v: '—', u: 's'},
  ];
  return (
    <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
      {items.map((item) => (
        <div key={item.k} className='rounded-lg border border-border bg-card px-3.5 py-3'>
          <div className='text-xs text-muted-foreground'>{item.k}</div>
          <div className='mt-1 font-mono text-[26px] font-semibold leading-none tracking-tight'>
            {item.v}
            <small className='ml-1 text-xs font-normal text-muted-foreground'>{item.u}</small>
          </div>
          {data?.source === 'prometheus' ? (
            <div className='mt-1 text-[11.5px] text-muted-foreground'>{t('heroFromSnapshot')}</div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function MultiLineChart({
  series,
  annotations,
  yMax,
  percent,
}: {
  series: {name: string; color: string; points: [number, number][]}[];
  annotations?: SyncJobTimeseries['annotations'];
  yMax?: number;
  percent?: boolean;
}) {
  const data = useMemo(() => {
    const tsSet = new Set<number>();
    series.forEach((s) => s.points.forEach(([ts]) => tsSet.add(ts)));
    const timestamps = Array.from(tsSet).sort((a, b) => a - b);
    return timestamps.map((ts) => {
      const row: Record<string, number | string> = {
        ts,
        label: new Date(ts).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'}),
      };
      series.forEach((s) => {
        const hit = s.points.find((p) => p[0] === ts);
        row[s.name] = hit ? hit[1] : undefined as unknown as number;
      });
      return row;
    });
  }, [series]);

  if (!series.some((s) => s.points.length > 0)) {
    return (
      <div className='flex h-[200px] items-center justify-center text-xs text-muted-foreground'>—</div>
    );
  }

  return (
    <div className='h-[200px] w-full'>
      <ResponsiveContainer width='100%' height='100%'>
        <LineChart data={data} margin={{top: 8, right: 8, left: 0, bottom: 0}}>
          <CartesianGrid strokeDasharray='3 3' className='stroke-border/60' />
          <XAxis dataKey='label' tick={{fontSize: 10}} minTickGap={24} />
          <YAxis
            tick={{fontSize: 10}}
            domain={yMax != null ? [0, yMax] : ['auto', 'auto']}
            tickFormatter={(v) => (percent ? `${Math.round(Number(v) * 100)}%` : String(v))}
            width={48}
          />
          <Tooltip
            contentStyle={{fontSize: 12}}
            formatter={(value: number, name: string) => [
              percent ? `${(Number(value) * 100).toFixed(1)}%` : Number(value).toLocaleString(),
              name,
            ]}
          />
          <Legend wrapperStyle={{fontSize: 11}} />
          {(annotations ?? []).map((ann) => (
            <ReferenceLine
              key={`${ann.ts_ms}-${ann.kind}`}
              x={new Date(ann.ts_ms).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'})}
              stroke='currentColor'
              strokeDasharray='3 3'
              className='text-muted-foreground/50'
              label={{value: ann.kind === 'schema_change' ? 'DDL' : ann.kind === 'restart' ? '重启' : ann.kind, position: 'top', fontSize: 9}}
            />
          ))}
          {series.map((s) => (
            <Line
              key={s.name}
              type='monotone'
              dataKey={s.name}
              stroke={s.color}
              strokeWidth={2}
              dot={false}
              connectNulls
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function fmtNum(n: number) {
  if (!Number.isFinite(n)) return '—';
  return Math.round(n).toLocaleString();
}
