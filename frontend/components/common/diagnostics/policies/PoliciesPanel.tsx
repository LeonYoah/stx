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
 * 自动巡检策略面板：对齐告警中心通知面板的「搜索 + 状态分段 + TableCard 列表 +
 * 常驻详情栏 / 抽屉 + 居中新建弹窗」结构，关闭 / 切行 / 切视图统一走脏表单拦截。
 * Auto-inspection policy panel: mirrors the monitoring notify panel layout —
 * search + status segments + TableCard list + persistent rail / drawer +
 * centered create dialog — with the same dirty-form interception on close,
 * row switch, and panel exit.
 */
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import {useTranslations} from 'next-intl';
import {History, Plus} from 'lucide-react';
import {toast} from '@/lib/toast';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Badge} from '@/components/arc/badge/badge';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Switch} from '@/components/arc/switch/switch';
import {TableCard} from '@/components/common/layout/TableCard';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import services from '@/lib/services';
import {useLocale} from '@/lib/i18n';
import {resolvePreferredClusterId} from '@/lib/cluster-preference';
import type {
  InspectionAutoPolicy,
  InspectionConditionTemplate,
  DiagnosticsClusterOption,
} from '@/lib/services/diagnostics';
import type {AlertRuleCatalogItem} from '@/lib/services/monitoring/types';
import {cn} from '@/lib/utils';
import {DiagnosticsClusterFilter} from '../DiagnosticsClusterFilter';
import {PolicyFormBody} from './PolicyFormBody';
import {INVALID_REASON_CODES} from './policy-form-utils';
import {describePolicySentence} from './policy-describe';
import {minutesAgo, formatAgo} from '../../monitoring/monitoring-utils';
import {DirtyCloseDialog, useBeforeUnload} from '@/components/common/layout';
import {DetailHost, useDetailNarrow} from '@/components/common/layout';
import {NeutralChip} from '@/components/common/layout';
import layout from '@/components/common/layout/list-detail.module.css';

const PAGE_SIZE = 10;
type PolicyStatusFilter = 'all' | 'enabled' | 'invalid';
// 详情底部「查看触发记录」也要过脏表单拦截，借 discardTarget 挂起跳转。
// The detail footer's "view trigger records" rides the same dirty-form
// interception via a dedicated discard target.
type DiscardTarget = number | '__new__' | 'close' | {recordsFor: number};

export interface PoliciesPanelProps {
  /** 当前工作台所选集群；未选时显示全部策略。Selected workspace cluster; all policies when unset. */
  clusterId?: number;
  clusterOptions: DiagnosticsClusterOption[];
  /** 向工作台上报表单脏态，用于顶层标签切换拦截。Report form dirty state so the workspace can intercept tab switches. */
  onDirtyChange?: (dirty: boolean) => void;
  /** 宿主注入的分段切换控件（工具栏最左，与记录视图共享 layoutGroup）。Host-provided view segment rendered first in the toolbar. */
  viewSwitch?: ReactNode;
  /** URL 深链的目标策略 id；到达时打开详情。Deep-linked policy id; opens the detail when it arrives. */
  focusPolicyId?: number;
  /** 详情开合时回填 policy_id。Writes policy_id while the detail opens/closes. */
  onSelectPolicy?: (policyId: number | null) => void;
  /** 详情底部「查看触发记录」。Detail footer "view trigger records" for this policy. */
  onViewRecords?: (policyId: number) => void;
  /** 工具栏集群筛选变化时回填 cluster_id。Writes cluster_id back through the toolbar filter. */
  onClusterChange?: (clusterId: number | undefined) => void;
}

export function PoliciesPanel({
  clusterId,
  clusterOptions,
  onDirtyChange,
  viewSwitch,
  focusPolicyId,
  onSelectPolicy,
  onViewRecords,
  onClusterChange,
}: PoliciesPanelProps) {
  const t = useTranslations('diagnosticsCenter.autoPolicies');
  const commonT = useTranslations('common');
  const tMonitoring = useTranslations('monitoring');
  const {locale} = useLocale();
  const narrow = useDetailNarrow();

  const [policies, setPolicies] = useState<InspectionAutoPolicy[]>([]);
  const [templates, setTemplates] = useState<InspectionConditionTemplate[]>([]);
  const [ruleCatalog, setRuleCatalog] = useState<AlertRuleCatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editKey, setEditKey] = useState<number | '__new__' | null>(null);
  const [formDirty, setFormDirty] = useState(false);
  const [discardTarget, setDiscardTarget] = useState<DiscardTarget | null>(
    null,
  );
  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<PolicyStatusFilter>('all');
  const searchRef = useRef<HTMLInputElement>(null);
  // 名称飞入来源行：仅点击/回车打开时写入，J/K、头部 ↑↓ 与深链不写。
  // Fly-in origin row: written only on click/Enter open — never by J/K,
  // header arrows or deep links.
  const flyOriginRef = useRef<HTMLElement | null>(null);

  useBeforeUnload(formDirty && editKey !== null);
  useEffect(() => {
    onDirtyChange?.(formDirty && editKey !== null);
  }, [formDirty, editKey, onDirtyChange]);

  const editingPolicy = policies.find((p) => p.id === editKey) ?? null;
  const drawerOpen = editKey !== null && editKey !== '__new__';
  const drawerPolicy = useDrawerItem(editingPolicy);

  // 详情开合时同步 policy_id 深链（仅真实策略 id；__new__ 与关闭都清掉）。
  // Sync the policy_id deep link on detail open/close (only real ids; '__new__'
  // and close both clear it).
  const syncPolicySelection = useCallback(
    (key: number | '__new__' | null) => {
      onSelectPolicy?.(typeof key === 'number' ? key : null);
    },
    [onSelectPolicy],
  );

  // 表单有改动时关闭、切行都走未保存确认。
  // Closing or switching rows with a dirty form goes through the unsaved-changes confirmation.
  const requestEdit = useCallback(
    (key: number | '__new__' | null) => {
      if (key === editKey) {
        return;
      }
      if (formDirty && editKey !== null) {
        setDiscardTarget(key ?? 'close');
        return;
      }
      setEditKey(key);
      setDiscardTarget(null);
      syncPolicySelection(key);
    },
    [formDirty, editKey, syncPolicySelection],
  );

  const discardAndCommit = useCallback(() => {
    const target = discardTarget;
    setDiscardTarget(null);
    setFormDirty(false);
    if (target && typeof target === 'object') {
      onViewRecords?.(target.recordsFor);
      return;
    }
    const next = target && target !== 'close' ? target : null;
    setEditKey(next);
    syncPolicySelection(next);
  }, [discardTarget, onViewRecords, syncPolicySelection]);

  useEffect(() => {
    if (!formDirty) {
      setDiscardTarget(null);
    }
  }, [formDirty]);

  const load = useCallback(async () => {
    const [policiesResult, templatesResult, catalogResult] = await Promise.all([
      services.diagnostics.listAutoPoliciesSafe({page_size: 100}),
      services.diagnostics.listBuiltinConditionTemplatesSafe(),
      services.monitoring.getAlertRuleCatalogSafe(locale),
    ]);
    if (policiesResult.success && policiesResult.data) {
      setPolicies(policiesResult.data.items || []);
      setError(null);
    } else {
      setError(policiesResult.error || t('loadPoliciesError'));
    }
    if (templatesResult.success && templatesResult.data) {
      setTemplates(templatesResult.data);
    }
    if (catalogResult.success && catalogResult.data) {
      setRuleCatalog(catalogResult.data);
    }
    setLoading(false);
  }, [t, locale]);

  useEffect(() => {
    void load();
  }, [load]);

  const retryLoad = useCallback(() => {
    setError(null);
    setLoading(true);
    void load();
  }, [load]);

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

  // policy_id 深链：列表加载完成后打开对应详情（同 report_id / group_id 的
  // deep-link 语义）。requestEdit 内部会处理脏表单拦截，不在列表里的 id
  // 静默忽略。
  // policy_id deep link: once the list loads, open the matching detail (same
  // semantics as report_id / group_id). requestEdit still intercepts dirty
  // forms; ids outside the list are ignored silently.
  const focusedRef = useRef<number | null>(null);
  useEffect(() => {
    if (loading || !focusPolicyId || focusPolicyId === focusedRef.current) {
      return;
    }
    if (policies.some((p) => p.id === focusPolicyId)) {
      focusedRef.current = focusPolicyId;
      requestEdit(focusPolicyId);
    }
  }, [loading, focusPolicyId, policies, requestEdit]);

  // 「查看触发记录」：脏表单先挂起跳转，复用 discardTarget 确认流。
  // "View trigger records": suspend the navigation behind the discard dialog
  // when the form is dirty.
  const requestViewRecords = useCallback(
    (policyId: number) => {
      if (formDirty && editKey !== null) {
        setDiscardTarget({recordsFor: policyId});
        return;
      }
      onViewRecords?.(policyId);
    },
    [formDirty, editKey, onViewRecords],
  );

  const scopeLabel = useCallback(
    (policy: InspectionAutoPolicy) =>
      policy.cluster_id === 0
        ? t('scopeGlobal')
        : (clusterOptions.find((c) => c.cluster_id === policy.cluster_id)
            ?.cluster_name ?? t('scopeCluster', {id: policy.cluster_id})),
    [clusterOptions, t],
  );

  const invalidReasonText = useCallback(
    (policy: InspectionAutoPolicy) =>
      t(
        `invalidReasons.${INVALID_REASON_CODES.has(policy.invalid_reason) ? policy.invalid_reason : 'unknown'}`,
        {detail: policy.invalid_detail ?? ''},
      ),
    [t],
  );

  // 集群筛选先行：选中集群时只看该集群与全局策略，未选时看全部。
  // Cluster scope first: a selected cluster shows its own plus global
  // policies; no selection shows everything.
  const scoped = useMemo(
    () =>
      clusterId == null
        ? policies
        : policies.filter(
            (p) => p.cluster_id === 0 || p.cluster_id === clusterId,
          ),
    [clusterId, policies],
  );

  const statusCounts = useMemo(
    () => ({
      all: scoped.length,
      enabled: scoped.filter((p) => p.enabled).length,
      invalid: scoped.filter((p) => Boolean(p.invalid_reason)).length,
    }),
    [scoped],
  );

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return scoped.filter((p) => {
      if (statusFilter === 'enabled' && !p.enabled) {
        return false;
      }
      if (statusFilter === 'invalid' && !p.invalid_reason) {
        return false;
      }
      if (!q) {
        return true;
      }
      return p.name.toLowerCase().includes(q);
    });
  }, [scoped, query, statusFilter]);

  const [page, setPage] = useState(1);
  // 筛选变化回第一页。Filters reset to page 1.
  useEffect(() => {
    setPage(1);
  }, [query, statusFilter, clusterId]);
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const pageItems = filtered.slice(
    (safePage - 1) * PAGE_SIZE,
    safePage * PAGE_SIZE,
  );

  const activeIndex = editingPolicy
    ? pageItems.findIndex((p) => p.id === editingPolicy.id)
    : -1;
  const detailNav =
    editingPolicy && activeIndex >= 0
      ? {
          index: activeIndex,
          total: pageItems.length,
          onPrev: () =>
            activeIndex > 0 && requestEdit(pageItems[activeIndex - 1].id),
          onNext: () =>
            activeIndex < pageItems.length - 1 &&
            requestEdit(pageItems[activeIndex + 1].id),
        }
      : undefined;

  const onRowKeyDown = (e: React.KeyboardEvent<HTMLElement>, id: number) => {
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
      if (editKey !== id) {
        flyOriginRef.current = e.currentTarget;
      }
      requestEdit(id);
    }
  };

  const togglePolicy = useCallback(
    async (policy: InspectionAutoPolicy, enabled: boolean) => {
      setPolicies((ps) =>
        ps.map((p) => (p.id === policy.id ? {...p, enabled} : p)),
      );
      const result = await services.diagnostics.updateAutoPolicySafe(
        policy.id,
        {enabled},
      );
      if (!result.success) {
        setPolicies((ps) =>
          ps.map((p) => (p.id === policy.id ? {...p, enabled: !enabled} : p)),
        );
        toast.error(result.error || t('updateError'));
      }
    },
    [t],
  );

  const deletePolicy = useCallback(
    async (id: number) => {
      const result = await services.diagnostics.deleteAutoPolicySafe(id);
      if (!result.success) {
        toast.error(result.error || t('deleteError'));
        throw new Error(result.error || t('deleteError'));
      }
      toast.success(t('deleteSuccess'));
      setEditKey(null);
      setFormDirty(false);
      syncPolicySelection(null);
      await load();
    },
    [load, t, syncPolicySelection],
  );

  const defaultClusterId =
    clusterId ??
    resolvePreferredClusterId(
      clusterOptions.map((item) => ({id: item.cluster_id})),
    ) ??
    0;

  const seg = (key: PolicyStatusFilter, label: string, count: number) => ({
    value: key,
    label,
    accessory: <span className={layout.count}>{count}</span>,
  });

  return (
    <div className={layout.page} style={{overflowY: 'auto'}}>
      <div className={layout.toolbar}>
        {viewSwitch}
        <SegmentedControl
          label={t('statusLabel')}
          value={statusFilter}
          onValueChange={(v) => setStatusFilter(v as PolicyStatusFilter)}
          options={[
            seg('all', t('statusAll'), statusCounts.all),
            seg('enabled', t('statusEnabled'), statusCounts.enabled),
            seg('invalid', t('invalidBadge'), statusCounts.invalid),
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
        <DiagnosticsClusterFilter
          value={clusterId}
          clusterOptions={clusterOptions}
          onChange={onClusterChange}
        />
        <div className={layout.toolbarRight}>
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() => requestEdit('__new__')}
            data-testid='policy-create'
          >
            <Plus className='size-3.5' aria-hidden />
            {t('create')}
          </Button>
        </div>
      </div>

      <div className={layout.split}>
        <div className={layout.listCol} data-mcols='policies'>
          {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）。
              Single main card; skeleton/error/empty states are owned by TableCard. */}
          <TableCard
            loading={loading}
            hasData={filtered.length > 0}
            skeletonColumns={5}
            skeletonRows={5}
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
                title={scoped.length === 0 ? t('empty') : t('filteredEmpty')}
                description={
                  scoped.length === 0 ? t('emptyDesc') : t('filteredEmptyDesc')
                }
                action={
                  scoped.length === 0 ? (
                    <Button
                      type='button'
                      size='sm'
                      onClick={() => requestEdit('__new__')}
                    >
                      {t('create')}
                    </Button>
                  ) : query || statusFilter !== 'all' ? (
                    <Button
                      type='button'
                      size='sm'
                      variant='secondary'
                      onClick={() => {
                        setQuery('');
                        setStatusFilter('all');
                      }}
                    >
                      {commonT('clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            }
          >
            <div className={layout.rowHead} aria-hidden>
              <span>{t('nameLabel')}</span>
              <span>{t('clusterLabel')}</span>
              <span>{t('lastTriggeredColumn')}</span>
              <span className={layout.right}>{t('triggers7dColumn')}</span>
              <span className={layout.right}>{t('enabledLabel')}</span>
            </div>
            <div className={layout.tableScroll}>
              {pageItems.map((p) => (
                <div
                  key={p.id}
                  data-mrow={p.id}
                  data-testid={`policy-row-${p.id}`}
                  role='button'
                  tabIndex={0}
                  onClick={(e) => {
                    if (
                      (e.target as HTMLElement).closest(
                        'button, input, [role="switch"], [role="checkbox"]',
                      )
                    ) {
                      return;
                    }
                    if (editKey !== p.id) {
                      flyOriginRef.current = e.currentTarget;
                    }
                    requestEdit(p.id);
                  }}
                  onKeyDown={(e) => onRowKeyDown(e, p.id)}
                  className={cn(
                    layout.row,
                    editKey === p.id && layout.rowActive,
                    !p.enabled && layout.rowMuted,
                  )}
                >
                  <div className={layout.rowName}>
                    <div className={layout.rowNameInline}>
                      <span
                        className={layout.name}
                        title={p.name}
                        data-detail-name
                      >
                        {p.name}
                      </span>
                      {/* 失效徽标 + 原因行：后端只回原因码 + 取值，文案由本侧按 i18n 翻译。
                          Invalid badge + reason line: the backend returns a code + value; the message is rendered via i18n here. */}
                      {p.invalid_reason ? (
                        <Badge tone='danger' size='sm'>
                          {t('invalidBadge')}
                        </Badge>
                      ) : null}
                    </div>
                    {/* 失效原因优先于行为句——先解释「为什么不会跑」再看条件。
                        The invalid reason outranks the behavior sentence —
                        explain why it won't run before the conditions. */}
                    {p.invalid_reason ? (
                      <span
                        className={cn(layout.sub, layout.dangerText)}
                        title={invalidReasonText(p)}
                      >
                        {invalidReasonText(p)}
                      </span>
                    ) : (
                      <span
                        className={layout.sub}
                        title={describePolicySentence(p, {
                          templates,
                          ruleCatalog,
                          clusterOptions,
                          t,
                        })}
                      >
                        {describePolicySentence(p, {
                          templates,
                          ruleCatalog,
                          clusterOptions,
                          t,
                        })}
                      </span>
                    )}
                  </div>
                  <NeutralChip title={scopeLabel(p)}>
                    {scopeLabel(p)}
                  </NeutralChip>
                  {/* 触发统计来自后端聚合（最近一次 + 7 天计数），未触发显示 —。
                      Trigger stats come from backend aggregates (latest + 7d
                      count); untriggered policies show —. */}
                  <span className={layout.sub}>
                    {p.last_triggered_at
                      ? formatAgo(
                          minutesAgo(p.last_triggered_at),
                          tMonitoring,
                        )
                      : '—'}
                  </span>
                  <span className={cn(layout.sub, layout.right)}>
                    {(p.trigger_count_7d ?? 0) > 0
                      ? t('triggers7dValue', {count: p.trigger_count_7d ?? 0})
                      : '—'}
                  </span>
                  <div className={layout.rowActions}>
                    {/* 「查看触发记录」是行级导航操作，不进表单底栏（导航 ≠ 表单提交）。
                        "View trigger records" is a per-row navigation action, not a
                        form action, so it lives on the row rather than the form footer. */}
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      iconOnly
                      aria-label={t('viewRecords')}
                      title={t('viewRecords')}
                      data-testid={`policy-view-records-${p.id}`}
                      onClick={() => requestViewRecords(p.id)}
                    >
                      <History className='size-3.5' aria-hidden />
                    </Button>
                    <Switch
                      checked={p.enabled}
                      onCheckedChange={(v) => void togglePolicy(p, v)}
                      aria-label={t('enabledLabel')}
                    />
                  </div>
                </div>
              ))}
            </div>
          </TableCard>
        </div>

        {/* 编辑走常驻栏/抽屉；新建用居中 Arc 弹窗，两者共用同一 PolicyFormBody。
            Editing uses the rail/drawer; creation is a centered Arc dialog. Both
            containers host the same PolicyFormBody. */}
        {editKey === '__new__' ? (
          <Dialog open onOpenChange={(o) => !o && requestEdit(null)}>
            <DialogContent
              className='w-[min(calc(100vw-2rem),560px)]!'
              title={t('createTitle')}
              bodyClassName='flex! min-h-0! flex-col! overflow-hidden! p-0! h-[min(80dvh,720px)]!'
            >
              <PolicyFormBody
                key='__new__'
                policy={null}
                templates={templates}
                ruleCatalog={ruleCatalog}
                clusterOptions={clusterOptions}
                defaultClusterId={defaultClusterId}
                onSaved={() => {
                  setEditKey(null);
                  setFormDirty(false);
                  void load();
                }}
                onCancel={() => requestEdit(null)}
                onDirtyChange={setFormDirty}
              />
            </DialogContent>
          </Dialog>
        ) : (
          <DetailHost
            open={drawerOpen}
            hasContent={drawerPolicy != null}
            onOpenChange={(o) => !o && requestEdit(null)}
            narrow={narrow}
            title={t('editTitle')}
            meta={drawerPolicy?.name}
            nav={detailNav}
            wide
            drawerWidth='w-[min(92vw,520px)]'
            flyOrigin={flyOriginRef}
            nameSlot='meta'
          >
            {drawerPolicy ? (
              <PolicyFormBody
                key={`${drawerPolicy.id}:${drawerPolicy.updated_at}`}
                policy={drawerPolicy}
                templates={templates}
                ruleCatalog={ruleCatalog}
                clusterOptions={clusterOptions}
                defaultClusterId={defaultClusterId}
                onDelete={
                  editingPolicy
                    ? () => deletePolicy(editingPolicy.id)
                    : undefined
                }
                onSaved={() => {
                  setFormDirty(false);
                  void load();
                }}
                onCancel={() => requestEdit(null)}
                onDirtyChange={setFormDirty}
              />
            ) : null}
          </DetailHost>
        )}
        <DirtyCloseDialog
          open={discardTarget !== null}
          onDiscard={discardAndCommit}
          onKeepEditing={() => setDiscardTarget(null)}
        />
      </div>
    </div>
  );
}
