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
import {useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {Activity} from 'lucide-react';
import {BarChart} from '@/components/arc/bar-chart/bar-chart';
import {BrushChart} from '@/components/arc/brush-chart/brush-chart';
import {DonutChart} from '@/components/arc/donut-chart/donut-chart';
import {LineChart} from '@/components/arc/line-chart/line-chart';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Switch} from '@/components/arc/switch/switch';
import type {DateRange} from '@/components/arc/date-range-picker/date-range-picker';
import {ArcDateRangePicker} from '@/components/common/layout/ArcDateRangePicker';
import {Button} from '@/components/ui/button';
import {useLocale} from '@/lib/i18n';
import {cn} from '@/lib/utils';
import {SyncService, type SyncJobInstance, type SyncJobTimeseries} from '@/lib/services/sync';
import {
  TREND_COLORS,
  brushMinSpan,
  MAX_TREND_WINDOW_MS,
  colorByName,
  colorByTable,
  jobLifetimeWindow,
  latestValue,
  scalePoints,
  toBarChartData,
  toBrushAnnotations,
  toBrushChartData,
  toLineChartData,
  toPipelineShare,
  type NamedSeries,
  type TrendWindow,
} from './metrics-trend-data';

const RANGES = ['15m', '1h', '6h', '24h', '7d'] as const;
const LIFETIME = 'lifetime';

type RangeKey = (typeof RANGES)[number];

/**
 * 趋势时间窗：相对当前的预设、作业全程，或日历选出的自定义起止（毫秒）。
 * Trend window: a preset relative to now, the job's lifetime, or a custom start/end (ms) picked on the calendar.
 */
type TrendSelection =
  | {kind: 'preset'; range: RangeKey}
  | {kind: 'lifetime'}
  | {kind: 'custom'; window: TrendWindow};

const isRunningStatus = (status?: string) => {
  const value = String(status || '').toLowerCase();
  return value === 'running' || value === 'pending' || value.includes('cancel') || value.includes('savepoint');
};

/**
 * 默认时间窗：已结束的作业看全程（相对当前的预设一过就看不到它了），运行中的看最近 1 小时。
 * Default window: finished jobs show their lifetime (a now-relative preset soon scrolls past them); running jobs show the
 * last hour.
 */
const defaultSelection = (job: SyncJobInstance | null): TrendSelection =>
  job && !isRunningStatus(job.status) && jobLifetimeWindow(job, false, Date.now())
    ? {kind: 'lifetime'}
    : {kind: 'preset', range: '1h'};

/**
 * 指标趋势视图：Arc 折线 / 刷选 / 柱状 / 环形图 + Prometheus 时序。
 * Metrics trend view: Arc line / brush / bar / donut charts over Prometheus series.
 */
export function MetricsTrendView({job, active}: {job: SyncJobInstance | null; active: boolean}) {
  const t = useTranslations('workbenchStudio.jobDetail.metricsView');
  const {locale} = useLocale();
  const intlLocale = locale === 'zh' ? 'zh-CN' : 'en-US';
  const [selection, setSelection] = useState<TrendSelection>(() => defaultSelection(job));
  const [selectionJobId, setSelectionJobId] = useState(job?.id);
  // 换作业时回到该作业的默认时间窗（渲染期重置，避免先用旧窗口发一次请求）。
  // Switching jobs resets to that job's default window (reset during render so the old window is never queried).
  if (selectionJobId !== job?.id) {
    setSelectionJobId(job?.id);
    setSelection(defaultSelection(job));
  }
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [data, setData] = useState<SyncJobTimeseries | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fetchedAt, setFetchedAt] = useState<number | null>(null);

  const timeFormat = useMemo(
    () => new Intl.DateTimeFormat(intlLocale, {hour: '2-digit', minute: '2-digit'}),
    [intlLocale],
  );
  const dateTimeFormat = useMemo(
    () => new Intl.DateTimeFormat(intlLocale, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }),
    [intlLocale],
  );
  const formatTime = useCallback((ts: number) => timeFormat.format(new Date(ts)), [timeFormat]);
  const formatDateTime = useCallback(
    (ts: number) => dateTimeFormat.format(new Date(ts)),
    [dateTimeFormat],
  );

  const running = useMemo(() => isRunningStatus(job?.status), [job?.status]);
  const canLifetime = useMemo(() => jobLifetimeWindow(job, running, Date.now()) != null, [job, running]);

  const load = useCallback(async (signal?: AbortSignal) => {
    if (!job?.id) return;
    let query: {range?: string; start?: number; end?: number};
    if (selection.kind === 'preset') {
      query = {range: selection.range};
    } else if (selection.kind === 'custom') {
      query = {start: selection.window.start, end: selection.window.end};
    } else {
      const lifetime = jobLifetimeWindow(job, running, Date.now());
      if (!lifetime) return;
      // 运行中不传 end，让后端取当前时间，自动刷新时窗口跟着往后长。
      // While running, omit end so the backend uses now and the window grows with each auto refresh.
      query = running ? {start: lifetime.start} : lifetime;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await SyncService.getJobTimeseries(job.id, {...query, signal});
      setData(result);
      setFetchedAt(Date.now());
    } catch (err) {
      // 切时间窗会 abort 上一请求，不要把 Canceled 当成可见错误。
      // Switching the range aborts the previous request; do not surface that as an error.
      if (signal?.aborted) {
        return;
      }
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (!signal?.aborted) {
        setLoading(false);
      }
    }
  }, [job, running, selection]);

  useEffect(() => {
    if (!active || !job?.id) return;
    const ac = new AbortController();
    void load(ac.signal);
    return () => ac.abort();
  }, [active, job?.id, load]);

  useEffect(() => {
    // 自定义起止是固定窗口，刷新也不会有新数据。 / A custom window is fixed, so refreshing would add nothing.
    if (!active || !autoRefresh || !running || !job?.id || selection.kind === 'custom') return;
    const timer = window.setInterval(() => {
      void load();
    }, 30_000);
    return () => window.clearInterval(timer);
  }, [active, autoRefresh, running, job?.id, load, selection.kind]);

  // 选择器回显：自定义显示所选起止，作业全程显示后端实际查询的窗口，预设显示占位文案。
  // Picker echo: custom shows the chosen bounds, lifetime the window the backend actually queried, presets a placeholder.
  const pickerValue = useMemo<DateRange | null>(() => {
    if (selection.kind === 'custom') {
      return {start: new Date(selection.window.start), end: new Date(selection.window.end)};
    }
    if (selection.kind === 'lifetime' && data?.range === 'custom' && data.start_ms > 0 && data.end_ms > data.start_ms) {
      return {start: new Date(data.start_ms), end: new Date(data.end_ms)};
    }
    return null;
  }, [data?.end_ms, data?.range, data?.start_ms, selection]);
  const pickerMinDate = useMemo(() => {
    const lifetime = jobLifetimeWindow(job, running, Date.now());
    return lifetime ? new Date(lifetime.start) : undefined;
  }, [job, running]);
  const handlePickRange = useCallback((picked: DateRange) => {
    const end = picked.end.getTime();
    // 与后端同样限制 31 天，超出时保留靠近结束的一段。 / Cap at 31 days like the backend, keeping the part nearest the end.
    setSelection({kind: 'custom', window: {start: Math.max(picked.start.getTime(), end - MAX_TREND_WINDOW_MS), end}});
  }, []);
  const segmentValue = selection.kind === 'preset' ? selection.range : selection.kind === 'lifetime' ? LIFETIME : '';

  const seriesByKey = useCallback(
    (key: string) => (data?.series ?? []).filter((item) => item.key === key || item.key.startsWith(key)),
    [data?.series],
  );

  const annotationLabel = useCallback(
    (kind: string) => {
      if (t.has(`annotations.${kind}`)) {
        return t(`annotations.${kind}`);
      }
      return kind;
    },
    [t],
  );

  const charts = useMemo(() => {
    // 忙碌度与背压按算子名共用一套颜色，同一算子在两张图里同色。
    // Busy and backpressure share one name-to-color map so a vertex keeps its color in both charts.
    const vertexColors = colorByName(
      [...seriesByKey('busy'), ...seriesByKey('backpressure')].map((item) => item.label || item.key),
    );
    const named = (key: string, name: string, scale = 1): NamedSeries[] =>
      seriesByKey(key).map((item, index) => ({
        key: `${item.key}-${index}`,
        name: item.label || name,
        points: scale === 1 ? item.points : scalePoints(item.points, scale),
        color: vertexColors.get(item.label || item.key),
      }));

    const throughput = toLineChartData(
      [
        {key: 'read', name: t('series.read'), points: seriesByKey('throughput_read')[0]?.points ?? [], color: TREND_COLORS.read},
        {key: 'write', name: t('series.write'), points: seriesByKey('throughput_write')[0]?.points ?? [], color: TREND_COLORS.write},
      ],
      formatDateTime,
      formatTime,
    );
    const rows = toLineChartData(
      [
        {key: 'read', name: t('series.read'), points: seriesByKey('rows_read')[0]?.points ?? [], color: TREND_COLORS.read},
        {key: 'write', name: t('series.write'), points: seriesByKey('rows_write')[0]?.points ?? [], color: TREND_COLORS.write},
        {
          key: 'committed',
          name: t('series.committed'),
          points: seriesByKey('rows_committed')[0]?.points ?? [],
          color: TREND_COLORS.committed,
        },
      ],
      formatDateTime,
      formatTime,
    );
    const cdcPoints = seriesByKey('cdc_delay')[0]?.points ?? [];
    const checkpointPoints = seriesByKey('checkpoint_duration')[0]?.points ?? [];
    // 表级序列：表名在两图间固定同色，读/写用名字后缀区分；后端聚合桶 `other` 显示为「其他」。
    // Per-table series: a table keeps one color in both charts, direction shows as a name suffix;
    // the backend overflow bucket `other` renders as the localized label.
    const tableNames = ['table_throughput_read', 'table_throughput_write', 'table_rows_read', 'table_rows_write']
      .flatMap((key) => seriesByKey(key).map((item) => item.label || item.key));
    const tableColors = colorByTable(tableNames);
    const tableNamed = (key: string, directionLabel: string): NamedSeries[] =>
      seriesByKey(key).map((item) => {
        const raw = item.label || item.key;
        const table = raw === 'other' ? t('otherLabel') : raw;
        return {
          key: `${key}|${raw}`,
          name: `${table}·${directionLabel}`,
          points: item.points,
          color: tableColors.get(raw),
        };
      });
    const tableThroughput = toLineChartData(
      [
        ...tableNamed('table_throughput_read', t('series.read')),
        ...tableNamed('table_throughput_write', t('series.write')),
      ],
      formatDateTime,
      formatTime,
    );
    const tableRows = toLineChartData(
      [
        ...tableNamed('table_rows_read', t('series.read')),
        ...tableNamed('table_rows_write', t('series.write')),
      ],
      formatDateTime,
      formatTime,
    );
    const busySeries = named('busy', t('charts.busy'), 100);
    const backpressureSeries = named('backpressure', t('charts.backpressure'), 100);
    const busy = toLineChartData(
      busySeries.length > 0 ? busySeries : [{key: 'busy', name: t('charts.busy'), points: []}],
      formatDateTime,
      formatTime,
    );
    const backpressure = toLineChartData(
      backpressureSeries.length > 0 ? backpressureSeries : [{key: 'backpressure', name: t('charts.backpressure'), points: []}],
      formatDateTime,
      formatTime,
    );
    const pipeline = toPipelineShare(
      latestValue(seriesByKey('rows_read')[0]?.points ?? []),
      latestValue(seriesByKey('rows_write')[0]?.points ?? []),
      latestValue(seriesByKey('rows_committed')[0]?.points ?? []),
      {
        committed: t('pipeline.committed'),
        written: t('pipeline.written'),
        backlog: t('pipeline.backlog'),
      },
    );
    const barPeriod = checkpointPoints.length >= 2
      ? t('periodSpan', {
        from: formatDateTime(checkpointPoints[0][0]),
        to: formatDateTime(checkpointPoints[checkpointPoints.length - 1][0]),
      })
      : t('periodCurrent');

    return {
      throughput,
      rows,
      tableThroughput,
      tableRows,
      hasTableSeries: tableNames.length > 0,
      cdc: toBrushChartData(cdcPoints),
      cdcMinSpan: brushMinSpan(cdcPoints, data?.step_seconds ?? 0),
      checkpoint: toBarChartData(checkpointPoints, formatDateTime, formatTime),
      barPeriod,
      busy,
      backpressure,
      pipeline,
      annotations: toBrushAnnotations(data?.annotations, annotationLabel),
    };
  }, [annotationLabel, data?.annotations, data?.step_seconds, formatDateTime, formatTime, seriesByKey, t]);

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
          <div className='text-[15px] font-medium'>{t('emptyTitle')}</div>
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

  const engineObs = Boolean(data?.engine_observability_enabled);
  const formatCount = (value: number) => Math.round(value).toLocaleString(intlLocale);
  const formatRate = (value: number) => value.toLocaleString(intlLocale, {maximumFractionDigits: 1});
  const formatSeconds = (value: number) => `${value.toLocaleString(intlLocale, {maximumFractionDigits: 2})}`;

  return (
    <div className='flex flex-col gap-4 pb-2'>
      <div className='flex flex-wrap items-center gap-3'>
        <SegmentedControl
          label={t('range')}
          value={segmentValue}
          onValueChange={(value) =>
            setSelection(value === LIFETIME ? {kind: 'lifetime'} : {kind: 'preset', range: value as RangeKey})}
          options={[
            ...RANGES.map((item) => ({value: item, label: item})),
            ...(canLifetime ? [{value: LIFETIME, label: t('rangeLifetime')}] : []),
          ]}
        />
        <ArcDateRangePicker
          value={pickerValue}
          onChange={handlePickRange}
          label={t('customRange')}
          placeholder={t('customRange')}
          minDate={pickerMinDate}
          withTime
        />
        <Switch
          checked={autoRefresh}
          onCheckedChange={setAutoRefresh}
          label={t('autoRefresh')}
        />
        <span className='text-xs text-muted-foreground'>
          {fetchedAt ? t('lastRefresh', {time: timeFormat.format(new Date(fetchedAt))}) : null}
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

      <div className='grid gap-4 md:grid-cols-2'>
        <TrendSection title={t('charts.throughput')} unit='records/s'>
          <LineChart
            label={t('charts.throughput')}
            unit='records/s'
            data={charts.throughput.data}
            series={charts.throughput.series}
            height={200}
            loading={loading}
            emptyLabel={t('emptyChart')}
            resetLabel={t('legendClear')}
            categoryLabel={t('categoryTime')}
            formatValue={formatRate}
          />
        </TrendSection>
        <TrendSection title={t('charts.rows')} unit={t('unit.rows')}>
          <LineChart
            label={t('charts.rows')}
            unit={t('unit.rows')}
            data={charts.rows.data}
            series={charts.rows.series}
            height={200}
            loading={loading}
            emptyLabel={t('emptyChart')}
            resetLabel={t('legendClear')}
            categoryLabel={t('categoryTime')}
            formatValue={formatCount}
          />
        </TrendSection>
        {/* 表级分解紧跟总览两张图；作业没有表级指标时整块不渲染（如 FakeSource 单流）。 / The per-table
            breakdown follows the totals; it stays hidden for jobs without per-table metrics (e.g. a single FakeSource stream). */}
        {charts.hasTableSeries ? (
          <TrendSection title={t('charts.tableThroughput')} unit='records/s'>
            <LineChart
              label={t('charts.tableThroughput')}
              unit='records/s'
              data={charts.tableThroughput.data}
              series={charts.tableThroughput.series}
              height={200}
              loading={loading}
              emptyLabel={t('emptyChart')}
              resetLabel={t('legendClear')}
              categoryLabel={t('categoryTime')}
              formatValue={formatRate}
            />
          </TrendSection>
        ) : null}
        {charts.hasTableSeries ? (
          <TrendSection title={t('charts.tableRows')} unit={t('unit.rows')}>
            <LineChart
              label={t('charts.tableRows')}
              unit={t('unit.rows')}
              data={charts.tableRows.data}
              series={charts.tableRows.series}
              height={200}
              loading={loading}
              emptyLabel={t('emptyChart')}
              resetLabel={t('legendClear')}
              categoryLabel={t('categoryTime')}
              formatValue={formatCount}
            />
          </TrendSection>
        ) : null}
        {/* 吞吐 / 条数对批任务也成立；CDC 放第三，避免离线作业一上来先看到 CDC。
            Throughput and row counts apply to batch jobs too; CDC is third so offline jobs do not lead with it. */}
        <TrendSection title={t('charts.cdc')} unit={t('unit.seconds')} className='md:col-span-2'>
          <BrushChart
            data={charts.cdc}
            label={t('charts.cdc')}
            unit={t('unit.seconds')}
            height={220}
            overviewHeight={48}
            minSpan={charts.cdcMinSpan}
            annotations={charts.annotations}
            formatDate={(date) => formatDateTime(date.getTime())}
            formatTime={(date) => formatTime(date.getTime())}
            formatValue={formatSeconds}
            emptyLabel={t('emptyChart')}
          />
        </TrendSection>
        <TrendSection
          title={t('charts.checkpoint')}
          unit={t('unit.seconds')}
          // Arc BarChart 只读 --accent；在本区块内把它换成 Checkpoint 阶段色，不影响别处。
          // Arc BarChart only reads --accent; rebind it to the checkpoint color inside this section only.
          style={{'--accent': TREND_COLORS.checkpoint} as CSSProperties}
        >
          <BarChart
            data={charts.checkpoint}
            label={t('charts.checkpoint')}
            period={charts.barPeriod}
            unit={t('unit.seconds')}
            averageLabel={t('averageLabel')}
            valueLabel={t('valueLabel')}
            categoryLabel={t('categoryTime')}
            formatValue={formatSeconds}
          />
        </TrendSection>
        <TrendSection title={t('pipeline.label')} unit={t('unit.rows')}>
          <DonutChart
            label={t('pipeline.label')}
            unit={t('unit.rows')}
            data={charts.pipeline}
            totalLabel={t('totalLabel')}
            emptyLabel={t('emptyChart')}
            otherLabel={t('otherLabel')}
            formatValue={formatCount}
            size={196}
          />
        </TrendSection>
        <TrendSection title={t('charts.busy')} unit='%'>
          {engineObs ? (
            <LineChart
              label={t('charts.busy')}
              unit='%'
              data={charts.busy.data}
              series={charts.busy.series}
              height={200}
              loading={loading}
              emptyLabel={t('emptyChart')}
              resetLabel={t('legendClear')}
              categoryLabel={t('categoryTime')}
              formatValue={(value) => `${value.toFixed(1)}%`}
            />
          ) : (
            <EngineObsEmpty t={t} />
          )}
        </TrendSection>
        <TrendSection title={t('charts.backpressure')} unit='%'>
          {engineObs ? (
            <LineChart
              label={t('charts.backpressure')}
              unit='%'
              data={charts.backpressure.data}
              series={charts.backpressure.series}
              height={200}
              loading={loading}
              emptyLabel={t('emptyChart')}
              resetLabel={t('legendClear')}
              categoryLabel={t('categoryTime')}
              formatValue={(value) => `${value.toFixed(1)}%`}
            />
          ) : (
            <EngineObsEmpty t={t} />
          )}
        </TrendSection>
      </div>
    </div>
  );
}

function EngineObsEmpty({t}: {t: ReturnType<typeof useTranslations>}) {
  return (
    <div className='flex h-[200px] flex-col items-center justify-center gap-1 rounded-md border border-dashed border-border text-muted-foreground'>
      <div className='text-[12.5px] text-foreground'>{t('engineObsEmptyTitle')}</div>
      <div className='text-[11.5px]'>{t('engineObsEmptyBody')}</div>
    </div>
  );
}

function TrendSection({
  title,
  unit,
  className,
  style,
  children,
}: {
  title: string;
  unit?: string;
  className?: string;
  style?: CSSProperties;
  children: ReactNode;
}) {
  return (
    <section className={cn('min-w-0 rounded-lg border border-border p-3', className)} style={style} aria-label={title}>
      <div className='mb-2 flex h-5 items-center gap-2'>
        <h3 className='text-[13px] font-medium'>{title}</h3>
        {unit ? <span className='text-[11.5px] text-muted-foreground'>{unit}</span> : null}
      </div>
      {children}
    </section>
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
  // 读 / 写卡片带与折线同色的小色标，把顶部数字和下方曲线对上。
  // Read / write cards carry a swatch in the line color, tying the numbers to the curves below.
  const items: {k: string; v: string; u: string; color?: string}[] = [
    {k: t('hero.readQps'), v: fmtNum(readQps), u: 'rec/s', color: TREND_COLORS.read},
    {k: t('hero.writeQps'), v: fmtNum(writeQps), u: 'rec/s', color: TREND_COLORS.write},
    {k: t('hero.lag'), v: fmtNum(lag), u: 'rec/s'},
    {k: t('hero.cdc'), v: cdc == null ? '-' : cdc.toFixed(1), u: 's'},
    {k: t('hero.checkpoint'), v: '-', u: 's'},
  ];
  return (
    <div className='grid grid-cols-2 gap-3 md:grid-cols-5'>
      {items.map((item) => (
        <div key={item.k} className='rounded-lg border border-border px-3.5 py-3'>
          <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
            {item.color ? (
              <span aria-hidden className='h-[2.5px] w-2.5 shrink-0 rounded-full' style={{background: item.color}} />
            ) : null}
            {item.k}
          </div>
          <div className='mt-1 font-mono text-[26px] font-medium leading-none tracking-tight tabular-nums'>
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

function fmtNum(n: number) {
  if (!Number.isFinite(n)) return '-';
  return Math.round(n).toLocaleString();
}
