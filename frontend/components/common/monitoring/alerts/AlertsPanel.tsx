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
 * Alerts panel: status segmented control with counts, one-row toolbar
 * (search + popover filters + time range), bordered table surface with
 * whole-row buttons, and a persistent detail rail (drawer below 1080px).
 * 告警面板：带计数的状态分段、单行工具栏（搜索 + Popover 筛选 + 时间）、
 * 边框表格表面（整行按钮 + 方向键）、≥1080px 常驻详情栏（design.md §9.7）。
 * 筛选与页码同步 URL，筛选变化重置 page=1。
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useLocale, useTranslations} from 'next-intl';
import {
  Activity,
  BellOff,
  CheckCheck,
  ChevronRight,
  ExternalLink,
  Lightbulb,
  MoreHorizontal,
  X,
} from 'lucide-react';
import {
  AlertStatusChip,
  DeliveryChip,
  NeutralChip,
  SeverityChip,
} from '../status-chips';
import {Button} from '@/components/arc/button/button';
import {Combobox} from '@/components/arc/combobox/combobox';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {
  CompactTimeFilter,
  formatToDateTimeLocal,
} from '@/components/common/layout/CompactTimeFilter';
import {TableCard} from '@/components/common/layout/TableCard';
import {TroubleshootingMemoryCard} from '@/components/common/troubleshooting/TroubleshootingMemoryCard';
import {SaveMemoryDialog} from '@/components/common/troubleshooting/SaveMemoryDialog';
import {syncAutoCloseReason} from '@/components/common/sync/task-alert-utils';
import services from '@/lib/services';
import {wallTimeToRFC3339} from '@/lib/timezone';
import type {TroubleshootingMemoryEntry} from '@/lib/services/troubleshooting';
import type {
  AlertInstance,
  AlertInstanceFilterParams,
  AlertInstanceStats,
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
  type AlertRowStatus,
  type MonitoringTranslator,
} from '../monitoring-utils';
import {DetailHost, useDetailNarrow} from '../detail-host';
import styles from '../monitoring.module.css';

const PAGE_SIZE = 10;
const SILENCE_MINUTES = 30;
/** Radix Select 不允许空字符串选项值，「全部」用哨兵值再映射回 ''。
    Radix Select rejects empty-string option values; "all" uses a sentinel mapped back to ''. */
const ALL_VALUE = '__all__';

/** 列表筛选的 URL/请求参数键，写回 URL 时只动这组键避免踩到别的 tab 参数。 / Alert filter keys owned by this panel. */
const FILTER_KEYS = [
  'scope',
  'status',
  'severity',
  'cluster_id',
  'q',
  'page',
  'start_time',
  'end_time',
] as const;

/** datetime-local 墙上时间转 RFC3339（与旧告警中心一致的时区处理）。 / Wall-time input value to RFC3339, same timezone handling as the old alerts center. */
function toRFC3339(value: string): string | undefined {
  if (!value) {
    return undefined;
  }
  return wallTimeToRFC3339(value);
}

function StatusCell({
  status,
  t,
}: {
  status: AlertRowStatus;
  t: MonitoringTranslator;
}) {
  // 软色状态徽章沿用旧版设计（status-chips.tsx）。 / Soft status chip keeps the legacy badge design.
  return <AlertStatusChip status={status} label={t(`status.${status}`)} />;
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

function readFilters(
  sp: URLSearchParams,
  initialClusterId?: string | null,
): AlertFilters {
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
 * 应用筛选补丁：筛选变化时重置 page=1，仅翻页则保留当前页码。
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

export function buildAlertInstanceParams(
  f: AlertFilters,
): AlertInstanceFilterParams {
  const params: AlertInstanceFilterParams = {
    page: f.page,
    page_size: PAGE_SIZE,
  };
  if (f.scope === 'cluster' || f.scope === 'job') {
    params.scope = f.scope;
  }
  if (f.status === 'acknowledged' || f.status === 'silenced') {
    params.handling_status = f.status;
    params.status = 'firing';
  } else if (
    f.status === 'firing' ||
    f.status === 'resolved' ||
    f.status === 'closed'
  ) {
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

// 旧 YAML 告警名映射到目录 rule_key 后，迁移前按原名保存的排障经验指纹不再命中；
// 先用目录 key 匹配，落空再用 source_ref.original_alert_name 兜底一次。
// 保存新经验仍写目录 key（见 SaveMemoryDialog initialData）。
// After legacy YAML names were mapped onto catalog rule keys, memories saved under
// the old fingerprint stop matching; fall back to source_ref.original_alert_name
// once when the catalog key misses. New memories still save the catalog key.
export function findAlertMemories(
  alert: AlertInstance,
  title?: string,
): TroubleshootingMemoryEntry[] {
  const matches = services.troubleshooting.findMatchingMemories({
    fingerprint: alert.rule_key || alert.alert_name,
    title: title || alert.alert_name || alert.rule_key,
    target_type: 'alert',
  });
  const original = alert.source_ref?.original_alert_name?.trim();
  if (matches.length > 0 || !original) {
    return matches;
  }
  return services.troubleshooting.findMatchingMemories({
    fingerprint: original,
    title: original,
    target_type: 'alert',
  });
}

interface AlertsPanelProps {
  /** cluster_id 深链初值。Initial cluster filter from the cluster-detail deep link. */
  initialClusterId?: string | null;
  onOpenRule?: (ruleKey: string) => void;
  /** 空态「查看规则」动作：切到规则 tab。Empty-state action that switches to the rules tab. */
  onGoRules?: () => void;
}

export function AlertsPanel({
  initialClusterId,
  onOpenRule,
  onGoRules,
}: AlertsPanelProps) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const narrow = useDetailNarrow();
  const ago = useCallback(
    (iso?: string | null) => formatAgo(minutesAgo(iso), t),
    [t],
  );
  const displayName = useCallback(
    (a: AlertInstance) =>
      locale === 'en' ? a.alert_name_en || a.alert_name : a.alert_name,
    [locale],
  );

  const filters = useMemo(
    () =>
      readFilters(
        new URLSearchParams(searchParams.toString()),
        initialClusterId,
      ),
    [searchParams, initialClusterId],
  );
  const {scope, status, severity, clusterId, query, page, startTime, endTime} =
    filters;

  const [rows, setRows] = useState<AlertInstance[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // 重试不打 URL 参数也能触发重拉（nonce 进查询依赖）。
  // Retry bumps a nonce in the fetch deps instead of touching URL params.
  const [fetchNonce, setFetchNonce] = useState(0);
  // 分段计数走一次无筛选的轻量请求（page_size=1），不为每个分段单独计数。
  // Segment counts come from one unfiltered lightweight request, not one per segment.
  const [segStats, setSegStats] = useState<{
    total: number | null;
    stats: AlertInstanceStats | null;
  }>({
    total: null,
    stats: null,
  });
  const [clusterOptions, setClusterOptions] = useState<
    {value: string; label: string}[]
  >([]);
  const [active, setActive] = useState<AlertInstance | null>(null);
  const [actionBusy, setActionBusy] = useState(false);
  // 排障经验版本号：保存经验后 +1 触发行指示与详情命中重算。
  // Memory version bumped after saving so row indicators and detail matches recompute.
  const [memoriesVersion, setMemoriesVersion] = useState(0);
  const searchRef = useRef<HTMLInputElement>(null);

  const writeFilters = useCallback(
    (patch: Partial<AlertFilters>, resetPage: boolean) => {
      const params = new URLSearchParams(searchParams.toString());
      for (const k of FILTER_KEYS) {
        params.delete(k);
      }
      const next = applyAlertFilterChange(filters, patch, resetPage);
      if (next.scope !== 'all') {
        params.set('scope', next.scope);
      }
      if (next.status) {
        params.set('status', next.status);
      }
      if (next.severity) {
        params.set('severity', next.severity);
      }
      if (next.clusterId) {
        params.set('cluster_id', next.clusterId);
      }
      if (next.query) {
        params.set('q', next.query);
      }
      if (next.startTime) {
        params.set('start_time', next.startTime);
      }
      if (next.endTime) {
        params.set('end_time', next.endTime);
      }
      if (next.page > 1) {
        params.set('page', String(next.page));
      }
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
        if (!alive) {
          return;
        }
        setRows(Array.isArray(data?.alerts) ? data.alerts : []);
        setTotal(data?.total ?? 0);
        setLoading(false);
      })
      .catch((err) => {
        if (!alive) {
          return;
        }
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [filters, fetchNonce]);

  // 状态分段计数（一次无筛选请求）与集群筛选项。
  // Status segment counts (one unfiltered request) and cluster filter options.
  useEffect(() => {
    let alive = true;
    Promise.all([
      services.monitoring
        .getAlertInstances({page: 1, page_size: 1})
        .catch(() => null),
      services.cluster.getClusters({current: 1, size: 200}).catch(() => null),
    ])
      .then(([unfiltered, clusters]) => {
        if (!alive) {
          return;
        }
        setSegStats({
          total: unfiltered?.total ?? 0,
          stats: unfiltered?.stats ?? null,
        });
        const items = clusters?.clusters ?? [];
        setClusterOptions(
          items.map((c: {id: number; name: string}) => ({
            value: String(c.id),
            label: c.name,
          })),
        );
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, []);

  // 「/」聚焦搜索。「/」 focuses the search field.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (
        e.key === '/' &&
        !el?.closest('input, textarea, select, [contenteditable="true"]')
      ) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const patchInstance = useCallback(
    (alertId: string, result: Partial<AlertInstance>) => {
      setRows((prev) =>
        prev.map((a) => (a.alert_id === alertId ? {...a, ...result} : a)),
      );
      setActive((prev) =>
        prev?.alert_id === alertId ? {...prev, ...result} : prev,
      );
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

  const hasActiveFilter = Boolean(
    status || severity || clusterId || scope !== 'all',
  );
  // 任意筛选激活（含时间与搜索词）时给出「清除筛选」空态分支。
  // Any active filter (including time and query) selects the filtered empty-state branch.
  const anyFilterActive = Boolean(
    status ||
    severity ||
    clusterId ||
    query.trim() ||
    startTime ||
    endTime ||
    scope !== 'all',
  );
  const clearAllFilters = useCallback(
    () =>
      writeFilters(
        {
          status: '',
          severity: '',
          clusterId: '',
          query: '',
          startTime: '',
          endTime: '',
          scope: 'all',
        },
        true,
      ),
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
          startTime: formatToDateTimeLocal(
            new Date(now.getTime() - minutes * 60_000),
          ),
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
      if (findAlertMemories(a, displayName(a)).length > 0) {
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

  const drawerAlert = useDrawerItem(active);

  const activeIndex = active
    ? pageItems.findIndex((a) => a.alert_id === active.alert_id)
    : -1;
  const detailNav =
    active && activeIndex >= 0
      ? {
          index: activeIndex,
          total: pageItems.length,
          onPrev: () =>
            activeIndex > 0 && setActive(pageItems[activeIndex - 1]),
          onNext: () =>
            activeIndex < pageItems.length - 1 &&
            setActive(pageItems[activeIndex + 1]),
        }
      : undefined;

  const onRowKeyDown = (
    e: React.KeyboardEvent<HTMLElement>,
    alert: AlertInstance,
  ) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      e.currentTarget.parentElement?.querySelectorAll<HTMLElement>(
        '[data-mrow]',
      ) ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i + (e.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      setActive(alert);
    }
  };

  const segCount = (n?: number) =>
    n == null ? undefined : <span className={styles.count}>{n}</span>;

  const retry = useCallback(() => {
    setError(null);
    setFetchNonce((n) => n + 1);
  }, []);

  const statsCells: {
    label: string;
    value: number | null;
    active: boolean;
    onClick: () => void;
  }[] = [
    {
      label: t('chipAll'),
      value: segStats.total,
      active: !status,
      onClick: () => writeFilters({status: ''}, true),
    },
    {
      label: t('status.firing'),
      value: segStats.stats?.firing ?? null,
      active: status === 'firing',
      onClick: () => writeFilters({status: 'firing'}, true),
    },
    {
      label: t('status.resolved'),
      value: segStats.stats?.resolved ?? null,
      active: status === 'resolved',
      onClick: () => writeFilters({status: 'resolved'}, true),
    },
  ];

  return (
    <div className={styles.page}>
      {/* 汇总指标合并为单框 + 内分割线，格点击复用状态筛选（§9.7 防框堆叠）。
          Summary metrics merge into one frame with inner dividers; cells reuse the status filter. */}
      <div
        className={styles.statsBar}
        role='group'
        aria-label={t('alertStatusLabel')}
      >
        {statsCells.map((m) => (
          <button
            key={m.label}
            type='button'
            aria-pressed={m.active}
            onClick={m.onClick}
            className={cn(styles.statCell, m.active && styles.statCellActive)}
          >
            <span className={styles.statLabel}>{m.label}</span>
            <span className={styles.statValue}>{m.value ?? '–'}</span>
          </button>
        ))}
      </div>

      <div className={styles.toolbar}>
        {/* 状态分段：计数来自一次无筛选请求；已确认/已静默无廉价计数，只显示文字。
            Status segments: counts from one unfiltered request; acknowledged /
            silenced have no cheap count and show text only. */}
        <SegmentedControl
          label={t('alertStatusLabel')}
          value={status || 'all'}
          onValueChange={(v) =>
            writeFilters({status: v === 'all' ? '' : v}, true)
          }
          options={[
            {
              value: 'all',
              label: t('chipAll'),
              accessory: segCount(segStats.total ?? undefined),
            },
            {
              value: 'firing',
              label: t('status.firing'),
              accessory: segCount(segStats.stats?.firing),
            },
            {value: 'acknowledged', label: t('status.acknowledged')},
            {value: 'silenced', label: t('status.silenced')},
            {
              value: 'resolved',
              label: t('status.resolved'),
              accessory: segCount(segStats.stats?.resolved),
            },
          ]}
        />
        <div className={styles.toolbarSearch}>
          <SearchField
            ref={searchRef}
            label={t('alertSearchLabel')}
            value={query}
            onValueChange={(v) => writeFilters({query: v}, true)}
            placeholder={t('alertSearchPlaceholder')}
            clearLabel={t('searchClear')}
            onKeyDown={(e) => {
              if (e.key === 'Escape' && query) {
                writeFilters({query: ''}, true);
              }
            }}
          />
        </div>
        {/* 范围 / 级别是小枚举用 Select；集群可超过 10 项用可搜索 Combobox（§18 选型）。
            Scope/severity are small enums (Select); clusters can exceed 10 so they
            get the searchable Combobox (§18 selection rules). */}
        <div className={cn(styles.filterCtrl, styles.filterWsm)}>
          <Select
            label={t('alertScopeLabel')}
            value={scope}
            onValueChange={(v) => writeFilters({scope: v}, true)}
            displayValue={scope === 'all' ? t('alertScopeLabel') : undefined}
            options={[
              {value: 'all', label: t('scopeAll')},
              {value: 'cluster', label: t('scopeCluster')},
              {value: 'job', label: t('scopeJob')},
            ]}
          />
        </div>
        <div className={cn(styles.filterCtrl, styles.filterWsm)}>
          <Select
            label={t('fieldSeverity')}
            value={severity || ALL_VALUE}
            onValueChange={(v) =>
              writeFilters({severity: v === ALL_VALUE ? '' : v}, true)
            }
            displayValue={!severity ? t('fieldSeverity') : undefined}
            options={[
              {value: ALL_VALUE, label: t('scopeAll')},
              {value: 'warning', label: t('severityWarning')},
              {value: 'critical', label: t('severityCritical')},
            ]}
          />
        </div>
        <div className={cn(styles.filterCtrl, styles.filterW)}>
          <Combobox
            label={t('colCluster')}
            value={clusterId || ALL_VALUE}
            onValueChange={(v) =>
              writeFilters(
                {clusterId: v === ALL_VALUE || v === '' ? '' : v},
                true,
              )
            }
            displayValue={!clusterId ? t('colCluster') : undefined}
            placeholder={t('colCluster')}
            options={[
              {value: ALL_VALUE, label: t('scopeAll')},
              ...clusterOptions,
            ]}
          />
        </div>
        {/* 时间范围：预设 + 自定义起止，改动重置 page=1 并落 URL（旧告警中心同款）。
            Time range: presets and custom bounds, resets page=1 and lands in the URL. */}
        <CompactTimeFilter
          className='border-t-0 pt-0'
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
        {hasActiveFilter ? (
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={clearAllFilters}
          >
            {t('clearFilters')}
          </Button>
        ) : null}
      </div>

      <div className={styles.split}>
        <div className={styles.listCol} data-mcols='alerts'>
          {/* 唯一主卡片：加载条/骨架/错误/空态/底部分页都由 TableCard 统一承载（§4.4）。
              The single main card: TableCard owns bar/skeleton/error/empty/pagination. */}
          <TableCard
            loading={loading}
            hasData={pageItems.length > 0}
            skeletonColumns={7}
            skeletonRows={8}
            error={error}
            onRetry={retry}
            empty={
              // 无告警与「有筛选无结果」两种空态文案不同；后者给清除筛选动作。
              // Two empty states: no alerts at all vs. filters yielding nothing.
              <EmptyState
                title={
                  anyFilterActive
                    ? t('emptyFilteredTitle')
                    : t('emptyAlertsTitle')
                }
                description={
                  anyFilterActive
                    ? t('emptyFilteredDesc')
                    : t('emptyAlertsDesc')
                }
                action={
                  anyFilterActive ? (
                    <Button
                      type='button'
                      variant='secondary'
                      size='sm'
                      onClick={clearAllFilters}
                    >
                      {t('clearFilters')}
                    </Button>
                  ) : (
                    <Button type='button' size='sm' onClick={onGoRules}>
                      {t('emptyAlertsAction')}
                    </Button>
                  )
                }
              />
            }
            pagination={{
              currentPage: page,
              totalPages,
              pageSize: PAGE_SIZE,
              totalItems: total,
              onPageChange: (p) => writeFilters({page: p}, false),
            }}
          >
            <div className={cn(styles.rowHead, styles.tableArea)} aria-hidden>
              <span>{t('colStatus')}</span>
              <span>{t('colAlert')}</span>
              <span>{t('fieldSeverity')}</span>
              <span>{t('colScope')}</span>
              <span>{t('colCluster')}</span>
              <span>{t('colTime')}</span>
              <span />
            </div>
            <div className={cn(styles.tableScroll, styles.tableArea)}>
              {pageItems.map((alert) => {
                const rowStatus = alertRowStatus(alert);
                return (
                  <div
                    key={alert.alert_id}
                    data-testid={`alert-row-${alert.alert_id}`}
                    data-mrow={alert.alert_id}
                    role='button'
                    tabIndex={0}
                    onClick={(e) => {
                      if (
                        (e.target as HTMLElement).closest(
                          'button, a, input, select, [role="menu"], [role="checkbox"]',
                        )
                      ) {
                        return;
                      }
                      setActive(alert);
                    }}
                    onKeyDown={(e) => onRowKeyDown(e, alert)}
                    className={cn(
                      styles.row,
                      active?.alert_id === alert.alert_id && styles.rowActive,
                    )}
                  >
                    <StatusCell status={rowStatus} t={t} />
                    <div className={styles.rowName}>
                      <span className={styles.rowNameInline}>
                        <span
                          className={styles.name}
                          title={displayName(alert)}
                        >
                          {displayName(alert)}
                        </span>
                        {alertMemoryMap.get(alert.alert_id) ? (
                          <Lightbulb
                            className={cn(
                              'size-3.5 shrink-0',
                              styles.accentText,
                            )}
                            aria-hidden
                          />
                        ) : null}
                      </span>
                      <span className={cn(styles.sub, styles.mono)}>
                        {alert.rule_key}
                      </span>
                    </div>
                    <SeverityChip
                      severity={alert.severity}
                      label={
                        isCritical(alert)
                          ? t('severityCritical')
                          : t('severityWarning')
                      }
                    />
                    <NeutralChip>
                      {alertScopeOf(alert) === 'cluster'
                        ? t('scopeCluster')
                        : t('scopeJob')}
                    </NeutralChip>
                    <span className={styles.sub} title={alert.cluster_name}>
                      {alert.cluster_name}
                    </span>
                    <span className={cn(styles.sub, styles.num, styles.nowrap)}>
                      {ago(alert.firing_at || alert.created_at)}
                    </span>
                    <div className={styles.rowActions}>
                      <DropdownMenu
                        trigger={
                          <Button
                            type='button'
                            variant='ghost'
                            size='sm'
                            iconOnly
                            aria-label={t('moreActions')}
                          >
                            <MoreHorizontal className='size-4' aria-hidden />
                          </Button>
                        }
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
                  </div>
                );
              })}
            </div>
            <div className={styles.mobileList}>
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
          </TableCard>
        </div>

        <DetailHost
          open={active != null}
          hasContent={drawerAlert != null}
          onOpenChange={(o) => !o && setActive(null)}
          narrow={narrow}
          title={drawerAlert ? displayName(drawerAlert) : ''}
          meta={drawerAlert?.cluster_name}
          nav={detailNav}
        >
          {drawerAlert ? (
            <AlertDetailBody
              key={drawerAlert.alert_id}
              alert={drawerAlert}
              ago={ago}
              memoriesVersion={memoriesVersion}
              onMemorySaved={() => setMemoriesVersion((v) => v + 1)}
              onNavigateDiagnostics={navigateToDiagnostics}
              onOpenRule={onOpenRule}
              onAck={() => void runAction(drawerAlert, 'ack')}
              onSilence={() => void runAction(drawerAlert, 'silence')}
            />
          ) : null}
        </DetailHost>
      </div>
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
    <button type='button' className={styles.mobileCard} onClick={onOpen}>
      <span className={styles.rowNameInline}>
        <StatusCell status={alertRowStatus(alert)} t={t} />
        <span className={cn(styles.name, 'min-w-0 flex-1')}>
          {locale === 'en'
            ? alert.alert_name_en || alert.alert_name
            : alert.alert_name}
        </span>
        {hasMemory ? (
          <Lightbulb
            className={cn('size-3.5 shrink-0', styles.accentText)}
            aria-hidden
          />
        ) : null}
        <ChevronRight
          className={cn('size-4 shrink-0', styles.muted)}
          aria-hidden
        />
      </span>
      <span className={styles.mobileCardMeta}>
        <SeverityChip
          severity={alert.severity}
          label={
            isCritical(alert) ? t('severityCritical') : t('severityWarning')
          }
        />
        <NeutralChip>
          {alertScopeOf(alert) === 'cluster'
            ? t('scopeCluster')
            : t('scopeJob')}
        </NeutralChip>
        <span className={styles.truncate}>{alert.cluster_name}</span>
        <span className={cn(styles.nowrap, styles.num)}>
          {ago(alert.firing_at || alert.created_at)}
        </span>
      </span>
    </button>
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
  const alertDisplayName =
    locale === 'en'
      ? alert.alert_name_en || alert.alert_name
      : alert.alert_name;
  const router = useRouter();
  const rowStatus = alertRowStatus(alert);
  const [rule, setRule] = useState<ClusterRule | null>(null);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [memoryDialogOpen, setMemoryDialogOpen] = useState(false);

  // 命中排障经验：目录 key 优先，旧 YAML 指纹经 findAlertMemories 兜底。
  // Memory match: catalog key first, legacy YAML fingerprint via findAlertMemories.
  const matchedMemories = useMemo(
    () => findAlertMemories(alert, alertDisplayName),
    // findMatchingMemories 读 localStorage，版本号是唯一的重算信号。
    // The lookup reads localStorage; the version bump is the only recompute signal.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [alert, memoriesVersion],
  );
  const primaryMemory: TroubleshootingMemoryEntry | null =
    matchedMemories[0] ?? null;

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
      .listNotificationDeliveries({
        cluster_id: alert.cluster_id || undefined,
        page: 1,
        page_size: 50,
      })
      .then((d) => {
        if (!alive) {
          return;
        }
        setDeliveries(
          (d?.deliveries ?? [])
            .filter((x) => x.alert_id === alert.alert_id)
            .slice(0, 10),
        );
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [alert]);

  const override = rule?.overrides?.find(
    (o) => String(o.cluster_id) === alert.cluster_id,
  );
  const timelineEvents = useMemo(() => {
    const evs: {key: string; at?: string | null; note?: string}[] = [
      {key: 'triggered', at: alert.firing_at || alert.created_at},
    ];
    for (const d of deliveries) {
      evs.push({
        key: 'notified',
        at: d.sent_at || d.created_at,
        note: d.channel_name,
      });
    }
    if (alert.acknowledged_at) {
      evs.push({
        key: 'acknowledged',
        at: alert.acknowledged_at,
        note: alert.acknowledged_by,
      });
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
    <div className={styles.detailCol}>
      <div className={styles.metaBand}>
        <SeverityChip
          severity={alert.severity}
          label={
            isCritical(alert) ? t('severityCritical') : t('severityWarning')
          }
        />
        <StatusCell status={rowStatus} t={t} />
      </div>

      <div className={styles.detailScroll}>
        {alert.summary || alert.description ? (
          <JsonOrText
            text={alert.summary || alert.description || ''}
            className={styles.descText}
          />
        ) : null}
        {alert.description && alert.description !== alert.summary ? (
          <JsonOrText text={alert.description} className={styles.hintBox} />
        ) : null}

        {alert.rule_key ? (
          <div className={styles.groupInner}>
            <div className={styles.kvRow}>
              <span className={styles.kvKey}>{t('hitRule')}</span>
              <span className={styles.kvVal}>
                <button
                  type='button'
                  className={styles.linkBtn}
                  style={{fontSize: 'inherit', fontWeight: 500}}
                  onClick={() => onOpenRule?.(alert.rule_key)}
                >
                  {alert.rule_key}
                </button>
                {rule ? (
                  <span className={styles.muted}>
                    {' · '}
                    {override ? t('fromException') : t('fromGlobal')}
                    {' · '}
                    <span className={styles.nowrap}>
                      {formatRuleThreshold(rule, t)}
                    </span>
                  </span>
                ) : null}
              </span>
            </div>
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
        <div className={styles.sectionStack} style={{gap: 8}}>
          <span className={styles.sectionTitle}>
            {tMc('alerts.contextAndTopology')}
          </span>
          <div className={styles.groupInner}>
            <div className={styles.kvRow}>
              <span className={styles.kvKey}>
                {tMc('alerts.belongCluster')}
              </span>
              <span className={styles.kvVal}>
                {alert.cluster_name || alert.cluster_id || '-'}
              </span>
            </div>
            {alert.source_ref?.hostname ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>
                  {tMc('alerts.affectedHost')}
                </span>
                <span className={cn(styles.kvVal, styles.mono)}>
                  {alert.source_ref.hostname}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.process_name ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>
                  {tMc('alerts.associatedProcess')}
                </span>
                <span className={cn(styles.kvVal, styles.mono)}>
                  {alert.source_ref.process_name}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.event_id ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>
                  {tMc('alerts.associatedEventId')}
                </span>
                <span className={cn(styles.kvVal, styles.mono)}>
                  #{alert.source_ref.event_id}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.original_alert_name ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>{t('originalAlertName')}</span>
                <span className={cn(styles.kvVal, styles.mono)}>
                  {alert.source_ref.original_alert_name}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.task_id ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>
                  {tAlert('policyTaskColumn')}
                </span>
                <span
                  className={styles.kvVal}
                  title={alert.source_ref.task_name}
                >
                  {alert.source_ref.task_name || `#${alert.source_ref.task_id}`}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.job_id ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>{tAlert('jobLabel')}</span>
                <span className={cn(styles.kvVal, styles.mono, styles.nowrap)}>
                  #{alert.source_ref.job_id}
                  {alert.source_ref.engine_job_id
                    ? ` · ${alert.source_ref.engine_job_id}`
                    : ''}
                </span>
              </div>
            ) : null}
            {syncAutoCloseReason(alert.source_ref?.resolve_reason) ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>
                  {tAlert('closeReasonLabel')}
                </span>
                <span className={styles.kvVal}>
                  {tAlert(
                    `closeReason_${syncAutoCloseReason(alert.source_ref?.resolve_reason)}`,
                  )}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.reminders_paused ? (
              <div className={styles.kvRow}>
                <span className={styles.warningText}>
                  {tAlert('remindersPausedHint')}
                </span>
              </div>
            ) : null}
            {(alert.source_ref?.occurrence_count ?? 0) > 0 ? (
              <div className={styles.kvRow}>
                <span className={styles.kvKey}>
                  {tAlert('occurrenceLabel')}
                </span>
                <span className={styles.kvVal}>
                  {tAlert('occurrences', {
                    count: alert.source_ref?.occurrence_count ?? 0,
                  })}
                </span>
              </div>
            ) : null}
            {alert.source_ref?.link ? (
              <div className={styles.kvRow}>
                <span className={styles.detailFootGrow} />
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  data-testid='alert-open-job'
                  onClick={() =>
                    router.push(alert.source_ref?.link || '/workbench')
                  }
                >
                  <ExternalLink className='size-3.5' aria-hidden />
                  {alert.source_ref.job_id
                    ? tAlert('openJob')
                    : tAlert('openTask')}
                </Button>
              </div>
            ) : null}
          </div>
        </div>

        {/* 处理记录：确认时间 / 静默至 / 最新备注。Handling record. */}
        {alert.silenced_until || alert.acknowledged_at || alert.latest_note ? (
          <div className={styles.sectionStack} style={{gap: 8}}>
            <span className={styles.sectionTitle}>
              {tMc('alerts.handlingNotes')}
            </span>
            <div className={styles.groupInner}>
              {alert.silenced_until ? (
                <div className={styles.kvRow}>
                  <span className={styles.kvKey}>
                    {tMc('alerts.silencedUntilLabel')}
                  </span>
                  <span className={cn(styles.kvVal, styles.mono)}>
                    {ago(alert.silenced_until)}
                  </span>
                </div>
              ) : null}
              {alert.acknowledged_at ? (
                <div className={styles.kvRow}>
                  <span className={styles.kvKey}>
                    {tMc('alerts.acknowledgedTimeLabel')}
                  </span>
                  <span className={cn(styles.kvVal, styles.mono)}>
                    {ago(alert.acknowledged_at)}
                    {alert.acknowledged_by ? ` · ${alert.acknowledged_by}` : ''}
                  </span>
                </div>
              ) : null}
              {alert.latest_note ? (
                <div
                  className={styles.kvRow}
                  style={{alignItems: 'flex-start'}}
                >
                  <span className={styles.kvKey}>
                    {tMc('alerts.latestNoteLabel')}
                  </span>
                  <span className={cn(styles.kvVal, 'whitespace-normal')}>
                    {alert.latest_note}
                  </span>
                </div>
              ) : null}
            </div>
          </div>
        ) : null}

        {/* 前往诊断：沿旧告警中心的 /diagnostics 参数拼法。
            Diagnostics deep link, same query shape as the old alerts center. */}
        <div>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() => onNavigateDiagnostics(alert)}
          >
            <Activity className='size-3.5' aria-hidden />
            {tMc('alerts.navigateToDiagnostics')}
            <ExternalLink className='size-3' aria-hidden />
          </Button>
        </div>

        <div className={styles.sectionStack} style={{gap: 8}}>
          <span className={styles.sectionTitle}>{t('timeline')}</span>
          <ol className={styles.timeline}>
            {timelineEvents.map((ev, i) => (
              <li key={i} className={styles.tlItem}>
                <span className={styles.tlDot}>
                  <TimelineDot kind={ev.key} />
                </span>
                <span className={styles.tlHead}>
                  <span>{t(`timelineKind.${ev.key}`)}</span>
                  <span className={cn(styles.sub, styles.num, styles.nowrap)}>
                    {ago(ev.at)}
                  </span>
                </span>
                {ev.note ? (
                  <div className={styles.tlNote}>{ev.note}</div>
                ) : null}
              </li>
            ))}
          </ol>
        </div>

        <div className={styles.sectionStack} style={{gap: 8}}>
          <span className={styles.sectionTitle}>{t('deliveries')}</span>
          {deliveries.length === 0 ? (
            <p className={styles.hintBox}>{t('deliveriesEmpty')}</p>
          ) : (
            <ul className={styles.lineList}>
              {deliveries.map((d) => (
                <li key={d.id} className={styles.lineItem}>
                  <span
                    className={cn(styles.truncate, styles.mono, styles.xsText)}
                  >
                    {d.channel_name ?? d.channel_id}
                  </span>
                  <span className='flex shrink-0 items-center gap-2'>
                    <DeliveryChip
                      status={d.status}
                      label={t(`delivery.${d.status}`)}
                    />
                    <span className={cn(styles.sub, styles.num, styles.nowrap)}>
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
            tags:
              primaryMemory?.tags ||
              ['alert', alert.source_type || 'metric'].filter(Boolean),
            cluster_id: alert.cluster_id,
            cluster_name: alert.cluster_name,
            author: primaryMemory?.author,
          }}
          onSaved={onMemorySaved}
        />
      </div>

      {/* 处理动作钉在底部操作栏（仅触发中可确认/静默）。
          Handling actions pinned to the bottom bar for firing alerts. */}
      {rowStatus === 'firing' ? (
        <div className={styles.detailFoot}>
          <Button type='button' size='sm' onClick={onAck}>
            <CheckCheck className='size-3.5' aria-hidden />
            {t('actionAck')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={onSilence}
          >
            <BellOff className='size-3.5' aria-hidden />
            {t('actionSilence')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function TimelineDot({kind}: {kind: string}) {
  const cls =
    {
      triggered: styles.dotFiring,
      notified: styles.dotSilenced,
      acknowledged: styles.dotAcknowledged,
      resolved: styles.dotResolved,
      closed: styles.dotClosed,
    }[kind] ?? styles.dotClosed;
  return <span className={cn(styles.tlDotInner, cls)} aria-hidden />;
}

// 可用性告警的 summary/description 可能是原始 JSON 负载（含被截断、缺开头
// 花括号的片段）：能解析就美化，解析不了但明显是 JSON 片段也按代码块排版。
// Raw JSON payloads — including truncated fragments — render as a code block.
function JsonOrText({text, className}: {text: string; className: string}) {
  const pretty = useMemo(() => {
    const trimmed = text.trim();
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch {
      return /"[^"]*"\s*:/.test(trimmed) ? trimmed : null;
    }
  }, [text]);
  return pretty != null ? (
    <pre className={styles.codeBlock}>{pretty}</pre>
  ) : (
    <p className={className}>{text}</p>
  );
}
