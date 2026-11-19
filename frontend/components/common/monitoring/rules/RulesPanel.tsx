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
 * Rules panel: cluster-level rules list + detail rail, and the job-level view.
 * 规则面板：集群级规则列表 + 常驻详情栏 / 抽屉，以及作业级视图。
 * 版式遵循 design.md §9.7：状态分段带计数、工具栏一行、边框表格表面、
 * 整行按钮 + 方向键、底部居中批量条（可撤销）、≥1080px 常驻详情栏。
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useLocale, useTranslations} from 'next-intl';
import {ArrowDownWideNarrow, Check, LayoutGrid, List, Undo2, X} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {SeverityChip} from '../status-chips';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Switch} from '@/components/arc/switch/switch';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  AlertNotificationDefaults,
  ClusterRule,
  ClusterRuleLayerRequest,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {formatRuleThreshold} from '../monitoring-utils';
import {DetailHost, useDetailNarrow} from '../detail-host';
import styles from '../monitoring.module.css';
import {RuleDetailPanel} from './RuleDetailPanel';
import {JobRulesPanel} from './JobRulesPanel';

export interface RulesPanelProps {
  /** 深链初始选中规则。Deep-linked rule key to open on mount. */
  initialActiveKey?: string;
  onConfigureNotify?: () => void;
}

type RuleStatusFilter = 'all' | 'enabled' | 'disabled' | 'exceptions';
type RuleSort = 'default' | 'name' | 'severity';

/** 整层替换 PUT 的体：raw 层字段原样回传，调用方只覆盖要改的字段。 / Whole-layer PUT body builder. */
export function layerBody(
  raw: {
    enabled: boolean | null;
    threshold: number | null;
    for_seconds: number | null;
    severity: string | null;
    notify_mode: string | null;
    channel_ids: number[];
    receiver_user_ids: number[];
  },
  patch: Partial<ClusterRuleLayerRequest>,
): ClusterRuleLayerRequest {
  return {
    enabled: raw.enabled,
    threshold: raw.threshold,
    for_seconds: raw.for_seconds,
    severity: (raw.severity as ClusterRuleLayerRequest['severity']) ?? null,
    notify_mode:
      (raw.notify_mode as ClusterRuleLayerRequest['notify_mode']) ?? null,
    channel_ids: raw.channel_ids?.length ? raw.channel_ids : null,
    receiver_user_ids: raw.receiver_user_ids?.length
      ? raw.receiver_user_ids
      : null,
    ...patch,
  };
}

export function RulesPanel({
  initialActiveKey,
  onConfigureNotify,
}: RulesPanelProps) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const narrow = useDetailNarrow();
  const [scope, setScope] = useState('cluster');
  const [statusFilter, setStatusFilter] = useState<RuleStatusFilter>('all');
  const [sort, setSort] = useState<RuleSort>('default');
  const [query, setQuery] = useState('');
  const [activeKey, setActiveKey] = useState<string | null>(
    initialActiveKey ?? null,
  );
  // 关闭拦截三态：dirty 由详情上报；armed 时 footer 显示确认行；
  // pendingKey 记住放弃后要跳转的目标行（null = 关闭）。
  // Dirty-close interception: dirty is reported by the detail; armed swaps
  // the footer for a confirm row; pendingKey remembers where to land on discard.
  const [ruleDirty, setRuleDirty] = useState(false);
  const [discardArmed, setDiscardArmed] = useState(false);
  const [pendingKey, setPendingKey] = useState<string | null | 'close'>(null);
  const view = searchParams.get('view') === 'grid' ? 'grid' : 'list';
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [rules, setRules] = useState<ClusterRule[]>([]);
  const [defaults, setDefaults] = useState<AlertNotificationDefaults | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<Set<string>>(new Set());
  const [undo, setUndo] = useState<{
    enabled: boolean;
    layers: {key: string; raw: ClusterRuleLayerRequest}[];
  } | null>(null);
  const [batchBusy, setBatchBusy] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);

  const load = useCallback(async () => {
    const [ruleData, defData] = await Promise.all([
      services.monitoring.listClusterRules(),
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
    ]);
    setRules(Array.isArray(ruleData) ? ruleData : []);
    setDefaults(defData);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load().catch((err) => {
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    });
  }, [load]);

  // 「/」聚焦搜索，Esc 在搜索框内清空。'/' focuses search; Esc clears inside it.
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

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = rules.filter((r) => {
      if (statusFilter === 'enabled' && !r.enabled) {
        return false;
      }
      if (statusFilter === 'disabled' && r.enabled) {
        return false;
      }
      if (statusFilter === 'exceptions' && r.override_count === 0) {
        return false;
      }
      const name = locale === 'en' ? r.name_en || r.name : r.name;
      if (
        q &&
        !name.toLowerCase().includes(q) &&
        !r.key.toLowerCase().includes(q)
      ) {
        return false;
      }
      return true;
    });
    if (sort === 'name') {
      return [...list].sort((a, b) => {
        const an = locale === 'en' ? a.name_en || a.name : a.name;
        const bn = locale === 'en' ? b.name_en || b.name : b.name;
        return an.localeCompare(bn, locale === 'en' ? 'en' : 'zh');
      });
    }
    if (sort === 'severity') {
      return [...list].sort(
        (a, b) =>
          Number(b.severity === 'critical') - Number(a.severity === 'critical'),
      );
    }
    return list;
  }, [rules, statusFilter, query, locale, sort]);

  const segCounts = useMemo(
    () => ({
      all: rules.length,
      enabled: rules.filter((r) => r.enabled).length,
      disabled: rules.filter((r) => !r.enabled).length,
      exceptions: rules.filter((r) => r.override_count > 0).length,
    }),
    [rules],
  );

  const active = rules.find((r) => r.key === activeKey) ?? null;
  // 关闭动画期间保留最后打开的 rules，抽屉滑出时内容不先消失。
  // Keep the last opened rule mounted while the drawer slides out.
  const drawerRule = useDrawerItem(active);

  // 有未保存草稿时先拦截：详情保持打开并在 footer 露出确认行；
  // 放弃修改才真正落地 pendingKey（换行或关闭）。
  // Dirty drafts intercept close/row-switch first: the detail stays open with a
  // footer confirm row; only discard commits the pending target.
  const requestKey = useCallback(
    (key: string | null) => {
      if (key === activeKey) {
        return;
      }
      if (ruleDirty && activeKey != null) {
        setPendingKey(key);
        setDiscardArmed(true);
        return;
      }
      setActiveKey(key);
      setRuleDirty(false);
      setDiscardArmed(false);
      setPendingKey(null);
    },
    [activeKey, ruleDirty],
  );

  const commitPending = useCallback(() => {
    setActiveKey(pendingKey === 'close' ? null : pendingKey);
    setRuleDirty(false);
    setDiscardArmed(false);
    setPendingKey(null);
  }, [pendingKey]);

  useEffect(() => {
    if (!ruleDirty) {
      setDiscardArmed(false);
      setPendingKey(null);
    }
  }, [ruleDirty]);

  const setView = useCallback(
    (v: string) => {
      const params = new URLSearchParams(searchParams.toString());
      if (v === 'list') {
        params.delete('view');
      } else {
        params.set('view', v);
      }
      router.replace(`${pathname}?${params.toString()}`, {scroll: false});
    },
    [pathname, router, searchParams],
  );

  const toggleSelect = (key: string, on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) {
        next.add(key);
      } else {
        next.delete(key);
      }
      return next;
    });
  };

  /** 行内即时开关：整层 PUT，失败回滚并在行内提示。 Inline enable toggle with rollback. */
  const toggleEnabled = useCallback(
    async (rule: ClusterRule, on: boolean) => {
      if (toggling.has(rule.key)) {
        return;
      }
      setToggling((s) => new Set(s).add(rule.key));
      setError(null);
      const prev = rule.enabled;
      setRules((rs) =>
        rs.map((r) => (r.key === rule.key ? {...r, enabled: on} : r)),
      );
      try {
        const updated = await services.monitoring.updateClusterRule(
          rule.key,
          layerBody(rule.global, {enabled: on}),
        );
        setRules((rs) => rs.map((r) => (r.key === rule.key ? updated : r)));
      } catch (err) {
        setRules((rs) =>
          rs.map((r) => (r.key === rule.key ? {...r, enabled: prev} : r)),
        );
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setToggling((s) => {
          const next = new Set(s);
          next.delete(rule.key);
          return next;
        });
      }
    },
    [toggling],
  );

  /** 批量开关：逐条 PUT 全局层；undo 保存旧层供一键还原。 Batch toggle; undo keeps prior layers. */
  const batchSet = useCallback(
    async (enabled: boolean) => {
      const targets = rules.filter(
        (r) => selected.has(r.key) && r.enabled !== enabled,
      );
      if (targets.length === 0) {
        setSelected(new Set());
        return;
      }
      setBatchBusy(true);
      setError(null);
      const prevLayers = targets.map((r) => ({
        key: r.key,
        raw: layerBody(r.global, {}),
      }));
      try {
        for (const r of targets) {
          const updated = await services.monitoring.updateClusterRule(
            r.key,
            layerBody(r.global, {enabled}),
          );
          setRules((rs) => rs.map((x) => (x.key === r.key ? updated : x)));
        }
        setUndo({enabled, layers: prevLayers});
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        await load().catch(() => undefined);
      } finally {
        setBatchBusy(false);
        setSelected(new Set());
      }
    },
    [rules, selected, load],
  );

  const undoBatch = useCallback(async () => {
    if (!undo) {
      return;
    }
    const layers = undo.layers;
    setUndo(null);
    setBatchBusy(true);
    try {
      for (const {key, raw} of layers) {
        const updated = await services.monitoring.updateClusterRule(key, raw);
        setRules((rs) => rs.map((x) => (x.key === key ? updated : x)));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBatchBusy(false);
    }
  }, [undo]);

  // 「已停用 N 条 · 撤销」约 5 秒后自动消失。
  // The undo bar dismisses itself after ~5s.
  useEffect(() => {
    if (!undo) {
      return;
    }
    const timer = setTimeout(() => setUndo(null), 5000);
    return () => clearTimeout(timer);
  }, [undo]);

  const applySaved = useCallback((updated: ClusterRule) => {
    setRules((rs) => rs.map((r) => (r.key === updated.key ? updated : r)));
  }, []);

  const defaultsMissing = defaults != null && !defaults.configured;

  const retryLoad = useCallback(() => {
    setError(null);
    setLoading(true);
    void load().catch((err) => {
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    });
  }, [load]);

  const activeIndex = active
    ? filtered.findIndex((r) => r.key === active.key)
    : -1;
  const detailNav =
    active && activeIndex >= 0
      ? {
          index: activeIndex,
          total: filtered.length,
          onPrev: () =>
            activeIndex > 0 && requestKey(filtered[activeIndex - 1].key),
          onNext: () =>
            activeIndex < filtered.length - 1 &&
            requestKey(filtered[activeIndex + 1].key),
        }
      : undefined;

  const statusSeg = (key: RuleStatusFilter, label: string, count: number) => ({
    value: key,
    label,
    accessory: <span className={styles.count}>{count}</span>,
  });

  return (
    <div className={styles.page}>
      {defaultsMissing ? (
        <Alert tone='warning' title={t('bannerMissingDefaultNotify')}>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={onConfigureNotify}
          >
            {t('bannerAction')}
          </Button>
        </Alert>
      ) : null}

      <div className={styles.toolbar}>
        <SegmentedControl
          label={t('panelTitle')}
          value={scope}
          onValueChange={setScope}
          options={[
            {value: 'cluster', label: t('scopeCluster')},
            {value: 'job', label: t('scopeJob')},
          ]}
        />
        {scope === 'cluster' ? (
          <>
            <SegmentedControl
              label={t('alertStatusLabel')}
              value={statusFilter}
              onValueChange={(v) => setStatusFilter(v as RuleStatusFilter)}
              options={[
                statusSeg('all', t('chipAll'), segCounts.all),
                statusSeg('enabled', t('chipEnabled'), segCounts.enabled),
                statusSeg('disabled', t('chipDisabled'), segCounts.disabled),
                statusSeg(
                  'exceptions',
                  t('chipExceptions'),
                  segCounts.exceptions,
                ),
              ]}
            />
            <div className={styles.toolbarSearch}>
              <SearchField
                ref={searchRef}
                label={t('searchLabel')}
                value={query}
                onValueChange={setQuery}
                placeholder={t('searchPlaceholder')}
                clearLabel={t('searchClear')}
                onKeyDown={(e) => {
                  if (e.key === 'Escape' && query) {
                    setQuery('');
                  }
                }}
              />
            </div>
            <div className={styles.toolbarRight}>
              <DropdownMenu
                label={t('sortLabel')}
                icon={<ArrowDownWideNarrow className='size-4' aria-hidden />}
                items={(
                  [
                    ['default', t('sortDefault')],
                    ['name', t('sortNameAsc')],
                    ['severity', t('sortSeverityFirst')],
                  ] as const
                ).map(([v, label]) => ({
                  label,
                  icon:
                    sort === v ? (
                      <Check className='size-4' aria-hidden />
                    ) : (
                      <span className='size-4' />
                    ),
                  onSelect: () => setSort(v),
                }))}
              />
              <SegmentedControl
                label={t('viewModeLabel')}
                value={view}
                onValueChange={setView}
                options={[
                  {
                    value: 'list',
                    label: <List className='size-4' aria-hidden />,
                    ariaLabel: t('viewList'),
                  },
                  {
                    value: 'grid',
                    label: <LayoutGrid className='size-4' aria-hidden />,
                    ariaLabel: t('viewGrid'),
                  },
                ]}
              />
            </div>
          </>
        ) : null}
      </div>

      {scope === 'cluster' ? (
        <div className={styles.split}>
          <div className={styles.listCol} data-mcols='rules'>
            {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）；21 条客户端
                规则无分页。Single main card via TableCard; ~21 client-side rules need no pagination. */}
            <TableCard
              loading={loading}
              hasData={filtered.length > 0}
              skeletonColumns={7}
              skeletonRows={8}
              error={error}
              onRetry={retryLoad}
              empty={
                <EmptyState
                  title={t('emptyFiltered')}
                  description={t('emptyFilteredDesc')}
                  action={
                    statusFilter !== 'all' || query ? (
                      <Button
                        type='button'
                        size='sm'
                        variant='secondary'
                        onClick={() => {
                          setStatusFilter('all');
                          setQuery('');
                        }}
                      >
                        {t('clearFilters')}
                      </Button>
                    ) : undefined
                  }
                />
              }
            >
              {view === 'grid' ? (
                <div className={styles.cardsGrid}>
                  {filtered.map((rule) => (
                    <RuleCard
                      key={rule.key}
                      rule={rule}
                      active={activeKey === rule.key}
                      onOpen={() => requestKey(rule.key)}
                      onToggleEnabled={(on) => void toggleEnabled(rule, on)}
                    />
                  ))}
                </div>
              ) : (
                <RuleListView
                  rules={filtered}
                  activeKey={activeKey}
                  selected={selected}
                  toggling={toggling}
                  onOpen={requestKey}
                  onSelect={toggleSelect}
                  onToggleEnabled={toggleEnabled}
                />
              )}
            </TableCard>

            {selected.size > 0 ? (
              <div className={styles.batchBarWrap}>
                <div className={styles.batchBar} role='status'>
                  <span>{t('selected', {count: selected.size})}</span>
                  <span className={styles.batchActions}>
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      disabled={batchBusy}
                      onClick={() => void batchSet(true)}
                    >
                      {t('batchEnable')}
                    </Button>
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      disabled={batchBusy}
                      onClick={() => void batchSet(false)}
                    >
                      {t('batchDisable')}
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      aria-label={t('batchClear')}
                      onClick={() => setSelected(new Set())}
                    >
                      <X className='size-4' aria-hidden />
                    </Button>
                  </span>
                </div>
              </div>
            ) : undo ? (
              <div className={styles.batchBarWrap}>
                <div className={styles.batchBar} role='status'>
                  <span>
                    {t(
                      undo.enabled
                        ? 'batchAppliedEnabled'
                        : 'batchAppliedDisabled',
                      {
                        count: undo.layers.length,
                      },
                    )}
                  </span>
                  <Button
                    type='button'
                    size='sm'
                    variant='ghost'
                    disabled={batchBusy}
                    onClick={() => void undoBatch()}
                  >
                    <Undo2 className='size-4' aria-hidden />
                    {t('undo')}
                  </Button>
                </div>
              </div>
            ) : null}
          </div>

          <DetailHost
            open={active != null}
            hasContent={drawerRule != null}
            onOpenChange={(o) => !o && requestKey(null)}
            narrow={narrow}
            title={
              drawerRule
                ? locale === 'en'
                  ? drawerRule.name_en || drawerRule.name
                  : drawerRule.name
                : ''
            }
            meta={drawerRule?.key}
            nav={detailNav}
            drawerWidth='w-[min(92vw,480px)]'
          >
            {drawerRule ? (
              <RuleDetailPanel
                key={drawerRule.key}
                rule={drawerRule}
                onClose={() => requestKey(null)}
                onSaved={applySaved}
                onToggleEnabled={toggleEnabled}
                onConfigureNotify={onConfigureNotify}
                onDirtyChange={setRuleDirty}
                discardArmed={discardArmed}
                onDiscard={commitPending}
                onKeepEditing={() => {
                  setDiscardArmed(false);
                  setPendingKey(null);
                }}
              />
            ) : null}
          </DetailHost>
        </div>
      ) : (
        <JobRulesPanel />
      )}
    </div>
  );
}

function RuleListView({
  rules,
  activeKey,
  selected,
  toggling,
  onOpen,
  onSelect,
  onToggleEnabled,
}: {
  rules: ClusterRule[];
  activeKey: string | null;
  selected: Set<string>;
  toggling: Set<string>;
  onOpen: (key: string) => void;
  onSelect: (key: string, on: boolean) => void;
  onToggleEnabled: (rule: ClusterRule, on: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const listRef = useRef<HTMLDivElement>(null);

  // 方向键在行间移动焦点，Enter / Space 打开。
  // Arrow keys move focus across rows; Enter / Space opens.
  const onRowKeyDown = (e: React.KeyboardEvent<HTMLElement>) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      listRef.current?.querySelectorAll<HTMLElement>('[data-mrow]') ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      const next = rows[i + (e.key === 'ArrowDown' ? 1 : -1)];
      next?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onOpen(e.currentTarget.dataset.mrow!);
    }
  };

  return (
    <>
      <div className={styles.rowHead} aria-hidden>
        <span />
        <span>{t('colRule')}</span>
        <span>{t('fieldSeverity')}</span>
        <span>{t('colStatus')}</span>
        <span className='hidden lg:block'>{t('colThreshold')}</span>
        <span className={styles.right}>{t('colExceptions')}</span>
        <span className={styles.right}>{t('fieldEnabled')}</span>
      </div>
      <div className={styles.tableScroll} ref={listRef}>
        {rules.map((rule) => {
          const name = locale === 'en' ? rule.name_en || rule.name : rule.name;
          const description =
            locale === 'en'
              ? rule.description_en || rule.description
              : rule.description;
          return (
            <div
              key={rule.key}
              data-testid={`rule-row-${rule.key}`}
              data-mrow={rule.key}
              role='button'
              tabIndex={0}
              onClick={(e) => {
                if (
                  (e.target as HTMLElement).closest(
                    'button, input, [role="checkbox"], [role="switch"], [role="combobox"]',
                  )
                ) {
                  return;
                }
                onOpen(rule.key);
              }}
              onKeyDown={onRowKeyDown}
              className={cn(
                styles.row,
                activeKey === rule.key && styles.rowActive,
                !rule.enabled && styles.rowMuted,
              )}
            >
              <Checkbox
                checked={selected.has(rule.key)}
                onCheckedChange={(v) => onSelect(rule.key, v === true)}
                aria-label={t('selectRule', {name})}
              />
              <div className={styles.rowName}>
                <span className={styles.name} title={name}>
                  {name}
                </span>
                <span className={styles.sub} title={description}>
                  {description}
                </span>
                <span className={cn(styles.sub, styles.mono)}>{rule.key}</span>
              </div>
              <SeverityChip
                severity={rule.severity}
                label={
                  rule.severity === 'critical'
                    ? t('severityCritical')
                    : t('severityWarning')
                }
              />
              <span className={cn(styles.statusCell, styles.statusIdle)}>
                {rule.enabled ? (
                  rule.external_channel_missing ? (
                    <span className={styles.warningText}>
                      <span
                        className={cn(styles.dot, styles.dotSilenced)}
                        aria-hidden
                      />{' '}
                      {t('channelMissing')}
                    </span>
                  ) : (
                    t('statusHealthy')
                  )
                ) : (
                  t('firingNone')
                )}
              </span>
              <span className={cn(styles.sub, styles.num, 'hidden lg:block')}>
                {formatRuleThreshold(rule, t)}
              </span>
              <span className={cn(styles.sub, styles.num, styles.right)}>
                {rule.override_count > 0 ? rule.override_count : '—'}
              </span>
              <div className={styles.rowActions}>
                <Switch
                  checked={rule.enabled}
                  disabled={toggling.has(rule.key)}
                  onCheckedChange={(on) => void onToggleEnabled(rule, on)}
                  aria-label={t('fieldEnabled')}
                  onClick={(e) => e.stopPropagation()}
                />
              </div>
            </div>
          );
        })}
      </div>
    </>
  );
}

function RuleCard({
  rule,
  active,
  onOpen,
  onToggleEnabled,
}: {
  rule: ClusterRule;
  active: boolean;
  onOpen: () => void;
  onToggleEnabled: (on: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const name = locale === 'en' ? rule.name_en || rule.name : rule.name;
  const description =
    locale === 'en'
      ? rule.description_en || rule.description
      : rule.description;
  return (
    <div
      role='button'
      tabIndex={0}
      data-testid={`rule-card-${rule.key}`}
      onClick={(e) => {
        if (
          (e.target as HTMLElement).closest('button, input, [role="switch"]')
        ) {
          return;
        }
        onOpen();
      }}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) {
          return;
        }
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onOpen();
        }
      }}
      className={cn(
        styles.card,
        active && styles.cardActive,
        !rule.enabled && styles.rowMuted,
      )}
    >
      <div className={styles.cardRow}>
        <span className={cn(styles.name, 'min-w-0 flex-1')} title={name}>
          {name}
        </span>
        <SeverityChip
          severity={rule.severity}
          label={
            rule.severity === 'critical'
              ? t('severityCritical')
              : t('severityWarning')
          }
          className='shrink-0'
        />
      </div>
      <p className={cn(styles.sub, styles.clamp2)} title={description}>
        {description}
      </p>
      <div className={styles.cardRow} style={{alignItems: 'center'}}>
        <span className={cn(styles.sub, styles.num, 'min-w-0 truncate')}>
          {formatRuleThreshold(rule, t)}
          {rule.override_count > 0
            ? ` · ${t('exceptions', {count: rule.override_count})}`
            : ''}
        </span>
        <Switch
          checked={rule.enabled}
          onCheckedChange={onToggleEnabled}
          onClick={(e) => e.stopPropagation()}
          aria-label={t('fieldEnabled')}
          className='shrink-0'
        />
      </div>
    </div>
  );
}
