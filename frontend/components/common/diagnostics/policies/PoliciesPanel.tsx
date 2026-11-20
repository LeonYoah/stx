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
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Plus} from 'lucide-react';
import {toast} from 'sonner';
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
import {PolicyFormBody} from './PolicyFormBody';
import {INVALID_REASON_CODES} from './policy-form-utils';
// shortcut: 暂直接复用告警中心的详情宿主 / 脏关确认 / 列表样式；告警中心组件的并行改动落地后，统一提升到
// common/layout。
// Temporary direct reuse of the monitoring detail host, dirty-close guard and list styles; promote to common/layout
// once the in-flight monitoring component changes land.
import {
  UnsavedChangesDialog,
  useBeforeUnload,
} from '../../monitoring/dirty-close';
import {DetailHost, useDetailNarrow} from '../../monitoring/detail-host';
import {NeutralChip} from '../../monitoring/status-chips';
import styles from '../../monitoring/monitoring.module.css';

const PAGE_SIZE = 10;
type PolicyStatusFilter = 'all' | 'enabled' | 'invalid';

export interface PoliciesPanelProps {
  /** 当前工作台所选集群；未选时显示全部策略。Selected workspace cluster; all policies when unset. */
  clusterId?: number;
  clusterOptions: DiagnosticsClusterOption[];
  /** 向工作台上报表单脏态，用于顶层标签切换拦截。Report form dirty state so the workspace can intercept tab switches. */
  onDirtyChange?: (dirty: boolean) => void;
}

export function PoliciesPanel({
  clusterId,
  clusterOptions,
  onDirtyChange,
}: PoliciesPanelProps) {
  const t = useTranslations('diagnosticsCenter.autoPolicies');
  const commonT = useTranslations('common');
  const {locale} = useLocale();
  const narrow = useDetailNarrow();

  const [policies, setPolicies] = useState<InspectionAutoPolicy[]>([]);
  const [templates, setTemplates] = useState<InspectionConditionTemplate[]>([]);
  const [ruleCatalog, setRuleCatalog] = useState<AlertRuleCatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editKey, setEditKey] = useState<number | '__new__' | null>(null);
  const [formDirty, setFormDirty] = useState(false);
  const [discardTarget, setDiscardTarget] = useState<
    number | '__new__' | 'close' | null
  >(null);
  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<PolicyStatusFilter>('all');
  const searchRef = useRef<HTMLInputElement>(null);

  useBeforeUnload(formDirty && editKey !== null);
  useEffect(() => {
    onDirtyChange?.(formDirty && editKey !== null);
  }, [formDirty, editKey, onDirtyChange]);

  const editingPolicy = policies.find((p) => p.id === editKey) ?? null;
  const drawerOpen = editKey !== null && editKey !== '__new__';
  const drawerPolicy = useDrawerItem(editingPolicy);

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
    },
    [formDirty, editKey],
  );

  const discardAndCommit = useCallback(() => {
    const target = discardTarget;
    setDiscardTarget(null);
    setFormDirty(false);
    setEditKey(target && target !== 'close' ? target : null);
  }, [discardTarget]);

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
      await load();
    },
    [load, t],
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
    accessory: <span className={styles.count}>{count}</span>,
  });

  return (
    <div className={styles.page} style={{overflowY: 'auto'}}>
      <div className={styles.toolbar}>
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

      <div className={styles.split}>
        <div className={styles.listCol} data-mcols='policies'>
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
            <div className={styles.rowHead} aria-hidden>
              <span>{t('nameLabel')}</span>
              <span>{t('clusterLabel')}</span>
              <span>{t('conditionsLabel')}</span>
              <span>{t('autoBundleColumn')}</span>
              <span className={styles.right}>{t('enabledLabel')}</span>
            </div>
            <div className={styles.tableScroll}>
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
                    requestEdit(p.id);
                  }}
                  onKeyDown={(e) => onRowKeyDown(e, p.id)}
                  className={cn(
                    styles.row,
                    editKey === p.id && styles.rowActive,
                    !p.enabled && styles.rowMuted,
                  )}
                >
                  <div className={styles.rowName}>
                    <div className={styles.rowNameInline}>
                      <span className={styles.name} title={p.name}>
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
                    {p.invalid_reason ? (
                      <span
                        className={cn(styles.sub, styles.dangerText)}
                        title={invalidReasonText(p)}
                      >
                        {invalidReasonText(p)}
                      </span>
                    ) : null}
                  </div>
                  <NeutralChip title={scopeLabel(p)}>
                    {scopeLabel(p)}
                  </NeutralChip>
                  <span className={styles.sub}>
                    {t('conditionsSummary', {
                      count: (p.conditions || []).length,
                      minutes: p.cooldown_minutes,
                    })}
                  </span>
                  <span className={styles.sub}>
                    {p.auto_create_task
                      ? t('autoBundleSummary', {
                          mode: p.auto_start_task
                            ? t('autoBundleModeCreateAndStart')
                            : t('autoBundleModeCreateOnly'),
                        })
                      : '—'}
                  </span>
                  <div className={styles.rowActions}>
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
        <UnsavedChangesDialog
          open={discardTarget !== null}
          onDiscard={discardAndCommit}
          onKeepEditing={() => setDiscardTarget(null)}
        />
      </div>
    </div>
  );
}
