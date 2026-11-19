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

/**
 * Alerts panel: metric strip, filter toolbar, server-driven table, detail drawer.
 * 告警面板：指标行（点击即筛选）、filter-toolbar、服务端分页 sortable-data-table、
 * Arc drawer 详情（命中规则 / 时间线 / 投递记录）。筛选与页码同步 URL，筛选变化重置 page=1。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useLocale, useTranslations} from 'next-intl';
import {
  Activity,
  BellOff,
  CheckCheck,
  ChevronRight,
  ExternalLink,
  Lightbulb,
  X,
} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Pagination} from '@/components/arc/pagination/pagination';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {SortableDataTable, type DataColumn} from '@/components/arc/sortable-data-table/sortable-data-table';
import {Button} from '@/components/ui/button';
import {CompactTimeFilter, formatToDateTimeLocal} from '@/components/common/layout/CompactTimeFilter';
import {TroubleshootingMemoryCard} from '@/components/common/troubleshooting/TroubleshootingMemoryCard';
import {SaveMemoryDialog} from '@/components/common/troubleshooting/SaveMemoryDialog';
import {syncAutoCloseReason} from '@/components/common/sync/task-alert-utils';
import services from '@/lib/services';
import {wallTimeToRFC3339} from '@/lib/timezone';
import type {TroubleshootingMemoryEntry} from '@/lib/services/troubleshooting';
import type {
  AlertInstance,
  AlertInstanceFilterParams,
  ClusterRule,
  NotificationDelivery,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {
  alertRowStatus,
  alertScopeOf,
  formatAgo,
  formatRuleThreshold,
  isCritical,
  minutesAgo,
  severityTone,
  type AlertRowStatus,
  type MonitoringTranslator,
} from '../monitoring-utils';

const PAGE_SIZE = 10;
const SILENCE_MINUTES = 30;
// Radix Select 的 option value 不允许空串，用哨兵值表示「全部」。
// Radix select options cannot use an empty string, so a sentinel stands in for "all".
const ALL_VALUE = '__all__';

/** 列表筛选的 URL/请求参数键，写回 URL 时只动这组键避免踩到别的 tab 参数。 / Alert filter keys owned by this panel. */
const FILTER_KEYS = ['scope', 'status', 'severity', 'cluster_id', 'q', 'page', 'start_time', 'end_time'] as const;

/** datetime-local 墙上时间转 RFC3339（与旧告警中心一致的时区处理）。 / Wall-time input value to RFC3339, same timezone handling as the old alerts center. */
function toRFC3339(value: string): string | undefined {
  if (!value) {
    return undefined;
  }
  return wallTimeToRFC3339(value);
}

function StatusCell({status, t}: {status: AlertRowStatus; t: MonitoringTranslator}) {
  return (
    <span className='flex items-center gap-1.5 whitespace-nowrap text-xs'>
      <span
        className={cn('size-1.5 shrink-0 rounded-full', {
          'animate-pulse bg-destructive': status === 'firing',
          'bg-muted-foreground/50': status === 'acknowledged',
          'bg-(--warning)': status === 'silenced',
          'bg-muted-foreground/30': status === 'resolved',
          'bg-muted-foreground/60': status === 'closed',
        })}
        aria-hidden
      />
      <span className={status === 'firing' ? 'text-destructive' : 'text-muted-foreground'}>
        {t(`status.${status}`)}
      </span>
    </span>
  );
}

export interface AlertFilters {
  scope: string;
  status: string;
  severity: string;
  clusterId: string;
  query: string;
  page: number;
  startTime: string;
  endTime: string;
}

function readFilters(sp: URLSearchParams, initialClusterId?: string | null): AlertFilters {
  return {
    scope: sp.get('scope') ?? 'all',
    status: sp.get('status') ?? '',
    severity: sp.get('severity') ?? '',
    clusterId: sp.get('cluster_id') ?? initialClusterId ?? '',
    query: sp.get('q') ?? '',
    page: Math.max(1, Number(sp.get('page')) || 1),
    startTime: sp.get('start_time') ?? '',
    endTime: sp.get('end_time') ?? '',
  };
}

/**
 * フィルタ変更を適用し、フィルタ変更時は page=1 に戻す。ページ移動だけなら page を維持。
 * Applies a filter patch; a filter change resets to page 1 while paging itself keeps the page.
 */
export function applyAlertFilterChange(
  filters: AlertFilters,
  patch: Partial<AlertFilters>,
  resetPage: boolean,
): AlertFilters {
  const next: AlertFilters = {...filters, ...patch};
  if (resetPage) {
    next.page = 1;
  }
  return next;
}

export function buildAlertInstanceParams(f: AlertFilters): AlertInstanceFilterParams {
  const params: AlertInstanceFilterParams = {page: f.page, page_size: PAGE_SIZE};
  if (f.scope === 'cluster' || f.scope === 'job') {
    params.scope = f.scope;
  }
  if (f.status === 'acknowledged' || f.status === 'silenced') {
    params.handling_status = f.status;
    params.status = 'firing';
  } else if (f.status === 'firing' || f.status === 'resolved' || f.status === 'closed') {
    params.status = f.status;
  }
  if (f.severity === 'warning' || f.severity === 'critical') {
    params.severity = f.severity;
  }
  if (f.clusterId) {
    params.cluster_id = f.clusterId;
  }
  const startTime = toRFC3339(f.startTime);
  const endTime = toRFC3339(f.endTime);
  if (startTime) {
    params.start_time = startTime;
  }
  if (endTime) {
    params.end_time = endTime;
  }
  return params;
}

interface AlertsPanelProps {
  /** cluster_id 深链初值。Initial cluster filter from the cluster-detail deep link. */
  initialClusterId?: string | null;
  onOpenRule?: (ruleKey: string) => void;
  /** 空态「查看规则」动作：切到规则 tab。Empty-state action that switches to the rules tab. */
  onGoRules?: () => void;
}

export function AlertsPanel({initialClusterId, onOpenRule, onGoRules}: AlertsPanelProps) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const ago = useCallback((iso?: string | null) => formatAgo(minutesAgo(iso), t), [t]);
  const displayName = useCallback(
    (a: AlertInstance) => (locale === 'en' ? a.alert_name_en || a.alert_name : a.alert_name),
    [locale],
  );

  const filters = useMemo(
    () => readFilters(new URLSearchParams(searchParams.toString()), initialClusterId),
    [searchParams, initialClusterId],
  );
  const {scope, status, severity, clusterId, query, page, startTime, endTime} = filters;

  const [rows, setRows] = useState<AlertInstance[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [metrics, setMetrics] = useState({firing: 0, critical: 0, new24h: 0});
  const [clusterOptions, setClusterOptions] = useState<{value: string; label: string}[]>([]);
  const [active, setActive] = useState<AlertInstance | null>(null);
  const [actionBusy, setActionBusy] = useState(false);
  // 排障经验版本号：保存经验后 +1 触发行指示与详情命中重算。
  // Memory version bumped after saving so row indicators and detail matches recompute.
  const [memoriesVersion, setMemoriesVersion] = useState(0);

  const writeFilters = useCallback(
    (patch: Partial<AlertFilters>, resetPage: boolean) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const k of FILTER_KEYS) {
        params.delete(k);
      }
      const next = applyAlertFilterChange(filters, patch, resetPage);
      if (next.scope !== 'all') {params.set('scope', next.scope);}
      if (next.status) {params.set('status', next.status);}
      if (next.severity) {params.set('severity', next.severity);}
      if (next.clusterId) {params.set('cluster_id', next.clusterId);}
      if (next.query) {params.set('q', next.query);}
      if (next.startTime) {params.set('start_time', next.startTime);}
      if (next.endTime) {params.set('end_time', next.endTime);}
      if (next.page > 1) {params.set('page', String(next.page));}
      router.replace(`${pathname}?${params.toString()}`, {scroll: false});
    },
    [filters, pathname, router, searchParams],
  );

  // 列表查询：参数全部由服务端应用，翻页/排序走接口。Server-driven list query.
  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError(null);
    services.monitoring
      .getAlertInstances(buildAlertInstanceParams(filters))
      .then((data) => {
        if (!alive) {return;}
        setRows(Array.isArray(data?.alerts) ? data.alerts : []);
        setTotal(data?.total ?? 0);
        setLoading(false);
      })
      .catch((err) => {
        if (!alive) {return;}
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [filters]);

  // 指标卡计数与可选集群列表。Metric strip counts and the cluster filter options.
  useEffect(() => {
    let alive = true;
    const dayAgo = new Date(Date.now() - 86400_000).toISOString();
    Promise.all([
      services.monitoring.getAlertInstances({status: 'firing', page: 1, page_size: 1}),
      services.monitoring.getAlertInstances({status: 'firing', severity: 'critical', page: 1, page_size: 1}),
      services.monitoring.getAlertInstances({start_time: dayAgo, page: 1, page_size: 1}),
      services.cluster.getClusters({current: 1, size: 200}).catch(() => null),
    ])
      .then(([firing, critical, fresh, clusters]) => {
        if (!alive) {return;}
        setMetrics({
          firing: firing?.stats?.firing ?? firing?.total ?? 0,
          critical: critical?.total ?? 0,
          new24h: fresh?.total ?? 0,
        });
        const items = clusters?.clusters ?? [];
        setClusterOptions(
          items.map((c: {id: number; name: string}) => ({value: String(c.id), label: c.name})),
        );
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  const patchInstance = useCallback(
    (alertId: string, result: Partial<AlertInstance>) => {
      setRows((prev) => prev.map((a) => (a.alert_id === alertId ? {...a, ...result} : a)));
      setActive((prev) => (prev?.alert_id === alertId ? {...prev, ...result} : prev));
    },
    [],
  );

  const runAction = useCallback(
    async (alert: AlertInstance, kind: 'ack' | 'silence' | 'close') => {
      setActionBusy(true);
      try {
        const result =
          kind === 'ack'
            ? await services.monitoring.acknowledgeAlertInstance(alert.alert_id)
            : kind === 'silence'
              ? await services.monitoring.silenceAlertInstance(alert.alert_id, {
                  duration_minutes: SILENCE_MINUTES,
                })
              : await services.monitoring.closeAlertInstance(alert.alert_id);
        patchInstance(alert.alert_id, result as Partial<AlertInstance>);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setActionBusy(false);
      }
    },
    [patchInstance],
  );

  // 行内筛选下拉：'all' 哨兵对应空筛选，URL 里不落地。选项标签统一带字段名前缀，
  // 触发器才不会三个都只显示「全部」。'all' sentinel maps to an empty filter; option
  // labels carry the field prefix so the triggers don't all read "All".
  const label = (field: string, value: string) => t('filterLabel', {field, value});
  const statusOptions = [
    {value: ALL_VALUE, label: label(t('alertStatusLabel'), t('scopeAll'))},
    ...(['firing', 'acknowledged', 'silenced', 'resolved'] as const).map((v) => ({
      value: v,
      label: label(t('alertStatusLabel'), t(`status.${v}`)),
    })),
  ];
  const severityOptions = [
    {value: ALL_VALUE, label: label(t('fieldSeverity'), t('scopeAll'))},
    {value: 'warning', label: label(t('fieldSeverity'), t('severityWarning'))},
    {value: 'critical', label: label(t('fieldSeverity'), t('severityCritical'))},
  ];
  const clusterFilterOptions = [
    {value: ALL_VALUE, label: label(t('colCluster'), t('scopeAll'))},
    ...clusterOptions.map((o) => ({value: o.value, label: label(t('colCluster'), o.label)})),
  ];
  const hasActiveFilter = Boolean(status || severity || clusterId);
  // 任意筛选激活（含时间与搜索词）时给出「清除筛选」空态分支。
  // Any active filter (including time and query) selects the filtered empty-state branch.
  const anyFilterActive = Boolean(status || severity || clusterId || query.trim() || startTime || endTime);
  const clearAllFilters = useCallback(
    () => writeFilters({status: '', severity: '', clusterId: '', query: '', startTime: '', endTime: '', scope: 'all'}, true),
    [writeFilters],
  );

  // 时间预设：预设窗口结束于 now，开始于 now-minutes（与旧告警中心一致）。
  // Time presets end at now and start minutes ago, matching the old alerts center.
  const [activePreset, setActivePreset] = useState<number | null>(null);
  const applyTimePreset = useCallback(
    (minutes: number) => {
      setActivePreset(minutes);
      const now = new Date();
      writeFilters(
        {
          startTime: formatToDateTimeLocal(new Date(now.getTime() - minutes * 60_000)),
          endTime: formatToDateTimeLocal(now),
        },
        true,
      );
    },
    [writeFilters],
  );

  // 当前页告警的排障经验命中标记。Memory-match flags for the current page rows.
  const alertMemoryMap = useMemo(() => {
    const map = new Map<string, boolean>();
    for (const a of rows) {
      const matches = services.troubleshooting.findMatchingMemories({
        fingerprint: a.rule_key || a.alert_name,
        title: displayName(a) || a.rule_key,
        target_type: 'alert',
      });
      if (matches.length > 0) {
        map.set(a.alert_id, true);
      }
    }
    return map;
    // findMatchingMemories 读 localStorage，版本号是唯一的重算信号。
    // The lookup reads localStorage; the version bump is the only recompute signal.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rows, memoriesVersion]);

  const navigateToDiagnostics = useCallback(
    (alert: AlertInstance) => {
      const params = new URLSearchParams();
      if (alert.cluster_id) {
        params.set('cluster_id', alert.cluster_id);
      }
      params.set('source', 'alerts');
      params.set('alert_id', alert.alert_id);
      router.push(`/diagnostics?${params.toString()}`);
    },
    [router],
  );

  // 搜索是接口参数之外的客户端收窄，只作用于已返回的当前页。Search narrows the returned page client-side.
  const pageItems = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) {
      return rows;
    }
    return rows.filter(
      (a) =>
        a.alert_name.toLowerCase().includes(q) ||
        (a.alert_name_en ?? '').toLowerCase().includes(q) ||
        a.rule_key.toLowerCase().includes(q) ||
        a.cluster_name.toLowerCase().includes(q),
    );
  }, [rows, query]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const columns: DataColumn<Record<string, unknown>>[] = [
    {
      key: 'status',
      label: t('colStatus'),
      sortable: true,
      width: 96,
      render: (_, row) => <StatusCell status={alertRowStatus(row.alert as AlertInstance)} t={t} />,
    },
    {
      key: 'name',
      label: t('colAlert'),
      sortable: true,
      render: (_, row) => {
        const alert = row.alert as AlertInstance;
        return (
          <button
            type='button'
            className='min-w-0 text-left'
            onClick={() => setActive(alert)}
            data-testid={`alert-name-${alert.alert_id}`}
          >
            <div className='flex items-center gap-1.5'>
              <span className='truncate text-sm font-medium hover:underline'>{displayName(alert)}</span>
              {alertMemoryMap.get(alert.alert_id) ? (
                <Lightbulb className='size-3.5 shrink-0 text-emerald-500' aria-hidden />
              ) : null}
            </div>
            <div className='truncate font-mono text-xs text-muted-foreground'>{alert.rule_key}</div>
          </button>
        );
      },
    },
    {
      key: 'severity',
      label: t('fieldSeverity'),
      sortable: true,
      width: 76,
      render: (_, row) => {
        const alert = row.alert as AlertInstance;
        return (
          <Badge tone={severityTone(alert.severity)} size='sm'>
            {isCritical(alert) ? t('severityCritical') : t('severityWarning')}
          </Badge>
        );
      },
    },
    {
      key: 'scope',
      label: t('colScope'),
      width: 76,
      render: (_, row) => (
        <Badge size='sm' outline>
          {alertScopeOf(row.alert as AlertInstance) === 'cluster' ? t('scopeCluster') : t('scopeJob')}
        </Badge>
      ),
    },
    {
      key: 'cluster',
      label: t('colCluster'),
      sortable: true,
      render: (_, row) => (
        <span className='block min-w-0 truncate text-xs text-muted-foreground'>
          {(row.alert as AlertInstance).cluster_name}
        </span>
      ),
    },
    {
      key: 'time',
      label: t('colTime'),
      sortable: true,
      numeric: true,
      width: 92,
      render: (_, row) => {
        const alert = row.alert as AlertInstance;
        return (
          <span className='whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
            {ago(alert.firing_at || alert.created_at)}
          </span>
        );
      },
    },
    {
      key: 'actions',
      label: '',
      width: 140,
      render: (_, row) => {
        const alert = row.alert as AlertInstance;
        const rowStatus = alertRowStatus(alert);
        return (
          <div className='flex justify-end'>
            <DropdownMenu
              label={t('moreActions')}
              items={[
                {
                  label: t('actionAck'),
                  icon: <CheckCheck className='size-4' aria-hidden />,
                  disabled: rowStatus !== 'firing' || actionBusy,
                  onSelect: () => void runAction(alert, 'ack'),
                },
                {
                  label: t('actionSilence'),
                  icon: <BellOff className='size-4' aria-hidden />,
                  disabled: rowStatus !== 'firing' || actionBusy,
                  onSelect: () => void runAction(alert, 'silence'),
                },
                {
                  label: t('actionClose'),
                  icon: <X className='size-4' aria-hidden />,
                  destructive: true,
                  separatorBefore: true,
                  disabled: alert.status === 'closed' || actionBusy,
                  onSelect: () => void runAction(alert, 'close'),
                },
              ]}
            />
          </div>
        );
      },
    },
  ];

  const tableRows = pageItems.map((a) => ({
    id: a.alert_id,
    status: alertRowStatus(a),
    name: displayName(a),
    severity: a.severity,
    scope: alertScopeOf(a),
    cluster: a.cluster_name,
    time: a.firing_at || a.created_at,
    alert: a,
  }));

  return (
    <div className='relative flex h-full flex-col gap-3'>
      <div className='grid grid-cols-3 divide-x divide-border/60 rounded-lg border border-border/60 bg-card'>
        {(
          [
            {
              label: t('metricFiring'),
              value: metrics.firing,
              active: status === 'firing' && !severity,
              onClick: () => writeFilters({status: 'firing', severity: ''}, true),
            },
            {
              label: t('metricCritical'),
              value: metrics.critical,
              active: severity === 'critical',
              onClick: () => writeFilters({status: 'firing', severity: 'critical'}, true),
            },
            {
              label: t('metricNew24h'),
              value: metrics.new24h,
              active: false,
              onClick: () => writeFilters({status: '', severity: ''}, true),
            },
          ]
        ).map((m) => (
          <button
            key={m.label}
            type='button'
            onClick={m.onClick}
            className={cn('px-4 py-3 text-left transition-colors hover:bg-muted/40', m.active && 'bg-muted/50')}
          >
            <div className='text-xs text-muted-foreground'>{m.label}</div>
            <div className='mt-1 text-2xl font-medium tabular-nums'>{m.value}</div>
          </button>
        ))}
      </div>

      <div className='flex flex-wrap items-center gap-3'>
        <SegmentedControl
          label={t('alertScopeLabel')}
          value={scope}
          onValueChange={(v) => writeFilters({scope: v}, true)}
          options={[
            {value: 'all', label: t('scopeAll')},
            {value: 'cluster', label: t('scopeCluster')},
            {value: 'job', label: t('scopeJob')},
          ]}
        />
        <div className='flex flex-wrap items-center gap-2 [&_label]:sr-only'>
          <div className='w-36'>
            <Select
              label={t('alertStatusLabel')}
              value={status || ALL_VALUE}
              onValueChange={(v) => writeFilters({status: v === ALL_VALUE ? '' : v}, true)}
              options={statusOptions}
            />
          </div>
          <div className='w-32'>
            <Select
              label={t('fieldSeverity')}
              value={severity || ALL_VALUE}
              onValueChange={(v) => writeFilters({severity: v === ALL_VALUE ? '' : v}, true)}
              options={severityOptions}
            />
          </div>
          <div className='w-40'>
            <Select
              label={t('colCluster')}
              value={clusterId || ALL_VALUE}
              onValueChange={(v) => writeFilters({clusterId: v === ALL_VALUE ? '' : v}, true)}
              options={clusterFilterOptions}
            />
          </div>
          {hasActiveFilter ? (
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='h-8 px-2 text-xs text-muted-foreground'
              onClick={() => writeFilters({status: '', severity: '', clusterId: ''}, true)}
            >
              {t('clearFilters')}
            </Button>
          ) : null}
        </div>
        <div className='w-56 min-w-[200px] max-w-sm flex-1 [&_label]:sr-only'>
          <SearchField
            label={t('alertSearchLabel')}
            value={query}
            onValueChange={(v) => writeFilters({query: v}, true)}
            placeholder={t('alertSearchPlaceholder')}
            clearLabel={t('searchClear')}
          />
        </div>
        {/* 时间范围：预设 + 自定义起止，改动重置 page=1 并落 URL（旧告警中心同款）。
            Time range: presets and custom bounds, resets page=1 and lands in the URL. */}
        <CompactTimeFilter
          className='w-full border-t-0 pt-0'
          startTime={startTime}
          endTime={endTime}
          activePreset={activePreset}
          onApplyPreset={applyTimePreset}
          onStartTimeChange={(v) => {
            setActivePreset(null);
            writeFilters({startTime: v}, true);
          }}
          onEndTimeChange={(v) => {
            setActivePreset(null);
            writeFilters({endTime: v}, true);
          }}
          onClear={() => {
            setActivePreset(null);
            writeFilters({startTime: '', endTime: ''}, true);
          }}
        />
      </div>

      {error ? (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive'>
          {error}
        </div>
      ) : null}

      <div className='min-h-0 flex-1 overflow-y-auto'>
        {loading && rows.length === 0 ? (
          <div className='space-y-2 py-2' aria-busy>
            {Array.from({length: 5}).map((_, i) => (
              <Skeleton key={i} className='h-12 w-full' lines={1} />
            ))}
          </div>
        ) : pageItems.length === 0 ? (
          // 无告警与「有筛选无结果」两种空态文案不同；后者给清除筛选动作。
          // Two empty states: no alerts at all vs. filters yielding nothing.
          <EmptyState
            title={anyFilterActive ? t('emptyFilteredTitle') : t('emptyAlertsTitle')}
            description={anyFilterActive ? t('emptyFilteredDesc') : t('emptyAlertsDesc')}
            action={
              anyFilterActive ? (
                <Button type='button' variant='outline' size='sm' onClick={clearAllFilters}>
                  {t('clearFilters')}
                </Button>
              ) : (
                <Button type='button' size='sm' onClick={onGoRules}>
                  {t('emptyAlertsAction')}
                </Button>
              )
            }
          />
        ) : (
          <>
            {/* vendored SortableDataTable 不暴露行回调：点击在外层委托，行内名称按钮提供键盘路径。
                The vendored table exposes no row callback: clicks are delegated on the
                wrapper; the name button keeps a keyboard path. */}
            <div
              className='hidden sm:block [&_tbody_tr]:cursor-pointer'
              onClick={(e) => {
                const el = e.target as HTMLElement;
                if (el.closest('button, a, input, select, textarea, [role="button"], [role="menu"], [role="checkbox"]')) {return;}
                const tr = el.closest('tr');
                const marker = tr?.querySelector('[data-testid^="alert-name-"]');
                const id = marker?.getAttribute('data-testid')?.slice('alert-name-'.length);
                const alert = id ? pageItems.find((a) => a.alert_id === id) : undefined;
                if (alert) {setActive(alert);}
              }}
            >
              <SortableDataTable
                rows={tableRows}
                columns={columns}
                rowKey='id'
                caption={t('tabAlerts')}
                emptyMessage={t('alertsEmpty')}
                itemName={{one: t('itemAlert'), other: t('itemAlerts')}}
              />
            </div>
            <div className='sm:hidden'>
              {pageItems.map((alert) => (
                <MobileAlertCard
                  key={alert.alert_id}
                  alert={alert}
                  ago={ago}
                  hasMemory={alertMemoryMap.get(alert.alert_id) ?? false}
                  onOpen={() => setActive(alert)}
                />
              ))}
            </div>
          </>
        )}
      </div>

      <Pagination page={page} pageCount={totalPages} onPageChange={(p) => writeFilters({page: p}, false)} />

      <Drawer open={active != null} onOpenChange={(open) => !open && setActive(null)}>
        {active ? (
          <DrawerContent
            side='right'
            title={displayName(active)}
            description={active.cluster_name}
            className='w-[min(92vw,460px)] rounded-none!'
          >
            <AlertDetailBody
              alert={active}
              ago={ago}
              memoriesVersion={memoriesVersion}
              onMemorySaved={() => setMemoriesVersion((v) => v + 1)}
              onNavigateDiagnostics={navigateToDiagnostics}
              onOpenRule={onOpenRule}
              onAck={() => void runAction(active, 'ack')}
              onSilence={() => void runAction(active, 'silence')}
            />
          </DrawerContent>
        ) : null}
      </Drawer>
    </div>
  );
}

function MobileAlertCard({
  alert,
  ago,
  hasMemory,
  onOpen,
}: {
  alert: AlertInstance;
  ago: (iso?: string | null) => string;
  hasMemory: boolean;
  onOpen: () => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  return (
    <div className='border-b border-border/40 px-2 py-3'>
      <button type='button' className='flex w-full items-center gap-2 text-left' onClick={onOpen}>
        <StatusCell status={alertRowStatus(alert)} t={t} />
        <span className='min-w-0 flex-1 truncate text-sm font-medium'>
          {locale === 'en' ? alert.alert_name_en || alert.alert_name : alert.alert_name}
        </span>
        {hasMemory ? <Lightbulb className='size-3.5 shrink-0 text-emerald-500' aria-hidden /> : null}
        <ChevronRight className='size-4 shrink-0 text-muted-foreground' aria-hidden />
      </button>
      <div className='mt-1.5 flex flex-wrap items-center gap-2 pl-4 text-xs text-muted-foreground'>
        <Badge tone={severityTone(alert.severity)} size='sm'>
          {isCritical(alert) ? t('severityCritical') : t('severityWarning')}
        </Badge>
        <Badge size='sm' outline>
          {alertScopeOf(alert) === 'cluster' ? t('scopeCluster') : t('scopeJob')}
        </Badge>
        <span className='min-w-0 truncate'>{alert.cluster_name}</span>
        <span className='whitespace-nowrap tabular-nums'>{ago(alert.firing_at || alert.created_at)}</span>
      </div>
    </div>
  );
}

function AlertDetailBody({
  alert,
  ago,
  memoriesVersion,
  onMemorySaved,
  onNavigateDiagnostics,
  onOpenRule,
  onAck,
  onSilence,
}: {
  alert: AlertInstance;
  ago: (iso?: string | null) => string;
  memoriesVersion: number;
  onMemorySaved: () => void;
  onNavigateDiagnostics: (alert: AlertInstance) => void;
  onOpenRule?: (ruleKey: string) => void;
  onAck: () => void;
  onSilence: () => void;
}) {
  const t = useTranslations('monitoring');
  const tMc = useTranslations('monitoringCenter');
  const tAlert = useTranslations('syncAlert');
  const tsT = useTranslations('troubleshooting');
  const locale = useLocale();
  const alertDisplayName = locale === 'en' ? alert.alert_name_en || alert.alert_name : alert.alert_name;
  const router = useRouter();
  const rowStatus = alertRowStatus(alert);
  const [rule, setRule] = useState<ClusterRule | null>(null);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [memoryDialogOpen, setMemoryDialogOpen] = useState(false);

  // 命中排障经验：fingerprint 取 rule_key 兜底 alert_name，与旧告警中心一致。
  // Memory match: fingerprint falls back to alert_name like the old alerts center.
  const matchedMemories = useMemo(
    () =>
      services.troubleshooting.findMatchingMemories({
        fingerprint: alert.rule_key || alert.alert_name,
        title: alertDisplayName || alert.rule_key,
        target_type: 'alert',
      }),
    // findMatchingMemories 读 localStorage，版本号是唯一的重算信号。
    // The lookup reads localStorage; the version bump is the only recompute signal.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [alert, memoriesVersion],
  );
  const primaryMemory: TroubleshootingMemoryEntry | null = matchedMemories[0] ?? null;

  // 集群级告警拉一次规则视图拿生效阈值与例外归属；投递记录按集群拉一页再按 alert_id 收窄。
  // Cluster alerts fetch the rule view for effective threshold/override info; deliveries are fetched
  // per cluster then narrowed to this alert id (the delivery API has no alert_id filter).
  useEffect(() => {
    let alive = true;
    if (alertScopeOf(alert) === 'cluster' && alert.rule_key) {
      services.monitoring
        .getClusterRule(alert.rule_key)
        .then((r) => alive && setRule(r))
        .catch(() => undefined);
    }
    services.monitoring
      .listNotificationDeliveries({cluster_id: alert.cluster_id || undefined, page: 1, page_size: 50})
      .then((d) => {
        if (!alive) {return;}
        setDeliveries(
          (d?.deliveries ?? []).filter((x) => x.alert_id === alert.alert_id).slice(0, 10),
        );
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [alert]);

  const override = rule?.overrides?.find((o) => String(o.cluster_id) === alert.cluster_id);
  const timelineEvents = useMemo(() => {
    const evs: {key: string; at?: string | null; note?: string}[] = [
      {key: 'triggered', at: alert.firing_at || alert.created_at},
    ];
    for (const d of deliveries) {
      evs.push({key: 'notified', at: d.sent_at || d.created_at, note: d.channel_name});
    }
    if (alert.acknowledged_at) {
      evs.push({key: 'acknowledged', at: alert.acknowledged_at, note: alert.acknowledged_by});
    }
    if (alert.resolved_at) {
      evs.push({key: 'resolved', at: alert.resolved_at});
    }
    if (alert.closed_at) {
      evs.push({key: 'closed', at: alert.closed_at, note: alert.closed_by});
    }
    return evs;
  }, [alert, deliveries]);

  return (
    <div className='space-y-4 px-1 pb-4'>
      <div className='flex items-center gap-2'>
        <Badge tone={severityTone(alert.severity)} size='sm'>
          {isCritical(alert) ? t('severityCritical') : t('severityWarning')}
        </Badge>
        <StatusCell status={rowStatus} t={t} />
      </div>

      {alert.summary || alert.description ? (
        <p className='text-sm text-muted-foreground'>{alert.summary || alert.description}</p>
      ) : null}
      {alert.description && alert.description !== alert.summary ? (
        <p className='rounded-lg bg-muted/30 px-3 py-2 text-xs leading-relaxed text-muted-foreground'>
          {alert.description}
        </p>
      ) : null}

      {alert.rule_key ? (
        <div className='rounded-lg border border-border/60 px-3 py-2.5 text-sm'>
          <span className='text-muted-foreground'>{t('hitRule')}</span>{' '}
          <button
            type='button'
            className='font-medium text-primary underline-offset-2 hover:underline'
            onClick={() => onOpenRule?.(alert.rule_key)}
          >
            {alert.rule_key}
          </button>
          {rule ? (
            <span className='text-muted-foreground'>
              {' · '}
              {override ? t('fromException') : t('fromGlobal')}
              {' · '}
              <span className='whitespace-nowrap'>{formatRuleThreshold(rule, t)}</span>
            </span>
          ) : null}
        </div>
      ) : null}

      {rowStatus === 'firing' ? (
        <div className='flex gap-2'>
          <Button type='button' size='sm' className='h-8 rounded-md text-xs' onClick={onAck}>
            <CheckCheck className='mr-1.5 size-3.5' aria-hidden />
            {t('actionAck')}
          </Button>
          <Button type='button' size='sm' variant='outline' className='h-8 rounded-md text-xs' onClick={onSilence}>
            <BellOff className='mr-1.5 size-3.5' aria-hidden />
            {t('actionSilence')}
          </Button>
        </div>
      ) : null}

      {/* 历史排障经验 / Troubleshooting memory */}
      <TroubleshootingMemoryCard
        matchedMemory={primaryMemory}
        totalMatches={matchedMemories.length}
        onAddOrEdit={() => setMemoryDialogOpen(true)}
      />

      {/* 上下文：集群 / 主机 / 进程 / 事件 ID，作业告警附任务与自动关闭原因。
          Context: cluster/host/process/event id plus sync-episode extras. */}
      <div className='space-y-2'>
        <span className='text-sm font-medium'>{tMc('alerts.contextAndTopology')}</span>
        <div className='divide-y rounded-lg border border-border/60 text-xs'>
          <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
            <span className='text-muted-foreground'>{tMc('alerts.belongCluster')}</span>
            <span className='min-w-0 truncate font-medium'>
              {alert.cluster_name || alert.cluster_id || '-'}
            </span>
          </div>
          {alert.source_ref?.hostname ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{tMc('alerts.affectedHost')}</span>
              <span className='min-w-0 truncate font-mono font-medium'>{alert.source_ref.hostname}</span>
            </div>
          ) : null}
          {alert.source_ref?.process_name ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{tMc('alerts.associatedProcess')}</span>
              <span className='min-w-0 truncate font-mono font-medium'>{alert.source_ref.process_name}</span>
            </div>
          ) : null}
          {alert.source_ref?.event_id ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{tMc('alerts.associatedEventId')}</span>
              <span className='font-mono font-medium'>#{alert.source_ref.event_id}</span>
            </div>
          ) : null}
          {alert.source_ref?.original_alert_name ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{t('originalAlertName')}</span>
              <span className='min-w-0 truncate font-mono font-medium'>
                {alert.source_ref.original_alert_name}
              </span>
            </div>
          ) : null}
          {alert.source_ref?.task_id ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{tAlert('policyTaskColumn')}</span>
              <span className='min-w-0 truncate font-medium' title={alert.source_ref.task_name}>
                {alert.source_ref.task_name || `#${alert.source_ref.task_id}`}
              </span>
            </div>
          ) : null}
          {alert.source_ref?.job_id ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{tAlert('jobLabel')}</span>
              <span className='whitespace-nowrap font-mono font-medium'>
                #{alert.source_ref.job_id}
                {alert.source_ref.engine_job_id ? ` · ${alert.source_ref.engine_job_id}` : ''}
              </span>
            </div>
          ) : null}
          {syncAutoCloseReason(alert.source_ref?.resolve_reason) ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='whitespace-nowrap text-muted-foreground'>{tAlert('closeReasonLabel')}</span>
              <span className='min-w-0 text-right'>
                {tAlert(`closeReason_${syncAutoCloseReason(alert.source_ref?.resolve_reason)}`)}
              </span>
            </div>
          ) : null}
          {alert.source_ref?.reminders_paused ? (
            <div className='px-3 py-2.5 text-(--warning)'>{tAlert('remindersPausedHint')}</div>
          ) : null}
          {(alert.source_ref?.occurrence_count ?? 0) > 0 ? (
            <div className='flex items-center justify-between gap-3 px-3 py-2.5'>
              <span className='text-muted-foreground'>{tAlert('occurrenceLabel')}</span>
              <span className='whitespace-nowrap'>
                {tAlert('occurrences', {count: alert.source_ref?.occurrence_count ?? 0})}
              </span>
            </div>
          ) : null}
          {alert.source_ref?.link ? (
            <div className='flex items-center justify-end px-3 py-2.5'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='h-7 gap-1 rounded-md text-xs'
                data-testid='alert-open-job'
                onClick={() => router.push(alert.source_ref?.link || '/workbench')}
              >
                <ExternalLink className='size-3' aria-hidden />
                {alert.source_ref.job_id ? tAlert('openJob') : tAlert('openTask')}
              </Button>
            </div>
          ) : null}
        </div>
      </div>

      {/* 处理记录：确认时间 / 静默至 / 最新备注。Handling record. */}
      {alert.silenced_until || alert.acknowledged_at || alert.latest_note ? (
        <div className='space-y-2'>
          <span className='text-sm font-medium'>{tMc('alerts.handlingNotes')}</span>
          <div className='space-y-1.5 rounded-lg border border-border/60 bg-muted/20 px-3 py-2.5 text-xs'>
            {alert.silenced_until ? (
              <div className='flex items-center gap-2'>
                <span className='text-muted-foreground'>{tMc('alerts.silencedUntilLabel')}:</span>
                <span className='font-mono font-medium'>{ago(alert.silenced_until)}</span>
              </div>
            ) : null}
            {alert.acknowledged_at ? (
              <div className='flex items-center gap-2'>
                <span className='text-muted-foreground'>{tMc('alerts.acknowledgedTimeLabel')}:</span>
                <span className='font-mono font-medium'>
                  {ago(alert.acknowledged_at)}
                  {alert.acknowledged_by ? ` · ${alert.acknowledged_by}` : ''}
                </span>
              </div>
            ) : null}
            {alert.latest_note ? (
              <div className='space-y-1 border-t border-border/50 pt-1.5'>
                <span className='text-muted-foreground'>{tMc('alerts.latestNoteLabel')}:</span>
                <p className='text-foreground'>{alert.latest_note}</p>
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      {/* 前往诊断：沿旧告警中心的 /diagnostics 参数拼法。
          Diagnostics deep link, same query shape as the old alerts center. */}
      <Button
        type='button'
        size='sm'
        className='h-8 rounded-md text-xs'
        onClick={() => onNavigateDiagnostics(alert)}
      >
        <Activity className='mr-1.5 size-3.5' aria-hidden />
        {tMc('alerts.navigateToDiagnostics')}
        <ExternalLink className='ml-1 size-3 opacity-70' aria-hidden />
      </Button>

      <div className='space-y-2'>
        <span className='text-sm font-medium'>{t('timeline')}</span>
        <ol className='relative space-y-3 border-l border-border/60 pl-4'>
          {timelineEvents.map((ev, i) => (
            <li key={i} className='relative text-sm'>
              <span className='absolute -left-[21px] top-0.5 flex size-3.5 items-center justify-center rounded-full bg-card'>
                <TimelineDot kind={ev.key} />
              </span>
              <div className='flex items-baseline justify-between gap-2'>
                <span>{t(`timelineKind.${ev.key}`)}</span>
                <span className='shrink-0 whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
                  {ago(ev.at)}
                </span>
              </div>
              {ev.note ? <div className='mt-0.5 text-xs text-muted-foreground'>{ev.note}</div> : null}
            </li>
          ))}
        </ol>
      </div>

      <div className='space-y-2'>
        <span className='text-sm font-medium'>{t('deliveries')}</span>
        {deliveries.length === 0 ? (
          <p className='rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>
            {t('deliveriesEmpty')}
          </p>
        ) : (
          <ul className='space-y-1.5'>
            {deliveries.map((d) => (
              <li
                key={d.id}
                className='flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm'
              >
                <span className='truncate font-mono text-xs'>{d.channel_name ?? d.channel_id}</span>
                <span className='flex shrink-0 items-center gap-2'>
                  <Badge
                    size='sm'
                    tone={d.status === 'sent' ? 'success' : d.status === 'failed' ? 'danger' : 'neutral'}
                    outline
                  >
                    {t(`delivery.${d.status}`)}
                  </Badge>
                  <span className='whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
                    {ago(d.sent_at || d.created_at)}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* 沉淀/更新排障经验：字段沿用旧告警中心（fingerprint=rule_key||alert_name，target=alert）。
          Save/update troubleshooting memory; same field mapping as the old alerts center. */}
      <SaveMemoryDialog
        open={memoryDialogOpen}
        onOpenChange={setMemoryDialogOpen}
        initialData={{
          id: primaryMemory?.id,
          target_type: 'alert',
          fingerprint: alert.rule_key || alert.alert_name,
          title:
            primaryMemory?.title ||
            `${alertDisplayName || alert.rule_key} ${tsT('defaultTitleSuffix')}`,
          error_summary: alert.summary || alert.description,
          root_cause: primaryMemory?.root_cause,
          solution: primaryMemory?.solution || '',
          preventive_tips: primaryMemory?.preventive_tips,
          tags: primaryMemory?.tags || ['alert', alert.source_type || 'metric'].filter(Boolean),
          cluster_id: alert.cluster_id,
          cluster_name: alert.cluster_name,
          author: primaryMemory?.author,
        }}
        onSaved={onMemorySaved}
      />
    </div>
  );
}

function TimelineDot({kind}: {kind: string}) {
  const cls = {
    triggered: 'bg-destructive',
    notified: 'bg-(--info, var(--accent))',
    acknowledged: 'bg-(--warning)',
    resolved: 'bg-(--success)',
    closed: 'bg-muted-foreground/60',
  }[kind] ?? 'bg-muted-foreground/40';
  return <span className={cn('size-2 rounded-full', cls)} aria-hidden />;
}
