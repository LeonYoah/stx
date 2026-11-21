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
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useLocale, useTranslations} from 'next-intl';
import {ArrowDownWideNarrow, Check, LayoutGrid, List, Undo2} from 'lucide-react';
import {LayoutGroup, motion} from 'motion/react';
import {Alert} from '@/components/arc/alert/alert';
import {SeverityChip} from '../status-chips';
import {NeutralChip} from '@/components/common/layout';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Combobox} from '@/components/arc/combobox/combobox';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Switch} from '@/components/arc/switch/switch';
import {TableCard} from '@/components/common/layout/TableCard';
import {ViewToggle} from '@/components/common/ViewToggle';
import services from '@/lib/services';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {
  AlertNotificationDefaults,
  ClusterRule,
  ClusterRuleLayerRequest,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {formatRuleThreshold, resolveRuleForCluster} from '../monitoring-utils';
import {useViewMorph} from '@/lib/animations/view-morph';
import {DetailHost, useDetailNarrow} from '@/components/common/layout';
import {DirtyCloseDialog} from '@/components/common/layout';
import {BatchBar, usePagedSelection} from '@/components/common/layout';
import styles from '../monitoring.module.css';
import layout from '@/components/common/layout/list-detail.module.css';
import {RuleDetailPanel} from './RuleDetailPanel';
import {
  JobRulesPanel,
  type JobSegCounts,
  type JobStatusFilter,
} from './JobRulesPanel';

export interface RulesPanelProps {
  /** 深链初始选中规则。Deep-linked rule key to open on mount. */
  initialActiveKey?: string;
  onConfigureNotify?: () => void;
}

type RuleStatusFilter = 'all' | 'enabled' | 'disabled' | 'exceptions';
type RuleSort = 'default' | 'name' | 'severity';
type PendingNav =
  | {type: 'key'; key: string | null}
  | {type: 'cluster'; clusterId: number | null};

const PAGE_SIZE = 10;

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
  // 作业级筛选态在此托管，让两级共用同一工具栏行（交互样式对齐集群级）。
  // Job-level filter state lives here so both scopes share one toolbar row.
  const [jobStatusFilter, setJobStatusFilter] = useState<JobStatusFilter>('all');
  const [jobQuery, setJobQuery] = useState('');
  const [jobCounts, setJobCounts] = useState<JobSegCounts>({
    all: 0,
    enabled: 0,
    disabled: 0,
  });
  const [activeKey, setActiveKey] = useState<string | null>(
    initialActiveKey ?? null,
  );
  // 视角切换：null = 全局默认基线，number = 查看并直接编辑该集群的生效配置。
  // Context perspective: null = global baseline; number = view & edit effective rules for one cluster.
  const [contextClusterId, setContextClusterId] = useState<number | null>(null);
  // 未保存拦截：dirty 由详情上报；pendingNav 非空时弹出居中确认弹窗，
  // 确认放弃后才落地目标行或切换集群视角。
  // Dirty-close guard: dirty comes from detail; pendingNav opens the confirm dialog
  // before switching rule or cluster perspective.
  const [ruleDirty, setRuleDirty] = useState(false);
  const [pendingNav, setPendingNav] = useState<PendingNav | null>(null);
  const view = searchParams.get('view') === 'grid' ? 'grid' : 'list';
  const [rules, setRules] = useState<ClusterRule[]>([]);
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [defaults, setDefaults] = useState<AlertNotificationDefaults | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<Set<string>>(new Set());
  const [undo, setUndo] = useState<{
    enabled: boolean;
    layers: {
      key: string;
      clusterId: number | null;
      existed: boolean;
      raw: ClusterRuleLayerRequest | null;
    }[];
  } | null>(null);
  const [batchBusy, setBatchBusy] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  // 名称飞入来源行：仅点击/回车打开时写入，J/K、头部 ↑↓ 与深链不写。
  // Fly-in origin row: written only on click/Enter open — never by J/K,
  // header arrows or deep links.
  const flyOriginRef = useRef<HTMLElement | null>(null);

  const load = useCallback(async () => {
    const [ruleData, defData, clusterData] = await Promise.all([
      services.monitoring.listClusterRules(),
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
      // 后端 ListClustersRequest 限制 size<=100，超过会被 binding 拒绝。
      // Backend caps ListClustersRequest size at 100; larger values fail binding.
      services.cluster.getClusters({current: 1, size: 100}).catch(() => null),
    ]);
    setRules(Array.isArray(ruleData) ? ruleData : []);
    setDefaults(defData);
    setClusters((clusterData?.clusters ?? []) as ClusterInfo[]);
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

  const clusterOverrideCounts = useMemo(() => {
    const map = new Map<number, number>();
    for (const r of rules) {
      for (const ov of r.overrides) {
        map.set(ov.cluster_id, (map.get(ov.cluster_id) ?? 0) + 1);
      }
    }
    return map;
  }, [rules]);

  const contextOptions = useMemo(() => {
    const base = [{value: 'global', label: t('contextGlobal')}];
    const seen = new Set<number>();
    for (const c of clusters) {
      seen.add(c.id);
      const cnt = clusterOverrideCounts.get(c.id) ?? 0;
      base.push({
        value: String(c.id),
        label:
          cnt > 0
            ? t('contextClusterCount', {name: c.name, count: cnt})
            : c.name,
      });
    }
    // 补充不在首页集群列表中但已有专属配置的集群。
    // Include overridden clusters absent from the fetched cluster page.
    for (const [id, cnt] of clusterOverrideCounts.entries()) {
      if (!seen.has(id)) {
        const ovName =
          rules
            .flatMap((r) => r.overrides)
            .find((o) => o.cluster_id === id)?.cluster_name || `#${id}`;
        base.push({
          value: String(id),
          label: t('contextClusterCount', {name: ovName, count: cnt}),
        });
      }
    }
    return base;
  }, [clusters, clusterOverrideCounts, rules, t]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = rules.filter((r) => {
      const resolved = resolveRuleForCluster(r, contextClusterId);
      if (statusFilter === 'enabled' && !resolved.enabled) {
        return false;
      }
      if (statusFilter === 'disabled' && resolved.enabled) {
        return false;
      }
      if (statusFilter === 'exceptions' && !resolved.isOverridden) {
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
          Number(
            resolveRuleForCluster(b, contextClusterId).severity === 'critical',
          ) -
          Number(
            resolveRuleForCluster(a, contextClusterId).severity === 'critical',
          ),
      );
    }
    return list;
  }, [rules, contextClusterId, statusFilter, query, locale, sort]);

  const segCounts = useMemo(() => {
    let enabled = 0;
    let disabled = 0;
    let exceptions = 0;
    for (const r of rules) {
      const resolved = resolveRuleForCluster(r, contextClusterId);
      if (resolved.enabled) {
        enabled += 1;
      } else {
        disabled += 1;
      }
      if (resolved.isOverridden) {
        exceptions += 1;
      }
    }
    return {all: rules.length, enabled, disabled, exceptions};
  }, [rules, contextClusterId]);

  // 客户端分页：目录 ~21 条，分页保持列表区高度稳定。
  // Client-side pagination keeps the list region a stable height.
  const [page, setPage] = useState(1);
  useEffect(() => {
    setPage(1);
  }, [statusFilter, query, sort, view, contextClusterId]);
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const pageItems = filtered.slice(
    (safePage - 1) * PAGE_SIZE,
    safePage * PAGE_SIZE,
  );
  const pageKeys = useMemo(() => pageItems.map((r) => r.key), [pageItems]);
  const {
    ids: selected,
    setIds: setSelected,
    toggle: toggleSelect,
    togglePage: toggleSelectAllPage,
  } = usePagedSelection(pageKeys);

  const active = rules.find((r) => r.key === activeKey) ?? null;
  // 关闭动画期间保留最后打开的 rules，抽屉滑出时内容不先消失。
  // Keep the last opened rule mounted while the drawer slides out.
  const drawerRule = useDrawerItem(active);

  // 有未保存草稿时先拦截并弹出确认框；确认放弃后才落地目标行或切换视角。
  // Dirty drafts intercept close/row-switch/context-switch; only discard commits.
  const requestKey = useCallback(
    (key: string | null) => {
      if (key === activeKey) {
        return;
      }
      if (ruleDirty && activeKey != null) {
        setPendingNav({type: 'key', key});
        return;
      }
      setActiveKey(key);
      setRuleDirty(false);
      setPendingNav(null);
    },
    [activeKey, ruleDirty],
  );

  const requestContextCluster = useCallback(
    (clusterId: number | null) => {
      if (clusterId === contextClusterId) {
        return;
      }
      if (ruleDirty && activeKey != null) {
        setPendingNav({type: 'cluster', clusterId});
        return;
      }
      setContextClusterId(clusterId);
      setUndo(null);
      setRuleDirty(false);
      setPendingNav(null);
    },
    [contextClusterId, ruleDirty, activeKey],
  );

  const commitPending = useCallback(() => {
    if (!pendingNav) {
      return;
    }
    if (pendingNav.type === 'key') {
      setActiveKey(pendingNav.key);
    } else {
      setContextClusterId(pendingNav.clusterId);
      setUndo(null);
    }
    setRuleDirty(false);
    setPendingNav(null);
  }, [pendingNav]);

  useEffect(() => {
    if (!ruleDirty) {
      setPendingNav(null);
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

  /** 行内即时开关：按当前视角写全局层或单集群覆盖层，失败回滚并在行内提示。
   * Inline enable toggle: updates global or cluster-override layer depending on active perspective. */
  const toggleEnabled = useCallback(
    async (rule: ClusterRule, on: boolean) => {
      if (toggling.has(rule.key)) {
        return;
      }
      setToggling((s) => new Set(s).add(rule.key));
      setError(null);
      const snapshotRule = rule;
      if (contextClusterId == null) {
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
            rs.map((r) => (r.key === rule.key ? snapshotRule : r)),
          );
          setError(err instanceof Error ? err.message : String(err));
        } finally {
          setToggling((s) => {
            const next = new Set(s);
            next.delete(rule.key);
            return next;
          });
        }
        return;
      }

      const ov =
        rule.overrides.find((o) => o.cluster_id === contextClusterId) ?? null;
      setRules((rs) =>
        rs.map((r) => {
          if (r.key !== rule.key) {
            return r;
          }
          if (ov) {
            return {
              ...r,
              overrides: r.overrides.map((o) =>
                o.cluster_id === contextClusterId
                  ? {...o, enabled: on, effective: {...o.effective, enabled: on}}
                  : o,
              ),
            };
          }
          return r;
        }),
      );
      try {
        const body = ov ? layerBody(ov, {enabled: on}) : {enabled: on};
        const updated = await services.monitoring.updateClusterRuleOverride(
          rule.key,
          contextClusterId,
          body,
        );
        setRules((rs) => rs.map((r) => (r.key === rule.key ? updated : r)));
      } catch (err) {
        setRules((rs) =>
          rs.map((r) => (r.key === rule.key ? snapshotRule : r)),
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
    [toggling, contextClusterId],
  );

  /** 批量开关：按当前视角更新全局或集群覆盖层；undo 保存旧状态供一键还原。
   * Batch toggle on active perspective; undo restores prior layers (or removes created overrides). */
  const batchSet = useCallback(
    async (enabled: boolean) => {
      const targets = rules.filter(
        (r) =>
          selected.has(r.key) &&
          resolveRuleForCluster(r, contextClusterId).enabled !== enabled,
      );
      if (targets.length === 0) {
        setSelected(new Set());
        return;
      }
      setBatchBusy(true);
      setError(null);
      const prevLayers = targets.map((r) => {
        if (contextClusterId == null) {
          return {
            key: r.key,
            clusterId: null,
            existed: true,
            raw: layerBody(r.global, {}),
          };
        }
        const ov =
          r.overrides.find((o) => o.cluster_id === contextClusterId) ?? null;
        return {
          key: r.key,
          clusterId: contextClusterId,
          existed: ov != null,
          raw: ov ? layerBody(ov, {}) : null,
        };
      });
      try {
        for (const r of targets) {
          if (contextClusterId == null) {
            const updated = await services.monitoring.updateClusterRule(
              r.key,
              layerBody(r.global, {enabled}),
            );
            setRules((rs) => rs.map((x) => (x.key === r.key ? updated : x)));
          } else {
            const ov =
              r.overrides.find((o) => o.cluster_id === contextClusterId) ??
              null;
            const updated =
              await services.monitoring.updateClusterRuleOverride(
                r.key,
                contextClusterId,
                ov ? layerBody(ov, {enabled}) : {enabled},
              );
            setRules((rs) => rs.map((x) => (x.key === r.key ? updated : x)));
          }
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
    [rules, selected, contextClusterId, load],
  );

  const undoBatch = useCallback(async () => {
    if (!undo) {
      return;
    }
    const layers = undo.layers;
    setUndo(null);
    setBatchBusy(true);
    try {
      for (const {key, clusterId, existed, raw} of layers) {
        if (clusterId == null) {
          if (raw) {
            const updated = await services.monitoring.updateClusterRule(
              key,
              raw,
            );
            setRules((rs) => rs.map((x) => (x.key === key ? updated : x)));
          }
        } else if (existed && raw) {
          const updated = await services.monitoring.updateClusterRuleOverride(
            key,
            clusterId,
            raw,
          );
          setRules((rs) => rs.map((x) => (x.key === key ? updated : x)));
        } else {
          await services.monitoring.deleteClusterRuleOverride(key, clusterId);
          const fresh = await services.monitoring.getClusterRule(key);
          setRules((rs) => rs.map((x) => (x.key === key ? fresh : x)));
        }
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
    ? pageItems.findIndex((r) => r.key === active.key)
    : -1;
  const detailNav =
    active && activeIndex >= 0
      ? {
          index: activeIndex,
          total: pageItems.length,
          onPrev: () =>
            activeIndex > 0 && requestKey(pageItems[activeIndex - 1].key),
          onNext: () =>
            activeIndex < pageItems.length - 1 &&
            requestKey(pageItems[activeIndex + 1].key),
        }
      : undefined;

  const statusSeg = (key: RuleStatusFilter, label: string, count: number) => ({
    value: key,
    label,
    accessory: <span className={layout.count}>{count}</span>,
  });

  return (
    <div className={cn(layout.page, layout.pageViewport)}>
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

      <div className={layout.toolbar}>
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
            <div className={cn(layout.filterCtrl, styles.filterWlg)}>
              <Combobox
                label={t('contextLabel')}
                value={contextClusterId != null ? String(contextClusterId) : 'global'}
                onValueChange={(v) => {
                  if (!v || v === 'global') {
                    requestContextCluster(null);
                    return;
                  }
                  const id = Number(v);
                  if (Number.isFinite(id) && id > 0) {
                    requestContextCluster(id);
                  }
                }}
                options={contextOptions}
                placeholder={t('contextGlobal')}
                emptyMessage={t('comboboxEmpty')}
              />
            </div>
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
            <div className={layout.toolbarSearch}>
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
            <div className={layout.toolbarRight}>
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
              <ViewToggle
                label={t('viewModeLabel')}
                value={view}
                onValueChange={setView}
                options={[
                  {
                    value: 'list',
                    icon: List,
                    ariaLabel: t('viewList'),
                  },
                  {
                    value: 'grid',
                    icon: LayoutGrid,
                    ariaLabel: t('viewGrid'),
                  },
                ]}
              />
            </div>
          </>
        ) : (
          <>
            <SegmentedControl
              label={t('alertStatusLabel')}
              value={jobStatusFilter}
              onValueChange={(v) => setJobStatusFilter(v as JobStatusFilter)}
              options={[
                statusSeg('all', t('chipAll'), jobCounts.all),
                statusSeg('enabled', t('chipEnabled'), jobCounts.enabled),
                statusSeg('disabled', t('chipDisabled'), jobCounts.disabled),
              ]}
            />
            <div className={layout.toolbarSearch}>
              <SearchField
                ref={searchRef}
                label={t('searchLabel')}
                value={jobQuery}
                onValueChange={setJobQuery}
                placeholder={t('jobSearchPlaceholder')}
                clearLabel={t('searchClear')}
                onKeyDown={(e) => {
                  if (e.key === 'Escape' && jobQuery) {
                    setJobQuery('');
                  }
                }}
              />
            </div>
          </>
        )}
      </div>

      {scope === 'cluster' ? (
        <div className={layout.split}>
          <div className={layout.listCol} data-mcols='rules'>
            {/* 唯一主卡片：骨架/错误/空态/分页由 TableCard 统一承载（§4.4）。
                Single main card via TableCard: skeleton/error/empty/pagination. */}
            <TableCard
              loading={loading}
              hasData={filtered.length > 0}
              skeletonColumns={7}
              skeletonRows={8}
              error={error}
              onRetry={retryLoad}
              pagination={{
                currentPage: safePage,
                totalPages,
                pageSize: PAGE_SIZE,
                totalItems: filtered.length,
                onPageChange: setPage,
              }}
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
              <LayoutGroup>
                {view === 'grid' ? (
                  <div className={styles.cardsGrid}>
                    {pageItems.map((rule) => (
                      <RuleCard
                        key={rule.key}
                        rule={rule}
                        contextClusterId={contextClusterId}
                        active={activeKey === rule.key}
                        flyOrigin={flyOriginRef}
                        onOpen={() => requestKey(rule.key)}
                        onToggleEnabled={(on) => void toggleEnabled(rule, on)}
                      />
                    ))}
                  </div>
                ) : (
                  <RuleListView
                    rules={pageItems}
                    contextClusterId={contextClusterId}
                    activeKey={activeKey}
                    selected={selected}
                    toggling={toggling}
                    flyOrigin={flyOriginRef}
                    onOpen={requestKey}
                    onSelect={toggleSelect}
                    onSelectAll={toggleSelectAllPage}
                    onToggleEnabled={toggleEnabled}
                  />
                )}
              </LayoutGroup>
            </TableCard>

            {selected.size > 0 ? (
              <BatchBar
                selectedCount={selected.size}
                busy={batchBusy}
                onClear={() => setSelected(new Set())}
              >
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
              </BatchBar>
            ) : undo ? (
              <BatchBar
                selectedCount={0}
                result={t(
                  undo.enabled
                    ? 'batchAppliedEnabled'
                    : 'batchAppliedDisabled',
                  {count: undo.layers.length},
                )}
                onClear={() => setUndo(null)}
              >
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
              </BatchBar>
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
            flyOrigin={flyOriginRef}
            nameSlot='title'
          >
            {drawerRule ? (
              <RuleDetailPanel
                key={`${drawerRule.key}:${contextClusterId ?? 'global'}`}
                rule={drawerRule}
                contextClusterId={contextClusterId}
                clusters={clusters}
                onClose={() => requestKey(null)}
                onSaved={applySaved}
                onToggleEnabled={toggleEnabled}
                onConfigureNotify={onConfigureNotify}
                onSwitchToGlobal={() => requestContextCluster(null)}
                onDirtyChange={setRuleDirty}
              />
            ) : null}
          </DetailHost>
          <DirtyCloseDialog
            open={pendingNav !== null}
            onDiscard={commitPending}
            onKeepEditing={() => setPendingNav(null)}
          />
        </div>
      ) : (
        <JobRulesPanel
          statusFilter={jobStatusFilter}
          query={jobQuery}
          onStatusFilterChange={setJobStatusFilter}
          onQueryChange={setJobQuery}
          onCountsChange={setJobCounts}
        />
      )}
    </div>
  );
}

function RuleListView({
  rules,
  contextClusterId,
  activeKey,
  selected,
  toggling,
  flyOrigin,
  onOpen,
  onSelect,
  onSelectAll,
  onToggleEnabled,
}: {
  rules: ClusterRule[];
  contextClusterId: number | null;
  activeKey: string | null;
  selected: Set<string>;
  toggling: Set<string>;
  /** 名称飞入来源行容器：点击/回车打开时写入当前行。Fly-in origin ref; the opened row is written on click/Enter. */
  flyOrigin: RefObject<HTMLElement | null>;
  onOpen: (key: string) => void;
  onSelect: (key: string, on: boolean) => void;
  onSelectAll: (on: boolean) => void;
  onToggleEnabled: (rule: ClusterRule, on: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const listRef = useRef<HTMLDivElement>(null);
  // 表/卡共用锚点：同名 layoutId 让切换时名称、级别、描述做形变位移。
  // Shared morph anchors so title/severity/description FLIP between row and card.
  const morph = useViewMorph('monitoring-rules');
  const allSelected = rules.length > 0 && rules.every((r) => selected.has(r.key));
  const someSelected = !allSelected && rules.some((r) => selected.has(r.key));

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
      const key = e.currentTarget.dataset.mrow!;
      if (activeKey !== key) {
        flyOrigin.current = e.currentTarget;
      }
      onOpen(key);
    }
  };

  return (
    <>
      <div className={layout.rowHead}>
        <Checkbox
          checked={allSelected ? true : someSelected ? 'indeterminate' : false}
          onCheckedChange={(v) => onSelectAll(v === true)}
          aria-label={t('selectAll')}
        />
        <span>{t('colRule')}</span>
        <span>{t('fieldSeverity')}</span>
        <span>{t('colStatus')}</span>
        <span className='hidden lg:block'>{t('colThreshold')}</span>
        <span className={layout.right}>
          {contextClusterId != null ? t('colSource') : t('colExceptions')}
        </span>
        <span className={layout.right}>{t('fieldEnabled')}</span>
      </div>
      <div className={layout.tableScroll} ref={listRef}>
        {rules.map((rule) => {
          const resolved = resolveRuleForCluster(rule, contextClusterId);
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
                if (activeKey !== rule.key) {
                  flyOrigin.current = e.currentTarget;
                }
                onOpen(rule.key);
              }}
              onKeyDown={onRowKeyDown}
              className={cn(
                layout.row,
                activeKey === rule.key && layout.rowActive,
                !resolved.enabled && layout.rowMuted,
              )}
            >
              <Checkbox
                checked={selected.has(rule.key)}
                onCheckedChange={(v) => onSelect(rule.key, v === true)}
                aria-label={t('selectRule', {name})}
              />
              <div className={layout.rowName}>
                <motion.span
                  layoutId={morph.id(rule.key, 'title')}
                  transition={morph.transition}
                  className={layout.name}
                  title={name}
                  data-detail-name
                >
                  {name}
                </motion.span>
                <motion.span
                  layoutId={morph.id(rule.key, 'desc')}
                  transition={morph.transition}
                  className={layout.sub}
                  title={description}
                >
                  {description}
                </motion.span>
                <span className={cn(layout.sub, styles.mono)}>{rule.key}</span>
              </div>
              <motion.span
                layoutId={morph.id(rule.key, 'sev')}
                transition={morph.transition}
                className='min-w-0'
              >
                <SeverityChip
                  severity={resolved.severity}
                  label={
                    resolved.severity === 'critical'
                      ? t('severityCritical')
                      : t('severityWarning')
                  }
                />
              </motion.span>
              <span className={cn(styles.statusCell, styles.statusIdle)}>
                {resolved.enabled ? (
                  resolved.externalChannelMissing ? (
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
              <span className={cn(layout.sub, styles.num, 'hidden lg:block')}>
                {formatRuleThreshold(
                  {
                    threshold: resolved.threshold,
                    for_seconds: resolved.forSeconds,
                    key: rule.key,
                  },
                  t,
                )}
              </span>
              <span className={cn(layout.sub, styles.num, layout.right)}>
                {contextClusterId != null ? (
                  resolved.isOverridden ? (
                    <NeutralChip>{t('sourceCustom')}</NeutralChip>
                  ) : (
                    t('sourceGlobal')
                  )
                ) : rule.override_count > 0 ? (
                  rule.override_count
                ) : (
                  '—'
                )}
              </span>
              <div className={layout.rowActions}>
                <Switch
                  checked={resolved.enabled}
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
  contextClusterId,
  active,
  flyOrigin,
  onOpen,
  onToggleEnabled,
}: {
  rule: ClusterRule;
  contextClusterId: number | null;
  active: boolean;
  /** 名称飞入来源容器：点击/回车打开时写入本卡片。Fly-in origin ref; this card is written on click/Enter. */
  flyOrigin: RefObject<HTMLElement | null>;
  onOpen: () => void;
  onToggleEnabled: (on: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const morph = useViewMorph('monitoring-rules');
  const resolved = resolveRuleForCluster(rule, contextClusterId);
  const name = locale === 'en' ? rule.name_en || rule.name : rule.name;
  const description =
    locale === 'en'
      ? rule.description_en || rule.description
      : rule.description;
  const metaSuffix =
    contextClusterId != null
      ? resolved.isOverridden
        ? ` · ${t('sourceCustom')}`
        : ''
      : rule.override_count > 0
        ? ` · ${t('exceptions', {count: rule.override_count})}`
        : '';
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
        if (!active) {
          flyOrigin.current = e.currentTarget;
        }
        onOpen();
      }}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) {
          return;
        }
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          if (!active) {
            flyOrigin.current = e.currentTarget;
          }
          onOpen();
        }
      }}
      className={cn(
        styles.card,
        active && styles.cardActive,
        !resolved.enabled && layout.rowMuted,
      )}
    >
      <div className={styles.cardRow}>
        <motion.span
          layoutId={morph.id(rule.key, 'title')}
          transition={morph.transition}
          className={cn(layout.name, 'min-w-0 flex-1')}
          title={name}
          data-detail-name
        >
          {name}
        </motion.span>
        <motion.span
          layoutId={morph.id(rule.key, 'sev')}
          transition={morph.transition}
          className='shrink-0'
        >
          <SeverityChip
            severity={resolved.severity}
            label={
              resolved.severity === 'critical'
                ? t('severityCritical')
                : t('severityWarning')
            }
          />
        </motion.span>
      </div>
      <motion.p
        layoutId={morph.id(rule.key, 'desc')}
        transition={morph.transition}
        className={cn(layout.sub, styles.clamp2)}
        title={description}
      >
        {description}
      </motion.p>
      <div className={styles.cardRow} style={{alignItems: 'center'}}>
        <span className={cn(layout.sub, styles.num, 'min-w-0 truncate')}>
          {formatRuleThreshold(
            {
              threshold: resolved.threshold,
              for_seconds: resolved.forSeconds,
              key: rule.key,
            },
            t,
          )}
          {metaSuffix}
        </span>
        <Switch
          checked={resolved.enabled}
          onCheckedChange={onToggleEnabled}
          onClick={(e) => e.stopPropagation()}
          aria-label={t('fieldEnabled')}
          className='shrink-0'
        />
      </div>
    </div>
  );
}
