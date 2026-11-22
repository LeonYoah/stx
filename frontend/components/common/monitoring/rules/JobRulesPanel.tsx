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
 * Job-level rules: sync_job alert policies list + detail rail hosting the shared
 * TaskAlertForm (same save API as task settings). 版式同集群级列表（§9.7）。
 * 作业级规则：sync_job 策略列表 + 常驻详情栏 / 抽屉，承载 TaskAlertForm，
 * 保存契约与任务设置页一致（PUT /sync/tasks/:id/alert）。
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Undo2} from 'lucide-react';
import {NeutralChip, SoftChip, TruncatedText} from '@/components/common/layout';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Switch} from '@/components/arc/switch/switch';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  AlertPolicy,
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import type {
  SyncTaskAlertConfig,
  SyncTaskAlertRequest,
} from '@/lib/services/sync';
import {TaskAlertForm} from '@/components/common/sync/TaskAlertForm';
import {cn} from '@/lib/utils';
import {DirtyCloseDialog, useBeforeUnload} from '@/components/common/layout';
import {DetailHost, useDetailNarrow} from '@/components/common/layout';
import {BatchBar, usePagedSelection} from '@/components/common/layout';
import styles from '../monitoring.module.css';
import layout from '@/components/common/layout/list-detail.module.css';

export type JobStatusFilter = 'all' | 'enabled' | 'disabled';

export interface JobSegCounts {
  all: number;
  enabled: number;
  disabled: number;
}

const PAGE_SIZE = 10;

function policyToggleBody(policy: AlertPolicy, enabled: boolean) {
  return {
    name: policy.name,
    description: policy.description,
    cluster_id: policy.cluster_id,
    policy_type: 'sync_job' as const,
    severity: policy.severity,
    enabled,
    cooldown_minutes: policy.cooldown_minutes,
    send_recovery: policy.send_recovery,
    conditions: policy.conditions || undefined,
    receiver_user_ids: policy.receiver_user_ids,
    notification_channel_ids: policy.notification_channel_ids,
    task_id: policy.task_id,
    notify_task_owner: policy.notify_task_owner,
  };
}

export function JobRulesPanel({
  statusFilter,
  query,
  onStatusFilterChange,
  onQueryChange,
  onCountsChange,
}: {
  // 筛选态由 RulesPanel 托管，筛选控件渲染在共享工具栏行内，与集群级一致。
  // Filter state is owned by RulesPanel so its controls render inside the
  // shared toolbar row, matching the cluster-level layout.
  statusFilter: JobStatusFilter;
  query: string;
  onStatusFilterChange: (v: JobStatusFilter) => void;
  onQueryChange: (v: string) => void;
  onCountsChange?: (c: JobSegCounts) => void;
}) {
  const t = useTranslations('monitoring');
  const ts = useTranslations('syncAlert');
  const narrow = useDetailNarrow();
  const [policies, setPolicies] = useState<AlertPolicy[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<number | null>(null);
  const [batchBusy, setBatchBusy] = useState(false);
  const [undo, setUndo] = useState<{
    enabled: boolean;
    policies: AlertPolicy[];
  } | null>(null);
  const [editing, setEditing] = useState<{
    taskId: number;
    taskName?: string;
    clusterId?: number | null;
    config: SyncTaskAlertConfig | null;
  } | null>(null);
  const [saving, setSaving] = useState(false);
  const [formDirty, setFormDirty] = useState(false);
  // 放弃后要跳转的目标：null = 无弹窗，'close' = 关闭，AlertPolicy = 切到另一行。
  // Pending discard target: null = dialog closed, 'close' = close, AlertPolicy = switch row.
  const [discardTarget, setDiscardTarget] = useState<
    AlertPolicy | 'close' | null
  >(null);
  // 名称飞入来源行：仅点击/回车打开时写入，J/K、头部 ↑↓ 与深链不写。
  // Fly-in origin row: written only on click/Enter open — never by J/K,
  // header arrows or deep links.
  const flyOriginRef = useRef<HTMLElement | null>(null);
  useBeforeUnload(formDirty && editing != null);

  // 打开动画期间保留最后一次编辑对象，抽屉滑出时表单不先消失。
  // Keep the last editing object mounted while the drawer slides out.
  const drawerEditing = useDrawerItem(editing);

  // dirty 时关闭（X / Esc / 遮罩 / 表单取消 / 切换行）先弹出确认框；确认放弃才落地。
  // A dirty close is intercepted with a confirm dialog; only discard actually closes.
  const requestClose = useCallback(() => {
    if (saving) {
      return;
    }
    if (formDirty) {
      setDiscardTarget('close');
      return;
    }
    setEditing(null);
    setDiscardTarget(null);
  }, [formDirty, saving]);

  const openPolicy = useCallback((policy: AlertPolicy) => {
    const taskId = policy.task_id;
    if (!taskId) {
      return;
    }
    setEditing({
      taskId,
      taskName: policy.task_name,
      clusterId: Number(policy.cluster_id) || null,
      config: null,
    });
    void services.sync
      .getTaskAlert(taskId)
      .then((config) =>
        setEditing((cur) => (cur?.taskId === taskId ? {...cur, config} : cur)),
      )
      .catch((err) => {
        setError(err instanceof Error ? err.message : String(err));
        setEditing(null);
      });
  }, []);

  const discardAndClose = useCallback(() => {
    const target = discardTarget;
    setDiscardTarget(null);
    setFormDirty(false);
    setEditing(null);
    if (target && target !== 'close') {
      openPolicy(target);
    }
  }, [discardTarget, openPolicy]);

  useEffect(() => {
    if (!formDirty) {
      setDiscardTarget(null);
    }
  }, [formDirty]);

  const load = useCallback(async () => {
    const [pol, ch, us] = await Promise.all([
      services.monitoring.listAlertPolicies({policy_type: 'sync_job'}),
      services.monitoring
        .listNotificationChannels()
        .catch(() => ({channels: []})),
      services.monitoring.listNotifiableUsers().catch(() => ({users: []})),
    ]);
    setPolicies(pol?.policies ?? []);
    setChannels(ch.channels ?? []);
    setUsers(us.users ?? []);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load().catch((err) => {
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    });
  }, [load]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return policies.filter((p) => {
      if (statusFilter === 'enabled' && !p.enabled) {
        return false;
      }
      if (statusFilter === 'disabled' && p.enabled) {
        return false;
      }
      const name = p.task_name ?? `#${p.task_id}`;
      if (q && !name.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
  }, [policies, statusFilter, query]);

  // 客户端分页保持列表区高度稳定。Client-side pagination keeps a stable height.
  const [page, setPage] = useState(1);
  useEffect(() => {
    setPage(1);
  }, [statusFilter, query]);
  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, totalPages);
  const pageItems = filtered.slice(
    (safePage - 1) * PAGE_SIZE,
    safePage * PAGE_SIZE,
  );
  const pageIds = useMemo(() => pageItems.map((p) => p.id), [pageItems]);
  const {
    ids: selected,
    setIds: setSelected,
    toggle: toggleSelect,
    togglePage: selectAllOnPage,
    pageAllSelected: allPageSelected,
    pageSomeSelected: somePageSelected,
  } = usePagedSelection(pageIds);

  const segCounts = useMemo(
    () => ({
      all: policies.length,
      enabled: policies.filter((p) => p.enabled).length,
      disabled: policies.filter((p) => !p.enabled).length,
    }),
    [policies],
  );

  // 段计数上报给共享工具栏。Counts feed the shared toolbar segments.
  useEffect(() => {
    onCountsChange?.(segCounts);
  }, [segCounts, onCountsChange]);

  const toggle = useCallback(
    async (policy: AlertPolicy, enabled: boolean) => {
      if (toggling != null) {
        return;
      }
      setToggling(policy.id);
      setError(null);
      const prev = policy.enabled;
      setPolicies((ps) =>
        ps.map((p) => (p.id === policy.id ? {...p, enabled} : p)),
      );
      try {
        const updated = await services.monitoring.updateAlertPolicy(
          policy.id,
          policyToggleBody(policy, enabled),
        );
        setPolicies((ps) =>
          ps.map((p) => (p.id === policy.id ? {...p, ...updated} : p)),
        );
      } catch (err) {
        setPolicies((ps) =>
          ps.map((p) => (p.id === policy.id ? {...p, enabled: prev} : p)),
        );
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setToggling(null);
      }
    },
    [toggling],
  );

  const batchSet = useCallback(
    async (enabled: boolean) => {
      const targets = policies.filter(
        (p) => selected.has(p.id) && p.enabled !== enabled,
      );
      if (targets.length === 0) {
        setSelected(new Set());
        return;
      }
      setBatchBusy(true);
      setError(null);
      try {
        for (const p of targets) {
          const updated = await services.monitoring.updateAlertPolicy(
            p.id,
            policyToggleBody(p, enabled),
          );
          setPolicies((ps) =>
            ps.map((x) => (x.id === p.id ? {...x, ...updated} : x)),
          );
        }
        setUndo({enabled, policies: targets});
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        await load().catch(() => undefined);
      } finally {
        setBatchBusy(false);
        setSelected(new Set());
      }
    },
    [policies, selected, load],
  );

  const undoBatch = useCallback(async () => {
    if (!undo) {
      return;
    }
    const items = undo.policies;
    setUndo(null);
    setBatchBusy(true);
    try {
      for (const p of items) {
        const updated = await services.monitoring.updateAlertPolicy(
          p.id,
          policyToggleBody(p, p.enabled),
        );
        setPolicies((ps) =>
          ps.map((x) => (x.id === p.id ? {...x, ...updated} : x)),
        );
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBatchBusy(false);
    }
  }, [undo]);

  // 撤销条约 5 秒自动消失，与集群级批量栏保持一致。Auto-dismiss undo bar after ~5s.
  useEffect(() => {
    if (!undo) {
      return;
    }
    const timer = setTimeout(() => setUndo(null), 5000);
    return () => clearTimeout(timer);
  }, [undo]);

  const openEditor = useCallback(
    (policy: AlertPolicy) => {
      if (!policy.task_id || policy.task_id === editing?.taskId) {
        return;
      }
      if (formDirty) {
        // 未保存草稿时切换行同样走放弃确认弹窗。Switching rows opens the discard dialog.
        setDiscardTarget(policy);
        return;
      }
      openPolicy(policy);
    },
    [editing?.taskId, formDirty, openPolicy],
  );

  const save = useCallback(
    async (request: SyncTaskAlertRequest) => {
      if (!editing) {
        return;
      }
      setSaving(true);
      try {
        await services.sync.updateTaskAlert(editing.taskId, request);
        setFormDirty(false);
        setEditing(null);
        await load();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setSaving(false);
      }
    },
    [editing, load],
  );

  const remove = useCallback(async () => {
    if (!editing) {
      return;
    }
    setSaving(true);
    try {
      await services.sync.deleteTaskAlert(editing.taskId);
      setFormDirty(false);
      setEditing(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }, [editing, load]);

  const onRowKeyDown = (
    e: React.KeyboardEvent<HTMLElement>,
    policy: AlertPolicy,
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
      if (policy.task_id && policy.task_id !== editing?.taskId) {
        flyOriginRef.current = e.currentTarget;
      }
      openEditor(policy);
    }
  };

  const activeIndex = editing
    ? pageItems.findIndex((p) => p.task_id === editing.taskId)
    : -1;
  const detailNav =
    editing && activeIndex >= 0
      ? {
          index: activeIndex,
          total: pageItems.length,
          onPrev: () =>
            activeIndex > 0 && openEditor(pageItems[activeIndex - 1]),
          onNext: () =>
            activeIndex < pageItems.length - 1 &&
            openEditor(pageItems[activeIndex + 1]),
        }
      : undefined;

  return (
    <div className={layout.split}>
      <div className={layout.listCol} data-mcols='job'>
        {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）。
            Single main card; skeleton/error/empty states are owned by TableCard. */}
        <TableCard
          loading={loading}
          hasData={filtered.length > 0}
          skeletonColumns={4}
          skeletonRows={6}
          error={error}
          onRetry={() => {
            setError(null);
            setLoading(true);
            void load().catch((err) => {
              setError(err instanceof Error ? err.message : String(err));
              setLoading(false);
            });
          }}
          pagination={{
            currentPage: safePage,
            totalPages,
            pageSize: PAGE_SIZE,
            totalItems: filtered.length,
            onPageChange: setPage,
          }}
          empty={
            <EmptyState
              title={policies.length === 0 ? t('jobEmpty') : t('emptyFiltered')}
              description={
                policies.length === 0
                  ? t('jobRulesEmptyDesc')
                  : t('emptyFilteredDesc')
              }
              action={
                statusFilter !== 'all' || query ? (
                  <Button
                    type='button'
                    size='sm'
                    variant='secondary'
                    onClick={() => {
                      onStatusFilterChange('all');
                      onQueryChange('');
                    }}
                  >
                    {t('clearFilters')}
                  </Button>
                ) : undefined
              }
            />
          }
        >
          <div className={layout.rowHead}>
            <Checkbox
              checked={
                allPageSelected
                  ? true
                  : somePageSelected
                    ? 'indeterminate'
                    : false
              }
              onCheckedChange={(v) => selectAllOnPage(v === true)}
              aria-label={t('selectAll')}
            />
            <span>{t('jobColTask')}</span>
            <span>{t('colAlert')}</span>
            <span className={layout.right}>{t('fieldEnabled')}</span>
          </div>
          <div className={layout.tableScroll}>
            {pageItems.map((policy) => (
              <div
                key={policy.id}
                data-testid={`job-rule-row-${policy.id}`}
                data-mrow={policy.id}
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
                  if (policy.task_id && policy.task_id !== editing?.taskId) {
                    flyOriginRef.current = e.currentTarget;
                  }
                  openEditor(policy);
                }}
                onKeyDown={(e) => onRowKeyDown(e, policy)}
                className={cn(
                  layout.row,
                  editing?.taskId === policy.task_id && layout.rowActive,
                  !policy.enabled && layout.rowMuted,
                )}
              >
                <Checkbox
                  checked={selected.has(policy.id)}
                  onCheckedChange={(v) => toggleSelect(policy.id, v === true)}
                  aria-label={t('selectRule', {
                    name: policy.task_name ?? `#${policy.task_id}`,
                  })}
                />
                <div className={layout.rowName}>
                  <TruncatedText
                    text={policy.task_name ?? `#${policy.task_id}`}
                    className={cn(layout.name, styles.mono)}
                    detailName
                  />
                </div>
                <span className={cn(styles.inlineBadgeRow, 'min-w-0')}>
                  {(policy.conditions ?? []).slice(0, 3).map((c, i) => (
                    <NeutralChip key={i}>
                      {ts.has?.(`rule_${c.metric_key}`)
                        ? ts(`rule_${c.metric_key}`)
                        : c.metric_key}
                    </NeutralChip>
                  ))}
                  <SoftChip
                    tone={
                      policy.notification_channel_ids?.length ||
                      policy.receiver_user_ids?.length
                        ? 'blue'
                        : 'muted'
                    }
                  >
                    {policy.notification_channel_ids?.length ||
                    policy.receiver_user_ids?.length
                      ? t('notify_custom')
                      : t('notify_none')}
                  </SoftChip>
                </span>
                <div className={layout.rowActions}>
                  <Switch
                    checked={policy.enabled}
                    disabled={toggling === policy.id}
                    onCheckedChange={(on) => void toggle(policy, on)}
                    onClick={(e) => e.stopPropagation()}
                    aria-label={t('fieldEnabled')}
                  />
                </div>
              </div>
            ))}
          </div>
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
              {count: undo.policies.length},
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
        open={editing != null}
        hasContent={drawerEditing != null}
        onOpenChange={(o) => !o && requestClose()}
        narrow={narrow}
        title={ts('dialogTitle')}
        meta={drawerEditing?.taskName}
        nav={detailNav}
        wide
        drawerWidth='w-[min(92vw,560px)]'
        flyOrigin={flyOriginRef}
        nameSlot='meta'
      >
        {drawerEditing ? (
          drawerEditing.config ? (
            <div className={layout.detailCol}>
              <div className={layout.detailFill}>
                <TaskAlertForm
                  initial={drawerEditing.config}
                  clusterId={drawerEditing.clusterId}
                  channels={channels}
                  users={users}
                  saving={saving}
                  pinnedFooter
                  onSubmit={(request) => void save(request)}
                  onCancel={requestClose}
                  onDelete={() => void remove()}
                  onDirtyChange={setFormDirty}
                />
              </div>
            </div>
          ) : (
            <div className={layout.detailCol}>
              <div className={layout.detailScroll} aria-busy>
                {Array.from({length: 6}).map((_, i) => (
                  <Skeleton key={i} className='h-10 w-full' lines={1} />
                ))}
              </div>
            </div>
          )
        ) : null}
      </DetailHost>
      <DirtyCloseDialog
        open={discardTarget !== null}
        onDiscard={discardAndClose}
        onKeepEditing={() => setDiscardTarget(null)}
      />
    </div>
  );
}
